// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/UserExistsError/conpty"
	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
	"douya/internal/pathutil"
)

const vramCheckInterval = 500 * time.Millisecond
const vramCheckTimeout = 15
const healthCheckTimeout = 2 * time.Second // 健康检查超时

// allowedCacheTypes 列出 llama.cpp 允许的 KV cache 类型
// 与 llama.cpp 源码 common/arg.cpp 的 kv_cache_types 列表保持一致：
// f32, f16, bf16, q8_0, q4_0, q4_1, iq4_nl, q5_0, q5_1
// 注意：nvfp4 是模型权重量化类型（MOSTLY_NVFP4），不是 KV cache 类型，
// llama-server 收到 --cache-type-v nvfp4 会抛 runtime_error 导致启动失败。
// 已删除的类型：q2_k, q3_k, q4_k, q5_k, q6_k, iq4_xs, nvfp4
var allowedCacheTypes = map[string]bool{
	"f32":    true,
	"f16":    true,
	"bf16":   true,
	"q8_0":   true,
	"q4_0":   true,
	"q4_1":   true,
	"iq4_nl": true,
	"q5_0":   true,
	"q5_1":   true,
}

// isValidCacheType 校验 cache 类型是否被支持
func isValidCacheType(t string) bool {
	return allowedCacheTypes[strings.ToLower(t)]
}

