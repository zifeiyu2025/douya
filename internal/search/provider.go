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
	"sync"
	"time"

	"douya/internal/httputil"

	"github.com/rs/zerolog/log"
)

// ---------------------------------------------------------------------------
// 搜索结果与选项
// ---------------------------------------------------------------------------

type SearchResponse struct {
	Results []SearchResult
	Answer  string // API 类引擎返回的直接回答（如 Tavily）
	Error   string
	Engine  string
}

type SearchResult struct {
	Title      string  `json:"title"`
	URL        string  `json:"url"`
	Snippet    string  `json:"snippet"`
	RawContent string  `json:"raw_content,omitempty"`
	Score      float64 `json:"score,omitempty"`
}

type SearchOpts struct {
	MaxResults        int
	IncludeAnswer     bool
	IncludeRawContent bool
}

// ---------------------------------------------------------------------------
// Provider 接口与分类包装
// ---------------------------------------------------------------------------

type Provider interface {
	Name() string
	Search(ctx context.Context, query string) (*SearchResponse, error)
	SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error)
}

// SearchProvider 是 Provider 的类型别名，保持向后兼容。
type SearchProvider = Provider

type CategorizedProvider struct {
	Provider   Provider
	Categories []string
}

// ---------------------------------------------------------------------------
// 熔断器（Circuit Breaker）
// ---------------------------------------------------------------------------

type CircuitState int

const (
	CircuitClosed   CircuitState = iota // 正常
	CircuitOpen                         // 熔断中，跳过该 provider
	CircuitHalfOpen                     // 试探性放行一次请求
)

