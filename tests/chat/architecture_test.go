// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"douya/internal/chat"
	"douya/internal/llm"
	"douya/internal/store"
)

func TestEstimateMessageTokens_IncludesImages(t *testing.T) {
	msg := &store.Message{
		Role:    "user",
		Content: "hello",
		Images:  `["data:image/png;base64,iVBOR...verylongbase64data..."]`,
	}
	tokens := chat.EstimateMessageTokens(msg)
	contentTokens := len([]rune(msg.Content)) * 2
	if tokens <= contentTokens {
		t.Errorf("estimateMessageTokens should account for Images field: got %d, content-only would be %d", tokens, contentTokens)
	}
}

func TestEstimateMessageTokens_IncludesToolCalls(t *testing.T) {
	msg := &store.Message{
		Role:      "assistant",
		Content:   "let me search",
		ToolCalls: `[{"id":"call_1","function":{"name":"search","arguments":"{\"query\":\"test\"}"}}]`,
	}
	tokens := chat.EstimateMessageTokens(msg)
	contentTokens := chat.EstimateTokensByLang(msg.Content, "en")
	if tokens <= contentTokens {
		t.Errorf("estimateMessageTokens should account for ToolCalls field: got %d, content-only would be %d", tokens, contentTokens)
	}
}

func TestEstimateMessageTokens_IncludesSearchResults(t *testing.T) {
	msg := &store.Message{
		Role:          "assistant",
		Content:       "answer",
		SearchResults: `[{"title":"test","url":"http://example.com","snippet":"long snippet here"}]`,
	}
	tokens := chat.EstimateMessageTokens(msg)
	contentTokens := chat.EstimateTokensByLang(msg.Content, "en")
	if tokens <= contentTokens {
		t.Errorf("estimateMessageTokens should account for SearchResults field: got %d, content-only would be %d", tokens, contentTokens)
	}
}

func TestEstimateMessageTokens_IncludesThinkingContent(t *testing.T) {
	msg := &store.Message{
		Role:            "assistant",
		Content:         "answer",
		ThinkingContent: "I need to think about this carefully and analyze the problem step by step",
	}
	tokens := chat.EstimateMessageTokens(msg)
	contentTokens := chat.EstimateTokensByLang(msg.Content, "en")
	if tokens <= contentTokens {
		t.Errorf("estimateMessageTokens should account for ThinkingContent field: got %d, content-only would be %d", tokens, contentTokens)
	}
}

func TestEstimateMessageTokens_AllFieldsCombined(t *testing.T) {
	msg := &store.Message{
		Role:            "assistant",
		Content:         "answer",
		ThinkingContent: "thinking",
		SearchResults:   `[{"title":"t"}]`,
		ToolCalls:       `[{"id":"c1"}]`,
		Images:          `["img1"]`,
	}
	tokens := chat.EstimateMessageTokens(msg)
	expectedMin := len([]rune(msg.Content))*2 +
		len([]rune(msg.ThinkingContent))*2 +
		len([]rune(msg.SearchResults))*2 +
		len([]rune(msg.ToolCalls))*2 +
		len([]rune(msg.Images))*2
	if tokens < expectedMin {
		t.Errorf("estimateMessageTokens should account for all fields: got %d, expected at least %d", tokens, expectedMin)
	}
}

func TestEstimateMessageTokens_ZeroReturnsOne(t *testing.T) {
	msg := &store.Message{Role: "user"}
	tokens := chat.EstimateMessageTokens(msg)
	// 空消息返回最小值 11（10 chat template 开销 + 最小值保护）
	if tokens != 11 {
		t.Errorf("empty message should estimate 11 tokens (chat template overhead + min), got %d", tokens)
	}
}

