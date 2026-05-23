// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
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
	"douya/internal/store"
	"douya/internal/system"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type SwitchResult struct {
	Success       bool                    `json:"success"`
	Error         string                  `json:"error,omitempty"`
	CurrentModel  string                  `json:"current_model,omitempty"`
	Capabilities  *llm.ModelCapabilities  `json:"capabilities,omitempty"`
	PreviousModel string                  `json:"previous_model,omitempty"`
	RolledBack    bool                    `json:"rolled_back,omitempty"`
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
	presetRelPaths    map[string]string
	currentModelMu   sync.RWMutex
	currentModelName string
	switchingMu      sync.Mutex
	isSwitching      atomic.Bool
	switchingTo      string
	switchingToMu    sync.RWMutex
	ragVS            *rag.VectorStore
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
		log.Printf("[appDir] failed to get executable path: %v", err)
		cachedAppDir = "."
		return cachedAppDir
	}

	exeDir := filepath.Dir(exePath)
	log.Printf("[appDir] exe path: %s, exe dir: %s", exePath, exeDir)

	if _, err := os.Stat(filepath.Join(exeDir, "config.json")); err == nil {
		log.Printf("[appDir] found config.json in exe directory: %s", exeDir)
		cachedAppDir = exeDir
		return cachedAppDir
	}

	dir := exeDir
	for i := 0; i < 5; i++ {
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
		if _, err := os.Stat(filepath.Join(dir, "config.json")); err == nil {
			log.Printf("[appDir] found config.json in parent directory (level %d): %s", i+1, dir)
			cachedAppDir = dir
			return cachedAppDir
		}
	}

	log.Printf("[appDir] config.json not found, creating default in exe directory: %s", exeDir)
	defaultCfg := config.DefaultConfig()
	cfgPath := filepath.Join(exeDir, "config.json")
	if err := config.Save(cfgPath, defaultCfg); err != nil {
		log.Printf("[appDir] failed to create default config: %v", err)
	}
	cachedAppDir = exeDir
	return cachedAppDir
}

func resolvePath(p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	baseDir := appDir()
	candidate := filepath.Join(baseDir, p)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return filepath.Join(baseDir, p)
	}
	return abs
}

func (a *App) buildServerConfig() *llm.ServerConfig {
	absServerPath := resolvePath(a.config.LlamaServerPath)
	modelsDir := filepath.Join(appDir(), "models")

	sp := system.CalculateSmartParams(a.hwInfo, "")
	log.Printf("[smart-params] models_dir=%s gpu_layers=%d threads=%d flash=%v cache=%s/%s mlock=%v mmproj_offload=%v",
		modelsDir, sp.GPULayers, sp.Threads, sp.FlashAttn, sp.CacheTypeK, sp.CacheTypeV, sp.Mlock, sp.MmprojOffload)

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

	return &llm.ServerConfig{
		ModelsDir:        modelsDir,
		ServerPath:       absServerPath,
		Port:             a.config.Port,
		GPULayers:        gpuLayers,
		Threads:          sp.Threads,
		FlashAttn:        sp.FlashAttn,
		CacheTypeK:       sp.CacheTypeK,
		CacheTypeV:       sp.CacheTypeV,
		Mlock:            sp.Mlock,
		MmprojAuto:       a.config.MmprojAuto,
		MmprojOffload:    sp.MmprojOffload,
		Repack:           true,
		OpOffload:        true,
		KVUnified:        a.config.KVUnified,
		CacheIdleSlots:   a.config.CacheIdleSlots,
		CacheRAM:         a.config.CacheRAM,
		ImageMinTokens:   a.config.ImageMinTokens,
		ImageMaxTokens:   a.config.ImageMaxTokens,
		FitTarget:        a.config.FitTarget,
		FitCtx:           a.config.FitCtx,
		Reasoning:        a.config.Reasoning,
		ReasoningBudget:  a.config.ReasoningBudget,
		ReasoningFormat:  a.config.ReasoningFormat,
		APIBase:          a.config.APIBase,
		AppDir:           appDir(),
		ModelsPreset:     presetPath,
		ModelsMax:        modelsMax,
		SleepIdleSeconds: sleepIdle,
	}
}

