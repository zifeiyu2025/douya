// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type CircuitState int

const (
	CircuitClosed   CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

type ProviderWithCircuit struct {
	Provider         SearchProvider
	State            CircuitState
	Failures         int
	LastFailure      time.Time
	FailureThreshold int
	ResetTimeout     time.Duration
	Categories       []string
	mu               sync.Mutex
}

type SearchChain struct {
	providers []*ProviderWithCircuit
}

func NewSearchChain(providers ...SearchProvider) *SearchChain {
	chain := &SearchChain{}
	for _, p := range providers {
		chain.providers = append(chain.providers, &ProviderWithCircuit{
			Provider:         p,
			State:            CircuitClosed,
			FailureThreshold: 3,
			ResetTimeout:     60 * time.Second,
		})
	}
	return chain
}

func NewCategorizedSearchChain(entries []CategorizedProvider) *SearchChain {
	chain := &SearchChain{}
	for _, e := range entries {
		chain.providers = append(chain.providers, &ProviderWithCircuit{
			Provider:         e.Provider,
			State:            CircuitClosed,
			FailureThreshold: 3,
			ResetTimeout:     60 * time.Second,
			Categories:       e.Categories,
		})
	}
	return chain
}

type CategorizedProvider struct {
	Provider   SearchProvider
	Categories []string
}

func (c *SearchChain) Providers() []*ProviderWithCircuit {
	return c.providers
}

func (p *ProviderWithCircuit) canAttempt() bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch p.State {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Since(p.LastFailure) > p.ResetTimeout {
			p.State = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		return true
	default:
		return false
	}
}

func (p *ProviderWithCircuit) recordSuccess() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Failures = 0
	p.State = CircuitClosed
}

func (p *ProviderWithCircuit) recordFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.Failures++
	p.LastFailure = time.Now()
	if p.Failures >= p.FailureThreshold {
		p.State = CircuitOpen
	}
}

func (c *SearchChain) Search(ctx context.Context, query string) *SearchResponse {
	return c.SearchWithCategory(ctx, query, "general")
}

func (c *SearchChain) SearchWithCategory(ctx context.Context, query string, category string) *SearchResponse {
	return c.deepSearchInternal(ctx, query, category, false)
}

func (c *SearchChain) DeepSearch(ctx context.Context, query string, category string) *SearchResponse {
	return c.deepSearchInternal(ctx, query, category, true)
}

func (c *SearchChain) deepSearchInternal(ctx context.Context, query string, category string, deep bool) *SearchResponse {
	opts := SearchOpts{
		MaxResults:        10,
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
		if pw.canAttempt() {
			eligible = append(eligible, pw)
		}
	}

	if len(eligible) == 0 {
		return &SearchResponse{
			Engine: "none",
			Error:  fmt.Sprintf("no search providers available for query: %s", query),
		}
	}

	// deep=false：按顺序串行调用，凑够 MaxResults 条或所有 provider 都调完
	// deep=true：所有 provider 并发调用后合并去重
	if !deep {
		return c.serialSearchWithDedup(ctx, query, eligible, opts)
	}
	return c.deepParallelSearchWithDedup(ctx, query, eligible, opts)
}

// serialSearchWithDedup 串行调用多个 provider，结果按 URL 去重，直到达到 MaxResults 或遍历完所有 provider
func (c *SearchChain) serialSearchWithDedup(ctx context.Context, query string, eligible []*ProviderWithCircuit, opts SearchOpts) *SearchResponse {
	urlMap := make(map[string]*SearchResult)
	var engines []string

	for _, pw := range eligible {
		select {
		case <-ctx.Done():
			return &SearchResponse{
				Engine: "cancelled",
				Error:  fmt.Sprintf("search cancelled: %v", ctx.Err()),
			}
		default:
		}

		resp, err := pw.Provider.SearchWithOpts(ctx, query, opts)
		if err != nil {
			pw.recordFailure()
			continue
		}
		pw.recordSuccess()
		if resp == nil || len(resp.Results) == 0 {
			continue
		}
		engines = append(engines, pw.Provider.Name())

		for i := range resp.Results {
			r := &resp.Results[i]
			normalizedURL := normalizeURL(r.URL)
			if normalizedURL == "" {
				continue
			}
			if existing, ok := urlMap[normalizedURL]; ok {
				existing.Sources++
				if r.Score > existing.Score {
					existing.Score = r.Score
				}
				if r.Snippet != "" && len(r.Snippet) > len(existing.Snippet) {
					existing.Snippet = r.Snippet
				}
			} else {
				urlMap[normalizedURL] = &SearchResult{
					Title:   r.Title,
					URL:     normalizedURL,
					Snippet: r.Snippet,
					Score:   r.Score,
					Sources: 1,
				}
			}
		}

		// 已凑够 MaxResults，提前返回
		if len(urlMap) >= opts.MaxResults {
			break
		}
	}

	if len(urlMap) == 0 {
		return &SearchResponse{
			Engine: "none",
			Error:  fmt.Sprintf("no search results found for query: %s", query),
		}
	}

	allResults := rankAndDedupeResults(urlMap, opts.MaxResults)

	engineStr := "chain"
	if len(engines) > 0 {
		engineStr = "chain:" + strings.Join(engines, ",")
	}

	return &SearchResponse{
		Results: allResults,
		Engine:  engineStr,
	}
}

