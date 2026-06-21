// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

type Config struct {
	ModelPath         string `json:"model_path"`
	MmprojAuto        bool   `json:"mmproj_auto"`
	MmprojOffload     bool   `json:"mmproj_offload"`
	LlamaServerPath   string `json:"llama_server_path"`
	APIBase           string `json:"api_base"`
	Port              int    `json:"port"`
	ContextSize       int    `json:"context_size"`
	Temperature       float64 `json:"temperature"`
	TopP              float64 `json:"top_p"`
	TopK              int    `json:"top_k"`
	RepeatPenalty     float64 `json:"repeat_penalty"`
	KVUnified         bool   `json:"kv_unified"`
	CacheIdleSlots    bool   `json:"cache_idle_slots"`
	CacheRAM          int    `json:"cache_ram"`
	ImageMinTokens    int    `json:"image_min_tokens"`
	ImageMaxTokens    int    `json:"image_max_tokens"`
	FitTarget         int    `json:"fit_target"`
	FitCtx            int    `json:"fit_ctx"`
	Reasoning         string `json:"reasoning"`
	ReasoningBudget   int    `json:"reasoning_budget"`
	ReasoningFormat   string `json:"reasoning_format"`
	SystemPrompt      string `json:"system_prompt"`
	SystemPromptMode  string `json:"system_prompt_mode"` // "append" (追加) or "replace" (替换), 默认 "append"
	ChatBackground        string  `json:"chat_background"`
	ChatBackgroundOpacity float64 `json:"chat_background_opacity"`
	UserAvatar        string `json:"user_avatar"`
	AiAvatar          string `json:"ai_avatar"`
	SearchMode        string `json:"search_mode"` // "off", "auto", "on"
	// Deprecated: 已迁移到 Reasoning 字段，保留仅为向后兼容
	ThinkingEnabled     bool   `json:"thinking_enabled"`
	// Deprecated: 已迁移到 Reasoning 字段，保留仅为向后兼容
	ThinkingSoftSwitch  string `json:"thinking_soft_switch"`
	SleepIdleSeconds  int    `json:"sleep_idle_seconds"`
	ModelsMax         int    `json:"models_max"`
	RAGEnabled        bool   `json:"rag_enabled"`
	RAGActiveKB       string `json:"rag_active_kb"`
	RAGTopK           int    `json:"rag_top_k"`
	RAGMinScore       float64 `json:"rag_min_score"`
	RAGChunkSize      int     `json:"rag_chunk_size"`
	RAGChunkOverlap   int     `json:"rag_chunk_overlap"`
	EmbeddingModel    string  `json:"embedding_model"` // 专用嵌入模型路径（可选，为空则用聊天模型）
	ReasoningBudgetMessage string `json:"reasoning_budget_message"`
	Mmap              bool    `json:"mmap"`
	KVOffload         bool    `json:"kv_offload"`
	ContextShift      bool    `json:"context_shift"`
	MinP              float64 `json:"min_p"`
	DryMultiplier     float64 `json:"dry_multiplier"`
	DryBase           float64 `json:"dry_base"`
	DryAllowedLength  int     `json:"dry_allowed_length"`
	Device            string  `json:"device"`
	Parallel          int     `json:"parallel"`
	CacheTypeK        string  `json:"cache_type_k"`
	CacheTypeV        string  `json:"cache_type_v"`
	SpecType          string `json:"spec_type"`
	SpecDraftNMax     int    `json:"spec_draft_n_max"`
	SpecDraftNMin     int    `json:"spec_draft_n_min"`
	CacheTypeKDraft   string `json:"cache_type_k_draft"`
	CacheTypeVDraft   string `json:"cache_type_v_draft"`
	SpecNgramModNMin   int    `json:"spec_ngram_mod_n_min"`
	SpecNgramModNMax   int    `json:"spec_ngram_mod_n_max"`
	SpecNgramModNMatch int    `json:"spec_ngram_mod_n_match"`
	SpecNgramSimpleSizeN   int    `json:"spec_ngram_simple_size_n"`
	SpecNgramSimpleSizeM   int    `json:"spec_ngram_simple_size_m"`
	SpecNgramSimpleMinHits int    `json:"spec_ngram_simple_min_hits"`
	SpecNgramMapKSizeN     int    `json:"spec_ngram_map_k_size_n"`
	SpecNgramMapKSizeM     int    `json:"spec_ngram_map_k_size_m"`
	SpecNgramMapKMinHits   int    `json:"spec_ngram_map_k_min_hits"`
	SpecNgramMapK4VSizeN   int    `json:"spec_ngram_map_k4v_size_n"`
	SpecNgramMapK4VSizeM   int    `json:"spec_ngram_map_k4v_size_m"`
	SpecNgramMapK4VMinHits int    `json:"spec_ngram_map_k4v_min_hits"`
	LookupCacheStatic  string `json:"lookup_cache_static"`
	LookupCacheDynamic string `json:"lookup_cache_dynamic"`
	SpecDraftModel     string `json:"spec_draft_model"`
	ServerAPIKeyEnabled bool  `json:"server_api_key_enabled"`
	ExposeServer       bool  `json:"expose_server"` // 暴露服务器地址，允许局域网访问
	SwaFull              bool    `json:"swa_full"`
	CtxCheckpoints       int     `json:"ctx_checkpoints"`
	CheckpointMinStep    int     `json:"checkpoint_min_step"`
	Tools                string  `json:"tools"`
	PrefillAssistant     bool    `json:"prefill_assistant"`
	SlotPromptSimilarity float64 `json:"slot_prompt_similarity"`
	SkipChatParsing      bool    `json:"skip_chat_parsing"`
	APIPrefix            string  `json:"api_prefix"`
	SimpleIO             bool    `json:"simple_io"`
	GPULayers            int     `json:"gpu_layers"`  // 0=自动（99全部卸载），正数=指定层数
	FlashAttn            *bool   `json:"flash_attn"`  // nil=自动，指针类型区分"未设置"和"false"
	Mlock                *bool   `json:"mlock"`       // nil=自动
	Threads              int     `json:"threads"`    // 0=自动
	BatchSize            int     `json:"batch_size"`  // 0=自动
	CloseAction          string  `json:"close_action"` // "ask"(默认), "tray"(最小化到托盘), "exit"(直接退出)
	// RAG 重排序配置
	RerankerModelPath string `json:"reranker_model_path"` // reranker 模型路径（可选，为空则不启用重排序）
	RerankTopN        int    `json:"rerank_top_n"`         // 重排序后返回的 top-N 结果数（默认5）
	// KV 缓存持久化配置
	SlotSavePath    string `json:"slot_save_path"`     // KV 缓存保存路径（为空则使用默认路径 appDir/slots/）
	SlotSaveEnabled bool   `json:"slot_save_enabled"`  // 是否启用 KV 缓存持久化
	// Draft 模型 GPU 配置（Eagle3 等需要独立 draft 模型的场景）
	SpecDraftNgl    int    `json:"spec_draft_ngl"`     // draft 模型 GPU 层数（0=不传递）
	SpecDraftDevice string `json:"spec_draft_device"`  // draft 模型设备（如 "cuda:0"，为空则不传递）
	// Agent 模式与 MCP CORS 代理（llama.cpp 新特性）
	Agent      bool `json:"agent"`        // 一键启用 CORS 代理 + 所有内置工具
	UIMcpProxy bool `json:"ui_mcp_proxy"` // 仅启用 MCP CORS 代理（Agent 已包含此项）
	// LoRA 适配器路径（逗号分隔，启动时通过 --lora 加载，配合 --lora-init-without-apply 默认不应用）
	LoraPaths string `json:"lora_paths"`
}

