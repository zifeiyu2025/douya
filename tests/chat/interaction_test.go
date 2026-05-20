// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/search"
	"douya/internal/store"
)

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

type mockSearchProvider struct {
	name    string
	results *search.SearchResponse
	err     error
	calls   []string
}

func (m *mockSearchProvider) Name() string { return m.name }

func (m *mockSearchProvider) Search(ctx context.Context, query string) (*search.SearchResponse, error) {
	m.calls = append(m.calls, query)
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func (m *mockSearchProvider) SearchWithOpts(ctx context.Context, query string, opts search.SearchOpts) (*search.SearchResponse, error) {
	return m.Search(ctx, query)
}

func newInteractionTestService(t *testing.T, llmServer *httptest.Server, searchProvider search.SearchProvider) *chat.Service {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Init(dbPath)
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	cfg := &config.Config{
		ContextSize:  8192,
		SystemPrompt: "",
		Temperature:  0.7,
	}

	llmClient := llm.NewClient(llmServer.URL)

	var chain *search.SearchChain
	if searchProvider != nil {
		chain = search.NewSearchChain(searchProvider)
	} else {
		chain = search.NewSearchChain()
	}

	return chat.NewService(llmClient, chain, db, cfg)
}

func makeSSEData(chunks []string) string {
	var b strings.Builder
	for _, c := range chunks {
		b.WriteString("data: ")
		b.WriteString(c)
		b.WriteString("\n\n")
	}
	b.WriteString("data: [DONE]\n\n")
	return b.String()
}

func makeContentChunk(content string) string {
	chunk := llm.SSEChunk{
		ID: "chatcmpl-test",
		Choices: []llm.SSEChoice{{
			Index: 0,
			Delta: llm.ChatMessage{Content: content},
		}},
	}
	data, _ := json.Marshal(chunk)
	return string(data)
}

func makeReasoningChunk(reasoning string) string {
	chunk := llm.SSEChunk{
		ID: "chatcmpl-test",
		Choices: []llm.SSEChoice{{
			Index: 0,
			Delta: llm.ChatMessage{ReasoningContent: reasoning},
		}},
	}
	data, _ := json.Marshal(chunk)
	return string(data)
}

func makeToolCallChunk(id, fnName, args string) string {
	chunk := llm.SSEChunk{
		ID: "chatcmpl-test",
		Choices: []llm.SSEChoice{{
			Index: 0,
			Delta: llm.ChatMessage{
				ToolCalls: []llm.ToolCall{{
					ID:   id,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      fnName,
						Arguments: args,
					},
				}},
			},
		}},
	}
	data, _ := json.Marshal(chunk)
	return string(data)
}

func makeFinishChunk(reason string) string {
	chunk := llm.SSEChunk{
		ID: "chatcmpl-test",
		Choices: []llm.SSEChoice{{
			Index:        0,
			Delta:        llm.ChatMessage{},
			FinishReason: &reason,
		}},
	}
	data, _ := json.Marshal(chunk)
	return string(data)
}

func TestSendMessage_ModelAutonomousThinking(t *testing.T) {
	sseData := makeSSEData([]string{
		makeReasoningChunk("用户在询问Go语言的并发模型..."),
		makeReasoningChunk("我需要解释goroutine和channel..."),
		makeContentChunk("Go语言使用goroutine实现并发，"),
		makeContentChunk("通过channel进行通信。"),
		makeFinishChunk("stop"),
	})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:        "Go语言如何实现并发？",
		SearchEnabled:  true,
		ConversationID: "",
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	if len(convs) == 0 {
		t.Fatal("expected at least 1 conversation")
	}

	assistantMsgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)

	for _, m := range assistantMsgs {
		if m.Role == "assistant" {
			if m.ThinkingContent == "" {
				t.Error("assistant message should have ThinkingContent when model outputs reasoning")
			}
			if !strings.Contains(m.ThinkingContent, "goroutine") {
				t.Errorf("ThinkingContent should contain 'goroutine', got: %s", m.ThinkingContent)
			}
			if !strings.Contains(m.Content, "goroutine") {
				t.Errorf("Content should contain 'goroutine', got: %s", m.Content)
			}
			return
		}
	}
	t.Error("expected assistant message with thinking content")
}