func TestStreamAccumulator_ResetForNextCall_PreservesAccumulatedState(t *testing.T) {
	acc := chat.NewStreamAccumulator("conv1", func(string, any) {}, func(string, string, any) {})
	acc.FullContent.WriteString("first response")
	acc.FullThinking.WriteString("first thinking")
	acc.LastSearchJSON = `{"results":[1]}`
	acc.FinishReason = "tool_calls"
	acc.ToolCallMap[0] = &llm.ToolCall{Index: 0, ID: "tc1"}

	chat.ResetForNextCall(acc)

	if acc.FullContent.String() != "" {
		t.Errorf("FullContent should be reset, got %q", acc.FullContent.String())
	}
	if acc.FullThinking.String() != "first thinking" {
		t.Errorf("FullThinking should be preserved across calls, got %q", acc.FullThinking.String())
	}
	if acc.LastSearchJSON != `{"results":[1]}` {
		t.Errorf("LastSearchJSON should be preserved across calls, got %q", acc.LastSearchJSON)
	}
	if acc.FinishReason != "" {
		t.Errorf("FinishReason should be reset, got %q", acc.FinishReason)
	}
	if len(acc.ToolCallMap) != 0 {
		t.Errorf("ToolCallMap should be reset, got %d entries", len(acc.ToolCallMap))
	}
}

func TestStreamAccumulator_ThinkingDurationCalculation(t *testing.T) {
	acc := chat.NewStreamAccumulator("conv1", func(string, any) {}, func(string, string, any) {})

	if acc.ThinkingDuration != 0 {
		t.Errorf("initial ThinkingDuration should be 0, got %f", acc.ThinkingDuration)
	}
	if !acc.ThinkingStartTime.IsZero() {
		t.Errorf("initial ThinkingStartTime should be zero")
	}
}

func TestStoreMsgToChat_PreservesThinkingContent(t *testing.T) {
	now := time.Now()
	storeMsg := &store.Message{
		ID:              "msg-1",
		ConversationID:  "conv-1",
		Role:            "assistant",
		Content:         "answer",
		ThinkingContent: "thinking<tool_call\nmore thinking</tool_call\nfinal",
		CreatedAt:       now,
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	if chatMsg.ThinkingContent != storeMsg.ThinkingContent {
		t.Errorf("ThinkingContent should be preserved as-is, got: %q, want: %q", chatMsg.ThinkingContent, storeMsg.ThinkingContent)
	}
}

func TestUpdateConfig_SavePathConsistency(t *testing.T) {
	svc := newTestService()
	cfg := svc.GetConfig()
	cfg.Temperature = 0.5

	svc.UpdateConfig(cfg)

	if svc.GetConfig().Temperature != 0.5 {
		t.Errorf("config should be updated in memory, got Temperature=%f", svc.GetConfig().Temperature)
	}
}

func TestStoreMsgToChat_PreservesAllFields(t *testing.T) {
	now := time.Now()
	storeMsg := &store.Message{
		ID:               "msg-1",
		ConversationID:   "conv-1",
		Role:             "assistant",
		Content:          "answer",
		ThinkingContent:  "thinking",
		ThinkingDuration: 2.5,
		SearchResults:    `[{"title":"test"}]`,
		Images:           `["img1"]`,
		CreatedAt:        now,
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	if chatMsg.ID != storeMsg.ID {
		t.Errorf("ID mismatch: got %q, want %q", chatMsg.ID, storeMsg.ID)
	}
	if chatMsg.ConversationID != storeMsg.ConversationID {
		t.Errorf("ConversationID mismatch: got %q, want %q", chatMsg.ConversationID, storeMsg.ConversationID)
	}
	if chatMsg.Role != storeMsg.Role {
		t.Errorf("Role mismatch: got %q, want %q", chatMsg.Role, storeMsg.Role)
	}
	if chatMsg.Content != storeMsg.Content {
		t.Errorf("Content mismatch: got %q, want %q", chatMsg.Content, storeMsg.Content)
	}
	if chatMsg.ThinkingContent != storeMsg.ThinkingContent {
		t.Errorf("ThinkingContent mismatch: got %q, want %q", chatMsg.ThinkingContent, storeMsg.ThinkingContent)
	}
	if chatMsg.ThinkingDuration != storeMsg.ThinkingDuration {
		t.Errorf("ThinkingDuration mismatch: got %f, want %f", chatMsg.ThinkingDuration, storeMsg.ThinkingDuration)
	}
	if chatMsg.SearchResults != storeMsg.SearchResults {
		t.Errorf("SearchResults mismatch: got %q, want %q", chatMsg.SearchResults, storeMsg.SearchResults)
	}
	if chatMsg.Images != storeMsg.Images {
		t.Errorf("Images mismatch: got %q, want %q", chatMsg.Images, storeMsg.Images)
	}
}

func TestExportConversation_FiltersToolMessages(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "Test"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "hello",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "",
		ToolCalls:      `[{"id":"tc1","function":{"name":"search","arguments":"{}"}}]`,
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        "search results",
		ToolCallID:     "tc1",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "answer",
	}, nil)

	result, err := svc.ExportConversation(conv.ID, "markdown")
	if err != nil {
		t.Fatalf("ExportConversation failed: %v", err)
	}

	if strings.Contains(result, "search results") {
		t.Errorf("export should not contain tool message content, got: %s", result)
	}
	if !strings.Contains(result, "hello") {
		t.Errorf("export should contain user message, got: %s", result)
	}
	if !strings.Contains(result, "answer") {
		t.Errorf("export should contain assistant answer, got: %s", result)
	}
}

