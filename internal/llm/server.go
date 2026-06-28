// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"github.com/UserExistsError/conpty"
	"github.com/rs/zerolog/log"
)

const vramCheckInterval = 500 * time.Millisecond
const vramCheckTimeout = 15
const healthCheckTimeout = 2 * time.Second // 健康检查超时

// allowedCacheTypes 列出 llama.cpp 允许的 KV cache 类型
// 已删除的类型：q2_k, q3_k, q4_k, q5_k, q6_k, iq4_xs
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
	ModelsDir        string
	MmprojAuto       bool
	MmprojOffload    bool
	ServerPath       string
	Port             int
	GPULayers        string
	Threads          int
	FlashAttn        string // "on"/"off"/"auto"，对应 llama.cpp --flash-attn 参数
	CacheTypeK       string
	CacheTypeV       string
	Mlock            bool
	KVUnified        bool
	CacheIdleSlots   bool
	CacheRAM         int
	ImageMinTokens   int
	ImageMaxTokens   int
	FitTarget        int
	FitCtx           int
	Reasoning        string
	ReasoningBudget  int
	ReasoningFormat  string
	ReasoningBudgetMessage string
	APIBase          string
	AppDir           string
	ModelsPreset     string
	ModelsMax        int
	SleepIdleSeconds int
	Mmap             bool
	KVOffload        bool
	ContextShift     bool
	MinP             float64
	DryMultiplier    float64
	DryBase          float64
	DryAllowedLength int
	DrySequenceBreaker string
	DryPenaltyLastN    int
	GrpAttnN          int
	GrpAttnW          int
	Jinja             *bool  // Jinja2 模板引擎开关
	CachePrompt       *bool  // Prompt 缓存控制
	Metrics           bool   // 服务器指标端点开关
	Verbose           bool   // 详细日志开关
	SpecDraftThreads      int  // Draft 模型线程数
	SpecDraftThreadsBatch int  // Draft 模型批处理线程数
	SpecDefault           bool // 使用默认推测解码配置
	Device           string
	Parallel         int
	APIKey           string
	SpecType         string
	SpecDraftNMax    int
	SpecDraftNMin    int
	CacheTypeKDraft  string
	CacheTypeVDraft  string
	SpecNgramModNMin   int
	SpecNgramModNMax   int
	SpecNgramModNMatch int
	SpecNgramSimpleSizeN   int
	SpecNgramSimpleSizeM   int
	SpecNgramSimpleMinHits int
	SpecNgramMapKSizeN     int
	SpecNgramMapKSizeM     int
	SpecNgramMapKMinHits   int
	SpecNgramMapK4VSizeN   int
	SpecNgramMapK4VSizeM   int
	SpecNgramMapK4VMinHits int
	LookupCacheStatic  string
	LookupCacheDynamic string
	SpecDraftModel     string
	Embedding          bool   // 启用 /v1/embeddings API（RAG 知识库需要）
	Pooling            string // 嵌入池化类型（mean/cls），解决聊天模型 pooling=none 不兼容 OAI embedding API
	ExposeServer       bool   // 暴露服务器地址，允许局域网访问
	SwaFull              bool
	CtxCheckpoints       int
	CheckpointMinStep    int
	Tools                string
	PrefillAssistant     bool
	SlotPromptSimilarity float64
	SkipChatParsing      bool
	APIPrefix            string
	SimpleIO             bool
	BatchSize            int
	UBatchSize           int
	ContextSize          int
	// KV 缓存持久化
	SlotSavePath    string // 启用后传递 --slot-save-path
	SlotSaveEnabled bool
	CacheReuse      int    // KV 缓存复用块大小（0=禁用）
	// Draft 模型 GPU 配置（Eagle3 等场景）
	SpecDraftNgl    int    // draft 模型 GPU 层数
	SpecDraftDevice string // draft 模型设备（如 "cuda:0"）
	// Draft 模型推测解码参数
	SpecDraftPSplit     float64 // 推测解码 split 概率（默认 0.10）
	SpecDraftPMin       float64 // 最小推测解码概率（默认 0.00）
	SpecDraftBackendSampling *bool // draft 模型后端采样（nil=默认启用）
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
}