func (s CircuitState) String() string {
	switch s {
	case CircuitClosed:
		return "closed"
	case CircuitOpen:
		return "open"
	case CircuitHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

const (
	DefaultFailureThreshold = 3
	DefaultResetTimeout     = 30 * time.Second
	DefaultMaxRetries       = 0 // 额外重试次数（0 表示不重试，可外部调整）
	DefaultSearchTimeout    = 35 * time.Second
)

// ProviderWithCircuit 为 Provider 附加熔断与重试能力。
// 导出的配置字段（FailureThreshold / ResetTimeout / MaxRetries）可在外部调整。
type ProviderWithCircuit struct {
	Provider Provider

	// 配置
	FailureThreshold int
	ResetTimeout     time.Duration
	MaxRetries       int

	// 运行时状态（通过 record* 方法更新，测试中可直接读取）
	mu            sync.Mutex
	State         CircuitState
	Failures      int
	LastFailureAt time.Time

	// 分类信息（由 SearchChain 构造函数注入）
	categories []string
}

func newProviderWithCircuit(p Provider) *ProviderWithCircuit {
	return &ProviderWithCircuit{
		Provider:         p,
		FailureThreshold: DefaultFailureThreshold,
		ResetTimeout:     DefaultResetTimeout,
		MaxRetries:       DefaultMaxRetries,
		State:            CircuitClosed,
	}
}

// IsOpen 判断熔断器是否处于打开状态（应跳过该 provider）。
// 若已超过 ResetTimeout，自动转为 HalfOpen 允许一次试探。
func (pw *ProviderWithCircuit) IsOpen() bool {
	pw.mu.Lock()
	defer pw.mu.Unlock()

	if pw.State == CircuitOpen {
		if !pw.LastFailureAt.IsZero() && time.Since(pw.LastFailureAt) > pw.ResetTimeout {
			pw.State = CircuitHalfOpen
			return false
		}
		return true
	}
	return false
}

func (pw *ProviderWithCircuit) recordSuccess() {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	pw.State = CircuitClosed
	pw.Failures = 0
}

func (pw *ProviderWithCircuit) recordFailure() {
	pw.mu.Lock()
	defer pw.mu.Unlock()
	pw.Failures++
	pw.LastFailureAt = time.Now()
	if pw.Failures >= pw.FailureThreshold {
		pw.State = CircuitOpen
	}
}

// ---------------------------------------------------------------------------
// SearchChain —— 多引擎顺序降级调度
// ---------------------------------------------------------------------------

type SearchChain struct {
	providers []*ProviderWithCircuit
}

// NewCategorizedSearchChain 从带分类信息的 provider 列表构建搜索链。
func NewCategorizedSearchChain(providers []CategorizedProvider) *SearchChain {
	wrapped := make([]*ProviderWithCircuit, len(providers))
	for i, cp := range providers {
		pw := newProviderWithCircuit(cp.Provider)
		pw.categories = cp.Categories
		wrapped[i] = pw
	}
	return &SearchChain{providers: wrapped}
}

// NewSearchChain 是便捷构造函数，将所有 provider 归入通用分类（无类别过滤）。
func NewSearchChain(providers ...Provider) *SearchChain {
	wrapped := make([]*ProviderWithCircuit, len(providers))
	for i, p := range providers {
		wrapped[i] = newProviderWithCircuit(p)
	}
	return &SearchChain{providers: wrapped}
}

// Providers 返回链中所有 provider 的引用，便于外部调整熔断参数。
func (c *SearchChain) Providers() []*ProviderWithCircuit {
	return c.providers
}

// Search 以默认参数执行搜索（category=general, maxResults=5）。
func (c *SearchChain) Search(ctx context.Context, query string) *SearchResponse {
	return c.SearchWithCategory(ctx, query, "general", 5)
}

// SearchWithCategory 在指定分类下按优先级顺序调度符合条件的 provider，
// 每个 provider 内部支持指数退避重试，同时具备熔断降级能力。
// 当某个 provider 成功返回结果时立即返回，失败时自动降级到下一个 provider。
func (c *SearchChain) SearchWithCategory(ctx context.Context, query string, category string, maxResults ...int) *SearchResponse {
	startTime := time.Now()

	mr := 5
	if len(maxResults) > 0 {
		mr = maxResults[0]
	}
	opts := SearchOpts{
		MaxResults:        mr,
		IncludeAnswer:     true,
		IncludeRawContent: false,
	}

	// 1. 筛选符合 category 的 provider
	var eligible []*ProviderWithCircuit
	for _, pw := range c.providers {
		if !matchCategory(pw, category) {
			continue
		}
		eligible = append(eligible, pw)
	}

	if len(eligible) == 0 {
		log.Warn().
			Str("category", category).
			Str("query", query).
			Msg("[search] no eligible providers for category")
		return &SearchResponse{Engine: "none", Error: "no providers available"}
	}

	// 2. 设置全局搜索超时
	searchCtx, cancel := context.WithTimeout(ctx, DefaultSearchTimeout)
	defer cancel()

	// 3. 顺序降级 —— 按优先级依次尝试各 provider
	var errMsgs []string
	for _, pw := range eligible {
		if pw.IsOpen() {
			log.Debug().
				Str("engine", pw.Provider.Name()).
				Str("state", pw.State.String()).
				Msg("[search] circuit breaker open, skipping provider")
			errMsgs = append(errMsgs, fmt.Sprintf("%s: circuit open", pw.Provider.Name()))
			continue
		}

		resp, err := c.callWithRetry(searchCtx, pw, query, opts)
		if err != nil {
			pw.recordFailure()
			log.Warn().
				Str("engine", pw.Provider.Name()).
				Err(err).
				Int("failures", pw.Failures).
				Str("state", pw.State.String()).
				Msg("[search] provider failed")
			errMsgs = append(errMsgs, fmt.Sprintf("%s: %v", pw.Provider.Name(), err))
			continue
		}

		if resp != nil && len(resp.Results) > 0 {
			pw.recordSuccess()
			elapsed := time.Since(startTime)
			log.Info().
				Str("engine", pw.Provider.Name()).
				Int("results", len(resp.Results)).
				Dur("elapsed", elapsed).
				Msg("[search] provider succeeded")
			return resp
		}

		// 成功但无结果，视为软失败
		pw.recordFailure()
		errMsgs = append(errMsgs, fmt.Sprintf("%s: empty results", pw.Provider.Name()))
	}

	// 4. 全部失败 —— 优雅降级
	elapsed := time.Since(startTime)
	log.Error().
		Str("query", query).
		Str("category", category).
		Dur("elapsed", elapsed).
		Int("tried", len(eligible)).
		Msg("[search] all providers failed")

	return &SearchResponse{
		Engine: "none",
		Error:  fmt.Sprintf("all search providers failed: %s", strings.Join(errMsgs, "; ")),
	}
}

// callWithRetry 对单个 provider 执行带指数退避的重试调用。
func (c *SearchChain) callWithRetry(ctx context.Context, pw *ProviderWithCircuit, query string, opts SearchOpts) (*SearchResponse, error) {
	maxAttempts := pw.MaxRetries + 1
	if maxAttempts < 1 {
		maxAttempts = 1
	}

	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<(attempt-1)) * 200 * time.Millisecond
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			log.Debug().
				Str("engine", pw.Provider.Name()).
				Int("attempt", attempt+1).
				Dur("backoff", backoff).
				Msg("[search] retrying provider")
		}

		resp, err := pw.Provider.SearchWithOpts(ctx, query, opts)
		if err == nil {
			return resp, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

// matchCategory 检查 provider 是否属于指定分类。
// 未设置 Categories 的 provider 匹配所有分类。
func matchCategory(pw *ProviderWithCircuit, category string) bool {
	if len(pw.categories) == 0 {
		return true
	}
	for _, cat := range pw.categories {
		if cat == category {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// HTML 处理工具
// ---------------------------------------------------------------------------

var (
	reHTMLTag    = regexp.MustCompile(`<[^>]+>`)
	reNumEntity  = regexp.MustCompile(`&#(\d+);`)
	reHexEntity  = regexp.MustCompile(`&#x([0-9a-fA-F]+);`)
	reMultiSpace = regexp.MustCompile(`\s+`)
	reLink       = regexp.MustCompile(`<a[^>]+href="(https?://[^"]+)"[^>]*>`)
	reH3         = regexp.MustCompile(`<h[1-6][^>]*>(.*?)</h[1-6]>`)
)

// readBody 读取 HTTP 响应体，限制最大 10MB 防止内存耗尽。
func readBody(r io.Reader) ([]byte, error) {
	return httputil.ReadBodyLimited(r, 10*1024*1024)
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

// parseGenericSearchResults 从 HTML 中提取通用搜索结果。
func parseGenericSearchResults(html string) []SearchResult {
	linkMatches := reLink.FindAllStringSubmatch(html, -1)

	var results []SearchResult
	seen := make(map[string]bool)

	for _, match := range linkMatches {
		if len(match) < 2 {
			continue
		}
		link := match[1]

		// 跳过搜索引擎自身链接和无效链接
		if isSearchEngineSelfLink(link) || seen[link] {
			continue
		}

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

		if title == "" {
			u, err := url.Parse(link)
			if err == nil {
				title = u.Path
				if title == "" {
					title = u.Host
				}
			}
		}

		// 提取链接附近的文本作为 snippet
		snippet := extractSnippetNearLink(html, link)

		seen[link] = true
		results = append(results, SearchResult{
			Title:   title,
			URL:     link,
			Snippet: snippet,
			Score:   0.5,
		})
	}

	return results
}

// isSearchEngineSelfLink 判断是否为搜索引擎自身的链接
func isSearchEngineSelfLink(link string) bool {
	lower := strings.ToLower(link)
	selfDomains := []string{"www.so.com", "www.bing.com", "www.google.com", "duckduckgo.com", "search.yahoo.com"}
	for _, d := range selfDomains {
		if strings.Contains(lower, d) {
			return true
		}
	}
	return false
}

// extractSnippetNearLink 尝试从链接附近提取文本摘要
func extractSnippetNearLink(html, link string) string {
	// 查找链接在 HTML 中的位置
	linkIdx := strings.Index(html, link)
	if linkIdx < 0 {
		return ""
	}
	// 从链接位置向后搜索 2000 字符范围内的 <p> 标签内容
	searchEnd := linkIdx + 2000
	if searchEnd > len(html) {
		searchEnd = len(html)
	}
	region := html[linkIdx:searchEnd]

	// 尝试匹配 <p> 标签内容
	pRe := regexp.MustCompile(`<p[^>]*>(.*?)</p>`)
	pMatches := pRe.FindAllStringSubmatch(region, -1)
	for _, m := range pMatches {
		if len(m) >= 2 {
			text := stripHTML(m[1])
			if len(text) > 20 { // 忽略太短的文本
				return text
			}
		}
	}
	return ""
}

// containsCJK 检查字符串是否包含中日韩文字
func containsCJK(s string) bool {
	for _, r := range s {
		if (r >= 0x4e00 && r <= 0x9fff) || (r >= 0x3400 && r <= 0x4dbf) || (r >= 0x3040 && r <= 0x30ff) {
			return true
		}
	}
	return false
}

// dedupAndFilterResults 对搜索结果去重和过滤
func dedupAndFilterResults(results []SearchResult) []SearchResult {
	seen := make(map[string]bool)
	var filtered []SearchResult
	for _, r := range results {
		// 跳过空标题或空链接
		if r.Title == "" || r.URL == "" {
			continue
		}
		// 跳过搜索引擎自身链接
		if isSearchEngineSelfLink(r.URL) {
			continue
		}
		// 去重
		if seen[r.URL] {
			continue
		}
		seen[r.URL] = true
		filtered = append(filtered, r)
	}
	return filtered
}
