// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/system"

	zlog "github.com/rs/zerolog/log"
)

// derivedServerParams 封装 buildServerConfig 中从用户配置和智能参数派生出的中间值。
// 这些派生值需要先计算再赋值给 ServerConfig，独立成结构体便于在子函数间传递。
//
// 生活类比：就像菜谱中的"预处理食材"清单——把需要切洗腌的食材先准备好放一个篮子里，
// 后面炒菜时直接用，不用临时再切。
type derivedServerParams struct {
	GPULayers       string // GPU 层数（"auto" 或具体数字）
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
// 这是 buildServerConfig 的主调度器，实际逻辑拆分到以下子函数：
//   - resolveDerivedServerParams: 解析用户配置与智能参数的派生值
//   - buildServerConfigFromFields: 构建 ServerConfig 结构体（字段赋值）
//   - applySmartParamOverrides: 用户配置覆盖智能参数
//   - autoEnableEagle3: Eagle3 自动启用
//   - autoRecommendReasoning: 推理模式自动推荐
//   - loadServerAPIKey: 从数据库加载 API Key
//
// 生活类比：就像餐厅出餐流程——主厨（buildServerConfig）只负责按流程分派任务，
// 备料（resolveDerived）、摆盘（buildFromFields）、调味（applyOverrides）、
// 装饰（autoEnable/autoRecommend）、上桌前检查（loadAPIKey）各有专人负责。
func (a *App) buildServerConfig() *llm.ServerConfig {
	cfg := a.getConfig()

	// 解析后端类型：优先用 startup 中缓存的解析结果（已 EnsureBackendInstalled），
	// 未缓存时（如热重载场景）重新根据硬件和配置解析。
	// 生活类比：用户选了"自动挡"，就根据车库里的车（硬件）来选合适的发动机。
	resolvedBackend := a.resolvedBackend
	if resolvedBackend == "" {
		// 热重载场景：runtime 已就绪，无需运行时预校验，直接按硬件推断
		resolvedBackend = llm.ResolveBackendType(a.hwInfo, cfg.BackendType)
	}

	// 解析 ServerPath：优先用 startup 中缓存的路径（已 EnsureBackendInstalled），
	// 未缓存时回退到配置中的 LlamaServerPath（兼容旧版布局）。
	absServerPath := a.resolvedServerPath
	if absServerPath == "" {
		absServerPath = resolvePath(cfg.LlamaServerPath)
	}

	modelsDir := filepath.Join(appDir(), "models")

	// 传入已解析的后端类型（resolvedBackend），让 SmartParams 根据后端调整参数
	// 生活类比：告诉智能参数模块"我们这趟用的是电/油/柴"，让它据此调发动机参数
	sp := system.CalculateSmartParams(a.hwInfo, resolvePath(cfg.ModelPath), string(resolvedBackend), cfg.PerformanceMode)
	zlog.Info().
		Str("models_dir", modelsDir).
		Str("backend", resolvedBackend.String()).
		Int("gpu_layers", sp.GPULayers).
		Int("threads", sp.Threads).
		Bool("flash", sp.FlashAttn).
		Str("cache_k", sp.CacheTypeK).
		Str("cache_v", sp.CacheTypeV).
		Bool("mlock", sp.Mlock).
		Bool("mmproj_offload", sp.MmprojOffload).
		Msg("[smart-params] params")

	presetPath := resolvePresetPath()
	derived := resolveDerivedServerParams(cfg, *sp)
	mediaPath := a.resolveMediaPath(cfg.MediaPath)
	serverCfg := buildServerConfigFromFields(cfg, *sp, derived, modelsDir, absServerPath, presetPath, mediaPath)
	// 设置当前使用的后端类型，供后续逻辑（如参数调优、日志记录）使用
	serverCfg.BackendType = resolvedBackend

	applySmartParamOverrides(serverCfg, cfg, *sp)
	autoEnableEagle3(serverCfg, cfg, *sp)
	autoRecommendReasoning(serverCfg, *sp)
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

// resolveDerivedServerParams 解析用户配置与智能参数的派生值。
// 用户显式设置优先，未设置时用智能参数兜底。
//
// 生活类比：就像汽车驾驶模式——用户手动调节的参数（座椅、后视镜）听用户的，
// 没调过的用自动模式（根据驾驶员身材自动调整）。
func resolveDerivedServerParams(cfg *config.Config, sp system.SmartParams) *derivedServerParams {
	d := &derivedServerParams{}

	// GPU层数：用户设置优先，否则用智能参数
	d.GPULayers = "auto"
	if cfg.GPULayers > 0 {
		d.GPULayers = fmt.Sprintf("%d", cfg.GPULayers)
	} else if sp.GPULayers > 0 {
		d.GPULayers = fmt.Sprintf("%d", sp.GPULayers)
	}

	// Flash Attention：用户设置优先，支持 on/off/auto 三值
	d.FlashAttn = ""
	if sp.FlashAttn {
		d.FlashAttn = "on"
	}
	if cfg.FlashAttn != nil {
		if *cfg.FlashAttn {
			d.FlashAttn = "on"
		} else {
			d.FlashAttn = "off"
		}
	}

	// Mlock：用户设置优先
	d.Mlock = sp.Mlock
	if cfg.Mlock != nil {
		d.Mlock = *cfg.Mlock
	}

	// MmprojOffload：用户设置优先（config.json 中 mmproj_offload=true 则启用）
	// smartparams 根据硬件判断是否有 GPU（有 GPU 则推荐开启）
	// 日志证据：mmproj_offload 不是 Vulkan 栈溢出根因，gpu_layers 过大才是
	// 用户可通过 config.json 的 mmproj_offload=false 来关闭
	d.MmprojOffload = sp.MmprojOffload
	if cfg.MmprojOffload {
		d.MmprojOffload = true
	}

	// 推测解码参数：用户设置优先，未配置时用智能参数自动启用
	d.SpecType = cfg.SpecType
	if d.SpecType == "" {
		d.SpecType = sp.SpecType
	}
	d.SpecDraftNMax = resolveIntDerived(cfg.SpecDraftNMax, sp.SpecDraftNMax)
	d.SpecDraftNMin = resolveIntDerived(cfg.SpecDraftNMin, sp.SpecDraftNMin)
	d.CacheTypeKDraft = resolveStringDerived(cfg.CacheTypeKDraft, sp.CacheTypeKDraft)
	d.CacheTypeVDraft = resolveStringDerived(cfg.CacheTypeVDraft, sp.CacheTypeVDraft)
	d.NgramModNMin = resolveIntDerived(cfg.SpecNgramModNMin, sp.NgramModNMin)
	d.NgramModNMax = resolveIntDerived(cfg.SpecNgramModNMax, sp.NgramModNMax)
	d.NgramModNMatch = resolveIntDerived(cfg.SpecNgramModNMatch, sp.NgramModNMatch)

	// 线程数：用户设置优先
	d.Threads = resolveIntDerived(cfg.Threads, sp.Threads)

	// Batch Size：用户设置优先
	d.BatchSize = resolveIntDerived(cfg.BatchSize, sp.BatchSize)
	d.UBatchSize = sp.UBatchSize
	if cfg.BatchSize > 0 {
		d.UBatchSize = d.BatchSize / 2
	}

	// 上下文长度：用户设置优先，否则用智能参数
	d.ContextSize = resolveIntDerived(cfg.ContextSize, sp.ContextSize)

	// SleepIdleSeconds：尊重用户显式设置
	// -1 表示禁用空闲休眠（与 llama.cpp 默认值对齐），0 视为未设置也禁用
	d.SleepIdle = cfg.SleepIdleSeconds

	// ModelsMax：默认 1
	d.ModelsMax = cfg.ModelsMax
	if d.ModelsMax <= 0 {
		d.ModelsMax = 1
	}

	// reasoning_format 不再硬编码设置：
	// llama-server 默认值 COMMON_REASONING_FORMAT_DEEPSEEK 已能正确处理所有模型的思考内容分离
	d.ReasoningFormat = cfg.ReasoningFormat

	return d
}

// resolveIntDerived 解析整型派生值：用户值 > 0 时用用户值，否则用智能参数值。
func resolveIntDerived(userVal, smartVal int) int {
	if userVal > 0 {
		return userVal
	}
	return smartVal
}

// resolveStringDerived 解析字符串派生值：用户值非空时用用户值，否则用智能参数值。
func resolveStringDerived(userVal, smartVal string) string {
	if userVal != "" {
		return userVal
	}
	return smartVal
}

// buildServerConfigFromFields 根据配置和派生值构建 ServerConfig 结构体。
// 这是纯字段赋值函数，不含逻辑分支。
func buildServerConfigFromFields(
	cfg *config.Config,
	sp system.SmartParams,
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
		CacheTypeK:             sp.CacheTypeK,
		CacheTypeV:             sp.CacheTypeV,
		Mlock:                  d.Mlock,
		MmprojAuto:             cfg.MmprojAuto,
		MmprojOffload:          d.MmprojOffload,
		KVUnified:              cfg.KVUnified,
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
		KeepSize:                 512,
		MinP:                     cfg.MinP,
		DryMultiplier:            cfg.DryMultiplier,
		DryBase:                  cfg.DryBase,
		DryAllowedLength:         cfg.DryAllowedLength,
		DrySequenceBreaker:       cfg.DrySequenceBreaker,
		DryPenaltyLastN:          cfg.DryPenaltyLastN,
		GrpAttnN:                 cfg.GrpAttnN,
		GrpAttnW:                 cfg.GrpAttnW,
		Jinja:                    cfg.Jinja,
		CachePrompt:              cfg.CachePrompt,
		Metrics:                  cfg.Metrics,
		Verbose:                  cfg.Verbose,
		SpecDraftThreads:         cfg.SpecDraftThreads,
		SpecDraftThreadsBatch:    cfg.SpecDraftThreadsBatch,
		SpecDefault:              cfg.SpecDefault,
		Device:                   cfg.Device,
		Parallel:                 cfg.Parallel,
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
		UIMcpProxy:               cfg.UIMcpProxy,
		BackendSampling:          cfg.BackendSampling,
		SsePingInterval:          cfg.SsePingInterval,
		LoraPaths:                cfg.LoraPaths,
		RerankerModelPath:        cfg.RerankerModelPath,
		DirectIO:                 cfg.DirectIO,
		CPUMoe:                   cfg.CPUMoe,
		NCpuMoe:                  cfg.NCpuMoe,
		OpOffload:                cfg.OpOffload,
	}
}

// applySmartParamOverrides 用用户显式配置覆盖智能参数推荐值。
// 当用户在 config.json 中显式设置了某个参数时，优先使用用户值。
func applySmartParamOverrides(serverCfg *llm.ServerConfig, cfg *config.Config, sp system.SmartParams) {
	if cfg.CacheTypeK != "" {
		serverCfg.CacheTypeK = cfg.CacheTypeK
	}
	if cfg.CacheTypeV != "" {
		serverCfg.CacheTypeV = cfg.CacheTypeV
	}
	if cfg.CacheTypeKDraft != "" {
		serverCfg.CacheTypeKDraft = cfg.CacheTypeKDraft
	}
	if cfg.CacheTypeVDraft != "" {
		serverCfg.CacheTypeVDraft = cfg.CacheTypeVDraft
	}
	// 用户未设置 SpecType 但智能参数推荐了时，用智能参数的完整推测解码配置
	if cfg.SpecType == "" && sp.SpecType != "" {
		serverCfg.SpecType = sp.SpecType
		serverCfg.SpecDraftNMax = sp.SpecDraftNMax
		serverCfg.SpecDraftNMin = sp.SpecDraftNMin
		if serverCfg.CacheTypeKDraft == "" {
			serverCfg.CacheTypeKDraft = sp.CacheTypeKDraft
		}
		if serverCfg.CacheTypeVDraft == "" {
			serverCfg.CacheTypeVDraft = sp.CacheTypeVDraft
		}
		if serverCfg.SpecNgramModNMin == 0 && sp.NgramModNMin > 0 {
			serverCfg.SpecNgramModNMin = sp.NgramModNMin
		}
		if serverCfg.SpecNgramModNMax == 0 && sp.NgramModNMax > 0 {
			serverCfg.SpecNgramModNMax = sp.NgramModNMax
		}
		if serverCfg.SpecNgramModNMatch == 0 && sp.NgramModNMatch > 0 {
			serverCfg.SpecNgramModNMatch = sp.NgramModNMatch
		}
	}
}

// autoEnableEagle3 在模型支持 Eagle3 且用户配置了 draft 模型但未显式设置 SpecType 时，
// 自动启用 draft-eagle3 推测解码。
//
// 生活类比：就像检测到你插了耳机就自动切换音频输出到耳机一样。
func autoEnableEagle3(serverCfg *llm.ServerConfig, cfg *config.Config, sp system.SmartParams) {
	if serverCfg.SpecType == "" && sp.SupportsEagle3 && cfg.SpecDraftModel != "" {
		serverCfg.SpecType = "draft-eagle3"
		zlog.Info().Str("draft_model", cfg.SpecDraftModel).Msg("[smart-params] Eagle3 supported and draft model configured, auto-enabling draft-eagle3")
	}
}

// autoRecommendReasoning 在用户未设置推理模式时，使用智能参数推荐值。
//
// 生活类比：就像你没手动调空调温度时，汽车自动用舒适温度一样。
func autoRecommendReasoning(serverCfg *llm.ServerConfig, sp system.SmartParams) {
	if serverCfg.Reasoning == "" && sp.ReasoningMode != "" {
		serverCfg.Reasoning = sp.ReasoningMode
	}
	if serverCfg.ReasoningBudget == 0 && sp.ReasoningBudget != 0 {
		serverCfg.ReasoningBudget = sp.ReasoningBudget
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
