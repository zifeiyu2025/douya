// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"

	"douya/internal/apperror"

	"github.com/rs/zerolog/log"
)

type Config struct {
	// Version 配置 schema 版本号，用于版本化迁移。
	// 0 表示旧版本（无此字段的历史配置），加载时按迁移链升级到当前版本。
	Version         int    `json:"version"`
	ModelPath       string `json:"model_path"`
	MmprojAuto      bool   `json:"mmproj_auto"`
	MmprojOffload   bool   `json:"mmproj_offload"`
	LlamaServerPath string `json:"llama_server_path"`
	APIBase         string `json:"api_base"`
	Port            int    `json:"port"`
	ContextSize     int    `json:"context_size"`
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
	// 推理内容保留开关（nil=不传递，true=--reasoning-preserve，false=--no-reasoning-preserve）
	ReasoningPreserve     *bool   `json:"reasoning_preserve"`
	SystemPrompt          string  `json:"system_prompt"`
	SystemPromptMode      string  `json:"system_prompt_mode"` // "append" (追加) or "replace" (替换), 默认 "append"
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
	SpecAdviceEnabled       bool    `json:"spec_advice_enabled"`       // 推测解码智能提醒开关（检测到模型支持但未配置 draft 时提醒用户）
	Device                  string  `json:"device"`
	Parallel                int     `json:"parallel"`
	CacheTypeK              string  `json:"cache_type_k"`
	CacheTypeV              string  `json:"cache_type_v"`
	SpecType                string  `json:"spec_type"`
	SpecDraftNMax           int     `json:"spec_draft_n_max"`
	SpecDraftNMin           int     `json:"spec_draft_n_min"`
	CacheTypeKDraft         string  `json:"cache_type_k_draft"`
	CacheTypeVDraft         string  `json:"cache_type_v_draft"`
	SpecNgramModNMin        int     `json:"spec_ngram_mod_n_min"`
	SpecNgramModNMax        int     `json:"spec_ngram_mod_n_max"`
	SpecNgramModNMatch      int     `json:"spec_ngram_mod_n_match"`
	SpecNgramSimpleSizeN    int     `json:"spec_ngram_simple_size_n"`
	SpecNgramSimpleSizeM    int     `json:"spec_ngram_simple_size_m"`
	SpecNgramSimpleMinHits  int     `json:"spec_ngram_simple_min_hits"`
	SpecNgramMapKSizeN      int     `json:"spec_ngram_map_k_size_n"`
	SpecNgramMapKSizeM      int     `json:"spec_ngram_map_k_size_m"`
	SpecNgramMapKMinHits    int     `json:"spec_ngram_map_k_min_hits"`
	SpecNgramMapK4VSizeN    int     `json:"spec_ngram_map_k4v_size_n"`
	SpecNgramMapK4VSizeM    int     `json:"spec_ngram_map_k4v_size_m"`
	SpecNgramMapK4VMinHits  int     `json:"spec_ngram_map_k4v_min_hits"`
	LookupCacheStatic       string  `json:"lookup_cache_static"`
	LookupCacheDynamic      string  `json:"lookup_cache_dynamic"`
	SpecDraftModel          string  `json:"spec_draft_model"`
	ServerAPIKeyEnabled     bool    `json:"server_api_key_enabled"`
	ExposeServer            bool    `json:"expose_server"` // 暴露服务器地址，允许局域网访问
	EnableWebUI             bool    `json:"enable_web_ui"` // 启用 llama-server 自带的原生 Web UI（默认关闭）
	SwaFull                 bool    `json:"swa_full"`
	CtxCheckpoints          int     `json:"ctx_checkpoints"`
	CheckpointMinStep       int     `json:"checkpoint_min_step"`
	Tools                   string  `json:"tools"`
	PrefillAssistant        bool    `json:"prefill_assistant"`
	SlotPromptSimilarity    float64 `json:"slot_prompt_similarity"`
	SkipChatParsing         bool    `json:"skip_chat_parsing"`
	APIPrefix               string  `json:"api_prefix"`
	SimpleIO                bool    `json:"simple_io"`
	GPULayers               int     `json:"gpu_layers"`   // 0=自动（99全部卸载），正数=指定层数
	FlashAttn               *bool   `json:"flash_attn"`   // nil=自动，指针类型区分"未设置"和"false"
	Mlock                   *bool   `json:"mlock"`        // nil=自动
	Threads                 int     `json:"threads"`      // 0=自动
	ThreadsHTTP             int     `json:"threads_http"` // HTTP 请求处理线程数（0=自动，llama.cpp 默认）
	BatchSize               int     `json:"batch_size"`   // 0=自动
	CloseAction             string  `json:"close_action"` // "ask"(默认), "tray"(最小化到托盘), "exit"(直接退出)
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
	// 后端采样（实验性，将采样逻辑移到 GPU 执行，不兼容 grammar 和 reasoning budget）
	BackendSampling bool `json:"backend_sampling"`
	// SSE ping 间隔秒数（0=使用服务器默认 30 秒，用于保持长连接活跃）
	SsePingInterval int `json:"sse_ping_interval"`
	// LoRA 适配器路径（逗号分隔，启动时通过 --lora 加载，配合 --lora-init-without-apply 默认不应用）
	LoraPaths string `json:"lora_paths"`
	// 直接 I/O（绕过操作系统页面缓存，加速大模型加载，避免内存污染）
	DirectIO bool `json:"direct_io"`
	// MoE 权重 CPU 卸载（将所有专家权重保留在 CPU，显存不足时启用）
	CPUMoe bool `json:"cpu_moe"`
	// 前 N 层 MoE 权重 CPU 卸载（0=不启用，精细控制 --cpu-moe 的影响范围）
	NCpuMoe int `json:"n_cpu_moe"`
	// 算子卸载开关（nil=使用默认值 true，true=--op-offload，false=--no-op-offload）
	OpOffload *bool `json:"op_offload"`
}

