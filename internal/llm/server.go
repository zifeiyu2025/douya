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
	"github.com/rs/zerolog/log"
)

const vramCheckInterval = 500 * time.Millisecond
const vramCheckTimeout = 15

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
	AdaptiveDecay  float64 // 自适应采样衰减率（0-1，默认 0.5）
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
	// LoRA 适配器路径（逗号分隔，启动时通过 --lora 加载，配合 --lora-init-without-apply 默认不应用）
	LoraPaths string
}

type Server struct {
	cmd                  *exec.Cmd
	config               *ServerConfig
	status               ServerStatus
	ctx                  context.Context
	cancel               context.CancelFunc
	mu                   sync.RWMutex
	job                  *JobObject
	stderrBuf            *RingBuffer
	mtpFallbackDisabled  bool
	lastStartTime        time.Time
	onLog                func(line string) // 日志行回调（用于实时推送到前端）
}

func NewServer(cfg *ServerConfig) *Server {
	return &Server{
		config: cfg,
		status: ServerStatus{Running: false},
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

	args := []string{
		"--models-dir", s.config.ModelsDir,
		"--port", fmt.Sprintf("%d", s.config.Port),
		"--jinja",
		"--fit", "on",
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
	if s.config.ModelsMax > 0 {
		args = append(args, "--models-max", fmt.Sprintf("%d", s.config.ModelsMax))
	}
	if s.config.SleepIdleSeconds > 0 {
		args = append(args, "--sleep-idle-seconds", fmt.Sprintf("%d", s.config.SleepIdleSeconds))
	}
	if s.config.GPULayers != "" {
		args = append(args, "--gpu-layers", s.config.GPULayers)
	}
	if s.config.FlashAttn != "" {
		args = append(args, "--flash-attn", s.config.FlashAttn)
	}
	if s.config.CacheTypeK != "" {
		args = append(args, "--cache-type-k", s.config.CacheTypeK)
	}
	if s.config.CacheTypeV != "" {
		args = append(args, "--cache-type-v", s.config.CacheTypeV)
	}
	if s.config.Mlock {
		args = append(args, "--mlock")
	}
	if s.config.Threads > 0 {
		args = append(args, "-t", fmt.Sprintf("%d", s.config.Threads))
	}
	if s.config.BatchSize > 0 {
		args = append(args, "-b", fmt.Sprintf("%d", s.config.BatchSize))
	}
	if s.config.UBatchSize > 0 {
		args = append(args, "-ub", fmt.Sprintf("%d", s.config.UBatchSize))
	}
	if s.config.ContextSize > 0 {
		args = append(args, "-c", fmt.Sprintf("%d", s.config.ContextSize))
	}
	if s.config.MmprojAuto {
		args = append(args, "--mmproj-auto")
	}
	if s.config.MmprojOffload {
		args = append(args, "--mmproj-offload")
	}
	if s.config.Reasoning != "" {
		args = append(args, "--reasoning", s.config.Reasoning)
	}
	if s.config.ReasoningBudget > 0 {
		args = append(args, "--reasoning-budget", fmt.Sprintf("%d", s.config.ReasoningBudget))
	}
	if s.config.ReasoningFormat != "" {
		args = append(args, "--reasoning-format", s.config.ReasoningFormat)
	}
	if s.config.ReasoningBudgetMessage != "" {
		args = append(args, "--reasoning-budget-message", s.config.ReasoningBudgetMessage)
	}
	if s.config.KVUnified {
		args = append(args, "--kv-unified")
	}
	if s.config.CacheIdleSlots {
		args = append(args, "--cache-idle-slots")
	}
	if s.config.CacheRAM > 0 {
		args = append(args, "--cache-ram", fmt.Sprintf("%d", s.config.CacheRAM))
	}
	if s.config.ImageMinTokens > 0 {
		args = append(args, "--image-min-tokens", fmt.Sprintf("%d", s.config.ImageMinTokens))
	}
	if s.config.ImageMaxTokens > 0 {
		args = append(args, "--image-max-tokens", fmt.Sprintf("%d", s.config.ImageMaxTokens))
	}
	if s.config.FitTarget > 0 {
		args = append(args, "--fit-target", fmt.Sprintf("%d", s.config.FitTarget))
	}
	if s.config.FitCtx > 0 {
		args = append(args, "--fit-ctx", fmt.Sprintf("%d", s.config.FitCtx))
	}
	if !s.config.Mmap {
		args = append(args, "--no-mmap")
	}
	if !s.config.KVOffload {
		args = append(args, "--no-kv-offload")
	}
	if s.config.ContextShift {
		args = append(args, "--context-shift")
	}
	if s.config.MinP > 0 {
		args = append(args, "--min-p", fmt.Sprintf("%.2f", s.config.MinP))
	}
	if s.config.DryMultiplier > 0 {
		args = append(args, "--dry-multiplier", fmt.Sprintf("%.2f", s.config.DryMultiplier))
		if s.config.DryBase > 0 {
			args = append(args, "--dry-base", fmt.Sprintf("%.2f", s.config.DryBase))
		}
		if s.config.DryAllowedLength > 0 {
			args = append(args, "--dry-allowed-length", fmt.Sprintf("%d", s.config.DryAllowedLength))
		}
	}
	if s.config.Device != "" {
		args = append(args, "--device", s.config.Device)
	}
	if s.config.Parallel > 0 {
		args = append(args, "--parallel", fmt.Sprintf("%d", s.config.Parallel))
	}
	args = append(args, "--timeout", "900")
	if s.config.APIKey != "" {
		args = append(args, "--api-key", s.config.APIKey)
	}
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
		args = append(args, "--spec-draft-type-k", s.config.CacheTypeKDraft)
	}
	if s.config.CacheTypeVDraft != "" && !s.mtpFallbackDisabled {
		args = append(args, "--spec-draft-type-v", s.config.CacheTypeVDraft)
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
	if s.config.Embedding {
		args = append(args, "--embedding")
	}
	// 嵌入池化类型：聊天模型默认 pooling=none 不兼容 OAI embedding API，需指定 mean
	if s.config.Pooling != "" {
		args = append(args, "--pooling", s.config.Pooling)
	}

	// 新增参数
	if s.config.SwaFull {
		args = append(args, "--swa-full")
	}
	if s.config.CtxCheckpoints > 0 {
		args = append(args, "--ctx-checkpoints", fmt.Sprintf("%d", s.config.CtxCheckpoints))
	}
	if s.config.CheckpointMinStep > 0 {
		args = append(args, "--checkpoint-min-step", fmt.Sprintf("%d", s.config.CheckpointMinStep))
	}
	if s.config.Tools != "" {
		args = append(args, "--tools", s.config.Tools)
	}
	if !s.config.PrefillAssistant {
		args = append(args, "--no-prefill-assistant")
	}
	if s.config.SlotPromptSimilarity > 0 {
		args = append(args, "--slot-prompt-similarity", fmt.Sprintf("%.2f", s.config.SlotPromptSimilarity))
	}
	if s.config.SkipChatParsing {
		args = append(args, "--skip-chat-parsing")
	}
	if s.config.APIPrefix != "" {
		args = append(args, "--api-prefix", s.config.APIPrefix)
	}
	if s.config.SimpleIO {
		args = append(args, "--simple-io")
	}

	// Agent 模式：一键启用 CORS 代理 + 所有内置工具
	// 与 UIMcpProxy 互斥（Agent 已包含 MCP CORS 代理）
	if s.config.Agent {
		args = append(args, "--agent")
	} else if s.config.UIMcpProxy {
		args = append(args, "--ui-mcp-proxy")
	}

	// 后端采样（实验性，将采样逻辑移到 GPU 执行）
	if s.config.BackendSampling {
		args = append(args, "--backend-sampling")
	}

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
	if s.config.CacheReuse > 0 {
		args = append(args, "--cache-reuse", fmt.Sprintf("%d", s.config.CacheReuse))
	}

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
	if s.config.MtmdBatchMaxTokens > 0 {
		args = append(args, "--mtmd-batch-max-tokens", fmt.Sprintf("%d", s.config.MtmdBatchMaxTokens))
	}
	// 自适应采样（llama.cpp 新增）
	if s.config.AdaptiveTarget > 0 {
		args = append(args, "--adaptive-target", fmt.Sprintf("%.4f", s.config.AdaptiveTarget))
	}
	if s.config.AdaptiveDecay > 0 {
		args = append(args, "--adaptive-decay", fmt.Sprintf("%.4f", s.config.AdaptiveDecay))
	}
	// 模型标签
	if s.config.Tags != "" {
		args = append(args, "--tags", s.config.Tags)
	}
	// 媒体路径（多模态模型额外媒体文件目录）
	if s.config.MediaPath != "" {
		args = append(args, "--media-path", s.config.MediaPath)
	}
	// 离线模式（禁用所有网络请求）
	if s.config.Offline {
		args = append(args, "--offline")
	}
	// 模型重打包（启动时重新打包模型权重）
	if s.config.Repack {
		args = append(args, "--repack")
	}

	s.cmd = exec.Command(s.config.ServerPath, args...)
	// llama-server.exe 与 DLL 同目录（runtime/），直接用 exe 所在目录作为工作目录
	runtimeDir := filepath.Dir(s.config.ServerPath)
	s.cmd.Dir = runtimeDir

	s.stderrBuf = NewRingBuffer(500) // 增大缓冲区到 500 行，便于控制台查看历史
	if s.onLog != nil {
		s.stderrBuf.SetOnChange(s.onLog)
	}
	s.cmd.Stdout = s.stderrBuf.TeeWriter(os.Stderr)
	s.cmd.Stderr = s.stderrBuf.TeeWriter(os.Stderr)

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
	s.cmd.Env = filtered

	s.cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}

	s.lastStartTime = time.Now()

	if err := s.cmd.Start(); err != nil {
		s.status = ServerStatus{Running: false, Error: fmt.Sprintf("failed to start server: %v", err)}
		s.mu.Unlock()
		return fmt.Errorf("failed to start server: %w", err)
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

	// 清理旧的 cancel 函数并创建新的 context，避免重复调用 Start() 时旧 cancel 被覆盖导致资源泄漏
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
	client := &http.Client{Timeout: 2 * time.Second}
	url := fmt.Sprintf("http://127.0.0.1:%d/health", s.config.Port)

	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
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
	client := &http.Client{Timeout: 2 * time.Second}

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
