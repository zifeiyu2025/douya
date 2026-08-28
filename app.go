// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"douya/internal/appdata"
	"douya/internal/apperror"
	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/modelruntime"
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
	// ParamsRestored: 切换时是否恢复了该模型的专属生成参数预设。
	// 前端据此在"已就绪"提示中追加"已恢复专属参数"，无需依赖事件监听。
	ParamsRestored bool `json:"params_restored,omitempty"`
}

// SearchAPIKeys 用于前端展示搜索 API Key 的设置状态，不暴露实际密钥值
type SearchAPIKeys struct {
	OllamaAPIKey    string `json:"ollama_api_key"`
	TavilyAPIKey    string `json:"tavily_api_key"`
	OllamaAPIKeySet bool   `json:"ollama_api_key_set"`
	TavilyAPIKeySet bool   `json:"tavily_api_key_set"`
}

type App struct {
	ctx      context.Context
	config   *config.Config
	configMu sync.RWMutex
	server   *llm.Server
	serverMu sync.RWMutex
	// backendMu 保护 resolvedBackend / resolvedServerPath 的并发读写。
	// 生活类比：车辆登记证的"已装发动机型号"被多个部门读取（Wails 前端查询后端状态、
	// 启动流程写入回退结果），用一把锁登记进出，防止读取到写了一半的登记证。
	backendMu sync.RWMutex
	client    *llm.Client
	// clientMu 保护 client 字段的并发读写。
	// 生活类比：像公共打印机的"使用登记本"——多人可同时查看谁在用（RLock），
	// 但更换打印机时必须等所有人用完（Lock）。
	// 风险背景：a.client 在 Wails 主线程、startServerAndWatch goroutine、
	// health goroutine 等多处被读写，无锁保护会导致数据竞争甚至 panic。
	clientMu sync.RWMutex
	db       *sql.DB
	service  *chat.Service
	hwInfo   *system.HardwareInfo
	// resolvedBackend 是 startup 中解析后的后端类型（不含 auto），供 validatePaths 和 buildServerConfig 复用。
	// 生活类比：就像车辆登记证上写明的"已装发动机型号"，后续保养（校验/启动）都看这个。
	resolvedBackend llm.BackendType
	// resolvedServerPath 是 startup 中 EnsureBackendInstalled 返回的 llama-server.exe 绝对路径。
	// 为空表示未缓存，调用方应回退到配置中的 LlamaServerPath。
	resolvedServerPath string
	// presetGenFailed 标记 preset 文件生成是否失败，用于启动后通知前端显示警告
	presetGenFailed bool
	ready           atomic.Bool
	serverReady     atomic.Bool
	watchCancel     context.CancelFunc
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
	logChan         chan string
	stopOnce        sync.Once
	cleanupResult   []*chat.AbnormalConversation
	cleanupResultMu sync.Mutex
	presets         []llm.ModelPreset
	presetRelPaths  map[string]string
	presetsMu       sync.RWMutex
	// modelSession owns current-model and switch-transition state. Keeping the
	// state together prevents readers from observing a mixed lock/atomic view.
	modelSession     *modelruntime.Session
	modelSessionOnce sync.Once
	ragVS            *rag.VectorStore
	ragDS            *rag.DocumentStore
	ragEmbedder      *rag.ClientEmbedder
	encKey           []byte
	exiting          atomic.Bool
	serverLoadFailed atomic.Bool // 模型加载彻底失败后锁定状态，防止监控循环覆盖错误状态
	lastServerError  string      // 最后一次服务器/模型加载错误信息
	// modelsEmpty 标记 models 目录为空（无 .gguf 文件）的首次使用状态。
	// 与 serverLoadFailed 独立：这是"待引导下载"的正常状态，而非加载失败，
	// 不触发监控循环的失败锁定/回退逻辑，仅用于让 GetServerStatus 稳定返回
	// "模型目录为空"错误，供前端据此放行引导流程。
	modelsEmpty atomic.Bool
	// downloadMu 保护 downloadingBackends 的并发访问，防止同一后端重复下载。
	// 生活类比：正在装修的房间门上挂个"施工中"牌子，避免两队施工队同时开干。
	downloadMu          sync.Mutex
	downloadingBackends map[string]bool
	// downloadingModels 防止同一模型文件重复下载（key: provider/repoID/file）。
	// 生活类比：同一个粽子同时只能包一个，避免煮糊。
	downloadingModels map[string]bool
	// switchMu 防止短时间内的重复后端切换操作（最小冷却间隔 3s）。
	// 生活类比：发动机切换有冷却时间，不能刚熄火就立刻再换挡。
	switchMu        sync.Mutex
	lastSwitchTime  time.Time
	lastServerErrMu sync.RWMutex
	// fileLoader 是本地文件服务的引用，托盘最小化时调用 ClearCache 释放内存
	fileLoader *LocalFileLoader

	// startupError 保存最近一次启动期致命错误（标题/简述/详情），供前端启动错误卡展示。
	// 采用原子值避免并发读写的竞态；nil 表示无致命错误。
	// startupErrorChan 用于阻塞等待前端确认致命错误（前端渲染错误卡后调用 ConfirmStartupError 放行）。
	// 店门口的红灯（startupError）亮了——顾客得先看到"暂停营业的原因"（错误卡）并点确认，
	// 店家（后端）收到确认（写 channel）才会关门（forceQuit）。nil 表示当前无致命错误等待确认。
	startupErrorMu   sync.Mutex
	startupError     *StartupError
	startupErrorChan chan struct{}
}