// deepParallelSearchWithDedup 并发调用多个 provider，按 URL 去重排序后截断到 MaxResults
func (c *SearchChain) deepParallelSearchWithDedup(ctx context.Context, query string, eligible []*ProviderWithCircuit, opts SearchOpts) *SearchResponse {
	type providerResult struct {
		resp     *SearchResponse
		err      error
		provider string
	}

	results := make(chan providerResult, len(eligible))
	var wg sync.WaitGroup
	maxConcurrent := 3
	semaphore := make(chan struct{}, maxConcurrent)

	for _, pw := range eligible {
		wg.Add(1)
		go func(pw *ProviderWithCircuit) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			resp, err := pw.Provider.SearchWithOpts(ctx, query, opts)
			if err != nil {
				pw.recordFailure()
			} else {
				pw.recordSuccess()
			}
			results <- providerResult{resp: resp, err: err, provider: pw.Provider.Name()}
		}(pw)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	urlMap := make(map[string]*SearchResult)
	var engines []string

	for pr := range results {
		select {
		case <-ctx.Done():
			return &SearchResponse{
				Engine: "deep",
				Error:  fmt.Sprintf("search cancelled: %v", ctx.Err()),
			}
		default:
		}

		if pr.err != nil || pr.resp == nil {
			continue
		}
		engines = append(engines, pr.provider)
		for i := range pr.resp.Results {
			r := &pr.resp.Results[i]
			normalizedURL := normalizeURL(r.URL)
			if normalizedURL == "" {
				continue
			}
			if existing, ok := urlMap[normalizedURL]; ok {
				existing.Sources++
				if r.Score > existing.Score {
					existing.Score = r.Score
				}
				if r.Snippet != "" && len(r.Snippet) > len(existing.Snippet) {
					existing.Snippet = r.Snippet
				}
			} else {
				urlMap[normalizedURL] = &SearchResult{
					Title:   r.Title,
					URL:     normalizedURL,
					Snippet: r.Snippet,
					Score:   r.Score,
					Sources: 1,
				}
			}
		}
	}

	if len(urlMap) == 0 {
		return &SearchResponse{
			Engine: "deep",
			Error:  fmt.Sprintf("no search results found for query: %s", query),
		}
	}

	allResults := rankAndDedupeResults(urlMap, opts.MaxResults)

	engineStr := "deep:" + strings.Join(engines, ",")

	return &SearchResponse{
		Results: allResults,
		Engine:  engineStr,
	}
}

// rankAndDedupeResults 对去重后的结果按分数排序并截断到 maxResults
func rankAndDedupeResults(urlMap map[string]*SearchResult, maxResults int) []SearchResult {
	allResults := make([]SearchResult, 0, len(urlMap))
	for _, r := range urlMap {
		r.Score = r.Score * (1.0 + float64(r.Sources-1)*0.2)
		allResults = append(allResults, *r)
	}

	sort.Slice(allResults, func(i, j int) bool {
		return allResults[i].Score > allResults[j].Score
	})

	if len(allResults) > maxResults {
		allResults = allResults[:maxResults]
	}
	return allResults
}

func normalizeURL(u string) string {
	if u == "" {
		return ""
	}
	parsed, err := url.Parse(u)
	if err != nil {
		return u
	}
	// Normalize scheme to https
	if parsed.Scheme == "http" || parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	// Remove www prefix
	parsed.Host = strings.TrimPrefix(parsed.Host, "www.")
	// Remove fragment
	parsed.Fragment = ""
	// Remove trailing slash from path
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	// Keep query params? No, remove them for dedup (search results may have tracking params)
	parsed.RawQuery = ""
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String()
}
