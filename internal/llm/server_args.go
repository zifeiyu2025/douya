// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog/log"
)

// buildStartArgs 根据配置组装 llama-server 启动命令行参数。
// 这是纯函数：只读 s.config 和 s.mtpFallbackDisabled，不修改任何状态，无副作用。
// 注意：API Key 通过环境变量传递（涉及 s.cmdEnv 状态修改），相关逻辑保留在 Start() 中。
//
// 拆分说明：原函数 347 行，按业务类别拆成 12 个子函数，主函数仅做调度。
// 生活类比：就像餐厅出菜单，主厨（buildStartArgs）只负责按类别分派任务给各个厨师
// （baseArgs/appendModelLoadArgs/...），每个厨师只做自己擅长的那道菜。
func (s *Server) buildStartArgs() []string {
	args := s.baseArgs()
	args = s.appendModelLoadArgs(args)
	args = s.appendRuntimeArgs(args)
	args = s.appendReasoningArgs(args)
	args = s.appendKVCacheArgs(args)
	args = s.appendSamplingArgs(args)
	args = s.appendSwitchArgs(args)
	args = s.appendServiceArgs(args)
	args = s.appendSpeculativeArgs(args)
	args = s.appendNewFeatureArgs(args)
	args = s.appendLoraArgs(args)
	args = s.appendAdvancedArgs(args)
	return args
}

// baseArgs 返回启动命令的基础参数（必传项）。
// 包括模型目录、端口、FIT 模式、禁用原生 webui、绑定 host。
// 注意：Jinja2 模板开关由 appendSwitchArgs 统一控制，此处不再硬编码 --jinja，
// 避免与 appendSwitchArgs 中的 --jinja/--no-jinja 重复。
func (s *Server) baseArgs() []string {
	args := []string{
		"--models-dir", s.config.ModelsDir,
		"--port", fmt.Sprintf("%d", s.config.Port),
		"--fit", "on",
	}
	// 默认禁用 llama-server 自带的 Web UI（豆芽有自己的 Vue 前端）。
	// 仅当用户在设置中启用时才放开，允许通过浏览器访问原生 webui（供高级用户调试）。
	// 注意：b10454+ 将 --webui/--no-webui 弃用，统一改用 --ui/--no-ui。
	if !s.config.EnableWebUI {
		args = append(args, "--no-ui")
	}
	// 根据配置决定绑定地址：暴露则 0.0.0.0（局域网可访问），否则 127.0.0.1（仅本机）
	if s.config.ExposeServer {
		args = append(args, "--host", "0.0.0.0")
	} else {
		args = append(args, "--host", "127.0.0.1")
	}
	return args
}

// appendModelLoadArgs 追加模型加载控制相关参数。
// 包括 models-preset、no-models-autoload、models-max、sleep-idle、gpu-layers、flash-attn、cache-type-k/v。
func (s *Server) appendModelLoadArgs(args []string) []string {
	if s.config.ModelsPreset != "" {
		// 禁用路由器自动加载：豆芽通过 /models/load API 显式控制模型加载时机
		// 原版 llama.cpp 默认 models_autoload=true，会在请求到来时自动加载模型，
		// 这与豆芽的显式加载逻辑冲突，可能导致子进程参数不完整或加载状态混乱
		args = append(args, "--models-preset", s.config.ModelsPreset, "--no-models-autoload")
	}
	args = appendIntArg(args, "--models-max", s.config.ModelsMax)
	args = appendIntArg(args, "--sleep-idle-seconds", s.config.SleepIdleSeconds)
	// 崩溃降级级别 2：gpu-layers 设为 auto（让 llama.cpp 自决层数）
	// 生活类比：连续抛锚后挂空挡，让拖车（llama.cpp）自己决定怎么拖
	gpuLayers := s.config.GPULayers
	if s.crashDegradeLevel.Load() >= 2 {
		log.Warn().Str("original_ngl", gpuLayers).Msg("[server] degrade level 2: gpu-layers overridden to auto")
		gpuLayers = "auto"
	}
	args = appendStringArg(args, "--gpu-layers", gpuLayers)
	args = appendStringArg(args, "--flash-attn", s.config.FlashAttn)
	// KV Cache 量化类型校验：llama-server 9631+ 移除了 q2_k/q3_k/q4_k/q5_k/q6_k/iq4_xs
	args = s.appendValidatedCacheType(args, "--cache-type-k", s.config.CacheTypeK)
	args = s.appendValidatedCacheType(args, "--cache-type-v", s.config.CacheTypeV)
	return args
}

