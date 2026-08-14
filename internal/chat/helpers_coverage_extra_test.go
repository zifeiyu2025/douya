// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/store"
)

// =============================================================================
// service.go 简单 getter/setter 测试
// 这些方法本身逻辑简单（加锁读写），但 0% 覆盖率，批量补充以提升整体覆盖率。
// 生活类比：像检查每个电灯开关是否能正常开关——虽然简单，但全部验证一遍才放心。
// =============================================================================

// TestSetHostContextAndGet 设置并读取 hostCtx
func TestSetHostContextAndGet(t *testing.T) {
	s := &Service{}
	if got := s.getHostContextSnapshot(); got != nil {
		t.Errorf("初始 hostCtx 应为 nil, 实际 %v", got)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.SetHostContext(ctx)
	if got := s.getHostContextSnapshot(); got != ctx {
		t.Error("SetHostContext 后应能读到同一 ctx")
	}
}

// TestLastPromptTokens 读写最近 prompt token 数
func TestLastPromptTokens(t *testing.T) {
	s := &Service{}
	if got := s.LastPromptTokens(); got != 0 {
		t.Errorf("初始值应为 0, 实际 %d", got)
	}
	s.tokenCalibMu.Lock()
	s.lastPromptTokens = 1234
	s.tokenCalibMu.Unlock()
	if got := s.LastPromptTokens(); got != 1234 {
		t.Errorf("设置后应返回 1234, 实际 %d", got)
	}
}

// TestCurrentConvIDAndIsGenerating 当前会话 ID 与生成状态
func TestCurrentConvIDAndIsGenerating(t *testing.T) {
	s := &Service{}
	if got := s.CurrentConvID(); got != "" {
		t.Errorf("初始 convID 应为空, 实际 %q", got)
	}
	if s.IsGenerating() {
		t.Error("初始状态不应在生成中")
	}
	s.mutex.Lock()
	s.currentConvID = "conv-123"
	s.mutex.Unlock()
	if got := s.CurrentConvID(); got != "conv-123" {
		t.Errorf("设置后应返回 conv-123, 实际 %q", got)
	}
	if !s.IsGenerating() {
		t.Error("设置 convID 后应在生成中")
	}
}

// TestUpdateClient 更新 LLM 客户端
func TestUpdateClient(t *testing.T) {
	s := &Service{}
	if got := s.getClientSnapshot(); got != nil {
		t.Error("初始 client 应为 nil")
	}
	// 传入 nil 也是合法的（清空）
	s.UpdateClient(nil)
	if got := s.getClientSnapshot(); got != nil {
		t.Error("UpdateClient(nil) 后应为 nil")
	}
}

// TestSetRAGCollection 设置 RAG 集合名
func TestSetRAGCollection(t *testing.T) {
	s := &Service{}
	s.SetRAGCollection("docs")
	s.ragMu.RLock()
	got := s.ragCollection
	s.ragMu.RUnlock()
	if got != "docs" {
		t.Errorf("期望 docs, 实际 %q", got)
	}
}

// TestSetRAGEnabled 启用/禁用 RAG
func TestSetRAGEnabled(t *testing.T) {
	s := &Service{}
	s.SetRAGEnabled(true)
	s.ragMu.RLock()
	enabled := s.ragEnabled
	s.ragMu.RUnlock()
	if !enabled {
		t.Error("应为 true")
	}
	s.SetRAGEnabled(false)
	s.ragMu.RLock()
	enabled = s.ragEnabled
	s.ragMu.RUnlock()
	if enabled {
		t.Error("应为 false")
	}
}

// TestStoreMsgToChat store.Message 转 chat.Message
func TestStoreMsgToChat(t *testing.T) {
	now := time.Now()
	m := &store.Message{
		ID:              "msg-1",
		ConversationID:  "conv-1",
		Role:            "user",
		Content:         "你好",
		ThinkingContent: "思考",
		SearchResults:   "搜索结果",
		Images:          "img1",
		CreatedAt:       now,
	}
	got := storeMsgToChat(m)
	if got.ID != "msg-1" || got.Role != "user" || got.Content != "你好" {
		t.Errorf("基本字段转换错误: %+v", got)
	}
	if got.ThinkingContent != "思考" {
		t.Errorf("ThinkingContent 期望 '思考', 实际 %q", got.ThinkingContent)
	}
	if got.SearchResults != "搜索结果" {
		t.Errorf("SearchResults 期望 '搜索结果', 实际 %q", got.SearchResults)
	}
	if got.CreatedAt != now.Format(time.RFC3339) {
		t.Errorf("CreatedAt 格式错误")
	}
}

// TestStoreMsgToChat_WithAttachments 含附件的转换
func TestStoreMsgToChat_WithAttachments(t *testing.T) {
	atts := []Attachment{{Type: "image", Name: "pic.png", MimeType: "image/png", Data: "abc"}}
	attJSON, _ := json.Marshal(atts)
	m := &store.Message{
		Role:        "user",
		Content:     "看图",
		Attachments: string(attJSON),
	}
	got := storeMsgToChat(m)
	if len(got.Attachments) != 1 {
		t.Fatalf("应有 1 个附件摘要, 实际 %d", len(got.Attachments))
	}
	if got.Attachments[0].Type != "image" || got.Attachments[0].Name != "pic.png" {
		t.Errorf("附件摘要错误: %+v", got.Attachments[0])
	}
}

// TestStoreMsgToChat_InvalidAttachments 无效附件 JSON 不崩溃
func TestStoreMsgToChat_InvalidAttachments(t *testing.T) {
	m := &store.Message{
		Role:        "user",
		Content:     "test",
		Attachments: "not json",
	}
	got := storeMsgToChat(m)
	if len(got.Attachments) != 0 {
		t.Errorf("无效 JSON 应无附件摘要, 实际 %d", len(got.Attachments))
	}
}

// TestStoreMsgToChat_Exported 导出包装函数
func TestStoreMsgToChat_Exported(t *testing.T) {
	m := &store.Message{ID: "x", Role: "assistant", Content: "hi"}
	got := StoreMsgToChat(m)
	if got.ID != "x" || got.Role != "assistant" {
		t.Error("导出包装应与内部函数行为一致")
	}
}

// TestGetDB 导出函数返回 db
func TestGetDB(t *testing.T) {
	s := &Service{}
	if got := GetDB(s); got != nil {
		t.Error("初始 db 应为 nil")
	}
}

// TestSetCurrentCancel 导出函数设置 cancel
func TestSetCurrentCancel(t *testing.T) {
	s := &Service{}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	SetCurrentCancel(s, cancel)
	s.mutex.RLock()
	got := s.currentCancel
	s.mutex.RUnlock()
	if got == nil {
		t.Error("SetCurrentCancel 后应为非 nil")
	}
}

// =============================================================================
// service_methods.go 简单方法
// =============================================================================

// TestGetCurrentCompletionID 读取 completion ID
func TestGetCurrentCompletionID(t *testing.T) {
	s := &Service{}
	if got := s.GetCurrentCompletionID(); got != "" {
		t.Errorf("初始应为空, 实际 %q", got)
	}
	s.setCurrentCompletionID("comp-1")
	if got := s.GetCurrentCompletionID(); got != "comp-1" {
		t.Errorf("期望 comp-1, 实际 %q", got)
	}
}

// TestGetConfig 返回当前配置
func TestGetConfig(t *testing.T) {
	s := &Service{}
	if got := s.GetConfig(); got != nil {
		t.Error("初始 config 应为 nil")
	}
	cfg := &config.Config{ContextSize: 8192}
	s.config = cfg
	if got := s.GetConfig(); got != cfg {
		t.Error("应返回设置的 config")
	}
}

// =============================================================================
// service_model.go 测试
// =============================================================================

// TestGetDetectedModelName 读取检测到的模型名
func TestGetDetectedModelName(t *testing.T) {
	s := &Service{}
	if got := s.GetDetectedModelName(); got != "" {
		t.Errorf("初始应为空, 实际 %q", got)
	}
	s.SetDetectedModelName("qwen3-8b")
	if got := s.GetDetectedModelName(); got != "qwen3-8b" {
		t.Errorf("期望 qwen3-8b, 实际 %q", got)
	}
}

// TestSetDetectedModelName_InvalidatesPromptCache 设置模型名应清空 prompt 缓存
func TestSetDetectedModelName_InvalidatesPromptCache(t *testing.T) {
	s := &Service{}
	// 预填 prompt 缓存
	s.promptMu.Lock()
	s.sysPromptCache = "old"
	s.sysPromptDate = "2024-01-01"
	s.sysPromptConfig = "old"
	s.promptMu.Unlock()
	s.SetDetectedModelName("new-model")
	s.promptMu.RLock()
	cache := s.sysPromptCache
	date := s.sysPromptDate
	conf := s.sysPromptConfig
	s.promptMu.RUnlock()
	if cache != "" || date != "" || conf != "" {
		t.Error("SetDetectedModelName 应清空 prompt 缓存")
	}
}

// TestSetCachedProps 缓存 ServerProps
func TestSetCachedProps(t *testing.T) {
	s := &Service{}
	props := &llm.ServerProps{BuildInfo: "test"}
	s.SetCachedProps(props)
	s.cachedPropsMu.RLock()
	got := s.cachedProps
	s.cachedPropsMu.RUnlock()
	if got != props {
		t.Error("应返回设置的 props")
	}
}

// TestInvalidatePromptCache 清空 prompt 缓存
func TestInvalidatePromptCache(t *testing.T) {
	s := &Service{}
	s.promptMu.Lock()
	s.sysPromptCache = "cache"
	s.sysPromptDate = "2024-01-01"
	s.sysPromptConfig = "cfg"
	s.promptMu.Unlock()
	s.InvalidatePromptCache()
	s.promptMu.RLock()
	cache := s.sysPromptCache
	date := s.sysPromptDate
	conf := s.sysPromptConfig
	s.promptMu.RUnlock()
	if cache != "" || date != "" || conf != "" {
		t.Error("InvalidatePromptCache 应清空所有缓存字段")
	}
}

// TestGetSetModelCapabilities 读写模型能力
func TestGetSetModelCapabilities(t *testing.T) {
	s := &Service{}
	caps := s.GetModelCapabilities()
	if caps.ImageInput {
		t.Error("初始不应有 ImageInput")
	}
	newCaps := llm.ModelCapabilities{ImageInput: true, ToolCallSupport: true}
	s.SetModelCapabilities(newCaps)
	got := s.GetModelCapabilities()
	if !got.ImageInput || !got.ToolCallSupport {
		t.Errorf("设置后应反映新能力: %+v", got)
	}
}

// TestResolveModelPath 解析模型路径
func TestResolveModelPath(t *testing.T) {
	s := &Service{}
	// 空路径
	if got := s.resolveModelPath(""); got != "" {
		t.Errorf("空路径应返回空, 实际 %q", got)
	}
	// 绝对路径（用系统临时目录确保跨平台兼容）
	abs := filepath.Join(os.TempDir(), "model.gguf")
	if got := s.resolveModelPath(abs); got != abs {
		t.Errorf("绝对路径应原样返回, 期望 %q, 实际 %q", abs, got)
	}
	// 相对路径 + appDir
	s.appDir = filepath.Join(os.TempDir(), "app")
	want := filepath.Join(s.appDir, "models", "qwen.gguf")
	if got := s.resolveModelPath(filepath.Join("models", "qwen.gguf")); got != want {
		t.Errorf("相对路径应拼接 appDir, 期望 %q, 实际 %q", want, got)
	}
	// 相对路径 + 空 appDir
	s.appDir = ""
	rel := filepath.Join("model.gguf")
	if got := s.resolveModelPath(rel); got != rel {
		t.Errorf("空 appDir 时应原样返回, 期望 %q, 实际 %q", rel, got)
	}
}

// TestGetThinkingSoftSwitch_NoConfig 无配置返回 auto
func TestGetThinkingSoftSwitch_NoConfig(t *testing.T) {
	s := &Service{}
	if got := s.GetThinkingSoftSwitch(); got != "auto" {
		t.Errorf("无配置应返回 auto, 实际 %q", got)
	}
}

// TestGetThinkingSoftSwitch_WithConfig 各 Reasoning 值映射
func TestGetThinkingSoftSwitch_WithConfig(t *testing.T) {
	cases := []struct {
		reasoning string
		want      string
	}{
		{"on", "think"},
		{"off", "no_think"},
		{"auto", "auto"},
		{"", "auto"},
		{"unknown", "auto"},
	}
	for _, tc := range cases {
		s := &Service{config: &config.Config{Reasoning: tc.reasoning}}
		if got := s.GetThinkingSoftSwitch(); got != tc.want {
			t.Errorf("Reasoning=%q 期望 %q, 实际 %q", tc.reasoning, tc.want, got)
		}
	}
}

// TestModelNameForRequest 请求用的模型名
func TestModelNameForRequest(t *testing.T) {
	s := &Service{}
	if got := s.modelNameForRequest(); got != "default" {
		t.Errorf("空名应返回 default, 实际 %q", got)
	}
	s.detectedModelName = "qwen3-8b"
	if got := s.modelNameForRequest(); got != "qwen3-8b" {
		t.Errorf("期望 qwen3-8b, 实际 %q", got)
	}
}

// TestApplyThinkingControl_NoneMode 无思考模式不改请求
func TestApplyThinkingControl_NoneMode(t *testing.T) {
	s := &Service{}
	// 显式设置 ThinkingModeNone（零值空字符串不等于 "none"）
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeNone}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if req.ReasoningControl {
		t.Error("NoneMode 不应启用 ReasoningControl")
	}
}

