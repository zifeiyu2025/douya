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
    text_input: boolean
    reasoning: boolean
    mmproj_loaded: boolean
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

export interface Config {
    model_path: string
    mmproj_path?: string
    mmproj_auto?: boolean
    mmproj_offload?: boolean
    llama_server_path: string
    api_base: string
    port: number
    context_size: number
    temperature: number
    top_p: number
    top_k: number
    repeat_penalty: number
    kv_unified?: boolean
    cache_idle_slots?: boolean
    cache_ram?: number
    image_min_tokens?: number
    image_max_tokens?: number
    fit_target?: number
    fit_ctx?: number
    reasoning?: string
    reasoning_budget?: number
    reasoning_format?: string
    system_prompt: string
    chat_background?: string
    user_avatar?: string
    ai_avatar?: string
    search_engines?: {
        ollama_api_key?: string
        tavily_api_key?: string
        github_api_key?: string
    }
    search_enabled: boolean
    sleep_idle_seconds?: number
    models_max?: number
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
    getConfig: GetConfig as () => Promise<unknown> as () => Promise<Config>,
    getCleanupResult: GetCleanupResult as () => Promise<Array<{ id: string, title: string, reason: string }>>,
    updateConfig: (cfg: Config) => UpdateConfig(cfg as any),
    getServerStatus: GetServerStatus as () => Promise<ServerStatus>,
    deleteMessage: DeleteMessage as (id: string) => Promise<void>,
    regenerateMessage: RegenerateMessage as (userMessageID: string, searchEnabled: boolean) => Promise<void>,
    getAvailableModels: GetAvailableModels as () => Promise<ModelOption[]>,
    switchModel: SwitchModel as unknown as (modelName: string) => Promise<SwitchResult>,
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
}
