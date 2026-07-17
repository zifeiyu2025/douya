// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

import { computed, type ComputedRef } from 'vue'

// PromptProgressData 描述 prompt 处理进度的原始数据。
// 与 chat.ts 中 ConvState.promptProgress 字段类型保持一致。
export interface PromptProgressData {
  total: number // prompt 总 token 数（含缓存命中）
  cache: number // 缓存命中的 token 数
  processed: number // 已处理的 token 数（含缓存命中）
  timeMs: number // 已耗时（毫秒）
}

/**
 * usePromptProgress 提供 prompt 处理进度的派生计算（百分比 + ETA）。
 *
 * 抽取原因（基于 F-1.3+F-3.11）：MessageList.vue 和 TokenCounter.vue 中
 * promptPercent 和 promptEta 计算逻辑完全重复（各约 20 行），提取为 composable 统一维护，
 * 避免一处改漏导致两处显示不一致。
 *
 * 计算说明：
 *   - actualTotal = total - cache（实际需要处理的 token 数，扣除缓存命中）
 *   - actualProcessed = processed - cache（实际已处理的 token 数）
 *   - percent = actualProcessed / actualTotal * 100（向上取整，上限 100）
 *   - eta = elapsedSec * (actualTotal / actualProcessed - 1)（剩余秒数，<1 秒返回 null）
 *
 * 生活类比：像工厂的"进度看板"——原料总量（total）、已入库（cache）、已加工（processed）、
 * 加工耗时（timeMs）这些原始数据扔进来，看板自动算出"完成度 X%"和"预计剩余 Y 秒"。
 *
 * @param promptProgress prompt 进度的 getter 函数（返回 PromptProgressData | null）
 */
export function usePromptProgress(promptProgress: () => PromptProgressData | null): {
  percent: ComputedRef<number>
  eta: ComputedRef<number | null>
} {
  // percent 计算实际处理进度百分比（扣除缓存命中部分）。
  // 返回 0 表示无有效数据或进度未开始。
  const percent = computed(() => {
    const pp = promptProgress()
    if (!pp || pp.total <= 0) return 0
    const actualTotal = pp.total - pp.cache
    const actualProcessed = pp.processed - pp.cache
    if (actualTotal <= 0) return 0
    return Math.min(100, Math.round((actualProcessed / actualTotal) * 100))
  })

  // eta 基于当前处理速度估算剩余秒数。
  // 返回 null 表示数据不足或剩余时间过短（<1 秒），不应展示。
  const eta = computed(() => {
    const pp = promptProgress()
    if (!pp || pp.processed <= 0 || pp.timeMs <= 0) return null
    const actualProcessed = pp.processed - pp.cache
    const actualTotal = pp.total - pp.cache
    if (actualProcessed <= 0 || actualTotal <= 0) return null
    const elapsedSec = pp.timeMs / 1000
    const remaining = elapsedSec * (actualTotal / actualProcessed - 1)
    if (remaining < 1) return null
    return Math.ceil(remaining)
  })

  return { percent, eta }
}
