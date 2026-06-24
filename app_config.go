package main

import (
	"fmt"
	"os"
	"path/filepath"

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

func (a *App) validatePaths() []string {
	var missing []string
	baseDir := appDir()

	cfg := a.getConfig()
	serverPath := resolvePath(cfg.LlamaServerPath)
	if _, err := os.Stat(serverPath); err != nil {
		missing = append(missing, fmt.Sprintf("引擎程序: %s", serverPath))
	}

	modelsDir := filepath.Join(baseDir, "models")
	if info, err := os.Stat(modelsDir); err != nil || !info.IsDir() {
		missing = append(missing, fmt.Sprintf("模型目录: %s", modelsDir))
	}

	runtimeDir := filepath.Join(baseDir, "runtime")
	if info, err := os.Stat(runtimeDir); err != nil || !info.IsDir() {
		missing = append(missing, fmt.Sprintf("运行时目录: %s", runtimeDir))
	}

	return missing
}
