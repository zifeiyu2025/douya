// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// mockHandler 自定义端点处理函数
// 接收 HTTP 请求，返回响应体（会被 JSON 编码）和 HTTP 状态码
// 如果返回 nil 响应体，则不写入响应体（仅写入状态码）
type mockHandler func(r *http.Request) (any, int)

// mockServerBuilder 用于构建 mock llama-server
// 封装了所有必要端点的默认响应，支持通过 WithHandler 自定义覆盖
type mockServerBuilder struct {
	handlers map[string]mockHandler
}

// MockServer 封装模拟的 llama-server
// 自动记录所有收到的请求，便于测试中断言请求内容
type MockServer struct {
	*httptest.Server
	// requests 按端点（"METHOD /path"）记录所有收到的请求
	requests map[string][]*http.Request
	// bodies 按端点记录所有请求体（已读取的字节副本）
	bodies map[string][][]byte
}

// newMockServerBuilder 创建默认的 mock server 构建器
// 注册所有必要端点的默认成功响应：
//   - POST /models/load
//   - GET /v1/models
//   - POST /tokenize
//   - POST /apply-template
//   - GET /lora-adapters
//   - POST /lora-adapters
//   - GET /slots
//   - POST /v1/chat/completions/input_tokens
//   - DELETE /models
func newMockServerBuilder() *mockServerBuilder {
	b := &mockServerBuilder{
		handlers: make(map[string]mockHandler),
	}

	// POST /models/load - 默认返回成功
	b.handlers["POST /models/load"] = func(r *http.Request) (any, int) {
		return map[string]string{"status": "ok"}, http.StatusOK
	}

	// GET /v1/models - 默认返回一个已加载的模型
	b.handlers["GET /v1/models"] = func(r *http.Request) (any, int) {
		return map[string]any{
			"data": []map[string]any{
				{
					"id":           "test-model",
					"capabilities": []string{},
					"status":       map[string]string{"value": "loaded"},
				},
			},
		}, http.StatusOK
	}

	// POST /tokenize - 默认返回 token ID 列表
	b.handlers["POST /tokenize"] = func(r *http.Request) (any, int) {
		return map[string]any{"tokens": []int{1, 2, 3}}, http.StatusOK
	}

	// POST /apply-template - 默认返回格式化后的 prompt
	b.handlers["POST /apply-template"] = func(r *http.Request) (any, int) {
		return map[string]string{"prompt": "formatted prompt"}, http.StatusOK
	}

	// GET /lora-adapters - 默认返回空列表
	b.handlers["GET /lora-adapters"] = func(r *http.Request) (any, int) {
		return []map[string]any{}, http.StatusOK
	}

	// POST /lora-adapters - 默认返回成功
	b.handlers["POST /lora-adapters"] = func(r *http.Request) (any, int) {
		return map[string]string{"status": "ok"}, http.StatusOK
	}

	// GET /slots - 默认返回空列表
	b.handlers["GET /slots"] = func(r *http.Request) (any, int) {
		return []map[string]any{}, http.StatusOK
	}

	// POST /v1/chat/completions/input_tokens - 默认返回 token 数
	b.handlers["POST /v1/chat/completions/input_tokens"] = func(r *http.Request) (any, int) {
		return map[string]int{"input_tokens": 10}, http.StatusOK
	}

	// DELETE /models - 默认返回成功
	b.handlers["DELETE /models"] = func(r *http.Request) (any, int) {
		return map[string]string{"status": "ok"}, http.StatusOK
	}

	return b
}

// WithHandler 自定义某个端点的处理函数，覆盖默认响应
// method: HTTP 方法，如 "GET"、"POST"、"DELETE"
// path: 端点路径，如 "/models/load"
// 返回 builder 本身以支持链式调用
func (b *mockServerBuilder) WithHandler(method, path string, handler mockHandler) *mockServerBuilder {
	b.handlers[method+" "+path] = handler
	return b
}

// Build 构建 mock server 和对应的 client
// 返回的 MockServer 会自动记录所有请求，调用者负责在测试结束后调用 Close()
func (b *mockServerBuilder) Build(t *testing.T) (*MockServer, *Client) {
	t.Helper()

	ms := &MockServer{
		requests: make(map[string][]*http.Request),
		bodies:   make(map[string][][]byte),
	}

	ms.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path

		// 读取并记录请求体（在 handler 处理之前）
		if r.Body != nil {
			bodyBytes, _ := io.ReadAll(r.Body)
			ms.bodies[key] = append(ms.bodies[key], bodyBytes)
			// 重新填充 body 以便 handler 可以再次读取
			r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		// 记录请求
		ms.requests[key] = append(ms.requests[key], r)

		handler, ok := b.handlers[key]
		if !ok {
			// 未注册的端点返回 404
			w.WriteHeader(http.StatusNotFound)
			return
		}

		resp, status := handler(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if resp != nil {
			// 使用 json.NewEncoder 编码响应体
			if err := json.NewEncoder(w).Encode(resp); err != nil {
				t.Errorf("编码响应体失败: %v", err)
			}
		}
	}))

	client := NewClient(ms.URL, "")
	return ms, client
}

// RequestsFor 返回某个端点收到的所有请求
// method: HTTP 方法；path: 端点路径
func (ms *MockServer) RequestsFor(method, path string) []*http.Request {
	return ms.requests[method+" "+path]
}

// BodiesFor 返回某个端点收到的所有请求体（字节切片）
func (ms *MockServer) BodiesFor(method, path string) [][]byte {
	return ms.bodies[method+" "+path]
}

// RequestCount 返回某个端点收到的请求数
func (ms *MockServer) RequestCount(method, path string) int {
	return len(ms.requests[method+" "+path])
}

// LastBodyFor 返回某个端点最后一次收到的请求体
// 如果没有请求，返回 nil
func (ms *MockServer) LastBodyFor(method, path string) []byte {
	bodies := ms.bodies[method+" "+path]
	if len(bodies) == 0 {
		return nil
	}
	return bodies[len(bodies)-1]
}
