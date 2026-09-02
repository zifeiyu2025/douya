// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"douya/internal/config"
	"douya/internal/llm"
)

// derivedServerParams 封装 buildServerConfig 中从用户配置派生出的中间值。
// 这些派生值需要先计算再赋值给 ServerConfig，独立成结构体便于在子函数间传递。
//
// 生活类比：就像菜谱中的"预处理食材"清单——把需要切洗腌的食材先准备好放一个篮子里，
// 后面炒菜时直接用，不用临时再切。
type derivedServerParams struct {
	GPULayers       string // GPU 层数（用户显式设置的具体数字；空=交给 llama.cpp 默认 -1=自动卸载）
	FlashAttn       string // Flash Attention 模式（"on"/"off"/""）
	Mlock           bool   // 内存锁定
	MmprojOffload   bool   // mmproj GPU 卸载
	SpecType        string // 推测解码类型
	SpecDraftNMax   int    // 草稿 token 最大数
	SpecDraftNMin   int    // 草稿 token 最小数
	CacheTypeKDraft string // Draft K 缓存类型
	CacheTypeVDraft string // Draft V 缓存类型
	NgramModNMin    int    // ngram-mod N 最小值
	NgramModNMax    int    // ngram-mod N 最大值
	NgramModNMatch  int    // ngram-mod 匹配数
	Threads         int    // 线程数
	BatchSize       int    // 批处理大小
	UBatchSize      int    // 微批处理大小
	ContextSize     int    // 上下文长度
	SleepIdle       int    // 空闲休眠秒数
	ModelsMax       int    // 最大并行模型数
	ReasoningFormat string // 推理输出格式
}

// buildServerConfig 从应用配置构建 llama-server 所需的 ServerConfig。
//
// 模型加载参数完全依赖 llama.cpp 原生的自动参数识别能力
// （--gpu-layers 默认 -1 = 自动卸载到 VRAM、flash-attn 默认开启、上下文默认 4096 等），
// 不再做任何自定义硬件/模型分析与参数计算。
// 用户通过配置显式设置的值作为覆盖项；未设置的字段保持空/零值，
// 由 server_args.go 的 append*Arg 跳过，最终交给 llama.cpp 使用其内置默认。
func (a *App) buildServerConfig() *llm.ServerConfig {
	cfg := a.getConfig()

	// 解析后端类型：优先用 startup 中缓存的解析结果（已 EnsureBackendInstalled），
	// 未缓存时（如热重载场景）重新根据硬件和配置解析。
	resolvedBackend, resolvedServerPath := a.resolvedBackendSnapshot()
	if resolvedBackend == "" {
		// 热重载场景：runtime 已就绪，无需运行时预校验，直接按硬件推断
		resolvedBackend = llm.ResolveBackendType(a.hwInfo, cfg.BackendType)
	}

	// 解析 ServerPath：优先用 startup 中缓存的路径（已 EnsureBackendInstalled），
	// 未缓存时回退到配置中的 LlamaServerPath（兼容旧版布局）。
	absServerPath := resolvedServerPath
	if absServerPath == "" {
		absServerPath = resolvePath(cfg.LlamaServerPath)
	}

	modelsDir := filepath.Join(appDir(), "models")

	// 参数完全依赖 llama.cpp 原生自动识别：仅以用户显式配置为准，未设置则留空/零值。
	derived := resolveDerivedServerParams(cfg)
	mediaPath := a.resolveMediaPath(cfg.MediaPath)
	presetPath := resolvePresetPath()
	serverCfg := buildServerConfigFromFields(cfg, derived, modelsDir, absServerPath, presetPath, mediaPath)
	// 设置当前使用的后端类型，供后续逻辑（如参数调优、日志记录）使用
	serverCfg.BackendType = resolvedBackend

	a.loadServerAPIKey(serverCfg)

	// 后端安全限制：在所有用户覆盖之后应用，确保后端硬限制不可被绕过
	// 已下沉为 llm.ServerConfig 的方法，app 层只做调用
	serverCfg.ApplyBackendSafetyLimits(resolvedBackend)

	return serverCfg
}

