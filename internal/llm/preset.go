package llm

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"douya/internal/system"
)

var (
	quantSuffixRe = regexp.MustCompile(`(?i)-(Q\d+(_[A-Z0-9]+)+|IQ\d+_[A-Z0-9]+|BF16|F16|F32)$`)
	uncensoredRe  = regexp.MustCompile(`(?i)[-_]U[-_]`)
)

type ModelPreset struct {
	Name            string
	ModelPath       string
	MmprojPath      string
	MmprojVision    bool
	MmprojAudio  bool
	MmprojVideo  bool   `json:"mmproj_video"`
	MmprojOffload bool
	Alias           string
	CtxSize         int
	BatchSize       int
	UBatchSize      int
	Threads         int
	FlashAttn       string // "on"/"off"/"auto"
	CacheTypeK      string
	CacheTypeV      string
	Mlock           bool
	ImageMinTokens  int
	ImageMaxTokens  int
	Reasoning       string
	ReasoningBudget int
	ReasoningFormat string
	Jinja           bool
	SleepIdle       int
}

type ModelOption struct {
	Name         string `json:"name"`
	ModelPath    string `json:"model_path"`
	FileName     string `json:"file_name"`
	IsDefault    bool   `json:"is_default"`
	IsLoaded     bool   `json:"is_loaded"`
	MmprojVision bool   `json:"mmproj_vision"`
	MmprojAudio  bool   `json:"mmproj_audio"`
	MmprojVideo  bool   `json:"mmproj_video"`
	Status       string `json:"status"`
}

// writeStringField 写入字符串字段（值为空则跳过）。
// 生活类比：像填表时，某栏没填就不写那行，避免空白行。
func writeStringField(sb *strings.Builder, name, val string) {
	if val != "" {
		sb.WriteString(fmt.Sprintf("%s = %s\n", name, val))
	}
}

// writeIntField 写入整数字段（值 <= 0 则跳过）。
func writeIntField(sb *strings.Builder, name string, val int) {
	if val > 0 {
		sb.WriteString(fmt.Sprintf("%s = %d\n", name, val))
	}
}

// writeBoolField 写入布尔字段（值为 false 则跳过，true 写入 "1"）。
func writeBoolField(sb *strings.Builder, name string, val bool) {
	if val {
		sb.WriteString(fmt.Sprintf("%s = 1\n", name))
	}
}

func GeneratePreset(presets []ModelPreset, globalDefaults map[string]string) string {
	var sb strings.Builder

	if len(globalDefaults) > 0 {
		sb.WriteString("[*]\n")
		keys := make([]string, 0, len(globalDefaults))
		for k := range globalDefaults {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			sb.WriteString(fmt.Sprintf("%s = %s\n", k, globalDefaults[k]))
		}
		sb.WriteString("\n")
	}

	for _, p := range presets {
		sb.WriteString(fmt.Sprintf("[%s]\n", p.Name))
		sb.WriteString(fmt.Sprintf("model = %s\n", p.ModelPath))

		writeStringField(&sb, "mmproj", p.MmprojPath)
		writeBoolField(&sb, "mmproj-offload", p.MmprojOffload)
		writeStringField(&sb, "alias", p.Alias)
		writeIntField(&sb, "ctx-size", p.CtxSize)
		writeIntField(&sb, "batch-size", p.BatchSize)
		writeIntField(&sb, "ubatch-size", p.UBatchSize)
		writeIntField(&sb, "threads", p.Threads)
		writeStringField(&sb, "flash-attn", p.FlashAttn)
		writeStringField(&sb, "cache-type-k", p.CacheTypeK)
		writeStringField(&sb, "cache-type-v", p.CacheTypeV)
		writeBoolField(&sb, "mlock", p.Mlock)
		writeIntField(&sb, "image-min-tokens", p.ImageMinTokens)
		writeIntField(&sb, "image-max-tokens", p.ImageMaxTokens)
		writeStringField(&sb, "reasoning", p.Reasoning)
		writeIntField(&sb, "reasoning-budget", p.ReasoningBudget)
		writeStringField(&sb, "reasoning-format", p.ReasoningFormat)
		writeBoolField(&sb, "jinja", p.Jinja)
		writeIntField(&sb, "sleep-idle-seconds", p.SleepIdle)

		sb.WriteString("\n")
	}

	return sb.String()
}

