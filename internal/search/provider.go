// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
)

type SearchResponse struct {
	Results []SearchResult
	Error   string
	Engine  string
}

type SearchResult struct {
	Title       string
	URL         string
	Snippet     string
	RawContent  string
	Score       float64
}

type SearchOpts struct {
	MaxResults        int
	IncludeAnswer     bool
	IncludeRawContent bool
}

type Provider interface {
	Name() string
	Search(ctx context.Context, query string) (*SearchResponse, error)
	SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error)
}

type CategorizedProvider struct {
	Provider Provider
	Categories []string
}

type SearchChain struct {
	providers []CategorizedProvider
}

func NewCategorizedSearchChain(providers []CategorizedProvider) *SearchChain {
	return &SearchChain{providers: providers}
}

func (c *SearchChain) Search(ctx context.Context, query string) *SearchResponse {
	return c.SearchWithCategory(ctx, query, "general", 5)
}

func (c *SearchChain) SearchWithCategory(ctx context.Context, query string, category string, maxResults int) *SearchResponse {
	opts := SearchOpts{
		MaxResults:        maxResults,
		IncludeAnswer:     true,
		IncludeRawContent: false,
	}

	// 收集该 category 下所有可用的 provider
	var eligible []*ProviderWithCircuit
	for _, pw := range c.providers {
		if len(pw.Categories) > 0 {
			matched := false
			for _, cat := range pw.Categories {
				if cat == category {
					matched = true
					break
				}
			}
			if !matched {
				continue
			}
		}
		eligible = append(eligible, &ProviderWithCircuit{Provider: pw.Provider})
	}

	// 串行逐步降级搜索
	for _, pw := range eligible {
		resp, err := pw.Provider.SearchWithOpts(ctx, query, opts)
		if err != nil {
			continue
		}
		if resp != nil && len(resp.Results) > 0 {
			return resp
		}
	}

	return nil
}

type ProviderWithCircuit struct {
	Provider Provider
}

var (
	reHTMLTag      = regexp.MustCompile(`<[^>]+>`)
	reNumEntity    = regexp.MustCompile(`&#(\d+);`)
	reHexEntity    = regexp.MustCompile(`&#x([0-9a-fA-F]+);`)
	reMultiSpace   = regexp.MustCompile(`\s+`)
	reLink         = regexp.MustCompile(`<a[^>]+href="(https?://[^"]+)"[^>]*>`)
	reH3           = regexp.MustCompile(`<h[1-6][^>]*>(.*?)</h[1-6]>`)
)

func readBody(r io.Reader) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, 10*1024*1024))
}

func stripHTML(s string) string {
	s = reHTMLTag.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "&ensp;", " ")
	s = strings.ReplaceAll(s, "&emsp;", "  ")
	s = strings.ReplaceAll(s, "&thinsp;", " ")
	s = strings.ReplaceAll(s, "&middot;", "·")
	s = strings.ReplaceAll(s, "&mdash;", "—")
	s = strings.ReplaceAll(s, "&ndash;", "–")
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&lt;", "<")
	s = strings.ReplaceAll(s, "&gt;", ">")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#39;", "'")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	s = reNumEntity.ReplaceAllStringFunc(s, func(match string) string {
		sub := reNumEntity.FindStringSubmatch(match)
		if len(sub) >= 2 {
			var code int
			fmt.Sscanf(sub[1], "%d", &code)
			if code > 0 && code < 0x10FFFF {
				return string(rune(code))
			}
		}
		return match
	})
	s = reHexEntity.ReplaceAllStringFunc(s, func(match string) string {
		sub := reHexEntity.FindStringSubmatch(match)
		if len(sub) >= 2 {
			var code int
			fmt.Sscanf(sub[1], "%x", &code)
			if code > 0 && code < 0x10FFFF {
				return string(rune(code))
			}
		}
		return match
	})
	s = reMultiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// parseGenericSearchResults 从 HTML 中提取通用搜索结果
func parseGenericSearchResults(html string) []SearchResult {
	// 找到所有外部链接
	linkMatches := reLink.FindAllStringSubmatch(html, -1)
	
	var results []SearchResult
	
	for _, match := range linkMatches {
		if len(match) < 2 {
			continue
		}
		link := match[1]
		
		// 为每个链接找最近的 h3 标题
		title := ""
		h3Matches := reH3.FindAllStringSubmatch(html, -1)
		for _, h3Match := range h3Matches {
			if len(h3Match) < 2 {
				continue
			}
			h3Title := stripHTML(h3Match[1])
			if strings.Contains(link, h3Title) || strings.Contains(h3Title, link) {
				title = h3Title
				break
			}
		}
		
		// 如果没有找到标题，使用 URL 的最后一部分作为标题
		if title == "" {
			u, err := url.Parse(link)
			if err == nil {
				title = u.Path
				if title == "" {
					title = u.Host
				}
			}
		}
		
		results = append(results, SearchResult{
			Title:  title,
			URL:    link,
			Snippet: "搜索结果内容",
			Score:  0.5,
		})
	}
	
	return results
}