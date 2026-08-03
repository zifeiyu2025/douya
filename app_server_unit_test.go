// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"errors"
	"strings"
	"testing"

	"douya/internal/apperror"
)

// --- findModelMatch 测试 ---

// TestFindModelMatch_ExactMatch 验证精确匹配优先于模糊匹配
func TestFindModelMatch_ExactMatch(t *testing.T) {
	statuses := map[string]string{
		"qwen2.5-7b": "loaded",
		"llama-3-8b": "loading",
	}
	found, status := findModelMatch("qwen2.5-7b", statuses)
	if !found {
		t.Error("精确匹配应返回 found=true")
	}
	if status != "loaded" {
		t.Errorf("精确匹配状态应为 loaded，实际: %s", status)
	}
}

// TestFindModelMatch_FuzzyMatch 验证精确匹配失败后走模糊匹配
func TestFindModelMatch_FuzzyMatch(t *testing.T) {
	// 模型名 "Qwen2.5-7B" 与列表中的 "qwen2.5-7b" 大小写不同，应通过模糊匹配命中
	statuses := map[string]string{
		"qwen2.5-7b": "loaded",
	}
	found, status := findModelMatch("Qwen2.5-7B", statuses)
	if !found {
		t.Error("模糊匹配应返回 found=true")
	}
	if status != "loaded" {
		t.Errorf("模糊匹配状态应为 loaded，实际: %s", status)
	}
}

// TestFindModelMatch_NoMatch 验证完全不匹配时返回 false
func TestFindModelMatch_NoMatch(t *testing.T) {
	statuses := map[string]string{
		"qwen2.5-7b": "loaded",
	}
	found, _ := findModelMatch("nonexistent-model", statuses)
	if found {
		t.Error("不匹配时应返回 found=false")
	}
}

// TestFindModelMatch_EmptyStatuses 验证空状态映射返回 false
func TestFindModelMatch_EmptyStatuses(t *testing.T) {
	found, _ := findModelMatch("any-model", map[string]string{})
	if found {
		t.Error("空状态映射应返回 found=false")
	}
}

// --- isAlreadyRunningError 测试 ---

// TestIsAlreadyRunningError_Nil 验证 nil 输入返回 false
func TestIsAlreadyRunningError_Nil(t *testing.T) {
	if isAlreadyRunningError(nil) {
		t.Error("nil 错误应返回 false")
	}
}

// TestIsAlreadyRunningError_ConflictKind 验证 apperror.ErrConflict 类型化错误被识别
func TestIsAlreadyRunningError_ConflictKind(t *testing.T) {
	err := apperror.New(apperror.KindConflict, "model already loaded")
	if !isAlreadyRunningError(err) {
		t.Error("KindConflict 错误应返回 true")
	}
}

