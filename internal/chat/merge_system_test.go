// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"reflect"
	"strings"
	"testing"

	"douya/internal/llm"
)

// TestMergeSystemIntoUser_BasicMerge 基本合并：一条 system + 一条 user
// 期望：system 内容合并到 user 前面，system 消息消失
func TestMergeSystemIntoUser_BasicMerge(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: "你是豆芽"},
		{Role: "user", Content: "你好"},
	}
	result := mergeSystemIntoUser(messages)

	if len(result) != 1 {
		t.Fatalf("期望 1 条消息（system 合并进 user），实际 %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("合并后应为 user 角色，实际 %s", result[0].Role)
	}
	content, ok := result[0].Content.(string)
	if !ok {
		t.Fatal("content 不是字符串")
	}
	if !strings.Contains(content, "你是豆芽") {
		t.Errorf("合并后应包含原 system 内容，实际: %q", content)
	}
	if !strings.Contains(content, "你好") {
		t.Errorf("合并后应包含原 user 内容，实际: %q", content)
	}
}

// TestMergeSystemIntoUser_MultipleSystem 多条 system 消息合并
// 期望：所有 system 内容用空行连接后合并到第一条 user 前
func TestMergeSystemIntoUser_MultipleSystem(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: "规则一"},
		{Role: "system", Content: "规则二"},
		{Role: "user", Content: "问题"},
		{Role: "assistant", Content: "回答"},
	}
	result := mergeSystemIntoUser(messages)

	if len(result) != 2 {
		t.Fatalf("期望 2 条消息（2 个 system 合并进 user，剩 user+assistant），实际 %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("第一条应为 user，实际 %s", result[0].Role)
	}
	content := result[0].Content.(string)
	if !strings.Contains(content, "规则一") || !strings.Contains(content, "规则二") {
		t.Errorf("应包含两条 system 内容，实际: %q", content)
	}
	if !strings.Contains(content, "问题") {
		t.Errorf("应包含原 user 内容，实际: %q", content)
	}
	if result[1].Role != "assistant" {
		t.Errorf("第二条应为 assistant，实际 %s", result[1].Role)
	}
}

// TestMergeSystemIntoUser_NoSystem 没有 system 消息时原样返回
func TestMergeSystemIntoUser_NoSystem(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "你好！"},
	}
	result := mergeSystemIntoUser(messages)

	if len(result) != 2 {
		t.Fatalf("没有 system 时应原样返回 2 条，实际 %d", len(result))
	}
	if result[0].Role != "user" || result[0].Content != "你好" {
		t.Errorf("第一条消息被修改了: %+v", result[0])
	}
}

// TestMergeSystemIntoUser_NoUser 没有 user 消息时，system 转成 user
// 场景：异常情况下消息列表只有 system（理论上不应出现，但要防御性处理）
func TestMergeSystemIntoUser_NoUser(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: "系统提示"},
	}
	result := mergeSystemIntoUser(messages)

	if len(result) != 1 {
		t.Fatalf("期望 1 条消息（system 转成 user），实际 %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("应转为 user 角色，实际 %s", result[0].Role)
	}
	if result[0].Content != "系统提示" {
		t.Errorf("内容应保持不变，实际: %v", result[0].Content)
	}
}

// TestMergeSystemIntoUser_EmptySystemContent 空 system 内容应被忽略
func TestMergeSystemIntoUser_EmptySystemContent(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: ""},
		{Role: "user", Content: "你好"},
	}
	result := mergeSystemIntoUser(messages)

	// 空 system 内容不应合并（避免在 user 前加无意义的空行）
	if len(result) != 1 {
		t.Fatalf("期望 1 条消息（空 system 被忽略），实际 %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("应为 user，实际 %s", result[0].Role)
	}
	content := result[0].Content.(string)
	if content != "你好" {
		t.Errorf("user 内容不应被修改，实际: %q", content)
	}
}

// TestMergeSystemIntoUser_PreservesMiddleMessages 合并不影响中间和后续消息
func TestMergeSystemIntoUser_PreservesMiddleMessages(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: "系统"},
		{Role: "user", Content: "问题1"},
		{Role: "assistant", Content: "回答1"},
		{Role: "user", Content: "问题2"},
		{Role: "assistant", Content: "回答2"},
	}
	result := mergeSystemIntoUser(messages)

	if len(result) != 4 {
		t.Fatalf("期望 4 条消息（system 合并进第一个 user），实际 %d", len(result))
	}
	// 第一条 user 应包含 system 内容
	firstContent := result[0].Content.(string)
	if !strings.Contains(firstContent, "系统") || !strings.Contains(firstContent, "问题1") {
		t.Errorf("第一条 user 应合并 system 内容，实际: %q", firstContent)
	}
	// 第二个 user 不应被改动
	secondUser := result[2]
	if secondUser.Role != "user" || secondUser.Content != "问题2" {
		t.Errorf("第二个 user 不应被修改: %+v", secondUser)
	}
}

// TestMergeSystemIntoUser_TypedContentContent user 是 typed content（非字符串）时的处理
// 场景：多模态消息的 content 是数组（如含图片），无法简单字符串合并
// 期望：在 user 消息前插入一条 user 消息承载 system 内容
func TestMergeSystemIntoUser_TypedContent(t *testing.T) {
	// 模拟 typed content（实际是 []map[string]any，这里用 []any 代替）
	typedContent := []any{"图片内容"}
	messages := []llm.ChatMessage{
		{Role: "system", Content: "系统提示"},
		{Role: "user", Content: typedContent},
	}
	result := mergeSystemIntoUser(messages)

	// 应插入一条 user 消息承载 system 内容，原 user 保留
	if len(result) != 2 {
		t.Fatalf("期望 2 条消息（插入 system user + 原 typed user），实际 %d", len(result))
	}
	if result[0].Role != "user" {
		t.Errorf("插入的第一条应为 user，实际 %s", result[0].Role)
	}
	if result[0].Content != "系统提示" {
		t.Errorf("插入的 user 应承载 system 内容，实际: %v", result[0].Content)
	}
	// 原 typed user 保留不变
	if !reflect.DeepEqual(result[1].Content, typedContent) {
		t.Errorf("原 typed user 内容不应被修改")
	}
}
