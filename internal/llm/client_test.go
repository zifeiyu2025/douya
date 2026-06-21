// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestDeleteModel 测试 DeleteModel 方法
// 验证 DELETE /models 请求，请求体包含 {"model": "test-model"}
func TestDeleteModel(t *testing.T) {
	tests := []struct {
		name        string
		modelName   string
		handler     mockHandler // 自定义 DELETE /models 的响应
		wantErr     bool
		errContains string
	}{
		{
			name:      "成功删除模型",
			modelName: "test-model",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"status": "ok"}, http.StatusOK
			},
			wantErr: false,
		},
		{
			name:      "删除不存在的模型返回404",
			modelName: "nonexistent-model",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"error": "model not found"}, http.StatusNotFound
			},
			wantErr:     true,
			errContains: "404",
		},
		{
			name:      "服务器内部错误返回500",
			modelName: "test-model",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"error": "internal error"}, http.StatusInternalServerError
			},
			wantErr:     true,
			errContains: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, client := newMockServerBuilder().
				WithHandler(http.MethodDelete, "/models", tt.handler).
				Build(t)
			defer ms.Close()

			err := client.DeleteModel(context.Background(), tt.modelName)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，但得到 nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("期望错误包含 %q，实际为 %v", tt.errContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("不期望错误，但得到: %v", err)
				}
			}

			// 验证请求方法和路径
			if got := ms.RequestCount(http.MethodDelete, "/models"); got != 1 {
				t.Fatalf("期望 1 次 DELETE /models 请求，实际 %d 次", got)
			}

			// 验证请求体包含正确的 model 字段
			body := ms.LastBodyFor(http.MethodDelete, "/models")
			if body == nil {
				t.Fatal("期望收到请求体，但得到 nil")
			}
			var req map[string]string
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("解析请求体失败: %v", err)
			}
			if req["model"] != tt.modelName {
				t.Fatalf("期望 model=%q，实际 %q", tt.modelName, req["model"])
			}
		})
	}
}

// TestCountTokens 测试 CountTokens 方法
// 验证 POST /v1/chat/completions/input_tokens 请求，正确解析返回的 token 数
func TestCountTokens(t *testing.T) {
	tests := []struct {
		name         string
		messages     []ChatMessage
		handler      mockHandler
		wantTokens   int
		wantErr      bool
		errContains  string
	}{
		{
			name:     "成功获取token数",
			messages: []ChatMessage{NewTextMessage("user", "hello")},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]int{"input_tokens": 42}, http.StatusOK
			},
			wantTokens: 42,
			wantErr:    false,
		},
		{
			name:     "空消息列表返回0",
			messages: []ChatMessage{},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]int{"input_tokens": 0}, http.StatusOK
			},
			wantTokens: 0,
			wantErr:    false,
		},
		{
			name:     "多消息对话",
			messages: []ChatMessage{
				NewTextMessage("system", "you are helpful"),
				NewTextMessage("user", "what is 1+1?"),
				NewTextMessage("assistant", "2"),
			},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]int{"input_tokens": 100}, http.StatusOK
			},
			wantTokens: 100,
			wantErr:    false,
		},
		{
			name:     "服务器返回500错误",
			messages: []ChatMessage{NewTextMessage("user", "test")},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"error": "server error"}, http.StatusInternalServerError
			},
			wantErr:     true,
			errContains: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, client := newMockServerBuilder().
				WithHandler(http.MethodPost, "/v1/chat/completions/input_tokens", tt.handler).
				Build(t)
			defer ms.Close()

			tokens, err := client.CountTokens(context.Background(), tt.messages)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，但得到 nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("期望错误包含 %q，实际为 %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("不期望错误，但得到: %v", err)
			}
			if tokens != tt.wantTokens {
				t.Fatalf("期望 token 数 %d，实际 %d", tt.wantTokens, tokens)
			}

			// 验证请求方法和路径
			if got := ms.RequestCount(http.MethodPost, "/v1/chat/completions/input_tokens"); got != 1 {
				t.Fatalf("期望 1 次请求，实际 %d 次", got)
			}

			// 验证请求体包含 messages 字段
			body := ms.LastBodyFor(http.MethodPost, "/v1/chat/completions/input_tokens")
			var req map[string]interface{}
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("解析请求体失败: %v", err)
			}
			msgs, ok := req["messages"]
			if !ok {
				t.Fatal("请求体中缺少 messages 字段")
			}
			msgsArr, ok := msgs.([]interface{})
			if !ok {
				t.Fatalf("messages 字段不是数组，实际类型 %T", msgs)
			}
			if len(msgsArr) != len(tt.messages) {
				t.Fatalf("期望 messages 长度 %d，实际 %d", len(tt.messages), len(msgsArr))
			}
		})
	}
}

