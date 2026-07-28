// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"testing"
)

// TestCalculateCacheTypes 测试 cache-type 推荐逻辑
// 根据模型占用显存的比例推荐不同的 KV cache 量化类型
func TestCalculateCacheTypes(t *testing.T) {
	// 计算 ratio 时使用：ratio = modelBytes / vramBytes
	// vramBytes = GPUVRAMMB * 1024 * 1024
	// 为简化，使用 GPUVRAMMB=10000（约 9.77GB），通过 FileSize 控制比例
	const vramMB = 10000
	vramBytes := float64(vramMB) * 1024 * 1024

	tests := []struct {
		name  string
		hw    *HardwareInfo
		meta  *GGUFMetadata
		wantK string
		wantV string
	}{
		{
			name:  "ratio ≤ 0.5 显存充裕",
			hw:    &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:  &GGUFMetadata{FileSize: int64(0.3 * vramBytes)}, // ratio ≈ 0.3
			wantK: "q8_0",
			wantV: "q8_0",
		},
		{
			name:  "ratio 0.5-0.7 显存较充裕",
			hw:    &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:  &GGUFMetadata{FileSize: int64(0.6 * vramBytes)}, // ratio ≈ 0.6
			wantK: "q8_0",
			wantV: "q4_0",
		},
		{
			name:  "ratio 0.7-0.85 显存紧张",
			hw:    &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:  &GGUFMetadata{FileSize: int64(0.8 * vramBytes)}, // ratio ≈ 0.8
			wantK: "q4_0",
			wantV: "q4_0",
		},
		{
			name:  "ratio > 0.85 显存很紧张",
			hw:    &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:  &GGUFMetadata{FileSize: int64(0.9 * vramBytes)}, // ratio ≈ 0.9
			wantK: "q4_1",
			wantV: "iq4_nl",
		},
		{
			name:  "无 GPU 返回 q4_0",
			hw:    &HardwareInfo{HasGPU: false, GPUVRAMMB: 0},
			meta:  &GGUFMetadata{FileSize: int64(0.3 * vramBytes)},
			wantK: "q4_0",
			wantV: "q4_0",
		},
		{
			name:  "有 GPU 但 GPUVRAMMB 为 0",
			hw:    &HardwareInfo{HasGPU: true, GPUVRAMMB: 0},
			meta:  &GGUFMetadata{FileSize: int64(0.3 * vramBytes)},
			wantK: "q4_0",
			wantV: "q4_0",
		},
		{
			name:  "有 GPU 但 meta 为 nil",
			hw:    &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:  nil,
			wantK: "q8_0",
			wantV: "q4_0",
		},
		{
			name:  "有 GPU 但 FileSize 为 0",
			hw:    &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:  &GGUFMetadata{FileSize: 0},
			wantK: "q8_0",
			wantV: "q4_0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotK, gotV := calculateCacheTypes(tt.hw, tt.meta)
			if gotK != tt.wantK {
				t.Errorf("CacheTypeK 期望 %s，实际 %s", tt.wantK, gotK)
			}
			if gotV != tt.wantV {
				t.Errorf("CacheTypeV 期望 %s，实际 %s", tt.wantV, gotV)
			}
		})
	}
}

// TestCalculateCacheTypes_MoE 测试 MoE 模型的 cache-type 推荐
// MoE 模型按激活参数折算模型权重占用
func TestCalculateCacheTypes_MoE(t *testing.T) {
	const vramMB = 10000
	vramBytes := float64(vramMB) * 1024 * 1024

	// MoE 模型：FileSize 很大，但激活参数只占一小部分
	// ExpertCount=60, ExpertUsed=4，折算比例为 4/60 ≈ 0.067
	// 原始 ratio = 0.9，折算后 ratio ≈ 0.06，应落入 ratio ≤ 0.5 区间
	meta := &GGUFMetadata{
		FileSize:    int64(0.9 * vramBytes),
		ExpertCount: 60,
		ExpertUsed:  4,
	}
	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB}

	gotK, gotV := calculateCacheTypes(hw, meta)
	if gotK != "q8_0" || gotV != "q8_0" {
		t.Errorf("MoE 折算后 ratio ≤ 0.5，期望 (q8_0, q8_0)，实际 (%s, %s)", gotK, gotV)
	}
}

