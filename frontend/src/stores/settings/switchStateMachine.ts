// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

import type { Ref } from 'vue'
import { watch } from 'vue'
import { logError, logWarn } from '../../utils/logger'
import type {
  ModelSwitchState,
  BackendProgressStage,
  SwitchProgressStage
} from '../../types/settings'
import { SWITCH_TIMING } from '../../types/settings'

/**
 * 模型切换状态机 composable：从 useSettingsStore 提取，降低主 store 复杂度。
 *
 * 拆分说明：原 store 内 10 个状态机函数（startSwitch/reportProgress/finishSuccess/
 * finishFailure/finishFirstLoadFailure/finishTimeout/reset/onServerReady/beginFirstLoad）
 * + 启动兜底轮询 + 看门狗 + watch 副作用，整体围绕 switchState 一个状态变量，
 * 适合提取为独立 composable。
 *
 * 状态流转函数职责：startSwitch 发起切换进入 switching 态，finishSuccess 到达就绪态，
 * finishFailure 故障报错，finishTimeout 超时兜底，reset 手动复位。
 * watch 副作用负责自动流转：ready_after_switch 后 800ms 自动回 idle，failed 后 5s 自动复位。
 */

/** 状态机所需的共享状态依赖 */
export interface SwitchStateDeps {
  switchState: Ref<ModelSwitchState>
  currentModel: Ref<string>
  hasEverBeenReady: Ref<boolean>
  /** 外部提供的 checkServerStatus，用于轮询兜底 */
  checkServerStatus: () => Promise<void>
}