// appendValidatedCacheType 追加经过校验的 cache-type 参数。
// 无效类型会记录警告并跳过，避免 llama-server 启动失败。
func (s *Server) appendValidatedCacheType(args []string, flag, t string) []string {
	if t == "" {
		return args
	}
	if isValidCacheType(t) {
		return append(args, flag, t)
	}
	log.Warn().Str("type", t).Str("flag", flag).Msg("[server] unsupported cache type, skipping (removed q2_k/q3_k/q4_k/q5_k/q6_k/iq4_xs)")
	return args
}

// appendRuntimeArgs 追加运行时资源参数。
// 包括 mlock、线程、batch、上下文大小、mmproj 投影。
func (s *Server) appendRuntimeArgs(args []string) []string {
	// 模型加载模式（llama.cpp #20834 统一为 --load-mode）：
	// DirectIO > Mlock > 非 Mmap > 默认 mmap。LoadMode() 始终返回非空，
	// 显式传递以保持行为确定性（受支持二进制版本的合法取值：none/mmap/mlock/mmap+mlock/dio）。
	if mode := s.config.LoadMode(); mode != "" {
		args = append(args, "--load-mode", mode)
	}
	args = appendIntArg(args, "-t", s.config.Threads)
	args = appendIntArg(args, "-b", s.config.BatchSize)
	args = appendIntArg(args, "-ub", s.config.UBatchSize)
	args = appendIntArg(args, "--threads-http", s.config.ThreadsHTTP)
	// 崩溃降级级别 1：ctx-size 减半（最小 2048，避免过小无法使用）
	// 生活类比：连续抛锚后限速，先把最高时速砍半
	ctxSize := s.config.ContextSize
	if s.crashDegradeLevel.Load() >= 1 && ctxSize > 2048 {
		halved := ctxSize / 2
		if halved < 2048 {
			halved = 2048
		}
		log.Warn().Int("original_ctx", ctxSize).Int("degraded_ctx", halved).Msg("[server] degrade level 1: ctx-size halved")
		ctxSize = halved
	}
	args = appendIntArg(args, "-c", ctxSize)
	args = appendBoolArg(args, "--mmproj-auto", s.config.MmprojAuto)
	args = appendBoolArg(args, "--mmproj-offload", s.config.MmprojOffload)
	return args
}

// appendReasoningArgs 追加推理控制相关参数。
// 包括 reasoning 模式、budget、format、message、preserve 开关。
// 注意：后端采样与推理预算互斥，启用后端采样时强制跳过 reasoning-budget。
func (s *Server) appendReasoningArgs(args []string) []string {
	args = appendStringArg(args, "--reasoning", s.config.Reasoning)
	// 安全实践：后端采样与推理预算互斥，仅前端 UI 联动不够，后端需强制跳过
	if s.config.BackendSampling && s.config.ReasoningBudget > 0 {
		log.Warn().Int("reasoning_budget", s.config.ReasoningBudget).Msg("[server] backend_sampling is enabled, skipping --reasoning-budget (mutually exclusive)")
	} else {
		args = appendIntArg(args, "--reasoning-budget", s.config.ReasoningBudget)
	}
	args = appendStringArg(args, "--reasoning-format", s.config.ReasoningFormat)
	args = appendStringArg(args, "--reasoning-budget-message", s.config.ReasoningBudgetMessage)
	// 推理内容保留开关（v9840+，nil=不传递，使用服务器默认值）
	if s.config.ReasoningPreserve != nil {
		if *s.config.ReasoningPreserve {
			args = append(args, "--reasoning-preserve")
		} else {
			args = append(args, "--no-reasoning-preserve")
		}
	}
	return args
}

