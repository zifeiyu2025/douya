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
// 这里用 temperature=999 触发校验失败，期望返回的配置 Temperature 等于默认值 0.8。
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
	if cfg.Temperature != 0.8 {
		t.Errorf("期望回退到默认 Temperature=0.8，实际得到: %.2f", cfg.Temperature)
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

// ===== 任务17：补充字段校验测试 =====

// TestValidate_Threads 验证 Threads 为负数时返回错误
func TestValidate_Threads(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Threads = -1
	if err := cfg.Validate(); err == nil {
		t.Error("期望 Threads=-1 时返回错误，实际返回 nil")
	}
}

// TestValidate_BatchSize 验证 BatchSize 为负数时返回错误
func TestValidate_BatchSize(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BatchSize = -1
	if err := cfg.Validate(); err == nil {
		t.Error("期望 BatchSize=-1 时返回错误，实际返回 nil")
	}
}

// TestValidate_GPULayers 验证 GPULayers 为负数时返回错误
func TestValidate_GPULayers(t *testing.T) {
	cfg := DefaultConfig()
	cfg.GPULayers = -1
	if err := cfg.Validate(); err == nil {
		t.Error("期望 GPULayers=-1 时返回错误，实际返回 nil")
	}
}

// TestValidate_CacheRAM 验证 CacheRAM 为负数时返回错误
func TestValidate_CacheRAM(t *testing.T) {
	cfg := DefaultConfig()
	cfg.CacheRAM = -1
	if err := cfg.Validate(); err == nil {
		t.Error("期望 CacheRAM=-1 时返回错误，实际返回 nil")
	}
}

// TestValidate_ImageTokens 验证 ImageMinTokens > ImageMaxTokens（两者都 > 0）时返回错误
func TestValidate_ImageTokens(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ImageMinTokens = 100
	cfg.ImageMaxTokens = 50
	if err := cfg.Validate(); err == nil {
		t.Error("期望 ImageMinTokens>ImageMaxTokens 时返回错误，实际返回 nil")
	}
}

// TestValidate_ImageTokens_Valid 验证 ImageMinTokens <= ImageMaxTokens 的合法情况通过校验
func TestValidate_ImageTokens_Valid(t *testing.T) {
	// min < max
	cfg1 := DefaultConfig()
	cfg1.ImageMinTokens = 50
	cfg1.ImageMaxTokens = 100
	if err := cfg1.Validate(); err != nil {
		t.Errorf("期望 ImageMinTokens<ImageMaxTokens 时通过，实际返回错误: %v", err)
	}
	// 两者都为 0（默认，表示自动）
	cfg2 := DefaultConfig()
	cfg2.ImageMinTokens = 0
	cfg2.ImageMaxTokens = 0
	if err := cfg2.Validate(); err != nil {
		t.Errorf("期望 ImageMinTokens=ImageMaxTokens=0 时通过，实际返回错误: %v", err)
	}
}

// TestValidate_GrpAttn 验证 GrpAttnN 和 GrpAttnW 不同时非零/为零时返回错误
func TestValidate_GrpAttn(t *testing.T) {
	// N 非零、W 为零
	cfg1 := DefaultConfig()
	cfg1.GrpAttnN = 4
	cfg1.GrpAttnW = 0
	if err := cfg1.Validate(); err == nil {
		t.Error("期望 GrpAttnN=4 GrpAttnW=0 时返回错误，实际返回 nil")
	}
	// N 为零、W 非零
	cfg2 := DefaultConfig()
	cfg2.GrpAttnN = 0
	cfg2.GrpAttnW = 512
	if err := cfg2.Validate(); err == nil {
		t.Error("期望 GrpAttnN=0 GrpAttnW=512 时返回错误，实际返回 nil")
	}
}

// TestValidate_GrpAttn_Valid 验证 GrpAttnN 和 GrpAttnW 合法组合通过校验
func TestValidate_GrpAttn_Valid(t *testing.T) {
	// 同时为零（禁用分组注意力）
	cfg1 := DefaultConfig()
	cfg1.GrpAttnN = 0
	cfg1.GrpAttnW = 0
	if err := cfg1.Validate(); err != nil {
		t.Errorf("期望 GrpAttnN=GrpAttnW=0 时通过，实际返回错误: %v", err)
	}
	// 同时非零（启用分组注意力）
	cfg2 := DefaultConfig()
	cfg2.GrpAttnN = 4
	cfg2.GrpAttnW = 512
	if err := cfg2.Validate(); err != nil {
		t.Errorf("期望 GrpAttnN=4 GrpAttnW=512 时通过，实际返回错误: %v", err)
	}
}

// TestValidate_RerankTopN 验证 RerankTopN <= 0 时返回错误
func TestValidate_RerankTopN(t *testing.T) {
	// RerankTopN = 0
	cfg1 := DefaultConfig()
	cfg1.RerankTopN = 0
	if err := cfg1.Validate(); err == nil {
		t.Error("期望 RerankTopN=0 时返回错误，实际返回 nil")
	}
	// RerankTopN 为负
	cfg2 := DefaultConfig()
	cfg2.RerankTopN = -1
	if err := cfg2.Validate(); err == nil {
		t.Error("期望 RerankTopN=-1 时返回错误，实际返回 nil")
	}
}

// ===== 任务29：版本化迁移测试 =====

// TestLoad_LegacyVersionMigratesToV1 验证旧版本（无 version 字段，Version=0）的配置
// 加载后正确迁移到 Version=1，且旧版 thinking 字段被迁移到 reasoning 字段。
func TestLoad_LegacyVersionMigratesToV1(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// 构造一份旧版本配置：无 version 字段（Version=0），使用旧版 thinking 字段
	// 故意不包含 version 和 reasoning 字段，模拟历史配置
	legacyData := map[string]interface{}{
		"port":                 8080,
		"context_size":         8192,
		"temperature":          0.8,
		"thinking_enabled":     true,
		"thinking_soft_switch": "think",
	}
	data, err := json.MarshalIndent(legacyData, "", "  ")
	if err != nil {
		t.Fatalf("序列化配置失败: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("写入临时配置文件失败: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load 返回了非预期的错误: %v", err)
	}
	// 旧版本（Version=0）应迁移到 Version=1
	if cfg.Version != 1 {
		t.Errorf("期望迁移后 Version=1，实际得到: %d", cfg.Version)
	}
	// 旧版 thinking_soft_switch="think" 应迁移为 reasoning="on"
	if cfg.Reasoning != "on" {
		t.Errorf("期望迁移后 Reasoning=\"on\"，实际得到: %q", cfg.Reasoning)
	}
}

// TestMigrate_LegacyVersion 验证 migrate 方法将 Version=0 升级到 1，并执行 thinking 迁移
func TestMigrate_LegacyVersion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 0
	cfg.ThinkingEnabled = true
	cfg.ThinkingSoftSwitch = "no_think"
	cfg.Reasoning = "" // 模拟未设置

	// 构造不含 reasoning 字段的原始数据，触发 thinking 迁移
	rawData := []byte(`{"thinking_enabled":true,"thinking_soft_switch":"no_think"}`)
	cfg.migrate(rawData)

	if cfg.Version != 1 {
		t.Errorf("期望迁移后 Version=1，实际得到: %d", cfg.Version)
	}
	// thinking_soft_switch="no_think" 应迁移为 reasoning="off"
	if cfg.Reasoning != "off" {
		t.Errorf("期望 thinking_soft_switch=no_think 迁移为 Reasoning=\"off\"，实际得到: %q", cfg.Reasoning)
	}
}

// TestMigrate_AlreadyCurrentVersion 验证已是当前版本的配置不触发迁移
func TestMigrate_AlreadyCurrentVersion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 1
	cfg.Reasoning = "auto"
	// 原始数据包含 version 字段（值为当前版本），不应触发 v0->v1 迁移，reasoning 不被覆盖
	rawData := []byte(`{"version":1,"thinking_enabled":true,"thinking_soft_switch":"think"}`)
	cfg.migrate(rawData)

	if cfg.Version != 1 {
		t.Errorf("期望 Version 保持 1，实际得到: %d", cfg.Version)
	}
	if cfg.Reasoning != "auto" {
		t.Errorf("期望 Reasoning 不被覆盖（仍为 auto），实际得到: %q", cfg.Reasoning)
	}
}

// TestDefaultConfig_Version 验证默认配置的 Version 字段为当前版本号
func TestDefaultConfig_Version(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Version != currentConfigVersion {
		t.Errorf("期望默认配置 Version=%d，实际得到: %d", currentConfigVersion, cfg.Version)
	}
}
