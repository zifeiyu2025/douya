// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"encoding/json"
	"fmt"
	"strings"

	"douya/internal/config"

	zlog "github.com/rs/zerolog/log"
)

// ModelParams 存储每个模型专属的生成参数。
//
// 设计说明：
//   - 只包含"生成参数"子集（采样、推理、上下文等），不含全局设置（api_base、port 等）
//   - 与 config.Config 中的同名字段一一对应，便于双向同步
//   - 序列化为 JSON 存储在 settings KV 表中，key 格式：model_params:{model_name}
//
// 生活类比：就像每个员工有各自的"办公偏好卡片"——座椅高度、灯光亮度、桌面布局，
// 换工位时把卡片拿出来按自己的习惯调整，不用每次重新调一遍。
type ModelParams struct {
	// ===== 采样参数 =====
	Temperature    float64 `json:"temperature"`
	TopP           float64 `json:"top_p"`
	TopK           int     `json:"top_k"`
	RepeatPenalty  float64 `json:"repeat_penalty"`
	MinP           float64 `json:"min_p"`
	Samplers       string  `json:"samplers"`
	IgnoreEos      bool    `json:"ignore_eos"`
	AdaptiveTarget float64 `json:"adaptive_target"`
	AdaptiveDecay  float64 `json:"adaptive_decay"`

	// ===== 上下文 =====
	ContextSize                int     `json:"context_size"`
	ProactiveCompressThreshold float64 `json:"proactive_compress_threshold"`

	// ===== 推理参数 =====
	Reasoning         string `json:"reasoning"`
	ReasoningEffort   string `json:"reasoning_effort"`
	ReasoningBudget   int    `json:"reasoning_budget"`
	ReasoningFormat   string `json:"reasoning_format"`
	ReasoningPreserve *bool  `json:"reasoning_preserve,omitempty"`

	// ===== Dry 采样 =====
	DryMultiplier      float64 `json:"dry_multiplier"`
	DryBase            float64 `json:"dry_base"`
	DryAllowedLength   int     `json:"dry_allowed_length"`
	DrySequenceBreaker string  `json:"dry_sequence_breaker"`
	DryPenaltyLastN    int     `json:"dry_penalty_last_n"`

	// ===== 分组注意力 =====
	GrpAttnN int `json:"grp_attn_n"`
	GrpAttnW int `json:"grp_attn_w"`

	// ===== 图像 token =====
	ImageMinTokens int `json:"image_min_tokens"`
	ImageMaxTokens int `json:"image_max_tokens"`
}

// modelParamsKeyPrefix 是 settings KV 表中模型参数的 key 前缀。
const modelParamsKeyPrefix = "model_params:"

// modelParamsKey 构造指定模型的 KV key。
func modelParamsKey(modelName string) string {
	return modelParamsKeyPrefix + modelName
}

// GetModelParams 读取指定模型的生成参数。
// 返回 nil 表示该模型未保存过参数（调用方应回退到全局 Config 默认值）。
//
// 生活类比：去档案柜找某个模型的"偏好卡片"，没有就是没存过，用默认设置即可。
func (s *Service) GetModelParams(modelName string) (*ModelParams, error) {
	if modelName == "" {
		return nil, nil
	}
	raw, err := s.GetSetting(modelParamsKey(modelName))
	if err != nil {
		return nil, fmt.Errorf("读取模型参数失败: %w", err)
	}
	if raw == "" {
		return nil, nil // 未保存过，不是错误
	}
	var params ModelParams
	if err := json.Unmarshal([]byte(raw), &params); err != nil {
		zlog.Warn().Err(err).Str("model", modelName).Msg("[model_params] JSON 解析失败，回退到默认值")
		return nil, nil // 解析失败按"未保存"处理，不阻塞切换
	}
	return &params, nil
}

// SetModelParams 保存指定模型的生成参数。
//
// 生活类比：把调整好的参数写成"偏好卡片"存入该模型的档案。
func (s *Service) SetModelParams(modelName string, params *ModelParams) error {
	if modelName == "" {
		return fmt.Errorf("模型名不能为空")
	}
	if params == nil {
		return s.ClearModelParams(modelName)
	}
	data, err := json.Marshal(params)
	if err != nil {
		return fmt.Errorf("序列化模型参数失败: %w", err)
	}
	return s.SetSetting(modelParamsKey(modelName), string(data))
}

