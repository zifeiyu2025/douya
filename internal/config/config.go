// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"douya/internal/apperror"

	"github.com/rs/zerolog/log"
)

type Config struct {
	// Version 配置 schema 版本号，用于版本化迁移。
	// 0 表示旧版本（无此字段的历史配置），加载时按迁移链升级到当前版本。
	Version    int    `json:"version"`
	ModelPath  string `json:"model_path"`
	MmprojAuto bool   `json:"mmproj_auto"`
	// MmprojOffload mmproj GPU 卸载开关。
	// nil=自动（由 llama.cpp 原生判断），true=强制启用，false=强制关闭。
	// 使用 *bool 是为了区分"未设置"与"显式 false"：
	// 之前是 bool 且默认 true，导致用户设 mmproj_offload=false 时被 `if cfg.MmprojOffload { d.MmprojOffload = true }`
	// 单方向覆盖，"关闭"路径永远不可达。改为 *bool 后 false 可真正生效。
	MmprojOffload   *bool  `json:"mmproj_offload"`
	LlamaServerPath string `json:"llama_server_path"`
	// BackendType 计算后端类型（auto/cuda/hip/sycl/vulkan/openvino/cpu）。
	// auto 表示根据硬件自动检测最合适的后端，其他值明确指定后端类型。
	// 生活类比：就像选发动机型号——auto 是"让系统帮你选"，其他是明确指定用哪种发动机。
	BackendType string `json:"backend_type"`
	// LastSuccessfulBackend 上一次成功启动的后端类型（用于启动失败时回退）。
	// 生活类比：换发动机前先给现在的发动机舱拍张照，新发动机打不着火就按照片装回去。
	// 非空表示用户切换过后端但新后端尚未通过启动验证；启动成功后会被清空。
	LastSuccessfulBackend string `json:"last_successful_backend"`
	APIBase               string `json:"api_base"`
	Port                  int    `json:"port"`
	ContextSize           int    `json:"context_size"`
	// ProactiveCompressThreshold 主动压缩阈值：当估算 token 占比 >= 该阈值时，
	// 提前触发上下文压缩（不等溢出），为后续对话留出空间。
	// 默认 0.8（80%），范围 0.5-0.95。值越小越激进（更早压缩）。
	ProactiveCompressThreshold float64 `json:"proactive_compress_threshold"`
	Temperature                float64 `json:"temperature"`
	TopP                       float64 `json:"top_p"`
	TopK                       int     `json:"top_k"`
	RepeatPenalty              float64 `json:"repeat_penalty"`
	KVUnified                  bool    `json:"kv_unified"`
	CacheIdleSlots             bool    `json:"cache_idle_slots"`
	CacheRAM                   int     `json:"cache_ram"`
	ImageMinTokens             int     `json:"image_min_tokens"`
	ImageMaxTokens             int     `json:"image_max_tokens"`
	FitTarget                  int     `json:"fit_target"`
	FitCtx                     int     `json:"fit_ctx"`
	Reasoning                  string  `json:"reasoning"`
	ReasoningBudget            int     `json:"reasoning_budget"`
	ReasoningFormat            string  `json:"reasoning_format"`
	// 思考强度（模板级 reasoning_effort，空=不传递跟随模型默认）
	// 由聊天请求直接设置请求级 OAI 字段 ReasoningEffort，
	// 交由新版 llama-server（#26045 + #27041）原生转发给支持该参数的模板
	// （如 DeepSeek-V4 / openai-gpt-oss-120b / 腾讯混元 Hy3 等），
	// 服务器层仅对顶层 "none" 有语义，其余值由模板自行解释。
	ReasoningEffort string `json:"reasoning_effort"`
	// 推理内容保留开关（nil=不传递，true=--reasoning-preserve，false=--no-reasoning-preserve）
	ReasoningPreserve *bool  `json:"reasoning_preserve"`
	SystemPrompt      string `json:"system_prompt"`
	SystemPromptMode  string `json:"system_prompt_mode"` // "append" (追加) or "replace" (替换), 默认 "append"
	// 编程助手模式：控制默认提示词使用通用版还是编程版。
	// "auto"（默认）：检测到 coder 类模型自动启用编程版；"on"：始终启用；"off"：始终禁用。
	ProgrammingMode       string  `json:"programming_mode"`
	ChatBackground        string  `json:"chat_background"`
	ChatBackgroundOpacity float64 `json:"chat_background_opacity"`
	UserAvatar            string  `json:"user_avatar"`
	AiAvatar              string  `json:"ai_avatar"`
	SearchMode            string  `json:"search_mode"` // "off", "auto", "on"
	// Deprecated: 已迁移到 Reasoning 字段，保留仅为向后兼容
	ThinkingEnabled bool `json:"thinking_enabled"`
	// Deprecated: 已迁移到 Reasoning 字段，保留仅为向后兼容
	ThinkingSoftSwitch     string  `json:"thinking_soft_switch"`
	SleepIdleSeconds       int     `json:"sleep_idle_seconds"`
	ModelsMax              int     `json:"models_max"`
	RAGEnabled             bool    `json:"rag_enabled"`
	RAGActiveKB            string  `json:"rag_active_kb"`
	RAGTopK                int     `json:"rag_top_k"`
	RAGMinScore            float64 `json:"rag_min_score"`
	RAGChunkSize           int     `json:"rag_chunk_size"`
	RAGChunkOverlap        int     `json:"rag_chunk_overlap"`
	EmbeddingModel         string  `json:"embedding_model"` // 专用嵌入模型路径（可选，为空则用聊天模型）
	ReasoningBudgetMessage string  `json:"reasoning_budget_message"`
	// 请求级 reasoning 扩展字段（v9744+，为空则不传递，使用服务器默认值）
	ReasoningBudgetStartTag string  `json:"reasoning_budget_start_tag"` // 思考预算区间起始标记
	ReasoningBudgetEndTag   string  `json:"reasoning_budget_end_tag"`   // 思考预算区间结束标记
	Mmap                    bool    `json:"mmap"`
	KVOffload               bool    `json:"kv_offload"`
	ContextShift            bool    `json:"context_shift"`
	MinP                    float64 `json:"min_p"`
	DryMultiplier           float64 `json:"dry_multiplier"`
	DryBase                 float64 `json:"dry_base"`
	DryAllowedLength        int     `json:"dry_allowed_length"`
	DrySequenceBreaker      string  `json:"dry_sequence_breaker"`     // Dry 采样序列中断符（逗号分隔，如 "\n","。"）
	DryPenaltyLastN         int     `json:"dry_penalty_last_n"`       // Dry 惩罚窗口大小（0=使用 repeat_last_n）
	GrpAttnN                int     `json:"grp_attn_n"`               // 分组注意力组数（0=禁用，默认1）
	GrpAttnW                int     `json:"grp_attn_w"`               // 分组注意力窗口宽度（0=禁用，默认512）
	Jinja                   *bool   `json:"jinja"`                    // Jinja2 模板引擎开关（nil=不传递，true=--jinja，false=--no-jinja）
	CachePrompt             *bool   `json:"cache_prompt"`             // Prompt 缓存控制（nil=不传递，true=--cache-prompt，false=--no-cache-prompt）
	Metrics                 bool    `json:"metrics"`                  // 服务器指标端点开关
	Verbose                 bool    `json:"verbose"`                  // 详细日志开关
	SpecDraftThreads        int     `json:"spec_draft_threads"`       // Draft 模型线程数（0=不传递）
	SpecDraftThreadsBatch   int     `json:"spec_draft_threads_batch"` // Draft 模型批处理线程数（0=不传递）
	SpecDefault             bool    `json:"spec_default"`             // 使用默认推测解码配置
	Device                  string  `json:"device"`
	// 多 GPU 原生参数。空 split_mode 表示不覆盖 llama.cpp 默认的 layer 模式；
	// main_gpu=-1 表示不传递，让 llama.cpp 使用默认设备。
	SplitMode              string  `json:"split_mode"`
	TensorSplit            string  `json:"tensor_split"`
	MainGPU                int     `json:"main_gpu"`
	Parallel               int     `json:"parallel"`
	CacheTypeK             string  `json:"cache_type_k"`
	CacheTypeV             string  `json:"cache_type_v"`
	SpecType               string  `json:"spec_type"`
	SpecDraftNMax          int     `json:"spec_draft_n_max"`
	SpecDraftNMin          int     `json:"spec_draft_n_min"`
	CacheTypeKDraft        string  `json:"cache_type_k_draft"`
	CacheTypeVDraft        string  `json:"cache_type_v_draft"`
	SpecNgramModNMin       int     `json:"spec_ngram_mod_n_min"`
	SpecNgramModNMax       int     `json:"spec_ngram_mod_n_max"`
	SpecNgramModNMatch     int     `json:"spec_ngram_mod_n_match"`
	SpecNgramSimpleSizeN   int     `json:"spec_ngram_simple_size_n"`
	SpecNgramSimpleSizeM   int     `json:"spec_ngram_simple_size_m"`
	SpecNgramSimpleMinHits int     `json:"spec_ngram_simple_min_hits"`
	SpecNgramMapKSizeN     int     `json:"spec_ngram_map_k_size_n"`
	SpecNgramMapKSizeM     int     `json:"spec_ngram_map_k_size_m"`
	SpecNgramMapKMinHits   int     `json:"spec_ngram_map_k_min_hits"`
	SpecNgramMapK4VSizeN   int     `json:"spec_ngram_map_k4v_size_n"`
	SpecNgramMapK4VSizeM   int     `json:"spec_ngram_map_k4v_size_m"`
	SpecNgramMapK4VMinHits int     `json:"spec_ngram_map_k4v_min_hits"`
	LookupCacheStatic      string  `json:"lookup_cache_static"`
	LookupCacheDynamic     string  `json:"lookup_cache_dynamic"`
	SpecDraftModel         string  `json:"spec_draft_model"`
	ServerAPIKeyEnabled    bool    `json:"server_api_key_enabled"`
	ExposeServer           bool    `json:"expose_server"` // 暴露服务器地址，允许局域网访问
	EnableWebUI            bool    `json:"enable_web_ui"` // 启用 llama-server 自带的原生 Web UI（默认关闭）
	SwaFull                bool    `json:"swa_full"`
	CtxCheckpoints         int     `json:"ctx_checkpoints"`
	CheckpointMinStep      int     `json:"checkpoint_min_step"`
	Tools                  string  `json:"tools"`
	EnableBuiltinTools     bool    `json:"enable_builtin_tools"` // 启用 llama.cpp 全部内置工具（--tools all）
	PrefillAssistant       bool    `json:"prefill_assistant"`
	SlotPromptSimilarity   float64 `json:"slot_prompt_similarity"`
	SkipChatParsing        bool    `json:"skip_chat_parsing"`
	APIPrefix              string  `json:"api_prefix"`
	SimpleIO               bool    `json:"simple_io"`
	GPULayers              int     `json:"gpu_layers"`   // 0=自动（99全部卸载），正数=指定层数
	FlashAttn              *bool   `json:"flash_attn"`   // nil=自动，指针类型区分"未设置"和"false"
	Mlock                  *bool   `json:"mlock"`        // nil=自动
	Threads                int     `json:"threads"`      // 0=自动
	ThreadsHTTP            int     `json:"threads_http"` // HTTP 请求处理线程数（0=自动，llama.cpp 默认）
	BatchSize              int     `json:"batch_size"`   // 0=自动
	CloseAction            string  `json:"close_action"` // "ask"(默认), "tray"(最小化到托盘), "exit"(直接退出)
	// RAG 重排序配置
	RerankerModelPath string `json:"reranker_model_path"` // reranker 模型路径（可选，为空则不启用重排序）
	RerankTopN        int    `json:"rerank_top_n"`        // 重排序后返回的 top-N 结果数（默认5）
	// KV 缓存持久化配置
	SlotSavePath    string `json:"slot_save_path"`    // KV 缓存保存路径（为空则使用默认路径 appDir/slots/）
	SlotSaveEnabled bool   `json:"slot_save_enabled"` // 是否启用 KV 缓存持久化
	CacheReuse      int    `json:"cache_reuse"`       // KV 缓存复用块大小（0=禁用，默认0）
	// Draft 模型 GPU 配置（Eagle3 等需要独立 draft 模型的场景）
	SpecDraftNgl    int    `json:"spec_draft_ngl"`    // draft 模型 GPU 层数（0=不传递）
	SpecDraftDevice string `json:"spec_draft_device"` // draft 模型设备（如 "cuda:0"，为空则不传递）
	// Draft 模型推测解码参数
	SpecDraftPSplit          float64 `json:"spec_draft_p_split"`          // 推测解码 split 概率（0=默认 0.10）
	SpecDraftPMin            float64 `json:"spec_draft_p_min"`            // 最小推测解码概率（0=默认 0.00）
	SpecDraftBackendSampling *bool   `json:"spec_draft_backend_sampling"` // draft 后端采样（nil=默认）
	// 多模态批处理
	MtmdBatchMaxTokens int `json:"mtmd_batch_max_tokens"` // 图像编码 batch 最大 token 数（0=默认 1024）
	// 自适应采样（llama.cpp 新增，动态调整采样参数）
	AdaptiveTarget float64 `json:"adaptive_target"` // 自适应采样目标概率（0=禁用，0-1）
	AdaptiveDecay  float64 `json:"adaptive_decay"`  // 自适应采样衰减率（0=默认 0.5，0-1）
	// 请求级采样扩展字段（llama.cpp 新增）
	Samplers  string `json:"samplers"`   // 自定义采样器顺序（逗号分隔，如 "top_k,top_p,temperature"）
	IgnoreEos bool   `json:"ignore_eos"` // 忽略 EOS 继续生成
	// 模型标签（逗号分隔，用于 /v1/models 返回的 tags 字段）
	Tags string `json:"tags"`
	// 媒体路径（多模态模型额外媒体文件目录）
	MediaPath string `json:"media_path"`
	// 离线模式（禁用所有网络请求，如模型下载等）
	Offline bool `json:"offline"`
	// 模型重打包（启动时重新打包模型权重，用于优化加载速度）
	Repack bool `json:"repack"`
	// Agent 模式与 MCP CORS 代理（llama.cpp 新特性）
	Agent      bool `json:"agent"`        // 一键启用 CORS 代理 + 所有内置工具
	UIMcpProxy bool `json:"ui_mcp_proxy"` // 仅启用 MCP CORS 代理（Agent 已包含此项）
	// 细粒度 CORS 配置（上游 --cors-*，llama.cpp #25655）
	// 优先于 llama.cpp 内置的 localhost 默认值，用于自定义浏览器跨域来源。
	// 生活类比：像校门口访客登记——默认只放行本班（localhost），登记表可额外写明放行哪些班级（外部来源）。
	CorsOrigins     string `json:"cors_origins"`     // 允许的来源，逗号分隔（如 "http://localhost:5173,*"），空=使用 llama.cpp 默认
	CorsMethods     string `json:"cors_methods"`     // 允许的 HTTP 方法，逗号分隔（空=使用 llama.cpp 默认）
	CorsHeaders     string `json:"cors_headers"`     // 允许的请求头，逗号分隔（空=使用 llama.cpp 默认）
	CorsCredentials bool   `json:"cors_credentials"` // 是否允许携带凭证（true 且 origins=* 时服务端会回显 Origin 并始终允许凭证）
	// 后端采样（实验性，将采样逻辑移到 GPU 执行，不兼容 grammar 和 reasoning budget）
	BackendSampling bool `json:"backend_sampling"`

	// ===== TTS 文本转语音设置 =====
	// 生活类比：像"播音员调度台"的配置——挑哪个播音员、调快慢、调音调、调音量。
	// 前端用浏览器原生 SpeechSynthesis API 实现，无需后端参与推理，
	// 后端只负责持久化配置（用户关闭程序后下次打开仍在）。
	// TtsEnabled 是否启用朗读按钮（关闭则隐藏朗读入口）
	TtsEnabled bool `json:"tts_enabled"`
	// TtsVoice 发音人名称（空=自动按优先级挑选，如 "Microsoft Xiaoxiao"）
	TtsVoice string `json:"tts_voice"`
	// TtsRate 语速：0.5（慢）- 2.0（快），1.0 = 正常速度
	TtsRate float64 `json:"tts_rate"`
	// TtsPitch 音调：0（低）- 2（高），1.0 = 正常音调
	TtsPitch float64 `json:"tts_pitch"`
	// TtsVolume 音量：0（静音）- 1（最大），1.0 = 最大音量
	TtsVolume float64 `json:"tts_volume"`
	// TtsOnline 是否优先使用微软在线 TTS（Edge TTS / 晓晓）。
	// 开启：有网用微软在线晓晓，无网自动回退本地语音；关闭：始终本地 Web Speech API。
	TtsOnline bool `json:"tts_online"`
	// SSE ping 间隔秒数（0=使用服务器默认 30 秒，用于保持长连接活跃）
	SsePingInterval int `json:"sse_ping_interval"`
	// LoRA 适配器路径（逗号分隔，启动时通过 --lora 加载，配合 --lora-init-without-apply 默认不应用）
	LoraPaths string `json:"lora_paths"`
	// ChatTemplateFile 自定义聊天模板文件路径（.jinja 文件，通过 --chat-template-file 传递给 llama-server）
	// 配置后优先于模型 GGUF 自带模板，用于自定义对话格式（如多轮提示、角色扮演模板）
	ChatTemplateFile string `json:"chat_template_file"`
	// 直接 I/O（绕过操作系统页面缓存，加速大模型加载，避免内存污染）
	DirectIO bool `json:"direct_io"`
	// MoE 权重 CPU 卸载（将所有专家权重保留在 CPU，显存不足时启用）
	CPUMoe bool `json:"cpu_moe"`
	// 前 N 层 MoE 权重 CPU 卸载（0=不启用，精细控制 --cpu-moe 的影响范围）
	NCpuMoe int `json:"n_cpu_moe"`
	// 算子卸载开关（nil=使用默认值 true，true=--op-offload，false=--no-op-offload）
	OpOffload *bool `json:"op_offload"`
	// MCP 服务器列表（由豆芽写入 mcp_servers.json，llama-server 通过 --mcp-servers-config 加载）
	// 空数组表示不启用任何 MCP server，不影响现有行为
	MCPServers []MCPServerConfig `json:"mcp_servers"`
}