type Server struct {
	cmd                  *exec.Cmd
	pty                  *conpty.ConPty // ConPTY 伪控制台（非 nil 表示使用 ConPTY 模式）
	config               *ServerConfig
	status               ServerStatus
	ctx                  context.Context
	cancel               context.CancelFunc
	mu                   sync.RWMutex
	job                  *JobObject
	stderrBuf            *RingBuffer
	mtpFallbackDisabled  bool
	lastStartTime        time.Time
	cmdEnv               []string                  // 安全传递给子进程的环境变量（如 API Key）
	onLog                func(line string)          // 日志行回调（用于实时推送到前端）
	onTerminalData       func(data []byte)          // 终端原始字节流回调（用于 xterm.js 渲染）
	healthClient         *http.Client               // 复用的健康检查 HTTP 客户端（WaitForReady/GracefulStop 共用）
}

func NewServer(cfg *ServerConfig) *Server {
	return &Server{
		config:       cfg,
		status:       ServerStatus{Running: false},
		healthClient: &http.Client{Timeout: healthCheckTimeout},
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

// buildStartArgs 根据配置组装 llama-server 启动命令行参数。
// 从 Start() 抽出以降低单函数复杂度。
// 这是纯函数：只读 s.config 和 s.mtpFallbackDisabled，不修改任何状态，无副作用。
// 注意：API Key 通过环境变量传递（涉及 s.cmdEnv 状态修改），相关逻辑保留在 Start() 中。
func (s *Server) buildStartArgs() []string {
	args := []string{
		"--models-dir", s.config.ModelsDir,
		"--port", fmt.Sprintf("%d", s.config.Port),
		"--jinja",
		"--fit", "on",
		// 禁用 llama-server 自带的 Web UI：豆芽有自己的 Vue 前端，不使用原生 webui。
		// 好处：减少不必要的 HTTP 路由和静态资源占用，避免用户误访问原生 webui 造成混淆。
		"--no-webui",
	}

	// 根据配置决定绑定地址：暴露则 0.0.0.0（局域网可访问），否则 127.0.0.1（仅本机）
	if s.config.ExposeServer {
		args = append(args, "--host", "0.0.0.0")
	} else {
		args = append(args, "--host", "127.0.0.1")
	}

	if s.config.ModelsPreset != "" {
		args = append(args, "--models-preset", s.config.ModelsPreset)
		// 禁用路由器自动加载：豆芽通过 /models/load API 显式控制模型加载时机
		// 原版 llama.cpp 默认 models_autoload=true，会在请求到来时自动加载模型，
		// 这与豆芽的显式加载逻辑冲突，可能导致子进程参数不完整或加载状态混乱
		args = append(args, "--no-models-autoload")
	}
	args = appendIntArg(args, "--models-max", s.config.ModelsMax)
	args = appendIntArg(args, "--sleep-idle-seconds", s.config.SleepIdleSeconds)
	args = appendStringArg(args, "--gpu-layers", s.config.GPULayers)
	args = appendStringArg(args, "--flash-attn", s.config.FlashAttn)
	if s.config.CacheTypeK != "" {
		if isValidCacheType(s.config.CacheTypeK) {
			args = append(args, "--cache-type-k", s.config.CacheTypeK)
		} else {
			log.Warn().Str("type", s.config.CacheTypeK).Msg("[server] unsupported cache type, skipping --cache-type-k (removed q2_k/q3_k/q4_k/q5_k/q6_k/iq4_xs)")
		}
	}
	if s.config.CacheTypeV != "" {
		if isValidCacheType(s.config.CacheTypeV) {
			args = append(args, "--cache-type-v", s.config.CacheTypeV)
		} else {
			log.Warn().Str("type", s.config.CacheTypeV).Msg("[server] unsupported cache type, skipping --cache-type-v (removed q2_k/q3_k/q4_k/q5_k/q6_k/iq4_xs)")
		}
	}
	args = appendBoolArg(args, "--mlock", s.config.Mlock)
	args = appendIntArg(args, "-t", s.config.Threads)
	args = appendIntArg(args, "-b", s.config.BatchSize)
	args = appendIntArg(args, "-ub", s.config.UBatchSize)
	args = appendIntArg(args, "-c", s.config.ContextSize)
	args = appendBoolArg(args, "--mmproj-auto", s.config.MmprojAuto)
	args = appendBoolArg(args, "--mmproj-offload", s.config.MmprojOffload)
	args = appendStringArg(args, "--reasoning", s.config.Reasoning)
	args = appendIntArg(args, "--reasoning-budget", s.config.ReasoningBudget)
	args = appendStringArg(args, "--reasoning-format", s.config.ReasoningFormat)
	args = appendStringArg(args, "--reasoning-budget-message", s.config.ReasoningBudgetMessage)
	args = appendBoolArg(args, "--kv-unified", s.config.KVUnified)
	args = appendBoolArg(args, "--cache-idle-slots", s.config.CacheIdleSlots)
	args = appendIntArg(args, "--cache-ram", s.config.CacheRAM)
	args = appendIntArg(args, "--image-min-tokens", s.config.ImageMinTokens)
	args = appendIntArg(args, "--image-max-tokens", s.config.ImageMaxTokens)
	args = appendIntArg(args, "--fit-target", s.config.FitTarget)
	args = appendIntArg(args, "--fit-ctx", s.config.FitCtx)
	if !s.config.Mmap {
		args = append(args, "--no-mmap")
	}
	if !s.config.KVOffload {
		args = append(args, "--no-kv-offload")
	}
	args = appendBoolArg(args, "--context-shift", s.config.ContextShift)
	args = appendFloatArg(args, "--min-p", s.config.MinP, "%.2f")
	if s.config.DryMultiplier > 0 {
		args = append(args, "--dry-multiplier", fmt.Sprintf("%.2f", s.config.DryMultiplier))
		if s.config.DryBase > 0 {
			args = appendFloatArg(args, "--dry-base", s.config.DryBase, "%.2f")
		}
		if s.config.DryAllowedLength > 0 {
			args = appendIntArg(args, "--dry-allowed-length", s.config.DryAllowedLength)
		}
		// Dry 采样扩展参数
		if s.config.DrySequenceBreaker != "" {
			for _, breaker := range strings.Split(s.config.DrySequenceBreaker, ",") {
				breaker = strings.TrimSpace(breaker)
				if breaker != "" {
					args = append(args, "--dry-sequence-breaker", breaker)
				}
			}
		}
		if s.config.DryPenaltyLastN > 0 {
			args = appendIntArg(args, "--dry-penalty-last-n", s.config.DryPenaltyLastN)
		}
	}
	// 分组注意力参数
	args = appendIntArg(args, "--grp-attn-n", s.config.GrpAttnN)
	args = appendIntArg(args, "--grp-attn-w", s.config.GrpAttnW)
	// Jinja2 模板引擎开关
	if s.config.Jinja != nil {
		if *s.config.Jinja {
			args = append(args, "--jinja")
		} else {
			args = append(args, "--no-jinja")
		}
	}
	// Prompt 缓存控制
	if s.config.CachePrompt != nil {
		if *s.config.CachePrompt {
			args = append(args, "--cache-prompt")
		} else {
			args = append(args, "--no-cache-prompt")
		}
	}
	// 服务器指标端点
	args = appendBoolArg(args, "--metrics", s.config.Metrics)
	// 详细日志
	args = appendBoolArg(args, "--verbose", s.config.Verbose)
	// 重排序端点：配置了 reranker 模型时自动启用
	if s.config.RerankerModelPath != "" {
		args = append(args, "--rerank")
	}
	args = appendStringArg(args, "--device", s.config.Device)
	args = appendIntArg(args, "--parallel", s.config.Parallel)
	args = append(args, "--timeout", "900")
	// 注意：API Key 通过环境变量 LLAMA_API_KEY 传递，相关逻辑（s.cmdEnv 修改）保留在 Start() 中
	if s.config.SpecType != "" && !s.mtpFallbackDisabled {
		args = append(args, "--spec-type", s.config.SpecType)
	}
	if s.config.SpecDraftNMax > 0 && !s.mtpFallbackDisabled {
		args = append(args, "--spec-draft-n-max", fmt.Sprintf("%d", s.config.SpecDraftNMax))
	}
	if s.config.SpecDraftNMin > 0 && !s.mtpFallbackDisabled {
		args = append(args, "--spec-draft-n-min", fmt.Sprintf("%d", s.config.SpecDraftNMin))
	}
	if s.config.CacheTypeKDraft != "" && !s.mtpFallbackDisabled {
		if isValidCacheType(s.config.CacheTypeKDraft) {
			args = append(args, "--spec-draft-type-k", s.config.CacheTypeKDraft)
		} else {
			log.Warn().Str("type", s.config.CacheTypeKDraft).Msg("[server] unsupported cache type, skipping --spec-draft-type-k (removed q2_k/q3_k/q4_k/q5_k/q6_k/iq4_xs)")
		}
	}
	if s.config.CacheTypeVDraft != "" && !s.mtpFallbackDisabled {
		if isValidCacheType(s.config.CacheTypeVDraft) {
			args = append(args, "--spec-draft-type-v", s.config.CacheTypeVDraft)
		} else {
			log.Warn().Str("type", s.config.CacheTypeVDraft).Msg("[server] unsupported cache type, skipping --spec-draft-type-v (removed q2_k/q3_k/q4_k/q5_k/q6_k/iq4_xs)")
		}
	}
	if s.config.SpecNgramModNMin > 0 && s.config.SpecType == "ngram-mod" {
		args = append(args, "--spec-ngram-mod-n-min", fmt.Sprintf("%d", s.config.SpecNgramModNMin))
	}
	if s.config.SpecNgramModNMax > 0 && s.config.SpecType == "ngram-mod" {
		args = append(args, "--spec-ngram-mod-n-max", fmt.Sprintf("%d", s.config.SpecNgramModNMax))
	}
	if s.config.SpecNgramModNMatch > 0 && s.config.SpecType == "ngram-mod" {
		args = append(args, "--spec-ngram-mod-n-match", fmt.Sprintf("%d", s.config.SpecNgramModNMatch))
	}
	// ngram-simple 子参数
	if s.config.SpecNgramSimpleSizeN > 0 && s.config.SpecType == "ngram-simple" {
		args = append(args, "--spec-ngram-simple-size-n", fmt.Sprintf("%d", s.config.SpecNgramSimpleSizeN))
	}
	if s.config.SpecNgramSimpleSizeM > 0 && s.config.SpecType == "ngram-simple" {
		args = append(args, "--spec-ngram-simple-size-m", fmt.Sprintf("%d", s.config.SpecNgramSimpleSizeM))
	}
	if s.config.SpecNgramSimpleMinHits > 0 && s.config.SpecType == "ngram-simple" {
		args = append(args, "--spec-ngram-simple-min-hits", fmt.Sprintf("%d", s.config.SpecNgramSimpleMinHits))
	}
	// ngram-map-k 子参数
	if s.config.SpecNgramMapKSizeN > 0 && s.config.SpecType == "ngram-map-k" {
		args = append(args, "--spec-ngram-map-k-size-n", fmt.Sprintf("%d", s.config.SpecNgramMapKSizeN))
	}
	if s.config.SpecNgramMapKSizeM > 0 && s.config.SpecType == "ngram-map-k" {
		args = append(args, "--spec-ngram-map-k-size-m", fmt.Sprintf("%d", s.config.SpecNgramMapKSizeM))
	}
	if s.config.SpecNgramMapKMinHits > 0 && s.config.SpecType == "ngram-map-k" {
		args = append(args, "--spec-ngram-map-k-min-hits", fmt.Sprintf("%d", s.config.SpecNgramMapKMinHits))
	}
	// ngram-map-k4v 子参数
	if s.config.SpecNgramMapK4VSizeN > 0 && s.config.SpecType == "ngram-map-k4v" {
		args = append(args, "--spec-ngram-map-k4v-size-n", fmt.Sprintf("%d", s.config.SpecNgramMapK4VSizeN))
	}
	if s.config.SpecNgramMapK4VSizeM > 0 && s.config.SpecType == "ngram-map-k4v" {
		args = append(args, "--spec-ngram-map-k4v-size-m", fmt.Sprintf("%d", s.config.SpecNgramMapK4VSizeM))
	}
	if s.config.SpecNgramMapK4VMinHits > 0 && s.config.SpecType == "ngram-map-k4v" {
		args = append(args, "--spec-ngram-map-k4v-min-hits", fmt.Sprintf("%d", s.config.SpecNgramMapK4VMinHits))
	}
	// lookup-cache 仅在 ngram-cache 模式下传递
	if s.config.LookupCacheStatic != "" && s.config.SpecType == "ngram-cache" {
		args = append(args, "--lookup-cache-static", s.config.LookupCacheStatic)
	}
	if s.config.LookupCacheDynamic != "" && s.config.SpecType == "ngram-cache" {
		args = append(args, "--lookup-cache-dynamic", s.config.LookupCacheDynamic)
	}
	// draft 模型路径：仅在 draft-eagle3/draft-simple 模式下传递
	if s.config.SpecDraftModel != "" && (s.config.SpecType == "draft-eagle3" || s.config.SpecType == "draft-simple") {
		args = append(args, "--spec-draft-model", s.config.SpecDraftModel)
	}

	// 启用 embedding API（RAG 知识库需要 /v1/embeddings 接口）
	args = appendBoolArg(args, "--embedding", s.config.Embedding)
	// 嵌入池化类型：聊天模型默认 pooling=none 不兼容 OAI embedding API，需指定 mean
	args = appendStringArg(args, "--pooling", s.config.Pooling)

	// 新增参数
	args = appendBoolArg(args, "--swa-full", s.config.SwaFull)
	args = appendIntArg(args, "--ctx-checkpoints", s.config.CtxCheckpoints)
	args = appendIntArg(args, "--checkpoint-min-step", s.config.CheckpointMinStep)
	args = appendStringArg(args, "--tools", s.config.Tools)
	if !s.config.PrefillAssistant {
		args = append(args, "--no-prefill-assistant")
	}
	args = appendFloatArg(args, "--slot-prompt-similarity", s.config.SlotPromptSimilarity, "%.2f")
	args = appendBoolArg(args, "--skip-chat-parsing", s.config.SkipChatParsing)
	args = appendStringArg(args, "--api-prefix", s.config.APIPrefix)
	args = appendBoolArg(args, "--simple-io", s.config.SimpleIO)

	// Agent 模式：一键启用 CORS 代理 + 所有内置工具
	// 与 UIMcpProxy 互斥（Agent 已包含 MCP CORS 代理）
	if s.config.Agent {
		args = append(args, "--agent")
	} else if s.config.UIMcpProxy {
		args = append(args, "--ui-mcp-proxy")
	}

	// 后端采样（实验性，将采样逻辑移到 GPU 执行）
	args = appendBoolArg(args, "--backend-sampling", s.config.BackendSampling)

	// SSE ping 间隔（保持长连接活跃，防止代理/防火墙超时断连）
	args = appendIntArg(args, "--sse-ping-interval", s.config.SsePingInterval)

	// LoRA 适配器：启动时加载但默认不应用（scale=0），用户可通过设置界面热切换
	if s.config.LoraPaths != "" {
		for _, p := range strings.Split(s.config.LoraPaths, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				args = append(args, "--lora", p)
			}
		}
		args = append(args, "--lora-init-without-apply")
	}

	// KV 缓存持久化：启用后传递 --slot-save-path
	if s.config.SlotSaveEnabled && s.config.SlotSavePath != "" {
		args = append(args, "--slot-save-path", s.config.SlotSavePath)
	}

	// KV 缓存复用
	args = appendIntArg(args, "--cache-reuse", s.config.CacheReuse)

	// Draft 模型 GPU 配置（Eagle3 等场景）
	if s.config.SpecDraftNgl > 0 && !s.mtpFallbackDisabled {
		args = append(args, "--spec-draft-ngl", fmt.Sprintf("%d", s.config.SpecDraftNgl))
	}
	if s.config.SpecDraftDevice != "" && !s.mtpFallbackDisabled {
		args = append(args, "--spec-draft-device", s.config.SpecDraftDevice)
	}
	// Draft 模型推测解码参数
	if s.config.SpecDraftPSplit > 0 && !s.mtpFallbackDisabled {
		args = append(args, "--spec-draft-p-split", fmt.Sprintf("%.2f", s.config.SpecDraftPSplit))
	}
	if s.config.SpecDraftPMin > 0 && !s.mtpFallbackDisabled {
		args = append(args, "--spec-draft-p-min", fmt.Sprintf("%.2f", s.config.SpecDraftPMin))
	}
	if s.config.SpecDraftBackendSampling != nil && !s.mtpFallbackDisabled {
		if *s.config.SpecDraftBackendSampling {
			args = append(args, "--spec-draft-backend-sampling")
		} else {
			args = append(args, "--no-spec-draft-backend-sampling")
		}
	}
	// 多模态批处理
	args = appendIntArg(args, "--mtmd-batch-max-tokens", s.config.MtmdBatchMaxTokens)
	// 自适应采样（llama.cpp 新增）
	args = appendFloatArg(args, "--adaptive-target", s.config.AdaptiveTarget, "%.4f")
	args = appendFloatArg(args, "--adaptive-decay", s.config.AdaptiveDecay, "%.4f")
	// 模型标签
	args = appendStringArg(args, "--tags", s.config.Tags)
	// 媒体路径（多模态模型额外媒体文件目录）
	args = appendStringArg(args, "--media-path", s.config.MediaPath)
	// 离线模式（禁用所有网络请求）
	args = appendBoolArg(args, "--offline", s.config.Offline)
	// 模型重打包（启动时重新打包模型权重）
	args = appendBoolArg(args, "--repack", s.config.Repack)
	// Draft 模型线程配置
	if s.config.SpecDraftThreads > 0 && !s.mtpFallbackDisabled {
		args = append(args, "--spec-draft-threads", fmt.Sprintf("%d", s.config.SpecDraftThreads))
	}
	if s.config.SpecDraftThreadsBatch > 0 && !s.mtpFallbackDisabled {
		args = append(args, "--spec-draft-threads-batch", fmt.Sprintf("%d", s.config.SpecDraftThreadsBatch))
	}
	// 默认推测解码配置
	args = appendBoolArg(args, "--spec-default", s.config.SpecDefault)

	return args
}

func (s *Server) Start() error {
	s.mu.Lock()

	if s.status.Running && s.isAlive() {
		log.Info().Msg("stopping existing model server before starting new one...")
		s.mu.Unlock()
		if err := s.stopInternal(); err != nil {
			log.Error().Err(err).Msg("stop existing server before restart")
		}
		s.mu.Lock()
	}

	args := s.buildStartArgs()
	// 安全：API Key 通过环境变量传递，而非命令行参数
	// 基于 GO-CONFIG-001 安全实践：避免命令行参数被同权限进程通过 tasklist/WMI 读取
	// llama-server 支持 LLAMA_API_KEY 环境变量（见 llama.cpp/tools/server/README.md）
	if s.config.APIKey != "" {
		s.cmdEnv = append(s.cmdEnv, "LLAMA_API_KEY="+s.config.APIKey)
	}

	s.cmd = exec.Command(s.config.ServerPath, args...)
	// llama-server.exe 与 DLL 同目录（runtime/），直接用 exe 所在目录作为工作目录
	runtimeDir := filepath.Dir(s.config.ServerPath)
	s.cmd.Dir = runtimeDir

	s.stderrBuf = NewRingBuffer(500) // 增大缓冲区到 500 行，便于控制台查看历史
	if s.onLog != nil {
		s.stderrBuf.SetOnChange(s.onLog)
	}

	currentPath := os.Getenv("PATH")
	newPath := runtimeDir
	if currentPath != "" {
		newPath = runtimeDir + ";" + currentPath
	}
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if !strings.HasPrefix(e, "PATH=") {
			filtered = append(filtered, e)
		}
	}
	filtered = append(filtered, "PATH="+newPath)
	// 追加安全传递的环境变量（如 API Key）
	// 基于 GO-CONFIG-001 安全实践：通过环境变量而非命令行参数传递敏感信息
	filtered = append(filtered, s.cmdEnv...)

	s.lastStartTime = time.Now()

	// 尝试用 ConPTY 启动（获得原生终端输出：ANSI 颜色码、进度条）
	// 生活类比：ConPTY 就像一个"虚拟显示器"，让 llama-server 以为自己在真正的终端里运行
	pty, ptyErr := startWithConPTY(s.config.ServerPath, args, runtimeDir, filtered, 120, 40)
	if ptyErr != nil {
		log.Warn().Err(ptyErr).Msg("ConPTY unavailable, falling back to exec.Cmd")
		s.pty = nil
		// 回退到 exec.Cmd 方式（原有逻辑）
		s.cmd.Stdout = s.stderrBuf.TeeWriter(os.Stderr)
		s.cmd.Stderr = s.stderrBuf.TeeWriter(os.Stderr)
		s.cmd.Env = filtered
		s.cmd.SysProcAttr = &syscall.SysProcAttr{
			HideWindow:    true,
			CreationFlags: 0x08000000,
		}

		if err := s.cmd.Start(); err != nil {
			enhancedErr := enhanceStartError(err)
			s.status = ServerStatus{Running: false, Error: fmt.Sprintf("启动 llama-server 失败: %v", enhancedErr)}
			s.mu.Unlock()
			return fmt.Errorf("启动 llama-server 失败: %w", enhancedErr)
		}

		if s.job == nil {
			job, err := CreateJobObject()
			if err != nil {
				log.Error().Err(err).Msg("create job object failed (child process not bound)")
			} else {
				s.job = job
			}
		}

		if s.job != nil {
			if err := s.job.AssignProcess(s.cmd.Process.Pid); err != nil {
				log.Error().Err(err).Msg("assign process to job object failed (child process not bound)")
			} else {
				log.Info().Int("pid", s.cmd.Process.Pid).Msg("llama-server bound to job object (will auto-kill on parent exit)")
			}
		}

		s.replaceContext()
		s.status = ServerStatus{Running: true}

		go func() {
			err := s.cmd.Wait()
			s.mu.Lock()
			s.status = ServerStatus{Running: false}
			if err != nil && s.ctx.Err() == nil {
				errMsg := fmt.Sprintf("server exited with error: %v", err)
				if s.stderrBuf != nil {
					if tail := s.stderrBuf.String(); tail != "" {
						errMsg += "\n" + tail
					}
				}
				s.status.Error = errMsg
			}
			s.mu.Unlock()
		}()

		s.mu.Unlock()
		return nil
	}

	// ConPTY 启动成功
	s.pty = pty
	s.cmd = nil
	log.Info().Int("pid", pty.Pid()).Msg("llama-server started with ConPTY (native terminal output: ANSI colors + progress bars)")

	if s.job == nil {
		job, err := CreateJobObject()
		if err != nil {
			log.Error().Err(err).Msg("create job object failed (child process not bound)")
		} else {
			s.job = job
		}
	}

	if s.job != nil {
		if err := s.job.AssignProcess(pty.Pid()); err != nil {
			log.Error().Err(err).Msg("assign process to job object failed (child process not bound)")
		} else {
			log.Info().Int("pid", pty.Pid()).Msg("llama-server bound to job object (will auto-kill on parent exit)")
		}
	}

	s.replaceContext()
	s.status = ServerStatus{Running: true}

	// 启动 ConPTY 输出读取 goroutine（批量发送到前端 xterm.js）
	go s.readConPTYOutput()

	// 启动等待 goroutine
	go func() {
		exitCode, err := pty.Wait(s.ctx)
		s.mu.Lock()
		s.status = ServerStatus{Running: false}
		if err != nil && s.ctx.Err() == nil {
			errMsg := fmt.Sprintf("server exited with error: %v (exit code: %d)", err, exitCode)
			if s.stderrBuf != nil {
				if tail := s.stderrBuf.String(); tail != "" {
					errMsg += "\n" + tail
				}
			}
			// 检测 DLL 缺失导致的立即崩溃（进程刚启动就退出且 stderr 包含 DLL 相关信息）
			if exitCode != 0 && s.lastStartTime.Before(time.Now().Add(-10*time.Second)) {
				if enhanced := enhanceStartError(fmt.Errorf("%s", errMsg)); enhanced != nil {
					errMsg = enhanced.Error()
				}
			}
			s.status.Error = errMsg
		}
		s.mu.Unlock()
	}()

	s.mu.Unlock()
	return nil
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
				s.stderrBuf.Write(buf[:n])
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
			return fmt.Errorf("%s", errMsg)
		}
		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("server did not become ready within %v", timeout)
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

		// 先尝试正常终止进程树
		terminateCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T")
		terminateCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if err := terminateCmd.Run(); err != nil {
			log.Debug().Err(err).Int("pid", pid).Msg("terminate process (may already be dead)")
		}

		// 等待进程退出（带超时）
		waitDone := make(chan struct{})
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			pty.Wait(ctx)
			close(waitDone)
		}()

		timer := time.NewTimer(3 * time.Second)
		defer timer.Stop()

		select {
		case <-waitDone:
			pty.Close()
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
				log.Debug().Err(err).Int("pid", pid).Msg("force kill process (may already be dead)")
			}
			pty.Close()
			s.mu.Lock()
			s.status = ServerStatus{Running: false}
			if s.cancel != nil {
				s.cancel()
			}
			s.mu.Unlock()
			return fmt.Errorf("server did not terminate gracefully, force killed")
		}
	}

	// exec.Cmd 模式（原有逻辑）
	if s.cmd == nil || s.cmd.Process == nil {
		s.mu.Unlock()
		return nil
	}
	pid := s.cmd.Process.Pid
	s.mu.Unlock()

	terminateCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/T")
	terminateCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := terminateCmd.Run(); err != nil {
		log.Debug().Err(err).Int("pid", pid).Msg("terminate process (may already be dead)")
	}

	cmd := s.cmd
	waitDone := make(chan struct{})
	go func() {
		cmd.Wait()
		close(waitDone)
	}()

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	select {
	case <-waitDone:
		s.mu.Lock()
		s.status = ServerStatus{Running: false}
		if s.cancel != nil {
			s.cancel()
		}
		s.mu.Unlock()
		return nil
	case <-timer.C:
		killCmd := exec.Command("taskkill", "/PID", fmt.Sprintf("%d", pid), "/F", "/T")
		killCmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
		if err := killCmd.Run(); err != nil {
			log.Debug().Err(err).Int("pid", pid).Msg("force kill process (may already be dead)")
		}
		<-waitDone
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
	currentBackoff := 2 * time.Second
	const maxBackoff = 60 * time.Second
	const maxRestartAttempts = 10

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if !s.IsRunning() {
			if restartCount >= maxRestartAttempts {
				s.SetStatus(false, fmt.Sprintf("server crashed repeatedly (%d times), waiting %v before next attempt", restartCount, maxBackoff))
				if onStatusChange != nil {
					onStatusChange(s.Status())
				}

				select {
				case <-ctx.Done():
					return
				case <-time.After(maxBackoff):
				}

				restartCount = 0
				currentBackoff = 2 * time.Second
				continue
			}

			backoff := currentBackoff
			restartCount++
			currentBackoff = currentBackoff * 2
			if currentBackoff > maxBackoff {
				currentBackoff = maxBackoff
			}

			// 推测解码崩溃回退：若推测解码已启用且服务器崩溃，自动禁用推测解码
			// 窗口设为 120 秒以覆盖大模型加载时间（加载期间崩溃也视为推测解码问题）
			if !s.mtpFallbackDisabled && s.config.SpecType != "" {
				runDuration := time.Since(s.lastStartTime)
				if runDuration < 120*time.Second {
					s.mtpFallbackDisabled = true
					log.Warn().
						Dur("run_duration", runDuration).
						Str("spec_type", s.config.SpecType).
						Msg("[server] speculative decoding crash detected, restarting without speculative decoding")
					backoff = 1 * time.Second // 快速重启
				}
			}

			s.SetStatus(false, fmt.Sprintf("server crashed, restarting in %v (attempt %d/%d)", backoff, restartCount, maxRestartAttempts))
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
			currentBackoff = 2 * time.Second
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
		case <-time.After(1 * time.Second):
		}
	}
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
	if s.mtpFallbackDisabled {
		return ""
	}
	return s.config.SpecType
}
