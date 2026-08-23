/**
 * 聊天页采样参数抽屉（改进计划 C-4 第④项）：开关状态 + 草稿 + 写入管线
 *
 * 生活类比：
 *   抽屉像汽车方向盘上的快捷旋钮——不用下车回车库（设置页）也能顺手调空调；
 *   但所有旋钮最终接的都是同一台行车电脑（config.json），调完自动落盘。
 *   因此这里与设置页共享同一条写入管线（getConfig → updateConfig），
 *   两边改的永远是同一份数据，不存在"两处各记各的"。
 *
 * 设计要点：
 *   - isOpen 为模块级单例：ChatToolbar 直接 import openParamsPanel() 打开，
 *     免去 ChatView → ChatInput → ChatToolbar 的 emit 链
 *   - 打开时拉取后端最新 config 填充草稿快照（与 ChatToolbar.handleDeepReasonClick
 *     同理：避免用过期的 settingsStore.config 覆盖其他路径刚写入的修改）
 *   - 滑块拖动会高频触发 update：300ms 防抖合并成一次写入；
 *     抽屉关闭时立即 flush 一次，保证最后一次拖动不丢失
 *   - 写入失败通过 lastError 通知组件层弹 toast（composable 自身不碰 UI）
 */
import { ref, computed } from 'vue'
import { wails } from '../services/wails'
import { useSettingsStore, matchModelRef } from '../stores/settings'
import { MODEL_REFS, type ModelRefConfig } from '../utils/modelRefs'
import { logError } from '../utils/logger'
import type { Config } from '../types/chat'

/** 抽屉可调的六个采样参数键（Config 字段子集） */
export type SamplingKey =
  'temperature' | 'top_p' | 'top_k' | 'min_p' | 'repeat_penalty' | 'dry_multiplier'

interface SliderDef {
  key: SamplingKey
  label: string
  tip: string
  min: number
  max: number
  step: number
  /** 是否存在官方推荐值（min_p / dry_multiplier 无公认推荐，不显示角标） */
  recommendable: boolean
}

/** 六滑块元数据表：文案与量程逐字对齐设置页 AIChatSettings.vue */
export const SAMPLER_SLIDERS: SliderDef[] = [
  {
    key: 'temperature',
    label: 'Temperature',
    tip: '控制回答的随机性。值越低越确定保守，值越高越多样创意。推荐范围 0.3-0.8',
    min: 0,
    max: 2,
    step: 0.01,
    recommendable: true
  },
  {
    key: 'top_p',
    label: 'Top P',
    tip: '核采样。从概率最高的候选词中筛选，只考虑累计概率达到此阈值的词。0.95 表示保留前 95% 概率的词',
    min: 0,
    max: 1,
    step: 0.01,
    recommendable: true
  },
  {
    key: 'top_k',
    label: 'Top K',
    tip: '只从概率最高的 K 个候选词中选择。值越小选择越少越确定，0 表示不限制',
    min: 0,
    max: 100,
    step: 1,
    recommendable: true
  },
  {
    key: 'min_p',
    label: 'Min-P',
    tip: '根据最高概率词动态过滤低概率词。0.05 表示过滤掉概率不到最高词 5% 的候选词',
    min: 0,
    max: 1,
    step: 0.01,
    recommendable: false
  },
  {
    key: 'repeat_penalty',
    label: 'Repeat Penalty',
    tip: '大于 1 时惩罚重复内容，防止 AI 反复说同样的话。1.0 表示不惩罚',
    min: 0,
    max: 2,
    step: 0.01,
    recommendable: true
  },
  {
    key: 'dry_multiplier',
    label: 'DRY Multiplier',
    tip: '防止 AI 重复相同句式。0 表示关闭，大于 0 时值越强越不容易重复',
    min: 0,
    max: 5,
    step: 0.01,
    recommendable: false
  }
]

const SLIDER_KEYS = SAMPLER_SLIDERS.map(s => s.key)