func DefaultConfig() *Config {
	return &Config{
		Version:         2, // 当前配置 schema 版本号（P4.3 与 currentConfigVersion/migrate 对齐）
		ModelPath:       "",
		MmprojAuto:      true,
		MmprojOffload:   nil,
		LlamaServerPath: "runtime/llama-server.exe",
		BackendType:     "auto", // 默认自动检测最合适的后端
		APIBase:         "http://127.0.0.1:8080",
		Port:            8080,
		// P4.1 修复：默认 context_size 改为 0（未设置），让 smart-params 按
		// GPU 显存预算计算合适的上下文长度。此前默认 8192 会覆盖智能计算
		// （resolveIntDerived 中任何 >0 用户值都赢），导致 4-6GB 显卡首启
		// 用 8192 上下文反而 KV OOM。0 表示"未设置"，走智能值。
		ContextSize:                0,
		ProactiveCompressThreshold: 0.8, // P1-A1: 80% 时主动压缩，为后续对话留出 20% 空间
		Temperature:                0.8, // 与 llama.cpp 默认值对齐
		TopP:                       0.95,
		TopK:                       40, // 与 llama.cpp 默认值对齐
		RepeatPenalty:              1,
		KVUnified:                  false,
		CacheIdleSlots:             true, // 与 llama.cpp 默认值对齐，空闲 slot 缓存保留
		CacheRAM:                   0,
		ImageMinTokens:             0,
		ImageMaxTokens:             0,
		FitTarget:                  0,
		FitCtx:                     0,
		Reasoning:                  "off",
		ReasoningBudget:            0,
		ReasoningFormat:            "",
		ReasoningEffort:            "",
		SystemPrompt:               "",
		SystemPromptMode:           "append", // 默认使用追加模式
		ProgrammingMode:            "auto",   // 默认自动检测（coder 模型启用编程版提示词）
		ChatBackground:             "",
		ChatBackgroundOpacity:      0.9,
		UserAvatar:                 "",
		AiAvatar:                   "",
		SearchMode:                 "off",
		ThinkingEnabled:            true,
		ThinkingSoftSwitch:         "auto",
		SleepIdleSeconds:           -1, // 与 llama.cpp 默认值对齐，-1 禁用空闲休眠
		ModelsMax:                  1,
		RAGEnabled:                 false,
		RAGActiveKB:                "default",
		RAGTopK:                    3,
		RAGMinScore:                0.3,
		RAGChunkSize:               512,
		RAGChunkOverlap:            64,
		ReasoningBudgetMessage:     "",
		ReasoningBudgetStartTag:    "",
		ReasoningBudgetEndTag:      "",
		Mmap:                       true,
		// KV 缓存 GPU 卸载：默认关闭，由用户在设置中按需开启。
		// 开启后可将 KV cache 卸载到 GPU 降低首 token 延迟，但会增加显存占用。
		KVOffload: false,
		// 上下文移位（context-shift）：默认关闭。
		// 应用层已有主动压缩 + 摘要机制，无需 llama-server 兜底移位；
		// 用户如需更激进的自动兜底，可在设置中手动开启。
		ContextShift:             false,
		MinP:                     0.05,
		DryMultiplier:            0,
		DryBase:                  1.75,
		DryAllowedLength:         2,
		DrySequenceBreaker:       "",
		DryPenaltyLastN:          0,
		GrpAttnN:                 0,
		GrpAttnW:                 0,
		Jinja:                    new(true), // 默认开启 Jinja2 模板引擎，支持更复杂的 chat template 语法
		CachePrompt:              new(true), // 显式启用 prompt 缓存，多轮对话时复用前缀 KV，降低首 token 延迟
		Metrics:                  false,
		Verbose:                  false,
		SpecDraftThreads:         0,
		SpecDraftThreadsBatch:    0,
		SpecDefault:              false,
		Device:                   "",
		SplitMode:                "",
		TensorSplit:              "",
		MainGPU:                  -1,
		Parallel:                 0,
		CacheTypeK:               "",
		CacheTypeV:               "",
		SpecType:                 "",
		SpecDraftNMax:            0,
		SpecDraftNMin:            0,
		CacheTypeKDraft:          "",
		CacheTypeVDraft:          "",
		ServerAPIKeyEnabled:      false,
		ExposeServer:             false,
		EnableWebUI:              false,
		SwaFull:                  false,
		CtxCheckpoints:           32,  // 与 llama.cpp 默认值对齐，长上下文检查点回滚
		CheckpointMinStep:        256, // 与 llama.cpp 默认值对齐，检查点最小步长
		Tools:                    "",
		EnableBuiltinTools:       false,
		PrefillAssistant:         true,
		SlotPromptSimilarity:     0.1, // 与 llama.cpp 默认值对齐，slot 缓存 prompt 相似度阈值
		SkipChatParsing:          false,
		APIPrefix:                "",
		SimpleIO:                 false,
		GPULayers:                0,
		FlashAttn:                nil,
		Mlock:                    nil,
		Threads:                  0,
		ThreadsHTTP:              0,
		BatchSize:                0,
		CloseAction:              "ask",
		RerankerModelPath:        "",
		RerankTopN:               5,
		SlotSavePath:             "",
		SlotSaveEnabled:          true,
		SpecDraftNgl:             0,
		SpecDraftDevice:          "",
		SpecDraftPSplit:          0,
		SpecDraftPMin:            0,
		SpecDraftBackendSampling: nil,
		MtmdBatchMaxTokens:       0,
		AdaptiveTarget:           0,
		AdaptiveDecay:            0,
		Samplers:                 "",
		IgnoreEos:                false,
		Tags:                     "",
		MediaPath:                "",
		Offline:                  false,
		Repack:                   false,
		EmbeddingModel:           "",
		SpecNgramModNMin:         0,
		SpecNgramModNMax:         0,
		SpecNgramModNMatch:       0,
		SpecNgramSimpleSizeN:     0,
		SpecNgramSimpleSizeM:     0,
		SpecNgramSimpleMinHits:   0,
		SpecNgramMapKSizeN:       0,
		SpecNgramMapKSizeM:       0,
		SpecNgramMapKMinHits:     0,
		SpecNgramMapK4VSizeN:     0,
		SpecNgramMapK4VSizeM:     0,
		SpecNgramMapK4VMinHits:   0,
		LookupCacheStatic:        "",
		LookupCacheDynamic:       "",
		SpecDraftModel:           "",
		// 默认 256：启用 KV 缓存块复用，对重复的 system prompt 前缀加速
		// 256 是合理块大小，覆盖豆芽 system prompt（约 200-400 token）
		CacheReuse:      256,
		Agent:           false,
		UIMcpProxy:      false,
		BackendSampling: false,
		// 细粒度 CORS：空值走 llama.cpp 默认（localhost），保持与升级前行为一致
		CorsOrigins:     "",
		CorsMethods:     "",
		CorsHeaders:     "",
		CorsCredentials: false,
		// TTS 文本转语音默认配置
		// 默认启用朗读按钮，发音人留空（自动按优先级挑选晓晓等自然语音）
		TtsEnabled: true,
		TtsVoice:   "",
		TtsRate:    1.0,
		TtsPitch:   1.0,
		TtsVolume:  1.0,
		// TtsOnline 默认开启：有网优先使用微软在线晓伊，无网自动回退本地语音
		TtsOnline:        true,
		SsePingInterval:  0,
		LoraPaths:        "",
		ChatTemplateFile: "",
		DirectIO:         false,
		CPUMoe:           false,
		NCpuMoe:          0,
		OpOffload:        nil,
		MCPServers:       []MCPServerConfig{},
	}
}

