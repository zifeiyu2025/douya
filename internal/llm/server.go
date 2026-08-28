// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"fmt"
	"net/http"
	"os/exec"
	"regexp"
	"strconv"
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

// exitCodeRegexp 匹配 "exit_code=-123"、"exit_code: 123"、"exit_code=3221226507" 等格式，
// 提取退出码数字。包级变量只编译一次，避免 enhanceExitError 每次调用都重新编译。
// P2-3 修复：原实现用 strings.Contains 硬匹配 "exit_code=-1073740791"，
// 一旦格式变化（如多余空格、正数形式、分隔符不同）就会漏匹配。
// 生活类比：不再逐字比对化验单上的"exit_code=-1073740791"这串字符，
// 而是用模板"exit_code 后面跟着一个数字"把数字抠出来，再按数值判断病情。
var exitCodeRegexp = regexp.MustCompile(`exit_code[=: ]\s*(-?\d+)`)

// stackOverflowExitCodes 栈溢出相关退出码（含无符号正数形式）
// 0xC0000409 STATUS_STACK_BUFFER_OVERRUN = -1073740791 / 3221226507
// 0xC00000FD STATUS_STACK_OVERFLOW = -1073741571 / 3221225725
var stackOverflowExitCodes = map[int]bool{
	-1073740791: true, // STATUS_STACK_BUFFER_OVERRUN
	3221226507:  true,
	-1073741571: true, // STATUS_STACK_OVERFLOW
	3221225725:  true,
}

// IsStackOverflowExit 检测错误信息是否包含栈溢出崩溃退出码。
// 供 app 层复用（app_server_watch.go isStackOverflowCrash 原先用 strings.Contains
// 硬匹配四种格式，格式一旦变化即漏匹配；统一走正则提取 + 数值判断，单一事实来源）。
// 生活类比：像统一的"诊断仪"——app 层不再各自用土办法猜，都插到这台仪器上读数。
func IsStackOverflowExit(errStr string) bool {
	m := exitCodeRegexp.FindStringSubmatch(errStr)
	if m == nil {
		return false
	}
	code, err := strconv.Atoi(m[1])
	if err != nil {
		return false
	}
	return stackOverflowExitCodes[code]
}

