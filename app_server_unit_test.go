// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"errors"
	"strings"
	"testing"

	"douya/internal/apperror"
	"douya/internal/config"
	"douya/internal/llm"
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

// --- backendFallbackChain 测试 ---

// TestBackendFallbackChain_Auto 验证 auto 模式下回退链为"已解析后端 → Vulkan → CPU"。
func TestBackendFallbackChain_Auto(t *testing.T) {
	app := NewApp()
	app.resolvedBackend = llm.BackendCUDA
	app.config = &config.Config{BackendType: string(llm.BackendAuto)}

	chain := app.backendFallbackChain()
	if len(chain) != 3 {
		t.Fatalf("期望 3 个候选（CUDA/Vulkan/CPU），实际: %v", chain)
	}
	if chain[0] != llm.BackendCUDA || chain[1] != llm.BackendVulkan || chain[2] != llm.BackendCPU {
		t.Errorf("回退顺序应为 CUDA→Vulkan→CPU，实际: %v", chain)
	}
}

// TestBackendFallbackChain_Auto_ResolvedCPU 验证已解析为 CPU 时不重复添加 CPU。
func TestBackendFallbackChain_Auto_ResolvedCPU(t *testing.T) {
	app := NewApp()
	app.resolvedBackend = llm.BackendCPU
	app.config = &config.Config{BackendType: string(llm.BackendAuto)}

	chain := app.backendFallbackChain()
	if len(chain) != 2 {
		t.Fatalf("期望 2 个候选（CPU/Vulkan），实际: %v", chain)
	}
	if chain[0] != llm.BackendCPU || chain[1] != llm.BackendVulkan {
		t.Errorf("期望 CPU→Vulkan，实际: %v", chain)
	}
}

// TestBackendFallbackChain_Manual 用户显式指定后端时不自动回退（尊重用户选择）。
func TestBackendFallbackChain_Manual(t *testing.T) {
	app := NewApp()
	app.resolvedBackend = llm.BackendCUDA
	app.config = &config.Config{BackendType: string(llm.BackendCUDA)}

	chain := app.backendFallbackChain()
	if len(chain) != 1 || chain[0] != llm.BackendCUDA {
		t.Errorf("手动指定后端时应仅返回该后端，实际: %v", chain)
	}
}

// TestBackendFallbackChain_NilConfig 配置为 nil（未初始化）时按 auto 处理，回退到 Vulkan/CPU。
func TestBackendFallbackChain_NilConfig(t *testing.T) {
	app := NewApp()
	app.resolvedBackend = ""

	chain := app.backendFallbackChain()
	if len(chain) != 2 || chain[0] != llm.BackendVulkan || chain[1] != llm.BackendCPU {
		t.Errorf("nil 配置应按 Vulkan→CPU 回退，实际: %v", chain)
	}
}

// --- resolveDerivedServerParams 测试（无智能参数：仅以用户配置为准，未设置交给 llama.cpp 原生默认） ---

// boolPtr 返回指向给定布尔值的指针，用于构造 *bool 配置字段。
func boolPtr(b bool) *bool { return &b }

// TestDerivedMmprojOffload_Nil 验证 mmproj_offload 未设置（nil）时默认 false（交给 llama.cpp 原生判断）。
func TestDerivedMmprojOffload_Nil(t *testing.T) {
	cfg := &config.Config{MmprojOffload: nil}
	d := resolveDerivedServerParams(cfg)
	if d.MmprojOffload {
		t.Error("nil 配置应默认 false（不再自动启用），实际 true")
	}
}

// TestDerivedMmprojOffload_ExplicitTrue 验证 mmproj_offload=true 时强制启用。
func TestDerivedMmprojOffload_ExplicitTrue(t *testing.T) {
	cfg := &config.Config{MmprojOffload: boolPtr(true)}
	d := resolveDerivedServerParams(cfg)
	if !d.MmprojOffload {
		t.Error("用户显式 true 应启用 mmproj_offload")
	}
}

// TestDerivedMmprojOffload_ExplicitFalse 验证 mmproj_offload=false 可真正关闭。
func TestDerivedMmprojOffload_ExplicitFalse(t *testing.T) {
	cfg := &config.Config{MmprojOffload: boolPtr(false)}
	d := resolveDerivedServerParams(cfg)
	if d.MmprojOffload {
		t.Error("用户显式 false 应关闭 mmproj_offload，不可被自动逻辑覆盖")
	}
}

// TestDerivedFlashAttn_Nil 验证 Flash Attention 未设置（nil）时留空（交给 llama.cpp 默认开启）。
func TestDerivedFlashAttn_Nil(t *testing.T) {
	cfg := &config.Config{FlashAttn: nil}
	d := resolveDerivedServerParams(cfg)
	if d.FlashAttn != "" {
		t.Errorf("Flash Attention 未设置应留空（原生默认），实际 %q", d.FlashAttn)
	}
}

// TestDerivedFlashAttn_On 验证用户显式开启派生为 "on"。
func TestDerivedFlashAttn_On(t *testing.T) {
	cfg := &config.Config{FlashAttn: boolPtr(true)}
	d := resolveDerivedServerParams(cfg)
	if d.FlashAttn != "on" {
		t.Errorf("用户开启应派生 flash-attn=on，实际 %q", d.FlashAttn)
	}
}

// TestDerivedFlashAttn_Off 验证用户显式关闭派生为 "off"。
func TestDerivedFlashAttn_Off(t *testing.T) {
	cfg := &config.Config{FlashAttn: boolPtr(false)}
	d := resolveDerivedServerParams(cfg)
	if d.FlashAttn != "off" {
		t.Errorf("用户关闭应派生 flash-attn=off，实际 %q", d.FlashAttn)
	}
}