func Load(path string) (*Config, error) {
	log.Info().Str("path", path).Msg("[config] 开始加载配置文件")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info().Msg("[config] 配置文件不存在，创建默认配置")
			cfg := DefaultConfig()
			if saveErr := Save(path, cfg); saveErr != nil {
				return nil, apperror.Wrap(apperror.KindInvalidConfig, "创建默认配置文件失败", saveErr)
			}
			return cfg, nil
		}
		log.Error().Err(err).Str("path", path).Msg("[config] 读取配置文件失败")
		return nil, apperror.Wrap(apperror.KindInvalidConfig, "读取配置文件失败", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		if strings.HasPrefix(strings.TrimSpace(string(data)), "\"") {
			var inner string
			if unquoteErr := json.Unmarshal(data, &inner); unquoteErr == nil {
				if innerErr := json.Unmarshal([]byte(inner), cfg); innerErr == nil {
					cfg.migrate([]byte(inner))
					if saveErr := Save(path, cfg); saveErr != nil {
						log.Warn().Err(saveErr).Msg("[config] 保存迁移后配置失败")
					}
					// 校验配置，若失败则回退到默认配置并写盘，避免每次启动都告警
					if validateErr := cfg.Validate(); validateErr != nil {
						log.Warn().Err(validateErr).Msg("[config] 配置校验失败，回退到默认配置并写盘")
						fallback := DefaultConfig()
						if saveErr := Save(path, fallback); saveErr != nil {
							log.Warn().Err(saveErr).Msg("[config] 保存回退配置失败")
						}
						return fallback, nil
					}
					return cfg, nil
				}
			}
		}
		// F-1 修复：配置文件损坏时，备份原文件后用默认配置启动，避免用户完全无法进入应用。
		// 生活类比：菜谱被水泡模糊了，先把旧菜谱收起来（备份），用标准菜谱继续营业，
		// 用户可以再自己调整口味。
		log.Error().Err(err).Msg("[config] 解析配置文件失败，备份原文件后用默认配置启动")
		backupPath := path + ".corrupt-" + time.Now().Format("20060102-150405")
		if backupErr := os.WriteFile(backupPath, data, 0o600); backupErr != nil {
			log.Warn().Err(backupErr).Msg("[config] 备份损坏的配置文件失败")
		} else {
			log.Info().Str("backup", backupPath).Msg("[config] 已备份损坏的配置文件")
		}
		fallback := DefaultConfig()
		if saveErr := Save(path, fallback); saveErr != nil {
			log.Warn().Err(saveErr).Msg("[config] 保存默认配置失败")
		}
		return fallback, nil
	}

	cfg.migrate(data)

	// 校验配置，若失败则逐字段修复无效值，保留用户其他设置
	if validateErr := cfg.Validate(); validateErr != nil {
		log.Warn().Err(validateErr).Msg("[config] 配置校验失败，开始逐字段修复")
		repairedFields := cfg.repairInvalidFields()
		if len(repairedFields) > 0 {
			log.Info().Strs("fields", repairedFields).Msg("[config] 已修复无效字段")
		}
		// 修复后重新校验
		if reValidateErr := cfg.Validate(); reValidateErr != nil {
			// 修复后仍校验失败（理论上不应发生），回退到默认配置保底
			log.Error().Err(reValidateErr).Msg("[config] 修复后仍校验失败，回退到默认配置")
			fallback := DefaultConfig()
			if saveErr := Save(path, fallback); saveErr != nil {
				log.Warn().Err(saveErr).Msg("[config] 保存回退配置失败")
			}
			return fallback, nil
		}
		// 修复后校验通过，保存修复后的配置
		if saveErr := Save(path, cfg); saveErr != nil {
			log.Warn().Err(saveErr).Msg("[config] 保存修复后配置失败")
		}
	}
	// 补全缺失的配置项（新增字段），值用 cfg 当前值（含迁移结果），保留用户已有值
	ensureConfigFields(path, data, cfg)
	log.Info().Str("path", path).Msg("[config] 配置加载完成")
	return cfg, nil
}

