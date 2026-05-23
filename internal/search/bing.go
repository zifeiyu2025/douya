package search

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type BingProvider struct {
	httpClient *http.Client
}

func NewBingProvider() *BingProvider {
	return &BingProvider{
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

func (p *BingProvider) Name() string {
	return "bing"
}

func (p *BingProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	return p.Search(ctx, query)
}

var (
	reBingAlgo    = regexp.MustCompile(`<li class="b_algo"[\s>]`)
	reBingH2Link  = regexp.MustCompile(`<h2[^>]*>\s*<a[^>]*href="(https?://[^"]+)"[^>]*>(.*?)</a>`)
	reBingCaption = regexp.MustCompile(`<div class="b_caption">\s*<p[^>]*>(.*?)</p>`)
)

func (p *BingProvider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	u := fmt.Sprintf("https://www.bing.com/search?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("bing create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bing request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("bing read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bing returned status %d", resp.StatusCode)
	}

	html := string(body)

	searchResp := &SearchResponse{Engine: p.Name()}

	algoIndices := reBingAlgo.FindAllStringIndex(html, -1)

	for i, loc := range algoIndices {
		if i >= 10 {
			break
		}

		chunkEnd := len(html)
		if i+1 < len(algoIndices) {
			chunkEnd = algoIndices[i+1][0]
		}
		chunk := html[loc[0]:chunkEnd]

		linkMatch := reBingH2Link.FindStringSubmatch(chunk)
		if len(linkMatch) < 3 {
			continue
		}

		href := stripHTML(linkMatch[1])
		title := stripHTML(linkMatch[2])

		if href == "" || title == "" {
			continue
		}

		if strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "//") {
			href = "https://www.bing.com" + href
		}

		snippet := ""
		captionMatch := reBingCaption.FindStringSubmatch(chunk)
		if len(captionMatch) >= 2 {
			snippet = stripHTML(captionMatch[1])
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
