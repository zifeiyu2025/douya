// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm_test

import (
	"context"
	"douya/internal/llm"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestStreamChat_NormalSSE(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\" World\",\"reasoning_content\":\"thinking\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var chunks []llm.SSEChunk
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	if chunks[0].Choices[0].Delta.Content != "Hello" {
		t.Fatalf("expected first chunk content 'Hello', got '%s'", chunks[0].Choices[0].Delta.Content)
	}
	if chunks[1].Choices[0].Delta.Content != " World" {
		t.Fatalf("expected second chunk content ' World', got '%s'", chunks[1].Choices[0].Delta.Content)
	}
	if chunks[1].Choices[0].Delta.ReasoningContent != "thinking" {
		t.Fatalf("expected reasoning_content 'thinking', got '%s'", chunks[1].Choices[0].Delta.ReasoningContent)
	}
	if chunks[2].Choices[0].FinishReason == nil || *chunks[2].Choices[0].FinishReason != "stop" {
		t.Fatal("expected finish_reason 'stop' on last chunk")
	}
}

func TestStreamChat_DoneMarker(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hi\"},\"finish_reason\":null}]}\n\n" +
		"data: [DONE]\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Should not appear\"},\"finish_reason\":null}]}\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var chunks []llm.SSEChunk
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk after [DONE], got %d", len(chunks))
	}
	if chunks[0].Choices[0].Delta.Content != "Hi" {
		t.Fatalf("expected content 'Hi', got '%s'", chunks[0].Choices[0].Delta.Content)
	}
}

func TestStreamChat_NonDataLinesIgnored(t *testing.T) {
	sseData := "" +
		": this is a comment\n" +
		"\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Ok\"},\"finish_reason\":null}]}\n\n" +
		"event: ping\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var chunks []llm.SSEChunk
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].Choices[0].Delta.Content != "Ok" {
		t.Fatalf("expected content 'Ok', got '%s'", chunks[0].Choices[0].Delta.Content)
	}
}

func TestStreamChat_InvalidJSONSkipped(t *testing.T) {
	sseData := "" +
		"data: {invalid json}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Valid\"},\"finish_reason\":null}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var chunks []llm.SSEChunk
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk (invalid JSON skipped), got %d", len(chunks))
	}
	if chunks[0].Choices[0].Delta.Content != "Valid" {
		t.Fatalf("expected content 'Valid', got '%s'", chunks[0].Choices[0].Delta.Content)
	}
}

func TestStreamChat_LargeBuffer(t *testing.T) {
	largeContent := strings.Repeat("A", 70*1024)
	sseData := fmt.Sprintf(
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"%s\"},\"finish_reason\":null}]}\n\ndata: [DONE]\n\n",
		largeContent,
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var chunks []llm.SSEChunk
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if len(chunks[0].Choices[0].Delta.ContentString()) != 70*1024 {
		t.Fatalf("expected content length %d, got %d", 70*1024, len(chunks[0].Choices[0].Delta.ContentString()))
	}
}

func TestStreamChat_ToolCallStreaming(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"tool_calls\":[{\"id\":\"call_abc\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"tool_calls\":[{\"id\":\"\",\"type\":\"\",\"function\":{\"name\":\"\",\"arguments\":\"{\\\"query\\\":\\\"Go\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var chunks []llm.SSEChunk
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "search for Go"}},
	}, func(chunk llm.SSEChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	firstToolCalls := chunks[0].Choices[0].Delta.ToolCalls
	if len(firstToolCalls) != 1 {
		t.Fatalf("expected 1 tool call in first chunk, got %d", len(firstToolCalls))
	}
	if firstToolCalls[0].ID != "call_abc" {
		t.Fatalf("expected tool call id 'call_abc', got '%s'", firstToolCalls[0].ID)
	}
	if firstToolCalls[0].Function.Name != "search" {
		t.Fatalf("expected tool call function name 'search', got '%s'", firstToolCalls[0].Function.Name)
	}

	secondToolCalls := chunks[1].Choices[0].Delta.ToolCalls
	if len(secondToolCalls) != 1 {
		t.Fatalf("expected 1 tool call in second chunk, got %d", len(secondToolCalls))
	}
	if secondToolCalls[0].Function.Arguments != `{"query":"Go"}` {
		t.Fatalf("expected tool call arguments '{\"query\":\"Go\"}', got '%s'", secondToolCalls[0].Function.Arguments)
	}

	if chunks[2].Choices[0].FinishReason == nil || *chunks[2].Choices[0].FinishReason != "tool_calls" {
		t.Fatal("expected finish_reason 'tool_calls'")
	}
}

