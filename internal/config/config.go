// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config

import (
	"encoding/json"
	"fmt"
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
	ThinkingEnabled     bool   `json:"thinking_enabled"`
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
					_ = Save(path, cfg)
					return cfg, nil
				}
			}
		}
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return cfg, nil
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
	if c.ContextSize <= 0 {
		return fmt.Errorf("invalid context_size: %d (must be > 0)", c.ContextSize)
	}
	if c.Temperature < 0 || c.Temperature > 2 {
		return fmt.Errorf("invalid temperature: %.2f (must be 0-2)", c.Temperature)
	}
	if c.TopP < 0 || c.TopP > 1 {
		return fmt.Errorf("invalid top_p: %.2f (must be 0-1)", c.TopP)
	}
	if c.RepeatPenalty < 0 {
		return fmt.Errorf("invalid repeat_penalty: %.2f (must be >= 0)", c.RepeatPenalty)
	}
	if c.ChatBackgroundOpacity < 0 || c.ChatBackgroundOpacity > 1 {
		return fmt.Errorf("invalid chat_background_opacity: %.2f (must be 0-1)", c.ChatBackgroundOpacity)
	}
	return nil
}
