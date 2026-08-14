// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"testing"

	"douya/internal/llm"
)

// TestMatchModelKeywords_Qwen3SoftSwitch 验证 Qwen3 系列匹配 Template 模式 + 软开关
// 生活类比：就像机场安检的 VIP 通道清单，Qwen3 系列在清单最前面，
// 匹配后直接走 VIP 通道（Template 模式）并获得软开关特权（/think /no_think）。
func TestMatchModelKeywords_Qwen3SoftSwitch(t *testing.T) {
	configs := []modelKeywordConfig{
		{keywords: []string{"qwen3", "qwen3moe"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: true},
		{keywords: []string{"gemma4"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: false},
	}
	cases := []string{
		"qwen3-7b",
		"qwen3moe-15b",
		"qwen3-instruct", // 实际调用前已转小写
	}
	for _, target := range cases {
		mode, reasoning, soft := matchModelKeywords(target, configs)
		if mode != llm.ThinkingModeTemplate {
			t.Errorf("matchModelKeywords(%q) mode = %q, 期望 %q", target, mode, llm.ThinkingModeTemplate)
		}
		if !reasoning {
			t.Errorf("matchModelKeywords(%q) reasoning 应为 true", target)
		}
		if !soft {
			t.Errorf("matchModelKeywords(%q) soft 应为 true（Qwen3 支持软开关）", target)
		}
	}
}

// TestMatchModelKeywords_Gemma4NoSoftSwitch 验证 Gemma4 匹配 Template 但无软开关
func TestMatchModelKeywords_Gemma4NoSoftSwitch(t *testing.T) {
	configs := []modelKeywordConfig{
		{keywords: []string{"qwen3"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: true},
		{keywords: []string{"gemma4", "gemma-4"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: false},
	}
	mode, reasoning, soft := matchModelKeywords("gemma4-12b-it", configs)
	if mode != llm.ThinkingModeTemplate {
		t.Errorf("gemma4 mode = %q, 期望 %q", mode, llm.ThinkingModeTemplate)
	}
	if !reasoning {
		t.Errorf("gemma4 reasoning 应为 true")
	}
	if soft {
		t.Errorf("gemma4 soft 应为 false（Gemma4 不支持软开关）")
	}
}

// TestMatchModelKeywords_DeepSeekReasoning 验证 DeepSeek 匹配 Reasoning 模式
func TestMatchModelKeywords_DeepSeekReasoning(t *testing.T) {
	configs := []modelKeywordConfig{
		{keywords: []string{"qwen3"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: true},
		{keywords: []string{"deepseek3", "deepseek-v3"}, thinkingMode: llm.ThinkingModeReasoning, softSwitch: false},
	}
	cases := []string{
		"deepseek3-67b",
		"deepseek-v3-chat",
	}
	for _, target := range cases {
		mode, reasoning, soft := matchModelKeywords(target, configs)
		if mode != llm.ThinkingModeReasoning {
			t.Errorf("matchModelKeywords(%q) mode = %q, 期望 %q", target, mode, llm.ThinkingModeReasoning)
		}
		if !reasoning {
			t.Errorf("matchModelKeywords(%q) reasoning 应为 true", target)
		}
		if soft {
			t.Errorf("matchModelKeywords(%q) soft 应为 false", target)
		}
	}
}

// TestMatchModelKeywords_NoMatch 验证未匹配时返回 None
func TestMatchModelKeywords_NoMatch(t *testing.T) {
	configs := []modelKeywordConfig{
		{keywords: []string{"qwen3"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: true},
		{keywords: []string{"gemma4"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: false},
	}
	cases := []string{
		"llama2-7b",
		"mistral-7b",
		"",
		"unknown-model",
	}
	for _, target := range cases {
		mode, reasoning, soft := matchModelKeywords(target, configs)
		if mode != llm.ThinkingModeNone {
			t.Errorf("matchModelKeywords(%q) mode = %q, 期望 %q", target, mode, llm.ThinkingModeNone)
		}
		if reasoning {
			t.Errorf("matchModelKeywords(%q) reasoning 应为 false", target)
		}
		if soft {
			t.Errorf("matchModelKeywords(%q) soft 应为 false", target)
		}
	}
}

// TestMatchModelKeywords_EmptyConfigs 验证空配置列表返回 None
func TestMatchModelKeywords_EmptyConfigs(t *testing.T) {
	mode, reasoning, soft := matchModelKeywords("qwen3-7b", nil)
	if mode != llm.ThinkingModeNone {
		t.Errorf("空配置列表 mode 应为 None，实际: %q", mode)
	}
	if reasoning || soft {
		t.Errorf("空配置列表 reasoning 和 soft 应为 false")
	}
}

// TestMatchModelKeywords_PriorityFirstMatch 验证按优先级匹配（第一个匹配的配置生效）
// 如果 qwen3 在第一个配置中，即使后面的配置也有匹配关键词，也应返回第一个配置的结果
func TestMatchModelKeywords_PriorityFirstMatch(t *testing.T) {
	// qwen3 在第一个配置中是 Template+softSwitch
	// qwen3 也在第二个配置中是 Reasoning（模拟配置错误）
	// 应返回第一个匹配的结果
	configs := []modelKeywordConfig{
		{keywords: []string{"qwen3"}, thinkingMode: llm.ThinkingModeTemplate, softSwitch: true},
		{keywords: []string{"qwen3"}, thinkingMode: llm.ThinkingModeReasoning, softSwitch: false},
	}
	mode, _, soft := matchModelKeywords("qwen3-7b", configs)
	if mode != llm.ThinkingModeTemplate {
		t.Errorf("优先级匹配：应返回第一个配置的 mode %q，实际: %q", llm.ThinkingModeTemplate, mode)
	}
	if !soft {
		t.Errorf("优先级匹配：应返回第一个配置的 soft=true")
	}
}

// TestThinkingModeFromTemplate 验证模板内容分析判定思考模式。
//
// 生活类比：冰箱的"制冷"开关决定能否制冷，但"速冻"档位则决定制冷强度。
// 这里根据模板里的不同标记，判断模型的思考是"可按需开关"（Template）
// 还是"固定推理"（Reasoning）。
func TestThinkingModeFromTemplate(t *testing.T) {
	tests := []struct {
		name       string
		template   string
		wantMode   string
		wantReason bool
		wantSoft   bool
	}{
		{"空模板", "", llm.ThinkingModeNone, false, false},
		{"普通模板无思考标记", "{{- range .Messages }}{{.Role}}{{.Content}}{{ end }}", llm.ThinkingModeNone, false, false},
		{"含 <|think|> 标记", "{%- if enable_thinking %}<|think|>{%- endif %}", llm.ThinkingModeTemplate, true, true},
		{"含 enable_thinking 开关", "{%- if enable_thinking %}{%- endif %}", llm.ThinkingModeTemplate, true, true},
		{"含 enable_think 开关", "{%- if enable_think %}{%- endif %}", llm.ThinkingModeTemplate, true, true},
		{"含 startthinking 指令", "{%- if startthinking %}startthinking{%- endif %}", llm.ThinkingModeTemplate, true, false},
		{"含 reasoning_effort 参数", "Reasoning Effort: {{ reasoning_effort }}", llm.ThinkingModeReasoning, true, false},
		{"含 reasoning_content 字段", "{{ reasoning_content }}", llm.ThinkingModeReasoning, true, false},
		{"含 <|reasoning_start|> 标记", "<|reasoning_start|>...<|reasoning_end|>", llm.ThinkingModeReasoning, true, false},
		{"大小写不敏感", "ENABLE_THINKING", llm.ThinkingModeTemplate, true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mode, reason, soft := thinkingModeFromTemplate(tt.template)
			if mode != tt.wantMode {
				t.Errorf("thinkingModeFromTemplate(%q) mode = %q, 期望 %q", tt.template, mode, tt.wantMode)
			}
			if reason != tt.wantReason {
				t.Errorf("thinkingModeFromTemplate(%q) reasoning = %v, 期望 %v", tt.template, reason, tt.wantReason)
			}
			if soft != tt.wantSoft {
				t.Errorf("thinkingModeFromTemplate(%q) soft = %v, 期望 %v", tt.template, soft, tt.wantSoft)
			}
		})
	}
}
