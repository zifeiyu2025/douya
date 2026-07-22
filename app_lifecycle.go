package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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

	// 初始化应用级 rootCtx：生命周期贯穿整个 App 运行期，
	// shutdownInternal 会 rootCancel() 通知所有被跟踪的长生命周期 goroutine 退出。
	// 必须在任何 trackedGo 调用之前完成初始化。
	a.rootCtx, a.rootCancel = context.WithCancel(context.Background())

	llm.KillOrphanLlamaServers()

	a.hwInfo = system.DetectHardware()

	var err error

	cfgPath := filepath.Join(appDir(), "config.json")
	loadedCfg, err := config.Load(cfgPath)
	if err != nil {
		zlog.Error().Err(err).Msg("load config failed")
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "配置加载失败",
			Message: fmt.Sprintf("加载配置文件失败: %v", err),
		})
		// startup 中的 return 只会结束启动函数，必须主动退出应用。
		// 见 forceQuit 注释：需先设置 exiting 标志绕过 beforeClose 拦截，再退出 Wails + 托盘。
		a.forceQuit()
		return
	}
	a.setConfig(loadedCfg)

	if checkResult := a.validatePaths(); checkResult.HasRuntimeIssues() || checkResult.HasModelIssues() {
		// ===== 分层提示：runtime 问题（致命）和 models 问题（警告）分开处理 =====

		// 1. runtime 目录不完整 —— 致命错误，必须终止启动
		if checkResult.HasRuntimeIssues() {
			var msg strings.Builder
			msg.WriteString("⚠️ AI 推理引擎（runtime 目录）不完整，无法启动应用。\n\n")
			msg.WriteString("缺失的文件：\n")
			for _, p := range checkResult.RuntimeMissing {
				msg.WriteString("  ❌ " + p + "\n")
			}
			msg.WriteString("\n")
			msg.WriteString("【这是什么】\n")
			msg.WriteString("runtime 目录包含 AI 推理引擎（llama-server.exe）和配套的 DLL 动态库，\n")
			msg.WriteString("是应用运行的核心依赖，缺失任何一个文件都无法启动。\n\n")
			msg.WriteString("【如何修复】\n")
			msg.WriteString("1. 从官方发布包获取完整的 runtime 目录：\n")
			msg.WriteString("   https://github.com/ggml-org/llama.cpp/releases\n")
			msg.WriteString("   下载 Windows CUDA 版本（如 llama-bXXXX-bin-win-cuda-cu13.x.zip）\n")
			msg.WriteString("2. 解压后将以下文件放入 runtime 目录：\n")
			msg.WriteString("   - llama-server.exe（主程序）\n")
			msg.WriteString("   - llama.dll / ggml.dll 等（核心引擎库）\n")
			msg.WriteString("   - cudart64_13.dll / cublas64_13.dll 等（CUDA 加速库，NVIDIA 显卡才需要）\n\n")
			fmt.Fprintf(&msg, "【应用根目录】\n%s", appDir())

			zlog.Error().Strs("runtime_missing", checkResult.RuntimeMissing).Msg("[startup] runtime incomplete, aborting")
			_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
				Type:    runtime.ErrorDialog,
				Title:   "AI 推理引擎不完整",
				Message: msg.String(),
			})
			// 通知前端启动失败
			runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
				Running:    false,
				ModelReady: false,
				Error:      "runtime 目录不完整，请检查 AI 推理引擎文件",
			})
			// startup 中的 return 只会结束启动函数，必须主动退出应用。
			// 见 forceQuit 注释：需先设置 exiting 标志绕过 beforeClose 拦截，再退出 Wails + 托盘。
			a.forceQuit()
			return
		}

		// 2. models 目录为空或不存在 —— 致命错误，必须终止启动
		// 生活类比：车没有引擎再豪华也跑不起来——模型文件就是 AI 的引擎，
		// 缺了它应用启动了也没意义，不如直接停在车库里等用户把引擎装好。
		if checkResult.HasModelIssues() {
			var msg strings.Builder
			msg.WriteString("⚠️ 模型目录为空，无法启动应用。\n\n")
			if checkResult.ModelsDirMissing {
				fmt.Fprintf(&msg, "模型目录不存在：%s\n", checkResult.ModelsDir)
			} else {
				fmt.Fprintf(&msg, "模型目录：%s\n", checkResult.ModelsDir)
				msg.WriteString("该目录下未找到任何 .gguf 模型文件。\n")
			}
			msg.WriteString("\n")
			msg.WriteString("【这是什么】\n")
			msg.WriteString("模型文件（.gguf）是 AI 的「大脑」，没有模型文件应用无法进行对话。\n")
			msg.WriteString("应用本身不内置模型，需要您自行下载。\n\n")
			msg.WriteString("【如何获取模型】\n")
			msg.WriteString("1. 访问 HuggingFace 搜索 GGUF 格式的模型：\n")
			msg.WriteString("   https://huggingface.co/models?other=gguf\n")
			msg.WriteString("2. 推荐的入门模型（Q4_K_M 量化，平衡速度与效果）：\n")
			msg.WriteString("   - Qwen3-8B（通义千问，中文友好）\n")
			msg.WriteString("   - Gemma-3-4B（轻量，适合低配机器）\n")
			msg.WriteString("3. 下载 .gguf 文件后放入上面的模型目录\n")
			msg.WriteString("4. 重新启动应用，在顶部模型下拉框选择刚放入的模型\n\n")
			msg.WriteString("【提示】\n")
			msg.WriteString("点击「确定」后应用将退出，请按上述步骤准备好模型文件后再次启动。")

			zlog.Error().Str("models_dir", checkResult.ModelsDir).
				Bool("dir_missing", checkResult.ModelsDirMissing).
				Bool("empty", checkResult.ModelsEmpty).
				Msg("[startup] models directory empty, aborting")
			_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
				Type:    runtime.ErrorDialog,
				Title:   "模型目录为空，无法启动应用",
				Message: msg.String(),
			})
			// 通知前端启动失败
			runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
				Running:    false,
				ModelReady: false,
				Error:      "模型目录为空，请下载 .gguf 模型文件后放入 models 目录",
			})
			// startup 中的 return 只会结束启动函数，必须主动退出应用。
			// 见 forceQuit 注释：需先设置 exiting 标志绕过 beforeClose 拦截，再退出 Wails + 托盘。
			a.forceQuit()
			return
		}
	}

	dbPath := filepath.Join(appDir(), "data", "douya.db")

	// 加载加密密钥，用于对话内容和 API Key 等敏感数据的加密存储
	// 注意：若密钥文件已损坏（长度不为 32 字节），LoadOrCreateKey 会返回错误而不是静默覆盖，
	// 因为覆盖会导致所有用旧密钥加密的历史数据永久无法解密。此时必须阻止启动，由用户手动处理。
	keyPath := filepath.Join(appDir(), "data", ".enc_key")
	a.encKey, err = secrets.LoadOrCreateKey(keyPath)
	if err != nil {
		zlog.Error().Err(err).Msg("[startup] load encryption key failed")
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "加密密钥加载失败",
			Message: fmt.Sprintf("加载加密密钥失败：\n%v\n\n请按上述提示处理后重新启动应用。", err),
		})
		// startup 中的 return 只会结束启动函数，必须主动退出应用。
		// 见 forceQuit 注释：需先设置 exiting 标志绕过 beforeClose 拦截，再退出 Wails + 托盘。
		a.forceQuit()
		return
	}

	a.db, err = store.Init(dbPath, a.encKey)
	if err != nil {
		zlog.Error().Err(err).Msg("init database failed")
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "数据库初始化失败",
			Message: fmt.Sprintf("初始化数据库失败: %v", err),
		})
		// startup 中的 return 只会结束启动函数，必须主动退出应用。
		// 见 forceQuit 注释：需先设置 exiting 标志绕过 beforeClose 拦截，再退出 Wails + 托盘。
		a.forceQuit()
		return
	}

	if raw, rawErr := config.LoadRaw(cfgPath); rawErr == nil {
		if se, ok := raw["search_engines"]; ok {
			if seMap, ok := se.(map[string]any); ok {
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
						_ = setFn("search_ollama_api_key", fmt.Sprintf("%v", v))
						migrated = true
					}
				}
				if v, ok := seMap["tavily_api_key"]; ok && v != "" {
					if existing := getFn("search_tavily_api_key"); existing == "" {
						_ = setFn("search_tavily_api_key", fmt.Sprintf("%v", v))
						migrated = true
					}
				}
				if migrated {
					zlog.Info().Msg("[startup] migrated search API keys from config.json to database")
					// 保存前校验，失败记录日志但不阻塞保存（避免阻塞搜索引擎迁移功能）
					if err := a.getConfig().Validate(); err != nil {
						zlog.Warn().Err(err).Msg("[startup] 配置校验失败（搜索引擎迁移），仍保存")
					}
					if err := config.Save(cfgPath, a.getConfig()); err != nil {
						zlog.Warn().Err(err).Msg("[startup] 保存配置失败（搜索引擎迁移）")
					}
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

	a.service = chat.NewService(a.client, searchChain, a.db, cfg, secrets.NewCipher(a.encKey), appDir())
	a.service.SetContext(ctx)

	// Initialize RAG (Badger-backed vector store + LLM embedder)
	ragDir := filepath.Join(appDir(), "data", "rag")
	ragVS, err := rag.NewVectorStore(ragDir)
	if err != nil {
		zlog.Error().Err(err).Msg("[startup] RAG vector store init failed (RAG disabled)")
	} else {
		a.ragVS = ragVS
		a.ragDS = rag.NewDocumentStore(ragVS)
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

	// 创建日志 channel 和消费者 goroutine（trackedGo 跟踪）
	// 生活类比：就像一个邮筒（logChan），邮递员（llama-server）把每封信（日志行）投进邮筒，
	// 后台有一个邮局职员（消费者 goroutine）负责把信件转交给收件人（前端）。
	// 用单个职员而不是每封信都派一个职员（原 go func 实现），避免 goroutine 泛滥。
	a.logChan = make(chan string, 1024)
	a.trackedGo(func() {
		for {
			select {
			case <-a.rootCtx.Done():
				// shutdown 信号：排空剩余日志后退出
				// 不显式 close(logChan)：SetOnLog 可能在 srv.Stop 之前仍被调用，
				// 关闭 channel 会导致 send on closed channel panic。
				// rootCtx.Done() 已足够让消费者退出，channel 随 App 一起被 GC。
				for {
					select {
					case l := <-a.logChan:
						if a.ctx != nil {
							runtime.EventsEmit(a.ctx, "server:log", l)
						}
					default:
						return
					}
				}
			case l := <-a.logChan:
				if a.ctx != nil {
					runtime.EventsEmit(a.ctx, "server:log", l)
				}
			}
		}
	})

	a.serverMu.Lock()
	a.server = llm.NewServer(a.buildServerConfig())
	// 设置 llama-server 日志实时推送到前端控制台（exec.Cmd 回调模式使用）
	// 日志通过 logChan 投递给消费者 goroutine，由其统一 EventsEmit，避免每行日志创建 goroutine
	a.server.SetOnLog(func(line string) {
		// 非阻塞投递到 logChan：缓冲区满时丢弃，避免阻塞 llama-server 输出
		select {
		case a.logChan <- line:
		default:
			// 缓冲区满，丢弃日志（极端情况：日志产生速度远超消费速度）
		}
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

	// 注：此处未使用 trackedGo，因为该 goroutine 为短生命周期且已有 defer recover()，
	// 完成后即退出，无需 ctx 取消。见安全审查 #26。
	go func() {
		// 防止 panic 导致整个进程崩溃（启动清理涉及 DB 操作和消息解密，可能 panic）
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[startup] CleanupAbnormalConversations panic")
			}
		}()
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

			runtime.EventsEmit(ctx, "chat:abnormal_cleanup", map[string]any{
				"count":   len(removed),
				"removed": removed,
			})
		}
	}()

	// startServerAndWatch 是一次性启动流程（同步执行模型检测/加载后返回），
	// 内部会通过 trackedGo 启动 watcher/health 等长生命周期 goroutine。
	// 此处不直接 trackedGo：它使用 Wails ctx 而非 rootCtx，且 shutdown 不必等待模型加载完成。
	go a.startServerAndWatch(a.server, ctx)
}

// shutdownInternal 是合并后的统一关闭逻辑，由 shutdown 和 PrepareShutdown 复用。
// stopOnce.Do 保证幂等：无论调用多少次，关闭逻辑只执行一次。
//
// 资源释放顺序（生活类比：下班关店流程）：
//  1. 停止生成（让正在进行的对话停下来）
//  2. rootCancel（广播"下班了"，通知被跟踪的长生命周期 goroutine 退出）
//  3. watchCancel（取消 server watch ctx，watcher/health 监听随之退出）
//  4. g.Wait（在门口签到表前等所有被跟踪 goroutine 出来，避免锁门时还有人留在里面）
//  5. srv.Stop + CloseJob（关闭 llama-server 进程）
//  6. ragVS.Close（关闭知识库向量存储）
//  7. db.Close（关闭数据库）
//
// waitForServerStop 当前实现下不影响行为（srv.Stop 本身为同步阻塞），
// 保留该参数以匹配任务规约签名，并为未来"异步停止"差异预留扩展点。
func (a *App) shutdownInternal(ctx context.Context, waitForServerStop bool) {
	a.stopOnce.Do(func() {
		if a.service != nil {
			a.service.StopGeneration()
		}

		// 1. 取消应用级 rootCtx，通知所有被跟踪的长生命周期 goroutine 退出
		if a.rootCancel != nil {
			a.rootCancel()
		}

		// 2. 取消 server watch ctx（watcher/health 监听依赖此 ctx 退出）
		a.serverMu.Lock()
		if a.watchCancel != nil {
			a.watchCancel()
			a.watchCancel = nil
		}
		srv := a.server
		a.serverMu.Unlock()

		// 3. 等待被跟踪的 goroutine 退出，确保关闭底层资源后不会被访问
		a.g.Wait()

		// 4. 停止 llama-server 进程（同步：taskkill + 3s 超时 + force kill）
		if srv != nil {
			if err := srv.Stop(); err != nil {
				zlog.Error().Err(err).Msg("shutting down: stop server failed")
			}
			srv.CloseJob()
		}

		// 5. 关闭 RAG 向量库
		if a.ragVS != nil {
			if err := a.ragVS.Close(); err != nil {
				zlog.Error().Err(err).Msg("shutting down: close RAG vector store failed")
			}
		}

		// 6. 关闭数据库
		if a.db != nil {
			if err := a.db.Close(); err != nil {
				zlog.Error().Err(err).Msg("shutting down: close database failed")
			}
		}

		_ = waitForServerStop // 当前实现统一同步停止，保留参数以匹配任务规约签名
	})
}

func (a *App) shutdown(ctx context.Context) {
	a.shutdownInternal(ctx, false)
}

// PrepareShutdown 由前端在准备退出时调用（Wails 绑定，无参）。
// 合并后行为与 shutdown 一致：执行完整资源释放，后续 OnShutdown 因 stopOnce 而成为 no-op。
func (a *App) PrepareShutdown() {
	a.shutdownInternal(context.Background(), true)
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

// forceQuit 用于启动阶段遇到致命错误时强制退出应用。
//
// 与 GracefulExit 的区别：此时 db / server / ragVS 等资源尚未初始化，
// 无需执行资源清理流程，直接退出即可。
//
// 为什么需要先设置 exiting 标志：
//
//	runtime.Quit 会触发 OnBeforeClose → beforeClose，
//	而 beforeClose 在 exiting 为 false 时会返回 true 阻止关闭
//	（根据 CloseAction 配置，可能只是隐藏窗口到托盘）。
//	必须先将 exiting 置为 true，beforeClose 才会放行，Wails 进程才能真正退出。
//
// 为什么还需要 systray.Quit：
//
//	systray.Run 在独立 goroutine 中运行（见 main.go），
//	runtime.Quit 只关闭 Wails 窗口，不影响托盘。
//	不调用 systray.Quit 会导致托盘图标残留，用户仍可操作菜单。
func (a *App) forceQuit() {
	a.exiting.Store(true)
	runtime.Quit(a.ctx)
	systray.Quit()
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
		a.clearFileCache()
		return true
	default: // "ask" 或未设置
		runtime.WindowHide(ctx)
		a.hidden.Store(true)
		a.clearFileCache()
		return true
	}
}

// clearFileCache 清空本地文件 LRU 缓存，释放内存。
// 在窗口最小化到托盘时调用——用户不查看图片时无需占用 50MB 缓存。
func (a *App) clearFileCache() {
	if a.fileLoader != nil {
		a.fileLoader.ClearCache()
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
	// 采用"复制→修改副本→替换指针"模式，避免直接修改 getConfig() 返回的指针
	old := a.getConfig()
	newCfg := *old
	newCfg.CloseAction = action
	a.setConfig(&newCfg)
	// 保存前校验，失败记录日志但不阻塞保存（避免阻塞关闭动作设置功能）
	if err := newCfg.Validate(); err != nil {
		zlog.Warn().Err(err).Msg("[SetCloseAction] 配置校验失败，仍保存")
	}
	_ = config.Save(filepath.Join(appDir(), "config.json"), &newCfg)
}

func (a *App) GracefulExit() {
	if !a.tryStartExit() {
		return
	}

	runtime.WindowShow(a.ctx)
	a.hidden.Store(false)

	go func() {
		// L-1：优雅关闭流程涉及 DB/进程/事件多类资源，panic 会中断关闭导致资源泄漏
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[shutdown] GracefulExit panic, exit forced")
			}
		}()
		a.stopOnce.Do(func() {
			if a.service != nil {
				a.service.StopGeneration()
			}
			runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]any{
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
				runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]any{
					"stage":   "stopping_server",
					"message": "正在关闭服务...",
				})
				if err := srv.Stop(); err != nil {
					zlog.Error().Err(err).Msg("graceful exit: stop server failed")
				}
				srv.CloseJob()
			}

			if a.ragVS != nil {
				runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]any{
					"stage":   "closing_rag",
					"message": "正在关闭知识库...",
				})
				if err := a.ragVS.Close(); err != nil {
					zlog.Error().Err(err).Msg("graceful exit: close RAG vector store failed")
				}
			}

			if a.db != nil {
				runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]any{
					"stage":   "closing_db",
					"message": "正在关闭数据库...",
				})
				if err := a.db.Close(); err != nil {
					zlog.Error().Err(err).Msg("graceful exit: close database failed")
				}
			}

			runtime.EventsEmit(a.ctx, "shutdown:progress", map[string]any{
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

	// 托盘菜单 goroutine：长期运行的 UI 循环，仅调用 ShowWindow/GracefulExit，
	// 不访问 db/ragVS/server 等被 shutdown 管理的资源，故不纳入 trackedGo 跟踪。
	// 通过 mQuit 点击或 systray.Quit() 退出，已有 recover 保护。
	go func() {
		// L-1：托盘菜单 goroutine 长期运行，panic 会导致菜单失效
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[systray] menu goroutine panic")
			}
		}()
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

	// 保留原扩展名
	ext := strings.ToLower(filepath.Ext(srcPath))

	// 计算源文件内容的 SHA256 哈希，作为去重依据。
	// 生活类比：给图片盖一个"身份证号"，内容相同则号码相同。
	// 用哈希前16位作为文件名，相同内容的图片只会保存一份。
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return "", fmt.Errorf("打开源文件失败: %w", err)
	}

	hasher := sha256.New()
	if _, err := io.Copy(hasher, srcFile); err != nil {
		srcFile.Close()
		return "", fmt.Errorf("计算文件哈希失败: %w", err)
	}
	srcFile.Close()

	hashHex := hex.EncodeToString(hasher.Sum(nil))[:16]
	dstName := "bg_" + hashHex + ext
	dstPath := filepath.Join(imagesDir, dstName)

	// 若目标文件已存在，说明内容相同的图片已保存过，直接复用，不再写入
	if _, statErr := os.Stat(dstPath); statErr == nil {
		if rel, relErr := filepath.Rel(appDir(), dstPath); relErr == nil {
			return filepath.ToSlash(rel), nil
		}
	}

	// 目标文件不存在，复制源文件到目标位置（保留原文件）
	srcFile, err = os.Open(srcPath)
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
