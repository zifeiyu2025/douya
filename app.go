// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/rag"
	"douya/internal/search"
	"douya/internal/secrets"
	"douya/internal/store"
	"douya/internal/system"

	"fyne.io/systray"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
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

type SearchAPIKeys struct {
	OllamaAPIKey string `json:"ollama_api_key"`
	TavilyAPIKey string `json:"tavily_api_key"`
	GitHubAPIKey string `json:"github_api_key"`
}

type App struct {
	ctx              context.Context
	config           *config.Config
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
	// 文件不存在时仍基于 appDir() 返回，让调用方得到清晰的“文件不存在”错误
	return filepath.Join(baseDir, p)
}

func (a *App) buildServerConfig() *llm.ServerConfig {
	absServerPath := resolvePath(a.config.LlamaServerPath)
	modelsDir := filepath.Join(appDir(), "models")

	sp := system.CalculateSmartParams(a.hwInfo, resolvePath(a.config.ModelPath))
	zlog.Info().Str("models_dir", modelsDir).Int("gpu_layers", sp.GPULayers).Int("threads", sp.Threads).Bool("flash", sp.FlashAttn).Str("cache_k", sp.CacheTypeK).Str("cache_v", sp.CacheTypeV).Bool("mlock", sp.Mlock).Bool("mmproj_offload", sp.MmprojOffload).Msg("[smart-params] params")

	// reasoning_format 不再硬编码设置：
	// llama-server 默认值 COMMON_REASONING_FORMAT_DEEPSEEK 已能正确处理所有模型的思考内容分离
	// （包括 DeepSeek-R1 的 </think>` 标签、Gemma 4 的 <|channel>thought 标签、Qwen3 的思考标签等）
	// 仅在用户手动配置时才传值
	reasoningFormat := a.config.ReasoningFormat

	presetPath := filepath.Join(appDir(), "router-preset.ini")
	if _, err := os.Stat(presetPath); err != nil {
		presetPath = ""
	}

	gpuLayers := "auto"
	if sp.GPULayers > 0 {
		gpuLayers = fmt.Sprintf("%d", sp.GPULayers)
	}

	sleepIdle := a.config.SleepIdleSeconds
	if sleepIdle <= 0 {
		sleepIdle = 120
	}
	modelsMax := a.config.ModelsMax
	if modelsMax <= 0 {
		modelsMax = 1
	}

	serverCfg := &llm.ServerConfig{
		ModelsDir:              modelsDir,
		ServerPath:             absServerPath,
		Port:                   a.config.Port,
		GPULayers:              gpuLayers,
		Threads:                sp.Threads,
		FlashAttn:              sp.FlashAttn,
		CacheTypeK:             sp.CacheTypeK,
		CacheTypeV:             sp.CacheTypeV,
		Mlock:                  sp.Mlock,
		MmprojAuto:             a.config.MmprojAuto,
		MmprojOffload:          sp.MmprojOffload,
		Repack:                 true,
		OpOffload:              true,
		KVUnified:              a.config.KVUnified,
		CacheIdleSlots:         a.config.CacheIdleSlots,
		CacheRAM:               a.config.CacheRAM,
		ImageMinTokens:         a.config.ImageMinTokens,
		ImageMaxTokens:         a.config.ImageMaxTokens,
		FitTarget:              a.config.FitTarget,
		FitCtx:                 a.config.FitCtx,
		Reasoning:              a.config.Reasoning,
		ReasoningBudget:        a.config.ReasoningBudget,
		ReasoningFormat:        reasoningFormat,
		ReasoningBudgetMessage: a.config.ReasoningBudgetMessage,
		APIBase:                a.config.APIBase,
		AppDir:                 appDir(),
		ModelsPreset:           presetPath,
		ModelsMax:              modelsMax,
		SleepIdleSeconds:       sleepIdle,
		Mmap:                   a.config.Mmap,
		KVOffload:              a.config.KVOffload,
		ContextShift:           a.config.ContextShift,
		MinP:                   a.config.MinP,
		DryMultiplier:          a.config.DryMultiplier,
		DryBase:                a.config.DryBase,
		DryAllowedLength:       a.config.DryAllowedLength,
		Device:                 a.config.Device,
		Parallel:               a.config.Parallel,
		SpecType:               a.config.SpecType,
		SpecDraftNMax:          a.config.SpecDraftNMax,
		SpecDraftNMin:          a.config.SpecDraftNMin,
		CacheTypeKDraft:        a.config.CacheTypeKDraft,
		CacheTypeVDraft:        a.config.CacheTypeVDraft,
		SSEPingInterval:        0,
	}

	if a.config.CacheTypeK != "" {
		serverCfg.CacheTypeK = a.config.CacheTypeK
	}
	if a.config.CacheTypeV != "" {
		serverCfg.CacheTypeV = a.config.CacheTypeV
	}
	if a.config.CacheTypeKDraft != "" {
		serverCfg.CacheTypeKDraft = a.config.CacheTypeKDraft
	}
	if a.config.CacheTypeVDraft != "" {
		serverCfg.CacheTypeVDraft = a.config.CacheTypeVDraft
	}
	if a.config.SpecType == "" && sp.SpecType != "" {
		serverCfg.SpecType = sp.SpecType
		serverCfg.SpecDraftNMax = sp.SpecDraftNMax
		serverCfg.SpecDraftNMin = sp.SpecDraftNMin
		if serverCfg.CacheTypeKDraft == "" {
			serverCfg.CacheTypeKDraft = sp.CacheTypeKDraft
		}
		if serverCfg.CacheTypeVDraft == "" {
			serverCfg.CacheTypeVDraft = sp.CacheTypeVDraft
		}
	}

	if a.db != nil {
		if a.encKey != nil {
			if key, err := store.GetEncryptedSetting(a.db, "server_api_key", a.encKey); err == nil && key != "" {
				serverCfg.APIKey = key
			}
		} else {
			if key, err := store.GetSetting(a.db, "server_api_key"); err == nil && key != "" {
				serverCfg.APIKey = key
			}
		}
	}

	return serverCfg
}

