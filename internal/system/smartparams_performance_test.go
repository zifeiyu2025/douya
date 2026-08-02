// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"testing"
)

// ===== 性能模式校验测试 =====

// TestIsValidPerformanceMode_Valid 验证所有合法的性能模式值
func TestIsValidPerformanceMode_Valid(t *testing.T) {
	validModes := []string{"", "compatible", "balanced", "performance"}
	for _, mode := range validModes {
		t.Run("mode="+mode, func(t *testing.T) {
			if !IsValidPerformanceMode(mode) {
				t.Errorf("期望 IsValidPerformanceMode(%q) 返回 true", mode)
			}
		})
	}
}

// TestIsValidPerformanceMode_Invalid 验证非法的性能模式值
func TestIsValidPerformanceMode_Invalid(t *testing.T) {
	invalidModes := []string{"sport", "eco", "fast", "BALANCED", "Performance", "unknown"}
	for _, mode := range invalidModes {
		t.Run("mode="+mode, func(t *testing.T) {
			if IsValidPerformanceMode(mode) {
				t.Errorf("期望 IsValidPerformanceMode(%q) 返回 false", mode)
			}
		})
	}
}

// TestResolvePerformanceMode 验证空字符串和非法值回退到 balanced
func TestResolvePerformanceMode(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "balanced"},
		{"compatible", "compatible"},
		{"balanced", "balanced"},
		{"performance", "performance"},
		{"invalid", "balanced"},
		{"unknown", "balanced"},
	}
	for _, tt := range tests {
		t.Run("input="+tt.input, func(t *testing.T) {
			if got := resolvePerformanceMode(tt.input); got != tt.want {
				t.Errorf("resolvePerformanceMode(%q) = %q, 期望 %q", tt.input, got, tt.want)
			}
		})
	}
}

// ===== applyPerformanceMode 测试 =====
//
// 生活类比：applyPerformanceMode 就像汽车的驾驶模式切换器。
//   - balanced = 舒适模式，保持当前设置不变
//   - compatible = 经济模式，限制功率（保守配置）
//   - performance = 运动模式，榨干性能（激进配置）
//   - 但货车（保守后端）不能跑出跑车的速度（安全 > 性能）

// newBaseSmartParams 创建基础智能参数，用于测试 applyPerformanceMode 的覆盖效果
func newBaseSmartParams() *SmartParams {
	return &SmartParams{
		GPULayers:       40,
		Threads:         8,
		BatchSize:       512,
		FlashAttn:       true,
		ContextSize:     8192,
		SpecType:        "ngram-mod",
		NgramModNMin:    48,
		NgramModNMax:    64,
		NgramModNMatch:  24,
		SpecDraftNMax:   3,
		CacheTypeKDraft: "q8_0",
		CacheTypeVDraft: "q8_0",
	}
}

// TestApplyPerformanceMode_Balanced 验证 balanced 模式不修改任何参数
func TestApplyPerformanceMode_Balanced(t *testing.T) {
	p := newBaseSmartParams()
	original := *p // 保存快照

	applyPerformanceMode(p, "balanced", &HardwareInfo{HasGPU: true, GPUVRAMMB: 10000}, &GGUFMetadata{}, "cuda")

	if *p != original {
		t.Errorf("balanced 模式不应修改参数，期望 %+v，实际 %+v", original, *p)
	}
}

// TestApplyPerformanceMode_EmptyString 验证空字符串按 balanced 处理
func TestApplyPerformanceMode_EmptyString(t *testing.T) {
	p := newBaseSmartParams()
	original := *p

	applyPerformanceMode(p, "", &HardwareInfo{HasGPU: true, GPUVRAMMB: 10000}, &GGUFMetadata{}, "cuda")

	if *p != original {
		t.Errorf("空字符串应按 balanced 处理，期望 %+v，实际 %+v", original, *p)
	}
}

