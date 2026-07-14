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
    tokens_per_second?: number  // 生成速度（tokens/s），仅流式完成时携带
    predicted_n?: number        // 生成的 token 数
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

/** 搜索结果项（对齐 Go search.SearchResult） */
export interface SearchResult {
    title: string
    url: string
    snippet: string
    raw_content?: string
    score?: number
}

/** 事件基础接口：所有事件均可能携带 conversation_id */
interface StreamEventBase {
    conversation_id?: string
}

/** token 增量（content 为增量文本） */
export interface TokenEvent extends StreamEventBase {
    type: 'token'
    content: string
}

/** 思考内容增量（content 为增量思考文本） */
export interface ThinkingEvent extends StreamEventBase {
    type: 'thinking'
    content: string
}

/** 工具调用开始（content 为工具名与查询参数，含 tool_call_id 用于并发关联） */
export interface ToolCallStartEvent extends StreamEventBase {
    type: 'tool_call_start'
    content: { tool_call_id: string; tool: string; query: string }
}

/** 搜索开始（content 为搜索查询参数字符串） */
export interface SearchStartEvent extends StreamEventBase {
    type: 'search_start'
    content: string
}

/** 搜索结果（content 为含 tool_call_id 的结果对象，用于并发 tool call 关联） */
export interface SearchResultEvent extends StreamEventBase {
    type: 'search_result'
    content: { tool_call_id: string; results: SearchResult[] } | SearchResult[]
}

/** 生成速度（content 为速度统计） */
export interface TokenSpeedEvent extends StreamEventBase {
    type: 'token_speed'
    content: { tokensPerSecond?: number; predictedN?: number; tokens_per_second?: number }
}

/** 提示词处理进度（content 为进度统计） */
export interface PromptProgressEvent extends StreamEventBase {
    type: 'prompt_progress'
    content: { total?: number; cache?: number; processed?: number; timeMs?: number }
}

/** 上下文裁剪通知（content 为裁剪详情） */
export interface ContextTrimmedEvent extends StreamEventBase {
    type: 'context_trimmed'
    content: { reason: string; prompt_tokens?: number; context_size?: number; messages_after?: number }
}

/** 生成完成（无 content） */
export interface DoneEvent extends StreamEventBase {
    type: 'done'
    content?: null
}

/** 生成停止（无 content） */
export interface StoppedEvent extends StreamEventBase {
    type: 'stopped'
    content?: null
}

/** 错误（content 为错误消息） */
export interface ErrorEvent extends StreamEventBase {
    type: 'error'
    content: string
}

/** 会话创建（content 为新会话） */
export interface ConversationCreatedEvent extends StreamEventBase {
    type: 'conversation_created'
    content: Conversation
}

/** 助手消息（content 为消息对象） */
export interface AssistantMessageEvent extends StreamEventBase {
    type: 'assistant_message'
    content: Message
}

/** 用户消息（content 为消息对象） */
export interface UserMessageEvent extends StreamEventBase {
    type: 'user_message'
    content: Message
}

/** 会话更新（content 为会话对象） */
export interface ConversationUpdatedEvent extends StreamEventBase {
    type: 'conversation_updated'
    content: Conversation
}

/** 会话删除（content 为会话 ID 或含 id 的对象） */
export interface ConversationDeletedEvent extends StreamEventBase {
    type: 'conversation_deleted'
    content: string | { id: string }
}

/** 消息删除（content 为消息 ID 或含 id 的对象） */
export interface MessageDeletedEvent extends StreamEventBase {
    type: 'message_deleted'
    content: string | { id: string }
}

/**
 * 流式事件判别联合类型（任务 31，L-18 + 前端-15）
 *
 * 以 Go 后端 internal/chat/service_stream.go 的 emitForConv 调用为准，
 * 为每类事件定义具体的 content 类型，替代原先的 { type: string; content: any }。
 *
 * 渐进式迁移说明：
 * - 当前 streamHandlers 的 handler 仍保留 (convId, content: any, isCurrentConv) 签名，
 *   部分 handler 内部保留 typeof/JSON.parse 兜底，待逐个切换后删除。
 * - 新增 handler 应直接使用 Extract<StreamEvent, { type: 'xxx' }> 推导 content 类型。
 * - 各事件 content 类型与 Go 发送侧严格对齐（参见 service_stream.go 中 emitForConv 调用）。
 *
 * 使用方可通过 Extract<StreamEvent, { type: 'token' }> 窄化具体事件。
 */
export type StreamEvent =
    | TokenEvent
    | ThinkingEvent
    | ToolCallStartEvent
    | SearchStartEvent
    | SearchResultEvent
    | TokenSpeedEvent
    | PromptProgressEvent
    | ContextTrimmedEvent
    | DoneEvent
    | StoppedEvent
    | ErrorEvent
    | ConversationCreatedEvent
    | AssistantMessageEvent
    | UserMessageEvent
    | ConversationUpdatedEvent
    | ConversationDeletedEvent
    | MessageDeletedEvent