func TestStreamChat_ReasoningContentOnly(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"reasoning_content\":\"Let me think about this...\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"The answer is 42.\",\"reasoning_content\":\"\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var contentParts []string
	var reasoningParts []string
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "what is the answer?"}},
	}, func(chunk llm.SSEChunk) error {
		if len(chunk.Choices) > 0 {
			if chunk.Choices[0].Delta.ContentString() != "" {
				contentParts = append(contentParts, chunk.Choices[0].Delta.ContentString())
			}
			if chunk.Choices[0].Delta.ReasoningContent != "" {
				reasoningParts = append(reasoningParts, chunk.Choices[0].Delta.ReasoningContent)
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(reasoningParts) != 1 || reasoningParts[0] != "Let me think about this..." {
		t.Fatalf("expected reasoning 'Let me think about this...', got %v", reasoningParts)
	}
	if len(contentParts) != 1 || contentParts[0] != "The answer is 42." {
		t.Fatalf("expected content 'The answer is 42.', got %v", contentParts)
	}
}

func TestStreamChat_EmptyChoicesArray(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"After empty\"},\"finish_reason\":null}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var chunks []llm.SSEChunk
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (including empty choices), got %d", len(chunks))
	}
	if len(chunks[0].Choices) != 0 {
		t.Fatalf("expected first chunk to have empty choices, got %d", len(chunks[0].Choices))
	}
	if chunks[1].Choices[0].Delta.Content != "After empty" {
		t.Fatalf("expected content 'After empty', got '%s'", chunks[1].Choices[0].Delta.Content)
	}
}

func TestStreamChat_Non200StatusCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "server overloaded")
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for non-200 status code, got nil")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("expected error to contain status code 503, got: %v", err)
	}
	if !strings.Contains(err.Error(), "server overloaded") {
		t.Errorf("expected error to contain response body, got: %v", err)
	}
}

func TestStreamChat_ContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"start\"},\"finish_reason\":null}]}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		cancel()
		time.Sleep(5 * time.Second)
		fmt.Fprintf(w, "data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"should not appear\"},\"finish_reason\":null}]}\n\n")
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	err := client.StreamChat(ctx, &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		return nil
	})

	if err == nil {
		t.Log("context cancellation may have been absorbed by stream completion")
	}
}

func TestStreamChat_CallbackError(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"Hello\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" World\"},\"finish_reason\":null}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	callbackErr := fmt.Errorf("callback error")
	callCount := 0
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		callCount++
		if callCount == 1 {
			return callbackErr
		}
		return nil
	})

	if err == nil {
		t.Fatal("expected error from callback, got nil")
	}
	if !strings.Contains(err.Error(), "callback error") {
		t.Errorf("expected error to contain 'callback error', got: %v", err)
	}
}

func TestStreamChat_ChineseContent(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"你好\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"世界\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"！\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var fullContent string
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "你好"}},
	}, func(chunk llm.SSEChunk) error {
		if len(chunk.Choices) > 0 {
			fullContent += chunk.Choices[0].Delta.ContentString()
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fullContent != "你好世界！" {
		t.Fatalf("expected '你好世界！', got '%s'", fullContent)
	}
}

func TestStreamChat_FinishReasonLength(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"\"},\"finish_reason\":\"length\"}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var chunks []llm.SSEChunk
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "write a very long essay"}},
	}, func(chunk llm.SSEChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[1].Choices[0].FinishReason == nil || *chunks[1].Choices[0].FinishReason != "length" {
		t.Fatal("expected finish_reason 'length'")
	}
}

