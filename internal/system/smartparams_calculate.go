// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"github.com/rs/zerolog/log"
)

func DetectModelTier(resolvedModelPath string) (ModelTier, *GGUFMetadata) {
	meta, err := ParseGGUFMetadataCached(resolvedModelPath)
	if err != nil {
		log.Error().Err(err).Msg("[smart-params] GGUF parse failed, using unknown tier")
		return ModelTierUnknown, nil
	}

	log.Info().Str("arch", meta.Architecture).Int("block_count", meta.BlockCount).Int("embedding_length", meta.EmbeddingLength).Int("context_length", meta.ContextLength).Msg("[smart-params] GGUF metadata")

	if meta.BlockCount <= 0 || meta.EmbeddingLength <= 0 {
		return ModelTierUnknown, meta
	}

	return estimateModelTier(meta.BlockCount, meta.EmbeddingLength), meta
}

func estimateModelTier(blockCount, embeddingLength int) ModelTier {
	score := blockCount * embeddingLength / 1000

	switch {
	case score < 80:
		return ModelTierTiny
	case score < 160:
		return ModelTierSmall
	case score < 300:
		return ModelTierMedium
	case score < 500:
		return ModelTierLarge
	default:
		return ModelTierXL
	}
}

func calculateContextSize(hw *HardwareInfo, resolvedModelPath string) int {
	tier, meta := DetectModelTier(resolvedModelPath)

	if !hw.HasGPU || hw.GPUVRAMMB <= 0 {
		// 有 CUDA 后端但无 VRAM 信息时，使用保守的 8192
		if hw.HasCUDABackend {
			return 8192
		}
		return 4096
	}

	vramBytes := float64(hw.GPUVRAMMB) * 1024 * 1024

	// 如果有 GGUF 元数据，尝试基于显存预算精确计算上下文长度
	if meta != nil && meta.FileSize > 0 && meta.BlockCount > 0 {
		// 先确定 cache-type
		cacheK, cacheV := calculateCacheTypes(hw, meta)

		// 模型权重占用
		modelBytes := float64(meta.FileSize)
		// MoE 模型：mmap 模式下实际内存占用更少，但显存中仍需加载激活参数
		if meta.ExpertCount > 0 && meta.ExpertUsed > 0 {
			modelBytes = modelBytes * float64(meta.ExpertUsed) / float64(meta.ExpertCount)
		}

		// 安全余量 15%（给临时缓冲区、CUDA 内核等）
		safetyMargin := 0.15
		availableForKV := vramBytes*(1.0-safetyMargin) - modelBytes

		if availableForKV <= 0 {
			// 显存不足以容纳模型权重，返回最小上下文（512）。
			// P4.2 修复：此前返回 2048 会超出显存预算，KV cache 分配时仍会 OOM
			//（崩溃降级链的 ctx 减半也救不回来——2048 就是它的下限）。
			// 512 是"能对话但很紧"的保守值，配合 llama-server 的 gpu-layers auto
			// 尽量减小 KV 占用；若仍 OOM，崩溃降级链会进一步把 gpu-layers 设为 auto。
			// 生活类比：油箱只够跑 10 公里就别硬设 40 公里的续航，先把目的地调近。
			log.Warn().
				Float64("vram_gb", vramBytes/1024/1024/1024).
				Float64("model_gb", modelBytes/1024/1024/1024).
				Msg("[smart-params] model does not fit in VRAM, using minimum ctx=512")
			return 512
		}

		// KV cache 每token 显存消耗
		kvCostPerToken := estimateKVCostPerToken(meta, cacheK, cacheV)
		if kvCostPerToken <= 0 {
			// 无法估算，回退到查表法
			return calculateContextSizeFallback(tier, hw, meta)
		}

		// 反推最大上下文长度
		maxCtx := int(availableForKV / float64(kvCostPerToken))

		// P4.2 修复：对齐到 256 的整数倍，但下限改为 512（而非 2048）。
		// 2048 的下限在显存极紧时会超出预算，512 是更安全的兜底。
		// 若对齐后仍不足 512，说明可用显存确实非常有限，直接用 512。
		maxCtx = max((maxCtx/256)*256, 512)

		// 不超过模型原生上下文长度
		if meta.ContextLength > 0 && maxCtx > meta.ContextLength {
			maxCtx = meta.ContextLength
		}

		// 不超过 131072（128K）
		if maxCtx > 131072 {
			maxCtx = 131072
		}

		// Blackwell 架构（RTX 50 系）保守限制：ctx-size 上限 32768
		// 原因：llama.cpp 在 Blackwell 上的 CUDA kernel 可能对超大上下文有兼容性问题，
		// 96K 上下文加载时触发 STATUS_STACK_BUFFER_OVERRUN 崩溃。
		// 32K 足够日常对话使用，等 llama.cpp 修复后可放宽。
		// 生活类比：新车刚上市时先开慢点，等厂家召回修复后再跑高速。
		if hw.GPUArchitecture == "Blackwell" && maxCtx > 32768 {
			log.Warn().Int("original_ctx", maxCtx).Int("capped_ctx", 32768).
				Msg("[smart-params] Blackwell architecture detected, capping ctx-size to 32768 for stability")
			maxCtx = 32768
		}

		log.Info().
			Float64("vram_gb", vramBytes/1024/1024/1024).
			Float64("model_gb", modelBytes/1024/1024/1024).
			Float64("available_kv_gb", availableForKV/1024/1024/1024).
			Int64("kv_cost_per_token", kvCostPerToken).
			Int("max_ctx", maxCtx).
			Msg("[smart-params] context size calculated from VRAM budget")

		return maxCtx
	}

	// 无 GGUF 元数据，回退到查表法
	return calculateContextSizeFallback(tier, hw, meta)
}

