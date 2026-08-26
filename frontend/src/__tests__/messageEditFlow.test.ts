import { afterEach, beforeEach, describe, expect, it, vi, type Mock } from 'vitest'
import { createApp, defineComponent, h, nextTick, type App as VueApp } from 'vue'
import { createPinia, setActivePinia } from 'pinia'
import MessageItem from '../components/chat/MessageItem.vue'
import { useChatStore } from '../stores/chat'
import { useSettingsStore } from '../stores/settings'
import { wails, DEFAULT_CONFIG, type Message } from '../services/wails'

vi.mock('naive-ui', () => ({
  NButton: { name: 'NButtonStub', template: '<button><slot /></button>' },
  useMessage: () => ({ success: vi.fn(), error: vi.fn() })
}))

vi.mock('../utils/markdown', () => ({
  renderMarkdown: vi.fn(async (content: string) => `<div>${content}</div>`),
  escapeHtml: vi.fn((text: string) => text)
}))

vi.mock('../components/chat/ThinkBlock.vue', () => ({
  default: { name: 'ThinkBlockStub', template: '<div class="think-block-stub" />' }
}))
vi.mock('../components/chat/SearchStatus.vue', () => ({
  default: { name: 'SearchStatusStub', template: '<div class="search-status-stub" />' }
}))
vi.mock('../components/ui/AppIcon.vue', () => ({
  default: { name: 'AppIconStub', template: '<span class="app-icon-stub" />' }
}))

// TTS composable 涉及后端音频管线与事件订阅，测试中按其真实返回接口整体桩化
vi.mock('../composables/useTTS', () => ({
  useTTS: () => ({
    isSupported: { value: false },
    currentBackend: { value: null },
    isSpeaking: vi.fn(() => false),
    speak: vi.fn(async () => undefined),
    stop: vi.fn()
  })
}))

function createUserMessage(content: string): Message {
  return {
    id: 'user-1',
    conversation_id: 'conv-1',
    role: 'user',
    content,
    search_results: '',
    created_at: '2026-06-24T00:00:00Z'
  }
}

async function flushRendering() {
  await Promise.resolve()
  await nextTick()
  await Promise.resolve()
  await nextTick()
}

describe('MessageItem 用户消息编辑流', () => {
  let app: VueApp<Element> | null = null
  let host: HTMLDivElement
  let editMessageSpy: Mock<(messageID: string, newContent: string) => Promise<void>>

  beforeEach(() => {
    host = document.createElement('div')
    document.body.appendChild(host)

    const pinia = createPinia()
    setActivePinia(pinia)

    const settingsStore = useSettingsStore()
    settingsStore.config = { ...DEFAULT_CONFIG }

    // wails.editMessage 是保存落库的唯一出口，spy 阻断真实后端调用
    editMessageSpy = vi.fn(async (_messageID: string, _newContent: string) => undefined)
    vi.spyOn(wails, 'editMessage').mockImplementation(editMessageSpy)

    const Host = defineComponent({
      name: 'EditFlowHost',
      setup() {
        return () => h(MessageItem, { message: createUserMessage('原始问题') })
      }
    })

    app = createApp(Host)
    app.use(pinia)
    app.mount(host)
  })

  afterEach(() => {
    if (app) app.unmount()
    app = null
    host.remove()
    vi.restoreAllMocks()
  })

  async function enterEditMode(): Promise<HTMLTextAreaElement> {
    await flushRendering()
    const editBtn = host.querySelector<HTMLButtonElement>('.action-btn[title="编辑"]')
    expect(editBtn).not.toBeNull()
    editBtn!.click()
    await flushRendering()
    const textarea = host.querySelector<HTMLTextAreaElement>('.edit-textarea')
    expect(textarea).not.toBeNull()
    return textarea!
  }

  it('保存修改后的内容：落库 + 本地同步 + 截断重生成', async () => {
    const store = useChatStore()
    store.messages = [createUserMessage('原始问题')]
    const regenerateSpy = vi.spyOn(store, 'regenerateMessage').mockResolvedValue(undefined)

    const textarea = await enterEditMode()
    expect(textarea.value).toBe('原始问题')

    textarea.value = '修改后的问题'
    textarea.dispatchEvent(new Event('input'))
    await flushRendering()

    const saveBtn = Array.from(host.querySelectorAll('button')).find(b =>
      b.textContent?.includes('保存并重新生成')
    )
    expect(saveBtn).toBeDefined()
    saveBtn!.click()
    await flushRendering()

    expect(editMessageSpy).toHaveBeenCalledWith('user-1', '修改后的问题')
    expect(regenerateSpy).toHaveBeenCalledTimes(1)
    expect(regenerateSpy.mock.calls[0][0]).toBe('user-1')
    // 本地消息内容立即同步，重生成期间 UI 反映新文本
    expect(store.messages[0].content).toBe('修改后的问题')
    // 保存成功后退出编辑态
    expect(host.querySelector('.edit-textarea')).toBeNull()
  })

  it('空内容保存被拒绝：不落库、静默退出编辑态', async () => {
    const store = useChatStore()
    store.messages = [createUserMessage('原始问题')]

    const textarea = await enterEditMode()
    textarea.value = '   '
    textarea.dispatchEvent(new Event('input'))
    await flushRendering()

    const saveBtn = Array.from(host.querySelectorAll('button')).find(b =>
      b.textContent?.includes('保存并重新生成')
    )
    saveBtn!.click()
    await flushRendering()

    expect(editMessageSpy).not.toHaveBeenCalled()
    expect(store.messages[0].content).toBe('原始问题')
    expect(host.querySelector('.edit-textarea')).toBeNull()
  })

  it('任意会话开始生成时强制退出编辑态', async () => {
    const store = useChatStore()
    store.messages = [createUserMessage('原始问题')]

    const textarea = await enterEditMode()
    expect(textarea).not.toBeNull()

    // 模拟另一会话进入生成状态（isAnyGenerating 变 true）
    store.generatingConvId = 'conv-other'
    store.convStreamingStates.set('conv-other', {
      isGenerating: true,
      streamingContent: '',
      streamingChunks: [],
      thinkingContent: '',
      thinkingChunks: [],
      searchResults: '',
      isSearching: false,
      isThinking: false,
      thinkingStartTime: 0,
      thinkingDuration: 0,
      searchQuery: '',
      searchError: '',
      contextTrimmed: null,
      outputTruncated: false,
      tokensPerSecond: 0,
      predictedN: 0,
      promptProgress: null
    })
    await flushRendering()

    expect(host.querySelector('.edit-textarea')).toBeNull()
  })
})