func DefaultConfig() *Config {
	return &Config{
		Version:                    1, // 当前配置 schema 版本号
		ModelPath:                  "",
		MmprojAuto:                 true,
		MmprojOffload:              true,
		LlamaServerPath:            "runtime/llama-server.exe",
		APIBase:                    "http://127.0.0.1:8080",
		Port:                       8080,
		ContextSize:                8192,
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
		SystemPrompt:               "",
		SystemPromptMode:           "append", // 默认使用追加模式
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
		KVOffload:                  true,
		// 默认启用 context-shift 作为兜底：应用层压缩失败时由 llama-server 自动移位，
		// 避免请求直接报错。--keep 512 保护 system prompt 不被移位
		ContextShift:             true,
		MinP:                     0.05,
		DryMultiplier:            0,
		DryBase:                  1.75,
		DryAllowedLength:         2,
		DrySequenceBreaker:       "",
		DryPenaltyLastN:          0,
		GrpAttnN:                 0,
		GrpAttnW:                 0,
		Jinja:                    nil,
		CachePrompt:              new(true), // 显式启用 prompt 缓存，多轮对话时复用前缀 KV，降低首 token 延迟
		Metrics:                  false,
		Verbose:                  false,
		SpecDraftThreads:         0,
		SpecDraftThreadsBatch:    0,
		SpecDefault:              false,
		SpecAdviceEnabled:        true, // 默认开启：检测到模型支持推测解码但未配置 draft 时提醒用户
		Device:                   "",
		Parallel:                 0,
		CacheTypeK:               "",
		CacheTypeV:               "",
		SpecType:                 "",
		SpecDraftNMax:            0,
		SpecDraftNMin:            0,
		CacheTypeKDraft:          "",
		CacheTypeVDraft:          "",
		ServerAPIKeyEnabled:      true,
		ExposeServer:             false,
		EnableWebUI:              false,
		SwaFull:                  false,
		CtxCheckpoints:           32,  // 与 llama.cpp 默认值对齐，长上下文检查点回滚
		CheckpointMinStep:        256, // 与 llama.cpp 默认值对齐，检查点最小步长
		Tools:                    "",
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
		SlotSaveEnabled:          false,
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
		SsePingInterval: 0,
		LoraPaths:       "",
		DirectIO:        false,
		CPUMoe:          false,
		NCpuMoe:         0,
		OpOffload:       nil,
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
				return nil, fmt.Errorf("创建默认配置文件失败: %w", saveErr)
			}
			return cfg, nil
		}
		log.Error().Err(err).Str("path", path).Msg("[config] 读取配置文件失败")
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		if strings.HasPrefix(strings.TrimSpace(string(data)), "\"") {
			var inner string
			if unquoteErr := json.Unmarshal(data, &inner); unquoteErr == nil {
				if innerErr := json.Unmarshal([]byte(inner), cfg); innerErr == nil {
					cfg.migrate([]byte(inner))
					_ = Save(path, cfg)
					// 校验配置，若失败则回退到默认配置并写盘，避免每次启动都告警
					if validateErr := cfg.Validate(); validateErr != nil {
						log.Warn().Err(validateErr).Msg("[config] 配置校验失败，回退到默认配置并写盘")
						fallback := DefaultConfig()
						_ = Save(path, fallback)
						return fallback, nil
					}
					return cfg, nil
				}
			}
		}
		log.Error().Err(err).Msg("[config] 解析配置文件失败")
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
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
			_ = Save(path, fallback)
			return fallback, nil
		}
		// 修复后校验通过，保存修复后的配置
		_ = Save(path, cfg)
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
			_ = os.WriteFile(path, merged, 0o644)
		}
	}
}

