/**
 * conversationGroups —— 会话列表展示工具（纯函数，无组件依赖）
 *
 * 从旧 Sidebar 抽取的日期分组语义，供会话抽屉与单元测试共用：
 * 1. 按"日历天"而非毫秒差分组：跨午夜后"23 小时前"应显示为"昨天"；
 * 2. 分组顺序固定为 今天 / 昨天 / 最近 7 天 / 更早，空组不出现；
 * 3. 无效时间戳安全回落到"更早"；
 * 4. 组内保持传入排序；stagger 入场索引跨组连续递增且封顶 12。
 */
import type { Conversation } from '../services/wails'

export type DateGroupKey = 'today' | 'yesterday' | 'week' | 'older'

export const GROUP_LABELS: Record<DateGroupKey, string> = {
  today: '今天',
  yesterday: '昨天',
  week: '最近 7 天',
  older: '更早'
}

const GROUP_ORDER: DateGroupKey[] = ['today', 'yesterday', 'week', 'older']

/** stagger 上限：长列表入场动画到第 13 项起不再递增延迟 */
const STAGGER_CAP = 12

/** 取某天的零点时间戳（本地时区），用于按日历天比较 */
function startOfDay(date: Date): number {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime()
}

/** 把更新时间归类到分组键；解析失败时安全回落到"更早" */
export function classifyByDate(dateStr: string): DateGroupKey {
  const d = new Date(dateStr)
  const dayDiff = Math.floor((startOfDay(new Date()) - startOfDay(d)) / (24 * 60 * 60 * 1000))
  if (Number.isNaN(dayDiff)) return 'older'
  if (dayDiff <= 0) return 'today'
  if (dayDiff === 1) return 'yesterday'
  if (dayDiff < 7) return 'week'
  return 'older'
}

export interface ConversationGroupItem {
  conv: Conversation
  staggerIdx: number
}

export interface ConversationGroup {
  key: DateGroupKey
  label: string
  items: ConversationGroupItem[]
}

/** 过滤后的会话按日期分组；空组不出现，组内保持原排序，stagger 索引跨组连续递增 */
export function groupConversationsByDate(conversations: Conversation[]): ConversationGroup[] {
  const buckets = new Map<DateGroupKey, Conversation[]>()
  for (const conv of conversations) {
    const key = classifyByDate(conv.updated_at)
    const list = buckets.get(key)
    if (list) list.push(conv)
    else buckets.set(key, [conv])
  }
  let staggerIdx = 0
  return GROUP_ORDER.filter(key => buckets.has(key)).map(key => ({
    key,
    label: GROUP_LABELS[key],
    items: buckets
      .get(key)!
      .map(conv => ({ conv, staggerIdx: Math.min(staggerIdx++, STAGGER_CAP) }))
  }))
}

/** 相对时间文案：刚刚 / N 分钟前 / HH:mm / 昨天 / 周X / M/D */
export function formatRelativeTime(dateStr: string): string {
  const d = new Date(dateStr)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  const oneDay = 24 * 60 * 60 * 1000

  if (diff < 60000) return '刚刚'
  if (diff < 3600000) return `${Math.floor(diff / 60000)} 分钟前`
  if (diff < oneDay) {
    const hour = String(d.getHours()).padStart(2, '0')
    const minute = String(d.getMinutes()).padStart(2, '0')
    return `${hour}:${minute}`
  }
  if (diff < oneDay * 2) return '昨天'
  if (diff < oneDay * 7) {
    const weekDays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
    return weekDays[d.getDay()]
  }
  return `${d.getMonth() + 1}/${d.getDate()}`
}