// TestGetLoraAdapters 测试 GetLoraAdapters 方法
// 验证 GET /lora-adapters，正确解析 []LoraAdapter 响应
func TestGetLoraAdapters(t *testing.T) {
	tests := []struct {
		name        string
		handler     mockHandler
		wantAdapters []LoraAdapter
		wantErr     bool
		errContains string
	}{
		{
			name: "成功获取空适配器列表",
			handler: func(r *http.Request) (interface{}, int) {
				return []interface{}{}, http.StatusOK
			},
			wantAdapters: []LoraAdapter{},
			wantErr:      false,
		},
		{
			name: "成功获取多个适配器",
			handler: func(r *http.Request) (interface{}, int) {
				return []map[string]interface{}{
					{"id": 0, "path": "/path/to/lora1.bin", "scale": 0.5},
					{"id": 1, "path": "/path/to/lora2.bin", "scale": 1.0},
				}, http.StatusOK
			},
			wantAdapters: []LoraAdapter{
				{ID: 0, Path: "/path/to/lora1.bin", Scale: 0.5},
				{ID: 1, Path: "/path/to/lora2.bin", Scale: 1.0},
			},
			wantErr: false,
		},
		{
			name: "单个适配器",
			handler: func(r *http.Request) (interface{}, int) {
				return []map[string]interface{}{
					{"id": 5, "path": "/models/adapter.gguf", "scale": 0.8},
				}, http.StatusOK
			},
			wantAdapters: []LoraAdapter{
				{ID: 5, Path: "/models/adapter.gguf", Scale: 0.8},
			},
			wantErr: false,
		},
		{
			name: "服务器返回403错误",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"error": "forbidden"}, http.StatusForbidden
			},
			wantErr:     true,
			errContains: "403",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, client := newMockServerBuilder().
				WithHandler(http.MethodGet, "/lora-adapters", tt.handler).
				Build(t)
			defer ms.Close()

			adapters, err := client.GetLoraAdapters(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，但得到 nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("期望错误包含 %q，实际为 %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("不期望错误，但得到: %v", err)
			}
			if len(adapters) != len(tt.wantAdapters) {
				t.Fatalf("期望 %d 个适配器，实际 %d 个", len(tt.wantAdapters), len(adapters))
			}
			for i, want := range tt.wantAdapters {
				if adapters[i].ID != want.ID {
					t.Errorf("适配器 %d: 期望 ID=%d，实际 ID=%d", i, want.ID, adapters[i].ID)
				}
				if adapters[i].Path != want.Path {
					t.Errorf("适配器 %d: 期望 Path=%q，实际 Path=%q", i, want.Path, adapters[i].Path)
				}
				if adapters[i].Scale != want.Scale {
					t.Errorf("适配器 %d: 期望 Scale=%v，实际 Scale=%v", i, want.Scale, adapters[i].Scale)
				}
			}

			// 验证请求方法和路径
			if got := ms.RequestCount(http.MethodGet, "/lora-adapters"); got != 1 {
				t.Fatalf("期望 1 次请求，实际 %d 次", got)
			}
		})
	}
}

