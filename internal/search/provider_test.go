// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestDoSearch_ErrorDoesNotLeakHeaders 验证 doSearch 在收到 401 等非 200 响应时，
// 返回的 error 字符串不包含 headers 中的敏感信息（如 Authorization、Bearer、API Key 值）。
//
// 这是任务 36 的核心安全验证：错误信息只能含 method/url(脱敏)/statusCode/bodySnippet，
// 显式不包含 headers，避免凭证泄露到错误链路或日志。
func TestDoSearch_ErrorDoesNotLeakHeaders(t *testing.T) {
	// 启动一个始终返回 401 的测试服务器
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	bp := &BaseProvider{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	// 模拟 Tavily/Ollama 真实请求时携带的敏感 headers
	secretToken := "super-secret-token-12345"
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + secretToken,
	}

	_, err := bp.doSearch(context.Background(), http.MethodPost, server.URL, strings.NewReader(`{}`), headers)
	if err == nil {
		t.Fatal("期望 401 响应返回 error，实际为 nil")
	}

	errStr := err.Error()

	// 核心断言：error 字符串绝不能包含敏感凭证
	if strings.Contains(errStr, "Bearer") {
		t.Errorf("error 字符串不应包含 'Bearer'，实际: %s", errStr)
	}
	if strings.Contains(errStr, "Authorization") {
		t.Errorf("error 字符串不应包含 'Authorization'，实际: %s", errStr)
	}
	if strings.Contains(errStr, secretToken) {
		t.Errorf("error 字符串不应包含 secret token 值，实际: %s", errStr)
	}

	// 验证 error 字符串包含必要的安全字段
	if !strings.Contains(errStr, "401") {
		t.Errorf("error 字符串应包含状态码 401，实际: %s", errStr)
	}
	if !strings.Contains(errStr, "statusCode=") {
		t.Errorf("error 字符串应包含 statusCode 字段，实际: %s", errStr)
	}
	if !strings.Contains(errStr, "method=") {
		t.Errorf("error 字符串应包含 method 字段，实际: %s", errStr)
	}
}

// TestDoSearch_URLQuerySanitized 验证 doSearch 错误信息中的 URL 已脱敏 query 参数，
// 避免 query 中的 api_key 等敏感参数泄露。
func TestDoSearch_URLQuerySanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`internal error`))
	}))
	defer server.Close()

	// 构造带敏感 query 参数的 URL
	urlWithQuery := server.URL + "/search?api_key=secret-query-token&query=test"

	bp := &BaseProvider{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}
	headers := map[string]string{
		"Authorization": "Bearer should-not-leak",
	}

	_, err := bp.doSearch(context.Background(), http.MethodGet, urlWithQuery, nil, headers)
	if err == nil {
		t.Fatal("期望 500 响应返回 error，实际为 nil")
	}

	errStr := err.Error()
	// query 中的敏感参数不应出现在 error 字符串中
	if strings.Contains(errStr, "secret-query-token") {
		t.Errorf("error 字符串不应包含 URL query 中的敏感参数，实际: %s", errStr)
	}
	if strings.Contains(errStr, "api_key=secret-query-token") {
		t.Errorf("error 字符串不应包含原始 query 字符串，实际: %s", errStr)
	}
}

// TestDoSearch_BodySnippetLimitedTo512 验证 bodySnippet 最多只取响应体前 512 字节，
// 避免大响应体撑爆错误信息/日志。
func TestDoSearch_BodySnippetLimitedTo512(t *testing.T) {
	// 构造一个超过 512 字节的响应体
	longBody := strings.Repeat("a", 2000)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(longBody))
	}))
	defer server.Close()

	bp := &BaseProvider{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	_, err := bp.doSearch(context.Background(), http.MethodGet, server.URL, nil, nil)
	if err == nil {
		t.Fatal("期望 502 响应返回 error，实际为 nil")
	}

	errStr := err.Error()
	// bodySnippet 应只含 512 个 'a'，而非全部 2000 个
	if strings.Count(errStr, "a") > 600 { // 容许少量 'a' 来自其他字段
		t.Errorf("error 字符串中的 bodySnippet 应限制在 512 字节内，实际包含过多 'a': %d", strings.Count(errStr, "a"))
	}
}

