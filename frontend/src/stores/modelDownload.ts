// 模型下载共享状态：进度、完成结果、重试参数。
// 抽出为独立 store 的原因：下载进度需要在任意页面可见（全局悬浮卡），
// 设置页的下载器也要发起/重试下载——两侧共享同一份状态与事件订阅（store 单例只订阅一次）。
import { defineStore } from 'pinia'
import { computed, ref } from 'vue'
import {
  wails,
  type ModelDownloadComplete,
  type ModelDownloadProgress
} from '../services/wails'
import { useSettingsStore } from './settings'
import { logError } from '../utils/logger'

/** 一次下载请求的完整参数（失败后重试时原样重发） */
export interface ModelDownloadAttempt {
  provider: string
  repoId: string
  mainFile: string
  mmproj: string
}

// 空模型错误文案不算"激活失败"（那是首启引导态，不是错误）
const EMPTY_MODEL_ERROR_RE = /模型目录为空|没有可用的模型/

export const useModelDownloadStore = defineStore('modelDownload', () => {
  const settingsStore = useSettingsStore()

  // ===== 状态 =====
  // 下载进度表：file_path → 进度（多文件下载时含主文件与 MMProj）
  const progressMap = ref<Record<string, ModelDownloadProgress>>({})
  // 最近一次下载请求参数（供一键重试）
  const lastAttempt = ref<ModelDownloadAttempt | null>(null)
  // 最近一次下载完成结果（null = 没有可展示的完成状态）
  const lastComplete = ref<ModelDownloadComplete | null>(null)
  // 用户手动关闭了悬浮卡（新下载/重试会重新弹出）
  const dismissed = ref(false)
  const retrying = ref(false)
  const restarting = ref(false)

  // ===== 派生 =====
  // 进行中的下载项（downloading/paused/waiting），failed/completed 项不在此列
  const activeItems = computed(() =>
    Object.values(progressMap.value).filter(
      p => p.status === 'downloading' || p.status === 'paused' || p.status === 'waiting'
    )
  )
  const hasActive = computed(() => activeItems.value.length > 0)
  const modelReady = computed(() => settingsStore.serverStatus.model_ready === true)
  // 自动加载失败：activate=auto 且收到非空模型的错误状态
  const activateFailed = computed(() => {
    if (lastComplete.value?.activate !== 'auto') return false
    const err = settingsStore.serverStatus.error
    return !modelReady.value && !!err && !EMPTY_MODEL_ERROR_RE.test(err)
  })
  // 悬浮卡可见：有活动下载，或有一次未关闭的完成/失败结果
  const visible = computed(
    () => !dismissed.value && (hasActive.value || lastComplete.value !== null)
  )

  // ===== 动作 =====
  /** 发起下载前调用：留底参数、清除旧完成态、占位进度条让悬浮卡即时出现 */
  function recordAttempt(attempt: ModelDownloadAttempt) {
    lastAttempt.value = attempt
    lastComplete.value = null
    dismissed.value = false
    progressMap.value = {
      [attempt.mainFile]: {
        provider: attempt.provider,
        repo_id: attempt.repoId,
        file_path: attempt.mainFile,
        total_bytes: 0,
        downloaded: 0,
        percent: 0,
        status: 'downloading',
        error: ''
      },
      ...(attempt.mmproj
        ? {
            [attempt.mmproj]: {
              provider: attempt.provider,
              repo_id: attempt.repoId,
              file_path: attempt.mmproj,
              total_bytes: 0,
              downloaded: 0,
              percent: 0,
              status: 'waiting',
              error: ''
            }
          }
        : {})
    }
  }

  /** 一键重试：用上次留底的参数原样重发；Go 侧已保留断点，自动从已下载字节处续传 */
  async function retry() {
    const attempt = lastAttempt.value
    if (!attempt || hasActive.value || retrying.value) return
    retrying.value = true
    try {
      recordAttempt(attempt)
      await wails.downloadHubModel(attempt.provider, attempt.repoId, attempt.mainFile, attempt.mmproj)
    } catch (e) {
      logError('重试下载发起失败', e)
    } finally {
      retrying.value = false
    }
  }

  /** 重启应用（自动加载不可用/失败时的兜底，让下载完成的模型生效） */
  async function restartApp() {
    restarting.value = true
    try {
      await wails.restartApp()
    } catch (e) {
      restarting.value = false
      logError('重启应用失败', e)
    }
  }

  function dismiss() {
    dismissed.value = true
  }

  // ===== 事件订阅（store 单例，全应用只订阅一次，随应用生命周期存续） =====
  wails.subscribeModelDownloadProgress(p => {
    progressMap.value[p.file_path] = p
  })
  wails.subscribeModelDownloadComplete(result => {
    if (result.success) {
      // 成功：进度条功成身退，悬浮卡切换为完成/自动加载状态
      progressMap.value = {}
    }
    // 失败：保留进度项（含错误详情）供悬浮卡展示
    lastComplete.value = result
    dismissed.value = false
  })

  return {
    progressMap,
    lastAttempt,
    lastComplete,
    retrying,
    restarting,
    activeItems,
    hasActive,
    modelReady,
    activateFailed,
    visible,
    recordAttempt,
    retry,
    restartApp,
    dismiss
  }
})
