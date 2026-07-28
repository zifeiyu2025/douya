// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"strings"

	"github.com/rs/zerolog/log"
)

type ModelTier int

const (
	ModelTierTiny ModelTier = iota
	ModelTierSmall
	ModelTierMedium
	ModelTierLarge
	ModelTierXL
	ModelTierUnknown
)

type SmartParams struct {
	GPULayers       int
	Threads         int
	BatchSize       int
	UBatchSize      int
	FlashAttn       bool
	CacheTypeK      string
	CacheTypeV      string
	Mlock           bool
	MmprojOffload   bool
	ContextSize     int
	SpecType        string
	SpecDraftNMax   int
	SpecDraftNMin   int
	CacheTypeKDraft string
	CacheTypeVDraft string
	NgramModNMin    int
	NgramModNMax    int
	NgramModNMatch  int
	SupportsEagle3  bool // 模型支持 Eagle3 推测解码（需用户配置 draft 模型才启用）
	// 推理模式自动推荐
	ReasoningMode   string // "on"/"off"/"auto"，检测到推理模型时自动设置为 "auto"
	ReasoningBudget int    // 推理 token 预算，-1=无限（默认），0=立即结束，N>0=预算
}

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

// CalculateSmartParams 根据硬件信息和模型元数据计算智能参数
// backendType 参数为已解析的后端类型（"cuda"/"hip"/"sycl"/"vulkan"/"openvino"/"cpu"），
// 不会是 "auto"，由调用方解析。空字符串时按 CUDA 行为处理（保持向后兼容）。
func CalculateSmartParams(hw *HardwareInfo, resolvedModelPath string, backendType string) *SmartParams {
	p := &SmartParams{}

	// GPU 层数：有完整 GPU 信息时全部卸载；仅检测到 CUDA 驱动时也尝试全部卸载
	// 生活类比：即使仪表盘坏了（nvidia-smi 失败），只要发动机还在（nvcuda.dll），
	// 仍然挂最高档（99层），让引擎自己决定能跑多快
	if hw.HasGPU || hw.HasCUDABackend {
		p.GPULayers = 99
	} else {
		p.GPULayers = 0
	}

	physicalCores := hw.CPUCores / 2
	if physicalCores < 1 {
		physicalCores = hw.CPUCores
	}
	p.Threads = max(physicalCores-2, 2)

	// Flash Attention：有 GPU 或 CUDA 后端时开启
	p.FlashAttn = hw.HasGPU || hw.HasCUDABackend
	_, meta := DetectModelTier(resolvedModelPath)
	p.CacheTypeK, p.CacheTypeV = calculateCacheTypes(hw, meta)
	p.Mlock = true
	p.MmprojOffload = hw.HasGPU || hw.HasCUDABackend

	p.ContextSize = calculateContextSize(hw, resolvedModelPath)
	p.BatchSize = calculateBatchSizeFromRatio(hw, meta)
	p.UBatchSize = p.BatchSize / 2

	// Eagle3 支持标志传递（实际启用在 app.go buildServerConfig 中根据用户配置的 draft 模型决定）
	if meta != nil && meta.SupportsEagle3 {
		p.SupportsEagle3 = true
	}

	// 推理模式自动推荐：检测到推理模型时自动设置 reasoning=auto
	// 生活类比：就像汽车检测到驾驶员系了安全带就自动启用辅助驾驶一样
	if meta != nil && meta.HasReasoning {
		p.ReasoningMode = "auto"
		p.ReasoningBudget = -1 // 无限（由模型自行决定思考时长）
		log.Info().Str("reasoning", p.ReasoningMode).Msg("[smart-params] reasoning model detected, auto-setting reasoning=auto")
	}

	if meta != nil && meta.HasMTP {
		// 检查 VRAM 是否有足够余量支持 MTP（draft heads 额外消耗显存）
		if hw.HasGPU && hw.GPUVRAMMB > 0 && meta.FileSize > 0 {
			vramBytes := float64(hw.GPUVRAMMB) * 1024 * 1024
			modelBytes := float64(meta.FileSize)
			ratio := modelBytes / vramBytes
			if meta.ExpertCount > 0 && meta.ExpertUsed > 0 {
				ratio = ratio * float64(meta.ExpertUsed) / float64(meta.ExpertCount)
			}
			if ratio > 0.8 {
				log.Warn().Float64("ratio", ratio).Msg("[smart-params] VRAM headroom insufficient for MTP, skipping auto-enable")
				return p
			}
			// 根据 VRAM 比率动态缩减上下文大小，为 MTP draft KV cache 预留显存
			if ratio > 0.5 {
				p.ContextSize = int(float64(p.ContextSize) * 0.8) // 缩减 20%
				log.Info().Float64("ratio", ratio).Int("ctx_reduced", p.ContextSize).Msg("[smart-params] MTP: context size reduced 20% for VRAM headroom")
			} else {
				p.ContextSize = int(float64(p.ContextSize) * 0.9) // 缩减 10%
				log.Info().Float64("ratio", ratio).Int("ctx_reduced", p.ContextSize).Msg("[smart-params] MTP: context size reduced 10% for VRAM headroom")
			}
		}

		p.SpecType = "draft-mtp"
		p.CacheTypeKDraft = "q8_0"
		p.CacheTypeVDraft = "q8_0"
		// MTP + 思考模型：降低 draft token 数量，减少跨越思考/正文边界的 token 回退风险
		if meta.HasReasoning {
			p.SpecDraftNMax = 2
			log.Info().Msg("[smart-params] MTP + reasoning model detected, reducing SpecDraftNMax to 2")
		} else {
			p.SpecDraftNMax = 3
		}
	} else if hw.HasGPU || hw.HasCUDABackend {
		// 非 MTP 模型：自动启用 ngram_mod 推测解码加速（无需 MTP 头，任何模型可用）
		// 但需要检查条件：显存太紧张或模型太小则跳过
		shouldEnableNgram := true
		if meta != nil && meta.FileSize > 0 && hw.GPUVRAMMB > 0 {
			vramBytes := float64(hw.GPUVRAMMB) * 1024 * 1024
			modelBytes := float64(meta.FileSize)
			ratio := modelBytes / vramBytes
			if meta.ExpertCount > 0 && meta.ExpertUsed > 0 {
				ratio = ratio * float64(meta.ExpertUsed) / float64(meta.ExpertCount)
			}
			if ratio > 0.85 {
				shouldEnableNgram = false
				log.Info().Float64("ratio", ratio).Msg("[smart-params] VRAM too tight for ngram-mod, skipping")
			}
			if meta.NParams > 0 && meta.NParams < 3_000_000_000 {
				shouldEnableNgram = false
				log.Info().Int64("n_params", meta.NParams).Msg("[smart-params] model too small for ngram-mod, skipping")
			}
		}
		if shouldEnableNgram {
			p.SpecType = "ngram-mod"
			p.NgramModNMin = 48
			p.NgramModNMax = 64
			p.NgramModNMatch = 24
			log.Info().Msg("[smart-params] non-MTP model with GPU detected, auto-enabling ngram_mod speculative decoding")
		}
	}

	// 架构特定参数调整（DeepSeek32/GLM4/Cohere2MoE 等新架构）
	applyArchSpecificParams(p, meta)

	// 后端特定参数调整（CUDA/HIP/SYCL/Vulkan/OpenVINO/CPU）
	// 生活类比：架构调整是按车型调，后端调整是按燃料类型调
	applyBackendSpecificParams(p, backendType, hw, meta)

	return p
}