type ServerConfig struct {
	ModelsDir              string
	MmprojAuto             bool
	MmprojOffload          bool
	ServerPath             string
	Port                   int
	GPULayers              string
	Threads                int
	FlashAttn              string // "on"/"off"/"auto"，对应 llama.cpp --flash-attn 参数
	CacheTypeK             string
	CacheTypeV             string
	Mlock                  bool
	KVUnified              bool
	CacheIdleSlots         bool
	CacheRAM               int
	ImageMinTokens         int
	ImageMaxTokens         int
	FitTarget              int
	FitCtx                 int
	Reasoning              string
	ReasoningBudget        int
	ReasoningFormat        string
	ReasoningBudgetMessage string
	ReasoningPreserve      *bool // 推理内容保留开关（nil=不传递，true=--reasoning-preserve，false=--no-reasoning-preserve）
	APIBase                string
	AppDir                 string
	ModelsPreset           string
	ModelsMax              int
	SleepIdleSeconds       int
	Mmap                   bool
	KVOffload              bool
	ContextShift           bool
	// KeepSize 保护初始 prompt 中前 N 个 token 不被 context-shift 移位（通常用于保护 system prompt）。
	// 仅当 ContextShift=true 且 KeepSize>0 时传递 --keep 给 llama-server。
	// 默认 0=不传递；app_server.go 会赋一个保守默认值（512）。
	KeepSize              int
	MinP                  float64
	DryMultiplier         float64
	DryBase               float64
	DryAllowedLength      int
	DrySequenceBreaker    string
	DryPenaltyLastN       int
	GrpAttnN              int
	GrpAttnW              int
	Jinja                 *bool // Jinja2 模板引擎开关
	CachePrompt           *bool // Prompt 缓存控制
	Metrics               bool  // 服务器指标端点开关
	Verbose               bool  // 详细日志开关
	SpecDraftThreads      int   // Draft 模型线程数
	SpecDraftThreadsBatch int   // Draft 模型批处理线程数
	SpecDefault           bool  // 使用默认推测解码配置
	Device                string
	Parallel              int
	APIKey                string
	ServerAPIKeyEnabled   bool // 是否启用服务 API Key 验证（暴露到局域网时强制要求）

	SpecType               string
	SpecDraftNMax          int
	SpecDraftNMin          int
	CacheTypeKDraft        string
	CacheTypeVDraft        string
	SpecNgramModNMin       int
	SpecNgramModNMax       int
	SpecNgramModNMatch     int
	SpecNgramSimpleSizeN   int
	SpecNgramSimpleSizeM   int
	SpecNgramSimpleMinHits int
	SpecNgramMapKSizeN     int
	SpecNgramMapKSizeM     int
	SpecNgramMapKMinHits   int
	SpecNgramMapK4VSizeN   int
	SpecNgramMapK4VSizeM   int
	SpecNgramMapK4VMinHits int
	LookupCacheStatic      string
	LookupCacheDynamic     string
	SpecDraftModel         string
	Embedding              bool   // 启用 /v1/embeddings API（RAG 知识库需要）
	Pooling                string // 嵌入池化类型（mean/cls），解决聊天模型 pooling=none 不兼容 OAI embedding API
	ExposeServer           bool   // 暴露服务器地址，允许局域网访问
	EnableWebUI            bool   // 启用 llama-server 自带的原生 Web UI（false=加 --no-webui）
	SwaFull                bool
	CtxCheckpoints         int
	CheckpointMinStep      int
	Tools                  string
	PrefillAssistant       bool
	SlotPromptSimilarity   float64
	SkipChatParsing        bool
	APIPrefix              string
	SimpleIO               bool
	BatchSize              int
	UBatchSize             int
	ThreadsHTTP            int // HTTP 请求处理线程数（0=使用 llama-server 默认值）
	ContextSize            int
	// KV 缓存持久化
	SlotSavePath    string // 启用后传递 --slot-save-path
	SlotSaveEnabled bool
	CacheReuse      int // KV 缓存复用块大小（0=禁用）
	// Draft 模型 GPU 配置（Eagle3 等场景）
	SpecDraftNgl    int    // draft 模型 GPU 层数
	SpecDraftDevice string // draft 模型设备（如 "cuda:0"）
	// Draft 模型推测解码参数
	SpecDraftPSplit          float64 // 推测解码 split 概率（默认 0.10）
	SpecDraftPMin            float64 // 最小推测解码概率（默认 0.00）
	SpecDraftBackendSampling *bool   // draft 模型后端采样（nil=默认启用）
	// 多模态批处理
	MtmdBatchMaxTokens int // 图像编码每个 batch 的最大 token 数（默认 1024）
	// 自适应采样（llama.cpp 新增，动态调整采样参数）
	AdaptiveTarget float64 // 自适应采样目标概率（0-1，默认 0.0=禁用）
	AdaptiveDecay  float64 // 自适应采样衰减率（0-1，默认 0.90）
	// 模型标签（逗号分隔，用于 /v1/models 返回的 tags 字段）
	Tags string
	// 媒体路径（多模态模型额外媒体文件目录）
	MediaPath string
	// 离线模式（禁用所有网络请求，如模型下载等）
	Offline bool
	// 模型重打包（启动时重新打包模型权重，用于优化加载速度）
	Repack bool
	// Agent 模式与 MCP CORS 代理
	Agent      bool // 一键启用 CORS 代理 + 所有内置工具
	UIMcpProxy bool // 仅启用 MCP CORS 代理
	// 后端采样（实验性，将采样逻辑移到 GPU 执行，不兼容 grammar 和 reasoning budget）
	BackendSampling bool
	// SSE ping 间隔秒数（0=使用服务器默认 30 秒，用于保持长连接活跃）
	SsePingInterval int
	// LoRA 适配器路径（逗号分隔，启动时通过 --lora 加载，配合 --lora-init-without-apply 默认不应用）
	LoraPaths string
	// Reranker 模型路径（配置后自动启用 --rerank 端点）
	RerankerModelPath string
	// 直接 I/O（绕过操作系统页面缓存，加速大模型加载）
	DirectIO bool
	// MoE 权重 CPU 卸载（将所有专家权重保留在 CPU）
	CPUMoe bool
	// 前 N 层 MoE 权重 CPU 卸载（0=不启用）
	NCpuMoe int
	// 算子卸载开关（nil=使用默认值，true=--op-offload，false=--no-op-offload）
	OpOffload *bool
}