// ensureConfigFields 补全磁盘配置文件中缺失的配置项。
// 仅当 cfg 中存在但用户磁盘文件中缺失的字段时，将这些字段补入磁盘文件。
// 用户已有的字段值完全保留，不修改。
// 原因：新版本新增字段时，旧 config.json 缺失这些字段，虽然内存中用默认值能正常运行，
// 但磁盘文件不更新会导致下次启动仍缺失，且迁移后的字段值（如 reasoning）无法持久化。
func ensureConfigFields(path string, userData []byte, cfg *Config) {
	// 处理字符串包裹的 JSON（双重序列化情况）
	raw := userData
	if strings.HasPrefix(strings.TrimSpace(string(raw)), "\"") {
		var inner string
		if json.Unmarshal(raw, &inner) == nil {
			raw = []byte(inner)
		}
	}

	var userMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &userMap); err != nil {
		return // 解析失败走原有逻辑，不补全
	}

	// cfg 已经过迁移和校验，其 JSON 包含所有应有字段及其正确值
	cfgJSON, err := json.Marshal(cfg)
	if err != nil {
		return
	}
	var cfgMap map[string]json.RawMessage
	if err := json.Unmarshal(cfgJSON, &cfgMap); err != nil {
		return
	}

	// 找出缺失的 key，用 cfg 的值补入
	missing := false
	for k, v := range cfgMap {
		if _, ok := userMap[k]; !ok {
			userMap[k] = v
			missing = true
		}
	}

	if missing {
		if merged, err := json.MarshalIndent(userMap, "", "  "); err == nil {
			// P3.7：改用原子写，避免写一半崩溃留下损坏的 config.json
			if writeErr := writeFileAtomic(path, merged, 0o600); writeErr != nil {
				log.Warn().Err(writeErr).Msg("[config] 写入补全字段后的配置失败")
			}
		}
	}
}