type ServerConfig struct {
	ModelsDir     string
	MmprojAuto    bool
	MmprojOffload bool
	MmprojDevice  string // 视觉投影专用 GPU 设备名（多显卡分卡）；空=不传递，none=关闭卸载
	ServerPath    string
	// BackendType 当前使用的计算后端类型（cuda/hip/sycl/vulkan/openvino/cpu），不含 auto。
	// 由启动流程根据硬件和配置解析后传入，供后续逻辑（如参数调优、日志记录）使用。
	// 生活类比：记录当前车装的是什么型号的发动机，供后续保养（参数调优）参考。
	BackendType BackendType
	Port        int
	GPULayers   string
	Threads     int
	FlashAttn   string // "on"/"off"/"auto"，对应 llama.cpp --flash-attn 参数
	CacheTypeK  string
	CacheTypeV  string
	Mlock       bool
	KVUnified   bool
	// KVUnifiedPerSlot 统一 KV 池下每个并行 slot 的独立上下文上限（0=不传，跟随上游默认）。
	// 上游 b10675 新增（--kv-unified-per-slot），见 config.KVUnifiedPerSlot。
	KVUnifiedPerSlot       int
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
	// 多 GPU 原生参数（对应 llama.cpp --split-mode / --tensor-split / --main-gpu）
	// SplitMode 为空表示不覆盖 llama.cpp 默认的 layer 模式
	SplitMode string
	// TensorSplit 为逗号分隔的设备权重（如 "3,1" 表示 75%/25%），空表示不传递
	TensorSplit string
	// MainGPU 主 GPU 索引（-1=不传递，让 llama.cpp 使用默认设备）
	MainGPU             int
	APIKey              string
	ServerAPIKeyEnabled bool // 是否启用服务 API Key 验证（暴露到局域网时强制要求）

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
	EnableBuiltinTools     bool
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
	Agent      bool   // 一键启用 CORS 代理 + 所有内置工具
	UIMcpProxy bool   // 仅启用 MCP CORS 代理
	AgentCwd   string // Agent 工具的工作目录（read_file/exec 等相对路径的解析基准；空=引擎所在目录）
	// 细粒度 CORS 配置（上游 --cors-*，llama.cpp #25655）
	CorsOrigins     string // 允许的来源，逗号分隔，空=使用 llama.cpp 默认
	CorsMethods     string // 允许的 HTTP 方法，逗号分隔
	CorsHeaders     string // 允许的请求头，逗号分隔
	CorsCredentials bool   // 是否允许携带凭证
	// 后端采样（实验性，将采样逻辑移到 GPU 执行，不兼容 grammar 和 reasoning budget）
	BackendSampling bool
	// SSE ping 间隔秒数（0=使用服务器默认 30 秒，用于保持长连接活跃）
	SsePingInterval int
	// LoRA 适配器路径（逗号分隔，启动时通过 --lora 加载，配合 --lora-init-without-apply 默认不应用）
	LoraPaths string
	// ChatTemplateFile 自定义聊天模板文件路径（.jinja 文件）
	// 通过 --chat-template-file 传递给 llama-server，优先于模型 GGUF 自带模板
	ChatTemplateFile string
	// Reranker 模型路径（配置后自动启用 --rerank 端点）
	RerankerModelPath string
	// 直接 I/O（绕过操作系统页面缓存，加速大模型加载）
	DirectIO bool
	// MoE 权重 CPU 卸载（将所有专家权重保留在 CPU）
	CPUMoe bool
	// 前 N 层 MoE 权重 CPU 卸载（0=不启用）
	NCpuMoe int
	// 前 N 层 FFN 权重 CPU 卸载（0=不启用）。上游 b10675 新增（--n-cpu-ffn），
	// 覆盖面比 --n-cpu-moe 广（非 MoE 模型也可用），见 config.NCpuFfn。
	NCpuFfn int
	// 算子卸载开关（nil=使用默认值，true=--op-offload，false=--no-op-offload）
	OpOffload *bool
}

