// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"testing"

	"douya/internal/llm"
)

// TestCleanToolCallPairs_LeadingOrphanTool 验证删除开头的孤立 tool 消息
//
// 业务场景：TrimMessagesToFit 裁剪后，第一条消息可能是孤立的 tool 消息
// （对应的 assistant(tool_calls) 被裁剪掉了），API 会报错，需要清理。
func TestCleanToolCallPairs_LeadingOrphanTool(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "tool", Content: "tool result 1"}, // 孤立 tool
		{Role: "tool", Content: "tool result 2"}, // 孤立 tool
		{Role: "user", Content: "用户消息"},
		{Role: "assistant", Content: "助手回复"},
	}

	result := cleanToolCallPairs(messages)

	if len(result) != 2 {
		t.Fatalf("期望 2 条消息（删除 2 个孤立 tool），实际: %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("第一条应为 user，实际: %s", result[0].Role)
	}
	if result[1].Role != "assistant" {
		t.Errorf("第二条应为 assistant，实际: %s", result[1].Role)
	}
}

// TestCleanToolCallPairs_AllToolMessages 验证全部是 tool 消息时返回空
func TestCleanToolCallPairs_AllToolMessages(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "tool", Content: "result 1"},
		{Role: "tool", Content: "result 2"},
		{Role: "tool", Content: "result 3"},
	}

	result := cleanToolCallPairs(messages)

	if len(result) != 0 {
		t.Errorf("全部 tool 消息应返回空，实际: %d 条", len(result))
	}
}

// TestCleanToolCallPairs_ValidPairPreserved 验证正常的 tool call 配对被保留
func TestCleanToolCallPairs_ValidPairPreserved(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: "查询天气"},
		{
			Role:      "assistant",
			ToolCalls: []llm.ToolCall{{ID: "tc-1", Function: llm.FunctionCall{Name: "search"}}},
		},
		{Role: "tool", ToolCallID: "tc-1", Content: "晴天"},
		{Role: "assistant", Content: "今天晴天"},
	}

	result := cleanToolCallPairs(messages)

	if len(result) != 4 {
		t.Fatalf("正常配对应保留 4 条消息，实际: %d", len(result))
	}
}

// TestCleanToolCallPairs_OrphanAssistantWithToolCalls 验证移除孤立的 assistant(tool_calls)
//
// 业务场景：assistant 发起 tool_calls 但对应的 tool response 丢失（被裁剪或未生成），
// 这种消息会导致 API 报错，需要清理。
func TestCleanToolCallPairs_OrphanAssistantWithToolCalls(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: "用户消息"},
		{
			Role:      "assistant", // 孤立 assistant（后面没有 tool 消息）
			ToolCalls: []llm.ToolCall{{ID: "tc-1", Function: llm.FunctionCall{Name: "search"}}},
		},
		{Role: "user", Content: "下一个用户消息"},
		{Role: "assistant", Content: "回复"},
	}

	result := cleanToolCallPairs(messages)

	if len(result) != 3 {
		t.Fatalf("期望 3 条消息（移除 1 个孤立 assistant），实际: %d", len(result))
	}
	if result[1].Role != "user" {
		t.Errorf("第二条应为 user，实际: %s", result[1].Role)
	}
}

// TestCleanToolCallPairs_EmptyMessages 验证空消息列表不报错
func TestCleanToolCallPairs_EmptyMessages(t *testing.T) {
	result := cleanToolCallPairs([]llm.ChatMessage{})
	if len(result) != 0 {
		t.Errorf("空消息列表应返回空，实际: %d 条", len(result))
	}
}

// TestCleanToolCallPairs_MultipleOrphanAssistants 验证连续多个孤立 assistant 被清理
func TestCleanToolCallPairs_MultipleOrphanAssistants(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: "用户消息"},
		{
			Role:      "assistant", // 孤立 1
			ToolCalls: []llm.ToolCall{{ID: "tc-1"}},
		},
		{
			Role:      "assistant", // 孤立 2
			ToolCalls: []llm.ToolCall{{ID: "tc-2"}},
		},
		{Role: "assistant", Content: "正常回复"},
	}

	result := cleanToolCallPairs(messages)

	if len(result) != 2 {
		t.Fatalf("期望 2 条消息（移除 2 个孤立 assistant），实际: %d", len(result))
	}
	if result[1].Role != "assistant" {
		t.Errorf("第二条应为 assistant，实际: %s", result[1].Role)
	}
	if len(result[1].ToolCalls) != 0 {
		t.Errorf("保留的 assistant 不应有 ToolCalls，实际: %d", len(result[1].ToolCalls))
	}
}

// TestCleanToolCallPairs_ToolAfterNonTool 验证 tool 消息在中间位置不被删除
//
// 业务场景：user → assistant(tool_calls) → tool → assistant → tool（孤立？）
// 只有开头的 tool 会被删除，中间的 tool 不会被删除（因为它前面有对应的 assistant）
func TestCleanToolCallPairs_ToolInMiddle(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: "用户消息"},
		{
			Role:      "assistant",
			ToolCalls: []llm.ToolCall{{ID: "tc-1"}},
		},
		{Role: "tool", ToolCallID: "tc-1", Content: "result 1"},
		{Role: "assistant", Content: "回复"},
	}

	result := cleanToolCallPairs(messages)

	if len(result) != 4 {
		t.Fatalf("中间的 tool 不应被删除，期望 4 条，实际: %d", len(result))
	}
}
