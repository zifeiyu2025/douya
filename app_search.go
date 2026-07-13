package main

import (
	"fmt"
	"os"

	"douya/internal/search"
	"douya/internal/store"
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
		if a.encKey != nil {
			return store.SetEncryptedSetting(a.db, dbKey, value, a.encKey)
		}
		return store.SetSetting(a.db, dbKey, value)
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
		if a.encKey != nil {
			return store.GetEncryptedSetting(a.db, key, a.encKey)
		}
		return store.GetSetting(a.db, key)
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
// 搜索源优先级：Tavily（高质量） > Ollama > Bing（免 Key 兜底）
// Bing 作为兜底搜索引擎，即使未配置任何 API Key 也能提供基础搜索能力
func (a *App) buildSearchChain() *search.SearchChain {
	var searchProviders []search.CategorizedProvider
	keys := a.loadSearchAPIKeys()

	if keys.TavilyAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewTavilyProvider(keys.TavilyAPIKey), Categories: []string{"general", "code"}})
	}
	if keys.OllamaAPIKey != "" {
		searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewOllamaProvider(keys.OllamaAPIKey), Categories: []string{"general", "code"}})
	}
	// Bing 作为免 API Key 兜底搜索引擎，始终加入链尾
	// 覆盖 general 和 code 两个分类，确保任何查询都有搜索引擎可用
	searchProviders = append(searchProviders, search.CategorizedProvider{Provider: search.NewBingProvider(), Categories: []string{"general", "code"}})
	return search.NewCategorizedSearchChain(searchProviders)
}
