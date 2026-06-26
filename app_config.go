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

// validatePaths 检查启动所需的关键文件是否缺失
// 返回缺失项列表，每项格式为 "类型: 路径"
// 检查范围：引擎 exe、核心 DLL、CUDA 运行时 DLL、models 目录、模型文件
func (a *App) validatePaths() []string {
	var missing []string
	baseDir := appDir()

	cfg := a.getConfig()
	serverPath := resolvePath(cfg.LlamaServerPath)
	if _, err := os.Stat(serverPath); err != nil {
		missing = append(missing, fmt.Sprintf("引擎程序: %s", serverPath))
	}

	runtimeDir := filepath.Join(baseDir, "runtime")
	if info, err := os.Stat(runtimeDir); err != nil || !info.IsDir() {
		missing = append(missing, fmt.Sprintf("运行时目录: %s", runtimeDir))
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
				missing = append(missing, fmt.Sprintf("核心DLL: %s", dllPath))
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
					missing = append(missing, fmt.Sprintf("CUDA运行时DLL: %s", dllPath))
				}
			}
		}
	}

	modelsDir := filepath.Join(baseDir, "models")
	if info, err := os.Stat(modelsDir); err != nil || !info.IsDir() {
		missing = append(missing, fmt.Sprintf("模型目录: %s", modelsDir))
	} else {
		// models/ 目录存在，检查是否有至少一个 .gguf 模型文件
		// 生活类比：厨房建好了，但里面没有食材也没法做菜
		hasModel := false
		entries, err := os.ReadDir(modelsDir)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".gguf") {
					hasModel = true
					break
				}
			}
		}
		if !hasModel {
			missing = append(missing, fmt.Sprintf("模型文件: %s 目录下未找到任何 .gguf 文件", modelsDir))
		}
	}

	return missing
}
