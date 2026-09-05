// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
	"douya/internal/llm"
	"douya/internal/system"
)

func (s *Service) DetectModelArchitecture() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	client := s.getClientSnapshot()
	if client == nil {
		return s.DetectModelArchitectureForModel("")
	}
	info, err := client.GetModelInfo(ctx)
	if err != nil || info == nil {
		// 服务器返回 nil-info 但无错误时也回退到通用检测，避免 *info 解引用 panic
		return s.DetectModelArchitectureForModel("")
	}
	return s.DetectModelArchitectureForModel(info.Name)
}

// templateModeMarkers 是 chat template 中代表"模板式思考"（ThinkingModeTemplate）的控制标记。
// 这类标记通过 enable_thinking / <|think|> 等开关与边界块控制思考，对应前端可软开关的思考按钮。
var templateModeMarkers = []string{
	"<|think|>",
	"enable_thinking",
	"enable_think",
	"startthinking",
}

// reasoningModeMarkers 是 chat template 中代表"推理式思考"（ThinkingModeReasoning）的控制标记。
// 这类标记（DeepSeek 系）通过 reasoning_effort / reasoning_content 等参数控制思考，
// 不依赖 enable_thinking 开关。
var reasoningModeMarkers = []string{
	"reasoning_effort",
	"reasoning_content",
	"<|reasoning_start|>",
}

// thinkingModeFromTemplate 分析 chat template 内容，判定模型思考模式与能力。
// 返回 (thinkingMode, supportsReasoning, softSwitchSupport)。
// 未发现任何思考标记时返回 (llm.ThinkingModeNone, false, false)，调用方应回退到白名单检测。
func thinkingModeFromTemplate(template string) (string, bool, bool) {
	if template == "" {
		return llm.ThinkingModeNone, false, false
	}
	// 模板不含任何思考标记，直接判定为不支持思考
	if !system.HasThinkingInTemplate(template) {
		return llm.ThinkingModeNone, false, false
	}
	lower := strings.ToLower(template)
	hasTemplate := matchAnyMarker(lower, templateModeMarkers)
	hasReasoning := matchAnyMarker(lower, reasoningModeMarkers)
	switch {
	case hasTemplate:
		// 含 enable_thinking 开关的模板支持思考软开关（auto/on/off）
		soft := strings.Contains(lower, "enable_thinking") || strings.Contains(lower, "enable_think")
		return llm.ThinkingModeTemplate, true, soft
	case hasReasoning:
		return llm.ThinkingModeReasoning, true, false
	default:
		return llm.ThinkingModeNone, false, false
	}
}

// matchAnyMarker 检查 target 是否包含 markers 中的任意标记（子串匹配）。
func matchAnyMarker(target string, markers []string) bool {
	for _, m := range markers {
		if strings.Contains(target, m) {
			return true
		}
	}
	return false
}

// modelKeywordConfig 定义模型关键词匹配配置
type modelKeywordConfig struct {
	keywords     []string
	thinkingMode string
	softSwitch   bool
}

// matchModelKeywords 根据配置列表按优先级匹配模型关键词，
// 返回 (thinkingMode, supportsReasoning, softSwitchSupport)。
// 未匹配时 thinkingMode 为 llm.ThinkingModeNone。
func matchModelKeywords(target string, configs []modelKeywordConfig) (string, bool, bool) {
	for _, cfg := range configs {
		for _, kw := range cfg.keywords {
			if strings.Contains(target, kw) {
				return cfg.thinkingMode, true, cfg.softSwitch
			}
		}
	}
	return llm.ThinkingModeNone, false, false
}

