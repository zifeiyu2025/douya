package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
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

	// 初始化应用级 rootCtx：生命周期贯穿整个 App 运行期，
	// shutdownInternal 会 rootCancel() 通知所有被跟踪的长生命周期 goroutine 退出。
	// 必须在任何 trackedGo 调用之前完成初始化。
	a.rootCtx, a.rootCancel = context.WithCancel(context.Background())

	a.cleanupOrphanProcesses()
	a.initHardware()

	cfgPath, err := a.loadAndValidateConfig(ctx)
	if err != nil {
		a.forceQuit()
		return
	}

	runtimeDir, _ := a.ensureDirectories()

	if a.installBackend(ctx, runtimeDir) {
		return
	}

	a.handleMissingModels(ctx)

	dbPath := filepath.Join(appDir(), "data", "douya.db")

	if err := a.loadSecrets(ctx); err != nil {
		a.forceQuit()
		return
	}

	if err := a.initDatabase(ctx, dbPath); err != nil {
		a.forceQuit()
		return
	}

	// 提前创建 chat.Service（用 nil client/searchChain 占位），供后续 getServerAPIKey /
	// buildSearchChain 等通过 service 访问 settings，避免 App 层直接 import store（QUAL-3）。
	// 后续 UpdateClient / UpdateSearchChain 会填充真实依赖。
	a.service = chat.NewService(nil, nil, a.db, a.getConfig(), secrets.NewCipher(a.encKey), appDir())
	a.service.SetHostContext(ctx)
	a.service.SetEventPublisher(newWailsChatEventPublisher(ctx))

	a.migrateSearchEngines(cfgPath)

	a.buildService(ctx)

	a.cleanupOrphanSessions(ctx)

	// startServerAndWatch 是一次性启动流程（同步执行模型检测/加载后返回），
	// 内部会通过 trackedGo 启动 watcher/health 等长生命周期 goroutine。
	// 此处不直接 trackedGo：它使用 Wails ctx 而非 rootCtx，且 shutdown 不必等待模型加载完成。
	go a.startServerAndWatch(a.server, ctx)
}

// ===== startup 子函数：将原超长 startup 拆分为职责单一的子函数 =====

// cleanupOrphanProcesses 清理上次进程残留的孤儿 llama-server。
// 生活类比：开店前先清理前一天遗留的垃圾，避免影响今天的运营。
func (a *App) cleanupOrphanProcesses() {
	llm.KillOrphanLlamaServers()
}

// initHardware 检测硬件信息（CPU/GPU/内存等），供后续选择推理后端使用。
func (a *App) initHardware() {
	a.hwInfo = system.DetectHardware()
}

// loadAndValidateConfig 加载配置文件，失败时弹窗提示并返回 error。
// 返回 cfgPath 供后续迁移使用，配置已通过 setConfig 缓存到 App。
func (a *App) loadAndValidateConfig(ctx context.Context) (string, error) {
	cfgPath := filepath.Join(appDir(), "config.json")
	loadedCfg, err := config.Load(cfgPath)
	if err != nil {
		zlog.Error().Err(err).Msg("load config failed")
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "配置加载失败",
			Message: fmt.Sprintf("加载配置文件失败: %v", err),
		})
		return "", err
	}
	a.setConfig(loadedCfg)
	return cfgPath, nil
}

// ensureDirectories 确保 models 和 runtime 目录存在（不存在则自动创建）。
// 返回 runtimeDir 和 modelsDir 路径，供后续流程使用。
// 生活类比：开门营业前先确保仓库（runtime）和展厅（models）建好，
// 即使是空仓也能让后续流程正常运转（后端按需下载、模型稍后放入）。
func (a *App) ensureDirectories() (runtimeDir, modelsDir string) {
	runtimeDir = filepath.Join(appDir(), "runtime")
	modelsDir = filepath.Join(appDir(), "models")
	for _, dir := range []string{runtimeDir, modelsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			zlog.Warn().Err(err).Str("dir", dir).Msg("[startup] 创建目录失败")
		}
	}
	return
}