func (a *App) startServerAndWatch(srv *llm.Server, ctx context.Context) {
	if err := srv.Start(); err != nil {
		zlog.Error().Err(err).Msg("start llama-server failed")
		runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
			Running: false,
			Error:   fmt.Sprintf("启动 llama-server 失败: %v", err),
		})
		return
	}

	if err := srv.WaitForReady(60e9); err != nil {
		zlog.Error().Err(err).Msg("wait for server ready failed")
		runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
			Running: false,
			Error:   fmt.Sprintf("llama-server 未就绪: %v", err),
		})
		return
	}

	a.presetsMu.RLock()
	presetsSnapshot := make([]llm.ModelPreset, len(a.presets))
	copy(presetsSnapshot, a.presets)
	a.presetsMu.RUnlock()

	foundDefault := false
	for _, p := range presetsSnapshot {
		if p.Alias == "default" {
			a.currentModelMu.Lock()
			a.currentModelName = p.Name
			a.currentModelMu.Unlock()
			foundDefault = true
			break
		}
	}
	if !foundDefault && len(presetsSnapshot) > 0 {
		a.currentModelMu.Lock()
		a.currentModelName = presetsSnapshot[0].Name
		a.currentModelMu.Unlock()
		a.currentModelMu.RLock()
		zlog.Info().Str("model", a.currentModelName).Msg("[server] no default preset found, using first model")
		a.currentModelMu.RUnlock()
	}

	a.currentModelMu.RLock()
	modelForDetect := a.currentModelName
	a.currentModelMu.RUnlock()
	if err := a.service.DetectModelArchitectureForModel(modelForDetect); err != nil {
		zlog.Error().Err(err).Msg("detect model architecture failed")
	}

	// 启动后自动加载默认模型
	if modelForDetect != "" && a.client != nil {
		zlog.Info().Str("model", modelForDetect).Msg("[server] auto-loading default model")
		runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
			Running:     false,
			Switching:   true,
			SwitchingTo: modelForDetect,
		})
		if err := a.client.LoadModel(ctx, modelForDetect); err != nil {
			if isAlreadyRunningError(err) {
				// 模型已在运行（llama-server 启动时自动加载了默认模型），视为成功
				zlog.Info().Str("model", modelForDetect).Msg("[server] default model is already running")
				a.serverReady.Store(true)
				a.emitSwitchSuccess(modelForDetect)
			} else {
				zlog.Error().Err(err).Str("model", modelForDetect).Msg("[server] auto-load default model failed")
				runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
					Running: false,
					Error:   fmt.Sprintf("默认模型加载失败: %v（可手动切换模型）", err),
				})
			}
		} else {
			if err := a.client.WaitForModelLoaded(ctx, modelForDetect, 120*time.Second); err != nil {
				zlog.Error().Err(err).Str("model", modelForDetect).Msg("[server] auto-load default model wait failed")
				runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
					Running: false,
					Error:   fmt.Sprintf("默认模型加载超时: %v（可手动切换模型）", err),
				})
			} else {
				zlog.Info().Str("model", modelForDetect).Msg("[server] default model loaded and ready")
				a.serverReady.Store(true)
				a.emitSwitchSuccess(modelForDetect)
			}
		}
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	a.serverMu.Lock()
	a.watchCancel = watchCancel
	a.serverMu.Unlock()
	go srv.WatchWithCallback(watchCtx, func(status llm.ServerStatus) {
		if a.isSwitching.Load() {
			return
		}
		a.currentModelMu.RLock()
		curModel := a.currentModelName
		a.currentModelMu.RUnlock()
		if status.Running {
			a.serverReady.Store(true)
			caps := a.service.GetModelCapabilities()
			status.Capabilities = &caps
			status.CurrentModel = curModel
		} else {
			a.serverReady.Store(false)
		}
		runtime.EventsEmit(ctx, "server:status", status)
	}, func() {
		a.currentModelMu.RLock()
		modelForDetect2 := a.currentModelName
		a.currentModelMu.RUnlock()
		if err := a.service.DetectModelArchitectureForModel(modelForDetect2); err != nil {
			zlog.Error().Err(err).Msg("detect model architecture after restart failed")
		}
		// 重启后重新加载当前模型，加载完成后才设置 serverReady
		if modelForDetect2 != "" && a.client != nil {
			zlog.Info().Str("model", modelForDetect2).Msg("[server] reloading model after restart")
			if err := a.client.LoadModel(ctx, modelForDetect2); err != nil {
				if isAlreadyRunningError(err) {
					zlog.Info().Str("model", modelForDetect2).Msg("[server] model is already running after restart")
					a.serverReady.Store(true)
					runtime.EventsEmit(ctx, "server:status", a.runningStatus())
				} else {
					zlog.Error().Err(err).Str("model", modelForDetect2).Msg("[server] reload model after restart failed")
					runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
						Running: false,
						Error:   fmt.Sprintf("重启后模型加载失败: %v", err),
					})
				}
			} else if err := a.client.WaitForModelLoaded(ctx, modelForDetect2, 120*time.Second); err != nil {
				zlog.Error().Err(err).Str("model", modelForDetect2).Msg("[server] reload model wait after restart failed")
				runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
					Running: false,
					Error:   fmt.Sprintf("重启后模型加载超时: %v", err),
				})
			} else {
				zlog.Info().Str("model", modelForDetect2).Msg("[server] model reloaded and ready after restart")
				a.serverReady.Store(true)
				runtime.EventsEmit(ctx, "server:status", a.runningStatus())
			}
		} else {
			a.serverReady.Store(true)
		}
	})
}

