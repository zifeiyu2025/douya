// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import (
	"strings"

	"github.com/rs/zerolog/log"
)

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
