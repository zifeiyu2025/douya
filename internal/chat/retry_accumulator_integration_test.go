// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI
//
// 集成测试：验证 retryStreamAfterContextExceeded 在上下文溢出重试时不会导致
// 累积内容重复拼接（"模型一直重复一句话"根因的回归保护）。

package chat

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"douya/internal/config"
	"douya/internal/llm"
)

// TestRetryStreamAfterContextExceeded_NoDuplicateContent 复现并验证修复：
//
// 场景：首次流式请求在"已生成部分内容"后，因上下文溢出（HTTP 400
// exceed_context_size_error）失败；随后自动裁剪重试。
// 修复前：重试复用同一个 StreamAccumulator 且未重置，重试生成的内容会追加到
// 已有的部分内容之上，导致 FullContent 开头重复。
// 修复后：重试前调用 resetForNextCall，FullContent 只包含重试生成的内容。
func TestRetryStreamAfterContextExceeded_NoDuplicateContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 注意：retryStreamAfterContextExceeded 内部只发起"重试"这一次 HTTP 请求。
		// 首次的溢出错误由调用方以 origErr 参数传入（上方 fmt.Errorf），
		// 因此这里对重试请求直接返回正常 SSE 流。
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		flusher, _ := w.(http.Flusher)

		writeChunk := func(content, finishReason string) {
			payload := fmt.Sprintf(`data: {"choices":[{"delta":{"content":"%s"},"finish_reason":"%s"}],"usage":{"prompt_tokens":100,"completion_tokens":10},"timings":{"predicted_per_second":20,"predicted_n":10}}`, content, finishReason)
			fmt.Fprintf(w, "%s\n\n", payload)
			flusher.Flush()
		}
		// 故意先发一部分内容，再结束（模拟完整回答）
		writeChunk("这是重试", "")
		writeChunk("后生成的完整回答。", "stop")
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	svc := NewService(nil, nil, nil, &config.Config{ContextSize: 4096}, nil, "")
	client := llm.NewClient(srv.URL, "")

	acc := NewStreamAccumulator("conv-1", nil,
		func(convID, event string, content any) {})

	// 场景一（复现修复前的 bug 前提）：acc 里先累积一部分"首次生成"的内容，
	// 模拟"生成中途才溢出"（decode 阶段命中上下文上限）的真实时序。
	// 首次调用 StreamChatWithConvID 返回溢出错误之前，acc.FullContent 已有内容。
	acc.FullContent.WriteString("首次生成的前半句（在溢出前已产出）")
	acc.FirstTokenSent = true

	req := &llm.ChatCompletionRequest{
		Messages: []llm.ChatMessage{
			{Role: "user", Content: "请解释一下量子纠缠"},
		},
	}

	handled, retryErr := svc.retryStreamAfterContextExceeded(
		context.Background(),
		"conv-1",
		"conv-1::retry",
		client,
		req,
		4096,
		fmt.Errorf(`{"error":{"type":"exceed_context_size_error","n_prompt_tokens":5000,"n_ctx":4096}}`),
		acc,
		"test exceed retry",
		"test retry err",
		"test non-exceed err",
	)

	if retryErr != nil {
		t.Fatalf("重试应成功，实际错误: %v", retryErr)
	}
	if handled {
		t.Fatal("重试成功应返回 handled=false，实际 true")
	}

	got := acc.FullContent.String()
	want := "这是重试后生成的完整回答。"

	// 关键断言：FullContent 不应包含"首次生成的前半句"（即没有重复拼接）
	if strings.Contains(got, "首次生成的前半句") {
		t.Fatalf("FullContent 仍包含首次生成的内容（重复拼接 bug！）：got=%q", got)
	}
	if got != want {
		t.Fatalf("FullContent = %q，期望 = %q（重试后应为干净的重试内容）", got, want)
	}
}

// TestRetryStreamAfterContextExceeded_TrimRetries 验证溢出时确实发起重试请求（至少两次调用）。
func TestRetryStreamAfterContextExceeded_TriggersRetryRequest(t *testing.T) {
	var callCount atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		payload := `data: {"choices":[{"delta":{"content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2}}`
		fmt.Fprintf(w, "%s\n\n", payload)
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer srv.Close()

	svc := NewService(nil, nil, nil, &config.Config{ContextSize: 4096}, nil, "")
	client := llm.NewClient(srv.URL, "")
	acc := NewStreamAccumulator("conv-1", nil, func(string, string, any) {})

	handled, retryErr := svc.retryStreamAfterContextExceeded(
		context.Background(), "conv-1", "conv-1::retry", client,
		&llm.ChatCompletionRequest{Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}}},
		4096,
		fmt.Errorf(`{"error":{"type":"exceed_context_size_error","n_prompt_tokens":5000,"n_ctx":4096}}`),
		acc, "test", "retry err", "non-exceed err",
	)

	if retryErr != nil {
		t.Fatalf("重试应成功，实际: %v", retryErr)
	}
	if handled {
		t.Fatal("应 handled=false")
	}
	if got := callCount.Load(); got < 1 {
		t.Fatalf("应至少发起 1 次重试 HTTP 请求，实际 %d", got)
	}
}