func (a *App) validatePaths() []string {
	var missing []string
	baseDir := appDir()

	serverPath := resolvePath(a.config.LlamaServerPath)
	if _, err := os.Stat(serverPath); err != nil {
		missing = append(missing, fmt.Sprintf("引擎程序: %s", serverPath))
	}

	modelsDir := filepath.Join(baseDir, "models")
	if info, err := os.Stat(modelsDir); err != nil || !info.IsDir() {
		missing = append(missing, fmt.Sprintf("模型目录: %s", modelsDir))
	}

	runtimeDir := filepath.Join(baseDir, "runtime")
	if info, err := os.Stat(runtimeDir); err != nil || !info.IsDir() {
		missing = append(missing, fmt.Sprintf("运行时目录: %s", runtimeDir))
	}

	return missing
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	llm.KillOrphanLlamaServers()

	a.hwInfo = system.DetectHardware()

	var err error

	cfgPath := filepath.Join(appDir(), "config.json")
	a.config, err = config.Load(cfgPath)
	if err != nil {
		zlog.Error().Err(err).Msg("load config failed")
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "配置加载失败",
			Message: fmt.Sprintf("加载配置文件失败: %v", err),
		})
		return
	}

	if missingPaths := a.validatePaths(); len(missingPaths) > 0 {
		msg := "以下关键文件或目录缺失：\n\n"
		for _, p := range missingPaths {
			msg += "❌ " + p + "\n"
		}
		msg += fmt.Sprintf("\n应用根目录: %s\n请确保所有文件位于正确位置。", appDir())
		zlog.Error().Interface("paths", missingPaths).Msg("[startup] missing paths")
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "关键文件缺失",
			Message: msg,
		})
	}

	dbPath := filepath.Join(appDir(), "data", "douya.db")

	// 加载加密密钥，用于对话内容和 API Key 等敏感数据的加密存储
	keyPath := filepath.Join(appDir(), "data", ".enc_key")
	a.encKey, err = secrets.LoadOrCreateKey(keyPath)
	if err != nil {
		zlog.Error().Err(err).Msg("[startup] load encryption key failed")
		// 加密密钥加载失败不阻止启动，但敏感数据将以明文存储
	}

	a.db, err = store.Init(dbPath, a.encKey)
	if err != nil {
		zlog.Error().Err(err).Msg("init database failed")
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "数据库初始化失败",
			Message: fmt.Sprintf("初始化数据库失败: %v", err),
		})
		return
	}

	if raw, rawErr := config.LoadRaw(cfgPath); rawErr == nil {
		if se, ok := raw["search_engines"]; ok {
			if seMap, ok := se.(map[string]interface{}); ok {
				migrated := false
				setFn := func(key, value string) error {
					if a.encKey != nil {
						return store.SetEncryptedSetting(a.db, key, value, a.encKey)
					}
					return store.SetSetting(a.db, key, value)
				}
				getFn := func(key string) string {
					if a.encKey != nil {
						v, _ := store.GetEncryptedSetting(a.db, key, a.encKey)
						return v
					}
					v, _ := store.GetSetting(a.db, key)
					return v
				}
				if v, ok := seMap["ollama_api_key"]; ok && v != "" {
					if existing := getFn("search_ollama_api_key"); existing == "" {
						setFn("search_ollama_api_key", fmt.Sprintf("%v", v))
						migrated = true
					}
				}
				if v, ok := seMap["tavily_api_key"]; ok && v != "" {
					if existing := getFn("search_tavily_api_key"); existing == "" {
						setFn("search_tavily_api_key", fmt.Sprintf("%v", v))
						migrated = true
					}
				}
				if v, ok := seMap["github_api_key"]; ok && v != "" {
					if existing := getFn("search_github_api_key"); existing == "" {
						setFn("search_github_api_key", fmt.Sprintf("%v", v))
						migrated = true
					}
				}
				if migrated {
					zlog.Info().Msg("[startup] migrated search API keys from config.json to database")
					config.Save(cfgPath, a.config)
				}
			}
		}
	}

	if err := a.generatePresetFile(); err != nil {
		zlog.Error().Err(err).Msg("[startup] generate preset file failed")
	}

	a.client = llm.NewClient(a.config.APIBase, a.getServerAPIKey())

	searchChain := a.buildSearchChain()

	a.service = chat.NewService(a.client, searchChain, a.db, a.config, a.encKey, appDir())
	a.service.SetContext(ctx)

	// Initialize RAG (Badger-backed vector store + LLM embedder)
	ragDir := filepath.Join(appDir(), "data", "rag")
	ragVS, err := rag.NewVectorStore(ragDir, rag.DefaultHNSWConfig())
	if err != nil {
		zlog.Error().Err(err).Msg("[startup] RAG vector store init failed (RAG disabled)")
	} else {
		a.ragVS = ragVS
		a.ragDS = rag.NewDocumentStore(ragVS.DB())
		a.currentModelMu.RLock()
		modelName := a.currentModelName
		a.currentModelMu.RUnlock()
		embedder := &rag.ClientEmbedder{Client: a.client}
		embedder.SetModel(modelName)
		a.ragEmbedder = embedder
		collection := a.config.RAGActiveKB
		if collection == "" {
			collection = "default"
		}
		a.service.SetRAG(ragVS, a.ragDS, embedder, collection, a.config.RAGEnabled)
		zlog.Info().Str("dir", ragDir).Str("collection", collection).Bool("enabled", a.config.RAGEnabled).Msg("[startup] RAG initialized")
	}

	a.serverMu.Lock()
	a.server = llm.NewServer(a.buildServerConfig())
	a.serverMu.Unlock()

	a.ready.Store(true)

	go func() {
		removed := a.service.CleanupAbnormalConversations()
		if len(removed) > 0 {
			titles := make([]string, 0, len(removed))
			for _, ac := range removed {
				titles = append(titles, ac.Title)
			}
			zlog.Info().Int("count", len(removed)).Interface("titles", titles).Msg("[startup] removed abnormal conversations")

			a.cleanupResultMu.Lock()
			a.cleanupResult = removed
			a.cleanupResultMu.Unlock()

			runtime.EventsEmit(ctx, "chat:abnormal_cleanup", map[string]interface{}{
				"count":   len(removed),
				"removed": removed,
			})
		}
	}()

	go a.startServerAndWatch(a.server, ctx)
}

func (a *App) shutdown(ctx context.Context) {
	a.stopOnce.Do(func() {
		if a.service != nil {
			a.service.StopGeneration()
		}

		a.serverMu.Lock()
		if a.watchCancel != nil {
			a.watchCancel()
			a.watchCancel = nil
		}
		srv := a.server
		a.serverMu.Unlock()

		if srv != nil {
			if err := srv.Stop(); err != nil {
				zlog.Error().Err(err).Msg("shutting down: stop server failed")
			}
			srv.CloseJob()
		}

		if a.ragVS != nil {
			if err := a.ragVS.Close(); err != nil {
				zlog.Error().Err(err).Msg("shutting down: close RAG vector store failed")
			}
		}

		if a.db != nil {
			if err := a.db.Close(); err != nil {
				zlog.Error().Err(err).Msg("shutting down: close database failed")
			}
		}
	})
}

func (a *App) SendMessage(params chat.SendMessageParams) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪，请检查配置和数据。")
	}

	if !a.serverReady.Load() {
		return fmt.Errorf("AI 服务未启动，请等待服务就绪或检查配置。")
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				zlog.Error().Interface("panic", r).Msg("SendMessage panic")
				convID := a.service.CurrentConvID()
				if convID == "" {
					convID = params.ConversationID
				}
				runtime.EventsEmit(a.ctx, "chat:stream", chat.StreamEvent{
					Type:           "error",
					Content:        fmt.Sprintf("内部错误: %v", r),
					ConversationID: convID,
				})
			}
		}()
		if err := a.service.SendMessage(a.ctx, params); err != nil {
			zlog.Error().Err(err).Msg("SendMessage error")
			convID := a.service.CurrentConvID()
			if convID == "" {
				convID = params.ConversationID
			}
			runtime.EventsEmit(a.ctx, "chat:stream", chat.StreamEvent{
				Type:           "error",
				Content:        err.Error(),
				ConversationID: convID,
			})
		}
	}()
	return nil
}

