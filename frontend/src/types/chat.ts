/**
 * 集中类型定义：聊天 / 消息 / 模型 / 配置 / 状态 / 搜索
 */

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
    search_mode: string
    images?: string[]
    attachments?: Attachment[]
}

export interface StreamEvent {
    type: string
    content: any
    conversation_id?: string
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
    tool_call_support: boolean
}

export interface SearchAPIKeys {
    ollama_api_key: string
    tavily_api_key: string
    ollama_api_key_set: boolean
    tavily_api_key_set: boolean
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
    search_mode: string
    /** @deprecated 已由 reasoning 字段替代，保留向后兼容 */
    thinking_enabled: boolean
    /** @deprecated 已由 reasoning 字段替代，保留向后兼容 */
    thinking_soft_switch: 'auto' | 'think' | 'no_think'
    sleep_idle_seconds: number
    models_max: number
    rag_enabled: boolean
    rag_active_kb: string
    rag_top_k: number
    rag_min_score: number
    rag_chunk_size: number
    rag_chunk_overlap: number
    embedding_model: string
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
    spec_draft_n_min: number
    spec_ngram_mod_n_min: number
    spec_ngram_mod_n_max: number
    spec_ngram_mod_n_match: number
    spec_ngram_simple_size_n: number
    spec_ngram_simple_size_m: number
    spec_ngram_simple_min_hits: number
    spec_ngram_map_k_size_n: number
    spec_ngram_map_k_size_m: number
    spec_ngram_map_k_min_hits: number
    spec_ngram_map_k4v_size_n: number
    spec_ngram_map_k4v_size_m: number
    spec_ngram_map_k4v_min_hits: number
    lookup_cache_static: string
    lookup_cache_dynamic: string
    spec_draft_model: string
    cache_type_k_draft: string
    cache_type_v_draft: string
    server_api_key_enabled: boolean
    expose_server: boolean
    swa_full: boolean
    ctx_checkpoints: number
    checkpoint_min_step: number
    tools: string
    prefill_assistant: boolean
    slot_prompt_similarity: number
    skip_chat_parsing: boolean
    api_prefix: string
    simple_io: boolean
    gpu_layers: number
    flash_attn: boolean | null
    mlock: boolean | null
    threads: number
    batch_size: number
    close_action: string
    // 推理配置（对应后端 Reasoning / ReasoningBudget / ReasoningBudgetMessage / ReasoningFormat）
    reasoning: 'on' | 'off' | 'auto'
    reasoning_budget: number
    reasoning_budget_message: string
    reasoning_format: string
    // RAG 重排序配置
    reranker_model_path: string
    rerank_top_n: number
    // KV 缓存持久化配置
    slot_save_path: string
    slot_save_enabled: boolean
    // Draft 模型 GPU 配置
    spec_draft_ngl: number
    spec_draft_device: string
}

export interface SmartParamsInfo {
    hardware: {
        cpu_cores: number
        has_gpu: boolean
        gpu_name: string
        gpu_vram_mb: number
    }
    model: {
        architecture: string
        block_count: number
        embedding_length: number
        context_length: number
        file_size_mb: number
        expert_count: number
        expert_used: number
        has_mtp: boolean
        has_reasoning: boolean
        n_params: number
        size_label: string
    }
    params: {
        gpu_layers: number
        threads: number
        batch_size: number
        ubatch_size: number
        flash_attn: boolean
        cache_type_k: string
        cache_type_v: string
        mlock: boolean
        mmproj_offload: boolean
        context_size: number
        spec_type: string
        spec_draft_n_max: number
        spec_draft_n_min: number
        ngram_mod_n_min: number
        ngram_mod_n_max: number
        ngram_mod_n_match: number
    }
    overrides: {
        gpu_layers: boolean
        flash_attn: boolean
        mlock: boolean
        threads: boolean
        batch_size: boolean
        context_size: boolean
        cache_type_k: boolean
        cache_type_v: boolean
        spec_type: boolean
    }
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
    search_mode: 'off',
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
    embedding_model: '',
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
    spec_draft_n_min: 0,
    spec_ngram_mod_n_min: 0,
    spec_ngram_mod_n_max: 0,
    spec_ngram_mod_n_match: 0,
    spec_ngram_simple_size_n: 0,
    spec_ngram_simple_size_m: 0,
    spec_ngram_simple_min_hits: 0,
    spec_ngram_map_k_size_n: 0,
    spec_ngram_map_k_size_m: 0,
    spec_ngram_map_k_min_hits: 0,
    spec_ngram_map_k4v_size_n: 0,
    spec_ngram_map_k4v_size_m: 0,
    spec_ngram_map_k4v_min_hits: 0,
    lookup_cache_static: '',
    lookup_cache_dynamic: '',
    spec_draft_model: '',
    cache_type_k_draft: '',
    cache_type_v_draft: '',
    server_api_key_enabled: true,
    expose_server: false,
    swa_full: false,
    ctx_checkpoints: 0,
    checkpoint_min_step: 0,
    tools: '',
    prefill_assistant: true,
    slot_prompt_similarity: 0.0,
    skip_chat_parsing: false,
    api_prefix: '',
    simple_io: false,
    gpu_layers: 0,
    flash_attn: null,
    mlock: null,
    threads: 0,
    batch_size: 0,
    close_action: 'ask',
    reasoning: 'off',
    reasoning_budget: 0,
    reasoning_budget_message: '',
    reasoning_format: '',
    reranker_model_path: '',
    rerank_top_n: 5,
    slot_save_path: '',
    slot_save_enabled: false,
    spec_draft_ngl: 0,
    spec_draft_device: '',
}

export interface ServerStatus {
    running: boolean
    model_ready?: boolean
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
    rollback_success?: boolean
}

/** 流式状态：单个会话的临时状态 */
export interface ConvStreamingState {
    isGenerating: boolean
    streamingContent: string
    thinkingContent: string
    searchResults: string
    isSearching: boolean
    isThinking: boolean
    thinkingStartTime: number
    thinkingDuration: number
    searchQuery: string
    contextTrimmed: { reason: string; promptTokens?: number; contextSize?: number; messagesAfter?: number } | null
}
