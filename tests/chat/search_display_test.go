package chat_test

import (
	"encoding/json"
	"testing"

	"douya/internal/chat"
	"douya/internal/search"
	"douya/internal/store"
)

func TestStoreMsgToChat_PreservesSearchResults(t *testing.T) {
	searchJSON := `[{"title":"test","url":"http://example.com","snippet":"test snippet"}]`
	msg := &store.Message{
		ID:            "msg-1",
		ConversationID: "conv-1",
		Role:          "assistant",
		Content:       "response content",
		SearchResults: searchJSON,
	}

	result := chat.StoreMsgToChat(msg)

	if result.SearchResults != searchJSON {
		t.Errorf("expected SearchResults %q, got %q", searchJSON, result.SearchResults)
	}
	if result.Content != "response content" {
		t.Errorf("expected Content 'response content', got %q", result.Content)
	}
}

func TestStoreMsgToChat_EmptySearchResults(t *testing.T) {
	msg := &store.Message{
		ID:            "msg-1",
		ConversationID: "conv-1",
		Role:          "assistant",
		Content:       "response",
		SearchResults: "",
	}

	result := chat.StoreMsgToChat(msg)

	if result.SearchResults != "" {
		t.Errorf("expected empty SearchResults, got %q", result.SearchResults)
	}
}

func TestStoreMsgToChat_AllFieldsPreserved(t *testing.T) {
	msg := &store.Message{
		ID:               "msg-1",
		ConversationID:   "conv-1",
		Role:             "assistant",
		Content:          "response",
		ThinkingContent:  "thinking process",
		ThinkingDuration: 5.5,
		SearchResults:    `[{"title":"test"}]`,
	}

	result := chat.StoreMsgToChat(msg)

	if result.ThinkingContent != "thinking process" {
		t.Errorf("expected ThinkingContent 'thinking process', got %q", result.ThinkingContent)
	}
	if result.ThinkingDuration != 5.5 {
		t.Errorf("expected ThinkingDuration 5.5, got %f", result.ThinkingDuration)
	}
	if result.SearchResults != `[{"title":"test"}]` {
		t.Errorf("expected SearchResults, got %q", result.SearchResults)
	}
}

func TestMessageJSON_Serialization_SearchResultsNotOmitted(t *testing.T) {
	msg := chat.Message{
		ID:            "msg-1",
		ConversationID: "conv-1",
		Role:          "assistant",
		Content:       "response",
		SearchResults: `[{"title":"test"}]`,
		CreatedAt:     "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if _, ok := parsed["search_results"]; !ok {
		t.Error("search_results field should be present in JSON when it has a value")
	}
}