// installBackend 解析并安装推理后端，处理 runtime 缺失时的下载弹窗。
// 返回 shouldReturn：true 表示 startup 应直接返回（用户取消下载或已触发异步下载）。
//
// 流程：
//  1. 根据硬件和配置解析 auto 为具体后端（如 cuda/hip/cpu）
//  2. EnsureBackendInstalled 确保后端已解压到 runtime 目录（幂等：已安装则直接返回路径）
//  3. 如果安装失败，尝试回退到 CPU 后端
//  4. 将解析结果缓存到 App 结构体，供 validatePaths 和 buildServerConfig 复用
//  5. 若 runtime 缺失，弹窗询问用户是否下载；用户选「是」则异步下载并返回 true
func (a *App) installBackend(ctx context.Context, runtimeDir string) bool {
	cfg := a.getConfig()
	// P3 改进：使用带运行时预校验的解析函数，auto 模式下优先选择已安装的后端，
	// 避免推断出未下载的后端（如 Vulkan）后走下载流程（原实现会失败再回退 CPU）
	resolvedBackend := llm.ResolveBackendTypeWithRuntime(a.hwInfo, cfg.BackendType, runtimeDir)
	serverPath, err := llm.EnsureBackendInstalled(resolvedBackend, runtimeDir, nil)
	if err != nil {
		zlog.Warn().Err(err).Str("backend", resolvedBackend.String()).Msg("[startup] 后端安装失败，尝试回退到 CPU")
		if resolvedBackend != llm.BackendCPU {
			fallbackPath, cpuErr := llm.EnsureBackendInstalled(llm.BackendCPU, runtimeDir, nil)
			if cpuErr != nil {
				// CPU 后端也失败：不终止启动，validatePaths 会报告缺失文件
				zlog.Error().Err(cpuErr).Msg("[startup] CPU 后端也安装失败，validatePaths 将报告缺失文件")
			} else {
				serverPath = fallbackPath
				resolvedBackend = llm.BackendCPU
			}
		}
	}

	a.resolvedBackend = resolvedBackend
	a.resolvedServerPath = serverPath

	// CUDA 后端：确保 cudart 包也已解压（幂等）
	// 主包已安装但 cudart 包未解压时，validatePaths 会检测到厂商 DLL 缺失，
	// 导致无限提示下载。此处主动解压已有的 cudart zip 包，避免重复下载。
	// 生活类比：电脑装好了但外设配件包还没拆，开机前先把配件包装好。
	if resolvedBackend == llm.BackendCUDA && serverPath != "" {
		if cudartErr := llm.EnsureCudartInstalled(runtimeDir, nil); cudartErr != nil {
			zlog.Warn().Err(cudartErr).Msg("[startup] cudart 包未安装，validatePaths 将报告缺失")
		}
	}

	// ===== 统一 runtime 完整性检测（合并原两处弹窗为一个） =====
	// 原逻辑中存在两处弹窗：serverPath 为空时弹一次，validatePaths 报告缺失时再弹一次。
	// 现合并为单次弹窗：只要 serverPath 为空 或 validatePaths 报告 runtime 缺失，统一询问用户。
	// 生活类比：提车前只做一次全面检查，发现问题一次性告知，不会先问"发动机没装要不要装"，
	// 紧接着又问"变速箱也缺要不要买"——那样会让顾客被反复打断。
	checkResult := a.validatePaths()
	needDownload := serverPath == "" || checkResult.HasRuntimeIssues()

	if !needDownload {
		return false
	}

	info := llm.GetBackendInfo(resolvedBackend)
	gpuName := "未知"
	if a.hwInfo != nil && a.hwInfo.GPUName != "" {
		gpuName = a.hwInfo.GPUName
	}

	// 构造缺失文件清单：validatePaths 有结果时用其清单，否则用通用提示
	var missingMsg strings.Builder
	if checkResult.HasRuntimeIssues() {
		for _, p := range checkResult.RuntimeMissing {
			missingMsg.WriteString("  ❌ ")
			missingMsg.WriteString(p)
			missingMsg.WriteString("\n")
		}
	} else {
		missingMsg.WriteString("  ❌ 推理引擎（llama-server.exe）及依赖文件缺失\n")
	}

	askMsg := fmt.Sprintf(
		"检测到您的显卡：%s\n"+
			"推荐后端：%s\n\n"+
			"runtime 目录缺少以下文件：\n%s\n"+
			"是否从 GitHub 自动下载并安装？\n"+
			"（来源：https://github.com/ggml-org/llama.cpp/releases）\n\n"+
			"点击「是」将在启动界面显示下载进度，完成后自动重启应用。\n"+
			"点击「否」将直接退出应用。",
		gpuName, info.DisplayName, missingMsg.String())

	dlResult, _ := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.QuestionDialog,
		Title:   "缺少推理后端，是否下载？",
		Message: askMsg,
		Buttons: []string{"是", "否"},
	})

	// 记录返回值用于调试（Wails MessageDialog 在不同 Windows 版本下返回值可能有编码差异）
	zlog.Info().Str("dlResult", dlResult).Msg("[startup] MessageDialog 返回值")

	// Windows 上 QuestionDialog 默认显示"是/否"按钮：
	//   - 点"是" → 下载（默认行为，也兼容"Yes"、"下载"等返回值）
	//   - 点"否" → 退出（明确匹配"否"、"No"、"退出"等）
	// 逻辑采用"白名单退出"：只有明确选择否定意图才退出，避免编码不匹配导致误退出
	if dlResult == "否" || dlResult == "No" || dlResult == "退出" || dlResult == "Cancel" {
		zlog.Info().Msg("[startup] 用户取消下载，退出应用")
		a.forceQuit()
		return true
	}

	// 用户选择「是」：异步下载+安装（带重试 3 次），startup 直接返回
	// 下载进度通过事件推送到前端，在启动动效中展示
	zlog.Info().Str("backend", resolvedBackend.String()).Msg("[startup] 用户选择从 GitHub 下载后端")

	// 通知前端进入下载阶段（splashScreen 将切换到 downloading 阶段并显示进度条）
	runtime.EventsEmit(ctx, "backend:downloadStart", map[string]any{
		"backend": resolvedBackend.String(),
		"name":    info.DisplayName,
	})

	a.resolvedBackend = resolvedBackend
	a.resolvedServerPath = ""

	// 异步下载+安装（CUDA 额外下载 cudart 包，失败重试最多 3 次）
	backendToDownload := resolvedBackend
	go func() {
		// 防止 panic 导致整个进程崩溃（下载涉及网络和文件 IO，可能 panic）
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[startup] 下载后端 goroutine panic")
				runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
					Backend: backendToDownload,
					Status:  "failed",
					Error:   fmt.Sprintf("下载后端发生内部错误：%v", r),
				})
			}
		}()
		if dlErr := a.downloadBackendWithRetry(backendToDownload, runtimeDir, 3); dlErr != nil {
			zlog.Error().Err(dlErr).Str("backend", backendToDownload.String()).Msg("[startup] 下载后端失败（已重试 3 次）")
			runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: backendToDownload,
				Status:  "failed",
				Error:   fmt.Sprintf("已重试 3 次仍失败：%v", dlErr),
			})
		}
	}()

	// startup 直接返回，跳过后续流程（数据库/llama-server 启动等）
	// 应用窗口正常显示，前端监听下载事件展示进度，完成后提示用户重启
	return true
}