func (a *App) ListKnowledgeBases() ([]rag.CollectionInfo, error) {
	if a.ragVS == nil {
		return nil, fmt.Errorf("知识库未初始化")
	}
	return a.ragVS.ListCollections()
}

func (a *App) CreateKnowledgeBase(name string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	if name == "" {
		return fmt.Errorf("知识库名称不能为空")
	}
	return a.ragVS.CreateCollection(name, 0)
}

func (a *App) DeleteKnowledgeBase(name string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	if name == "default" {
		return fmt.Errorf("不能删除默认知识库")
	}
	return a.ragVS.DeleteCollection(name)
}

// 上传文档允许的文件扩展名
var allowedDocExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true,
	".xml": true, ".html": true, ".yaml": true, ".yml": true,
	".toml": true, ".ini": true, ".cfg": true, ".log": true,
	".sql": true,
	".go":  true, ".py": true, ".js": true, ".ts": true,
	".java": true, ".c": true, ".cpp": true, ".h": true,
	".rs": true, ".sh": true, ".rb": true, ".php": true,
	".swift": true, ".kt": true,
	".pdf": true, ".docx": true,
}

// 上传文档允许的 MIME 类型
var allowedDocMIMETypes = map[string]bool{
	"text/plain": true, "text/markdown": true, "text/csv": true,
	"application/json": true, "application/xml": true, "text/xml": true,
	"text/html": true, "text/yaml": true, "application/x-yaml": true,
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
}

const maxUploadSize = 50 * 1024 * 1024 // 50MB

func (a *App) UploadDocument(kbName string, fileName string, fileData string, mimeType string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	if !a.serverReady.Load() {
		return fmt.Errorf("AI 服务未启动，无法生成嵌入向量")
	}

	// 验证文件扩展名
	ext := strings.ToLower(filepath.Ext(fileName))
	if !allowedDocExts[ext] {
		return fmt.Errorf("不支持的文件类型: %s", ext)
	}

	// 验证 MIME 类型（如果前端提供了）
	if mimeType != "" && !allowedDocMIMETypes[mimeType] {
		return fmt.Errorf("不支持的 MIME 类型: %s", mimeType)
	}

	// 验证文件大小
	decodedLen := base64.StdEncoding.DecodedLen(len(fileData))
	if decodedLen > maxUploadSize {
		return fmt.Errorf("文件大小超过限制（最大 %d MB）", maxUploadSize/(1024*1024))
	}

	embedder := a.ragEmbedder
	if embedder == nil {
		return fmt.Errorf("知识库未初始化")
	}
	chunkCfg := rag.ChunkConfig{
		ChunkSize:    a.config.RAGChunkSize,
		ChunkOverlap: a.config.RAGChunkOverlap,
	}
	if chunkCfg.ChunkSize <= 0 {
		chunkCfg.ChunkSize = 512
	}
	if chunkCfg.ChunkOverlap <= 0 {
		chunkCfg.ChunkOverlap = 64
	}
	_, err := rag.IngestFileFromBase64(a.ctx, a.ragVS, a.ragDS, embedder, kbName, fileName, fileData, mimeType, chunkCfg)
	if err != nil {
		return fmt.Errorf("上传文档失败: %w", err)
	}
	return nil
}

func (a *App) ListDocuments(kbName string) ([]rag.DocumentMeta, error) {
	if a.ragDS == nil {
		return nil, fmt.Errorf("知识库未初始化")
	}
	return a.ragDS.List(kbName)
}

func (a *App) DeleteDocument(kbName string, docID string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	if a.ragDS != nil {
		if err := a.ragDS.Delete(kbName, docID); err != nil {
			zlog.Error().Err(err).Msg("[rag] delete document meta failed")
		}
	}
	return a.ragVS.DeleteDocument(kbName, docID)
}

func (a *App) SetActiveKnowledgeBase(kbName string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	a.config.RAGActiveKB = kbName
	a.service.SetRAGCollection(kbName)
	return config.Save(filepath.Join(appDir(), "config.json"), a.config)
}

func (a *App) GetActiveKnowledgeBase() string {
	return a.config.RAGActiveKB
}

func (a *App) SetRAGEnabled(enabled bool) {
	a.config.RAGEnabled = enabled
	a.service.SetRAGEnabled(enabled)
	if err := config.Save(filepath.Join(appDir(), "config.json"), a.config); err != nil {
		zlog.Error().Err(err).Msg("[rag] save config failed")
	}
}

func (a *App) IsRAGEnabled() bool {
	return a.config.RAGEnabled
}

func (a *App) StopGeneration() {
	if a.service != nil {
		a.service.StopGeneration()
	}
}

func (a *App) GetConversations() ([]*chat.Conversation, error) {
	if !a.ready.Load() {
		return nil, fmt.Errorf("应用未就绪。")
	}
	return a.service.GetConversations()
}

func (a *App) GetMessages(conversationID string) ([]*chat.Message, error) {
	if !a.ready.Load() {
		return nil, fmt.Errorf("应用未就绪。")
	}
	return a.service.GetMessages(conversationID)
}

func (a *App) CreateConversation() (*chat.Conversation, error) {
	if !a.ready.Load() {
		return nil, fmt.Errorf("应用未就绪。")
	}
	return a.service.CreateConversation()
}

func (a *App) RenameConversation(id string, title string) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪。")
	}
	return a.service.RenameConversation(id, title)
}

func (a *App) DeleteConversation(id string) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪。")
	}
	return a.service.DeleteConversation(id)
}

func (a *App) SearchMessages(query string) ([]*chat.Message, error) {
	if !a.ready.Load() {
		return nil, fmt.Errorf("应用未就绪。")
	}
	return a.service.SearchMessages(query)
}

func (a *App) ExportConversation(id string, format string) (string, error) {
	if !a.ready.Load() {
		return "", fmt.Errorf("应用未就绪。")
	}
	return a.service.ExportConversation(id, format)
}

