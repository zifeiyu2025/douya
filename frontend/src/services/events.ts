/**
 * 前后端事件名常量
 * 与 Go 端 app_events.go 中的常量一一对应，修改时必须两端同步。
 * 事件名是前后端约定的通信标识，双方保持一致才能正常收发消息；
 * 集中定义避免硬编码字符串散落各处，防止拼写不一致导致的通信故障。
 */
export const EventChatStream = 'chat:stream'
export const EventChatAbnormalCleanup = 'chat:abnormal_cleanup'
export const EventServerStatus = 'server:status'
export const EventServerLog = 'server:log'
export const EventServerTerminal = 'server:terminal'
export const EventServerWarning = 'server:warning'
export const EventServerSwitchProgress = 'server:switchProgress'
export const EventServerMmprojUnavailable = 'server:mmprojUnavailable'
export const EventModelLoadProgress = 'modelLoadProgress'
export const EventBackendSwitched = 'backend:switched'
export const EventBackendDownloadStart = 'backend:downloadStart'
export const EventBackendDownloadProgress = 'backend:downloadProgress'
export const EventBackendDownloadComplete = 'backend:downloadComplete'
// 模型下载（内置下载器，来源 ModelScope / HF 镜像）事件，与 Go 端 app_events.go 同步。
export const EventModelDownloadProgress = 'model:downloadProgress'
export const EventModelDownloadComplete = 'model:downloadComplete'
export const EventWindowCloseRequest = 'window:closeRequest'
export const EventShutdownProgress = 'shutdown:progress'
export const EventSearchAutoDisabled = 'search:autoDisabled'
// 启动期致命错误：后端遇到无法继续启动的错误时推送，前端在启动屏上展示错误卡，
// 用户确认后后端才会退出。与 Go 端 EventStartupError 对应。
export const EventStartupError = 'startup:error'
// 后端下载确认：runtime 缺失需下载时推送，前端弹"是否下载后端"对话框。
// 与 Go 端 EventBackendDownloadRequest 对应。
export const EventBackendDownloadRequest = 'startup:backendDownloadRequest'
// 知识库（RAG）初始化失败：非阻塞提示前端"知识库已禁用，基本对话不受影响"。
// 与 Go 端 EventStartupRagDisabled 对应。
export const EventStartupRagDisabled = 'startup:ragDisabled'
// 无可用模型引导文案：非阻塞提示前端"如何下载模型"。
// 与 Go 端 EventStartupModelNotice 对应。
export const EventStartupModelNotice = 'startup:modelNotice'
// P3.6 修复：EventUpdateCheck 已移除——前端使用同步 CheckUpdate RPC，
// 无事件监听者。EventUpdateProgress 保留（wails.ts 有订阅）。
export const EventUpdateProgress = 'update:progress'
