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
 *
 * 生活类比：像文件柜管理——加载列表（loadConversations）、打开文件夹（selectConversation）、
 * 新建文件夹（createConversation）、重命名（renameConversation）、删除（deleteConversation）、
 * 搜索内容（searchMessages）、导出（exportConversation）。
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
    messagesRequestVersion
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

    // 递增版本号，使 handleTerminalAsync 中进行中的旧请求失效（M-前2 TOCTOU 防护）
    messagesRequestVersion.value++
    currentConversationId.value = id
    try {
      messages.value = await wails.getMessages(id)
    } catch (e) {
      logError('加载消息失败', e)
      messages.value = []
    }

    const state = convStreamingStates.get(id)
    if (state) {
      generatingConvId.value = state.isGenerating ? id : ''
    } else {
      generatingConvId.value = ''
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
    exportConversation,
    exportConversationWithDialog
  }
}