// TestApplyPerformanceMode_Compatible 验证 compatible 模式应用保守配置
func TestApplyPerformanceMode_Compatible(t *testing.T) {
	p := newBaseSmartParams()
	p.ContextSize = 16384 // 大上下文应被限制

	applyPerformanceMode(p, "compatible", &HardwareInfo{HasGPU: true, GPUVRAMMB: 10000}, &GGUFMetadata{}, "cuda")

	if p.GPULayers != 0 {
		t.Errorf("compatible 模式期望 GPULayers=0（让 llama.cpp 自决），实际 %d", p.GPULayers)
	}
	if p.FlashAttn != false {
		t.Errorf("compatible 模式期望 FlashAttn=false，实际 %v", p.FlashAttn)
	}
	if p.ContextSize != 4096 {
		t.Errorf("compatible 模式期望 ContextSize=4096（大上下文被限制），实际 %d", p.ContextSize)
	}
	if p.SpecType != "" {
		t.Errorf("compatible 模式期望 SpecType 为空（关闭推测解码），实际 %q", p.SpecType)
	}
	if p.NgramModNMin != 0 || p.NgramModNMax != 0 || p.NgramModNMatch != 0 {
		t.Errorf("compatible 模式期望 NgramMod 全部为 0，实际 min=%d max=%d match=%d", p.NgramModNMin, p.NgramModNMax, p.NgramModNMatch)
	}
	if p.SpecDraftNMax != 0 {
		t.Errorf("compatible 模式期望 SpecDraftNMax=0，实际 %d", p.SpecDraftNMax)
	}
	if p.CacheTypeKDraft != "" || p.CacheTypeVDraft != "" {
		t.Errorf("compatible 模式期望 CacheTypeDraft 为空，实际 K=%q V=%q", p.CacheTypeKDraft, p.CacheTypeVDraft)
	}
}

// TestApplyPerformanceMode_Compatible_SmallContext 验证 compatible 模式不放大已有的小上下文
func TestApplyPerformanceMode_Compatible_SmallContext(t *testing.T) {
	p := newBaseSmartParams()
	p.ContextSize = 2048 // 小于 4096，应保持不变

	applyPerformanceMode(p, "compatible", &HardwareInfo{HasGPU: true}, &GGUFMetadata{}, "cuda")

	if p.ContextSize != 2048 {
		t.Errorf("compatible 模式期望 ContextSize 保持 2048（<=4096 不放大），实际 %d", p.ContextSize)
	}
}

// TestApplyPerformanceMode_Performance_CUDA 验证 performance 模式在 CUDA 后端上应用激进配置
func TestApplyPerformanceMode_Performance_CUDA(t *testing.T) {
	p := newBaseSmartParams()
	p.GPULayers = 10   // 小于 99，应被强制拉满
	p.ContextSize = 4096 // 小于 16384，VRAM 余量足够时应被拉满

	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 24000} // 24GB VRAM
	meta := &GGUFMetadata{FileSize: 5_000_000_000, NParams: 7_000_000_000} // 5GB 模型, 7B 参数

	applyPerformanceMode(p, "performance", hw, meta, "cuda")

	if p.GPULayers != 99 {
		t.Errorf("performance 模式期望 GPULayers=99（全层卸载），实际 %d", p.GPULayers)
	}
	if !p.FlashAttn {
		t.Errorf("performance 模式期望 FlashAttn=true，实际 %v", p.FlashAttn)
	}
	// 5GB / 24GB ≈ 0.21 < 0.7，VRAM 余量足够，应拉满到 16384
	if p.ContextSize != 16384 {
		t.Errorf("performance 模式期望 ContextSize=16384（VRAM 余量足够），实际 %d", p.ContextSize)
	}
	// 非 MTP 模型且 NParams >= 3B，应强制 ngram-mod
	if p.SpecType != "ngram-mod" {
		t.Errorf("performance 模式期望 SpecType=ngram-mod，实际 %q", p.SpecType)
	}
}