// appendKVCacheArgs 追加 KV Cache 与上下文移位参数。
// 包括 kv-unified、cache-idle-slots、cache-ram、image-tokens、fit、mmap、kv-offload、context-shift、keep。
func (s *Server) appendKVCacheArgs(args []string) []string {
	args = appendBoolArg(args, "--kv-unified", s.config.KVUnified)
	args = appendBoolArg(args, "--cache-idle-slots", s.config.CacheIdleSlots)
	args = appendIntArg(args, "--cache-ram", s.config.CacheRAM)
	args = appendIntArg(args, "--image-min-tokens", s.config.ImageMinTokens)
	args = appendIntArg(args, "--image-max-tokens", s.config.ImageMaxTokens)
	args = appendIntArg(args, "--fit-target", s.config.FitTarget)
	args = appendIntArg(args, "--fit-ctx", s.config.FitCtx)
	if !s.config.KVOffload {
		args = append(args, "--no-kv-offload")
	}
	args = appendBoolArg(args, "--context-shift", s.config.ContextShift)
	// 启用 context-shift 时传递 --keep，保护 system prompt 不被移位（P0-B3）
	// 否则一旦启用滑窗，豆芽的身份/规则等 system prompt 可能被从前面丢弃
	if s.config.ContextShift && s.config.KeepSize > 0 {
		args = appendIntArg(args, "--keep", s.config.KeepSize)
	}
	return args
}