// TestApplyThinkingControl_TemplateMode 模板思考模式
func TestApplyThinkingControl_TemplateMode(t *testing.T) {
	s := &Service{
		config: &config.Config{ReasoningBudget: 1024, Reasoning: "on"},
	}
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeTemplate}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if !req.ReasoningControl {
		t.Error("TemplateMode 应启用 ReasoningControl")
	}
	if req.ReasoningBudget != 1024 {
		t.Errorf("ReasoningBudget 期望 1024, 实际 %d", req.ReasoningBudget)
	}
	if req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs["enable_thinking"] != true {
		t.Error("TemplateMode + Reasoning=on 应设置 enable_thinking=true")
	}
}

// TestApplyThinkingControl_TemplateMode_Off Reasoning=off 时 enable_thinking=false
func TestApplyThinkingControl_TemplateMode_Off(t *testing.T) {
	s := &Service{
		config: &config.Config{Reasoning: "off"},
	}
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeTemplate}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if req.ChatTemplateKwargs["enable_thinking"] != false {
		t.Error("Reasoning=off 时 enable_thinking 应为 false")
	}
	// M-TO: reasoning=off 同时写 per-request reasoning_effort=none（llama.cpp #26045 逃逸口）
	if req.ReasoningEffort != "none" {
		t.Errorf("Reasoning=off 时 ReasoningEffort 应为 none，实际 %q", req.ReasoningEffort)
	}
}

// TestApplyThinkingControl_TemplateMode_Auto Reasoning=auto 时不应强制设置 enable_thinking。
// auto 应交给 llama-server --reasoning auto 与模板默认行为决定（模型自主），
// 从而让 Qwen3.5 等小模型按模板默认插入空思考块、简单问题不再被强制思考。
func TestApplyThinkingControl_TemplateMode_Auto(t *testing.T) {
	for _, reasoning := range []string{"auto", ""} {
		s := &Service{
			config: &config.Config{Reasoning: reasoning},
		}
		s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeTemplate}
		req := &llm.ChatCompletionRequest{}
		s.applyThinkingControl(req)
		if v, ok := req.ChatTemplateKwargs["enable_thinking"]; ok {
			t.Errorf("Reasoning=%q 时不应显式设置 enable_thinking，实际 %v", reasoning, v)
		}
	}
}

// TestApplyThinkingControl_TemplateMode_Auto_RespectModelDefault 验证 auto 模式下尊重模型自身行为：
// 即使模板默认不自主思考（DefaultThinkingAuto=="off"，如 Gemma），也不显式设置 enable_thinking，
// 避免简单问候与简单问题被强制长时间思考。
func TestApplyThinkingControl_TemplateMode_Auto_RespectModelDefault(t *testing.T) {
	for _, reasoning := range []string{"auto", ""} {
		s := &Service{
			config: &config.Config{Reasoning: reasoning},
		}
		s.modelCaps = llm.ModelCapabilities{
			ThinkingMode:        llm.ThinkingModeTemplate,
			DefaultThinkingAuto: "off",
		}
		req := &llm.ChatCompletionRequest{}
		s.applyThinkingControl(req)
		if v, ok := req.ChatTemplateKwargs["enable_thinking"]; ok {
			t.Errorf("Reasoning=%q 且默认不自主思考时，也应尊重模型默认，不设置 enable_thinking，实际 %v", reasoning, v)
		}
	}
}

// TestApplyThinkingControl_TemplateMode_Auto_DefaultOn 验证 auto 模式下，
// 对模板默认自主思考（DefaultThinkingAuto=="on"，如 Qwen）的模型，不显式设置 enable_thinking。
func TestApplyThinkingControl_TemplateMode_Auto_DefaultOn(t *testing.T) {
	s := &Service{
		config: &config.Config{Reasoning: "auto"},
	}
	s.modelCaps = llm.ModelCapabilities{
		ThinkingMode:        llm.ThinkingModeTemplate,
		DefaultThinkingAuto: "on",
	}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if v, ok := req.ChatTemplateKwargs["enable_thinking"]; ok {
		t.Errorf("默认自主思考为 on 时不应显式设置 enable_thinking，实际 %v", v)
	}
}

// TestApplyThinkingControl_ReasoningEffort_On 推理开启（非 off）时不发送 reasoning_effort=none
func TestApplyThinkingControl_ReasoningEffort_On(t *testing.T) {
	s := &Service{
		config: &config.Config{Reasoning: "on"},
	}
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeReasoning}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if req.ReasoningEffort == "none" {
		t.Error("Reasoning=on 时不应发送 reasoning_effort=none")
	}
}

// TestApplyThinkingControl_ReasoningEffort_Forward 思考开启且配置了思考强度时，
// 通过 chat_template_kwargs.reasoning_effort 透传给模板
func TestApplyThinkingControl_ReasoningEffort_Forward(t *testing.T) {
	s := &Service{
		config: &config.Config{Reasoning: "on", ReasoningEffort: "high"},
	}
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeTemplate}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs["reasoning_effort"] != "high" {
		t.Errorf("应透传 reasoning_effort=high 到 chat_template_kwargs，实际 %v", req.ChatTemplateKwargs)
	}
	if req.ReasoningEffort == "none" {
		t.Error("Reasoning=on + 设置思考强度时不应发送 reasoning_effort=none")
	}
}

