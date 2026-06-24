// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/rag"
	"douya/internal/system"

	zlog "github.com/rs/zerolog/log"
)

type SwitchResult struct {
	Success         bool                   `json:"success"`
	Error           string                 `json:"error,omitempty"`
	CurrentModel    string                 `json:"current_model,omitempty"`
	Capabilities    *llm.ModelCapabilities `json:"capabilities,omitempty"`
	PreviousModel   string                 `json:"previous_model,omitempty"`
	RolledBack      bool                   `json:"rolled_back,omitempty"`
	RollbackSuccess bool                   `json:"rollback_success,omitempty"`
}

// SearchAPIKeys 用于前端展示搜索 API Key 的设置状态，不暴露实际密钥值
type SearchAPIKeys struct {
	OllamaAPIKey    string `json:"ollama_api_key"`
	TavilyAPIKey    string `json:"tavily_api_key"`
	OllamaAPIKeySet bool   `json:"ollama_api_key_set"`
	TavilyAPIKeySet bool   `json:"tavily_api_key_set"`
}

type App struct {
	ctx              context.Context
	config           *config.Config
	configMu         sync.RWMutex
	server           *llm.Server
	serverMu         sync.Mutex
	client           *llm.Client
	db               *sql.DB
	service          *chat.Service
	hwInfo           *system.HardwareInfo
	ready            atomic.Bool
	serverReady      atomic.Bool
	watchCancel      context.CancelFunc
	stopOnce         sync.Once
	cleanupResult    []*chat.AbnormalConversation
	cleanupResultMu  sync.Mutex
	presets          []llm.ModelPreset
	presetRelPaths   map[string]string
	presetsMu        sync.RWMutex
	currentModelMu   sync.RWMutex
	currentModelName string
	isSwitching      atomic.Bool
	switchingTo      string
	switchingToMu    sync.RWMutex
	ragVS            *rag.VectorStore
	ragDS            *rag.DocumentStore
	ragEmbedder      *rag.ClientEmbedder
	encKey           []byte
	hidden           atomic.Bool
	exiting          atomic.Bool
}

func NewApp() *App {
	return &App{}
}

var cachedAppDir string

func appDir() string {
	if cachedAppDir != "" {
		return cachedAppDir
	}

	exePath, err := os.Executable()
	if err != nil {
		zlog.Error().Err(err).Msg("[appDir] 获取可执行文件路径失败")
		cachedAppDir = "."
		return cachedAppDir
	}
	exeDir := filepath.Dir(exePath)

	// 查找应用根目录的优先级：
	// 1. 可执行文件同目录（便携模式 / 开发模式）
	// 2. 可执行文件的上层目录（发布构建：exe 在 bin/ 下，资源在上层）
	// 3. 用户数据目录（标准安装模式，如 %APPDATA%/douya）
	searchDirs := []string{exeDir}

	// 向上查找最多 3 层（覆盖 release/bin/ → release/ 这类结构）
	dir := exeDir
	for i := 0; i < 3; i++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		searchDirs = append(searchDirs, parent)
		dir = parent
	}

	// 用户数据目录
	if userDir, err := os.UserConfigDir(); err == nil {
		searchDirs = append(searchDirs, filepath.Join(userDir, "douya"))
	}

	// 优先查找 config.json。
	// 当 config.json 存在时，额外检查它是否值得信任：
	// - 如果 model_path 为空，且上层目录存在资源（runtime/ 或 models/），
	//   说明该 config.json 是上版本自动生成的默认配置，应该跳过。
	isValidConfig := func(d string) bool {
		cfgPath := filepath.Join(d, "config.json")
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			return false
		}
		// 解析为通用 map 检测 model_path 字段
		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			// 尝试双重序列化容错
			if len(data) > 0 && data[0] == '"' {
				var inner string
				if err := json.Unmarshal(data, &inner); err == nil {
					if err := json.Unmarshal([]byte(inner), &raw); err != nil {
						return true // 无法解析内容时保守信任
					}
				} else {
					return true
				}
			} else {
				return true // 解析失败时保守信任
			}
		}
		// model_path 为空字符串说明是默认配置
		if mp, ok := raw["model_path"].(string); ok && mp == "" {
			// 检查上层目录（filepath.Dir(d)）是否有资源
			parent := filepath.Dir(d)
			for _, p := range []string{"runtime", "models"} {
				if info, err := os.Stat(filepath.Join(parent, p)); err == nil && info.IsDir() {
					return false // 是默认配置 + 上层有资源 = 跳过
				}
			}
		}
		return true
	}

	for _, d := range searchDirs {
		cfgPath := filepath.Join(d, "config.json")
		if _, err := os.Stat(cfgPath); err == nil {
			if !isValidConfig(d) {
				zlog.Info().Str("dir", d).Msg("[appDir] 跳过自动生成的默认配置")
				continue
			}
			zlog.Info().Str("dir", d).Msg("[appDir] 找到配置文件目录")
			cachedAppDir = d
			return cachedAppDir
		}
	}

	// 没有找到 config.json，尝试通过资源目录定位应用根目录
	for _, d := range searchDirs {
		if info, err := os.Stat(filepath.Join(d, "models")); err == nil && info.IsDir() {
			zlog.Info().Str("dir", d).Msg("[appDir] 通过资源目录定位到应用根目录")
			cachedAppDir = d
			// 在找到的根目录创建默认配置文件
			cfgPath := filepath.Join(d, "config.json")
			if err := config.Save(cfgPath, config.DefaultConfig()); err != nil {
				zlog.Error().Err(err).Msg("[appDir] 创建默认配置失败")
			}
			return cachedAppDir
		}
	}

	// 均未找到，在可执行文件目录创建默认配置
	zlog.Info().Str("dir", exeDir).Msg("[appDir] 未找到配置文件或资源目录，在可执行文件目录创建默认配置")
	defaultCfg := config.DefaultConfig()
	cfgPath := filepath.Join(exeDir, "config.json")
	if err := config.Save(cfgPath, defaultCfg); err != nil {
		zlog.Error().Err(err).Msg("[appDir] 创建默认配置失败")
	}
	cachedAppDir = exeDir
	return cachedAppDir
}

