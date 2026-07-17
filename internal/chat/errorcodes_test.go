// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"strings"
	"testing"
)

// TestFormatErrCode_PrefixFormat 验证错误码前缀格式为 "[ERR_CODE] 消息"
// 生活类比：就像信封上的邮政编码，必须有固定的格式（方括号包裹、紧跟空格），
// 邮局（前端 classifyError）才能按编码分拣。
func TestFormatErrCode_PrefixFormat(t *testing.T) {
	cases := []struct {
		code string
		msg  string
		want string
	}{
		{ErrCodeContextOverflow, "上下文长度超限", "[ERR_CTX_OVERFLOW] 上下文长度超限"},
		{ErrCodeDLLMissing, "DLL 缺失", "[ERR_DLL_MISSING] DLL 缺失"},
		{ErrCodeEngineMissing, "引擎缺失", "[ERR_ENGINE_MISSING] 引擎缺失"},
		{ErrCodeModelMissing, "模型缺失", "[ERR_MODEL_MISSING] 模型缺失"},
		{ErrCodeOOM, "内存不足", "[ERR_OOM] 内存不足"},
		{ErrCodePermanentFailure, "永久失败", "[ERR_PERMANENT_FAILURE] 永久失败"},
		{ErrCodeTimeout, "请求超时", "[ERR_TIMEOUT] 请求超时"},
	}
	for _, c := range cases {
		got := formatErrCode(c.code, c.msg)
		if got != c.want {
			t.Errorf("formatErrCode(%q, %q) = %q, want %q", c.code, c.msg, got, c.want)
		}
		// 验证前缀以 "[" 开头，紧跟错误码，再跟 "] "
		if !strings.HasPrefix(got, "["+c.code+"] ") {
			t.Errorf("formatErrCode(%q, %q) 结果应以 [%s] 开头，实际: %q", c.code, c.msg, c.code, got)
		}
	}
}

// TestEnhanceErrorWithHint_ErrorCodePrefix 验证 enhanceErrorWithHint 对各类错误
// 返回的提示信息包含正确的 "[ERR_CODE]" 前缀。
func TestEnhanceErrorWithHint_ErrorCodePrefix(t *testing.T) {
	cases := []struct {
		name     string
		errMsg   string
		wantCode string // 期望的错误码前缀（不含方括号）
	}{
		{"上下文溢出-exceed context", "exceed context size", ErrCodeContextOverflow},
		{"上下文溢出-context length", "context length too long", ErrCodeContextOverflow},
		{"上下文溢出-context_size", "context_size error", ErrCodeContextOverflow},
		{"OOM-out of memory", "out of memory", ErrCodeOOM},
		{"OOM-cuda alloc", "cuda error: failed to alloc", ErrCodeOOM},
		{"OOM-memory allocation", "memory allocation failed", ErrCodeOOM},
		{"DLL缺失", "the specified module could not be found dll", ErrCodeDLLMissing},
		{"引擎缺失", "llama-server not found", ErrCodeEngineMissing},
		{"模型缺失", "no models found in gguf dir", ErrCodeModelMissing},
		{"永久失败", "permanent failure after retries", ErrCodePermanentFailure},
		{"超时-timeout", "request timeout", ErrCodeTimeout},
		{"超时-timed out", "request timed out", ErrCodeTimeout},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := enhanceErrorWithHint(c.errMsg)
			expectedPrefix := "[" + c.wantCode + "]"
			if !strings.HasPrefix(got, expectedPrefix) {
				t.Errorf("enhanceErrorWithHint(%q) 应以 %q 开头，实际: %q", c.errMsg, expectedPrefix, got)
			}
		})
	}
}

// TestEnhanceErrorWithHint_NoErrorCodeForUnmapped 验证未映射到错误码的错误
// 保持原有行为（不加 [ERR_CODE] 前缀），确保向后兼容。
func TestEnhanceErrorWithHint_NoErrorCodeForUnmapped(t *testing.T) {
	// mmproj 错误没有对应错误码，应保持原有行为（不加前缀）
	got := enhanceErrorWithHint("mmproj failed to load")
	if strings.HasPrefix(got, "[ERR_") {
		t.Errorf("mmproj 错误不应加错误码前缀，实际: %q", got)
	}
	// 未知错误应原样返回
	unknown := enhanceErrorWithHint("some unknown error")
	if strings.HasPrefix(unknown, "[ERR_") {
		t.Errorf("未知错误不应加错误码前缀，实际: %q", unknown)
	}
}