// TestCalculateCacheTypes_NoNVFP4 验证所有架构都不会使用 nvfp4 作为 KV cache 类型
// 原因：nvfp4 是模型权重量化类型（MOSTLY_NVFP4），不是 KV cache 类型，
// llama.cpp 源码 common/arg.cpp 的 kv_cache_types 不包含 nvfp4，
// llama-server 收到 --cache-type-v nvfp4 会抛 runtime_error 导致启动失败。
func TestCalculateCacheTypes_NoNVFP4(t *testing.T) {
	const vramMB = 10000
	vramBytes := float64(vramMB) * 1024 * 1024

	// 所有架构（含 Blackwell）在所有 ratio 区间都不应返回 nvfp4
	archs := []string{"Blackwell", "Ada", "Ampere", "Turing", "Unknown", ""}
	ratios := []float64{0.3, 0.6, 0.8, 0.9}

	for _, arch := range archs {
		for _, ratio := range ratios {
			hw := &HardwareInfo{
				HasGPU:          true,
				GPUVRAMMB:       vramMB,
				GPUArchitecture: arch,
			}
			meta := &GGUFMetadata{FileSize: int64(ratio * vramBytes)}

			gotK, gotV := calculateCacheTypes(hw, meta)
			if gotK == "nvfp4" || gotV == "nvfp4" {
				t.Errorf("架构 %q ratio=%.1f 不应使用 nvfp4，实际 (K=%s, V=%s)", arch, ratio, gotK, gotV)
			}
		}
	}
}

// TestCalculateContextSize_BlackwellCap 验证 Blackwell 架构 ctx-size 被限制为 32768
// 原因：llama.cpp 在 Blackwell 上对超大上下文有兼容性问题，96K 会触发栈溢出崩溃
// 注意：此测试验证 fallback 路径（无 GGUF 元数据），验证 Blackwell 限制在所有路径都生效
func TestCalculateContextSize_BlackwellCap(t *testing.T) {
	// 模拟群友环境：RTX 5070 Ti, 16GB VRAM
	hw := &HardwareInfo{
		HasGPU:          true,
		GPUVRAMMB:       16303,
		GPUArchitecture: "Blackwell",
	}

	// 无模型文件时走 fallback 路径，Blackwell 限制应生效
	got := calculateContextSize(hw, "")
	if got > 32768 {
		t.Errorf("Blackwell 架构 ctx-size 应被限制为 32768，实际 %d", got)
	}
}

// TestCalculateSmartParams_BlackwellKeepsNgramMod 验证 Blackwell 架构保留 ngram-mod
// 设计决策：ngram-mod 推测解码是成熟特性，在所有架构上都能工作。
// Blackwell 崩溃根因是 ctx-size 过大（96K），不是 ngram-mod。
// 通过只限制 ctx-size 来定位根因，保留 ngram-mod 的加速收益（对 4B 模型有 1.5-2x 加速）。
func TestCalculateSmartParams_BlackwellKeepsNgramMod(t *testing.T) {
	hw := &HardwareInfo{
		HasGPU:          true,
		GPUVRAMMB:       16303,
		GPUArchitecture: "Blackwell",
	}

	// 非 MTP 模型，Blackwell 架构应保留 ngram-mod（无 GGUF 元数据时也启用）
	// 传入 "cuda" 保持与原行为一致（CUDA 后端不调整 ngram-mod）
	sp := CalculateSmartParams(hw, "", "cuda")
	if sp.SpecType != "ngram-mod" {
		t.Errorf("Blackwell 架构应保留 ngram-mod，实际 SpecType=%s", sp.SpecType)
	}
}