// TestDoSearch_SuccessReturnsBody 验证 200 响应正常返回 body，不受脱敏逻辑影响。
func TestDoSearch_SuccessReturnsBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	bp := &BaseProvider{
		httpClient: &http.Client{Timeout: 5 * time.Second},
	}

	resp, err := bp.doSearch(context.Background(), http.MethodGet, server.URL, nil, nil)
	if err != nil {
		t.Fatalf("200 响应不应返回 error，实际: %v", err)
	}
	if string(resp) != `{"ok":true}` {
		t.Errorf("响应体不匹配，实际: %s", string(resp))
	}
}

// ===== 纯函数测试 =====

// TestIsPrivateOrLoopback_Loopback 验证回环地址被识别为内网
//
// 生活类比：就像门卫检查信封地址，"127.0.0.1" 是本机内部地址，不允许外部搜索请求重定向到这里。
func TestIsPrivateOrLoopback_Loopback(t *testing.T) {
	cases := []string{
		"127.0.0.1",
		"::1",
		"localhost",
	}
	for _, host := range cases {
		if !isPrivateOrLoopback(host) {
			t.Errorf("isPrivateOrLoopback(%q) 期望 true（回环地址），实际 false", host)
		}
	}
}

// TestIsPrivateOrLoopback_PrivateIP 验证内网 IP 被识别
func TestIsPrivateOrLoopback_PrivateIP(t *testing.T) {
	cases := []string{
		"10.0.0.1",
		"172.16.0.1",
		"192.168.1.1",
		"169.254.1.1", // 链路本地
		"0.0.0.0",     // 未指定
	}
	for _, host := range cases {
		if !isPrivateOrLoopback(host) {
			t.Errorf("isPrivateOrLoopback(%q) 期望 true（内网地址），实际 false", host)
		}
	}
}

// TestIsPrivateOrLoopback_PublicIP 验证公网 IP 不被识别为内网
func TestIsPrivateOrLoopback_PublicIP(t *testing.T) {
	cases := []string{
		"8.8.8.8",
		"1.1.1.1",
		"114.114.114.114",
	}
	for _, host := range cases {
		if isPrivateOrLoopback(host) {
			t.Errorf("isPrivateOrLoopback(%q) 期望 false（公网地址），实际 true", host)
		}
	}
}

// TestIsSearchEngineSelfLink_KnownSelfLinks 验证已知搜索引擎自身链接被识别
func TestIsSearchEngineSelfLink_KnownSelfLinks(t *testing.T) {
	cases := []string{
		"https://www.so.com/s?q=test",
		"https://www.bing.com/search",
		"https://www.google.com/search",
		"https://duckduckgo.com/?q=test",
		"https://search.yahoo.com/search",
	}
	for _, link := range cases {
		if !isSearchEngineSelfLink(link) {
			t.Errorf("isSearchEngineSelfLink(%q) 期望 true，实际 false", link)
		}
	}
}

// TestIsSearchEngineSelfLink_NormalLink 验证普通链接不被识别为搜索引擎自身链接
func TestIsSearchEngineSelfLink_NormalLink(t *testing.T) {
	cases := []string{
		"https://example.com/article",
		"https://github.com/repo",
		"https://zh.wikipedia.org/wiki/Go",
		"https://blog.csdn.net/post/123",
	}
	for _, link := range cases {
		if isSearchEngineSelfLink(link) {
			t.Errorf("isSearchEngineSelfLink(%q) 期望 false，实际 true", link)
		}
	}
}

// TestIsSearchEngineSelfLink_CaseInsensitive 验证大小写不敏感
func TestIsSearchEngineSelfLink_CaseInsensitive(t *testing.T) {
	cases := []string{
		"HTTPS://WWW.BING.COM/search",
		"https://WWW.So.COM/s",
	}
	for _, link := range cases {
		if !isSearchEngineSelfLink(link) {
			t.Errorf("isSearchEngineSelfLink(%q) 大小写不敏感应返回 true，实际 false", link)
		}
	}
}

