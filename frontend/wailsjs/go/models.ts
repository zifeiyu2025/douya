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
	
	export class Config {
	    model_path: string;
	    mmproj_auto: boolean;
	    mmproj_offload: boolean;
	    llama_server_path: string;
	    api_base: string;
	    port: number;
	    context_size: number;
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
	    system_prompt: string;
	    chat_background: string;
	    user_avatar: string;
	    ai_avatar: string;
	    search_mode: string;
	    sleep_idle_seconds: number;
	    models_max: number;
	    rag_enabled: boolean;
	    rag_active_kb: string;
	    rag_top_k: number;
	    rag_min_score: number;
	    rag_chunk_size: number;
	    rag_chunk_overlap: number;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.model_path = source["model_path"];
	        this.mmproj_auto = source["mmproj_auto"];
	        this.mmproj_offload = source["mmproj_offload"];
	        this.llama_server_path = source["llama_server_path"];
	        this.api_base = source["api_base"];
	        this.port = source["port"];
	        this.context_size = source["context_size"];
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
	        this.system_prompt = source["system_prompt"];
	        this.chat_background = source["chat_background"];
	        this.user_avatar = source["user_avatar"];
	        this.ai_avatar = source["ai_avatar"];
	        this.search_mode = source["search_mode"];
        this.sleep_idle_seconds = source["sleep_idle_seconds"];
	        this.models_max = source["models_max"];
	        this.rag_enabled = source["rag_enabled"];
	        this.rag_active_kb = source["rag_active_kb"];
	        this.rag_top_k = source["rag_top_k"];
	        this.rag_min_score = source["rag_min_score"];
	        this.rag_chunk_size = source["rag_chunk_size"];
	        this.rag_chunk_overlap = source["rag_chunk_overlap"];
	    }
	}

}

export namespace llm {

	export class ChatMessage {
	    role: string;
	    content: any;
	    reasoning_content?: string;
	    tool_call_id?: string;

	    static createFrom(source: any = {}) {
	        return new ChatMessage(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.role = source["role"];
	        this.content = source["content"];
	        this.reasoning_content = source["reasoning_content"];
	        this.tool_call_id = source["tool_call_id"];
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
	export class ModelCapabilities {
	    image_input: boolean;
	    audio_input: boolean;
	    text_input: boolean;
	    reasoning: boolean;
	    mmproj_loaded: boolean;
	    tool_call_support: boolean;

	    static createFrom(source: any = {}) {
	        return new ModelCapabilities(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.image_input = source["image_input"];
	        this.audio_input = source["audio_input"];
	        this.text_input = source["text_input"];
	        this.reasoning = source["reasoning"];
	        this.mmproj_loaded = source["mmproj_loaded"];
	        this.tool_call_support = source["tool_call_support"];
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
	    status: string;
	
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
	        this.status = source["status"];
	    }
	}
	export class ServerStatus {
	    running: boolean;
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

}

export namespace main {

	export class SmartParamsInfo {
	    hardware: { cpu_cores: number; has_gpu: boolean; gpu_name: string; gpu_vram_mb: number; };
	    model: { architecture: string; block_count: number; embedding_length: number; context_length: number; file_size_mb: number; expert_count: number; expert_used: number; has_mtp: boolean; has_reasoning: boolean; n_params: number; size_label: string; };
	    params: { gpu_layers: number; threads: number; batch_size: number; ubatch_size: number; flash_attn: boolean; cache_type_k: string; cache_type_v: string; mlock: boolean; mmproj_offload: boolean; context_size: number; spec_type: string; spec_draft_n_max: number; spec_draft_n_min: number; ngram_mod_n_min: number; ngram_mod_n_max: number; ngram_mod_n_match: number; };
	    overrides: { gpu_layers: boolean; flash_attn: boolean; mlock: boolean; threads: boolean; batch_size: boolean; context_size: boolean; cache_type_k: boolean; cache_type_v: boolean; spec_type: boolean; };

	    static createFrom(source: any = {}) {
	        return new SmartParamsInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.hardware = source["hardware"];
	        this.model = source["model"];
	        this.params = source["params"];
	        this.overrides = source["overrides"];
	    }
	}
	export class SearchAPIKeys {
	    ollama_api_key: string;
	    tavily_api_key: string;

	    static createFrom(source: any = {}) {
	        return new SearchAPIKeys(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ollama_api_key = source["ollama_api_key"];
	        this.tavily_api_key = source["tavily_api_key"];
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

