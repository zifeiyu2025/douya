// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
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
	GPULayers     int
	Threads       int
	BatchSize     int
	UBatchSize    int
	FlashAttn     bool
	CacheTypeK    string
	CacheTypeV    string
	Mlock         bool
	MmprojOffload bool
	ContextSize   int
	SpecType         string
	SpecDraftNMax    int
	CacheTypeKDraft  string
	CacheTypeVDraft  string
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

func CalculateSmartParams(hw *HardwareInfo, resolvedModelPath string) *SmartParams {
	p := &SmartParams{}

	if hw.HasGPU {
		p.GPULayers = 99
	} else {
		p.GPULayers = 0
	}

	physicalCores := hw.CPUCores / 2
	if physicalCores < 1 {
		physicalCores = hw.CPUCores
	}
	p.Threads = physicalCores - 2
	if p.Threads < 2 {
		p.Threads = 2
	}

	p.FlashAttn = hw.HasGPU
	_, meta := DetectModelTier(resolvedModelPath)
	p.CacheTypeK, p.CacheTypeV = calculateCacheTypes(hw, meta)
	p.Mlock = true
	p.MmprojOffload = hw.HasGPU

	p.ContextSize = calculateContextSize(hw, resolvedModelPath)
	p.BatchSize = calculateBatchSize(hw)
	p.UBatchSize = p.BatchSize / 2

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
	}

	return p
}

func calculateContextSize(hw *HardwareInfo, resolvedModelPath string) int {
	tier, meta := DetectModelTier(resolvedModelPath)

	if !hw.HasGPU || hw.GPUVRAMMB <= 0 {
		return 4096
	}

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

func calculateCacheTypes(hw *HardwareInfo, meta *GGUFMetadata) (string, string) {
	if !hw.HasGPU || hw.GPUVRAMMB <= 0 {
		return "q4_0", "q4_0"
	}

	if meta == nil || meta.FileSize <= 0 {
		return "q8_0", "q4_0"
	}

	vramBytes := float64(hw.GPUVRAMMB) * 1024 * 1024
	modelBytes := float64(meta.FileSize)
	ratio := modelBytes / vramBytes
	if meta.ExpertCount > 0 && meta.ExpertUsed > 0 {
		ratio = ratio * float64(meta.ExpertUsed) / float64(meta.ExpertCount)
	}

	switch {
	case ratio <= 0.7:
		return "q8_0", "q4_0"
	case ratio <= 1.5:
		return "q8_0", "turbo3"
	default:
		return "turbo3", "turbo2"
	}
}
