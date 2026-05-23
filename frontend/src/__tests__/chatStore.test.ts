import { describe, it, expect, beforeEach, vi } from 'vitest'
import { setActivePinia, createPinia } from 'pinia'
import { useChatStore } from '../stores/chat'
import type { StreamEvent, Conversation, Message } from '../services/wails'

vi.mock('../services/wails', () => ({
    wails: {
        getConversations: vi.fn().mockResolvedValue([]),
        getMessages: vi.fn().mockResolvedValue([]),
        sendMessage: vi.fn(),
        stopGeneration: vi.fn(),
        createConversation: vi.fn().mockResolvedValue({ id: 'conv-1', title: '新对话', created_at: '', updated_at: '' }),
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
        onChatStream: vi.fn(),
        onServerStatus: vi.fn(),
        onSwitchProgress: vi.fn(),
        offChatStream: vi.fn(),
        offServerStatus: vi.fn(),
        offSwitchProgress: vi.fn(),
        prepareShutdown: vi.fn(),
        onAbnormalCleanup: vi.fn(),
        offAbnormalCleanup: vi.fn(),
    },
}))

function setupGeneratingState(store: ReturnType<typeof useChatStore>, convId: string = '') {
    store.generatingConvId = convId
    store.convStreamingStates.set(convId, {
        isGenerating: true,
        streamingContent: '',
        thinkingContent: '',
        searchResults: '',
        isSearching: false,
        isThinking: false,
        thinkingStartTime: 0,
        thinkingDuration: 0,
        searchQuery: '',
    })
}

describe('chat store - handleStreamEvent', () => {
    beforeEach(() => {
        setActivePinia(createPinia())
    })

    it('should handle token event', () => {
        const store = useChatStore()
        setupGeneratingState(store)

        store.handleStreamEvent({ type: 'token', content: '你' } as StreamEvent)
        store.handleStreamEvent({ type: 'token', content: '好' } as StreamEvent)

        expect(store.streamingContent).toBe('你好')
    })

    it('should handle thinking event', () => {
        const store = useChatStore()
        setupGeneratingState(store)

        store.handleStreamEvent({ type: 'thinking', content: '分析中' } as StreamEvent)
        store.handleStreamEvent({ type: 'thinking', content: '...' } as StreamEvent)

        expect(store.thinkingContent).toBe('分析中...')
    })

    it('should handle search_start event', () => {
        const store = useChatStore()
        setupGeneratingState(store)

        store.handleStreamEvent({ type: 'search_start', content: 'test query' } as StreamEvent)

        expect(store.isSearching).toBe(true)
    })

    it('should handle search_result event with string content', () => {
        const store = useChatStore()
        setupGeneratingState(store)

        const searchResults = JSON.stringify([
            { title: '测试结果', url: 'https://example.com', snippet: '摘要' }
        ])
        store.handleStreamEvent({ type: 'search_result', content: searchResults } as StreamEvent)

        expect(store.isSearching).toBe(false)
        expect(store.searchResults).toBe(searchResults)
    })

    it('should handle search_result event with object content', () => {
        const store = useChatStore()
        setupGeneratingState(store)

        const searchResp = {
            results: [
                { title: '测试结果', url: 'https://example.com', snippet: '摘要' }
            ],
            engine: 'test'
        }
        store.handleStreamEvent({ type: 'search_result', content: searchResp } as StreamEvent)

        expect(store.isSearching).toBe(false)
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
            updated_at: '2026-05-06T14:30:00+08:00',
        }
        store.handleStreamEvent({ type: 'conversation_created', content: conv } as StreamEvent)

        expect(store.currentConversationId).toBe('conv-new')
    })

    it('should handle conversation_updated event', () => {
        const store = useChatStore()

        store.conversations = [
            {
                id: 'conv-1',
                title: '新对话',
                created_at: '2026-05-06T14:30:00+08:00',
                updated_at: '2026-05-06T14:30:00+08:00',
            }
        ] as Conversation[]

        store.handleStreamEvent({
            type: 'conversation_updated',
            content: {
                id: 'conv-1',
                title: '更新后的标题',
                created_at: '2026-05-06T14:30:00+08:00',
                updated_at: '2026-05-06T15:00:00+08:00',
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
            created_at: '2026-05-06T14:30:00+08:00',
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
            created_at: '2026-05-06T14:30:05+08:00',
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
                created_at: '2026-05-06T14:30:05+08:00',
            }
        ] as Message[]

        const updatedMsg = {
            id: 'msg-2',
            conversation_id: 'conv-1',
            role: 'assistant',
            content: '完整回复内容',
            thinking_content: '思考过程',
            created_at: '2026-05-06T14:30:05+08:00',
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
            updated_at: '2026-05-06T14:30:00+08:00',
        }
        store.handleStreamEvent({ type: 'conversation_created', content: conv } as StreamEvent)

        expect(store.currentConversationId).toBe('conv-time-test')
    })
})