func TestExportConversation_FiltersAssistantToolCallMessages(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "Test"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "search for Go",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "",
		ToolCalls:      `[{"id":"tc1","function":{"name":"search","arguments":"{\"query\":\"Go\"}"}}]`,
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        "Go results",
		ToolCallID:     "tc1",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Go is a programming language",
	}, nil)

	result, err := svc.ExportConversation(conv.ID, "json")
	if err != nil {
		t.Fatalf("ExportConversation failed: %v", err)
	}

	var messages []struct {
		Role       string `json:"role"`
		Content    string `json:"content"`
		ToolCalls  string `json:"tool_calls"`
		ToolCallID string `json:"tool_call_id"`
	}
	if err := json.Unmarshal([]byte(result), &messages); err != nil {
		t.Fatalf("unmarshal export: %v", err)
	}

	for _, m := range messages {
		if m.Role == "tool" {
			t.Errorf("export should not contain tool messages, found: role=%s content=%s", m.Role, m.Content)
		}
		if m.Role == "assistant" && m.ToolCalls != "" {
			t.Errorf("export should not contain assistant tool-call messages, found: role=%s tool_calls=%s", m.Role, m.ToolCalls)
		}
	}
}

func TestGetMessages_PreservesThinkingContent(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "Test"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "hello",
	}, nil)
	rawThinking := "thinking<tool_call\nmore thinking</tool_call\nfinal"
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID:  conv.ID,
		Role:            "assistant",
		Content:         "answer",
		ThinkingContent: rawThinking,
	}, nil)

	msgs, err := svc.GetMessages(conv.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}

	for _, m := range msgs {
		if m.Role == "assistant" && m.ThinkingContent != rawThinking {
			t.Errorf("GetMessages should preserve raw ThinkingContent, got: %q, want: %q", m.ThinkingContent, rawThinking)
		}
	}
}

func TestSearchMessages_PreservesThinkingContent(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "Test Search"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("create conversation: %v", err)
	}

	rawThinking := "analysis<tool_call\ndeep analysis</tool_call\nconclusion"
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID:  conv.ID,
		Role:            "assistant",
		Content:         "golang programming answer",
		ThinkingContent: rawThinking,
	}, nil)

	msgs, err := svc.SearchMessages("golang")
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}

	for _, m := range msgs {
		if m.ThinkingContent != rawThinking {
			t.Errorf("SearchMessages should preserve raw ThinkingContent, got: %q, want: %q", m.ThinkingContent, rawThinking)
		}
	}
}
