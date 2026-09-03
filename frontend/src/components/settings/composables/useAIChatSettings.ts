import { ref, computed } from 'vue'
import type { MessageApi } from 'naive-ui'
import { useSettingsStore, matchModelRef } from '../../../stores/settings'
import { MODEL_REFS } from '../../../utils/modelRefs'
import { showSuccess } from '../../../utils/showError'
import { logError } from '../../../utils/logger'
import { DEFAULT_CONFIG } from '../../../services/wails'
import { wails } from '../../../services/wails'
import { contextSizeSteps, findClosestStepIndex } from '../../../utils/contextSize'
import type { SettingsCore } from './useSettingsCore'
import type { PerformanceSettingsApi } from './usePerformanceSettings'

/**
 * AI 对话域：模型参考参数、推理支持、上下文推荐、模型专属参数预设。
 * 自 SettingsView 迁出，方法体逐字保留。
 * 依赖链：core ← performance ← aiChat（applyModelRef 需要写入 performance.contextSizeIndex）
 */
export function useAIChatSettings(
  core: SettingsCore,
  performance: PerformanceSettingsApi,
  message: MessageApi
) {
  const settingsStore = useSettingsStore()
  const { formConfig, autoSave } = core

  const supportsReasoning = computed(() => settingsStore.modelCapabilities.reasoning)

  const refShowThinking = ref(false)

  const currentModelRef = computed(() => {
    return matchModelRef(settingsStore.currentModel, MODEL_REFS)
  })

  const activeModelRefRaw = computed(() => {
    const ref = currentModelRef.value
    if (!ref) {
      // 无推荐参数时回退到全局默认采样参数（与 DEFAULT_CONFIG 一致，避免"点推荐 vs 默认"行为差异）
      return {
        temperature: DEFAULT_CONFIG.temperature,
        top_p: DEFAULT_CONFIG.top_p,
        top_k: DEFAULT_CONFIG.top_k,
        context_size: DEFAULT_CONFIG.context_size,
        repeat_penalty: DEFAULT_CONFIG.repeat_penalty
      }
    }
    const useThinking = settingsStore.thinkingEnabled && ref.raw_thinking
    return useThinking ? ref.raw_thinking! : ref.raw
  })

  function applyModelRef() {
    const ref = currentModelRef.value
    if (!ref) return
    const useThinking = settingsStore.thinkingEnabled && ref.raw_thinking
    const raw = useThinking ? ref.raw_thinking! : ref.raw
    formConfig.value.temperature = raw.temperature
    formConfig.value.top_p = raw.top_p
    formConfig.value.top_k = raw.top_k
    formConfig.value.repeat_penalty = raw.repeat_penalty
    const idx = findClosestStepIndex(raw.context_size)
    performance.contextSizeIndex.value = idx
    formConfig.value.context_size = contextSizeSteps[idx]
    const modeLabel = useThinking ? '深度思考' : '快速回答'
    showSuccess(message, `已应用 ${ref.name} ${modeLabel}参考参数`)
  }

  // 将当前模型的推荐上下文长度同步到滑块（AIChatSettings 面板使用）
  function applyContextSizeRef() {
    const raw = activeModelRefRaw.value
    const idx = findClosestStepIndex(raw.context_size)
    performance.contextSizeIndex.value = idx
    formConfig.value.context_size = contextSizeSteps[idx]
  }

  // ===== 模型专属参数预设 =====
  // 每个模型保存各自的生成参数，切换模型时自动恢复用户习惯
  const hasModelPreset = ref(false)
  const savingModelPreset = ref(false)

  // 加载当前模型的预设状态（切换模型后调用）
  async function loadModelPresetStatus() {
    const model = settingsStore.currentModel
    if (!model) {
      hasModelPreset.value = false
      return
    }
    try {
      hasModelPreset.value = await wails.hasModelParams(model)
    } catch {
      hasModelPreset.value = false
    }
  }

  // 保存当前参数为该模型的预设
  async function saveModelPreset() {
    const model = settingsStore.currentModel
    if (!model) {
      message.warning('请先加载模型再保存预设')
      return
    }
    savingModelPreset.value = true
    try {
      // 先保存当前配置到后端，确保保存的是最新参数
      await autoSave()
      await wails.saveModelParams(model)
      hasModelPreset.value = true
      showSuccess(message, `已保存 ${model} 的参数预设，下次切换到此模型将自动恢复`)
    } catch (e) {
      logError('保存模型预设失败', e)
      message.error('保存模型预设失败')
    } finally {
      savingModelPreset.value = false
    }
  }

  // 清除该模型的预设
  async function clearModelPreset() {
    const model = settingsStore.currentModel
    if (!model) return
    savingModelPreset.value = true
    try {
      await wails.clearModelParams(model)
      hasModelPreset.value = false
      showSuccess(message, `已清除 ${model} 的参数预设，切换到此模型将使用全局默认参数`)
    } catch (e) {
      logError('清除模型预设失败', e)
      message.error('清除模型预设失败')
    } finally {
      savingModelPreset.value = false
    }
  }

  // 模型切换响应（原 currentModel watch 的 AI 对话部分，顺序保持在 performance 之后）
  core.onModelSwitch(async inCleanBlock => {
    if (inCleanBlock) {
      if (currentModelRef.value) {
        applyModelRef()
      }
      // 如果当前 spec_type 为 draft-mtp 但模型不支持 MTP，自动重置为空（自动检测）
      if (formConfig.value.spec_type === 'draft-mtp' && !settingsStore.modelCapabilities.has_mtp) {
        formConfig.value.spec_type = ''
      }
      // 非推理模型：自动重置 reasoning 为 off
      if (!settingsStore.modelCapabilities.reasoning && formConfig.value.reasoning !== 'off') {
        formConfig.value.reasoning = 'off'
        formConfig.value.reasoning_budget = -1
      }
    }
    // 加载新模型的预设状态（显示"已保存/未保存"标记）
    loadModelPresetStatus()
  })

  /** 初始化：加载当前模型的预设状态 */
  async function init() {
    await loadModelPresetStatus()
  }

  return {
    supportsReasoning,
    currentModelRef,
    activeModelRefRaw,
    refShowThinking,
    applyModelRef,
    applyContextSizeRef,
    hasModelPreset,
    savingModelPreset,
    saveModelPreset,
    clearModelPreset,
    init
  }
}
