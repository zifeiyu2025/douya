import { defineStore } from 'pinia'
import { ref, reactive, computed, nextTick } from 'vue'
import { wails, type Conversation, type Message, type StreamEvent, type Attachment } from '../services/wails'
import { fixUtf8 } from '../utils/utf8'
import { useSettingsStore } from './settings'
import type { ConvStreamingState } from '../types/chat'

/** 流式状态创建 */
function createEmptyStreamingState(): ConvStreamingState {
    return {
        isGenerating: false,
        streamingContent: '',
        thinkingContent: '',
        searchResults: '',
        isSearching: false,
        isThinking: false,
        thinkingStartTime: 0,
        thinkingDuration: 0,
        searchQuery: '',
        contextTrimmed: null,
        tokensPerSecond: 0,
        predictedN: 0,
        promptProgress: null,
    }
}

/** 清空流式状态（保留 contextTrimmed,contextTrimmed 是通知性事件） */
function clearConvState(state: ConvStreamingState) {
    state.isGenerating = false
    state.streamingContent = ''
    state.thinkingContent = ''
    state.searchResults = ''
    state.isSearching = false
    state.isThinking = false
    state.thinkingDuration = 0
    state.searchQuery = ''
}

export const useChatStore = defineStore('chat', () => {
    const settingsStore = useSettingsStore()
    const conversations = ref<Conversation[]>([])
    const currentConversationId = ref<string>('')
    const messages = ref<Message[]>([])
    const lastError = ref('')
    const generatingConvId = ref('')
    const convStreamingStates = reactive(new Map<string, ConvStreamingState>())
    const isLoadingConversations = ref(false)
    const waitingFirstToken = ref(false)

    // ----- 集中 timer 管理（防止泄漏/竞态） -----
    const timers: ReturnType<typeof setTimeout>[] = []
    function addTimer(fn: () => void, ms: number) {
        const id = setTimeout(() => {
            const idx = timers.indexOf(id)
            if (idx >= 0) timers.splice(idx, 1)
            fn()
        }, ms)
        timers.push(id)
        return id
    }
    function clearTimers() {
        for (const t of timers) clearTimeout(t)
        timers.length = 0
    }

    function getConvState(convId: string): ConvStreamingState {
        let state = convStreamingStates.get(convId)
        if (!state) {
            state = createEmptyStreamingState()
            convStreamingStates.set(convId, state)
        }
        return state
    }

    const currentConvState = computed<ConvStreamingState>(() => {
        const id = generatingConvId.value || currentConversationId.value
        const state = id ? convStreamingStates.get(id) : undefined
        if (state) return state
        if (generatingConvId.value === '' && convStreamingStates.has('')) {
            return convStreamingStates.get('')!
        }
        return createEmptyStreamingState()
    })

    const isGenerating = computed(() => currentConvState.value.isGenerating)
    const streamingContent = computed(() => currentConvState.value.streamingContent)
    const thinkingContent = computed(() => currentConvState.value.thinkingContent)
    const searchResults = computed(() => currentConvState.value.searchResults)
    const isSearching = computed(() => currentConvState.value.isSearching)
    const isThinking = computed(() => currentConvState.value.isThinking)
    const thinkingDuration = computed(() => currentConvState.value.thinkingDuration)
    const searchQuery = computed(() => currentConvState.value.searchQuery)
    const contextTrimmed = computed(() => currentConvState.value.contextTrimmed)
    const tokensPerSecond = computed(() => currentConvState.value.tokensPerSecond)
    const predictedN = computed(() => currentConvState.value.predictedN)
    const promptProgress = computed(() => currentConvState.value.promptProgress)

    const lastAIMessageId = computed(() => {
        for (let i = messages.value.length - 1; i >= 0; i--) {
            if (messages.value[i].role === 'assistant') {
                return messages.value[i].id
            }
        }
        return ''
    })

    // ----- 计时器封装（每次 start 都会先 clear） -----
    function startGeneratingTimeout() {
        clearTimers()
        const timeout = settingsStore.thinkingEnabled ? 300 * 1000 : 120 * 1000
        addTimer(() => {
            if (generatingConvId.value) {
                const state = getConvState(generatingConvId.value)
                if (state.isGenerating) {
                    clearConvState(state)
                    generatingConvId.value = ''
                    waitingFirstToken.value = false
                    lastError.value = ''
                    nextTick(() => {
                        lastError.value = '生成超时，请重试\n💡 如果频繁超时，可尝试：设置 → 减小上下文长度，或使用更小的模型'
                    })
                }
            }
        }, timeout)
    }

    function startFirstTokenTimeout() {
        clearTimers()
        waitingFirstToken.value = true
        addTimer(() => {
            if (waitingFirstToken.value && generatingConvId.value) {
                const state = getConvState(generatingConvId.value)
                if (state.isGenerating && !state.streamingContent && !state.thinkingContent) {
                    clearConvState(state)
                    generatingConvId.value = ''
                    waitingFirstToken.value = false
                    lastError.value = ''
                    nextTick(() => {
                        lastError.value = 'AI 服务响应超时，请检查服务是否正常运行\n💡 可尝试：等待模型加载完成，或重启应用'
                    })
                }
            }
        }, 60 * 1000)
    }

    function clearFirstTokenOnResponse() {
        clearTimers()
        waitingFirstToken.value = false
    }

    function forceResetGenerating() {
        clearTimers()
        if (generatingConvId.value) {
            const state = getConvState(generatingConvId.value)
            clearConvState(state)
        }
        generatingConvId.value = ''
    }

    // ----- 会话与消息 -----
    async function loadConversations() {
        isLoadingConversations.value = true
        try {
            const convs = await wails.getConversations()
            const newConvs = (convs as Conversation[]).map((c) => ({ ...c, title: fixUtf8(c.title) }))
            const newIdSet = new Set(newConvs.map((c) => c.id))
            const keptOld = conversations.value.filter((c) => !newIdSet.has(c.id))
            conversations.value = [...keptOld, ...newConvs]

            for (const key of convStreamingStates.keys()) {
                if (!newIdSet.has(key) && key !== '') {
                    convStreamingStates.delete(key)
                }
            }

            if (!currentConversationId.value && conversations.value.length > 0) {
                await selectConversation(conversations.value[0].id)
            }
        } catch (e) {
            console.error('加载会话列表失败:', e)
        } finally {
            isLoadingConversations.value = false
        }
    }

    async function selectConversation(id: string) {
        if (id === currentConversationId.value) return

        currentConversationId.value = id
        try {
            messages.value = await wails.getMessages(id) || []
        } catch (e) {
            console.error('加载消息失败:', e)
            messages.value = []
        }

        const state = convStreamingStates.get(id)
        if (state) {
            generatingConvId.value = state.isGenerating ? id : ''
        } else {
            generatingConvId.value = ''
        }
    }

    function handleTerminalEvent(convId: string) {
        clearTimers()
        const state = convStreamingStates.get(convId)
        if (state) {
            clearConvState(state)
        }
        if (convId === '' || generatingConvId.value === convId) {
            generatingConvId.value = ''
        }
    }

    // ----- 流式事件 reducer（独立小函数,易单测） -----

    function handleToken(convId: string, content: any) {
        startGeneratingTimeout()
        clearFirstTokenOnResponse()
        const state = getConvState(convId)
        if (state.isThinking && state.thinkingContent) {
            state.isThinking = false
            state.thinkingDuration = (Date.now() - state.thinkingStartTime) / 1000
        } else if (!state.isThinking && state.thinkingContent && state.thinkingDuration === 0) {
            state.thinkingDuration = (Date.now() - state.thinkingStartTime) / 1000
        }
        state.streamingContent += content
    }

    function handleThinking(convId: string, content: any) {
        startGeneratingTimeout()
        clearFirstTokenOnResponse()
        const state = getConvState(convId)
        if (!state.isThinking && !state.thinkingContent) {
            state.isThinking = true
            state.thinkingStartTime = Date.now()
            state.thinkingDuration = 0
        }
        state.thinkingContent += content
    }

    function handleToolCallStart(convId: string, content: any) {
        startGeneratingTimeout()
        clearFirstTokenOnResponse()
        const state = getConvState(convId)
        state.isSearching = true
        state.searchQuery = content?.query || ''
    }

    function handleSearchStart(convId: string, content: any) {
        startGeneratingTimeout()
        clearFirstTokenOnResponse()
        const state = getConvState(convId)
        state.isSearching = true
        if (typeof content === 'string') {
            try {
                const parsed = JSON.parse(content)
                state.searchQuery = parsed.query || content
            } catch {
                state.searchQuery = content
            }
        }
    }

    function handleSearchResult(convId: string, content: any) {
        startGeneratingTimeout()
        clearFirstTokenOnResponse()
        const state = getConvState(convId)
        state.isSearching = false
        state.searchQuery = ''
        try {
            let c: unknown = content
            if (typeof c === 'string') {
                try { c = JSON.parse(c) } catch { c = null }
            }
            if (Array.isArray(c) && c.length > 0) {
                state.searchResults = JSON.stringify(c)
            } else {
                state.searchResults = ''
            }
        } catch {
            state.searchResults = ''
        }
    }

    /** 处理 token_speed 事件：实时更新生成速度 */
    function handleTokenSpeed(convId: string, content: any) {
        const state = getConvState(convId)
        try {
            let c: unknown = content
            if (typeof c === 'string') {
                try { c = JSON.parse(c) } catch { return }
            }
            const data = c as { tokensPerSecond?: number; predictedN?: number }
            if (data.tokensPerSecond && data.tokensPerSecond > 0) {
                state.tokensPerSecond = data.tokensPerSecond
                state.predictedN = data.predictedN || 0
            }
        } catch { /* 忽略解析错误 */ }
    }

    /** 处理 prompt_progress 事件：实时更新提示词处理进度 */
    function handlePromptProgress(convId: string, content: any) {
        const state = getConvState(convId)
        try {
            let c: unknown = content
            if (typeof c === 'string') {
                try { c = JSON.parse(c) } catch { return }
            }
            const data = c as { total?: number; cache?: number; processed?: number; timeMs?: number }
            if (data.processed && data.processed > 0) {
                state.promptProgress = {
                    total: data.total || 0,
                    cache: data.cache || 0,
                    processed: data.processed || 0,
                    timeMs: data.timeMs || 0,
                }
            }
        } catch { /* 忽略解析错误 */ }
    }

    async function handleTerminalAsync(convId: string) {
        const targetConvId = convId || generatingConvId.value
        if (targetConvId) {
            try {
                const msgs = (await wails.getMessages(targetConvId)) as Message[]
                if (targetConvId === currentConversationId.value || targetConvId === generatingConvId.value) {
                    // 保留 tokens_per_second 数据（数据库中不存储此字段，从 assistant_message 事件获取）
                    const speedMap = new Map<string, number>()
                    for (const m of messages.value) {
                        if (m.tokens_per_second && m.tokens_per_second > 0) {
                            speedMap.set(m.id, m.tokens_per_second)
                        }
                    }
                    for (const m of (msgs || [])) {
                        if (speedMap.has(m.id)) {
                            m.tokens_per_second = speedMap.get(m.id)
                        }
                    }
                    messages.value = msgs || []
                }
                nextTick(() => handleTerminalEvent(convId))
            } catch {
                handleTerminalEvent(convId)
            }
        } else {
            handleTerminalEvent(convId)
        }
        if (convId) {
            // 终态后刷新会话列表
            loadConversations()
        }
    }

    function handleError(convId: string, content: any, isCurrentConv: boolean) {
        handleTerminalEvent(convId)
        if (isCurrentConv || !convId) {
            lastError.value = ''
            nextTick(() => {
                lastError.value = String(content || '生成过程中发生错误，请查看日志了解详情')
            })
        }
    }

    function handleContextTrimmed(convId: string, content: any) {
        const state = getConvState(convId)
        state.contextTrimmed = {
            reason: content?.reason || 'unknown',
            promptTokens: content?.prompt_tokens,
            contextSize: content?.context_size,
            messagesAfter: content?.messages_after,
        }
    }

    function handleConvCreated(content: any) {
        if (!content?.id) return
        const newConvId = content.id
        const oldState = convStreamingStates.get('')
        if (oldState) {
            convStreamingStates.delete('')
            convStreamingStates.set(newConvId, oldState)
        }
        generatingConvId.value = newConvId
        currentConversationId.value = newConvId
        const tempIdx = messages.value.findIndex((m: Message) => m.id.startsWith('temp-'))
        if (tempIdx >= 0) {
            messages.value[tempIdx].conversation_id = newConvId
        }
        conversations.value.unshift({ ...content, title: fixUtf8(content.title) })
    }

    function handleAssistantMsg(content: any, isCurrentConv: boolean) {
        if (!content || !isCurrentConv) return
        const idx = messages.value.findIndex((m: Message) => m.id === content.id)
        if (idx >= 0) {
            messages.value[idx] = content
        } else {
            messages.value.push(content)
        }
    }

    function handleUserMsg(content: any, isCurrentConv: boolean) {
        if (!content || !isCurrentConv) return
        const exists = messages.value.some((m: Message) =>
            m.role === 'user' && m.content === content.content
        )
        if (!exists) {
            messages.value.push(content)
        } else {
            const idx = messages.value.findIndex((m: Message) =>
                m.role === 'user' && m.content === content.content && m.id.startsWith('temp-')
            )
            if (idx >= 0) messages.value[idx] = content
        }
    }

    function handleConvUpdated(content: any) {
        if (!content?.id) return
        const conv = conversations.value.find((c: Conversation) => c.id === content.id)
        if (conv) {
            conv.title = fixUtf8(content.title)
            conv.updated_at = content.updated_at
        }
    }

    function handleConvDeleted(content: any) {
        const deletedId = typeof content === 'string' ? content : content?.id
        if (!deletedId) return
        conversations.value = conversations.value.filter((c: Conversation) => c.id !== deletedId)
        convStreamingStates.delete(deletedId)
        if (generatingConvId.value === deletedId) generatingConvId.value = ''
        if (currentConversationId.value === deletedId) {
            currentConversationId.value = ''
            messages.value = []
        }
    }

    function handleMsgDeleted(content: any, isCurrentConv: boolean) {
        const deletedId = typeof content === 'string' ? content : content?.id
        if (deletedId && isCurrentConv) {
            messages.value = messages.value.filter((m: Message) => m.id !== deletedId)
        }
    }

    // ----- 事件分发表（reducer map） -----
    type StreamHandler = (convId: string, content: any, isCurrentConv: boolean) => void | Promise<void>

    const streamHandlers: Record<string, StreamHandler> = {
        token: (id, c) => handleToken(id, c),
        thinking: (id, c) => handleThinking(id, c),
        tool_call_start: (id, c) => handleToolCallStart(id, c),
        search_start: (id, c) => handleSearchStart(id, c),
        search_result: (id, c) => handleSearchResult(id, c),
        token_speed: (id, c) => handleTokenSpeed(id, c),
        prompt_progress: (id, c) => handlePromptProgress(id, c),
        done: (id) => { void handleTerminalAsync(id) },
        stopped: (id) => { void handleTerminalAsync(id) },
        error: (id, c, current) => handleError(id, c, current),
        context_trimmed: (id, c) => handleContextTrimmed(id, c),
        conversation_created: (_, c) => handleConvCreated(c),
        assistant_message: (_, c, current) => handleAssistantMsg(c, current),
        user_message: (_, c, current) => handleUserMsg(c, current),
        conversation_updated: (_, c) => handleConvUpdated(c),
        conversation_deleted: (_, c) => handleConvDeleted(c),
        message_deleted: (_, c, current) => handleMsgDeleted(c, current),
    }

    function handleStreamEvent(event: StreamEvent) {
        const convId = event.conversation_id || ''
        const isCurrentConv = convId === currentConversationId.value
            || convId === generatingConvId.value
            || (generatingConvId.value === '' && !currentConversationId.value)
        const handler = streamHandlers[event.type]
        if (handler) {
            handler(convId, event.content, isCurrentConv)
        }
    }

    // ----- 业务函数 -----
    async function deleteMessage(id: string) {
        try {
            const msg = messages.value.find((m: Message) => m.id === id)
            if (!msg) return

            const idsToRemove = [id]
            if (msg.role === 'user') {
                const idx = messages.value.findIndex((m: Message) => m.id === id)
                for (let i = idx + 1; i < messages.value.length; i++) {
                    if (messages.value[i].role === 'assistant') {
                        idsToRemove.push(messages.value[i].id)
                    } else {
                        break
                    }
                }
            }

            messages.value = messages.value.filter((m: Message) => !idsToRemove.includes(m.id))
            await wails.deleteMessage(id)
        } catch (e) {
            console.error('删除消息失败:', e)
        }
    }

    async function regenerateMessage(userMessageID: string, searchMode: string) {
        if (isGenerating.value) return

        const convId = currentConversationId.value
        if (!convId) return

        const userMsgIdx = messages.value.findIndex((m: Message) => m.id === userMessageID)
        if (userMsgIdx >= 0) {
            messages.value = messages.value.filter((_m, idx) => {
                if (idx <= userMsgIdx) return true
                return messages.value[idx].role !== 'assistant'
            })
        }

        generatingConvId.value = convId
        const state = getConvState(convId)
        clearConvState(state)
        state.isGenerating = true
        startGeneratingTimeout()
        startFirstTokenTimeout()

        try {
            await wails.regenerateMessage(userMessageID, searchMode)
        } catch (e) {
            clearTimers()
            clearConvState(state)
            generatingConvId.value = ''
            console.error('重新生成失败:', e)
        }
    }

    async function sendMessage(content: string, searchMode: string, images?: string[], attachments?: Attachment[]) {
        if (isGenerating.value) return

        const convId = currentConversationId.value
        generatingConvId.value = convId || ''
        const state = getConvState(convId || '')
        clearConvState(state)
        state.contextTrimmed = null
        state.isGenerating = true
        startGeneratingTimeout()
        startFirstTokenTimeout()

        const tempUserMsg: Message = {
            id: 'temp-' + Date.now(),
            conversation_id: convId,
            role: 'user',
            content: content,
            search_results: '',
            created_at: new Date().toISOString(),
        }
        if (images && images.length > 0) {
            tempUserMsg.images = JSON.stringify(images)
        }
        if (attachments && attachments.length > 0) {
            tempUserMsg.attachments = attachments.map(a => ({
                type: a.type,
                name: a.name,
                mime_type: a.mime_type,
            }))
        }
        if (!messages.value) messages.value = []
        messages.value.push(tempUserMsg)

        try {
            await wails.sendMessage({
                conversation_id: convId,
                content,
                search_mode: searchMode,
                images,
                attachments,
            })
        } catch (e) {
            clearTimers()
            const currentGenId = generatingConvId.value
            if (currentGenId) {
                const currentState = getConvState(currentGenId)
                clearConvState(currentState)
            }
            generatingConvId.value = ''
            messages.value = messages.value.filter((m: Message) => !m.id.startsWith('temp-'))
            lastError.value = ''
            nextTick(() => {
                lastError.value = String(e || '发送消息失败') + '\n💡 如果服务未启动，请等待模型加载完成后再试'
            })
            console.error('发送消息失败:', e)
        }
    }

    async function stopGeneration() {
        try {
            await wails.stopGeneration()
        } catch (e) {
            console.error('停止生成失败:', e)
        }
        clearTimers()

        addTimer(() => {
            const convId = generatingConvId.value
            if (convId) {
                const state = getConvState(convId)
                clearConvState(state)
            }
            generatingConvId.value = ''
        }, 5000)
    }

    // 新建对话：懒创建模式，只清空当前会话状态
    // 实际会话在首条消息发送时由后端自动创建（handleConvCreated 回调处理）
    function createConversation() {
        currentConversationId.value = ''
        messages.value = []
        convStreamingStates.delete('')
        generatingConvId.value = ''
        lastError.value = ''
    }

    async function renameConversation(id: string, title: string) {
        try {
            const fixedTitle = fixUtf8(title)
            await wails.renameConversation(id, fixedTitle)
            const conv = conversations.value.find((c: Conversation) => c.id === id)
            if (conv) conv.title = fixedTitle
        } catch (e) {
            console.error('重命名会话失败:', e)
        }
    }

    async function deleteConversation(id: string) {
        try {
            await wails.deleteConversation(id)
            conversations.value = conversations.value.filter((c: Conversation) => c.id !== id)
            convStreamingStates.delete(id)
            if (generatingConvId.value === id) generatingConvId.value = ''
            if (currentConversationId.value === id) {
                currentConversationId.value = ''
                messages.value = []
            }
        } catch (e) {
            console.error('删除会话失败:', e)
        }
    }

    async function searchMessages(query: string): Promise<Message[]> {
        try {
            return await wails.searchMessages(query)
        } catch (e) {
            console.error('搜索消息失败:', e)
            return []
        }
    }

    async function exportConversation(id: string, format: string): Promise<string> {
        try {
            return await wails.exportConversation(id, format)
        } catch (e) {
            console.error('导出会话失败:', e)
            return ''
        }
    }

    async function exportConversationWithDialog(id: string, format: string): Promise<boolean> {
        try {
            return await wails.exportConversationWithDialog(id, format)
        } catch (e) {
            console.error('导出会话失败:', e)
            return false
        }
    }

    function initStreamListener() {
        wails.onChatStream((event: StreamEvent) => {
            handleStreamEvent(event)
        })
    }

    function cleanupStreamListener() {
        wails.offChatStream()
        clearTimers()
    }

    return {
        conversations,
        currentConversationId,
        messages,
        convStreamingStates,
        isGenerating,
        streamingContent,
        thinkingContent,
        searchResults,
        isSearching,
        isThinking,
        thinkingDuration,
        searchQuery,
        contextTrimmed,
        tokensPerSecond,
        predictedN,
        promptProgress,
        lastError,
        generatingConvId,
        waitingFirstToken,
        isLoadingConversations,
        lastAIMessageId,
        loadConversations,
        selectConversation,
        sendMessage,
        stopGeneration,
        createConversation,
        renameConversation,
        deleteConversation,
        searchMessages,
        exportConversation,
        exportConversationWithDialog,
        deleteMessage,
        regenerateMessage,
        initStreamListener,
        cleanupStreamListener,
        handleStreamEvent,
        forceResetGenerating,
    }
})