// TestEstimateModelTier 测试模型层级估算
// score = blockCount * embeddingLength / 1000
func TestEstimateModelTier(t *testing.T) {
	tests := []struct {
		name            string
		blockCount      int
		embeddingLength int
		wantTier        ModelTier
	}{
		// score < 80: ModelTierTiny
		{"score=10 Tiny", 10, 1000, ModelTierTiny},
		{"score=50 Tiny", 25, 2000, ModelTierTiny}, // 25*2000/1000=50
		// score 80-160: ModelTierSmall
		{"score=80 Small", 80, 1000, ModelTierSmall},   // 80*1000/1000=80
		{"score=120 Small", 120, 1000, ModelTierSmall}, // 120*1000/1000=120
		// score 160-300: ModelTierMedium
		{"score=160 Medium", 160, 1000, ModelTierMedium}, // 160*1000/1000=160
		{"score=250 Medium", 250, 1000, ModelTierMedium}, // 250*1000/1000=250
		// score 300-500: ModelTierLarge
		{"score=300 Large", 300, 1000, ModelTierLarge}, // 300*1000/1000=300
		{"score=450 Large", 450, 1000, ModelTierLarge}, // 450*1000/1000=450
		// score >= 500: ModelTierXL
		{"score=500 XL", 500, 1000, ModelTierXL}, // 500*1000/1000=500
		{"score=1000 XL", 1000, 1000, ModelTierXL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateModelTier(tt.blockCount, tt.embeddingLength)
			if got != tt.wantTier {
				t.Errorf("estimateModelTier(%d, %d) 期望 %v，实际 %v",
					tt.blockCount, tt.embeddingLength, tt.wantTier, got)
			}
		})
	}
}

// TestCalculateBatchSizeFromRatio 测试 batch size 计算
// 根据模型占用显存比例推荐不同的 batch size
func TestCalculateBatchSizeFromRatio(t *testing.T) {
	const vramMB = 10000
	vramBytes := float64(vramMB) * 1024 * 1024

	tests := []struct {
		name      string
		hw        *HardwareInfo
		meta      *GGUFMetadata
		wantBatch int
	}{
		{
			name:      "ratio ≤ 0.5 返回 1024",
			hw:        &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:      &GGUFMetadata{FileSize: int64(0.3 * vramBytes)},
			wantBatch: 1024,
		},
		{
			name:      "ratio 0.5-0.7 返回 512",
			hw:        &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:      &GGUFMetadata{FileSize: int64(0.6 * vramBytes)},
			wantBatch: 512,
		},
		{
			name:      "ratio 0.7-0.85 返回 256",
			hw:        &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:      &GGUFMetadata{FileSize: int64(0.8 * vramBytes)},
			wantBatch: 256,
		},
		{
			name:      "ratio > 0.85 返回 128",
			hw:        &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB},
			meta:      &GGUFMetadata{FileSize: int64(0.9 * vramBytes)},
			wantBatch: 128,
		},
		{
			name:      "无 GPU 返回 64",
			hw:        &HardwareInfo{HasGPU: false, GPUVRAMMB: 0},
			meta:      &GGUFMetadata{FileSize: int64(0.3 * vramBytes)},
			wantBatch: 64,
		},
		{
			name:      "有 GPU 但 GPUVRAMMB 为 0 返回 64",
			hw:        &HardwareInfo{HasGPU: true, GPUVRAMMB: 0},
			meta:      &GGUFMetadata{FileSize: int64(0.3 * vramBytes)},
			wantBatch: 64,
		},
		{
			name:      "meta 为 nil 回退到 VRAM 计算",
			hw:        &HardwareInfo{HasGPU: true, GPUVRAMMB: 16384}, // 16GB → 1024
			meta:      nil,
			wantBatch: 1024,
		},
		{
			name:      "FileSize 为 0 回退到 VRAM 计算",
			hw:        &HardwareInfo{HasGPU: true, GPUVRAMMB: 16384},
			meta:      &GGUFMetadata{FileSize: 0},
			wantBatch: 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateBatchSizeFromRatio(tt.hw, tt.meta)
			if got != tt.wantBatch {
				t.Errorf("calculateBatchSizeFromRatio 期望 %d，实际 %d", tt.wantBatch, got)
			}
		})
	}
}