/** 所有合法的事件类型字面量（用于 streamHandlers 的键约束） */
export type StreamEventType = StreamEvent['type']

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
    supports_preserve_reasoning: boolean
}

export interface SearchAPIKeys {
    ollama_api_key: string
    tavily_api_key: string
    ollama_api_key_set: boolean
    tavily_api_key_set: boolean
}

/**
 * 配置类型（与后端 internal/config/config.go 的 Config struct 对齐）
 *
 * 路线图：未来可用 `go generate` 从 Go struct 自动生成 TS interface，
 * 避免手动维护两份字段清单导致漂移（参见任务 29.5 评估）。
 * 当前仍手工维护，新增字段时务必同步 Go Config / TS Config / DEFAULT_CONFIG 三处。
 */
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
    cache_reuse: number
    cache_ram: number
    image_min_tokens: number
    image_max_tokens: number
    fit_target: number
    fit_ctx: number
    system_prompt: string
    // 系统提示词模式："append"（追加）或 "replace"（替换），默认 "append"
    system_prompt_mode: 'append' | 'replace' | ''
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
    dry_sequence_breaker: string   // Dry 采样序列中断符（逗号分隔）
    dry_penalty_last_n: number     // Dry 惩罚窗口大小
    grp_attn_n: number             // 分组注意力组数
    grp_attn_w: number             // 分组注意力窗口宽度
    // 注意：后端 Go 为 *bool（nil=不传递），TS 侧统一为 boolean | null，nil 对应 null
    jinja: boolean | null                 // Jinja2 模板引擎开关
    // 注意：后端 Go 为 *bool（nil=不传递，默认 true），TS 侧统一为 boolean | null，nil 对应 null
    cache_prompt: boolean | null          // Prompt 缓存控制
    metrics: boolean               // 服务器指标端点开关
    verbose: boolean               // 详细日志开关
    spec_draft_threads: number     // Draft 模型线程数
    spec_draft_threads_batch: number // Draft 模型批处理线程数
    spec_default: boolean          // 使用默认推测解码配置
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
    agent: boolean
    ui_mcp_proxy: boolean
    lora_paths: string
    gpu_layers: number
    flash_attn: boolean | null
    mlock: boolean | null
    threads: number
    threads_http: number
    batch_size: number
    close_action: string
    // 推理配置（对应后端 Reasoning / ReasoningBudget / ReasoningBudgetMessage / ReasoningFormat / ReasoningPreserve）
    reasoning: 'on' | 'off' | 'auto'
    reasoning_budget: number
    reasoning_budget_message: string
    // 思考预算区间起始/结束标记（v9744+，为空则不传递，使用服务器默认值）
    reasoning_budget_start_tag: string
    reasoning_budget_end_tag: string
    reasoning_format: string
    // 注意：后端 Go 为 *bool（nil=不传递），TS 侧统一为 boolean | null，nil 对应 null
    reasoning_preserve: boolean | null
    // RAG 重排序配置
    reranker_model_path: string
    rerank_top_n: number
    // KV 缓存持久化配置
    slot_save_path: string
    slot_save_enabled: boolean
    // Draft 模型 GPU 配置
    spec_draft_ngl: number
    spec_draft_device: string
    // 后端采样（实验性）
    backend_sampling: boolean
    // 请求级采样配置
    samplers: string                 // 自定义采样器顺序（逗号分隔）
    ignore_eos: boolean              // 忽略 EOS 继续生成
    adaptive_target: number          // 请求级自适应采样目标
    adaptive_decay: number           // 请求级自适应采样衰减
    // ===== 以下字段对齐 Go Config（任务 29 补齐） =====
    // Draft 模型推测解码参数
    spec_draft_p_split: number          // 推测解码 split 概率（0=默认 0.10）
    spec_draft_p_min: number            // 最小推测解码概率（0=默认 0.00）
    spec_draft_backend_sampling: boolean | null // draft 后端采样（null=默认，对应 Go *bool）
    // 多模态批处理
    mtmd_batch_max_tokens: number       // 图像编码 batch 最大 token 数（0=默认 1024）
    // 模型标签（逗号分隔，用于 /v1/models 返回的 tags 字段）
    tags: string
    // 媒体路径（多模态模型额外媒体文件目录）
    media_path: string
    // 离线模式（禁用所有网络请求，如模型下载等）
    offline: boolean
    // 模型重打包（启动时重新打包模型权重，用于优化加载速度）
    repack: boolean
    // SSE ping 间隔秒数（0=使用服务器默认 30 秒，用于保持长连接活跃）
    sse_ping_interval: number
    // 直接 I/O（绕过操作系统页面缓存，加速大模型加载，避免内存污染）
    direct_io: boolean
    // MoE 权重 CPU 卸载（将所有专家权重保留在 CPU，显存不足时启用）
    cpu_moe: boolean
    // 前 N 层 MoE 权重 CPU 卸载（0=不启用，精细控制 cpu_moe 的影响范围）
    n_cpu_moe: number
    // 算子卸载开关（null=使用默认值 true，对应 Go *bool）
    op_offload: boolean | null
}

