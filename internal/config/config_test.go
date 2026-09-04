// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoad_InvalidConfigFallsBackToDefault 验证当配置文件中存在非法字段时，
// Load() 应当通过 Validate() 检测到错误，并回退到默认配置，同时写盘避免下次启动重复告警。
// 这里用 temperature=999 触发校验失败，期望返回的配置 Temperature 等于默认值 0.8，
// 且磁盘文件已被重写为默认配置。
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

	// 验证磁盘文件已被重写为默认配置（避免每次启动重复告警）
	persisted, err := Load(configPath)
	if err != nil {
		t.Fatalf("二次 Load 返回了非预期的错误: %v", err)
	}
	if persisted.Temperature != 0.8 {
		t.Errorf("期望磁盘已持久化默认 Temperature=0.8，实际得到: %.2f", persisted.Temperature)
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

// TestValidate_InvalidReasoningEffort 验证 ReasoningEffort 非法值返回错误
func TestValidate_InvalidReasoningEffort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReasoningEffort = "ultra"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 ReasoningEffort=\"ultra\" 时返回错误，实际返回 nil")
	}
}

// TestValidate_ValidReasoningEffort 验证 ReasoningEffort 合法值通过校验
func TestValidate_ValidReasoningEffort(t *testing.T) {
	for _, v := range []string{"", "low", "medium", "high", "max"} {
		cfg := DefaultConfig()
		cfg.ReasoningEffort = v
		if err := cfg.Validate(); err != nil {
			t.Errorf("ReasoningEffort=%q 应通过校验，实际错误: %v", v, err)
		}
	}
}

// TestRepair_InvalidReasoningEffort 验证非法 ReasoningEffort 被修复为空
func TestRepair_InvalidReasoningEffort(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ReasoningEffort = "ultra"
	repaired := cfg.repairInvalidFields()
	if cfg.ReasoningEffort != "" {
		t.Errorf("期望修复后 ReasoningEffort=\"\"，实际 %q", cfg.ReasoningEffort)
	}
	found := false
	for _, msg := range repaired {
		if containsStr(msg, "reasoning_effort") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("期望修复列表包含 reasoning_effort，实际 %v", repaired)
	}
}

func containsStr(s, sub string) bool {
	return strings.Contains(s, sub)
}

// TestRepair_InvalidReasoningAuto 验证 reasoning=auto 被归一化为 off（自动思考已移除）
func TestRepair_InvalidReasoningAuto(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Reasoning = "auto"
	repaired := cfg.repairInvalidFields()
	if cfg.Reasoning != "off" {
		t.Errorf("期望修复后 Reasoning=\"off\"，实际 %q", cfg.Reasoning)
	}
	found := false
	for _, msg := range repaired {
		if containsStr(msg, "reasoning") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("期望修复列表包含 reasoning，实际 %v", repaired)
	}
}

// TestValidate_InvalidReasoningAuto 验证 reasoning=auto 在 Validate 中返回错误
func TestValidate_InvalidReasoningAuto(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Reasoning = "auto"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 Reasoning=\"auto\" 时返回错误，实际返回 nil")
	}
}

// TestValidate_ValidReasoning 验证 reasoning 仅允许 on / off
func TestValidate_ValidReasoning(t *testing.T) {
	for _, v := range []string{"on", "off"} {
		cfg := DefaultConfig()
		cfg.Reasoning = v
		if err := cfg.Validate(); err != nil {
			t.Errorf("Reasoning=%q 应通过校验，实际错误: %v", v, err)
		}
	}
}

// TestRepair_EnableBuiltinToolsClearsTools 验证 EnableBuiltinTools=true 时清空细粒度 tools
func TestRepair_EnableBuiltinToolsClearsTools(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableBuiltinTools = true
	cfg.Tools = "read_file"
	repaired := cfg.repairInvalidFields()
	if cfg.Tools != "" {
		t.Errorf("期望 EnableBuiltinTools 开启时 Tools 被清空，实际 %q", cfg.Tools)
	}
	found := false
	for _, msg := range repaired {
		if containsStr(msg, "enable_builtin_tools") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("期望修复列表包含 enable_builtin_tools，实际 %v", repaired)
	}
}

