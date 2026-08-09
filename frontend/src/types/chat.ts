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
  tokens_per_second?: number // 生成速度（tokens/s），仅流式完成时携带
  predicted_n?: number // 生成的 token 数
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

/** 搜索失败（content 为用户友好的错误提示字符串） */
export interface SearchErrorEvent extends StreamEventBase {
  type: 'search_error'
  content: string
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
  content: {
    reason: string
    prompt_tokens?: number
    context_size?: number
    messages_after?: number
  }
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
  | SearchErrorEvent
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
 * 字段同步保障：运行 `go run ./cmd/checkconfig` 可自动检测 Go Config / TS Config /
 * DEFAULT_CONFIG 三处字段是否一致，CI 也会在每次提交时自动运行此检查。
 * 新增字段时务必同步三处，否则 CI 会报错。
 */
export interface Config {
  // 配置 schema 版本号（与 Go Config.Version 对齐）
  version: number
  model_path: string
  mmproj_auto: boolean
  // mmproj GPU 卸载：null=自动（按硬件判断），true=强制启用，false=强制关闭
  mmproj_offload: boolean | null
  llama_server_path: string
  // 计算后端类型：auto(自动检测)/cuda/hip/sycl/vulkan/openvino/cpu
  // 生活类比：像选发动机型号——auto 是"让系统帮你选"，其他是明确指定用哪种发动机
  backend_type: string
  // 上一次成功启动的后端类型（非空表示切换过但新后端未通过启动验证，启动成功后清空）
  last_successful_backend: string
  // 性能模式：compatible(兼容)/balanced(平衡)/performance(性能)
  // 生活类比：像汽车的 ECO/COMFORT/SPORT 驾驶模式，按模式批量调整参数组合
  performance_mode: string
  // ===== TTS 文本转语音配置 =====
  // 生活类比：播音员调度台的设置——挑人、调快慢、调音调、调音量
  tts_enabled: boolean // 是否启用朗读按钮
  tts_voice: string // 发音人名称（空=自动按优先级挑选）
  tts_rate: number // 语速 0.5-2.0，1.0 = 正常
  tts_pitch: number // 音调 0-2，1.0 = 正常
  tts_volume: number // 音量 0-1，1.0 = 最大
  api_base: string
  port: number
  context_size: number
  // 主动压缩阈值：当估算 token 占比 >= 该阈值时，提前触发上下文压缩（不等溢出）
  // 默认 0.8（80%），范围 0.5-0.95。值越小越激进（更早压缩）
  proactive_compress_threshold: number
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
  // 编程助手模式："auto"（检测到 coder 模型自动启用）/ "on"（始终启用）/ "off"（始终禁用），默认 "auto"
  programming_mode: 'auto' | 'on' | 'off'
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
  dry_sequence_breaker: string // Dry 采样序列中断符（逗号分隔）
  dry_penalty_last_n: number // Dry 惩罚窗口大小
  grp_attn_n: number // 分组注意力组数
  grp_attn_w: number // 分组注意力窗口宽度
  // 注意：后端 Go 为 *bool（nil=不传递），TS 侧统一为 boolean | null，nil 对应 null
  jinja: boolean | null // Jinja2 模板引擎开关
  // 注意：后端 Go 为 *bool（nil=不传递，默认 true），TS 侧统一为 boolean | null，nil 对应 null
  cache_prompt: boolean | null // Prompt 缓存控制
  metrics: boolean // 服务器指标端点开关
  verbose: boolean // 详细日志开关
  spec_draft_threads: number // Draft 模型线程数
  spec_draft_threads_batch: number // Draft 模型批处理线程数
  spec_default: boolean // 使用默认推测解码配置
  device: string
  parallel: number
  // 多 GPU 原生参数（llama.cpp --split-mode / --tensor-split / --main-gpu）
  split_mode: string // 分割模式：layer/row/tensor/none，空=使用 llama.cpp 默认
  tensor_split: string // 张量分割权重（逗号分隔，如 "3,1"），空=不传递
  main_gpu: number // 主 GPU 索引，-1=不传递使用默认
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
  chat_template_file: string // 自定义聊天模板文件路径（.jinja），空=使用模型自带模板
  server_api_key_enabled: boolean
  expose_server: boolean
  enable_web_ui: boolean
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
  cors_origins: string
  cors_methods: string
  cors_headers: string
  cors_credentials: boolean
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
  // 思考强度（模板级 reasoning_effort，空=不传递跟随模型默认；模板支持时生效，如 DeepSeek-V4）
  reasoning_effort: string
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
  samplers: string // 自定义采样器顺序（逗号分隔）
  ignore_eos: boolean // 忽略 EOS 继续生成
  adaptive_target: number // 请求级自适应采样目标
  adaptive_decay: number // 请求级自适应采样衰减
  // ===== 以下字段对齐 Go Config（任务 29 补齐） =====
  // Draft 模型推测解码参数
  spec_draft_p_split: number // 推测解码 split 概率（0=默认 0.10）
  spec_draft_p_min: number // 最小推测解码概率（0=默认 0.00）
  spec_draft_backend_sampling: boolean | null // draft 后端采样（null=默认，对应 Go *bool）
  // 多模态批处理
  mtmd_batch_max_tokens: number // 图像编码 batch 最大 token 数（0=默认 1024）
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
  // MCP 服务器列表（豆芽原生 MCP 客户端，通过 stdio 连接外部 MCP server）
  mcp_servers: MCPServerConfig[]
}

// MCP 服务器配置
export interface MCPServerConfig {
  name: string
  command: string
  args: string[]
  env: Record<string, string>
  enabled: boolean
}

// MCP 工具信息
export interface MCPToolInfo {
  name: string
  description: string
  input_schema: Record<string, any>
}

// MCP 服务器运行状态
export interface MCPServerStatus {
  connected: boolean
  error?: string
  tool_count: number
}

// MCP 连接测试结果
export interface MCPConnectResult {
  name: string
  success: boolean
  error?: string
  tool_count: number
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
  version: 2, // 与 Go Config.Version 对齐（配置 schema 版本号，P4.3 统一为 2）
  model_path: '',
  mmproj_auto: true,
  mmproj_offload: null,
  llama_server_path: 'runtime/llama-server.exe',
  backend_type: 'auto', // 与 Go DefaultConfig 对齐（自动检测最合适的后端）
  last_successful_backend: '', // 与 Go DefaultConfig 对齐（无历史回退后端）
  performance_mode: 'balanced', // 与 Go DefaultConfig 对齐（平衡模式，兼顾性能与稳定性）
  // TTS 文本转语音默认配置（与 Go DefaultConfig 对齐）
  tts_enabled: true, // 与 Go DefaultConfig 对齐（运行时默认开启朗读按钮）
  tts_voice: '', // 空字符串 = 自动按优先级挑选（晓晓→云希→...）
  tts_rate: 1.0, // 正常语速
  tts_pitch: 1.0, // 正常音调
  tts_volume: 1.0, // 最大音量
  api_base: 'http://127.0.0.1:8080',
  port: 8080,
  // P4.1: 默认 0 = 未设置，让 smart-params 按 GPU 显存预算计算（与 Go DefaultConfig 对齐）
  context_size: 0,
  proactive_compress_threshold: 0.8, // P1-A1: 80% 时主动压缩，为后续对话留出 20% 空间
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
  programming_mode: 'auto', // 默认自动检测（coder 模型启用编程版提示词，与 Go DefaultConfig 对齐）
  chat_background: '',
  chat_background_opacity: 0.9, // 与 Go DefaultConfig 对齐（默认背景不透明度 0.9）
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
  kv_offload: false,
  context_shift: false,
  min_p: 0.05,
  dry_multiplier: 0,
  dry_base: 1.75,
  dry_allowed_length: 2,
  dry_sequence_breaker: '',
  dry_penalty_last_n: 0,
  grp_attn_n: 0,
  grp_attn_w: 0,
  jinja: true, // 与 Go DefaultConfig 对齐（默认开启 Jinja2 模板引擎）
  cache_prompt: true, // 与 Go DefaultConfig 对齐（显式启用 prompt 缓存）
  metrics: false,
  verbose: false,
  spec_draft_threads: 0,
  spec_draft_threads_batch: 0,
  spec_default: false,
  device: '',
  parallel: 0,
  split_mode: '', // 空=使用 llama.cpp 默认 layer 模式
  tensor_split: '', // 空=不传递
  main_gpu: -1, // -1=不传递
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
  chat_template_file: '', // 空=使用模型自带模板
  server_api_key_enabled: false,
  expose_server: false,
  enable_web_ui: false,
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
  cors_origins: '',
  cors_methods: '',
  cors_headers: '',
  cors_credentials: false,
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
  reasoning_budget_end_tag: '', // 思考预算区间结束标记（空=不传递）
  reasoning_format: '',
  reasoning_effort: '', // 思考强度（空=跟随模型默认；模板支持时生效）
  reasoning_preserve: null, // 与 Go DefaultConfig 对齐（nil=不传递）
  reranker_model_path: '',
  rerank_top_n: 5,
  slot_save_path: '',
  slot_save_enabled: false,
  spec_draft_ngl: 0,
  spec_draft_device: '',
  backend_sampling: false,
  // 请求级采样配置
  samplers: '', // 空字符串=不自定义
  ignore_eos: false,
  adaptive_target: 0, // 0=禁用
  adaptive_decay: 0, // 0=禁用
  // ===== 以下默认值对齐 Go DefaultConfig（任务 29 补齐） =====
  spec_draft_p_split: 0, // 0=默认 0.10
  spec_draft_p_min: 0, // 0=默认 0.00
  spec_draft_backend_sampling: null, // null=默认（对应 Go *bool nil）
  mtmd_batch_max_tokens: 0, // 0=默认 1024
  tags: '', // 空字符串=无标签
  media_path: '', // 空字符串=无额外媒体目录
  offline: false, // 默认不禁用网络
  repack: false, // 默认不重打包
  sse_ping_interval: 0, // 0=使用服务器默认 30 秒
  direct_io: false, // 默认不绕过页面缓存
  cpu_moe: false, // 默认不卸载 MoE 权重到 CPU
  n_cpu_moe: 0, // 0=不启用
  op_offload: null, // null=使用默认值 true（对应 Go *bool nil）
  mcp_servers: [] // 默认不连接任何 MCP server
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

/** llama-server /metrics 端点的关键指标摘要 */
export interface MetricsSummary {
  tokens_prompt_total: number
  prompt_seconds_total: number
  tokens_predicted_total: number
  predicted_seconds_total: number
  n_decode_total: number
  n_tokens_max: number
  prompt_tokens_per_second: number
  predict_tokens_per_second: number
  processing_requests: number
  deferred_requests: number
  busy_slots_per_decode: number
  // 推测解码指标（llama.cpp b10287 / PR #26389 引入）
  // 推测解码未启用时这些计数器恒为 0；启用后用于评估命中率（accepted/draft）
  spec_draft_tokens_total: number // 草稿模型生成的 token 总数
  spec_accepted_tokens_total: number // 被目标模型接受的草稿 token 总数
  spec_drafts_total: number // 推测解码验证步骤总数
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
  /** 切换时是否恢复了该模型的专属生成参数预设 */
  params_restored?: boolean
}

/** 流式状态：单个会话的临时状态 */
export interface ConvStreamingState {
  isGenerating: boolean
  streamingContent: string
  streamingChunks: string[] // 流式内容分块累积（M-前5：避免 += 的 O(N²) 字符串拼接）
  thinkingContent: string
  thinkingChunks: string[] // 思考内容分块累积（与 streamingChunks 同理，避免 += 的 O(N²) 拼接）
  searchResults: string
  isSearching: boolean
  isThinking: boolean
  thinkingStartTime: number
  thinkingDuration: number
  searchQuery: string
  searchError: string // 搜索失败的友好错误提示（空字符串表示无错误）
  contextTrimmed: {
    reason: string
    promptTokens?: number
    contextSize?: number
    messagesAfter?: number
  } | null
  tokensPerSecond: number // 实时生成速度（tokens/s），0 表示未获取
  predictedN: number // 已生成的 token 数
  promptProgress: { total: number; cache: number; processed: number; timeMs: number } | null
}

/**
 * 显卡后端状态信息（对齐 Go 后端 main.BackendStatus）
 *
 * 生活类比：就像车辆仪表盘上的"发动机状态"显示区——
 * 当前用什么发动机、用户选了什么模式、车上装了什么发动机、手头有哪些可用。
 */
export interface BackendStatus {
  /** 当前后端类型："cuda"/"vulkan"/"cpu" 等（已解析，不含 auto），为空表示尚未启动 */
  current_backend: string
  /** 配置中的值："auto" 或具体后端（cuda/hip/sycl/vulkan/openvino/cpu） */
  config_backend: string
  /** 检测到的 GPU 厂商："nvidia"/"amd"/"intel"/"vulkan"/""（空表示未检测到） */
  gpu_vendor: string
  /** GPU 名称（如 "NVIDIA GeForce RTX 4090"），无 GPU 时为空 */
  gpu_name: string
  /** GPU 显存（MB），无 GPU 或检测失败时为 0 */
  gpu_vram_mb: number
  /** 已安装的后端列表（runtime 目录中已有 llama-server.exe 的后端，不含 auto） */
  installed_backends: string[]
  /** 所有可选后端列表（含 auto），供下拉框展示 */
  available_backends: string[]
}

/** 显卡后端切换事件（backend:switched 事件载荷） */
export interface BackendSwitchedEvent {
  /** 切换后的后端状态（与 BackendStatus 结构相同） */
  status: BackendStatus
}
