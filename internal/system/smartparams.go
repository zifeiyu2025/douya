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
}

func detectModelTier(resolvedModelPath string) ModelTier {
	meta, err := ParseGGUFMetadata(resolvedModelPath)
	if err != nil {
		log.Error().Err(err).Msg("[smart-params] GGUF parse failed, using unknown tier")
		return ModelTierUnknown
	}

	log.Info().Str("arch", meta.Architecture).Int("block_count", meta.BlockCount).Int("embedding_length", meta.EmbeddingLength).Msg("[smart-params] GGUF metadata")

	if meta.BlockCount <= 0 || meta.EmbeddingLength <= 0 {
		return ModelTierUnknown
	}

	return estimateModelTier(meta.BlockCount, meta.EmbeddingLength)
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
	p.CacheTypeK = "q8_0"
	p.CacheTypeV = "q4_0"
	p.Mlock = true
	p.MmprojOffload = hw.HasGPU

	p.ContextSize = calculateContextSize(hw, resolvedModelPath)
	p.BatchSize = calculateBatchSize(hw)
	p.UBatchSize = p.BatchSize / 2

	return p
}

func calculateContextSize(hw *HardwareInfo, resolvedModelPath string) int {
	tier := detectModelTier(resolvedModelPath)

	if !hw.HasGPU || hw.GPUVRAMMB <= 0 {
		return 4096
	}

	vramGB := float64(hw.GPUVRAMMB) / 1024.0

	switch tier {
	case ModelTierTiny:
		if vramGB >= 8 {
			return 32768
		}
		if vramGB >= 4 {
			return 16384
		}
		return 8192
	case ModelTierSmall:
		if vramGB >= 12 {
			return 32768
		}
		if vramGB >= 8 {
			return 16384
		}
		if vramGB >= 6 {
			return 8192
		}
		return 4096
	case ModelTierMedium:
		if vramGB >= 16 {
			return 32768
		}
		if vramGB >= 12 {
			return 16384
		}
		if vramGB >= 8 {
			return 8192
		}
		if vramGB >= 6 {
			return 4096
		}
		return 2048
	case ModelTierLarge:
		if vramGB >= 24 {
			return 16384
		}
		if vramGB >= 16 {
			return 8192
		}
		if vramGB >= 12 {
			return 4096
		}
		return 2048
	case ModelTierXL:
		if vramGB >= 24 {
			return 8192
		}
		if vramGB >= 16 {
			return 4096
		}
		return 2048
	default:
		if vramGB >= 12 {
			return 16384
		}
		if vramGB >= 8 {
			return 8192
		}
		return 4096
	}
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
