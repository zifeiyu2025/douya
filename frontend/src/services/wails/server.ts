/**
 * Wails 服务门面 - 服务器域
 * 状态/指标/日志/优雅关闭/终端（ConPTY）
 * （从原 wails.ts 迁移,方法体逐字搬移,逻辑零变化）
 */
import {
  GetServerStatus,
  GetMetrics,
  GetServerLogs,
  PrepareShutdown,
  GetTerminalHistory,
  ResizeTerminal,
  IsConPTYMode
} from '../../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime'
import {
  EventServerStatus,
  EventServerWarning,
  EventShutdownProgress,
  EventServerLog,
  EventServerTerminal
} from '../events'
import type { MetricsSummary, ServerStatus } from '../../types/chat'
import type { ServerWarningEvent, ShutdownProgressEvent } from './types'

export const serverMethods = {
  getServerLogs: async (): Promise<string> => {
    return await GetServerLogs()
  },
  getServerStatus: async (): Promise<ServerStatus> => {
    return (await GetServerStatus()) as ServerStatus
  },
  getMetrics: async (): Promise<MetricsSummary> => {
    return (await GetMetrics()) as MetricsSummary
  },
  prepareShutdown: PrepareShutdown,
  subscribeServerStatus: (callback: (status: ServerStatus) => void): (() => void) => {
    EventsOn(EventServerStatus, callback)
    return () => EventsOff(EventServerStatus)
  },
  subscribeServerWarning: (callback: (data: ServerWarningEvent) => void): (() => void) => {
    EventsOn(EventServerWarning, callback)
    return () => EventsOff(EventServerWarning)
  },
  subscribeShutdownProgress: (
    callback: (progress: ShutdownProgressEvent) => void
  ): (() => void) => {
    EventsOn(EventShutdownProgress, callback)
    return () => EventsOff(EventShutdownProgress)
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
  }
} as const
