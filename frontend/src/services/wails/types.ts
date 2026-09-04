/**
 * Wails 服务门面 - 类型定义层
 * 集中门面对外暴露的全部接口与类型别名（从原 wails.ts 迁移，值零变化）
 */
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
} from '../../types/chat'
import type { CollectionInfo, DocumentMeta } from '../../types/search'
import { DEFAULT_CONFIG } from '../../types/chat'

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

/** 后端下载进度事件（对齐 Go llm.DownloadProgress 的 JSON tag，字段名必须两端同步） */
export interface BackendDownloadProgress {
  backend: string
  asset_name: string
  tag_name: string
  total_bytes: number
  downloaded: number
  percent: number
  status: string
  error: string
  label: string
}

// BackendDownloadStart 启动阶段下载开始时推送的信息
export interface BackendDownloadStart {
  backend: string
  name: string
}

/** 启动致命错误 payload：后端无法继续启动时推送，前端在启动屏展示错误卡 */
export interface StartupErrorPayload {
  title: string
  brief: string
  detail: string
}

/** 后端下载完成事件 */
export interface BackendDownloadComplete {
  backend: string
  success: boolean
  error?: string
  server_path?: string
}

/** 模型下载源上的一个模型仓库 */
export interface HubModel {
  provider: string
  repo_id: string
  name: string
  downloads: number
  likes: number
  /** 主 .gguf 文件大小（字节）：仓库内最小的主文件（入门量化档），查询失败为 0 */
  main_file_size?: number
}

/** 仓库内的一个可下载文件 */
export interface HubFile {
  provider: string
  repo_id: string
  path: string
  size: number
  is_gguf: boolean
  is_mmproj: boolean
  url: string
}

/** 模型下载进度事件 */
export interface ModelDownloadProgress {
  provider: string
  repo_id: string
  file_path: string
  total_bytes: number
  downloaded: number
  percent: number
  status: string
  error: string
}

/** 模型下载完成事件 */
export interface ModelDownloadComplete {
  repo_id: string
  file: string
  success: boolean
  error?: string
  /** 下载成功后的生效方式：auto=后端自动加载，listed=已入列表可手动切换，restart=需重启 */
  activate?: 'auto' | 'listed' | 'restart'
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

/** 聊天消息（用于 token 计数和模板应用） */
export interface ChatMessage {
  role: string
  content: string // 移除 | any，避免类型擦除
  reasoning_content?: string
  tool_call_id?: string
}

/** 手动压缩返回结果 */
export interface CompressResult {
  shortSummary: string
  longSummary: string
  trimmedCount: number
  message: string
}
