package search

import (
	"context"
	"fmt"
	"time"
)

// OllamaProvider 使用 Ollama API 的搜索 Provider。
//
// 通过嵌入 BaseHTTPSearchProvider 复用 "marshal 请求 → 鉴权 → doSearch → unmarshal" 通用流程，
// 仅定义差异部分：搜索 URL、请求体字段、响应字段映射、HTTP 超时。
type OllamaProvider struct {
	BaseHTTPSearchProvider
}

func NewOllamaProvider(apiKey string) *OllamaProvider {
	return &OllamaProvider{
		BaseHTTPSearchProvider: BaseHTTPSearchProvider{
			BaseProvider: BaseProvider{
				httpClient: newSearchHTTPClient(20 * time.Second),
			},
			apiKey: apiKey,
		},
	}
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	resp, err := p.Search(ctx, query)
	if err != nil {
		return nil, err
	}
	// 安全实践：尊重调用方传入的 MaxResults 限制
	if opts.MaxResults > 0 && len(resp.Results) > opts.MaxResults {
		resp.Results = resp.Results[:opts.MaxResults]
	}
	return resp, nil
}

func (p *OllamaProvider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("ollama api key is empty")
	}

	reqBody := map[string]string{"query": query}

	var result struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := p.doSearchJSON(ctx, "https://ollama.com/api/web_search", reqBody, &result); err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}

	searchResp := &SearchResponse{Engine: p.Name()}
	for _, r := range result.Results {
		searchResp.Results = append(searchResp.Results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
		})
	}

	return searchResp, nil
}
