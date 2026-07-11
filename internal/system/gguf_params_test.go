// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package system

import "testing"

// TestParseScale_KnownScales 验证已知缩放字母对应的倍率
//
// 生活类比：就像度量衡前缀，k=千、M=百万、G=十亿，
// parseScale 把字母翻译成数字倍率。
func TestParseScale_KnownScales(t *testing.T) {
	cases := []struct {
		input string
		want  float64
	}{
		{"k", 1e3},
		{"K", 1e3},
		{"m", 1e6},
		{"M", 1e6},
		{"g", 1e9},
		{"G", 1e9},
		{"t", 1e12},
		{"T", 1e12},
		{"q", 1e15},
		{"Q", 1e15},
	}
	for _, c := range cases {
		got := parseScale(c.input)
		if got != c.want {
			t.Errorf("parseScale(%q) = %v, 期望 %v", c.input, got, c.want)
		}
	}
}

// TestParseScale_Default 验证未知字母默认返回 1e9（G 级别）
// 这是因为大多数 LLM 都是几十亿参数级别，默认 G 是合理的兜底
func TestParseScale_Default(t *testing.T) {
	cases := []string{"", "x", "b", "B"}
	for _, s := range cases {
		got := parseScale(s)
		if got != 1e9 {
			t.Errorf("parseScale(%q) 默认应返回 1e9，实际: %v", s, got)
		}
	}
}

// TestEstimateNParamsFromSizeLabel_Simple 验证简单 size_label 解析
// 格式：数字+单位（如 "7B"、"13B"、"70B"）
func TestEstimateNParamsFromSizeLabel_Simple(t *testing.T) {
	cases := []struct {
		name      string
		sizeLabel string
		want      int64
	}{
		{"7B", "7B", 7_000_000_000},
		{"13B", "13B", 13_000_000_000},
		{"70B", "70B", 70_000_000_000},
		{"0.5B", "0.5B", 500_000_000},
		{"1.5B", "1.5B", 1_500_000_000},
		{"7b 小写", "7b", 7_000_000_000},
		// 注意：正则要求以 b/B 结尾，7M/7K 不匹配，返回 0
		// 7MB 不是有效的 size_label 格式
		{"7MB", "7MB", 7_000_000},
		{"7KB", "7KB", 7_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EstimateNParamsFromSizeLabel(c.sizeLabel)
			if got != c.want {
				t.Errorf("EstimateNParamsFromSizeLabel(%q) = %d, 期望 %d", c.sizeLabel, got, c.want)
			}
		})
	}
}

// TestEstimateNParamsFromSizeLabel_MoE 验证 MoE 模型的 size_label 解析
// 格式：专家数x每专家参数+单位（如 "60x3B"、"8x7B"）
func TestEstimateNParamsFromSizeLabel_MoE(t *testing.T) {
	cases := []struct {
		name      string
		sizeLabel string
		want      int64
	}{
		{"8x7B", "8x7B", 56_000_000_000},
		{"60x3B", "60x3B", 180_000_000_000},
		{"4x0.5B", "4x0.5B", 2_000_000_000},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EstimateNParamsFromSizeLabel(c.sizeLabel)
			if got != c.want {
				t.Errorf("EstimateNParamsFromSizeLabel(%q) = %d, 期望 %d", c.sizeLabel, got, c.want)
			}
		})
	}
}

// TestEstimateNParamsFromSizeLabel_Empty 验证空字符串返回 0
func TestEstimateNParamsFromSizeLabel_Empty(t *testing.T) {
	got := EstimateNParamsFromSizeLabel("")
	if got != 0 {
		t.Errorf("空字符串应返回 0，实际: %d", got)
	}
}

// TestEstimateNParamsFromSizeLabel_Invalid 验证无效格式返回 0
func TestEstimateNParamsFromSizeLabel_Invalid(t *testing.T) {
	cases := []string{
		"invalid",
		"7",
		"B",
		"7BB",
		"abc7B",
	}
	for _, s := range cases {
		got := EstimateNParamsFromSizeLabel(s)
		if got != 0 {
			t.Errorf("EstimateNParamsFromSizeLabel(%q) 无效格式应返回 0，实际: %d", s, got)
		}
	}
}

// TestResolveNParams_ServerPriority 验证 server API 返回的参数优先级最高
func TestResolveNParams_ServerPriority(t *testing.T) {
	meta := &GGUFMetadata{
		NParams:   7_000_000_000,
		SizeLabel: "7B",
	}
	// serverNParams > 0 时应直接返回，忽略 GGUF 元数据
	got := ResolveNParams(13_000_000_000, meta)
	if got != 13_000_000_000 {
		t.Errorf("serverNParams 优先级最高，期望 13B，实际: %v", got)
	}
}

// TestResolveNParams_GGUFNParams 验证 GGUF 元数据的 NParams 作为第二优先级
func TestResolveNParams_GGUFNParams(t *testing.T) {
	meta := &GGUFMetadata{
		NParams:   7_000_000_000,
		SizeLabel: "13B", // NParams 优先于 SizeLabel
	}
	got := ResolveNParams(0, meta)
	if got != 7_000_000_000 {
		t.Errorf("GGUF NParams 应作为第二优先级，期望 7B，实际: %v", got)
	}
}

// TestResolveNParams_SizeLabelFallback 验证 SizeLabel 作为最后兜底
func TestResolveNParams_SizeLabelFallback(t *testing.T) {
	meta := &GGUFMetadata{
		NParams:   0,
		SizeLabel: "7B",
	}
	got := ResolveNParams(0, meta)
	if got != 7_000_000_000 {
		t.Errorf("SizeLabel 应作为兜底，期望 7B，实际: %v", got)
	}
}

// TestResolveNParams_NilMeta 验证 nil 元数据返回 0
func TestResolveNParams_NilMeta(t *testing.T) {
	got := ResolveNParams(0, nil)
	if got != 0 {
		t.Errorf("nil 元数据应返回 0，实际: %v", got)
	}
}

// TestResolveNParams_AllZero 验证所有来源都为 0 时返回 0
func TestResolveNParams_AllZero(t *testing.T) {
	meta := &GGUFMetadata{
		NParams:   0,
		SizeLabel: "",
	}
	got := ResolveNParams(0, meta)
	if got != 0 {
		t.Errorf("所有来源都为 0 时应返回 0，实际: %v", got)
	}
}