// LoadMode 根据配置推导 llama.cpp --load-mode 值（上游 #20834 将 mlock/mmap/direct-io 合并）。
//
// 优先级（按用户意图从强到弱）：
//  1. DirectIO=true        → "dio"（绕过页面缓存直读盘）
//  2. Mlock=true           → "mlock"（mmap + 锁定到 RAM）
//  3. Mmap=false           → "none"（关闭内存映射）
//  4. 其他情况             → "mmap"（二进制原生默认值）
//
// 注意：本应用捆绑的 llama-server（llama.cpp b10355）的 --load-mode 仅接受
// none / mmap / mlock / mmap+mlock / dio，并不支持上游后续引入的 "auto"。
// 因此默认分支返回 "mmap"（即该版本的默认值），显式传递以保持行为确定性，
// 同时保证在所有受支持的二进制版本上都是合法取值。调用方在结果非空时始终传递 --load-mode。
// 生活类比：就像汽车换挡——同时踩了多个开关时，按顺序取第一个生效的档位。
func (c *ServerConfig) LoadMode() string {
	if c.DirectIO {
		return "dio"
	}
	if c.Mlock {
		return "mlock"
	}
	if !c.Mmap {
		return "none"
	}
	return "mmap"
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
	mu        sync.RWMutex
	job       *JobObject
	stderrBuf *RingBuffer
	// RF-3 修复：mtpFallbackDisabled 改用 atomic.Bool，消除 WatchWithCallback goroutine
	// 与 buildStartArgs/GetSpecType 之间的数据竞争（goroutine 无锁写入 vs 持锁读取）。
	// 生活类比：调度中心的总开关状态牌，任何值班员都能直接看/拨（atomic 操作），
	// 不需要先去前台排队借钥匙（mu 锁），避免排班冲突。
	mtpFallbackDisabled atomic.Bool
	// crashDegradeLevel 崩溃降级级别（atomic，WatchWithCallback 写、buildStartArgs 读）
	// 0=无降级，1=ctx-size 减半，2=gpu-layers 设为 auto（让 llama.cpp 自决）
	// 生活类比：汽车连续抛锚后，维修工会逐级降档——先限速（ctx 减半），
	// 再挂空挡让拖车拖（gpu-layers auto），最后才换发动机（后端回滚）。
	// 每级降级都记录日志并推送前端提示，启动成功后自动重置为 0。
	crashDegradeLevel atomic.Int32
	// lastStartTime 存储上次启动的时刻（UnixNano），用 atomic.Int64 避免
	// WatchWithCallback 中无锁读取与 Start 中持锁写入之间的数据竞争。
	// 生活类比：调度中心的电子时钟，任何值班员都能直接抬头看（atomic 读取），
	// 不需要去前台排队借钥匙（mu 锁），也不会被正在调时的同事挡住。
	lastStartTime      atomic.Int64
	cmdEnv             []string          // 安全传递给子进程的环境变量（如 API Key）
	onLog              func(line string) // 日志行回调（用于实时推送到前端）
	onTerminalData     func(data []byte) // 终端原始字节流回调（用于 xterm.js 渲染）
	healthClient       *http.Client      // 复用的健康检查 HTTP 客户端（WaitForReady/GracefulStop 共用）
	permanentFailure   bool              // 永久失败标志：服务器反复崩溃后不再自动重启
	maxRestartAttempts int               // 最大重启尝试次数（默认 10，可配置便于测试）
	initialBackoff     time.Duration     // 初始退避时间（默认 2s，可配置便于测试）
	pollInterval       time.Duration     // 轮询间隔（默认 1s，可配置便于测试）
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

// ApplyBackendSafetyLimits 对 ServerConfig 应用后端硬限制。
// 在所有用户覆盖之后调用，确保后端安全限制不可被绕过。
//
// 修复的 bug：原实现中 system.applyBackendSpecificParams 修改 sp.GPULayers
// （如 Vulkan 限到 50），但 app 层 resolveDerivedServerParams 中用户显式
// cfg.GPULayers=99 会直接用用户值，绕过了 sp 中已应用的 Vulkan 50 层限制。
// 此方法作为最后兜底，作用在最终 ServerConfig 上。
//
// 生活类比：就像交规限速——不管司机（用户）把油门踩多深，限速牌（后端硬限制）说了算。
func (c *ServerConfig) ApplyBackendSafetyLimits(backend BackendType) {
	switch backend {
	case BackendVulkan:
		// Vulkan 后端安全限制（v2 放宽版，对齐 llama.cpp 原生 auto 行为）：
		//   - gpu_layers <= 99：允许全层卸载。旧版限 50 是针对当年 Vulkan 栈溢出
		//     崩溃（0xC0000409）的临时措施；上游 llama.cpp 已修复且豆芽跟踪最新构建，
		//     继续限 50 会导致大模型（>50 层，如 32B 系列）在 AMD/Intel 卡上只能半载。
		//     恢复路径：崩溃时 enhanceExitError 提示 + 后端回退链仍生效。
		//   - ctx_size <= 32768：与 CUDA/Blackwell 规则对齐（防超长上下文 OOM），
		//     旧版 8192 封顶导致非 N 卡用户长上下文不可用。
		if ngl, err := strconv.Atoi(c.GPULayers); err == nil && ngl > 99 {
			log.Warn().Str("original_ngl", c.GPULayers).Int("capped_ngl", 99).
				Msg("[server-config] Vulkan backend safety limit: gpu-layers capped to 99")
			c.GPULayers = "99"
		}
		if c.ContextSize > 32768 {
			log.Warn().Int("original_ctx", c.ContextSize).Int("capped_ctx", 32768).
				Msg("[server-config] Vulkan backend safety limit: ctx-size capped to 32768")
			c.ContextSize = 32768
		}
	case BackendCPU:
		// CPU 后端硬限制：gpu_layers = 0（无 GPU），ctx_size <= 32768
		// ctx 上限与 GPU 后端统一（KV 走内存，llama.cpp 原生不封顶，此处仅防极端 OOM）
		if ngl, err := strconv.Atoi(c.GPULayers); err == nil && ngl > 0 {
			log.Warn().Str("original_ngl", c.GPULayers).Int("capped_ngl", 0).
				Msg("[server-config] CPU backend safety limit: gpu-layers forced to 0")
			c.GPULayers = "0"
		}
		if c.ContextSize > 32768 {
			log.Warn().Int("original_ctx", c.ContextSize).Int("capped_ctx", 32768).
				Msg("[server-config] CPU backend safety limit: ctx-size capped to 32768")
			c.ContextSize = 32768
		}
	}
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

// enhanceStartError 增强启动错误信息，检测 DLL 缺失、端口占用等常见问题
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
		return apperror.Wrap(apperror.KindInternal, "启动引擎失败，可能是 DLL 文件缺失\n请检查 runtime/ 目录是否包含所有必要的 DLL 文件", err)
	}

	// 引擎 exe 本身不存在
	if strings.Contains(lower, "the system cannot find the file specified") ||
		strings.Contains(lower, "no such file or directory") {
		return apperror.Wrap(apperror.KindInternal, "启动引擎失败，引擎程序文件不存在\n请检查 config.json 中的 llama_server_path 配置", err)
	}

	return err
}

