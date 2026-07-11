// Package httputil 提供 HTTP 相关的公共工具函数。
package httputil

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestReadBodyLimited 正常读取小于限制的内容
func TestReadBodyLimited(t *testing.T) {
	input := "hello world"
	r := strings.NewReader(input)
	got, err := ReadBodyLimited(r, 1024)
	if err != nil {
		t.Fatalf("ReadBodyLimited returned error: %v", err)
	}
	if string(got) != input {
		t.Fatalf("expected %q, got %q", input, string(got))
	}
}

// TestReadBodyLimitedTruncates 超过限制的内容应被截断
func TestReadBodyLimitedTruncates(t *testing.T) {
	// 生成 100 字节内容，限制为 10 字节
	input := strings.Repeat("a", 100)
	r := strings.NewReader(input)
	got, err := ReadBodyLimited(r, 10)
	if err != nil {
		t.Fatalf("ReadBodyLimited returned error: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("expected 10 bytes, got %d", len(got))
	}
}

// TestReadBodyLimitedEmpty 空输入返回空字节
func TestReadBodyLimitedEmpty(t *testing.T) {
	r := strings.NewReader("")
	got, err := ReadBodyLimited(r, 1024)
	if err != nil {
		t.Fatalf("ReadBodyLimited returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d bytes", got)
	}
}

// ===== DoAndUnmarshal 测试 =====
//
// DoAndUnmarshal 执行 HTTP 请求并反序列化响应体，是所有外部 API 调用的核心工具。
// 需要覆盖：成功路径、请求失败、非200状态码、反序列化失败、响应体截断等场景。

// TestDoAndUnmarshal_Success 正常的 JSON 响应应被正确反序列化
func TestDoAndUnmarshal_Success(t *testing.T) {
	type payload struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"豆芽","age":1}`))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	result, body, err := DoAndUnmarshal[payload](srv.Client(), req, 1024)
	if err != nil {
		t.Fatalf("DoAndUnmarshal 返回错误: %v", err)
	}
	if result == nil {
		t.Fatal("result 不应为 nil")
	}
	if result.Name != "豆芽" {
		t.Errorf("Name 期望 '豆芽', 实际 %q", result.Name)
	}
	if result.Age != 1 {
		t.Errorf("Age 期望 1, 实际 %d", result.Age)
	}
	if len(body) == 0 {
		t.Error("body 不应为空")
	}
}

// TestDoAndUnmarshal_RequestError 请求失败（连接拒绝）应返回错误
func TestDoAndUnmarshal_RequestError(t *testing.T) {
	// 使用一个不存在的端口模拟连接失败
	req, _ := http.NewRequest("GET", "http://127.0.0.1:1", nil)
	_, _, err := DoAndUnmarshal[map[string]any](http.DefaultClient, req, 1024)
	if err == nil {
		t.Fatal("连接失败应返回错误")
	}
	if !strings.Contains(err.Error(), "request failed") {
		t.Errorf("错误信息应包含 'request failed', 实际: %v", err)
	}
}

// TestDoAndUnmarshal_NonOKStatus 非200状态码应返回错误，body 仍返回供诊断
func TestDoAndUnmarshal_NonOKStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	result, body, err := DoAndUnmarshal[map[string]any](srv.Client(), req, 1024)
	if err == nil {
		t.Fatal("非200状态码应返回错误")
	}
	if result != nil {
		t.Error("非200时 result 应为 nil")
	}
	if !strings.Contains(string(body), "internal server error") {
		t.Errorf("body 应包含错误信息供诊断, 实际: %s", string(body))
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("错误信息应包含状态码 500, 实际: %v", err)
	}
}

// TestDoAndUnmarshal_UnmarshalFail JSON 格式错误应返回反序列化错误
func TestDoAndUnmarshal_UnmarshalFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{invalid json`))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, body, err := DoAndUnmarshal[map[string]any](srv.Client(), req, 1024)
	if err == nil {
		t.Fatal("JSON 格式错误应返回错误")
	}
	if !strings.Contains(err.Error(), "unmarshal failed") {
		t.Errorf("错误信息应包含 'unmarshal failed', 实际: %v", err)
	}
	if len(body) == 0 {
		t.Error("反序列化失败时 body 仍应返回供诊断")
	}
}

// TestDoAndUnmarshal_BodyTruncated 超过 maxBodySize 的响应体应被截断
func TestDoAndUnmarshal_BodyTruncated(t *testing.T) {
	// 返回一个很大的 JSON，但 maxBodySize 设为 10 字节
	// 截断后的内容不是有效 JSON，应返回 unmarshal 错误
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"name":"this is a very long response that exceeds the limit"}`))
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, body, err := DoAndUnmarshal[map[string]any](srv.Client(), req, 10)
	if err == nil {
		t.Fatal("截断后应因 JSON 不完整而返回错误")
	}
	if len(body) != 10 {
		t.Errorf("body 应被截断为 10 字节, 实际 %d 字节", len(body))
	}
}

// TestDoAndUnmarshal_EmptyBody 空响应体应返回反序列化错误（空不是有效 JSON）
func TestDoAndUnmarshal_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, _ := http.NewRequest("GET", srv.URL, nil)
	_, _, err := DoAndUnmarshal[map[string]any](srv.Client(), req, 1024)
	if err == nil {
		t.Fatal("空响应体应返回反序列化错误")
	}
}

// ===== ReadErrorBody 测试 =====
//
// ReadErrorBody 读取非200响应的 body 并格式化错误信息。

// TestReadErrorBody_NormalError 正常的错误响应应格式化包含状态码和body
func TestReadErrorBody_NormalError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"invalid api key"}`))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("HTTP 请求失败: %v", err)
	}
	defer resp.Body.Close()

	err = ReadErrorBody(resp, "API认证失败")
	if err == nil {
		t.Fatal("ReadErrorBody 应返回错误")
	}
	errMsg := err.Error()
	if !strings.Contains(errMsg, "API认证失败") {
		t.Errorf("错误信息应包含前缀 'API认证失败', 实际: %s", errMsg)
	}
	if !strings.Contains(errMsg, "401") {
		t.Errorf("错误信息应包含状态码 401, 实际: %s", errMsg)
	}
	if !strings.Contains(errMsg, "invalid api key") {
		t.Errorf("错误信息应包含 body 内容, 实际: %s", errMsg)
	}
}

// TestReadErrorBody_EmptyBody 空body的错误响应应正常格式化
func TestReadErrorBody_EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("HTTP 请求失败: %v", err)
	}
	defer resp.Body.Close()

	err = ReadErrorBody(resp, "服务不可用")
	if err == nil {
		t.Fatal("ReadErrorBody 应返回错误")
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("错误信息应包含状态码 502, 实际: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "服务不可用") {
		t.Errorf("错误信息应包含前缀 '服务不可用', 实际: %s", err.Error())
	}
}

// TestReadErrorBody_LargeBody 超大错误响应体应被截断（64KB限制）
func TestReadErrorBody_LargeBody(t *testing.T) {
	largeBody := strings.Repeat("x", 100*1024) // 100KB，超过 64KB 限制
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(largeBody))
	}))
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("HTTP 请求失败: %v", err)
	}
	defer resp.Body.Close()

	err = ReadErrorBody(resp, "服务超载")
	if err == nil {
		t.Fatal("ReadErrorBody 应返回错误")
	}
	// 错误信息不应包含完整的 100KB body（被截断为 64KB）
	// 验证错误信息长度远小于 100KB
	if len(err.Error()) > 70*1024 {
		t.Errorf("错误信息应被截断, 实际长度 %d", len(err.Error()))
	}
}
