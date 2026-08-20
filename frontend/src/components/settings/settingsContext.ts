import type { Ref, ComputedRef } from 'vue'
import type { Config, SearchAPIKeys } from '../../services/wails'
import type { ModelRefConfig, ModelRefRaw } from '../../utils/modelRefs'
import type { UploadCustomRequestOptions } from 'naive-ui'

export interface SettingsContext {
  // 核心响应式状态
  formConfig: Ref<Config>
  autoSave: () => Promise<void>
  genParamsDirty: Ref<boolean>

  // 外观相关
  backgroundImageUrl: ComputedRef<string>
  selectBackgroundImage: () => Promise<void>
  clearBackground: () => void
  handleAvatarUpload: (
    data: UploadCustomRequestOptions,
    fieldName: 'user_avatar' | 'ai_avatar'
  ) => Promise<void>
  clearUserAvatar: () => void
  clearAIAvatar: () => void
  defaultUserAvatar: string
  defaultAiAvatar: string

  // 推理相关
  supportsReasoning: ComputedRef<boolean>

  // 模型参考参数
  currentModelRef: ComputedRef<ModelRefConfig | null>
  activeModelRefRaw: ComputedRef<ModelRefRaw>
  refShowThinking: Ref<boolean>
  applyModelRef: () => void

  // 模型专属参数预设（每模型保存各自的生成参数，切换时自动恢复）
  hasModelPreset: Ref<boolean>
  saveModelPreset: () => Promise<void>
  clearModelPreset: () => Promise<void>
  savingModelPreset: Ref<boolean>

  // 上下文长度
  contextSizeIndex: Ref<number>
  contextSizeSteps: number[]
  contextSizeMarks: Record<number, string>
  formatContextSize: (size: number) => string
  applyContextSizeRef: () => void

  // API Key 相关
  newOllamaApiKey: Ref<string>
  newTavilyApiKey: Ref<string>
  searchKeys: Ref<SearchAPIKeys>
  saveSearchKeys: () => void
  savingSearchKeys: Ref<boolean>
  hasServerApiKey: Ref<boolean>
  generateServerApiKey: () => Promise<void>
  savingServerApiKey: Ref<boolean>
  // 一次性展示的生成结果（仅生成时由后端返回明文，关闭展示即丢弃，后端不再提供查看接口）
  generatedServerApiKey: Ref<string>
  copyGeneratedApiKey: () => void
  dismissGeneratedApiKey: () => void
  onServerAPIKeyToggle: () => Promise<void>
  onExposeServerToggle: () => Promise<void>
  onEnableWebUIToggle: () => Promise<void>

  // 高级设置选项
  cacheTypeKOptions: ComputedRef<{ label: string; value: string }[]>
  cacheTypeVOptions: ComputedRef<{ label: string; value: string }[]>
  specTypeOptions: ComputedRef<{ label: string; value: string }[]>

  // 实验设置
  handleAgentChange: () => void
  handleBackendSamplingChange: () => void

  // Store（pinia store 类型过于复杂，保留 any；实际访问的属性均有类型保障）
  settingsStore: any
}

export const SETTINGS_CONTEXT_KEY = Symbol('settingsContext')