func NewApp() *App {
	return &App{}
}

func (a *App) modelRuntimeSession() *modelruntime.Session {
	// NewApp is the production construction path. The fallback preserves the
	// zero-value App used by a few focused tests and legacy helpers.
	a.modelSessionOnce.Do(func() {
		a.modelSession = modelruntime.NewSession()
	})
	return a.modelSession
}

func (a *App) currentModel() string { return a.modelRuntimeSession().CurrentModel() }

func (a *App) setCurrentModel(model string) { a.modelRuntimeSession().SetCurrentModel(model) }

func (a *App) beginModelSwitch(target string) bool {
	return a.modelRuntimeSession().BeginSwitch(target)
}

func (a *App) endModelSwitch() { a.modelRuntimeSession().EndSwitch() }

func (a *App) clearSwitchTarget() { a.modelRuntimeSession().ClearTarget() }

func (a *App) modelSessionSnapshot() modelruntime.SessionSnapshot {
	return a.modelRuntimeSession().Snapshot()
}

// getClient 返回当前 llm.Client 的快照指针。
// 调用方拿到指针后，llm.Client 内部方法本身是并发安全的（基于 http.Client），
// 因此只需保护 a.client 指针字段的读写，无需在调用方法期间持锁。
// 生活类比：像从登记本上抄下"当前打印机型号"，抄完后就可以放心去用，不用一直按着登记本。
func (a *App) getClient() *llm.Client {
	a.clientMu.RLock()
	defer a.clientMu.RUnlock()
	return a.client
}

// setClient 原子地替换 llm.Client 指针。
// 生活类比：像更换打印机——必须等所有人都用完旧的（Lock），才能换上新的。
func (a *App) setClient(c *llm.Client) {
	a.clientMu.Lock()
	defer a.clientMu.Unlock()
	a.client = c
}

// getServer 返回当前 llama-server 的快照指针（受 serverMu 保护）。
// 生活类比：从登记本上抄下"当前正在运行的服务"，抄完即可放心使用；
// 服务可能被 initServer 替换（后端回退场景），必须经由此处读取，避免读到写了一半的指针。
func (a *App) getServer() *llm.Server {
	a.serverMu.RLock()
	defer a.serverMu.RUnlock()
	return a.server
}

// setResolvedBackend 在锁保护下更新已解析的后端与服务器路径。
// 生活类比：车辆登记证上的"已装发动机型号"由维修工（启动流程）更新，
// 更新时锁门（Lock）防止其他部门读到一半的信息。
func (a *App) setResolvedBackend(bt llm.BackendType, serverPath string) {
	a.backendMu.Lock()
	defer a.backendMu.Unlock()
	a.resolvedBackend = bt
	a.resolvedServerPath = serverPath
}

// resolvedBackendSnapshot 在锁保护下读取已解析后端与服务器路径。
// 生活类比：各部门查"已装发动机型号"时也要排队（RLock），保证读到完整的值。
func (a *App) resolvedBackendSnapshot() (bt llm.BackendType, serverPath string) {
	a.backendMu.RLock()
	defer a.backendMu.RUnlock()
	return a.resolvedBackend, a.resolvedServerPath
}

