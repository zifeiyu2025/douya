/**
 * Wails 服务门面 - 模型域
 * 模型列表/切换/删除/下载/LoRA/专属参数/模型市场（ModelScope / HF 镜像）
 * （从原 wails.ts 迁移,方法体逐字搬移,逻辑零变化）
 */
import {
  GetAvailableModels,
  SwitchModel,
  ReloadModels,
  DeleteModel,
  DownloadModel,
  RerankEnabled,
  GetLoraAdapters,
  SetLoraAdapters,
  SelectLoraFile,
  SaveModelParams,
  ClearModelParams,
  HasModelParams,
  SearchHubModels,
  ListHubModelFiles,
  DownloadHubModel
} from '../../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime'
import {
  EventServerSwitchProgress,
  EventModelLoadProgress,
  EventServerMmprojUnavailable,
  EventSearchAutoDisabled,
  EventStartupModelNotice,
  EventModelDownloadProgress,
  EventModelDownloadComplete
} from '../events'
import type { ModelOption, SwitchResult } from '../../types/chat'
import type {
  HubFile,
  HubModel,
  LoraAdapter,
  ModelDownloadComplete,
  ModelDownloadProgress,
  ModelLoadProgressEvent,
  SwitchProgressEvent
} from './types'
import { toWailsLoraAdapters } from './adapters'

export const modelMethods = {
  getAvailableModels: async (): Promise<ModelOption[]> => {
    return (await GetAvailableModels()) as ModelOption[]
  },
  switchModel: async (modelName: string): Promise<SwitchResult> => {
    return (await SwitchModel(modelName)) as SwitchResult
  },
  reloadModels: async (): Promise<void> => {
    await ReloadModels()
  },
  deleteModel: async (modelName: string): Promise<void> => {
    await DeleteModel(modelName)
  },
  downloadModel: async (modelName: string): Promise<void> => {
    await DownloadModel(modelName)
  },
  rerankEnabled: async (): Promise<boolean> => {
    return await RerankEnabled()
  },
  getLoraAdapters: async (): Promise<LoraAdapter[]> => {
    return (await GetLoraAdapters()) as LoraAdapter[]
  },
  setLoraAdapters: async (adapters: LoraAdapter[]): Promise<void> => {
    await SetLoraAdapters(toWailsLoraAdapters(adapters))
  },
  selectLoraFile: async (): Promise<string> => {
    return await SelectLoraFile()
  },
  // 模型专属生成参数：每个模型保存各自的生成参数，切换时自动恢复
  saveModelParams: async (modelName: string): Promise<void> => {
    await SaveModelParams(modelName)
  },
  clearModelParams: async (modelName: string): Promise<void> => {
    await ClearModelParams(modelName)
  },
  hasModelParams: async (modelName: string): Promise<boolean> => {
    return await HasModelParams(modelName)
  },
  subscribeSwitchProgress: (callback: (progress: SwitchProgressEvent) => void): (() => void) => {
    EventsOn(EventServerSwitchProgress, callback)
    return () => EventsOff(EventServerSwitchProgress)
  },
  subscribeModelLoadProgress: (
    callback: (progress: ModelLoadProgressEvent) => void
  ): (() => void) => {
    EventsOn(EventModelLoadProgress, callback)
    return () => EventsOff(EventModelLoadProgress)
  },
  subscribeMmprojUnavailable: (callback: () => void): (() => void) => {
    EventsOn(EventServerMmprojUnavailable, callback)
    return () => EventsOff(EventServerMmprojUnavailable)
  },
  subscribeSearchAutoDisabled: (callback: () => void): (() => void) => {
    EventsOn(EventSearchAutoDisabled, callback)
    return () => EventsOff(EventSearchAutoDisabled)
  },
  // 无可用模型：非阻塞提示"如何下载模型"的引导文案
  subscribeModelNotice: (callback: (data: { message: string }) => void): (() => void) => {
    EventsOn(EventStartupModelNotice, callback)
    return () => EventsOff(EventStartupModelNotice)
  },
  // ============ 模型下载（内置下载器，来源 ModelScope / HF 镜像） ============
  // 生活类比：像"网购模型"——在下载源上搜索（第 page 页，从 1 起）、挑仓库、选文件，然后快递到家（models 目录）。
  searchHubModels: async (provider: string, query: string, page = 1): Promise<HubModel[]> => {
    return (await SearchHubModels(provider, query, page)) as HubModel[]
  },
  listHubModelFiles: async (provider: string, repoID: string): Promise<HubFile[]> => {
    return (await ListHubModelFiles(provider, repoID)) as HubFile[]
  },
  downloadHubModel: async (
    provider: string,
    repoID: string,
    mainFile: string,
    mmprojFile: string
  ): Promise<void> => {
    await DownloadHubModel(provider, repoID, mainFile, mmprojFile ?? '')
  },
  // 监听模型下载进度事件：下载过程中实时推送进度
  subscribeModelDownloadProgress: (
    callback: (progress: ModelDownloadProgress) => void
  ): (() => void) => {
    EventsOn(EventModelDownloadProgress, callback)
    return () => EventsOff(EventModelDownloadProgress)
  },
  // 监听模型下载完成事件：下载全部完成/失败后推送结果
  subscribeModelDownloadComplete: (
    callback: (result: ModelDownloadComplete) => void
  ): (() => void) => {
    EventsOn(EventModelDownloadComplete, callback)
    return () => EventsOff(EventModelDownloadComplete)
  }
} as const
