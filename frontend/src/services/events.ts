/**
 * 前后端事件名常量
 * 与 Go 端 app_events.go 中的常量一一对应，修改时必须两端同步。
 * 生活类比：事件名是前后端之间的"电报频道号"——双方约定好频道号才能互相收发消息。
 * 集中定义避免硬编码字符串散落各处，防止"频道号写错"导致的通信故障。
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
export const EventWindowCloseRequest = 'window:closeRequest'
export const EventShutdownProgress = 'shutdown:progress'
export const EventSearchAutoDisabled = 'search:autoDisabled'
// P3.6 修复：EventUpdateCheck 已移除——前端使用同步 CheckUpdate RPC，
// 无事件监听者。EventUpdateProgress 保留（wails.ts 有订阅）。
export const EventUpdateProgress = 'update:progress'
