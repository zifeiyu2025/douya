import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useChatStore } from '../stores/chat'
import type { StreamEvent, Conversation, Message } from '../services/wails'

vi.mock('../services/wails', async importOriginal => {
  const actual = (await importOriginal()) as Record<string, unknown>
  return {
    ...actual,
    wails: {
      getConversations: vi.fn().mockResolvedValue([]),
      getMessages: vi.fn().mockResolvedValue([]),
      sendMessage: vi.fn(),
      stopGeneration: vi.fn(),
      createConversation: vi
        .fn()
        .mockResolvedValue({ id: 'conv-1', title: '新对话', created_at: '', updated_at: '' }),
      renameConversation: vi.fn(),
      deleteConversation: vi.fn(),
      searchMessages: vi.fn().mockResolvedValue([]),
      exportConversation: vi.fn(),
      getConfig: vi.fn().mockResolvedValue({}),
      getCleanupResult: vi.fn().mockResolvedValue([]),
      updateConfig: vi.fn(),
      getServerStatus: vi.fn().mockResolvedValue({ running: false }),
      restartServer: vi.fn(),
      deleteMessage: vi.fn(),
      regenerateMessage: vi.fn(),
      getAvailableModels: vi.fn().mockResolvedValue([]),
      switchModel: vi.fn(),
      // F-1.10：subscribe 函数返回 unsubscribe，替代原 onXxx/offXxx 配对
      subscribeChatStream: vi.fn().mockReturnValue(() => {}),
      subscribeServerStatus: vi.fn().mockReturnValue(() => {}),
      subscribeSwitchProgress: vi.fn().mockReturnValue(() => {}),
      subscribeAbnormalCleanup: vi.fn().mockReturnValue(() => {}),
      prepareShutdown: vi.fn(),
      getLastPromptTokens: vi.fn().mockResolvedValue(0)
    }
  }
})

function setupGeneratingState(store: ReturnType<typeof useChatStore>, convId: string = '') {
  store.generatingConvId = convId
  store.convStreamingStates.set(convId, {
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
    tokensPerSecond: 0,
    predictedN: 0,
    promptProgress: null
  })
}