// TestCalculateBatchSizeFromRatio_MoE 测试 MoE 模型的 batch size 计算
func TestCalculateBatchSizeFromRatio_MoE(t *testing.T) {
	const vramMB = 10000
	vramBytes := float64(vramMB) * 1024 * 1024

	// MoE 模型：FileSize 很大，但激活参数只占一小部分
	// 原始 ratio = 0.9，折算后 ratio ≈ 0.06，应返回 1024
	meta := &GGUFMetadata{
		FileSize:    int64(0.9 * vramBytes),
		ExpertCount: 60,
		ExpertUsed:  4,
	}
	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: vramMB}

	got := calculateBatchSizeFromRatio(hw, meta)
	if got != 1024 {
		t.Errorf("MoE 折算后 ratio ≤ 0.5，期望 1024，实际 %d", got)
	}
}

// TestCacheTypeSize 测试 cache type size 函数
// 返回每种量化类型每个元素占用的字节数
// 注意：nvfp4 不在此列表中，因为它是模型权重量化类型，不是 KV cache 类型
func TestCacheTypeSize(t *testing.T) {
	tests := []struct {
		name     string
		ct       string
		wantSize float64
	}{
		{"f32", "f32", 4.0},
		{"f16", "f16", 2.0},
		{"bf16", "bf16", 2.0},
		{"q8_0", "q8_0", 1.0},
		{"q5_1", "q5_1", 0.75},
		{"q5_0", "q5_0", 0.6875},
		{"q4_1", "q4_1", 0.625},
		{"q4_0", "q4_0", 0.5625},
		{"iq4_nl", "iq4_nl", 0.5},
		{"nvfp4 不是 cache 类型应回退默认", "nvfp4", 0.5625},
		{"未知类型默认 q4_0", "unknown", 0.5625},
		{"空字符串默认 q4_0", "", 0.5625},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := cacheTypeSize(tt.ct)
			if got != tt.wantSize {
				t.Errorf("cacheTypeSize(%q) 期望 %f，实际 %f", tt.ct, tt.wantSize, got)
			}
		})
	}
}

// TestEstimateKVCostPerToken 测试 KV cache 每token显存消耗估算
func TestEstimateKVCostPerToken(t *testing.T) {
	tests := []struct {
		name          string
		meta          *GGUFMetadata
		cacheTypeK    string
		cacheTypeV    string
		wantMinCost   int64 // 期望最小值（验证非零）
		wantExactCost int64 // 期望精确值（0 表示只验证非零）
	}{
		{
			name: "基本估算 f32",
			meta: &GGUFMetadata{
				BlockCount:      28,
				EmbeddingLength: 1024,
				HeadDimKV:       128,
				KVHeadCount:     8,
			},
			cacheTypeK:    "f32",
			cacheTypeV:    "f32",
			wantExactCost: int64(28 * 128 * 8 * 4.0 * 2), // 2 = K+V
		},
		{
			name: "q8_0 量化",
			meta: &GGUFMetadata{
				BlockCount:      28,
				EmbeddingLength: 1024,
				HeadDimKV:       128,
				KVHeadCount:     8,
			},
			cacheTypeK:    "q8_0",
			cacheTypeV:    "q8_0",
			wantExactCost: int64(28 * 128 * 8 * 1.0 * 2),
		},
		{
			name:          "meta 为 nil 返回 0",
			meta:          nil,
			cacheTypeK:    "f32",
			cacheTypeV:    "f32",
			wantExactCost: 0,
		},
		{
			name: "BlockCount 为 0 返回 0",
			meta: &GGUFMetadata{
				BlockCount:      0,
				EmbeddingLength: 1024,
				HeadDimKV:       128,
				KVHeadCount:     8,
			},
			cacheTypeK:    "f32",
			cacheTypeV:    "f32",
			wantExactCost: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateKVCostPerToken(tt.meta, tt.cacheTypeK, tt.cacheTypeV)
			if tt.wantExactCost != 0 || tt.meta == nil || tt.meta.BlockCount <= 0 {
				if got != tt.wantExactCost {
					t.Errorf("estimateKVCostPerToken 期望 %d，实际 %d", tt.wantExactCost, got)
				}
			} else if got <= 0 {
				t.Errorf("estimateKVCostPerToken 期望大于 0，实际 %d", got)
			}
		})
	}
}