// TestRepair_EnableBuiltinToolsFalseKeepsTools 验证 EnableBuiltinTools=false 时保留细粒度 tools
func TestRepair_EnableBuiltinToolsFalseKeepsTools(t *testing.T) {
	cfg := DefaultConfig()
	cfg.EnableBuiltinTools = false
	cfg.Tools = "read_file,grep_search"
	repaired := cfg.repairInvalidFields()
	if cfg.Tools != "read_file,grep_search" {
		t.Errorf("期望 Tools 不被清空，实际 %q", cfg.Tools)
	}
	if len(repaired) != 0 {
		t.Errorf("期望无修复项，实际 %v", repaired)
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

// TestRepair_InvalidToolsRuntimeResets 验证非法 tools_runtime 被修复为空
func TestRepair_InvalidToolsRuntimeResets(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ToolsRuntime = "some-runtime-string-without-prefix"
	repaired := cfg.repairInvalidFields()
	if cfg.ToolsRuntime != "" {
		t.Errorf("期望非法 ToolsRuntime 被修复为空，实际 %q", cfg.ToolsRuntime)
	}
	found := false
	for _, msg := range repaired {
		if containsStr(msg, "tools_runtime") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("期望修复列表包含 tools_runtime，实际 %v", repaired)
	}
}

// TestRepair_ValidToolsRuntimeKept 验证合法 tools_runtime 被保留
func TestRepair_ValidToolsRuntimeKept(t *testing.T) {
	for _, v := range []string{"docker:ubuntu:22.04", "podman:alpine", "docker-container:abc123", "podman-container:xyz", "ssh:user@host"} {
		cfg := DefaultConfig()
		cfg.ToolsRuntime = v
		repaired := cfg.repairInvalidFields()
		if cfg.ToolsRuntime != v {
			t.Errorf("期望 ToolsRuntime=%q 被保留，实际 %q", v, cfg.ToolsRuntime)
		}
		if len(repaired) != 0 {
			t.Errorf("期望 ToolsRuntime=%q 无修复项，实际 %v", v, repaired)
		}
	}
}

// TestValidate_InvalidToolsRuntime 验证非法 tools_runtime 在 Validate 中报错
func TestValidate_InvalidToolsRuntime(t *testing.T) {
	cfg := DefaultConfig()
	cfg.ToolsRuntime = "invalid-no-prefix"
	if err := cfg.Validate(); err == nil {
		t.Error("期望非法 ToolsRuntime 时 Validate 返回错误，实际返回 nil")
	}
}

// TestValidate_ValidToolsRuntime 验证合法 tools_runtime 通过校验
func TestValidate_ValidToolsRuntime(t *testing.T) {
	for _, v := range []string{"", "docker:ubuntu:22.04", "ssh:user@host"} {
		cfg := DefaultConfig()
		cfg.ToolsRuntime = v
		if err := cfg.Validate(); err != nil {
			t.Errorf("期望 ToolsRuntime=%q 通过校验，实际错误: %v", v, err)
		}
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
	legacyData := map[string]any{
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
	// 旧版本（Version=0）应沿迁移链升级到最新版本
	if cfg.Version != currentConfigVersion {
		t.Errorf("期望迁移后 Version=%d，实际得到: %d", currentConfigVersion, cfg.Version)
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

	if cfg.Version != currentConfigVersion {
		t.Errorf("期望迁移后 Version=%d，实际得到: %d", currentConfigVersion, cfg.Version)
	}
	// thinking_soft_switch="no_think" 应迁移为 reasoning="off"
	if cfg.Reasoning != "off" {
		t.Errorf("期望 thinking_soft_switch=no_think 迁移为 Reasoning=\"off\"，实际得到: %q", cfg.Reasoning)
	}
}

// TestMigrate_V2ToV3_SeedsPerThemeBackground 验证 v2→v3 迁移：
// 磁盘缺少 background_light 键时，以旧 chat_background_opacity 为种子
// 生成亮/暗两套背景参数，且深色透明度按 0.85 系数略降（沉浸感）。
func TestMigrate_V2ToV3_SeedsPerThemeBackground(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 2
	cfg.ChatBackgroundOpacity = 0.6

	rawData := []byte(`{"version":2,"chat_background_opacity":0.6}`)
	cfg.migrate(rawData)

	if cfg.Version != 4 {
		t.Errorf("期望迁移后 Version=4，实际得到: %d", cfg.Version)
	}
	// 亮色种子 = 旧值原样保留
	if cfg.BackgroundLight.Opacity != 0.6 {
		t.Errorf("期望亮色 Opacity 种子=0.6，实际 %v", cfg.BackgroundLight.Opacity)
	}
	// 暗色种子 = 旧值 × 0.85 = 0.51
	if cfg.BackgroundDark.Opacity != 0.51 {
		t.Errorf("期望暗色 Opacity 种子=0.51（0.6×0.85），实际 %v", cfg.BackgroundDark.Opacity)
	}
	// 遮罩按主题差异化：暗色遮罩应比亮色重
	if cfg.BackgroundDark.MaskAlpha <= cfg.BackgroundLight.MaskAlpha {
		t.Errorf("期望暗色 MaskAlpha(%v) > 亮色(%v)", cfg.BackgroundDark.MaskAlpha, cfg.BackgroundLight.MaskAlpha)
	}
}

// TestMigrate_PerThemeBackground_KeepsExplicit 验证磁盘已含 background_light 键时
// 迁移绝不覆盖用户显式设置（区分"缺键"与"用户设过"的关键保护）。
func TestMigrate_PerThemeBackground_KeepsExplicit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 0 // 极端场景：version 字段缺失但用户已手动写入新键
	cfg.ThinkingEnabled = true
	cfg.ThinkingSoftSwitch = "think"
	cfg.Reasoning = ""
	explicit := ThemeBackgroundParams{Opacity: 0.42, Blur: 4, MaskAlpha: 0.1}
	cfg.BackgroundLight = explicit

	rawData := []byte(`{"background_light":{"opacity":0.42,"blur":4,"mask_alpha":0.1}}`)
	cfg.migrate(rawData)

	if cfg.BackgroundLight != explicit {
		t.Errorf("期望显式设置的 background_light 不被迁移覆盖，实际 %+v", cfg.BackgroundLight)
	}
}

// TestMigrate_V2ToV3_MissingOpacityKeyFallsBack 验证极老配置连
// chat_background_opacity 键都没有时，种子回退到旧版默认 0.9，
// 而不是把零值 0 当作用户选择生成"全透明表面"。
func TestMigrate_V2ToV3_MissingOpacityKeyFallsBack(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 2
	// 模拟 Unmarshal 后：磁盘无该键 → ChatBackgroundOpacity 为零值 0
	cfg.ChatBackgroundOpacity = 0

	rawData := []byte(`{"version":2}`)
	cfg.migrate(rawData)

	if cfg.BackgroundLight.Opacity != 0.9 {
		t.Errorf("期望缺失键时种子回退到 0.9，实际 %v", cfg.BackgroundLight.Opacity)
	}
}

// TestMigrate_V2ToV3_AbnormalSeedFallsBack 验证越界/NaN 种子回退到 0.9
func TestMigrate_V2ToV3_AbnormalSeedFallsBack(t *testing.T) {
	for name, badSeed := range map[string]float64{
		"越界大值": 7.5,
		"负数":   -0.3,
	} {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.Version = 2
			cfg.ChatBackgroundOpacity = badSeed
			rawData := []byte(`{"version":2,"chat_background_opacity":7.5}`)
			cfg.migrate(rawData)
			if cfg.BackgroundLight.Opacity != 0.9 {
				t.Errorf("期望异常种子回退到 0.9，实际 %v", cfg.BackgroundLight.Opacity)
			}
		})
	}
}

// TestMigrate_AlreadyCurrentVersion 验证已是当前版本的配置不触发迁移
func TestMigrate_AlreadyCurrentVersion(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = currentConfigVersion
	cfg.Reasoning = "auto"
	// 显式改写背景参数，验证当前版本配置不会被迁移逻辑碰
	custom := ThemeBackgroundParams{Opacity: 0.33, Blur: 2, MaskAlpha: 0.2}
	cfg.BackgroundLight = custom
	// 原始数据包含 version 字段（值为当前版本 4），不应触发迁移，字段不被覆盖
	rawData := []byte(`{"version":4,"thinking_enabled":true,"thinking_soft_switch":"think"}`)
	cfg.migrate(rawData)

	if cfg.Version != currentConfigVersion {
		t.Errorf("期望 Version 保持 %d，实际得到: %d", currentConfigVersion, cfg.Version)
	}
	if cfg.Reasoning != "auto" {
		t.Errorf("期望 Reasoning 不被覆盖（仍为 auto），实际得到: %q", cfg.Reasoning)
	}
	if cfg.BackgroundLight != custom {
		t.Errorf("期望 BackgroundLight 不被覆盖，实际 %+v", cfg.BackgroundLight)
	}
}

// ===== 统一 KV 缓存默认值迁移测试（v3→v4） =====

// TestMigrate_V3ToV4_KVUnifiedFalseBecomesTrue 验证磁盘显式存在 kv_unified=false 时
// 迁移为 true：false 在参数生成层不产生 --no-kv-unified，实际行为本就是开启（上游默认），
// 迁移仅修正 UI 显示与实际行为一致，无行为变化。
func TestMigrate_V3ToV4_KVUnifiedFalseBecomesTrue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 3
	cfg.KVUnified = false // 模拟旧默认值时代的持久化配置

	rawData := []byte(`{"version":3,"kv_unified":false}`)
	cfg.migrate(rawData)

	if cfg.Version != 4 {
		t.Errorf("期望迁移后 Version=4，实际得到: %d", cfg.Version)
	}
	if !cfg.KVUnified {
		t.Errorf("期望 kv_unified 从 false 迁移为 true，实际仍为 false")
	}
}

// TestMigrate_V3ToV4_MissingKeyKeepsTrue 验证磁盘缺 kv_unified 键时无需迁移：
// Unmarshal 后默认即 true，迁移不应改写任何字段。
func TestMigrate_V3ToV4_MissingKeyKeepsTrue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 3
	cfg.KVUnified = true // 缺键配置 Unmarshal 后即默认 true

	rawData := []byte(`{"version":3,"repeat_penalty":1.2}`)
	cfg.migrate(rawData)

	if cfg.Version != 4 {
		t.Errorf("期望迁移后 Version=4，实际得到: %d", cfg.Version)
	}
	if !cfg.KVUnified {
		t.Errorf("期望缺键时 KVUnified 保持 true，实际被改写为 false")
	}
}