func (a *App) ExportConversationWithDialog(id string, format string) (bool, error) {
	if !a.ready.Load() {
		return false, fmt.Errorf("应用未就绪。")
	}

	content, err := a.service.ExportConversation(id, format)
	if err != nil {
		return false, err
	}

	var defaultName string
	var filterName string
	var filterPattern string
	switch format {
	case "json":
		defaultName = "对话导出.json"
		filterName = "JSON 文件 (*.json)"
		filterPattern = "*.json"
	case "txt", "plain", "plaintext":
		defaultName = "对话导出.txt"
		filterName = "纯文本文件 (*.txt)"
		filterPattern = "*.txt"
	case "csv":
		defaultName = "对话导出.csv"
		filterName = "CSV 文件 (*.csv)"
		filterPattern = "*.csv"
	default:
		defaultName = "对话导出.md"
		filterName = "Markdown 文件 (*.md)"
		filterPattern = "*.md"
	}

	savePath, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		DefaultFilename: defaultName,
		Title:           "导出对话",
		Filters: []runtime.FileFilter{
			{
				DisplayName: filterName,
				Pattern:     filterPattern,
			},
		},
	})
	if err != nil {
		return false, err
	}
	if savePath == "" {
		return false, nil
	}

	err = os.WriteFile(savePath, []byte(content), 0644)
	if err != nil {
		return false, fmt.Errorf("写入文件失败: %w", err)
	}

	return true, nil
}

func (a *App) SelectImageFile() (string, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择图片",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "图片文件",
				Pattern:     "*.jpg;*.jpeg;*.png;*.gif;*.webp;*.bmp;*.svg",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("选择文件失败: %w", err)
	}
	return filePath, nil
}

func (a *App) GetSearchAPIKeys() SearchAPIKeys {
	return a.loadSearchAPIKeys()
}

func (a *App) SetSearchAPIKeys(keys SearchAPIKeys) error {
	setFn := func(dbKey, value string) error {
		// 空值表示用户未修改，跳过更新
		if value == "" {
			return nil
		}
		if a.encKey != nil {
			return store.SetEncryptedSetting(a.db, dbKey, value, a.encKey)
		}
		return store.SetSetting(a.db, dbKey, value)
	}
	if err := setFn("search_ollama_api_key", keys.OllamaAPIKey); err != nil {
		return fmt.Errorf("save ollama api key: %w", err)
	}
	if err := setFn("search_tavily_api_key", keys.TavilyAPIKey); err != nil {
		return fmt.Errorf("save tavily api key: %w", err)
	}
	if err := setFn("search_github_api_key", keys.GitHubAPIKey); err != nil {
		return fmt.Errorf("save github api key: %w", err)
	}
	return nil
}

func (a *App) loadSearchAPIKeys() SearchAPIKeys {
	keys := a.loadSearchAPIKeysFromDB()
	a.applyEnvOverrides(&keys)
	return keys
}

// loadSearchAPIKeysFromDB 仅从数据库/加密存储加载 API Key
func (a *App) loadSearchAPIKeysFromDB() SearchAPIKeys {
	keys := SearchAPIKeys{}
	getFn := func(key string) (string, error) {
		if a.encKey != nil {
			return store.GetEncryptedSetting(a.db, key, a.encKey)
		}
		return store.GetSetting(a.db, key)
	}
	if v, err := getFn("search_ollama_api_key"); err == nil {
		keys.OllamaAPIKey = v
	}
	if v, err := getFn("search_tavily_api_key"); err == nil {
		keys.TavilyAPIKey = v
	}
	if v, err := getFn("search_github_api_key"); err == nil {
		keys.GitHubAPIKey = v
	}
	return keys
}

// applyEnvOverrides 用环境变量覆盖数据库值（优先级：环境变量 > 数据库）
func (a *App) applyEnvOverrides(keys *SearchAPIKeys) {
	if apiKey := os.Getenv("OLLAMA_API_KEY"); apiKey != "" {
		keys.OllamaAPIKey = apiKey
	}
	if apiKey := os.Getenv("TAVILY_API_KEY"); apiKey != "" {
		keys.TavilyAPIKey = apiKey
	}
	if apiKey := os.Getenv("GITHUB_API_KEY"); apiKey != "" {
		keys.GitHubAPIKey = apiKey
	}
}

// buildSearchChain 根据当前 API Key 配置构建搜索链
func (a *App) buildSearchChain() *search.SearchChain {
	var searchProviders []search.CategorizedProvider
	keys := a.loadSearchAPIKeys()
	if keys.TavilyAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewTavilyProvider(keys.TavilyAPIKey), Categories: []string{"general", "code"}})
	}
	if keys.OllamaAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewOllamaProvider(keys.OllamaAPIKey), Categories: []string{"general", "code"}})
	}
	searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewDuckDuckGoProvider(), Categories: []string{"general"}})
	searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewBingProvider(), Categories: []string{"general"}})
	if keys.GitHubAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewGitHubProvider(keys.GitHubAPIKey), Categories: []string{"code"}})
	}
	return search.NewCategorizedSearchChain(searchProviders)
}

// HasServerAPIKey 返回是否已设置 API Key（不暴露实际密钥值给前端）
func (a *App) HasServerAPIKey() bool {
	return a.getServerAPIKey() != ""
}

// getServerAPIKey 内部方法，获取实际的 API Key 值
// 当 ServerAPIKeyEnabled 为 false 时返回空字符串，不发送 API Key
func (a *App) getServerAPIKey() string {
	if !a.config.ServerAPIKeyEnabled {
		return ""
	}
	var value string
	if a.encKey != nil {
		if v, err := store.GetEncryptedSetting(a.db, "server_api_key", a.encKey); err == nil {
			value = v
		}
	}
	if value == "" {
		if v, err := store.GetSetting(a.db, "server_api_key"); err == nil {
			value = v
		}
	}
	return value
}

func (a *App) SetServerAPIKey(key string) error {
	if a.encKey != nil {
		return store.SetEncryptedSetting(a.db, "server_api_key", key, a.encKey)
	}
	return store.SetSetting(a.db, "server_api_key", key)
}

func (a *App) GetConfig() *config.Config {
	if a.config == nil {
		cfgPath := filepath.Join(appDir(), "config.json")
		cfg, err := config.Load(cfgPath)
		if err != nil || cfg == nil {
			cfg = config.DefaultConfig()
		}
		a.config = cfg
	}
	return a.config
}

func (a *App) UpdateConfig(cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	if a.service != nil {
		a.service.UpdateConfig(cfg)
	}
	a.config = cfg

	a.client = llm.NewClient(a.config.APIBase, a.getServerAPIKey())

	searchChain := a.buildSearchChain()

	if a.service != nil {
		a.service.UpdateClient(a.client)
		a.service.UpdateSearchChain(searchChain)
	}

	return config.Save(filepath.Join(appDir(), "config.json"), cfg)
}