// currentConfigVersion 当前配置 schema 版本号，与 DefaultConfig 中的 Version 保持一致。
// 每次新增 schema 变更时递增此常量，并在 migrate 中追加对应迁移分支。
const currentConfigVersion = 1

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
	// v1 -> v2：默认启用 context-shift 作为上下文溢出兜底 + 启用 cache-reuse 加速
	// 老版本默认 false/0 且会被写入 config.json，此处一次性迁移为推荐值，
	// 让老用户也能享受到应用层压缩失败时的自动兜底和 KV 缓存复用加速。
	// 用户如需关闭，可在设置中手动切换（下次启动 Version 已是 2，不会再迁移）
	if c.Version < 2 {
		c.ContextShift = true
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
	default: // "auto" 或空
		c.Reasoning = "auto"
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

func Save(path string, cfg *Config) error {
	log.Debug().Str("path", path).Msg("[config] 保存配置文件")
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		log.Error().Err(err).Msg("[config] 序列化配置失败")
		return err
	}
	// 注：配置文件不收紧 ACL（icacls），本地单用户应用收益有限且可能导致运行时权限问题。
	// 敏感数据（API Key）已用 AES-GCM 加密存储。见安全审查 #6（已评估，风险可接受）。
	if err := os.WriteFile(path, data, 0o644); err != nil {
		log.Error().Err(err).Str("path", path).Msg("[config] 写入配置文件失败")
		return err
	}
	return nil
}