// TestSetLoraAdapters 测试 SetLoraAdapters 方法
// 验证 POST /lora-adapters，正确发送适配器列表
func TestSetLoraAdapters(t *testing.T) {
	tests := []struct {
		name        string
		adapters    []LoraAdapter
		handler     mockHandler
		wantErr     bool
		errContains string
	}{
		{
			name: "成功设置空适配器列表",
			adapters: []LoraAdapter{},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"status": "ok"}, http.StatusOK
			},
			wantErr: false,
		},
		{
			name: "成功设置单个适配器",
			adapters: []LoraAdapter{
				{ID: 0, Path: "/path/to/lora.bin", Scale: 0.5},
			},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"status": "ok"}, http.StatusOK
			},
			wantErr: false,
		},
		{
			name: "成功设置多个适配器",
			adapters: []LoraAdapter{
				{ID: 0, Path: "/path/to/lora1.bin", Scale: 0.5},
				{ID: 1, Path: "/path/to/lora2.bin", Scale: 1.0},
			},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"status": "ok"}, http.StatusOK
			},
			wantErr: false,
		},
		{
			name: "服务器返回400错误",
			adapters: []LoraAdapter{
				{ID: 0, Path: "/invalid/path", Scale: 0.5},
			},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"error": "invalid adapter"}, http.StatusBadRequest
			},
			wantErr:     true,
			errContains: "400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, client := newMockServerBuilder().
				WithHandler(http.MethodPost, "/lora-adapters", tt.handler).
				Build(t)
			defer ms.Close()

			err := client.SetLoraAdapters(context.Background(), tt.adapters)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，但得到 nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("期望错误包含 %q，实际为 %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("不期望错误，但得到: %v", err)
			}

			// 验证请求方法和路径
			if got := ms.RequestCount(http.MethodPost, "/lora-adapters"); got != 1 {
				t.Fatalf("期望 1 次请求，实际 %d 次", got)
			}

			// 验证请求体是适配器列表的 JSON 数组
			body := ms.LastBodyFor(http.MethodPost, "/lora-adapters")
			if body == nil {
				t.Fatal("期望收到请求体，但得到 nil")
			}
			var sentAdapters []LoraAdapter
			if err := json.Unmarshal(body, &sentAdapters); err != nil {
				t.Fatalf("解析请求体失败: %v", err)
			}
			if len(sentAdapters) != len(tt.adapters) {
				t.Fatalf("期望发送 %d 个适配器，实际 %d 个", len(tt.adapters), len(sentAdapters))
			}
			for i, want := range tt.adapters {
				if sentAdapters[i].ID != want.ID {
					t.Errorf("适配器 %d: 期望 ID=%d，实际 ID=%d", i, want.ID, sentAdapters[i].ID)
				}
				if sentAdapters[i].Path != want.Path {
					t.Errorf("适配器 %d: 期望 Path=%q，实际 Path=%q", i, want.Path, sentAdapters[i].Path)
				}
				if sentAdapters[i].Scale != want.Scale {
					t.Errorf("适配器 %d: 期望 Scale=%v，实际 Scale=%v", i, want.Scale, sentAdapters[i].Scale)
				}
			}
		})
	}
}