func (a *App) GetServerStatus() llm.ServerStatus {
	a.serverMu.Lock()
	srv := a.server
	a.serverMu.Unlock()
	if srv != nil {
		status := srv.Status()
		if status.Running {
			if a.service != nil {
				caps := a.service.GetModelCapabilities()
				status.Capabilities = &caps
			}
			a.currentModelMu.RLock()
			status.CurrentModel = a.currentModelName
			a.currentModelMu.RUnlock()
		}
		if a.isSwitching.Load() {
			status.Switching = true
			a.switchingToMu.RLock()
			status.SwitchingTo = a.switchingTo
			a.switchingToMu.RUnlock()
		}
		return status
	}
	return llm.ServerStatus{Running: false, Error: "server not initialized"}
}

func (a *App) PrepareShutdown() {
	a.stopOnce.Do(func() {
		if a.service != nil {
			a.service.StopGeneration()
		}
		a.serverMu.Lock()
		srv := a.server
		a.serverMu.Unlock()
		if srv != nil {
			// 同步停止服务器，确保进程在应用退出前完全终止
			if err := srv.Stop(); err != nil {
				zlog.Error().Err(err).Msg("prepare shutdown: stop failed")
			}
			srv.CloseJob()
		}
	})
}

func (a *App) shouldPreventClose() bool {
	return !a.exiting.Load()
}

func (a *App) tryStartExit() bool {
	if a.ctx == nil {
		return false
	}
	return a.exiting.CompareAndSwap(false, true)
}

func (a *App) beforeClose(ctx context.Context) bool {
	if !a.shouldPreventClose() {
		return false
	}
	runtime.WindowHide(ctx)
	a.hidden.Store(true)
	return true
}

func (a *App) ShowWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
		a.hidden.Store(false)
	}
}

func (a *App) GracefulExit() {
	if !a.tryStartExit() {
		return
	}

	runtime.WindowShow(a.ctx)
	a.hidden.Store(false)

	go func() {
		a.stopOnce.Do(func() {
			if a.service != nil {
				a.service.StopGeneration()
			}
			runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]interface{}{
				"stage":   "stopping_generation",
				"message": "正在停止生成...",
			})

			a.serverMu.Lock()
			if a.watchCancel != nil {
				a.watchCancel()
				a.watchCancel = nil
			}
			srv := a.server
			a.serverMu.Unlock()

			if srv != nil {
				runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]interface{}{
					"stage":   "stopping_server",
					"message": "正在关闭服务...",
				})
				if err := srv.Stop(); err != nil {
					zlog.Error().Err(err).Msg("graceful exit: stop server failed")
				}
				srv.CloseJob()
			}

			if a.ragVS != nil {
				runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]interface{}{
					"stage":   "closing_rag",
					"message": "正在关闭知识库...",
				})
				if err := a.ragVS.Close(); err != nil {
					zlog.Error().Err(err).Msg("graceful exit: close RAG vector store failed")
				}
			}

			if a.db != nil {
				runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]interface{}{
					"stage":   "closing_db",
					"message": "正在关闭数据库...",
				})
				if err := a.db.Close(); err != nil {
					zlog.Error().Err(err).Msg("graceful exit: close database failed")
				}
			}

			runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]interface{}{
				"stage":   "done",
				"message": "再见 👋",
			})
		})

		runtime.Quit(a.ctx)
		systray.Quit()
	}()
}

func (a *App) onSystrayReady() {
	systray.SetTitle("豆芽")
	systray.SetTooltip("豆芽 - AI 聊天助手")
	systray.SetIcon(iconData)

	systray.SetOnTapped(func() {
		a.ShowWindow()
	})

	mShow := systray.AddMenuItem("显示豆芽", "显示主窗口")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("退出豆芽", "退出程序")

	go func() {
		for {
			select {
			case <-mShow.ClickedCh:
				a.ShowWindow()
			case <-mQuit.ClickedCh:
				a.GracefulExit()
				return
			}
		}
	}()
}

func (a *App) onSystrayExit() {
	systray.SetIcon([]byte{})
}

func (a *App) DeleteMessage(id string) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪。")
	}
	return a.service.DeleteMessage(id)
}

func (a *App) RegenerateMessage(userMessageID string, searchEnabled bool) error {
	if !a.ready.Load() {
		return fmt.Errorf("应用未就绪。")
	}

	if !a.serverReady.Load() {
		return fmt.Errorf("AI 服务未启动，请等待服务就绪或检查配置。")
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				zlog.Error().Interface("panic", r).Msg("RegenerateMessage panic")
				convID := a.service.CurrentConvID()
				runtime.EventsEmit(a.ctx, "chat:stream", chat.StreamEvent{
					Type:           "error",
					Content:        fmt.Sprintf("内部错误: %v", r),
					ConversationID: convID,
				})
			}
		}()
		if err := a.service.RegenerateMessage(userMessageID, searchEnabled); err != nil {
			zlog.Error().Err(err).Msg("RegenerateMessage error")
			convID := a.service.CurrentConvID()
			runtime.EventsEmit(a.ctx, "chat:stream", chat.StreamEvent{
				Type:           "error",
				Content:        err.Error(),
				ConversationID: convID,
			})
		}
	}()
	return nil
}

func (a *App) runningStatus() llm.ServerStatus {
	caps := a.service.GetModelCapabilities()
	a.currentModelMu.RLock()
	modelName := a.currentModelName
	a.currentModelMu.RUnlock()
	return llm.ServerStatus{
		Running:      true,
		CurrentModel: modelName,
		Capabilities: &caps,
	}
}

func (a *App) GetCleanupResult() []*chat.AbnormalConversation {
	a.cleanupResultMu.Lock()
	defer a.cleanupResultMu.Unlock()
	result := a.cleanupResult
	a.cleanupResult = nil
	return result
}

func (a *App) GetModelCapabilities() llm.ModelCapabilities {
	if a.service == nil {
		return llm.ModelCapabilities{TextInput: true}
	}
	return a.service.GetModelCapabilities()
}