// enhanceExitError 增强进程退出错误信息，检测端口占用等常见运行时问题。
//
// F-2 修复：llama-server 因端口冲突崩溃时，错误信息通常包含 "address already in use"，
// 但原始错误对用户不友好，这里翻译成明确的中文提示。
// 生活类比：发动机启动后又熄火了，技师通过故障码判断是排气管被堵（端口占用），
// 直接告诉驾驶员而不是让他自己看故障码。
//
// B-5 增强：检测 Windows 进程退出码，识别栈溢出等常见崩溃并给出明确诊断。
// 常见退出码：
//   - 0xC0000409 (-1073740791): STATUS_STACK_BUFFER_OVERRUN 栈缓冲区溢出
//     常见原因：Vulkan 后端 gpu_layers 过大、ctx-size 过大、模型架构不兼容
//   - 0xC00000FD (-1073741571): STATUS_STACK_OVERFLOW 栈溢出
//   - 0xC0000005 (-1073741819): STATUS_ACCESS_VIOLATION 内存访问违规
//     常见原因：DLL 不兼容、驱动问题、模型文件损坏
func enhanceExitError(errMsg string, port int) string {
	lower := strings.ToLower(errMsg)
	if strings.Contains(lower, "address already in use") ||
		strings.Contains(lower, "port is already in use") ||
		strings.Contains(lower, "bind: address already in use") {
		return fmt.Sprintf("%s\n\n提示：端口 %d 被占用，可能是其他程序或残留的 llama-server 进程占用了该端口。\n请关闭占用该端口的程序，或在设置中修改端口号后重启", errMsg, port)
	}

	// 检测 Windows 进程退出码，给出针对性诊断
	// P2-3 修复：用正则提取 exit_code 后的数字，按数值匹配，兼容
	// "exit_code=-1073740791"、"exit_code: 3221226507"、"exit_code =-1073741819" 等多种格式。
	// 生活类比：故障码就像医生的化验单，不同数值代表不同疾病，对症下药才有效
	if m := exitCodeRegexp.FindStringSubmatch(errMsg); m != nil {
		if code, convErr := strconv.Atoi(m[1]); convErr == nil {
			switch code {
			case -1073740791, 3221226507: // 0xC0000409 STATUS_STACK_BUFFER_OVERRUN
				return errMsg + "\n\n诊断：进程栈缓冲区溢出（0xC0000409）。\n" +
					"常见原因：Vulkan 后端 gpu_layers 过大、ctx-size 过大、或模型架构不兼容。\n" +
					"建议：1) 切换到 CUDA 后端（NVIDIA 显卡）；2) 减小 gpu_layers；3) 减小 ctx-size；4) 更新显卡驱动。"
			case -1073741571, 3221225725: // 0xC00000FD STATUS_STACK_OVERFLOW
				return errMsg + "\n\n诊断：进程栈溢出（0xC00000FD）。\n" +
					"常见原因：模型层数过多、gpu_layers 设置过高、或后端兼容性问题。\n" +
					"建议：1) 减小 gpu_layers；2) 切换到其他后端；3) 使用更小的模型。"
			case -1073741819, 3221225477: // 0xC0000005 STATUS_ACCESS_VIOLATION
				return errMsg + "\n\n诊断：内存访问违规（0xC0000005）。\n" +
					"常见原因：DLL 不兼容、显卡驱动问题、或模型文件损坏。\n" +
					"建议：1) 更新显卡驱动；2) 重新下载模型文件；3) 切换到其他后端。"
			}
		}
	}

	return errMsg
}