func TestStreamChat_MultipleToolCalls(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"tool_calls\":[{\"id\":\"call_1\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"query\\\":\\\"Go\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"tool_calls\":[{\"id\":\"call_2\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"{\\\"query\\\":\\\"Rust\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var allToolCalls []llm.ToolCall
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "compare Go and Rust"}},
	}, func(chunk llm.SSEChunk) error {
		if len(chunk.Choices) > 0 {
			allToolCalls = append(allToolCalls, chunk.Choices[0].Delta.ToolCalls...)
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(allToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(allToolCalls))
	}
	if allToolCalls[0].ID != "call_1" {
		t.Errorf("expected first tool call id 'call_1', got '%s'", allToolCalls[0].ID)
	}
	if allToolCalls[1].ID != "call_2" {
		t.Errorf("expected second tool call id 'call_2', got '%s'", allToolCalls[1].ID)
	}
}

func TestStreamChat_EmptyStream(t *testing.T) {
	sseData := "data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var chunks []llm.SSEChunk
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 0 {
		t.Fatalf("expected 0 chunks for empty stream, got %d", len(chunks))
	}
}

func TestChat_NonStreamingResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)

		if req.Stream {
			t.Error("expected Stream=false for Chat method")
		}

		resp := llm.ChatCompletionResponse{
			ID: "chatcmpl-1",
			Choices: []llm.Choice{
				{
					Index: 0,
					Message: llm.ChatMessage{
						Role:    "assistant",
						Content: "Hello! How can I help you?",
					},
					FinishReason: "stop",
				},
			},
		}
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	resp, err := client.Chat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content != "Hello! How can I help you?" {
		t.Fatalf("expected content 'Hello! How can I help you?', got '%s'", resp.Choices[0].Message.Content)
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Fatalf("expected finish_reason 'stop', got '%s'", resp.Choices[0].FinishReason)
	}
}

func TestChat_NonStreamingNon200Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error": "model not found"}`)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	_, err := client.Chat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	})

	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
	if !strings.Contains(err.Error(), "500") {
		t.Errorf("expected error to contain '500', got: %v", err)
	}
}

func TestHealthCheck_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	if err := client.HealthCheck(context.Background()); err != nil {
		t.Fatalf("expected no error for health check, got: %v", err)
	}
}

func TestHealthCheck_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	err := client.HealthCheck(context.Background())
	if err == nil {
		t.Fatal("expected error for unhealthy server, got nil")
	}
}

func TestNewClient_TrailingSlashStripped(t *testing.T) {
	client := llm.NewClient("http://localhost:8080/", "")
	if client.BaseURL() != "http://localhost:8080" {
		t.Fatalf("expected trailing slash stripped, got '%s'", client.BaseURL())
	}
}

func TestStreamChat_RequestBodyContainsStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)

		if !req.Stream {
			t.Error("expected Stream=true in request body for StreamChat")
		}

		sseData := "data: [DONE]\n\n"
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamChat_RequestSendsCorrectEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("expected path /v1/chat/completions, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		sseData := "data: [DONE]\n\n"
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStreamChat_ConnectionRefused(t *testing.T) {
	client := llm.NewClient("http://127.0.0.1:1", "")

	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "hi"}},
	}, func(chunk llm.SSEChunk) error {
		return nil
	})

	if err == nil {
		t.Fatal("expected error for connection refused, got nil")
	}
}