type Server struct {
	cmd    *exec.Cmd
	pty    *conpty.ConPty // ConPTY 伪控制台（非 nil 表示使用 ConPTY 模式）
	config *ServerConfig
	status ServerStatus
	ctx    context.Context
	cancel context.CancelFunc
	// mu 保护下方所有字段（cmd/pty/config/status/job/stderrBuf/mtpFallbackDisabled/...）。
	// 安全说明（基于 GO-CONC-001 #9）：当前使用单一粗粒度 RWMutex 保护所有字段，
	// 读写访问通过 s.mu.RLock()/s.mu.Lock() 统一加锁。当前所有访问路径已审计无数据竞争。
	// 若未来扩展并发场景出现锁竞争，可参考 internal/chat/service.go 的做法
	// （每个语义字段配独立锁，或简单标量改用 atomic.Bool/atomic.Int64）。
	// 拆分锁时需特别注意 Start/stopInternal 等方法内部的临时释放模式（s.mu.Unlock()），
	// 避免引入死锁。CI 已配置 go test -race 持续监控（见 .github/workflows/govulncheck.yml 之外的 test workflow）。
	mu                  sync.RWMutex
	job                 *JobObject
	stderrBuf           *RingBuffer
	// RF-3 修复：mtpFallbackDisabled 改用 atomic.Bool，消除 WatchWithCallback goroutine
	// 与 buildStartArgs/GetSpecType 之间的数据竞争（goroutine 无锁写入 vs 持锁读取）。
	// 生活类比：调度中心的总开关状态牌，任何值班员都能直接看/拨（atomic 操作），
	// 不需要先去前台排队借钥匙（mu 锁），避免排班冲突。
	mtpFallbackDisabled atomic.Bool
	lastStartTime       time.Time
	cmdEnv              []string          // 安全传递给子进程的环境变量（如 API Key）
	onLog               func(line string) // 日志行回调（用于实时推送到前端）
	onTerminalData      func(data []byte) // 终端原始字节流回调（用于 xterm.js 渲染）
	healthClient        *http.Client      // 复用的健康检查 HTTP 客户端（WaitForReady/GracefulStop 共用）
	permanentFailure    bool              // 永久失败标志：服务器反复崩溃后不再自动重启
	maxRestartAttempts  int               // 最大重启尝试次数（默认 10，可配置便于测试）
	initialBackoff      time.Duration     // 初始退避时间（默认 2s，可配置便于测试）
	pollInterval        time.Duration     // 轮询间隔（默认 1s，可配置便于测试）
}

func NewServer(cfg *ServerConfig) *Server {
	return &Server{
		config:             cfg,
		status:             ServerStatus{Running: false},
		healthClient:       &http.Client{Timeout: healthCheckTimeout},
		maxRestartAttempts: 10,
		initialBackoff:     2 * time.Second,
		pollInterval:       1 * time.Second,
	}
}

// SetOnLog 设置日志行回调，llama-server 每输出一行日志都会触发
// 用于将控制台输出实时推送到前端 GUI
func (s *Server) SetOnLog(cb func(line string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onLog = cb
	if s.stderrBuf != nil {
		s.stderrBuf.SetOnChange(cb)
	}
}

// SetOnTerminalData 设置终端原始字节流回调，用于 xterm.js 渲染
// ConPTY 模式下，llama-server 的输出（含 ANSI 颜色码）会通过此回调批量推送
func (s *Server) SetOnTerminalData(cb func(data []byte)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onTerminalData = cb
}

// ResizeTerminal 调整 ConPTY 终端尺寸（前端 xterm.js 尺寸变化时调用）
func (s *Server) ResizeTerminal(width, height int) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.pty == nil {
		return nil // exec.Cmd 模式无需调整
	}
	return s.pty.Resize(width, height)
}

// IsConPTYMode 返回当前是否使用 ConPTY 模式
func (s *Server) IsConPTYMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pty != nil
}

// appendStringArg 当 val 非空时追加字符串参数。
// 生活类比：像寄快递时，收件人姓名填了才写"收件人：xxx"，空白就不写那行。
func appendStringArg(args []string, flag, val string) []string {
	if val != "" {
		return append(args, flag, val)
	}
	return args
}

// appendIntArg 当 val > 0 时追加整数控件参数。
func appendIntArg(args []string, flag string, val int) []string {
	if val > 0 {
		return append(args, flag, fmt.Sprintf("%d", val))
	}
	return args
}

// appendFloatArg 当 val > 0 时追加浮点数参数，format 指定格式（如 "%.2f"）。
func appendFloatArg(args []string, flag string, val float64, format string) []string {
	if val > 0 {
		return append(args, flag, fmt.Sprintf(format, val))
	}
	return args
}

// appendBoolArg 当 val 为 true 时追加布尔标志参数（无值）。
func appendBoolArg(args []string, flag string, val bool) []string {
	if val {
		return append(args, flag)
	}
	return args
}

// resolvePath 将相对路径解析为相对于 AppDir 的绝对路径。
// 已是绝对路径则原样返回，避免重复处理。
// 安全实践：复用 pathutil.ResolveInBase 统一路径遍历防护，与 app.go 的 resolvePath 实现对齐（见安全审查 #1/#20）
func (s *Server) resolvePath(p string) string {
	return pathutil.ResolveInBase(s.config.AppDir, p)
}