// appendSamplingArgs 追加采样参数。
// 包括 min-p、DRY 采样器系列、分组注意力。
func (s *Server) appendSamplingArgs(args []string) []string {
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
			for breaker := range strings.SplitSeq(s.config.DrySequenceBreaker, ",") {
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
	return args
}

// appendSwitchArgs 追加各类开关参数。
// 包括 Jinja2 模板、Prompt 缓存、指标端点、详细日志、Embedding API、池化类型。
func (s *Server) appendSwitchArgs(args []string) []string {
	// Jinja2 模板引擎开关
	// 默认开启（nil 按 true 处理，兼容旧配置升级）
	jinjaEnabled := true
	if s.config.Jinja != nil {
		jinjaEnabled = *s.config.Jinja
	}
	if jinjaEnabled {
		args = append(args, "--jinja")
	} else {
		args = append(args, "--no-jinja")
	}
	// 自定义聊天模板文件（.jinja）：配置后优先于模型 GGUF 自带模板
	// 仅在文件实际存在时传递，避免指向不存在的文件导致启动失败
	if s.config.ChatTemplateFile != "" {
		resolvedTemplate := s.resolvePath(s.config.ChatTemplateFile)
		if info, err := os.Stat(resolvedTemplate); err == nil && !info.IsDir() {
			args = append(args, "--chat-template-file", resolvedTemplate)
		} else {
			log.Warn().Str("chat_template_file", resolvedTemplate).Msg("[server] chat-template-file does not exist, skipping --chat-template-file")
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
	// 启用 embedding API（RAG 知识库需要 /v1/embeddings 接口）
	args = appendBoolArg(args, "--embedding", s.config.Embedding)
	// 嵌入池化类型：聊天模型默认 pooling=none 不兼容 OAI embedding API，需指定 mean
	args = appendStringArg(args, "--pooling", s.config.Pooling)
	return args
}

// appendServiceArgs 追加服务相关参数。
// 包括 reranker、device、parallel、timeout。
func (s *Server) appendServiceArgs(args []string) []string {
	// 重排序端点：配置了 reranker 模型时自动启用
	if s.config.RerankerModelPath != "" {
		args = append(args, "--rerank")
	}
	args = appendStringArg(args, "--device", s.config.Device)
	// 多 GPU 参数（llama.cpp 原生能力，仅配置了有效值时传递）：
	//   --split-mode：layer（按层分割，默认）/ row（按行分割）/ tensor（按张量分割）/ none（禁用多卡）
	//   --tensor-split：逗号分隔的显存分配权重，如 "3,1" = 75%/25%
	//   --main-gpu：指定主 GPU（计算优先级最高的卡），-1=不传递使用默认
	// 注意：--tensor-split 与 --split-mode none 互斥，config 校验层已保证不会同时设置
	if s.config.SplitMode != "" {
		args = append(args, "--split-mode", s.config.SplitMode)
	}
	if s.config.TensorSplit != "" {
		args = append(args, "--tensor-split", s.config.TensorSplit)
	}
	if s.config.MainGPU >= 0 {
		args = append(args, "--main-gpu", fmt.Sprintf("%d", s.config.MainGPU))
	}
	args = appendIntArg(args, "--parallel", s.config.Parallel)
	args = append(args, "--timeout", "900")
	return args
}

// appendSpeculativeArgs 追加推测解码相关参数。
// 启用默认推测配置(spec_default)时跳过所有具体推测参数（互斥）。
// mtpFallbackDisabled=true 时跳过需要 mtp 支持的参数。
func (s *Server) appendSpeculativeArgs(args []string) []string {
	// 安全实践：启用默认推测配置(spec_default)时，推测类型选择需禁用且其他推测参数将被忽略（互斥）
	if s.config.SpecDefault {
		return args
	}
	args = s.appendSpecBasicArgs(args)
	args = s.appendSpecNgramArgs(args)
	args = s.appendSpecLookupArgs(args)
	args = s.appendSpecDraftModelArgs(args)
	return args
}

// appendSpecBasicArgs 追加推测解码基础参数（spec-type、spec-draft-n-max/min、draft cache-type）。
// RF-3 修复：mtpFallbackDisabled 改用 atomic.Bool.Load() 读取
func (s *Server) appendSpecBasicArgs(args []string) []string {
	mtpDisabled := s.mtpFallbackDisabled.Load()
	if s.config.SpecType != "" && !mtpDisabled {
		args = append(args, "--spec-type", s.config.SpecType)
	}
	if s.config.SpecDraftNMax > 0 && !mtpDisabled {
		args = append(args, "--spec-draft-n-max", fmt.Sprintf("%d", s.config.SpecDraftNMax))
	}
	if s.config.SpecDraftNMin > 0 && !mtpDisabled {
		args = append(args, "--spec-draft-n-min", fmt.Sprintf("%d", s.config.SpecDraftNMin))
	}
	args = s.appendValidatedCacheTypeIf(args, "--spec-draft-type-k", s.config.CacheTypeKDraft, mtpDisabled)
	args = s.appendValidatedCacheTypeIf(args, "--spec-draft-type-v", s.config.CacheTypeVDraft, mtpDisabled)
	return args
}

// appendValidatedCacheTypeIf 在条件满足时追加经过校验的 cache-type 参数。
// 用于推测解码的 draft cache-type，需要额外检查 mtpFallbackDisabled。
func (s *Server) appendValidatedCacheTypeIf(args []string, flag, t string, skip bool) []string {
	if skip || t == "" {
		return args
	}
	if isValidCacheType(t) {
		return append(args, flag, t)
	}
	log.Warn().Str("type", t).Str("flag", flag).Msg("[server] unsupported cache type, skipping (removed q2_k/q3_k/q4_k/q5_k/q6_k/iq4_xs)")
	return args
}

// appendSpecNgramArgs 追加 ngram 系列推测解码参数。
// 根据 spec-type 分别处理 ngram-mod/ngram-simple/ngram-map-k/ngram-map-k4v 四种子类型。
func (s *Server) appendSpecNgramArgs(args []string) []string {
	if s.config.SpecType == "ngram-mod" {
		if s.config.SpecNgramModNMin > 0 {
			args = append(args, "--spec-ngram-mod-n-min", fmt.Sprintf("%d", s.config.SpecNgramModNMin))
		}
		if s.config.SpecNgramModNMax > 0 {
			args = append(args, "--spec-ngram-mod-n-max", fmt.Sprintf("%d", s.config.SpecNgramModNMax))
		}
		if s.config.SpecNgramModNMatch > 0 {
			args = append(args, "--spec-ngram-mod-n-match", fmt.Sprintf("%d", s.config.SpecNgramModNMatch))
		}
	}
	if s.config.SpecType == "ngram-simple" {
		if s.config.SpecNgramSimpleSizeN > 0 {
			args = append(args, "--spec-ngram-simple-size-n", fmt.Sprintf("%d", s.config.SpecNgramSimpleSizeN))
		}
		if s.config.SpecNgramSimpleSizeM > 0 {
			args = append(args, "--spec-ngram-simple-size-m", fmt.Sprintf("%d", s.config.SpecNgramSimpleSizeM))
		}
		if s.config.SpecNgramSimpleMinHits > 0 {
			args = append(args, "--spec-ngram-simple-min-hits", fmt.Sprintf("%d", s.config.SpecNgramSimpleMinHits))
		}
	}
	if s.config.SpecType == "ngram-map-k" {
		if s.config.SpecNgramMapKSizeN > 0 {
			args = append(args, "--spec-ngram-map-k-size-n", fmt.Sprintf("%d", s.config.SpecNgramMapKSizeN))
		}
		if s.config.SpecNgramMapKSizeM > 0 {
			args = append(args, "--spec-ngram-map-k-size-m", fmt.Sprintf("%d", s.config.SpecNgramMapKSizeM))
		}
		if s.config.SpecNgramMapKMinHits > 0 {
			args = append(args, "--spec-ngram-map-k-min-hits", fmt.Sprintf("%d", s.config.SpecNgramMapKMinHits))
		}
	}
	if s.config.SpecType == "ngram-map-k4v" {
		if s.config.SpecNgramMapK4VSizeN > 0 {
			args = append(args, "--spec-ngram-map-k4v-size-n", fmt.Sprintf("%d", s.config.SpecNgramMapK4VSizeN))
		}
		if s.config.SpecNgramMapK4VSizeM > 0 {
			args = append(args, "--spec-ngram-map-k4v-size-m", fmt.Sprintf("%d", s.config.SpecNgramMapK4VSizeM))
		}
		if s.config.SpecNgramMapK4VMinHits > 0 {
			args = append(args, "--spec-ngram-map-k4v-min-hits", fmt.Sprintf("%d", s.config.SpecNgramMapK4VMinHits))
		}
	}
	return args
}

// appendSpecLookupArgs 追加 lookup-cache 参数（仅在 ngram-cache 模式下传递）。
func (s *Server) appendSpecLookupArgs(args []string) []string {
	if s.config.LookupCacheStatic != "" && s.config.SpecType == "ngram-cache" {
		args = append(args, "--lookup-cache-static", s.resolvePath(s.config.LookupCacheStatic))
	}
	if s.config.LookupCacheDynamic != "" && s.config.SpecType == "ngram-cache" {
		args = append(args, "--lookup-cache-dynamic", s.config.LookupCacheDynamic)
	}
	return args
}

// appendSpecDraftModelArgs 追加 draft 模型路径参数。
// 仅在 draft-eagle3/draft-dflash/draft-simple/draft-dspark 模式下传递。
// draft-dspark（DSpark 推测解码，llama.cpp b10355 新增）同样通过 --spec-draft-model
// 传入带 Markov 头的草稿模型，采用 anchor-first block layout。
func (s *Server) appendSpecDraftModelArgs(args []string) []string {
	if s.config.SpecDraftModel != "" && (s.config.SpecType == "draft-eagle3" || s.config.SpecType == "draft-dflash" || s.config.SpecType == "draft-simple" || s.config.SpecType == "draft-dspark") {
		args = append(args, "--spec-draft-model", s.resolvePath(s.config.SpecDraftModel))
	}
	return args
}

// appendNewFeatureArgs 追加新特性参数。
// 包括 swa-full、ctx-checkpoints、tools、prefill-assistant、slot-prompt-similarity、
// skip-chat-parsing、api-prefix、simple-io、Agent/MCP、后端采样、SSE ping。
func (s *Server) appendNewFeatureArgs(args []string) []string {
	args = appendBoolArg(args, "--swa-full", s.config.SwaFull)
	args = appendIntArg(args, "--ctx-checkpoints", s.config.CtxCheckpoints)
	args = appendIntArg(args, "--checkpoint-min-step", s.config.CheckpointMinStep)
	// 内置工具：启用全量开关时传 --tools all（覆盖全部内置工具），
	// 否则按细粒度 tools 字符串拼接（config 中二者互斥，EnableBuiltinTools 优先）。
	// 生活类比：全量开关 = 直接开"全员进厨房"权限；细粒度 = 逐个点名放行。
	if s.config.EnableBuiltinTools {
		args = append(args, "--tools", "all")
	} else {
		args = appendStringArg(args, "--tools", s.config.Tools)
	}
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

	// 细粒度 CORS 配置（上游 #25655）：仅当用户显式配置时才传递，避免覆盖 llama.cpp 内置默认。
	// 生活类比：默认只放行本班（localhost），用户登记表（配置）非空时才额外放行。
	args = appendStringArg(args, "--cors-origins", s.config.CorsOrigins)
	args = appendStringArg(args, "--cors-methods", s.config.CorsMethods)
	args = appendStringArg(args, "--cors-headers", s.config.CorsHeaders)
	args = appendBoolArg(args, "--cors-credentials", s.config.CorsCredentials)

	// MCP 服务器配置文件：豆芽在 AppDir 下生成 mcp_servers.json，
	// 由 llama-server 通过 --mcp-servers-config 加载并管理所有 MCP 子进程。
	// 仅当文件存在时传递，避免 llama-server 因找不到文件而启动失败。
	// 注意：修改 MCP 配置后需重启 llama-server 才能生效（豆芽无热重载能力）。
	if s.config.AppDir != "" {
		mcpConfigPath := filepath.Join(s.config.AppDir, "mcp_servers.json")
		if info, err := os.Stat(mcpConfigPath); err == nil && !info.IsDir() {
			args = append(args, "--mcp-servers-config", mcpConfigPath)
		}
	}

	// 后端采样（实验性，将采样逻辑移到 GPU 执行）
	args = appendBoolArg(args, "--backend-sampling", s.config.BackendSampling)
	// SSE ping 间隔（保持长连接活跃，防止代理/防火墙超时断连）
	args = appendIntArg(args, "--sse-ping-interval", s.config.SsePingInterval)
	return args
}

// appendLoraArgs 追加 LoRA 适配器与 KV 缓存持久化参数。
// LoRA 启动时加载但默认不应用（scale=0），用户可通过设置界面热切换。
// KV 缓存持久化启用时自动创建目录并填充默认路径。
func (s *Server) appendLoraArgs(args []string) []string {
	// LoRA 适配器：启动时加载但默认不应用（scale=0），用户可通过设置界面热切换
	if s.config.LoraPaths != "" {
		for p := range strings.SplitSeq(s.config.LoraPaths, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				args = append(args, "--lora", p)
			}
		}
		args = append(args, "--lora-init-without-apply")
	}

	// KV 缓存持久化：启用后传递 --slot-save-path
	if s.config.SlotSaveEnabled {
		slotPath := s.config.SlotSavePath
		if slotPath == "" {
			// 启用但路径为空时自动填充默认路径，避免 llama-server 因缺少参数报错
			slotPath = filepath.Join(s.config.AppDir, "slots")
			log.Warn().Str("slot_save_path", slotPath).Msg("[server] SlotSaveEnabled is true but SlotSavePath is empty, using default path")
		}
		// 确保目录存在，避免 llama-server 写入失败
		if err := os.MkdirAll(slotPath, 0o755); err != nil {
			log.Warn().Err(err).Str("slot_save_path", slotPath).Msg("[server] failed to create slot save directory")
		}
		args = append(args, "--slot-save-path", slotPath)
	}

	// KV 缓存复用
	args = appendIntArg(args, "--cache-reuse", s.config.CacheReuse)
	return args
}

// appendAdvancedArgs 追加高级参数。
// 包括 Draft GPU 配置、多模态批处理、自适应采样、媒体路径、离线模式、
// 模型重打包、Draft 线程、默认推测、Direct IO、MoE 卸载、OpOffload。
func (s *Server) appendAdvancedArgs(args []string) []string {
	args = s.appendDraftGpuArgs(args)
	args = s.appendMultimodalArgs(args)
	args = s.appendMediaOfflineArgs(args)
	args = s.appendDraftThreadsArgs(args)
	args = s.appendCPUMoeArgs(args)
	return args
}

// appendDraftGpuArgs 追加 Draft 模型 GPU 与推测解码参数。
// mtpFallbackDisabled=true 时跳过所有参数。
// RF-3 修复：mtpFallbackDisabled 改用 atomic.Bool.Load() 读取
func (s *Server) appendDraftGpuArgs(args []string) []string {
	if s.mtpFallbackDisabled.Load() {
		return args
	}
	if s.config.SpecDraftNgl > 0 {
		args = append(args, "--spec-draft-ngl", fmt.Sprintf("%d", s.config.SpecDraftNgl))
	}
	if s.config.SpecDraftDevice != "" {
		args = append(args, "--spec-draft-device", s.config.SpecDraftDevice)
	}
	if s.config.SpecDraftPSplit > 0 {
		args = append(args, "--spec-draft-p-split", fmt.Sprintf("%.2f", s.config.SpecDraftPSplit))
	}
	if s.config.SpecDraftPMin > 0 {
		args = append(args, "--spec-draft-p-min", fmt.Sprintf("%.2f", s.config.SpecDraftPMin))
	}
	if s.config.SpecDraftBackendSampling != nil {
		if *s.config.SpecDraftBackendSampling {
			args = append(args, "--spec-draft-backend-sampling")
		} else {
			args = append(args, "--no-spec-draft-backend-sampling")
		}
	}
	return args
}

// appendMultimodalArgs 追加多模态与自适应采样参数。
func (s *Server) appendMultimodalArgs(args []string) []string {
	// 多模态批处理
	args = appendIntArg(args, "--mtmd-batch-max-tokens", s.config.MtmdBatchMaxTokens)
	// 自适应采样（llama.cpp 新增）
	args = appendFloatArg(args, "--adaptive-target", s.config.AdaptiveTarget, "%.4f")
	args = appendFloatArg(args, "--adaptive-decay", s.config.AdaptiveDecay, "%.4f")
	// 模型标签
	args = appendStringArg(args, "--tags", s.config.Tags)
	return args
}

// appendMediaOfflineArgs 追加媒体路径、离线模式、模型重打包参数。
// 媒体路径仅在目录实际存在时传递，避免指向不存在的目录导致启动失败。
func (s *Server) appendMediaOfflineArgs(args []string) []string {
	// 媒体路径（多模态模型额外媒体文件目录）：仅在目录实际存在时传递，避免指向不存在的目录导致启动失败
	if s.config.MediaPath != "" {
		resolvedMediaPath := s.resolvePath(s.config.MediaPath)
		if info, err := os.Stat(resolvedMediaPath); err == nil && info.IsDir() {
			args = append(args, "--media-path", resolvedMediaPath)
		} else {
			log.Warn().Str("media_path", resolvedMediaPath).Msg("[server] media-path directory does not exist, skipping --media-path")
		}
	}
	// 离线模式（禁用所有网络请求）
	args = appendBoolArg(args, "--offline", s.config.Offline)
	// 模型重打包（启动时重新打包模型权重）
	args = appendBoolArg(args, "--repack", s.config.Repack)
	return args
}

// appendDraftThreadsArgs 追加 Draft 模型线程配置与默认推测解码开关。
// mtpFallbackDisabled=true 时跳过线程参数。
// RF-3 修复：mtpFallbackDisabled 改用 atomic.Bool.Load() 读取
func (s *Server) appendDraftThreadsArgs(args []string) []string {
	if !s.mtpFallbackDisabled.Load() {
		if s.config.SpecDraftThreads > 0 {
			args = append(args, "--spec-draft-threads", fmt.Sprintf("%d", s.config.SpecDraftThreads))
		}
		if s.config.SpecDraftThreadsBatch > 0 {
			args = append(args, "--spec-draft-threads-batch", fmt.Sprintf("%d", s.config.SpecDraftThreadsBatch))
		}
	}
	// 默认推测解码配置
	args = appendBoolArg(args, "--spec-default", s.config.SpecDefault)
	return args
}

// appendCPUMoeArgs 追加 Direct IO、MoE 卸载、OpOffload 参数。
func (s *Server) appendCPUMoeArgs(args []string) []string {
	// 直接 I/O 已合并进 --load-mode（由 appendRuntimeArgs 按优先级统一处理）
	// MoE 权重 CPU 卸载
	if s.config.CPUMoe {
		args = append(args, "--cpu-moe")
	}
	args = appendIntArg(args, "--n-cpu-moe", s.config.NCpuMoe)
	// 算子卸载开关（nil=使用默认值，true=--op-offload，false=--no-op-offload）
	if s.config.OpOffload != nil {
		if *s.config.OpOffload {
			args = append(args, "--op-offload")
		} else {
			args = append(args, "--no-op-offload")
		}
	}
	return args
}