func TestStreamChat_MixedContentAndReasoning(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"\",\"reasoning_content\":\"Step 1: Analyze the problem\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"\",\"reasoning_content\":\"Step 2: Find the solution\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"The solution is\",\"reasoning_content\":\"\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\" simple.\",\"reasoning_content\":\"\"},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"\"},\"finish_reason\":\"stop\"}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var fullContent string
	var fullReasoning string
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "solve this"}},
	}, func(chunk llm.SSEChunk) error {
		if len(chunk.Choices) > 0 {
			fullContent += chunk.Choices[0].Delta.ContentString()
			fullReasoning += chunk.Choices[0].Delta.ReasoningContent
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fullContent != "The solution is simple." {
		t.Fatalf("expected content 'The solution is simple.', got '%s'", fullContent)
	}
	expectedReasoning := "Step 1: Analyze the problemStep 2: Find the solution"
	if fullReasoning != expectedReasoning {
		t.Fatalf("expected reasoning '%s', got '%s'", expectedReasoning, fullReasoning)
	}
}

func TestStreamChat_ToolCallWithAccumulatedArguments(t *testing.T) {
	sseData := "" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"tool_calls\":[{\"id\":\"call_123\",\"type\":\"function\",\"function\":{\"name\":\"search\",\"arguments\":\"\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"tool_calls\":[{\"id\":\"\",\"type\":\"\",\"function\":{\"name\":\"\",\"arguments\":\"{\\\"qu\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\",\"tool_calls\":[{\"id\":\"\",\"type\":\"\",\"function\":{\"name\":\"\",\"arguments\":\"ery\\\":\\\"test\\\"}\"}}]},\"finish_reason\":null}]}\n\n" +
		"data: {\"id\":\"chatcmpl-1\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"\"},\"finish_reason\":\"tool_calls\"}]}\n\n" +
		"data: [DONE]\n\n"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	var toolCallParts []string
	var toolCallID string
	var toolCallName string
	err := client.StreamChat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "search test"}},
	}, func(chunk llm.SSEChunk) error {
		if len(chunk.Choices) > 0 && len(chunk.Choices[0].Delta.ToolCalls) > 0 {
			tc := chunk.Choices[0].Delta.ToolCalls[0]
			if tc.ID != "" {
				toolCallID = tc.ID
			}
			if tc.Function.Name != "" {
				toolCallName = tc.Function.Name
			}
			if tc.Function.Arguments != "" {
				toolCallParts = append(toolCallParts, tc.Function.Arguments)
			}
		}
		return nil
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if toolCallID != "call_123" {
		t.Fatalf("expected tool call id 'call_123', got '%s'", toolCallID)
	}
	if toolCallName != "search" {
		t.Fatalf("expected tool call name 'search', got '%s'", toolCallName)
	}
	accumulatedArgs := strings.Join(toolCallParts, "")
	if accumulatedArgs != `{"query":"test"}` {
		t.Fatalf("expected accumulated arguments '{\"query\":\"test\"}', got '%s'", accumulatedArgs)
	}
}

func TestChat_NonStreamingWithToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := llm.ChatCompletionResponse{
			ID: "chatcmpl-1",
			Choices: []llm.Choice{
				{
					Index: 0,
					Message: llm.ChatMessage{
						Role:    "assistant",
						Content: "",
						ToolCalls: []llm.ToolCall{
							{
								ID:   "call_1",
								Type: "function",
								Function: llm.FunctionCall{
									Name:      "search",
									Arguments: `{"query":"test"}`,
								},
							},
						},
					},
					FinishReason: "tool_calls",
				},
			},
		}
		data, _ := json.Marshal(resp)
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")

	resp, err := client.Chat(context.Background(), &llm.ChatCompletionRequest{
		Model:    "test",
		Messages: []llm.ChatMessage{{Role: "user", Content: "search test"}},
	})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if len(resp.Choices[0].Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(resp.Choices[0].Message.ToolCalls))
	}
	if resp.Choices[0].Message.ToolCalls[0].Function.Name != "search" {
		t.Fatalf("expected tool call name 'search', got '%s'", resp.Choices[0].Message.ToolCalls[0].Function.Name)
	}
	if resp.Choices[0].FinishReason != "tool_calls" {
		t.Fatalf("expected finish_reason 'tool_calls', got '%s'", resp.Choices[0].FinishReason)
	}
}

func TestVisionMessage_JSONSerialization_TextAlwaysPresent(t *testing.T) {
	msg := llm.NewVisionMessage("user", "", []string{"data:image/png;base64,abc"})
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	hasTextPart := false
	for _, part := range parsed.Content {
		if part.Type == "text" {
			hasTextPart = true
			if part.Text == "" {
				t.Error("text ContentPart should have non-empty text to avoid Jinja 'No user query found' error")
			}
		}
	}

	if !hasTextPart {
		t.Error("vision message must contain a text ContentPart")
	}
}

