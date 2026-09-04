// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

import type { Ref, ShallowReactive } from 'vue'
import { wails, type Conversation, type Message } from '../../services/wails'
import { fixUtf8 } from '../../utils/utf8'
import { logError } from '../../utils/logger'
import type { ConvStreamingState } from '../../types/chat'

/**
 * 会话管理 composable：从 useChatStore 提取，降低主 store 复杂度。
 *
 * 拆分说明：原 store 内 8 个会话管理函数（load/select/create/rename/delete/search/export）
 * 相对独立，主要依赖 wails API 和几个 ref，适合提取为独立 composable。
 */

/** 会话管理所需的共享状态依赖 */
export interface ConversationDeps {
  conversations: Ref<Conversation[]>
  currentConversationId: Ref<string>
  messages: Ref<Message[]>
  generatingConvId: Ref<string>
  convStreamingStates: ShallowReactive<Map<string, ConvStreamingState>>
  isLoadingConversations: Ref<boolean>
  lastError: Ref<string>
  /** 消息请求版本号（TOCTOU 防护）：用 ref 包装以便跨 composable 共享 */
  messagesRequestVersion: Ref<number>
  /** 清理定时器回调（删除会话时清理孤儿定时器，防止内存泄漏） */
  clearFlushTimer: (convId: string) => void
  /** 待定位高亮的消息 ID（历史全文搜索结果跳转用）：MessageList 消费后自行清除 */
  pendingHighlightMessageId: Ref<string>
}

export function useConversations(deps: ConversationDeps) {
  const {
    conversations,
    currentConversationId,
    messages,
    generatingConvId,
    convStreamingStates,
    isLoadingConversations,
    lastError,
    messagesRequestVersion,
    pendingHighlightMessageId
  } = deps

  async function loadConversations() {
    isLoadingConversations.value = true
    try {
      const convs = await wails.getConversations()
      const newConvs = (convs as Conversation[]).map(c => ({ ...c, title: fixUtf8(c.title) }))
      const newIdSet = new Set(newConvs.map(c => c.id))
      const keptOld = conversations.value.filter(c => !newIdSet.has(c.id))
      conversations.value = [...keptOld, ...newConvs]

      for (const key of convStreamingStates.keys()) {
        if (!newIdSet.has(key) && key !== '') {
          convStreamingStates.delete(key)
        }
      }

      if (!currentConversationId.value && conversations.value.length > 0) {
        await selectConversation(conversations.value[0].id)
      }
      // 加载成功后清除错误
      lastError.value = ''
    } catch (e) {
      logError('加载会话列表失败', e)
      lastError.value = e instanceof Error ? e.message : String(e || '加载会话列表失败')
    } finally {
      isLoadingConversations.value = false
    }
  }

  async function selectConversation(id: string) {
    if (id === currentConversationId.value) return

    // 递增版本号，使 handleTerminalAsync 中进行中的旧请求失效（TOCTOU 防护）
    messagesRequestVersion.value++
    const requestVersion = messagesRequestVersion.value
    currentConversationId.value = id
    try {
      const msgs = await wails.getMessages(id)
      // await 返回后校验版本号，避免快速切换 A→B→C 时 B 的旧请求覆盖 C 的消息
      if (requestVersion !== messagesRequestVersion.value) return
      messages.value = msgs
    } catch (e) {
      if (requestVersion !== messagesRequestVersion.value) return
      logError('加载消息失败', e)
      messages.value = []
    }

    // 只在切换到的会话正在生成时才更新 generatingConvId，
    // 否则保留旧值（可能其他会话还在生成），避免破坏"当前会话/生成会话"解耦设计。
    // generatingConvId 的清空由 handleTerminalEvent 在生成结束时统一处理。
    const state = convStreamingStates.get(id)
    if (state && state.isGenerating) {
      generatingConvId.value = id
    }
  }

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
      logError('重命名会话失败', e)
    }
  }

  async function deleteConversation(id: string) {
    try {
      await wails.deleteConversation(id)
      conversations.value = conversations.value.filter((c: Conversation) => c.id !== id)
      convStreamingStates.delete(id)
      // 清理可能残留的 flush 定时器，防止孤儿定时器内存泄漏
      deps.clearFlushTimer(id)
      if (generatingConvId.value === id) generatingConvId.value = ''
      if (currentConversationId.value === id) {
        currentConversationId.value = ''
        messages.value = []
      }
    } catch (e) {
      logError('删除会话失败', e)
    }
  }

  async function searchMessages(query: string): Promise<Message[]> {
    try {
      return await wails.searchMessages(query)
    } catch (e) {
      logError('搜索消息失败', e)
      return []
    }
  }

  /**
   * 打开会话并定位到指定消息（历史全文搜索结果跳转）。
   * 时序：先确保目标会话已激活、消息已加载，再置 pendingHighlightMessageId，
   * MessageList 消费该值后滚动定位并高亮，完成后自行清空。
   * 若目标会话已处于激活态（消息已在内存），直接置位即可。
   */
  async function selectConversationAndLocate(id: string, messageId: string) {
    if (currentConversationId.value !== id) {
      await selectConversation(id)
    }
    pendingHighlightMessageId.value = messageId
  }

  async function exportConversation(id: string, format: string): Promise<string> {
    try {
      return await wails.exportConversation(id, format)
    } catch (e) {
      logError('导出会话失败', e)
      return ''
    }
  }

  async function exportConversationWithDialog(id: string, format: string): Promise<boolean> {
    try {
      return await wails.exportConversationWithDialog(id, format)
    } catch (e) {
      logError('导出会话失败', e)
      return false
    }
  }

  return {
    loadConversations,
    selectConversation,
    createConversation,
    renameConversation,
    deleteConversation,
    searchMessages,
    selectConversationAndLocate,
    exportConversation,
    exportConversationWithDialog
  }
}