// resolvePresetPath 解析 router-preset.ini 路径，文件不存在则返回空字符串。
func resolvePresetPath() string {
	presetPath := filepath.Join(appDir(), "router-preset.ini")
	if _, err := os.Stat(presetPath); err != nil {
		return ""
	}
	return presetPath
}

// resolveDerivedServerParams 从用户配置派生 ServerConfig 的中间值。
// 仅以用户显式配置为准；未设置的字段保持空/零值，表示交给 llama.cpp 原生自动识别
// （--gpu-layers 默认 -1 自动卸载、flash-attn 默认开启、上下文默认 4096 等），
// 不再做任何自定义硬件/模型分析，也不自动计算参数。
func resolveDerivedServerParams(cfg *config.Config) *derivedServerParams {
	d := &derivedServerParams{}

	// GPU 层数：用户显式设置则用，未设置则留空（server_args.go 跳过 -> llama.cpp 默认 -1=auto）
	if cfg.GPULayers > 0 {
		d.GPULayers = fmt.Sprintf("%d", cfg.GPULayers)
	}

	// Flash Attention：用户显式设置则用 on/off，未设置则留空（llama.cpp 默认开启）
	if cfg.FlashAttn != nil {
		if *cfg.FlashAttn {
			d.FlashAttn = "on"
		} else {
			d.FlashAttn = "off"
		}
	}

	// Mlock：用户显式设置则用，否则 false（影响 --load-mode）
	if cfg.Mlock != nil {
		d.Mlock = *cfg.Mlock
	}

	// MmprojOffload：用户显式设置则用，否则 false
	if cfg.MmprojOffload != nil {
		d.MmprojOffload = *cfg.MmprojOffload
	}

	// 推测解码参数：仅用户显式配置生效（不再自动推荐）
	d.SpecType = cfg.SpecType
	d.SpecDraftNMax = cfg.SpecDraftNMax
	d.SpecDraftNMin = cfg.SpecDraftNMin
	d.CacheTypeKDraft = cfg.CacheTypeKDraft
	d.CacheTypeVDraft = cfg.CacheTypeVDraft
	d.NgramModNMin = cfg.SpecNgramModNMin
	d.NgramModNMax = cfg.SpecNgramModNMax
	d.NgramModNMatch = cfg.SpecNgramModNMatch

	// 线程数 / 批大小：用户显式设置 > 0 时用，否则留 0（llama.cpp 自动）
	d.Threads = cfg.Threads
	d.BatchSize = cfg.BatchSize
	if cfg.BatchSize > 0 {
		d.UBatchSize = cfg.BatchSize / 2
	}

	// 上下文长度：用户显式设置 > 0 时用，否则留 0（llama.cpp 默认 4096）
	d.ContextSize = cfg.ContextSize

	// SleepIdleSeconds / ModelsMax：尊重用户/默认
	d.SleepIdle = cfg.SleepIdleSeconds
	d.ModelsMax = cfg.ModelsMax
	if d.ModelsMax <= 0 {
		d.ModelsMax = 1 // 至少支持 1 个模型
	}

	// reasoning_format：不再硬编码，遵循 llama-server 默认
	d.ReasoningFormat = cfg.ReasoningFormat

	return d
}