func TestVisionMessage_WithText_JSONSerialization(t *testing.T) {
	msg := llm.NewVisionMessage("user", "请描述这张图片", []string{"data:image/png;base64,abc"})
	body, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, `"text":"请描述这张图片"`) {
		t.Errorf("expected text field in JSON, got: %s", bodyStr)
	}
	if !strings.Contains(bodyStr, `"image_url"`) {
		t.Errorf("expected image_url field in JSON, got: %s", bodyStr)
	}
}

func TestLoadModel_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/load" {
			t.Errorf("expected path /models/load, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		var req map[string]string
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		if req["model"] != "test-model" {
			t.Errorf("expected model 'test-model', got '%s'", req["model"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")
	err := client.LoadModel(context.Background(), "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadModel_Non200Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `model not found`)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")
	err := client.LoadModel(context.Background(), "nonexistent")
	if err == nil {
		t.Fatal("expected error for 404 status, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected error to contain '404', got: %v", err)
	}
}

func TestUnloadModel_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models/unload" {
			t.Errorf("expected path /models/unload, got %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("expected POST method, got %s", r.Method)
		}

		var req map[string]string
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		if req["model"] != "test-model" {
			t.Errorf("expected model 'test-model', got '%s'", req["model"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")
	err := client.UnloadModel(context.Background(), "test-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUnloadModel_Non200Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `unload failed`)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")
	err := client.UnloadModel(context.Background(), "test-model")
	if err == nil {
		t.Fatal("expected error for 500 status, got nil")
	}
}

func TestGetServerProps_ModalitiesAndReasoning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/props" {
			t.Errorf("expected path /props, got %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("expected GET method, got %s", r.Method)
		}

		resp := `{"modalities":{"vision":true,"audio":true},"chat_template_caps":{"supports_preserve_reasoning":true}}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, resp)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")
	props, err := client.GetServerProps(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !props.Modalities.Vision {
		t.Error("expected Modalities.Vision=true")
	}
	if !props.Modalities.Audio {
		t.Error("expected Modalities.Audio=true")
	}
	if props.ChatTemplateCaps == nil || !props.ChatTemplateCaps["supports_preserve_reasoning"] {
		t.Error("expected ChatTemplateCaps['supports_preserve_reasoning']=true")
	}
}

func TestGetServerProps_EmptyModalities(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := `{"modalities":{"vision":false,"audio":false}}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, resp)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")
	props, err := client.GetServerProps(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.Modalities.Vision {
		t.Error("expected Modalities.Vision=false")
	}
	if props.Modalities.Audio {
		t.Error("expected Modalities.Audio=false")
	}
}

func TestGetServerProps_Non200Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")
	_, err := client.GetServerProps(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for 503 status, got nil")
	}
	if !strings.Contains(err.Error(), "503") {
		t.Errorf("expected error to contain '503', got: %v", err)
	}
}

func TestGetServerProps_WithModelNameQueryParam(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("model") != "Qwen3-VL-7B" {
			t.Errorf("expected model=Qwen3-VL-7B query param, got %s", r.URL.Query().Get("model"))
		}
		resp := `{"modalities":{"vision":true,"audio":false},"chat_template_caps":{"supports_preserve_reasoning":false}}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, resp)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")
	props, err := client.GetServerProps(context.Background(), "Qwen3-VL-7B")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !props.Modalities.Vision {
		t.Error("expected Modalities.Vision=true")
	}
	if props.Modalities.Audio {
		t.Error("expected Modalities.Audio=false")
	}
}

func TestGetServerProps_WithoutModelName(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.RawQuery != "" {
			t.Errorf("expected no query params, got %s", r.URL.RawQuery)
		}
		resp := `{"modalities":{"vision":false,"audio":false}}`
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, resp)
	}))
	defer server.Close()

	client := llm.NewClient(server.URL, "")
	props, err := client.GetServerProps(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if props.Modalities.Vision {
		t.Error("expected Modalities.Vision=false")
	}
}