// handleMissingModels 检查 models 目录，若为空则弹窗提示用户下载模型。
// 不阻塞启动，用户点击「确定」后继续进入界面。
//
// runtime 已完整时才检查 models，避免与 runtime 弹窗叠加。
func (a *App) handleMissingModels(ctx context.Context) {
	checkResult := a.validatePaths()
	if !checkResult.HasModelIssues() {
		return
	}

	var msg strings.Builder
	msg.WriteString("⚠️ 还没有可用的 AI 模型，暂时无法对话。\n\n")
	if checkResult.ModelsDirMissing {
		fmt.Fprintf(&msg, "模型目录（将自动创建）：%s\n", checkResult.ModelsDir)
	} else {
		fmt.Fprintf(&msg, "模型目录：%s\n", checkResult.ModelsDir)
		msg.WriteString("该目录下还没有 .gguf 模型文件。\n")
	}
	msg.WriteString("\n")
	msg.WriteString("【如何下载模型】\n")
	msg.WriteString("豆芽使用 GGUF 格式的模型文件，推荐从以下站点下载（国内访问快）：\n\n")
	msg.WriteString("1. ModelScope（魔搭社区，阿里出品，中文友好）\n")
	msg.WriteString("   https://www.modelscope.cn/\n")
	msg.WriteString("   搜索关键词：GGUF\n\n")
	msg.WriteString("2. HF 镜像（HuggingFace 国内镜像站）\n")
	msg.WriteString("   https://hf-mirror.com/\n")
	msg.WriteString("   搜索关键词：gguf\n\n")
	msg.WriteString("【推荐的入门模型】（选 Q4_K_M 量化，速度与效果均衡）\n")
	msg.WriteString("   - Qwen3-8B（通义千问，中文最强入门）\n")
	msg.WriteString("   - Gemma-3-4B（轻量，适合低配机器）\n")
	msg.WriteString("   - Llama-3.1-8B（Meta 出品，英文能力强）\n\n")
	msg.WriteString("【下载后如何使用】\n")
	msg.WriteString("   1. 下载 .gguf 文件（通常 3~6 GB）\n")
	msg.WriteString("   2. 将文件放入上面的模型目录\n")
	msg.WriteString("   3. 重启豆芽，在顶部模型下拉框选择即可\n\n")
	msg.WriteString("点击「确定」先进入界面，模型文件可以稍后再放入。")

	zlog.Warn().Str("models_dir", checkResult.ModelsDir).
		Bool("dir_missing", checkResult.ModelsDirMissing).
		Bool("empty", checkResult.ModelsEmpty).
		Msg("[startup] models directory empty, continuing startup")
	_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:    runtime.WarningDialog,
		Title:   "模型目录为空",
		Message: msg.String(),
	})
	runtime.EventsEmit(ctx, "server:status", llm.ServerStatus{
		Running:    false,
		ModelReady: false,
		Error:      "模型目录为空，请下载 .gguf 模型文件后放入 models 目录",
	})
}

