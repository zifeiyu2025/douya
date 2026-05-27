import { defineStore } from 'pinia'
import { ref, reactive, computed, nextTick } from 'vue'
import { wails, type Conversation, type Message, type StreamEvent, type Attachment } from '../services/wails'
import { fixUtf8 } from '../utils/utf8'
import { useSettingsStore } from './settings'

interface ConvStreamingState {
    isGenerating: boolean
    streamingContent: string
    thinkingContent: string
    searchResults: string
    isSearching: boolean
    isThinking: boolean
    thinkingStartTime: number
    thinkingDuration: number
    searchQuery: string
}

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
    }
}

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
    let generatingTimeout: ReturnType<typeof setTimeout> | null = null
    let firstTokenTimeout: ReturnType<typeof setTimeout> | null = null
    let stopTimeout: ReturnType<typeof setTimeout> | null = null
    const waitingFirstToken = ref(false)

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

    const lastAIMessageId = computed(() => {
        for (let i = messages.value.length - 1; i >= 0; i--) {
            if (messages.value[i].role === 'assistant') {
                return messages.value[i].id
            }
        }
        return ''
    })

    function forceResetGenerating() {
        clearGeneratingTimeout()
        clearFirstTokenOnResponse()
        if (generatingConvId.value) {
            const state = getConvState(generatingConvId.value)
            clearConvState(state)
        }
        generatingConvId.value = ''
    }

    function startGeneratingTimeout() {
        clearGeneratingTimeout()
        const timeout = settingsStore.thinkingEnabled ? 300 * 1000 : 120 * 1000
        generatingTimeout = setTimeout(() => {
            if (generatingConvId.value) {
                const state = getConvState(generatingConvId.value)
                if (state.isGenerating) {
                    clearConvState(state)
                    generatingConvId.value = ''
                    waitingFirstToken.value = false
                    lastError.value = ''
                    nextTick(() => {
                        lastError.value = '生成超时，请重试'
                    })
                }
            }
        }, timeout)
    }

    function startFirstTokenTimeout() {
        clearFirstTokenTimeout()
        waitingFirstToken.value = true
        firstTokenTimeout = setTimeout(() => {
            if (waitingFirstToken.value && generatingConvId.value) {
                const state = getConvState(generatingConvId.value)
                if (state.isGenerating && !state.streamingContent && !state.thinkingContent) {
                    clearConvState(state)
                    generatingConvId.value = ''
                    waitingFirstToken.value = false
                    lastError.value = ''
                    nextTick(() => {
                        lastError.value = 'AI 服务响应超时，请检查服务是否正常运行'
                    })
                }
            }
        }, 60 * 1000)
    }

    function clearFirstTokenTimeout() {
        if (firstTokenTimeout) {
            clearTimeout(firstTokenTimeout)
            firstTokenTimeout = null
        }
    }

    function clearFirstTokenOnResponse() {
        clearFirstTokenTimeout()
        waitingFirstToken.value = false
    }

    function resetGeneratingTimeout() {
        startGeneratingTimeout()
    }

    function clearGeneratingTimeout() {
        if (generatingTimeout) {
            clearTimeout(generatingTimeout)
            generatingTimeout = null
        }
    }

    async function loadConversations() {
        isLoadingConversations.value = true
        try {
            const convs = await wails.getConversations()
            const newConvs = convs.map((c: any) => ({
                ...c,
                title: fixUtf8(c.title)
            }))
            const newIdSet = new Set(newConvs.map((c: any) => c.id))
            const keptOld = conversations.value.filter((c: any) => !newIdSet.has(c.id))
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
        clearGeneratingTimeout()
        clearFirstTokenOnResponse()
        if (stopTimeout !== null) {
            clearTimeout(stopTimeout)
            stopTimeout = null
        }
        const state = convStreamingStates.get(convId)
        if (state) {
            clearConvState(state)
        }
        if (convId === '' || generatingConvId.value === convId) {
            generatingConvId.value = ''
        }
    }

    function handleStreamEvent(event: StreamEvent) {
        const convId = event.conversation_id || ''
        const isCurrentConv = convId === currentConversationId.value
            || convId === generatingConvId.value
            || (generatingConvId.value === '' && !currentConversationId.value)

        switch (event.type) {
            case 'token': {
                resetGeneratingTimeout()
                clearFirstTokenOnResponse()
                const state = getConvState(convId)
                if (state.isThinking && state.thinkingContent) {
                    state.isThinking = false
                    state.thinkingDuration = (Date.now() - state.thinkingStartTime) / 1000
                } else if (!state.isThinking && state.thinkingContent && state.thinkingDuration === 0) {
                    state.thinkingDuration = (Date.now() - state.thinkingStartTime) / 1000
                }
                state.streamingContent += event.content
                break
            }
            case 'thinking': {
                resetGeneratingTimeout()
                clearFirstTokenOnResponse()
                const state = getConvState(convId)
                if (!state.isThinking && !state.thinkingContent) {
                    state.isThinking = true
                    state.thinkingStartTime = Date.now()
                    state.thinkingDuration = 0
                }
                state.thinkingContent += event.content
                break
            }
            case 'tool_call_start': {
                resetGeneratingTimeout()
                clearFirstTokenOnResponse()
                const state = getConvState(convId)
                state.isSearching = true
                const content = event.content as any
                state.searchQuery = content?.query || ''
                break
            }
            case 'search_start': {
                resetGeneratingTimeout()
                clearFirstTokenOnResponse()
                const state = getConvState(convId)
                state.isSearching = true
                if (typeof event.content === 'string') {
                    try {
                        const parsed = JSON.parse(event.content)
                        state.searchQuery = parsed.query || event.content
                    } catch {
                        state.searchQuery = event.content
                    }
                }
                break
            }
            case 'search_result': {
                resetGeneratingTimeout()
                clearFirstTokenOnResponse()
                const state = getConvState(convId)
                state.isSearching = false
                state.searchQuery = ''
                try {
                    let content = event.content
                    if (typeof content === 'string') {
                        try { content = JSON.parse(content) } catch { content = null }
                    }
                    if (Array.isArray(content) && content.length > 0) {
                        state.searchResults = JSON.stringify(content)
                    } else {
                        state.searchResults = ''
                    }
                } catch {
                    state.searchResults = ''
                }
                break
            }
            case 'done': {
                const targetConvId = convId || generatingConvId.value
                if (targetConvId) {
                    wails.getMessages(targetConvId).then((msgs: Message[]) => {
                        if (targetConvId === currentConversationId.value || targetConvId === generatingConvId.value) {
                            messages.value = msgs || []
                        }
                        handleTerminalEvent(convId)
                    }).catch(() => {
                        handleTerminalEvent(convId)
                    })
                } else {
                    handleTerminalEvent(convId)
                }
                loadConversations()
                break
            }
            case 'stopped': {
                const targetConvId = convId || generatingConvId.value
                if (targetConvId) {
                    wails.getMessages(targetConvId).then((msgs: Message[]) => {
                        if (targetConvId === currentConversationId.value || targetConvId === generatingConvId.value) {
                            messages.value = msgs || []
                        }
                        handleTerminalEvent(convId)
                    }).catch(() => {
                        handleTerminalEvent(convId)
                    })
                } else {
                    handleTerminalEvent(convId)
                }
                break
            }
            case 'error': {
                handleTerminalEvent(convId)
                if (isCurrentConv || !convId) {
                    lastError.value = ''
                    nextTick(() => {
                        lastError.value = String(event.content || '生成过程中发生错误')
                    })
                }
                break
            }
            case 'conversation_created': {
                if (event.content?.id) {
                    const newConvId = event.content.id
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
                    conversations.value.unshift({
                        ...event.content,
                        title: fixUtf8(event.content.title)
                    })
                }
                break
            }
            case 'assistant_message': {
                if (event.content) {
                    if (isCurrentConv) {
                        const idx = messages.value.findIndex((m: Message) => m.id === event.content.id)
                        if (idx >= 0) {
                            messages.value[idx] = event.content
                        } else {
                            messages.value.push(event.content)
                        }
                    }
                }
                break
            }
            case 'user_message': {
                if (event.content) {
                    if (isCurrentConv) {
                        const exists = messages.value.some((m: Message) =>
                            m.role === 'user' && m.content === event.content.content
                        )
                        if (!exists) {
                            messages.value.push(event.content)
                        } else {
                            const idx = messages.value.findIndex((m: Message) =>
                                m.role === 'user' && m.content === event.content.content && m.id.startsWith('temp-')
                            )
                            if (idx >= 0) {
                                messages.value[idx] = event.content
                            }
                        }
                    }
                }
                break
            }
            case 'conversation_updated': {
                if (event.content?.id) {
                    const conv = conversations.value.find((c: Conversation) => c.id === event.content.id)
                    if (conv) {
                        conv.title = fixUtf8(event.content.title)
                        conv.updated_at = event.content.updated_at
                    }
                }
                break
            }
            case 'conversation_deleted': {
                if (event.content) {
                    const deletedId = typeof event.content === 'string' ? event.content : event.content.id
                    if (deletedId) {
                        conversations.value = conversations.value.filter((c: Conversation) => c.id !== deletedId)
                        convStreamingStates.delete(deletedId)
                        if (generatingConvId.value === deletedId) {
                            generatingConvId.value = ''
                        }
                        if (currentConversationId.value === deletedId) {
                            currentConversationId.value = ''
                            messages.value = []
                        }
                    }
                }
                break
            }
            case 'message_deleted': {
                if (event.content) {
                    const deletedMsgId = typeof event.content === 'string' ? event.content : event.content.id
                    if (deletedMsgId && isCurrentConv) {
                        messages.value = messages.value.filter((m: Message) => m.id !== deletedMsgId)
                    }
                }
                break
            }
        }
    }

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

    async function regenerateMessage(userMessageID: string, searchEnabled: boolean) {
        if (isGenerating.value) return

        const convId = currentConversationId.value
        if (!convId) return

        const userMsgIdx = messages.value.findIndex((m: Message) => m.id === userMessageID)
        if (userMsgIdx >= 0) {
            messages.value = messages.value.filter((m: Message, idx: number) => {
                if (idx <= userMsgIdx) return true
                return m.role !== 'assistant'
            })
        }

        generatingConvId.value = convId
        const state = getConvState(convId)
        clearConvState(state)
        state.isGenerating = true
        startGeneratingTimeout()
        startFirstTokenTimeout()

        try {
            await wails.regenerateMessage(userMessageID, searchEnabled)
        } catch (e) {
            clearGeneratingTimeout()
            clearFirstTokenOnResponse()
            clearConvState(state)
            generatingConvId.value = ''
            console.error('重新生成失败:', e)
        }
    }

    async function sendMessage(content: string, searchEnabled: boolean, images?: string[], attachments?: Attachment[]) {
        if (isGenerating.value) return

        const convId = currentConversationId.value
        generatingConvId.value = convId || ''
        const state = getConvState(convId || '')
        clearConvState(state)
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
            const params: any = {
                conversation_id: convId,
                content,
                search_enabled: searchEnabled,
            }
            if (images && images.length > 0) {
                params.images = images
            }
            if (attachments && attachments.length > 0) {
                params.attachments = attachments
            }
            await wails.sendMessage(params)
        } catch (e: any) {
            clearGeneratingTimeout()
            clearFirstTokenOnResponse()
            const currentGenId = generatingConvId.value
            if (currentGenId) {
                const currentState = getConvState(currentGenId)
                clearConvState(currentState)
            }
            generatingConvId.value = ''
            messages.value = messages.value.filter((m: Message) => !m.id.startsWith('temp-'))
            lastError.value = ''
            nextTick(() => {
                lastError.value = String(e || '发送消息失败')
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
        clearGeneratingTimeout()
        clearFirstTokenOnResponse()

        if (stopTimeout !== null) {
            clearTimeout(stopTimeout)
        }
        stopTimeout = setTimeout(() => {
            const convId = generatingConvId.value
            if (convId) {
                const state = getConvState(convId)
                clearConvState(state)
            }
            generatingConvId.value = ''
            stopTimeout = null
        }, 5000)
    }

    async function createConversation() {
        try {
            const conv = await wails.createConversation()
            conv.title = fixUtf8(conv.title)
            conversations.value.unshift(conv)
            await selectConversation(conv.id)
        } catch (e) {
            console.error('创建会话失败:', e)
        }
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
            if (generatingConvId.value === id) {
                generatingConvId.value = ''
            }
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
