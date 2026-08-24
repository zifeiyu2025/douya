// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"errors"
	"strings"
	"testing"

	"douya/internal/store"
)

// TestParseExceedContextError_NilError 验证 nil error 返回 nil
func TestParseExceedContextError_NilError(t *testing.T) {
	info := ParseExceedContextError(nil)
	if info != nil {
		t.Errorf("nil error 应返回 nil，实际: %+v", info)
	}
}

// TestParseExceedContextError_NonContextError 验证非上下文溢出错误返回 nil
func TestParseExceedContextError_NonContextError(t *testing.T) {
	err := errors.New("some other error")
	info := ParseExceedContextError(err)
	if info != nil {
		t.Errorf("非上下文溢出错误应返回 nil，实际: %+v", info)
	}
}

// TestParseExceedContextError_ExceedContextSizeError 验证解析 exceed_context_size_error 格式
func TestParseExceedContextError_ExceedContextSizeError(t *testing.T) {
	// 模拟 llama.cpp 返回的错误格式
	err := errors.New(`{"error":{"type":"exceed_context_size_error","n_prompt_tokens":5000,"n_ctx":4096}}`)
	info := ParseExceedContextError(err)
	if info == nil {
		t.Fatal("exceed_context_size_error 错误应返回非 nil")
	}
	if !info.Exceeded {
		t.Error("Exceeded 应为 true")
	}
	if info.PromptTokens != 5000 {
		t.Errorf("PromptTokens 期望 5000，实际: %d", info.PromptTokens)
	}
	if info.ContextSize != 4096 {
		t.Errorf("ContextSize 期望 4096，实际: %d", info.ContextSize)
	}
}

// TestParseExceedContextError_ExceedsAvailableFormat 验证解析 "exceeds available context size" 格式
func TestParseExceedContextError_ExceedsAvailableFormat(t *testing.T) {
	err := errors.New("request (8000 tokens) exceeds available context size (4096 tokens)")
	info := ParseExceedContextError(err)
	if info == nil {
		t.Fatal("exceeds available context size 错误应返回非 nil")
	}
	if !info.Exceeded {
		t.Error("Exceeded 应为 true")
	}
	if info.PromptTokens != 8000 {
		t.Errorf("PromptTokens 期望 8000，实际: %d", info.PromptTokens)
	}
	if info.ContextSize != 4096 {
		t.Errorf("ContextSize 期望 4096，实际: %d", info.ContextSize)
	}
}

// TestParseExceedContextError_ContextSizeExceeded 验证解析 "context size exceeded" 格式
func TestParseExceedContextError_ContextSizeExceeded(t *testing.T) {
	err := errors.New("context size exceeded, n_ctx=2048")
	info := ParseExceedContextError(err)
	if info == nil {
		t.Fatal("context size exceeded 错误应返回非 nil")
	}
	if !info.Exceeded {
		t.Error("Exceeded 应为 true")
	}
	if info.ContextSize != 2048 {
		t.Errorf("ContextSize 期望 2048，实际: %d", info.ContextSize)
	}
}

// TestParseExceedContextError_PartialInfo 验证部分信息缺失时仍返回可用结果
func TestParseExceedContextError_PartialInfo(t *testing.T) {
	// 只有 prompt_tokens，没有 n_ctx
	err := errors.New("exceed_context_size_error n_prompt_tokens=3000")
	info := ParseExceedContextError(err)
	if info == nil {
		t.Fatal("应返回非 nil")
	}
	if !info.Exceeded {
		t.Error("Exceeded 应为 true")
	}
	if info.PromptTokens != 3000 {
		t.Errorf("PromptTokens 期望 3000，实际: %d", info.PromptTokens)
	}
	// ContextSize 应为 0（未找到）
	if info.ContextSize != 0 {
		t.Errorf("ContextSize 期望 0（未找到），实际: %d", info.ContextSize)
	}
}

// TestCalcSlidingWindowSize_SmallContext 验证小上下文返回 6
func TestCalcSlidingWindowSize_SmallContext(t *testing.T) {
	cases := []int{1, 1024, 4096, 8192}
	for _, ctx := range cases {
		got := CalcSlidingWindowSize(ctx)
		if got != 6 {
			t.Errorf("contextSize=%d 期望 6，实际: %d", ctx, got)
		}
	}
}

// TestCalcSlidingWindowSize_MediumContext 验证中等上下文返回 12
func TestCalcSlidingWindowSize_MediumContext(t *testing.T) {
	cases := []int{8193, 16384, 32767}
	for _, ctx := range cases {
		got := CalcSlidingWindowSize(ctx)
		if got != 12 {
			t.Errorf("contextSize=%d 期望 12，实际: %d", ctx, got)
		}
	}
}

// TestCalcSlidingWindowSize_LargeContext 验证大上下文返回 20
func TestCalcSlidingWindowSize_LargeContext(t *testing.T) {
	cases := []int{32768, 65536, 131072, 1000000}
	for _, ctx := range cases {
		got := CalcSlidingWindowSize(ctx)
		if got != 20 {
			t.Errorf("contextSize=%d 期望 20，实际: %d", ctx, got)
		}
	}
}

// TestCalcSlidingWindowSize_ZeroOrNegative 验证 0 或负数使用默认值 4096
func TestCalcSlidingWindowSize_ZeroOrNegative(t *testing.T) {
	cases := []int{0, -1, -100}
	for _, ctx := range cases {
		got := CalcSlidingWindowSize(ctx)
		if got != 6 {
			t.Errorf("contextSize=%d 应使用默认 4096 返回 6，实际: %d", ctx, got)
		}
	}
}