// calculateContextSizeFallback 查表法计算上下文长度（无 GGUF 元数据时的回退方案）
func calculateContextSizeFallback(tier ModelTier, hw *HardwareInfo, meta *GGUFMetadata) int {
	vramGB := float64(hw.GPUVRAMMB) / 1024.0

	var vramBased int
	switch tier {
	case ModelTierTiny:
		if vramGB >= 8 {
			vramBased = 32768
		} else if vramGB >= 4 {
			vramBased = 16384
		} else {
			vramBased = 8192
		}
	case ModelTierSmall:
		if vramGB >= 12 {
			vramBased = 32768
		} else if vramGB >= 8 {
			vramBased = 16384
		} else if vramGB >= 6 {
			vramBased = 8192
		} else {
			vramBased = 4096
		}
	case ModelTierMedium:
		if vramGB >= 16 {
			vramBased = 32768
		} else if vramGB >= 12 {
			vramBased = 16384
		} else if vramGB >= 8 {
			vramBased = 8192
		} else if vramGB >= 6 {
			vramBased = 4096
		} else {
			vramBased = 2048
		}
	case ModelTierLarge:
		if vramGB >= 24 {
			vramBased = 16384
		} else if vramGB >= 16 {
			vramBased = 8192
		} else if vramGB >= 12 {
			vramBased = 4096
		} else {
			vramBased = 2048
		}
	case ModelTierXL:
		if vramGB >= 24 {
			vramBased = 8192
		} else if vramGB >= 16 {
			vramBased = 4096
		} else {
			vramBased = 2048
		}
	default:
		if vramGB >= 12 {
			vramBased = 16384
		} else if vramGB >= 8 {
			vramBased = 8192
		} else {
			vramBased = 4096
		}
	}

	if meta != nil && meta.ContextLength > 0 {
		if vramBased > meta.ContextLength {
			vramBased = meta.ContextLength
		}
	}

	if meta != nil && meta.ExpertCount > 0 && vramBased < 4096 {
		vramBased = 4096
	}

	// Blackwell 架构保守限制（与精确计算路径保持一致）
	if hw.GPUArchitecture == "Blackwell" && vramBased > 32768 {
		log.Warn().Int("original_ctx", vramBased).Int("capped_ctx", 32768).
			Msg("[smart-params] Blackwell architecture detected (fallback path), capping ctx-size to 32768 for stability")
		vramBased = 32768
	}

	return vramBased
}

func calculateBatchSize(hw *HardwareInfo) int {
	if !hw.HasGPU {
		return 64
	}

	vramGB := float64(hw.GPUVRAMMB) / 1024.0

	switch {
	case vramGB >= 16:
		return 1024
	case vramGB >= 12:
		return 512
	case vramGB >= 8:
		return 512
	case vramGB >= 6:
		return 256
	case vramGB >= 4:
		return 128
	default:
		return 64
	}
}

// calculateBatchSizeFromRatio 根据模型占用比例计算 batch size
func calculateBatchSizeFromRatio(hw *HardwareInfo, meta *GGUFMetadata) int {
	if !hw.HasGPU || hw.GPUVRAMMB <= 0 {
		// 有 CUDA 后端但无 VRAM 信息时，使用保守的 512
		if hw.HasCUDABackend {
			return 512
		}
		return 64
	}

	// 如果没有元数据，回退到仅基于 VRAM 的计算
	if meta == nil || meta.FileSize <= 0 {
		return calculateBatchSize(hw)
	}

	vramBytes := float64(hw.GPUVRAMMB) * 1024 * 1024
	modelBytes := float64(meta.FileSize)
	ratio := modelBytes / vramBytes
	if meta.ExpertCount > 0 && meta.ExpertUsed > 0 {
		ratio = ratio * float64(meta.ExpertUsed) / float64(meta.ExpertCount)
	}

	switch {
	case ratio <= 0.5:
		return 1024
	case ratio <= 0.7:
		return 512
	case ratio <= 0.85:
		return 256
	default:
		return 128
	}
}

