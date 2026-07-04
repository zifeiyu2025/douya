// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"errors"
	"testing"
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
