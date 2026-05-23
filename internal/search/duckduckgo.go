package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"
)

type DuckDuckGoProvider struct {
	httpClient *http.Client
}

func NewDuckDuckGoProvider() *DuckDuckGoProvider {
	return &DuckDuckGoProvider{
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

func (p *DuckDuckGoProvider) Name() string {
	return "duckduckgo"
}

func (p *DuckDuckGoProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	return p.Search(ctx, query)
}

var (
	reDDGResult  = regexp.MustCompile(`<div class="result results_links results_links_deep web-result"`)
	reDDGTitle   = regexp.MustCompile(`<a class="result__a" href="(https?://[^"]+)"[^>]*>(.*?)</a>`)
	reDDGSnippet = regexp.MustCompile(`<a class="result__snippet"[^>]*>(.*?)</a>`)
)

func (p *DuckDuckGoProvider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	u := fmt.Sprintf("https://html.duckduckgo.com/html/?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("duckduckgo read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("duckduckgo returned status %d", resp.StatusCode)
	}

	html := string(body)

	searchResp := &SearchResponse{Engine: p.Name()}

	resultIndices := reDDGResult.FindAllStringIndex(html, -1)

	for i, loc := range resultIndices {
		if i >= 10 {
			break
		}

		chunkEnd := len(html)
		if i+1 < len(resultIndices) {
			chunkEnd = resultIndices[i+1][0]
		}
		chunk := html[loc[0]:chunkEnd]

		titleMatch := reDDGTitle.FindStringSubmatch(chunk)
		if len(titleMatch) < 3 {
			continue
		}

		href := stripHTML(titleMatch[1])
		title := stripHTML(titleMatch[2])

		if href == "" || title == "" {
			continue
		}

		snippet := ""
		snippetMatch := reDDGSnippet.FindStringSubmatch(chunk)
		if len(snippetMatch) >= 2 {
			snippet = stripHTML(snippetMatch[1])
		}

		searchResp.Results = append(searchResp.Results, SearchResult{
			Title:   title,
			URL:     href,
			Snippet: snippet,
			Score:   0.5,
		})
	}

	return searchResp, nil
}
