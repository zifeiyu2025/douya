// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"errors"
	"strings"
	"testing"

	"douya/internal/llm"
	"douya/internal/store"
)

// =============================================================================
// formatSearchErrorHint 测试（0% → 全面覆盖）
// formatSearchErrorHint 把 SearchChain 返回的 Error 字段转成用户友好的中文提示。
// 生活类比：像快递客服把一堆物流异常码翻译成客户能听懂的话。
// =============================================================================

// TestFormatSearchErrorHint_Empty 空字符串应返回空
func TestFormatSearchErrorHint_Empty(t *testing.T) {
	if got := formatSearchErrorHint(""); got != "" {
		t.Errorf("空字符串应返回空，实际: %q", got)
	}
}

// TestFormatSearchErrorHint_NoProviders 无可用 provider 的提示
func TestFormatSearchErrorHint_NoProviders(t *testing.T) {
	cases := []string{
		"no providers available",
		"No Providers Available",
		"error: no providers available for search",
	}
	for _, errMsg := range cases {
		got := formatSearchErrorHint(errMsg)
		if !strings.Contains(got, "未配置搜索 API Key") {
			t.Errorf("no providers 应提示未配置 API Key，实际: %q", got)
		}
	}
}

// TestFormatSearchErrorHint_TavilyErrors Tavily 各类错误的提示
func TestFormatSearchErrorHint_TavilyErrors(t *testing.T) {
	cases := []struct {
		name   string
		errMsg string
		want   string
	}{
		{"认证失败", "tavily: 401 unauthorized", "Tavily API Key 无效"},
		{"API Key 错误", "tavily: invalid api key", "Tavily API Key 无效"},
		{"超时", "tavily: timeout", "Tavily 搜索超时"},
		{"timed out", "tavily: request timed out", "Tavily 搜索超时"},
		{"连接拒绝", "tavily: connection refused", "Tavily 网络连接失败"},
		{"无主机", "tavily: no such host", "Tavily 网络连接失败"},
		{"dial 错误", "tavily: dial tcp error", "Tavily 网络连接失败"},
		{"空结果", "tavily: empty results", ""}, // 空结果不提示
		{"其他错误", "tavily: internal server error", "Tavily 搜索失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSearchErrorHint(tc.errMsg)
			if tc.want == "" {
				// 空结果不算错误，hints 为空，函数兜底返回 "搜索失败"
				if got == "" {
					t.Errorf("空结果不应返回空字符串")
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("期望包含 %q，实际: %q", tc.want, got)
			}
		})
	}
}

// TestFormatSearchErrorHint_OllamaErrors Ollama 各类错误的提示
func TestFormatSearchErrorHint_OllamaErrors(t *testing.T) {
	cases := []struct {
		name   string
		errMsg string
		want   string
	}{
		{"认证失败", "ollama: 401 unauthorized", "Ollama API Key 无效"},
		{"超时", "ollama: timeout", "Ollama 搜索超时"},
		{"连接拒绝", "ollama: connection refused", "Ollama 网络连接失败"},
		{"无主机", "ollama: no such host", "Ollama 网络连接失败"},
		{"dial 错误", "ollama: dial error", "Ollama 网络连接失败"},
		{"其他错误", "ollama: unknown error", "Ollama 搜索失败"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := formatSearchErrorHint(tc.errMsg)
			if !strings.Contains(got, tc.want) {
				t.Errorf("期望包含 %q，实际: %q", tc.want, got)
			}
		})
	}
}

// TestFormatSearchErrorHint_GenericErrors 通用错误兜底（无 provider 关键词）
func TestFormatSearchErrorHint_GenericErrors(t *testing.T) {
	// 通用超时
	got := formatSearchErrorHint("request timeout")
	if !strings.Contains(got, "搜索超时") {
		t.Errorf("通用超时应包含 '搜索超时'，实际: %q", got)
	}

	// 通用连接拒绝
	got = formatSearchErrorHint("connection refused")
	if !strings.Contains(got, "网络连接失败") {
		t.Errorf("通用连接拒绝应包含 '网络连接失败'，实际: %q", got)
	}

	// 通用 no such host
	got = formatSearchErrorHint("no such host")
	if !strings.Contains(got, "网络连接失败") {
		t.Errorf("no such host 应包含 '网络连接失败'，实际: %q", got)
	}

	// 通用 dial 错误
	got = formatSearchErrorHint("dial tcp: connection refused")
	if !strings.Contains(got, "网络连接失败") {
		t.Errorf("dial 错误应包含 '网络连接失败'，实际: %q", got)
	}

	// 未知错误：应截断到 120 字符
	longErr := strings.Repeat("x", 200)
	got = formatSearchErrorHint(longErr)
	if !strings.Contains(got, "搜索失败：") {
		t.Errorf("未知错误应包含 '搜索失败：'，实际: %q", got)
	}
	if len(got) > 200 {
		t.Errorf("未知错误应截断，实际长度: %d", len(got))
	}
	if !strings.Contains(got, "...") {
		t.Errorf("截断的错误应包含 '...'，实际: %q", got)
	}
}

// TestFormatSearchErrorHint_MultipleProviders 多 provider 错误应同时提示
func TestFormatSearchErrorHint_MultipleProviders(t *testing.T) {
	errMsg := "all search providers failed: tavily: 401 unauthorized; ollama: timeout"
	got := formatSearchErrorHint(errMsg)
	if !strings.Contains(got, "Tavily") {
		t.Errorf("应包含 Tavily 提示，实际: %q", got)
	}
	if !strings.Contains(got, "Ollama") {
		t.Errorf("应包含 Ollama 提示，实际: %q", got)
	}
	// 应使用分号分隔
	if !strings.Contains(got, "；") {
		t.Errorf("多提示应使用 '；' 分隔，实际: %q", got)
	}
}

