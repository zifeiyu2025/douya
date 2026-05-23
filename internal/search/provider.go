// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

type SearchResult struct {
	Title   string  `json:"title"`
	URL     string  `json:"url"`
	Snippet string  `json:"snippet"`
	Score   float64 `json:"score,omitempty"`
	Sources int     `json:"sources,omitempty"`
}

type SearchResponse struct {
	Results []SearchResult `json:"results"`
	Engine  string         `json:"engine"`
	Error   string         `json:"error,omitempty"`
}

type SearchProvider interface {
	Name() string
	Search(ctx context.Context, query string) (*SearchResponse, error)
	SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error)
}

type SearchOpts struct {
	MaxResults       int
	IncludeAnswer    bool
	IncludeRawContent bool
}

var (
	reHTMLTag      = regexp.MustCompile(`<[^>]+>`)
	reNumEntity    = regexp.MustCompile(`&#(\d+);`)
	reHexEntity    = regexp.MustCompile(`&#x([0-9a-fA-F]+);`)
	reMultiSpace   = regexp.MustCompile(`\s+`)
)

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