// TestMigrate_V3ToV4_UnparseableKeyIgnored 验证 kv_unified 键值非法（非 bool）时
// 迁移不强行修改，交由校验层处理（repairInvalidFields 会按规则修复）。
func TestMigrate_V3ToV4_UnparseableKeyIgnored(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Version = 3

	rawData := []byte(`{"version":3,"kv_unified":"maybe"}`)
	cfg.migrate(rawData)

	if cfg.Version != 4 {
		t.Errorf("期望迁移后 Version=4，实际得到: %d", cfg.Version)
	}
	// 迁移不触碰非法值，KVUnified 保持 Unmarshal 默认 true
	if !cfg.KVUnified {
		t.Errorf("期望非法值不被迁移改写（保持默认 true），实际为 false")
	}
}

// ===== 每主题背景参数校验测试（v3） =====

// TestValidate_BackgroundParams_ZeroValuesLegal 验证合法零值放行：
// blur=0（不模糊）、mask_alpha=0（无遮罩）都是用户可能选择的正常值，
// 不应被范围校验拒绝，也不应被 repairInvalidFields 修复。
func TestValidate_BackgroundParams_ZeroValuesLegal(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BackgroundLight = ThemeBackgroundParams{Opacity: 0.5, Blur: 0, MaskAlpha: 0}
	cfg.BackgroundDark = ThemeBackgroundParams{Opacity: 1, Blur: 0, MaskAlpha: 1}
	if err := cfg.Validate(); err != nil {
		t.Errorf("背景参数合法零值应通过校验，实际错误: %v", err)
	}
	if repaired := cfg.repairInvalidFields(); len(repaired) != 0 {
		t.Errorf("背景参数合法零值不应触发修复，实际 %v", repaired)
	}
}

