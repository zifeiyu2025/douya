package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/rag"
	"douya/internal/secrets"
	"douya/internal/store"
	"douya/internal/system"

	"fyne.io/systray"
	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	llm.KillOrphanLlamaServers()

	a.hwInfo = system.DetectHardware()

	var err error

	cfgPath := filepath.Join(appDir(), "config.json")
	loadedCfg, err := config.Load(cfgPath)
	if err != nil {
		zlog.Error().Err(err).Msg("load config failed")
		runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "配置加载失败",
			Message: fmt.Sprintf("加载配置文件失败: %v", err),
		})
		return
	}
	a.setConfig(loadedCfg)

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
		// 关键文件缺失，终止启动流程
		// 尽力通知前端（前端可能还未注册监听器，但轮询机制会恢复状态）
		runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
			Running:    false,
			ModelReady: false,
			Error:      "关键文件缺失，请检查 runtime/ 和 models/ 目录",
		})
		zlog.Error().Strs("missing_paths", missingPaths).Msg("[startup] critical files missing, aborting startup")
		return
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
				if migrated {
					zlog.Info().Msg("[startup] migrated search API keys from config.json to database")
					config.Save(cfgPath, a.getConfig())
				}
			}
		}
	}

	if err := a.generatePresetFile(); err != nil {
		zlog.Error().Err(err).Msg("[startup] generate preset file failed")
	}

	cfg := a.getConfig()
	a.client = llm.NewClient(cfg.APIBase, a.getServerAPIKey())

	searchChain := a.buildSearchChain()

	a.service = chat.NewService(a.client, searchChain, a.db, cfg, a.encKey, appDir())
	a.service.SetContext(ctx)

	// Initialize RAG (Badger-backed vector store + LLM embedder)
	ragDir := filepath.Join(appDir(), "data", "rag")
	ragVS, err := rag.NewVectorStore(ragDir)
	if err != nil {
		zlog.Error().Err(err).Msg("[startup] RAG vector store init failed (RAG disabled)")
	} else {
		a.ragVS = ragVS
		a.ragDS = rag.NewDocumentStore(ragVS.DB())
		// 嵌入模型：优先使用专用嵌入模型，否则使用当前聊天模型
		embedModel := cfg.EmbeddingModel
		if embedModel != "" {
			embedModel = resolvePath(embedModel)
		}
		if embedModel == "" {
			a.currentModelMu.RLock()
			embedModel = a.currentModelName
			a.currentModelMu.RUnlock()
		}
		embedder := &rag.ClientEmbedder{Client: a.client}
		embedder.SetModel(embedModel)
		// 当专用嵌入模型为空时，动态获取当前聊天模型名
		embedder.SetCurrentModelFn(func() string {
			a.currentModelMu.RLock()
			defer a.currentModelMu.RUnlock()
			return a.currentModelName
		})
		a.ragEmbedder = embedder
		collection := cfg.RAGActiveKB
		if collection == "" {
			collection = "default"
		}
		ragEnabled := cfg.RAGEnabled
		a.service.SetRAG(ragVS, a.ragDS, embedder, collection, ragEnabled)
		zlog.Info().Str("dir", ragDir).Str("collection", collection).Str("embed_model", embedModel).Bool("enabled", ragEnabled).Msg("[startup] RAG initialized")
	}

	a.serverMu.Lock()
	a.server = llm.NewServer(a.buildServerConfig())
	// 设置 llama-server 日志实时推送到前端控制台（exec.Cmd 回退模式使用）
	a.server.SetOnLog(func(line string) {
		// 异步推送，避免阻塞 llama-server 输出
		go func(l string) {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "server:log", l)
			}
		}(line)
	})
	// 设置终端原始字节流推送到前端 xterm.js（ConPTY 模式使用）
	// 数据已批量合并（50ms 窗口），直接同步发送即可
	a.server.SetOnTerminalData(func(data []byte) {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "server:terminal", data)
		}
	})
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
	// 根据 close_action 配置决定行为
	cfg := a.getConfig()
	switch cfg.CloseAction {
	case "exit":
		go a.GracefulExit()
		return true // 阻止默认关闭，由 GracefulExit 处理
	case "tray":
		runtime.WindowHide(ctx)
		a.hidden.Store(true)
		return true
	default: // "ask" 或未设置
		runtime.WindowHide(ctx)
		a.hidden.Store(true)
		return true
	}
}

