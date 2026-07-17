package llm_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"douya/internal/llm"
)

func TestSetDefaultAlias_ClearsPreviousAliases(t *testing.T) {
	presets := []llm.ModelPreset{
		{Name: "Model-A", ModelPath: "models/model-a.gguf", Alias: "default"},
		{Name: "Model-B", ModelPath: "models/model-b.gguf", Alias: ""},
		{Name: "Model-C", ModelPath: "models/model-c.gguf", Alias: ""},
	}

	llm.SetDefaultAlias(presets, "models/model-b.gguf")

	defaultCount := 0
	for _, p := range presets {
		if p.Alias == "default" {
			defaultCount++
		}
	}
	if defaultCount != 1 {
		t.Errorf("expected exactly 1 preset with alias 'default', got %d", defaultCount)
	}
	if presets[0].Alias != "" {
		t.Errorf("expected Model-A alias to be cleared, got %q", presets[0].Alias)
	}
	if presets[1].Alias != "default" {
		t.Errorf("expected Model-B alias to be 'default', got %q", presets[1].Alias)
	}
}

func TestSetDefaultAlias_MatchesByBaseName(t *testing.T) {
	presets := []llm.ModelPreset{
		{Name: "Model-A", ModelPath: "models\\model-a.gguf", Alias: "default"},
		{Name: "Model-B", ModelPath: "models\\model-b.gguf", Alias: ""},
	}

	llm.SetDefaultAlias(presets, "models/model-b.gguf")

	if presets[0].Alias != "" {
		t.Errorf("expected Model-A alias to be cleared, got %q", presets[0].Alias)
	}
	if presets[1].Alias != "default" {
		t.Errorf("expected Model-B alias to be 'default', got %q", presets[1].Alias)
	}
}

func TestSetDefaultAlias_NoMatchFallsBackToFirst(t *testing.T) {
	presets := []llm.ModelPreset{
		{Name: "Model-A", ModelPath: "models/model-a.gguf", Alias: "default"},
		{Name: "Model-B", ModelPath: "models/model-b.gguf", Alias: ""},
	}

	llm.SetDefaultAlias(presets, "models/nonexistent.gguf")

	if presets[0].Alias != "default" {
		t.Errorf("expected first model to be default when no match, got %q", presets[0].Alias)
	}
	if presets[1].Alias != "" {
		t.Errorf("expected Model-B alias to be empty, got %q", presets[1].Alias)
	}
}

func TestSetDefaultAlias_EmptyDefaultPath(t *testing.T) {
	presets := []llm.ModelPreset{
		{Name: "Model-A", ModelPath: "models/model-a.gguf", Alias: "default"},
		{Name: "Model-B", ModelPath: "models/model-b.gguf", Alias: ""},
	}

	llm.SetDefaultAlias(presets, "")

	if presets[0].Alias != "default" {
		t.Errorf("expected first model to be default when path is empty, got %q", presets[0].Alias)
	}
	if presets[1].Alias != "" {
		t.Errorf("expected Model-B alias to be empty, got %q", presets[1].Alias)
	}
}

func TestGeneratePreset_NoDuplicateAliases(t *testing.T) {
	presets := []llm.ModelPreset{
		{Name: "Model-A", ModelPath: "models/model-a.gguf", Alias: "default", CtxSize: 4096, Jinja: true, SleepIdle: 120},
		{Name: "Model-B", ModelPath: "models/model-b.gguf", Alias: "default", CtxSize: 8192, Jinja: true, SleepIdle: 120},
	}

	llm.SetDefaultAlias(presets, "models/model-b.gguf")

	content := llm.GeneratePreset(presets, nil)

	defaultCount := strings.Count(content, "alias = default")
	if defaultCount != 1 {
		t.Errorf("expected exactly 1 'alias = default' in preset, got %d\ncontent:\n%s", defaultCount, content)
	}
}

func TestGeneratePreset_UsesAbsoluteModelPaths(t *testing.T) {
	presets := []llm.ModelPreset{
		{Name: "Model-A", ModelPath: "C:\\app\\models\\model-a.gguf", CtxSize: 4096, Jinja: true, SleepIdle: 120},
	}

	content := llm.GeneratePreset(presets, nil)

	if !strings.Contains(content, "model = C:\\app\\models\\model-a.gguf") {
		t.Errorf("expected absolute model path in preset, got:\n%s", content)
	}
}