func TestMessageJSON_Serialization_SearchResultsAlwaysPresent(t *testing.T) {
	msg := chat.Message{
		ID:            "msg-1",
		ConversationID: "conv-1",
		Role:          "assistant",
		Content:       "response",
		SearchResults: "",
		CreatedAt:     "2024-01-01T00:00:00Z",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if _, ok := parsed["search_results"]; !ok {
		t.Error("search_results field should always be present (no omitempty)")
	}
}

func TestSearchResponseJSON_Serialization_NilResults(t *testing.T) {
	resp := &search.SearchResponse{}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	results, ok := parsed["results"]
	if !ok {
		t.Error("results field should be present in SearchResponse JSON")
	}
	if results != nil {
		t.Errorf("expected nil results for empty SearchResponse, got %v", results)
	}
}

func TestSearchResponseJSON_Serialization_EmptyResults(t *testing.T) {
	resp := &search.SearchResponse{
		Results: []search.SearchResult{},
		Engine:  "test",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	results, ok := parsed["results"]
	if !ok {
		t.Error("results field should be present in SearchResponse JSON")
	}
	arr, ok := results.([]interface{})
	if !ok {
		t.Errorf("expected results to be an array, got %T", results)
	}
	if len(arr) != 0 {
		t.Errorf("expected empty results array, got %d items", len(arr))
	}
}

func TestSearchResponseJSON_Serialization_WithResults(t *testing.T) {
	resp := &search.SearchResponse{
		Results: []search.SearchResult{
			{Title: "Test", URL: "http://example.com", Snippet: "Test snippet"},
		},
		Engine: "test",
	}

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	results, ok := parsed["results"]
	if !ok {
		t.Error("results field should be present in SearchResponse JSON")
	}
	arr, ok := results.([]interface{})
	if !ok {
		t.Errorf("expected results to be an array, got %T", results)
	}
	if len(arr) != 1 {
		t.Errorf("expected 1 result, got %d", len(arr))
	}
}

func TestStreamEventJSON_Serialization_WithSearchResults(t *testing.T) {
	event := chat.StreamEvent{
		Type: "search_result",
		Content: []search.SearchResult{
			{Title: "Test", URL: "http://example.com", Snippet: "Test snippet"},
		},
		ConversationID: "conv-1",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	content, ok := parsed["content"].([]interface{})
	if !ok {
		t.Fatalf("expected content to be an array, got %T", parsed["content"])
	}

	if len(content) != 1 {
		t.Errorf("expected 1 result, got %d", len(content))
	}

	if parsed["conversation_id"] != "conv-1" {
		t.Errorf("expected conversation_id 'conv-1', got %v", parsed["conversation_id"])
	}
}

func TestStreamEventJSON_Serialization_WithEmptySearchResults(t *testing.T) {
	event := chat.StreamEvent{
		Type:           "search_result",
		Content:        []search.SearchResult{},
		ConversationID: "conv-1",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	content, ok := parsed["content"].([]interface{})
	if !ok {
		t.Fatalf("expected content to be an array, got %T", parsed["content"])
	}

	if len(content) != 0 {
		t.Errorf("expected 0 results, got %d", len(content))
	}
}

func TestGetMessages_FiltersToolMessages(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "搜索测试"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "搜索测试",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "",
		ToolCalls:      `[{"id":"tc1","type":"function","function":{"name":"search","arguments":"{\"query\":\"test\"}"}}]`,
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        "search results content",
		ToolCallID:     "tc1",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "基于补充信息的回答",
		SearchResults:  `[{"title":"Test","url":"http://example.com","snippet":"Test"}]`,
	}, nil)

	msgs, err := svc.GetMessages(conv.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 display messages (user + final assistant), got %d", len(msgs))
	}

	if msgs[0].Role != "user" {
		t.Errorf("expected first message role 'user', got %q", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("expected second message role 'assistant', got %q", msgs[1].Role)
	}
	if msgs[1].SearchResults == "" {
		t.Error("assistant message should have SearchResults")
	}

	var searchResults []map[string]interface{}
	if err := json.Unmarshal([]byte(msgs[1].SearchResults), &searchResults); err != nil {
		t.Fatalf("failed to parse SearchResults JSON: %v", err)
	}
	if len(searchResults) != 1 {
		t.Errorf("expected 1 search result, got %d", len(searchResults))
	}
}

func TestGetMessages_SearchResultsPreserved(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "补充信息保持测试"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	searchJSON := `[{"title":"Go语言","url":"https://golang.org","snippet":"The Go Programming Language"}]`

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "什么是Go语言？",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID:  conv.ID,
		Role:            "assistant",
		Content:         "Go是一种编程语言[1]",
		ThinkingContent: "用户问Go语言",
		ThinkingDuration: 2.5,
		SearchResults:   searchJSON,
	}, nil)

	msgs, err := svc.GetMessages(conv.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	assistantMsg := msgs[1]
	if assistantMsg.SearchResults != searchJSON {
		t.Errorf("SearchResults not preserved: expected %q, got %q", searchJSON, assistantMsg.SearchResults)
	}
	if assistantMsg.ThinkingContent != "用户问Go语言" {
		t.Errorf("ThinkingContent not preserved: expected '用户问Go语言', got %q", assistantMsg.ThinkingContent)
	}
	if assistantMsg.ThinkingDuration != 2.5 {
		t.Errorf("ThinkingDuration not preserved: expected 2.5, got %f", assistantMsg.ThinkingDuration)
	}
}

func TestFormatSearchResults(t *testing.T) {
	results := []search.SearchResult{
		{Title: "Go", URL: "https://golang.org", Snippet: "The Go Programming Language"},
		{Title: "Go Tour", URL: "https://tour.golang.org", Snippet: "A Tour of Go"},
	}

	formatted := chat.FormatSearchResults(results)

	if formatted == "" {
		t.Error("FormatSearchResults should return non-empty string")
	}
	if !contains(formatted, "Go") {
		t.Error("formatted results should contain title 'Go'")
	}
	if !contains(formatted, "https://golang.org") {
		t.Error("formatted results should contain URL")
	}
	if !contains(formatted, "The Go Programming Language") {
		t.Error("formatted results should contain snippet")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstr(s, substr))
}

func containsSubstr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