// TestIsSearchEngineSelfLink_SubstringNoFalsePositive 回归：子串包含不应误判
// 域名仅作为路径/参数一部分出现时，不应被误判为搜索引擎自身链接。
func TestIsSearchEngineSelfLink_SubstringNoFalsePositive(t *testing.T) {
	cases := []string{
		"https://example.com/bing.com/report",
		"https://example.com/search?q=google.com",
		"https://bing-com.evil.com/",
		"https://www.google.com.evil.org/x",
	}
	for _, link := range cases {
		if isSearchEngineSelfLink(link) {
			t.Errorf("isSearchEngineSelfLink(%q) 期望 false，实际 true", link)
		}
	}
}

// TestDedupAndFilterResults_RemovesEmptyTitleOrURL 验证空标题或空链接被过滤
func TestDedupAndFilterResults_RemovesEmptyTitleOrURL(t *testing.T) {
	results := []SearchResult{
		{Title: "正常结果", URL: "https://example.com/1"},
		{Title: "", URL: "https://example.com/2"}, // 空标题
		{Title: "空链接", URL: ""},                   // 空链接
		{Title: "都空", URL: ""},                    // 都空
		{Title: "另一个正常", URL: "https://example.com/3"},
	}
	got := dedupAndFilterResults(results)
	if len(got) != 2 {
		t.Errorf("过滤后应有 2 个结果，实际: %d", len(got))
	}
}

// TestDedupAndFilterResults_RemovesSelfLinks 验证搜索引擎自身链接被过滤
func TestDedupAndFilterResults_RemovesSelfLinks(t *testing.T) {
	results := []SearchResult{
		{Title: "正常", URL: "https://example.com/1"},
		{Title: "搜索引擎自身", URL: "https://www.bing.com/search"},
		{Title: "正常2", URL: "https://example.com/2"},
	}
	got := dedupAndFilterResults(results)
	if len(got) != 2 {
		t.Errorf("过滤后应有 2 个结果（排除搜索引擎自身），实际: %d", len(got))
	}
}

// TestSafeDialControl_BlocksPrivateIPs 验证 safeDialControl 拦截内网/回环 IP（H2 修复）
//
// 生活类比：门卫（safeDialControl）在快递员出发前核对实际门牌号，
// 发现是"内部宿舍"（127.0.0.1）或"内网仓库"（10.0.0.1）时立即拦下。
func TestSafeDialControl_BlocksPrivateIPs(t *testing.T) {
	cases := []string{
		"127.0.0.1:80",
		"[::1]:80",
		"10.0.0.1:443",
		"172.16.0.1:443",
		"192.168.1.1:8080",
		"169.254.1.1:80", // 链路本地
		"0.0.0.0:80",     // 未指定
	}
	for _, addr := range cases {
		err := safeDialControl("tcp4", addr, nil)
		if err == nil {
			t.Errorf("safeDialControl(%q) 期望 error（拦截内网），实际 nil", addr)
		}
	}
}

// TestSafeDialControl_AllowsPublicIPs 验证 safeDialControl 放行公网 IP
func TestSafeDialControl_AllowsPublicIPs(t *testing.T) {
	cases := []string{
		"8.8.8.8:443",
		"1.1.1.1:443",
		"114.114.114.114:80",
	}
	for _, addr := range cases {
		err := safeDialControl("tcp4", addr, nil)
		if err != nil {
			t.Errorf("safeDialControl(%q) 期望 nil（放行公网），实际: %v", addr, err)
		}
	}
}

// TestSafeDialControl_RejectsNonIP 验证非 IP 地址被拒绝（理论上不应发生，但兜底）
func TestSafeDialControl_RejectsNonIP(t *testing.T) {
	err := safeDialControl("tcp4", "example.com:80", nil)
	if err == nil {
		t.Errorf("safeDialControl(%q) 期望 error（非 IP 应被拒绝），实际 nil", "example.com:80")
	}
}