// buildServerConfigFromFields 根据配置和派生值构建 ServerConfig 结构体。
// 这是纯字段赋值函数，不含逻辑分支。
func buildServerConfigFromFields(
	cfg *config.Config,
	d *derivedServerParams,
	modelsDir, absServerPath, presetPath, mediaPath string,
) *llm.ServerConfig {
	return &llm.ServerConfig{
		ModelsDir:              modelsDir,
		ServerPath:             absServerPath,
		Port:                   cfg.Port,
		GPULayers:              d.GPULayers,
		Threads:                d.Threads,
		FlashAttn:              d.FlashAttn,
		CacheTypeK:             cfg.CacheTypeK,
		CacheTypeV:             cfg.CacheTypeV,
		Mlock:                  d.Mlock,
		MmprojAuto:             cfg.MmprojAuto,
		MmprojOffload:          d.MmprojOffload,
		MmprojDevice:           cfg.MmprojDevice,
		KVUnified:              cfg.KVUnified,
		KVUnifiedPerSlot:       cfg.KVUnifiedPerSlot,
		CacheIdleSlots:         cfg.CacheIdleSlots,
		CacheRAM:               cfg.CacheRAM,
		ImageMinTokens:         cfg.ImageMinTokens,
		ImageMaxTokens:         cfg.ImageMaxTokens,
		FitTarget:              cfg.FitTarget,
		FitCtx:                 cfg.FitCtx,
		Reasoning:              cfg.Reasoning,
		ReasoningBudget:        cfg.ReasoningBudget,
		ReasoningFormat:        d.ReasoningFormat,
		ReasoningBudgetMessage: cfg.ReasoningBudgetMessage,
		ReasoningPreserve:      cfg.ReasoningPreserve,
		APIBase:                cfg.APIBase,
		AppDir:                 appDir(),
		ModelsPreset:           presetPath,
		ModelsMax:              d.ModelsMax,
		SleepIdleSeconds:       d.SleepIdle,
		Mmap:                   cfg.Mmap,
		KVOffload:              cfg.KVOffload,
		ContextShift:           cfg.ContextShift,
		// P0-B3: 启用 context-shift 时保护 system prompt 不被移位。
		// 512 是保守值，足够覆盖豆芽的 system prompt（约 200-400 token）。
		KeepSize:              512,
		MinP:                  cfg.MinP,
		DryMultiplier:         cfg.DryMultiplier,
		DryBase:               cfg.DryBase,
		DryAllowedLength:      cfg.DryAllowedLength,
		DrySequenceBreaker:    cfg.DrySequenceBreaker,
		DryPenaltyLastN:       cfg.DryPenaltyLastN,
		GrpAttnN:              cfg.GrpAttnN,
		GrpAttnW:              cfg.GrpAttnW,
		Jinja:                 cfg.Jinja,
		CachePrompt:           cfg.CachePrompt,
		Metrics:               cfg.Metrics,
		Verbose:               cfg.Verbose,
		SpecDraftThreads:      cfg.SpecDraftThreads,
		SpecDraftThreadsBatch: cfg.SpecDraftThreadsBatch,
		SpecDefault:           cfg.SpecDefault,
		Device:                cfg.Device,
		Parallel:              cfg.Parallel,
		// 多 GPU 参数接线：config.go 已有字段与校验（isValidSplitMode/validateTensorSplit），
		// 此处映射到 ServerConfig，由 server_args.go 生成 --split-mode/--tensor-split/--main-gpu
		SplitMode:                cfg.SplitMode,
		TensorSplit:              cfg.TensorSplit,
		MainGPU:                  cfg.MainGPU,
		SpecType:                 d.SpecType,
		SpecDraftNMax:            d.SpecDraftNMax,
		SpecDraftNMin:            d.SpecDraftNMin,
		CacheTypeKDraft:          d.CacheTypeKDraft,
		CacheTypeVDraft:          d.CacheTypeVDraft,
		SpecNgramModNMin:         d.NgramModNMin,
		SpecNgramModNMax:         d.NgramModNMax,
		SpecNgramModNMatch:       d.NgramModNMatch,
		SpecNgramSimpleSizeN:     cfg.SpecNgramSimpleSizeN,
		SpecNgramSimpleSizeM:     cfg.SpecNgramSimpleSizeM,
		SpecNgramSimpleMinHits:   cfg.SpecNgramSimpleMinHits,
		SpecNgramMapKSizeN:       cfg.SpecNgramMapKSizeN,
		SpecNgramMapKSizeM:       cfg.SpecNgramMapKSizeM,
		SpecNgramMapKMinHits:     cfg.SpecNgramMapKMinHits,
		SpecNgramMapK4VSizeN:     cfg.SpecNgramMapK4VSizeN,
		SpecNgramMapK4VSizeM:     cfg.SpecNgramMapK4VSizeM,
		SpecNgramMapK4VMinHits:   cfg.SpecNgramMapK4VMinHits,
		LookupCacheStatic:        cfg.LookupCacheStatic,
		LookupCacheDynamic:       cfg.LookupCacheDynamic,
		SpecDraftModel:           cfg.SpecDraftModel,
		Embedding:                true,   // 启用 embedding API（RAG 知识库需要）
		Pooling:                  "mean", // 聊天模型 pooling=none 不兼容 OAI embedding API
		ExposeServer:             cfg.ExposeServer,
		EnableWebUI:              cfg.EnableWebUI,
		ServerAPIKeyEnabled:      cfg.ServerAPIKeyEnabled,
		SwaFull:                  cfg.SwaFull,
		CtxCheckpoints:           cfg.CtxCheckpoints,
		CheckpointMinStep:        cfg.CheckpointMinStep,
		Tools:                    cfg.Tools,
		EnableBuiltinTools:       cfg.EnableBuiltinTools,
		ToolsRuntime:             cfg.ToolsRuntime,
		PrefillAssistant:         cfg.PrefillAssistant,
		SlotPromptSimilarity:     cfg.SlotPromptSimilarity,
		SkipChatParsing:          cfg.SkipChatParsing,
		APIPrefix:                cfg.APIPrefix,
		SimpleIO:                 cfg.SimpleIO,
		BatchSize:                d.BatchSize,
		UBatchSize:               d.UBatchSize,
		ThreadsHTTP:              cfg.ThreadsHTTP,
		ContextSize:              d.ContextSize,
		SlotSavePath:             cfg.SlotSavePath,
		SlotSaveEnabled:          cfg.SlotSaveEnabled,
		CacheReuse:               cfg.CacheReuse,
		SpecDraftNgl:             cfg.SpecDraftNgl,
		SpecDraftDevice:          cfg.SpecDraftDevice,
		SpecDraftPSplit:          cfg.SpecDraftPSplit,
		SpecDraftPMin:            cfg.SpecDraftPMin,
		SpecDraftBackendSampling: cfg.SpecDraftBackendSampling,
		MtmdBatchMaxTokens:       cfg.MtmdBatchMaxTokens,
		AdaptiveTarget:           cfg.AdaptiveTarget,
		AdaptiveDecay:            cfg.AdaptiveDecay,
		Tags:                     cfg.Tags,
		MediaPath:                mediaPath,
		Offline:                  cfg.Offline,
		Repack:                   cfg.Repack,
		Agent:                    cfg.Agent,
		AgentCwd:                 cfg.AgentCwd,
		BackendSampling:          cfg.BackendSampling,
		SsePingInterval:          cfg.SsePingInterval,
		LoraPaths:                cfg.LoraPaths,
		ChatTemplateFile:         cfg.ChatTemplateFile,
		RerankerModelPath:        cfg.RerankerModelPath,
		DirectIO:                 cfg.DirectIO,
		CPUMoe:                   cfg.CPUMoe,
		NCpuMoe:                  cfg.NCpuMoe,
		NCpuFfn:                  cfg.NCpuFfn,
		OpOffload:                cfg.OpOffload,
	}
}

// loadServerAPIKey 从数据库加载加密存储的服务器 API Key。
// 优先使用加密密钥解密，回退到明文存储（兼容旧版本）。
func (a *App) loadServerAPIKey(serverCfg *llm.ServerConfig) {
	if a.db == nil {
		return
	}
	if key, err := a.service.GetEncryptedSetting("server_api_key"); err == nil && key != "" {
		serverCfg.APIKey = key
	}
}