// loadSecrets 加载加密密钥，用于对话内容和 API Key 等敏感数据的加密存储。
// 若密钥文件已损坏（长度不为 32 字节），返回 error 阻止启动——
// 因为覆盖会导致所有用旧密钥加密的历史数据永久无法解密，此时必须由用户手动处理。
func (a *App) loadSecrets(ctx context.Context) error {
	keyPath := filepath.Join(appDir(), "data", ".enc_key")
	key, err := secrets.LoadOrCreateKey(keyPath)
	if err != nil {
		zlog.Error().Err(err).Msg("[startup] load encryption key failed")
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "加密密钥加载失败",
			Message: fmt.Sprintf("加载加密密钥失败：\n%v\n\n请按上述提示处理后重新启动应用。", err),
		})
		return err
	}
	a.encKey = key
	return nil
}

// initDatabase 初始化数据库，失败时弹窗提示并返回 error。
func (a *App) initDatabase(ctx context.Context, dbPath string) error {
	db, err := store.Init(dbPath, a.encKey)
	if err != nil {
		zlog.Error().Err(err).Msg("init database failed")
		_, _ = runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
			Type:    runtime.ErrorDialog,
			Title:   "数据库初始化失败",
			Message: fmt.Sprintf("初始化数据库失败: %v", err),
		})
		return err
	}
	a.db = db
	return nil
}