// enhanceStartError 增强启动错误信息，检测 DLL 缺失等常见问题
// 生活类比：就像翻译官把晦涩的英文报错翻译成通俗易懂的中文提示
func enhanceStartError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	lower := strings.ToLower(errStr)

	// Windows DLL 缺失错误通常包含 "The specified module could not be found" 或 DLL 文件名
	if strings.Contains(lower, "the specified module could not be found") ||
		strings.Contains(lower, "dll not found") ||
		strings.Contains(lower, ".dll") && (strings.Contains(lower, "not found") || strings.Contains(lower, "cannot find")) {
		return fmt.Errorf("启动引擎失败，可能是 DLL 文件缺失: %w\n请检查 runtime/ 目录是否包含所有必要的 DLL 文件", err)
	}

	// 引擎 exe 本身不存在
	if strings.Contains(lower, "the system cannot find the file specified") ||
		strings.Contains(lower, "no such file or directory") {
		return fmt.Errorf("启动引擎失败，引擎程序文件不存在: %w\n请检查 config.json 中的 llama_server_path 配置", err)
	}

	return err
}

// readConPTYOutput 持续读取 ConPTY 输出，50ms 窗口批量发送到前端
// 生活类比：就像邮递员收集信件，不是收到一封就跑一趟，而是攒一批一起送，效率更高
func (s *Server) readConPTYOutput() {
	buf := make([]byte, 8192)
	var pending []byte
	lastFlush := time.Now()

	for {
		n, err := s.pty.Read(buf)
		if n > 0 {
			// 追加到待发送缓冲
			data := make([]byte, n)
			copy(data, buf[:n])
			pending = append(pending, data...)
			// 同时写入 RingBuffer（用于错误诊断，按行切分存储）
			if s.stderrBuf != nil {
				_, _ = s.stderrBuf.Write(buf[:n])
			}
		}

		// 50ms 窗口批量发送（降低 IPC 频次 10-50 倍）
		now := time.Now()
		if len(pending) > 0 && now.Sub(lastFlush) >= 50*time.Millisecond {
			s.mu.RLock()
			cb := s.onTerminalData
			s.mu.RUnlock()
			if cb != nil {
				cb(pending)
			}
			pending = pending[:0]
			lastFlush = now
		}

		if err != nil {
			// 读取出错或 EOF，发送剩余数据后退出
			if len(pending) > 0 {
				s.mu.RLock()
				cb := s.onTerminalData
				s.mu.RUnlock()
				if cb != nil {
					cb(pending)
				}
			}
			return
		}

		// 检查 context 是否取消
		if s.ctx.Err() != nil {
			if len(pending) > 0 {
				s.mu.RLock()
				cb := s.onTerminalData
				s.mu.RUnlock()
				if cb != nil {
					cb(pending)
				}
			}
			return
		}
	}
}

// replaceContext 清理旧的 cancel 函数并创建新的 context，避免资源泄漏
//
// 生活类比：就像换新电池前先关掉旧电池供电的设备。
// 每次 Start() 都会创建新的 context 和 cancel 函数，如果旧 cancel 未被调用就被覆盖，
// 旧 context 衍生的资源（如 goroutine、定时器）将无法被回收，造成资源泄漏。
// 调用此方法会先触发旧 cancel（通知旧 context 的所有消费者退出），再创建新 context。
func (s *Server) replaceContext() {
	// 清理旧的 cancel 函数，避免资源泄漏
	if s.cancel != nil {
		s.cancel()
	}
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *Server) WaitForReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := s.healthClient
	url := fmt.Sprintf("http://127.0.0.1:%d/health", s.config.Port)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			ready := resp.StatusCode == http.StatusOK
			resp.Body.Close()
			if ready {
				return nil
			}
		}
		if !s.IsRunning() {
			errMsg := "server process exited while waiting for ready"
			s.mu.RLock()
			if s.stderrBuf != nil {
				if tail := s.stderrBuf.String(); tail != "" {
					errMsg += "\n" + tail
				}
			}
			s.mu.RUnlock()
			return apperror.Newf(apperror.KindUnavailable, "%s", errMsg)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return apperror.Newf(apperror.KindTimeout, "server did not become ready within %v", timeout)
}