func (a *App) generatePresetFile() error {
	modelsDir := filepath.Join(appDir(), "models")
	presets, err := llm.ScanModelsDir(modelsDir)
	if err != nil {
		return fmt.Errorf("scan models dir: %w", err)
	}

	if len(presets) == 0 {
		return fmt.Errorf("no models found in %s", modelsDir)
	}

	defaultModelPath := a.config.ModelPath
	llm.SetDefaultAlias(presets, defaultModelPath)

	presetRelPaths := make(map[string]string, len(presets))
	for i := range presets {
		presetRelPaths[presets[i].Name] = presets[i].ModelPath
	}

	for i := range presets {
		absModelPath := resolvePath(presets[i].ModelPath)
		presets[i].ModelPath = absModelPath

		if presets[i].MmprojPath != "" {
			presets[i].MmprojPath = resolvePath(presets[i].MmprojPath)
		}
	}

	a.presetsMu.Lock()
	a.presets = presets
	a.presetRelPaths = presetRelPaths
	a.presetsMu.Unlock()

	var globalDefaults map[string]string
	if a.hwInfo != nil {
		defaultModelPath := ""
		if len(presets) > 0 {
			defaultModelPath = presets[0].ModelPath
			for _, p := range presets {
				if p.Alias == "default" {
					defaultModelPath = p.ModelPath
					break
				}
			}
		}
		sp := system.CalculateSmartParams(a.hwInfo, defaultModelPath)
		globalDefaults = map[string]string{
			"ctx-size": fmt.Sprintf("%d", sp.ContextSize),
		}
		zlog.Info().Int("ctx-size", sp.ContextSize).Msg("[preset] global defaults")
	}

	content := llm.GeneratePreset(presets, globalDefaults)
	presetPath := filepath.Join(appDir(), "router-preset.ini")
	if err := llm.WritePresetFile(presetPath, content); err != nil {
		return fmt.Errorf("write preset file: %w", err)
	}

	zlog.Info().Str("path", presetPath).Int("count", len(presets)).Msg("[preset] generated preset file")
	return nil
}

// findModelMatch 在模型状态映射中查找匹配的模型
// 先精确匹配，再模糊匹配（排除 "default" 这种太通用的 ID）
func findModelMatch(name string, statuses map[string]string) (bool, string) {
	if status, ok := statuses[name]; ok {
		return true, status
	}
	for id, status := range statuses {
		if llm.FuzzyMatchModelID(id, name) {
			return true, status
		}
	}
	return false, ""
}

func (a *App) GetAvailableModels() ([]llm.ModelOption, error) {
	a.presetsMu.RLock()
	presetsCopy := make([]llm.ModelPreset, len(a.presets))
	copy(presetsCopy, a.presets)
	a.presetsMu.RUnlock()

	options := make([]llm.ModelOption, 0, len(presetsCopy))

	modelStatuses := map[string]string{}
	if a.client != nil && a.serverReady.Load() {
		if models, err := a.client.GetModelsList(a.ctx); err == nil {
			for _, m := range models {
				modelStatuses[m.ID] = m.Status
			}
		}
	}

	for _, p := range presetsCopy {
		isDefault := p.Alias == "default"
		fileName := filepath.Base(p.ModelPath)
		isLoaded, status := findModelMatch(p.Name, modelStatuses)
		options = append(options, llm.ModelOption{
			Name:         p.Name,
			ModelPath:    p.ModelPath,
			FileName:     fileName,
			IsDefault:    isDefault,
			IsLoaded:     isLoaded,
			MmprojVision: p.MmprojVision,
			MmprojAudio:  p.MmprojAudio,
			MmprojVideo:  p.MmprojVideo,
			Status:       status,
		})
	}

	return options, nil
}

func isAlreadyRunningError(err error) bool {
	if err == nil {
		return false
	}
	errMsg := strings.ToLower(err.Error())
	return strings.Contains(errMsg, "already running") ||
		strings.Contains(errMsg, "already loaded")
}

// emitSwitchingStatus emits a server status event indicating a model switch is in progress.
func (a *App) emitSwitchingStatus(modelName string) {
	runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
		Running:     false,
		Switching:   true,
		SwitchingTo: modelName,
	})
}

// emitSwitchSuccess emits a server status event indicating the model switch succeeded.
func (a *App) emitSwitchSuccess(modelName string) {
	caps := a.service.GetModelCapabilities()
	runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
		Running:      true,
		CurrentModel: modelName,
		Capabilities: &caps,
	})
}

// emitSwitchProgress emits a progress event for model switch.
func (a *App) emitSwitchProgress(stage, targetModel string) {
	runtime.EventsEmit(a.ctx, "server:switchProgress", map[string]interface{}{
		"stage":       stage,
		"targetModel": targetModel,
	})
}

// SwitchModel 切换模型（主流程编排）
func (a *App) SwitchModel(modelName string) SwitchResult {
	// 预检查
	if errMsg := a.switchPreCheck(); errMsg != "" {
		return SwitchResult{Error: errMsg}
	}

	// 停止当前生成，记录旧模型，设置切换状态
	previousModel := a.switchPrepare(modelName)

	// 加载新模型
	alreadyRunning, loadErr := a.switchLoadModel(modelName)
	if loadErr != "" {
		return a.handleSwitchFailure(modelName, previousModel, loadErr)
	}

	// 等待模型就绪（已运行的模型跳过）
	if !alreadyRunning {
		if waitErr := a.switchWaitReady(modelName); waitErr != "" {
			return a.handleSwitchFailure(modelName, previousModel, waitErr)
		}
	}

	// 完成切换：更新状态、保存配置、检测架构
	return a.switchFinalize(modelName, previousModel)
}

// switchPreCheck 预检查：服务器是否启动、是否正在切换
func (a *App) switchPreCheck() string {
	if a.server == nil || a.client == nil {
		return "服务器未启动"
	}
	if !a.isSwitching.CompareAndSwap(false, true) {
		return "正在切换模型中，请稍候。"
	}
	return ""
}

// switchPrepare 停止当前生成、记录旧模型、设置切换状态
func (a *App) switchPrepare(modelName string) string {
	if a.service != nil {
		a.service.StopGeneration()
	}

	a.currentModelMu.RLock()
	previousModel := a.currentModelName
	a.currentModelMu.RUnlock()

	a.switchingToMu.Lock()
	a.switchingTo = modelName
	a.switchingToMu.Unlock()

	a.serverReady.Store(false)

	a.emitSwitchingStatus(modelName)
	a.emitSwitchProgress("loading", modelName)

	return previousModel
}