// currentConfigVersion 当前配置 schema 版本号，与 DefaultConfig 中的 Version 保持一致。
// 每次新增 schema 变更时递增此常量，并在 migrate 中追加对应迁移分支。
// P4.3 修复：此前为 1，但 migrate 中已有 v1→v2 分支（CacheReuse），
// 实际迁移产物版本是 2，常量与 DefaultConfig 的 1 均不匹配真实 schema 版本。
const currentConfigVersion = 2

// migrate 执行配置 schema 版本迁移链。
// 根据 cfg.Version 逐步升级到 currentConfigVersion。
// data 为原始磁盘文件内容，用于检测旧字段是否存在。
// 迁移采用"按版本号分支"策略，避免靠"检测业务字段是否存在"推断版本时
// 多个历史版本字段交错导致迁移分支脆弱。
func (c *Config) migrate(data []byte) {
	// 检测原始数据是否包含 version 字段。
	// 旧版本配置无此字段，强制视为 Version=0，按迁移链升级。
	// 注意：检测 version 字段是确定版本号的正当手段（version 本身就是版本标识），
	// 与"检测 thinking_enabled 等业务字段推断版本"有本质区别。
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["version"]; !ok {
			c.Version = 0
		}
	}
	// Version=0 表示旧版本（无 version 字段的历史配置），需迁移到 v1
	if c.Version < 1 {
		// v0 -> v1：将旧版 thinking 配置迁移到 Reasoning 字段
		c.migrateLegacyThinking(data)
		c.Version = 1
	}
	// v1 -> v2：启用 cache-reuse 加速 KV 缓存复用。
	// 注意：原迁移逻辑会强制开启 context-shift，但已改为默认关闭（应用层压缩已足够兜底），
	// 因此迁移时不再强制设置 ContextShift，仅迁移 CacheReuse。
	// 老用户如已在 v2 迁移时开启 context-shift，其 config.json 中已写入 true，读取时保持用户选择。
	if c.Version < 2 {
		if c.CacheReuse == 0 {
			c.CacheReuse = 256
		}
		c.Version = 2
	}
	// 未来版本迁移在此追加，例如：
	// if c.Version < 3 { /* v2 -> v3 迁移逻辑 */; c.Version = 3 }
}

// migrateLegacyThinking 将旧版 thinking 配置迁移到 Reasoning 字段。
// 仅当原始配置数据中缺少 reasoning 字段时执行迁移，避免覆盖用户显式设置。
func (c *Config) migrateLegacyThinking(data []byte) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err == nil {
		if _, ok := raw["reasoning"]; ok {
			return
		}
	}
	if !c.ThinkingEnabled {
		c.Reasoning = "off"
		return
	}
	switch c.ThinkingSoftSwitch {
	case "think":
		c.Reasoning = "on"
	case "no_think":
		c.Reasoning = "off"
	default: // "auto" 或空：自动思考已移除，默认关闭
		c.Reasoning = "off"
	}
}