// resolvedBackendString 在锁保护下读取已解析后端的字符串形式（可能为空）。
func (a *App) resolvedBackendString() string {
	bt, _ := a.resolvedBackendSnapshot()
	return string(bt)
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

// recoverLog 恢复 panic 并记录警告日志，用于 goroutine 内的 defer 调用。
// 仅适用于"只需记录日志"的简单 recover 场景；有额外副作用（如 EventsEmit、错误事件发送）
// 的 recover 仍需手写 defer func() 以保留自定义逻辑。
// 生活类比：像安全气囊的标准弹出动作——只记录"这里出了问题"，
// 特殊场景需要联动其他系统（如报警、灭火）的仍需定制化处理。
func recoverLog(msg string) {
	if r := recover(); r != nil {
		zlog.Warn().Interface("panic", r).Msg(msg)
	}
}

// requireReady 检查应用是否已就绪（配置加载、数据库初始化等完成）。
// 生活类比：像大楼入口的门禁——没有工牌（未就绪）就不让进，直接返回错误。
// 用于消除各 App 方法开头重复的 if !a.ready.Load() { return ... } 模式。
func (a *App) requireReady() error {
	if !a.ready.Load() {
		return apperror.New(apperror.KindUnavailable, "应用未就绪")
	}
	return nil
}

// requireServer 检查 AI 服务（llama-server）是否已启动并就绪。
// 生活类比：像使用会议室前检查投影仪是否已开机——没开机就无法开会。
// 用于消除需要 AI 推理能力的方法中重复的 serverReady 检查。
func (a *App) requireServer() error {
	if !a.serverReady.Load() {
		return apperror.New(apperror.KindUnavailable, "AI 服务未启动，请等待服务就绪或检查配置")
	}
	return nil
}

var (
	cachedAppDir string
	appDirOnce   sync.Once
)

func appDir() string {
	appDirOnce.Do(func() {
		cachedAppDir = resolveAppDir()
	})
	return cachedAppDir
}

// resolveAppDir 查找并缓存应用根目录（统一为微软商店版布局）。
//
// 豆芽仅发布微软商店（MSIX）一个版本：安装目录（WindowsApps 下）只读，
// 配置/数据/模型统一写入 %LOCALAPPDATA%\Douya（由 internal/appdata 统一管理，
// 含旧版本遗留数据的一次性迁移）。
func resolveAppDir() string {
	exePath, err := os.Executable()
	if err != nil {
		zlog.Error().Err(err).Msg("[appDir] 获取可执行文件路径失败，回退到工作目录")
		if wd, werr := os.Getwd(); werr == nil {
			return wd
		}
		return "."
	}

	dir := appdata.EnsureDataDir(exePath)

	// 生成默认配置（若无），保证后续 loadAndValidateConfig 能正常读到配置
	cfgPath := filepath.Join(dir, "config.json")
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := config.Save(cfgPath, config.DefaultConfig()); err != nil {
			zlog.Error().Err(err).Msg("[appDir] 创建默认配置失败")
		}
	}
	return dir
}

// bundledRuntimeDir 返回随安装包内置的 runtime 目录（含推理引擎与依赖）。
//
// MSIX 安装目录（WindowsApps 下）只读，引擎随包分发、开箱即用；
// 数据目录中的 runtime 仅存放运行期下载的后端（内容下载，非应用自更新）。
func bundledRuntimeDir() string {
	exePath, err := os.Executable()
	if err != nil {
		zlog.Error().Err(err).Msg("[appDir] 获取可执行文件路径失败，无法定位内置 runtime")
		return ""
	}
	return filepath.Join(filepath.Dir(exePath), "runtime")
}

// runtimeDirCandidates 返回按优先级排列的候选 runtime 目录列表：
// [内置目录（只读、随包分发）, 数据目录（可写、运行期下载）]。
// 顺序即优先级：内置引擎保证首启即用，不被数据目录的旧文件干扰。
func runtimeDirCandidates() []string {
	return []string{
		bundledRuntimeDir(),
		filepath.Join(appDir(), "runtime"),
	}
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
