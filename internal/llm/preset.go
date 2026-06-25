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

var quantSuffixRe = regexp.MustCompile(`(?i)-(Q\d+(_[A-Z0-9]+)+|IQ\d+_[A-Z0-9]+|BF16|F16|F32)$`)

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

		if p.MmprojPath != "" {
			sb.WriteString(fmt.Sprintf("mmproj = %s\n", p.MmprojPath))
		}

		if p.MmprojOffload {
			sb.WriteString("mmproj-offload = 1\n")
		}

		if p.Alias != "" {
			sb.WriteString(fmt.Sprintf("alias = %s\n", p.Alias))
		}

		if p.CtxSize > 0 {
			sb.WriteString(fmt.Sprintf("ctx-size = %d\n", p.CtxSize))
		}

		if p.BatchSize > 0 {
			sb.WriteString(fmt.Sprintf("batch-size = %d\n", p.BatchSize))
		}

		if p.UBatchSize > 0 {
			sb.WriteString(fmt.Sprintf("ubatch-size = %d\n", p.UBatchSize))
		}

		if p.Threads > 0 {
			sb.WriteString(fmt.Sprintf("threads = %d\n", p.Threads))
		}

		if p.FlashAttn != "" {
			sb.WriteString(fmt.Sprintf("flash-attn = %s\n", p.FlashAttn))
		}

		if p.CacheTypeK != "" {
			sb.WriteString(fmt.Sprintf("cache-type-k = %s\n", p.CacheTypeK))
		}

		if p.CacheTypeV != "" {
			sb.WriteString(fmt.Sprintf("cache-type-v = %s\n", p.CacheTypeV))
		}

		if p.Mlock {
			sb.WriteString("mlock = 1\n")
		}

		if p.ImageMinTokens > 0 {
			sb.WriteString(fmt.Sprintf("image-min-tokens = %d\n", p.ImageMinTokens))
		}
		if p.ImageMaxTokens > 0 {
			sb.WriteString(fmt.Sprintf("image-max-tokens = %d\n", p.ImageMaxTokens))
		}

		if p.Reasoning != "" {
			sb.WriteString(fmt.Sprintf("reasoning = %s\n", p.Reasoning))
		}

		if p.ReasoningBudget > 0 {
			sb.WriteString(fmt.Sprintf("reasoning-budget = %d\n", p.ReasoningBudget))
		}

		if p.ReasoningFormat != "" {
			sb.WriteString(fmt.Sprintf("reasoning-format = %s\n", p.ReasoningFormat))
		}

		if p.Jinja {
			sb.WriteString("jinja = 1\n")
		}

		if p.SleepIdle > 0 {
			sb.WriteString(fmt.Sprintf("sleep-idle-seconds = %d\n", p.SleepIdle))
		}

		sb.WriteString("\n")
	}

	return sb.String()
}

func WritePresetFile(path string, content string) error {
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
		baseName := strings.TrimSuffix(mf, filepath.Ext(mf))
		mmprojPath := findMmprojInDir(modelsDir, baseName)
		name := DeriveModelName(baseName)

		preset := ModelPreset{
			Name:       name,
			ModelPath:  filepath.Join("models", mf),
			MmprojPath: mmprojPath,
			Jinja:      true,
			SleepIdle:  -1, // 与 llama.cpp 9793 默认值对齐，-1 禁用空闲休眠（不写入 preset.ini，继承全局默认）
		}

		if mmprojPath != "" {
			mmprojCaps := ReadMmprojCapabilities(mmprojPath)
			preset.MmprojVision = mmprojCaps.HasVision
			preset.MmprojAudio = mmprojCaps.HasAudio
			preset.MmprojVideo = mmprojCaps.HasVideo
		}

		presets = append(presets, preset)
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
			baseName := strings.TrimSuffix(mf, filepath.Ext(mf))
			mmprojPath := findMmprojInDir(subdir, baseName)
			name := DeriveModelName(baseName)

			preset := ModelPreset{
				Name:       name,
				ModelPath:  filepath.Join("models", entry.Name(), mf),
				MmprojPath: mmprojPath,
				Jinja:      true,
				SleepIdle:  120,
			}

			if mmprojPath != "" {
				mmprojCaps := ReadMmprojCapabilities(mmprojPath)
				preset.MmprojVision = mmprojCaps.HasVision
				preset.MmprojAudio = mmprojCaps.HasAudio
				preset.MmprojVideo = mmprojCaps.HasVideo
			}

			presets = append(presets, preset)
		}
	}

	sort.Slice(presets, func(i, j int) bool {
		return presets[i].Name < presets[j].Name
	})

	return presets, nil
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
	uncensoredRe := regexp.MustCompile(`(?i)[-_]U[-_]`)
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
