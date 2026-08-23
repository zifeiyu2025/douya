/**
 * Wails 服务门面 - 聚合出口
 * 将各域方法聚合为单一 wails 对象,保持与拆分前完全一致的对外签名。
 * TS 会把 '../services/wails' 解析到本目录的 index.ts,
 * 因此全项目调用点无需任何改动（目录模块解析机制）。
 *
 * 域划分：
 * - chat.ts       聊天/会话/槽位/token 工具
 * - models.ts     模型列表/切换/LoRA/模型市场
 * - server.ts     服务器状态/日志/终端/优雅关闭
 * - knowledge.ts  知识库/RAG/搜索 API Keys
 * - mcp.ts        MCP 服务器管理
 * - backend.ts    显卡后端管理
 * - system.ts     配置/更新/TTS/启动期对话框
 */
import { chatMethods } from './chat'
import { modelMethods } from './models'
import { serverMethods } from './server'
import { knowledgeMethods } from './knowledge'
import { mcpMethods } from './mcp'
import { backendMethods } from './backend'
import { systemMethods } from './system'

export * from './types'

export const wails = {
  ...chatMethods,
  ...modelMethods,
  ...serverMethods,
  ...knowledgeMethods,
  ...mcpMethods,
  ...backendMethods,
  ...systemMethods
} as const

export type WailsService = typeof wails
