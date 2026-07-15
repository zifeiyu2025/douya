// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type TavilyProvider struct {
	BaseProvider
	apiKey string
}

func NewTavilyProvider(apiKey string) *TavilyProvider {
	return &TavilyProvider{
		BaseProvider: BaseProvider{
			// 超时 30s：符合项目记忆规则，支持 advanced search 深度检索。
			// Tavily 异常时由全局 DefaultSearchTimeout（50s）和熔断器统一降级，
			// 不在单端过早放弃，避免丢失可用结果。
			httpClient: newSearchHTTPClient(30 * time.Second),
		},
		apiKey: apiKey,
	}
}

func (p *TavilyProvider) Name() string {
	return "tavily"
}

func (p *TavilyProvider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	return p.SearchWithOpts(ctx, query, SearchOpts{MaxResults: 5})
}

func (p *TavilyProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	if p.apiKey == "" {
		return nil, fmt.Errorf("tavily api key is empty")
	}

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 10
	}

	reqBody := map[string]any{
		"query":               query,
		"max_results":         maxResults,
		"include_answer":      opts.IncludeAnswer,
		"include_raw_content": opts.IncludeRawContent,
		"search_depth":        "basic",
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("tavily marshal request: %w", err)
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + p.apiKey,
	}
	respBody, err := p.doSearch(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body), headers)
	if err != nil {
		return nil, fmt.Errorf("tavily: %w", err)
	}

	var result struct {
		Answer  string `json:"answer"`
		Results []struct {
			Title   string  `json:"title"`
			URL     string  `json:"url"`
			Content string  `json:"content"`
			Score   float64 `json:"score"`
		} `json:"results"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("tavily unmarshal response: %w", err)
	}

	searchResp := &SearchResponse{Engine: p.Name()}
	for _, r := range result.Results {
		searchResp.Results = append(searchResp.Results, SearchResult{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: r.Content,
			Score:   r.Score,
		})
	}
	searchResp.Answer = result.Answer

	return searchResp, nil
}