// WritePresetFile 写入预设文件，仅在文件不存在或内容不一致时才覆盖。
// 原因：避免每次启动都无意义地覆盖 router-preset.ini，减少磁盘写入并保持文件时间戳稳定。
func WritePresetFile(path string, content string) error {
	// 文件已存在且内容一致时跳过写入
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) == content {
			return nil
		}
	} else if !os.IsNotExist(err) {
		// 其他读取错误（如权限问题）也继续尝试写入
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create preset dir: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0644)
}

func SetDefaultAlias(presets []ModelPreset, defaultModelPath string) {
	for i := range presets {
		presets[i].Alias = ""
	}

	if defaultModelPath != "" {
		for i := range presets {
			if presets[i].ModelPath == defaultModelPath || filepath.Base(presets[i].ModelPath) == filepath.Base(defaultModelPath) {
				presets[i].Alias = "default"
				return
			}
		}
	}

	if len(presets) > 0 {
		presets[0].Alias = "default"
	}
}

func ScanModelsDir(modelsDir string) ([]ModelPreset, error) {
	flatModels, err := scanFlatModels(modelsDir)
	if err != nil {
		return nil, err
	}
	if len(flatModels) > 0 {
		return flatModels, nil
	}
	return scanSubdirModels(modelsDir)
}

func scanFlatModels(modelsDir string) ([]ModelPreset, error) {
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return nil, err
	}

	var modelFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(strings.ToLower(name), ".gguf") && !strings.HasPrefix(strings.ToLower(name), "mmproj-") {
			modelFiles = append(modelFiles, name)
		}
	}

	if len(modelFiles) == 0 {
		return nil, nil
	}

	sort.Strings(modelFiles)

	presets := make([]ModelPreset, 0, len(modelFiles))
	for _, mf := range modelFiles {
		// 扁平结构：modelPath = models/<filename>，SleepIdle = -1（禁用空闲休眠）
		presets = append(presets, buildPresetFromModelFile(modelsDir, mf, filepath.Join("models", mf), -1))
	}

	return presets, nil
}

func scanSubdirModels(modelsDir string) ([]ModelPreset, error) {
	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		return nil, fmt.Errorf("read models dir: %w", err)
	}

	var presets []ModelPreset
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		subdir := filepath.Join(modelsDir, entry.Name())
		subEntries, err := os.ReadDir(subdir)
		if err != nil {
			continue
		}

		var modelFiles []string
		for _, se := range subEntries {
			if se.IsDir() {
				continue
			}
			name := se.Name()
			if strings.HasSuffix(strings.ToLower(name), ".gguf") && !strings.HasPrefix(strings.ToLower(name), "mmproj-") {
				modelFiles = append(modelFiles, name)
			}
		}

		for _, mf := range modelFiles {
			// 子目录结构：modelPath = models/<subdir>/<filename>，SleepIdle = 120
			presets = append(presets, buildPresetFromModelFile(subdir, mf, filepath.Join("models", entry.Name(), mf), 120))
		}
	}

	sort.Slice(presets, func(i, j int) bool {
		return presets[i].Name < presets[j].Name
	})

	return presets, nil
}