// TestEstimateKVCostPerToken_HeadDimFallback 测试 HeadDimKV 缺失时的回退逻辑
func TestEstimateKVCostPerToken_HeadDimFallback(t *testing.T) {
	// HeadDimKV 为 0，应使用 EmbeddingLength 作为回退
	meta := &GGUFMetadata{
		BlockCount:      28,
		EmbeddingLength: 128, // 小于 256，直接使用
		HeadDimKV:       0,
		KVHeadCount:     8,
	}
	cost := estimateKVCostPerToken(meta, "f32", "f32")
	if cost <= 0 {
		t.Errorf("HeadDim 回退后应返回非零值，实际 %d", cost)
	}
	// 期望：28 * 128 * 8 * 4.0 * 2
	expected := int64(28 * 128 * 8 * 4.0 * 2)
	if cost != expected {
		t.Errorf("HeadDim 回退估算 期望 %d，实际 %d", expected, cost)
	}
}

// TestEstimateKVCostPerToken_KVHeadsFallback 测试 KVHeadCount 缺失时的回退逻辑
func TestEstimateKVCostPerToken_KVHeadsFallback(t *testing.T) {
	// KVHeadCount 为 0，应使用 EmbeddingLength / headDim 作为回退
	meta := &GGUFMetadata{
		BlockCount:      28,
		EmbeddingLength: 1024,
		HeadDimKV:       128,
		KVHeadCount:     0,
	}
	cost := estimateKVCostPerToken(meta, "f32", "f32")
	if cost <= 0 {
		t.Errorf("KVHead 回退后应返回非零值，实际 %d", cost)
	}
	// 期望：kvHeads = 1024 / 128 = 8
	// cost = 28 * 128 * 8 * 4.0 * 2
	expected := int64(28 * 128 * 8 * 4.0 * 2)
	if cost != expected {
		t.Errorf("KVHead 回退估算 期望 %d，实际 %d", expected, cost)
	}
}

// TestEstimateKVCostPerToken_NonStandardEmbeddingLength 验证当 EmbeddingLength
// 不是 128 的倍数且 HeadDimKV 缺失时，KV cache 估算不会被严重高估。
//
// Bug 场景：HeadDimKV=0, KVHeadCount>0, EmbeddingLength>256 且不是 128 的倍数时，
// 旧代码的 headDim 回退逻辑（要求 EmbeddingLength%128==0 才设为 128）会保留
// EmbeddingLength 作为 headDim，导致 KV cost 被高估 N 倍。
//
// 影响：部分模型（如 n_embd=1440）会被推荐过小的上下文长度（如 2048 而非 16384+），
// 严重损害用户体验。
//
// 生活类比：估算停车位需求时，把"整个停车场宽度"误当成"单车位宽度"，
// 结果算出来只需要几个车位，实际上能停的远不止这些。
func TestEstimateKVCostPerToken_NonStandardEmbeddingLength(t *testing.T) {
	// 构造一个 GQA 模型：HeadDimKV 缺失（0），KVHeadCount=8，EmbeddingLength=1440（非 128 倍数）
	meta := &GGUFMetadata{
		BlockCount:      28,
		EmbeddingLength: 1440, // 1440 / 128 = 11.25，不是整数
		HeadDimKV:       0,    // 缺失，需要回退
		KVHeadCount:     8,    // GQA：8 个 KV head
	}

	cost := estimateKVCostPerToken(meta, "q8_0", "q8_0")

	// 正确的估算：headDim 应回退为 128，kvHeads=8
	// KV cost = 28 * 128 * 8 * 1.0 * 2 = 57344
	expected := int64(28 * 128 * 8 * 1.0 * 2)

	if cost != expected {
		t.Errorf("非标准 EmbeddingLength 时 KV cost = %d, 期望 %d (headDim 应回退为 128)", cost, expected)
	}

	// 验证：旧 bug 会给出 headDim=1440 的结果（高估约 11 倍）
	buggyCost := int64(28 * 1440 * 8 * 1.0 * 2)
	if cost == buggyCost {
		t.Errorf("KV cost = %d，与 buggy 值 %d 相同，说明 headDim 回退逻辑未修复", cost, buggyCost)
	}
}