func (a *App) startServerAndWatch(srv *llm.Server, ctx context.Context) {
	if err := srv.Start(); err != nil {
		log.Printf("start llama-server: %v", err)
		runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
			Running: false,
			Error:   fmt.Sprintf("启动 llama-server 失败: %v", err),
		})
		return
	}

	if err := srv.WaitForReady(60e9); err != nil {
		log.Printf("wait for server ready: %v", err)
		runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
			Running: false,
			Error:   fmt.Sprintf("llama-server 未就绪: %v", err),
		})
		return
	}

	a.serverReady.Store(true)

	foundDefault := false
	for _, p := range a.presets {
		if p.Alias == "default" {
			a.currentModelMu.Lock()
			a.currentModelName = p.Name
			a.currentModelMu.Unlock()
			foundDefault = true
			break
		}
	}
	if !foundDefault && len(a.presets) > 0 {
		a.currentModelMu.Lock()
		a.currentModelName = a.presets[0].Name
		a.currentModelMu.Unlock()
		a.currentModelMu.RLock()
		log.Printf("[server] no default preset found, using first model: %s", a.currentModelName)
		a.currentModelMu.RUnlock()
	}

	a.currentModelMu.RLock()
	modelForDetect := a.currentModelName
	a.currentModelMu.RUnlock()
	if err := a.service.DetectModelArchitectureForModel(modelForDetect); err != nil {
		log.Printf("detect model architecture: %v", err)
	}
	a.isSwitching.Store(false)
	runtime.EventsEmit(ctx, "server:status", a.runningStatus())

	watchCtx, watchCancel := context.WithCancel(ctx)
	a.serverMu.Lock()
	a.watchCancel = watchCancel
	a.serverMu.Unlock()
	go srv.WatchWithCallback(watchCtx, func(status llm.ServerStatus) {
		if a.isSwitching.Load() {
			return
		}
		if status.Running {
			a.serverReady.Store(true)
			caps := a.service.GetModelCapabilities()
			status.Capabilities = &caps
			status.CurrentModel = a.currentModelName
		} else {
			a.serverReady.Store(false)
		}
		runtime.EventsEmit(ctx, "server:status", status)
	}, func() {
		a.currentModelMu.RLock()
		modelForDetect2 := a.currentModelName
		a.currentModelMu.RUnlock()
		if err := a.service.DetectModelArchitectureForModel(modelForDetect2); err != nil {
			log.Printf("detect model architecture after restart: %v", err)
		}
		a.serverReady.Store(true)
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
		log.Printf("load config: %v", err)
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
		log.Printf("[startup] missing paths: %v", missingPaths)
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "关键文件缺失",
			Message: msg,
		})
	}

	dbPath := filepath.Join(appDir(), "data", "douya.db")
	a.db, err = store.Init(dbPath)
	if err != nil {
		log.Printf("init database: %v", err)
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "数据库初始化失败",
			Message: fmt.Sprintf("初始化数据库失败: %v", err),
		})
		return
	}

	if err := a.generatePresetFile(); err != nil {
		log.Printf("[startup] generate preset file: %v", err)
	}

	a.client = llm.NewClient(a.config.APIBase)

	var searchProviders []search.CategorizedProvider
	if a.config.SearchEngines.TavilyAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewTavilyProvider(a.config.SearchEngines.TavilyAPIKey), Categories: []string{"general", "code"}})
	}
	if a.config.SearchEngines.OllamaAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewOllamaProvider(a.config.SearchEngines.OllamaAPIKey), Categories: []string{"general", "code"}})
	}
	searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewDuckDuckGoProvider(), Categories: []string{"general"}})
	searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewBingProvider(), Categories: []string{"general"}})
	if a.config.SearchEngines.GitHubAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewGitHubProvider(a.config.SearchEngines.GitHubAPIKey), Categories: []string{"code"}})
	}
	searchChain := search.NewCategorizedSearchChain(searchProviders)

	a.service = chat.NewService(a.client, searchChain, a.db, a.config)
	a.service.SetContext(ctx)

	// Initialize RAG (Badger-backed vector store + LLM embedder)
	ragDir := filepath.Join(appDir(), "data", "rag")
	ragVS, err := rag.NewVectorStore(ragDir, rag.DefaultHNSWConfig())
	if err != nil {
		log.Printf("[startup] RAG vector store init failed (RAG disabled): %v", err)
	} else {
		a.ragVS = ragVS
		embedder := &rag.ClientEmbedder{Client: a.client}
		a.service.SetRAG(ragVS, embedder, "default")
		log.Printf("[startup] RAG initialized: dir=%s", ragDir)
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
			log.Printf("[startup] removed %d abnormal conversations: %v", len(removed), titles)

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
			log.Println("shutting down: stopping llama-server immediately to release VRAM...")
			if err := srv.Stop(); err != nil {
				log.Printf("shutting down: stop server failed: %v", err)
			}
			srv.CloseJob()
			log.Println("shutting down: llama-server stopped, VRAM released")
		}

		if a.ragVS != nil {
			if err := a.ragVS.Close(); err != nil {
				log.Printf("shutting down: close RAG vector store: %v", err)
			}
		}

		if a.db != nil {
			if err := a.db.Close(); err != nil {
				log.Printf("shutting down: close database: %v", err)
			}
		}

		log.Println("shutting down: cleanup complete")
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
				log.Printf("SendMessage panic: %v", r)
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
			log.Printf("SendMessage error: %v", err)
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

	a.client = llm.NewClient(a.config.APIBase)

	var searchProviders []search.CategorizedProvider
	if a.config.SearchEngines.TavilyAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewTavilyProvider(a.config.SearchEngines.TavilyAPIKey), Categories: []string{"general", "code"}})
	}
	if a.config.SearchEngines.OllamaAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewOllamaProvider(a.config.SearchEngines.OllamaAPIKey), Categories: []string{"general", "code"}})
	}
	searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewDuckDuckGoProvider(), Categories: []string{"general"}})
	searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewBingProvider(), Categories: []string{"general"}})
	if a.config.SearchEngines.GitHubAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewGitHubProvider(a.config.SearchEngines.GitHubAPIKey), Categories: []string{"code"}})
	}
	searchChain := search.NewCategorizedSearchChain(searchProviders)

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
			caps := a.service.GetModelCapabilities()
			status.Capabilities = &caps
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
			go func() {
				if err := srv.Stop(); err != nil {
					log.Printf("prepare shutdown: stop failed: %v", err)
				}
				srv.CloseJob()
			}()
		}
	})
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
				log.Printf("RegenerateMessage panic: %v", r)
				convID := a.service.CurrentConvID()
				runtime.EventsEmit(a.ctx, "chat:stream", chat.StreamEvent{
					Type:           "error",
					Content:        fmt.Sprintf("内部错误: %v", r),
					ConversationID: convID,
				})
			}
		}()
		if err := a.service.RegenerateMessage(userMessageID, searchEnabled); err != nil {
			log.Printf("RegenerateMessage error: %v", err)
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

	a.presetRelPaths = make(map[string]string, len(presets))
	for i := range presets {
		a.presetRelPaths[presets[i].Name] = presets[i].ModelPath
	}

	for i := range presets {
		absModelPath := resolvePath(presets[i].ModelPath)
		presets[i].ModelPath = absModelPath

		if presets[i].MmprojPath != "" {
			presets[i].MmprojPath = resolvePath(presets[i].MmprojPath)
		}
	}

	a.presets = presets

	content := llm.GeneratePreset(presets, nil)
	presetPath := filepath.Join(appDir(), "router-preset.ini")
	if err := llm.WritePresetFile(presetPath, content); err != nil {
		return fmt.Errorf("write preset file: %w", err)
	}

	log.Printf("[preset] generated %s with %d models", presetPath, len(presets))
	return nil
}