func (s *Server) GracefulStop(timeout time.Duration) error {
	s.mu.RLock()
	running := s.status.Running
	apiBase := s.config.APIBase
	s.mu.RUnlock()

	if !running {
		return nil
	}

	shutdownURL := apiBase + "/shutdown"
	client := s.healthClient

	resp, err := client.Post(shutdownURL, "application/json", nil)
	if err != nil {
		log.Error().Err(err).Msg("graceful shutdown request failed (will force stop)")
	} else {
		defer resp.Body.Close()
		log.Info().Msg("graceful shutdown request sent, waiting for server to exit...")
	}

	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		if !s.IsRunning() {
			s.SetStatus(false, "")
			if s.cancel != nil {
				s.cancel()
			}
			return nil
		}
		<-ticker.C
	}

	log.Warn().Dur("timeout", timeout).Msg("server did not exit within timeout, forcing stop")
	return s.Stop()
}

func (s *Server) stopInternal() error {
	s.mu.Lock()

	// ConPTY 模式：用 taskkill 终止进程树，然后关闭 ConPTY
	if s.pty != nil {
		pid := s.pty.Pid()
		pty := s.pty
		s.mu.Unlock()

		// 安全实践（基于 B-1.5）：ConPTY 与 exec.Cmd 路径共用 stopProcessWithTimeout，
		// 仅 waitFn/cleanupFn 不同，避免维护时改一条路径漏改另一条
		return s.stopProcessWithTimeout(pid, func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			_, _ = pty.Wait(ctx)
		}, func() {
			pty.Close()
		}, "pty")
	}

	// exec.Cmd 模式（原有逻辑）
	if s.cmd == nil || s.cmd.Process == nil {
		s.mu.Unlock()
		return nil
	}
	pid := s.cmd.Process.Pid
	cmd := s.cmd
	s.mu.Unlock()

	return s.stopProcessWithTimeout(pid, func() {
		_ = cmd.Wait()
	}, nil, "cmd")
}

// stopProcessWithTimeout 统一处理"taskkill 正常终止 → 等待 → 超时强制 kill → 更新 status"流程
// 安全实践（基于 B-1.5）：消除 ConPTY 和 exec.Cmd 两条路径的重复逻辑
//
// 参数说明：
//   - pid：进程 ID，用于 taskkill
//   - waitFn：等待进程退出的阻塞函数（ConPTY 用 pty.Wait(ctx)，exec.Cmd 用 cmd.Wait()）
//   - cleanupFn：超时后额外的清理函数（ConPTY 需 pty.Close()，exec.Cmd 无需额外清理传 nil）
//   - mode：日志标识，"pty" 或 "cmd"
//
// 返回值：正常退出返回 nil，超时强制 kill 返回 error
func (s *Server) stopProcessWithTimeout(pid int, waitFn func(), cleanupFn func(), mode string) error {
	// 先尝试正常终止进程树
	terminateCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T")
	terminateCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := terminateCmd.Run(); err != nil {
		log.Debug().Err(err).Int("pid", pid).Str("mode", mode).Msg("terminate process (may already be dead)")
	}

	// 等待进程退出（带超时）
	waitDone := make(chan struct{})
	go func() {
		// L-3：确保 waitDone 一定被关闭，否则外层 select 永远阻塞（虽有 3s 兜底但会浪费超时时间）
		defer func() {
			if r := recover(); r != nil {
				log.Debug().Interface("panic", r).Str("mode", mode).Msg("[server] wait-done goroutine panic")
			}
			close(waitDone)
		}()
		waitFn()
	}()

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	select {
	case <-waitDone:
		if cleanupFn != nil {
			cleanupFn()
		}
		s.mu.Lock()
		s.status = ServerStatus{Running: false}
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		return nil
	case <-timer.C:
		// 超时后强制 kill
		killCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/F", "/T")
		killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if err := killCmd.Run(); err != nil {
			log.Debug().Err(err).Int("pid", pid).Str("mode", mode).Msg("force kill process (may already be dead)")
		}
		if cleanupFn != nil {
			cleanupFn()
		}
		// exec.Cmd 路径需要额外等待 goroutine 结束（cmd.Wait() 会在 kill 后返回）
		// ConPTY 路径的 pty.Wait(ctx) 已在 3s 超时后返回，无需再次等待
		if mode == "cmd" {
			<-waitDone
		}
		s.mu.Lock()
		s.status = ServerStatus{Running: false}
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		return fmt.Errorf("server did not terminate gracefully, force killed")
	}
}

func (s *Server) Stop() error {
	return s.stopInternal()
}

func (s *Server) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status.Running && s.isAlive()
}

func (s *Server) Status() ServerStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.Running && !s.isAlive() {
		s.status = ServerStatus{Running: false}
	}
	return s.status
}