// buildPresetFromModelFile 从模型文件构建 ModelPreset。
// 生活类比：像档案管理员根据文件名和所在目录，填好一张标准档案卡片（preset）。
//
// 参数：
//   - dir: 模型文件所在目录（用于查找同目录下的 mmproj 文件）
//   - fileName: 模型文件名（如 "gemma-3-4b.gguf"）
//   - modelPath: preset 中存储的相对路径（如 "models/gemma-3-4b.gguf" 或 "models/sub/gemma-3-4b.gguf"）
//   - sleepIdle: 空闲休眠参数（扁平结构 -1，子目录结构 120）
func buildPresetFromModelFile(dir, fileName, modelPath string, sleepIdle int) ModelPreset {
	baseName := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	mmprojPath := findMmprojInDir(dir, baseName)

	preset := ModelPreset{
		Name:       DeriveModelName(baseName),
		ModelPath:  modelPath,
		MmprojPath: mmprojPath,
		Jinja:      true,
		SleepIdle:  sleepIdle,
	}

	if mmprojPath != "" {
		mmprojCaps := ReadMmprojCapabilities(mmprojPath)
		preset.MmprojVision = mmprojCaps.HasVision
		preset.MmprojAudio = mmprojCaps.HasAudio
		preset.MmprojVideo = mmprojCaps.HasVideo
	}

	return preset
}

func findMmprojInDir(dir string, modelBaseName string) string {
	allMmprojFiles, err := filepath.Glob(filepath.Join(dir, "mmproj-*.gguf"))
	if err != nil || len(allMmprojFiles) == 0 {
		return ""
	}

	if len(allMmprojFiles) == 1 {
		return makeRelativeModelPath(dir, filepath.Base(allMmprojFiles[0]))
	}

	keywords := extractKeywords(modelBaseName)
	var bestMatch string
	var bestMatchScore int

	for _, mmprojFile := range allMmprojFiles {
		mmprojName := filepath.Base(mmprojFile)
		score := 0
		for _, kw := range keywords {
			if strings.Contains(strings.ToLower(mmprojName), strings.ToLower(kw)) {
				score++
			}
		}
		if score > bestMatchScore {
			bestMatchScore = score
			bestMatch = mmprojName
		}
	}

	if bestMatch != "" {
		return makeRelativeModelPath(dir, bestMatch)
	}

	return ""
}

func makeRelativeModelPath(dir string, fileName string) string {
	parts := strings.SplitAfterN(filepath.ToSlash(dir), "models/", 2)
	if len(parts) == 2 {
		return filepath.Join("models", parts[1], fileName)
	}
	return filepath.Join("models", fileName)
}

type MmprojCapabilities struct {
	HasVision bool
	HasAudio  bool
	HasVideo  bool
}

func ReadMmprojCapabilities(mmprojPath string) MmprojCapabilities {
	caps := MmprojCapabilities{}

	absPath := mmprojPath
	if !filepath.IsAbs(absPath) {
		absPath = filepath.Join(".", mmprojPath)
	}

	kvMap, err := system.ParseGGUFKV(absPath)
	if err != nil {
		return caps
	}

	if v, ok := kvMap["clip.has_vision_encoder"].(bool); ok {
		caps.HasVision = v
	}
	if v, ok := kvMap["clip.has_audio_encoder"].(bool); ok {
		caps.HasAudio = v
	}
	if v, ok := kvMap["clip.has_video_encoder"].(bool); ok {
		caps.HasVideo = v
	}

	return caps
}

func StripQuantSuffix(name string) string {
	return quantSuffixRe.ReplaceAllString(name, "")
}

func extractKeywords(modelBase string) []string {
	// 先用常见分隔符拆分
	parts := strings.FieldsFunc(modelBase, func(r rune) bool {
		return r == '-' || r == '_' || r == ' '
	})

	// 过滤掉一些常见的没有意义的词
	var keywords []string
	for _, p := range parts {
		if p != "" && len(p) > 1 {
			keywords = append(keywords, p)
		}
	}

	return keywords
}

func DeriveModelName(filename string) string {
	name := filename

	name = strings.TrimSuffix(name, ".gguf")
	name = strings.TrimSuffix(name, ".GGUF")

	name = quantSuffixRe.ReplaceAllString(name, "")

	// Normalize "uncensored" markers: -U-, -U_, _U-, _U_
	name = uncensoredRe.ReplaceAllString(name, "-")

	// Replace underscores with hyphens for display (common convention)
	name = strings.ReplaceAll(name, "_", "-")

	// Collapse multiple consecutive hyphens
	for strings.Contains(name, "--") {
		name = strings.ReplaceAll(name, "--", "-")
	}
	name = strings.Trim(name, "-")

	return name
}