func (a *App) GetAvailableModels() ([]llm.ModelOption, error) {
	options := make([]llm.ModelOption, 0, len(a.presets))

	loadedModels := map[string]bool{}
	modelStatuses := map[string]string{}
	if a.client != nil && a.serverReady.Load() {
		if models, err := a.client.GetModelsList(a.ctx); err == nil {
			for _, m := range models {
				loadedModels[m.ID] = true
				modelStatuses[m.ID] = m.Status
			}
		}
	}

	for _, p := range a.presets {
		isDefault := p.Alias == "default"
		fileName := filepath.Base(p.ModelPath)
		options = append(options, llm.ModelOption{
			Name:         p.Name,
			ModelPath:    p.ModelPath,
			FileName:     fileName,
			IsDefault:    isDefault,
			IsLoaded:     loadedModels[p.Name],
			MmprojVision: p.MmprojVision,
			MmprojAudio:  p.MmprojAudio,
			Status:       modelStatuses[p.Name],
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
		strings.Contains(errMsg, "409") ||
		strings.Contains(errMsg, "conflict") ||
		strings.Contains(errMsg, "model is already loaded")
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

func (a *App) SwitchModel(modelName string) SwitchResult {
	// 1. Pre-checks
	if a.server == nil || a.client == nil {
		return SwitchResult{Error: "服务器未启动"}
	}
	if a.isSwitching.Load() {
		return SwitchResult{Error: "正在切换模型中，请稍候。"}
	}

	// 2. Stop in-progress generation
	if a.service != nil {
		a.service.StopGeneration()
	}

	// 3. Record previous model, set switching state
	a.currentModelMu.RLock()
	previousModel := a.currentModelName
	a.currentModelMu.RUnlock()

	a.isSwitching.Store(true)
	a.switchingToMu.Lock()
	a.switchingTo = modelName
	a.switchingToMu.Unlock()

	a.serverReady.Store(false)

	// 4. Emit switching status
	a.emitSwitchingStatus(modelName)

	// 5. Load new model (router handles LRU unloading automatically)
	loadErr := a.client.LoadModel(a.ctx, modelName)
	if loadErr != nil {
		if isAlreadyRunningError(loadErr) {
			// Model already running — treat as success
			log.Printf("[router] model %s is already running, treating as switch success", modelName)
			a.currentModelMu.Lock()
			a.currentModelName = modelName
			a.currentModelMu.Unlock()
		} else {
			log.Printf("[router] model switch failed: %v", loadErr)
			a.isSwitching.Store(false)
			a.switchingToMu.Lock()
			a.switchingTo = ""
			a.switchingToMu.Unlock()
			if previousModel != "" && previousModel != modelName {
				log.Printf("[router] attempting to restore model %s", previousModel)
				if restoreErr := a.client.LoadModel(a.ctx, previousModel); restoreErr == nil {
					_ = a.client.WaitForModelLoaded(a.ctx, previousModel, 60*time.Second)
					a.currentModelMu.Lock()
					a.currentModelName = previousModel
					a.currentModelMu.Unlock()
					a.emitSwitchSuccess(previousModel)
					a.serverReady.Store(true)
				} else {
					log.Printf("[router] failed to restore model %s: %v", previousModel, restoreErr)
					runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
						Running: false,
						Error:   fmt.Sprintf("模型加载失败，恢复旧模型也失败: %v", loadErr),
					})
				}
			} else {
				runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
					Running: false,
					Error:   fmt.Sprintf("模型加载失败: %v", loadErr),
				})
			}
			return SwitchResult{
				Error:         fmt.Sprintf("模型加载失败: %v", loadErr),
				PreviousModel: previousModel,
				RolledBack:    previousModel != "" && previousModel != modelName,
			}
		}
	}

	// 6. Wait for model loaded (skip if already running)
	if !isAlreadyRunningError(loadErr) {
		waitCtx, waitCancel := context.WithTimeout(a.ctx, 120*time.Second)
		defer waitCancel()

		if err := a.client.WaitForModelLoaded(waitCtx, modelName, 120*time.Second); err != nil {
			log.Printf("[router] WaitForModelLoaded failed for %s: %v", modelName, err)
			a.isSwitching.Store(false)
			a.switchingToMu.Lock()
			a.switchingTo = ""
			a.switchingToMu.Unlock()
			if previousModel != "" && previousModel != modelName {
				log.Printf("[router] attempting to restore model %s after timeout", previousModel)
				if restoreErr := a.client.LoadModel(a.ctx, previousModel); restoreErr == nil {
					_ = a.client.WaitForModelLoaded(a.ctx, previousModel, 60*time.Second)
					a.currentModelMu.Lock()
					a.currentModelName = previousModel
					a.currentModelMu.Unlock()
					a.emitSwitchSuccess(previousModel)
					a.serverReady.Store(true)
				} else {
					log.Printf("[router] failed to restore model %s: %v", previousModel, restoreErr)
					runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
						Running: false,
						Error:   fmt.Sprintf("模型加载超时，恢复旧模型也失败: %v", err),
					})
				}
			} else {
				runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
					Running: false,
					Error:   fmt.Sprintf("模型加载超时: %v", err),
				})
			}
			return SwitchResult{
				Error:         fmt.Sprintf("模型加载超时: %v", err),
				PreviousModel: previousModel,
				RolledBack:    previousModel != "" && previousModel != modelName,
			}
		}
	}

	// 7. Update current model name (with lock)
	a.currentModelMu.Lock()
	a.currentModelName = modelName
	a.currentModelMu.Unlock()

	// 8. Save config
	if relPath, ok := a.presetRelPaths[modelName]; ok {
		a.config.ModelPath = relPath
		if err := config.Save(filepath.Join(appDir(), "config.json"), a.config); err != nil {
			log.Printf("[router] save config after model switch: %v", err)
			runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
				Running:      true,
				CurrentModel: modelName,
				Error:        fmt.Sprintf("config save failed, model may revert on restart: %v", err),
			})
		}
	}

	// 9. Detect model architecture
	a.service.SetDetectedModelName(modelName)
	if err := a.service.DetectModelArchitectureForModel(modelName); err != nil {
		log.Printf("[router] detect model architecture after switch: %v", err)
		runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
			Running:      true,
			CurrentModel: modelName,
			Error:        fmt.Sprintf("模型架构检测失败: %v", err),
		})
	}

	// 10. Clear switching state
	a.isSwitching.Store(false)
	a.switchingToMu.Lock()
	a.switchingTo = ""
	a.switchingToMu.Unlock()

	// 11. Emit success status
	a.emitSwitchSuccess(modelName)

	a.serverReady.Store(true)

	log.Printf("[router] model switched to %s (from %s)", modelName, previousModel)

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

func (a *App) ReloadModels() error {
	if a.client == nil {
		return fmt.Errorf("客户端未初始化")
	}
	if err := a.client.ReloadModels(a.ctx); err != nil {
		return fmt.Errorf("热重载模型列表失败: %w", err)
	}
	if err := a.generatePresetFile(); err != nil {
		log.Printf("[reload] regenerate preset file: %v", err)
	}
	return nil
}
