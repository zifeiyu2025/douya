// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_InvalidConfigFallsBackToDefault 验证当配置文件中存在非法字段时，
// Load() 应当通过 Validate() 检测到错误，并回退到默认配置。
// 这里用 temperature=999 触发校验失败，期望返回的配置 Temperature 等于默认值 0.6。
func TestLoad_InvalidConfigFallsBackToDefault(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// 构造一份非法配置：temperature=999（合法范围是 0-2）
	invalidCfg := DefaultConfig()
	invalidCfg.Temperature = 999
	data, err := json.MarshalIndent(invalidCfg, "", "  ")
	if err != nil {
		t.Fatalf("序列化配置失败: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("写入临时配置文件失败: %v", err)
	}

	// 调用 Load()，期望它检测到非法值并回退到默认配置
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load 返回了非预期的错误: %v", err)
	}
	if cfg.Temperature != 0.6 {
		t.Errorf("期望回退到默认 Temperature=0.6，实际得到: %.2f", cfg.Temperature)
	}
}

// TestValidate_ContextSizeTooLarge 验证 ContextSize 超过上限 131072 时返回错误
func TestValidate_ContextSizeTooLarge(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ContextSize = 200000
	if err := cfg.Validate(); err == nil {
		t.Error("期望 ContextSize=200000 时返回错误，实际返回 nil")
	}
}

// TestValidate_InvalidSearchMode 验证 SearchMode 不是合法值时返回错误
func TestValidate_InvalidSearchMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SearchMode = "always"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 SearchMode=\"always\" 时返回错误，实际返回 nil")
	}
}

// TestValidate_InvalidSystemPromptMode 验证 SystemPromptMode 不是合法值时返回错误
func TestValidate_InvalidSystemPromptMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SystemPromptMode = "custom"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 SystemPromptMode=\"custom\" 时返回错误，实际返回 nil")
	}
}

// TestValidate_TopK 验证 TopK 为负数时返回错误
func TestValidate_TopK(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TopK = -1
	if err := cfg.Validate(); err == nil {
		t.Error("期望 TopK=-1 时返回错误，实际返回 nil")
	}
}

// TestValidate_MinP 验证 MinP 越界时返回错误
func TestValidate_MinP(t *testing.T) {
	// MinP 小于 0
	cfg := DefaultConfig()
	cfg.MinP = -0.1
	if err := cfg.Validate(); err == nil {
		t.Error("期望 MinP=-0.1 时返回错误，实际返回 nil")
	}

	// MinP 大于 1
	cfg2 := DefaultConfig()
	cfg2.MinP = 1.5
	if err := cfg2.Validate(); err == nil {
		t.Error("期望 MinP=1.5 时返回错误，实际返回 nil")
	}
}

// TestValidate_DryFields 验证 Dry 相关字段为负数时返回错误
func TestValidate_DryFields(t *testing.T) {
	// DryMultiplier 为负
	cfg1 := DefaultConfig()
	cfg1.DryMultiplier = -1
	if err := cfg1.Validate(); err == nil {
		t.Error("期望 DryMultiplier=-1 时返回错误，实际返回 nil")
	}

	// DryBase 为负
	cfg2 := DefaultConfig()
	cfg2.DryBase = -1
	if err := cfg2.Validate(); err == nil {
		t.Error("期望 DryBase=-1 时返回错误，实际返回 nil")
	}

	// DryAllowedLength 为负
	cfg3 := DefaultConfig()
	cfg3.DryAllowedLength = -1
	if err := cfg3.Validate(); err == nil {
		t.Error("期望 DryAllowedLength=-1 时返回错误，实际返回 nil")
	}
}

// TestValidate_RAGFields 验证 RAG 相关字段越界时返回错误
func TestValidate_RAGFields(t *testing.T) {
	// RAGTopK = 0
	cfg1 := DefaultConfig()
	cfg1.RAGTopK = 0
	if err := cfg1.Validate(); err == nil {
		t.Error("期望 RAGTopK=0 时返回错误，实际返回 nil")
	}

	// RAGTopK 为负
	cfg2 := DefaultConfig()
	cfg2.RAGTopK = -1
	if err := cfg2.Validate(); err == nil {
		t.Error("期望 RAGTopK=-1 时返回错误，实际返回 nil")
	}

	// RAGMinScore 小于 0
	cfg3 := DefaultConfig()
	cfg3.RAGMinScore = -0.1
	if err := cfg3.Validate(); err == nil {
		t.Error("期望 RAGMinScore=-0.1 时返回错误，实际返回 nil")
	}

	// RAGMinScore 大于 1
	cfg4 := DefaultConfig()
	cfg4.RAGMinScore = 1.5
	if err := cfg4.Validate(); err == nil {
		t.Error("期望 RAGMinScore=1.5 时返回错误，实际返回 nil")
	}
}

// TestValidate_RAGChunkOverlap 验证当 RAGChunkOverlap >= RAGChunkSize（且两者都 > 0）时返回错误
func TestValidate_RAGChunkOverlap(t *testing.T) {
	// Overlap 等于 ChunkSize
	cfg1 := DefaultConfig()
	cfg1.RAGChunkSize = 512
	cfg1.RAGChunkOverlap = 512
	if err := cfg1.Validate(); err == nil {
		t.Error("期望 RAGChunkOverlap==RAGChunkSize 时返回错误，实际返回 nil")
	}

	// Overlap 大于 ChunkSize
	cfg2 := DefaultConfig()
	cfg2.RAGChunkSize = 512
	cfg2.RAGChunkOverlap = 600
	if err := cfg2.Validate(); err == nil {
		t.Error("期望 RAGChunkOverlap>RAGChunkSize 时返回错误，实际返回 nil")
	}
}

// TestValidate_ValidConfig 验证默认配置应当通过校验
func TestValidate_ValidConfig(t *testing.T) {
	cfg := DefaultConfig()
	if err := cfg.Validate(); err != nil {
		t.Errorf("默认配置应当通过校验，实际返回错误: %v", err)
	}
}