func LoadRaw(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(data))
	if strings.HasPrefix(trimmed, "\"") {
		var inner string
		if unquoteErr := json.Unmarshal(data, &inner); unquoteErr == nil {
			data = []byte(inner)
		}
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

// configSaveMu 串行化 Save 写盘，避免多 goroutine 并发写 config.json 互相覆盖/交错。
// 生活类比：多个人同时往同一张表格上写字会写得乱七八糟，
// 加一把"笔筒锁"（互斥锁），一次只让一个人写。
var configSaveMu sync.Mutex

func Save(path string, cfg *Config) error {
	log.Debug().Str("path", path).Msg("[config] 保存配置文件")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("[config] 序列化配置失败")
		return err
	}
	// 注：配置含 MCP 服务器 Env 等明文凭据，收紧为 0o600 仅所有者可读写，
	// 避免同机其他用户读取。API Key 本身已用 AES-GCM 加密存储。
	// 见安全审查 #6（M3 修复：原 0o644 全局可读）。
	configSaveMu.Lock()
	defer configSaveMu.Unlock()
	if err := writeFileAtomic(path, data, 0o600); err != nil {
		log.Error().Err(err).Str("path", path).Msg("[config] 写入配置文件失败")
		return err
	}
	return nil
}

// writeFileAtomic 原子写文件：先写临时文件再重命名。
// 避免写一半时进程崩溃留下损坏的 config.json（下次启动 Load 会失败）。
// 生活类比：写作业先打草稿（临时文件），确认无误再誊写到作业本（重命名），
// 即使中途被叫走，作业本也保持完好。
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, perm); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// intFieldRule 描述一个 int 字段的校验规则，由 repairInvalidFields 与 Validate 共享，
// 消除两份重复的字段表（QUAL-2）。
// 生活类比：像一份通用体检项目表，复查（修复）和初检（报错）都照着同一张表逐项核对。
type intFieldRule struct {
	name   string
	get    func(*Config) int
	min    int
	max    int
	set    func(*Config, int)
	errMsg string // fmt.Sprintf 模板，参数为当前值
}

// floatFieldRule 描述一个 float64 字段的校验规则，由 repairInvalidFields 与 Validate 共享。
type floatFieldRule struct {
	name   string
	get    func(*Config) float64
	min    float64
	max    float64
	set    func(*Config, float64)
	errMsg string
}

// intFieldRules 是所有 int 字段的校验规则表。
// 默认值不在表中硬编码，而是通过 get(DefaultConfig()) 动态获取，保证与 DefaultConfig 单一来源。
var intFieldRules = []intFieldRule{
	{"port", func(c *Config) int { return c.Port }, 1, 65535, func(c *Config, v int) { c.Port = v }, "invalid port: %d (must be 1-65535)"},
	// P4.1 修复：context_size 允许 0（表示"未设置"，走 smart-params 智能计算）。
	// 此前默认 8192 且 min=1，0 会被 Validate 拒绝；现在 0 是合法的"自动"值。
	{"context_size", func(c *Config) int { return c.ContextSize }, 0, 131072, func(c *Config, v int) { c.ContextSize = v }, "invalid context_size: %d (must be 0-131072, 0=auto)"},
	{"top_k", func(c *Config) int { return c.TopK }, 0, math.MaxInt32, func(c *Config, v int) { c.TopK = v }, "invalid top_k: %d (必须 >= 0)"},
	{"dry_allowed_length", func(c *Config) int { return c.DryAllowedLength }, 0, math.MaxInt32, func(c *Config, v int) { c.DryAllowedLength = v }, "invalid dry_allowed_length: %d (必须 >= 0)"},
	{"rag_top_k", func(c *Config) int { return c.RAGTopK }, 1, math.MaxInt32, func(c *Config, v int) { c.RAGTopK = v }, "invalid rag_top_k: %d (必须 > 0)"},
	{"threads", func(c *Config) int { return c.Threads }, 0, math.MaxInt32, func(c *Config, v int) { c.Threads = v }, "invalid threads: %d (threads 不能为负数)"},
	{"batch_size", func(c *Config) int { return c.BatchSize }, 0, math.MaxInt32, func(c *Config, v int) { c.BatchSize = v }, "invalid batch_size: %d (batch_size 不能为负数)"},
	{"gpu_layers", func(c *Config) int { return c.GPULayers }, 0, math.MaxInt32, func(c *Config, v int) { c.GPULayers = v }, "invalid gpu_layers: %d (gpu_layers 不能为负数)"},
	{"main_gpu", func(c *Config) int { return c.MainGPU }, -1, math.MaxInt32, func(c *Config, v int) { c.MainGPU = v }, "invalid main_gpu: %d (main_gpu 必须 >= -1)"},
	{"cache_ram", func(c *Config) int { return c.CacheRAM }, 0, math.MaxInt32, func(c *Config, v int) { c.CacheRAM = v }, "invalid cache_ram: %d (cache_ram 不能为负数)"},
	{"rerank_top_n", func(c *Config) int { return c.RerankTopN }, 1, math.MaxInt32, func(c *Config, v int) { c.RerankTopN = v }, "invalid rerank_top_n: %d (rerank_top_n 必须 > 0)"},
}

// floatFieldRules 是所有 float64 字段的校验规则表。
var floatFieldRules = []floatFieldRule{
	{"temperature", func(c *Config) float64 { return c.Temperature }, 0, 2, func(c *Config, v float64) { c.Temperature = v }, "invalid temperature: %.2f (must be 0-2)"},
	{"top_p", func(c *Config) float64 { return c.TopP }, 0, 1, func(c *Config, v float64) { c.TopP = v }, "invalid top_p: %.2f (must be 0-1)"},
	{"min_p", func(c *Config) float64 { return c.MinP }, 0, 1, func(c *Config, v float64) { c.MinP = v }, "invalid min_p: %.2f (必须为 0-1)"},
	{"repeat_penalty", func(c *Config) float64 { return c.RepeatPenalty }, 0, math.MaxFloat64, func(c *Config, v float64) { c.RepeatPenalty = v }, "invalid repeat_penalty: %.2f (must be >= 0)"},
	{"chat_background_opacity", func(c *Config) float64 { return c.ChatBackgroundOpacity }, 0, 1, func(c *Config, v float64) { c.ChatBackgroundOpacity = v }, "invalid chat_background_opacity: %.2f (must be 0-1)"},
	{"dry_multiplier", func(c *Config) float64 { return c.DryMultiplier }, 0, math.MaxFloat64, func(c *Config, v float64) { c.DryMultiplier = v }, "invalid dry_multiplier: %.2f (必须 >= 0)"},
	{"dry_base", func(c *Config) float64 { return c.DryBase }, 0, math.MaxFloat64, func(c *Config, v float64) { c.DryBase = v }, "invalid dry_base: %.2f (必须 >= 0)"},
	{"rag_min_score", func(c *Config) float64 { return c.RAGMinScore }, 0, 1, func(c *Config, v float64) { c.RAGMinScore = v }, "invalid rag_min_score: %.2f (必须为 0-1)"},
}

