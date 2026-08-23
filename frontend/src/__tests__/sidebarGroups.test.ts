import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, type App as VueApp } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import Sidebar from '../components/layout/Sidebar.vue'
import { useChatStore } from '../stores/chat'
import type { Conversation } from '../services/wails'

vi.mock('naive-ui', () => ({
  NIcon: { name: 'NIconStub', template: '<span><slot /></span>' },
  NInput: {
    name: 'NInputStub',
    template: '<div class="n-input-stub"><slot /></div>'
  },
  NDropdown: { name: 'NDropdownStub', template: '<div />' },
  useDialog: () => ({ create: vi.fn() }),
  useMessage: () => ({ success: vi.fn(), error: vi.fn() })
}))

// showSuccess 内部走 naive-ui discrete API，测试中桩掉避免副作用
vi.mock('../utils/showError', () => ({
  showSuccess: vi.fn(),
  showError: vi.fn()
}))

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

async function flushRendering() {
  await Promise.resolve()
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

describe('Sidebar 会话日期分组', () => {
  let app: VueApp<Element> | null = null
  let host: HTMLDivElement

  beforeEach(() => {
    host = document.createElement('div')
    document.body.appendChild(host)
  })

  afterEach(() => {
    if (app) app.unmount()
    app = null
    host.remove()
    vi.restoreAllMocks()
  })

  function mountSidebar(): ReturnType<typeof useChatStore> {
    const pinia = createPinia()
    setActivePinia(pinia)

    // 模板 footer 使用全局 $router，注入桩实现避免引入完整 router
    const Host = defineComponent({
      name: 'SidebarHost',
      setup() {
        return () => h(Sidebar, { collapsed: false })
      }
    })

    app = createApp(Host)
    app.use(pinia)
    ;(app.config.globalProperties as { $router?: unknown }).$router = { push: vi.fn() }
    app.mount(host)
    return useChatStore()
  }

  it('按日历天归入 今天/昨天/最近 7 天/更早 四组并按序渲染', async () => {
    const store = mountSidebar()
    store.conversations = [
      makeConv('c-old', '十天前', daysAgo(10)),
      makeConv('c-today', '今天写的', new Date()),
      makeConv('c-week', '三天前', daysAgo(3)),
      makeConv('c-yesterday', '昨天深夜', daysAgo(1))
    ]
    await flushRendering()

    const labels = Array.from(host.querySelectorAll('.group-label')).map(el =>
      el.textContent?.trim()
    )
    expect(labels).toEqual(['今天', '昨天', '最近 7 天', '更早'])

    const groups = host.querySelectorAll('.conversation-group')
    expect(groups.length).toBe(4)

    const firstGroupItems = groups[0].querySelectorAll('.conversation-item-title')
    expect(firstGroupItems[0].textContent?.trim()).toBe('今天写的')

    // 组内保持传入排序：更早组里"十天前"在前
    const lastGroupItems = groups[3].querySelectorAll('.conversation-item-title')
    expect(lastGroupItems[0].textContent?.trim()).toBe('十天前')
  })

  it('空组不出现标题；无效日期安全回落到"更早"', async () => {
    const store = mountSidebar()
    store.conversations = [
      makeConv('c-a', '有效今天', new Date()),
      makeConv('c-b', '坏时间戳', 'not-a-date')
    ]
    await flushRendering()

    const labels = Array.from(host.querySelectorAll('.group-label')).map(el =>
      el.textContent?.trim()
    )
    // 昨天与最近7天两组无数据，不应出现空组标题
    expect(labels).toEqual(['今天', '更早'])

    const olderGroup = host.querySelectorAll('.conversation-group')[1]
    expect(olderGroup.querySelector('.conversation-item-title')?.textContent?.trim()).toBe(
      '坏时间戳'
    )
  })

  it('stagger 索引跨组连续递增且封顶 12', async () => {
    const store = mountSidebar()
    store.conversations = [
      ...Array.from({ length: 14 }, (_, i) => makeConv(`t-${i}`, `今天${i}`, new Date())),
      makeConv('o-0', '更早独苗', daysAgo(30))
    ]
    await flushRendering()

    const allItems = host.querySelectorAll('.conversation-item')
    expect(allItems.length).toBe(15)

    const firstIdx = Number((allItems[0] as HTMLElement).style.getPropertyValue('--stagger-idx'))
    const lastTodayIdx = Number(
      (allItems[13] as HTMLElement).style.getPropertyValue('--stagger-idx')
    )
    const olderIdx = Number((allItems[14] as HTMLElement).style.getPropertyValue('--stagger-idx'))

    expect(firstIdx).toBe(0)
    // 第 14 个今天的会话索引已到 13，被封顶在 12
    expect(lastTodayIdx).toBe(12)
    // 更早组的会话继续沿用连续递增后的封顶值
    expect(olderIdx).toBe(12)
  })
})
