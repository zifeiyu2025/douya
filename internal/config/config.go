// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config

import (
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"os"
)

type SearchEngines struct {
	OllamaAPIKey string `json:"ollama_api_key"`
	TavilyAPIKey string `json:"tavily_api_key"`
	GitHubAPIKey string `json:"github_api_key"`
}

type Config struct {
	ModelPath        string        `json:"model_path"`
	MmprojAuto       bool          `json:"mmproj_auto"`
	MmprojOffload    bool          `json:"mmproj_offload"`
	LlamaServerPath  string        `json:"llama_server_path"`
	APIBase          string        `json:"api_base"`
	Port             int           `json:"port"`
	ContextSize      int           `json:"context_size"`
	Temperature      float64       `json:"temperature"`
	TopP             float64       `json:"top_p"`
	TopK             int           `json:"top_k"`
	RepeatPenalty    float64       `json:"repeat_penalty"`
	KVUnified        bool          `json:"kv_unified"`
	CacheIdleSlots   bool          `json:"cache_idle_slots"`
	CacheRAM         int           `json:"cache_ram"`
	ImageMinTokens   int           `json:"image_min_tokens"`
	ImageMaxTokens   int           `json:"image_max_tokens"`
	FitTarget        int           `json:"fit_target"`
	FitCtx           int           `json:"fit_ctx"`
	Reasoning        string        `json:"reasoning"`
	ReasoningBudget  int           `json:"reasoning_budget"`
	ReasoningFormat  string        `json:"reasoning_format"`
	SystemPrompt     string        `json:"system_prompt"`
	ChatBackground   string        `json:"chat_background"`
	UserAvatar       string        `json:"user_avatar"`
	AiAvatar         string        `json:"ai_avatar"`
	SearchEngines    SearchEngines `json:"search_engines"`
	SearchEnabled    bool          `json:"search_enabled"`
	SleepIdleSeconds int           `json:"sleep_idle_seconds"`
	ModelsMax        int           `json:"models_max"`
	RAGEnabled       bool          `json:"rag_enabled"`
	RAGActiveKB      string        `json:"rag_active_kb"`
	RAGTopK          int           `json:"rag_top_k"`
	RAGMinScore      float64       `json:"rag_min_score"`
	RAGChunkSize     int           `json:"rag_chunk_size"`
	RAGChunkOverlap  int           `json:"rag_chunk_overlap"`
}

func DefaultConfig() *Config {
	return &Config{
		ModelPath:        "models/Gemma-4-E4B-U-Q4_K_M/Gemma-4-E4B-U-Q4_K_M.gguf",
		ReasoningFormat:  "",
		MmprojAuto:       true,
		MmprojOffload:    true,
		LlamaServerPath:  "engines/llama-server.exe",
		APIBase:          "http://127.0.0.1:8080",
		Port:             8080,
		ContextSize:      8192,
		Temperature:      0.8,
		TopP:             0.95,
		TopK:             20,
		RepeatPenalty:    1.0,
		Reasoning:        "auto",
		SystemPrompt:     "",
		SearchEngines:    SearchEngines{},
		SearchEnabled:    false,
		SleepIdleSeconds: 120,
		ModelsMax:        1,
		RAGEnabled:       false,
		RAGActiveKB:      "default",
		RAGTopK:          3,
		RAGMinScore:      0.3,
		RAGChunkSize:     512,
		RAGChunkOverlap:  64,
	}
}

func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Info().Str("path", path).Msg("[config] config file not found, using defaults")
		} else {
			log.Error().Err(err).Msg("[config] failed to read config")
		}
	} else {
		if err := json.Unmarshal(data, cfg); err != nil {
			return nil, err
		}
		log.Info().Str("path", path).Msg("[config] loaded config")
	}

	if apiKey := os.Getenv("OLLAMA_API_KEY"); apiKey != "" {
		cfg.SearchEngines.OllamaAPIKey = apiKey
		log.Info().Msg("[config] using OLLAMA_API_KEY from environment")
	}
	if apiKey := os.Getenv("TAVILY_API_KEY"); apiKey != "" {
		cfg.SearchEngines.TavilyAPIKey = apiKey
		log.Info().Msg("[config] using TAVILY_API_KEY from environment")
	}
	if apiKey := os.Getenv("GITHUB_API_KEY"); apiKey != "" {
		cfg.SearchEngines.GitHubAPIKey = apiKey
		log.Info().Msg("[config] using GITHUB_API_KEY from environment")
	}

	return cfg, nil
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
	return nil
}