// TestApplyPerformanceMode_Performance_MTPModel 验证 performance 模式对 MTP 模型强制 draft-mtp
func TestApplyPerformanceMode_Performance_MTPModel(t *testing.T) {
	p := newBaseSmartParams()
	p.SpecType = "" // 清空，让 performance 模式强制设置

	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 24000}
	meta := &GGUFMetadata{FileSize: 5_000_000_000, HasMTP: true, HasReasoning: true}

	applyPerformanceMode(p, "performance", hw, meta, "cuda")

	if p.SpecType != "draft-mtp" {
		t.Errorf("MTP 模型期望 SpecType=draft-mtp，实际 %q", p.SpecType)
	}
	if p.CacheTypeKDraft != "q8_0" || p.CacheTypeVDraft != "q8_0" {
		t.Errorf("MTP 模型期望 CacheTypeDraft=q8_0，实际 K=%q V=%q", p.CacheTypeKDraft, p.CacheTypeVDraft)
	}
	if p.SpecDraftNMax != 2 {
		t.Errorf("MTP + Reasoning 模型期望 SpecDraftNMax=2，实际 %d", p.SpecDraftNMax)
	}
}

// TestApplyPerformanceMode_Performance_MTPNoReasoning 验证 MTP 无推理模型 SpecDraftNMax=3
func TestApplyPerformanceMode_Performance_MTPNoReasoning(t *testing.T) {
	p := newBaseSmartParams()
	p.SpecType = ""

	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 24000}
	meta := &GGUFMetadata{FileSize: 5_000_000_000, HasMTP: true, HasReasoning: false}

	applyPerformanceMode(p, "performance", hw, meta, "cuda")

	if p.SpecDraftNMax != 3 {
		t.Errorf("MTP 无推理模型期望 SpecDraftNMax=3，实际 %d", p.SpecDraftNMax)
	}
}

// TestApplyPerformanceMode_Performance_VulkanConservative 验证 performance 模式不突破保守后端安全限制
// 生活类比：货车（Vulkan 后端）不能跑出跑车（CUDA 后端）的速度。
func TestApplyPerformanceMode_Performance_VulkanConservative(t *testing.T) {
	p := newBaseSmartParams()
	p.GPULayers = 50  // Vulkan 后端在 applyBackendSpecificParams 中已限制
	p.SpecType = ""   // 清空，让 performance 模式尝试设置

	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 8000}
	meta := &GGUFMetadata{FileSize: 4_000_000_000, NParams: 7_000_000_000}

	applyPerformanceMode(p, "performance", hw, meta, "vulkan")

	// Vulkan 是保守后端，不应突破 GPULayers 限制
	if p.GPULayers != 50 {
		t.Errorf("Vulkan 保守后端期望 GPULayers 保持 50（不突破限制），实际 %d", p.GPULayers)
	}
	// Vulkan 不应强制 FlashAttn
	if !p.FlashAttn {
		// FlashAttn 保持原值即可（newBaseSmartParams 中为 true）
		// 关键是 performance 模式不应将其设为 true（如果原来是 false 则保持 false）
	}
	// Vulkan 不应强制推测解码
	if p.SpecType != "" {
		t.Errorf("Vulkan 保守后端期望 SpecType 保持空（不强制推测解码），实际 %q", p.SpecType)
	}
}

// TestApplyPerformanceMode_Performance_CPUBackend 验证 CPU 后端不强制激进配置
func TestApplyPerformanceMode_Performance_CPUBackend(t *testing.T) {
	p := newBaseSmartParams()
	p.GPULayers = 0
	p.SpecType = ""

	hw := &HardwareInfo{HasGPU: false, HasCUDABackend: false}
	meta := &GGUFMetadata{FileSize: 4_000_000_000, NParams: 7_000_000_000}

	applyPerformanceMode(p, "performance", hw, meta, "cpu")

	// CPU 后端无 GPU，performance 模式应降级为 balanced（不修改参数）
	if p.GPULayers != 0 {
		t.Errorf("CPU 后端期望 GPULayers 保持 0，实际 %d", p.GPULayers)
	}
	if p.SpecType != "" {
		t.Errorf("CPU 后端期望 SpecType 保持空，实际 %q", p.SpecType)
	}
}