export function useSwitchStateMachine(deps: SwitchStateDeps) {
  const { switchState, currentModel, hasEverBeenReady, checkServerStatus } = deps

  function startSwitch(modelName: string) {
    switchState.value = {
      phase: 'switching',
      startedAt: Date.now(),
      targetModel: modelName,
      previousModel: currentModel.value,
      progressStage: 'preparing'
    }
  }

  /**
   * 上报后端进度（server:switchProgress 事件的 stage 字段）。
   *
   * 后端在切换过程中持续推送进度阶段，
   * 前端在这里把每一条状态更新到 switchState.progressStage，UI 就能实时显示当前阶段，
   * 而不是一直停留在"准备切换"直到 wails.switchModel 返回。
   *
   * 注意：警告类 stage（vram-warning/spec-warning）不改变主进度阶段，
   * 仅作为提示信息（UI 可选展示），避免警告事件打断正常进度显示。
   */
  function reportProgress(stage: BackendProgressStage) {
    // 状态机单向流转：终态不接受进度事件
    if (
      switchState.value.phase === 'idle' ||
      switchState.value.phase === 'ready_after_switch' ||
      switchState.value.phase === 'failed' ||
      switchState.value.phase === 'timeout'
    ) {
      return
    }
    // 警告类事件不改变主进度阶段，避免打断正常进度显示
    if (stage === 'vram-warning' || stage === 'spec-warning') {
      return
    }
    // switching 阶段：实时更新 progressStage，让 UI 能显示 loading/waiting/detecting 等中间阶段
    if (switchState.value.phase === 'switching') {
      switchState.value = {
        ...switchState.value,
        progressStage: stage as SwitchProgressStage
      }
    }
    // first_load 阶段：暂不更新子阶段（首次加载的进度展示逻辑保持原样）
  }

  /** 切换成功 */
  function finishSuccess(model: string) {
    switchState.value = {
      phase: 'ready_after_switch',
      startedAt:
        switchState.value.phase !== 'idle' && 'startedAt' in switchState.value
          ? switchState.value.startedAt
          : Date.now(),
      targetModel: model
    }
  }

  /** 切换失败 */
  function finishFailure(err: string, prev: string, rolledBack: boolean, rbSuccess: boolean) {
    const s = switchState.value
    const startedAt = 'startedAt' in s ? s.startedAt : Date.now()
    const targetModel = s.phase === 'idle' ? '' : s.targetModel
    switchState.value = {
      phase: 'failed',
      error: err,
      targetModel: targetModel || '',
      rolledBack,
      rollbackSuccess: rbSuccess,
      startedAt
    }
    if (prev && !rolledBack) {
      currentModel.value = prev
    }
  }

  /** 首次启动加载失败（终态，不自动恢复） */
  function finishFirstLoadFailure(err: string, targetModel: string) {
    const s = switchState.value
    const startedAt = 'startedAt' in s ? s.startedAt : Date.now()
    switchState.value = {
      phase: 'first_load_failed',
      error: err,
      targetModel: targetModel || '',
      startedAt
    }
  }

  /** 切换超时 */
  function finishTimeout() {
    const s = switchState.value
    if (s.phase === 'idle') return
    const targetModel = 'targetModel' in s ? s.targetModel : ''
    const startedAt = 'startedAt' in s ? s.startedAt : Date.now()
    switchState.value = { phase: 'timeout', targetModel, startedAt }
  }

  /** 主动重置（用户主动关闭遮罩等） */
  function reset() {
    switchState.value = { phase: 'idle' }
  }

  /** 收到 server:status 事件时的状态机处理（首次加载完成） */
  function onServerReady() {
    const s = switchState.value
    if (s.phase === 'first_load') {
      switchState.value = { phase: 'idle' }
    }
  }

  /** 首次启动时记录 "first_load" 阶段 */
  function beginFirstLoad(targetModel: string) {
    if (switchState.value.phase === 'idle') {
      switchState.value = {
        phase: 'first_load',
        startedAt: Date.now(),
        targetModel
      }
    }
  }

  // ----- 启动兜底：周期性状态轮询 + 看门狗（防止事件监听器注册晚于后端事件发射导致无限转圈） -----
  let startupPollingTimer: ReturnType<typeof setInterval> | null = null
  let startupWatchdogTimer: ReturnType<typeof setTimeout> | null = null
  let receivedAnyStatusEvent = false

  /** 标记已收到状态事件（供外部 initStatusListener 调用） */
  function markStatusEventReceived() {
    receivedAnyStatusEvent = true
  }

  /** 启动周期性状态轮询（每 3s 兜底检查服务器状态） */
  function startStartupPolling() {
    // 监听器重复初始化时先清除旧定时器
    stopStartupPolling()
    startupPollingTimer = setInterval(() => {
      checkServerStatus()
    }, 3000)
  }

  /** 停止周期性状态轮询 + 清除看门狗定时器 */
  function stopStartupPolling() {
    if (startupPollingTimer) {
      clearInterval(startupPollingTimer)
      startupPollingTimer = null
    }
    if (startupWatchdogTimer) {
      clearTimeout(startupWatchdogTimer)
      startupWatchdogTimer = null
    }
  }

  /** 启动看门狗：60s 内未收到任何状态事件则主动轮询，轮询失败则标记 failed */
  function startStartupWatchdog() {
    // 重复初始化时先清除旧定时器
    if (startupWatchdogTimer) {
      clearTimeout(startupWatchdogTimer)
      startupWatchdogTimer = null
    }
    startupWatchdogTimer = setTimeout(async () => {
      if (!receivedAnyStatusEvent && !hasEverBeenReady.value) {
        logWarn('[startup] watchdog: no status event received in 60s, polling manually')
        try {
          await checkServerStatus()
        } catch (e) {
          logError('[startup] watchdog: manual polling failed', e)
          finishFailure(String(e) || '启动看门狗触发失败', currentModel.value, false, false)
        }
      }
    }, 60000)
  }

  // ----- 副作用：状态机驱动的定时器（集中管理,自动清理） -----
  let pendingTransitions: ReturnType<typeof setTimeout>[] = []

  function clearAllTimers() {
    for (const t of pendingTransitions) clearTimeout(t)
    pendingTransitions = []
  }

  watch(switchState, newState => {
    clearAllTimers()
    // 在 ready_after_switch 后 800ms 自动回到 idle
    if (newState.phase === 'ready_after_switch') {
      pendingTransitions.push(
        setTimeout(() => {
          if (switchState.value.phase === 'ready_after_switch') {
            switchState.value = { phase: 'idle' }
          }
        }, 800)
      )
    }
    // failed 后 5s 自动回到 idle
    if (newState.phase === 'failed') {
      pendingTransitions.push(
        setTimeout(() => {
          if (switchState.value.phase === 'failed') {
            switchState.value = { phase: 'idle' }
          }
        }, 5000)
      )
    }
    // first_load 长时间未完成 → 视为失败
    if (newState.phase === 'first_load') {
      pendingTransitions.push(
        setTimeout(() => {
          if (switchState.value.phase === 'first_load') {
            finishTimeout()
          }
        }, SWITCH_TIMING.SERVER_POLL_TIMEOUT_MS)
      )
    }
    // switching 阶段超时保护，避免后端卡死导致前端无限等待
    if (newState.phase === 'switching') {
      pendingTransitions.push(
        setTimeout(() => {
          if (switchState.value.phase === 'switching') {
            finishTimeout()
          }
        }, SWITCH_TIMING.SERVER_POLL_TIMEOUT_MS)
      )
    }
  })

  // 启动兜底：当已就绪或进入终态时停止周期性轮询
  watch(hasEverBeenReady, ready => {
    if (ready) {
      stopStartupPolling()
    }
  })
  watch(
    () => switchState.value.phase,
    phase => {
      // phase 进入 done(ready_after_switch)/failed/timeout/first_load_failed 时停止轮询
      if (
        phase === 'ready_after_switch' ||
        phase === 'failed' ||
        phase === 'timeout' ||
        phase === 'first_load_failed'
      ) {
        stopStartupPolling()
      }
    }
  )

  return {
    startSwitch,
    reportProgress,
    finishSuccess,
    finishFailure,
    finishFirstLoadFailure,
    finishTimeout,
    reset,
    onServerReady,
    beginFirstLoad,
    startStartupPolling,
    stopStartupPolling,
    startStartupWatchdog,
    markStatusEventReceived,
    clearAllTimers
  }
}
