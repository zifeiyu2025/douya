/**
 * Wails 服务门面 - 聊天域
 * 发送/流式订阅/消息管理/会话 CRUD/槽位/token 工具
 * （从原 wails.ts 迁移,方法体逐字搬移,逻辑零变化）
 */
import {
  SendMessage,
  StopGeneration,
  GetConversations,
  GetMessages,
  CreateConversation,
  RenameConversation,
  DeleteConversation,
  SearchMessages,
  ExportConversation,
  ExportConversationWithDialog,
  DeleteMessage,
  EditMessage,
  CompressConversation,
  RegenerateMessage,
  CountTokens,
  Tokenize,
  GetLastPromptTokens,
  ApplyTemplate,
  StopThinking,
  SaveSlot,
  RestoreSlot,
  EraseSlot,
  GetSlots
} from '../../../wailsjs/go/main/App'
import { chat as ChatModel } from '../../../wailsjs/go/models'
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime'
import { EventChatStream, EventChatAbnormalCleanup } from '../events'
import type { Conversation, Message, SendMessageParams, StreamEvent } from '../../types/chat'
import type { AbnormalCleanupEvent, ChatMessage, CompressResult, SlotInfo } from './types'
import { toWailsChatMessages, toWailsSendMessageParams } from './adapters'

export const chatMethods = {
  sendMessage: async (params: SendMessageParams): Promise<void> => {
    // 使用 wailsjs 生成的 createFrom 适配类型
    const wailsParams = ChatModel.SendMessageParams.createFrom({
      conversation_id: params.conversation_id,
      content: params.content,
      search_mode: params.search_mode,
      images: params.images,
      attachments: params.attachments
    })
    await SendMessage(toWailsSendMessageParams(wailsParams))
  },
  stopGeneration: StopGeneration,
  getConversations: async (): Promise<Conversation[]> => {
    return (await GetConversations()) as Conversation[]
  },
  getMessages: async (conversationID: string): Promise<Message[]> => {
    return (await GetMessages(conversationID)) as Message[]
  },
  createConversation: async (): Promise<Conversation> => {
    return (await CreateConversation()) as Conversation
  },
  renameConversation: RenameConversation,
  deleteConversation: DeleteConversation,
  searchMessages: async (query: string): Promise<Message[]> => {
    return (await SearchMessages(query)) as Message[]
  },
  exportConversation: ExportConversation,
  exportConversationWithDialog: async (id: string, format: string): Promise<boolean> => {
    return (await ExportConversationWithDialog(id, format)) as boolean
  },
  stopThinking: async (): Promise<void> => {
    await StopThinking()
  },
  saveSlot: async (conversationId: string): Promise<void> => {
    await SaveSlot(conversationId)
  },
  restoreSlot: async (conversationId: string): Promise<void> => {
    await RestoreSlot(conversationId)
  },
  eraseSlot: async (slotID: number): Promise<void> => {
    await EraseSlot(slotID)
  },
  getSlots: async (): Promise<SlotInfo[]> => {
    return (await GetSlots()) as SlotInfo[]
  },
  countTokens: async (messages: ChatMessage[]): Promise<number> => {
    return await CountTokens(toWailsChatMessages(messages))
  },
  tokenize: async (text: string): Promise<number[]> => {
    return await Tokenize(text)
  },
  getLastPromptTokens: async (): Promise<number> => {
    return await GetLastPromptTokens()
  },
  applyTemplate: async (messages: ChatMessage[]): Promise<string> => {
    return await ApplyTemplate(toWailsChatMessages(messages))
  },
  deleteMessage: async (id: string): Promise<void> => {
    await DeleteMessage(id)
  },
  // C-4：消息编辑——后端仅落库新内容（单一职责），
  // "截断后续 + 重新生成"的编排由前端驱动（复用 chatStore.regenerateMessage）
  editMessage: async (messageID: string, newContent: string): Promise<void> => {
    await EditMessage(messageID, newContent)
  },
  // P2-A3: 手动触发对话压缩
  compressConversation: async (convID: string): Promise<CompressResult> => {
    return (await CompressConversation(convID)) as CompressResult
  },
  regenerateMessage: async (userMessageID: string, searchMode: string): Promise<void> => {
    await RegenerateMessage(userMessageID, searchMode)
  },
  // F-1.10+F-3.5：事件订阅统一为 subscribeXxx(callback): () => void 模式
  // 返回的 unsubscribe 函数用于取消订阅，替代原来的 onXxx/offXxx 配对调用
  // 生活类比：像订报纸——订阅（subscribe）后拿到一个"退订凭证"（unsubscribe 函数），
  // 想不看了就凭它退订，不用记住自己订的是哪份报纸（事件名）。
  subscribeChatStream: (callback: (event: StreamEvent) => void): (() => void) => {
    EventsOn(EventChatStream, callback)
    return () => EventsOff(EventChatStream)
  },
  subscribeAbnormalCleanup: (callback: (data: AbnormalCleanupEvent) => void): (() => void) => {
    EventsOn(EventChatAbnormalCleanup, callback)
    return () => EventsOff(EventChatAbnormalCleanup)
  }
} as const
