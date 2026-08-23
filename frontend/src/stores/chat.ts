import { defineStore } from 'pinia'
import { ref, shallowReactive, computed, nextTick, watch } from 'vue'
import {
  wails,
  type Conversation,
  type Message,
  type StreamEvent,
  type Attachment
} from '../services/wails'
import { fixUtf8 } from '../utils/utf8'
import { logError } from '../utils/logger'
import { useSettingsStore } from './settings'
import { useConversations } from './chat/conversations'
import type { ConvStreamingState, StreamEventType } from '../types/chat'

/** 流式状态创建
 *  使用 shallowReactive 包裹（任务 22）：
 *  - Map 本身改为 shallowReactive，不再深度代理值对象
 *  - 此处返回 shallowReactive 保证值对象的顶层属性修改仍触发响应式
 *    （如 state.streamingContent = ... / state.isSearching = ...）
 *  - 深层属性（如 streamingChunks.push）不触发响应式，
 *    但 streamingChunks 仅作内部累积缓冲，UI 依赖的 streamingContent 由节流定时器顶层 set 更新
 */
function createEmptyStreamingState(): ConvStreamingState {
  return shallowReactive({
    isGenerating: false,
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

// P4 修复：模块级空状态单例，避免 currentConvState/generatingConvState 每次重算都新建 shallowReactive 对象。
// 安全前提：空状态是只读的"无状态"表示，不会被修改（clearConvState 只作用于 Map 中的实际 state 对象）。
const EMPTY_STREAMING_STATE = createEmptyStreamingState()

/** 清空流式状态（保留 contextTrimmed,contextTrimmed 是通知性事件） */
function clearConvState(state: ConvStreamingState) {
  state.isGenerating = false
  state.streamingContent = ''
  state.streamingChunks = []
  state.thinkingContent = ''
  state.thinkingChunks = []
  state.searchResults = ''
  state.isSearching = false
  state.isThinking = false
  state.thinkingDuration = 0
  state.searchQuery = ''
  state.searchError = ''
}

export const useChatStore = defineStore('chat', () => {
  const settingsStore = useSettingsStore()
  const conversations = ref<Conversation[]>([])
  const currentConversationId = ref<string>('')
  const messages = ref<Message[]>([])
  const lastError = ref('')
  const generatingConvId = ref('')
  // 任务 22：使用 shallowReactive 替代 reactive
  // - Map 的 set/delete/clear 仍触发响应式（streamHandlers 各处依赖 Map.get 的 computed 能正常更新）
  // - 值对象不深度代理，避免流式场景下对 ConvStreamingState 内部字段的深层响应式开销
  // - 值对象顶层属性响应式由 createEmptyStreamingState 中的 shallowReactive 保证
  const convStreamingStates = shallowReactive(new Map<string, ConvStreamingState>())
  const isLoadingConversations = ref(false)
  const waitingFirstToken = ref(false)
  // 生成速度（tokens/s），由后端 token_speed 事件实时推送（每 500ms 降频），0 表示未获取
  const generationSpeed = ref(0)
  // 最近一次请求的 prompt_tokens（来自 llama-server usage），持久化显示总上下文已用 token 数
  const lastPromptTokens = ref(0)
  // 消息请求版本号（M-前2）：防止 handleTerminalAsync 的 await 期间用户切换会话，
  // 旧请求返回后覆盖新会话的消息列表（TOCTOU 竞态）。每次发起新请求递增，响应返回后校验。
  // 用 ref 包装以便跨 composable 共享（useConversations 也需要递增此值）。
  const messagesRequestVersion = ref(0)

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
  /** 清空指定会话的 flush 定时器（会话清理/重置时调用，防止定时器触发已销毁状态） */
  function clearFlushTimer(convId: string) {
    const t = flushTimers.get(convId)
    if (t) {
      clearTimeout(t)
      flushTimers.delete(convId)
    }
  }

  function getConvState(convId: string): ConvStreamingState {
    let state = convStreamingStates.get(convId)
    if (!state) {
      state = createEmptyStreamingState()
      convStreamingStates.set(convId, state)
    }
    return state
  }

  // ----- 流式内容刷新（M-前5：避免 += 的 O(N²) 字符串拼接）-----
  // handleToken 将分块 push 到 streamingChunks 数组（O(1)），由定时器
  // 合并到 streamingContent。FLUSH_INTERVAL = 0 表示下一个宏任务即 flush
  //（setTimeout 0 实际约 4ms，比原 20ms 快 5 倍），让 token 尽快到达 UI。
  // 渲染频率由 useMorphRender 的 RAF 合帧保证 60fps，不会因 flush 频繁而过度渲染。
  // 旧值 20ms 是为已删除的打字动画设计，快速生成时一次 flush 积压多 token 导致"蹦字"。
  // handleThinking 同理用 thinkingChunks 累积，复用同一定时器同时 flush 两者。
  const FLUSH_INTERVAL = 0
  const flushTimers = new Map<string, ReturnType<typeof setTimeout>>()
  function scheduleStreamingFlush(convId: string) {
    if (flushTimers.has(convId)) return
    const id = setTimeout(() => {
      flushTimers.delete(convId)
      const state = convStreamingStates.get(convId)
      if (!state) return
      if (state.streamingChunks.length > 0) {
        state.streamingContent = state.streamingChunks.join('')
      }
      if (state.thinkingChunks.length > 0) {
        state.thinkingContent = state.thinkingChunks.join('')
      }
    }, FLUSH_INTERVAL)
    flushTimers.set(convId, id)
  }
  function flushStreamingImmediately(convId: string) {
    const t = flushTimers.get(convId)
    if (t) {
      clearTimeout(t)
      flushTimers.delete(convId)
    }
    const state = convStreamingStates.get(convId)
    if (!state) return
    if (state.streamingChunks.length > 0) {
      state.streamingContent = state.streamingChunks.join('')
    }
    if (state.thinkingChunks.length > 0) {
      state.thinkingContent = state.thinkingChunks.join('')
    }
  }

  // M10: 解耦"当前查看会话状态"与"正在生成会话状态"，避免切会话时 UI 状态串台
  // 生活类比：客厅电视（currentConvState）显示你正在看的频道，厨房摄像头（generatingConvState）显示正在炒的菜。
  // 切换客厅频道不会让厨房的菜跑到客厅屏幕上。
  // currentConvState：当前查看会话的状态，消息流相关 UI（占位气泡、流式内容、速度显示）使用
  const currentConvState = computed<ConvStreamingState>(() => {
    const id = currentConversationId.value
    const state = id ? convStreamingStates.get(id) : undefined
    if (state) return state
    // 没有当前会话时，fallback 到空会话状态（用于"无选中会话但能生成"的场景，如未选中会话时直接发消息）
    if (convStreamingStates.has('')) {
      return convStreamingStates.get('')!
    }
    return EMPTY_STREAMING_STATE
  })

  // generatingConvState：正在生成会话的状态，全局生成控制 UI（禁用发送按钮、停止按钮）使用
  const generatingConvState = computed<ConvStreamingState>(() => {
    const id = generatingConvId.value
    if (!id) return EMPTY_STREAMING_STATE
    const state = convStreamingStates.get(id)
    if (state) return state
    return EMPTY_STREAMING_STATE
  })

  // isGenerating：当前查看会话是否在生成（消息流相关 UI 使用）
  const isGenerating = computed(() => currentConvState.value.isGenerating)
  // isAnyGenerating：全局是否有会话在生成（ChatInput 禁用发送、store 并发守卫使用）
  // 防止 A 生成中切到 B 时，B 的 isGenerating=false 误判为可发消息导致并发生成状态混乱
  const isAnyGenerating = computed(() => generatingConvState.value.isGenerating)
  const streamingContent = computed(() => currentConvState.value.streamingContent)
  const thinkingContent = computed(() => currentConvState.value.thinkingContent)
  const searchResults = computed(() => currentConvState.value.searchResults)
  const isSearching = computed(() => currentConvState.value.isSearching)
  const isThinking = computed(() => currentConvState.value.isThinking)
  const thinkingDuration = computed(() => currentConvState.value.thinkingDuration)
  const searchQuery = computed(() => currentConvState.value.searchQuery)
  const searchError = computed(() => currentConvState.value.searchError)
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
        const convId = generatingConvId.value
        const state = getConvState(convId)
        if (state.isGenerating) {
          clearFlushTimer(convId)
          clearConvState(state)
          generatingConvId.value = ''
          waitingFirstToken.value = false
          lastError.value = ''
          nextTick(() => {
            lastError.value =
              '生成超时，请重试\n💡 如果频繁超时，可尝试：设置 → 减小上下文长度，或使用更小的模型'
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
            lastError.value =
              'AI 服务响应超时，请检查服务是否正常运行\n💡 可尝试：等待模型加载完成，或重启应用'
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
      const convId = generatingConvId.value
      clearFlushTimer(convId)
      const state = getConvState(convId)
      clearConvState(state)
    }
    generatingConvId.value = ''
    generationSpeed.value = 0
  }

  // ----- 会话管理（提取到 chat/conversations.ts） -----
  const {
    loadConversations,
    selectConversation,
    createConversation,
    renameConversation,
    deleteConversation,
    searchMessages,
    exportConversation,
    exportConversationWithDialog
  } = useConversations({
    conversations,
    currentConversationId,
    messages,
    generatingConvId,
    convStreamingStates,
    isLoadingConversations,
    lastError,
    messagesRequestVersion,
    clearFlushTimer
  })

  // 切换对话时重置 lastPromptTokens（新对话的已用 token 数未知，显示 0 直到下次请求完成）
  watch(currentConversationId, () => {
    lastPromptTokens.value = 0
  })

  function handleTerminalEvent(convId: string) {
    clearTimers()
    // 终止前立即刷新剩余的流式分块，确保最终内容完整
    flushStreamingImmediately(convId)
    const state = convStreamingStates.get(convId)
    if (state) {
      clearConvState(state)
    }
    if (convId === '' || generatingConvId.value === convId) {
      generatingConvId.value = ''
      // 生成结束，重置速度显示（UI 仅在 isGenerating 时展示，重置可避免残留）
      generationSpeed.value = 0
      // 生成结束后从后端拉取最近一次请求的 prompt_tokens，持久化显示总上下文用量
      wails
        .getLastPromptTokens()
        .then(n => {
          // P3 修复：校验当前会话是否仍是生成结束时的会话，
          // 避免 A 生成结束→用户切到 B→A 的 promise 返回→B 的 TokenCounter 显示 A 的 token 数
          if (currentConversationId.value === convId) {
            lastPromptTokens.value = n || 0
          }
        })
        .catch(() => {})
    }
  }

  // ----- 流式事件 reducer（独立小函数,易单测） -----

  // 已迁移：content 类型对齐 TokenEvent['content']（任务 31.3）
  function handleToken(convId: string, content: string) {
    startGeneratingTimeout()
    clearFirstTokenOnResponse()
    const state = getConvState(convId)
    const wasThinking = state.isThinking && !!state.thinkingContent
    if (state.isThinking && state.thinkingContent) {
      state.isThinking = false
      state.thinkingDuration = (Date.now() - state.thinkingStartTime) / 1000
    } else if (!state.isThinking && state.thinkingContent && state.thinkingDuration === 0) {
      state.thinkingDuration = (Date.now() - state.thinkingStartTime) / 1000
    }
    // 修复（M-前5）：用数组累积替代字符串 +=，避免长文本输出时的 O(N²) 拼接开销。
    // 分块 push 是 O(1)，由 scheduleStreamingFlush 节流（50ms）合并到 streamingContent。
    state.streamingChunks.push(content)
    // 首个 token 或思考结束：立即 flush，消除首字延迟和 streamingContent="" 空窗口
    // - 首字延迟是用户感知最强的卡顿源，50ms 节流对后续 token 合理，但首字应立即显示
    // - 思考切换时立即 flush 避免 thinkingAsContent 短暂为 true 吞掉首 token
    if (wasThinking || state.streamingChunks.length === 1) {
      flushStreamingImmediately(convId)
    } else {
      scheduleStreamingFlush(convId)
    }
  }

  // 已迁移：content 类型对齐 ThinkingEvent['content']（任务 31.3）
  function handleThinking(convId: string, content: string) {
    startGeneratingTimeout()
    clearFirstTokenOnResponse()
    const state = getConvState(convId)
    if (!state.isThinking && !state.thinkingContent) {
      state.isThinking = true
      state.thinkingStartTime = Date.now()
      state.thinkingDuration = 0
    }
    // 与 handleToken 同理：用数组累积替代字符串 +=，避免长思考时 O(N²) 拼接
    state.thinkingChunks.push(content)
    // 首个思考 token 立即 flush（消除首字延迟）；后续节流合并
    if (state.thinkingChunks.length === 1) {
      flushStreamingImmediately(convId)
    } else {
      scheduleStreamingFlush(convId)
    }
  }

  // 已迁移：content 类型对齐 ToolCallStartEvent['content']（任务 24）
  // P1-2 修复：content 现包含 tool_call_id，用于并发 tool call 关联
  function handleToolCallStart(
    convId: string,
    content: Extract<StreamEvent, { type: 'tool_call_start' }>['content']
  ) {
    startGeneratingTimeout()
    clearFirstTokenOnResponse()
    const state = getConvState(convId)
    state.isSearching = true
    state.searchQuery = content.query || ''
    // tool_call_id 用于未来细粒度状态跟踪（当前保持单值兼容，多 tool call 时最后一个覆盖）
  }

  // 已迁移：content 类型对齐 SearchStartEvent['content']（任务 24）
  // 后端在 tool call 场景发送 JSON 字符串（紧跟 tool_call_start 事件会覆盖 searchQuery），
  // 预搜索场景发送普通字符串，统一按 string 直接使用即可。
  function handleSearchStart(
    convId: string,
    content: Extract<StreamEvent, { type: 'search_start' }>['content']
  ) {
    startGeneratingTimeout()
    clearFirstTokenOnResponse()
    const state = getConvState(convId)
    state.isSearching = true
    state.searchQuery = content
    // 清空上次的搜索错误，避免新搜索开始时残留旧错误提示
    state.searchError = ''
  }

  // 已迁移：content 类型对齐 SearchResultEvent['content']（任务 24）
  // C-7 协议唯一事实化：后端预搜索与 tool call 路径统一发射 { tool_call_id, results } 结构，
  // 历史的 string / 裸数组两种兼容分支已随协议收敛删除。
  // 存储保持 JSON 字符串形态（与消息持久化格式一致，SearchStatus 直接解析渲染）
  function handleSearchResult(
    convId: string,
    content: Extract<StreamEvent, { type: 'search_result' }>['content']
  ) {
    startGeneratingTimeout()
    clearFirstTokenOnResponse()
    const state = getConvState(convId)
    state.isSearching = false
    state.searchQuery = ''
    state.searchResults = content.results.length > 0 ? JSON.stringify(content.results) : ''
  }

  // 处理搜索失败事件：把后端推送的友好错误提示写入 searchError 状态
  // 前端 SearchStatus 组件根据 searchError 显示红色警告条
  // 注意：search_error 不改变 isSearching（由 search_result 负责置 false），
  // 因为 tool call 模式下搜索失败后模型仍会继续生成回答
  function handleSearchError(
    convId: string,
    content: Extract<StreamEvent, { type: 'search_error' }>['content']
  ) {
    const state = getConvState(convId)
    state.searchError = content || ''
  }

  /** 处理 token_speed 事件：实时更新生成速度（会话级 + 全局）
   *  合并了原 generation_speed 事件的功能，后端每 500ms 降频发射一次
   *  已迁移：content 类型对齐 TokenSpeedEvent['content']（任务 24） */
  function handleTokenSpeed(
    convId: string,
    content: Extract<StreamEvent, { type: 'token_speed' }>['content']
  ) {
    const state = getConvState(convId)
    if (content.tokensPerSecond && content.tokensPerSecond > 0) {
      state.tokensPerSecond = content.tokensPerSecond
      state.predictedN = content.predictedN || 0
      // 同时更新全局生成速度（原 generation_speed 事件功能，仅当前生成会话）
      // C-7 协议唯一事实化：后端已移除 tokens_per_second 重复字段，仅读驼峰命名
      if (convId === generatingConvId.value || convId === '') {
        generationSpeed.value = content.tokensPerSecond
      }
    }
  }

  /** 处理 prompt_progress 事件：实时更新提示词处理进度
   *  已迁移：content 类型对齐 PromptProgressEvent['content']（任务 24） */
  function handlePromptProgress(
    convId: string,
    content: Extract<StreamEvent, { type: 'prompt_progress' }>['content']
  ) {
    const state = getConvState(convId)
    if (content.processed && content.processed > 0) {
      state.promptProgress = {
        total: content.total || 0,
        cache: content.cache || 0,
        processed: content.processed || 0,
        timeMs: content.timeMs || 0
      }
    }
  }

  /** 局部更新消息列表（M-前6）：避免全量替换导致所有 MessageItem 组件重渲染。
   *  流式生成结束时通常只有最后一条消息内容变化，用 splice 原地替换仅触发该组件更新。 */
  function updateMessagesIncremental(newMsgs: Message[]) {
    const oldMsgs = messages.value
    if (oldMsgs.length === 0) {
      messages.value = newMsgs
      return
    }

    // 保留 tokens_per_second（数据库不存储此字段，来自 assistant_message 事件）
    const speedMap = new Map<string, number>()
    for (const m of oldMsgs) {
      if (m.tokens_per_second && m.tokens_per_second > 0) {
        speedMap.set(m.id, m.tokens_per_second)
      }
    }
    for (const m of newMsgs) {
      if (speedMap.has(m.id)) {
        m.tokens_per_second = speedMap.get(m.id)
      }
    }

    // 情况1：消息数量相同，且前 N-1 条 id 一致 → 仅原地替换最后一条
    if (newMsgs.length === oldMsgs.length && newMsgs.length > 0) {
      let prefixMatch = true
      for (let i = 0; i < oldMsgs.length - 1; i++) {
        if (oldMsgs[i].id !== newMsgs[i].id) {
          prefixMatch = false
          break
        }
      }
      if (prefixMatch) {
        messages.value.splice(oldMsgs.length - 1, 1, newMsgs[newMsgs.length - 1])
        return
      }
    }

    // 情况2：新消息多一条，且前缀完全匹配 → 仅追加新消息
    if (newMsgs.length === oldMsgs.length + 1) {
      let prefixMatch = true
      for (let i = 0; i < oldMsgs.length; i++) {
        if (oldMsgs[i].id !== newMsgs[i].id) {
          prefixMatch = false
          break
        }
      }
      if (prefixMatch) {
        messages.value.push(newMsgs[newMsgs.length - 1])
        return
      }
    }

    // 情况3：结构变化较大，回退到全量更新
    messages.value = newMsgs
  }

  async function handleTerminalAsync(convId: string) {
    const targetConvId = convId || generatingConvId.value
    if (targetConvId) {
      // 记录请求版本号，await 返回后校验，防止旧请求覆盖新会话消息（M-前2 TOCTOU 防护）
      const requestVersion = ++messagesRequestVersion.value
      try {
        const msgs = (await wails.getMessages(targetConvId)) as Message[]
        // 版本号不匹配说明期间用户已切换会话，丢弃本次响应
        if (requestVersion !== messagesRequestVersion.value) return
        if (
          targetConvId === currentConversationId.value ||
          targetConvId === generatingConvId.value
        ) {
          // 修复（M-前6）：原实现 messages.value = msgs || [] 全量替换，
          // 导致所有 MessageItem 组件重新渲染。改为局部更新：仅替换变化的最后一条。
          updateMessagesIncremental(msgs || [])
        }
        nextTick(() => handleTerminalEvent(convId))
      } catch {
        if (requestVersion !== messagesRequestVersion.value) return
        handleTerminalEvent(convId)
      }
    } else {
      handleTerminalEvent(convId)
    }
    // 会话列表的更新（标题、排序）已由 conversation_updated 事件的 handleConvUpdated 完成，
    // 无需在此全量刷新 loadConversations()，避免不必要的数据库查询。
  }

  // 已迁移：content 类型对齐 ErrorEvent['content']（任务 31.3）
  function handleError(convId: string, content: string, isCurrentConv: boolean) {
    handleTerminalEvent(convId)
    if (isCurrentConv || !convId) {
      lastError.value = ''
      nextTick(() => {
        lastError.value = String(content || '生成过程中发生错误，请查看日志了解详情')
      })
    }
  }

  // 已迁移：content 类型对齐 ContextTrimmedEvent['content']（任务 24）
  function handleContextTrimmed(
    convId: string,
    content: Extract<StreamEvent, { type: 'context_trimmed' }>['content']
  ) {
    const state = getConvState(convId)
    state.contextTrimmed = {
      reason: content.reason || 'unknown',
      promptTokens: content.prompt_tokens,
      contextSize: content.context_size,
      messagesAfter: content.messages_after
    }
  }

  // 已迁移：content 类型对齐 ConversationCreatedEvent['content']（任务 24）
  function handleConvCreated(
    content: Extract<StreamEvent, { type: 'conversation_created' }>['content']
  ) {
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

  // 已迁移：content 类型对齐 AssistantMessageEvent['content']（任务 24）
  function handleAssistantMsg(
    content: Extract<StreamEvent, { type: 'assistant_message' }>['content'],
    isCurrentConv: boolean
  ) {
    if (!isCurrentConv) return
    const idx = messages.value.findIndex((m: Message) => m.id === content.id)
    if (idx >= 0) {
      messages.value[idx] = content
    } else {
      messages.value.push(content)
    }
  }

  // 已迁移：content 类型对齐 UserMessageEvent['content']（任务 24）
  function handleUserMsg(
    content: Extract<StreamEvent, { type: 'user_message' }>['content'],
    isCurrentConv: boolean
  ) {
    if (!isCurrentConv) return
    // 合并原 some + findIndex 双遍历为单次遍历
    let tempIdx = -1
    let hasExisting = false
    for (let i = 0; i < messages.value.length; i++) {
      const m = messages.value[i]
      if (m.role === 'user' && m.content === content.content) {
        if (m.id.startsWith('temp-')) {
          tempIdx = i
          break
        }
        hasExisting = true
      }
    }
    if (tempIdx >= 0) {
      messages.value[tempIdx] = content
    } else if (!hasExisting) {
      messages.value.push(content)
    }
  }

  // 已迁移：content 类型对齐 ConversationUpdatedEvent['content']（任务 24）
  function handleConvUpdated(
    content: Extract<StreamEvent, { type: 'conversation_updated' }>['content']
  ) {
    const idx = conversations.value.findIndex((c: Conversation) => c.id === content.id)
    if (idx === -1) return
    // 更新字段
    conversations.value[idx].title = fixUtf8(content.title)
    conversations.value[idx].updated_at = content.updated_at
    // 移到列表首位（模拟后端 ORDER BY updated_at DESC 排序）
    // 只在不在首位时才移动，避免不必要的响应式触发
    if (idx > 0) {
      const [conv] = conversations.value.splice(idx, 1)
      conversations.value.unshift(conv)
    }
  }

  // 已迁移：content 类型对齐 ConversationDeletedEvent['content']（任务 24）
  // C-7 协议唯一事实化：后端三处发射点均为裸 string ID，{ id } 对象兼容分支已删除
  function handleConvDeleted(
    content: Extract<StreamEvent, { type: 'conversation_deleted' }>['content']
  ) {
    const deletedId = content
    conversations.value = conversations.value.filter((c: Conversation) => c.id !== deletedId)
    convStreamingStates.delete(deletedId)
    if (generatingConvId.value === deletedId) generatingConvId.value = ''
    if (currentConversationId.value === deletedId) {
      currentConversationId.value = ''
      messages.value = []
    }
  }

  // 已迁移：content 类型对齐 MessageDeletedEvent['content']（任务 24）
  function handleMsgDeleted(
    content: Extract<StreamEvent, { type: 'message_deleted' }>['content'],
    isCurrentConv: boolean
  ) {
    if (content && isCurrentConv) {
      messages.value = messages.value.filter((m: Message) => m.id !== content)
    }
  }

  // ----- 事件分发表（reducer map） -----
  // 任务 24：所有 handler 已迁移到具体事件 content 类型，使用映射类型替代原先的 any 签名。
  // 每个键对应的 handler content 类型由 Extract<StreamEvent, { type: K }>['content'] 推导，
  // 新增/重命名事件类型时编译器会即时校验。
  type StreamHandlerMap = {
    [K in StreamEventType]?: (
      convId: string,
      content: Extract<StreamEvent, { type: K }>['content'],
      isCurrentConv: boolean
    ) => void | Promise<void>
  }

  const streamHandlers: StreamHandlerMap = {
    token: (id, c) => handleToken(id, c),
    thinking: (id, c) => handleThinking(id, c),
    tool_call_start: (id, c) => handleToolCallStart(id, c),
    search_start: (id, c) => handleSearchStart(id, c),
    search_result: (id, c) => handleSearchResult(id, c),
    search_error: (id, c) => handleSearchError(id, c),
    token_speed: (id, c) => handleTokenSpeed(id, c),
    prompt_progress: (id, c) => handlePromptProgress(id, c),
    done: id => {
      void handleTerminalAsync(id)
    },
    stopped: id => {
      void handleTerminalAsync(id)
    },
    error: (id, c, current) => handleError(id, c, current),
    context_trimmed: (id, c) => handleContextTrimmed(id, c),
    conversation_created: (_, c) => handleConvCreated(c),
    assistant_message: (_, c, current) => handleAssistantMsg(c, current),
    user_message: (_, c, current) => handleUserMsg(c, current),
    conversation_updated: (_, c) => handleConvUpdated(c),
    conversation_deleted: (_, c) => handleConvDeleted(c),
    message_deleted: (_, c, current) => handleMsgDeleted(c, current)
  }

  // 安全实践：编译期断言确保 streamHandlers 覆盖所有 StreamEventType（见安全审查 #40）
  // 若新增 StreamEventType 未实现 handler，此断言会在使用处触发类型检查
  type _AssertStreamHandlers =
    typeof streamHandlers extends Record<StreamEventType, any> ? true : never

  function handleStreamEvent(event: StreamEvent) {
    const convId = event.conversation_id || ''
    const isCurrentConv =
      convId === currentConversationId.value ||
      convId === generatingConvId.value ||
      (generatingConvId.value === '' && !currentConversationId.value)
    const handler = streamHandlers[event.type]
    if (handler) {
      // event.type 与 event.content 是判别联合的关联字段，
      // 但 TS 无法从 event.type（联合类型）收窄 streamHandlers[event.type] 的 handler 类型，
      // 此处用断言将 handler 视为接受 event.content 类型的函数（运行时安全由判别联合保证）
      ;(handler as (convId: string, content: typeof event.content, isCurrentConv: boolean) => void)(
        convId,
        event.content,
        isCurrentConv
      )
    }
  }

  // ----- 业务函数 -----
  async function deleteMessage(id: string) {
    try {
      const msg = messages.value.find((m: Message) => m.id === id)
      if (!msg) return

      // 使用 Set 索引：filter 循环中查找为 O(1)，避免数组 includes 的 O(n) 遍历
      const idsToRemove = new Set<string>([id])
      if (msg.role === 'user') {
        const idx = messages.value.findIndex((m: Message) => m.id === id)
        for (let i = idx + 1; i < messages.value.length; i++) {
          if (messages.value[i].role === 'assistant') {
            idsToRemove.add(messages.value[i].id)
          } else {
            break
          }
        }
      }

      // C-3 修复：保存原始消息用于回滚，避免后端删除失败时 UI 与 DB 不一致
      // 生活类比：仓库销毁货品前先拍照存档，销毁失败时凭照片恢复记录
      const originalMessages = messages.value
      messages.value = messages.value.filter((m: Message) => !idsToRemove.has(m.id))
      try {
        await wails.deleteMessage(id)
      } catch (e) {
        // 后端删除失败：回滚 UI 到删除前状态
        messages.value = originalMessages
        throw e
      }
    } catch (e) {
      logError('删除消息失败', e)
    }
  }

  async function regenerateMessage(userMessageID: string, searchMode: string) {
    // M10: 用 isAnyGenerating 防止 A 生成中切到 B 时误判可发消息导致并发生成
    if (isAnyGenerating.value) return

    const convId = currentConversationId.value
    if (!convId) return

    // C-3 修复：保存原始消息用于回滚，避免后端调用失败时 UI 与 DB 不一致
    // 生活类比：重新做菜前先备份原菜品，做砸了还能上原菜
    const originalMessages = messages.value
    // 删除用户消息之后的所有回复，保留到用户消息为止
    const userMsgIdx = messages.value.findIndex((m: Message) => m.id === userMessageID)
    if (userMsgIdx >= 0) {
      messages.value = messages.value.slice(0, userMsgIdx + 1)
    }

    generatingConvId.value = convId
    clearFlushTimer(convId)
    const state = getConvState(convId)
    clearConvState(state)
    state.isGenerating = true
    // 生成开始时重置速度显示
    generationSpeed.value = 0
    startGeneratingTimeout()
    startFirstTokenTimeout()

    try {
      await wails.regenerateMessage(userMessageID, searchMode)
    } catch (e) {
      clearTimers()
      clearFlushTimer(convId)
      clearConvState(state)
      generatingConvId.value = ''
      // 后端调用失败：回滚 UI 到重新生成前状态
      messages.value = originalMessages
      logError('重新生成失败', e)
    }
  }

  async function sendMessage(
    content: string,
    searchMode: string,
    images?: string[],
    attachments?: Attachment[]
  ) {
    // M10: 用 isAnyGenerating 防止 A 生成中切到 B 时误判可发消息导致并发生成
    if (isAnyGenerating.value) return

    const convId = currentConversationId.value
    generatingConvId.value = convId || ''
    clearFlushTimer(convId || '')
    const state = getConvState(convId || '')
    clearConvState(state)
    state.contextTrimmed = null
    state.isGenerating = true
    // 生成开始时重置速度显示
    generationSpeed.value = 0
    startGeneratingTimeout()
    startFirstTokenTimeout()

    const tempUserMsg: Message = {
      id: 'temp-' + Date.now(),
      conversation_id: convId,
      role: 'user',
      content: content,
      search_results: '',
      created_at: new Date().toISOString()
    }
    if (images && images.length > 0) {
      tempUserMsg.images = JSON.stringify(images)
    }
    if (attachments && attachments.length > 0) {
      tempUserMsg.attachments = attachments.map(a => ({
        type: a.type,
        name: a.name,
        mime_type: a.mime_type
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
        attachments
      })
    } catch (e) {
      clearTimers()
      const currentGenId = generatingConvId.value
      if (currentGenId) {
        clearFlushTimer(currentGenId)
        const currentState = getConvState(currentGenId)
        clearConvState(currentState)
      }
      generatingConvId.value = ''
      messages.value = messages.value.filter((m: Message) => !m.id.startsWith('temp-'))
      lastError.value = ''
      nextTick(() => {
        lastError.value =
          String(e || '发送消息失败') + '\n💡 如果服务未启动，请等待模型加载完成后再试'
      })
      logError('发送消息失败', e)
    }
  }

  async function stopGeneration() {
    try {
      await wails.stopGeneration()
    } catch (e) {
      logError('停止生成失败', e)
    }
    clearTimers()

    // 兜底：若 stopped 事件未到达，2 秒后强制清除生成状态
    addTimer(() => {
      const convId = generatingConvId.value
      if (convId) {
        const state = getConvState(convId)
        clearConvState(state)
      }
      generatingConvId.value = ''
    }, 2000)
  }

  function registerStreamListener(): () => void {
    const unsubscribe = wails.subscribeChatStream((event: StreamEvent) => {
      handleStreamEvent(event)
    })
    return () => {
      unsubscribe()
      clearTimers()
      // P7 修复：清理所有 pending 的 flush 定时器，避免应用退出时残留回调
      flushTimers.forEach((t, k) => {
        clearTimeout(t)
        flushTimers.delete(k)
      })
    }
  }

  return {
    conversations,
    currentConversationId,
    messages,
    convStreamingStates,
    isGenerating,
    isAnyGenerating,
    streamingContent,
    thinkingContent,
    searchResults,
    isSearching,
    isThinking,
    thinkingDuration,
    searchQuery,
    searchError,
    contextTrimmed,
    tokensPerSecond,
    predictedN,
    promptProgress,
    lastError,
    generatingConvId,
    waitingFirstToken,
    generationSpeed,
    lastPromptTokens,
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
    registerStreamListener,
    handleStreamEvent,
    forceResetGenerating
  }
})
