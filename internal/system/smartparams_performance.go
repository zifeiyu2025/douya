// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"github.com/rs/zerolog/log"
)

// PerformanceMode 性能模式常量
// 生活类比：就像汽车的驾驶模式按钮——ECO/COMFORT/SPORT，
// 每个模式对应一组预设的发动机、变速箱、转向参数组合。
const (
	// PerformanceModeCompatible 兼容模式：保守配置，适合排查问题或首次运行未知模型
	// 策略：不强制全层卸载、关闭 Flash Attention、小上下文、关闭推测解码
	PerformanceModeCompatible = "compatible"
	// PerformanceModeBalanced 平衡模式（默认）：兼顾性能与稳定性
	// 策略：按智能参数推荐（CUDA 全层卸载、Flash Attention、ngram-mod/MTP 自动）
	PerformanceModeBalanced = "balanced"
	// PerformanceModePerformance 性能模式：榨干硬件性能
	// 策略：强制全层卸载、拉满上下文（受 VRAM 限制）、强制开启推测解码
	PerformanceModePerformance = "performance"
)

// IsValidPerformanceMode 校验性能模式字符串合法性（空字符串合法，运行时按 balanced 处理）
func IsValidPerformanceMode(mode string) bool {
	switch mode {
	case PerformanceModeCompatible, PerformanceModeBalanced, PerformanceModePerformance, "":
		return true
	}
	return false
}

// resolvePerformanceMode 将空字符串解析为默认 balanced，非法值也回退到 balanced
func resolvePerformanceMode(mode string) string {
	switch mode {
	case PerformanceModeCompatible, PerformanceModeBalanced, PerformanceModePerformance:
		return mode
	default:
		// 空字符串或非法值按 balanced 处理（向后兼容旧配置）
		return PerformanceModeBalanced
	}
}

