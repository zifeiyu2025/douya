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
