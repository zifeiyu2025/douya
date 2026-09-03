import { ref, computed, watch } from 'vue'
import { useSettingsStore } from '../../../stores/settings'
import { wails } from '../../../services/wails'
import { contextSizeSteps, findClosestStepIndex } from '../../../utils/contextSize'
import type { SettingsCore } from './useSettingsCore'

/**
 * 性能域：GPU 检测、上下文长度滑块、KV 缓存类型选项、推测解码选项。
 * 自 SettingsView 迁出，方法体逐字保留。
 */
export function usePerformanceSettings(core: SettingsCore) {
  const settingsStore = useSettingsStore()
  const { formConfig } = core

  // GPU 检测结果：默认 true（显示全部选项），init 时通过 getBackendStatus 更新
  const hasGPUInfo = ref(true)

  const contextSizeIndex = ref(2)

  watch(contextSizeIndex, idx => {
    formConfig.value.context_size = contextSizeSteps[idx]
  })

  // KV cache 类型选项：K/V 共用同一份可选值（GPU 可用时多出 bf16 / iq4_nl）
  const cacheTypeOptions = computed(() => {
    const hasGPU = hasGPUInfo.value
    const baseOptions = [
      { label: '自动', value: '' },
      { label: 'f32 (32bit)', value: 'f32' },
      { label: 'f16 (16bit)', value: 'f16' },
      { label: 'q8_0 (8bit)', value: 'q8_0' },
      { label: 'q5_1 (5bit)', value: 'q5_1' },
      { label: 'q5_0 (5bit)', value: 'q5_0' },
      { label: 'q4_1 (4bit)', value: 'q4_1' },
      { label: 'q4_0 (4bit)', value: 'q4_0' }
    ]
    if (hasGPU) {
      // GPU 模式：在 f16 后插入 bf16，在 q4_0 后追加 iq4_nl
      baseOptions.splice(3, 0, { label: 'bf16 (16bit)', value: 'bf16' })
      baseOptions.push({ label: 'iq4_nl (4bit)', value: 'iq4_nl' })
    }
    return baseOptions
  })

  const specTypeOptions = computed(() => {
    const caps = settingsStore.modelCapabilities
    const options = [{ label: '自动检测', value: '' }]
    // 仅当模型支持 MTP 时才显示 draft-mtp 选项
    if (caps.has_mtp) {
      options.push({ label: 'MTP 推测解码 🔥', value: 'draft-mtp' })
    }
    options.push(
      { label: 'Eagle3 推测解码', value: 'draft-eagle3' },
      { label: 'DFlash 推测解码', value: 'draft-dflash' },
      { label: 'Draft-Simple 推测解码', value: 'draft-simple' },
      { label: 'DSpark 推测解码', value: 'draft-dspark' },
      { label: 'Ngram-Mod 推测解码', value: 'ngram-mod' },
      { label: 'Ngram-Simple 推测解码', value: 'ngram-simple' },
      { label: 'Ngram-Map-K 推测解码', value: 'ngram-map-k' },
      { label: 'Ngram-Map-K4V 推测解码', value: 'ngram-map-k4v' },
      { label: 'Ngram-Cache 推测解码', value: 'ngram-cache' },
      { label: '关闭', value: 'none' }
    )
    return options
  })

  // GPU 状态变化时，自动重置不兼容的 KV cache 类型选中值（bf16/iq4_nl 仅 GPU 可用）
  watch(hasGPUInfo, hasGPU => {
    if (!hasGPU) {
      const kVal = formConfig.value.cache_type_k
      const vVal = formConfig.value.cache_type_v
      if (kVal === 'bf16' || kVal === 'iq4_nl') {
        formConfig.value.cache_type_k = ''
      }
      if (vVal === 'bf16' || vVal === 'iq4_nl') {
        formConfig.value.cache_type_v = ''
      }
    }
  })

  // 模型切换后同步滑块到新配置的上下文长度（仅无未保存修改时执行，保持原行为）
  core.onModelSwitch(inCleanBlock => {
    if (inCleanBlock) {
      contextSizeIndex.value = findClosestStepIndex(formConfig.value.context_size)
    }
  })

  /** 初始化：同步滑块初始位置 + 获取硬件信息判断是否有 GPU */
  async function init() {
    contextSizeIndex.value = findClosestStepIndex(formConfig.value.context_size)
    // 获取硬件信息以判断是否有 GPU（影响 KV cache 类型可选项），改用 getBackendStatus
    try {
      const status = await wails.getBackendStatus()
      hasGPUInfo.value = !!status.gpu_name || status.gpu_vram_mb > 0 || status.gpu_vendor !== ''
    } catch {
      // 获取失败时保持默认值（true），显示全部选项
    }
  }

  return {
    hasGPUInfo,
    contextSizeIndex,
    cacheTypeOptions,
    specTypeOptions,
    init
  }
}

export type PerformanceSettingsApi = ReturnType<typeof usePerformanceSettings>
