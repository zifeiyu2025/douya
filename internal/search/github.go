package search

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

type GitHubProvider struct {
	apiKey     string
	httpClient *http.Client
}

func NewGitHubProvider(apiKey string) *GitHubProvider {
	return &GitHubProvider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (p *GitHubProvider) Name() string {
	return "github"
}

func (p *GitHubProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	return p.Search(ctx, query)
}

func (p *GitHubProvider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	var searchResp *SearchResponse

	repoResp, err := p.searchRepositories(ctx, query)
	if err == nil && len(repoResp.Results) > 0 {
		searchResp = repoResp
	}

	codeResp, err := p.searchCode(ctx, query)
	if err == nil && len(codeResp.Results) > 0 {
		if searchResp == nil {
			searchResp = codeResp
		} else {
			searchResp.Results = append(searchResp.Results, codeResp.Results...)
			searchResp.Engine = "github"
		}
	}

	if searchResp == nil {
		return &SearchResponse{Engine: p.Name()}, nil
	}

	return searchResp, nil
}

func (p *GitHubProvider) searchRepositories(ctx context.Context, query string) (*SearchResponse, error) {
	u := fmt.Sprintf("https://api.github.com/search/repositories?q=%s&sort=stars&per_page=5", url.QueryEscape(query))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("github create request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/vnd.github.v3+json")
	httpReq.Header.Set("User-Agent", "DouYa/1.0")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "token "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("github request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("github read response: %w", err)
	}

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("github api rate limit exceeded")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github api returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Items []struct {
			FullName    string `json:"full_name"`
			HTMLURL     string `json:"html_url"`
			Description string `json:"description"`
			Stars       int    `json:"stargazers_count"`
		} `json:"items"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("github unmarshal response: %w", err)
	}

	searchResp := &SearchResponse{Engine: p.Name()}
	for _, item := range result.Items {
		snippet := item.Description
		if snippet == "" {
			snippet = fmt.Sprintf("⭐ %d stars", item.Stars)
		} else {
			snippet = fmt.Sprintf("%s (⭐ %d)", snippet, item.Stars)
		}
		searchResp.Results = append(searchResp.Results, SearchResult{
			Title:   item.FullName,
			URL:     item.HTMLURL,
			Snippet: snippet,
			Score:   float64(item.Stars)/1000.0 + 0.5,
		})
	}

	return searchResp, nil
}

func (p *GitHubProvider) searchCode(ctx context.Context, query string) (*SearchResponse, error) {
	u := fmt.Sprintf("https://api.github.com/search/code?q=%s&per_page=3", url.QueryEscape(query))

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("github code search create request: %w", err)
	}
	httpReq.Header.Set("Accept", "application/vnd.github.v3+json")
	httpReq.Header.Set("User-Agent", "DouYa/1.0")
	if p.apiKey != "" {
		httpReq.Header.Set("Authorization", "token "+p.apiKey)
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("github code search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("github code search read response: %w", err)
	}

	if resp.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("github api rate limit exceeded")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("github code search returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Items []struct {
			Name       string `json:"name"`
			HTMLURL    string `json:"html_url"`
			Repository struct {
				FullName string `json:"full_name"`
			} `json:"repository"`
		} `json:"items"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("github code search unmarshal response: %w", err)
	}

	searchResp := &SearchResponse{Engine: p.Name()}
	for _, item := range result.Items {
		searchResp.Results = append(searchResp.Results, SearchResult{
			Title:   item.Repository.FullName + "/" + item.Name,
			URL:     item.HTMLURL,
			Snippet: fmt.Sprintf("Code file: %s in %s", item.Name, item.Repository.FullName),
			Score:   0.3,
		})
	}

	return searchResp, nil
}
