/**
 * 推测解码智能提醒决策工具
 *
 * 设计目标：
 *  - 把"是否应该弹出模型加载完成通知"的判定逻辑从 UI 组件中抽出
 *  - 纯函数 + 无副作用，便于单元测试覆盖所有分支
 *
 * 决策条件（全部满足才弹通知）：
 *  1. 用户开关 adviceEnabled = true（尊重用户关闭意愿）
 *  2. 模型从 not ready → ready（边沿触发，避免每次轮询都弹）
 *  3. 后端返回了 specAdvice 数据
 *  4. 该 sidecar 类型未在 dismissedKeys 中（同 sidecar 只提醒一次）
 *
 * 生活类比：像一个"门铃"——
 *  - 主人开了门铃开关（adviceEnabled）
 *  - 客人刚到（currReady 从 false → true 的边沿）
 *  - 客人手里确实有礼物（specAdvice 非空）
 *  - 而且这位客人之前没按过门铃（未 dismiss）
 *  四个条件齐了，门铃才会响。
 */
import type { SpecAdvice } from '../types/chat'

export interface SpecAdviceDecision {
  /** 是否应该弹出通知 */
  shouldShow: boolean
  /** 用于 localStorage 记录已 dismiss 的 key（仅在 shouldShow=true 时有意义） */
  dismissKey: string
}

/**
 * 决定是否应该弹出推测解码通知
 *
 * @param opts.prevReady 上一次 serverStatus.model_ready
 * @param opts.currReady 当前 serverStatus.model_ready
 * @param opts.specAdvice 后端返回的建议数据
 * @param opts.adviceEnabled 用户开关
 * @param opts.dismissedKeys 已 dismiss 过的 key 列表（来自 localStorage）
 */
export function decideSpecAdviceNotification(opts: {
  prevReady: boolean
  currReady: boolean
  specAdvice: SpecAdvice | null
  adviceEnabled: boolean
  dismissedKeys: string[]
}): SpecAdviceDecision {
  const { prevReady, currReady, specAdvice, adviceEnabled, dismissedKeys } = opts

  // 条件1：用户开关必须开
  if (!adviceEnabled) {
    return { shouldShow: false, dismissKey: '' }
  }
  // 条件2：必须是 false → true 的边沿触发
  if (!currReady || prevReady) {
    return { shouldShow: false, dismissKey: '' }
  }
  // 条件3：后端必须返回建议数据
  if (!specAdvice) {
    return { shouldShow: false, dismissKey: '' }
  }
  // 条件4：同 sidecar 类型只提醒一次
  const dismissKey = `spec_advice:${specAdvice.sidecar}`
  if (dismissedKeys.includes(dismissKey)) {
    return { shouldShow: false, dismissKey: '' }
  }

  return { shouldShow: true, dismissKey }
}

// ----- localStorage 持久化（已 dismiss 的 sidecar 列表） -----
// 用一个 key 存 JSON array，避免 localStorage key 散落各处
const STORAGE_KEY = 'douya:spec_advice_dismissed'

/**
 * 读取已 dismiss 过的 sidecar key 列表
 *
 * 异常容错：localStorage 不可用（如隐身模式）或 JSON 损坏时返回空数组，
 * 让决策函数进入"未 dismiss"分支，最坏情况是再弹一次通知，不会阻塞功能。
 */
export function getSpecAdviceDismissedKeys(): string[] {
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter(k => typeof k === 'string') : []
  } catch {
    return []
  }
}

/**
 * 记录一个 dismissKey，避免后续重复弹窗
 *
 * 幂等：重复添加同一个 key 不会产生重复项。
 */
export function recordSpecAdviceDismissed(dismissKey: string): void {
  if (!dismissKey) return
  try {
    const keys = getSpecAdviceDismissedKeys()
    if (keys.includes(dismissKey)) return
    keys.push(dismissKey)
    localStorage.setItem(STORAGE_KEY, JSON.stringify(keys))
  } catch {
    // localStorage 写入失败静默忽略（隐身模式 / 配额满都不影响核心功能）
  }
}
