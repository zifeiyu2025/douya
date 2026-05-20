// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import "context"

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
