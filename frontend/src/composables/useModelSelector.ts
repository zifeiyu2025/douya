/**
 * useModelSelector —— 模型选择器的数据层单例
 *
 * 从旧 AppHeader 中抽取的模型逻辑：列表加载、选中同步、状态派生、
 * 切换（含回滚）、删除（含二次确认）。做成模块级单例的原因：
 * 命令条（TopCommandBar）与命令面板（CommandPalette）都要消费同一份
 * 模型列表与服务状态，单例保证只发一次请求、只挂一套 Wails 监听。
 *
 * 注意：内部全部是对 settings store 的纯派生 + discrete API 提示，
 * 不含 inject 依赖，可在任意组件 setup 中安全调用。
 */
import { computed, ref, watch } from 'vue'
import { useSettingsStore } from '../stores/settings'
import {
  formatModelName,
  formatModelNameFromPath,
  extractQuantSuffix,
  isEmbeddingModelName
} from '../utils/model'
import { classifyError } from '../utils/errorGuidance'
import { logError } from '../utils/logger'
import { discreteMessage, discreteDialog } from '../utils/discrete'
import type { ModelOption } from '../services/wails'
import { wails } from '../services/wails'

/** 下拉项的视图模型：在原始 ModelOption 上补充分类好的展示字段 */
export interface ModelOptionView {
  label: string
  value: string
  fullName: string
  /** 从文件名猜测的量化后缀（GGUF 解析失败时的回退） */
  quantSuffix: string
  /** GGUF 解析出的真实量化类型名，可能为空 */
  quantType?: string
  /** GGUF 参数量规模标签（如 "4B"），可能为空 */
  sizeLabel?: string
  isLoaded: boolean
  mmprojVision: boolean
  mmprojAudio: boolean
  mmprojVideo: boolean
  status: string
}