func DefaultConfig() *Config {
	return &Config{
		ModelPath:         "",
		MmprojAuto:        true,
		MmprojOffload:     true,
		LlamaServerPath:  "runtime/llama-server.exe",
		APIBase:           "http://127.0.0.1:8080",
		Port:              8080,
		ContextSize:       8192,
		Temperature:       0.6,
		TopP:              0.95,
		TopK:              20,
		RepeatPenalty:     1,
		KVUnified:         false,
		CacheIdleSlots:    false,
		CacheRAM:          0,
		ImageMinTokens:    0,
		ImageMaxTokens:    0,
		FitTarget:         0,
		FitCtx:            0,
		Reasoning:         "off",
		ReasoningBudget:   0,
		ReasoningFormat:     "",
		SystemPrompt:      "",
		SystemPromptMode:  "append", // 默认使用追加模式
		ChatBackground:        "",
		ChatBackgroundOpacity: 0.8,
		UserAvatar:        "",
		AiAvatar:          "",
		SearchMode:        "off",
		ThinkingEnabled:     true,
		ThinkingSoftSwitch:  "auto",
		SleepIdleSeconds: 120,
		ModelsMax:         1,
		RAGEnabled:        false,
		RAGActiveKB:      "default",
		RAGTopK:          3,
		RAGMinScore:      0.3,
		RAGChunkSize:     512,
		RAGChunkOverlap:  64,
		ReasoningBudgetMessage: "",
		Mmap:             true,
		KVOffload:        true,
		ContextShift:     false,
		MinP:             0.05,
		DryMultiplier:    0,
		DryBase:          1.75,
		DryAllowedLength: 2,
		Device:           "",
		Parallel:         0,
		CacheTypeK:       "",
		CacheTypeV:       "",
		SpecType:         "",
		SpecDraftNMax:    0,
		SpecDraftNMin:    0,
		CacheTypeKDraft:  "",
		CacheTypeVDraft:  "",
		ServerAPIKeyEnabled: true,
		ExposeServer:       false,
		SwaFull:              false,
		CtxCheckpoints:       0,
		CheckpointMinStep:    0,
		Tools:                "",
		PrefillAssistant:     true,
		SlotPromptSimilarity: 0.0,
		SkipChatParsing:      false,
		APIPrefix:            "",
		SimpleIO:             false,
		GPULayers:            0,
		FlashAttn:            nil,
		Mlock:                nil,
		Threads:              0,
		BatchSize:            0,
		CloseAction:          "ask",
		RerankerModelPath:    "",
		RerankTopN:           5,
		SlotSavePath:         "",
		SlotSaveEnabled:      false,
		SpecDraftNgl:         0,
		SpecDraftDevice:      "",
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			cfg := DefaultConfig()
			if saveErr := Save(path, cfg); saveErr != nil {
				return nil, fmt.Errorf("创建默认配置文件失败: %w", saveErr)
			}
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		if strings.HasPrefix(strings.TrimSpace(string(data)), "\"") {
			var inner string
			if unquoteErr := json.Unmarshal(data, &inner); unquoteErr == nil {
				if innerErr := json.Unmarshal([]byte(inner), cfg); innerErr == nil {
					cfg.migrateLegacyThinking([]byte(inner))
					_ = Save(path, cfg)
					// 校验配置，若失败则回退到默认配置
					if validateErr := cfg.Validate(); validateErr != nil {
						log.Printf("警告: 配置校验失败: %v，回退到默认配置", validateErr)
						return DefaultConfig(), nil
					}
					return cfg, nil
				}
			}
		}
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	cfg.migrateLegacyThinking(data)

	// 校验配置，若失败则回退到默认配置
	if validateErr := cfg.Validate(); validateErr != nil {
		log.Printf("警告: 配置校验失败: %v，回退到默认配置", validateErr)
		return DefaultConfig(), nil
	}
	return cfg, nil
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

func LoadRaw(path string) (map[string]interface{}, error) {
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
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	return raw, nil
}

func Save(path string, cfg *Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (c *Config) Validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d (must be 1-65535)", c.Port)
	}
	if c.ContextSize < 1 || c.ContextSize > 131072 {
		return fmt.Errorf("invalid context_size: %d (must be 1-131072)", c.ContextSize)
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		return fmt.Errorf("invalid temperature: %.2f (must be 0-2)", c.Temperature)
	}
	if c.TopP < 0 || c.TopP > 1 {
		return fmt.Errorf("invalid top_p: %.2f (must be 0-1)", c.TopP)
	}
	if c.TopK < 0 {
		return fmt.Errorf("invalid top_k: %d (必须 >= 0)", c.TopK)
	}
	if c.MinP < 0 || c.MinP > 1 {
		return fmt.Errorf("invalid min_p: %.2f (必须为 0-1)", c.MinP)
	}
	if c.RepeatPenalty < 0 {
		return fmt.Errorf("invalid repeat_penalty: %.2f (must be >= 0)", c.RepeatPenalty)
	}
	if c.ChatBackgroundOpacity < 0 || c.ChatBackgroundOpacity > 1 {
		return fmt.Errorf("invalid chat_background_opacity: %.2f (must be 0-1)", c.ChatBackgroundOpacity)
	}
	if c.DryMultiplier < 0 {
		return fmt.Errorf("invalid dry_multiplier: %.2f (必须 >= 0)", c.DryMultiplier)
	}
	if c.DryBase < 0 {
		return fmt.Errorf("invalid dry_base: %.2f (必须 >= 0)", c.DryBase)
	}
	if c.DryAllowedLength < 0 {
		return fmt.Errorf("invalid dry_allowed_length: %d (必须 >= 0)", c.DryAllowedLength)
	}
	if c.RAGTopK <= 0 {
		return fmt.Errorf("invalid rag_top_k: %d (必须 > 0)", c.RAGTopK)
	}
	if c.RAGMinScore < 0 || c.RAGMinScore > 1 {
		return fmt.Errorf("invalid rag_min_score: %.2f (必须为 0-1)", c.RAGMinScore)
	}
	// 当分块大小和重叠大小都 > 0 时，重叠大小必须小于分块大小
	if c.RAGChunkSize > 0 && c.RAGChunkOverlap > 0 && c.RAGChunkOverlap >= c.RAGChunkSize {
		return fmt.Errorf("invalid rag_chunk_overlap: %d (必须小于 rag_chunk_size: %d)", c.RAGChunkOverlap, c.RAGChunkSize)
	}
	// SearchMode 必须是 off / auto / on 之一
	switch c.SearchMode {
	case "off", "auto", "on":
	default:
		return fmt.Errorf("invalid search_mode: %q (必须是 off / auto / on)", c.SearchMode)
	}
	// SystemPromptMode 必须是 append / replace / 空字符串（视为 append）之一
	switch c.SystemPromptMode {
	case "append", "replace", "":
	default:
		return fmt.Errorf("invalid system_prompt_mode: %q (必须是 append / replace)", c.SystemPromptMode)
	}
	return nil
}
