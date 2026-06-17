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

type So360Provider struct {
	httpClient *http.Client
}

func NewSo360Provider() *So360Provider {
	return &So360Provider{
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

func (p *So360Provider) Name() string {
	return "so360"
}

func (p *So360Provider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	return p.Search(ctx, query)
}

var (
	re360Result = regexp.MustCompile(`<li[^>]*class="res-list"[^>]*>`)
	re360Title  = regexp.MustCompile(`<h3[^>]*class="res-title"[^>]*>\s*<a[^>]*>(.*?)</a>`)
	re360Href   = regexp.MustCompile(`<a[^>]+href="(https?://[^"]+)"[^>]*>`)
	re360Snip   = regexp.MustCompile(`<p[^>]*class="res-desc"[^>]*>(.*?)</p>`)
)

func (p *So360Provider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	// 中文查询无需额外处理，360 中文搜索质量好
	u := fmt.Sprintf("https://www.so.com/s?q=%s", url.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("360 create request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("360 request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("360 read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("360 returned status %d", resp.StatusCode)
	}

	html := string(body)
	searchResp := &SearchResponse{Engine: p.Name()}

	// 方法1：尝试匹配 res-list 结构
	resultMatches := re360Result.FindAllStringSubmatchIndex(html, -1)
	if len(resultMatches) > 0 {
		for i, loc := range resultMatches {
			if i >= 10 {
				break
			}
			chunkEnd := len(html)
			if i+1 < len(resultMatches) {
				chunkEnd = resultMatches[i+1][0]
			}
			chunk := html[loc[0]:chunkEnd]

			titleMatch := re360Title.FindStringSubmatch(chunk)
			if len(titleMatch) < 2 {
				continue
			}
			title := stripHTML(titleMatch[1])
			if title == "" {
				continue
			}

			hrefMatch := re360Href.FindStringSubmatch(chunk)
			if len(hrefMatch) < 2 {
				continue
			}
			href := stripHTML(hrefMatch[1])
			if href == "" || strings.HasPrefix(href, "https://www.so.com") {
				continue
			}

			snippet := ""
			snipMatch := re360Snip.FindStringSubmatch(chunk)
			if len(snipMatch) >= 2 {
				snippet = stripHTML(snipMatch[1])
			}

			searchResp.Results = append(searchResp.Results, SearchResult{
				Title:   title,
				URL:     href,
				Snippet: snippet,
				Score:   0.7,
			})
		}
	}

	// 方法2：如果方法1没有结果，使用通用链接+标题匹配
	if len(searchResp.Results) == 0 {
		searchResp.Results = parseGenericSearchResults(html)
	}

	if len(searchResp.Results) == 0 {
		return nil, fmt.Errorf("360: no results parsed for query: %s", query)
	}

	return searchResp, nil
}
