export namespace chat {
	
	export class AbnormalConversation {
	    id: string;
	    title: string;
	    reason: string;
	
	    static createFrom(source: any = {}) {
	        return new AbnormalConversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.reason = source["reason"];
	    }
	}
	export class Attachment {
	    type: string;
	    name: string;
	    mime_type: string;
	    data: string;
	    format?: string;
	
	    static createFrom(source: any = {}) {
	        return new Attachment(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.mime_type = source["mime_type"];
	        this.data = source["data"];
	        this.format = source["format"];
	    }
	}
	export class AttachmentSummary {
	    type: string;
	    name: string;
	    mime_type: string;
	
	    static createFrom(source: any = {}) {
	        return new AttachmentSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.type = source["type"];
	        this.name = source["name"];
	        this.mime_type = source["mime_type"];
	    }
	}
	export class CompressResult {
	    shortSummary: string;
	    longSummary: string;
	    trimmedCount: number;
	    message: string;
	
	    static createFrom(source: any = {}) {
	        return new CompressResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.shortSummary = source["shortSummary"];
	        this.longSummary = source["longSummary"];
	        this.trimmedCount = source["trimmedCount"];
	        this.message = source["message"];
	    }
	}
	export class Conversation {
	    id: string;
	    title: string;
	    created_at: string;
	    updated_at: string;
	
