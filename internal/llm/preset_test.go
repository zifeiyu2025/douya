package llm

import (
	"os"
	"path/filepath"
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

	// 以下子测试合并自 tests/llm/preset_test.go（黑盒测试）

	t.Run("清除所有旧的default别名并仅保留一个", func(t *testing.T) {
		// 验证：即使存在多个 preset，设置后只有一个 default
		presets := []ModelPreset{
			{Name: "Model-A", ModelPath: "models/model-a.gguf", Alias: "default"},
			{Name: "Model-B", ModelPath: "models/model-b.gguf", Alias: ""},
			{Name: "Model-C", ModelPath: "models/model-c.gguf", Alias: ""},
		}

		SetDefaultAlias(presets, "models/model-b.gguf")

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
	})

	t.Run("跨路径分隔符匹配basename", func(t *testing.T) {
		// 验证：Windows 反斜杠路径也能匹配正斜杠输入
		presets := []ModelPreset{
			{Name: "Model-A", ModelPath: "models\\model-a.gguf", Alias: "default"},
			{Name: "Model-B", ModelPath: "models\\model-b.gguf", Alias: ""},
		}

		SetDefaultAlias(presets, "models/model-b.gguf")

		if presets[0].Alias != "" {
			t.Errorf("expected Model-A alias to be cleared, got %q", presets[0].Alias)
		}
		if presets[1].Alias != "default" {
			t.Errorf("expected Model-B alias to be 'default', got %q", presets[1].Alias)
		}
	})

	t.Run("空defaultModelPath回退到第一个", func(t *testing.T) {
		// 验证：空路径时第一个模型作为默认
		presets := []ModelPreset{
			{Name: "Model-A", ModelPath: "models/model-a.gguf", Alias: "default"},
			{Name: "Model-B", ModelPath: "models/model-b.gguf", Alias: ""},
		}

		SetDefaultAlias(presets, "")

		if presets[0].Alias != "default" {
			t.Errorf("expected first model to be default when path is empty, got %q", presets[0].Alias)
		}
		if presets[1].Alias != "" {
			t.Errorf("expected Model-B alias to be empty, got %q", presets[1].Alias)
		}
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

	// 以下子测试合并自 tests/llm/preset_test.go（黑盒测试）

	t.Run("不产生重复的default别名", func(t *testing.T) {
		// 验证：两个 preset 都标为 default 时，SetDefaultAlias 后 GeneratePreset 只输出一个
		presets := []ModelPreset{
			{Name: "Model-A", ModelPath: "models/model-a.gguf", Alias: "default", CtxSize: 4096, Jinja: true, SleepIdle: 120},
			{Name: "Model-B", ModelPath: "models/model-b.gguf", Alias: "default", CtxSize: 8192, Jinja: true, SleepIdle: 120},
		}

		SetDefaultAlias(presets, "models/model-b.gguf")

		content := GeneratePreset(presets, nil)

		defaultCount := strings.Count(content, "alias = default")
		if defaultCount != 1 {
			t.Errorf("expected exactly 1 'alias = default' in preset, got %d\ncontent:\n%s", defaultCount, content)
		}
	})

	t.Run("使用绝对模型路径", func(t *testing.T) {
		// 验证：绝对路径原样输出
		presets := []ModelPreset{
			{Name: "Model-A", ModelPath: "C:\\app\\models\\model-a.gguf", CtxSize: 4096, Jinja: true, SleepIdle: 120},
		}

		content := GeneratePreset(presets, nil)

		if !strings.Contains(content, "model = C:\\app\\models\\model-a.gguf") {
			t.Errorf("expected absolute model path in preset, got:\n%s", content)
		}
	})

	t.Run("MmprojOffload为true时输出mmproj-offload", func(t *testing.T) {
		presets := []ModelPreset{
			{Name: "Model-A", ModelPath: "models/model-a.gguf", MmprojOffload: true, CtxSize: 4096, Jinja: true, SleepIdle: 120},
		}

		content := GeneratePreset(presets, nil)

		if !strings.Contains(content, "mmproj-offload = 1") {
			t.Errorf("expected 'mmproj-offload = 1' in preset, got:\n%s", content)
		}
	})

	t.Run("Reasoning相关字段输出", func(t *testing.T) {
		presets := []ModelPreset{
			{Name: "Model-A", ModelPath: "models/model-a.gguf", Reasoning: "on", ReasoningBudget: 4096, ReasoningFormat: "deepseek", CtxSize: 4096, Jinja: true, SleepIdle: 120},
		}

		content := GeneratePreset(presets, nil)

		if !strings.Contains(content, "reasoning = on") {
			t.Errorf("expected 'reasoning = on' in preset, got:\n%s", content)
		}
		if !strings.Contains(content, "reasoning-budget = 4096") {
			t.Errorf("expected 'reasoning-budget = 4096' in preset, got:\n%s", content)
		}
		if !strings.Contains(content, "reasoning-format = deepseek") {
			t.Errorf("expected 'reasoning-format = deepseek' in preset, got:\n%s", content)
		}
	})

	t.Run("MmprojOffload为false时不输出mmproj-offload", func(t *testing.T) {
		presets := []ModelPreset{
			{Name: "Model-A", ModelPath: "models/model-a.gguf", MmprojOffload: false, CtxSize: 4096, Jinja: true, SleepIdle: 120},
		}

		content := GeneratePreset(presets, nil)

		if strings.Contains(content, "mmproj-offload") {
			t.Errorf("expected no 'mmproj-offload' in preset when false, got:\n%s", content)
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

// TestMakeRelativeModelPath_WithModelsDir 验证 dir 包含 "models/" 时生成相对路径
//
// 生活类比：就像快递地址，如果收件地址是"北京市朝阳区 models 路 5 号"，
// 只需要保留 "models 路 5 号" 这个相对部分，再加上门牌号（fileName）。
func TestMakeRelativeModelPath_WithModelsDir(t *testing.T) {
	cases := []struct {
		name     string
		dir      string
		fileName string
		want     string
	}{
		{
			name:     "标准 models 子目录",
			dir:      "C:/app/models/qwen",
			fileName: "model.gguf",
			want:     filepath.Join("models", "qwen", "model.gguf"),
		},
		{
			name:     "models 在路径中间",
			dir:      "/home/user/models/sub/deepseek",
			fileName: "ds.gguf",
			want:     filepath.Join("models", "sub/deepseek", "ds.gguf"),
		},
		{
			name:     "Windows 反斜杠路径",
			dir:      `D:\app\models\gemma`,
			fileName: "gemma.gguf",
			want:     filepath.Join("models", "gemma", "gemma.gguf"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := makeRelativeModelPath(c.dir, c.fileName)
			if got != c.want {
				t.Errorf("makeRelativeModelPath(%q, %q) = %q, 期望 %q", c.dir, c.fileName, got, c.want)
			}
		})
	}
}

// TestMakeRelativeModelPath_WithoutModelsDir 验证 dir 不含 "models/" 时回退为 models/fileName
func TestMakeRelativeModelPath_WithoutModelsDir(t *testing.T) {
	cases := []struct {
		name     string
		dir      string
		fileName string
		want     string
	}{
		{
			name:     "普通目录",
			dir:      "C:/app/other",
			fileName: "model.gguf",
			want:     filepath.Join("models", "model.gguf"),
		},
		{
			name:     "空目录",
			dir:      "",
			fileName: "model.gguf",
			want:     filepath.Join("models", "model.gguf"),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := makeRelativeModelPath(c.dir, c.fileName)
			if got != c.want {
				t.Errorf("makeRelativeModelPath(%q, %q) = %q, 期望 %q", c.dir, c.fileName, got, c.want)
			}
		})
	}
}

// === 以下 ScanModelsDir 测试用例合并自 tests/llm/preset_test.go（黑盒测试） ===

// TestScanModelsDir_SetsNoDefaultAlias 验证 ScanModelsDir 不设置 default 别名
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

	presets, err := ScanModelsDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, p := range presets {
		if p.Alias != "" {
			t.Errorf("expected no default alias from ScanModelsDir, got %q for %s", p.Alias, p.Name)
		}
	}
}

// TestScanModelsDir_FindsMmprojForQwenModel 验证为 Qwen 模型查找 mmproj
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

	presets, err := ScanModelsDir(tmpDir)
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

// TestScanModelsDir_FindsMmprojForGemmaModel 验证为 Gemma 模型查找 mmproj
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

	presets, err := ScanModelsDir(tmpDir)
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

// TestScanModelsDir_SubdirStructure 验证子目录结构下的模型扫描
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

	presets, err := ScanModelsDir(modelsDir)
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

// TestScanModelsDir_SubdirMultipleModels 验证多个子目录模型的扫描
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

	presets, err := ScanModelsDir(modelsDir)
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

// TestScanModelsDir_SubdirNoMmproj 验证子目录无 mmproj 时返回空 MmprojPath
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

	presets, err := ScanModelsDir(modelsDir)
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

// TestScanModelsDir_FlatTakesPrecedenceOverSubdir 验证平铺文件优先于子目录
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

	presets, err := ScanModelsDir(modelsDir)
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

// TestScanModelsDir_EmptyDir 验证空目录返回空 preset 列表
func TestScanModelsDir_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "models-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	presets, err := ScanModelsDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) != 0 {
		t.Errorf("expected 0 presets for empty dir, got %d", len(presets))
	}
}
