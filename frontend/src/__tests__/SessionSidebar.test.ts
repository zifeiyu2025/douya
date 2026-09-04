import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { createApp, defineComponent, h, nextTick, type App as VueApp } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import SessionSidebar from '../components/layout/SessionSidebar.vue'
import { useChatStore } from '../stores/chat'
import type { Conversation, Message } from '../services/wails'

// 路由仅用于底部导航（知识库/设置），测试中提供桩即可
vi.mock('vue-router', () => ({
  useRouter: () => ({ push: vi.fn() })
}))

// discrete 仅在删除/导出时触发，测试覆盖不到，桩掉避免拉入 naive-ui 真实实例
vi.mock('../utils/discrete', () => ({
  discreteMessage: { success: vi.fn(), error: vi.fn(), warning: vi.fn(), info: vi.fn() },
  discreteDialog: { warning: vi.fn(), error: vi.fn() }
}))

vi.mock('../components/ui/AppIcon.vue', () => ({
  default: {
    name: 'AppIconStub',
    template: '<span class="app-icon-stub" />'
  }
}))

function makeConv(id: string, title: string): Conversation {
  const now = new Date().toISOString()
  return { id, title, created_at: now, updated_at: now }
}

function makeMessage(id: string, convId: string, role: string, content: string): Message {
  return {
    id,
    conversation_id: convId,
    role,
    content,
    search_results: '',
    created_at: new Date().toISOString()
  }
}

async function flushRendering() {
  await Promise.resolve()
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

/** 往搜索框输入关键词（模拟真实输入事件，驱动 v-model） */
function typeKeyword(host: HTMLElement, text: string) {
  const input = host.querySelector('.search-input') as HTMLInputElement
  input.value = text
  input.dispatchEvent(new Event('input', { bubbles: true }))
}

describe('SessionSidebar 会话搜索', () => {
  let app: VueApp<Element> | null = null
  let host: HTMLDivElement
  let chatStore: ReturnType<typeof useChatStore>
  let pinia: ReturnType<typeof createPinia>

  beforeEach(() => {
    host = document.createElement('div')
    document.body.appendChild(host)

    // 组件与测试共享同一个 pinia 实例，保证对 store 的 spy 与组件内读取一致
    pinia = createPinia()
    setActivePinia(pinia)
    chatStore = useChatStore()
    chatStore.conversations = [makeConv('conv-a', '周末计划'), makeConv('conv-b', '模型调优笔记')]
  })

  afterEach(() => {
    if (app && host.firstChild) {
      app.unmount()
    }
    app = null
    host.remove()
    vi.restoreAllMocks()
  })

  function mount() {
    const Host = defineComponent({
      name: 'SessionSidebarHost',
      setup() {
        return () => h(SessionSidebar)
      }
    })
    app = createApp(Host)
    app.use(pinia)
    app.mount(host)
  }

  it('标题过滤：输入关键词仅保留匹配会话，无匹配时展示空态', async () => {
    mount()
    await flushRendering()

    typeKeyword(host, '模型')
    await flushRendering()

    const titles = [...host.querySelectorAll('.conversation-item-title')].map(el =>
      el.textContent?.trim()
    )
    expect(titles).toEqual(['模型调优笔记'])

    typeKeyword(host, '不存在的会话')
    await flushRendering()
    expect(host.querySelector('.empty-hint')?.textContent).toContain('没有匹配的会话')
  })

  it('标题过滤模式下输入关键词后出现全文搜索入口', async () => {
    mount()
    await flushRendering()

    typeKeyword(host, '量子')
    await flushRendering()

    const entry = host.querySelector('.fulltext-entry') as HTMLElement | null
    expect(entry).not.toBeNull()
    expect(entry?.textContent).toContain('量子')
  })

  it('全文搜索：结果按会话聚合展示标题、条数与角色标签', async () => {
    const searchSpy = vi
      .spyOn(chatStore, 'searchMessages')
      .mockResolvedValue([
        makeMessage('m1', 'conv-a', 'user', '量子纠缠实验怎么做'),
        makeMessage('m2', 'conv-a', 'assistant', '量子通信的详细原理如下……'),
        makeMessage('m3', 'conv-b', 'user', '量子计算机的性能上限')
      ])
    mount()
    await flushRendering()

    typeKeyword(host, '量子')
    await flushRendering()
    ;(host.querySelector('.fulltext-entry') as HTMLElement).click()
    await flushRendering()

    expect(searchSpy).toHaveBeenCalledWith('量子')
    // 两个会话各聚合成一组
    const groups = [...host.querySelectorAll('.search-group')]
    expect(groups).toHaveLength(2)
    expect(groups[0].querySelector('.search-group-label')?.textContent).toContain('周末计划')
    expect(groups[0].querySelector('.search-group-count')?.textContent).toContain('2')
    expect(groups[1].querySelector('.search-group-label')?.textContent).toContain('模型调优笔记')
    expect(groups[1].querySelector('.search-group-count')?.textContent).toContain('1')
    // 角色标签正确区分 我 / AI
    expect(host.querySelector('.search-hit-role--user')).not.toBeNull()
    expect(host.querySelector('.search-hit-role--assistant')).not.toBeNull()
  })

  it('点击搜索结果：触发定位到消息并清空搜索', async () => {
    vi.spyOn(chatStore, 'searchMessages').mockResolvedValue([
      makeMessage('m1', 'conv-a', 'user', '量子纠缠实验怎么做')
    ])
    const locateSpy = vi.spyOn(chatStore, 'selectConversationAndLocate').mockResolvedValue()
    mount()
    await flushRendering()

    typeKeyword(host, '量子')
    await flushRendering()
    ;(host.querySelector('.fulltext-entry') as HTMLElement).click()
    await flushRendering()

    const hit = host.querySelector('.search-hit') as HTMLElement
    expect(hit).not.toBeNull()
    hit.click()
    await flushRendering()

    expect(locateSpy).toHaveBeenCalledWith('conv-a', 'm1')
    // 搜索已复位：回到标题过滤模式且输入框清空
    expect((host.querySelector('.search-input') as HTMLInputElement).value).toBe('')
    expect(host.querySelector('.fulltext-entry')).toBeNull()
  })

  it('全文搜索无结果时展示空态', async () => {
    vi.spyOn(chatStore, 'searchMessages').mockResolvedValue([])
    mount()
    await flushRendering()

    typeKeyword(host, '绝对不存在')
    await flushRendering()
    ;(host.querySelector('.fulltext-entry') as HTMLElement).click()
    await flushRendering()

    expect(host.querySelector('.empty-hint')?.textContent).toContain(
      '没有找到包含「绝对不存在」的消息'
    )
  })

  it('搜索结果过滤已删除会话：结果只保留仍存在的会话', async () => {
    const searchSpy = vi
      .spyOn(chatStore, 'searchMessages')
      .mockResolvedValue([
        makeMessage('m1', 'conv-a', 'user', '量子纠缠'),
        makeMessage('m-deleted', 'conv-deleted', 'user', '量子力学历史')
      ])
    mount()
    await flushRendering()

    typeKeyword(host, '量子')
    await flushRendering()
    ;(host.querySelector('.fulltext-entry') as HTMLElement).click()
    await flushRendering()

    expect(searchSpy).toHaveBeenCalled()
    // conv-deleted 不在会话列表中，其命中应被过滤
    expect(host.querySelectorAll('.search-group')).toHaveLength(1)
    expect(host.querySelectorAll('.search-hit')).toHaveLength(1)
  })
})