// TestCalculateBatchSize 测试基于 VRAM 的 batch size 计算（回退方案）
func TestCalculateBatchSize(t *testing.T) {
	tests := []struct {
		name      string
		hw        *HardwareInfo
		wantBatch int
	}{
		{"无 GPU 返回 64", &HardwareInfo{HasGPU: false, GPUVRAMMB: 0}, 64},
		{"VRAM 16GB 返回 1024", &HardwareInfo{HasGPU: true, GPUVRAMMB: 16384}, 1024},
		{"VRAM 12GB 返回 512", &HardwareInfo{HasGPU: true, GPUVRAMMB: 12288}, 512},
		{"VRAM 8GB 返回 512", &HardwareInfo{HasGPU: true, GPUVRAMMB: 8192}, 512},
		{"VRAM 6GB 返回 256", &HardwareInfo{HasGPU: true, GPUVRAMMB: 6144}, 256},
		{"VRAM 4GB 返回 128", &HardwareInfo{HasGPU: true, GPUVRAMMB: 4096}, 128},
		{"VRAM 2GB 返回 64", &HardwareInfo{HasGPU: true, GPUVRAMMB: 2048}, 64},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateBatchSize(tt.hw)
			if got != tt.wantBatch {
				t.Errorf("calculateBatchSize 期望 %d，实际 %d", tt.wantBatch, got)
			}
		})
	}
}

// TestModelTierConstants 测试 ModelTier 常量值
// 确保枚举值符合预期顺序
func TestModelTierConstants(t *testing.T) {
	if ModelTierTiny != 0 {
		t.Errorf("ModelTierTiny 期望 0，实际 %d", ModelTierTiny)
	}
	if ModelTierSmall != 1 {
		t.Errorf("ModelTierSmall 期望 1，实际 %d", ModelTierSmall)
	}
	if ModelTierMedium != 2 {
		t.Errorf("ModelTierMedium 期望 2，实际 %d", ModelTierMedium)
	}
	if ModelTierLarge != 3 {
		t.Errorf("ModelTierLarge 期望 3，实际 %d", ModelTierLarge)
	}
	if ModelTierXL != 4 {
		t.Errorf("ModelTierXL 期望 4，实际 %d", ModelTierXL)
	}
	if ModelTierUnknown != 5 {
		t.Errorf("ModelTierUnknown 期望 5，实际 %d", ModelTierUnknown)
	}
}

// TestCalculateSmartParams_Vulkan 验证 Vulkan 后端的保守配置
// Vulkan 后端：Flash Attention 关闭、推测解码关闭、ctx-size ≤ 8192、gpu_layers ≤ 50
// B-2/B-3 增强：防止 Vulkan 后端栈溢出崩溃（0xC0000409）
// 注意：mmproj_offload 不再强制关闭（日志证据：mmproj 不是栈溢出根因）
//
// 生活类比：Vulkan 像"通用适配器"，兼容性广但性能特性未知，
// 所以默认关闭高级特性（Flash Attention、推测解码），用保守配置确保稳定运行。
func TestCalculateSmartParams_Vulkan(t *testing.T) {
	hw := &HardwareInfo{
		HasGPU:    true,
		GPUVRAMMB: 16303,
	}
	// 传入 "vulkan"，应启用保守配置
	sp := CalculateSmartParams(hw, "", "vulkan")
	if sp.FlashAttn != false {
		t.Errorf("Vulkan 后端应关闭 Flash Attention，实际 FlashAttn=%v", sp.FlashAttn)
	}
	if sp.SpecType != "" {
		t.Errorf("Vulkan 后端应关闭推测解码，实际 SpecType=%s", sp.SpecType)
	}
	// B-3：ctx-size 限制收紧到 8192
	if sp.ContextSize > 8192 {
		t.Errorf("Vulkan 后端 ctx-size 应 ≤ 8192，实际 %d", sp.ContextSize)
	}
	// B-2：gpu_layers 限制到 50
	if sp.GPULayers > 50 {
		t.Errorf("Vulkan 后端 gpu_layers 应 ≤ 50，实际 %d", sp.GPULayers)
	}
}

