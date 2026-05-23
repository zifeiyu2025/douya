// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package config_test

import (
	"douya/internal/config"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.ContextSize != 8192 {
		t.Fatalf("expected ContextSize=8192, got %d", cfg.ContextSize)
	}
	if cfg.Temperature != 0.8 {
		t.Fatalf("expected Temperature=0.8, got %f", cfg.Temperature)
	}
	if cfg.TopP != 0.95 {
		t.Fatalf("expected TopP=0.95, got %f", cfg.TopP)
	}
	if cfg.TopK != 20 {
		t.Fatalf("expected TopK=20, got %d", cfg.TopK)
	}
	if cfg.RepeatPenalty != 1.0 {
		t.Fatalf("expected RepeatPenalty=1.0, got %f", cfg.RepeatPenalty)
	}
	if cfg.SystemPrompt != "" {
		t.Fatalf("expected SystemPrompt='', got '%s'", cfg.SystemPrompt)
	}
}

func TestLoad_FromFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	content := map[string]interface{}{
		"context_size": 8192,
		"temperature":  0.5,
		"top_p":        0.8,
		"top_k":        20,
	}
	data, _ := json.Marshal(content)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.ContextSize != 8192 {
		t.Fatalf("expected ContextSize=8192, got %d", cfg.ContextSize)
	}
	if cfg.Temperature != 0.5 {
		t.Fatalf("expected Temperature=0.5, got %f", cfg.Temperature)
	}
	if cfg.TopP != 0.8 {
		t.Fatalf("expected TopP=0.8, got %f", cfg.TopP)
	}
	if cfg.TopK != 20 {
		t.Fatalf("expected TopK=20, got %d", cfg.TopK)
	}
}

func TestSaveAndLoad_Roundtrip(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	original := &config.Config{
		ModelPath:       "/custom/model.gguf",
		LlamaServerPath: "/custom/llama-server",
		APIBase:         "http://127.0.0.1:9999",
		Port:            9999,
		ContextSize:     2048,
		Temperature:     0.3,
		TopP:            0.95,
		TopK:            50,
		RepeatPenalty:   1.2,
		SystemPrompt:    "You are a helpful assistant.",
	}

	if err := config.Save(cfgPath, original); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if loaded.ModelPath != original.ModelPath {
		t.Fatalf("expected ModelPath '%s', got '%s'", original.ModelPath, loaded.ModelPath)
	}
	if loaded.LlamaServerPath != original.LlamaServerPath {
		t.Fatalf("expected LlamaServerPath '%s', got '%s'", original.LlamaServerPath, loaded.LlamaServerPath)
	}
	if loaded.Port != original.Port {
		t.Fatalf("expected Port %d, got %d", original.Port, loaded.Port)
	}
	if loaded.ContextSize != original.ContextSize {
		t.Fatalf("expected ContextSize %d, got %d", original.ContextSize, loaded.ContextSize)
	}
	if loaded.Temperature != original.Temperature {
		t.Fatalf("expected Temperature %f, got %f", original.Temperature, loaded.Temperature)
	}
	if loaded.TopP != original.TopP {
		t.Fatalf("expected TopP %f, got %f", original.TopP, loaded.TopP)
	}
	if loaded.TopK != original.TopK {
		t.Fatalf("expected TopK %d, got %d", original.TopK, loaded.TopK)
	}
	if loaded.RepeatPenalty != original.RepeatPenalty {
		t.Fatalf("expected RepeatPenalty %f, got %f", original.RepeatPenalty, loaded.RepeatPenalty)
	}
	if loaded.SystemPrompt != original.SystemPrompt {
		t.Fatalf("expected SystemPrompt '%s', got '%s'", original.SystemPrompt, loaded.SystemPrompt)
	}
}

