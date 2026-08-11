/**
 * Wails 服务门面
 * 集中所有 Wails binding 的类型适配,避免散落在业务代码中的 `as unknown` 类型擦除
 */
import {
  SendMessage,
  StopGeneration,
  GetConversations,
  GetMessages,
  CreateConversation,
  RenameConversation,
  DeleteConversation,
  SearchMessages,
  ExportConversation,
  ExportConversationWithDialog,
  GetConfig,
  GetCleanupResult,
  GetBackendStatus,
  UpdateConfig,
  GetServerStatus,
  GetMetrics,
  DeleteMessage,
  CompressConversation,
  RegenerateMessage,
  PrepareShutdown,
  GetAvailableModels,
  SwitchModel,
  ReloadModels,
  ListKnowledgeBases,
  CreateKnowledgeBase,
  DeleteKnowledgeBase,
  UploadDocument,
  ListDocuments,
  DeleteDocument as DeleteDocumentAPI,
  SetActiveKnowledgeBase,
  GetActiveKnowledgeBase,
  SetRAGEnabled,
  IsRAGEnabled,
  GetSearchAPIKeys,
  SetSearchAPIKeys,
  HasServerAPIKey,
  SetServerAPIKey,
  GetModelParams,
  SaveModelParams,
  ClearModelParams,
  HasModelParams,
  SelectImageFile,
  HandleCloseRequest,
  SetCloseAction,
  GracefulExit,
  RestartApp,
  StopThinking,
  RerankEnabled,
  SaveSlot,
  RestoreSlot,
  EraseSlot,
  DeleteModel,
  DownloadModel,
  CountTokens,
  GetLoraAdapters,
  SetLoraAdapters,
  GetSlots,
  Tokenize,
  GetLastPromptTokens,
  ApplyTemplate,
  GetServerLogs,
  GetTerminalHistory,
  ResizeTerminal,
  IsConPTYMode,
  SelectLoraFile,
  GetMCPServers,
  SaveMCPServers,
  TestMCPConnection,
  GetMCPStatus,
  ListMCPTools,
  RefreshMcpTools,
  SwitchBackend,
  DownloadBackend,
  GetAppVersion,
  CheckUpdate,
  PerformUpdate,
  ResolveGpuTypeChoice
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import {
  EventChatStream,
  EventServerStatus,
  EventChatAbnormalCleanup,
  EventServerWarning,
  EventWindowCloseRequest,
  EventServerSwitchProgress,
  EventServerMmprojUnavailable,
  EventSearchAutoDisabled,
  EventShutdownProgress,
  EventModelLoadProgress,
  EventServerLog,
  EventServerTerminal,
  EventUpdateProgress,
  EventBackendSwitched,
  EventBackendDownloadStart,
  EventBackendDownloadProgress,
  EventBackendDownloadComplete,
  EventHardwareGpuTypeUnknown
} from './events'
import { chat as ChatModel } from '../../wailsjs/go/models'

import type {
  Conversation,
  Message,
  SendMessageParams,
  Config,
  ServerStatus,
  MetricsSummary,
  ModelOption,
  SwitchResult,
  StreamEvent,
  SearchAPIKeys,
  Attachment,
  AttachmentSummary,
  ModelCapabilities,
  MCPServerConfig,
  MCPToolInfo,
  MCPServerStatus,
  MCPConnectResult,
  BackendStatus
} from '../types/chat'
import type { CollectionInfo, DocumentMeta } from '../types/search'
import { DEFAULT_CONFIG } from '../types/chat'

// 重新导出类型,保持原导入路径兼容
export type {
  Conversation,
  Message,
  SendMessageParams,
  Config,
  ServerStatus,
  MetricsSummary,
  ModelOption,
  SwitchResult,
  StreamEvent,
  SearchAPIKeys,
  Attachment,
  AttachmentSummary,
  ModelCapabilities,
  CollectionInfo,
  DocumentMeta,
  MCPServerConfig,
  MCPToolInfo,
  MCPServerStatus,
  MCPConnectResult,
  BackendStatus
}
export { DEFAULT_CONFIG }

/** 切换进度事件 */
export interface SwitchProgressEvent {
  stage: string
  targetModel: string
}

/** 关闭进度事件 */
export interface ShutdownProgressEvent {
  stage: string
  message: string
}

/** 模型加载进度事件 */
export interface ModelLoadProgressEvent {
  model: string
  status: string
  progress: number
}

/** 后端下载进度事件 */
export interface BackendDownloadProgress {
  Backend: string
  AssetName: string
  TagName: string
  TotalBytes: number
  Downloaded: number
  Percent: number
  Status: string
  Error: string
  Label: string
}

// BackendDownloadStart 启动阶段下载开始时推送的信息
export interface BackendDownloadStart {
  backend: string
  name: string
}

/**
 * 灰色地带事件 payload：后端检测到未知的显卡状态时推送给前端，
 * 让用户在对话框中选择推理后端。
 */
export interface GpuTypeUnknownPayload {
  gpu_name: string
  gpu_vendor: string
  gpu_vram_mb: number
  gpu_type: string
  timeout_seconds: number
}

/** 后端下载完成事件 */
export interface BackendDownloadComplete {
  backend: string
  success: boolean
  error?: string
  server_path?: string
}

/** 异常清理事件 */
export interface AbnormalCleanupEvent {
  count: number
  removed: Array<{ id: string; title: string; reason: string }>
}

/** 服务器警告事件（如 preset 文件生成失败） */
export interface ServerWarningEvent {
  type: string
  message: string
}

/** 清理结果 */
export interface CleanupResult {
  id: string
  title: string
  reason: string
}

/** 导出格式 */
export type ExportFormat = 'markdown' | 'json' | 'txt' | 'csv'

/** LoRA 适配器信息 */
export interface LoraAdapter {
  id: number
  path: string
  scale: number
}

/** Slot 状态信息 */
export interface SlotInfo {
  id: number
  task: string
  n_prompt: number
  n_predicted: number
  n_gpu_layers: number
  model: string
  n_cache_tokens: number
  cache_shift: boolean
}

/** 更新信息 */
export interface UpdateInfo {
  has_update: boolean
  latest_version: string
  current_version: string
  download_url: string
  release_notes: string
  published_at: string
}

/** 更新下载进度事件 */
export interface UpdateProgressEvent {
  percent: number
  downloaded: number
  total: number
}

/** 聊天消息（用于 token 计数和模板应用） */
export interface ChatMessage {
  role: string
  content: string // 移除 | any，避免类型擦除（任务 28.2）
  reasoning_content?: string
  tool_call_id?: string
}

/** P2-A3: 手动压缩返回结果 */
export interface CompressResult {
  shortSummary: string
  longSummary: string
  trimmedCount: number
  message: string
}

/**
 * 适配 wails 生成的 binding 类型。
 * 后端 snake_case 与前端 snake_case 字段一致,所以这里仅做基本类型转接。
 */
function adaptConfig(raw: unknown): Config {
  if (!raw || typeof raw !== 'object') {
    return { ...DEFAULT_CONFIG }
  }
  return { ...DEFAULT_CONFIG, ...(raw as Partial<Config>) }
}

/**
 * 将前端 SendMessageParams 转换为 wails SendMessage 期望的参数类型。
 * ChatModel.SendMessageParams.createFrom 已构造 wails 端 class 实例，
 * 此处仅做精确的字段映射与类型断言，避免调用点使用 as unknown as（任务 23）。
 */
function toWailsSendMessageParams(
  params: ChatModel.SendMessageParams
): Parameters<typeof SendMessage>[0] {
  return {
    conversation_id: params.conversation_id,
    content: params.content,
    search_mode: params.search_mode,
    images: params.images,
    attachments: params.attachments
  } as Parameters<typeof SendMessage>[0]
}

/**
 * 将前端 Config 转换为 wails UpdateConfig 期望的参数类型。
 * 前端 Config 是 wails config.Config 的超集（含 UI 专用字段），
 * 后端 JSON 反序列化时会忽略多余字段，因此直接展开即可。
 */
function toWailsConfig(cfg: Config): Parameters<typeof UpdateConfig>[0] {
  return { ...cfg } as Parameters<typeof UpdateConfig>[0]
}

/**
 * 将前端 ChatMessage[] 转换为 CountTokens/ApplyTemplate 期望的参数类型。
 * 显式映射每个字段，避免 as any 导致的类型擦除（任务 23）。
 */
function toWailsChatMessages(messages: ChatMessage[]): Parameters<typeof CountTokens>[0] {
  return messages.map(m => ({
    role: m.role,
    content: m.content,
    reasoning_content: m.reasoning_content,
    tool_call_id: m.tool_call_id
  })) as Parameters<typeof CountTokens>[0]
}

/**
 * 将前端 LoraAdapter[] 转换为 SetLoraAdapters 期望的参数类型。
 * 字段完全一致，映射后断言为 wails 参数类型（任务 23）。
 */
function toWailsLoraAdapters(adapters: LoraAdapter[]): Parameters<typeof SetLoraAdapters>[0] {
  return adapters.map(a => ({
    id: a.id,
    path: a.path,
    scale: a.scale
  })) as Parameters<typeof SetLoraAdapters>[0]
}

export const wails = {
  sendMessage: async (params: SendMessageParams): Promise<void> => {
    // 使用 wailsjs 生成的 createFrom 适配类型
    const wailsParams = ChatModel.SendMessageParams.createFrom({
      conversation_id: params.conversation_id,
      content: params.content,
      search_mode: params.search_mode,
      images: params.images,
      attachments: params.attachments
    })
    await SendMessage(toWailsSendMessageParams(wailsParams))
  },
  stopGeneration: StopGeneration,
  getConversations: async (): Promise<Conversation[]> => {
    return (await GetConversations()) as Conversation[]
  },
  getMessages: async (conversationID: string): Promise<Message[]> => {
    return (await GetMessages(conversationID)) as Message[]
  },
  createConversation: async (): Promise<Conversation> => {
    return (await CreateConversation()) as Conversation
  },
  renameConversation: RenameConversation,
  deleteConversation: DeleteConversation,
  searchMessages: async (query: string): Promise<Message[]> => {
    return (await SearchMessages(query)) as Message[]
  },
  exportConversation: ExportConversation,
  exportConversationWithDialog: async (id: string, format: string): Promise<boolean> => {
    return (await ExportConversationWithDialog(id, format)) as boolean
  },
  getConfig: async (): Promise<Config> => adaptConfig(await GetConfig()),
  getCleanupResult: async (): Promise<CleanupResult[]> => {
    return (await GetCleanupResult()) as CleanupResult[]
  },
  updateConfig: async (cfg: Config): Promise<void> => {
    await UpdateConfig(toWailsConfig(cfg))
  },
  handleCloseRequest: async (): Promise<string> => {
    return await HandleCloseRequest()
  },
  setCloseAction: async (action: string): Promise<void> => {
    await SetCloseAction(action)
  },
  gracefulExit: async (): Promise<void> => {
    await GracefulExit()
  },
  // 重启应用：启动新进程后退出当前进程（用于下载完成后自动重启）
  restartApp: async (): Promise<void> => {
    await RestartApp()
  },
  stopThinking: async (): Promise<void> => {
    await StopThinking()
  },
  rerankEnabled: async (): Promise<boolean> => {
    return await RerankEnabled()
  },
  saveSlot: async (slotID: number): Promise<void> => {
    await SaveSlot(slotID)
  },
  restoreSlot: async (slotID: number): Promise<void> => {
    await RestoreSlot(slotID)
  },
  eraseSlot: async (slotID: number): Promise<void> => {
    await EraseSlot(slotID)
  },
  deleteModel: async (modelName: string): Promise<void> => {
    await DeleteModel(modelName)
  },
  downloadModel: async (modelName: string): Promise<void> => {
    await DownloadModel(modelName)
  },
  countTokens: async (messages: ChatMessage[]): Promise<number> => {
    return await CountTokens(toWailsChatMessages(messages))
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
  getSlots: async (): Promise<SlotInfo[]> => {
    return (await GetSlots()) as SlotInfo[]
  },
  tokenize: async (text: string): Promise<number[]> => {
    return await Tokenize(text)
  },
  getLastPromptTokens: async (): Promise<number> => {
    return await GetLastPromptTokens()
  },
  applyTemplate: async (messages: ChatMessage[]): Promise<string> => {
    return await ApplyTemplate(toWailsChatMessages(messages))
  },
  getServerLogs: async (): Promise<string> => {
    return await GetServerLogs()
  },
  getServerStatus: async (): Promise<ServerStatus> => {
    return (await GetServerStatus()) as ServerStatus
  },
  getMetrics: async (): Promise<MetricsSummary> => {
    return (await GetMetrics()) as MetricsSummary
  },
  deleteMessage: async (id: string): Promise<void> => {
    await DeleteMessage(id)
  },
  // P2-A3: 手动触发对话压缩
  compressConversation: async (convID: string): Promise<CompressResult> => {
    return (await CompressConversation(convID)) as CompressResult
  },
  regenerateMessage: async (userMessageID: string, searchMode: string): Promise<void> => {
    await RegenerateMessage(userMessageID, searchMode)
  },
  getAvailableModels: async (): Promise<ModelOption[]> => {
    return (await GetAvailableModels()) as ModelOption[]
  },
  switchModel: async (modelName: string): Promise<SwitchResult> => {
    return (await SwitchModel(modelName)) as SwitchResult
  },
  reloadModels: async (): Promise<void> => {
    await ReloadModels()
  },
  // F-1.10+F-3.5：事件订阅统一为 subscribeXxx(callback): () => void 模式
  // 返回的 unsubscribe 函数用于取消订阅，替代原来的 onXxx/offXxx 配对调用
  // 生活类比：像订报纸——订阅（subscribe）后拿到一个"退订凭证"（unsubscribe 函数），
  // 想不看了就凭它退订，不用记住自己订的是哪份报纸（事件名）。
  subscribeChatStream: (callback: (event: StreamEvent) => void): (() => void) => {
    EventsOn(EventChatStream, callback)
    return () => EventsOff(EventChatStream)
  },
  subscribeServerStatus: (callback: (status: ServerStatus) => void): (() => void) => {
    EventsOn(EventServerStatus, callback)
    return () => EventsOff(EventServerStatus)
  },
  prepareShutdown: PrepareShutdown,
  subscribeAbnormalCleanup: (callback: (data: AbnormalCleanupEvent) => void): (() => void) => {
    EventsOn(EventChatAbnormalCleanup, callback)
    return () => EventsOff(EventChatAbnormalCleanup)
  },
  subscribeServerWarning: (callback: (data: ServerWarningEvent) => void): (() => void) => {
    EventsOn(EventServerWarning, callback)
    return () => EventsOff(EventServerWarning)
  },
  subscribeCloseRequest: (callback: () => void): (() => void) => {
    EventsOn(EventWindowCloseRequest, callback)
    return () => EventsOff(EventWindowCloseRequest)
  },
  subscribeSwitchProgress: (callback: (progress: SwitchProgressEvent) => void): (() => void) => {
    EventsOn(EventServerSwitchProgress, callback)
    return () => EventsOff(EventServerSwitchProgress)
  },
  subscribeMmprojUnavailable: (callback: () => void): (() => void) => {
    EventsOn(EventServerMmprojUnavailable, callback)
    return () => EventsOff(EventServerMmprojUnavailable)
  },
  subscribeSearchAutoDisabled: (callback: () => void): (() => void) => {
    EventsOn(EventSearchAutoDisabled, callback)
    return () => EventsOff(EventSearchAutoDisabled)
  },
  subscribeShutdownProgress: (
    callback: (progress: ShutdownProgressEvent) => void
  ): (() => void) => {
    EventsOn(EventShutdownProgress, callback)
    return () => EventsOff(EventShutdownProgress)
  },
  subscribeModelLoadProgress: (
    callback: (progress: ModelLoadProgressEvent) => void
  ): (() => void) => {
    EventsOn(EventModelLoadProgress, callback)
    return () => EventsOff(EventModelLoadProgress)
  },
  subscribeServerLog: (callback: (line: string) => void): (() => void) => {
    EventsOn(EventServerLog, callback)
    return () => EventsOff(EventServerLog)
  },
  // ConPTY 终端原始字节流（base64 编码，用于 xterm.js 渲染）
  subscribeTerminalData: (callback: (data: string) => void): (() => void) => {
    EventsOn(EventServerTerminal, callback)
    return () => EventsOff(EventServerTerminal)
  },
  getTerminalHistory: async (): Promise<string> => {
    return await GetTerminalHistory()
  },
  resizeTerminal: async (cols: number, rows: number): Promise<void> => {
    await ResizeTerminal(cols, rows)
  },
  isConPTYMode: async (): Promise<boolean> => {
    return await IsConPTYMode()
  },
  listKnowledgeBases: async (): Promise<CollectionInfo[]> => {
    return (await ListKnowledgeBases()) as CollectionInfo[]
  },
  createKnowledgeBase: async (name: string): Promise<void> => {
    await CreateKnowledgeBase(name)
  },
  deleteKnowledgeBase: async (name: string): Promise<void> => {
    await DeleteKnowledgeBase(name)
  },
  uploadDocument: async (
    kbName: string,
    fileName: string,
    fileData: string,
    mimeType: string
  ): Promise<void> => {
    await UploadDocument(kbName, fileName, fileData, mimeType)
  },
  listDocuments: async (kbName: string): Promise<DocumentMeta[]> => {
    return (await ListDocuments(kbName)) as DocumentMeta[]
  },
  deleteDocument: async (kbName: string, docID: string): Promise<void> => {
    await DeleteDocumentAPI(kbName, docID)
  },
  setActiveKnowledgeBase: async (kbName: string): Promise<void> => {
    await SetActiveKnowledgeBase(kbName)
  },
  getActiveKnowledgeBase: async (): Promise<string> => {
    return (await GetActiveKnowledgeBase()) as string
  },
  setRAGEnabled: async (enabled: boolean): Promise<void> => {
    await SetRAGEnabled(enabled)
  },
  isRAGEnabled: async (): Promise<boolean> => {
    return (await IsRAGEnabled()) as boolean
  },
  getSearchAPIKeys: async (): Promise<SearchAPIKeys> => {
    return (await GetSearchAPIKeys()) as SearchAPIKeys
  },
  setSearchAPIKeys: async (keys: SearchAPIKeys): Promise<void> => {
    await SetSearchAPIKeys(keys)
  },
  hasServerAPIKey: async (): Promise<boolean> => {
    return (await HasServerAPIKey()) as boolean
  },
  setServerAPIKey: async (key: string): Promise<void> => {
    await SetServerAPIKey(key)
  },
  // 模型专属生成参数：每个模型保存各自的生成参数，切换时自动恢复
  getModelParams: async (modelName: string): Promise<any> => {
    return await GetModelParams(modelName)
  },
  saveModelParams: async (modelName: string): Promise<void> => {
    await SaveModelParams(modelName)
  },
  clearModelParams: async (modelName: string): Promise<void> => {
    await ClearModelParams(modelName)
  },
  hasModelParams: async (modelName: string): Promise<boolean> => {
    return await HasModelParams(modelName)
  },
  selectImageFile: async (): Promise<string> => {
    return (await SelectImageFile()) as string
  },
  // 更新相关方法：通过 wailsjs 绑定调用 Go 端
  getAppVersion: async (): Promise<string> => {
    return await GetAppVersion()
  },
  checkUpdate: async (): Promise<UpdateInfo> => {
    return (await CheckUpdate()) as UpdateInfo
  },
  performUpdate: async (downloadURL: string, latestVersion: string): Promise<void> => {
    await PerformUpdate(downloadURL, latestVersion)
  },
  subscribeUpdateProgress: (callback: (progress: UpdateProgressEvent) => void): (() => void) => {
    EventsOn(EventUpdateProgress, callback)
    return () => EventsOff(EventUpdateProgress)
  },
  // ============ MCP 服务器管理 ============
  // 新架构：豆芽不直接管理 MCP 子进程，而是生成 mcp_servers.json 交给 llama-server 加载。
  // 修改配置后需重启 llama-server 才能生效（无热重载）。
  // 生活类比：豆芽只负责填写外卖平台对接卡，调度中心（llama-server）才真正对接平台。
  getMCPServers: async (): Promise<MCPServerConfig[]> => {
    return (await GetMCPServers()) as MCPServerConfig[]
  },
  saveMCPServers: async (servers: MCPServerConfig[]): Promise<void> => {
    await SaveMCPServers(servers as Parameters<typeof SaveMCPServers>[0])
  },
  testMCPConnection: async (server: MCPServerConfig): Promise<MCPConnectResult> => {
    return (await TestMCPConnection(
      server as Parameters<typeof TestMCPConnection>[0]
    )) as MCPConnectResult
  },
  getMCPStatus: async (): Promise<Record<string, MCPServerStatus>> => {
    return (await GetMCPStatus()) as Record<string, MCPServerStatus>
  },
  listMCPTools: async (): Promise<MCPToolInfo[]> => {
    return (await ListMCPTools()) as MCPToolInfo[]
  },
  refreshMcpTools: async (): Promise<void> => {
    await RefreshMcpTools()
  },
  // ============ 显卡后端管理 ============
  // 后端类型在 llama-server 启动时确定，切换后端需重启应用才能生效。
  // 生活类比：像选发动机型号——选好后要重新点火才能用新发动机跑。
  getBackendStatus: async (): Promise<BackendStatus> => {
    return (await GetBackendStatus()) as BackendStatus
  },
  switchBackend: async (backendType: string): Promise<void> => {
    await SwitchBackend(backendType)
  },
  // 从 GitHub 下载指定后端的 zip 包并自动解压安装（异步，进度通过事件推送）
  downloadBackend: async (backendType: string): Promise<void> => {
    await DownloadBackend(backendType)
  },
  // 监听后端切换事件：切换配置后后端会推送最新状态，前端据此刷新显示
  subscribeBackendSwitched: (callback: (status: BackendStatus) => void): (() => void) => {
    EventsOn(EventBackendSwitched, callback)
    return () => EventsOff(EventBackendSwitched)
  },
  // 监听后端下载开始事件：启动阶段用户同意下载后推送，前端据此切换 splash 到下载阶段
  subscribeBackendDownloadStart: (callback: (info: BackendDownloadStart) => void): (() => void) => {
    EventsOn(EventBackendDownloadStart, callback)
    return () => EventsOff(EventBackendDownloadStart)
  },
  // 监听后端下载进度事件：下载过程中实时推送进度信息
  subscribeBackendDownloadProgress: (
    callback: (progress: BackendDownloadProgress) => void
  ): (() => void) => {
    EventsOn(EventBackendDownloadProgress, callback)
    return () => EventsOff(EventBackendDownloadProgress)
  },
  // 监听后端下载完成事件：下载和安装完成后推送结果
  subscribeBackendDownloadComplete: (
    callback: (result: BackendDownloadComplete) => void
  ): (() => void) => {
    EventsOn(EventBackendDownloadComplete, callback)
    return () => EventsOff(EventBackendDownloadComplete)
  },
  // ============ 灰色地带 GPU 类型选择 ============
  // auto 模式 + GPUType=unknown 时，后端检测到未知的显卡状态，让用户选择推理后端。
  // 生活类比：车检员无法判断是跑车还是电瓶车时，让车主自己选驾驶模式。
  subscribeHardwareGpuTypeUnknown: (
    callback: (payload: GpuTypeUnknownPayload) => void
  ): (() => void) => {
    EventsOn(EventHardwareGpuTypeUnknown, callback)
    return () => EventsOff(EventHardwareGpuTypeUnknown)
  },
  // 用户选择后端后调用，解除后端 startup 的阻塞等待
  resolveGpuTypeChoice: async (backend: string): Promise<void> => {
    await ResolveGpuTypeChoice(backend)
  }
} as const

export type WailsService = typeof wails