// TestNewSearchHTTPClient_BlocksLoopbackDial 集成测试：
// 验证 newSearchHTTPClient 实际连接 127.0.0.1 时被 Control 钩子拦截（H2 修复）
//
// 这是 DNS rebinding 防护的核心验证：即使 CheckRedirect 漏过，Control 仍会在 connect 前拦截。
// 用 httptest 启动本地服务器（绑定 127.0.0.1），用 newSearchHTTPClient 请求应失败。
func TestNewSearchHTTPClient_BlocksLoopbackDial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("should not reach"))
	}))
	defer server.Close()

	client := newSearchHTTPClient(5 * time.Second)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := client.Do(req)
	// 期望连接被 Control 拦截，返回 error
	if err == nil {
		resp.Body.Close()
		t.Fatalf("期望请求 127.0.0.1 被 Control 拦截，实际成功（状态码 %d）", resp.StatusCode)
	}
	if !strings.Contains(err.Error(), "private/loopback") {
		t.Errorf("期望 error 包含 'private/loopback'，实际: %v", err)
	}
}

// TestDedupAndFilterResults_DeduplicatesByURL 验证按 URL 去重
func TestDedupAndFilterResults_DeduplicatesByURL(t *testing.T) {
	results := []SearchResult{
		{Title: "第一次", URL: "https://example.com/dup"},
		{Title: "重复", URL: "https://example.com/dup"},
		{Title: "唯一", URL: "https://example.com/unique"},
	}
	got := dedupAndFilterResults(results)
	if len(got) != 2 {
		t.Errorf("去重后应有 2 个结果，实际: %d", len(got))
	}
}

// TestDedupAndFilterResults_EmptyInput 验证空输入返回 nil
func TestDedupAndFilterResults_EmptyInput(t *testing.T) {
	got := dedupAndFilterResults(nil)
	if got != nil {
		t.Errorf("空输入应返回 nil，实际: %v", got)
	}
}

// TestSanitizeSearchURL_RemovesQuery 验证 URL query 参数被清除
func TestSanitizeSearchURL_RemovesQuery(t *testing.T) {
	cases := []struct {
		name    string
		rawURL  string
		wantSub string
	}{
		{"带 api_key", "https://api.tavily.com/search?api_key=secret123&q=test", "api_key"},
		{"带 query", "https://example.com/search?q=hello&page=2", "q=hello"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeSearchURL(c.rawURL)
			if strings.Contains(got, c.wantSub) {
				t.Errorf("sanitizeSearchURL 应清除 query，%q 仍包含 %q", got, c.wantSub)
			}
		})
	}
}

// TestSanitizeSearchURL_PreservesPath 验证 URL 路径被保留
func TestSanitizeSearchURL_PreservesPath(t *testing.T) {
	got := sanitizeSearchURL("https://api.tavily.com/search?api_key=secret")
	if !strings.Contains(got, "/search") {
		t.Errorf("sanitizeSearchURL 应保留路径 /search，实际: %q", got)
	}
	if !strings.HasPrefix(got, "https://api.tavily.com") {
		t.Errorf("sanitizeSearchURL 应保留 scheme 和 host，实际: %q", got)
	}
}

// TestSanitizeSearchURL_InvalidURL 验证无效 URL 返回原值
func TestSanitizeSearchURL_InvalidURL(t *testing.T) {
	// url.Parse 对大多数字符串都能解析，这里用控制字符测试极端情况
	invalid := "://no-scheme"
	got := sanitizeSearchURL(invalid)
	// 即使解析失败，也应返回原值或合理结果，不应 panic
	if got == "" {
		t.Errorf("无效 URL 应返回原值，实际返回空字符串")
	}
}

// TestMatchCategory_NoCategories 验证未设置分类的 provider 匹配所有分类
func TestMatchCategory_NoCategories(t *testing.T) {
	pw := &ProviderWithCircuit{
		categories: nil,
	}
	if !matchCategory(pw, "general") {
		t.Error("未设置分类的 provider 应匹配所有分类")
	}
	if !matchCategory(pw, "code") {
		t.Error("未设置分类的 provider 应匹配所有分类")
	}
}