// applyArchSpecificParams 根据模型架构应用特定的参数调整
// 生活类比：就像不同品牌的汽车需要不同标号的汽油，不同架构的模型也有各自的最佳参数配置
func applyArchSpecificParams(p *SmartParams, meta *GGUFMetadata) {
	if meta == nil || meta.Architecture == "" {
		return
	}
	lowerArch := strings.ToLower(meta.Architecture)

	switch {
	// DeepSeek32 / DeepSeek-V3：大型 MoE 推理模型
	// 确保上下文窗口足够大以发挥推理能力
	case strings.Contains(lowerArch, "deepseek3") || strings.Contains(lowerArch, "deepseek-v3"):
		if p.ContextSize < 8192 {
			p.ContextSize = 8192
		}
		log.Info().Str("arch", meta.Architecture).Msg("[smart-params] DeepSeek32 detected, ensuring context >= 8192")

	// GLM4 / ChatGLM4：支持推理的通用模型
	case strings.Contains(lowerArch, "glm4") || strings.Contains(lowerArch, "chatglm4"):
		log.Info().Str("arch", meta.Architecture).Msg("[smart-params] GLM4 architecture detected")

	// Cohere2MoE / tiny-aya：Cohere MoE 多语言模型
	case strings.Contains(lowerArch, "cohere2moe") || strings.Contains(lowerArch, "tiny-aya"):
		log.Info().Str("arch", meta.Architecture).Msg("[smart-params] Cohere2MoE architecture detected")
	}
}

