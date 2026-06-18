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
	apiKey     string
	httpClient *http.Client
}

func NewOllamaProvider(apiKey string) *OllamaProvider {
	return &OllamaProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (p *OllamaProvider) Name() string {
	return "ollama"
}

func (p *OllamaProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	return p.Search(ctx, query)
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://ollama.com/api/web_search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("ollama create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama api returned status %d: %s", resp.StatusCode, string(respBody))
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