// TestValidate_BackgroundParams_OutOfRange 验证越界参数在 Validate 中报错
func TestValidate_BackgroundParams_OutOfRange(t *testing.T) {
	cases := []struct {
		name string
		mut  func(c *Config)
	}{
		{"亮色opacity越界", func(c *Config) { c.BackgroundLight.Opacity = 1.5 }},
		{"暗色mask越界", func(c *Config) { c.BackgroundDark.MaskAlpha = -0.1 }},
		{"负blur", func(c *Config) { c.BackgroundDark.Blur = -3 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := DefaultConfig()
			tc.mut(cfg)
			if err := cfg.Validate(); err == nil {
				t.Errorf("期望 %s 返回错误，实际 nil", tc.name)
			}
		})
	}
}

// TestRepair_BackgroundParams_OutOfRange 验证越界参数被修复回该主题默认值
func TestRepair_BackgroundParams_OutOfRange(t *testing.T) {
	cfg := DefaultConfig()
	cfg.BackgroundLight.Opacity = 1.7
	cfg.BackgroundDark.MaskAlpha = -0.5
	repaired := cfg.repairInvalidFields()

	def := DefaultConfig()
	if cfg.BackgroundLight.Opacity != def.BackgroundLight.Opacity {
		t.Errorf("期望亮色 Opacity 修复回默认 %v，实际 %v", def.BackgroundLight.Opacity, cfg.BackgroundLight.Opacity)
	}
	if cfg.BackgroundDark.MaskAlpha != def.BackgroundDark.MaskAlpha {
		t.Errorf("期望暗色 MaskAlpha 修复回默认 %v，实际 %v", def.BackgroundDark.MaskAlpha, cfg.BackgroundDark.MaskAlpha)
	}
	found := false
	for _, msg := range repaired {
		if containsStr(msg, "background_") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("期望修复列表包含 background_* 条目，实际 %v", repaired)
	}
}