// migrateSearchEngines 将 config.json 中的搜索引擎 API Key 迁移到数据库。
// 幂等：仅在数据库中不存在对应 key 时迁移。
func (a *App) migrateSearchEngines(cfgPath string) {
	raw, rawErr := config.LoadRaw(cfgPath)
	if rawErr != nil {
		return
	}
	se, ok := raw["search_engines"]
	if !ok {
		return
	}
	seMap, ok := se.(map[string]any)
	if !ok {
		return
	}
	migrated := false
	setFn := func(key, value string) error {
		return a.service.SetEncryptedSetting(key, value)
	}
	getFn := func(key string) string {
		v, _ := a.service.GetEncryptedSetting(key)
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

// initRAG 初始化 RAG（Badger-backed 向量存储 + LLM embedder）。
// 初始化失败时禁用 RAG 但不阻止启动。
func (a *App) initRAG(_ context.Context, cfg *config.Config) {
	ragDir := filepath.Join(appDir(), "data", "rag")
	ragVS, err := rag.NewVectorStore(ragDir)
	if err != nil {
		zlog.Error().Err(err).Msg("[startup] RAG vector store init failed (RAG disabled)")
		return
	}
	a.ragVS = ragVS
	a.ragDS = rag.NewDocumentStore(ragVS)
	// 嵌入模型：优先使用专用嵌入模型，否则使用当前聊天模型
	embedModel := cfg.EmbeddingModel
	if embedModel != "" {
		embedModel = resolvePath(embedModel)
	}
	if embedModel == "" {
		embedModel = a.currentModel()
	}
	embedder := &rag.ClientEmbedder{Client: a.getClient()}
	embedder.SetModel(embedModel)
	// 当专用嵌入模型为空时，动态获取当前聊天模型名
	embedder.SetCurrentModelFn(func() string {
		return a.currentModel()
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

// initLogChannel 创建日志 channel 和消费者 goroutine（trackedGo 跟踪）。
// 生活类比：就像一个邮筒（logChan），邮递员（llama-server）把每封信（日志行）投进邮筒，
// 后台有一个邮局职员（消费者 goroutine）负责把信件转交给收件人（前端）。
// 用单个职员而不是每封信都派一个职员（原 go func 实现），避免 goroutine 泛滥。
func (a *App) initLogChannel() {
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
}

// initServer 创建 llama-server 实例并配置日志/终端数据回调。
func (a *App) initServer() {
	a.serverMu.Lock()
	defer a.serverMu.Unlock()
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
}

// buildService 构建聊天服务：生成预设文件、同步 MCP 配置、创建 LLM client 和 service、
// 初始化 RAG、创建日志 channel 和 llama-server 实例。
func (a *App) buildService(ctx context.Context) {
	// F-3 修复：preset 文件生成失败不再静默处理，记录错误并通过事件通知前端
	// 生活类比：菜谱生成器坏了，厨师长（前端）需要知道，否则上菜时才发现没菜谱
	if err := a.generatePresetFile(); err != nil {
		zlog.Error().Err(err).Msg("[startup] generate preset file failed")
		a.presetGenFailed = true
	}

	// 同步 mcp_servers.json：让 llama-server 启动时通过 --mcp-servers-config 加载此文件，
	// 启用 /tools 端点并管理所有 MCP 子进程。
	a.ensureMcpServersFileExists()

	cfg := a.getConfig()
	a.setClient(llm.NewClient(cfg.APIBase, a.getServerAPIKey()))

	searchChain := a.buildSearchChain()

	// 填充提前创建的 service 的真实 client 和 searchChain
	a.service.UpdateClient(a.getClient())
	a.service.UpdateSearchChain(searchChain)

	// Initialize RAG (Badger-backed vector store + LLM embedder)
	a.initRAG(ctx, cfg)

	// MCP 服务器：豆芽不再自行启动 MCP 子进程，而是将配置写入 mcp_servers.json，
	// 由 llama-server 通过 --mcp-servers-config 参数加载并管理。
	// mcp_servers.json 在 startup 时通过 ensureMcpServersFileExists() 自动同步，
	// 用户在「设置 → MCP」修改配置时通过 SaveMCPServers() 重新生成。

	// 创建日志 channel 和消费者 goroutine
	a.initLogChannel()

	// 创建 llama-server 实例
	a.initServer()

	a.ready.Store(true)

	// 异步检查 llama.cpp 上游更新（不阻塞启动，失败不影响主流程）
	// 生活类比：车启动后，后台偷偷去应用商店看一眼有没有新版本，有就弹个提示
	go a.checkLlamaCppUpdate()
}

// checkLlamaCppUpdate 异步检查 llama.cpp 是否有新版本，通过 EventsEmit 通知前端。
//
// 设计说明：
//   - 不使用 trackedGo：短生命周期，完成后即退出，无需 shutdown 等待
//   - panic recover 保护：避免网络异常等导致进程崩溃
//   - 本地版本查询失败时静默跳过（llama-server 可能未安装）
//   - 有更新时通过 EventsEmit 推送前端，前端可显示更新提示
func (a *App) checkLlamaCppUpdate() {
	// panic 保护，避免网络异常等导致进程崩溃
	defer func() {
		if r := recover(); r != nil {
			zlog.Warn().Interface("panic", r).Msg("[update-check] panic")
		}
	}()

	// 获取 llama-server.exe 路径（优先用已缓存的解析路径）
	serverPath := a.resolvedServerPath
	if serverPath == "" {
		// 未缓存时从配置解析（兼容热重载场景）
		cfg := a.getConfig()
		serverPath = resolvePath(cfg.LlamaServerPath)
	}
	if serverPath == "" {
		zlog.Debug().Msg("[update-check] llama-server 路径为空，跳过更新检查")
		return
	}

	zlog.Info().Str("server", serverPath).Msg("[update-check] 开始检查 llama.cpp 更新")
	info := llm.CheckForUpdate(serverPath)

	// 通过 EventsEmit 推送结果到前端
	// 前端监听 "update:check" 事件，根据 HasUpdate 决定是否显示更新提示
	runtime.EventsEmit(a.ctx, "update:check", map[string]any{
		"local_version":  info.LocalVersion,
		"local_commit":   info.LocalCommit,
		"remote_version": info.RemoteVersion,
		"remote_tag":     info.RemoteTag,
		"has_update":     info.HasUpdate,
		"check_error":    info.CheckError,
	})

	if info.HasUpdate {
		zlog.Info().
			Int("local", info.LocalVersion).
			Int("remote", info.RemoteVersion).
			Str("remote_tag", info.RemoteTag).
			Msg("[update-check] 检测到 llama.cpp 有更新，已通知前端")
	} else if info.CheckError != "" {
		zlog.Debug().Str("error", info.CheckError).Msg("[update-check] 检查失败")
	}
}

// cleanupOrphanSessions 清理异常会话（如上次崩溃残留的对话）。
// 异步执行，不阻塞 startup；panic 会被 recover 保护，避免影响主进程。
//
// 注：此处未使用 trackedGo，因为该 goroutine 为短生命周期且已有 defer recover()，
// 完成后即退出，无需 ctx 取消。见安全审查 #26。
func (a *App) cleanupOrphanSessions(ctx context.Context) {
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
func (a *App) shutdownInternal(_ context.Context, waitForServerStop bool) {
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

		// 5. MCP 服务器无需主动断开（由 llama-server 进程退出时自动清理子进程）

		// 6. 关闭 RAG 向量库
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
//
// 为什么最后要调用 os.Exit(0)：
//
//	runtime.Quit 和 systray.Quit 都是异步的，发送退出信号后不会立即终止进程。
//	Wails 的关闭流程需要时间（触发 OnBeforeClose、OnShutdown 等回调）。
//	如果不调用 os.Exit，进程可能延迟数秒才退出，导致：
//	1. 单实例互斥体未释放，新进程检测到已有实例而退出（RestartApp 场景）
//	2. 用户看到旧窗口残留，以为应用没有关闭
//	os.Exit(0) 确保进程立即终止，互斥体立即释放。
//	在 forceQuit 场景下无需清理资源（db/server/ragVS 尚未初始化），直接退出是安全的。
func (a *App) forceQuit() {
	a.exiting.Store(true)
	runtime.Quit(a.ctx)
	systray.Quit()
	os.Exit(0)
}

// downloadBackendWithRetry 带重试的后端下载安装，仅用于启动阶段。
// 每次失败后推送"重试中"进度事件到前端，全部失败后返回最后一次错误。
//
// 生活类比：网购发货时快递可能中途丢失，签收失败就联系卖家重发，
// 最多重发 maxRetries 次，全部失败才放弃。
func (a *App) downloadBackendWithRetry(bt llm.BackendType, runtimeDir string, maxRetries int) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		zlog.Info().Str("backend", bt.String()).Int("attempt", attempt).Int("max", maxRetries).
			Msg("[startup] 下载后端尝试")
		// 推送重试进度到前端
		if attempt > 1 {
			runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: bt,
				Status:  "retrying",
				Error:   fmt.Sprintf("第 %d/%d 次重试中...", attempt, maxRetries),
			})
		}
		if err := a.downloadAndInstallBackend(bt, runtimeDir); err != nil {
			lastErr = err
			zlog.Warn().Err(err).Int("attempt", attempt).Int("max", maxRetries).
				Msg("[startup] 下载后端失败")
			continue
		}
		return nil
	}
	// 全部重试失败：补推 downloadComplete 事件（success=false），确保前端能收到完成通知
	runtime.EventsEmit(a.ctx, "backend:downloadComplete", map[string]any{
		"backend": bt.String(),
		"success": false,
		"error":   fmt.Sprintf("已重试 %d 次仍失败: %v", maxRetries, lastErr),
	})
	return fmt.Errorf("下载后端失败（已重试 %d 次）: %w", maxRetries, lastErr)
}

// downloadAndInstallBackend 下载并安装后端，CUDA 后端会额外下载并解压 cudart 包。
// 下载和安装过程通过事件推送进度到前端，完成后自动重启应用。
//
// 生活类比：买发动机时，CUDA 发动机需要额外配一套"管线配件包"（cudart），
// 两包货都到齐后才能装车。其他发动机（CPU/Vulkan 等）一包就够了。
func (a *App) downloadAndInstallBackend(bt llm.BackendType, runtimeDir string) error {
	// 步骤 1：下载后端主包（推理引擎 + 核心 DLL）
	zlog.Info().Str("backend", bt.String()).Msg("[startup] 开始下载后端主包")
	_, dlErr := llm.DownloadBackendZip(bt, runtimeDir, func(p llm.DownloadProgress) {
		p.Label = "推理后端"
		runtime.EventsEmit(a.ctx, "backend:downloadProgress", p)
	})
	if dlErr != nil {
		return fmt.Errorf("下载后端主包失败: %w", dlErr)
	}

	// 步骤 2：CUDA 后端额外下载 cudart 包
	// cudart 包提供 cudart64_*.dll、cublas64_*.dll 等厂商运行时 DLL，
	// 需解压到与主包相同的目录（runtime/cuda/）才能被 llama-server 找到。
	if bt == llm.BackendCUDA {
		zlog.Info().Msg("[startup] CUDA 后端检测到，开始下载 cudart 包")
		// 通知前端切换到 cudart 下载阶段
		runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
			Backend: bt,
			Status:  "downloading",
			Label:   "cudart 依赖包",
			Percent: 0,
		})
		_, cudartErr := llm.DownloadCudartZip(runtimeDir, func(p llm.DownloadProgress) {
			p.Label = "cudart 依赖包"
			runtime.EventsEmit(a.ctx, "backend:downloadProgress", p)
		})
		if cudartErr != nil {
			// cudart 下载失败直接返回错误，交由上层 downloadBackendWithRetry 重试。
			// 如果继续解压主包并重启，重启后会因厂商 DLL 缺失再次提示下载，形成无限循环。
			// 生活类比：配件包裹没到，整车就装不完整，与其装一半重启后又说缺配件，
			// 不如直接让上层重试，等配件包裹也到齐了再装车。
			zlog.Warn().Err(cudartErr).Msg("[startup] cudart 包下载失败，交由重试逻辑处理")
			return fmt.Errorf("下载 cudart 包失败: %w", cudartErr)
		}
	}

	// 步骤 3：解压安装（推送按文件数的解压进度）
	zlog.Info().Str("backend", bt.String()).Msg("[startup] 下载完成，开始解压安装")
	runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
		Backend: bt,
		Status:  "installing",
		Label:   "解压安装中",
		Percent: 0,
	})

	_, installErr := llm.EnsureBackendInstalled(bt, runtimeDir, func(current, total int) {
		percent := 0.0
		if total > 0 {
			percent = float64(current) / float64(total) * 100
		}
		runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
			Backend: bt,
			Status:  "installing",
			Label:   "解压安装中",
			Percent: percent,
		})
	})

	// complete 事件推送规则（避免重复推送）：
	// - 成功时：此处直接推送 complete {success: true}
	// - 失败时：此处不推送，由上层 downloadBackendWithRetry 在重试耗尽后统一推送
	//   原因：若此处推送失败事件，重试循环中前端会收到多次失败弹窗（C1 修复）
	if installErr != nil {
		return fmt.Errorf("解压安装失败: %w", installErr)
	}

	// 步骤 3.5：CUDA 后端额外解压 cudart 包到同一目录
	// cudart 包提供 cudart64_*.dll、cublas64_*.dll 等厂商运行时 DLL，
	// 必须解压到与主包相同的目录（runtime/cuda/）才能被 llama-server 找到。
	// 如果不解压，validatePaths 会检测到厂商 DLL 缺失，导致下次启动时又提示下载（无限循环）。
	if bt == llm.BackendCUDA {
		zlog.Info().Msg("[startup] CUDA 后端检测到，开始解压 cudart 包")
		runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
			Backend: bt,
			Status:  "installing",
			Label:   "安装 cudart 依赖包",
			Percent: 0,
		})
		if cudartInstallErr := llm.EnsureCudartInstalled(runtimeDir, func(current, total int) {
			percent := 0.0
			if total > 0 {
				percent = float64(current) / float64(total) * 100
			}
			runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: bt,
				Status:  "installing",
				Label:   "安装 cudart 依赖包",
				Percent: percent,
			})
		}); cudartInstallErr != nil {
			// cudart 解压失败不阻止重启：主包已安装，用户系统 PATH 中可能有 cudart
			// 但 validatePaths 会检测到缺失，下次启动可能又提示下载
			zlog.Warn().Err(cudartInstallErr).Msg("[startup] cudart 包解压失败")
			runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
				Backend: bt,
				Status:  "cudart_failed",
				Label:   "cudart 依赖包",
				Error:   fmt.Sprintf("cudart 依赖包解压失败：%v", cudartInstallErr),
			})
		}
	}

	// 成功：推送 complete 事件
	runtime.EventsEmit(a.ctx, "backend:downloadComplete", map[string]any{
		"backend": bt.String(),
		"success": true,
	})

	zlog.Info().Str("backend", bt.String()).Msg("[startup] 下载并安装完成，准备自动重启应用")

	// 推送"重启中"状态，前端据此显示"重启中"文字
	runtime.EventsEmit(a.ctx, "backend:downloadProgress", llm.DownloadProgress{
		Backend: bt,
		Status:  "completed",
		Label:   "重启中",
		Percent: 100,
	})

	// 延迟 1 秒后自动重启应用，给前端时间显示"重启中"状态
	go func() {
		// 防止 panic 导致整个进程崩溃
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[startup] 自动重启 goroutine panic")
			}
		}()
		time.Sleep(1 * time.Second)
		a.RestartApp()
	}()
	return nil
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
		// M5 修复：ALT+F4 / 系统关闭按钮 与前端关闭按钮行为一致，
		// 通过事件通知前端弹出询问对话框，而非直接隐藏到托盘。
		runtime.EventsEmit(ctx, "window:closeRequest", nil)
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
	if err := config.Save(filepath.Join(appDir(), "config.json"), &newCfg); err != nil {
		zlog.Warn().Err(err).Msg("[SetCloseAction] 配置保存失败")
	}
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

