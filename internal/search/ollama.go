package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type OllamaProvider struct {
	BaseProvider
	apiKey string
}

func NewOllamaProvider(apiKey string) *OllamaProvider {
	return &OllamaProvider{
		BaseProvider: BaseProvider{
			httpClient: newSearchHTTPClient(20 * time.Second),
		},
		apiKey: apiKey,
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
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("ollama marshal request: %w", err)
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + p.apiKey,
	}
	respBody, err := p.doSearch(ctx, http.MethodPost, "https://ollama.com/api/web_search", bytes.NewReader(body), headers)
	if err != nil {
		return nil, fmt.Errorf("ollama: %w", err)
	}

	var result struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("ollama unmarshal response: %w", err)
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