export interface SmartParamsInfo {
    hardware: {
        cpu_cores: number
        has_gpu: boolean
        has_cuda_backend: boolean
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
        ftype: string
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
    temperature: 0.8, // 与 Go DefaultConfig 对齐（llama.cpp 默认值）
    top_p: 0.95,
    top_k: 40, // 与 Go DefaultConfig 对齐（llama.cpp 默认值）
    repeat_penalty: 1,
    kv_unified: false,
    cache_idle_slots: true,
    cache_reuse: 256,
    cache_ram: 0, // 与 Go DefaultConfig 对齐
    image_min_tokens: 0,
    image_max_tokens: 0,
    fit_target: 0,
    fit_ctx: 0,
    system_prompt: '',
    system_prompt_mode: 'append', // 默认使用追加模式（与 Go DefaultConfig 对齐）
    chat_background: '',
    chat_background_opacity: 0.9,
    user_avatar: '',
    ai_avatar: '',
    search_mode: 'off',
    thinking_enabled: true,
    thinking_soft_switch: 'auto',
    sleep_idle_seconds: -1, // 与 Go DefaultConfig 对齐（-1 禁用空闲休眠）
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
    context_shift: true,
    min_p: 0.05,
    dry_multiplier: 0,
    dry_base: 1.75,
    dry_allowed_length: 2,
    dry_sequence_breaker: '',
    dry_penalty_last_n: 0,
    grp_attn_n: 0,
    grp_attn_w: 0,
    jinja: null, // 与 Go DefaultConfig 对齐（nil=不传递）
    cache_prompt: true, // 与 Go DefaultConfig 对齐（显式启用 prompt 缓存）
    metrics: false,
    verbose: false,
    spec_draft_threads: 0,
    spec_draft_threads_batch: 0,
    spec_default: false,
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
    ctx_checkpoints: 32,
    checkpoint_min_step: 256,
    tools: '',
    prefill_assistant: true,
    slot_prompt_similarity: 0.1,
    skip_chat_parsing: false,
    api_prefix: '',
    simple_io: false,
    agent: false,
    ui_mcp_proxy: false,
    lora_paths: '',
    gpu_layers: 0,
    flash_attn: null,
    mlock: null,
    threads: 0,
    threads_http: 0,
    batch_size: 0,
    close_action: 'ask',
    reasoning: 'off',
    reasoning_budget: 0,
    reasoning_budget_message: '',
    reasoning_budget_start_tag: '', // 思考预算区间起始标记（空=不传递）
    reasoning_budget_end_tag: '',   // 思考预算区间结束标记（空=不传递）
    reasoning_format: '',
    reasoning_preserve: null, // 与 Go DefaultConfig 对齐（nil=不传递）
    reranker_model_path: '',
    rerank_top_n: 5,
    slot_save_path: '',
    slot_save_enabled: false,
    spec_draft_ngl: 0,
    spec_draft_device: '',
    backend_sampling: false,
    // 请求级采样配置
    samplers: '',                     // 空字符串=不自定义
    ignore_eos: false,
    adaptive_target: 0,               // 0=禁用
    adaptive_decay: 0,                // 0=禁用
    // ===== 以下默认值对齐 Go DefaultConfig（任务 29 补齐） =====
    spec_draft_p_split: 0,            // 0=默认 0.10
    spec_draft_p_min: 0,              // 0=默认 0.00
    spec_draft_backend_sampling: null, // null=默认（对应 Go *bool nil）
    mtmd_batch_max_tokens: 0,         // 0=默认 1024
    tags: '',                         // 空字符串=无标签
    media_path: '',                   // 空字符串=无额外媒体目录
    offline: false,                   // 默认不禁用网络
    repack: false,                    // 默认不重打包
    sse_ping_interval: 0,             // 0=使用服务器默认 30 秒
    direct_io: false,                 // 默认不绕过页面缓存
    cpu_moe: false,                   // 默认不卸载 MoE 权重到 CPU
    n_cpu_moe: 0,                     // 0=不启用
    op_offload: null,                 // null=使用默认值 true（对应 Go *bool nil）
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
    streamingChunks: string[]  // 流式内容分块累积（M-前5：避免 += 的 O(N²) 字符串拼接）
    thinkingContent: string
    thinkingChunks: string[]   // 思考内容分块累积（与 streamingChunks 同理，避免 += 的 O(N²) 拼接）
    searchResults: string
    isSearching: boolean
    isThinking: boolean
    thinkingStartTime: number
    thinkingDuration: number
    searchQuery: string
    contextTrimmed: { reason: string; promptTokens?: number; contextSize?: number; messagesAfter?: number } | null
    tokensPerSecond: number  // 实时生成速度（tokens/s），0 表示未获取
    predictedN: number       // 已生成的 token 数
    promptProgress: { total: number; cache: number; processed: number; timeMs: number } | null
}