// TestApplyThinkingControl_ReasoningEffort_ForwardReasoningMode
// Reasoning 模式（DeepSeek）下同样透传思考强度，且不影响 enable_thinking
func TestApplyThinkingControl_ReasoningEffort_ForwardReasoningMode(t *testing.T) {
	s := &Service{
		config: &config.Config{Reasoning: "on", ReasoningEffort: "max"},
	}
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeReasoning}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if req.ChatTemplateKwargs == nil || req.ChatTemplateKwargs["reasoning_effort"] != "max" {
		t.Errorf("Reasoning 模式应透传 reasoning_effort=max，实际 %v", req.ChatTemplateKwargs)
	}
}

// TestApplyThinkingControl_ReasoningEffort_Off 思考关闭时不透传思考强度
func TestApplyThinkingControl_ReasoningEffort_Off(t *testing.T) {
	s := &Service{
		config: &config.Config{Reasoning: "off", ReasoningEffort: "high"},
	}
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeTemplate}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if req.ChatTemplateKwargs["reasoning_effort"] != nil {
		t.Error("Reasoning=off 时不应透传 reasoning_effort")
	}
	if req.ReasoningEffort != "none" {
		t.Errorf("Reasoning=off 时 ReasoningEffort 应为 none，实际 %q", req.ReasoningEffort)
	}
}

// TestApplyThinkingControl_ReasoningEffort_Empty 未配置思考强度时不写入 kwargs
func TestApplyThinkingControl_ReasoningEffort_Empty(t *testing.T) {
	s := &Service{
		config: &config.Config{Reasoning: "on", ReasoningEffort: ""},
	}
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeTemplate}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if req.ChatTemplateKwargs["reasoning_effort"] != nil {
		t.Error("ReasoningEffort 为空时不应写入 kwargs")
	}
}

// TestApplyThinkingControl_ReasoningMode 推理模式（DeepSeek）
func TestApplyThinkingControl_ReasoningMode(t *testing.T) {
	s := &Service{
		config: &config.Config{ReasoningBudget: 2048},
	}
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeReasoning}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if !req.ReasoningControl {
		t.Error("ReasoningMode 应启用 ReasoningControl")
	}
	if req.ReasoningBudget != 2048 {
		t.Errorf("ReasoningBudget 期望 2048, 实际 %d", req.ReasoningBudget)
	}
	// ReasoningMode 不设置 ChatTemplateKwargs
	if req.ChatTemplateKwargs != nil {
		t.Error("ReasoningMode 不应设置 ChatTemplateKwargs")
	}
}

// TestApplyThinkingControl_BudgetTags 预算标签传递
func TestApplyThinkingControl_BudgetTags(t *testing.T) {
	s := &Service{
		config: &config.Config{
			ReasoningBudgetStartTag: "<think>",
			ReasoningBudgetEndTag:   "</think>",
		},
	}
	s.modelCaps = llm.ModelCapabilities{ThinkingMode: llm.ThinkingModeTemplate}
	req := &llm.ChatCompletionRequest{}
	s.applyThinkingControl(req)
	if req.ReasoningBudgetStartTag != "<think>" {
		t.Errorf("StartTag 期望 <think>, 实际 %q", req.ReasoningBudgetStartTag)
	}
	if req.ReasoningBudgetEndTag != "</think>" {
		t.Errorf("EndTag 期望 </think>, 实际 %q", req.ReasoningBudgetEndTag)
	}
}

// TestApplySamplingParams_NoConfig 无配置不修改请求
func TestApplySamplingParams_NoConfig(t *testing.T) {
	s := &Service{}
	req := &llm.ChatCompletionRequest{}
	s.applySamplingParams(req)
	if req.Samplers != nil || req.IgnoreEos || req.Verbose {
		t.Error("无配置不应修改请求")
	}
}

// TestApplySamplingParams_WithConfig 各采样参数传递
func TestApplySamplingParams_WithConfig(t *testing.T) {
	s := &Service{
		config: &config.Config{
			Samplers:       "top_k, top_p, temperature",
			IgnoreEos:      true,
			Verbose:        true,
			AdaptiveTarget: 0.5,
			AdaptiveDecay:  0.9,
		},
	}
	req := &llm.ChatCompletionRequest{}
	s.applySamplingParams(req)
	if len(req.Samplers) != 3 || req.Samplers[0] != "top_k" {
		t.Errorf("Samplers 解析错误: %v", req.Samplers)
	}
	if !req.IgnoreEos {
		t.Error("IgnoreEos 应为 true")
	}
	if !req.Verbose {
		t.Error("Verbose 应为 true")
	}
	if req.AdaptiveTarget != 0.5 {
		t.Errorf("AdaptiveTarget 期望 0.5, 实际 %v", req.AdaptiveTarget)
	}
	if req.AdaptiveDecay != 0.9 {
		t.Errorf("AdaptiveDecay 期望 0.9, 实际 %v", req.AdaptiveDecay)
	}
}

// TestDetectToolCallFromGGUF_NoConfig 无配置返回 false
func TestDetectToolCallFromGGUF_NoConfig(t *testing.T) {
	s := &Service{}
	if s.detectToolCallFromGGUF() {
		t.Error("无配置应返回 false")
	}
}

// TestDetectHasMTP_NoConfig 无配置返回 false
func TestDetectHasMTP_NoConfig(t *testing.T) {
	s := &Service{}
	if s.detectHasMTP() {
		t.Error("无配置应返回 false")
	}
}

// TestResolveNParams_NoConfig 无配置返回 0
func TestResolveNParams_NoConfig(t *testing.T) {
	s := &Service{}
	if got := s.resolveNParams(0); got != 0 {
		t.Errorf("无配置应返回 0, 实际 %f", got)
	}
}

// TestResolveNParams_ServerPositive 服务端返回正值时直接用
func TestResolveNParams_ServerPositive(t *testing.T) {
	s := &Service{}
	if got := s.resolveNParams(7000); got != 7000 {
		t.Errorf("服务端正值应直接返回, 实际 %f", got)
	}
}

// =============================================================================
// service_stream.go 测试
// =============================================================================

// TestTruncateForLog 短字符串不截断
func TestTruncateForLog_Short(t *testing.T) {
	got := truncateForLog("short", 100)
	if got != "short" {
		t.Errorf("短字符串应原样返回, 实际 %q", got)
	}
}

// TestTruncateForLog_Exact 长度等于 maxLen 不截断
func TestTruncateForLog_Exact(t *testing.T) {
	s := strings.Repeat("x", 50)
	got := truncateForLog(s, 50)
	if got != s {
		t.Errorf("等于 maxLen 应原样返回")
	}
}

// TestTruncateForLog_Long 长字符串截断
func TestTruncateForLog_Long(t *testing.T) {
	s := strings.Repeat("x", 200)
	got := truncateForLog(s, 50)
	if !strings.HasSuffix(got, "...(truncated)") {
		t.Errorf("长字符串应以 ...(truncated) 结尾, 实际 %q", got)
	}
	if len(got) > 70 {
		t.Errorf("截断后长度应合理, 实际 %d", len(got))
	}
}

// TestBuildAvailableTools_NoSearch 不含 search 工具
func TestBuildAvailableTools_NoSearch(t *testing.T) {
	s := &Service{}
	tools := s.buildAvailableTools(false)
	if len(tools) != 0 {
		t.Errorf("无 MCP 缓存且不含 search 应返回空, 实际 %d 个", len(tools))
	}
}

// TestBuildAvailableTools_WithSearch 含 search 工具
func TestBuildAvailableTools_WithSearch(t *testing.T) {
	s := &Service{}
	tools := s.buildAvailableTools(true)
	if len(tools) != 1 {
		t.Fatalf("应只有 1 个 search 工具, 实际 %d", len(tools))
	}
	if tools[0].Function.Name != "search" {
		t.Errorf("工具名应为 search, 实际 %q", tools[0].Function.Name)
	}
}

// TestBuildAvailableTools_WithMCP 含 MCP 缓存工具
func TestBuildAvailableTools_WithMCP(t *testing.T) {
	s := &Service{}
	s.SetMcpToolsCache([]llm.ToolDefinition{
		{Type: "function", Function: llm.FunctionDef{Name: "weather"}},
	})
	tools := s.buildAvailableTools(true)
	if len(tools) != 2 {
		t.Errorf("应有 search + weather 共 2 个, 实际 %d", len(tools))
	}
}

// TestSetMcpToolsCache_Nil 清空缓存
func TestSetMcpToolsCache_Nil(t *testing.T) {
	s := &Service{}
	s.SetMcpToolsCache([]llm.ToolDefinition{{Function: llm.FunctionDef{Name: "x"}}})
	s.SetMcpToolsCache(nil)
	if s.mcpToolsInitialized {
		t.Error("nil 应标记为未初始化")
	}
}

// TestSetMcpToolsCache_EmptySlice 空切片标记为已初始化
func TestSetMcpToolsCache_EmptySlice(t *testing.T) {
	s := &Service{}
	s.SetMcpToolsCache([]llm.ToolDefinition{})
	if !s.mcpToolsInitialized {
		t.Error("空切片应标记为已初始化")
	}
	tools := s.GetCachedMcpTools()
	if len(tools) != 0 {
		t.Errorf("空切片应返回空列表, 实际 %d", len(tools))
	}
}

