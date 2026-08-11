// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"strings"
	"testing"
)

// TestIsCoderModel 验证编程类模型名识别。
func TestIsCoderModel(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"qwen2.5-coder-7b", true},
		{"qwen3-coder-30b", true},
		{"codestral-latest", true},
		{"deepseek-coder-6.7b", true},
		{"starcoder2-7b", true},
		{"CodexCl-34b", true},
		{"qwen2.5-7b-instruct", false},
		{"llama-3-8b", false},
		{"本地模型", false},
		{"", false},
	}
	for _, c := range cases {
		if got := isCoderModel(c.name); got != c.want {
			t.Errorf("isCoderModel(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestResolveProgrammingMode 验证编程模式解析逻辑。
func TestResolveProgrammingMode(t *testing.T) {
	cases := []struct {
		mode, model string
		want        bool
	}{
		{"on", "llama-3-8b", true},      // 强制开启
		{"off", "qwen-coder", false},    // 强制关闭覆盖自动检测
		{"auto", "qwen2.5-coder", true}, // 自动检测命中
		{"auto", "llama-3-8b", false},   // 自动检测未命中
		{"", "qwen2.5-coder", true},     // 空值走自动检测（向后兼容）
		{"", "llama-3-8b", false},
	}
	for _, c := range cases {
		if got := resolveProgrammingMode(c.mode, c.model); got != c.want {
			t.Errorf("resolveProgrammingMode(%q, %q) = %v, want %v", c.mode, c.model, got, c.want)
		}
	}
}

// TestBuildCoderSystemPrompt 验证编程版提示词包含编程强化指令。
func TestBuildCoderSystemPrompt(t *testing.T) {
	p := buildCoderSystemPrompt("qwen-coder", "")

	// 应声明编程身份
	if !strings.Contains(p, "编程助手模式") {
		t.Errorf("编程提示词应声明编程助手模式身份")
	}
	// 编程行为准则关键指令
	for _, frag := range []string{"完整可运行示例", "测试", "调试", "重构", "多文件结构", "编码风格"} {
		if !strings.Contains(p, frag) {
			t.Errorf("编程提示词应包含 %q 指令", frag)
		}
	}
}

// TestBuildCoderSystemPrompt_CapabilityOverride 验证 Agent 模式能力边界覆盖生效。
func TestBuildCoderSystemPrompt_CapabilityOverride(t *testing.T) {
	override := "在 Agent 模式下，你可通过内置工具执行文件读写、shell 命令等操作"
	p := buildCoderSystemPrompt("qwen-coder", override)

	if !strings.Contains(p, override) {
		t.Errorf("能力边界覆盖未生效，提示词应包含 %q", override)
	}
	// 覆盖后不应再出现通用版"无法执行代码/访问文件系统"的矛盾表述
	if strings.Contains(p, "无法执行代码、访问文件系统") {
		t.Errorf("Agent 覆盖下不应残留'无法执行代码/访问文件系统'的矛盾表述")
	}
}

// TestBuildDefaultSystemPrompt_GenericKeepsBoundary 验证通用版能力边界不受编程版影响。
func TestBuildDefaultSystemPrompt_GenericKeepsBoundary(t *testing.T) {
	p := buildDefaultSystemPrompt("llama-3-8b", false, "")
	if !strings.Contains(p, "无法执行代码、访问文件系统") {
		t.Errorf("通用版应保留'无法执行代码/访问文件系统'的能力边界")
	}
}