// repairInvalidFields 逐字段修复无效值为默认值，保留有效字段的用户设置
// 生活类比：像体检复查，只治疗异常指标，不把健康人送进ICU
// 返回已修复字段的描述列表，便于日志记录
func (c *Config) repairInvalidFields() []string {
	repaired := []string{}
	defaults := DefaultConfig()

	// int 字段范围检查与修复
	intFields := []struct {
		name    string
		val     int
		min     int
		max     int
		defVal  int
		setFunc func(int)
	}{
		{"port", c.Port, 1, 65535, defaults.Port, func(v int) { c.Port = v }},
		{"context_size", c.ContextSize, 1, 131072, defaults.ContextSize, func(v int) { c.ContextSize = v }},
		{"top_k", c.TopK, 0, math.MaxInt32, defaults.TopK, func(v int) { c.TopK = v }},
		{"dry_allowed_length", c.DryAllowedLength, 0, math.MaxInt32, defaults.DryAllowedLength, func(v int) { c.DryAllowedLength = v }},
		{"rag_top_k", c.RAGTopK, 1, math.MaxInt32, defaults.RAGTopK, func(v int) { c.RAGTopK = v }},
		{"threads", c.Threads, 0, math.MaxInt32, defaults.Threads, func(v int) { c.Threads = v }},
		{"batch_size", c.BatchSize, 0, math.MaxInt32, defaults.BatchSize, func(v int) { c.BatchSize = v }},
		{"gpu_layers", c.GPULayers, 0, math.MaxInt32, defaults.GPULayers, func(v int) { c.GPULayers = v }},
		{"cache_ram", c.CacheRAM, 0, math.MaxInt32, defaults.CacheRAM, func(v int) { c.CacheRAM = v }},
		{"rerank_top_n", c.RerankTopN, 1, math.MaxInt32, defaults.RerankTopN, func(v int) { c.RerankTopN = v }},
	}
	for _, f := range intFields {
		if f.val < f.min || f.val > f.max {
			repaired = append(repaired, fmt.Sprintf("%s: %d -> %d", f.name, f.val, f.defVal))
			f.setFunc(f.defVal)
		}
	}

	// float64 字段范围检查与修复
	floatFields := []struct {
		name    string
		val     float64
		min     float64
		max     float64
		defVal  float64
		setFunc func(float64)
	}{
		{"temperature", c.Temperature, 0, 2, defaults.Temperature, func(v float64) { c.Temperature = v }},
		{"top_p", c.TopP, 0, 1, defaults.TopP, func(v float64) { c.TopP = v }},
		{"min_p", c.MinP, 0, 1, defaults.MinP, func(v float64) { c.MinP = v }},
		{"repeat_penalty", c.RepeatPenalty, 0, math.MaxFloat64, defaults.RepeatPenalty, func(v float64) { c.RepeatPenalty = v }},
		{"chat_background_opacity", c.ChatBackgroundOpacity, 0, 1, defaults.ChatBackgroundOpacity, func(v float64) { c.ChatBackgroundOpacity = v }},
		{"dry_multiplier", c.DryMultiplier, 0, math.MaxFloat64, defaults.DryMultiplier, func(v float64) { c.DryMultiplier = v }},
		{"dry_base", c.DryBase, 0, math.MaxFloat64, defaults.DryBase, func(v float64) { c.DryBase = v }},
		{"rag_min_score", c.RAGMinScore, 0, 1, defaults.RAGMinScore, func(v float64) { c.RAGMinScore = v }},
	}
	for _, f := range floatFields {
		if f.val < f.min || f.val > f.max {
			repaired = append(repaired, fmt.Sprintf("%s: %.2f -> %.2f", f.name, f.val, f.defVal))
			f.setFunc(f.defVal)
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

	return repaired
}

func (c *Config) Validate() error {
	// B-3.10: 表驱动化校验，减少重复 if 分支
	// 生活类比：像安检清单，逐项核对，不合格直接报错
	// 无上限的字段使用 math.MaxInt32 / math.MaxFloat64 作为哨兵值

	// int 字段范围检查
	intChecks := []struct {
		val    int
		min    int
		max    int
		errMsg string // fmt.Sprintf 模板，参数为 val
	}{
		{c.Port, 1, 65535, "invalid port: %d (must be 1-65535)"},
		{c.ContextSize, 1, 131072, "invalid context_size: %d (must be 1-131072)"},
		{c.TopK, 0, math.MaxInt32, "invalid top_k: %d (必须 >= 0)"},
		{c.DryAllowedLength, 0, math.MaxInt32, "invalid dry_allowed_length: %d (必须 >= 0)"},
		{c.RAGTopK, 1, math.MaxInt32, "invalid rag_top_k: %d (必须 > 0)"},
		{c.Threads, 0, math.MaxInt32, "invalid threads: %d (threads 不能为负数)"},
		{c.BatchSize, 0, math.MaxInt32, "invalid batch_size: %d (batch_size 不能为负数)"},
		{c.GPULayers, 0, math.MaxInt32, "invalid gpu_layers: %d (gpu_layers 不能为负数)"},
		{c.CacheRAM, 0, math.MaxInt32, "invalid cache_ram: %d (cache_ram 不能为负数)"},
		{c.RerankTopN, 1, math.MaxInt32, "invalid rerank_top_n: %d (rerank_top_n 必须 > 0)"},
	}
	for _, chk := range intChecks {
		if chk.val < chk.min || chk.val > chk.max {
			return apperror.Newf(apperror.KindInvalidConfig, chk.errMsg, chk.val)
		}
	}

	// float64 字段范围检查
	floatChecks := []struct {
		val    float64
		min    float64
		max    float64
		errMsg string
	}{
		{c.Temperature, 0, 2, "invalid temperature: %.2f (must be 0-2)"},
		{c.TopP, 0, 1, "invalid top_p: %.2f (must be 0-1)"},
		{c.MinP, 0, 1, "invalid min_p: %.2f (必须为 0-1)"},
		{c.RepeatPenalty, 0, math.MaxFloat64, "invalid repeat_penalty: %.2f (must be >= 0)"},
		{c.ChatBackgroundOpacity, 0, 1, "invalid chat_background_opacity: %.2f (must be 0-1)"},
		{c.DryMultiplier, 0, math.MaxFloat64, "invalid dry_multiplier: %.2f (必须 >= 0)"},
		{c.DryBase, 0, math.MaxFloat64, "invalid dry_base: %.2f (必须 >= 0)"},
		{c.RAGMinScore, 0, 1, "invalid rag_min_score: %.2f (必须为 0-1)"},
	}
	for _, chk := range floatChecks {
		if chk.val < chk.min || chk.val > chk.max {
			return apperror.Newf(apperror.KindInvalidConfig, chk.errMsg, chk.val)
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
	return nil
}