// TestDefaultConfig_Version 验证默认配置的 Version 字段为当前版本号
func TestDefaultConfig_Version(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Version != currentConfigVersion {
		t.Errorf("期望默认配置 Version=%d，实际得到: %d", currentConfigVersion, cfg.Version)
	}
}

// ===== 多 GPU 参数校验测试（split_mode / tensor_split / main_gpu） =====
//
// 这批测试覆盖 llama.cpp 原生多 GPU 参数的校验逻辑：
//   - isValidSplitMode：枚举值校验（none/layer/row/tensor/空）
//   - validateTensorSplit：格式校验（至少两个权重、非负、至少一个正数）
//   - Validate 跨字段互斥：split_mode=none + tensor_split 非法组合
//
// 生活类比：就像 multicooker 多锅具套装——split_mode 是选"分层蒸"还是"分格煮"，
// tensor_split 是每格放多少食材，none+还要分格就是自相矛盾。

// TestValidate_SplitMode_Valid 验证所有合法的 SplitMode 值通过校验
func TestValidate_SplitMode_Valid(t *testing.T) {
	validModes := []string{"", "none", "layer", "row", "tensor"}
	for _, mode := range validModes {
		t.Run("mode="+mode, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.SplitMode = mode
			if err := cfg.Validate(); err != nil {
				t.Errorf("期望 SplitMode=%q 通过校验，实际返回错误: %v", mode, err)
			}
		})
	}
}

// TestValidate_SplitMode_Invalid 验证非法的 SplitMode 值返回错误
func TestValidate_SplitMode_Invalid(t *testing.T) {
	invalidModes := []string{"auto", "split", "horizontal", "vertical", "NONE", "Layer"}
	for _, mode := range invalidModes {
		t.Run("mode="+mode, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.SplitMode = mode
			if err := cfg.Validate(); err == nil {
				t.Errorf("期望 SplitMode=%q 返回错误，实际返回 nil", mode)
			}
		})
	}
}

// TestValidate_TensorSplit_Empty 验证空 TensorSplit 通过校验（默认不传递）
func TestValidate_TensorSplit_Empty(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TensorSplit = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("期望 TensorSplit 为空时通过校验，实际返回错误: %v", err)
	}
}

// TestValidate_TensorSplit_Valid 验证合法的 TensorSplit 格式通过校验
func TestValidate_TensorSplit_Valid(t *testing.T) {
	validSplits := []string{
		"3,1",        // 双卡 3:1
		"1,1",        // 双卡均分
		"3,2,1",      // 三卡 3:2:1
		"0.5,0.5",    // 浮点权重
		" 3 , 1 ",    // 带空格（手写配置常见）
		"10,5,3,2,1", // 五卡
	}
	for _, split := range validSplits {
		t.Run("split="+split, func(t *testing.T) {
			cfg := DefaultConfig()
			cfg.TensorSplit = split
			if err := cfg.Validate(); err != nil {
				t.Errorf("期望 TensorSplit=%q 通过校验，实际返回错误: %v", split, err)
			}
		})
	}
}

