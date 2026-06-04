package chat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"douya/internal/chat"
	"douya/internal/llm"
	"douya/internal/search"
	"douya/internal/store"
)

func TestFunctional_BasicQA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("The answer is 42."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content: "What is the answer to life?",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}

func TestFunctional_ChineseQA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("北京是中国的首都。"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content: "中国的首都是哪里？",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}

func TestFunctional_CodeQA(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("```go\nfmt.Println(\"hello\")\n```"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content: "Write a hello world in Go",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}

func TestFunctional_ForcedSearch_InjectsContext(t *testing.T) {
	var receivedMessages []llm.ChatMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("Based on search results..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine:  "testSearch",
			Results: []search.SearchResult{{Title: "Test", URL: "https://example.com", Snippet: "Test snippet"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "search test",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	hasSearchContext := false
	for _, m := range receivedMessages {
		if m.Role == "user" && strings.Contains(m.ContentString(), "[补充信息]") {
			hasSearchContext = true
		}
	}
	if !hasSearchContext {
		t.Error("forced search should inject [补充信息] context")
	}
}

func TestFunctional_ForcedSearch_NoTools(t *testing.T) {
	var receivedTools []llm.ToolDefinition
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedTools = req.Tools

		sseData := makeSSEData([]string{
			makeContentChunk("Answer"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine:  "testSearch",
			Results: []search.SearchResult{{Title: "T", URL: "https://a.com", Snippet: "S"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "search test",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(receivedTools) != 0 {
		t.Errorf("forced search should not provide tools, got %d tools", len(receivedTools))
	}
}

func TestFunctional_AutonomousSearch_ModelDecidesToSearch(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{"query":"latest news"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		} else {
			sseData := makeSSEData([]string{
				makeContentChunk("Here are the latest news..."),
				makeFinishChunk("stop"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		}
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine:  "testSearch",
			Results: []search.SearchResult{{Title: "News", URL: "https://news.com", Snippet: "Latest"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "What's the latest news?",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if callCount < 2 {
		t.Errorf("expected at least 2 LLM calls (tool_call + final), got %d", callCount)
	}
}

func TestFunctional_SearchNoResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("I don't have specific information."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name:    "testSearch",
		results: &search.SearchResponse{Engine: "testSearch", Results: nil},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "obscure query xyz123",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}

func TestFunctional_SingleToolCall(t *testing.T) {
	callCount := 0
	var firstReqTools []llm.ToolDefinition
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"object": "list",
				"data": []map[string]interface{}{
					{
						"id":   "test-strong-model-30b",
						"meta": map[string]interface{}{"n_params": float64(30e9)},
					},
				},
			})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/v1/models/") || r.URL.Path == "/props" {
			json.NewEncoder(w).Encode(map[string]interface{}{})
			return
		}
		callCount++
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)

		if callCount == 1 {
			firstReqTools = req.Tools
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{"query":"test query"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		} else {
			sseData := makeSSEData([]string{
				makeContentChunk("Search result answer"),
				makeFinishChunk("stop"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		}
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine:  "testSearch",
			Results: []search.SearchResult{{Title: "T", URL: "https://a.com", Snippet: "S"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	if err := svc.DetectModelArchitecture(); err != nil {
		t.Fatalf("DetectModelArchitecture failed: %v", err)
	}
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "search for something",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(firstReqTools) == 0 {
		t.Error("first request should provide tools for autonomous search")
	}
}

func TestFunctional_MaxRounds_NoToolsOnLastRound(t *testing.T) {
	callCount := 0
	var lastReqTools []llm.ToolDefinition
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)

		if callCount < 4 {
			sseData := makeSSEData([]string{
				makeToolCallChunk(fmt.Sprintf("call_%d", callCount), "search", `{"query":"test"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		} else {
			lastReqTools = req.Tools
			sseData := makeSSEData([]string{
				makeContentChunk("Final answer"),
				makeFinishChunk("stop"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		}
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine:  "testSearch",
			Results: []search.SearchResult{{Title: "T", URL: "https://a.com", Snippet: "S"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content: "keep searching",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(lastReqTools) != 0 {
		t.Errorf("last round should not provide tools, got %d tools", len(lastReqTools))
	}
}

func TestFunctional_InvalidToolArguments(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `invalid json`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		} else {
			sseData := makeSSEData([]string{
				makeContentChunk("I couldn't search properly."),
				makeFinishChunk("stop"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		}
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine:  "testSearch",
			Results: []search.SearchResult{},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content: "search something",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}

func TestFunctional_ThinkingContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeReasoningChunk("Let me think about this..."),
			makeContentChunk("The answer is 42."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content: "Think about the answer",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}

func TestFunctional_ThinkingBeforeToolCall(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeReasoningChunk("I need to search for this..."),
				makeToolCallChunk("call_1", "search", `{"query":"test"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		} else {
			sseData := makeSSEData([]string{
				makeContentChunk("Based on search..."),
				makeFinishChunk("stop"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		}
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine:  "testSearch",
			Results: []search.SearchResult{{Title: "T", URL: "https://a.com", Snippet: "S"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content: "search and think",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}

func TestFunctional_MultiTurnContext(t *testing.T) {
	var receivedMessages []llm.ChatMessage
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk(fmt.Sprintf("Response %d", callCount)),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{Content: "First question"})
	if err != nil {
		t.Fatalf("First SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc, nil))
	if len(convs) == 0 {
		t.Fatal("expected at least 1 conversation after first message")
	}
	convID := convs[0].ID

	err = svc.SendMessage(context.Background(), chat.SendMessageParams{Content: "Follow-up question", ConversationID: convID})
	if err != nil {
		t.Fatalf("Second SendMessage failed: %v", err)
	}

	userCount := 0
	for _, m := range receivedMessages {
		if m.Role == "user" {
			userCount++
		}
	}
	if userCount < 2 {
		t.Errorf("expected at least 2 user messages in second request, got %d", userCount)
	}
}

func TestFunctional_SpecialCharacters(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("Handled special chars."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	specialInputs := []string{
		"What about <script>alert('xss')</script>?",
		"How about {{template injection}}?",
		"What is ${system.env}?",
		"Tell me about \\n \\t \\r escape sequences",
	}

	for _, input := range specialInputs {
		err := svc.SendMessage(context.Background(), chat.SendMessageParams{Content: input})
		if err != nil {
			t.Errorf("SendMessage failed for special input %q: %v", input, err)
		}
	}
}

func TestFunctional_LongMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("Long message handled."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	longContent := strings.Repeat("This is a test sentence. ", 1000)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{Content: longContent})
	if err != nil {
		t.Fatalf("SendMessage failed for long message: %v", err)
	}
}

func TestFunctional_EmojiInput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("Emoji handled! 🎉"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content: "你好世界 🌍 Hello 🤖",
	})
	if err != nil {
		t.Fatalf("SendMessage failed for emoji input: %v", err)
	}
}

func TestFunctional_SearchI18n_ChineseQuery(t *testing.T) {
	var receivedMessages []llm.ChatMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("补充信息的回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine:  "testSearch",
			Results: []search.SearchResult{{Title: "测试", URL: "https://a.com", Snippet: "摘要"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "今天天气怎么样？",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	hasSearchInLastUserMsg := false
	for _, m := range receivedMessages {
		if m.Role == "user" && strings.Contains(m.ContentString(), "补充信息") {
			hasSearchInLastUserMsg = true
		}
	}
	if !hasSearchInLastUserMsg {
		t.Error("Chinese query should have search results appended to user message")
	}
}

func TestFunctional_SearchI18n_EnglishQuery(t *testing.T) {
	var receivedMessages []llm.ChatMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("Search result answer"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine:  "testSearch",
			Results: []search.SearchResult{{Title: "Test", URL: "https://a.com", Snippet: "Summary"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)
	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "What is the weather today?",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	hasSearchInLastUserMsg := false
	for _, m := range receivedMessages {
		if m.Role == "user" && (strings.Contains(m.ContentString(), "search results") || strings.Contains(m.ContentString(), "supplementary information")) {
			hasSearchInLastUserMsg = true
		}
	}
	if !hasSearchInLastUserMsg {
		t.Error("English query should have search results appended to user message")
	}
}
