/**
 * Wails 服务门面
 * 集中所有 Wails binding 的类型适配,避免散落在业务代码中的 `as unknown` 类型擦除
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
    GetConfig,
    GetCleanupResult,
    UpdateConfig,
    GetServerStatus,
    DeleteMessage,
    RegenerateMessage,
    PrepareShutdown,
    GetAvailableModels,
    SwitchModel,
    ReloadModels,
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
    SetServerAPIKey,
    SelectImageFile,
    GetSmartParams,
    HandleCloseRequest,
    SetCloseAction,
    GracefulExit,
    StopThinking,
    RerankEnabled,
    SaveSlot,
    RestoreSlot,
} from '../../wailsjs/go/main/App'
import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'
import { chat as ChatModel } from '../../wailsjs/go/models'

import type {
    Conversation,
    Message,
    SendMessageParams,
    Config,
    ServerStatus,
    ModelOption,
    SwitchResult,
    StreamEvent,
    SearchAPIKeys,
    Attachment,
    AttachmentSummary,
    ModelCapabilities,
    SmartParamsInfo,
} from '../types/chat'
import type { CollectionInfo, DocumentMeta } from '../types/search'
import { DEFAULT_CONFIG } from '../types/chat'

// 重新导出类型,保持原导入路径兼容
export type {
    Conversation,
    Message,
    SendMessageParams,
    Config,
    ServerStatus,
    ModelOption,
    SwitchResult,
    StreamEvent,
    SearchAPIKeys,
    Attachment,
    AttachmentSummary,
    ModelCapabilities,
    SmartParamsInfo,
    CollectionInfo,
    DocumentMeta,
}
export { DEFAULT_CONFIG }

/** 切换进度事件 */
export interface SwitchProgressEvent {
    stage: string
    targetModel: string
}

/** 关闭进度事件 */
export interface ShutdownProgressEvent {
    stage: string
    message: string
}

/** 异常清理事件 */
export interface AbnormalCleanupEvent {
    count: number
    removed: Array<{ id: string, title: string, reason: string }>
}

/** 清理结果 */
export interface CleanupResult {
    id: string
    title: string
    reason: string
}

/** 导出格式 */
export type ExportFormat = 'markdown' | 'json' | 'txt' | 'csv'

/**
 * 适配 wails 生成的 binding 类型。
 * 后端 snake_case 与前端 snake_case 字段一致,所以这里仅做基本类型转接。
 */
function adaptConfig(raw: unknown): Config {
    if (!raw || typeof raw !== 'object') {
        return { ...DEFAULT_CONFIG }
    }
    return { ...DEFAULT_CONFIG, ...(raw as Partial<Config>) }
}

export const wails = {
    sendMessage: async (params: SendMessageParams): Promise<void> => {
        // 使用 wailsjs 生成的 createFrom 适配类型
        const wailsParams = ChatModel.SendMessageParams.createFrom({
            conversation_id: params.conversation_id,
            content: params.content,
            search_mode: params.search_mode,
            images: params.images,
            attachments: params.attachments,
        })
        await SendMessage(wailsParams as unknown as Parameters<typeof SendMessage>[0])
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
    getConfig: async (): Promise<Config> => adaptConfig(await GetConfig()),
    getSmartParams: async (): Promise<SmartParamsInfo> => {
        return (await GetSmartParams()) as SmartParamsInfo
    },
    getCleanupResult: async (): Promise<CleanupResult[]> => {
        return (await GetCleanupResult()) as CleanupResult[]
    },
    updateConfig: async (cfg: Config): Promise<void> => {
        await UpdateConfig(cfg as unknown as Parameters<typeof UpdateConfig>[0])
    },
    handleCloseRequest: async (): Promise<string> => {
        return await HandleCloseRequest()
    },
    setCloseAction: async (action: string): Promise<void> => {
        await SetCloseAction(action)
    },
    gracefulExit: async (): Promise<void> => {
        await GracefulExit()
    },
    stopThinking: async (): Promise<void> => {
        await StopThinking()
    },
    rerankEnabled: async (): Promise<boolean> => {
        return await RerankEnabled()
    },
    saveSlot: async (slotID: number): Promise<void> => {
        await SaveSlot(slotID)
    },
    restoreSlot: async (slotID: number): Promise<void> => {
        await RestoreSlot(slotID)
    },
    getServerStatus: async (): Promise<ServerStatus> => {
        return (await GetServerStatus()) as ServerStatus
    },
    deleteMessage: async (id: string): Promise<void> => {
        await DeleteMessage(id)
    },
    regenerateMessage: async (userMessageID: string, searchMode: string): Promise<void> => {
        await RegenerateMessage(userMessageID, searchMode)
    },
    getAvailableModels: async (): Promise<ModelOption[]> => {
        return (await GetAvailableModels()) as ModelOption[]
    },
    switchModel: async (modelName: string): Promise<SwitchResult> => {
        return (await SwitchModel(modelName)) as SwitchResult
    },
    reloadModels: async (): Promise<void> => {
        await ReloadModels()
    },
    onChatStream: (callback: (event: StreamEvent) => void) => {
        EventsOn('chat:stream', callback)
    },
    onServerStatus: (callback: (status: ServerStatus) => void) => {
        EventsOn('server:status', callback)
    },
    offChatStream: () => EventsOff('chat:stream'),
    offServerStatus: () => EventsOff('server:status'),
    prepareShutdown: PrepareShutdown,
    onAbnormalCleanup: (callback: (data: AbnormalCleanupEvent) => void) => {
        EventsOn('chat:abnormal_cleanup', callback)
    },
    offAbnormalCleanup: () => EventsOff('chat:abnormal_cleanup'),
    onSwitchProgress: (callback: (progress: SwitchProgressEvent) => void) => {
        EventsOn('server:switchProgress', callback)
    },
    offSwitchProgress: () => EventsOff('server:switchProgress'),
    onMmprojUnavailable: (callback: () => void) => {
        EventsOn('server:mmprojUnavailable', callback)
    },
    offMmprojUnavailable: () => EventsOff('server:mmprojUnavailable'),
    onShutdownProgress: (callback: (progress: ShutdownProgressEvent) => void) => {
        EventsOn('shutdown:progress', callback)
    },
    offShutdownProgress: () => EventsOff('shutdown:progress'),
    listKnowledgeBases: async (): Promise<CollectionInfo[]> => {
        return (await ListKnowledgeBases()) as CollectionInfo[]
    },
    createKnowledgeBase: async (name: string): Promise<void> => {
        await CreateKnowledgeBase(name)
    },
    deleteKnowledgeBase: async (name: string): Promise<void> => {
        await DeleteKnowledgeBase(name)
    },
    uploadDocument: async (kbName: string, fileName: string, fileData: string, mimeType: string): Promise<void> => {
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
    setServerAPIKey: async (key: string): Promise<void> => {
        await SetServerAPIKey(key)
    },
    selectImageFile: async (): Promise<string> => {
        return (await SelectImageFile()) as string
    },
} as const

export type WailsService = typeof wails