	    static createFrom(source: any = {}) {
	        return new Conversation(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.title = source["title"];
	        this.created_at = source["created_at"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class Message {
	    id: string;
	    conversation_id: string;
	    role: string;
	    content: string;
	    thinking_content?: string;
	    thinking_duration?: number;
	    search_results: string;
	    images?: string;
	    attachments?: AttachmentSummary[];
	    created_at: string;
	    tokens_per_second?: number;
	    predicted_n?: number;
	
	    static createFrom(source: any = {}) {
	        return new Message(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.conversation_id = source["conversation_id"];
	        this.role = source["role"];
	        this.content = source["content"];
	        this.thinking_content = source["thinking_content"];
	        this.thinking_duration = source["thinking_duration"];
	        this.search_results = source["search_results"];
	        this.images = source["images"];
	        this.attachments = this.convertValues(source["attachments"], AttachmentSummary);
	        this.created_at = source["created_at"];
	        this.tokens_per_second = source["tokens_per_second"];
	        this.predicted_n = source["predicted_n"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ModelParams {
	    temperature: number;
	    top_p: number;
	    top_k: number;
	    repeat_penalty: number;
	    min_p: number;
	    samplers: string;
	    ignore_eos: boolean;
	    adaptive_target: number;
	    adaptive_decay: number;
	    context_size: number;
	    proactive_compress_threshold: number;
	    reasoning: string;
	    reasoning_effort: string;
	    reasoning_budget: number;
	    reasoning_format: string;
	    reasoning_preserve?: boolean;
	    dry_multiplier: number;
	    dry_base: number;
	    dry_allowed_length: number;
	    dry_sequence_breaker: string;
	    dry_penalty_last_n: number;
	    grp_attn_n: number;
	    grp_attn_w: number;
	    image_min_tokens: number;
	    image_max_tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.temperature = source["temperature"];
	        this.top_p = source["top_p"];
	        this.top_k = source["top_k"];
	        this.repeat_penalty = source["repeat_penalty"];
	        this.min_p = source["min_p"];
	        this.samplers = source["samplers"];
	        this.ignore_eos = source["ignore_eos"];
	        this.adaptive_target = source["adaptive_target"];
	        this.adaptive_decay = source["adaptive_decay"];
	        this.context_size = source["context_size"];
	        this.proactive_compress_threshold = source["proactive_compress_threshold"];
	        this.reasoning = source["reasoning"];
	        this.reasoning_effort = source["reasoning_effort"];
	        this.reasoning_budget = source["reasoning_budget"];
	        this.reasoning_format = source["reasoning_format"];
	        this.reasoning_preserve = source["reasoning_preserve"];
	        this.dry_multiplier = source["dry_multiplier"];
	        this.dry_base = source["dry_base"];
	        this.dry_allowed_length = source["dry_allowed_length"];
	        this.dry_sequence_breaker = source["dry_sequence_breaker"];
	        this.dry_penalty_last_n = source["dry_penalty_last_n"];
	        this.grp_attn_n = source["grp_attn_n"];
	        this.grp_attn_w = source["grp_attn_w"];
	        this.image_min_tokens = source["image_min_tokens"];
	        this.image_max_tokens = source["image_max_tokens"];
	    }
	}
	export class SendMessageParams {
	    conversation_id: string;
	    content: string;
	    search_mode: string;
	    images?: string[];
	    attachments?: Attachment[];
	
	    static createFrom(source: any = {}) {
	        return new SendMessageParams(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.conversation_id = source["conversation_id"];
	        this.content = source["content"];
	        this.search_mode = source["search_mode"];
	        this.images = source["images"];
	        this.attachments = this.convertValues(source["attachments"], Attachment);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace config {
	
	export class MCPServerConfig {
	    name: string;
	    command: string;
	    args: string[];
	    env: Record<string, string>;
	    enabled: boolean;
	
	    static createFrom(source: any = {}) {
	        return new MCPServerConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.command = source["command"];
	        this.args = source["args"];
	        this.env = source["env"];
	        this.enabled = source["enabled"];
	    }
	}
	export class Config {
	    version: number;
	    model_path: string;
	    mmproj_auto: boolean;
	    mmproj_offload?: boolean;
	    mmproj_device: string;
	    llama_server_path: string;
	    backend_type: string;
	    last_successful_backend: string;
	    api_base: string;
	    port: number;
	    context_size: number;
	    proactive_compress_threshold: number;
	    temperature: number;
	    top_p: number;
	    top_k: number;
	    repeat_penalty: number;
	    kv_unified: boolean;
	    cache_idle_slots: boolean;
	    cache_ram: number;
	    image_min_tokens: number;
	    image_max_tokens: number;
	    fit_target: number;
	    fit_ctx: number;
	    reasoning: string;
	    reasoning_budget: number;
	    reasoning_format: string;
	    reasoning_effort: string;
	    reasoning_preserve?: boolean;
	    system_prompt: string;
	    system_prompt_mode: string;
	    programming_mode: string;
	    chat_background: string;
	    chat_background_opacity: number;
	    user_avatar: string;
	    ai_avatar: string;
	    search_mode: string;
	    thinking_enabled: boolean;
	    thinking_soft_switch: string;
	    sleep_idle_seconds: number;
	    models_max: number;
	    rag_enabled: boolean;
	    rag_active_kb: string;
	    rag_top_k: number;
	    rag_min_score: number;
	    rag_chunk_size: number;
	    rag_chunk_overlap: number;
	    embedding_model: string;
	    reasoning_budget_message: string;
	    reasoning_budget_start_tag: string;
	    reasoning_budget_end_tag: string;
	    mmap: boolean;
	    kv_offload: boolean;
	    context_shift: boolean;
	    min_p: number;
	    dry_multiplier: number;
	    dry_base: number;
	    dry_allowed_length: number;
	    dry_sequence_breaker: string;
	    dry_penalty_last_n: number;
	    grp_attn_n: number;
	    grp_attn_w: number;
	    jinja?: boolean;
	    cache_prompt?: boolean;
	    metrics: boolean;
	    verbose: boolean;
	    spec_draft_threads: number;
	    spec_draft_threads_batch: number;
	    spec_default: boolean;
	    device: string;
	    split_mode: string;
	    tensor_split: string;
	    main_gpu: number;
	    parallel: number;
	    cache_type_k: string;
	    cache_type_v: string;
	    spec_type: string;
	    spec_draft_n_max: number;
	    spec_draft_n_min: number;
	    cache_type_k_draft: string;
	    cache_type_v_draft: string;
	    spec_ngram_mod_n_min: number;
	    spec_ngram_mod_n_max: number;
	    spec_ngram_mod_n_match: number;
	    spec_ngram_simple_size_n: number;
	    spec_ngram_simple_size_m: number;
	    spec_ngram_simple_min_hits: number;
	    spec_ngram_map_k_size_n: number;
	    spec_ngram_map_k_size_m: number;
	    spec_ngram_map_k_min_hits: number;
	    spec_ngram_map_k4v_size_n: number;
	    spec_ngram_map_k4v_size_m: number;
	    spec_ngram_map_k4v_min_hits: number;
	    lookup_cache_static: string;
	    lookup_cache_dynamic: string;
	    spec_draft_model: string;
	    server_api_key_enabled: boolean;
	    expose_server: boolean;
	    enable_web_ui: boolean;
	    swa_full: boolean;
	    ctx_checkpoints: number;
	    checkpoint_min_step: number;
	    tools: string;
	    enable_builtin_tools: boolean;
	    prefill_assistant: boolean;
	    slot_prompt_similarity: number;
	    skip_chat_parsing: boolean;
	    api_prefix: string;
	    simple_io: boolean;
	    gpu_layers: number;
	    flash_attn?: boolean;
	    mlock?: boolean;
	    threads: number;
	    threads_http: number;
	    batch_size: number;
	    close_action: string;
	    reranker_model_path: string;
	    rerank_top_n: number;
	    slot_save_path: string;
	    slot_save_enabled: boolean;
	    cache_reuse: number;
	    spec_draft_ngl: number;
	    spec_draft_device: string;
	    spec_draft_p_split: number;
	    spec_draft_p_min: number;
	    spec_draft_backend_sampling?: boolean;
	    mtmd_batch_max_tokens: number;
	    adaptive_target: number;
	    adaptive_decay: number;
	    samplers: string;
	    ignore_eos: boolean;
	    tags: string;
	    media_path: string;
	    offline: boolean;
	    repack: boolean;
	    agent: boolean;
	    ui_mcp_proxy: boolean;
	    cors_origins: string;
	    cors_methods: string;
	    cors_headers: string;
	    cors_credentials: boolean;
	    backend_sampling: boolean;
	    tts_enabled: boolean;
	    tts_voice: string;
	    tts_rate: number;
	    tts_pitch: number;
	    tts_volume: number;
	    tts_online: boolean;
	    sse_ping_interval: number;
	    lora_paths: string;
	    chat_template_file: string;
	    direct_io: boolean;
	    cpu_moe: boolean;
	    n_cpu_moe: number;
	    op_offload?: boolean;
	    mcp_servers: MCPServerConfig[];
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.version = source["version"];
	        this.model_path = source["model_path"];
	        this.mmproj_auto = source["mmproj_auto"];
	        this.mmproj_offload = source["mmproj_offload"];
	        this.mmproj_device = source["mmproj_device"];
	        this.llama_server_path = source["llama_server_path"];
	        this.backend_type = source["backend_type"];
	        this.last_successful_backend = source["last_successful_backend"];
	        this.api_base = source["api_base"];
	        this.port = source["port"];
	        this.context_size = source["context_size"];
	        this.proactive_compress_threshold = source["proactive_compress_threshold"];
	        this.temperature = source["temperature"];
	        this.top_p = source["top_p"];
	        this.top_k = source["top_k"];
	        this.repeat_penalty = source["repeat_penalty"];
	        this.kv_unified = source["kv_unified"];
	        this.cache_idle_slots = source["cache_idle_slots"];
	        this.cache_ram = source["cache_ram"];
	        this.image_min_tokens = source["image_min_tokens"];
	        this.image_max_tokens = source["image_max_tokens"];
	        this.fit_target = source["fit_target"];
	        this.fit_ctx = source["fit_ctx"];
	        this.reasoning = source["reasoning"];
	        this.reasoning_budget = source["reasoning_budget"];
	        this.reasoning_format = source["reasoning_format"];
	        this.reasoning_effort = source["reasoning_effort"];
	        this.reasoning_preserve = source["reasoning_preserve"];
	        this.system_prompt = source["system_prompt"];
	        this.system_prompt_mode = source["system_prompt_mode"];
	        this.programming_mode = source["programming_mode"];
	        this.chat_background = source["chat_background"];
	        this.chat_background_opacity = source["chat_background_opacity"];
	        this.user_avatar = source["user_avatar"];
	        this.ai_avatar = source["ai_avatar"];
	        this.search_mode = source["search_mode"];
	        this.thinking_enabled = source["thinking_enabled"];
	        this.thinking_soft_switch = source["thinking_soft_switch"];
	        this.sleep_idle_seconds = source["sleep_idle_seconds"];
	        this.models_max = source["models_max"];
	        this.rag_enabled = source["rag_enabled"];
	        this.rag_active_kb = source["rag_active_kb"];
	        this.rag_top_k = source["rag_top_k"];
	        this.rag_min_score = source["rag_min_score"];
	        this.rag_chunk_size = source["rag_chunk_size"];
	        this.rag_chunk_overlap = source["rag_chunk_overlap"];
	        this.embedding_model = source["embedding_model"];
	        this.reasoning_budget_message = source["reasoning_budget_message"];
	        this.reasoning_budget_start_tag = source["reasoning_budget_start_tag"];
	        this.reasoning_budget_end_tag = source["reasoning_budget_end_tag"];
	        this.mmap = source["mmap"];
	        this.kv_offload = source["kv_offload"];
	        this.context_shift = source["context_shift"];
	        this.min_p = source["min_p"];
	        this.dry_multiplier = source["dry_multiplier"];
	        this.dry_base = source["dry_base"];
	        this.dry_allowed_length = source["dry_allowed_length"];
	        this.dry_sequence_breaker = source["dry_sequence_breaker"];
	        this.dry_penalty_last_n = source["dry_penalty_last_n"];
	        this.grp_attn_n = source["grp_attn_n"];
	        this.grp_attn_w = source["grp_attn_w"];
	        this.jinja = source["jinja"];
	        this.cache_prompt = source["cache_prompt"];
	        this.metrics = source["metrics"];
	        this.verbose = source["verbose"];
	        this.spec_draft_threads = source["spec_draft_threads"];
	        this.spec_draft_threads_batch = source["spec_draft_threads_batch"];
	        this.spec_default = source["spec_default"];
	        this.device = source["device"];
	        this.split_mode = source["split_mode"];
	        this.tensor_split = source["tensor_split"];
	        this.main_gpu = source["main_gpu"];
	        this.parallel = source["parallel"];
	        this.cache_type_k = source["cache_type_k"];
	        this.cache_type_v = source["cache_type_v"];
	        this.spec_type = source["spec_type"];
	        this.spec_draft_n_max = source["spec_draft_n_max"];
	        this.spec_draft_n_min = source["spec_draft_n_min"];
	        this.cache_type_k_draft = source["cache_type_k_draft"];
	        this.cache_type_v_draft = source["cache_type_v_draft"];
	        this.spec_ngram_mod_n_min = source["spec_ngram_mod_n_min"];
	        this.spec_ngram_mod_n_max = source["spec_ngram_mod_n_max"];
	        this.spec_ngram_mod_n_match = source["spec_ngram_mod_n_match"];
	        this.spec_ngram_simple_size_n = source["spec_ngram_simple_size_n"];
	        this.spec_ngram_simple_size_m = source["spec_ngram_simple_size_m"];
	        this.spec_ngram_simple_min_hits = source["spec_ngram_simple_min_hits"];
	        this.spec_ngram_map_k_size_n = source["spec_ngram_map_k_size_n"];
	        this.spec_ngram_map_k_size_m = source["spec_ngram_map_k_size_m"];
	        this.spec_ngram_map_k_min_hits = source["spec_ngram_map_k_min_hits"];
	        this.spec_ngram_map_k4v_size_n = source["spec_ngram_map_k4v_size_n"];
	        this.spec_ngram_map_k4v_size_m = source["spec_ngram_map_k4v_size_m"];
	        this.spec_ngram_map_k4v_min_hits = source["spec_ngram_map_k4v_min_hits"];
	        this.lookup_cache_static = source["lookup_cache_static"];
	        this.lookup_cache_dynamic = source["lookup_cache_dynamic"];
	        this.spec_draft_model = source["spec_draft_model"];
	        this.server_api_key_enabled = source["server_api_key_enabled"];
	        this.expose_server = source["expose_server"];
	        this.enable_web_ui = source["enable_web_ui"];
	        this.swa_full = source["swa_full"];
	        this.ctx_checkpoints = source["ctx_checkpoints"];
	        this.checkpoint_min_step = source["checkpoint_min_step"];
	        this.tools = source["tools"];
	        this.enable_builtin_tools = source["enable_builtin_tools"];
	        this.prefill_assistant = source["prefill_assistant"];
	        this.slot_prompt_similarity = source["slot_prompt_similarity"];
	        this.skip_chat_parsing = source["skip_chat_parsing"];
	        this.api_prefix = source["api_prefix"];
	        this.simple_io = source["simple_io"];
	        this.gpu_layers = source["gpu_layers"];
	        this.flash_attn = source["flash_attn"];
	        this.mlock = source["mlock"];
	        this.threads = source["threads"];
	        this.threads_http = source["threads_http"];
	        this.batch_size = source["batch_size"];
	        this.close_action = source["close_action"];
	        this.reranker_model_path = source["reranker_model_path"];
	        this.rerank_top_n = source["rerank_top_n"];
	        this.slot_save_path = source["slot_save_path"];
	        this.slot_save_enabled = source["slot_save_enabled"];
	        this.cache_reuse = source["cache_reuse"];
	        this.spec_draft_ngl = source["spec_draft_ngl"];
	        this.spec_draft_device = source["spec_draft_device"];
	        this.spec_draft_p_split = source["spec_draft_p_split"];
	        this.spec_draft_p_min = source["spec_draft_p_min"];
	        this.spec_draft_backend_sampling = source["spec_draft_backend_sampling"];
	        this.mtmd_batch_max_tokens = source["mtmd_batch_max_tokens"];
	        this.adaptive_target = source["adaptive_target"];
	        this.adaptive_decay = source["adaptive_decay"];
	        this.samplers = source["samplers"];
	        this.ignore_eos = source["ignore_eos"];
	        this.tags = source["tags"];
	        this.media_path = source["media_path"];
	        this.offline = source["offline"];
	        this.repack = source["repack"];
	        this.agent = source["agent"];
	        this.ui_mcp_proxy = source["ui_mcp_proxy"];
	        this.cors_origins = source["cors_origins"];
	        this.cors_methods = source["cors_methods"];
	        this.cors_headers = source["cors_headers"];
	        this.cors_credentials = source["cors_credentials"];
	        this.backend_sampling = source["backend_sampling"];
	        this.tts_enabled = source["tts_enabled"];
	        this.tts_voice = source["tts_voice"];
	        this.tts_rate = source["tts_rate"];
	        this.tts_pitch = source["tts_pitch"];
	        this.tts_volume = source["tts_volume"];
	        this.tts_online = source["tts_online"];
	        this.sse_ping_interval = source["sse_ping_interval"];
	        this.lora_paths = source["lora_paths"];
	        this.chat_template_file = source["chat_template_file"];
	        this.direct_io = source["direct_io"];
	        this.cpu_moe = source["cpu_moe"];
	        this.n_cpu_moe = source["n_cpu_moe"];
	        this.op_offload = source["op_offload"];
	        this.mcp_servers = this.convertValues(source["mcp_servers"], MCPServerConfig);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class MCPConnectResult {
	    name: string;
	    success: boolean;
	    error?: string;
	    tool_count: number;
	
	    static createFrom(source: any = {}) {
	        return new MCPConnectResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.success = source["success"];
	        this.error = source["error"];
	        this.tool_count = source["tool_count"];
	    }
	}
	
	export class MCPToolInfo {
	    name: string;
	    description: string;
	    input_schema: Record<string, any>;
	
	    static createFrom(source: any = {}) {
	        return new MCPToolInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.description = source["description"];
	        this.input_schema = source["input_schema"];
	    }
	}

}

export namespace llm {
	
	export class FunctionCall {
	    name: string;
	    arguments: string;
	
	    static createFrom(source: any = {}) {
	        return new FunctionCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.arguments = source["arguments"];
	    }
	}
	export class ToolCall {
	    index: number;
	    id: string;
	    type: string;
	    function: FunctionCall;
	
	    static createFrom(source: any = {}) {
	        return new ToolCall(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.index = source["index"];
	        this.id = source["id"];
	        this.type = source["type"];
	        this.function = this.convertValues(source["function"], FunctionCall);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ChatMessage {
	    role: string;
	    content: any;
	    reasoning_content?: string;
	    tool_calls?: ToolCall[];
	    tool_call_id?: string;
	
	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.reasoning_content = source["reasoning_content"];
	        this.tool_calls = this.convertValues(source["tool_calls"], ToolCall);
	        this.tool_call_id = source["tool_call_id"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class HubFile {
	    provider: string;
	    repo_id: string;
	    path: string;
	    size: number;
	    is_gguf: boolean;
	    is_mmproj: boolean;
	    url: string;
	
	    static createFrom(source: any = {}) {
	        return new HubFile(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.repo_id = source["repo_id"];
	        this.path = source["path"];
	        this.size = source["size"];
	        this.is_gguf = source["is_gguf"];
	        this.is_mmproj = source["is_mmproj"];
	        this.url = source["url"];
	    }
	}
	export class HubModel {
	    provider: string;
	    repo_id: string;
	    name: string;
	    downloads: number;
	    likes: number;
	
	    static createFrom(source: any = {}) {
	        return new HubModel(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider = source["provider"];
	        this.repo_id = source["repo_id"];
	        this.name = source["name"];
	        this.downloads = source["downloads"];
	        this.likes = source["likes"];
	    }
	}
	export class LoraAdapter {
	    id: number;
	    path: string;
	    scale: number;
	
	    static createFrom(source: any = {}) {
	        return new LoraAdapter(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.path = source["path"];
	        this.scale = source["scale"];
	    }
	}
	export class MetricsSummary {
	    tokens_prompt_total: number;
	    prompt_seconds_total: number;
	    tokens_predicted_total: number;
	    predicted_seconds_total: number;
	    n_decode_total: number;
	    n_tokens_max: number;
	    prompt_tokens_per_second: number;
	    predict_tokens_per_second: number;
	    processing_requests: number;
	    deferred_requests: number;
	    busy_slots_per_decode: number;
	    spec_draft_tokens_total: number;
	    spec_accepted_tokens_total: number;
	    spec_drafts_total: number;
	    spec_accepted_tokens_per_pos_total: number;
	
	    static createFrom(source: any = {}) {
	        return new MetricsSummary(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.tokens_prompt_total = source["tokens_prompt_total"];
	        this.prompt_seconds_total = source["prompt_seconds_total"];
	        this.tokens_predicted_total = source["tokens_predicted_total"];
	        this.predicted_seconds_total = source["predicted_seconds_total"];
	        this.n_decode_total = source["n_decode_total"];
	        this.n_tokens_max = source["n_tokens_max"];
	        this.prompt_tokens_per_second = source["prompt_tokens_per_second"];
	        this.predict_tokens_per_second = source["predict_tokens_per_second"];
	        this.processing_requests = source["processing_requests"];
	        this.deferred_requests = source["deferred_requests"];
	        this.busy_slots_per_decode = source["busy_slots_per_decode"];
	        this.spec_draft_tokens_total = source["spec_draft_tokens_total"];
	        this.spec_accepted_tokens_total = source["spec_accepted_tokens_total"];
	        this.spec_drafts_total = source["spec_drafts_total"];
	        this.spec_accepted_tokens_per_pos_total = source["spec_accepted_tokens_per_pos_total"];
	    }
	}
	export class ModelCapabilities {
	    image_input: boolean;
	    audio_input: boolean;
	    video_input: boolean;
	    text_input: boolean;
	    reasoning: boolean;
	    mmproj_loaded: boolean;
	    has_mtp: boolean;
	    thinking_mode: string;
	    soft_switch_support: boolean;
	    default_thinking_auto: string;
	    n_params: number;
	    tool_call_support: boolean;
	    supports_preserve_reasoning: boolean;
	    supports_parallel_tool_calls: boolean;
	    supports_system_role: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelCapabilities(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image_input = source["image_input"];
	        this.audio_input = source["audio_input"];
	        this.video_input = source["video_input"];
	        this.text_input = source["text_input"];
	        this.reasoning = source["reasoning"];
	        this.mmproj_loaded = source["mmproj_loaded"];
	        this.has_mtp = source["has_mtp"];
	        this.thinking_mode = source["thinking_mode"];
	        this.soft_switch_support = source["soft_switch_support"];
	        this.default_thinking_auto = source["default_thinking_auto"];
	        this.n_params = source["n_params"];
	        this.tool_call_support = source["tool_call_support"];
	        this.supports_preserve_reasoning = source["supports_preserve_reasoning"];
	        this.supports_parallel_tool_calls = source["supports_parallel_tool_calls"];
	        this.supports_system_role = source["supports_system_role"];
	    }
	}
	export class ModelDetails {
	    n_params: number;
	    size_label: string;
	    quant_type: string;
	    context_length: number;
	    expert_count: number;
	    file_size_bytes: number;
	    has_vision: boolean;
	    has_audio: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ModelDetails(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.n_params = source["n_params"];
	        this.size_label = source["size_label"];
	        this.quant_type = source["quant_type"];
	        this.context_length = source["context_length"];
	        this.expert_count = source["expert_count"];
	        this.file_size_bytes = source["file_size_bytes"];
	        this.has_vision = source["has_vision"];
	        this.has_audio = source["has_audio"];
	    }
	}
	export class ModelOption {
	    name: string;
	    model_path: string;
	    file_name: string;
	    is_default: boolean;
	    is_loaded: boolean;
	    mmproj_vision: boolean;
	    mmproj_audio: boolean;
	    mmproj_video: boolean;
	    status: string;
	    size_label: string;
	    quant_type: string;
	    file_size_bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new ModelOption(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.model_path = source["model_path"];
	        this.file_name = source["file_name"];
	        this.is_default = source["is_default"];
	        this.is_loaded = source["is_loaded"];
	        this.mmproj_vision = source["mmproj_vision"];
	        this.mmproj_audio = source["mmproj_audio"];
	        this.mmproj_video = source["mmproj_video"];
	        this.status = source["status"];
	        this.size_label = source["size_label"];
	        this.quant_type = source["quant_type"];
	        this.file_size_bytes = source["file_size_bytes"];
	    }
	}
	export class ServerStatus {
	    running: boolean;
	    model_ready?: boolean;
	    error?: string;
	    switching?: boolean;
	    switching_to?: string;
	    current_model?: string;
	    capabilities?: ModelCapabilities;
	
	    static createFrom(source: any = {}) {
	        return new ServerStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.model_ready = source["model_ready"];
	        this.error = source["error"];
	        this.switching = source["switching"];
	        this.switching_to = source["switching_to"];
	        this.current_model = source["current_model"];
	        this.capabilities = this.convertValues(source["capabilities"], ModelCapabilities);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class SlotInfo {
	    id: number;
	    task: string;
	    n_prompt: number;
	    n_predicted: number;
	    n_gpu_layers: number;
	    model: string;
	    n_cache_tokens: number;
	    cache_shift: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SlotInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.task = source["task"];
	        this.n_prompt = source["n_prompt"];
	        this.n_predicted = source["n_predicted"];
	        this.n_gpu_layers = source["n_gpu_layers"];
	        this.model = source["model"];
	        this.n_cache_tokens = source["n_cache_tokens"];
	        this.cache_shift = source["cache_shift"];
	    }
	}

}

export namespace main {
	
	export class BackendStatus {
	    current_backend: string;
	    config_backend: string;
	    gpu_vendor: string;
	    gpu_name: string;
	    gpu_vram_mb: number;
	    installed_backends: string[];
	    available_backends: string[];
	
	    static createFrom(source: any = {}) {
	        return new BackendStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.current_backend = source["current_backend"];
	        this.config_backend = source["config_backend"];
	        this.gpu_vendor = source["gpu_vendor"];
	        this.gpu_name = source["gpu_name"];
	        this.gpu_vram_mb = source["gpu_vram_mb"];
	        this.installed_backends = source["installed_backends"];
	        this.available_backends = source["available_backends"];
	    }
	}
	export class HealthChat {
	    available: boolean;
	    generating: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HealthChat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.generating = source["generating"];
	    }
	}
	export class HealthHardware {
	    cpu_cores: number;
	    has_gpu: boolean;
	    gpu_name: string;
	    gpu_vram_mb: number;
	    has_cuda_backend: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HealthHardware(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.cpu_cores = source["cpu_cores"];
	        this.has_gpu = source["has_gpu"];
	        this.gpu_name = source["gpu_name"];
	        this.gpu_vram_mb = source["gpu_vram_mb"];
	        this.has_cuda_backend = source["has_cuda_backend"];
	    }
	}
	export class HealthRAG {
	    available: boolean;
	    vector_store_initialized: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HealthRAG(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.vector_store_initialized = source["vector_store_initialized"];
	    }
	}
	export class HealthDatabase {
	    available: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new HealthDatabase(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.error = source["error"];
	    }
	}
	export class HealthLLM {
	    running: boolean;
	    model_ready: boolean;
	    permanent_failure: boolean;
	    load_failed: boolean;
	    current_model: string;
	    switching: boolean;
	    switching_to: string;
	    port: number;
	    api_base: string;
	    last_error: string;
	
	    static createFrom(source: any = {}) {
	        return new HealthLLM(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.model_ready = source["model_ready"];
	        this.permanent_failure = source["permanent_failure"];
	        this.load_failed = source["load_failed"];
	        this.current_model = source["current_model"];
	        this.switching = source["switching"];
	        this.switching_to = source["switching_to"];
	        this.port = source["port"];
	        this.api_base = source["api_base"];
	        this.last_error = source["last_error"];
	    }
	}
	export class HealthComponents {
	    llm_server: HealthLLM;
	    database: HealthDatabase;
	    chat_service: HealthChat;
	    rag: HealthRAG;
	    hardware: HealthHardware;
	
	    static createFrom(source: any = {}) {
	        return new HealthComponents(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.llm_server = this.convertValues(source["llm_server"], HealthLLM);
	        this.database = this.convertValues(source["database"], HealthDatabase);
	        this.chat_service = this.convertValues(source["chat_service"], HealthChat);
	        this.rag = this.convertValues(source["rag"], HealthRAG);
	        this.hardware = this.convertValues(source["hardware"], HealthHardware);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	
	
	
	export class HealthRuntime {
	    goroutines: number;
	    mem_alloc_bytes: number;
	    mem_sys_bytes: number;
	
	    static createFrom(source: any = {}) {
	        return new HealthRuntime(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.goroutines = source["goroutines"];
	        this.mem_alloc_bytes = source["mem_alloc_bytes"];
	        this.mem_sys_bytes = source["mem_sys_bytes"];
	    }
	}
	export class HealthVersion {
	    app: string;
	    go: string;
	
	    static createFrom(source: any = {}) {
	        return new HealthVersion(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.app = source["app"];
	        this.go = source["go"];
	    }
	}
	export class HealthStatus {
	    status: string;
	    timestamp: string;
	    uptime_seconds: number;
	    app_ready: boolean;
	    config_loaded: boolean;
	    version: HealthVersion;
	    components: HealthComponents;
	    runtime: HealthRuntime;
	
	    static createFrom(source: any = {}) {
	        return new HealthStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.status = source["status"];
	        this.timestamp = source["timestamp"];
	        this.uptime_seconds = source["uptime_seconds"];
	        this.app_ready = source["app_ready"];
	        this.config_loaded = source["config_loaded"];
	        this.version = this.convertValues(source["version"], HealthVersion);
	        this.components = this.convertValues(source["components"], HealthComponents);
	        this.runtime = this.convertValues(source["runtime"], HealthRuntime);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class SearchAPIKeys {
	    ollama_api_key: string;
	    tavily_api_key: string;
	    ollama_api_key_set: boolean;
	    tavily_api_key_set: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SearchAPIKeys(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ollama_api_key = source["ollama_api_key"];
	        this.tavily_api_key = source["tavily_api_key"];
	        this.ollama_api_key_set = source["ollama_api_key_set"];
	        this.tavily_api_key_set = source["tavily_api_key_set"];
	    }
	}
	export class StartupError {
	    title: string;
	    brief: string;
	    detail: string;
	
	    static createFrom(source: any = {}) {
	        return new StartupError(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.brief = source["brief"];
	        this.detail = source["detail"];
	    }
	}
	export class SwitchResult {
	    success: boolean;
	    error?: string;
	    current_model?: string;
	    capabilities?: llm.ModelCapabilities;
	    previous_model?: string;
	    rolled_back?: boolean;
	    rollback_success?: boolean;
	    params_restored?: boolean;
	
	    static createFrom(source: any = {}) {
	        return new SwitchResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.success = source["success"];
	        this.error = source["error"];
	        this.current_model = source["current_model"];
	        this.capabilities = this.convertValues(source["capabilities"], llm.ModelCapabilities);
	        this.previous_model = source["previous_model"];
	        this.rolled_back = source["rolled_back"];
	        this.rollback_success = source["rollback_success"];
	        this.params_restored = source["params_restored"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class UpdateInfo {
	    has_update: boolean;
	    latest_version: string;
	    current_version: string;
	    download_url: string;
	    release_notes: string;
	    published_at: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.has_update = source["has_update"];
	        this.latest_version = source["latest_version"];
	        this.current_version = source["current_version"];
	        this.download_url = source["download_url"];
	        this.release_notes = source["release_notes"];
	        this.published_at = source["published_at"];
	    }
	}

}

export namespace rag {
	
	export class CollectionInfo {
	    name: string;
	    dim: number;
	    vector_count: number;
	
	    static createFrom(source: any = {}) {
	        return new CollectionInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.dim = source["dim"];
	        this.vector_count = source["vector_count"];
	    }
	}
	export class DocumentMeta {
	    id: string;
	    collection: string;
	    file_name: string;
	    file_size: number;
	    mime_type: string;
	    chunk_count: number;
	    ingested_at: string;
	    tags?: Record<string, string>;
	
	    static createFrom(source: any = {}) {
	        return new DocumentMeta(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.collection = source["collection"];
	        this.file_name = source["file_name"];
	        this.file_size = source["file_size"];
	        this.mime_type = source["mime_type"];
	        this.chunk_count = source["chunk_count"];
	        this.ingested_at = source["ingested_at"];
	        this.tags = source["tags"];
	    }
	}

}