// TestGetCachedMcpTools_ReturnsCopy 返回副本不影响原缓存
func TestGetCachedMcpTools_ReturnsCopy(t *testing.T) {
	s := &Service{}
	s.SetMcpToolsCache([]llm.ToolDefinition{{Function: llm.FunctionDef{Name: "a"}}})
	got := s.GetCachedMcpTools()
	got[0].Function.Name = "modified"
	// 原缓存不应被修改
	orig := s.GetCachedMcpTools()
	if orig[0].Function.Name != "a" {
		t.Error("GetCachedMcpTools 应返回副本, 修改副本不应影响原缓存")
	}
}

// TestNewStreamAccumulator 构造函数初始化
func TestNewStreamAccumulator(t *testing.T) {
	emitFn := func(string, any) {}
	emitForConvFn := func(string, string, any) {}
	acc := NewStreamAccumulator("conv-1", emitFn, emitForConvFn)
	if acc.ConvID != "conv-1" {
		t.Errorf("ConvID 期望 conv-1, 实际 %q", acc.ConvID)
	}
	if acc.EmitFn == nil || acc.EmitForConvFn == nil {
		t.Error("回调函数不应为 nil")
	}
	if acc.ToolCallMap == nil {
		t.Error("ToolCallMap 应已初始化")
	}
}

// TestStreamAccumulator_ToolCallsEmpty 空 tool call map 返回 nil
func TestStreamAccumulator_ToolCallsEmpty(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	if got := acc.toolCalls(); got != nil {
		t.Errorf("空 map 应返回 nil, 实际 %v", got)
	}
}

// TestStreamAccumulator_ToolCallsSorted 按 Index 升序
func TestStreamAccumulator_ToolCallsSorted(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	acc.ToolCallMap[2] = &llm.ToolCall{Index: 2, ID: "b"}
	acc.ToolCallMap[0] = &llm.ToolCall{Index: 0, ID: "a"}
	acc.ToolCallMap[1] = &llm.ToolCall{Index: 1, ID: "c"}
	got := acc.toolCalls()
	if len(got) != 3 {
		t.Fatalf("应有 3 个, 实际 %d", len(got))
	}
	if got[0].ID != "a" || got[1].ID != "c" || got[2].ID != "b" {
		t.Errorf("应按 Index 升序: %v", got)
	}
}

// TestStreamAccumulator_EmitTokenBatched 首 token 立即发送
func TestStreamAccumulator_EmitTokenBatched_FirstToken(t *testing.T) {
	var emitted string
	acc := NewStreamAccumulator("c", nil, func(_, _ string, content any) {
		emitted = content.(string)
	})
	acc.emitTokenBatched("hello")
	if emitted != "hello" {
		t.Errorf("首 token 应立即发送, 实际 %q", emitted)
	}
	if !acc.FirstTokenSent {
		t.Error("FirstTokenSent 应为 true")
	}
}

// TestStreamAccumulator_EmitTokenBatched_Batch 后续累积
func TestStreamAccumulator_EmitTokenBatched_Batch(t *testing.T) {
	var count int
	acc := NewStreamAccumulator("c", nil, func(_, _ string, _ any) {
		count++
	})
	acc.emitTokenBatched("first") // 首 token，count=1
	// 后续小段不触发
	acc.emitTokenBatched("a")
	acc.emitTokenBatched("b")
	if count != 1 {
		t.Errorf("小段累积不应触发, 实际触发 %d 次", count)
	}
}

// TestStreamAccumulator_FlushTokenBuffer flush 残留缓冲
func TestStreamAccumulator_FlushTokenBuffer(t *testing.T) {
	var emitted string
	acc := NewStreamAccumulator("c", nil, func(_, _ string, content any) {
		emitted = content.(string)
	})
	acc.FirstTokenSent = true
	acc.TokenBuf.WriteString("残留")
	acc.flushTokenBuffer()
	if emitted != "残留" {
		t.Errorf("应 flush 残留内容, 实际 %q", emitted)
	}
	if acc.TokenBuf.Len() != 0 {
		t.Error("flush 后 TokenBuf 应为空")
	}
}

// TestStreamAccumulator_FlushTokenBuffer_Empty 空缓冲不触发
func TestStreamAccumulator_FlushTokenBuffer_Empty(t *testing.T) {
	var called bool
	acc := NewStreamAccumulator("c", nil, func(_, _ string, _ any) {
		called = true
	})
	acc.flushTokenBuffer()
	if called {
		t.Error("空缓冲不应触发回调")
	}
}

// TestStreamAccumulator_ResetForNextCall 重置状态
func TestStreamAccumulator_ResetForNextCall(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	acc.FullContent.WriteString("content")
	acc.FullThinking.WriteString("thinking")
	acc.FinishReason = "stop"
	acc.ToolCallMap[0] = &llm.ToolCall{}
	acc.FirstTokenSent = true
	acc.resetForNextCall()
	if acc.FullContent.Len() != 0 {
		t.Error("FullContent 应已清空")
	}
	if acc.FinishReason != "" {
		t.Error("FinishReason 应已清空")
	}
	if len(acc.ToolCallMap) != 0 {
		t.Error("ToolCallMap 应已清空")
	}
	if acc.FirstTokenSent {
		t.Error("FirstTokenSent 应重置为 false")
	}
	// FirstRoundThinking 应在 reset 时保存
	if acc.FirstRoundThinking != "thinking" {
		t.Errorf("FirstRoundThinking 应保存 'thinking', 实际 %q", acc.FirstRoundThinking)
	}
}

// TestStreamAccumulator_ResetForNextCall_PreserveFirstRound 第二次 reset 不覆盖
func TestStreamAccumulator_ResetForNextCall_PreserveFirstRound(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	acc.FullThinking.WriteString("first")
	acc.resetForNextCall()
	acc.FullThinking.Reset()
	acc.FullThinking.WriteString("second")
	acc.resetForNextCall()
	if acc.FirstRoundThinking != "first" {
		t.Errorf("FirstRoundThinking 不应被覆盖, 实际 %q", acc.FirstRoundThinking)
	}
}

// TestResetForNextCall_Exported 导出包装
func TestResetForNextCall_Exported(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	acc.FullContent.WriteString("x")
	ResetForNextCall(acc)
	if acc.FullContent.Len() != 0 {
		t.Error("导出包装应正确重置")
	}
}

// TestGetFirstRoundThinking_Exported 导出包装
func TestGetFirstRoundThinking_Exported(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	acc.FirstRoundThinking = "round1"
	if got := GetFirstRoundThinking(acc); got != "round1" {
		t.Errorf("期望 round1, 实际 %q", got)
	}
}

// TestClampDuration_Exported 导出包装
func TestClampDuration_Exported(t *testing.T) {
	if got := ClampDuration(50.5); got != 50.5 {
		t.Errorf("正常值应原样返回, 实际 %f", got)
	}
	if got := ClampDuration(-1); got != 0 {
		t.Errorf("负值应返回 0, 实际 %f", got)
	}
	if got := ClampDuration(4000); got != 0 {
		t.Errorf("超过 3600 应返回 0, 实际 %f", got)
	}
}

// TestCalcMaxTokens_Exported 导出包装
func TestCalcMaxTokens_Exported(t *testing.T) {
	s := &Service{}
	// 无配置，ctxSize 默认 4096, prompt=1000 → max(3096, 512) = 3096
	if got := CalcMaxTokens(s, 1000); got != 3096 {
		t.Errorf("期望 3096, 实际 %d", got)
	}
	// prompt 很大，min 部分触发
	if got := CalcMaxTokens(s, 10000); got != 512 {
		t.Errorf("期望 512, 实际 %d", got)
	}
}

// TestCalcMaxTokens_WithConfig 有配置时
func TestCalcMaxTokens_WithConfig(t *testing.T) {
	s := &Service{config: &config.Config{ContextSize: 8192}}
	// 8192-1000=7192, min(7192,16384)=7192, max(7192,512)=7192
	if got := s.calcMaxTokens(1000); got != 7192 {
		t.Errorf("期望 7192, 实际 %d", got)
	}
}

// =============================================================================
// service_tool_call_loop.go updateTokenCount 测试
// =============================================================================

// TestUpdateTokenCount_NoNewMessages 无新增消息不累加
func TestUpdateTokenCount_NoNewMessages(t *testing.T) {
	st := &toolCallLoopState{totalTokens: 100}
	msgs := []llm.ChatMessage{{Role: "user", Content: "hi"}}
	st.updateTokenCount(msgs, 1) // prevMsgCount=1, 无新增
	if st.totalTokens != 100 {
		t.Errorf("无新增不应累加, 实际 %d", st.totalTokens)
	}
}

// TestUpdateTokenCount_NewMessages 有新增消息累加
func TestUpdateTokenCount_NewMessages(t *testing.T) {
	st := &toolCallLoopState{totalTokens: 0}
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "问题"},
		{Role: "assistant", Content: "回答"},
	}
	st.updateTokenCount(msgs, 0) // 全部新增
	if st.totalTokens <= 0 {
		t.Errorf("应累加 token, 实际 %d", st.totalTokens)
	}
}

// TestUpdateTokenCount_PartialNew 部分新增
func TestUpdateTokenCount_PartialNew(t *testing.T) {
	st := &toolCallLoopState{totalTokens: 100}
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "旧"},
		{Role: "assistant", Content: "旧答"},
		{Role: "user", Content: "新问题"},
	}
	st.updateTokenCount(msgs, 2) // 只累加索引 2
	if st.totalTokens <= 100 {
		t.Errorf("应累加新增消息 token, 实际 %d", st.totalTokens)
	}
}

