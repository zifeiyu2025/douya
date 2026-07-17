/**
 * 集中类型定义：模型切换状态机
 * 把原来散装的 7 个 ref + 4 个 timer 合并为单一状态
 */

import type { ModelCapabilities } from './chat'

/** 切换进度阶段 */
export type SwitchProgressStage =
  'idle' | 'preparing' | 'loading' | 'waiting' | 'detecting' | 'done' | 'failed' | 'rolling_back'

/** 切换进度（用于展示） */
export interface SwitchProgress {
  stage: SwitchProgressStage
  targetModel: string
  errorMessage: string
  startTime: number
  endTime: number
  rolledBack: boolean
}

/**
 * 模型切换状态机（FSM）
 * - 单一 source of truth,不可能出现"非法状态"
 * - 派生 computed 替代散装 ref
 */
export type ModelSwitchState =
  | { phase: 'idle' }
  | { phase: 'first_load'; startedAt: number; targetModel: string }
  | { phase: 'switching'; startedAt: number; targetModel: string; previousModel: string }
  | { phase: 'ready_after_switch'; startedAt: number; targetModel: string }
  | {
      phase: 'failed'
      error: string
      targetModel: string
      rolledBack: boolean
      rollbackSuccess: boolean
      startedAt: number
    }
  | { phase: 'timeout'; targetModel: string; startedAt: number }
  | { phase: 'first_load_failed'; error: string; targetModel: string; startedAt: number }

/** 模型能力标志 */
export type ModelCapabilityKey =
  'image_input' | 'audio_input' | 'video_input' | 'text_input' | 'reasoning'

/** 状态机的对外接口 */
export interface ModelSwitchContext {
  state: import('vue').Ref<ModelSwitchState>
  isSwitching: import('vue').ComputedRef<boolean>
  isFirstLoad: import('vue').ComputedRef<boolean>
  hasFailed: import('vue').ComputedRef<boolean>
  progressText: import('vue').ComputedRef<string>
  overlayModelName: import('vue').ComputedRef<string>
  startSwitch: (model: string, prev: string) => void
  reportProgress: (stage: SwitchProgressStage) => void
  finishSuccess: (model: string, caps?: ModelCapabilities) => void
  finishFailure: (err: string, prev: string, rolledBack: boolean, rbSuccess: boolean) => void
  finishTimeout: () => void
  reset: () => void
}

/** 切换时序常量(从原 settings.ts 提取) */
export const SWITCH_TIMING = {
  /** "First load" 标题最大显示时间(原 8s) */
  FIRST_LOAD_TITLE_TIMEOUT_MS: 8000,
  /** 切换标题最大显示时间(原 8s) */
  SWITCHING_TITLE_TIMEOUT_MS: 8000,
  /** ready after switch 标题显示(原 5s) */
  READY_TITLE_DURATION_MS: 5000,
  /** failed 阶段显示(原 30s 自动消失,但实际应保留到用户关闭) */
  FAILED_RETAIN_MS: 30_000,
  /** 等待 server ready 的 poll 间隔(原 800ms) */
  SERVER_POLL_INTERVAL_MS: 800,
  /** 等待 server ready 的最长时间(原 300s) */
  SERVER_POLL_TIMEOUT_MS: 300_000,
  /** model load 失败的回调等待(原 30s) */
  LOAD_FAILURE_TIMEOUT_MS: 30_000,
  /** 进度显示最低文字(占位) */
  PROGRESS_PLACEHOLDER: '...'
} as const