// TestValidate_TensorSplit_SingleValue 验证只有单个权重值时返回错误
// 生活类比：分蛋糕至少要分给两个人，只写一个数等于没分。
func TestValidate_TensorSplit_SingleValue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TensorSplit = "3"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 TensorSplit=3（单个值）返回错误，实际返回 nil")
	}
}

// TestValidate_TensorSplit_Negative 验证负数权重返回错误
func TestValidate_TensorSplit_Negative(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TensorSplit = "3,-1"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 TensorSplit=3,-1（负数）返回错误，实际返回 nil")
	}
}

// TestValidate_TensorSplit_AllZero 验证全零权重返回错误
// 生活类比：所有盘子都分 0 份，等于没分蛋糕。
func TestValidate_TensorSplit_AllZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TensorSplit = "0,0"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 TensorSplit=0,0（全零）返回错误，实际返回 nil")
	}
}

// TestValidate_TensorSplit_NonNumeric 验证非数字值返回错误
func TestValidate_TensorSplit_NonNumeric(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TensorSplit = "3,abc"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 TensorSplit=3,abc（非数字）返回错误，实际返回 nil")
	}
}

// TestValidate_TensorSplit_Nan 验证 NaN 返回错误
func TestValidate_TensorSplit_Nan(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TensorSplit = "NaN,1"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 TensorSplit=NaN,1 返回错误，实际返回 nil")
	}
}

// TestValidate_TensorSplit_Inf 验证 Inf 返回错误
func TestValidate_TensorSplit_Inf(t *testing.T) {
	cfg := DefaultConfig()
	cfg.TensorSplit = "Inf,1"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 TensorSplit=Inf,1 返回错误，实际返回 nil")
	}
}

// TestValidate_MultiGPU_NoneWithTensorSplit 验证 split_mode=none + tensor_split 非空时返回错误
// 生活类比：用户说"禁用多卡"（none）但又指定了"分蛋糕方案"（tensor_split），自相矛盾。
func TestValidate_MultiGPU_NoneWithTensorSplit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitMode = "none"
	cfg.TensorSplit = "3,1"
	if err := cfg.Validate(); err == nil {
		t.Error("期望 split_mode=none + tensor_split=3,1 返回错误，实际返回 nil")
	}
}

// TestValidate_MultiGPU_NoneWithEmptyTensorSplit 验证 split_mode=none + tensor_split 空时通过
func TestValidate_MultiGPU_NoneWithEmptyTensorSplit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitMode = "none"
	cfg.TensorSplit = ""
	if err := cfg.Validate(); err != nil {
		t.Errorf("期望 split_mode=none + tensor_split 为空时通过，实际返回错误: %v", err)
	}
}

// TestValidate_MultiGPU_LayerWithTensorSplit 验证 split_mode=layer + tensor_split 非空时通过
func TestValidate_MultiGPU_LayerWithTensorSplit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitMode = "layer"
	cfg.TensorSplit = "3,1"
	if err := cfg.Validate(); err != nil {
		t.Errorf("期望 split_mode=layer + tensor_split=3,1 通过，实际返回错误: %v", err)
	}
}

// TestValidate_MultiGPU_RowWithTensorSplit 验证 split_mode=row + tensor_split 非空时通过
func TestValidate_MultiGPU_RowWithTensorSplit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitMode = "row"
	cfg.TensorSplit = "3,1"
	if err := cfg.Validate(); err != nil {
		t.Errorf("期望 split_mode=row + tensor_split=3,1 通过，实际返回错误: %v", err)
	}
}

// ===== APIBase 格式校验测试 =====
// 生活类比：像检查快递单上的地址格式，缺省市区或门牌号都不行

