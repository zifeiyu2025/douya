import type { Ref, ComputedRef } from 'vue'
import type { Config, SearchAPIKeys } from '../../services/wails'

export interface SettingsContext {
  // 核心响应式状态
  formConfig: Ref<Config>
  autoSave: () => Promise<void>
  genParamsDirty: Ref<boolean>

  // 外观相关
  backgroundImageUrl: ComputedRef<string>
  selectBackgroundImage: () => Promise<void>
  clearBackground: () => void
  handleUserAvatarUpload: (data: any) => Promise<void>
  clearUserAvatar: () => void
  handleAIAvatarUpload: (data: any) => Promise<void>
  clearAIAvatar: () => void
  defaultUserAvatar: string
  defaultAiAvatar: string

  // 推理相关
  reasoningOptions: { label: string; value: string }[]
  supportsReasoning: ComputedRef<boolean>

  // 模型参考参数
  currentModelRef: ComputedRef<any>
  activeModelRefRaw: ComputedRef<any>
  refShowThinking: Ref<boolean>
  applyModelRef: () => void

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
  serverApiKey: Ref<string>
  hasServerApiKey: Ref<boolean>
  saveServerApiKey: () => void
  onServerAPIKeyToggle: () => Promise<void>
  onExposeServerToggle: () => Promise<void>

  // 高级设置选项
  cacheTypeKOptions: ComputedRef<{ label: string; value: string }[]>
  cacheTypeVOptions: ComputedRef<{ label: string; value: string }[]>
  specTypeOptions: ComputedRef<{ label: string; value: string }[]>

  // 实验设置
  handleAgentChange: () => void
  handleBackendSamplingChange: () => void

  // Store
  settingsStore: any
}

export const SETTINGS_CONTEXT_KEY = Symbol('settingsContext')