// =============================================================================
// estimateAttachmentTokensFromJSON 测试（6.2% → 全面覆盖）
// 生活类比：像快递总台统计一批包裹的总重量，逐个称重后累加。
// =============================================================================

// TestEstimateAttachmentTokensFromJSON_InvalidAudio 无效 JSON 含 audio 走 fallback
func TestEstimateAttachmentTokensFromJSON_InvalidAudio(t *testing.T) {
	got := estimateAttachmentTokensFromJSON("not json but has audio in it")
	if got != audioTokenEstimate {
		t.Errorf("无效JSON含audio应返回 %d, 实际 %d", audioTokenEstimate, got)
	}
}

// TestEstimateAttachmentTokensFromJSON_InvalidImage 无效 JSON 含 image 走 fallback
func TestEstimateAttachmentTokensFromJSON_InvalidImage(t *testing.T) {
	got := estimateAttachmentTokensFromJSON("not json but has image in it")
	if got != imageTokenEstimate {
		t.Errorf("无效JSON含image应返回 %d, 实际 %d", imageTokenEstimate, got)
	}
}

// TestEstimateAttachmentTokensFromJSON_ValidEmptyArray 有效空数组返回 0
func TestEstimateAttachmentTokensFromJSON_ValidEmptyArray(t *testing.T) {
	got := estimateAttachmentTokensFromJSON("[]")
	if got != 0 {
		t.Errorf("空数组应返回 0, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensFromJSON_TextAttachment 文本附件按内容估算
func TestEstimateAttachmentTokensFromJSON_TextAttachment(t *testing.T) {
	// 中文文本：4 字 * 2 token/字 + 1 = 9，+ 20 wrapper = 29
	json := `[{"type":"text","data":"你好世界"}]`
	got := estimateAttachmentTokensFromJSON(json)
	if got != 29 {
		t.Errorf("中文文本附件应返回 29, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensFromJSON_PdfAttachment PDF 附件按公式估算
func TestEstimateAttachmentTokensFromJSON_PdfAttachment(t *testing.T) {
	// data 长度 16: decodedBytes = 16*3/4 = 12, estimatedTextChars = 12/4 = 3, result = 3*3/2 + 20 = 24
	json := `[{"type":"pdf","data":"0123456789abcdef"}]`
	got := estimateAttachmentTokensFromJSON(json)
	if got != 24 {
		t.Errorf("PDF 附件应返回 24, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensFromJSON_UnknownTypeWithData 未知类型带数据返回 1500
func TestEstimateAttachmentTokensFromJSON_UnknownTypeWithData(t *testing.T) {
	json := `[{"type":"spreadsheet","data":"some data"}]`
	got := estimateAttachmentTokensFromJSON(json)
	if got != 1500 {
		t.Errorf("未知类型带数据应返回 1500, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensFromJSON_MultipleTypes 多种类型混合累加
func TestEstimateAttachmentTokensFromJSON_MultipleTypes(t *testing.T) {
	json := `[{"type":"image","data":"abc"},{"type":"video","data":"xyz"},{"type":"audio","data":"def"}]`
	got := estimateAttachmentTokensFromJSON(json)
	want := imageTokenEstimate + videoTokenEstimate + audioTokenEstimate
	if got != want {
		t.Errorf("多类型混合应返回 %d, 实际 %d", want, got)
	}
}

// =============================================================================
// EstimateAttachmentTokensWithData 测试（85% → 100%）
// 补充 text/pdf/default 分支的边界用例
// =============================================================================

// TestEstimateAttachmentTokensWithData_TextEmpty 文本附件空数据返回 0
func TestEstimateAttachmentTokensWithData_TextEmpty(t *testing.T) {
	if got := EstimateAttachmentTokensWithData("text", ""); got != 0 {
		t.Errorf("文本附件空数据应返回 0, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensWithData_TextChinese 中文文本附件
func TestEstimateAttachmentTokensWithData_TextChinese(t *testing.T) {
	// "你好" = 2 字 * 2 token + 1 = 5，+ 20 wrapper = 25
	got := EstimateAttachmentTokensWithData("text", "你好")
	if got != 25 {
		t.Errorf("中文文本应返回 25, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensWithData_TextEnglish 英文文本附件
func TestEstimateAttachmentTokensWithData_TextEnglish(t *testing.T) {
	// "hello" = 5 bytes, (5+2)/3 = 2, + 20 = 22
	got := EstimateAttachmentTokensWithData("text", "hello")
	if got != 22 {
		t.Errorf("英文文本应返回 22, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensWithData_PdfEmpty PDF 空数据返回 0
func TestEstimateAttachmentTokensWithData_PdfEmpty(t *testing.T) {
	if got := EstimateAttachmentTokensWithData("pdf", ""); got != 0 {
		t.Errorf("PDF 空数据应返回 0, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensWithData_PdfWithData PDF 带数据
func TestEstimateAttachmentTokensWithData_PdfWithData(t *testing.T) {
	// data 长度 8: decodedBytes = 8*3/4 = 6, estimatedTextChars = 6/4 = 1, result = 1*3/2 + 20 = 21
	got := EstimateAttachmentTokensWithData("pdf", "01234567")
	if got != 21 {
		t.Errorf("PDF 附件应返回 21, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensWithData_UnknownEmpty 未知类型空数据返回 0
func TestEstimateAttachmentTokensWithData_UnknownEmpty(t *testing.T) {
	if got := EstimateAttachmentTokensWithData("unknown", ""); got != 0 {
		t.Errorf("未知类型空数据应返回 0, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensWithData_UnknownWithData 未知类型带数据返回 1500
func TestEstimateAttachmentTokensWithData_UnknownWithData(t *testing.T) {
	if got := EstimateAttachmentTokensWithData("spreadsheet", "data"); got != 1500 {
		t.Errorf("未知类型带数据应返回 1500, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensWithData_CaseInsensitive 类型名大小写不敏感
func TestEstimateAttachmentTokensWithData_CaseInsensitive(t *testing.T) {
	if got := EstimateAttachmentTokensWithData("IMAGE", "abc"); got != imageTokenEstimate {
		t.Errorf("IMAGE 应返回 %d, 实际 %d", imageTokenEstimate, got)
	}
	if got := EstimateAttachmentTokensWithData("Video", "abc"); got != videoTokenEstimate {
		t.Errorf("Video 应返回 %d, 实际 %d", videoTokenEstimate, got)
	}
	if got := EstimateAttachmentTokensWithData("AUDIO", "abc"); got != audioTokenEstimate {
		t.Errorf("AUDIO 应返回 %d, 实际 %d", audioTokenEstimate, got)
	}
}

// =============================================================================
// matchErrorHintRule 测试（58.3% → 100%）
// matchErrorHintRule 检查小写错误消息是否匹配规则
// =============================================================================

// TestMatchErrorHintRule_AllKeywordsOnly 仅 allKeywords 的规则
func TestMatchErrorHintRule_AllKeywordsOnly(t *testing.T) {
	rule := errorHintRule{
		allKeywords: []string{"dll", "not found"},
	}
	// 全部包含 → 匹配
	if !matchErrorHintRule("dll not found", rule) {
		t.Error("全部包含 allKeywords 应匹配")
	}
	// 缺一个 → 不匹配
	if matchErrorHintRule("dll only", rule) {
		t.Error("缺少一个 allKeyword 不应匹配")
	}
	// 都不包含 → 不匹配
	if matchErrorHintRule("nothing here", rule) {
		t.Error("都不包含不应匹配")
	}
}

// TestMatchErrorHintRule_AnyKeywordsOnly 仅 anyKeywords 的规则
func TestMatchErrorHintRule_AnyKeywordsOnly(t *testing.T) {
	rule := errorHintRule{
		anyKeywords: []string{"timeout", "timed out"},
	}
	// 包含一个 → 匹配
	if !matchErrorHintRule("request timeout", rule) {
		t.Error("包含 timeout 应匹配")
	}
	if !matchErrorHintRule("request timed out", rule) {
		t.Error("包含 timed out 应匹配")
	}
	// 都不包含 → 不匹配
	if matchErrorHintRule("other error", rule) {
		t.Error("不包含任何 anyKeyword 不应匹配")
	}
}

// TestMatchErrorHintRule_AllAndAny 同时有 allKeywords 和 anyKeywords
func TestMatchErrorHintRule_AllAndAny(t *testing.T) {
	rule := errorHintRule{
		allKeywords: []string{"cuda"},
		anyKeywords: []string{"alloc", "memory"},
	}
	// allKeywords 全包含 + anyKeywords 包含一个 → 匹配
	if !matchErrorHintRule("cuda alloc failed", rule) {
		t.Error("allKeywords 全包含 + anyKeywords 含 alloc 应匹配")
	}
	if !matchErrorHintRule("cuda out of memory", rule) {
		t.Error("allKeywords 全包含 + anyKeywords 含 memory 应匹配")
	}
	// allKeywords 全包含 + anyKeywords 都不含 → 不匹配
	if matchErrorHintRule("cuda failed", rule) {
		t.Error("allKeywords 全包含但 anyKeywords 都不含不应匹配")
	}
	// allKeywords 缺一个 + anyKeywords 包含 → 不匹配
	if matchErrorHintRule("alloc failed", rule) {
		t.Error("allKeywords 不全包含不应匹配")
	}
}

// TestMatchErrorHintRule_EmptyRule 空规则（无关键词）应匹配任何消息
func TestMatchErrorHintRule_EmptyRule(t *testing.T) {
	rule := errorHintRule{}
	if !matchErrorHintRule("any message", rule) {
		t.Error("空规则应匹配任何消息")
	}
	if !matchErrorHintRule("", rule) {
		t.Error("空规则应匹配空消息")
	}
}

// =============================================================================
// applyErrorHintRule 测试（44.4% → 100%）
// applyErrorHintRule 根据规则构造返回消息
// =============================================================================

// TestApplyErrorHintRule_ReplaceMsg replaceMsg 优先级最高
func TestApplyErrorHintRule_ReplaceMsg(t *testing.T) {
	rule := errorHintRule{
		replaceMsg: "替换消息",
		errCode:    "ERR_TEST",
		hint:       "💡 不应出现",
	}
	got := applyErrorHintRule("原始错误", rule)
	if got != "替换消息" {
		t.Errorf("replaceMsg 优先级最高，实际: %q", got)
	}
}

// TestApplyErrorHintRule_ErrCodeWithHint 有错误码和提示
func TestApplyErrorHintRule_ErrCodeWithHint(t *testing.T) {
	rule := errorHintRule{
		errCode: ErrCodeOOM,
		hint:    "💡 减少GPU层数",
	}
	got := applyErrorHintRule("out of memory", rule)
	if !strings.HasPrefix(got, "["+ErrCodeOOM+"]") {
		t.Errorf("应有错误码前缀，实际: %q", got)
	}
	if !strings.Contains(got, "out of memory") {
		t.Errorf("应包含原始错误，实际: %q", got)
	}
	if !strings.Contains(got, "💡 减少GPU层数") {
		t.Errorf("应包含提示，实际: %q", got)
	}
}

// TestApplyErrorHintRule_ErrCodeOnly 只有错误码
func TestApplyErrorHintRule_ErrCodeOnly(t *testing.T) {
	rule := errorHintRule{
		errCode: ErrCodePermanentFailure,
	}
	got := applyErrorHintRule("permanent failure", rule)
	want := "[" + ErrCodePermanentFailure + "] permanent failure"
	if got != want {
		t.Errorf("期望 %q, 实际 %q", want, got)
	}
}

// TestApplyErrorHintRule_HintOnly 只有提示
func TestApplyErrorHintRule_HintOnly(t *testing.T) {
	rule := errorHintRule{
		hint: "💡 关闭Flash Attention",
	}
	got := applyErrorHintRule("flash_attn not supported", rule)
	if !strings.Contains(got, "flash_attn not supported") {
		t.Errorf("应包含原始错误，实际: %q", got)
	}
	if !strings.Contains(got, "💡 关闭Flash Attention") {
		t.Errorf("应包含提示，实际: %q", got)
	}
}

// TestApplyErrorHintRule_Nothing 无任何增强
func TestApplyErrorHintRule_Nothing(t *testing.T) {
	rule := errorHintRule{}
	got := applyErrorHintRule("some error", rule)
	if got != "some error" {
		t.Errorf("无增强时应原样返回，实际: %q", got)
	}
}

// =============================================================================
// enhanceErrorWithHint 测试（补充更多规则覆盖）
// =============================================================================

// TestEnhanceErrorWithHint_MoreRules 补充更多错误规则的覆盖
func TestEnhanceErrorWithHint_MoreRules(t *testing.T) {
	cases := []struct {
		name        string
		errMsg      string
		wantContain string
	}{
		{"上下文溢出", "exceed context size", "ERR_CTX_OVERFLOW"},
		{"context length", "context length too large", "ERR_CTX_OVERFLOW"},
		{"OOM", "out of memory error", "ERR_OOM"},
		{"CUDA alloc", "cuda alloc failed", "ERR_OOM"},
		{"memory allocation", "not enough memory allocation failed", "ERR_OOM"},
		{"mmproj", "mmproj failed to load", "💡"},
		{"DLL缺失", "dll not found", "ERR_DLL_MISSING"},
		{"引擎缺失", "llama-server not found", "ERR_ENGINE_MISSING"},
		{"引擎缺失中文", "引擎程序不存在", "ERR_ENGINE_MISSING"},
		{"模型缺失英文", "no models found gguf", "ERR_MODEL_MISSING"},
		{"模型缺失中文", "未找到任何 gguf 文件", "ERR_MODEL_MISSING"},
		{"永久失败", "permanent failure", "ERR_PERMANENT_FAILURE"},
		{"超时", "request timeout", "ERR_TIMEOUT"},
		{"连接拒绝", "connection refused", "AI 服务未启动"},
		{"连接重置", "connection reset", "连接中断"},
		{"broken pipe", "broken pipe error", "连接中断"},
		{"flash_attn", "flash_attn not supported", "💡"},
		{"cache_type", "cache_type not supported", "💡"},
		{"grammar reasoning", "grammar reasoning conflict", "💡"},
		{"backend sampling", "backend sampling not compatible", "💡"},
		{"speculative", "speculative decoding error", "💡"},
		{"draft failed", "draft model failed", "💡"},
		{"enable_thinking", "invalid type enable_thinking", "💡"},
		{"tool_call不支持", "tool_call not supported", "💡"},
		{"Tavily认证", "tavily 401 unauthorized api key", "Tavily"},
		{"Bing认证", "bing 401 error", "Bing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := enhanceErrorWithHint(tc.errMsg)
			if !strings.Contains(got, tc.wantContain) {
				t.Errorf("期望包含 %q，实际: %q", tc.wantContain, got)
			}
		})
	}
}

// =============================================================================
// estimateChatMessageTokens 测试（95% → 100%）
// 补充 ContentPart 和 []any 分支的覆盖
// =============================================================================

// TestEstimateChatMessageTokens_ContentPartsWithImage 含图片的 ContentPart
func TestEstimateChatMessageTokens_ContentPartsWithImage(t *testing.T) {
	msg := llm.ChatMessage{
		Role: "user",
		Content: []llm.ContentPart{
			{Type: "text", Text: "看这张图"},
			{Type: "image_url", ImageURL: &llm.ImageURL{URL: "data:image/png;base64,abc"}},
		},
	}
	got := estimateChatMessageTokens(msg)
	// 应包含 text token + imageTokenEstimate + 10(template)
	if got < imageTokenEstimate+10 {
		t.Errorf("含图片的 ContentPart 估算应 >= %d, 实际 %d", imageTokenEstimate+10, got)
	}
}

// TestEstimateChatMessageTokens_ContentPartsWithAudio 含音频的 ContentPart
func TestEstimateChatMessageTokens_ContentPartsWithAudio(t *testing.T) {
	msg := llm.ChatMessage{
		Role: "user",
		Content: []llm.ContentPart{
			{Type: "text", Text: "听这段录音"},
			{Type: "input_audio", InputAudio: &llm.InputAudio{Data: "base64data", Format: "wav"}},
		},
	}
	got := estimateChatMessageTokens(msg)
	// 应包含 text token + audioTokenEstimate + 10(template)
	if got < audioTokenEstimate+10 {
		t.Errorf("含音频的 ContentPart 估算应 >= %d, 实际 %d", audioTokenEstimate+10, got)
	}
}

// TestEstimateChatMessageTokens_AnySliceWithImage []any 含 image_url
func TestEstimateChatMessageTokens_AnySliceWithImage(t *testing.T) {
	msg := llm.ChatMessage{
		Role: "user",
		Content: []any{
			map[string]any{"type": "text", "text": "描述图片"},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,xyz"}},
		},
	}
	got := estimateChatMessageTokens(msg)
	if got < imageTokenEstimate+10 {
		t.Errorf("[]any 含 image_url 估算应 >= %d, 实际 %d", imageTokenEstimate+10, got)
	}
}

// TestEstimateChatMessageTokens_AnySliceWithAudio []any 含 input_audio
func TestEstimateChatMessageTokens_AnySliceWithAudio(t *testing.T) {
	msg := llm.ChatMessage{
		Role: "user",
		Content: []any{
			map[string]any{"type": "text", "text": "听音频"},
			map[string]any{"type": "input_audio", "input_audio": map[string]any{"data": "abc"}},
		},
	}
	got := estimateChatMessageTokens(msg)
	if got < audioTokenEstimate+10 {
		t.Errorf("[]any 含 input_audio 估算应 >= %d, 实际 %d", audioTokenEstimate+10, got)
	}
}

// TestEstimateChatMessageTokens_MinimumToken 空消息应返回最小值 11
func TestEstimateChatMessageTokens_MinimumToken(t *testing.T) {
	msg := llm.ChatMessage{Role: "user"}
	got := estimateChatMessageTokens(msg)
	if got != 11 {
		t.Errorf("空消息应返回最小值 11, 实际 %d", got)
	}
}

// =============================================================================
// estimateMessagesTokens 测试（80% → 100%）
// =============================================================================

// TestEstimateMessagesTokens_Empty 空列表返回 0
func TestEstimateMessagesTokens_Empty(t *testing.T) {
	got := estimateMessagesTokens([]llm.ChatMessage{})
	if got != 0 {
		t.Errorf("空列表应返回 0, 实际 %d", got)
	}
}

// TestEstimateMessagesTokens_Single 单条消息
func TestEstimateMessagesTokens_Single(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "你好"},
	}
	got := estimateMessagesTokens(msgs)
	if got <= 0 {
		t.Errorf("单条消息估算应 > 0, 实际 %d", got)
	}
}

// TestEstimateMessagesTokens_Multiple 多条消息累加
func TestEstimateMessagesTokens_Multiple(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: "system", Content: "系统提示"},
		{Role: "user", Content: "用户问题"},
		{Role: "assistant", Content: "助手回答"},
	}
	got := estimateMessagesTokens(msgs)
	// 每条至少 11 token，3 条至少 33
	if got < 33 {
		t.Errorf("3 条消息估算应 >= 33, 实际 %d", got)
	}
}

// =============================================================================
// computeMustKeepBudget 测试（80% → 100%）
// =============================================================================

// TestComputeMustKeepBudget_Empty 空列表
func TestComputeMustKeepBudget_Empty(t *testing.T) {
	indices, tokens := computeMustKeepBudget([]llm.ChatMessage{})
	if len(indices) != 0 {
		t.Errorf("空列表应无必保索引，实际 %v", indices)
	}
	if tokens != 0 {
		t.Errorf("空列表必保 token 应为 0, 实际 %d", tokens)
	}
}

// TestComputeMustKeepBudget_NoMustKeep 无必保消息
func TestComputeMustKeepBudget_NoMustKeep(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "普通问题"},
		{Role: "assistant", Content: "普通回答"},
	}
	indices, tokens := computeMustKeepBudget(msgs)
	if len(indices) != 0 {
		t.Errorf("无必保消息时索引应为空，实际 %v", indices)
	}
	if tokens != 0 {
		t.Errorf("无必保消息时 token 应为 0, 实际 %d", tokens)
	}
}

// TestComputeMustKeepBudget_WithMustKeep 含必保消息（代码块 +2 分不够，需要 >= 5 分）
func TestComputeMustKeepBudget_WithMustKeep(t *testing.T) {
	// 构造必保消息：含代码块(+3) + tool 角色(+2) = 5 分 → 必保
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "请记住这个重要决策"},
		{Role: "tool", Content: "```\ncode here\n```"},
		{Role: "assistant", Content: "普通回答"},
	}
	indices, tokens := computeMustKeepBudget(msgs)
	if len(indices) == 0 {
		t.Fatal("应至少有一条必保消息")
	}
	if tokens <= 0 {
		t.Errorf("必保消息 token 应 > 0, 实际 %d", tokens)
	}
}

// =============================================================================
// selectImportantMessages 测试（0% → 全面覆盖，补充已有测试的边界用例）
// =============================================================================

// TestSelectImportantMessages_NegativeBudget 负预算返回 nil
func TestSelectImportantMessages_NegativeBudget(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "问题"},
	}
	got := selectImportantMessages(msgs, -1)
	if got != nil {
		t.Errorf("负预算应返回 nil, 实际 %v", got)
	}
}

// TestSelectImportantMessages_OnlyMustKeep 仅必保消息被选中
func TestSelectImportantMessages_OnlyMustKeep(t *testing.T) {
	// 必保消息（tool+代码块=5分）+ 普通消息，预算为 0 只选必保
	msgs := []llm.ChatMessage{
		{Role: "user", Content: "普通问题"},
		{Role: "tool", Content: "```\nresult\n```"},
	}
	got := selectImportantMessages(msgs, 1) // 预算很小
	// 必保消息即使超预算也保留
	if len(got) == 0 {
		t.Error("应至少选中必保消息")
	}
}

// =============================================================================
// extractToolResponseContent 测试（0% → 全面覆盖，补充已有测试的边界用例）
// =============================================================================

// TestExtractToolResponseContent_PlainTextWithEmpty 空的 plain_text_response 走兜底
func TestExtractToolResponseContent_PlainTextWithEmpty(t *testing.T) {
	body := []byte(`{"plain_text_response":""}`)
	got := extractToolResponseContent(body)
	// plain_text_response 为空时回退到原始字符串
	if got != `{"plain_text_response":""}` {
		t.Errorf("空 plain_text_response 应回退到原始字符串，实际: %q", got)
	}
}

// TestExtractToolResponseContent_MCPNonTextOnly MCP 格式仅含非 text 类型
func TestExtractToolResponseContent_MCPNonTextOnly(t *testing.T) {
	body := []byte(`{"content":[{"type":"image","text":"ignored"}],"isError":false}`)
	got := extractToolResponseContent(body)
	// 没有 text 类型的 content，回退到原始字符串
	if got != string(body) {
		t.Errorf("仅含非 text 类型应回退到原始字符串，实际: %q", got)
	}
}

// TestExtractToolResponseContent_MCPEmptyContentArray MCP 格式空 content 数组
func TestExtractToolResponseContent_MCPEmptyContentArray(t *testing.T) {
	body := []byte(`{"content":[],"isError":false}`)
	got := extractToolResponseContent(body)
	// 空 content 数组，回退到原始字符串
	if got != string(body) {
		t.Errorf("空 content 数组应回退到原始字符串，实际: %q", got)
	}
}

// TestExtractToolResponseContent_MCPMultipleText MCP 格式多个 text 内容
func TestExtractToolResponseContent_MCPMultipleText(t *testing.T) {
	body := []byte(`{"content":[{"type":"text","text":"第一行"},{"type":"text","text":"第二行"},{"type":"text","text":"第三行"}],"isError":false}`)
	got := extractToolResponseContent(body)
	want := "第一行\n第二行\n第三行"
	if got != want {
		t.Errorf("多个 text 应换行拼接，期望 %q, 实际 %q", want, got)
	}
}

// TestExtractToolResponseContent_InvalidJSON 无效 JSON 返回原始字符串
func TestExtractToolResponseContent_InvalidJSON(t *testing.T) {
	body := []byte(`this is not json at all`)
	got := extractToolResponseContent(body)
	if got != "this is not json at all" {
		t.Errorf("无效 JSON 应返回原始字符串，实际: %q", got)
	}
}

// TestExtractToolResponseContent_EmptyBody 空响应体
func TestExtractToolResponseContent_EmptyBody(t *testing.T) {
	got := extractToolResponseContent([]byte{})
	if got != "" {
		t.Errorf("空响应体应返回空字符串，实际: %q", got)
	}
}

// TestExtractToolResponseContent_MCPTextEmpty MCP 格式 text 为空字符串
func TestExtractToolResponseContent_MCPTextEmpty(t *testing.T) {
	body := []byte(`{"content":[{"type":"text","text":""}],"isError":false}`)
	got := extractToolResponseContent(body)
	// text 为空，parts 为空，回退到原始字符串
	if got != string(body) {
		t.Errorf("text 为空应回退到原始字符串，实际: %q", got)
	}
}

// =============================================================================
// isMCPTool 测试（0% → 全面覆盖）
// isMCPTool 判断工具名是否为已缓存的 MCP 工具。
// 生活类比：检查这道菜是不是外卖平台提供的——查一下自家菜单里有没有。
// =============================================================================

// TestIsMCPTool_EmptyName 空名字返回 false
func TestIsMCPTool_EmptyName(t *testing.T) {
	s := &Service{}
	if s.isMCPTool("") {
		t.Error("空名字应返回 false")
	}
}

// TestIsMCPTool_SearchName "search" 返回 false
func TestIsMCPTool_SearchName(t *testing.T) {
	s := &Service{}
	if s.isMCPTool("search") {
		t.Error("'search' 应返回 false")
	}
}

// TestIsMCPTool_NotInCache 名字不在缓存中返回 false
func TestIsMCPTool_NotInCache(t *testing.T) {
	s := &Service{
		mcpToolsCache: []llm.ToolDefinition{
			{Function: llm.FunctionDef{Name: "weather"}},
		},
	}
	if s.isMCPTool("calculator") {
		t.Error("'calculator' 不在缓存中应返回 false")
	}
}

// TestIsMCPTool_InCache 名字在缓存中返回 true
func TestIsMCPTool_InCache(t *testing.T) {
	s := &Service{
		mcpToolsCache: []llm.ToolDefinition{
			{Function: llm.FunctionDef{Name: "weather"}},
			{Function: llm.FunctionDef{Name: "calculator"}},
		},
	}
	if !s.isMCPTool("weather") {
		t.Error("'weather' 在缓存中应返回 true")
	}
	if !s.isMCPTool("calculator") {
		t.Error("'calculator' 在缓存中应返回 true")
	}
}

// TestIsMCPTool_EmptyCache 空缓存返回 false
func TestIsMCPTool_EmptyCache(t *testing.T) {
	s := &Service{}
	if s.isMCPTool("any_tool") {
		t.Error("空缓存应返回 false")
	}
}

// =============================================================================
// SetImageTokenEstimate 测试
// =============================================================================

// TestSetImageTokenEstimate_Positive 正值更新图片 token 估算
func TestSetImageTokenEstimate_Positive(t *testing.T) {
	original := imageTokenEstimate
	defer func() { imageTokenEstimate = original }()

	SetImageTokenEstimate(5000)
	if imageTokenEstimate != 5000 {
		t.Errorf("正值应更新估算，期望 5000, 实际 %d", imageTokenEstimate)
	}
	if got := EstimateAttachmentTokens("image"); got != 5000 {
		t.Errorf("EstimateAttachmentTokens 应反映新值，期望 5000, 实际 %d", got)
	}
}

// TestSetImageTokenEstimate_Zero 零值恢复默认
func TestSetImageTokenEstimate_Zero(t *testing.T) {
	original := imageTokenEstimate
	defer func() { imageTokenEstimate = original }()

	SetImageTokenEstimate(5000)
	SetImageTokenEstimate(0) // 恢复默认
	if imageTokenEstimate != defaultImageTokenEstimate {
		t.Errorf("零值应恢复默认，期望 %d, 实际 %d", defaultImageTokenEstimate, imageTokenEstimate)
	}
}

// TestSetImageTokenEstimate_Negative 负值恢复默认
func TestSetImageTokenEstimate_Negative(t *testing.T) {
	original := imageTokenEstimate
	defer func() { imageTokenEstimate = original }()

	SetImageTokenEstimate(8000)
	SetImageTokenEstimate(-1) // 恢复默认
	if imageTokenEstimate != defaultImageTokenEstimate {
		t.Errorf("负值应恢复默认，期望 %d, 实际 %d", defaultImageTokenEstimate, imageTokenEstimate)
	}
}

// =============================================================================
// EstimateTokensByLang 测试
// =============================================================================

// TestEstimateTokensByLang_Chinese 中文按 2 token/字估算
func TestEstimateTokensByLang_Chinese(t *testing.T) {
	// 4 个中文字 = 4*2+1 = 9
	got := EstimateTokensByLang("你好世界", "zh")
	if got != 9 {
		t.Errorf("中文 4 字应返回 9, 实际 %d", got)
	}
}

// TestEstimateTokensByLang_English 英文按 1/3 token/byte 估算
func TestEstimateTokensByLang_English(t *testing.T) {
	// "hello" = 5 bytes, (5+2)/3 = 2
	got := EstimateTokensByLang("hello", "en")
	if got != 2 {
		t.Errorf("英文 'hello' 应返回 2, 实际 %d", got)
	}
}

// TestEstimateTokensByLang_Empty 空字符串
func TestEstimateTokensByLang_Empty(t *testing.T) {
	if got := EstimateTokensByLang("", "zh"); got != 1 {
		t.Errorf("中文空字符串应返回 1, 实际 %d", got)
	}
	if got := EstimateTokensByLang("", "en"); got != 0 {
		t.Errorf("英文空字符串应返回 0, 实际 %d", got)
	}
}

// TestEstimateTokensByLang_LongText 长文本
func TestEstimateTokensByLang_LongText(t *testing.T) {
	longZh := strings.Repeat("你", 1000)
	got := EstimateTokensByLang(longZh, "zh")
	if got != 2001 {
		t.Errorf("1000 个中文字应返回 2001, 实际 %d", got)
	}
}

// =============================================================================
// searchResultInstruction / SearchResultInstruction 测试
// =============================================================================

// TestSearchResultInstruction_Content 中文指令包含必要内容
func TestSearchResultInstruction_Content(t *testing.T) {
	zh := SearchResultInstruction("zh")
	if !strings.Contains(zh, "仅基于") {
		t.Errorf("中文指令应包含 '仅基于'，实际: %q", zh)
	}
	if !strings.Contains(zh, "编号引用格式") {
		t.Errorf("中文指令应包含 '编号引用格式'，实际: %q", zh)
	}
}

// TestSearchResultInstruction_EnglishContent 英文指令包含必要内容
func TestSearchResultInstruction_EnglishContent(t *testing.T) {
	en := SearchResultInstruction("en")
	if !strings.Contains(en, "strictly") {
		t.Errorf("英文指令应包含 'strictly'，实际: %q", en)
	}
	if !strings.Contains(en, "numbered citation") {
		t.Errorf("英文指令应包含 'numbered citation'，实际: %q", en)
	}
}

// =============================================================================
// detectLanguage / DetectLanguage 测试（补充更多语言用例）
// =============================================================================

// TestDetectLanguage_MoreCases 更多语言检测用例
func TestDetectLanguage_MoreCases(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"纯英文长文本", "The quick brown fox jumps over the lazy dog", "en"},
		{"中文长文本", "今天天气真好，我想出去散步", "zh"},
		// "日本語" 中日、本、語均为 CJK 统一汉字（U+4E00-U+9FFF），会被识别为中文
		{"日文_含汉字", "日本語のテスト", "zh"},
		{"韩文", "안녕하세요", "en"},          // 韩文不在检测范围
		{"俄文", "Привет мир", "en"},        // 西里尔字母不在检测范围
		{"阿拉伯文", "مرحبا بالعالم", "en"}, // 阿拉伯文不在检测范围
		{"数字", "1234567890", "en"},
		{"特殊字符", "!@#$%^&*()", "en"},
		{"混合中英日", "Hello 你好 こんにちは", "zh"}, // 有中文就归中文
		// 𠮷 (U+20BB7) 属于 CJK 扩展 B 区，不在 detectLanguage 检测的 U+4E00-U+9FFF / U+3400-U+4DBF 范围内
		{"扩展中文字符", "𠮷𠮷", "en"},
		{"纯假名", "こんにちは", "en"}, // 平假名不在 CJK 汉字范围
		{"片假名", "カタカナ", "en"},   // 片假名不在 CJK 汉字范围
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectLanguage(tc.content)
			if got != tc.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

// =============================================================================
// isCodeRelated / IsCodeRelated 测试（补充更多用例）
// =============================================================================

// TestIsCodeRelated_MoreCases 补充更多代码相关检测用例
func TestIsCodeRelated_MoreCases(t *testing.T) {
	// 代码语法关键词
	if !IsCodeRelated("func(") {
		t.Error("'func(' 应识别为代码相关")
	}
	if !IsCodeRelated("package main") {
		t.Error("'package main' 应识别为代码相关")
	}
	if !IsCodeRelated("interface{}") {
		t.Error("'interface{}' 应识别为代码相关")
	}
	if !IsCodeRelated("try { } catch { }") {
		t.Error("'try/catch' 应识别为代码相关")
	}
	if !IsCodeRelated("/** comment */") {
		t.Error("'/** comment */' 应识别为代码相关")
	}

	// 英文关键词
	if !IsCodeRelated("golang vs rust") {
		t.Error("'golang vs rust' 应识别为代码相关")
	}
	if !IsCodeRelated("typescript tutorial") {
		t.Error("'typescript tutorial' 应识别为代码相关")
	}
	if !IsCodeRelated("refactor this code") {
		t.Error("'refactor this code' 应识别为代码相关")
	}
	if !IsCodeRelated("k8s deployment") {
		t.Error("'k8s deployment' 应识别为代码相关")
	}

	// 中文关键词
	if !IsCodeRelated("微服务架构") {
		t.Error("'微服务架构' 应识别为代码相关")
	}
	if !IsCodeRelated("设计模式") {
		t.Error("'设计模式' 应识别为代码相关")
	}
	if !IsCodeRelated("排障") {
		t.Error("'排障' 应识别为代码相关")
	}

	// 非代码
	if IsCodeRelated("今天吃什么") {
		t.Error("'今天吃什么' 不应识别为代码相关")
	}
	if IsCodeRelated("how are you") {
		t.Error("'how are you' 不应识别为代码相关")
	}
}

// =============================================================================
// estimateMessageTokens / EstimateMessageTokens 测试（补充更多用例）
// =============================================================================

// TestEstimateMessageTokens_ToolCalls 工具调用字段的 token 估算
func TestEstimateMessageTokens_ToolCalls(t *testing.T) {
	m := &store.Message{
		Content:   "回答",
		ToolCalls: `[{"id":"call_1","function":{"name":"search"}}]`,
	}
	got := EstimateMessageTokens(m)
	if got <= 11 {
		t.Errorf("带 ToolCalls 的消息估算应 > 11, 实际 %d", got)
	}
}

// TestEstimateMessageTokens_SearchResults 搜索结果字段的 token 估算
func TestEstimateMessageTokens_SearchResults(t *testing.T) {
	m := &store.Message{
		Content:       "搜索",
		SearchResults: "搜索结果内容",
	}
	got := EstimateMessageTokens(m)
	if got <= 11 {
		t.Errorf("带 SearchResults 的消息估算应 > 11, 实际 %d", got)
	}
}

// TestEstimateMessageTokens_ThinkingContent 思考内容字段的 token 估算
func TestEstimateMessageTokens_ThinkingContent(t *testing.T) {
	m := &store.Message{
		Content:         "回答",
		ThinkingContent: "思考过程",
	}
	got := EstimateMessageTokens(m)
	if got <= 11 {
		t.Errorf("带 ThinkingContent 的消息估算应 > 11, 实际 %d", got)
	}
}

// TestEstimateMessageTokens_SingleImage 单张图片（非 JSON 数组格式）
func TestEstimateMessageTokens_SingleImage(t *testing.T) {
	m := &store.Message{
		Content: "看图",
		Images:  "url1", // 非数组格式
	}
	got := EstimateMessageTokens(m)
	if got < imageTokenEstimate+10 {
		t.Errorf("单张图片应 >= %d, 实际 %d", imageTokenEstimate+10, got)
	}
}

// TestEstimateMessageTokens_CommaSeparatedImages 逗号分隔的图片
func TestEstimateMessageTokens_CommaSeparatedImages(t *testing.T) {
	m := &store.Message{
		Content: "看图",
		Images:  "url1,url2,url3", // 逗号分隔但非数组
	}
	got := EstimateMessageTokens(m)
	// 3 张图片
	if got < 3*imageTokenEstimate+10 {
		t.Errorf("3 张逗号分隔图片应 >= %d, 实际 %d", 3*imageTokenEstimate+10, got)
	}
}

// TestEstimateMessageTokens_JSONArrayImages JSON 数组格式的图片
func TestEstimateMessageTokens_JSONArrayImages(t *testing.T) {
	m := &store.Message{
		Content: "看图",
		Images:  `["url1","url2","url3"]`, // JSON 数组格式
	}
	got := EstimateMessageTokens(m)
	// 3 张图片
	if got < 3*imageTokenEstimate+10 {
		t.Errorf("3 张 JSON 数组图片应 >= %d, 实际 %d", 3*imageTokenEstimate+10, got)
	}
}

// =============================================================================
// ParseExceedContextError 测试（补充更多用例）
// =============================================================================

// TestParseExceedContextError_RequestTokensFormat 验证 request tokens 格式
func TestParseExceedContextError_RequestTokensFormat(t *testing.T) {
	err := errors.New("request (12000 tokens) exceeds available context size (8192 tokens)")
	info := ParseExceedContextError(err)
	if info == nil {
		t.Fatal("应返回非 nil")
	}
	if info.PromptTokens != 12000 {
		t.Errorf("PromptTokens 期望 12000, 实际 %d", info.PromptTokens)
	}
	if info.ContextSize != 8192 {
		t.Errorf("ContextSize 期望 8192, 实际 %d", info.ContextSize)
	}
}

// TestParseExceedContextError_OnlyContextSize 仅有 context size
func TestParseExceedContextError_OnlyContextSize(t *testing.T) {
	err := errors.New("context size exceeded, available context size (4096 tokens)")
	info := ParseExceedContextError(err)
	if info == nil {
		t.Fatal("应返回非 nil")
	}
	if !info.Exceeded {
		t.Error("Exceeded 应为 true")
	}
	if info.ContextSize != 4096 {
		t.Errorf("ContextSize 期望 4096, 实际 %d", info.ContextSize)
	}
}