function createModelSelector() {
  const settingsStore = useSettingsStore()

  // ===================== 列表与选中态 =====================
  const modelOptions = ref<ModelOptionView[]>([])
  const availableModels = ref<ModelOption[]>([])
  const selectedModel = ref('')

  /** 下拉为空时用不可选项占位，引导用户放入 .gguf 文件，避免首启"死路" */
  const displayModelOptions = computed(() => {
    if (modelOptions.value.length === 0) {
      return [
        {
          label: '未找到模型，请将 .gguf 文件放入 models/ 目录',
          value: '__no_models__',
          disabled: true
        }
      ]
    }
    return modelOptions.value
  })

  /** 当前选中模型的完整信息（详情卡数据源；找不到时为 null 由卡片显示空态） */
  const currentModelDetail = computed<ModelOption | null>(() => {
    const found = availableModels.value.find(m => m.name === selectedModel.value)
    return found ?? null
  })

  /** 状态区展示名：优先下拉选中值，其次配置里的路径反解 */
  const modelName = computed(() => {
    if (selectedModel.value) {
      return formatModelName(selectedModel.value).display
    }
    const path = settingsStore.config.model_path
    if (!path) return ''
    return formatModelNameFromPath(path).display
  })

  // ===================== 服务状态派生（纯 computed） =====================
  const serverStatus = computed(() => settingsStore.serverStatus)
  const switchProgressStage = computed(() => settingsStore.switchProgress.stage)
  const isModelSwitching = computed(() => settingsStore.isModelSwitching)
  const switchingModelDisplay = computed(() => settingsStore.switchingModelDisplay)

  const switchingModelName = computed(() => {
    if (switchingModelDisplay.value) return switchingModelDisplay.value
    if (settingsStore.serverStatus.switching_to) {
      return formatModelName(settingsStore.serverStatus.switching_to).display
    }
    return ''
  })

  const isServerLoading = computed(() => {
    return (
      serverStatus.value.switching || (!serverStatus.value.model_ready && !serverStatus.value.error)
    )
  })
  const isFirstLoad = computed(() => settingsStore.isFirstLoad)
  const modelLoadFailed = computed(() => settingsStore.modelLoadFailed)
  const modelLoadProgress = computed(() => settingsStore.modelLoadProgress)

  const loadProgressModelName = computed(() => {
    if (!modelLoadProgress.value) return ''
    return formatModelName(modelLoadProgress.value.model).display
  })

  const errorModelName = computed(() => {
    if (switchingModelDisplay.value) return switchingModelDisplay.value
    if (serverStatus.value.switching_to) {
      return formatModelName(serverStatus.value.switching_to).display
    }
    return modelName.value || ''
  })

  // ===================== 加载与防抖刷新 =====================
  async function loadAvailableModels() {
    try {
      const models = await wails.getAvailableModels()
      availableModels.value = models
      modelOptions.value = models.map(m => {
        const { display } = formatModelName(m.name)
        return {
          label: display,
          value: m.name,
          fullName: m.file_name || m.name,
          quantSuffix: extractQuantSuffix(m.file_name || ''),
          quantType: m.quant_type || undefined,
          sizeLabel: m.size_label || undefined,
          isLoaded: m.is_loaded,
          mmprojVision: m.mmproj_vision,
          mmprojAudio: m.mmproj_audio,
          mmprojVideo: m.mmproj_video,
          status: m.status
        }
      })

      const defaultModel = models.find(m => m.is_default)
      if (defaultModel && !selectedModel.value) {
        selectedModel.value = defaultModel.name
      }
    } catch (e) {
      logError('Failed to load available models:', e)
    }
  }

  // 防抖：running 与 model_ready 几乎同时变化时避免重复拉取
  let refreshModelsTimer: ReturnType<typeof setTimeout> | null = null
  function debouncedRefreshModels() {
    if (refreshModelsTimer) clearTimeout(refreshModelsTimer)
    refreshModelsTimer = setTimeout(() => {
      loadAvailableModels()
      refreshModelsTimer = null
    }, 500)
  }

  // 服务从停止变为运行时刷新列表
  watch(
    () => serverStatus.value.running,
    (running, prev) => {
      if (running && !prev) debouncedRefreshModels()
    }
  )
  // 模型就绪时刷新以更新各模型的 is_loaded 标记
  watch(
    () => serverStatus.value.model_ready,
    ready => {
      if (ready) debouncedRefreshModels()
    }
  )

  // 后端当前模型变化 → 同步下拉选中项（切换进行中不抢跑）
  watch(
    () => settingsStore.currentModel,
    newModel => {
      if (isModelSwitching.value) return
      if (newModel && newModel !== selectedModel.value) {
        const match = modelOptions.value.find(m => m.value === newModel)
        if (match) selectedModel.value = newModel
      }
    }
  )

  // ===================== 切换 / 删除 =====================
  /** 切换失败的统一展示出口：有修复指引走详情对话框，否则走消息条 */
  function showSwitchError(errorText: string) {
    const guidance = classifyError(errorText)
    if (guidance) {
      const suggestions = guidance.suggestions.map((s, i) => `${i + 1}. ${s}`).join('\n')
      discreteDialog.error({
        title: guidance.title,
        content: `${guidance.description}\n\n修复建议：\n${suggestions}`,
        positiveText: '知道了',
        style: { whiteSpace: 'pre-wrap' }
      })
    } else {
      discreteMessage.error(errorText, { duration: 5000 })
    }
  }

  /** 切换到指定模型；成功提示、失败回滚与错误展示全部内聚于此 */
  async function switchToModel(value: string) {
    if (isModelSwitching.value) return

    const targetModel = availableModels.value.find(m => m.name === value)
    if (!targetModel) {
      logError('Unknown model:', value)
      return
    }

    const previousModel = settingsStore.currentModel || selectedModel.value
    selectedModel.value = value

    try {
      const result = await settingsStore.switchModel(value, previousModel)
      if (result.success) {
        selectedModel.value = result.current_model || value
        // 等 0.3s 过渡动画结束后再提示，避免与遮罩动画抢帧
        setTimeout(() => {
          const caps = result.capabilities || settingsStore.modelCapabilities
          const features: string[] = []
          if (caps.image_input) features.push('图片')
          if (caps.audio_input) features.push('音频')
          if (caps.reasoning) features.push('推理')
          const featureText = features.length > 0 ? ` · 支持${features.join('、')}` : ' · 仅文本'
          const restoredText = result.params_restored ? ' · 已恢复专属参数' : ''
          // 嵌入模型（如 bge-m3）不能聊天：切换成功但给出明确提醒，
          // 让用户在加载完成前就意识到该模型只能用于检索，避免误发消息报错
          const isEmbeddingOnly =
            caps.text_generation === false || isEmbeddingModelName(result.current_model || value)
          if (isEmbeddingOnly) {
            discreteDialog.warning({
              title: '已切换到嵌入模型',
              content: `「${formatModelName(result.current_model || value).display}」是嵌入模型，只能做文本向量化/检索（如知识库问答），不能进行对话回复。\n\n如需聊天，请切换回对话类模型。`,
              positiveText: '知道了',
              style: { whiteSpace: 'pre-wrap' }
            })
          } else {
            discreteMessage.success(
              `${formatModelName(result.current_model || value).display}${featureText}${restoredText} 已就绪`,
              { duration: 3000 }
            )
          }
          loadAvailableModels()
        }, 300)
      } else {
        selectedModel.value = result.rolled_back
          ? result.current_model || previousModel
          : previousModel
        setTimeout(() => showSwitchError(result.error || '模型加载失败'), 300)
      }
    } catch (e) {
      logError('切换模型失败:', e)
      selectedModel.value = previousModel
      setTimeout(() => showSwitchError(`切换模型失败: ${e}`), 300)
    }
  }

  // 单例创建即拉取一次列表（仅调用 wails.getAvailableModels，不依赖 config）
  void loadAvailableModels()

  return {
    // 列表与选中
    modelOptions,
    availableModels,
    selectedModel,
    displayModelOptions,
    currentModelDetail,
    modelName,
    // 状态派生
    serverStatus,
    switchProgressStage,
    isModelSwitching,
    switchingModelDisplay,
    switchingModelName,
    isServerLoading,
    isFirstLoad,
    modelLoadFailed,
    modelLoadProgress,
    loadProgressModelName,
    errorModelName,
    // 行为
    switchToModel,
    refreshModels: loadAvailableModels
  }
}

type ModelSelectorApi = ReturnType<typeof createModelSelector>

// 模块级单例：全应用共享一份模型数据与监听
let singleton: ModelSelectorApi | null = null

export function useModelSelector(): ModelSelectorApi {
  if (!singleton) singleton = createModelSelector()
  return singleton
}
