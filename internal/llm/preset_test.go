package llm

import (
	"strings"
	"testing"
)

// TestDeriveModelName 测试模型名称推导
func TestDeriveModelName(t *testing.T) {
	tests := []struct {
		filename string
		want     string
	}{
		// 带量化后缀的情况，应被移除
		{"Qwen2.5-7B-Instruct-Q4_0", "Qwen2.5-7B-Instruct"},
		{"gemma-2-9b-it-bf16", "gemma-2-9b-it"},
		{"llama-3.1-8b-instruct-Q4_K_M", "llama-3.1-8b-instruct"},
		// 无量化后缀，名称保持不变
		{"model-name", "model-name"},
		// 下划线应被替换为连字符
		{"my_model_name", "my-model-name"},
	}

	for _, tt := range tests {
		t.Run(tt.filename, func(t *testing.T) {
			got := DeriveModelName(tt.filename)
			if got != tt.want {
				t.Errorf("DeriveModelName(%q) = %q, want %q", tt.filename, got, tt.want)
			}
		})
	}
}

// TestStripQuantSuffix 测试量化后缀移除
func TestStripQuantSuffix(t *testing.T) {
	tests := []struct {
		name  string // 输入的模型名称
		quant string // 量化格式描述（仅用于子测试名称）
		want  string // 移除后缀后的结果
	}{
		{"model-Q4_0", "Q4_0", "model"},
		{"model-Q4_K_M", "Q4_K_M", "model"},
		{"model-Q8_0", "Q8_0", "model"},
		{"model-IQ3_M", "IQ3_M", "model"},
		{"model-BF16", "BF16", "model"},
		{"model-F16", "F16", "model"},
		{"model-F32", "F32", "model"},
		// 无量化后缀，应保持不变
		{"model-name", "无量化后缀", "model-name"},
	}

	for _, tt := range tests {
		t.Run(tt.quant, func(t *testing.T) {
			got := StripQuantSuffix(tt.name)
			if got != tt.want {
				t.Errorf("StripQuantSuffix(%q) = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

// TestSetDefaultAlias 测试设置默认别名
func TestSetDefaultAlias(t *testing.T) {
	t.Run("匹配ModelPath时设置别名", func(t *testing.T) {
		presets := []ModelPreset{
			{Name: "model-a", ModelPath: "models/a.gguf", Alias: "old"},
			{Name: "model-b", ModelPath: "models/b.gguf", Alias: "default"},
		}
		// 传入匹配第二个模型的路径
		SetDefaultAlias(presets, "models/b.gguf")

		// 所有旧别名应被清除
		if presets[0].Alias != "" {
			t.Errorf("presets[0].Alias = %q, 期望为空", presets[0].Alias)
		}
		// 匹配的模型应被设为 default
		if presets[1].Alias != "default" {
			t.Errorf("presets[1].Alias = %q, 期望 default", presets[1].Alias)
		}
	})

	t.Run("无匹配时第一个模型作为默认", func(t *testing.T) {
		presets := []ModelPreset{
			{Name: "model-a", ModelPath: "models/a.gguf"},
			{Name: "model-b", ModelPath: "models/b.gguf"},
		}
		// 传入不匹配任何模型的路径
		SetDefaultAlias(presets, "models/not-exist.gguf")

		// 第一个模型应被设为 default
		if presets[0].Alias != "default" {
			t.Errorf("presets[0].Alias = %q, 期望 default", presets[0].Alias)
		}
		if presets[1].Alias != "" {
			t.Errorf("presets[1].Alias = %q, 期望为空", presets[1].Alias)
		}
	})

	t.Run("空列表不panic", func(t *testing.T) {
		presets := []ModelPreset{}
		// 空列表不应 panic
		SetDefaultAlias(presets, "models/any.gguf")
	})
}

// TestGeneratePreset 测试预设文件生成
func TestGeneratePreset(t *testing.T) {
	t.Run("生成的字符串包含模型名和路径", func(t *testing.T) {
		presets := []ModelPreset{
			{Name: "Qwen2.5-7B-Instruct", ModelPath: "models/qwen.gguf"},
		}
		result := GeneratePreset(presets, nil)

		// 应包含模型名作为节标题
		if !strings.Contains(result, "[Qwen2.5-7B-Instruct]") {
			t.Error("生成结果应包含模型名作为节标题")
		}
		// 应包含模型路径
		if !strings.Contains(result, "model = models/qwen.gguf") {
			t.Error("生成结果应包含模型路径")
		}
	})

	t.Run("全局默认值部分", func(t *testing.T) {
		presets := []ModelPreset{
			{Name: "test-model", ModelPath: "models/test.gguf"},
		}
		globalDefaults := map[string]string{
			"flash-attn": "on",
			"ctx-size":   "4096",
		}
		result := GeneratePreset(presets, globalDefaults)

		// 应包含全局默认值节标题
		if !strings.Contains(result, "[*]") {
			t.Error("生成结果应包含全局默认值节标题 [*]")
		}
		// 应包含全局默认键值对
		if !strings.Contains(result, "flash-attn = on") {
			t.Error("生成结果应包含 flash-attn 全局默认值")
		}
		if !strings.Contains(result, "ctx-size = 4096") {
			t.Error("生成结果应包含 ctx-size 全局默认值")
		}
	})
}

// TestExtractKeywords 测试关键词提取
func TestExtractKeywords(t *testing.T) {
	t.Run("Qwen2.5-7B-Instruct", func(t *testing.T) {
		keywords := extractKeywords("Qwen2.5-7B-Instruct")

		// 应提取出有意义的词段（长度大于1）
		expected := []string{"Qwen2.5", "7B", "Instruct"}
		if len(keywords) != len(expected) {
			t.Errorf("提取到 %d 个关键词, 期望 %d 个: got %v, want %v",
				len(keywords), len(expected), keywords, expected)
			return
		}
		for i, kw := range keywords {
			if kw != expected[i] {
				t.Errorf("keywords[%d] = %q, 期望 %q", i, kw, expected[i])
			}
		}
	})
}