// applyBackendSpecificParams 根据后端类型调整智能参数
// 生活类比：就像不同燃料（柴油/汽油/电）需要不同的发动机参数，不同 GPU 后端也有各自的最佳配置
//
// 重要约束：传入 "cuda" 时行为必须与原 CalculateSmartParams 完全一致（无回归）。
// backendType 为已解析值（不会是 "auto"），空字符串按 CUDA 行为处理（安全）。
func applyBackendSpecificParams(p *SmartParams, backendType string, hw *HardwareInfo, meta *GGUFMetadata) {
	switch backendType {
	case "cuda", "":
		// CUDA 后端：保持现有逻辑，不做额外调整
		// 所有 CUDA 特定优化已在 CalculateSmartParams 主体中处理
		// 空字符串（未初始化）也走此分支，保持默认行为（安全）
	case "hip":
		// AMD HIP 后端：类似 CUDA 但无 NVFP4、无架构特定优化
		// Flash Attention 和 ngram-mod 保持与 CUDA 相同的行为
	case "sycl":
		// Intel SYCL 后端：类似 CUDA 但无 NVFP4
	case "vulkan":
		// Vulkan 后端：保守配置（防止栈溢出崩溃 0xC0000409）
		// 背景：llama.cpp Vulkan 后端对 gpu_layers=99（全层卸载）和较大 ctx-size 兼容性较差，
		// 加载 Gemma4 等模型时触发 STATUS_STACK_BUFFER_OVERRUN 崩溃。
		// 日志证据：移除 mmproj 后仍崩溃，说明根因是 gpu_layers 过大，不是 mmproj_offload。
		// 生活类比：Vulkan 后端像一辆载重有限的货车，不能装太多货物（gpu_layers），
		// 也不能开太快（ctx-size），否则会爆胎（栈溢出）。

		// Flash Attention 在 Vulkan 上支持有限，默认关闭
		p.FlashAttn = false

		// ngram-mod 推测解码在 Vulkan 上兼容性未知，关闭
		if p.SpecType == "ngram-mod" {
			p.SpecType = ""
			p.NgramModNMin = 0
			p.NgramModNMax = 0
			p.NgramModNMatch = 0
		}

		// MTP 推测解码在 Vulkan 上可能不兼容，关闭
		if p.SpecType == "draft-mtp" {
			p.SpecType = ""
			p.SpecDraftNMax = 0
			p.CacheTypeKDraft = ""
			p.CacheTypeVDraft = ""
		}

		// gpu_layers 保守限制：不超过 50
		// 原因：全层卸载（99）在 Vulkan 上容易导致栈溢出崩溃（日志已验证）
		// 50 层仍能覆盖大部分模型的关键层，兼顾性能和稳定性
		if p.GPULayers > 50 {
			p.GPULayers = 50
		}

		// ctx-size 保守限制：不超过 8192
		// 原因：16384 在 Vulkan + Gemma4 组合下仍会栈溢出，收紧到 8192
		if p.ContextSize > 8192 {
			p.ContextSize = 8192
		}

		// mmproj_offload：允许开启（由用户配置决定）
		// 日志证据：移除 mmproj 后仍崩溃，说明 mmproj_offload 不是栈溢出根因
		// gpu_layers<=50 和 ctx-size<=8192 已提供足够的栈溢出保护
		// 用户可通过 config.json 的 mmproj_offload=false 来关闭

		log.Info().Msg("[smart-params] Vulkan backend detected, applying conservative config (flash off, spec off, ngl<=50, ctx<=8192)")
	case "openvino":
		// OpenVINO 后端：Intel 专用，类似 SYCL
	case "cpu":
		// CPU 后端：纯 CPU 模式
		p.GPULayers = 0
		p.FlashAttn = false
		p.MmprojOffload = false
		// 关闭所有推测解码
		p.SpecType = ""
		p.NgramModNMin = 0
		p.NgramModNMax = 0
		p.NgramModNMatch = 0
		p.SpecDraftNMax = 0
		p.CacheTypeKDraft = ""
		p.CacheTypeVDraft = ""
		// ctx-size 保守限制：不超过 8192
		if p.ContextSize > 8192 {
			p.ContextSize = 8192
		}
		// CPU 模式下 KV cache 使用 q4_0 节省内存
		p.CacheTypeK = "q4_0"
		p.CacheTypeV = "q4_0"
		log.Info().Msg("[smart-params] CPU backend detected, applying CPU config (no GPU offload, q4_0 cache, ctx<=8192)")
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
			// 显存不足以加载模型，返回最小上下文
			return 2048
		}

		// KV cache 每token 显存消耗
		kvCostPerToken := estimateKVCostPerToken(meta, cacheK, cacheV)
		if kvCostPerToken <= 0 {
			// 无法估算，回退到查表法
			return calculateContextSizeFallback(tier, hw, meta)
		}

		// 反推最大上下文长度
		maxCtx := int(availableForKV / float64(kvCostPerToken))

		// 对齐到 256 的整数倍
		maxCtx = max((maxCtx/256)*256, 2048)

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