// TestMatchCategory_WithCategories 验证设置了分类的 provider 只匹配指定分类
func TestMatchCategory_WithCategories(t *testing.T) {
	pw := &ProviderWithCircuit{
		categories: []string{"general", "news"},
	}
	if !matchCategory(pw, "general") {
		t.Error("应匹配 general 分类")
	}
	if !matchCategory(pw, "news") {
		t.Error("应匹配 news 分类")
	}
	if matchCategory(pw, "code") {
		t.Error("不应匹配 code 分类")
	}
}

// TestCircuitState_String 验证熔断状态字符串表示
func TestCircuitState_String(t *testing.T) {
	cases := []struct {
		state CircuitState
		want  string
	}{
		{CircuitClosed, "closed"},
		{CircuitOpen, "open"},
		{CircuitHalfOpen, "half-open"},
		{CircuitState(99), "unknown"},
	}
	for _, c := range cases {
		got := c.state.String()
		if got != c.want {
			t.Errorf("CircuitState(%d).String() = %q, 期望 %q", c.state, got, c.want)
		}
	}
}

// TestHeaderKeys_ReturnsAllKeys 验证返回所有 header key
func TestHeaderKeys_ReturnsAllKeys(t *testing.T) {
	headers := map[string]string{
		"Authorization": "Bearer token",
		"Content-Type":  "application/json",
		"X-Custom":      "value",
	}
	keys := headerKeys(headers)
	if len(keys) != 3 {
		t.Errorf("应返回 3 个 key，实际: %d", len(keys))
	}
	// 验证每个 key 都在原 headers 中
	for _, k := range keys {
		if _, ok := headers[k]; !ok {
			t.Errorf("返回的 key %q 不在原 headers 中", k)
		}
	}
}

// TestHeaderKeys_EmptyMap 验证空 map 返回 nil
func TestHeaderKeys_EmptyMap(t *testing.T) {
	keys := headerKeys(nil)
	if keys != nil {
		t.Errorf("空 map 应返回 nil，实际: %v", keys)
	}
}

// TestSanitizeSearchURL 验证 URL 脱敏逻辑：清空 RawQuery，保留 scheme/host/path。
func TestSanitizeSearchURL(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string // 期望脱敏后不再包含的部分
	}{
		{"带 query 参数", "https://api.example.com/search?api_key=secret&q=test", "api_key=secret"},
		{"带多个 query 参数", "https://api.example.com/search?a=1&b=2&c=3", "a=1&b=2"},
		{"无 query 参数", "https://api.example.com/search", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := sanitizeSearchURL(c.input)
			if c.want != "" && strings.Contains(got, c.want) {
				t.Errorf("sanitizeSearchURL(%q) = %q，不应包含 %q", c.input, got, c.want)
			}
			// 验证 host 保留
			if !strings.Contains(got, "api.example.com") {
				t.Errorf("sanitizeSearchURL(%q) = %q，应保留 host", c.input, got)
			}
		})
	}
}

// TestHeaderKeys 验证 headerKeys 只返回 key 不返回 value。
func TestHeaderKeys(t *testing.T) {
	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer secret",
	}
	keys := headerKeys(headers)
	if len(keys) != 2 {
		t.Fatalf("期望返回 2 个 key，实际: %d", len(keys))
	}
	// 验证只含 key 不含 value
	joined := strings.Join(keys, ",")
	if strings.Contains(joined, "Bearer secret") {
		t.Errorf("headerKeys 不应包含 value，实际: %s", joined)
	}
	if strings.Contains(joined, "application/json") {
		t.Errorf("headerKeys 不应包含 value，实际: %s", joined)
	}
}

// TestHeaderKeys_NilMap 验证空 headers 返回 nil。
func TestHeaderKeys_NilMap(t *testing.T) {
	if got := headerKeys(nil); got != nil {
		t.Errorf("headerKeys(nil) 应返回 nil，实际: %v", got)
	}
	if got := headerKeys(map[string]string{}); got != nil {
		t.Errorf("headerKeys(空 map) 应返回 nil，实际: %v", got)
	}
}
