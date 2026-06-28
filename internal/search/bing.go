package search

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type BingProvider struct {
	BaseProvider
}

func NewBingProvider() *BingProvider {
	return &BingProvider{
		BaseProvider: BaseProvider{
			httpClient: &http.Client{Timeout: 8 * time.Second},
		},
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
	// 中文查询添加区域和语言参数，提升搜索结果质量
	if containsCJK(query) {
		u += "&cc=cn&setlang=zh-Hans"
	}

	headers := map[string]string{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
	}
	body, err := p.doSearch(ctx, http.MethodGet, u, nil, headers)
	if err != nil {
		return nil, fmt.Errorf("bing: %w", err)
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

	// 去重和过滤无效结果
	searchResp.Results = dedupAndFilterResults(searchResp.Results)

	return searchResp, nil
}