// TestGetSlots 测试 GetSlots 方法
// 验证 GET /slots，正确解析 []SlotInfo 响应
func TestGetSlots(t *testing.T) {
	tests := []struct {
		name        string
		handler     mockHandler
		wantSlots   []SlotInfo
		wantErr     bool
		errContains string
	}{
		{
			name: "成功获取空slot列表",
			handler: func(r *http.Request) (interface{}, int) {
				return []interface{}{}, http.StatusOK
			},
			wantSlots: []SlotInfo{},
			wantErr:   false,
		},
		{
			name: "成功获取单个slot",
			handler: func(r *http.Request) (interface{}, int) {
				return []map[string]interface{}{
					{
						"id":             0,
						"task":           "process",
						"n_prompt":       10,
						"n_predicted":    20,
						"n_gpu_layers":   30,
						"model":          "test-model",
						"n_cache_tokens": 15,
						"cache_shift":    false,
					},
				}, http.StatusOK
			},
			wantSlots: []SlotInfo{
				{ID: 0, Task: "process", NPrompt: 10, NPredicted: 20, NGpuLayers: 30, Model: "test-model", NCacheTokens: 15, CacheShift: false},
			},
			wantErr: false,
		},
		{
			name: "成功获取多个slot",
			handler: func(r *http.Request) (interface{}, int) {
				return []map[string]interface{}{
					{
						"id":             0,
						"task":           "idle",
						"n_prompt":       0,
						"n_predicted":    0,
						"n_gpu_layers":   30,
						"model":          "model-a",
						"n_cache_tokens": 0,
						"cache_shift":    false,
					},
					{
						"id":             1,
						"task":           "process",
						"n_prompt":       50,
						"n_predicted":    100,
						"n_gpu_layers":   30,
						"model":          "model-a",
						"n_cache_tokens": 50,
						"cache_shift":    true,
					},
				}, http.StatusOK
			},
			wantSlots: []SlotInfo{
				{ID: 0, Task: "idle", NPrompt: 0, NPredicted: 0, NGpuLayers: 30, Model: "model-a", NCacheTokens: 0, CacheShift: false},
				{ID: 1, Task: "process", NPrompt: 50, NPredicted: 100, NGpuLayers: 30, Model: "model-a", NCacheTokens: 50, CacheShift: true},
			},
			wantErr: false,
		},
		{
			name: "服务器返回503错误",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"error": "unavailable"}, http.StatusServiceUnavailable
			},
			wantErr:     true,
			errContains: "503",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, client := newMockServerBuilder().
				WithHandler(http.MethodGet, "/slots", tt.handler).
				Build(t)
			defer ms.Close()

			slots, err := client.GetSlots(context.Background())

			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，但得到 nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("期望错误包含 %q，实际为 %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("不期望错误，但得到: %v", err)
			}
			if len(slots) != len(tt.wantSlots) {
				t.Fatalf("期望 %d 个 slot，实际 %d 个", len(tt.wantSlots), len(slots))
			}
			for i, want := range tt.wantSlots {
				if slots[i].ID != want.ID {
					t.Errorf("slot %d: 期望 ID=%d，实际 ID=%d", i, want.ID, slots[i].ID)
				}
				if slots[i].Task != want.Task {
					t.Errorf("slot %d: 期望 Task=%q，实际 Task=%q", i, want.Task, slots[i].Task)
				}
				if slots[i].NPrompt != want.NPrompt {
					t.Errorf("slot %d: 期望 NPrompt=%d，实际 NPrompt=%d", i, want.NPrompt, slots[i].NPrompt)
				}
				if slots[i].NPredicted != want.NPredicted {
					t.Errorf("slot %d: 期望 NPredicted=%d，实际 NPredicted=%d", i, want.NPredicted, slots[i].NPredicted)
				}
				if slots[i].NGpuLayers != want.NGpuLayers {
					t.Errorf("slot %d: 期望 NGpuLayers=%d，实际 NGpuLayers=%d", i, want.NGpuLayers, slots[i].NGpuLayers)
				}
				if slots[i].Model != want.Model {
					t.Errorf("slot %d: 期望 Model=%q，实际 Model=%q", i, want.Model, slots[i].Model)
				}
				if slots[i].NCacheTokens != want.NCacheTokens {
					t.Errorf("slot %d: 期望 NCacheTokens=%d，实际 NCacheTokens=%d", i, want.NCacheTokens, slots[i].NCacheTokens)
				}
				if slots[i].CacheShift != want.CacheShift {
					t.Errorf("slot %d: 期望 CacheShift=%v，实际 CacheShift=%v", i, want.CacheShift, slots[i].CacheShift)
				}
			}

			// 验证请求方法和路径
			if got := ms.RequestCount(http.MethodGet, "/slots"); got != 1 {
				t.Fatalf("期望 1 次请求，实际 %d 次", got)
			}
		})
	}
}

// TestTokenize 测试 Tokenize 方法
// 验证 POST /tokenize，正确解析 token ID 列表
func TestTokenize(t *testing.T) {
	tests := []struct {
		name        string
		text        string
		handler     mockHandler
		wantTokens  []int
		wantErr     bool
		errContains string
	}{
		{
			name: "成功分词普通文本",
			text: "hello world",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]interface{}{"tokens": []int{1, 2, 3}}, http.StatusOK
			},
			wantTokens: []int{1, 2, 3},
			wantErr:    false,
		},
		{
			name: "成功分词中文文本",
			text: "你好世界",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]interface{}{"tokens": []int{100, 200, 300, 400}}, http.StatusOK
			},
			wantTokens: []int{100, 200, 300, 400},
			wantErr:    false,
		},
		{
			name: "空文本返回空token列表",
			text: "",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]interface{}{"tokens": []int{}}, http.StatusOK
			},
			wantTokens: []int{},
			wantErr:    false,
		},
		{
			name: "单个token",
			text: "a",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]interface{}{"tokens": []int{42}}, http.StatusOK
			},
			wantTokens: []int{42},
			wantErr:    false,
		},
		{
			name: "服务器返回500错误",
			text: "test",
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"error": "tokenizer error"}, http.StatusInternalServerError
			},
			wantErr:     true,
			errContains: "500",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, client := newMockServerBuilder().
				WithHandler(http.MethodPost, "/tokenize", tt.handler).
				Build(t)
			defer ms.Close()

			tokens, err := client.Tokenize(context.Background(), tt.text)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，但得到 nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("期望错误包含 %q，实际为 %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("不期望错误，但得到: %v", err)
			}
			if len(tokens) != len(tt.wantTokens) {
				t.Fatalf("期望 %d 个 token，实际 %d 个", len(tt.wantTokens), len(tokens))
			}
			for i, want := range tt.wantTokens {
				if tokens[i] != want {
					t.Errorf("token %d: 期望 %d，实际 %d", i, want, tokens[i])
				}
			}

			// 验证请求方法和路径
			if got := ms.RequestCount(http.MethodPost, "/tokenize"); got != 1 {
				t.Fatalf("期望 1 次请求，实际 %d 次", got)
			}

			// 验证请求体包含 content 字段
			body := ms.LastBodyFor(http.MethodPost, "/tokenize")
			if body == nil {
				t.Fatal("期望收到请求体，但得到 nil")
			}
			var req map[string]string
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("解析请求体失败: %v", err)
			}
			if req["content"] != tt.text {
				t.Fatalf("期望 content=%q，实际 %q", tt.text, req["content"])
			}
		})
	}
}