func TestScanModelsDir_SetsNoDefaultAlias(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "models-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	modelFiles := []string{"Model-A-Q4_K_M.gguf", "Model-B-Q4_K_M.gguf"}
	for _, mf := range modelFiles {
		f, err := os.Create(filepath.Join(tmpDir, mf))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	presets, err := llm.ScanModelsDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range presets {
		if p.Alias != "" {
			t.Errorf("expected no default alias from ScanModelsDir, got %q for %s", p.Alias, p.Name)
		}
	}
}

func TestScanModelsDir_FindsMmprojForQwenModel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "models-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	modelFile := "Qwen3.5U-9B-Q4_K_M.gguf"
	mmprojFile := "mmproj-Qwen3.5-9B-U-BF16.gguf"

	f, err := os.Create(filepath.Join(tmpDir, modelFile))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	f, err = os.Create(filepath.Join(tmpDir, mmprojFile))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	presets, err := llm.ScanModelsDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(presets) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(presets))
	}
	if presets[0].MmprojPath == "" {
		t.Error("expected mmproj path to be found for Qwen3.5U model, got empty")
	}
}

func TestScanModelsDir_FindsMmprojForGemmaModel(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "models-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	modelFile := "Gemma-4-E4B-U-Q4_K_M.gguf"
	mmprojFile := "mmproj-Gemma-4-E4B-U-f16.gguf"

	f, err := os.Create(filepath.Join(tmpDir, modelFile))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	f, err = os.Create(filepath.Join(tmpDir, mmprojFile))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	presets, err := llm.ScanModelsDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(presets) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(presets))
	}
	if presets[0].MmprojPath == "" {
		t.Error("expected mmproj path to be found for Gemma model, got empty")
	}
}

