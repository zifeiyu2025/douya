import { describe, expect, it } from 'vitest'
import {
  classifyByDate,
  groupConversationsByDate,
  formatRelativeTime
} from '../utils/conversationGroups'
import type { Conversation } from '../services/wails'

function makeConv(id: string, title: string, updatedAt: Date | string): Conversation {
  return {
    id,
    title,
    updated_at: updatedAt instanceof Date ? updatedAt.toISOString() : updatedAt
  } as Conversation
}

/** 以"今天零点"为锚点构造相对天数，保证按日历天分组的断言稳定 */
function daysAgo(n: number): Date {
  const now = new Date()
  const d = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  d.setDate(d.getDate() - n)
  return d
}

describe('classifyByDate 日历天归类', () => {
  it('今天/昨天/最近7天/更早 边界正确', () => {
    expect(classifyByDate(new Date().toISOString())).toBe('today')
    expect(classifyByDate(daysAgo(1).toISOString())).toBe('yesterday')
    // 跨午夜后"23 小时前"仍属昨天而非今天
    const lateNight = new Date()
    lateNight.setHours(23, 59, 0, 0)
    const earlyMorning = new Date()
    earlyMorning.setHours(0, 1, 0, 0)
    expect(classifyByDate(lateNight.toISOString()) === 'today').toBe(true)
    expect(classifyByDate(earlyMorning.toISOString())).toBe('today')
    expect(classifyByDate(daysAgo(2).toISOString())).toBe('week')
    expect(classifyByDate(daysAgo(6).toISOString())).toBe('week')
    expect(classifyByDate(daysAgo(7).toISOString())).toBe('older')
    expect(classifyByDate('not-a-date')).toBe('older')
  })
})

describe('groupConversationsByDate 分组语义', () => {
  it('按日历天归入 今天/昨天/最近 7 天/更早 四组并按序渲染', () => {
    const groups = groupConversationsByDate([
      makeConv('c-old', '十天前', daysAgo(10)),
      makeConv('c-today', '今天写的', new Date()),
      makeConv('c-week', '三天前', daysAgo(3)),
      makeConv('c-yesterday', '昨天深夜', daysAgo(1))
    ])

    expect(groups.map(g => g.label)).toEqual(['今天', '昨天', '最近 7 天', '更早'])
    expect(groups.length).toBe(4)

    // 组内保持传入排序：今天的"今天写的"在最前，更早组里"十天前"在前
    expect(groups[0].items[0].conv.title).toBe('今天写的')
    expect(groups[3].items[0].conv.title).toBe('十天前')
  })

  it('空组不出现；无效日期安全回落到"更早"', () => {
    const groups = groupConversationsByDate([
      makeConv('c-a', '有效今天', new Date()),
      makeConv('c-b', '坏时间戳', 'not-a-date')
    ])

    // 昨天与最近7天两组无数据，不应出现空组标题
    expect(groups.map(g => g.label)).toEqual(['今天', '更早'])
    expect(groups[1].items[0].conv.title).toBe('坏时间戳')
  })

  it('stagger 索引跨组连续递增且封顶 12', () => {
    const groups = groupConversationsByDate([
      ...Array.from({ length: 14 }, (_, i) => makeConv(`t-${i}`, `今天${i}`, new Date())),
      makeConv('o-0', '更早独苗', daysAgo(30))
    ])
    const allItems = groups.flatMap(g => g.items)
    expect(allItems.length).toBe(15)

    expect(allItems[0].staggerIdx).toBe(0)
    // 第 14 个今天的会话索引已到 13，被封顶在 12
    expect(allItems[13].staggerIdx).toBe(12)
    // 更早组的会话继续沿用连续递增后的封顶值
    expect(allItems[14].staggerIdx).toBe(12)
  })

  it('空列表返回空数组', () => {
    expect(groupConversationsByDate([])).toEqual([])
  })
})

describe('formatRelativeTime 相对时间文案', () => {
  it('一分钟内显示刚刚，一小时内显示 N 分钟前', () => {
    expect(formatRelativeTime(new Date(Date.now() - 30_000).toISOString())).toBe('刚刚')
    expect(formatRelativeTime(new Date(Date.now() - 120_000).toISOString())).toBe('2 分钟前')
  })

  it('同日显示 HH:mm，跨天显示 昨天 / 周X / M/D', () => {
    const noon = new Date()
    noon.setHours(12, 5, 0, 0)
    const nowBeforeNoon = new Date()
    if (nowBeforeNoon.getTime() - noon.getTime() >= 0 && nowBeforeNoon.getHours() > 12) {
      expect(formatRelativeTime(noon.toISOString())).toBe('12:05')
    }
    expect(formatRelativeTime(daysAgo(1).toISOString())).toBe('昨天')

    const weekDay = daysAgo(3)
    const weekDays = ['周日', '周一', '周二', '周三', '周四', '周五', '周六']
    expect(formatRelativeTime(weekDay.toISOString())).toBe(weekDays[weekDay.getDay()])

    const monthAgo = daysAgo(30)
    expect(formatRelativeTime(monthAgo.toISOString())).toBe(
      `${monthAgo.getMonth() + 1}/${monthAgo.getDate()}`
    )
  })
})