// TestCalculateSmartParams_CPU 验证 CPU 后端的保守配置
// CPU 后端：GPULayers=0、Flash Attention 关闭、ctx-size ≤ 8192、KV cache 用 q4_0
//
// 生活类比：CPU 模式就像"纯手动挡"，没有 GPU 加速，
// 所有 GPU 相关特性都关闭，KV cache 用最省内存的 q4_0。
func TestCalculateSmartParams_CPU(t *testing.T) {
	hw := &HardwareInfo{
		HasGPU:    true,
		GPUVRAMMB: 16303,
	}
	// 传入 "cpu"，应启用 CPU 配置
	sp := CalculateSmartParams(hw, "", "cpu")
	if sp.GPULayers != 0 {
		t.Errorf("CPU 后端 GPULayers 应为 0，实际 %d", sp.GPULayers)
	}
	if sp.FlashAttn != false {
		t.Errorf("CPU 后端应关闭 Flash Attention，实际 FlashAttn=%v", sp.FlashAttn)
	}
	if sp.ContextSize > 8192 {
		t.Errorf("CPU 后端 ctx-size 应 ≤ 8192，实际 %d", sp.ContextSize)
	}
	if sp.CacheTypeK != "q4_0" || sp.CacheTypeV != "q4_0" {
		t.Errorf("CPU 后端 KV cache 应为 q4_0，实际 K=%s V=%s", sp.CacheTypeK, sp.CacheTypeV)
	}
	if sp.SpecType != "" {
		t.Errorf("CPU 后端应关闭所有推测解码，实际 SpecType=%s", sp.SpecType)
	}
	if sp.MmprojOffload != false {
		t.Errorf("CPU 后端应关闭 MmprojOffload，实际 %v", sp.MmprojOffload)
	}
}

// TestCalculateSmartParams_CUDA_NoRegression 验证 CUDA 后端行为与原 CalculateSmartParams 一致
// 传入 "cuda" 时，applyBackendSpecificParams 不做任何调整，保持默认行为
//
// 这是回归测试：确保 Task 5 的修改没有破坏 Task 1-4 已有的 CUDA 行为。
func TestCalculateSmartParams_CUDA_NoRegression(t *testing.T) {
	hw := &HardwareInfo{
		HasGPU:    true,
		GPUVRAMMB: 16303,
	}
	// 传入 "cuda"，应保持原行为
	sp := CalculateSmartParams(hw, "", "cuda")
	// CUDA 后端：Flash Attention 开启（有 GPU 时）
	if sp.FlashAttn != true {
		t.Errorf("CUDA 后端有 GPU 时应开启 Flash Attention，实际 FlashAttn=%v", sp.FlashAttn)
	}
	// CUDA 后端：非 MTP 模型应启用 ngram-mod
	if sp.SpecType != "ngram-mod" {
		t.Errorf("CUDA 后端非 MTP 模型应启用 ngram-mod，实际 SpecType=%s", sp.SpecType)
	}
	// CUDA 后端：GPULayers 应为 99（全部卸载）
	if sp.GPULayers != 99 {
		t.Errorf("CUDA 后端有 GPU 时 GPULayers 应为 99，实际 %d", sp.GPULayers)
	}
}

