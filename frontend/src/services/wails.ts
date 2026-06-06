export interface Conversation {
    id: string
    title: string
    created_at: string
    updated_at: string
}

export interface AttachmentSummary {
    type: string
    name: string
    mime_type: string
}

export interface Message {
    id: string
    conversation_id: string
    role: string
    content: string
    thinking_content?: string
    thinking_duration?: number
    search_results: string
    images?: string
    attachments?: AttachmentSummary[]
    created_at: string
}

export interface ModelCapabilities {
    image_input: boolean
    audio_input: boolean
    video_input: boolean
    text_input: boolean
    reasoning: boolean
    mmproj_loaded: boolean
    has_mtp: boolean
    thinking_mode: string
    soft_switch_support: boolean
    n_params: number
}

export interface Attachment {
    type: string
    name: string
    mime_type: string
    data: string
    format?: string
}

export interface SendMessageParams {
    conversation_id: string
    content: string
    search_enabled: boolean
    images?: string[]
    attachments?: Attachment[]
}

export interface SearchAPIKeys {
    ollama_api_key: string
    tavily_api_key: string
    github_api_key: string
}

export interface Config {
    model_path: string
    mmproj_auto: boolean
    mmproj_offload: boolean
    llama_server_path: string
    api_base: string
    port: number
    context_size: number
    temperature: number
    top_p: number
    top_k: number
    repeat_penalty: number
    kv_unified: boolean
    cache_idle_slots: boolean
    cache_ram: number
    image_min_tokens: number
    image_max_tokens: number
    fit_target: number
    fit_ctx: number
    system_prompt: string
    chat_background: string
    chat_background_opacity: number
    user_avatar: string
    ai_avatar: string
    search_enabled: boolean
    thinking_enabled: boolean
    thinking_soft_switch: 'auto' | 'think' | 'no_think'
    sleep_idle_seconds: number
    models_max: number
    rag_enabled: boolean
    rag_active_kb: string
    rag_top_k: number
    rag_min_score: number
    rag_chunk_size: number
    rag_chunk_overlap: number
    mmap: boolean
    kv_offload: boolean
    context_shift: boolean
    min_p: number
    dry_multiplier: number
    dry_base: number
    dry_allowed_length: number
    device: string
    parallel: number
    cache_type_k: string
    cache_type_v: string
    spec_type: string
    spec_draft_n_max: number
    cache_type_k_draft: string
    cache_type_v_draft: string
    server_api_key_enabled: boolean
}

export const DEFAULT_CONFIG: Config = {
    model_path: '',
    mmproj_auto: true,
    mmproj_offload: true,
    llama_server_path: 'runtime/llama-server.exe',
    api_base: 'http://127.0.0.1:8080',
    port: 8080,
    context_size: 8192,
    temperature: 0.6,
    top_p: 0.95,
    top_k: 20,
    repeat_penalty: 1,
    kv_unified: false,
    cache_idle_slots: false,
    cache_ram: 0,
    image_min_tokens: 0,
    image_max_tokens: 0,
    fit_target: 0,
    fit_ctx: 0,
    system_prompt: '',
    chat_background: '',
    chat_background_opacity: 0.8,
    user_avatar: '',
    ai_avatar: '',
    search_enabled: false,
    thinking_enabled: true,
    thinking_soft_switch: 'auto',
    sleep_idle_seconds: 120,
    models_max: 1,
    rag_enabled: false,
    rag_active_kb: 'default',
    rag_top_k: 3,
    rag_min_score: 0.3,
    rag_chunk_size: 512,
    rag_chunk_overlap: 64,
    mmap: true,
    kv_offload: true,
    context_shift: false,
    min_p: 0.05,
    dry_multiplier: 0,
    dry_base: 1.75,
    dry_allowed_length: 2,
    device: '',
    parallel: 0,
    cache_type_k: '',
    cache_type_v: '',
    spec_type: '',
    spec_draft_n_max: 0,
    cache_type_k_draft: '',
    cache_type_v_draft: '',
    server_api_key_enabled: true,
}

export interface ServerStatus {
    running: boolean
    error?: string
    switching?: boolean
    switching_to?: string
    current_model?: string
    capabilities?: ModelCapabilities
}

export interface ModelOption {
    name: string
    model_path: string
    file_name: string
    is_default: boolean
    is_loaded: boolean
    mmproj_vision: boolean
    mmproj_audio: boolean
    mmproj_video: boolean
    status: string
}

export interface SwitchResult {
    success: boolean
    error?: string
    current_model?: string
    capabilities?: ModelCapabilities
    previous_model?: string
    rolled_back?: boolean
}