func TestScanModelsDir_SubdirStructure(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-root")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	modelsDir := filepath.Join(tmpDir, "models")
	gemmaDir := filepath.Join(modelsDir, "Gemma-4-E4B-U-Q4_K_M")
	if err := os.MkdirAll(gemmaDir, 0755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(gemmaDir, "Gemma-4-E4B-U-Q4_K_M.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	f, err = os.Create(filepath.Join(gemmaDir, "mmproj-Gemma-4-E4B-U-f16.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	presets, err := llm.ScanModelsDir(modelsDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(presets) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(presets))
	}

	p := presets[0]
	expectedModelPath := filepath.Join("models", "Gemma-4-E4B-U-Q4_K_M", "Gemma-4-E4B-U-Q4_K_M.gguf")
	if p.ModelPath != expectedModelPath {
		t.Errorf("expected model path %q, got %q", expectedModelPath, p.ModelPath)
	}

	expectedMmprojPath := filepath.Join("models", "Gemma-4-E4B-U-Q4_K_M", "mmproj-Gemma-4-E4B-U-f16.gguf")
	if p.MmprojPath != expectedMmprojPath {
		t.Errorf("expected mmproj path %q, got %q", expectedMmprojPath, p.MmprojPath)
	}
}

func TestScanModelsDir_SubdirMultipleModels(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-root")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	modelsDir := filepath.Join(tmpDir, "models")

	subdirs := []struct {
		dirName    string
		modelFile  string
		mmprojFile string
	}{
		{
			"Gemma-4-E4B-U-Q4_K_M",
			"Gemma-4-E4B-U-Q4_K_M.gguf",
			"mmproj-Gemma-4-E4B-U-f16.gguf",
		},
		{
			"Qwen3.5-9B-U-Q4_K_M",
			"Qwen3.5U-9B-Q4_K_M.gguf",
			"mmproj-Qwen3.5-9B-U-BF16.gguf",
		},
	}

	for _, sd := range subdirs {
		subdir := filepath.Join(modelsDir, sd.dirName)
		if err := os.MkdirAll(subdir, 0755); err != nil {
			t.Fatal(err)
		}
		f, err := os.Create(filepath.Join(subdir, sd.modelFile))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
		f, err = os.Create(filepath.Join(subdir, sd.mmprojFile))
		if err != nil {
			t.Fatal(err)
		}
		f.Close()
	}

	presets, err := llm.ScanModelsDir(modelsDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(presets) != 2 {
		t.Fatalf("expected 2 presets, got %d", len(presets))
	}

	for _, p := range presets {
		if p.MmprojPath == "" {
			t.Errorf("expected mmproj path for model %s, got empty", p.Name)
		}
		if !strings.HasPrefix(p.ModelPath, filepath.Join("models")) {
			t.Errorf("expected model path to start with 'models', got %q", p.ModelPath)
		}
	}
}

func TestScanModelsDir_SubdirNoMmproj(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-root")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	modelsDir := filepath.Join(tmpDir, "models")
	subdir := filepath.Join(modelsDir, "Model-A-Q4_K_M")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(subdir, "Model-A-Q4_K_M.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	presets, err := llm.ScanModelsDir(modelsDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(presets) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(presets))
	}
	if presets[0].MmprojPath != "" {
		t.Errorf("expected empty mmproj path, got %q", presets[0].MmprojPath)
	}
}

func TestScanModelsDir_FlatTakesPrecedenceOverSubdir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "test-root")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	modelsDir := filepath.Join(tmpDir, "models")
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		t.Fatal(err)
	}

	f, err := os.Create(filepath.Join(modelsDir, "FlatModel-Q4_K_M.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	subdir := filepath.Join(modelsDir, "SubdirModel-Q4_K_M")
	if err := os.MkdirAll(subdir, 0755); err != nil {
		t.Fatal(err)
	}
	f, err = os.Create(filepath.Join(subdir, "SubdirModel-Q4_K_M.gguf"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	presets, err := llm.ScanModelsDir(modelsDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(presets) != 1 {
		t.Fatalf("expected 1 preset (flat only), got %d", len(presets))
	}
	if presets[0].Name != "FlatModel" {
		t.Errorf("expected flat model 'FlatModel', got %q", presets[0].Name)
	}
}

func TestScanModelsDir_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "models-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	presets, err := llm.ScanModelsDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) != 0 {
		t.Errorf("expected 0 presets for empty dir, got %d", len(presets))
	}
}

func TestGeneratePreset_MmprojOffload(t *testing.T) {
	presets := []llm.ModelPreset{
		{Name: "Model-A", ModelPath: "models/model-a.gguf", MmprojOffload: true, CtxSize: 4096, Jinja: true, SleepIdle: 120},
	}

	content := llm.GeneratePreset(presets, nil)

	if !strings.Contains(content, "mmproj-offload = 1") {
		t.Errorf("expected 'mmproj-offload = 1' in preset, got:\n%s", content)
	}
}

func TestGeneratePreset_ReasoningBudget(t *testing.T) {
	presets := []llm.ModelPreset{
		{Name: "Model-A", ModelPath: "models/model-a.gguf", Reasoning: "on", ReasoningBudget: 4096, ReasoningFormat: "deepseek", CtxSize: 4096, Jinja: true, SleepIdle: 120},
	}

	content := llm.GeneratePreset(presets, nil)

	if !strings.Contains(content, "reasoning = on") {
		t.Errorf("expected 'reasoning = on' in preset, got:\n%s", content)
	}
	if !strings.Contains(content, "reasoning-budget = 4096") {
		t.Errorf("expected 'reasoning-budget = 4096' in preset, got:\n%s", content)
	}
	if !strings.Contains(content, "reasoning-format = deepseek") {
		t.Errorf("expected 'reasoning-format = deepseek' in preset, got:\n%s", content)
	}
}

func TestGeneratePreset_NoMmprojOffloadWhenFalse(t *testing.T) {
	presets := []llm.ModelPreset{
		{Name: "Model-A", ModelPath: "models/model-a.gguf", MmprojOffload: false, CtxSize: 4096, Jinja: true, SleepIdle: 120},
	}

	content := llm.GeneratePreset(presets, nil)

	if strings.Contains(content, "mmproj-offload") {
		t.Errorf("expected no 'mmproj-offload' in preset when false, got:\n%s", content)
	}
}