// readConPTYOutput 持续读取 ConPTY 输出，50ms 窗口批量发送到前端
// 生活类比：就像邮递员收集信件，不是收到一封就跑一趟，而是攒一批一起送，效率更高
//
// P1-3 修复：pty.Read 是阻塞 I/O，原实现直接在主循环调用，ctx 取消后
// 仍可能永久阻塞在 Read 上导致 goroutine 泄漏。现将 Read 放入独立 goroutine，
// 通过带缓冲 channel 传递结果，主循环用 select 同时监听 ctx.Done 和读取结果。
// ctx 取消时主循环立即返回；阻塞中的 Read goroutine 会在 pty.Close() 被调用后
// 返回，由于 readCh 缓冲为 1，goroutine 写入后即可退出，不会泄漏。
func (s *Server) readConPTYOutput() {
	buf := make([]byte, 8192)
	var pending []byte
	lastFlush := time.Now()

	// readCh 用于接收 pty.Read 的结果，缓冲为 1 使得读取 goroutine
	// 在主循环退出后仍能写入结果并退出，不会永久阻塞在 channel 发送上。
	// 生活类比：邮筒有容量，邮递员把信投进去就能下班，即使收件人出门了。
	readCh := make(chan struct {
		n   int
		err error
	}, 1)

	// startRead 发起一次非阻塞读取：启动 goroutine 执行 pty.Read，
	// 结果通过 readCh 返回。任意时刻最多只有一个 goroutine 在写 buf，
	// 且主循环在 select 收到结果后才读 buf，channel 同步保证 happens-before。
	startRead := func() {
		go func() {
			// 防止 panic 导致整个进程崩溃（pty.Read 可能因底层资源问题 panic）
			defer func() {
				if r := recover(); r != nil {
					readCh <- struct {
						n   int
						err error
					}{n: 0, err: apperror.Newf(apperror.KindInternal, "pty read panic: %v", r)}
				}
			}()
			n, err := s.pty.Read(buf)
			readCh <- struct {
				n   int
				err error
			}{n: n, err: err}
		}()
	}

	// 发起首次读取
	startRead()

	for {
		select {
		case <-s.ctx.Done():
			// ctx 取消：发送剩余数据后立即退出，不再等待下一次 Read。
			// 阻塞中的 Read goroutine 会在后续 pty.Close() 调用后返回，
			// readCh 有缓冲，goroutine 能写入后正常退出。
			s.flushConPTYPending(&pending)
			return
		case res := <-readCh:
			n, err := res.n, res.err
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
				s.flushConPTYPending(&pending)
				lastFlush = now
			}

			if err != nil {
				// 读取出错或 EOF，发送剩余数据后退出
				s.flushConPTYPending(&pending)
				return
			}

			// 发起下一次读取
			startRead()
		}
	}
}

// flushConPTYPending 将待发送缓冲通过 onTerminalData 回调一次性发出，并清空缓冲。
// 提取此辅助函数避免在 readConPTYOutput 的多个退出路径中重复加锁/回调逻辑。
func (s *Server) flushConPTYPending(pending *[]byte) {
	if len(*pending) == 0 {
		return
	}
	s.mu.RLock()
	cb := s.onTerminalData
	s.mu.RUnlock()
	if cb != nil {
		cb(*pending)
	}
	*pending = (*pending)[:0]
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
	return s.WaitForReadyCtx(context.Background(), timeout)
}