func TestLoad_FileNotExist(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/config.json")
	if err != nil {
		t.Fatalf("Load should not return error for missing file, got: %v", err)
	}
	defaults := config.DefaultConfig()
	if cfg.ContextSize != defaults.ContextSize {
		t.Fatalf("expected default ContextSize=%d, got %d", defaults.ContextSize, cfg.ContextSize)
	}
	if cfg.Temperature != defaults.Temperature {
		t.Fatalf("expected default Temperature=%f, got %f", defaults.Temperature, cfg.Temperature)
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(cfgPath, []byte("{invalid json content}"), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestDefaultConfig_AllFields(t *testing.T) {
	cfg := config.DefaultConfig()

	if cfg.ModelPath != "models/Gemma-4-E4B-U-Q4_K_M/Gemma-4-E4B-U-Q4_K_M.gguf" {
		t.Errorf("expected default ModelPath, got '%s'", cfg.ModelPath)
	}
	if cfg.LlamaServerPath != "engines/llama-server.exe" {
		t.Errorf("expected default LlamaServerPath, got '%s'", cfg.LlamaServerPath)
	}
	if cfg.APIBase != "http://127.0.0.1:8080" {
		t.Errorf("expected default APIBase, got '%s'", cfg.APIBase)
	}
	if cfg.Port != 8080 {
		t.Errorf("expected default Port 8080, got %d", cfg.Port)
	}
}

func TestLoad_PartialOverride(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	content := map[string]interface{}{
		"temperature": 0.3,
	}
	data, _ := json.Marshal(content)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Temperature != 0.3 {
		t.Fatalf("expected Temperature=0.3, got %f", cfg.Temperature)
	}
	if cfg.ContextSize != 8192 {
		t.Fatalf("expected default ContextSize=8192 for unspecified field, got %d", cfg.ContextSize)
	}
}

func TestLoad_EmptyFile(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	if err := os.WriteFile(cfgPath, []byte("{}"), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	defaults := config.DefaultConfig()
	if cfg.ContextSize != defaults.ContextSize {
		t.Errorf("expected default ContextSize=%d, got %d", defaults.ContextSize, cfg.ContextSize)
	}
}

func TestSave_CreatesReadableJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	cfg := config.DefaultConfig()
	err := config.Save(cfgPath, cfg)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		t.Error("config file should exist after Save")
	}

	data, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}
	if !json.Valid(data) {
		t.Error("saved config should be valid JSON")
	}
}

func TestSaveAndLoad_Overwrite(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	cfg1 := &config.Config{
		Temperature: 0.5,
		Port:        8080,
	}
	config.Save(cfgPath, cfg1)

	cfg2 := &config.Config{
		Temperature: 0.9,
		Port:        9090,
	}
	config.Save(cfgPath, cfg2)

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if loaded.Temperature != 0.9 {
		t.Errorf("expected Temperature=0.9 after overwrite, got %f", loaded.Temperature)
	}
	if loaded.Port != 9090 {
		t.Errorf("expected Port=9090 after overwrite, got %d", loaded.Port)
	}
}

func TestConfig_JSONSerialization(t *testing.T) {
	cfg := &config.Config{
		ModelPath:       "/path/to/model.gguf",
		LlamaServerPath: "/path/to/server",
		APIBase:         "http://localhost:8080",
		Port:            8080,
		ContextSize:     4096,
		Temperature:     0.7,
		TopP:            0.9,
		TopK:            40,
		RepeatPenalty:   1.0,
		SystemPrompt:    "你是一个有用的AI助手",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal config: %v", err)
	}

	if parsed["temperature"] != 0.7 {
		t.Errorf("expected temperature 0.7, got %v", parsed["temperature"])
	}
	if parsed["system_prompt"] != "你是一个有用的AI助手" {
		t.Errorf("expected system_prompt '你是一个有用的AI助手', got %v", parsed["system_prompt"])
	}
}

func TestLoad_WithSystemPrompt(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	content := map[string]interface{}{
		"system_prompt": "你是专业的Go语言开发者",
	}
	data, _ := json.Marshal(content)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.SystemPrompt != "你是专业的Go语言开发者" {
		t.Errorf("expected SystemPrompt '你是专业的Go语言开发者', got '%s'", cfg.SystemPrompt)
	}
}

func TestLoad_ExtremeValues(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	content := map[string]interface{}{
		"temperature":    2.0,
		"top_p":          0.0,
		"top_k":          0,
		"repeat_penalty": 0.0,
		"context_size":   1,
	}
	data, _ := json.Marshal(content)
	if err := os.WriteFile(cfgPath, data, 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Temperature != 2.0 {
		t.Errorf("expected Temperature=2.0, got %f", cfg.Temperature)
	}
	if cfg.TopP != 0.0 {
		t.Errorf("expected TopP=0.0, got %f", cfg.TopP)
	}
	if cfg.TopK != 0 {
		t.Errorf("expected TopK=0, got %d", cfg.TopK)
	}
	if cfg.ContextSize != 1 {
		t.Errorf("expected ContextSize=1, got %d", cfg.ContextSize)
	}
}

func TestSaveAndLoad_GenerationParams(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	original := &config.Config{
		ModelPath:       "models/Qwen3.5U-9B-Q4_K_M.gguf",
		LlamaServerPath: "engines/llama-server.exe",
		APIBase:         "http://127.0.0.1:8080",
		Port:            8080,
		ContextSize:     8192,
		Temperature:     0.6,
		TopP:            0.95,
		TopK:            20,
		RepeatPenalty:   1.0,
		SystemPrompt:    "You are Qwen.",
	}

	if err := config.Save(cfgPath, original); err != nil {
		t.Fatalf("Save returned error: %v", err)
	}

	loaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	if loaded.ContextSize != 8192 {
		t.Errorf("expected ContextSize=8192, got %d", loaded.ContextSize)
	}
	if loaded.Temperature != 0.6 {
		t.Errorf("expected Temperature=0.6, got %f", loaded.Temperature)
	}
	if loaded.TopP != 0.95 {
		t.Errorf("expected TopP=0.95, got %f", loaded.TopP)
	}
	if loaded.TopK != 20 {
		t.Errorf("expected TopK=20, got %d", loaded.TopK)
	}
	if loaded.RepeatPenalty != 1.0 {
		t.Errorf("expected RepeatPenalty=1.0, got %f", loaded.RepeatPenalty)
	}
	if loaded.SystemPrompt != "You are Qwen." {
		t.Errorf("expected SystemPrompt='You are Qwen.', got '%s'", loaded.SystemPrompt)
	}
}

func TestSaveAndLoad_ModifiedGenerationParams(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	defaults := config.DefaultConfig()
	if err := config.Save(cfgPath, defaults); err != nil {
		t.Fatalf("Save defaults returned error: %v", err)
	}

	loaded1, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	loaded1.Temperature = 0.3
	loaded1.TopP = 0.7
	loaded1.TopK = 10
	loaded1.ContextSize = 16384
	loaded1.RepeatPenalty = 1.5
	loaded1.SystemPrompt = "Custom prompt"

	if err := config.Save(cfgPath, loaded1); err != nil {
		t.Fatalf("Save modified returned error: %v", err)
	}

	loaded2, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Load after modify returned error: %v", err)
	}

	if loaded2.Temperature != 0.3 {
		t.Errorf("expected modified Temperature=0.3, got %f", loaded2.Temperature)
	}
	if loaded2.TopP != 0.7 {
		t.Errorf("expected modified TopP=0.7, got %f", loaded2.TopP)
	}
	if loaded2.TopK != 10 {
		t.Errorf("expected modified TopK=10, got %d", loaded2.TopK)
	}
	if loaded2.ContextSize != 16384 {
		t.Errorf("expected modified ContextSize=16384, got %d", loaded2.ContextSize)
	}
	if loaded2.RepeatPenalty != 1.5 {
		t.Errorf("expected modified RepeatPenalty=1.5, got %f", loaded2.RepeatPenalty)
	}
	if loaded2.SystemPrompt != "Custom prompt" {
		t.Errorf("expected modified SystemPrompt='Custom prompt', got '%s'", loaded2.SystemPrompt)
	}
}

func TestValidate_GenerationParams(t *testing.T) {
	cfg := &config.Config{
		Port:          8080,
		ContextSize:   4096,
		Temperature:   0.5,
		TopP:          0.9,
		RepeatPenalty: 1.0,
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("valid config should pass validation, got: %v", err)
	}

	invalidTemp := &config.Config{
		Port:          8080,
		ContextSize:   4096,
		Temperature:   3.0,
		TopP:          0.9,
		RepeatPenalty: 1.0,
	}
	if err := invalidTemp.Validate(); err == nil {
		t.Error("Temperature=3.0 should fail validation")
	}

	invalidTopP := &config.Config{
		Port:          8080,
		ContextSize:   4096,
		Temperature:   0.5,
		TopP:          1.5,
		RepeatPenalty: 1.0,
	}
	if err := invalidTopP.Validate(); err == nil {
		t.Error("TopP=1.5 should fail validation")
	}

	invalidCtx := &config.Config{
		Port:          8080,
		ContextSize:   -1,
		Temperature:   0.5,
		TopP:          0.9,
		RepeatPenalty: 1.0,
	}
	if err := invalidCtx.Validate(); err == nil {
		t.Error("ContextSize=-1 should fail validation")
	}
}