// cacheTypeSize 返回每种量化类型每个元素占用的字节数
// 仅包含 llama-server --cache-type-k/v 支持的类型：
// f32, f16, bf16, q8_0, q4_0, q4_1, iq4_nl, q5_0, q5_1
// 注意：nvfp4 是模型权重量化类型，不是 KV cache 类型，不在此列表中
func cacheTypeSize(ct string) float64 {
	switch ct {
	case "f32":
		return 4.0
	case "bf16":
		return 2.0
	case "f16":
		return 2.0
	case "q8_0":
		return 1.0
	case "q5_1":
		return 0.75
	case "q5_0":
		return 0.6875
	case "q4_1":
		return 0.625
	case "q4_0":
		return 0.5625
	case "iq4_nl":
		return 0.5
	default:
		return 0.5625 // 默认 q4_0
	}
}

// estimateKVCostPerToken 估算 KV cache 每个token的显存消耗（字节）
// KV cache = 2 (K+V) * num_layers * head_dim * num_kv_heads * sizeof(cache_type)
func estimateKVCostPerToken(meta *GGUFMetadata, cacheTypeK, cacheTypeV string) int64 {
	if meta == nil || meta.BlockCount <= 0 {
		return 0
	}

	// head_dim：优先使用 GGUF 元数据，否则从 embedding_length 推算
	headDim := meta.HeadDimKV
	if headDim <= 0 {
		headDim = meta.EmbeddingLength
		// embedding_length 是 n_embd（总嵌入维度），不是 head_dim
		// 对于 GQA 模型，head_dim 通常为 128 或 256
		// 当 n_embd > 256 时，几乎可以确定它不是 head_dim，回退为最常见的 128
		// 注意：不能用 n_embd%128==0 作为判据，非标准维度（如 1440）会导致 headDim
		// 保持为 n_embd 值，严重高估 KV cache 成本（见 TDD 修复）
		if headDim > 256 {
			headDim = 128
		}
	}

	// num_kv_heads：优先使用 GGUF 元数据
	kvHeads := meta.KVHeadCount
	if kvHeads <= 0 {
		// 无法确定，使用保守估计
		// 假设 head_dim=128, kv_heads = embedding_length / 128
		if headDim > 0 {
			kvHeads = meta.EmbeddingLength / headDim
		}
		if kvHeads <= 0 {
			kvHeads = 32 // 保守默认值
		}
	}

	// K cache 每token: num_layers * head_dim * num_kv_heads * sizeof(cache_type_k)
	kCost := float64(meta.BlockCount) * float64(headDim) * float64(kvHeads) * cacheTypeSize(cacheTypeK)
	// V cache 每token: num_layers * head_dim * num_kv_heads * sizeof(cache_type_v)
	vCost := float64(meta.BlockCount) * float64(headDim) * float64(kvHeads) * cacheTypeSize(cacheTypeV)

	return int64(kCost + vCost)
}

func calculateCacheTypes(hw *HardwareInfo, meta *GGUFMetadata) (string, string) {
	if !hw.HasGPU || hw.GPUVRAMMB <= 0 {
		// nvidia-smi 检测失败但有 CUDA 后端时，使用保守的 q8_0/q4_0
		// 因为不知道 VRAM 大小，不能激进压缩
		if hw.HasCUDABackend {
			return "q8_0", "q4_0"
		}
		return "q4_0", "q4_0"
	}

	if meta == nil || meta.FileSize <= 0 {
		return "q8_0", "q4_0"
	}

	vramBytes := float64(hw.GPUVRAMMB) * 1024 * 1024
	modelBytes := float64(meta.FileSize)
	ratio := modelBytes / vramBytes
	// MoE 模型：按激活参数折算模型权重占用，但 KV cache 不折算
	if meta.ExpertCount > 0 && meta.ExpertUsed > 0 {
		ratio = ratio * float64(meta.ExpertUsed) / float64(meta.ExpertCount)
	}

	// KV cache 量化策略（所有架构统一）
	// 注意：nvfp4 是模型权重量化类型，不是 KV cache 类型，不能用于 --cache-type-k/v。
	// llama.cpp 源码 common/arg.cpp 的 kv_cache_types 仅支持：
	// f32, f16, bf16, q8_0, q4_0, q4_1, iq4_nl, q5_0, q5_1
	switch {
	case ratio <= 0.5:
		// 显存充裕，最高精度
		return "q8_0", "q8_0"
	case ratio <= 0.7:
		// 显存较充裕，平衡精度
		return "q8_0", "q4_0"
	case ratio <= 0.85:
		// 显存紧张，压缩 K
		return "q4_0", "q4_0"
	default:
		// 显存很紧张，激进压缩（q4_1 比 q4_0 略大但精度更好，iq4_nl 是最低可用）
		return "q4_1", "iq4_nl"
	}
}