// repairInvalidFields 逐字段修复无效值为默认值，保留有效字段的用户设置
// 生活类比：像体检复查，只治疗异常指标，不把健康人送进ICU
// 返回已修复字段的描述列表，便于日志记录
func (c *Config) repairInvalidFields() []string {
	repaired := []string{}
	defaults := DefaultConfig()

	// int / float 字段范围检查与修复（与 Validate 共享 intFieldRules / floatFieldRules）
	for _, r := range intFieldRules {
		if val := r.get(c); val < r.min || val > r.max {
			defVal := r.get(defaults)
			repaired = append(repaired, fmt.Sprintf("%s: %d -> %d", r.name, val, defVal))
			r.set(c, defVal)
		}
	}
	for _, r := range floatFieldRules {
		if val := r.get(c); val < r.min || val > r.max {
			defVal := r.get(defaults)
			repaired = append(repaired, fmt.Sprintf("%s: %.2f -> %.2f", r.name, val, defVal))
			r.set(c, defVal)
		}
	}

	// 条件范围检查：仅当 > 0 时才校验 0.5-0.95
	if c.ProactiveCompressThreshold > 0 && (c.ProactiveCompressThreshold < 0.5 || c.ProactiveCompressThreshold > 0.95) {
		repaired = append(repaired, fmt.Sprintf("proactive_compress_threshold: %.2f -> %.2f", c.ProactiveCompressThreshold, defaults.ProactiveCompressThreshold))
		c.ProactiveCompressThreshold = defaults.ProactiveCompressThreshold
	}

	// 跨字段约束修复
	if c.RAGChunkSize > 0 && c.RAGChunkOverlap > 0 && c.RAGChunkOverlap >= c.RAGChunkSize {
		repaired = append(repaired, fmt.Sprintf("rag_chunk_overlap: %d -> %d", c.RAGChunkOverlap, defaults.RAGChunkOverlap))
		c.RAGChunkOverlap = defaults.RAGChunkOverlap
	}
	if c.ImageMinTokens > 0 && c.ImageMaxTokens > 0 && c.ImageMinTokens > c.ImageMaxTokens {
		repaired = append(repaired, fmt.Sprintf("image_min_tokens: %d -> %d", c.ImageMinTokens, defaults.ImageMinTokens))
		c.ImageMinTokens = defaults.ImageMinTokens
	}
	if (c.GrpAttnN == 0) != (c.GrpAttnW == 0) {
		repaired = append(repaired, fmt.Sprintf("grp_attn_n/w: %d/%d -> %d/%d", c.GrpAttnN, c.GrpAttnW, defaults.GrpAttnN, defaults.GrpAttnW))
		c.GrpAttnN = defaults.GrpAttnN
		c.GrpAttnW = defaults.GrpAttnW
	}
	if !isValidSplitMode(c.SplitMode) {
		repaired = append(repaired, fmt.Sprintf("split_mode: %q -> %q", c.SplitMode, defaults.SplitMode))
		c.SplitMode = defaults.SplitMode
	}
	if err := validateTensorSplit(c.TensorSplit); err != nil || (c.SplitMode == "none" && strings.TrimSpace(c.TensorSplit) != "") {
		repaired = append(repaired, fmt.Sprintf("tensor_split: %q -> %q", c.TensorSplit, defaults.TensorSplit))
		c.TensorSplit = defaults.TensorSplit
	}
	if c.BackendSampling && c.ReasoningBudget > 0 {
		repaired = append(repaired, fmt.Sprintf("reasoning_budget: %d -> %d (backend_sampling 互斥)", c.ReasoningBudget, defaults.ReasoningBudget))
		c.ReasoningBudget = defaults.ReasoningBudget
	}

	// 枚举检查
	switch c.SearchMode {
	case "off", "auto", "on":
	default:
		repaired = append(repaired, fmt.Sprintf("search_mode: %q -> %q", c.SearchMode, defaults.SearchMode))
		c.SearchMode = defaults.SearchMode
	}
	switch c.SystemPromptMode {
	case "append", "replace", "":
	default:
		repaired = append(repaired, fmt.Sprintf("system_prompt_mode: %q -> %q", c.SystemPromptMode, defaults.SystemPromptMode))
		c.SystemPromptMode = defaults.SystemPromptMode
	}
	// ReasoningEffort 枚举修复：空值保留（不传递跟随模型默认），非法值回退到空
	switch c.ReasoningEffort {
	case "", "low", "medium", "high", "max":
	default:
		repaired = append(repaired, fmt.Sprintf("reasoning_effort: %q -> %q", c.ReasoningEffort, defaults.ReasoningEffort))
		c.ReasoningEffort = defaults.ReasoningEffort
	}
	// EnableBuiltinTools 互斥：全量内置工具开关开启时，忽略细粒度 tools 字符串（全量已覆盖）
	if c.EnableBuiltinTools && strings.TrimSpace(c.Tools) != "" {
		repaired = append(repaired, fmt.Sprintf("tools: %q -> %q (enable_builtin_tools 开启，细粒度 tools 互斥)", c.Tools, defaults.Tools))
		c.Tools = defaults.Tools
	}
	// reasoning 枚举修复：自动思考已移除，auto/非法值归一化为 off（默认关闭）
	switch c.Reasoning {
	case "on", "off":
	default:
		repaired = append(repaired, fmt.Sprintf("reasoning: %q -> %q", c.Reasoning, "off"))
		c.Reasoning = "off"
	}
	return repaired
}

