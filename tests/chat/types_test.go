// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"douya/internal/chat"
	"douya/internal/store"
)

func TestStoreMsgToChat_FieldsMappedCorrectly(t *testing.T) {
	now := time.Now()
	storeMsg := &store.Message{
		ID:              "msg-1",
		ConversationID:  "conv-1",
		Role:            "assistant",
		Content:         "你好世界",
		ThinkingContent: "思考过程",
		SearchResults:   `[{"title":"test"}]`,
		CreatedAt:       now,
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	if chatMsg.ID != "msg-1" {
		t.Errorf("expected ID 'msg-1', got '%s'", chatMsg.ID)
	}
	if chatMsg.ConversationID != "conv-1" {
		t.Errorf("expected ConversationID 'conv-1', got '%s'", chatMsg.ConversationID)
	}
	if chatMsg.Role != "assistant" {
		t.Errorf("expected Role 'assistant', got '%s'", chatMsg.Role)
	}
	if chatMsg.Content != "你好世界" {
		t.Errorf("expected Content '你好世界', got '%s'", chatMsg.Content)
	}
	if chatMsg.ThinkingContent != "思考过程" {
		t.Errorf("expected ThinkingContent '思考过程', got '%s'", chatMsg.ThinkingContent)
	}
	if chatMsg.SearchResults != `[{"title":"test"}]` {
		t.Errorf("expected SearchResults '[{\"title\":\"test\"}]', got '%s'", chatMsg.SearchResults)
	}
}

func TestStoreMsgToChat_TimeFormatIsRFC3339(t *testing.T) {
	now := time.Date(2026, 5, 6, 14, 30, 0, 0, time.Local)
	storeMsg := &store.Message{
		ID:        "msg-1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: now,
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	expectedPrefix := "2026-05-06T"
	if !strings.HasPrefix(chatMsg.CreatedAt, expectedPrefix) {
		t.Errorf("expected CreatedAt to start with '%s', got '%s'", expectedPrefix, chatMsg.CreatedAt)
	}

	_, err := time.Parse(time.RFC3339, chatMsg.CreatedAt)
	if err != nil {
		t.Errorf("CreatedAt '%s' is not valid RFC3339: %v", chatMsg.CreatedAt, err)
	}
}

func TestStoreMsgToChat_EmptyFields(t *testing.T) {
	storeMsg := &store.Message{
		ID:        "msg-2",
		Role:      "user",
		Content:   "test",
		CreatedAt: time.Now(),
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	if chatMsg.ThinkingContent != "" {
		t.Errorf("expected empty ThinkingContent, got '%s'", chatMsg.ThinkingContent)
	}
	if chatMsg.SearchResults != "" {
		t.Errorf("expected empty SearchResults, got '%s'", chatMsg.SearchResults)
	}
}

func TestConversation_JSONSerialization_TimeFieldsAreString(t *testing.T) {
	conv := &chat.Conversation{
		ID:        "conv-1",
		Title:     "测试会话",
		CreatedAt: "2026-05-06T14:30:00+08:00",
		UpdatedAt: "2026-05-06T14:30:00+08:00",
	}

	data, err := json.Marshal(conv)
	if err != nil {
		t.Fatalf("failed to marshal Conversation: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal Conversation: %v", err)
	}

	if _, ok := parsed["created_at"].(string); !ok {
		t.Errorf("expected created_at to be string, got %T", parsed["created_at"])
	}
	if _, ok := parsed["updated_at"].(string); !ok {
		t.Errorf("expected updated_at to be string, got %T", parsed["updated_at"])
	}

	if parsed["title"] != "测试会话" {
		t.Errorf("expected title '测试会话', got '%v'", parsed["title"])
	}
}

func TestConversation_JSONSerialization_NoTimeObject(t *testing.T) {
	conv := &chat.Conversation{
		ID:        "conv-1",
		Title:     "test",
		CreatedAt: "2026-05-06T14:30:00+08:00",
		UpdatedAt: "2026-05-06T14:30:00+08:00",
	}

	data, err := json.Marshal(conv)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)

	if strings.Contains(jsonStr, `"Wall"`) || strings.Contains(jsonStr, `"Ext"`) {
		t.Errorf("time.Time internal fields leaked into JSON, indicating time.Time was not converted to string: %s", jsonStr)
	}
}

func TestMessage_JSONSerialization_TimeFieldIsString(t *testing.T) {
	msg := &chat.Message{
		ID:              "msg-1",
		ConversationID:  "conv-1",
		Role:            "assistant",
		Content:         "你好",
		ThinkingContent: "思考中",
		SearchResults:   `[{"title":"test"}]`,
		CreatedAt:       "2026-05-06T14:30:00+08:00",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal Message: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal Message: %v", err)
	}

	if _, ok := parsed["created_at"].(string); !ok {
		t.Errorf("expected created_at to be string, got %T", parsed["created_at"])
	}
}

func TestMessage_JSONSerialization_OmitEmpty(t *testing.T) {
	msg := &chat.Message{
		ID:             "msg-1",
		ConversationID: "conv-1",
		Role:           "user",
		Content:        "hello",
		CreatedAt:      "2026-05-06T14:30:00+08:00",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	jsonStr := string(data)

	if strings.Contains(jsonStr, "thinking_content") {
		t.Errorf("thinking_content should be omitted when empty, but found in: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, "search_results") {
		t.Errorf("search_results should always be present, but missing in: %s", jsonStr)
	}
}

func TestStreamEvent_JSONSerialization(t *testing.T) {
	event := &chat.StreamEvent{
		Type:    "token",
		Content: "你好",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal StreamEvent: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal StreamEvent: %v", err)
	}

	if parsed["type"] != "token" {
		t.Errorf("expected type 'token', got '%v'", parsed["type"])
	}
	if parsed["content"] != "你好" {
		t.Errorf("expected content '你好', got '%v'", parsed["content"])
	}
}

func TestStreamEvent_ConversationCreatedEvent(t *testing.T) {
	conv := &chat.Conversation{
		ID:        "conv-1",
		Title:     "新对话",
		CreatedAt: "2026-05-06T14:30:00+08:00",
		UpdatedAt: "2026-05-06T14:30:00+08:00",
	}

	event := &chat.StreamEvent{
		Type:    "conversation_created",
		Content: conv,
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	content, ok := parsed["content"].(map[string]any)
	if !ok {
		t.Fatal("content should be an object")
	}

	if _, ok := content["created_at"].(string); !ok {
		t.Errorf("content.created_at should be string, got %T", content["created_at"])
	}
	if content["title"] != "新对话" {
		t.Errorf("expected title '新对话', got '%v'", content["title"])
	}
}

func TestStoreMsgToChat_AttachmentSummary(t *testing.T) {
	storeMsg := &store.Message{
		ID:          "msg-1",
		Role:        "user",
		Content:     "请分析这个文件",
		CreatedAt:   time.Now(),
		Attachments: `[{"type":"text","name":"main.go","mime_type":"text/plain","data":"package main"},{"type":"pdf","name":"report.pdf","mime_type":"application/pdf","data":"base64data"},{"type":"audio","name":"recording.mp3","mime_type":"audio/mpeg","data":"base64audio","format":"mp3"}]`,
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	if len(chatMsg.Attachments) != 3 {
		t.Fatalf("expected 3 attachment summaries, got %d", len(chatMsg.Attachments))
	}

	if chatMsg.Attachments[0].Type != "text" || chatMsg.Attachments[0].Name != "main.go" {
		t.Errorf("expected first attachment type='text' name='main.go', got type='%s' name='%s'", chatMsg.Attachments[0].Type, chatMsg.Attachments[0].Name)
	}
	if chatMsg.Attachments[1].Type != "pdf" || chatMsg.Attachments[1].Name != "report.pdf" {
		t.Errorf("expected second attachment type='pdf' name='report.pdf', got type='%s' name='%s'", chatMsg.Attachments[1].Type, chatMsg.Attachments[1].Name)
	}
	if chatMsg.Attachments[2].Type != "audio" || chatMsg.Attachments[2].Name != "recording.mp3" {
		t.Errorf("expected third attachment type='audio' name='recording.mp3', got type='%s' name='%s'", chatMsg.Attachments[2].Type, chatMsg.Attachments[2].Name)
	}
}

func TestStoreMsgToChat_AttachmentSummaryNoDataLeak(t *testing.T) {
	storeMsg := &store.Message{
		ID:          "msg-1",
		Role:        "user",
		Content:     "test",
		CreatedAt:   time.Now(),
		Attachments: `[{"type":"text","name":"secret.txt","mime_type":"text/plain","data":"sensitive content here"}]`,
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	data, err := json.Marshal(chatMsg.Attachments)
	if err != nil {
		t.Fatalf("failed to marshal attachments: %v", err)
	}

	jsonStr := string(data)
	if strings.Contains(jsonStr, "sensitive content here") {
		t.Errorf("attachment data should not be included in summary, but found in: %s", jsonStr)
	}
	if !strings.Contains(jsonStr, "secret.txt") {
		t.Errorf("attachment name should be included in summary, but missing in: %s", jsonStr)
	}
}

func TestStoreMsgToChat_EmptyAttachments(t *testing.T) {
	storeMsg := &store.Message{
		ID:        "msg-1",
		Role:      "user",
		Content:   "hello",
		CreatedAt: time.Now(),
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	if len(chatMsg.Attachments) != 0 {
		t.Errorf("expected no attachments for message without attachments, got %d", len(chatMsg.Attachments))
	}
}

func TestStoreMsgToChat_InvalidAttachmentsJSON(t *testing.T) {
	storeMsg := &store.Message{
		ID:          "msg-1",
		Role:        "user",
		Content:     "hello",
		CreatedAt:   time.Now(),
		Attachments: "invalid json{{{",
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	if len(chatMsg.Attachments) != 0 {
		t.Errorf("expected no attachments for invalid JSON, got %d", len(chatMsg.Attachments))
	}
}

func TestAttachmentSummary_JSONFields(t *testing.T) {
	summary := chat.AttachmentSummary{
		Type:     "audio",
		Name:     "voice.mp3",
		MimeType: "audio/mpeg",
	}

	data, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("failed to marshal AttachmentSummary: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if _, ok := parsed["data"]; ok {
		t.Errorf("AttachmentSummary should not contain 'data' field, but found in: %s", string(data))
	}
	if _, ok := parsed["format"]; ok {
		t.Errorf("AttachmentSummary should not contain 'format' field, but found in: %s", string(data))
	}
	if parsed["type"] != "audio" {
		t.Errorf("expected type 'audio', got '%v'", parsed["type"])
	}
	if parsed["name"] != "voice.mp3" {
		t.Errorf("expected name 'voice.mp3', got '%v'", parsed["name"])
	}
}
