/**
 * Wails 服务门面 - 后端域
 * ============ 显卡后端管理 ============
 * 后端类型在 llama-server 启动时确定，切换后端需重启应用才能生效。
 * 生活类比：像选发动机型号——选好后要重新点火才能用新发动机跑。
 * （从原 wails.ts 迁移,方法体逐字搬移,逻辑零变化）
 */
import { GetBackendStatus, SwitchBackend, DownloadBackend } from '../../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime'
import {
  EventBackendSwitched,
  EventBackendDownloadStart,
  EventBackendDownloadProgress,
  EventBackendDownloadComplete
} from '../events'
import type { BackendStatus } from '../../types/chat'
import type {
  BackendDownloadComplete,
  BackendDownloadProgress,
  BackendDownloadStart
} from './types'

export const backendMethods = {
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
  }
} as const
