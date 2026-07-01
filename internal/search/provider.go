// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/httputil"
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
// BaseProvider —— 通用 HTTP 请求骨架
// ---------------------------------------------------------------------------

// BaseProvider 为搜索 Provider 提供通用的 HTTP 请求骨架，
// 消除各 Provider 中重复的"构造请求 → 设置 headers → Do → 状态码检查 → readBody"逻辑。
// 各 Provider 通过嵌入 BaseProvider 复用 doSearch 方法，并保留自己的响应解析逻辑。
type BaseProvider struct {
	httpClient *http.Client
}

// newSearchHTTPClient 创建带安全重定向策略的搜索 HTTP 客户端。
// 安全实践：限制重定向目标，禁止跳转至内网/回环地址，防止 SSRF（见安全审查 #25）。
func newSearchHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			host := req.URL.Hostname()
			if isPrivateOrLoopback(host) {
				return fmt.Errorf("redirect to private/loopback address blocked: %s", host)
			}
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}
}

// isPrivateOrLoopback 判断主机是否为内网/回环/链路本地/未指定地址。
// 无法解析的主机名视为公网（可能是本地域名），返回 false。
func isPrivateOrLoopback(host string) bool {
	ip := net.ParseIP(host)
	if ip == nil {
		ips, err := net.LookupIP(host)
		if err != nil || len(ips) == 0 {
			return false // 无法解析，允许（可能是本地域名）
		}
		ip = ips[0]
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified()
}

// doSearch 执行搜索请求并返回响应体字节，消除 4 个 Provider 的重复请求骨架。
// 通用流程：构造请求 → 设置 headers → Do → defer Close → readBody → 状态码检查
// 各 Provider 负责自己的响应解析逻辑（JSON / HTML），因此本方法只返回原始响应体 []byte。
//
// 安全说明：构造错误信息时严格脱敏，仅包含 method、url（清空 RawQuery）、
// statusCode、bodySnippet（响应体前 512 字节），显式不包含 headers，
// 避免 Authorization 等敏感凭证泄露到错误链路/日志中。
func (b *BaseProvider) doSearch(ctx context.Context, method, rawURL string, body io.Reader, headers map[string]string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, rawURL, body)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := httputil.ReadBodyLimited(resp.Body, 10*1024*1024)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		// 构造脱敏错误信息：仅含 method、url（脱敏 query）、statusCode、bodySnippet（前 512 字节）
		// 显式不包含 headers，避免 Authorization / API Key 等敏感信息进入 error 链路
		sanitizedURL := sanitizeSearchURL(rawURL)
		bodySnippet := respBody
		if len(bodySnippet) > 512 {
			bodySnippet = bodySnippet[:512]
		}
		// 调试时仅记录 headers 的 key（不记录 value），便于排查又不会泄露凭证
		log.Debug().
			Strs("headers_keys", headerKeys(headers)).
			Str("method", method).
			Str("url", sanitizedURL).
			Int("status", resp.StatusCode).
			Msg("[search] request failed with non-200 status")
		return nil, fmt.Errorf("search failed: method=%s url=%s statusCode=%d bodySnippet=%s",
			method, sanitizedURL, resp.StatusCode, string(bodySnippet))
	}

	return respBody, nil
}

// sanitizeSearchURL 脱敏 URL：清空 RawQuery，避免 query 参数中的敏感信息
// （如 api_key=xxx）泄露到错误信息中。解析失败时返回原值，保证可用性。
func sanitizeSearchURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = ""
	return u.String()
}

// headerKeys 返回 headers 的所有 key（不含 value），仅用于调试日志。
func headerKeys(headers map[string]string) []string {
	if len(headers) == 0 {
		return nil
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	return keys
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
	maxAttempts := max(pw.MaxRetries+1, 1)

	var lastErr error
	for attempt := range maxAttempts {
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
	return slices.Contains(pw.categories, category)
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