// TestIsAlreadyRunningError_StringMatch 验证字符串匹配兜底逻辑
func TestIsAlreadyRunningError_StringMatch(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"already running", errors.New("model is already running"), true},
		{"already loaded", errors.New("model already loaded in another slot"), true},
		{"unrelated error", errors.New("network timeout"), false},
		{"empty message", errors.New(""), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isAlreadyRunningError(tt.err); got != tt.want {
				t.Errorf("isAlreadyRunningError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// --- isStackOverflowCrash 测试 ---

// TestIsStackOverflowCrash_DetectsKnownCodes 验证所有已知的栈溢出退出码格式
func TestIsStackOverflowCrash_DetectsKnownCodes(t *testing.T) {
	knownPatterns := []string{
		"process exit_code=-1073740791",
		"exit_code: 3221226507",
		"crash exit_code=-1073741571",
		"exit_code: 3221225725 detected",
	}
	for _, p := range knownPatterns {
		if !isStackOverflowCrash(p) {
			t.Errorf("应检测到栈溢出: %q", p)
		}
	}
}

// TestIsStackOverflowCrash_RejectsNormalErrors 验证普通错误不被误判为栈溢出
func TestIsStackOverflowCrash_RejectsNormalErrors(t *testing.T) {
	normalErrors := []string{
		"",
		"model loading timeout",
		"exit_code=1",
		"exit_code: 0",
		"failed to load model",
	}
	for _, e := range normalErrors {
		if isStackOverflowCrash(e) {
			t.Errorf("普通错误不应被误判为栈溢出: %q", e)
		}
	}
}

// --- classifyWaitError 测试 ---

// TestClassifyWaitError_StackOverflow_NonVulkan 验证非 Vulkan 后端的栈溢出诊断
func TestClassifyWaitError_StackOverflow_NonVulkan(t *testing.T) {
	app := NewApp()
	// resolvedBackend 为空或非 vulkan
	err := errors.New("process exit_code=-1073740791")
	result := app.classifyWaitError(err, "")
	if !strings.Contains(result, "栈溢出") {
		t.Errorf("应包含'栈溢出'提示，实际: %s", result)
	}
	if !strings.Contains(result, "减小 gpu_layers") {
		t.Errorf("非 Vulkan 后端应给出减小 gpu_layers 建议，实际: %s", result)
	}
}

// TestClassifyWaitError_StackOverflow_Vulkan 验证 Vulkan 后端的栈溢出诊断
func TestClassifyWaitError_StackOverflow_Vulkan(t *testing.T) {
	app := NewApp()
	app.resolvedBackend = "vulkan"
	err := errors.New("exit_code: 3221226507")
	result := app.classifyWaitError(err, "")
	if !strings.Contains(result, "栈溢出") {
		t.Errorf("应包含'栈溢出'提示，实际: %s", result)
	}
	if !strings.Contains(result, "Vulkan") {
		t.Errorf("Vulkan 后端应提示切换后端，实际: %s", result)
	}
}

// TestClassifyWaitError_Crash 验证崩溃特征（非栈溢出）的诊断
func TestClassifyWaitError_Crash(t *testing.T) {
	app := NewApp()
	tests := []string{
		"model failed to load: CUDA error",
		"process crashed unexpectedly",
		"exit_code=1",
	}
	for _, errMsg := range tests {
		err := errors.New(errMsg)
		result := app.classifyWaitError(err, "")
		if !strings.Contains(result, "模型加载失败") {
			t.Errorf("崩溃错误应包含'模型加载失败'，实际: %s", result)
		}
	}
}

// TestClassifyWaitError_Crash_WithStderrHint 验证带 stderrHint 的崩溃诊断
func TestClassifyWaitError_Crash_WithStderrHint(t *testing.T) {
	app := NewApp()
	err := errors.New("model failed to load")
	result := app.classifyWaitError(err, "CUDA out of memory")
	if !strings.Contains(result, "模型加载失败") {
		t.Errorf("应包含'模型加载失败'，实际: %s", result)
	}
	if !strings.Contains(result, "CUDA out of memory") {
		t.Errorf("应包含 stderrHint 内容，实际: %s", result)
	}
}

// TestClassifyWaitError_Timeout 验证超时（非崩溃特征）的诊断
func TestClassifyWaitError_Timeout(t *testing.T) {
	app := NewApp()
	err := errors.New("context deadline exceeded")
	result := app.classifyWaitError(err, "")
	if !strings.Contains(result, "模型加载超时") {
		t.Errorf("超时应包含'模型加载超时'，实际: %s", result)
	}
}

// TestClassifyWaitError_Timeout_WithStderrHint 验证带 stderrHint 的超时诊断
func TestClassifyWaitError_Timeout_WithStderrHint(t *testing.T) {
	app := NewApp()
	err := errors.New("context deadline exceeded")
	result := app.classifyWaitError(err, "still loading...")
	if !strings.Contains(result, "模型加载超时") {
		t.Errorf("应包含'模型加载超时'，实际: %s", result)
	}
	if !strings.Contains(result, "still loading...") {
		t.Errorf("应包含 stderrHint 内容，实际: %s", result)
	}
}