func (a *App) ShowWindow() {
	if a.ctx != nil {
		runtime.WindowShow(a.ctx)
		a.hidden.Store(false)
	}
}

// HandleCloseRequest 处理前端关闭按钮点击，返回 "tray" 或 "exit" 表示应执行的操作
func (a *App) HandleCloseRequest() string {
	cfg := a.getConfig()
	switch cfg.CloseAction {
	case "exit":
		return "exit"
	case "tray":
		return "tray"
	default: // "ask" 或未设置
		return "ask"
	}
}

// SetCloseAction 设置关闭行为并持久化
func (a *App) SetCloseAction(action string) {
	cfg := a.getConfig()
	cfg.CloseAction = action
	a.setConfig(cfg)
	_ = config.Save(filepath.Join(appDir(), "config.json"), cfg)
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
	// 用户取消了选择
	if filePath == "" {
		return "", nil
	}

	// 解析为绝对路径，便于后续比较与复制
	srcPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("解析源文件路径失败: %w", err)
	}

	// 目标目录：appDir()/data/images/
	imagesDir := filepath.Join(appDir(), "data", "images")

	// 若原文件已经在 images 目录下，无需复制，直接返回相对路径
	if absDir, absErr := filepath.Abs(imagesDir); absErr == nil {
		if strings.HasPrefix(srcPath, absDir+string(filepath.Separator)) {
			if rel, relErr := filepath.Rel(appDir(), srcPath); relErr == nil {
				return filepath.ToSlash(rel), nil
			}
		}
	}

	// 创建目标目录（如不存在）
	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		return "", fmt.Errorf("创建图片目录失败: %w", err)
	}

	// 生成目标文件名：保留原扩展名，用时间戳避免冲突
	ext := strings.ToLower(filepath.Ext(srcPath))
	timestamp := time.Now().Format("20060102_150405")
	dstName := "bg_" + timestamp + ext
	dstPath := filepath.Join(imagesDir, dstName)
	// 同一秒内选择了多张图片时，追加序号避免覆盖
	for i := 1; ; i++ {
		if _, statErr := os.Stat(dstPath); os.IsNotExist(statErr) {
			break
		}
		dstName = fmt.Sprintf("bg_%s_%d%s", timestamp, i, ext)
		dstPath = filepath.Join(imagesDir, dstName)
	}

	// 复制源文件到目标文件（保留原文件）
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("打开源文件失败: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return "", fmt.Errorf("创建目标文件失败: %w", err)
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		// 复制失败时清理已创建的空文件
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("复制图片失败: %w", err)
	}
	if err := dstFile.Close(); err != nil {
		return "", fmt.Errorf("保存图片失败: %w", err)
	}

	// 返回相对路径，并用正斜杠（前端会用 URL 访问，Windows 下分隔符需转为 /）
	rel, err := filepath.Rel(appDir(), dstPath)
	if err != nil {
		return "", fmt.Errorf("计算相对路径失败: %w", err)
	}
	return filepath.ToSlash(rel), nil
}

// SelectLoraFile 打开文件对话框选择 LoRA 适配器文件
func (a *App) SelectLoraFile() (string, error) {
	filePath, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "选择 LoRA 适配器",
		Filters: []runtime.FileFilter{
			{
				DisplayName: "LoRA 适配器文件",
				Pattern:     "*.gguf;*.bin;*.safetensors",
			},
			{
				DisplayName: "所有文件",
				Pattern:     "*.*",
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("选择文件失败: %w", err)
	}
	return filePath, nil
}