// applyPerformanceMode 根据性能模式调整智能参数。
// 在所有智能参数计算（包括架构/后端特定调整）之后应用，是最后的"模式级"覆盖。
//
// 三档模式策略：
//   - compatible：保守优先。不强制全层卸载（让 llama.cpp 自决）、关闭 Flash Attention、
//     上下文限制到 4096、关闭推测解码。用于排查问题或首次运行未知模型。
//   - balanced：保持当前智能参数不变（智能计算已按硬件给出合理推荐）。
//   - performance：强制激进。全层卸载、Flash Attention 开、上下文拉满（受 VRAM 限制）、
//     强制开启推测解码（如果模型支持且后端允许）。
//
// 重要约束：性能模式不应降低后端硬限制的安全性。
// 例如 Vulkan 后端的 gpu_layers<=50 限制在 applyBackendSpecificParams 中已应用，
// 此处 performance 模式不应突破该限制（安全 > 性能）。
// 对于保守后端（Vulkan/CPU/SYCL），performance 模式不强制开启推测解码，
// 因为这些后端在 applyBackendSpecificParams 中已主动关闭推测解码（兼容性原因）。
//
// 生活类比：驾驶模式不能让货车（Vulkan 后端）跑出跑车（CUDA 后端）的速度，
// 但可以让跑车在 SPORT 模式下跑得更快。
func applyPerformanceMode(p *SmartParams, mode string, hw *HardwareInfo, meta *GGUFMetadata, backendType string) {
	resolved := resolvePerformanceMode(mode)
	if resolved == PerformanceModeBalanced {
		// 平衡模式：保持现有智能参数不变
		return
	}

	if resolved == PerformanceModeCompatible {
		// 兼容模式：保守优先
		// GPULayers 设为 0 表示"让 llama.cpp 自决"
		// resolveDerivedServerParams 中 sp.GPULayers=0 且 cfg.GPULayers=0 时，
		// derived.GPULayers 会是 "auto"，llama-server 收到 --gpu-layers auto
		p.GPULayers = 0
		p.FlashAttn = false
		// 上下文限制到 4096（保守值，适合排查问题）
		if p.ContextSize > 4096 {
			p.ContextSize = 4096
		}
		// 关闭所有推测解码（减少变量，便于排查）
		p.SpecType = ""
		p.NgramModNMin = 0
		p.NgramModNMax = 0
		p.NgramModNMatch = 0
		p.SpecDraftNMax = 0
		p.CacheTypeKDraft = ""
		p.CacheTypeVDraft = ""
		log.Info().Msg("[smart-params] compatible mode: conservative config applied (ngl=auto, flash off, ctx<=4096, spec off)")
		return
	}

	// performance 模式：强制激进配置
	// 注意：只在 GPU 可用时才激进，CPU 后端不强制（避免无意义）
	if hw.HasGPU || hw.HasCUDABackend {
		// 全层卸载：仅在非保守后端上强制 99
		// 保守后端（Vulkan/CPU/SYCL/OpenVINO）在 applyBackendSpecificParams 中已限制层数，
		// performance 模式不应突破该安全限制（安全 > 性能）
		isConservativeBackend := backendType == "vulkan" || backendType == "cpu" || backendType == "sycl" || backendType == "openvino"
		if !isConservativeBackend && p.GPULayers < 99 {
			p.GPULayers = 99
		}
		// Flash Attention：CUDA/HIP 后端强制开启，保守后端保持 applyBackendSpecificParams 的设置
		if !isConservativeBackend {
			p.FlashAttn = true
		}
		// 上下文拉满：不主动限制，由 calculateContextSize 的 VRAM 预算决定
		// （如果 VRAM 不足，calculateContextSize 已经给出了合理值，这里不覆盖）
		// 但如果当前 ctx 被某个保守逻辑限制到较小值，performance 模式尝试翻倍
		// 前提是 VRAM 有余量（通过 meta 和 hw 检查）
		if meta != nil && meta.FileSize > 0 && hw.GPUVRAMMB > 0 {
			vramBytes := float64(hw.GPUVRAMMB) * 1024 * 1024
			modelBytes := float64(meta.FileSize)
			if meta.ExpertCount > 0 && meta.ExpertUsed > 0 {
				modelBytes = modelBytes * float64(meta.ExpertUsed) / float64(meta.ExpertCount)
			}
			// 模型占用 VRAM 不到 70% 时，尝试拉满上下文
			if modelBytes/vramBytes < 0.7 && p.ContextSize < 16384 {
				// 拉满到 16384（保守的"性能"值，不直接拉到 32768+ 避免栈溢出风险）
				p.ContextSize = 16384
				log.Info().Msg("[smart-params] performance mode: context size boosted to 16384 (VRAM headroom sufficient)")
			}
		}
		// 强制开启推测解码：仅在非保守后端上执行
		// 保守后端（Vulkan/CPU/SYCL）在 applyBackendSpecificParams 中已关闭推测解码，
		// performance 模式不应覆盖该安全调整（避免后端不兼容导致崩溃）
		if p.SpecType == "" && !isConservativeBackend {
			if meta != nil && meta.HasMTP {
				p.SpecType = "draft-mtp"
				p.CacheTypeKDraft = "q8_0"
				p.CacheTypeVDraft = "q8_0"
				if meta.HasReasoning {
					p.SpecDraftNMax = 2
				} else {
					p.SpecDraftNMax = 3
				}
				log.Info().Msg("[smart-params] performance mode: forced MTP speculative decoding on")
			} else if (hw.HasGPU || hw.HasCUDABackend) && (meta == nil || meta.NParams >= 3_000_000_000) {
				// 非 MTP 模型且参数量 >= 3B，强制开启 ngram-mod
				p.SpecType = "ngram-mod"
				p.NgramModNMin = 48
				p.NgramModNMax = 64
				p.NgramModNMatch = 24
				log.Info().Msg("[smart-params] performance mode: forced ngram-mod speculative decoding on")
			}
		} else if isConservativeBackend {
			log.Info().Str("backend", backendType).Msg("[smart-params] performance mode: skip forcing speculative decoding on conservative backend")
		}
		log.Info().Msg("[smart-params] performance mode: aggressive config applied (ngl=99, flash on, spec forced)")
	} else {
		// 无 GPU 时 performance 模式无意义，降级为 balanced 行为
		log.Info().Msg("[smart-params] performance mode ignored on CPU-only system, fallback to balanced")
	}
}