func (c *Config) Validate() error {
	// B-3.10: 表驱动化校验，减少重复 if 分支
	// 生活类比：像安检清单，逐项核对，不合格直接报错
	// 无上限的字段使用 math.MaxInt32 / math.MaxFloat64 作为哨兵值
	// int / float 字段范围检查与 repairInvalidFields 共享同一字段规则表（QUAL-2）
	for _, r := range intFieldRules {
		if val := r.get(c); val < r.min || val > r.max {
			return apperror.Newf(apperror.KindInvalidConfig, r.errMsg, val)
		}
	}
	for _, r := range floatFieldRules {
		if val := r.get(c); val < r.min || val > r.max {
			return apperror.Newf(apperror.KindInvalidConfig, r.errMsg, val)
		}
	}

	// 条件范围检查（有额外前置条件，不适合纯表驱动）
	// P1-A1: 主动压缩阈值，> 0 时才校验 0.5-0.95
	if c.ProactiveCompressThreshold > 0 && (c.ProactiveCompressThreshold < 0.5 || c.ProactiveCompressThreshold > 0.95) {
		return apperror.Newf(apperror.KindInvalidConfig, "invalid proactive_compress_threshold: %.2f (must be 0.5-0.95 or 0 for default)", c.ProactiveCompressThreshold)
	}

	// 依赖/互斥检查（跨字段约束）
	if c.RAGChunkSize > 0 && c.RAGChunkOverlap > 0 && c.RAGChunkOverlap >= c.RAGChunkSize {
		return apperror.Newf(apperror.KindInvalidConfig, "invalid rag_chunk_overlap: %d (必须小于 rag_chunk_size: %d)", c.RAGChunkOverlap, c.RAGChunkSize)
	}
	if c.ImageMinTokens > 0 && c.ImageMaxTokens > 0 && c.ImageMinTokens > c.ImageMaxTokens {
		return apperror.Newf(apperror.KindInvalidConfig, "invalid image_min_tokens: %d (image_min_tokens 不能大于 image_max_tokens: %d)", c.ImageMinTokens, c.ImageMaxTokens)
	}
	if (c.GrpAttnN == 0) != (c.GrpAttnW == 0) {
		return apperror.Newf(apperror.KindInvalidConfig, "invalid grp_attn_n/grp_attn_w: n=%d w=%d (grp_attn_n 和 grp_attn_w 必须同时非零或同时为零)", c.GrpAttnN, c.GrpAttnW)
	}
	if c.BackendSampling && c.ReasoningBudget > 0 {
		return apperror.Newf(apperror.KindInvalidConfig, "backend_sampling and reasoning_budget are mutually exclusive (backend_sampling=true requires reasoning_budget <= 0, got %d)", c.ReasoningBudget)
	}

	// 多 GPU 参数严格校验（repairInvalidFields 负责修复，Validate 负责报错）
	if !isValidSplitMode(c.SplitMode) {
		return apperror.Newf(apperror.KindInvalidConfig, "invalid split_mode: %q (必须是 none / layer / row / tensor，或留空使用 llama.cpp 默认)", c.SplitMode)
	}
	if err := validateTensorSplit(c.TensorSplit); err != nil {
		return apperror.Wrap(apperror.KindInvalidConfig, "invalid tensor_split", err)
	}
	if c.SplitMode == "none" && strings.TrimSpace(c.TensorSplit) != "" {
		return apperror.New(apperror.KindInvalidConfig, "tensor_split cannot be used with split_mode=none")
	}

	// 枚举检查
	switch c.SearchMode {
	case "off", "auto", "on":
	default:
		return apperror.Newf(apperror.KindInvalidConfig, "invalid search_mode: %q (必须是 off / auto / on)", c.SearchMode)
	}
	switch c.SystemPromptMode {
	case "append", "replace", "":
	default:
		return apperror.Newf(apperror.KindInvalidConfig, "invalid system_prompt_mode: %q (必须是 append / replace)", c.SystemPromptMode)
	}
	switch c.ProgrammingMode {
	case "auto", "on", "off", "":
	default:
		return apperror.Newf(apperror.KindInvalidConfig, "invalid programming_mode: %q (必须是 auto / on / off)", c.ProgrammingMode)
	}
	// reasoning 枚举校验：自动思考已移除，仅允许 on / off
	switch c.Reasoning {
	case "on", "off":
	default:
		return apperror.Newf(apperror.KindInvalidConfig, "invalid reasoning: %q (必须是 on / off，auto 已移除)", c.Reasoning)
	}
	// ReasoningEffort 枚举校验：空值合法（不传递跟随模型默认）
	switch c.ReasoningEffort {
	case "", "low", "medium", "high", "max":
	default:
		return apperror.Newf(apperror.KindInvalidConfig, "invalid reasoning_effort: %q (必须是 low / medium / high / max，或留空跟随模型默认)", c.ReasoningEffort)
	}
	// TTS 参数范围校验：超范围自动钳制不报错（用户友好）
	// 生活类比：收音机音量旋钮就算拧过头也不会爆炸，最多就是最大音量。
	if c.TtsRate < 0.5 {
		c.TtsRate = 0.5
	} else if c.TtsRate > 2.0 {
		c.TtsRate = 2.0
	}
	if c.TtsPitch < 0 {
		c.TtsPitch = 0
	} else if c.TtsPitch > 2 {
		c.TtsPitch = 2
	}
	if c.TtsVolume < 0 {
		c.TtsVolume = 0
	} else if c.TtsVolume > 1 {
		c.TtsVolume = 1
	}

	// 后端类型校验：不合法时不返回错误，而是回退到 "auto" 并记录警告日志。
	// 生活类比：发动机型号填错了不报错，直接换成"自动"模式，保证车还能开。
	// 注意：这里只校验字符串合法性，不解析 auto（auto 的具体含义由启动流程根据硬件推断）。
	// 本地校验避免 config 包依赖 llm 包（保持 config 为基础包，无内部依赖）。
	if !isValidBackendType(c.BackendType) {
		log.Warn().Str("backend_type", c.BackendType).Msg("[config] 无效的后端类型，回退到 auto")
		c.BackendType = "auto"
	}

	// APIBase 格式校验：必须是合法的 HTTP/HTTPS URL 且包含端口
	// 生活类比：快递单上的地址必须有省市区（协议）和门牌号（端口），
	// 缺了任何一个快递员都送不到
	if err := validateAPIBase(c.APIBase); err != nil {
		return err
	}

	return nil
}

// validateAPIBase 校验 API 基础地址格式。
// 要求：以 http:// 或 https:// 开头，且 URL 中包含端口号。
// 空字符串不报错（由调用方保证默认值），但非空必须合法。
func validateAPIBase(apiBase string) error {
	if apiBase == "" {
		return nil
	}
	u, err := url.Parse(apiBase)
	if err != nil {
		return apperror.Newf(apperror.KindInvalidConfig, "invalid api_base: %q (URL 解析失败)", apiBase)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return apperror.Newf(apperror.KindInvalidConfig, "invalid api_base: %q (必须以 http:// 或 https:// 开头)", apiBase)
	}
	if u.Hostname() == "" {
		return apperror.Newf(apperror.KindInvalidConfig, "invalid api_base: %q (缺少主机地址)", apiBase)
	}
	// 必须包含端口：u.Port() 返回空字符串表示未指定端口
	if u.Port() == "" {
		return apperror.Newf(apperror.KindInvalidConfig, "invalid api_base: %q (必须包含端口号，如 http://127.0.0.1:8080)", apiBase)
	}
	return nil
}

// isValidBackendType 校验后端类型字符串合法性。
//
// 与 llm.IsValidBackendType 保持同步：合法值为 auto/cuda/hip/sycl/vulkan/openvino/cpu。
// 本地副本避免 config 包依赖 llm 包（config 应为最基础包，无内部依赖）。
//
// 同步提醒：新增后端类型时，需在此 switch 中添加对应字面量，
// 同时在 llm/backend.go 中添加 BackendType 常量。
func isValidBackendType(s string) bool {
	switch s {
	case "auto", "cuda", "hip", "sycl", "vulkan", "openvino", "cpu":
		return true
	}
	return false
}

func isValidSplitMode(mode string) bool {
	switch mode {
	case "", "none", "layer", "row", "tensor":
		return true
	}
	return false
}

// validateTensorSplit validates llama.cpp's comma-separated non-negative
// allocation weights. The values are proportions, not percentages; e.g. 3,1
// assigns 75% / 25%. Whitespace is accepted for hand-edited config files.
func validateTensorSplit(split string) error {
	split = strings.TrimSpace(split)
	if split == "" {
		return nil
	}
	parts := strings.Split(split, ",")
	if len(parts) < 2 {
		return apperror.New(apperror.KindInvalidConfig, "must contain at least two comma-separated device weights")
	}
	hasPositive := false
	for _, part := range parts {
		value, err := strconv.ParseFloat(strings.TrimSpace(part), 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
			return apperror.Newf(apperror.KindInvalidConfig, "%q is not a non-negative finite number", part)
		}
		if value > 0 {
			hasPositive = true
		}
	}
	if !hasPositive {
		return apperror.New(apperror.KindInvalidConfig, "at least one device weight must be greater than zero")
	}
	return nil
}
