package main

import (
	"fmt"
	"os"

	"douya/internal/search"
)

// GetSearchAPIKeys 返回搜索 API Key 的设置状态，不暴露实际密钥值
func (a *App) GetSearchAPIKeys() SearchAPIKeys {
	keys := a.loadSearchAPIKeys()
	return SearchAPIKeys{
		OllamaAPIKeySet: keys.OllamaAPIKey != "",
		TavilyAPIKeySet: keys.TavilyAPIKey != "",
	}
}

func (a *App) SetSearchAPIKeys(keys SearchAPIKeys) error {
	setFn := func(dbKey, value string) error {
		// 空值表示用户未修改，跳过更新
		if value == "" {
			return nil
		}
		return a.service.SetEncryptedSetting(dbKey, value)
	}
	if err := setFn("search_ollama_api_key", keys.OllamaAPIKey); err != nil {
		return fmt.Errorf("save ollama api key: %w", err)
	}
	if err := setFn("search_tavily_api_key", keys.TavilyAPIKey); err != nil {
		return fmt.Errorf("save tavily api key: %w", err)
	}
	return nil
}

func (a *App) loadSearchAPIKeys() SearchAPIKeys {
	keys := a.loadSearchAPIKeysFromDB()
	a.applyEnvOverrides(&keys)
	return keys
}

// loadSearchAPIKeysFromDB 仅从数据库/加密存储加载 API Key
func (a *App) loadSearchAPIKeysFromDB() SearchAPIKeys {
	keys := SearchAPIKeys{}
	getFn := func(key string) (string, error) {
		return a.service.GetEncryptedSetting(key)
	}
	if v, err := getFn("search_ollama_api_key"); err == nil {
		keys.OllamaAPIKey = v
	}
	if v, err := getFn("search_tavily_api_key"); err == nil {
		keys.TavilyAPIKey = v
	}
	return keys
}

// applyEnvOverrides 用环境变量覆盖数据库值（优先级：环境变量 > 数据库）
func (a *App) applyEnvOverrides(keys *SearchAPIKeys) {
	if apiKey := os.Getenv("OLLAMA_API_KEY"); apiKey != "" {
		keys.OllamaAPIKey = apiKey
	}
	if apiKey := os.Getenv("TAVILY_API_KEY"); apiKey != "" {
		keys.TavilyAPIKey = apiKey
	}
}

// buildSearchChain 根据当前 API Key 配置构建搜索链
// 搜索源优先级：Tavily（高质量） > Ollama
// 仅保留使用 API Key 的搜索引擎，不再使用免 Key 兜底（移除 Bing HTML 兜底）
// 若两个 API Key 均未配置，搜索链为空，调用方需自行处理无可用引擎的情况
func (a *App) buildSearchChain() *search.SearchChain {
	var searchProviders []search.CategorizedProvider
	keys := a.loadSearchAPIKeys()

	if keys.TavilyAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewTavilyProvider(keys.TavilyAPIKey), Categories: []string{"general", "code"}})
	}
	if keys.OllamaAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewOllamaProvider(keys.OllamaAPIKey), Categories: []string{"general", "code"}})
	}
	return search.NewCategorizedSearchChain(searchProviders)
}