// TestEstimateAttachmentTokens_KnownTypes 验证已知附件类型返回固定估算值
//
// 生活类比：就像快递公司对标准包裹（衣服、书本、电子产品）有固定运费表，
// 不管具体内容多少都按类型收费。
func TestEstimateAttachmentTokens_KnownTypes(t *testing.T) {
	cases := []struct {
		name    string
		attType string
		wantMin int
		wantMax int
	}{
		{"image", "image", 3500, 3500},
		{"IMAGE 大写", "IMAGE", 3500, 3500},
		{"video", "video", 5000, 5000},
		{"audio", "audio", 500, 500},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EstimateAttachmentTokens(c.attType)
			if got < c.wantMin || got > c.wantMax {
				t.Errorf("EstimateAttachmentTokens(%q) = %d, 期望 [%d, %d]", c.attType, got, c.wantMin, c.wantMax)
			}
		})
	}
}

// TestEstimateAttachmentTokens_UnknownType 验证未知类型返回 0
// 注意：这是无 data 版本的估算，未知类型无数据时返回 0 是正确的
func TestEstimateAttachmentTokens_UnknownType(t *testing.T) {
	cases := []string{"", "unknown", "spreadsheet", "text", "pdf"}
	for _, ct := range cases {
		got := EstimateAttachmentTokens(ct)
		if got != 0 {
			t.Errorf("EstimateAttachmentTokens(%q) 未知/无数据类型应返回 0，实际 %d", ct, got)
		}
	}
}

// TestSearchResultInstruction_Chinese 验证中文搜索结果指令
// 指令应引导模型总结信息并自然表达
func TestSearchResultInstruction_Chinese(t *testing.T) {
	got := SearchResultInstruction("zh")
	if !strings.Contains(got, "自然连贯") {
		t.Errorf("中文指令应包含 '自然连贯'，实际: %q", got)
	}
}

// TestSearchResultInstruction_English 验证英文搜索结果指令
func TestSearchResultInstruction_English(t *testing.T) {
	got := SearchResultInstruction("en")
	if !strings.Contains(got, "natural, coherent") {
		t.Errorf("英文指令应包含 'natural, coherent'，实际: %q", got)
	}
}

// TestSearchResultInstruction_OtherLang 验证非中文语言返回英文指令
func TestSearchResultInstruction_OtherLang(t *testing.T) {
	cases := []string{"", "ja", "fr", "code"}
	for _, lang := range cases {
		got := SearchResultInstruction(lang)
		if !strings.Contains(got, "Answer") {
			t.Errorf("非中文 %q 应返回英文指令，实际: %q", lang, got)
		}
	}
}

// TestEstimateMessageTokens_NilMessage 验证 nil 消息返回 0
func TestEstimateMessageTokens_NilMessage(t *testing.T) {
	got := EstimateMessageTokens(nil)
	if got != 0 {
		t.Errorf("nil 消息应返回 0，实际: %d", got)
	}
}

// TestEstimateMessageTokens_EmptyMessage 验证空消息返回最小值 11（10 模板开销 + 1）
func TestEstimateMessageTokens_EmptyMessage(t *testing.T) {
	m := &store.Message{}
	got := EstimateMessageTokens(m)
	if got != 11 {
		t.Errorf("空消息应返回 11（10 模板开销 + 最小 1），实际: %d", got)
	}
}

// TestEstimateMessageTokens_ContentOnly 验证仅文本内容的 token 估算
func TestEstimateMessageTokens_ContentOnly(t *testing.T) {
	m := &store.Message{
		Content: "你好世界", // 4 个中文字符，新系数 1 token/字 + 1 = 5 token
	}
	got := EstimateMessageTokens(m)
	// 5 (content) + 10 (template) = 15
	if got < 15 {
		t.Errorf("中文内容消息 token 估算 %d，期望 >= 15", got)
	}
}

// TestEstimateMessageTokens_WithImages 验证带图片消息累加图片 token
func TestEstimateMessageTokens_WithImages(t *testing.T) {
	m := &store.Message{
		Content: "看这张图",
		Images:  "[\"url1\",\"url2\"]", // 2 张图片
	}
	got := EstimateMessageTokens(m)
	// 应至少包含 2 * imageTokenEstimate (7000) + 10 (template)
	if got < 7000 {
		t.Errorf("带 2 张图片的消息 token 估算 %d，期望 >= 7000", got)
	}
}

// TestEstimateMessageTokens_WithAttachments 验证带附件消息累加附件 token
func TestEstimateMessageTokens_WithAttachments(t *testing.T) {
	// 构造包含 1 张图片和 1 段音频的附件 JSON
	attachmentsJSON := `[{"type":"image","data":"abc"},{"type":"audio","data":"xyz"}]`
	m := &store.Message{
		Content:     "带附件的消息",
		Attachments: attachmentsJSON,
	}
	got := EstimateMessageTokens(m)
	// image(3500) + audio(500) + content(若干) + 10(template)
	if got < 4000 {
		t.Errorf("带图片+音频附件的消息 token 估算 %d，期望 >= 4000", got)
	}
}

// TestEstimateMessageTokens_AllFields 验证所有字段都被累加
func TestEstimateMessageTokens_AllFields(t *testing.T) {
	m := &store.Message{
		Content:         "正文内容",
		ToolCalls:       "工具调用",
		SearchResults:   "搜索结果",
		ThinkingContent: "思考过程",
		Images:          "[\"url1\"]",
		Attachments:     `[{"type":"image","data":"abc"}]`,
	}
	got := EstimateMessageTokens(m)
	// 所有字段都有值，应比任何单一字段都大
	// 最小验证：至少包含 1 张图片 (3500) + 10 (template)
	if got < 3500 {
		t.Errorf("全字段消息 token 估算 %d，期望 >= 3500", got)
	}
}