// ClearModelParams 清除指定模型的生成参数。
//
// 生活类比：把该模型的"偏好卡片"从档案柜中抽出来扔掉，下次切换回到全局默认。
func (s *Service) ClearModelParams(modelName string) error {
	if modelName == "" {
		return fmt.Errorf("模型名不能为空")
	}
	// settings 表没有 DELETE API，用空字符串表示"已清除"
	// GetModelParams 遇到空字符串会返回 nil，语义一致
	return s.SetSetting(modelParamsKey(modelName), "")
}

// ModelParamsFromConfig 从全局 Config 提取生成参数子集，构造 ModelParams。
// 用于"保存当前参数为模型预设"功能。
//
// 生活类比：从大柜子里只拿出"生成参数"这层抽屉的东西，装进模型的偏好卡片。
func ModelParamsFromConfig(cfg *config.Config) *ModelParams {
	if cfg == nil {
		return nil
	}
	return &ModelParams{
		Temperature:                cfg.Temperature,
		TopP:                       cfg.TopP,
		TopK:                       cfg.TopK,
		RepeatPenalty:              cfg.RepeatPenalty,
		MinP:                       cfg.MinP,
		Samplers:                   cfg.Samplers,
		IgnoreEos:                  cfg.IgnoreEos,
		AdaptiveTarget:             cfg.AdaptiveTarget,
		AdaptiveDecay:              cfg.AdaptiveDecay,
		ContextSize:                cfg.ContextSize,
		ProactiveCompressThreshold: cfg.ProactiveCompressThreshold,
		Reasoning:                  cfg.Reasoning,
		ReasoningEffort:            cfg.ReasoningEffort,
		ReasoningBudget:            cfg.ReasoningBudget,
		ReasoningFormat:            cfg.ReasoningFormat,
		ReasoningPreserve:          cfg.ReasoningPreserve,
		DryMultiplier:              cfg.DryMultiplier,
		DryBase:                    cfg.DryBase,
		DryAllowedLength:           cfg.DryAllowedLength,
		DrySequenceBreaker:         cfg.DrySequenceBreaker,
		DryPenaltyLastN:            cfg.DryPenaltyLastN,
		GrpAttnN:                   cfg.GrpAttnN,
		GrpAttnW:                   cfg.GrpAttnW,
		ImageMinTokens:             cfg.ImageMinTokens,
		ImageMaxTokens:             cfg.ImageMaxTokens,
	}
}

// ApplyToConfig 将 ModelParams 合并应用到全局 Config（只覆盖生成参数字段）。
// 用于模型切换时恢复该模型的参数设置。
//
// 生活类比：把模型的"偏好卡片"内容逐项写到全局配置板上，只改生成参数部分。
func (p *ModelParams) ApplyToConfig(cfg *config.Config) {
	if p == nil || cfg == nil {
		return
	}
	cfg.Temperature = p.Temperature
	cfg.TopP = p.TopP
	cfg.TopK = p.TopK
	cfg.RepeatPenalty = p.RepeatPenalty
	cfg.MinP = p.MinP
	cfg.Samplers = p.Samplers
	cfg.IgnoreEos = p.IgnoreEos
	cfg.AdaptiveTarget = p.AdaptiveTarget
	cfg.AdaptiveDecay = p.AdaptiveDecay
	cfg.ContextSize = p.ContextSize
	cfg.ProactiveCompressThreshold = p.ProactiveCompressThreshold
	cfg.Reasoning = p.Reasoning
	cfg.ReasoningEffort = p.ReasoningEffort
	cfg.ReasoningBudget = p.ReasoningBudget
	cfg.ReasoningFormat = p.ReasoningFormat
	cfg.ReasoningPreserve = p.ReasoningPreserve
	cfg.DryMultiplier = p.DryMultiplier
	cfg.DryBase = p.DryBase
	cfg.DryAllowedLength = p.DryAllowedLength
	cfg.DrySequenceBreaker = p.DrySequenceBreaker
	cfg.DryPenaltyLastN = p.DryPenaltyLastN
	cfg.GrpAttnN = p.GrpAttnN
	cfg.GrpAttnW = p.GrpAttnW
	cfg.ImageMinTokens = p.ImageMinTokens
	cfg.ImageMaxTokens = p.ImageMaxTokens
}

// HasModelParams 检查指定模型是否已保存过生成参数。
// 用于前端显示"已保存"状态标记。
func (s *Service) HasModelParams(modelName string) bool {
	if modelName == "" {
		return false
	}
	raw, err := s.GetSetting(modelParamsKey(modelName))
	return err == nil && strings.TrimSpace(raw) != ""
}