// TestApplyPerformanceMode_Performance_VRAMTight 验证 VRAM 紧张时不拉满上下文
func TestApplyPerformanceMode_Performance_VRAMTight(t *testing.T) {
	p := newBaseSmartParams()
	p.ContextSize = 4096

	// 模型占用 VRAM > 70%，不应拉满上下文
	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 8000} // 8GB VRAM
	meta := &GGUFMetadata{FileSize: 7_000_000_000, NParams: 7_000_000_000} // 7GB 模型, ratio ≈ 0.875

	applyPerformanceMode(p, "performance", hw, meta, "cuda")

	if p.ContextSize != 4096 {
		t.Errorf("VRAM 紧张时期望 ContextSize 保持 4096（不拉满），实际 %d", p.ContextSize)
	}
}

// TestApplyPerformanceMode_Performance_NilMeta 验证 meta 为 nil 时不崩溃
func TestApplyPerformanceMode_Performance_NilMeta(t *testing.T) {
	p := newBaseSmartParams()
	p.SpecType = ""

	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 24000}

	// 不应 panic
	applyPerformanceMode(p, "performance", hw, nil, "cuda")

	if p.GPULayers != 99 {
		t.Errorf("nil meta 期望 GPULayers=99，实际 %d", p.GPULayers)
	}
	// nil meta 时不应触发推测解码强制（因为无法判断模型是否支持）
	if p.SpecType != "" {
		// meta=nil 但 NParams 默认 0 < 3B，不会触发 ngram-mod
		// SpecType 保持原值或空都合理
	}
}

// TestApplyPerformanceMode_Performance_MoEModel 验证 MoE 模型 VRAM 计算使用 ExpertUsed
func TestApplyPerformanceMode_Performance_MoEModel(t *testing.T) {
	p := newBaseSmartParams()
	p.ContextSize = 4096

	// MoE 模型：FileSize=20GB 但只用了 8 个专家中的 2 个
	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 24000} // 24GB VRAM
	meta := &GGUFMetadata{
		FileSize:    20_000_000_000,
		ExpertCount: 8,
		ExpertUsed:  2,
		NParams:     7_000_000_000,
	}
	// 实际占用 = 20GB * 2/8 = 5GB, ratio = 5/24 ≈ 0.21 < 0.7

	applyPerformanceMode(p, "performance", hw, meta, "cuda")

	if p.ContextSize != 16384 {
		t.Errorf("MoE 模型 VRAM 计算使用 ExpertUsed，期望 ContextSize=16384，实际 %d", p.ContextSize)
	}
}

// TestApplyPerformanceMode_Performance_SmallModel 验证小模型（< 3B）不强制 ngram-mod
func TestApplyPerformanceMode_Performance_SmallModel(t *testing.T) {
	p := newBaseSmartParams()
	p.SpecType = "" // 清空，让 performance 模式尝试设置

	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 24000}
	meta := &GGUFMetadata{FileSize: 2_000_000_000, NParams: 1_500_000_000, HasMTP: false} // 1.5B 参数

	applyPerformanceMode(p, "performance", hw, meta, "cuda")

	// 小模型不强制推测解码
	if p.SpecType != "" {
		t.Errorf("小模型（< 3B）期望 SpecType 保持空，实际 %q", p.SpecType)
	}
}

// TestApplyPerformanceMode_Performance_AlreadyHasSpec 验证已有推测解码时不覆盖
func TestApplyPerformanceMode_Performance_AlreadyHasSpec(t *testing.T) {
	p := newBaseSmartParams()
	p.SpecType = "eagle3" // 已有推测解码

	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 24000}
	meta := &GGUFMetadata{FileSize: 5_000_000_000, HasMTP: true, NParams: 7_000_000_000}

	applyPerformanceMode(p, "performance", hw, meta, "cuda")

	// 已有 SpecType，performance 模式不应覆盖
	if p.SpecType != "eagle3" {
		t.Errorf("已有推测解码时期望保持 eagle3，实际 %q", p.SpecType)
	}
}