// =============================================================================
// service_messages.go 纯函数测试
// =============================================================================

// TestGetOrDecodeBase64_FromCache 从缓存获取
func TestGetOrDecodeBase64_FromCache(t *testing.T) {
	cache := map[int][]byte{0: []byte("cached")}
	got, err := getOrDecodeBase64(cache, 0, "ignored")
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if string(got) != "cached" {
		t.Errorf("应从缓存返回, 实际 %q", string(got))
	}
}

// TestGetOrDecodeBase64_Decode 无缓存时解码
func TestGetOrDecodeBase64_Decode(t *testing.T) {
	cache := map[int][]byte{}
	// "hello" 的 base64
	got, err := getOrDecodeBase64(cache, 1, "aGVsbG8=")
	if err != nil {
		t.Fatalf("解码失败: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("期望 hello, 实际 %q", string(got))
	}
	// 应写入缓存
	if string(cache[1]) != "hello" {
		t.Error("解码后应写入缓存")
	}
}

// TestGetOrDecodeBase64_NilCache nil 缓存也能解码
func TestGetOrDecodeBase64_NilCache(t *testing.T) {
	got, err := getOrDecodeBase64(nil, 0, "dGVzdA==")
	if err != nil {
		t.Fatalf("nil 缓存应能解码: %v", err)
	}
	if string(got) != "test" {
		t.Errorf("期望 test, 实际 %q", string(got))
	}
}

// TestGetOrDecodeBase64_InvalidBase64 无效 base64 报错
func TestGetOrDecodeBase64_InvalidBase64(t *testing.T) {
	_, err := getOrDecodeBase64(nil, 0, "not valid base64!!!")
	if err == nil {
		t.Error("无效 base64 应报错")
	}
}

// TestCleanHistoryMessages_Empty 空列表
func TestCleanHistoryMessages_Empty(t *testing.T) {
	got := cleanHistoryMessages(nil)
	if len(got) != 0 {
		t.Errorf("空列表应返回空, 实际 %d", len(got))
	}
}

// TestCleanHistoryMessages_LeadingNonUser 开头非 user/system 被砍
func TestCleanHistoryMessages_LeadingNonUser(t *testing.T) {
	history := []llm.ChatMessage{
		{Role: "assistant", Content: "孤儿回复"},
		{Role: "user", Content: "问题"},
		{Role: "assistant", Content: "回答"},
	}
	got := cleanHistoryMessages(history)
	if len(got) != 2 {
		t.Fatalf("应砍掉开头 assistant, 保留 2 条, 实际 %d", len(got))
	}
	if got[0].Role != "user" {
		t.Errorf("第一条应为 user, 实际 %q", got[0].Role)
	}
}

// TestCleanHistoryMessages_MergeAssistant 合并连续 assistant
func TestCleanHistoryMessages_MergeAssistant(t *testing.T) {
	history := []llm.ChatMessage{
		{Role: "user", Content: "问题"},
		{Role: "assistant", Content: "回答1"},
		{Role: "assistant", Content: "回答2"},
		{Role: "assistant", Content: "回答3"},
	}
	got := cleanHistoryMessages(history)
	if len(got) != 2 {
		t.Fatalf("应合并为 2 条, 实际 %d", len(got))
	}
	if got[1].Content != "回答3" {
		t.Errorf("合并后应保留最后一条, 实际 %q", got[1].Content)
	}
}

// TestCleanHistoryMessages_ToolWithoutAssistant 孤儿 tool 消息被丢弃
func TestCleanHistoryMessages_ToolWithoutAssistant(t *testing.T) {
	history := []llm.ChatMessage{
		{Role: "user", Content: "问题"},
		{Role: "tool", Content: "孤儿结果", ToolCallID: "call-1"},
		{Role: "assistant", Content: "回答"},
	}
	got := cleanHistoryMessages(history)
	// tool 前面没有带 ToolCalls 的 assistant，应被丢弃
	for _, m := range got {
		if m.Role == "tool" {
			t.Error("孤儿 tool 消息应被丢弃")
		}
	}
}

// TestCleanHistoryMessages_AssistantWithToolCallsButNoTool 带 ToolCalls 但无后续 tool 的 assistant
func TestCleanHistoryMessages_AssistantWithToolCallsButNoTool(t *testing.T) {
	history := []llm.ChatMessage{
		{Role: "user", Content: "问题"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call-1", Function: llm.FunctionCall{Name: "search"}}}},
		{Role: "assistant", Content: "最终回答"},
	}
	got := cleanHistoryMessages(history)
	// 带 ToolCalls 的 assistant 没有匹配的后续 tool，应被丢弃
	for _, m := range got {
		if len(m.ToolCalls) > 0 {
			t.Error("无匹配 tool 的 assistant(ToolCalls) 应被丢弃")
		}
	}
}

// TestCleanHistoryMessages_ValidToolCallPair 有效的 tool call 对保留
func TestCleanHistoryMessages_ValidToolCallPair(t *testing.T) {
	history := []llm.ChatMessage{
		{Role: "user", Content: "问题"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call-1", Function: llm.FunctionCall{Name: "search"}}}},
		{Role: "tool", Content: "结果", ToolCallID: "call-1"},
		{Role: "assistant", Content: "最终回答"},
	}
	got := cleanHistoryMessages(history)
	if len(got) != 4 {
		t.Errorf("有效 tool call 对应全部保留, 实际 %d 条", len(got))
	}
}

// =============================================================================
// service_slot.go 测试（cfg nil 分支）
// =============================================================================

// TestTryRestoreSlot_NoConfig 无配置直接返回
func TestTryRestoreSlot_NoConfig(t *testing.T) {
	s := &Service{}
	// 不应 panic
	s.tryRestoreSlot(context.Background(), "conv-1")
}

// TestTryRestoreSlot_Disabled 配置禁用 SlotSave
func TestTryRestoreSlot_Disabled(t *testing.T) {
	s := &Service{config: &config.Config{SlotSaveEnabled: false}}
	s.tryRestoreSlot(context.Background(), "conv-1")
	// 无 client，应直接返回不报错
}

// TestTrySaveSlot_NoConfig 无配置直接返回
func TestTrySaveSlot_NoConfig(t *testing.T) {
	s := &Service{}
	s.trySaveSlot(context.Background(), "conv-1")
}

// TestTrySaveSlot_Disabled 配置禁用
func TestTrySaveSlot_Disabled(t *testing.T) {
	s := &Service{config: &config.Config{SlotSaveEnabled: false}}
	s.trySaveSlot(context.Background(), "conv-1")
}

// TestClearSavedSlot_NoConfig 无配置直接返回
func TestClearSavedSlot_NoConfig(t *testing.T) {
	s := &Service{}
	s.ClearSavedSlot(context.Background())
}

// TestClearSavedSlot_EmptyOldID 无旧 ID 跳过 erase
func TestClearSavedSlot_EmptyOldID(t *testing.T) {
	s := &Service{config: &config.Config{SlotSaveEnabled: true}}
	// lastSavedConvID 为空，应跳过 erase 不报错
	s.ClearSavedSlot(context.Background())
}

// TestClearSavedSlot_WithOldID 有旧 ID 但无 client 跳过
func TestClearSavedSlot_WithOldID(t *testing.T) {
	s := &Service{config: &config.Config{SlotSaveEnabled: true}}
	s.lastSavedSlotMu.Lock()
	s.lastSavedConvID = "old-conv"
	s.lastSavedSlotMu.Unlock()
	// 无 client，应跳过 erase 不报错
	s.ClearSavedSlot(context.Background())
	// 应清空 lastSavedConvID
	s.lastSavedSlotMu.RLock()
	got := s.lastSavedConvID
	s.lastSavedSlotMu.RUnlock()
	if got != "" {
		t.Error("应清空 lastSavedConvID")
	}
}

// =============================================================================
// types.go DecodeString 补全分支
// =============================================================================

// TestDecodeString_StringContent 字符串内容
func TestDecodeString_StringContent(t *testing.T) {
	e := &StreamEvent{Type: "token", Content: "hello"}
	got, err := e.DecodeString()
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	if got != "hello" {
		t.Errorf("期望 hello, 实际 %q", got)
	}
}

// TestDecodeString_NonStringContent 非字符串内容走 JSON 往返
func TestDecodeString_NonStringContent(t *testing.T) {
	// []byte 内容会走 JSON 往返
	e := &StreamEvent{Type: "token", Content: []byte("from bytes")}
	got, err := e.DecodeString()
	if err != nil {
		t.Fatalf("不应报错: %v", err)
	}
	// []byte 经 JSON marshal 会变成 base64 字符串
	if got == "" {
		t.Error("非字符串内容解码后不应为空")
	}
}

// TestDecodeString_NilContent nil 内容
func TestDecodeString_NilContent(t *testing.T) {
	e := &StreamEvent{Type: "token", Content: nil}
	got, err := e.DecodeString()
	if err != nil {
		t.Fatalf("nil 内容不应报错: %v", err)
	}
	if got != "" {
		t.Errorf("nil 内容应返回空字符串, 实际 %q", got)
	}
}

// TestDecodeContent_Error 不可序列化的内容报错
func TestDecodeContent_Error(t *testing.T) {
	// channel 不可 JSON 序列化
	ch := make(chan int)
	var target string
	if err := decodeContent(ch, &target); err == nil {
		t.Error("channel 应导致序列化错误")
	}
}

// =============================================================================
// service.go beginGeneration 测试（0% → 覆盖）
// beginGeneration 统一处理"开始新一轮生成"的锁与取消逻辑。
// 生活类比：调度中心接新单时的标准流程——记录旧单、派发新单号、通知旧单停止、清理台面。
// =============================================================================

// TestBeginGeneration_NoOldCancel 无旧 cancel 时直接创建新 ctx
func TestBeginGeneration_NoOldCancel(t *testing.T) {
	s := &Service{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelCtx, cleanup := s.beginGeneration(ctx, "conv-1")
	defer cleanup()
	if cancelCtx == nil {
		t.Fatal("cancelCtx 不应为 nil")
	}
	if got := s.CurrentConvID(); got != "conv-1" {
		t.Errorf("currentConvID 期望 conv-1, 实际 %q", got)
	}
}

// TestBeginGeneration_WithOldCancel 有旧 cancel 时应取消旧 ctx
func TestBeginGeneration_WithOldCancel(t *testing.T) {
	s := &Service{}
	// 设置旧 cancel
	oldCtx, oldCancel := context.WithCancel(context.Background())
	s.mutex.Lock()
	s.currentCancel = oldCancel
	s.currentConvID = "old-conv"
	s.mutex.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelCtx, cleanup := s.beginGeneration(ctx, "new-conv")
	defer cleanup()

	// 旧 ctx 应被取消
	select {
	case <-oldCtx.Done():
		// 正确，旧 ctx 已取消
	case <-time.After(100 * time.Millisecond):
		t.Error("旧 ctx 应被取消")
	}
	if cancelCtx == nil {
		t.Fatal("cancelCtx 不应为 nil")
	}
}

// TestBeginGeneration_Cleanup 清理函数清空状态
func TestBeginGeneration_Cleanup(t *testing.T) {
	s := &Service{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, cleanup := s.beginGeneration(ctx, "conv-1")
	cleanup()
	if got := s.CurrentConvID(); got != "" {
		t.Errorf("cleanup 后 currentConvID 应为空, 实际 %q", got)
	}
	s.mutex.RLock()
	cancelFn := s.currentCancel
	s.mutex.RUnlock()
	if cancelFn != nil {
		t.Error("cleanup 后 currentCancel 应为 nil")
	}
}

// TestBeginGeneration_EmptyInitialConvID 空初始 convID
func TestBeginGeneration_EmptyInitialConvID(t *testing.T) {
	s := &Service{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, cleanup := s.beginGeneration(ctx, "")
	defer cleanup()
	if got := s.CurrentConvID(); got != "" {
		t.Errorf("空初始 convID 时 currentConvID 应为空, 实际 %q", got)
	}
}

// TestBeginGeneration_OldCleanupKeepsNewSlot 竞态修复：旧代 cleanup 不得误清新一代槽位。
// 场景：A 生成中用户发起 B；A 被取消后于收尾时执行 cleanup，此时槽位应仍属于 B。
func TestBeginGeneration_OldCleanupKeepsNewSlot(t *testing.T) {
	s := &Service{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A 启动
	_, cleanupA := s.beginGeneration(ctx, "conv-A")

	// B 启动（此时槽位切换为 B，并取消 A）
	_, cleanupB := s.beginGeneration(ctx, "conv-B")

	// A 收尾时执行 cleanupA —— 不应清空 B 的槽位
	cleanupA()
	if got := s.CurrentConvID(); got != "conv-B" {
		t.Errorf("cleanupA 后 currentConvID 应仍为 conv-B, 实际 %q", got)
	}
	if cancelFn := s.currentCancel; cancelFn == nil {
		t.Error("cleanupA 后 currentCancel 不应为 nil（仍属于 B）")
	}

	// B 正常收尾时才清空
	cleanupB()
	if got := s.CurrentConvID(); got != "" {
		t.Errorf("cleanupB 后 currentConvID 应为空, 实际 %q", got)
	}
	if cancelFn := s.currentCancel; cancelFn != nil {
		t.Error("cleanupB 后 currentCancel 应为 nil")
	}
}

// =============================================================================
// service_messages_build.go 纯函数测试
// =============================================================================

// TestValidateAttachments_NoAttachments 无附件返回 nil
func TestValidateAttachments_NoAttachments(t *testing.T) {
	if err := validateAttachments(llm.ModelCapabilities{}, nil); err != nil {
		t.Errorf("无附件应返回 nil, 实际 %v", err)
	}
}

// TestValidateAttachments_ImageNotSupported 图片不支持
func TestValidateAttachments_ImageNotSupported(t *testing.T) {
	atts := []Attachment{{Type: "image", Name: "pic.png", MimeType: "image/png"}}
	err := validateAttachments(llm.ModelCapabilities{ImageInput: false}, atts)
	if err == nil {
		t.Error("模型不支持图片时应返回错误")
	}
}

// TestValidateAttachments_ImageSupported 图片支持
func TestValidateAttachments_ImageSupported(t *testing.T) {
	atts := []Attachment{{Type: "image", Name: "pic.png", MimeType: "image/png"}}
	err := validateAttachments(llm.ModelCapabilities{ImageInput: true}, atts)
	if err != nil {
		t.Errorf("模型支持图片时应返回 nil, 实际 %v", err)
	}
}

// TestValidateAttachments_AudioNotSupported 音频不支持
func TestValidateAttachments_AudioNotSupported(t *testing.T) {
	atts := []Attachment{{Type: "audio", Name: "rec.wav", MimeType: "audio/wav"}}
	err := validateAttachments(llm.ModelCapabilities{AudioInput: false}, atts)
	if err == nil {
		t.Error("模型不支持音频时应返回错误")
	}
}

// TestValidateAttachments_AudioSupported 音频支持
func TestValidateAttachments_AudioSupported(t *testing.T) {
	atts := []Attachment{{Type: "audio", Name: "rec.wav", MimeType: "audio/wav"}}
	err := validateAttachments(llm.ModelCapabilities{AudioInput: true}, atts)
	if err != nil {
		t.Errorf("模型支持音频时应返回 nil, 实际 %v", err)
	}
}

// TestEstimateCurrentMessageTokens_Empty 空消息列表返回 0
func TestEstimateCurrentMessageTokens_Empty(t *testing.T) {
	if got := estimateCurrentMessageTokens(nil, nil); got != 0 {
		t.Errorf("空列表应返回 0, 实际 %d", got)
	}
}

// TestEstimateCurrentMessageTokens_NoAttachments 无附件
func TestEstimateCurrentMessageTokens_NoAttachments(t *testing.T) {
	dbMsgs := []*store.Message{{Role: "user", Content: "你好"}}
	got := estimateCurrentMessageTokens(dbMsgs, nil)
	if got <= 0 {
		t.Errorf("应返回正数, 实际 %d", got)
	}
}

// TestEstimateCurrentMessageTokens_WithAttachments 有附件累加
func TestEstimateCurrentMessageTokens_WithAttachments(t *testing.T) {
	dbMsgs := []*store.Message{{Role: "user", Content: "看图"}}
	atts := []Attachment{{Type: "image", Data: "abc"}}
	got := estimateCurrentMessageTokens(dbMsgs, atts)
	if got < imageTokenEstimate {
		t.Errorf("应包含图片 token, 实际 %d", got)
	}
}

// TestCalculateContextBudget_Basic 基本预算计算
func TestCalculateContextBudget_Basic(t *testing.T) {
	s := &Service{}
	cfg := &config.Config{}
	estimated, effectiveMax := s.calculateContextBudget(cfg, 4096, "系统提示", "", nil, nil)
	if estimated <= 0 {
		t.Errorf("估算 token 应 > 0, 实际 %d", estimated)
	}
	if effectiveMax <= 0 || effectiveMax >= 4096 {
		t.Errorf("effectiveMax 应在 0-4096 之间, 实际 %d", effectiveMax)
	}
}

// TestCalculateContextBudget_WithRAG 有 RAG 上下文
func TestCalculateContextBudget_WithRAG(t *testing.T) {
	s := &Service{}
	cfg := &config.Config{}
	estimated1, _ := s.calculateContextBudget(cfg, 4096, "系统提示", "", nil, nil)
	estimated2, _ := s.calculateContextBudget(cfg, 4096, "系统提示", "RAG上下文", nil, nil)
	if estimated2 <= estimated1 {
		t.Errorf("有 RAG 时估算应更大, estimated1=%d estimated2=%d", estimated1, estimated2)
	}
}

// TestCalculateContextBudget_WithCalibration 有校准数据
func TestCalculateContextBudget_WithCalibration(t *testing.T) {
	s := &Service{}
	s.tokenCalibMu.Lock()
	s.lastPromptTokens = 2000
	s.lastEstimatedTokens = 1000
	s.tokenCalibMu.Unlock()
	cfg := &config.Config{}
	estimated, _ := s.calculateContextBudget(cfg, 4096, "系统提示", "", nil, nil)
	// 校准比率 2.0，估算应放大
	if estimated <= 0 {
		t.Errorf("有校准时估算应 > 0, 实际 %d", estimated)
	}
}

// TestCalculateContextBudget_ProactiveThreshold 主动压缩阈值
func TestCalculateContextBudget_ProactiveThreshold(t *testing.T) {
	s := &Service{}
	cfg := &config.Config{ProactiveCompressThreshold: 0.5}
	_, effectiveMax := s.calculateContextBudget(cfg, 4096, "sys", "", nil, nil)
	// 阈值 0.5 → proactiveReserve = 4096*0.5 = 2048 > max(409, 512)
	if effectiveMax != 4096-2048 {
		t.Errorf("阈值 0.5 时 effectiveMax 应为 2048, 实际 %d", effectiveMax)
	}
}

// =============================================================================
// service_messages.go validateAttachment 补充测试
// =============================================================================

// TestValidateAttachment_EmptyMIME 空 MIME 允许通过
func TestValidateAttachment_EmptyMIME(t *testing.T) {
	att := Attachment{Type: "text", Name: "doc", MimeType: ""}
	_, ok := validateAttachment(att, 0)
	if !ok {
		t.Error("空 MIME 应允许通过")
	}
}

// TestValidateAttachment_TextAttachment 文本附件
func TestValidateAttachment_TextAttachment(t *testing.T) {
	att := Attachment{Type: "text", Name: "doc.txt", MimeType: "text/plain", Data: "content"}
	decoded, ok := validateAttachment(att, 0)
	if !ok {
		t.Error("文本附件应通过")
	}
	if decoded != -1 {
		t.Errorf("文本附件 decodedLen 应为 -1, 实际 %d", decoded)
	}
}

// TestValidateAttachment_ImageAttachment 图片附件
func TestValidateAttachment_ImageAttachment(t *testing.T) {
	att := Attachment{Type: "image", Name: "pic.png", MimeType: "image/png", Data: "aGVsbG8="}
	_, ok := validateAttachment(att, 0)
	if !ok {
		t.Error("图片附件应通过")
	}
}

// TestValidateAttachment_PdfAttachmentWithCache PDF 附件带缓存
func TestValidateAttachment_PdfAttachmentWithCache(t *testing.T) {
	// "test" 的 base64
	att := Attachment{Type: "pdf", Name: "doc.pdf", MimeType: "application/pdf", Data: "dGVzdA=="}
	cache := map[int][]byte{}
	decoded, ok := validateAttachment(att, 0, cache)
	if !ok {
		t.Error("PDF 附件应通过")
	}
	if decoded != 4 {
		t.Errorf("decodedLen 期望 4, 实际 %d", decoded)
	}
	if string(cache[0]) != "test" {
		t.Error("应缓存解码结果")
	}
}

// TestValidateAttachment_InvalidPdfBase64 无效 PDF base64
func TestValidateAttachment_InvalidPdfBase64(t *testing.T) {
	att := Attachment{Type: "pdf", Name: "bad.pdf", MimeType: "application/pdf", Data: "!!!invalid!!!"}
	_, ok := validateAttachment(att, 0)
	if ok {
		t.Error("无效 base64 的 PDF 应被拒绝")
	}
}

// TestValidateAttachment_UnknownTypeRejected 未知类型拒绝
func TestValidateAttachment_UnknownTypeRejected(t *testing.T) {
	att := Attachment{Type: "unknown", Name: "file.xyz", MimeType: "application/octet-stream"}
	_, ok := validateAttachment(att, 0)
	if ok {
		t.Error("未知类型应被拒绝")
	}
}

// TestValidateAttachment_BadMIME 非白名单 MIME 拒绝
func TestValidateAttachment_BadMIME(t *testing.T) {
	att := Attachment{Type: "image", Name: "mal.exe", MimeType: "application/x-msdownload"}
	_, ok := validateAttachment(att, 0)
	if ok {
		t.Error("非白名单 MIME 应被拒绝")
	}
}

// =============================================================================
// service_messages.go buildMessageFromAttachments 补充分支
// =============================================================================

// TestBuildMessageFromAttachments_OnlyText 纯文本附件
func TestBuildMessageFromAttachments_OnlyText(t *testing.T) {
	atts := []Attachment{{Type: "text", Name: "note.txt", MimeType: "text/plain", Data: "笔记内容"}}
	msg := buildMessageFromAttachments("user", "请看", atts)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
	content, ok := msg.Content.(string)
	if !ok {
		t.Fatal("Content 应为 string")
	}
	if !strings.Contains(content, "笔记内容") {
		t.Errorf("应包含文本附件内容, 实际 %q", content)
	}
}

// TestBuildMessageFromAttachments_EmptyContent_NoAttachments 空内容无附件
func TestBuildMessageFromAttachments_EmptyContent_NoAttachments(t *testing.T) {
	msg := buildMessageFromAttachments("user", "", nil)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildMessageFromAttachments_ImageOnlyExtra 仅图片（补充测试）
func TestBuildMessageFromAttachments_ImageOnlyExtra(t *testing.T) {
	atts := []Attachment{{Type: "image", Name: "pic.png", MimeType: "image/png", Data: "data:image/png;base64,abc"}}
	msg := buildMessageFromAttachments("user", "", atts)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildMessageFromAttachments_AudioOnly 仅音频
func TestBuildMessageFromAttachments_AudioOnly(t *testing.T) {
	atts := []Attachment{{Type: "audio", Name: "rec.wav", MimeType: "audio/wav", Data: "data:audio/wav;base64,abc", Format: "wav"}}
	msg := buildMessageFromAttachments("user", "", atts)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildMessageFromAttachments_VideoOnly 仅视频
func TestBuildMessageFromAttachments_VideoOnly(t *testing.T) {
	atts := []Attachment{{Type: "video", Name: "vid.mp4", MimeType: "video/mp4", Data: "data:video/mp4;base64,abc", Format: "mp4"}}
	msg := buildMessageFromAttachments("user", "", atts)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// =============================================================================
// service_messages.go formatSearchResultsWithLang / escapeXML 补充
// =============================================================================

// TestEscapeXML_SpecialChars XML 特殊字符转义
func TestEscapeXML_SpecialChars(t *testing.T) {
	got := escapeXML("<tag>\"quoted\" & 'apostrophe'")
	if strings.Contains(got, "<") || strings.Contains(got, ">") {
		t.Errorf("应转义尖括号, 实际 %q", got)
	}
	if strings.Contains(got, "\"") {
		t.Errorf("应转义引号, 实际 %q", got)
	}
}

// TestEscapeXML_NoSpecialChars 无特殊字符
func TestEscapeXML_NoSpecialChars(t *testing.T) {
	got := escapeXML("普通文本")
	if got != "普通文本" {
		t.Errorf("无特殊字符应原样返回, 实际 %q", got)
	}
}

// TestEscapeXML_Empty 空字符串
func TestEscapeXML_Empty(t *testing.T) {
	if got := escapeXML(""); got != "" {
		t.Errorf("空字符串应返回空, 实际 %q", got)
	}
}

// =============================================================================
// service_stream.go generateConversationTitle 测试（0% → 覆盖）
// =============================================================================

// TestGenerateConversationTitle_Empty 空内容返回默认标题
func TestGenerateConversationTitle_Empty(t *testing.T) {
	if got := generateConversationTitle(""); got != "新对话" {
		t.Errorf("空内容应返回 '新对话', 实际 %q", got)
	}
	if got := generateConversationTitle("   "); got != "新对话" {
		t.Errorf("纯空白应返回 '新对话', 实际 %q", got)
	}
}

// TestGenerateConversationTitle_OnlyPunctuation 纯标点返回默认标题
func TestGenerateConversationTitle_OnlyPunctuation(t *testing.T) {
	if got := generateConversationTitle("！！！？？？"); got != "新对话" {
		t.Errorf("纯标点应返回 '新对话', 实际 %q", got)
	}
	if got := generateConversationTitle("😀😀😀"); got != "新对话" {
		t.Errorf("纯表情应返回 '新对话', 实际 %q", got)
	}
}

// TestGenerateConversationTitle_Short 短内容原样返回
func TestGenerateConversationTitle_Short(t *testing.T) {
	if got := generateConversationTitle("你好"); got != "你好" {
		t.Errorf("短内容应原样返回, 实际 %q", got)
	}
	if got := generateConversationTitle("Hello World"); got != "Hello World" {
		t.Errorf("短英文应原样返回, 实际 %q", got)
	}
}

// TestGenerateConversationTitle_Long 长内容截断
func TestGenerateConversationTitle_Long(t *testing.T) {
	long := strings.Repeat("你好世界测试", 20) // 120 字
	got := generateConversationTitle(long)
	if len([]rune(got)) > 55 {
		t.Errorf("截断后长度应合理, 实际 %d 字符", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("截断后应以 … 结尾, 实际 %q", got)
	}
}

// TestGenerateConversationTitle_LongEnglish 长英文截断
func TestGenerateConversationTitle_LongEnglish(t *testing.T) {
	long := strings.Repeat("hello world ", 20)
	got := generateConversationTitle(long)
	if !strings.HasSuffix(got, "…") {
		t.Errorf("截断后应以 … 结尾, 实际 %q", got)
	}
}

// =============================================================================
// service_messages.go buildVisionOrTextMessage 测试（0% → 覆盖）
// =============================================================================

// TestBuildVisionOrTextMessage_Supported 支持图片 + 有效 JSON
func TestBuildVisionOrTextMessage_Supported(t *testing.T) {
	msg := buildVisionOrTextMessage("user", "看图", `["url1","url2"]`, true)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildVisionOrTextMessage_NotSupported 不支持图片
func TestBuildVisionOrTextMessage_NotSupported(t *testing.T) {
	msg := buildVisionOrTextMessage("user", "看图", `["url1"]`, false)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildVisionOrTextMessage_InvalidJSON 支持图片但 JSON 无效
func TestBuildVisionOrTextMessage_InvalidJSON(t *testing.T) {
	msg := buildVisionOrTextMessage("user", "看图", "not json", true)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildVisionOrTextMessage_EmptyArray 支持图片但空数组
func TestBuildVisionOrTextMessage_EmptyArray(t *testing.T) {
	msg := buildVisionOrTextMessage("user", "看图", "[]", true)
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// =============================================================================
// service_messages.go buildHistoryUserMessage 测试（0% → 覆盖）
// =============================================================================

// TestBuildHistoryUserMessage_PlainText 普通文本消息
func TestBuildHistoryUserMessage_PlainText(t *testing.T) {
	m := &store.Message{Role: "user", Content: "你好"}
	msg := buildHistoryUserMessage(m, "你好", nil, false, llm.ModelCapabilities{})
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildHistoryUserMessage_CurrentMsgWithAttachments 当前消息带附件
func TestBuildHistoryUserMessage_CurrentMsgWithAttachments(t *testing.T) {
	m := &store.Message{Role: "user", Content: "看图"}
	atts := []Attachment{{Type: "image", Name: "pic.png", MimeType: "image/png", Data: "data:image/png;base64,abc"}}
	msg := buildHistoryUserMessage(m, "看图", atts, true, llm.ModelCapabilities{})
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildHistoryUserMessage_HistoryWithAttachmentsSupported 历史消息附件且模型支持
func TestBuildHistoryUserMessage_HistoryWithAttachmentsSupported(t *testing.T) {
	atts := []Attachment{{Type: "image", Name: "pic.png", MimeType: "image/png", Data: "data:image/png;base64,abc"}}
	attJSON, _ := json.Marshal(atts)
	m := &store.Message{Role: "user", Content: "看图", Attachments: string(attJSON)}
	msg := buildHistoryUserMessage(m, "看图", nil, false, llm.ModelCapabilities{ImageInput: true})
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildHistoryUserMessage_HistoryWithAttachmentsNotSupported 历史消息附件但模型不支持
func TestBuildHistoryUserMessage_HistoryWithAttachmentsNotSupported(t *testing.T) {
	atts := []Attachment{{Type: "image", Name: "pic.png", MimeType: "image/png", Data: "data:image/png;base64,abc"}}
	attJSON, _ := json.Marshal(atts)
	m := &store.Message{Role: "user", Content: "看图", Attachments: string(attJSON)}
	msg := buildHistoryUserMessage(m, "看图", nil, false, llm.ModelCapabilities{ImageInput: false})
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// TestBuildHistoryUserMessage_HistoryWithImages 旧格式图片字段
func TestBuildHistoryUserMessage_HistoryWithImages(t *testing.T) {
	m := &store.Message{Role: "user", Content: "看图", Images: `["url1"]`}
	msg := buildHistoryUserMessage(m, "看图", nil, false, llm.ModelCapabilities{ImageInput: true})
	if msg.Role != "user" {
		t.Errorf("Role 期望 user, 实际 %q", msg.Role)
	}
}

// =============================================================================
// service_stream.go callback 测试（0% → 部分覆盖）
// callback 是 StreamAccumulator 的核心回调，处理 SSE chunk。
// 生活类比：像流水线工人，每个零件（chunk）过来都要检查并放到正确的位置。
// =============================================================================

// TestCallback_EmptyChunk 空 chunk 不影响状态
func TestCallback_EmptyChunk(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	cb := acc.callback()
	if err := cb(llm.SSEChunk{}); err != nil {
		t.Fatalf("空 chunk 不应报错: %v", err)
	}
	if acc.FullContent.Len() != 0 {
		t.Error("空 chunk 不应写入内容")
	}
}

// TestCallback_Usage 提取 usage 信息
func TestCallback_Usage(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	cb := acc.callback()
	cb(llm.SSEChunk{Usage: &llm.SSEUsage{PromptTokens: 500}})
	if acc.PromptTokens != 500 {
		t.Errorf("PromptTokens 期望 500, 实际 %d", acc.PromptTokens)
	}
}

// TestCallback_Timings 提取 timings 并调用回调
func TestCallback_Timings(t *testing.T) {
	var called bool
	acc := NewStreamAccumulator("c", nil, nil)
	acc.OnTimings = func(t llm.SSETimings) { called = true }
	cb := acc.callback()
	cb(llm.SSEChunk{Timings: &llm.SSETimings{PredictedPerSecond: 10.5, PredictedN: 100}})
	if acc.TokensPerSecond != 10.5 {
		t.Errorf("TokensPerSecond 期望 10.5, 实际 %f", acc.TokensPerSecond)
	}
	if acc.PredictedN != 100 {
		t.Errorf("PredictedN 期望 100, 实际 %d", acc.PredictedN)
	}
	if !called {
		t.Error("OnTimings 应被调用")
	}
}

// TestCallback_PromptProgress 提取 prompt_progress
func TestCallback_PromptProgress(t *testing.T) {
	var called bool
	acc := NewStreamAccumulator("c", nil, nil)
	acc.OnPromptProgress = func(p llm.SSEPromptProgress) { called = true }
	cb := acc.callback()
	cb(llm.SSEChunk{PromptProgress: &llm.SSEPromptProgress{Processed: 10}})
	if !called {
		t.Error("OnPromptProgress 应被调用")
	}
}

// TestCallback_CompletionID 提取 completion ID
func TestCallback_CompletionID(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	cb := acc.callback()
	cb(llm.SSEChunk{ID: "comp-123"})
	if acc.CompletionID != "comp-123" {
		t.Errorf("CompletionID 期望 comp-123, 实际 %q", acc.CompletionID)
	}
}

// TestCallback_Content 写入正文内容
func TestCallback_Content(t *testing.T) {
	var emitted string
	acc := NewStreamAccumulator("c", nil, func(_, _ string, content any) {
		emitted = content.(string)
	})
	cb := acc.callback()
	cb(llm.SSEChunk{
		Choices: []llm.SSEChoice{{
			Delta: llm.ChatMessage{Content: "你好"},
		}},
	})
	if acc.FullContent.String() != "你好" {
		t.Errorf("FullContent 期望 '你好', 实际 %q", acc.FullContent.String())
	}
	if emitted != "你好" {
		t.Errorf("应 emit '你好', 实际 %q", emitted)
	}
}

// TestCallback_Thinking 写入思考内容
func TestCallback_Thinking(t *testing.T) {
	var emitted string
	acc := NewStreamAccumulator("c", nil, func(_, event string, content any) {
		if event == "thinking" {
			emitted = content.(string)
		}
	})
	cb := acc.callback()
	cb(llm.SSEChunk{
		Choices: []llm.SSEChoice{{
			Delta: llm.ChatMessage{ReasoningContent: "思考中"},
		}},
	})
	if acc.FullThinking.String() != "思考中" {
		t.Errorf("FullThinking 期望 '思考中', 实际 %q", acc.FullThinking.String())
	}
	if emitted != "思考中" {
		t.Errorf("应 emit thinking '思考中', 实际 %q", emitted)
	}
}

// TestCallback_ToolCalls 写入工具调用
func TestCallback_ToolCalls(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	cb := acc.callback()
	cb(llm.SSEChunk{
		Choices: []llm.SSEChoice{{
			Delta: llm.ChatMessage{
				ToolCalls: []llm.ToolCall{{
					Index: 0,
					ID:    "call-1",
					Type:  "function",
					Function: llm.FunctionCall{
						Name:      "search",
						Arguments: `{"query":"test"}`,
					},
				}},
			},
		}},
	})
	if len(acc.ToolCallMap) != 1 {
		t.Fatalf("ToolCallMap 应有 1 项, 实际 %d", len(acc.ToolCallMap))
	}
	tc := acc.ToolCallMap[0]
	if tc.ID != "call-1" || tc.Function.Name != "search" {
		t.Errorf("工具调用数据错误: %+v", tc)
	}
}

// TestCallback_FinishReason 处理结束标记
func TestCallback_FinishReason(t *testing.T) {
	var flushed string
	acc := NewStreamAccumulator("c", nil, func(_, event string, content any) {
		if event == "token" {
			flushed = content.(string)
		}
	})
	acc.FirstTokenSent = true
	acc.TokenBuf.WriteString("残留")
	cb := acc.callback()
	reason := "stop"
	cb(llm.SSEChunk{
		Choices: []llm.SSEChoice{{
			FinishReason: &reason,
		}},
	})
	if acc.FinishReason != "stop" {
		t.Errorf("FinishReason 期望 stop, 实际 %q", acc.FinishReason)
	}
	// finish_reason 时应 flush 残留
	if acc.TokenBuf.Len() != 0 {
		t.Error("FinishReason 时应 flush TokenBuf")
	}
	if flushed != "残留" {
		t.Errorf("应 flush '残留', 实际 %q", flushed)
	}
}

// TestCallback_BufferOverflow 缓冲区溢出报错
func TestCallback_BufferOverflow(t *testing.T) {
	acc := NewStreamAccumulator("c", nil, nil)
	// 模拟缓冲区已满
	acc.FullContent.WriteString(strings.Repeat("x", maxStreamBufferSize+1))
	cb := acc.callback()
	err := cb(llm.SSEChunk{
		Choices: []llm.SSEChoice{{
			Delta: llm.ChatMessage{Content: "more"},
		}},
	})
	if err == nil {
		t.Error("缓冲区溢出应报错")
	}
}