func (s *Service) DetectModelArchitectureForModel(modelName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// 在函数入口获取快照，避免 goroutine 中数据竞争
	client := s.getClientSnapshot()
	cfg := s.getConfigSnapshot()

	// Parallel fetch: model info and server props
	type infoResult struct {
		info *llm.ModelInfo
		err  error
	}
	type propsResult struct {
		props *llm.ServerProps
		err   error
	}

	infoCh := make(chan infoResult, 1)
	propsCh := make(chan propsResult, 1)

	// Check if we have cached props from a previous call (e.g., SwitchModel's mmproj wait)
	s.cachedPropsMu.RLock()
	cached := s.cachedProps
	s.cachedPropsMu.RUnlock()
	// Clear cache after reading (one-time use)
	if cached != nil {
		s.cachedPropsMu.Lock()
		s.cachedProps = nil
		s.cachedPropsMu.Unlock()
	}

	go func() {
		// 防止 panic 导致 infoCh 永不写入、调用方永久阻塞
		defer func() {
			if r := recover(); r != nil {
				infoCh <- infoResult{nil, apperror.Newf(apperror.KindInternal, "get model info panic: %v", r)}
			}
		}()
		var info *llm.ModelInfo
		var err error
		if client == nil {
			infoCh <- infoResult{nil, apperror.New(apperror.KindUnavailable, "llm client is nil")}
			return
		}
		if modelName != "" {
			info, err = client.GetModelInfoByName(ctx, modelName)
		} else {
			info, err = client.GetModelInfo(ctx)
		}
		infoCh <- infoResult{info, err}
	}()

	// 注：此处未使用 trackedGo，因为该 goroutine 为短生命周期且已有 defer recover()，
	// 通过 propsCh 返回结果，ctx 超时会自动退出。见安全审查 #26。
	go func() {
		// 防止 panic 导致 propsCh 永不写入、调用方永久阻塞
		defer func() {
			if r := recover(); r != nil {
				propsCh <- propsResult{nil, apperror.Newf(apperror.KindInternal, "get server props panic: %v", r)}
			}
		}()
		if cached != nil {
			propsCh <- propsResult{cached, nil}
			return
		}
		if client == nil {
			propsCh <- propsResult{nil, apperror.New(apperror.KindUnavailable, "llm client is nil")}
			return
		}
		props, err := client.GetServerProps(ctx, modelName)
		propsCh <- propsResult{props, err}
	}()

	// Wait for model info (required)
	ir := <-infoCh
	if ir.err != nil {
		// Drain props channel
		<-propsCh
		return apperror.Wrap(apperror.KindInternal, "failed to get model info", ir.err)
	}
	info := ir.info

	// 防御：服务器可能返回 nil-info 但无错误（GetModelInfo 契约未保证 info 非 nil）。
	// 直接 *info 解引用会 panic，这里显式判空并返回错误。
	if info == nil {
		return apperror.New(apperror.KindInternal, "model info is nil despite no error")
	}

	// Wait for props (optional, best-effort)
	pr := <-propsCh
	props, propsErr := pr.props, pr.err

	caps := llm.DetectCapabilities(*info)
	var supportsReasoning bool
	var softSwitchSupport bool
	var mmprojLoaded bool
	var supportsPreserveReasoning bool
	thinkingMode := llm.ThinkingModeNone
	defaultThinkingAuto := ""

	if propsErr == nil {
		log.Info().
			Bool("vision", props.Modalities.Vision).
			Bool("audio", props.Modalities.Audio).
			Bool("supports_tools", props.ChatTemplateCaps.SupportsTools).
			Bool("supports_preserve_reasoning", props.ChatTemplateCaps.SupportsPreserveReasoning).
			Str("build_info", props.BuildInfo).
			Msg("[model] /props")

		mmprojLoaded = props.Modalities.Vision || props.Modalities.Audio
		caps.ImageInput = props.Modalities.Vision
		caps.AudioInput = props.Modalities.Audio
		caps.VideoInput = props.Modalities.Video

		if props.ChatTemplateCaps.SupportsPreserveReasoning {
			supportsReasoning = true
			thinkingMode = llm.ThinkingModeTemplate
		}
		// 检测模型是否支持 tool call
		// 优先级：/props chat_template_caps.supports_tools > chat_template_tool_use > GGUF 元数据
		if props.ChatTemplateCaps.SupportsTools || props.ChatTemplateCaps.SupportsToolCalls {
			caps.ToolCallSupport = true
		} else if props.ChatTemplateToolUse != "" {
			caps.ToolCallSupport = true
		} else {
			// /props 未返回原生模板，回退到 GGUF 元数据判断
			caps.ToolCallSupport = s.detectToolCallFromGGUF()
		}
		// 并发 tool call 支持：直接从 /props 能力声明获取
		caps.SupportsParallelToolCalls = props.ChatTemplateCaps.SupportsParallelToolCalls
		// system role 支持：默认 true（兼容旧版 llama-server 未返回该字段的情况）
		// 仅当 /props 明确返回 false 时才设为 false（如 Gemma 系列）
		caps.SupportsSystemRole = true
		if !props.ChatTemplateCaps.SupportsSystemRole {
			caps.SupportsSystemRole = false
		}
	} else {
		log.Warn().Err(propsErr).Msg("[model] /props failed, using GGUF metadata as fallback")
		caps.ToolCallSupport = s.detectToolCallFromGGUF()
		// /props 不可用时，保守假设不支持并发 tool call，但支持 system role（多数模型支持）
		caps.SupportsParallelToolCalls = false
		caps.SupportsSystemRole = true
	}

	if thinkingMode == llm.ThinkingModeNone {
		// 优先使用 GGUF 元数据中的 architecture 字段推断
		var ggufMeta *system.GGUFMetadata
		modelPath := s.resolveModelPath(cfg.ModelPath)
		if modelPath != "" {
			if meta, err := system.ParseGGUFMetadataCached(modelPath); err == nil {
				ggufMeta = meta
			}
		}
		// 模板内容分析：模板是思考机制的载体，含思考标记即证明模型具备思考能力，
		// 可覆盖自定义/合并模型（如文件名不含标准关键词、但模板实际支持思考的模型）。
		// 优先级：模板分析 → architecture 字段 → 文件名关键词。
		if ggufMeta != nil && ggufMeta.ChatTemplate != "" {
			if mode, reasoning, soft := thinkingModeFromTemplate(ggufMeta.ChatTemplate); mode != llm.ThinkingModeNone {
				thinkingMode = mode
				supportsReasoning = reasoning
				softSwitchSupport = soft
				log.Info().Str("architecture", ggufMeta.Architecture).Str("mode", mode).Msg("[model] thinking detected from chat template")
			}
		}
		// 模板默认自主思考档位（无论是否命中 thinkingMode 都记录，供 auto 模式恢复思考用）
		if ggufMeta != nil {
			defaultThinkingAuto = ggufMeta.DefaultThinkingAuto
		}
		if thinkingMode == llm.ThinkingModeNone && ggufMeta != nil && ggufMeta.Architecture != "" {
			lowerArch := strings.ToLower(ggufMeta.Architecture)
			archConfigs := []modelKeywordConfig{
				{keywords: []string{"qwen3", "qwen3moe", "qwen3next", "qwen3vl", "qwen3vlmoe", "qwen35", "qwen35moe", "qwen36", "qwen4exp", "nemotron_h"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: true},
				{keywords: []string{"gemma2", "gemma4", "gemma4-assistant", "gemma3n", "llama4", "phi4", "mistral3", "mistral4", "glm4", "chatglm4", "glm4moe", "cohere2moe", "tiny-aya", "ernie4-5", "ernie4-5-moe", "minimax-m2", "minicpm5", "smollm3", "hunyuan-moe", "hunyuan-dense", "step35", "kimi-linear", "arcee", "dots1", "dream", "smallthinker"}, thinkingMode: llm.ThinkingModeTemplate},
				{keywords: []string{"deepseek3", "deepseek2", "deepseek32", "deepseek4", "deepseek-v4"}, thinkingMode: llm.ThinkingModeReasoning},
			}
			if mode, reasoning, soft := matchModelKeywords(lowerArch, archConfigs); mode != llm.ThinkingModeNone {
				thinkingMode = mode
				supportsReasoning = reasoning
				softSwitchSupport = soft
			}
		}

		// 兜底：文件名关键词匹配
		if thinkingMode == llm.ThinkingModeNone {
			lowerName := strings.ToLower(info.Name)
			nameConfigs := []modelKeywordConfig{
				{keywords: []string{"qwen3", "qwq", "qwen3moe", "qwen3-next", "qwen3next", "qwen3-vl", "qwen3vl", "qwen3.5", "qwen3.6", "qwen3.8", "qwen35", "qwen35moe", "qwen36", "qwen38", "qwen4exp", "nemotron-3", "nemotron3"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: true},
				{keywords: []string{"gemma-4-assistant", "gemma4-assistant", "gemma-4", "gemma4", "gemma-2", "gemma-3", "gemma3", "gemma-3n", "gemma3n", "llama-4", "llama4", "mistral-small-3", "mistral-small3", "mistral-small3.1", "mistral-3", "mistral3", "mistral-4", "mistral4", "phi-4-reasoning-plus", "glm4", "chatglm4", "glm-4-moe", "glm4moe", "cohere2moe", "tiny-aya", "ernie-4.5", "ernie4.5", "minimax-m2", "minimaxm2", "minicpm5", "minicpm-5", "smollm3", "smol-lm3", "hunyuan-moe", "hunyuan-dense", "step3.5", "step3.7", "kimi-linear", "arcee", "dots1", "dream", "smallthinker"}, thinkingMode: llm.ThinkingModeTemplate},
				{keywords: []string{"deepseek-r1", "deepseek-v2", "deepseek-v3", "deepseek-v4", "deepseek-r", "deepseek-3.2", "deepseek32", "phi-4-reasoning", "phi4-reasoning", "hy4", "hy-4", "hy_v4"}, thinkingMode: llm.ThinkingModeReasoning},
			}
			if mode, reasoning, soft := matchModelKeywords(lowerName, nameConfigs); mode != llm.ThinkingModeNone {
				thinkingMode = mode
				supportsReasoning = reasoning
				softSwitchSupport = soft
			}
		}
	}

	s.modelCapsMu.Lock()
	s.modelCaps = llm.ModelCapabilities{
		ImageInput:                caps.ImageInput,
		AudioInput:                caps.AudioInput,
		VideoInput:                caps.VideoInput,
		TextInput:                 caps.TextInput,
		TextGeneration:            caps.TextGeneration,
		Reasoning:                 supportsReasoning,
		MmprojLoaded:              mmprojLoaded,
		HasMTP:                    s.detectHasMTP(),
		ThinkingMode:              thinkingMode,
		SoftSwitchSupport:         softSwitchSupport,
		DefaultThinkingAuto:       defaultThinkingAuto,
		NParams:                   s.resolveNParams(info.Meta.NParams),
		ToolCallSupport:           caps.ToolCallSupport,
		SupportsPreserveReasoning: supportsPreserveReasoning,
		SupportsParallelToolCalls: caps.SupportsParallelToolCalls,
		SupportsSystemRole:        caps.SupportsSystemRole,
	}
	s.modelCapsMu.Unlock()
	// FIX: Only set detectedModelName when it's empty (called from DetectModelArchitecture without model name).
	// When called from SwitchModel, SetDetectedModelName() has already set the correct name.
	// Do NOT overwrite with info.Name, which may differ from the user-selected model name.
	s.detectedModelMu.Lock()
	if s.detectedModelName == "" {
		s.detectedModelName = info.Name
	}
	s.detectedModelMu.Unlock()
	log.Info().
		Str("name", info.Name).
		Str("model", modelName).
		Interface("server_caps", info.Capabilities).
		Bool("image", caps.ImageInput).
		Bool("audio", caps.AudioInput).
		Bool("text", caps.TextInput).
		Bool("reasoning", supportsReasoning).
		Str("thinking_mode", thinkingMode).
		Bool("soft_switch", softSwitchSupport).
		Msg("[model] detected capabilities")

	return nil
}

func (s *Service) GetDetectedModelName() string {
	s.detectedModelMu.RLock()
	defer s.detectedModelMu.RUnlock()
	return s.detectedModelName
}

func (s *Service) SetDetectedModelName(name string) {
	s.detectedModelMu.Lock()
	s.detectedModelName = name
	s.detectedModelMu.Unlock()
	s.InvalidatePromptCache()
}

// SetCachedProps caches a ServerProps result for use by DetectModelArchitectureForModel,
// avoiding a redundant HTTP call when the caller has already fetched props.
func (s *Service) SetCachedProps(props *llm.ServerProps) {
	s.cachedPropsMu.Lock()
	s.cachedProps = props
	s.cachedPropsMu.Unlock()
}

func (s *Service) InvalidatePromptCache() {
	s.promptMu.Lock()
	defer s.promptMu.Unlock()
	s.sysPromptCache = ""
	s.sysPromptDate = ""
	s.sysPromptConfig = ""
}

func (s *Service) GetModelCapabilities() llm.ModelCapabilities {
	s.modelCapsMu.RLock()
	defer s.modelCapsMu.RUnlock()
	return s.modelCaps
}

// IsTextGenerationAvailable 判断当前模型是否支持文本生成（对话）。
// 后端权威拦截的依据，SendMessage / RegenerateMessage 均据此拒绝嵌入模型。
// 采取"保守放行、确凿才拦"策略，两个信号取任一确凿证据即为不可用：
//  1. 能力已探测（TextGenerationKnown）且 TextGeneration == false
//     （llama-server 明确报告仅嵌入/重排能力）
//  2. 模型名兜底匹配（嵌入模型名片段）
//
// 关键：能力未探测时（TextGenerationKnown == false，如测试环境/启动早期）
// 一律放行，不能因 TextGeneration 的零值 false 误伤正常的对话模型。
func (s *Service) IsTextGenerationAvailable() bool {
	s.modelCapsMu.RLock()
	textGen := s.modelCaps.TextGeneration
	known := s.modelCaps.TextGenerationKnown
	s.modelCapsMu.RUnlock()
	// 已确凿探测到是不支持文本生成的模型 → 不可用
	if known && !textGen {
		return false
	}
	// 模型名兜底匹配 → 不可用
	return !llm.IsEmbeddingModelName(s.GetDetectedModelName())
}

// embeddingBlockedMessage 生成嵌入模型被拦截时的统一友好提示文案。
func (s *Service) embeddingBlockedMessage(action string) string {
	modelName := s.GetDetectedModelName()
	if modelName == "" {
		modelName = "当前模型"
	}
	return fmt.Sprintf("当前模型「%s」是嵌入模型，仅支持文本检索（如知识库问答），不能进行对话。请切换到对话类模型后再%s。", modelName, action)
}

// GetThinkingSoftSwitch 获取当前思考软开关状态（前端兼容用）
// 内部映射自 cfg.Reasoning：on → "think"，off/空 → "no_think"（默认不思考）
func (s *Service) GetThinkingSoftSwitch() string {
	cfg := s.getConfigSnapshot()
	if cfg == nil {
		return "no_think"
	}
	switch cfg.Reasoning {
	case "on":
		return "think"
	default:
		return "no_think"
	}
}

func (s *Service) SetModelCapabilities(caps llm.ModelCapabilities) {
	s.modelCapsMu.Lock()
	defer s.modelCapsMu.Unlock()
	s.modelCaps = caps
}

func (s *Service) resolveModelPath(p string) string {
	if p == "" {
		return ""
	}
	p = filepath.Clean(p)
	if filepath.IsAbs(p) {
		return p
	}
	if s.appDir != "" {
		return filepath.Join(s.appDir, p)
	}
	return p
}

// detectHasMTP 检测模型是否支持 MTP
func (s *Service) detectHasMTP() bool {
	cfg := s.getConfigSnapshot()
	if cfg == nil {
		return false
	}
	modelPath := s.resolveModelPath(cfg.ModelPath)
	if modelPath == "" {
		return false
	}
	meta, err := system.ParseGGUFMetadataCached(modelPath)
	if err != nil {
		log.Warn().Err(err).Str("path", modelPath).Msg("[model] GGUF parse failed for MTP detection")
		return false
	}
	if meta.HasMTP {
		log.Info().Str("path", modelPath).Msg("[model] MTP support detected from GGUF metadata")
	}
	return meta.HasMTP
}

func (s *Service) resolveNParams(serverNParams float64) float64 {
	if serverNParams > 0 {
		return serverNParams
	}
	cfg := s.getConfigSnapshot()
	if cfg == nil {
		return 0
	}
	modelPath := s.resolveModelPath(cfg.ModelPath)
	if modelPath == "" {
		return 0
	}
	meta, err := system.ParseGGUFMetadataCached(modelPath)
	if err != nil {
		return 0
	}
	return system.ResolveNParams(0, meta)
}

// detectToolCallFromGGUF 基于 GGUF 元数据判断模型是否支持 tool call
// 优先检查 chat_template_tool_use 字段，其次检查 ChatTemplate 中是否包含 tool 相关语法
func (s *Service) detectToolCallFromGGUF() bool {
	cfg := s.getConfigSnapshot()
	if cfg == nil {
		return false
	}
	modelPath := s.resolveModelPath(cfg.ModelPath)
	if modelPath == "" {
		return false
	}
	meta, err := system.ParseGGUFMetadataCached(modelPath)
	if err != nil {
		return false
	}
	// GGUF 元数据中有专门的 tool use 模板
	if meta.ChatTemplateToolUse != "" {
		return true
	}
	// 检查 ChatTemplate 中是否包含 tool 相关语法
	if meta.ChatTemplate != "" {
		lower := strings.ToLower(meta.ChatTemplate)
		if strings.Contains(lower, "tool_call") || strings.Contains(lower, "tool_use") {
			return true
		}
	}
	return false
}

func (s *Service) applyThinkingControl(req *llm.ChatCompletionRequest) {
	s.modelCapsMu.RLock()
	mode := s.modelCaps.ThinkingMode
	s.modelCapsMu.RUnlock()

	if mode == llm.ThinkingModeNone {
		return
	}

	cfg := s.getConfigSnapshot()
	budget := 0
	if cfg != nil {
		budget = cfg.ReasoningBudget
	}

	// 推理模型启用 reasoning_control，允许通过 /v1/chat/completions/control 实时结束思考
	req.ReasoningControl = true

	// enable_thinking 与请求级 Reasoning 状态均由 llama-server --reasoning 启动参数统一处理，
	// 此处仅保留 ReasoningBudget 作为请求级预算控制。
	if budget > 0 {
		req.ReasoningBudget = budget
	}

	// 请求级 reasoning_effort 逃逸口（llama.cpp #26045）：当用户配置 reasoning=off 时，
	// 额外写入 "none"，强制服务端 enable_thinking=false。
	// 生活类比：服务器默认档位（--reasoning）说了算，但这次请求要单独关掉思考，
	// 就给调度中心发一条"这单不走思考"的备注（reasoning_effort=none）。
	if cfg != nil && cfg.Reasoning == "off" {
		req.ReasoningEffort = "none"
	}

	// 思考强度透传（原生）：当用户配置了 reasoning_effort 且思考未关闭时，
	// 直接设置请求级 OAI 字段 ReasoningEffort，交由新版 llama-server（#26045 + #27041）
	// 原生转发给 jinja 模板（DeepSeek-V4 / openai-gpt-oss-120b / Hy3 等），
	// 由模板注入 "Reasoning Effort: ..." 等引导语。服务器层不识别 low/high/max，
	// 仅模板侧有语义；对不读取该参数的模板是无害的 no-op。
	// 生活类比：把"火力档位"写进订单（原生字段），服务端原生转发给厨师（模板）。
	if cfg != nil && cfg.Reasoning != "off" && cfg.ReasoningEffort != "" {
		req.ReasoningEffort = cfg.ReasoningEffort
	}

	// 传递请求级 reasoning 扩展字段（仅在用户显式配置时才传递，避免覆盖服务器默认值）
	if cfg != nil {
		if cfg.ReasoningBudgetStartTag != "" {
			req.ReasoningBudgetStartTag = cfg.ReasoningBudgetStartTag
		}
		if cfg.ReasoningBudgetEndTag != "" {
			req.ReasoningBudgetEndTag = cfg.ReasoningBudgetEndTag
		}
	}

	// 所有 ThinkingModeTemplate 模型通过 chat_template_kwargs 传递 enable_thinking。
	// 思考开关映射（基于 llama.cpp 模板语义）：
	//   - on:  显式 enable_thinking=true，强制要求模型思考
	//   - off: 显式 enable_thinking=false，强制要求模型不思考
	//   - 空: 不干预，交给 llama-server 与模板默认行为决定。
	//     尊重模型自身行为：模板默认不思考（如 Gemma）就不强制开启，
	//     否则简单问候与简单问题也会被强制长时间思考。
	// 对于 ThinkingModeReasoning 模型（DeepSeek），思考由服务端 reasoning 参数控制，无需 kwargs
	if mode == llm.ThinkingModeTemplate {
		explicit := false
		var enableThinking bool
		switch {
		case cfg != nil && cfg.Reasoning == "on":
			explicit, enableThinking = true, true
		case cfg != nil && cfg.Reasoning == "off":
			explicit, enableThinking = true, false
		default:
			// 空：不设置 enable_thinking，尊重模型模板自身的默认行为
		}
		if explicit {
			if req.ChatTemplateKwargs == nil {
				req.ChatTemplateKwargs = make(map[string]any)
			}
			req.ChatTemplateKwargs["enable_thinking"] = enableThinking
		}
	}
}

// applySamplingParams 将请求级采样参数从 Config 应用到 ChatCompletionRequest。
// 仅在配置非空/非零时传递，避免覆盖服务器默认值。
// 生活类比：就像厨师做菜时按需添加调料，只有客人明确要求时才加，否则用厨房的默认口味。
func (s *Service) applySamplingParams(req *llm.ChatCompletionRequest) {
	cfg := s.getConfigSnapshot()
	if cfg == nil {
		return
	}
	// 自定义采样器顺序（逗号分隔，如 "top_k,top_p,temperature"）
	if cfg.Samplers != "" {
		samplers := strings.Split(cfg.Samplers, ",")
		for i, v := range samplers {
			samplers[i] = strings.TrimSpace(v)
		}
		req.Samplers = samplers
	}
	// 忽略 EOS 继续生成
	if cfg.IgnoreEos {
		req.IgnoreEos = true
	}
	// 请求级 verbose（复用服务端 verbose 配置，在响应中包含调试信息）
	if cfg.Verbose {
		req.Verbose = true
	}
	// 请求级自适应采样目标（0=禁用，使用服务器启动参数）
	if cfg.AdaptiveTarget > 0 {
		req.AdaptiveTarget = cfg.AdaptiveTarget
	}
	// 请求级自适应采样衰减（0=禁用，使用服务器启动参数）
	if cfg.AdaptiveDecay > 0 {
		req.AdaptiveDecay = cfg.AdaptiveDecay
	}
}

func (s *Service) modelNameForRequest() string {
	s.detectedModelMu.RLock()
	defer s.detectedModelMu.RUnlock()
	if s.detectedModelName != "" {
		return s.detectedModelName
	}
	return "default"
}