func resolvePath(p string) string {
	// 清理路径，防止路径遍历
	p = filepath.Clean(p)
	if filepath.IsAbs(p) {
		return p
	}
	baseDir := appDir()
	candidate := filepath.Join(baseDir, p)
	// 验证结果路径仍在基准目录内
	absCandidate, err := filepath.Abs(candidate)
	if err == nil && !strings.HasPrefix(absCandidate, baseDir) {
		zlog.Warn().Str("path", p).Str("baseDir", baseDir).Msg("[resolvePath] path traversal detected")
		return filepath.Join(baseDir, filepath.Base(p))
	}
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	// 文件不存在时仍基于 appDir() 返回，让调用方得到清晰的"文件不存在"错误
	return filepath.Join(baseDir, p)
}

// SmartParamsInfo 返回当前模型+硬件的智能参数推荐值和模型元数据
type SmartParamsInfo struct {
	Hardware struct {
		CPUCores  int    `json:"cpu_cores"`
		HasGPU    bool   `json:"has_gpu"`
		GPUName   string `json:"gpu_name"`
		GPUVRAMMB int64  `json:"gpu_vram_mb"`
	} `json:"hardware"`

	Model struct {
		Architecture    string `json:"architecture"`
		BlockCount      int    `json:"block_count"`
		EmbeddingLength int    `json:"embedding_length"`
		ContextLength   int    `json:"context_length"`
		FileSizeMB      int64  `json:"file_size_mb"`
		ExpertCount     int    `json:"expert_count"`
		ExpertUsed      int    `json:"expert_used"`
		HasMTP          bool   `json:"has_mtp"`
		HasReasoning    bool   `json:"has_reasoning"`
		NParams         int64  `json:"n_params"`
		SizeLabel       string `json:"size_label"`
	} `json:"model"`

	Params struct {
		GPULayers      int    `json:"gpu_layers"`
		Threads        int    `json:"threads"`
		BatchSize      int    `json:"batch_size"`
		UBatchSize     int    `json:"ubatch_size"`
		FlashAttn      bool   `json:"flash_attn"`
		CacheTypeK     string `json:"cache_type_k"`
		CacheTypeV     string `json:"cache_type_v"`
		Mlock          bool   `json:"mlock"`
		MmprojOffload  bool   `json:"mmproj_offload"`
		ContextSize    int    `json:"context_size"`
		SpecType       string `json:"spec_type"`
		SpecDraftNMax  int    `json:"spec_draft_n_max"`
		SpecDraftNMin  int    `json:"spec_draft_n_min"`
		NgramModNMin   int    `json:"ngram_mod_n_min"`
		NgramModNMax   int    `json:"ngram_mod_n_max"`
		NgramModNMatch int    `json:"ngram_mod_n_match"`
	} `json:"params"`

	Overrides struct {
		GPULayers   bool `json:"gpu_layers"`
		FlashAttn   bool `json:"flash_attn"`
		Mlock       bool `json:"mlock"`
		Threads     bool `json:"threads"`
		BatchSize   bool `json:"batch_size"`
		ContextSize bool `json:"context_size"`
		CacheTypeK  bool `json:"cache_type_k"`
		CacheTypeV  bool `json:"cache_type_v"`
		SpecType    bool `json:"spec_type"`
	} `json:"overrides"`
}