// switchLoadModel 加载模型，返回 (是否已运行, 错误消息)
func (a *App) switchLoadModel(modelName string) (bool, string) {
	loadErr := a.client.LoadModel(a.ctx, modelName)
	if loadErr == nil {
		// LoadModel 返回 200 仅表示开始加载，需要等待模型真正就绪
		a.emitSwitchProgress("waiting", modelName)
		if waitErr := a.client.WaitForModelLoaded(a.ctx, modelName, 120*time.Second); waitErr != nil {
			return false, fmt.Sprintf("模型加载超时: %v", waitErr)
		}
		return false, ""
	}

	if isAlreadyRunningError(loadErr) {
		// 模型已在运行，视为切换成功（后续 switchFinalize 会发送 done 事件）
		zlog.Info().Str("model", modelName).Msg("[router] model is already running, treating as switch success")
		return true, ""
	}

	return false, fmt.Sprintf("模型加载失败: %v", loadErr)
}

// switchWaitReady 等待模型就绪（含 mmproj 回退检测）
// 注意：不需要在此调用 WaitForModelLoaded，因为调用方 switchLoadModel 已确保模型加载完成
func (a *App) switchWaitReady(modelName string) string {
	// 等待 mmproj 等后加载初始化完成
	// 使用指数退避：200ms → 300ms → 500ms → 800ms
	propsCtx, propsCancel := context.WithTimeout(a.ctx, 15*time.Second)
	defer propsCancel()
	backoffs := []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 600 * time.Millisecond, 800 * time.Millisecond, time.Second}
	var lastProps *llm.ServerProps
	for i := 0; i < 10; i++ {
		props, propsErr := a.client.GetServerProps(propsCtx, modelName)
		if propsErr == nil {
			lastProps = props
			// 不需要 mmproj — 立即退出
			if !props.Modalities.Vision && !props.Modalities.Audio {
				break
			}
			// mmproj 已加载 — 退出
			if i > 0 {
				break
			}
			// 首次成功但有 mmproj — 可能仍在加载，再检查一次
			continue
		}
		select {
		case <-propsCtx.Done():
			return ""
		case <-time.After(backoffs[min(i, len(backoffs)-1)]):
		}
	}
	// 缓存 props 结果，供 DetectModelArchitectureForModel 复用
	if lastProps != nil {
		a.service.SetCachedProps(lastProps)
	}

	return ""
}

// switchFinalize 完成切换：更新模型名、保存配置、检测架构、发射事件
func (a *App) switchFinalize(modelName, previousModel string) SwitchResult {
	// 更新当前模型名
	a.currentModelMu.Lock()
	a.currentModelName = modelName
	a.currentModelMu.Unlock()

	// 更新嵌入模型名
	if a.ragEmbedder != nil {
		a.ragEmbedder.SetModel(modelName)
	}

	// 保存配置
	a.presetsMu.RLock()
	relPath, hasRelPath := a.presetRelPaths[modelName]
	a.presetsMu.RUnlock()
	if hasRelPath {
		a.config.ModelPath = relPath
		if err := config.Save(filepath.Join(appDir(), "config.json"), a.config); err != nil {
			zlog.Error().Err(err).Msg("[router] save config after model switch failed")
			runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
				Running:      true,
				CurrentModel: modelName,
				Error:        fmt.Sprintf("config save failed, model may revert on restart: %v", err),
			})
		}
	}

	// 检测模型架构
	a.service.SetDetectedModelName(modelName)
	if err := a.service.DetectModelArchitectureForModel(modelName); err != nil {
		zlog.Error().Err(err).Msg("[router] detect model architecture after switch failed")
		runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
			Running:      true,
			CurrentModel: modelName,
			Error:        fmt.Sprintf("模型架构检测失败: %v", err),
		})
	}

	// 发射完成事件
	a.emitSwitchProgress("done", modelName)

	// 在清除切换状态之前发射成功事件
	// 确保前端在 WatchWithCallback 发出过时状态之前收到成功事件
	a.emitSwitchSuccess(modelName)
	a.serverReady.Store(true)

	// 清除切换状态
	a.isSwitching.Store(false)
	a.switchingToMu.Lock()
	a.switchingTo = ""
	a.switchingToMu.Unlock()

	zlog.Info().Str("model", modelName).Str("previous", previousModel).Msg("[router] model switched")

	a.currentModelMu.RLock()
	resultModel := a.currentModelName
	a.currentModelMu.RUnlock()
	caps := a.service.GetModelCapabilities()
	return SwitchResult{
		Success:       true,
		CurrentModel:  resultModel,
		Capabilities:  &caps,
		PreviousModel: previousModel,
	}
}

// handleSwitchFailure 处理模型切换失败：尝试恢复旧模型，清理状态，返回错误结果
func (a *App) handleSwitchFailure(modelName, previousModel, errMsg string) SwitchResult {
	zlog.Error().Str("error", errMsg).Msg("[router] model switch failed")
	a.emitSwitchProgress("failed", modelName)

	// 注意：isSwitching 在回滚完成后再清除，防止回滚期间用户发起新切换
	a.switchingToMu.Lock()
	a.switchingTo = ""
	a.switchingToMu.Unlock()

	rollbackSuccess := false
	if previousModel != "" && previousModel != modelName {
		zlog.Info().Str("model", previousModel).Msg("[router] attempting to restore model")
		restoreCtx, restoreCancel := context.WithTimeout(a.ctx, 30*time.Second)
		if restoreErr := a.client.LoadModel(restoreCtx, previousModel); restoreErr == nil {
			_ = a.client.WaitForModelLoaded(restoreCtx, previousModel, 30*time.Second)
			a.currentModelMu.Lock()
			a.currentModelName = previousModel
			a.currentModelMu.Unlock()
			a.emitSwitchSuccess(previousModel)
			a.serverReady.Store(true)
			rollbackSuccess = true
		} else {
			zlog.Error().Err(restoreErr).Str("model", previousModel).Msg("[router] failed to restore model")
			runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
				Running: false,
				Error:   fmt.Sprintf("%s，恢复旧模型也失败", errMsg),
			})
		}
		restoreCancel()
	} else {
		runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
			Running: false,
			Error:   errMsg,
		})
	}

	// 回滚完成后再清除 isSwitching
	a.isSwitching.Store(false)

	return SwitchResult{
		Error:           errMsg,
		PreviousModel:   previousModel,
		RolledBack:      previousModel != "" && previousModel != modelName,
		RollbackSuccess: rollbackSuccess,
	}
}

func (a *App) ReloadModels() error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	if err := a.client.ReloadModels(a.ctx); err != nil {
		return fmt.Errorf("热重载模型列表失败: %w", err)
	}
	system.InvalidateGGUFCache()
	if err := a.generatePresetFile(); err != nil {
		zlog.Error().Err(err).Msg("[reload] regenerate preset file failed")
	}
	return nil
}
