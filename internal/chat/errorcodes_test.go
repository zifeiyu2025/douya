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
		name    string
		errMsg  string
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
