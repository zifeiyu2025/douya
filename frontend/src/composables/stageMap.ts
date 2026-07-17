/**
 * 模型切换阶段进度映射常量（F-1.14 抽取）
 *
 * useAppLifecycle.ts 和 useModelSwitch.ts 原各自维护一份相同的 stageMap，
 * 新增阶段易漏改，抽取为单一常量确保两处一致。
 *
 * 生活类比：像火车站的到站显示屏——每个阶段（idle/loading/done/failed）
 * 对应一个固定的"已行驶百分比"，不管哪个候车室（composable）看都是同一块屏。
 */
import type { SwitchProgressStage } from '../types/settings'

/** 阶段 → 粗略进度百分比映射（仅作为无真实加载进度时的兜底） */
export const STAGE_PERCENT_MAP: Record<SwitchProgressStage, number> = {
  idle: 0,
  preparing: 5,
  loading: 10,
  waiting: 10,
  detecting: 90,
  done: 100,
  failed: 100,
  rolling_back: 50
}