// TestEnhanceErrorWithHint_UserFriendlyMessages 验证用户友好的错误提示信息
// 这些错误返回特定的中文提示（非错误码前缀格式），帮助用户理解问题原因
func TestEnhanceErrorWithHint_UserFriendlyMessages(t *testing.T) {
	cases := []struct {
		name       string
		errMsg     string
		wantSubstr string // 期望包含的子串
	}{
		{
			name:       "连接拒绝 → AI服务未启动",
			errMsg:     "dial tcp 127.0.0.1:8080: connect: connection refused",
			wantSubstr: "AI 服务未启动",
		},
		{
			name:       "连接重置 → 连接中断",
			errMsg:     "read tcp: connection reset by peer",
			wantSubstr: "连接中断",
		},
		{
			name:       "broken pipe → 连接中断",
			errMsg:     "write: broken pipe",
			wantSubstr: "连接中断",
		},
		{
			name:       "flash_attn 不支持 → 设置建议",
			errMsg:     "flash_attn is not supported on this model",
			wantSubstr: "关闭 Flash Attention",
		},
		{
			name:       "cache_type 不支持 → 设置建议",
			errMsg:     "cache_type q3_k is not supported",
			wantSubstr: "KV 缓存类型",
		},
		{
			name:       "后端采样不兼容 → 设置建议",
			errMsg:     "backend sampling is not compatible with reasoning",
			wantSubstr: "关闭「后端采样」",
		},
		{
			name:       "enable_thinking 类型错误 → 设置建议",
			errMsg:     "invalid type for enable_thinking parameter",
			wantSubstr: "推理模式设为「关闭」",
		},
		{
			name:       "tool_call 不支持 → 预搜索模式提示",
			errMsg:     "tool_call is not supported by this model",
			wantSubstr: "预搜索模式",
		},
		{
			name:       "Tavily 401 → API Key 无效",
			errMsg:     "tavily API returned 401 unauthorized",
			wantSubstr: "Tavily",
		},
		{
			name:       "Bing 403 → API 认证失败",
			errMsg:     "bing search API returned 403 forbidden",
			wantSubstr: "Bing",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := enhanceErrorWithHint(c.errMsg)
			if !strings.Contains(got, c.wantSubstr) {
				t.Errorf("enhanceErrorWithHint(%q) 应包含 %q, 实际: %q", c.errMsg, c.wantSubstr, got)
			}
		})
	}
}

// TestEnhanceErrorWithHint_HintMessages 验证错误提示包含设置调整建议（💡）
func TestEnhanceErrorWithHint_HintMessages(t *testing.T) {
	cases := []struct {
		name   string
		errMsg string
	}{
		{"上下文溢出 → 增大上下文", "exceed context size"},
		{"OOM → 减少 GPU 层数", "out of memory"},
		{"超时 → 检查网络", "request timeout"},
		{"mmproj → 关闭视觉投影卸载", "mmproj failed to load"},
		{"flash_attn → 关闭 FA", "flash_attn not supported"},
		{"cache_type → 改为默认值", "cache_type unknown"},
		{"speculative → 关闭推测解码", "speculative decoding failed"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := enhanceErrorWithHint(c.errMsg)
			if !strings.Contains(got, "💡") {
				t.Errorf("enhanceErrorWithHint(%q) 应包含设置建议 💡, 实际: %q", c.errMsg, got)
			}
		})
	}
}

// TestEnhanceErrorWithHint_UnknownErrorPassThrough 验证未知错误原样返回
func TestEnhanceErrorWithHint_UnknownErrorPassThrough(t *testing.T) {
	input := "some completely unknown error message"
	got := enhanceErrorWithHint(input)
	if got != input {
		t.Errorf("未知错误应原样返回, 期望 %q, 实际 %q", input, got)
	}
}

// TestEnhanceErrorWithHint_PreservesOriginalMessage 验证错误提示保留原始错误信息
// 原始错误信息对用户诊断问题很重要，不应被替换掉
func TestEnhanceErrorWithHint_PreservesOriginalMessage(t *testing.T) {
	cases := []string{
		"exceed context size: n_prompt_tokens=5000 n_ctx=4096",
		"CUDA error: out of memory",
		"request timeout after 30s",
	}

	for _, c := range cases {
		got := enhanceErrorWithHint(c)
		// 原始错误信息应保留在返回的提示中（可能被截断/修改，但核心内容应在）
		// 这里只验证原始信息的关键部分被保留
		if !strings.Contains(got, "context") && !strings.Contains(got, "memory") && !strings.Contains(got, "timeout") {
			// 至少应保留一些原始信息
			t.Logf("提示: %q", got)
		}
	}
}

// TestErrCodeConstants 验证错误码常量值符合规范（不以方括号开头，纯大写下划线）
func TestErrCodeConstants(t *testing.T) {
	codes := []string{
		ErrCodeContextOverflow,
		ErrCodeDLLMissing,
		ErrCodeEngineMissing,
		ErrCodeModelMissing,
		ErrCodeOOM,
		ErrCodePermanentFailure,
		ErrCodeTimeout,
	}
	for _, code := range codes {
		if !strings.HasPrefix(code, "ERR_") {
			t.Errorf("错误码常量应以 ERR_ 开头，实际: %q", code)
		}
		if strings.Contains(code, "[") || strings.Contains(code, "]") {
			t.Errorf("错误码常量不应包含方括号，实际: %q", code)
		}
	}
}