describe('chat store - handleStreamEvent', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
  })

  it('should handle token event', async () => {
    // handleToken 使用 20ms 节流刷新（scheduleStreamingFlush）：
    // 分块 push 到 streamingChunks 后，由 20ms 定时器合并到 streamingContent，
    // 不会同步更新。需用 fake timers 推进时间让 flush 触发后再校验。
    vi.useFakeTimers()
    const store = useChatStore()
    setupGeneratingState(store)

    store.handleStreamEvent({ type: 'token', content: '你' } as StreamEvent)
    store.handleStreamEvent({ type: 'token', content: '好' } as StreamEvent)

    await vi.advanceTimersByTimeAsync(20)

    expect(store.streamingContent).toBe('你好')
    vi.useRealTimers()
  })

  it('should handle thinking event', async () => {
    // handleThinking 首个 token 立即 flush，后续 token 走 20ms 节流（scheduleStreamingFlush），
    // 需用 fake timers 推进时间让第二个 token 的 flush 触发后再校验。
    vi.useFakeTimers()
    const store = useChatStore()
    setupGeneratingState(store)

    store.handleStreamEvent({ type: 'thinking', content: '分析中' } as StreamEvent)
    store.handleStreamEvent({ type: 'thinking', content: '...' } as StreamEvent)

    await vi.advanceTimersByTimeAsync(20)
    expect(store.thinkingContent).toBe('分析中...')
    vi.useRealTimers()
  })

  it('should handle search_start event', () => {
    const store = useChatStore()
    setupGeneratingState(store)

    store.handleStreamEvent({ type: 'search_start', content: 'test query' } as StreamEvent)

    expect(store.isSearching).toBe(true)
  })

  it('should handle search_result event (canonical tool_call_id+results payload)', () => {
    const store = useChatStore()
    setupGeneratingState(store)

    // C-7 协议唯一事实化：后端统一发射 { tool_call_id, results } 结构
    const results = [{ title: '测试结果', url: 'https://example.com', snippet: '摘要' }]
    store.handleStreamEvent({
      type: 'search_result',
      content: { tool_call_id: 'search_pre_1', results }
    } as StreamEvent)

    expect(store.isSearching).toBe(false)
    expect(store.searchResults).toBe(JSON.stringify(results))
  })

  it('should clear searchResults on empty search_result payload', () => {
    const store = useChatStore()
    setupGeneratingState(store)
    // 预置旧结果，验证空结果事件会清空展示
    const state = store.convStreamingStates.get('')
    if (state) state.searchResults = '[{"title":"old"}]'

    store.handleStreamEvent({
      type: 'search_result',
      content: { tool_call_id: 'search_pre_2', results: [] }
    } as StreamEvent)

    expect(store.isSearching).toBe(false)
    expect(store.searchResults).toBe('')
  })

  it('should handle done event', () => {
    const store = useChatStore()
    setupGeneratingState(store)
    const state = store.convStreamingStates.get('')!
    state.streamingContent = 'test'
    state.thinkingContent = 'think'

    store.handleStreamEvent({ type: 'done', content: null } as StreamEvent)

    expect(store.isGenerating).toBe(false)
    expect(store.streamingContent).toBe('')
    expect(store.thinkingContent).toBe('')
  })

  it('should handle conversation_created event', () => {
    const store = useChatStore()

    const conv = {
      id: 'conv-new',
      title: '新对话',
      created_at: '2026-05-06T14:30:00+08:00',
      updated_at: '2026-05-06T14:30:00+08:00'
    }
    store.handleStreamEvent({ type: 'conversation_created', content: conv } as StreamEvent)

    expect(store.currentConversationId).toBe('conv-new')
  })

  // -------------------------------------------------------------------------
  // Bug 复现：思考结束→正文切换的竞态
  // 场景：思考期间 thinkingContent 累积，isThinking=true。
  //   第一个正文 token 到达时 handleToken 把 isThinking=false，但 streamingContent
  //   由 50ms 节流定时器刷新，不会同步更新——存在 streamingContent="" 的窗口。
  //   此时组件层 thinkingAsContent = !isThinking && thinkingContent && !streamingContent
  //   会短暂为 true，把思考内容当正文显示，吞掉首个正文 token 的视觉响应。
  // 期望：第一个正文 token 到达时立即 flush streamingContent，消除空窗口。
  // -------------------------------------------------------------------------
  it('思考结束首个正文 token 应立即 flush streamingContent，消除空窗口', async () => {
    vi.useFakeTimers()
    const store = useChatStore()
    setupGeneratingState(store)
    const state = store.convStreamingStates.get('')!

    // 模拟思考阶段
    store.handleStreamEvent({ type: 'thinking', content: '分析中' } as StreamEvent)
    expect(state.isThinking).toBe(true)
    expect(state.thinkingContent).toBe('分析中')
    expect(state.streamingContent).toBe('')

    // 第一个正文 token 到达：handleToken 把 isThinking=false
    store.handleStreamEvent({ type: 'token', content: '你' } as StreamEvent)

    // 此时 isThinking 已变 false
    expect(state.isThinking).toBe(false)
    // Bug：streamingContent 仍为空（50ms 节流未刷新），
    // 导致组件层 thinkingAsContent = !isThinking && thinkingContent && !streamingContent 短暂为 true
    // 期望：首个 token 应立即 flush，streamingContent 非空
    expect(state.streamingContent).toBe('你')

    vi.useRealTimers()
  })

  it('连续多个 token 到达时 flush 定时器不被清空，token 合并显示', async () => {
    // 回归测试：修复前 startGeneratingTimeout 每次调用 clearTimers() 会把 flush 定时器也清掉，
    // 导致 token 间隔 < flush 间隔时 flush 永远不触发，内容堆积到最后一次性显示。
    // FLUSH_INTERVAL=0 后行为：后续 token 在下一个宏任务 flush（setTimeout 0）
    vi.useFakeTimers()
    const store = useChatStore()
    setupGeneratingState(store)
    const state = store.convStreamingStates.get('')!

    // 首 token 立即 flush（0ms）
    store.handleStreamEvent({ type: 'token', content: '第' } as StreamEvent)
    expect(state.streamingContent).toBe('第')

    // 后续 token 连续到达，设置 flush 定时器（setTimeout 0）
    store.handleStreamEvent({ type: 'token', content: '一' } as StreamEvent)
    // 定时器已存在，不重置
    store.handleStreamEvent({ type: 'token', content: '句' } as StreamEvent)
    store.handleStreamEvent({ type: 'token', content: '话' } as StreamEvent)

    // 定时器尚未触发（还在同一宏任务中），streamingContent 仍是首字
    expect(state.streamingContent).toBe('第')

    // 推进时间触发 flush 定时器，合并所有 chunk
    await vi.advanceTimersByTimeAsync(1)

    // 修复前：flush 定时器被 startGeneratingTimeout 每次清空，永远不触发，
    //         streamingContent 仍为 '第'，所有内容堆积在 chunks 中
    // 修复后：flush 定时器正常触发，streamingContent 合并所有 chunk
    expect(state.streamingContent).toBe('第一句话')

    vi.useRealTimers()
  })

  it('should handle conversation_updated event', () => {
    const store = useChatStore()

    store.conversations = [
      {
        id: 'conv-1',
        title: '新对话',
        created_at: '2026-05-06T14:30:00+08:00',
        updated_at: '2026-05-06T14:30:00+08:00'
      }
    ] as Conversation[]

    store.handleStreamEvent({
      type: 'conversation_updated',
      content: {
        id: 'conv-1',
        title: '更新后的标题',
        created_at: '2026-05-06T14:30:00+08:00',
        updated_at: '2026-05-06T15:00:00+08:00'
      }
    } as StreamEvent)

    expect(store.conversations[0].title).toBe('更新后的标题')
    expect(store.conversations[0].updated_at).toBe('2026-05-06T15:00:00+08:00')
  })

  it('should handle user_message event - new message', () => {
    const store = useChatStore()

    const userMsg = {
      id: 'msg-1',
      conversation_id: 'conv-1',
      role: 'user',
      content: '你好',
      created_at: '2026-05-06T14:30:00+08:00'
    }
    store.handleStreamEvent({ type: 'user_message', content: userMsg } as StreamEvent)

    expect(store.messages.length).toBe(1)
    expect(store.messages[0].content).toBe('你好')
  })

  it('should handle assistant_message event - new message', () => {
    const store = useChatStore()

    const aiMsg = {
      id: 'msg-2',
      conversation_id: 'conv-1',
      role: 'assistant',
      content: '你好！有什么可以帮你的吗？',
      thinking_content: '思考中...',
      created_at: '2026-05-06T14:30:05+08:00'
    }
    store.handleStreamEvent({ type: 'assistant_message', content: aiMsg } as StreamEvent)

    expect(store.messages.length).toBe(1)
    expect(store.messages[0].content).toBe('你好！有什么可以帮你的吗？')
    expect(store.messages[0].thinking_content).toBe('思考中...')
  })

  it('should handle assistant_message event - update existing', () => {
    const store = useChatStore()

    store.messages = [
      {
        id: 'msg-2',
        conversation_id: 'conv-1',
        role: 'assistant',
        content: '',
        created_at: '2026-05-06T14:30:05+08:00'
      }
    ] as Message[]

    const updatedMsg = {
      id: 'msg-2',
      conversation_id: 'conv-1',
      role: 'assistant',
      content: '完整回复内容',
      thinking_content: '思考过程',
      created_at: '2026-05-06T14:30:05+08:00'
    }
    store.handleStreamEvent({ type: 'assistant_message', content: updatedMsg } as StreamEvent)

    expect(store.messages.length).toBe(1)
    expect(store.messages[0].content).toBe('完整回复内容')
  })

  it('should handle error event', () => {
    const store = useChatStore()
    setupGeneratingState(store)
    const state = store.convStreamingStates.get('')!
    state.streamingContent = 'partial'

    store.handleStreamEvent({ type: 'error', content: 'connection failed' } as StreamEvent)

    expect(store.isGenerating).toBe(false)
    expect(store.streamingContent).toBe('')
  })

  it('should handle time fields as strings in conversation events', () => {
    const store = useChatStore()

    store.conversations = []

    const conv = {
      id: 'conv-time-test',
      title: '时间测试',
      created_at: '2026-05-06T14:30:00+08:00',
      updated_at: '2026-05-06T14:30:00+08:00'
    }
    store.handleStreamEvent({ type: 'conversation_created', content: conv } as StreamEvent)

    expect(store.currentConversationId).toBe('conv-time-test')
  })
})
