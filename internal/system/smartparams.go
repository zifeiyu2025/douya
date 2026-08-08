// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"runtime"

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

// CalculateSmartParams 根据硬件信息和模型元数据计算智能参数
// backendType 参数为已解析的后端类型（"cuda"/"hip"/"sycl"/"vulkan"/"openvino"/"cpu"），
// 不会是 "auto"，由调用方解析。空字符串时按 CUDA 行为处理（保持向后兼容）。
// performanceMode 参数为性能模式（"compatible"/"balanced"/"performance"/""），
// 空字符串按 "balanced" 处理（向后兼容旧调用方）。
func CalculateSmartParams(hw *HardwareInfo, resolvedModelPath string, backendType string, performanceMode string) *SmartParams {
	p := &SmartParams{}

	// P4.4 修复：nil-hw 防护。此前 hw 被无条件解引用（hw.HasGPU 等），
	// 调用方传入 nil 会 panic。正常流程保证 hwInfo 非 nil（startup 必检），
	// 但这是公开 API，防御性兜底为"无 GPU 的保守配置"。
	if hw == nil {
		log.Warn().Msg("[smart-params] nil HardwareInfo, using conservative CPU defaults")
		p.GPULayers = 0
		p.Threads = max(runtime.NumCPU()/2, 2)
		p.FlashAttn = false
		p.Mlock = false
		p.MmprojOffload = false
		p.CacheTypeK = ""
		p.CacheTypeV = ""
		p.ContextSize = 4096
		p.BatchSize = 512
		p.UBatchSize = 256
		return p
	}

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
				// 注意：此处不提前 return，继续走架构/后端/性能模式调整
				log.Warn().Float64("ratio", ratio).Msg("[smart-params] VRAM headroom insufficient for MTP, skipping auto-enable")
			} else {
				// 根据 VRAM 比率动态缩减上下文大小，为 MTP draft KV cache 预留显存
				if ratio > 0.5 {
					p.ContextSize = int(float64(p.ContextSize) * 0.8) // 缩减 20%
					log.Info().Float64("ratio", ratio).Int("ctx_reduced", p.ContextSize).Msg("[smart-params] MTP: context size reduced 20% for VRAM headroom")
				} else {
					p.ContextSize = int(float64(p.ContextSize) * 0.9) // 缩减 10%
					log.Info().Float64("ratio", ratio).Int("ctx_reduced", p.ContextSize).Msg("[smart-params] MTP: context size reduced 10% for VRAM headroom")
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
			}
		} else {
			// 无 VRAM 信息时仍尝试启用 MTP（由后端/性能模式后续调整）
			p.SpecType = "draft-mtp"
			p.CacheTypeKDraft = "q8_0"
			p.CacheTypeVDraft = "q8_0"
			if meta.HasReasoning {
				p.SpecDraftNMax = 2
			} else {
				p.SpecDraftNMax = 3
			}
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

	// 性能模式调整（compatible/balanced/performance）
	// 必须在架构/后端调整之后应用，是最后的"模式级"覆盖
	// 生活类比：选完车型和燃料，最后选驾驶模式（ECO/COMFORT/SPORT）
	applyPerformanceMode(p, performanceMode, hw, meta, backendType)

	return p
}
