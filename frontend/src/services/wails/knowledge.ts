/**
 * Wails 服务门面 - 知识库域
 * 知识库 CRUD/文档管理/RAG 开关/联网搜索 API Keys
 * （从原 wails.ts 迁移,方法体逐字搬移,逻辑零变化）
 */
import {
  ListKnowledgeBases,
  CreateKnowledgeBase,
  DeleteKnowledgeBase,
  UploadDocument,
  ListDocuments,
  DeleteDocument as DeleteDocumentAPI,
  SetActiveKnowledgeBase,
  GetActiveKnowledgeBase,
  SetRAGEnabled,
  IsRAGEnabled,
  GetSearchAPIKeys,
  SetSearchAPIKeys,
  HasServerAPIKey,
  GenerateServerAPIKey
} from '../../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime'
import { EventStartupRagDisabled } from '../events'
import type { SearchAPIKeys } from '../../types/chat'
import type { CollectionInfo, DocumentMeta } from '../../types/search'

export const knowledgeMethods = {
  listKnowledgeBases: async (): Promise<CollectionInfo[]> => {
    return (await ListKnowledgeBases()) as CollectionInfo[]
  },
  createKnowledgeBase: async (name: string): Promise<void> => {
    await CreateKnowledgeBase(name)
  },
  deleteKnowledgeBase: async (name: string): Promise<void> => {
    await DeleteKnowledgeBase(name)
  },
  uploadDocument: async (
    kbName: string,
    fileName: string,
    fileData: string,
    mimeType: string
  ): Promise<void> => {
    await UploadDocument(kbName, fileName, fileData, mimeType)
  },
  listDocuments: async (kbName: string): Promise<DocumentMeta[]> => {
    return (await ListDocuments(kbName)) as DocumentMeta[]
  },
  deleteDocument: async (kbName: string, docID: string): Promise<void> => {
    await DeleteDocumentAPI(kbName, docID)
  },
  setActiveKnowledgeBase: async (kbName: string): Promise<void> => {
    await SetActiveKnowledgeBase(kbName)
  },
  getActiveKnowledgeBase: async (): Promise<string> => {
    return (await GetActiveKnowledgeBase()) as string
  },
  setRAGEnabled: async (enabled: boolean): Promise<void> => {
    await SetRAGEnabled(enabled)
  },
  isRAGEnabled: async (): Promise<boolean> => {
    return (await IsRAGEnabled()) as boolean
  },
  getSearchAPIKeys: async (): Promise<SearchAPIKeys> => {
    return (await GetSearchAPIKeys()) as SearchAPIKeys
  },
  setSearchAPIKeys: async (keys: SearchAPIKeys): Promise<void> => {
    await SetSearchAPIKeys(keys)
  },
  hasServerAPIKey: async (): Promise<boolean> => {
    return (await HasServerAPIKey()) as boolean
  },
  generateServerAPIKey: async (): Promise<string> => {
    return (await GenerateServerAPIKey()) as string
  },
  // 知识库（RAG）初始化失败：非阻塞提示"知识库已禁用"
  subscribeRagDisabled: (callback: (data: { detail: string }) => void): (() => void) => {
    EventsOn(EventStartupRagDisabled, callback)
    return () => EventsOff(EventStartupRagDisabled)
  }
} as const
