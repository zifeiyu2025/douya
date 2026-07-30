// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"github.com/rs/zerolog/log"
)

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
