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

	"fyne.io/systray"
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
	presetRelPaths    map[string]string
	currentModelMu   sync.RWMutex
	currentModelName string
	switchingMu      sync.Mutex
	isSwitching      atomic.Bool
	switchingTo      string
	switchingToMu    sync.RWMutex
	ragVS            *rag.VectorStore
	ragDS            *rag.DocumentStore
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

	if raw, rawErr := config.LoadRaw(cfgPath); rawErr == nil {
		if se, ok := raw["search_engines"]; ok {
			if seMap, ok := se.(map[string]interface{}); ok {
				migrated := false
				if v, ok := seMap["ollama_api_key"]; ok && v != "" {
					if existing, _ := store.GetSetting(a.db, "search_ollama_api_key"); existing == "" {
						store.SetSetting(a.db, "search_ollama_api_key", fmt.Sprintf("%v", v))
						migrated = true
					}
				}
				if v, ok := seMap["tavily_api_key"]; ok && v != "" {
					if existing, _ := store.GetSetting(a.db, "search_tavily_api_key"); existing == "" {
						store.SetSetting(a.db, "search_tavily_api_key", fmt.Sprintf("%v", v))
						migrated = true
					}
				}
				if v, ok := seMap["github_api_key"]; ok && v != "" {
					if existing, _ := store.GetSetting(a.db, "search_github_api_key"); existing == "" {
						store.SetSetting(a.db, "search_github_api_key", fmt.Sprintf("%v", v))
						migrated = true
					}
				}
				if migrated {
					log.Printf("[startup] migrated search API keys from config.json to database")
					config.Save(cfgPath, a.config)
				}
			}
		}
	}

	if err := a.generatePresetFile(); err != nil {
		log.Printf("[startup] generate preset file: %v", err)
	}

	a.client = llm.NewClient(a.config.APIBase)

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
		a.ragDS = rag.NewDocumentStore(ragVS.DB())
		embedder := &rag.ClientEmbedder{Client: a.client}
		collection := a.config.RAGActiveKB
		if collection == "" {
			collection = "default"
		}
		a.service.SetRAG(ragVS, a.ragDS, embedder, collection, a.config.RAGEnabled)
		log.Printf("[startup] RAG initialized: dir=%s collection=%s enabled=%v", ragDir, collection, a.config.RAGEnabled)
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

func (a *App) UploadDocument(kbName string, fileName string, fileData string, mimeType string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	if !a.serverReady.Load() {
		return fmt.Errorf("AI 服务未启动，无法生成嵌入向量")
	}
	embedder := &rag.ClientEmbedder{Client: a.client}
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
			log.Printf("[rag] delete document meta: %v", err)
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
		log.Printf("[rag] save config: %v", err)
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

func (a *App) GetSearchAPIKeys() SearchAPIKeys {
	return a.loadSearchAPIKeys()
}

func (a *App) SetSearchAPIKeys(keys SearchAPIKeys) error {
	if err := store.SetSetting(a.db, "search_ollama_api_key", keys.OllamaAPIKey); err != nil {
		return fmt.Errorf("save ollama api key: %w", err)
	}
	if err := store.SetSetting(a.db, "search_tavily_api_key", keys.TavilyAPIKey); err != nil {
		return fmt.Errorf("save tavily api key: %w", err)
	}
	if err := store.SetSetting(a.db, "search_github_api_key", keys.GitHubAPIKey); err != nil {
		return fmt.Errorf("save github api key: %w", err)
	}
	return nil
}

func (a *App) loadSearchAPIKeys() SearchAPIKeys {
	keys := SearchAPIKeys{}
	if v, err := store.GetSetting(a.db, "search_ollama_api_key"); err == nil {
		keys.OllamaAPIKey = v
	}
	if v, err := store.GetSetting(a.db, "search_tavily_api_key"); err == nil {
		keys.TavilyAPIKey = v
	}
	if v, err := store.GetSetting(a.db, "search_github_api_key"); err == nil {
		keys.GitHubAPIKey = v
	}
	if apiKey := os.Getenv("OLLAMA_API_KEY"); apiKey != "" {
		keys.OllamaAPIKey = apiKey
	}
	if apiKey := os.Getenv("TAVILY_API_KEY"); apiKey != "" {
		keys.TavilyAPIKey = apiKey
	}
	if apiKey := os.Getenv("GITHUB_API_KEY"); apiKey != "" {
		keys.GitHubAPIKey = apiKey
	}
	return keys
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

func (a *App) beforeClose(ctx context.Context) bool {
	if a.exiting.Load() {
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
	if a.ctx == nil {
		return
	}

	if !a.exiting.CompareAndSwap(false, true) {
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
					"message": "正在卸载模型...",
				})
				if err := srv.Stop(); err != nil {
					log.Printf("graceful exit: stop server failed: %v", err)
				}
				srv.CloseJob()
				runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]interface{}{
					"stage":   "server_stopped",
					"message": "模型已卸载，显存已释放",
				})
			}

			if a.ragVS != nil {
				runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]interface{}{
					"stage":   "closing_rag",
					"message": "正在关闭知识库...",
				})
				if err := a.ragVS.Close(); err != nil {
					log.Printf("graceful exit: close RAG vector store: %v", err)
				}
			}

			if a.db != nil {
				runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]interface{}{
					"stage":   "closing_db",
					"message": "正在关闭数据库...",
				})
				if err := a.db.Close(); err != nil {
					log.Printf("graceful exit: close database: %v", err)
				}
			}

			runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]interface{}{
				"stage":   "done",
				"message": "清理完成",
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
		log.Printf("[preset] global defaults: ctx-size=%d", sp.ContextSize)
	}

	content := llm.GeneratePreset(presets, globalDefaults)
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

