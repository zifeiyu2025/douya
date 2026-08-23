/**
 * Wails 服务门面 - 参数适配层
 * 后端 snake_case 与前端 snake_case 字段一致,这里仅做基本类型转接
 * （从原 wails.ts 迁移,逻辑零变化;导出供各域文件复用）
 */
import {
  SendMessage,
  UpdateConfig,
  CountTokens,
  SetLoraAdapters
} from '../../../wailsjs/go/main/App'
import { chat as ChatModel } from '../../../wailsjs/go/models'
import type { Config } from '../../types/chat'
import type { ChatMessage, LoraAdapter } from './types'
import { DEFAULT_CONFIG } from '../../types/chat'

/**
 * 适配 wails 生成的 binding 类型。
 * 后端 snake_case 与前端 snake_case 字段一致,所以这里仅做基本类型转接。
 */
export function adaptConfig(raw: unknown): Config {
  if (!raw || typeof raw !== 'object') {
    return { ...DEFAULT_CONFIG }
  }
  return { ...DEFAULT_CONFIG, ...(raw as Partial<Config>) }
}

/**
 * 将前端 SendMessageParams 转换为 wails SendMessage 期望的参数类型。
 * ChatModel.SendMessageParams.createFrom 已构造 wails 端 class 实例，
 * 此处仅做精确的字段映射与类型断言，避免调用点使用 as unknown as（任务 23）。
 */
export function toWailsSendMessageParams(
  params: ChatModel.SendMessageParams
): Parameters<typeof SendMessage>[0] {
  return {
    conversation_id: params.conversation_id,
    content: params.content,
    search_mode: params.search_mode,
    images: params.images,
    attachments: params.attachments
  } as Parameters<typeof SendMessage>[0]
}

/**
 * 将前端 Config 转换为 wails UpdateConfig 期望的参数类型。
 * 前端 Config 是 wails config.Config 的超集（含 UI 专用字段），
 * 后端 JSON 反序列化时会忽略多余字段，因此直接展开即可。
 */
export function toWailsConfig(cfg: Config): Parameters<typeof UpdateConfig>[0] {
  return { ...cfg } as Parameters<typeof UpdateConfig>[0]
}

/**
 * 将前端 ChatMessage[] 转换为 CountTokens/ApplyTemplate 期望的参数类型。
 * 显式映射每个字段，避免 as any 导致的类型擦除（任务 23）。
 */
export function toWailsChatMessages(messages: ChatMessage[]): Parameters<typeof CountTokens>[0] {
  return messages.map(m => ({
    role: m.role,
    content: m.content,
    reasoning_content: m.reasoning_content,
    tool_call_id: m.tool_call_id
  })) as Parameters<typeof CountTokens>[0]
}

/**
 * 将前端 LoraAdapter[] 转换为 SetLoraAdapters 期望的参数类型。
 * 字段完全一致，映射后断言为 wails 参数类型（任务 23）。
 */
export function toWailsLoraAdapters(
  adapters: LoraAdapter[]
): Parameters<typeof SetLoraAdapters>[0] {
  return adapters.map(a => ({
    id: a.id,
    path: a.path,
    scale: a.scale
  })) as Parameters<typeof SetLoraAdapters>[0]
}