export interface StreamEvent {
    type: string
    content: any
    conversation_id?: string
}

export interface CollectionInfo {
    name: string
    dim: number
    vector_count: number
}

export interface DocumentMeta {
    id: string
    collection: string
    file_name: string
    file_size: number
    mime_type: string
    chunk_count: number
    ingested_at: string
    tags?: Record<string, string>
}

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
} from '../../wailsjs/go/main/App'

import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime'

export const wails = {
    sendMessage: SendMessage,
    stopGeneration: StopGeneration,
    getConversations: GetConversations as () => Promise<Conversation[]>,
    getMessages: GetMessages as (conversationID: string) => Promise<Message[]>,
    createConversation: CreateConversation as () => Promise<Conversation>,
    renameConversation: RenameConversation,
    deleteConversation: DeleteConversation,
    searchMessages: SearchMessages as (query: string) => Promise<Message[]>,
    exportConversation: ExportConversation,
    exportConversationWithDialog: ExportConversationWithDialog as (id: string, format: string) => Promise<boolean>,
    getConfig: async (): Promise<Config> => {
        const raw = await GetConfig()
        return { ...DEFAULT_CONFIG, ...(raw as unknown as Record<string, unknown>) }
    },
    getCleanupResult: GetCleanupResult as () => Promise<Array<{ id: string, title: string, reason: string }>>,
    updateConfig: async (cfg: Config): Promise<void> => {
        await UpdateConfig(cfg as unknown as Parameters<typeof UpdateConfig>[0])
    },
    getServerStatus: GetServerStatus as () => Promise<ServerStatus>,
    deleteMessage: DeleteMessage as (id: string) => Promise<void>,
    regenerateMessage: RegenerateMessage as (userMessageID: string, searchEnabled: boolean) => Promise<void>,
    getAvailableModels: GetAvailableModels as () => Promise<ModelOption[]>,
    switchModel: async (modelName: string): Promise<SwitchResult> => {
        return await SwitchModel(modelName) as SwitchResult
    },
    reloadModels: ReloadModels as () => Promise<void>,
    onChatStream: (callback: (event: StreamEvent) => void) => {
        EventsOn('chat:stream', callback)
    },
    onServerStatus: (callback: (status: ServerStatus) => void) => {
        EventsOn('server:status', callback)
    },
    offChatStream: () => EventsOff('chat:stream'),
    offServerStatus: () => EventsOff('server:status'),
    prepareShutdown: PrepareShutdown,
    onAbnormalCleanup: (callback: (data: { count: number, removed: Array<{ id: string, title: string, reason: string }> }) => void) => {
        EventsOn('chat:abnormal_cleanup', callback)
    },
    offAbnormalCleanup: () => EventsOff('chat:abnormal_cleanup'),
    onSwitchProgress: (callback: (progress: { stage: string, targetModel: string }) => void) => {
        EventsOn('server:switchProgress', callback)
    },
    offSwitchProgress: () => EventsOff('server:switchProgress'),
    onShutdownProgress: (callback: (progress: { stage: string, message: string }) => void) => {
        EventsOn('shutdown:progress', callback)
    },
    offShutdownProgress: () => EventsOff('shutdown:progress'),
    listKnowledgeBases: ListKnowledgeBases as () => Promise<CollectionInfo[]>,
    createKnowledgeBase: CreateKnowledgeBase as (name: string) => Promise<void>,
    deleteKnowledgeBase: DeleteKnowledgeBase as (name: string) => Promise<void>,
    uploadDocument: UploadDocument as (kbName: string, fileName: string, fileData: string, mimeType: string) => Promise<void>,
    listDocuments: ListDocuments as (kbName: string) => Promise<DocumentMeta[]>,
    deleteDocument: DeleteDocumentAPI as (kbName: string, docID: string) => Promise<void>,
    setActiveKnowledgeBase: SetActiveKnowledgeBase as (kbName: string) => Promise<void>,
    getActiveKnowledgeBase: GetActiveKnowledgeBase as () => Promise<string>,
    setRAGEnabled: SetRAGEnabled as (enabled: boolean) => Promise<void>,
    isRAGEnabled: IsRAGEnabled as () => Promise<boolean>,
    getSearchAPIKeys: GetSearchAPIKeys as () => Promise<SearchAPIKeys>,
    setSearchAPIKeys: SetSearchAPIKeys as (keys: SearchAPIKeys) => Promise<void>,
    hasServerAPIKey: HasServerAPIKey as () => Promise<boolean>,
    setServerAPIKey: SetServerAPIKey as (key: string) => Promise<void>,
    selectImageFile: SelectImageFile as () => Promise<string>,
}
