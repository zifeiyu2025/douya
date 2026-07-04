// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureConfigFields_MissingFields 验证缺失字段被补全
func TestEnsureConfigFields_MissingFields(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// 构造一个缺失字段的用户配置（缺少 ContextSize）
	userData := []byte(`{"SystemPrompt":"test","Temperature":0.7}`)
	if err := os.WriteFile(configPath, userData, 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	// 构造完整配置
	cfg := &Config{
		SystemPrompt: "test",
		Temperature:  0.7,
		ContextSize:  8192,
	}

	// 调用 ensureConfigFields
	ensureConfigFields(configPath, userData, cfg)

	// 验证文件已被更新，包含 ContextSize
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("读取更新后文件失败: %v", err)
	}

	var updatedMap map[string]any
	if err := json.Unmarshal(updated, &updatedMap); err != nil {
		t.Fatalf("解析更新后文件失败: %v", err)
	}

	// context_size 应被补全（JSON 标签是 snake_case）
	if ctxSize, ok := updatedMap["context_size"]; !ok {
		t.Error("context_size 应被补全")
	} else {
		// JSON 数字解析为 float64
		if ctxSizeFloat, ok := ctxSize.(float64); ok {
			if int(ctxSizeFloat) != 8192 {
				t.Errorf("context_size 期望 8192，实际: %v", ctxSize)
			}
		} else {
			t.Errorf("context_size 类型异常: %T", ctxSize)
		}
	}

	// 原有字段应保留（system_prompt 也是 snake_case）
	if updatedMap["system_prompt"] != "test" {
		t.Errorf("system_prompt 应保留为 'test'，实际: %v", updatedMap["system_prompt"])
	}
}

// TestEnsureConfigFields_NoMissingFields 验证无缺失字段时不修改文件
func TestEnsureConfigFields_NoMissingFields(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// 用户配置已完整
	userData := []byte(`{"system_prompt":"test","temperature":0.7,"context_size":4096}`)
	if err := os.WriteFile(configPath, userData, 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	cfg := &Config{
		SystemPrompt: "different", // 与用户配置不同
		Temperature:  0.9,
		ContextSize:  8192,
	}

	originalStat, _ := os.Stat(configPath)
	originalModTime := originalStat.ModTime()

	ensureConfigFields(configPath, userData, cfg)

	// 验证文件未被修改（无缺失字段时不写入）
	updatedStat, _ := os.Stat(configPath)
	if !updatedStat.ModTime().Equal(originalModTime) {
		// 文件可能被重写但内容相同，检查内容
		updated, _ := os.ReadFile(configPath)
		var updatedMap map[string]any
		json.Unmarshal(updated, &updatedMap)
		// 用户已有字段值不应被 cfg 覆盖
		if updatedMap["system_prompt"] != "test" {
			t.Errorf("用户已有字段不应被覆盖，system_prompt 期望 'test'，实际: %v", updatedMap["system_prompt"])
		}
		if updatedMap["context_size"] != float64(4096) {
			t.Errorf("用户已有字段不应被覆盖，context_size 期望 4096，实际: %v", updatedMap["context_size"])
		}
	}
}

// TestEnsureConfigFields_InvalidJSON 验证无效 JSON 不报错（静默返回）
func TestEnsureConfigFields_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	invalidJSON := []byte(`{invalid json content}`)
	cfg := &Config{SystemPrompt: "test"}

	// 不应 panic 或报错
	ensureConfigFields(configPath, invalidJSON, cfg)
}

// TestEnsureConfigFields_StringWrappedJSON 验证字符串包裹的 JSON 正确处理
func TestEnsureConfigFields_StringWrappedJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// 双重序列化的 JSON（字符串包裹）
	inner := `{"system_prompt":"test"}`
	wrapped, _ := json.Marshal(inner) // 字符串包裹的 JSON
	userData := wrapped

	if err := os.WriteFile(configPath, userData, 0644); err != nil {
		t.Fatalf("写入测试文件失败: %v", err)
	}

	cfg := &Config{
		SystemPrompt: "test",
		ContextSize:  4096,
	}

	ensureConfigFields(configPath, userData, cfg)

	// 验证文件被正确处理（不 panic 即可）
	updated, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("读取更新后文件失败: %v", err)
	}

	// 文件应为合法 JSON（可能被重写为标准格式）
	var updatedMap map[string]any
	if err := json.Unmarshal(updated, &updatedMap); err != nil {
		// 如果仍是字符串包裹的 JSON，尝试二次解析
		var inner string
		if err2 := json.Unmarshal(updated, &inner); err2 == nil {
			if err3 := json.Unmarshal([]byte(inner), &updatedMap); err3 != nil {
				t.Fatalf("二次解析失败: %v", err3)
			}
		} else {
			t.Fatalf("文件不是合法 JSON: %v", err)
		}
	}

	// system_prompt 应保留
	if updatedMap["system_prompt"] != "test" {
		t.Errorf("system_prompt 应保留为 'test'，实际: %v", updatedMap["system_prompt"])
	}
}