func TestSendMessage_ModelAutonomousToolCallSearch(t *testing.T) {
	firstCallCount := 0
	secondCallCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)

		firstCallCount++

		if firstCallCount == 1 {
			sseData := makeSSEData([]string{
				makeReasoningChunk("这个问题需要最新的信息，我应该搜索一下..."),
				makeToolCallChunk("call_search_1", "search", `{"query":"2026年最新AI技术趋势"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		secondCallCount++
		hasToolMsg := false
		for _, m := range req.Messages {
			if m.Role == "tool" {
				hasToolMsg = true
				if !strings.Contains(m.ContentString(), "内容:") {
					t.Errorf("tool message should contain search result format '内容:', got: %s", m.ContentString())
				}
				if !strings.Contains(m.ContentString(), "AI技术突破") {
					t.Errorf("tool message should contain search result 'AI技术突破', got: %s", m.ContentString())
				}
			}
			if m.Role == "assistant" && len(m.ToolCalls) > 0 {
				if m.ToolCalls[0].Function.Name != "search" {
					t.Errorf("assistant tool_call should have name 'search', got '%s'", m.ToolCalls[0].Function.Name)
				}
			}
		}
		if !hasToolMsg {
			t.Error("second LLM call should include tool message with search results")
		}

		sseData := makeSSEData([]string{
			makeContentChunk("2026年AI技术有以下趋势："),
			makeContentChunk("1. 多模态大模型持续发展"),
			makeContentChunk("2. AI Agent自主能力增强"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine: "testSearch",
			Results: []search.SearchResult{
				{Title: "AI技术突破", URL: "https://example.com/ai-2026", Snippet: "2026年AI技术最新突破..."},
				{Title: "AI趋势报告", URL: "https://example.com/trends", Snippet: "多模态和Agent是主要趋势..."},
			},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "2026年最新的AI技术趋势是什么？",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if firstCallCount != 2 {
		t.Errorf("expected 2 LLM calls (initial + after search), got %d", firstCallCount)
	}
	if secondCallCount != 1 {
		t.Errorf("expected 1 second LLM call, got %d", secondCallCount)
	}

	if len(searchProvider.calls) != 1 {
		t.Fatalf("expected 1 search call, got %d", len(searchProvider.calls))
	}
	if searchProvider.calls[0] != `2026年最新AI技术趋势` {
		t.Errorf("expected search query '2026年最新AI技术趋势', got '%s'", searchProvider.calls[0])
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	if len(convs) == 0 {
		t.Fatal("expected at least 1 conversation")
	}

	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)
	var assistantMsg *store.Message
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistantMsg = m
		}
	}

	if assistantMsg == nil {
		t.Fatal("expected assistant message")
	}
	if !strings.Contains(assistantMsg.Content, "2026年AI技术") {
		t.Errorf("assistant content should contain '2026年AI技术', got: %s", assistantMsg.Content)
	}
	if assistantMsg.SearchResults == "" {
		t.Error("assistant message should have SearchResults when search was performed")
	}
	if !strings.Contains(assistantMsg.SearchResults, "AI技术突破") {
		t.Errorf("SearchResults should contain 'AI技术突破', got: %s", assistantMsg.SearchResults)
	}
	if assistantMsg.ThinkingContent == "" {
		t.Error("assistant message should have ThinkingContent from reasoning")
	}
}

func TestSendMessage_SearchEnabled_UpfrontSearch(t *testing.T) {
	var receivedMessages []llm.ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("Go 1.24新增了以下特性..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine: "testSearch",
			Results: []search.SearchResult{
				{Title: "Go 1.24 Release", URL: "https://go.dev/doc/go1.24", Snippet: "Go 1.24新增泛型类型别名..."},
			},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "Go 1.24有什么新特性？",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(searchProvider.calls) != 1 {
		t.Fatalf("expected 1 search call, got %d", len(searchProvider.calls))
	}

	var searchContextMsg *llm.ChatMessage
	var originalUserMsg *llm.ChatMessage
	for i := range receivedMessages {
		if receivedMessages[i].Role == "user" {
			content := receivedMessages[i].ContentString()
			if strings.Contains(content, "[补充信息]") {
				searchContextMsg = &receivedMessages[i]
			} else {
				originalUserMsg = &receivedMessages[i]
			}
		}
	}

	if searchContextMsg == nil {
		t.Fatal("when SearchEnabled=true, search results should be injected as a context message containing [补充信息]")
	}
	if !strings.Contains(searchContextMsg.ContentString(), "Go 1.24 Release") {
		t.Errorf("search context should contain 'Go 1.24 Release', got: %s", searchContextMsg.ContentString())
	}
	if !strings.Contains(searchContextMsg.ContentString(), "内容:") {
		t.Errorf("search context should contain '内容:', got: %s", searchContextMsg.ContentString())
	}

	if originalUserMsg != nil && strings.Contains(originalUserMsg.ContentString(), "内容:") {
		t.Error("original user message should NOT contain search results")
	}
	if originalUserMsg != nil && !strings.Contains(originalUserMsg.ContentString(), "Go 1.24有什么新特性？") {
		t.Errorf("original user message should contain original query, got: %s", originalUserMsg.ContentString())
	}
}

func TestSendMessage_SearchEnabledFalse_ProvidesSearchTool(t *testing.T) {
	var receivedReq *llm.ChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedReq = &req

		sseData := makeSSEData([]string{
			makeContentChunk("直接回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "你好",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(receivedReq.Tools) != 1 {
		t.Fatalf("expected 1 tool definition when SearchEnabled=false, got %d", len(receivedReq.Tools))
	}
	if receivedReq.Tools[0].Function.Name != "search" {
		t.Errorf("expected tool name 'search', got '%s'", receivedReq.Tools[0].Function.Name)
	}
	if receivedReq.Tools[0].Type != "function" {
		t.Errorf("expected tool type 'function', got '%s'", receivedReq.Tools[0].Type)
	}
}

func TestSendMessage_SearchEnabledTrue_WithTool(t *testing.T) {
	var receivedReq *llm.ChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedReq = &req

		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
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
			Results: []search.SearchResult{},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "你好",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(receivedReq.Tools) != 0 {
		t.Errorf("expected 0 tools when SearchEnabled=true (search already done), got %d", len(receivedReq.Tools))
	}
}

func TestSendMessage_ToolCallSearch_EmptyResults(t *testing.T) {
	callCount := 0
	var secondCallMessages []llm.ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{"query":"冷门话题"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		secondCallMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("根据我的知识，关于这个话题..."),
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
			Results: []search.SearchResult{},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "冷门话题",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (initial + after empty search), got %d", callCount)
	}

	var toolMsg *llm.ChatMessage
	for i := range secondCallMessages {
		if secondCallMessages[i].Role == "tool" {
			toolMsg = &secondCallMessages[i]
		}
	}
	if toolMsg == nil {
		t.Fatal("expected tool message in second LLM call")
	}
	if !strings.Contains(toolMsg.ContentString(), "No search results found") {
		t.Errorf("tool message should indicate no results, got: %s", toolMsg.ContentString())
	}
	if !strings.Contains(toolMsg.ContentString(), "use your own knowledge") {
		t.Errorf("tool message should instruct model to use its own knowledge, got: %s", toolMsg.ContentString())
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)

	var assistantMsg *store.Message
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistantMsg = m
		}
	}

	if assistantMsg == nil {
		t.Fatal("expected assistant message")
	}
	if assistantMsg.Content == "" {
		t.Error("expected non-empty content when search returns empty results (model should use its own knowledge)")
	}
}

func TestSendMessage_ToolCallSearch_InvalidArguments(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{invalid json}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		sseData := makeSSEData([]string{
			makeContentChunk("让我重新搜索..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "测试无效参数",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage should not fail with invalid tool call args, got: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (model gets error feedback and can retry), got %d", callCount)
	}
}

func TestSendMessage_Timeliness_SystemPromptContainsDate(t *testing.T) {
	var receivedMessages []llm.ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("今天是"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "今天几号？",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(receivedMessages) == 0 {
		t.Fatal("expected at least 1 message")
	}

	systemMsg := receivedMessages[0]
	if systemMsg.Role != "system" {
		t.Fatalf("expected first message to be system, got '%s'", systemMsg.Role)
	}

	now := time.Now()
	dateStr := now.Format("2006-01-02")
	if !strings.Contains(systemMsg.ContentString(), dateStr) {
		t.Errorf("system prompt should contain current date '%s', got: %s", dateStr, systemMsg.ContentString())
	}

	timeStr := now.Format("15:04:05")
	if !strings.Contains(systemMsg.ContentString(), timeStr) {
		t.Errorf("system prompt should contain current time '%s', got: %s", timeStr, systemMsg.ContentString())
	}

	weekdayMap := map[string]string{
		"Sunday": "星期日", "Monday": "星期一", "Tuesday": "星期二",
		"Wednesday": "星期三", "Thursday": "星期四", "Friday": "星期五", "Saturday": "星期六",
	}
	expectedWeekday := weekdayMap[now.Weekday().String()]
	if !strings.Contains(systemMsg.ContentString(), expectedWeekday) {
		t.Errorf("system prompt should contain weekday '%s', got: %s", expectedWeekday, systemMsg.ContentString())
	}

	if !strings.Contains(systemMsg.ContentString(), "search工具") {
		t.Errorf("system prompt should contain search tool guidance, got: %s", systemMsg.ContentString())
	}
}

func TestSendMessage_CodeRelatedSearch_UsesCodeCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	codeProvider := &mockSearchProvider{
		name: "codeSearch",
		results: &search.SearchResponse{
			Engine:  "codeSearch",
			Results: []search.SearchResult{{Title: "Go code", URL: "http://go", Snippet: "Go code example"}},
		},
	}

	chain := search.NewCategorizedSearchChain([]search.CategorizedProvider{
		{Provider: codeProvider, Categories: []string{"code"}},
	})

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, _ := store.Init(dbPath)
	t.Cleanup(func() { db.Close() })

	cfg := &config.Config{ContextSize: 4096, Temperature: 0.7}
	llmClient := llm.NewClient(server.URL)
	svc := chat.NewService(llmClient, chain, db, cfg)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "如何用python写一个web服务器？",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(codeProvider.calls) != 1 {
		t.Fatalf("expected 1 search call, got %d", len(codeProvider.calls))
	}
}

func TestSendMessage_GeneralQuestion_UsesGeneralCategory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	generalProvider := &mockSearchProvider{
		name: "generalSearch",
		results: &search.SearchResponse{
			Engine:  "generalSearch",
			Results: []search.SearchResult{{Title: "天气", URL: "http://weather", Snippet: "今天天气"}},
		},
	}

	chain := search.NewCategorizedSearchChain([]search.CategorizedProvider{
		{Provider: generalProvider, Categories: []string{"general"}},
	})

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, _ := store.Init(dbPath)
	t.Cleanup(func() { db.Close() })

	cfg := &config.Config{ContextSize: 4096, Temperature: 0.7}
	llmClient := llm.NewClient(server.URL)
	svc := chat.NewService(llmClient, chain, db, cfg)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "今天北京天气怎么样？",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(generalProvider.calls) != 1 {
		t.Fatalf("expected 1 search call, got %d", len(generalProvider.calls))
	}
}

func TestSendMessage_ConversationTitleAutoGenerated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "这是一个关于Go语言并发编程的问题",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	if len(convs) == 0 {
		t.Fatal("expected at least 1 conversation")
	}

	title := convs[0].Title
	if title == "新对话" {
		t.Error("conversation title should be auto-generated from first message, still '新对话'")
	}
	if !strings.Contains(title, "Go语言并发编程") {
		t.Errorf("title should contain part of the first message, got: %s", title)
	}
}

func TestSendMessage_LongTitleTruncated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	longMessage := strings.Repeat("这是一段很长的消息内容用于测试标题截断功能", 5)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       longMessage,
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	if len(convs) == 0 {
		t.Fatal("expected at least 1 conversation")
	}

	title := convs[0].Title
	runeTitle := []rune(title)
	if len(runeTitle) > 23 {
		t.Errorf("title should be truncated to ~20 chars + '...', got %d chars: %s", len(runeTitle), title)
	}
	if strings.HasSuffix(title, "...") && len([]rune(strings.TrimSuffix(title, "..."))) != 20 {
		t.Errorf("truncated title should have 20 chars before '...', got: %s", title)
	}
}

func TestSendMessage_MultiRoundConversation(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)

		if callCount == 1 {
			if len(req.Messages) < 2 {
				t.Errorf("first call should have system + user message, got %d", len(req.Messages))
			}
			sseData := makeSSEData([]string{
				makeContentChunk("Go语言由Google开发。"),
				makeFinishChunk("stop"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		if callCount == 2 {
			hasHistory := false
			for _, m := range req.Messages {
				if m.Role == "assistant" && strings.Contains(m.ContentString(), "Google") {
					hasHistory = true
				}
			}
			if !hasHistory {
				t.Error("second call should include previous assistant response in history")
			}

			sseData := makeSSEData([]string{
				makeContentChunk("Go语言的主要特点包括：1.并发支持 2.编译速度快 3.语法简洁"),
				makeFinishChunk("stop"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
		}
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	var convID string

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "Go语言是谁开发的？",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("first SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	if len(convs) > 0 {
		convID = convs[0].ID
	}

	err = svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:        "它有什么特点？",
		SearchEnabled:  false,
		ConversationID: convID,
	})
	if err != nil {
		t.Fatalf("second SendMessage failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 LLM calls for 2 messages, got %d", callCount)
	}

	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convID)
	userCount := 0
	assistantCount := 0
	for _, m := range msgs {
		if m.Role == "user" {
			userCount++
		}
		if m.Role == "assistant" {
			assistantCount++
		}
	}
	if userCount != 2 {
		t.Errorf("expected 2 user messages, got %d", userCount)
	}
	if assistantCount != 2 {
		t.Errorf("expected 2 assistant messages, got %d", assistantCount)
	}
}

func TestSendMessage_SearchResultFormat(t *testing.T) {
	var receivedMessages []llm.ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine: "testSearch",
			Results: []search.SearchResult{
				{Title: "结果1", URL: "https://example.com/1", Snippet: "摘要1"},
				{Title: "结果2", URL: "https://example.com/2", Snippet: "摘要2"},
			},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "搜索测试",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	lastMsg := receivedMessages[len(receivedMessages)-1]

	var searchContext string
	for _, m := range receivedMessages {
		if m.Role == "user" && strings.Contains(m.ContentString(), "[补充信息]") {
			searchContext = m.ContentString()
		}
	}

	if searchContext == "" {
		t.Fatal("expected search context in a user message containing [补充信息]")
	}

	if !strings.Contains(searchContext, "内容:") {
		t.Errorf("search context should contain '内容:', got: %s", searchContext)
	}
	if !strings.Contains(searchContext, "结果1") || !strings.Contains(searchContext, "结果2") {
		t.Errorf("search context should contain both results, got: %s", searchContext)
	}
	if !strings.Contains(searchContext, "摘要1") {
		t.Errorf("search context should contain snippet, got: %s", searchContext)
	}
	if !strings.Contains(searchContext, "摘要2") {
		t.Errorf("search context should contain all results, got: %s", searchContext)
	}

	_ = lastMsg
}

func TestSendMessage_ToolCallSearchResultFormat(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)

		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{"query":"测试"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		var toolMsg *llm.ChatMessage
		var assistantToolMsg *llm.ChatMessage
		for i := range req.Messages {
			if req.Messages[i].Role == "tool" {
				toolMsg = &req.Messages[i]
			}
			if req.Messages[i].Role == "assistant" && len(req.Messages[i].ToolCalls) > 0 {
				assistantToolMsg = &req.Messages[i]
			}
		}

		if toolMsg == nil {
			t.Error("second call should include tool message")
		} else {
			if toolMsg.ToolCallID != "call_1" {
				t.Errorf("tool message ToolCallID should be 'call_1', got '%s'", toolMsg.ToolCallID)
			}
			if !strings.Contains(toolMsg.ContentString(), "内容:") {
				t.Errorf("tool message should contain search result format '内容:', got: %s", toolMsg.ContentString())
			}
			if !strings.Contains(toolMsg.ContentString(), "https://example.com") {
				t.Errorf("tool message should contain 'https://example.com', got: %s", toolMsg.ContentString())
			}
			if !strings.Contains(toolMsg.ContentString(), "搜索摘要") {
				t.Errorf("tool message should contain '搜索摘要', got: %s", toolMsg.ContentString())
			}
		}

		if assistantToolMsg == nil {
			t.Error("second call should include assistant message with tool_calls")
		} else {
			if assistantToolMsg.ToolCalls[0].ID != "call_1" {
				t.Errorf("assistant tool_call ID should be 'call_1', got '%s'", assistantToolMsg.ToolCalls[0].ID)
			}
		}

		sseData := makeSSEData([]string{
			makeContentChunk("根据当前信息..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine: "testSearch",
			Results: []search.SearchResult{
				{Title: "补充信息", URL: "https://example.com", Snippet: "搜索摘要"},
			},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "测试",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}
}

func TestSendMessage_ThinkingAndContentBothPresent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeReasoningChunk("我需要分析这个问题..."),
			makeContentChunk("答案是42。"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "生命的意义是什么？",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)

	var assistantMsg *store.Message
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistantMsg = m
		}
	}

	if assistantMsg == nil {
		t.Fatal("expected assistant message")
	}
	if assistantMsg.ThinkingContent != "我需要分析这个问题..." {
		t.Errorf("expected ThinkingContent '我需要分析这个问题...', got '%s'", assistantMsg.ThinkingContent)
	}
	if assistantMsg.Content != "答案是42。" {
		t.Errorf("expected Content '答案是42。', got '%s'", assistantMsg.Content)
	}
}

func TestSendMessage_OnlyThinkingNoContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeReasoningChunk("让我思考一下..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "思考一下",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)

	var assistantMsg *store.Message
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistantMsg = m
		}
	}

	if assistantMsg == nil {
		t.Fatal("expected assistant message")
	}
	if assistantMsg.ThinkingContent != "让我思考一下..." {
		t.Errorf("expected ThinkingContent, got '%s'", assistantMsg.ThinkingContent)
	}
	if assistantMsg.Content != "" {
		t.Errorf("expected empty Content when only reasoning, got '%s'", assistantMsg.Content)
	}
}

func TestSendMessage_OnlyContentNoThinking(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("直接回答，不需要思考。"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "1+1等于几？",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)

	var assistantMsg *store.Message
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistantMsg = m
		}
	}

	if assistantMsg == nil {
		t.Fatal("expected assistant message")
	}
	if assistantMsg.Content != "直接回答，不需要思考。" {
		t.Errorf("expected Content '直接回答，不需要思考。', got '%s'", assistantMsg.Content)
	}
	if assistantMsg.ThinkingContent != "" {
		t.Errorf("expected empty ThinkingContent when no reasoning, got '%s'", assistantMsg.ThinkingContent)
	}
}

func TestSendMessage_ChineseContentInToolCallArgs(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{"query":"2026年中国GDP增长预测"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		sseData := makeSSEData([]string{
			makeContentChunk("根据当前信息..."),
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
			Results: []search.SearchResult{{Title: "GDP预测", URL: "http://example.com", Snippet: "预计增长5%"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "2026年中国GDP增长预测",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(searchProvider.calls) != 1 {
		t.Fatalf("expected 1 search call, got %d", len(searchProvider.calls))
	}
	if searchProvider.calls[0] != "2026年中国GDP增长预测" {
		t.Errorf("expected search query '2026年中国GDP增长预测', got '%s'", searchProvider.calls[0])
	}
}

func TestSendMessage_LLMError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, "model loading error")
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "测试",
		SearchEnabled: false,
	})
	if err == nil {
		t.Fatal("expected error when LLM returns 500, got nil")
	}
	if !strings.Contains(err.Error(), "stream chat") {
		t.Errorf("expected error to contain 'stream chat', got: %v", err)
	}
}

func TestSendMessage_StopGeneration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("开始回答..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	svc.StopGeneration()
}

func TestDoSearch_CodeQueryUsesCodeCategory(t *testing.T) {
	svc := newTestService()

	codeProvider := &mockSearchProvider{
		name: "codeSearch",
		results: &search.SearchResponse{
			Engine:  "codeSearch",
			Results: []search.SearchResult{{Title: "code", URL: "http://c", Snippet: "s"}},
		},
	}
	generalProvider := &mockSearchProvider{
		name: "generalSearch",
		results: &search.SearchResponse{
			Engine:  "generalSearch",
			Results: []search.SearchResult{{Title: "general", URL: "http://g", Snippet: "s"}},
		},
	}

	chain := search.NewCategorizedSearchChain([]search.CategorizedProvider{
		{Provider: generalProvider, Categories: []string{"general"}},
		{Provider: codeProvider, Categories: []string{"code"}},
	})
	svc.UpdateSearchChain(chain)

	resp := chat.DoSearch(svc, context.Background(), "python web framework comparison")
	if resp.Engine != "codeSearch" {
		t.Errorf("expected codeSearch for code query, got '%s'", resp.Engine)
	}

	resp = chat.DoSearch(svc, context.Background(), "今天天气怎么样")
	if resp.Engine != "generalSearch" {
		t.Errorf("expected generalSearch for general query, got '%s'", resp.Engine)
	}
}

func TestSendMessage_SearchResultStoredInDB(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{"query":"Rust vs Go"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		sseData := makeSSEData([]string{
			makeContentChunk("Rust和Go各有优势..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine: "testSearch",
			Results: []search.SearchResult{
				{Title: "Rust vs Go", URL: "https://example.com/rust-vs-go", Snippet: "对比分析"},
			},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "Rust和Go哪个好？",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)

	var assistantMsg *store.Message
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistantMsg = m
		}
	}

	if assistantMsg == nil {
		t.Fatal("expected assistant message")
	}

	if assistantMsg.SearchResults == "" {
		t.Error("assistant message should have SearchResults stored in DB")
	}

	var searchResults []search.SearchResult
	if err := json.Unmarshal([]byte(assistantMsg.SearchResults), &searchResults); err != nil {
		t.Fatalf("SearchResults should be valid JSON: %v", err)
	}
	if len(searchResults) != 1 {
		t.Errorf("expected 1 search result in DB, got %d", len(searchResults))
	}
	if searchResults[0].Title != "Rust vs Go" {
		t.Errorf("expected search result title 'Rust vs Go', got '%s'", searchResults[0].Title)
	}
}

func TestSendMessage_FinishReasonLength(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("这是部分回答，由于token限制..."),
			makeFinishChunk("length"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "写一篇很长的文章",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)

	var assistantMsg *store.Message
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistantMsg = m
		}
	}

	if assistantMsg == nil {
		t.Fatal("expected assistant message even with finish_reason=length")
	}
	if !strings.Contains(assistantMsg.Content, "部分回答") {
		t.Errorf("partial content should be saved, got: %s", assistantMsg.Content)
	}
}

func TestSendMessage_AccumulatedToolCallArguments(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			chunk1 := `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"","tool_calls":[{"id":"call_abc","type":"function","function":{"name":"search","arguments":""}}]},"finish_reason":null}]}`
			chunk2 := `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"","tool_calls":[{"id":"","type":"","function":{"name":"","arguments":"{\"qu"}}]},"finish_reason":null}]}`
			chunk3 := `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"","tool_calls":[{"id":"","type":"","function":{"name":"","arguments":"ery\":\"Go并发\"}"}}]},"finish_reason":null}]}`
			chunk4 := `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":"tool_calls"}]}`

			sseData := "data: " + chunk1 + "\n\ndata: " + chunk2 + "\n\ndata: " + chunk3 + "\n\ndata: " + chunk4 + "\n\ndata: [DONE]\n\n"
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		sseData := makeSSEData([]string{
			makeContentChunk("Go并发使用goroutine..."),
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
			Results: []search.SearchResult{{Title: "Go并发", URL: "http://go", Snippet: "goroutine"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "Go并发编程",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(searchProvider.calls) != 1 {
		t.Fatalf("expected 1 search call, got %d", len(searchProvider.calls))
	}
	if searchProvider.calls[0] != "Go并发" {
		t.Errorf("expected accumulated search query 'Go并发', got '%s'", searchProvider.calls[0])
	}
}

func TestSendMessage_ProgrammingQuestionWithSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeReasoningChunk("这是一个编程问题，需要搜索最新的API文档..."),
			makeContentChunk("根据最新文档，Go 1.24的http包新增了..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "codeSearch",
		results: &search.SearchResponse{
			Engine: "codeSearch",
			Results: []search.SearchResult{
				{Title: "Go 1.24 http", URL: "https://pkg.go.dev/net/http", Snippet: "New features in http package"},
			},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "Go 1.24的http包有什么新API？",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(searchProvider.calls) != 1 {
		t.Fatalf("expected 1 search call for programming question, got %d", len(searchProvider.calls))
	}
}

func TestSendMessage_TimelinessQuestionTriggersSearch(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeReasoningChunk("这是时效性问题，需要搜索最新信息..."),
				makeToolCallChunk("call_1", "search", `{"query":"2026年5月最新科技新闻"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		sseData := makeSSEData([]string{
			makeContentChunk("2026年5月的科技新闻包括..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	searchProvider := &mockSearchProvider{
		name: "testSearch",
		results: &search.SearchResponse{
			Engine: "testSearch",
			Results: []search.SearchResult{
				{Title: "科技新闻", URL: "https://example.com/tech", Snippet: "最新科技动态"},
			},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "最近有什么科技新闻？",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (model decides to search), got %d", callCount)
	}
	if len(searchProvider.calls) != 1 {
		t.Errorf("expected 1 search call, got %d", len(searchProvider.calls))
	}
}

func TestSendMessage_MultipleToolCallsWithDifferentIndices(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			chunk1 := `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"","tool_calls":[{"index":0,"id":"call_search","type":"function","function":{"name":"search","arguments":""}}]},"finish_reason":null}]}`
			chunk2 := `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":"","tool_calls":[{"index":0,"id":"","type":"","function":{"name":"","arguments":"{\"query\":\"Go并发\"}"}}]},"finish_reason":null}]}`
			chunk3 := `{"id":"chatcmpl-1","choices":[{"index":0,"delta":{"role":"assistant","content":""},"finish_reason":"tool_calls"}]}`

			sseData := "data: " + chunk1 + "\n\ndata: " + chunk2 + "\n\ndata: " + chunk3 + "\n\ndata: [DONE]\n\n"
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		sseData := makeSSEData([]string{
			makeContentChunk("Go并发使用goroutine和channel..."),
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
			Results: []search.SearchResult{{Title: "Go并发", URL: "http://go", Snippet: "goroutine"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "Go并发编程",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(searchProvider.calls) != 1 {
		t.Fatalf("expected 1 search call, got %d", len(searchProvider.calls))
	}
	if searchProvider.calls[0] != "Go并发" {
		t.Errorf("expected search query 'Go并发', got '%s'", searchProvider.calls[0])
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)

	var assistantMsg *store.Message
	for _, m := range msgs {
		if m.Role == "assistant" {
			assistantMsg = m
		}
	}

	if assistantMsg == nil {
		t.Fatal("expected assistant message")
	}
	if !strings.Contains(assistantMsg.Content, "goroutine") {
		t.Errorf("assistant should contain search-based response, got: %s", assistantMsg.Content)
	}
}

func TestSendMessage_ToolMessagesPersistedToDB(t *testing.T) {
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{"query":"Go并发"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		sseData := makeSSEData([]string{
			makeContentChunk("Go并发使用goroutine..."),
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
			Results: []search.SearchResult{{Title: "Go并发", URL: "http://go", Snippet: "goroutine"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "Go并发编程",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID)

	roleCounts := make(map[string]int)
	for _, m := range msgs {
		roleCounts[m.Role]++
	}

	if roleCounts["assistant"] < 1 {
		t.Errorf("expected at least 1 assistant message, got %d", roleCounts["assistant"])
	}
	if roleCounts["tool"] < 1 {
		t.Errorf("expected at least 1 tool message persisted, got %d", roleCounts["tool"])
	}

	var toolMsg *store.Message
	for _, m := range msgs {
		if m.Role == "tool" {
			toolMsg = m
		}
	}
	if toolMsg != nil {
		if toolMsg.ToolCallID != "call_1" {
			t.Errorf("expected tool message ToolCallID 'call_1', got '%s'", toolMsg.ToolCallID)
		}
		if !strings.Contains(toolMsg.Content, "内容:") {
			t.Errorf("tool message should contain search results, got: %s", toolMsg.Content)
		}
	}
}

func TestSendMessage_ToolMessagesRestoredInMultiRound(t *testing.T) {
	callCount := 0
	var secondRoundMessages []llm.ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{"query":"Go并发"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		sseData := makeSSEData([]string{
			makeContentChunk("Go并发使用goroutine..."),
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
			Results: []search.SearchResult{{Title: "Go并发", URL: "http://go", Snippet: "goroutine"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "Go并发编程",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("first SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc))
	convID := convs[0].ID

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		secondRoundMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("继续..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server2.Close()
	svc.UpdateClient(llm.NewClient(server2.URL))

	err = svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:        "继续讲",
		ConversationID: convID,
		SearchEnabled:  false,
	})
	if err != nil {
		t.Fatalf("second SendMessage failed: %v", err)
	}

	hasToolMsg := false
	hasAssistantToolCallMsg := false
	for _, m := range secondRoundMessages {
		if m.Role == "tool" {
			hasToolMsg = true
		}
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			hasAssistantToolCallMsg = true
		}
	}
	if !hasToolMsg {
		t.Error("second round LLM messages should include persisted tool message from first round")
	}
	if !hasAssistantToolCallMsg {
		t.Error("second round LLM messages should include persisted assistant tool_call message from first round")
	}
}

func TestSendMessage_SystemPromptContainsSearchGuidance(t *testing.T) {
	var receivedMessages []llm.ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "测试",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	systemMsg := receivedMessages[0]
	if systemMsg.Role != "system" {
		t.Fatalf("expected first message to be system, got '%s'", systemMsg.Role)
	}

	if !strings.Contains(systemMsg.ContentString(), "search工具") || !strings.Contains(systemMsg.ContentString(), "搜索") {
		t.Errorf("system prompt should contain search guidance, got: %s", systemMsg.ContentString())
	}
	if !strings.Contains(systemMsg.ContentString(), "[1][2]") {
		t.Errorf("system prompt should mention citation requirements, got: %s", systemMsg.ContentString())
	}
}

func TestSendMessage_SearchEnabled_UsesSeparateContextBlock(t *testing.T) {
	var receivedMessages []llm.ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
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
			Results: []search.SearchResult{{Title: "结果1", URL: "https://example.com", Snippet: "摘要1"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "搜索测试",
		SearchEnabled: true,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	var originalUserContent string
	var searchContextContent string
	for _, m := range receivedMessages {
		if m.Role == "user" {
			content := m.ContentString()
			if strings.Contains(content, "[补充信息]") {
				searchContextContent = content
			} else {
				originalUserContent = content
			}
		}
	}
	if strings.Contains(originalUserContent, "内容:") {
		t.Errorf("original user message should NOT contain search results, got: %s", originalUserContent)
	}
	if searchContextContent == "" {
		t.Error("search results should be injected as a separate context message containing [补充信息]")
	}
	if !strings.Contains(searchContextContent, "内容:") {
		t.Error("search context message should contain search result content")
	}

	hasNoToolMessages := true
	for _, m := range receivedMessages {
		if m.Role == "tool" {
			hasNoToolMessages = false
		}
	}
	if !hasNoToolMessages {
		t.Error("forced search should NOT use tool messages (should use context injection)")
	}
}

func TestSendMessage_ToolCallInvalidArgs_FeedbackToModel(t *testing.T) {
	callCount := 0
	var secondCallMessages []llm.ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)

		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{invalid json}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		secondCallMessages = req.Messages
		sseData := makeSSEData([]string{
			makeContentChunk("让我重新搜索..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "测试无效参数",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage should not fail with invalid tool call args, got: %v", err)
	}

	if callCount != 2 {
		t.Errorf("expected 2 LLM calls (model should get feedback and retry), got %d", callCount)
	}

	var toolMsg *llm.ChatMessage
	for i := range secondCallMessages {
		if secondCallMessages[i].Role == "tool" {
			toolMsg = &secondCallMessages[i]
		}
	}
	if toolMsg == nil {
		t.Fatal("expected tool message with error feedback in second LLM call")
	}
	if !strings.Contains(toolMsg.ContentString(), "Error") {
		t.Errorf("tool message should contain error feedback for invalid args, got: %s", toolMsg.ContentString())
	}
}

func TestSendMessage_SecondLLMCallProvidesTools(t *testing.T) {
	callCount := 0
	var secondReq *llm.ChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)

		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeToolCallChunk("call_1", "search", `{"query":"test"}`),
				makeFinishChunk("tool_calls"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		secondReq = &req
		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
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
			Results: []search.SearchResult{{Title: "test", URL: "http://t", Snippet: "s"}},
		},
	}

	svc := newInteractionTestService(t, server, searchProvider)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "test",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if secondReq == nil {
		t.Fatal("expected second LLM call")
	}
	if len(secondReq.Tools) != 1 || secondReq.Tools[0].Function.Name != "search" {
		t.Errorf("second LLM call should also provide search tool, got %d tools", len(secondReq.Tools))
	}
}

func TestSendMessage_ToolDefinitionHasRichDescription(t *testing.T) {
	var receivedReq *llm.ChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body := make([]byte, 1024*1024)
		n, _ := r.Body.Read(body)
		json.Unmarshal(body[:n], &req)
		receivedReq = &req

		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:       "测试",
		SearchEnabled: false,
	})
	if err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	if len(receivedReq.Tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(receivedReq.Tools))
	}
	desc := receivedReq.Tools[0].Function.Description
	if len(desc) < 50 {
		t.Errorf("tool description should be detailed (at least 50 chars), got %d chars: %s", len(desc), desc)
	}
	if !strings.Contains(desc, "search") {
		t.Errorf("tool description should mention 'search', got: %s", desc)
	}
}
