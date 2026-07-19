// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/pathutil"
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
	ctx         context.Context
	config      *config.Config
	configMu    sync.RWMutex
	server      *llm.Server
	serverMu    sync.RWMutex
	client      *llm.Client
	db          *sql.DB
	service     *chat.Service
	hwInfo      *system.HardwareInfo
	ready       atomic.Bool
	serverReady atomic.Bool
	watchCancel context.CancelFunc
	// rootCtx 是应用级上下文，生命周期贯穿整个 App 运行期。
	// shutdownInternal 会调用 rootCancel 通知所有被跟踪的长生命周期 goroutine 退出。
	rootCtx    context.Context
	rootCancel context.CancelFunc
	// g 用于跟踪长生命周期 goroutine（watcher/health/SSE 等），
	// shutdownInternal 在关闭底层资源前会 g.Wait() 等待它们退出，避免资源释放后仍被访问。
	g sync.WaitGroup
	// logChan 用于 SetOnLog 回调的日志推送：生产者（llama-server 输出）非阻塞写入，
	// 消费者（trackedGo 启动的单个 goroutine）读取并 EventsEmit 到前端。
	// 替代原来"每行日志一个 goroutine"的实现，避免 goroutine 泛滥。
	logChan          chan string
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
	serverLoadFailed atomic.Bool // 模型加载彻底失败后锁定状态，防止监控循环覆盖错误状态
	lastServerError  string      // 最后一次服务器/模型加载错误信息
	lastServerErrMu  sync.RWMutex
}

func NewApp() *App {
	return &App{}
}

// trackedGo 启动一个被 App 跟踪的长生命周期 goroutine。
//
// 生活类比：就像给每个员工（goroutine）发一个工牌，App（公司）在下班（shutdown）时
// 先广播"下班了"（rootCancel），然后门口签到表（WaitGroup）等所有员工出来后再锁门
// （关闭 db/ragVS 等资源），避免把人锁在里面（资源释放后仍被访问导致 panic）。
//
// 自动加 recover 防 panic 崩溃整个进程；通过 sync.WaitGroup 跟踪生命周期，
// shutdownInternal 会在关闭底层资源前 g.Wait() 等待所有被跟踪 goroutine 退出。
// 短期一次性 goroutine 不必走此路径，直接 go func() 即可（建议自行加 recover）。
func (a *App) trackedGo(fn func()) {
	a.g.Go(func() {
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[goroutine] tracked goroutine panic recovered")
			}
		}()
		fn()
	})
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
	for range 3 {
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
	// 只要 config.json 能正常 JSON 解析就信任，不基于字段内容判断有效性。
	// 之前基于 model_path 空值判断"默认配置"的逻辑已移除（DefaultConfig 的 ModelPath 本就是空字符串）。
	isValidConfig := func(d string) bool {
		cfgPath := filepath.Join(d, "config.json")
		data, err := os.ReadFile(cfgPath)
		if err != nil {
			return false
		}
		// 解析为通用 map，验证是否为有效 JSON
		var raw map[string]any
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
		// 只要 config.json 能正常解析就信任，不基于 model_path 判断
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
	// 安全实践：复用 pathutil.ResolveInBase 统一路径遍历防护，避免多处实现不一致（见安全审查 #20）
	baseDir := appDir()
	resolved := pathutil.ResolveInBase(baseDir, p)
	if resolved == "" {
		return ""
	}
	// ResolveInBase 已处理绝对路径和遍历校验，这里补充 Stat 检查文件是否存在
	if _, err := os.Stat(resolved); err == nil {
		return resolved
	}
	// 文件不存在时仍基于 appDir() 返回，让调用方得到清晰的"文件不存在"错误
	return resolved
}

// SmartParamsInfo 返回当前模型+硬件的智能参数推荐值和模型元数据
type SmartParamsInfo struct {
	Hardware struct {
		CPUCores       int    `json:"cpu_cores"`
		HasGPU         bool   `json:"has_gpu"`
		HasCUDABackend bool   `json:"has_cuda_backend"`
		GPUName        string `json:"gpu_name"`
		GPUVRAMMB      int64  `json:"gpu_vram_mb"`
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
		FType           string `json:"ftype"`
	} `json:"model"`

	// SpecAdvice 推测解码智能提醒（nil 表示无需提醒）。
	// 前端通过 info.spec_advice 是否为 null 判断是否需要弹通知/显示静态提示。
	SpecAdvice *SpecAdviceInfo `json:"spec_advice"`

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

// SpecAdviceInfo 推测解码智能提醒信息（JSON 序列化后传给前端）。
//
// 豆芽检测到当前模型支持 Eagle3 推测解码但用户未配置 draft 模型时，
// 生成此提醒，前端在设置界面静态显示 + 模型加载后弹通知，
// 引导用户前往 hf-mirror.com 下载对应的 sidecar 模型。
//
// 生活类比：像手机检测到连接了慢充充电器时弹出的「建议使用原装快充充电器」通知，
// 包含充电器型号（Desc）和购买链接（DownloadURL），但不会强制你购买。
type SpecAdviceInfo struct {
	// Sidecar 推测解码类型："eagle3" 或 "dflash"
	Sidecar string `json:"sidecar"`
	// Desc 人类可读名称："Eagle3" 或 "DFlash"
	Desc string `json:"desc"`
	// DownloadURL hf-mirror.com 下载链接（仓库页或搜索页）
	DownloadURL string `json:"download_url"`
	// Reason 触发建议的原因（用于前端展示）
	Reason string `json:"reason"`
}