// WaitForReadyCtx 等待服务器就绪，支持通过 ctx 提前取消。
// 与 WaitForReady 的区别：每次轮询都会检查 ctx.Done()，
// 调用方传入 watchCtx/rootCtx 后，shutdown 可立即终止等待，
// 避免退出流程被 60s 的超时阻塞。
func (s *Server) WaitForReadyCtx(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	client := s.healthClient
	url := fmt.Sprintf("http://127.0.0.1:%d/health", s.config.Port)

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
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
			// F-2 修复：检测端口占用等常见退出原因，增强错误提示
			errMsg = enhanceExitError(errMsg, s.config.Port)
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
		// b10605+ 的 llama-server 路由表已移除 /shutdown 端点（返回 404/405）。
		// 注意 HTTP 404 不会产生 Go 层面的 err，若不在此拦截会误以为"请求已送达"
		// 而白等满整个超时才强制停止；此时应立即降级为强制停止
		if resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusMethodNotAllowed {
			log.Info().Int("status", resp.StatusCode).Msg("[server] /shutdown not supported by this llama-server build, falling back to force stop")
			return s.Stop()
		}
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
		// P2-6 修复：两条路径都应等待 waitDone goroutine 结束，避免 goroutine 泄漏。
		// exec.Cmd 路径：cmd.Wait() 在 force kill 后会很快返回，直接等待即可。
		// ConPTY 路径：pty.Wait(ctx) 受其内部 ctx（3s）控制，可能在 force kill 后仍未返回，
		// 这里额外给 2s 超时兜底，超时则放弃等待并记录日志，防止永久阻塞。
		if mode == "cmd" {
			<-waitDone
		} else {
			select {
			case <-waitDone:
			case <-time.After(2 * time.Second):
				log.Warn().Str("mode", mode).Int("pid", pid).Msg("[server] timed out waiting for pty wait goroutine after force kill")
			}
		}
		s.mu.Lock()
		s.status = ServerStatus{Running: false}
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		return apperror.New(apperror.KindInternal, "server did not terminate gracefully, force killed")
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
			// P1-1 修复：lastStartTime 改用 atomic.Int64，此处无锁读取也是 race-free
			if !s.mtpFallbackDisabled.Load() && s.config.SpecType != "" {
				startTime := time.Unix(0, s.lastStartTime.Load())
				runDuration := time.Since(startTime)
				if runDuration < 120*time.Second {
					s.mtpFallbackDisabled.Store(true)
					log.Warn().
						Dur("run_duration", runDuration).
						Str("spec_type", s.config.SpecType).
						Msg("[server] speculative decoding crash detected, restarting without speculative decoding")
					backoff = 1 * time.Second // 快速重启
				}
			}

			// 扩展降级链：崩溃时逐级降级 ctx-size 和 gpu-layers
			// 降级链：推测解码 Off → ctx-size 减半 → gpu-layers auto → 后端回滚（app 层）
			// 生活类比：汽车抛锚后先关空调（推测解码），再限速（ctx 减半），
			// 再挂空挡（gpu-layers auto），最后才换发动机（后端回滚）
			// P2 修复：此降级链不再以 mtpFallbackDisabled 为前提。此前仅在
			// 推测解码模型崩溃后才生效，普通模型 OOM 崩溃只会无限重启到 10 次封顶。
			// 现在任意模型在启动后 120 秒内崩溃都逐级降层。
			startTime := time.Unix(0, s.lastStartTime.Load())
			runDuration := time.Since(startTime)
			currentLevel := s.crashDegradeLevel.Load()
			// 只在启动后 120 秒内崩溃才触发降级（覆盖加载期崩溃）
			if runDuration < 120*time.Second {
				// P2-2 修复：OOM（显存/内存不足）崩溃直接跳到降级 2（gpu-layers auto）。
				// ctx-size 减半只能缓解 KV-cache 占显存，对模型权重占显存无益；
				// 而 gpu-layers auto 让 llama.cpp 按剩余显存自决层数，是 OOM 最直接的救星。
				// 生活类比：车没油（VRAM 满）时先别再纠结车速（ctx），直接挂空挡省油（auto）。
				oom := false
				s.mu.RLock()
				if s.stderrBuf != nil {
					oom = DetectOOMInStderr(s.stderrBuf.String())
				}
				s.mu.RUnlock()

				if currentLevel < 2 && oom {
					s.crashDegradeLevel.Store(2)
					log.Warn().
						Dur("run_duration", runDuration).
						Str("original_ngl", s.config.GPULayers).
						Msg("[server] OOM crash detected, jumping degrade level 2: setting gpu-layers to auto")
					backoff = 1 * time.Second
				} else if currentLevel < 1 && !oom {
					// 降级 1：ctx-size 减半（最小 2048，避免过小无法使用）
					s.crashDegradeLevel.Store(1)
					log.Warn().
						Dur("run_duration", runDuration).
						Int("original_ctx", s.config.ContextSize).
						Msg("[server] crash degrade level 1: halving ctx-size for next restart")
					backoff = 1 * time.Second
				} else if currentLevel < 2 && !oom {
					// 降级 2：gpu-layers 设为 auto（让 llama.cpp 自决层数）
					s.crashDegradeLevel.Store(2)
					log.Warn().
						Dur("run_duration", runDuration).
						Str("original_ngl", s.config.GPULayers).
						Msg("[server] crash degrade level 2: setting gpu-layers to auto for next restart")
					backoff = 1 * time.Second
				}
				// 降级 3 及以上交给 app 层的 tryRollbackBackend 处理（后端回滚）
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

			if err := s.WaitForReadyCtx(ctx, 60*time.Second); err != nil {
				// context 已取消则不再继续重启
				if ctx.Err() != nil {
					return
				}
				// F-2 修复：增强端口占用等常见错误的提示
				enhancedMsg := enhanceExitError(err.Error(), s.config.Port)
				s.SetStatus(false, fmt.Sprintf("server not ready after restart: %s", enhancedMsg))
				if onStatusChange != nil {
					onStatusChange(s.Status())
				}
				continue
			}

			s.SetStatus(true, "")
			restartCount = 0
			currentBackoff = initialBackoff
			// 启动成功后重置崩溃降级级别（推测解码回退不重置，避免反复崩溃循环）
			if s.crashDegradeLevel.Load() > 0 {
				log.Info().Int32("degrade_level", s.crashDegradeLevel.Load()).Msg("[server] restart success, resetting crash degrade level")
				s.crashDegradeLevel.Store(0)
			}
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
	// 与 bindProcessToJobObject 的写入侧（持有 s.mu）对齐：读取 s.job 必须持锁，
	// 否则应用退出清理可能与模型启动流程并发读写 s.job 构成数据竞争。
	s.mu.Lock()
	defer s.mu.Unlock()
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

// DetectOOMInStderr 判断 llama-server 的 stderr 输出是否属于 OOM（显存/内存不足）。
// 用于崩溃降级链：OOM 崩溃直接跳到 gpu-layers auto，比 ctx 减半更对症。
// 也供 app 层 detectOOMError 复用，保证各处的 OOM 关键词表是单一事实来源。
func DetectOOMInStderr(stderr string) bool {
	if stderr == "" {
		return false
	}
	lower := strings.ToLower(stderr)
	// P3.1 修复：统一关键词表（此前 app 层与 llm 层各维护一份，85% 重复但行为不一致：
	// 如 "gpu memory"、"std::bad_alloc" 只在 app 层出现，llm 崩溃降级链检测不到）。
	// 此处合并为一份，两个消费方共用，避免同一 OOM 场景一处提示一处不降级。
	oomPatterns := []string{
		// CUDA 显存不足
		"cuda error", "cuda_error_out_of_memory", "out of memory",
		"failed to allocate cuda", "failed to allocate gpu",
		"not enough gpu memory", "gpu memory", "vram",
		// 系统内存不足
		"bad_alloc", "std::bad_alloc", "bad allocation",
		"cannot allocate memory", "mmap failed", "memory allocation failed",
	}
	for _, p := range oomPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}