func (s *Server) WatchWithCallback(ctx context.Context, onStatusChange func(ServerStatus), onRestartSuccess func()) {
	restartCount := 0
	// 使用结构体字段以便测试可覆盖（默认 2s 退避、1s 轮询、10 次重试上限）
	initialBackoff := s.initialBackoff
	if initialBackoff == 0 {
		initialBackoff = 2 * time.Second
	}
	currentBackoff := initialBackoff
	const maxBackoff = 60 * time.Second
	maxAttempts := s.maxRestartAttempts
	if maxAttempts <= 0 {
		maxAttempts = 10
	}
	pollInt := s.pollInterval
	if pollInt == 0 {
		pollInt = 1 * time.Second
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !s.IsRunning() {
			if restartCount >= maxAttempts {
				// 永久失败：服务器反复崩溃，不再自动重启，避免无限循环消耗资源
				s.mu.Lock()
				s.permanentFailure = true
				s.mu.Unlock()
				s.SetStatus(false, "永久失败：服务器反复崩溃，请检查配置或重启应用")
				if onStatusChange != nil {
					onStatusChange(s.Status())
				}
				log.Error().
					Int("restart_attempts", restartCount).
					Msg("[server] permanent failure: server crashed repeatedly, giving up auto-restart")
				return
			}

			backoff := currentBackoff
			restartCount++
			currentBackoff = min(currentBackoff*2, maxBackoff)

			// 推测解码崩溃回退：若推测解码已启用且服务器崩溃，自动禁用推测解码
			// 窗口设为 120 秒以覆盖大模型加载时间（加载期间崩溃也视为推测解码问题）
			// RF-3 修复：用 atomic.Load/Store 替代直接读写，消除数据竞争
			if !s.mtpFallbackDisabled.Load() && s.config.SpecType != "" {
				runDuration := time.Since(s.lastStartTime)
				if runDuration < 120*time.Second {
					s.mtpFallbackDisabled.Store(true)
					log.Warn().
						Dur("run_duration", runDuration).
						Str("spec_type", s.config.SpecType).
						Msg("[server] speculative decoding crash detected, restarting without speculative decoding")
					backoff = 1 * time.Second // 快速重启
				}
			}

			s.SetStatus(false, fmt.Sprintf("server crashed, restarting in %v (attempt %d/%d)", backoff, restartCount, maxAttempts))
			if onStatusChange != nil {
				onStatusChange(s.Status())
			}

			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}

			if err := s.Start(); err != nil {
				s.SetStatus(false, fmt.Sprintf("restart failed: %v", err))
				if onStatusChange != nil {
					onStatusChange(s.Status())
				}
				continue
			}

			if err := s.WaitForReady(60 * time.Second); err != nil {
				// context 已取消则不再继续重启
				if ctx.Err() != nil {
					return
				}
				s.SetStatus(false, fmt.Sprintf("server not ready after restart: %v", err))
				if onStatusChange != nil {
					onStatusChange(s.Status())
				}
				continue
			}

			s.SetStatus(true, "")
			restartCount = 0
			currentBackoff = initialBackoff
			if onRestartSuccess != nil {
				onRestartSuccess()
			}
			if onStatusChange != nil {
				onStatusChange(s.Status())
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInt):
		}
	}
}

// IsPermanentFailure 返回服务器是否处于永久失败状态（反复崩溃后不再自动重启）
func (s *Server) IsPermanentFailure() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.permanentFailure
}

func (s *Server) isAlive() bool {
	if s.pty != nil {
		// ConPTY 模式：Wait goroutine 会在进程退出时设置 Running=false
		return s.status.Running
	}
	if s.cmd == nil || s.cmd.Process == nil {
		return false
	}
	return s.cmd.ProcessState == nil || !s.cmd.ProcessState.Exited()
}

func (s *Server) SetStatus(running bool, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = ServerStatus{Running: running, Error: errMsg}
}

func (s *Server) Ctx() context.Context {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ctx
}

func (s *Server) CloseJob() {
	if s.job != nil {
		s.job.Close()
	}
}

func (s *Server) LastOutput() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.stderrBuf == nil {
		return ""
	}
	return s.stderrBuf.String()
}

// GetSpecType 返回当前服务器的推测解码类型（MTP 崩溃回退后返回空字符串）
func (s *Server) GetSpecType() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	// RF-3 修复：atomic.Bool 读取，无需持锁也是 race-free，这里 RLock 仅保护 s.config.SpecType
	if s.mtpFallbackDisabled.Load() {
		return ""
	}
	return s.config.SpecType
}