/** ModelRefRaw 中存在官方推荐值的键（与 modelRefs.ts 的五字段求交集，排除 context_size） */
const RECOMMENDABLE_KEYS = ['temperature', 'top_p', 'top_k', 'repeat_penalty'] as const
export type RecommendableKey = (typeof RECOMMENDABLE_KEYS)[number]

type SamplingDraft = Pick<Config, SamplingKey>

// —— 模块级单例状态（跨 ChatToolbar / ParamsPanel 共享同一份） ——
const isOpen = ref(false)
// 草稿初值为中性占位，实际展示值始终由 openParamsPanel() 从后端 config 填充
const draft = ref<SamplingDraft>({
  temperature: 0.7,
  top_p: 0.95,
  top_k: 40,
  min_p: 0.05,
  repeat_penalty: 1.1,
  dry_multiplier: 0
})
const lastError = ref('')

let flushTimer: ReturnType<typeof setTimeout> | null = null

function clearFlushTimer() {
  if (flushTimer) {
    clearTimeout(flushTimer)
    flushTimer = null
  }
}

/** 把草稿六字段合入一份 Config（供 flush 使用） */
function applyDraftTo(cfg: Config) {
  for (const k of SLIDER_KEYS) {
    cfg[k] = draft.value[k]
  }
}

/** 立即落盘：拉最新全量配置 → 只覆盖六字段 → 经 store 统一入口写回 */
async function flush() {
  clearFlushTimer()
  const settingsStore = useSettingsStore()
  try {
    const fullConfig = await wails.getConfig()
    applyDraftTo(fullConfig)
    await settingsStore.updateConfig(fullConfig)
  } catch (e) {
    logError('保存采样参数失败:', e)
    lastError.value = '参数保存失败，请稍后在设置页重试'
  }
}

/** 滑块拖动防抖：停手 300ms 后才真正写一次后端 */
function scheduleFlush() {
  clearFlushTimer()
  flushTimer = setTimeout(() => {
    void flush()
  }, 300)
}

/** 打开抽屉：以后端最新配置填充草稿（失败则降级用 store 缓存，仍可查看编辑） */
export async function openParamsPanel() {
  let source: Config
  try {
    source = await wails.getConfig()
  } catch (e) {
    logError('读取采样参数失败，使用本地缓存:', e)
    source = useSettingsStore().config
  }
  for (const k of SLIDER_KEYS) {
    draft.value[k] = source[k]
  }
  isOpen.value = true
}

/** 关闭抽屉：立即 flush 兜底，确保最后一次拖动不因防抖定时器被丢弃 */
export async function closeParamsPanel() {
  isOpen.value = false
  await flush()
}

/**
 * 采样参数抽屉组合式函数（须在组件 setup 中调用）
 * 与 ParamsPanel.vue 配套；ChatToolbar 只需 import 上面的 openParamsPanel 直调
 */
export function useSamplingSettings() {
  const settingsStore = useSettingsStore()

  // 当前模型的官方参考配置（未匹配到已知模型时为 null → 角标整体隐藏）
  const matchedModelRef = computed<ModelRefConfig | null>(() =>
    matchModelRef(settingsStore.currentModel ?? '', MODEL_REFS)
  )
  const recommendedRaw = computed(() => matchedModelRef.value?.raw ?? null)

  /** 单项回官方推荐值 */
  function setToRecommended(key: SamplingKey) {
    const raw = recommendedRaw.value
    if (!raw || !(RECOMMENDABLE_KEYS as readonly string[]).includes(key)) return
    draft.value[key] = raw[key as RecommendableKey]
    scheduleFlush()
  }

  /** 一键全部回官方推荐值 */
  function applyAllRecommended() {
    const raw = recommendedRaw.value
    if (!raw) return
    for (const k of RECOMMENDABLE_KEYS) {
      draft.value[k] = raw[k]
    }
    scheduleFlush()
  }

  return {
    isOpen,
    draft,
    lastError,
    matchedModelRef,
    recommendedRaw,
    scheduleFlush,
    closeParamsPanel,
    setToRecommended,
    applyAllRecommended
  }
}