// emitSwitchProgress emits a progress event for model switch.
func (a *App) emitSwitchProgress(stage, targetModel string) {
	runtime.EventsEmit(a.ctx, "server:switchProgress", map[string]interface{}{
		"stage":       stage,
		"targetModel": targetModel,
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

	// 4. Emit switching status and initial progress
	a.emitSwitchingStatus(modelName)
	a.emitSwitchProgress("unloading", modelName)

	// 5. Simulate unloading stage for better UX
	time.Sleep(300 * time.Millisecond)

	// 6. Emit loading progress
	a.emitSwitchProgress("loading", modelName)

	// 7. Load new model (router handles LRU unloading automatically)
	loadErr := a.client.LoadModel(a.ctx, modelName)
	if loadErr != nil {
		if isAlreadyRunningError(loadErr) {
			// Model already running — treat as success
			log.Printf("[router] model %s is already running, treating as switch success", modelName)
			a.emitSwitchProgress("done", modelName)
			a.currentModelMu.Lock()
			a.currentModelName = modelName
			a.currentModelMu.Unlock()
		} else {
			log.Printf("[router] model switch failed: %v", loadErr)
			a.emitSwitchProgress("failed", modelName)
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
			a.emitSwitchProgress("failed", modelName)
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

		// Wait briefly for mmproj and other post-load initialization
		propsCtx, propsCancel := context.WithTimeout(a.ctx, 15*time.Second)
		defer propsCancel()
		for i := 0; i < 10; i++ {
			if _, propsErr := a.client.GetServerProps(propsCtx, modelName); propsErr == nil {
				break
			}
			select {
			case <-propsCtx.Done():
				break
			case <-time.After(500 * time.Millisecond):
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

	// 9. Emit waiting stage
	a.emitSwitchProgress("waiting", modelName)

	// 10. Detect model architecture
	a.service.SetDetectedModelName(modelName)
	if err := a.service.DetectModelArchitectureForModel(modelName); err != nil {
		log.Printf("[router] detect model architecture after switch: %v", err)
		runtime.EventsEmit(a.ctx, "server:status", llm.ServerStatus{
			Running:      true,
			CurrentModel: modelName,
			Error:        fmt.Sprintf("模型架构检测失败: %v", err),
		})
	}

	// 11. Emit done stage
	a.emitSwitchProgress("done", modelName)

	// 12. Emit success status BEFORE clearing switching state
	// This ensures the frontend receives the success event before
	// WatchWithCallback can emit a stale status
	a.emitSwitchSuccess(modelName)
	a.serverReady.Store(true)

	// 11. Clear switching state
	a.isSwitching.Store(false)
	a.switchingToMu.Lock()
	a.switchingTo = ""
	a.switchingToMu.Unlock()

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
