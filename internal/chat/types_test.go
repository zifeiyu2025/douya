// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"encoding/json"
	"testing"
)

// TestEventConstants 事件类型常量与前端 StreamEvent 联合类型成员对齐
// 防止发送侧/接收侧事件名拼写不一致（任务 31.5）
func TestEventConstants(t *testing.T) {
	want := map[string]string{
		EventToken:               "token",
		EventThinking:            "thinking",
		EventToolCallStart:       "tool_call_start",
		EventSearchStart:         "search_start",
		EventSearchResult:        "search_result",
		EventTokenSpeed:          "token_speed",
		EventPromptProgress:      "prompt_progress",
		EventContextTrimmed:      "context_trimmed",
		EventDone:                "done",
		EventStopped:             "stopped",
		EventError:               "error",
		EventConversationCreated: "conversation_created",
		EventAssistantMessage:    "assistant_message",
		EventUserMessage:         "user_message",
		EventConversationUpdated: "conversation_updated",
		EventConversationDeleted: "conversation_deleted",
		EventMessageDeleted:      "message_deleted",
	}
	for got, expected := range want {
		if got != expected {
			t.Errorf("常量 %q 与期望值 %q 不一致", got, expected)
		}
	}
}

// TestStreamEvent_StringContent 字符串 content 事件（token/thinking/error/search_start）的 JSON 往返
// 模拟发送侧直接传 string，验证 Marshal → Unmarshal → DecodeString 链路一致
func TestStreamEvent_StringContent(t *testing.T) {
	cases := []struct {
		name    string
		eventID string
		content string
	}{
		{"token", EventToken, "你好"},
		{"thinking", EventThinking, "让我想想"},
		{"error", EventError, "生成失败：超时"},
		{"search_start", EventSearchStart, `{"query":"天气"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orig := StreamEvent{Type: tc.eventID, Content: tc.content, ConversationID: "conv-1"}
			data, err := json.Marshal(orig)
			if err != nil {
				t.Fatalf("Marshal 失败: %v", err)
			}
			var round StreamEvent
			if err := json.Unmarshal(data, &round); err != nil {
				t.Fatalf("Unmarshal 失败: %v", err)
			}
			if round.Type != tc.eventID {
				t.Errorf("Type = %q, want %q", round.Type, tc.eventID)
			}
			if round.ConversationID != "conv-1" {
				t.Errorf("ConversationID = %q, want conv-1", round.ConversationID)
			}
			got, err := round.DecodeString()
			if err != nil {
				t.Fatalf("DecodeString 失败: %v", err)
			}
			if got != tc.content {
				t.Errorf("DecodeString = %q, want %q", got, tc.content)
			}
		})
	}
}

// TestStreamEvent_ToolCallStart 工具调用开始事件 JSON 往返
// 模拟发送侧用 map[string]string 发送，验证 DecodeToolCallStart 能正确解码
func TestStreamEvent_ToolCallStart(t *testing.T) {
	// 发送侧形态：map[string]string（对应 service_stream.go 实际发送）
	orig := StreamEvent{
		Type: EventToolCallStart,
		Content: map[string]string{
			"tool":  "web_search",
			"query": "今天天气",
		},
		ConversationID: "conv-2",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	var round StreamEvent
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	got, err := round.DecodeToolCallStart()
	if err != nil {
		t.Fatalf("DecodeToolCallStart 失败: %v", err)
	}
	if got.Tool != "web_search" || got.Query != "今天天气" {
		t.Errorf("DecodeToolCallStart = %+v, want {tool:web_search, query:今天天气}", got)
	}
}

// TestStreamEvent_TokenSpeed 生成速度事件 JSON 往返
// 模拟发送侧用 map[string]interface{} 发送，验证 DecodeTokenSpeed 能正确解码
func TestStreamEvent_TokenSpeed(t *testing.T) {
	orig := StreamEvent{
		Type: EventTokenSpeed,
		Content: map[string]any{
			"tokensPerSecond":   42.5,
			"predictedN":        100,
			"tokens_per_second": 42.5,
		},
		ConversationID: "conv-3",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	var round StreamEvent
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	got, err := round.DecodeTokenSpeed()
	if err != nil {
		t.Fatalf("DecodeTokenSpeed 失败: %v", err)
	}
	if got.TokensPerSecond != 42.5 || got.PredictedN != 100 {
		t.Errorf("DecodeTokenSpeed = %+v, want {tokensPerSecond:42.5, predictedN:100}", got)
	}
}

// TestStreamEvent_PromptProgress 提示词进度事件 JSON 往返
func TestStreamEvent_PromptProgress(t *testing.T) {
	orig := StreamEvent{
		Type: EventPromptProgress,
		Content: map[string]any{
			"total":     1024,
			"cache":     512,
			"processed": 768,
			"timeMs":    200,
		},
		ConversationID: "conv-4",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	var round StreamEvent
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	got, err := round.DecodePromptProgress()
	if err != nil {
		t.Fatalf("DecodePromptProgress 失败: %v", err)
	}
	if got.Total != 1024 || got.Cache != 512 || got.Processed != 768 || got.TimeMs != 200 {
		t.Errorf("DecodePromptProgress = %+v, want {total:1024, cache:512, processed:768, timeMs:200}", got)
	}
}

// TestStreamEvent_ContextTrimmed 上下文裁剪事件 JSON 往返
func TestStreamEvent_ContextTrimmed(t *testing.T) {
	orig := StreamEvent{
		Type: EventContextTrimmed,
		Content: map[string]any{
			"reason":         "exceed_context_size",
			"prompt_tokens":  9000,
			"context_size":   8192,
			"messages_after": 10,
		},
		ConversationID: "conv-5",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	var round StreamEvent
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	got, err := round.DecodeContextTrimmed()
	if err != nil {
		t.Fatalf("DecodeContextTrimmed 失败: %v", err)
	}
	if got.Reason != "exceed_context_size" || got.PromptTokens != 9000 || got.ContextSize != 8192 || got.MessagesAfter != 10 {
		t.Errorf("DecodeContextTrimmed = %+v, want {reason:exceed_context_size, prompt_tokens:9000, context_size:8192, messages_after:10}", got)
	}
}

// TestStreamEvent_Conversation 会话事件 JSON 往返（conversation_created / conversation_updated）
func TestStreamEvent_Conversation(t *testing.T) {
	orig := StreamEvent{
		Type: EventConversationCreated,
		Content: &Conversation{
			ID:        "conv-new",
			Title:     "新会话",
			CreatedAt: "2026-06-30T10:00:00Z",
			UpdatedAt: "2026-06-30T10:00:00Z",
		},
		ConversationID: "conv-new",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	var round StreamEvent
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	got, err := round.DecodeConversation()
	if err != nil {
		t.Fatalf("DecodeConversation 失败: %v", err)
	}
	if got.ID != "conv-new" || got.Title != "新会话" {
		t.Errorf("DecodeConversation = %+v, want {id:conv-new, title:新会话}", got)
	}
}

// TestStreamEvent_Message 消息事件 JSON 往返（assistant_message / user_message）
func TestStreamEvent_Message(t *testing.T) {
	orig := StreamEvent{
		Type: EventAssistantMessage,
		Content: Message{
			ID:             "msg-1",
			ConversationID: "conv-1",
			Role:           "assistant",
			Content:        "回复内容",
			SearchResults:  "",
			CreatedAt:      "2026-06-30T10:01:00Z",
		},
		ConversationID: "conv-1",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal 失败: %v", err)
	}
	var round StreamEvent
	if err := json.Unmarshal(data, &round); err != nil {
		t.Fatalf("Unmarshal 失败: %v", err)
	}
	got, err := round.DecodeMessage()
	if err != nil {
		t.Fatalf("DecodeMessage 失败: %v", err)
	}
	if got.ID != "msg-1" || got.Role != "assistant" || got.Content != "回复内容" {
		t.Errorf("DecodeMessage = %+v, want {id:msg-1, role:assistant, content:回复内容}", got)
	}
}

// TestStreamEvent_Deleted 删除事件 JSON 往返
// 验证两种形态：裸 string（ID）和 {"id":"..."} 对象
func TestStreamEvent_Deleted(t *testing.T) {
	t.Run("裸string形态", func(t *testing.T) {
		orig := StreamEvent{Type: EventConversationDeleted, Content: "conv-del"}
		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("Marshal 失败: %v", err)
		}
		var round StreamEvent
		if err := json.Unmarshal(data, &round); err != nil {
			t.Fatalf("Unmarshal 失败: %v", err)
		}
		got, err := round.DecodeDeleted()
		if err != nil {
			t.Fatalf("DecodeDeleted 失败: %v", err)
		}
		if got.ID != "conv-del" {
			t.Errorf("DecodeDeleted = %+v, want {id:conv-del}", got)
		}
	})
	t.Run("对象形态", func(t *testing.T) {
		orig := StreamEvent{
			Type:    EventMessageDeleted,
			Content: map[string]string{"id": "msg-del"},
		}
		data, err := json.Marshal(orig)
		if err != nil {
			t.Fatalf("Marshal 失败: %v", err)
		}
		var round StreamEvent
		if err := json.Unmarshal(data, &round); err != nil {
			t.Fatalf("Unmarshal 失败: %v", err)
		}
		got, err := round.DecodeDeleted()
		if err != nil {
			t.Fatalf("DecodeDeleted 失败: %v", err)
		}
		if got.ID != "msg-del" {
			t.Errorf("DecodeDeleted = %+v, want {id:msg-del}", got)
		}
	})
}

// TestStreamEvent_NilContent nil content 事件（done / stopped）JSON 往返
func TestStreamEvent_NilContent(t *testing.T) {
	cases := []string{EventDone, EventStopped}
	for _, eventType := range cases {
		t.Run(eventType, func(t *testing.T) {
			orig := StreamEvent{Type: eventType, Content: nil, ConversationID: "conv-fin"}
			data, err := json.Marshal(orig)
			if err != nil {
				t.Fatalf("Marshal 失败: %v", err)
			}
			var round StreamEvent
			if err := json.Unmarshal(data, &round); err != nil {
				t.Fatalf("Unmarshal 失败: %v", err)
			}
			if round.Type != eventType {
				t.Errorf("Type = %q, want %q", round.Type, eventType)
			}
			if round.Content != nil {
				t.Errorf("Content = %v, want nil", round.Content)
			}
		})
	}
}
