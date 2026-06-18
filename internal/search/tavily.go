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
	apiKey     string
	httpClient *http.Client
}

func NewTavilyProvider(apiKey string) *TavilyProvider {
	return &TavilyProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 30 * time.Second},
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

	reqBody := map[string]interface{}{
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.tavily.com/search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("tavily create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tavily request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("tavily read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("tavily api returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Answer string `json:"answer"`
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
