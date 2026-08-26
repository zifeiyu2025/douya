/**
 * 模型参考配置常量
 *
 * 从 SettingsView.vue 抽取，供设置页面显示模型推荐参数。
 * 包含各模型的官方推荐采样参数（温度、Top P、Top K 等）。
 *
 * 70 个模型的参数数据本体已迁移至 modelRefs.data.json（约 90KB 纯数据），
 * 此文件仅保留类型契约与统一导出，消费方导入路径与 API 完全不变；
 * 构建时 JSON 由 vite.config.ts codeSplitting.groups 拆为独立 model-refs 分块。
 */
import refsData from './modelRefs.data.json'

/** 模型参考参数的采样配置（raw 和 raw_thinking 共用） */
export interface ModelRefRaw {
  temperature: number
  top_p: number
  top_k: number
  context_size: number
  repeat_penalty: number
}

export interface ModelRefConfig {
  name: string
  raw: ModelRefRaw
  raw_thinking?: ModelRefRaw
  params: { label: string; value: string }[]
  params_thinking?: { label: string; value: string }[]
  note?: string
}

export const MODEL_REFS: Record<string, ModelRefConfig> = refsData