// TestApplyTemplate 测试 ApplyTemplate 方法
// 验证 POST /apply-template，正确解析格式化后的字符串
func TestApplyTemplate(t *testing.T) {
	tests := []struct {
		name        string
		messages    []ChatMessage
		handler     mockHandler
		wantPrompt  string
		wantErr     bool
		errContains string
	}{
		{
			name: "成功应用模板",
			messages: []ChatMessage{
				NewTextMessage("user", "hello"),
			},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"prompt": "<|user|>hello<|end|>"}, http.StatusOK
			},
			wantPrompt: "<|user|>hello<|end|>",
			wantErr:    false,
		},
		{
			name: "多消息对话模板",
			messages: []ChatMessage{
				NewTextMessage("system", "you are helpful"),
				NewTextMessage("user", "what is 1+1?"),
				NewTextMessage("assistant", "2"),
			},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"prompt": "<|system|>you are helpful<|end|><|user|>what is 1+1?<|end|><|assistant|>2<|end|>"}, http.StatusOK
			},
			wantPrompt: "<|system|>you are helpful<|end|><|user|>what is 1+1?<|end|><|assistant|>2<|end|>",
			wantErr:    false,
		},
		{
			name: "空prompt响应",
			messages: []ChatMessage{
				NewTextMessage("user", ""),
			},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"prompt": ""}, http.StatusOK
			},
			wantPrompt: "",
			wantErr:    false,
		},
		{
			name: "中文消息模板",
			messages: []ChatMessage{
				NewTextMessage("user", "你好"),
			},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"prompt": "用户：你好"}, http.StatusOK
			},
			wantPrompt: "用户：你好",
			wantErr:    false,
		},
		{
			name: "服务器返回422错误",
			messages: []ChatMessage{
				NewTextMessage("user", "test"),
			},
			handler: func(r *http.Request) (interface{}, int) {
				return map[string]string{"error": "template not found"}, http.StatusUnprocessableEntity
			},
			wantErr:     true,
			errContains: "422",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms, client := newMockServerBuilder().
				WithHandler(http.MethodPost, "/apply-template", tt.handler).
				Build(t)
			defer ms.Close()

			prompt, err := client.ApplyTemplate(context.Background(), tt.messages)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望返回错误，但得到 nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("期望错误包含 %q，实际为 %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("不期望错误，但得到: %v", err)
			}
			if prompt != tt.wantPrompt {
				t.Fatalf("期望 prompt=%q，实际 %q", tt.wantPrompt, prompt)
			}

			// 验证请求方法和路径
			if got := ms.RequestCount(http.MethodPost, "/apply-template"); got != 1 {
				t.Fatalf("期望 1 次请求，实际 %d 次", got)
			}

			// 验证请求体包含 messages 字段
			body := ms.LastBodyFor(http.MethodPost, "/apply-template")
			if body == nil {
				t.Fatal("期望收到请求体，但得到 nil")
			}
			var req map[string]interface{}
			if err := json.Unmarshal(body, &req); err != nil {
				t.Fatalf("解析请求体失败: %v", err)
			}
			msgs, ok := req["messages"]
			if !ok {
				t.Fatal("请求体中缺少 messages 字段")
			}
			msgsArr, ok := msgs.([]interface{})
			if !ok {
				t.Fatalf("messages 字段不是数组，实际类型 %T", msgs)
			}
			if len(msgsArr) != len(tt.messages) {
				t.Fatalf("期望 messages 长度 %d，实际 %d", len(tt.messages), len(msgsArr))
			}
		})
	}
}