// TestApplyBackendSpecificParams_Vulkan_ClosesNgramMod 单独测试 Vulkan 关闭 ngram-mod
// 验证 Vulkan 后端会关闭已启用的 ngram-mod 推测解码，并限制 ctx-size 和 gpu_layers
// B-2/B-3 增强：Vulkan 后端 gpu_layers<=50, ctx-size<=8192
// 注意：mmproj_offload 不再强制关闭（日志证据：mmproj 不是栈溢出根因）
func TestApplyBackendSpecificParams_Vulkan_ClosesNgramMod(t *testing.T) {
	// 构造一个已启用 ngram-mod 的 SmartParams
	p := &SmartParams{
		SpecType:       "ngram-mod",
		NgramModNMin:   48,
		NgramModNMax:   64,
		NgramModNMatch: 24,
		FlashAttn:      true,
		ContextSize:    32768, // 超过 8192，验证会被限制
		GPULayers:      99,    // 超过 50，验证会被限制
		MmprojOffload:  true,  // 不再强制关闭，保持原值
	}
	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 16303}

	applyBackendSpecificParams(p, "vulkan", hw, nil)

	// 验证 ngram-mod 被关闭
	if p.SpecType != "" {
		t.Errorf("Vulkan 后端应关闭 ngram-mod，实际 SpecType=%s", p.SpecType)
	}
	if p.NgramModNMin != 0 || p.NgramModNMax != 0 || p.NgramModNMatch != 0 {
		t.Errorf("Vulkan 后端应清零 ngram-mod 参数，实际 NMin=%d NMax=%d NMatch=%d",
			p.NgramModNMin, p.NgramModNMax, p.NgramModNMatch)
	}
	// 验证 Flash Attention 被关闭
	if p.FlashAttn != false {
		t.Errorf("Vulkan 后端应关闭 Flash Attention，实际 FlashAttn=%v", p.FlashAttn)
	}
	// B-3：验证 ctx-size 被限制到 8192
	if p.ContextSize != 8192 {
		t.Errorf("Vulkan 后端 ctx-size 应被限制为 8192，实际 %d", p.ContextSize)
	}
	// B-2：验证 gpu_layers 被限制到 50
	if p.GPULayers != 50 {
		t.Errorf("Vulkan 后端 gpu_layers 应被限制为 50，实际 %d", p.GPULayers)
	}
	// mmproj_offload 不再强制关闭，验证保持原值
	if p.MmprojOffload != true {
		t.Errorf("Vulkan 后端应保持 mmproj_offload 原值，实际 %v", p.MmprojOffload)
	}
}

// TestApplyBackendSpecificParams_CPU_ClosesAllSpec 单独测试 CPU 关闭所有推测解码
// 验证 CPU 后端会关闭所有推测解码（含 MTP），并应用 CPU 专用配置
func TestApplyBackendSpecificParams_CPU_ClosesAllSpec(t *testing.T) {
	// 构造一个已启用 MTP 推测解码的 SmartParams
	p := &SmartParams{
		SpecType:        "draft-mtp",
		SpecDraftNMax:   3,
		CacheTypeKDraft: "q8_0",
		CacheTypeVDraft: "q8_0",
		GPULayers:       99,
		FlashAttn:       true,
		MmprojOffload:   true,
		ContextSize:     16384, // 超过 8192，验证会被限制
		CacheTypeK:      "q8_0",
		CacheTypeV:      "q4_0",
	}
	hw := &HardwareInfo{HasGPU: true, GPUVRAMMB: 16303}

	applyBackendSpecificParams(p, "cpu", hw, nil)

	// 验证推测解码被关闭
	if p.SpecType != "" {
		t.Errorf("CPU 后端应关闭所有推测解码，实际 SpecType=%s", p.SpecType)
	}
	if p.SpecDraftNMax != 0 {
		t.Errorf("CPU 后端应清零 SpecDraftNMax，实际 %d", p.SpecDraftNMax)
	}
	if p.CacheTypeKDraft != "" || p.CacheTypeVDraft != "" {
		t.Errorf("CPU 后端应清空 Draft cache 类型，实际 K=%s V=%s",
			p.CacheTypeKDraft, p.CacheTypeVDraft)
	}
	// 验证 GPU 相关参数被关闭
	if p.GPULayers != 0 {
		t.Errorf("CPU 后端 GPULayers 应为 0，实际 %d", p.GPULayers)
	}
	if p.FlashAttn != false {
		t.Errorf("CPU 后端应关闭 Flash Attention，实际 FlashAttn=%v", p.FlashAttn)
	}
	if p.MmprojOffload != false {
		t.Errorf("CPU 后端应关闭 MmprojOffload，实际 %v", p.MmprojOffload)
	}
	// 验证 ctx-size 被限制到 8192
	if p.ContextSize != 8192 {
		t.Errorf("CPU 后端 ctx-size 应被限制为 8192，实际 %d", p.ContextSize)
	}
	// 验证 KV cache 被设置为 q4_0
	if p.CacheTypeK != "q4_0" || p.CacheTypeV != "q4_0" {
		t.Errorf("CPU 后端 KV cache 应为 q4_0，实际 K=%s V=%s", p.CacheTypeK, p.CacheTypeV)
	}
}
