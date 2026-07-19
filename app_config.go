package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/store"
)

// getConfig 在读锁保护下获取 config 指针快照，调用方仅用于读取字段，不应修改返回值。
func (a *App) getConfig() *config.Config {
	a.configMu.RLock()
	defer a.configMu.RUnlock()
	return a.config
}

// setConfig 在写锁保护下整体替换 config 指针。
func (a *App) setConfig(cfg *config.Config) {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	a.config = cfg
}

func (a *App) GetConfig() *config.Config {
	a.configMu.Lock()
	defer a.configMu.Unlock()
	if a.config == nil {
		cfgPath := filepath.Join(appDir(), "config.json")
		cfg, err := config.Load(cfgPath)
		if err != nil || cfg == nil {
			cfg = config.DefaultConfig()
		}
		a.config = cfg
	}
	return a.config
}

func (a *App) UpdateConfig(cfg *config.Config) error {
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	if a.service != nil {
		a.service.UpdateConfig(cfg)
	}
	a.setConfig(cfg)

	a.client = llm.NewClient(cfg.APIBase, a.getServerAPIKey())

	searchChain := a.buildSearchChain()

	if a.service != nil {
		a.service.UpdateClient(a.client)
		a.service.UpdateSearchChain(searchChain)
	}

	return config.Save(filepath.Join(appDir(), "config.json"), cfg)
}

// HasServerAPIKey 返回是否已设置 API Key（不暴露实际密钥值给前端）
func (a *App) HasServerAPIKey() bool {
	return a.getServerAPIKey() != ""
}

// getServerAPIKey 内部方法，获取实际的 API Key 值
// 当 ServerAPIKeyEnabled 为 false 时返回空字符串，不发送 API Key
func (a *App) getServerAPIKey() string {
	if !a.getConfig().ServerAPIKeyEnabled {
		return ""
	}
	var value string
	if a.encKey != nil {
		if v, err := store.GetEncryptedSetting(a.db, "server_api_key", a.encKey); err == nil {
			value = v
		}
	}
	if value == "" {
		if v, err := store.GetSetting(a.db, "server_api_key"); err == nil {
			value = v
		}
	}
	return value
}

func (a *App) SetServerAPIKey(key string) error {
	if a.encKey != nil {
		return store.SetEncryptedSetting(a.db, "server_api_key", key, a.encKey)
	}
	return store.SetSetting(a.db, "server_api_key", key)
}

// PathCheckResult 启动路径检查的结构化结果
//
// 生活类比：像出门前的行李检查清单，分成"必需品"（runtime）和"重要物品"（models）两类。
// 必需品缺失出不了门（终止启动），重要物品缺失可以出门但影响体验（警告但继续）。
type PathCheckResult struct {
	// RuntimeMissing runtime 目录缺失的文件列表（每项格式 "类型: 路径"）
	// 非空表示 AI 推理引擎不完整，必须修复才能启动
	RuntimeMissing []string
	// ModelsDir 模型目录路径
	ModelsDir string
	// ModelsEmpty models 目录是否为空（无 .gguf 文件）
	// 为 true 时警告但不阻止启动，允许用户进入应用后续下载模型
	ModelsEmpty bool
	// ModelsDirMissing models 目录本身不存在（比"空"更严重，但仍不阻止启动）
	ModelsDirMissing bool
}

// HasRuntimeIssues 是否存在 runtime 问题（需要终止启动）
func (r PathCheckResult) HasRuntimeIssues() bool {
	return len(r.RuntimeMissing) > 0
}

// HasModelIssues 是否存在 models 问题（警告但不终止）
func (r PathCheckResult) HasModelIssues() bool {
	return r.ModelsEmpty || r.ModelsDirMissing
}

// validatePaths 检查启动所需的关键文件是否缺失，返回分类的结构化结果。
//
// 检查范围：
//   - runtime/ 目录：引擎 exe、6 个核心 DLL、3 个 CUDA 运行时 DLL（仅当 ggml-cuda.dll 存在时）
//   - models/ 目录：是否存在、是否含有 .gguf 模型文件
//
// 生活类比：
//   - runtime 是"发动机舱"——里面有发动机(llama-server.exe)和传动系统(DLL)，缺一不可
//   - models 是"油箱"——空了车也能点火，但跑不起来，需要用户去"加油"（下载模型）
func (a *App) validatePaths() PathCheckResult {
	result := PathCheckResult{}
	baseDir := appDir()

	// ===== 1. 检查 runtime 目录完整性 =====
	cfg := a.getConfig()
	serverPath := resolvePath(cfg.LlamaServerPath)
	if _, err := os.Stat(serverPath); err != nil {
		result.RuntimeMissing = append(result.RuntimeMissing, fmt.Sprintf("引擎程序: %s", serverPath))
	}

	runtimeDir := filepath.Join(baseDir, "runtime")
	if info, err := os.Stat(runtimeDir); err != nil || !info.IsDir() {
		result.RuntimeMissing = append(result.RuntimeMissing, fmt.Sprintf("运行时目录: %s", runtimeDir))
	} else {
		// runtime/ 目录存在，检查核心引擎 DLL
		// 生活类比：引擎 exe 是"发动机"，这些 DLL 是"传动系统"，缺一不可
		coreDLLs := []string{
			"llama.dll",
			"llama-server-impl.dll",
			"llama-common.dll",
			"ggml.dll",
			"ggml-base.dll",
			"ggml-cpu.dll",
		}
		for _, dll := range coreDLLs {
			dllPath := filepath.Join(runtimeDir, dll)
			if _, err := os.Stat(dllPath); err != nil {
				result.RuntimeMissing = append(result.RuntimeMissing, fmt.Sprintf("核心DLL: %s", dllPath))
			}
		}

		// 检查 CUDA 运行时 DLL（仅当存在 ggml-cuda.dll 时才检查，表示用户有 NVIDIA GPU 环境）
		// 生活类比：如果车上装了涡轮增压器(ggml-cuda.dll)，那就必须有涡轮增压的配套管路(CUDA DLL)
		ggmlCudaPath := filepath.Join(runtimeDir, "ggml-cuda.dll")
		if _, err := os.Stat(ggmlCudaPath); err == nil {
			cudaDLLs := []string{
				"cudart64_13.dll",
				"cublas64_13.dll",
				"cublasLt64_13.dll",
			}
			for _, dll := range cudaDLLs {
				dllPath := filepath.Join(runtimeDir, dll)
				if _, err := os.Stat(dllPath); err != nil {
					result.RuntimeMissing = append(result.RuntimeMissing, fmt.Sprintf("CUDA运行时DLL: %s", dllPath))
				}
			}
		}
	}

	// ===== 2. 检查 models 目录 =====
	result.ModelsDir = filepath.Join(baseDir, "models")
	if info, err := os.Stat(result.ModelsDir); err != nil || !info.IsDir() {
		// models 目录本身不存在
		result.ModelsDirMissing = true
	} else {
		// models/ 目录存在，检查是否有至少一个 .gguf 模型文件
		// 生活类比：厨房建好了，但里面没有食材也没法做菜
		hasModel := false
		entries, err := os.ReadDir(result.ModelsDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".gguf") {
					hasModel = true
					break
				}
			}
		}
		if !hasModel {
			result.ModelsEmpty = true
		}
	}

	return result
}