// RestartApp 重启应用：先启动新进程，再退出当前进程。
// 通过临时 bat 脚本延迟启动新进程，确保旧进程完全退出后再启动新的，
// 避免端口/文件锁冲突。
//
// 生活类比：换班时，先让接班的人到岗准备好，老员工再下班，
// 中间留几秒钟交接时间，避免两人同时操作同一个岗位（端口/文件冲突）。
func (a *App) RestartApp() {
	exe, err := os.Executable()
	if err != nil {
		zlog.Error().Err(err).Msg("[restart] 获取可执行文件路径失败")
		a.forceQuit()
		return
	}

	// 安全校验 exe 路径：清理路径分隔符，检查是否包含 shell 元字符
	exe = filepath.Clean(exe)
	if strings.ContainsAny(exe, "&|><^()") {
		zlog.Warn().Str("exe", exe).Msg("[restart] exe 路径包含特殊字符，重启可能失败")
	}

	// 创建临时 bat 脚本：等待 2 秒后启动新进程，然后删除自身
	// 权限设为 0600，仅文件所有者可读写，避免被其他用户篡改
	batPath := filepath.Join(filepath.Dir(exe), "restart_douya.bat")
	batContent := fmt.Sprintf("@echo off\r\ntimeout /t 2 /nobreak >nul\r\nstart \"\" \"%s\"\r\ndel \"%%~f0\"\r\n", exe)
	if err := os.WriteFile(batPath, []byte(batContent), 0600); err != nil {
		zlog.Error().Err(err).Msg("[restart] 创建重启脚本失败")
		a.forceQuit()
		return
	}

	// 异步启动 bat 脚本（不等待其完成）
	cmd := exec.Command("cmd", "/c", batPath)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		zlog.Error().Err(err).Msg("[restart] 启动重启脚本失败")
		os.Remove(batPath)
		a.forceQuit()
		return
	}

	zlog.Info().Str("exe", exe).Msg("[restart] 重启脚本已启动，即将退出当前进程")

	// 短暂等待确保 bat 脚本已开始执行，然后退出当前进程
	go func() {
		// 防止 panic 导致整个进程崩溃
		defer func() {
			if r := recover(); r != nil {
				zlog.Warn().Interface("panic", r).Msg("[restart] 退出等待 goroutine panic")
			}
		}()
		time.Sleep(500 * time.Millisecond)
		a.forceQuit()
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
	if err = os.MkdirAll(imagesDir, 0o755); err != nil {
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
	if _, e := io.Copy(hasher, srcFile); e != nil {
		srcFile.Close()
		return "", fmt.Errorf("计算文件哈希失败: %w", e)
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
	if _, e := io.Copy(dstFile, srcFile); e != nil {
		dstFile.Close()
		// 复制失败时清理已创建的空文件
		_ = os.Remove(dstPath)
		return "", fmt.Errorf("复制图片失败: %w", e)
	}
	if e := dstFile.Close(); e != nil {
		return "", fmt.Errorf("保存图片失败: %w", e)
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