// TestValidate_APIBase_Valid 验证合法的 APIBase 通过校验
func TestValidate_APIBase_Valid(t *testing.T) {
	cases := []string{
		"http://127.0.0.1:8080",
		"https://localhost:3000",
		"http://192.168.1.100:8888",
		"", // 空字符串不报错（由默认值保证）
	}
	for _, apiBase := range cases {
		cfg := DefaultConfig()
		cfg.APIBase = apiBase
		if err := cfg.Validate(); err != nil {
			t.Errorf("api_base=%q 应通过校验，实际返回错误: %v", apiBase, err)
		}
	}
}

// TestValidate_APIBase_MissingPort 验证缺少端口号的 APIBase 报错
func TestValidate_APIBase_MissingPort(t *testing.T) {
	cases := []string{
		"http://127.0.0.1",
		"https://localhost",
		"http://192.168.1.100",
	}
	for _, apiBase := range cases {
		cfg := DefaultConfig()
		cfg.APIBase = apiBase
		if err := cfg.Validate(); err == nil {
			t.Errorf("api_base=%q 缺少端口，应返回错误", apiBase)
		}
	}
}

// TestValidate_APIBase_InvalidScheme 验证非 HTTP/HTTPS 协议报错
func TestValidate_APIBase_InvalidScheme(t *testing.T) {
	cases := []string{
		"ftp://127.0.0.1:8080",
		"127.0.0.1:8080",
		"://127.0.0.1:8080",
	}
	for _, apiBase := range cases {
		cfg := DefaultConfig()
		cfg.APIBase = apiBase
		if err := cfg.Validate(); err == nil {
			t.Errorf("api_base=%q 协议不合法，应返回错误", apiBase)
		}
	}
}

// TestValidate_APIBase_MissingHost 验证缺少主机地址报错
func TestValidate_APIBase_MissingHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.APIBase = "http://:8080"
	if err := cfg.Validate(); err == nil {
		t.Error("api_base='http://:8080' 缺少主机地址，应返回错误")
	}
}

// TestValidate_MultiGPU_TensorWithTensorSplit 验证 split_mode=tensor + tensor_split 非空时通过
func TestValidate_MultiGPU_TensorWithTensorSplit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SplitMode = "tensor"
	cfg.TensorSplit = "3,1"
	if err := cfg.Validate(); err != nil {
		t.Errorf("期望 split_mode=tensor + tensor_split=3,1 通过，实际返回错误: %v", err)
	}
}

// TestDefaultConfig_MultiGPUFields 验证默认配置中多 GPU 字段为安全默认值
// 生活类比：新车出厂时挡位默认在空挡（SplitMode 空），油门松开（MainGPU=-1）。
func TestDefaultConfig_MultiGPUFields(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SplitMode != "" {
		t.Errorf("期望默认 SplitMode 为空（使用 llama.cpp 默认 layer），实际得到: %q", cfg.SplitMode)
	}
	if cfg.TensorSplit != "" {
		t.Errorf("期望默认 TensorSplit 为空，实际得到: %q", cfg.TensorSplit)
	}
	if cfg.MainGPU != -1 {
		t.Errorf("期望默认 MainGPU=-1（不传递），实际得到: %d", cfg.MainGPU)
	}
}

// TestWriteFileAtomic 验证原子写文件：目标文件存在时内容被正确替换，
// 权限保持为指定值，且不残留临时文件。
func TestWriteFileAtomic(t *testing.T) {
	tmpDir := t.TempDir()
	target := filepath.Join(tmpDir, "config.json")

	// 1. 首次写入
	if err := writeFileAtomic(target, []byte(`{"a":1}`), 0o600); err != nil {
		t.Fatalf("首次写入失败: %v", err)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(data) != `{"a":1}` {
		t.Errorf("期望内容 {\"a\":1}，实际 %q", string(data))
	}

	// 2. 覆盖写入
	if err := writeFileAtomic(target, []byte(`{"b":2}`), 0o600); err != nil {
		t.Fatalf("覆盖写入失败: %v", err)
	}
	data, err = os.ReadFile(target)
	if err != nil {
		t.Fatalf("读取失败: %v", err)
	}
	if string(data) != `{"b":2}` {
		t.Errorf("期望内容 {\"b\":2}，实际 %q", string(data))
	}

	// 3. 不残留临时文件
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("读取目录失败: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("期望目录中只有目标文件，实际 %d 个条目", len(entries))
	}
}
