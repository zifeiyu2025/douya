// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"douya/internal/store"
)

// TestExportMarkdown 验证 Markdown 导出格式
// 生活类比：像把聊天记录整理成一份会议纪要，每条发言用标题分隔。
func TestExportMarkdown(t *testing.T) {
	s := &Service{}
	conv := &store.Conversation{ID: "c1", Title: "测试对话"}
	msgs := []*store.Message{
		{Role: "user", Content: "你好", CreatedAt: time.Now()},
		{Role: "assistant", Content: "你好！有什么可以帮你的吗？", CreatedAt: time.Now()},
	}

	got := s.exportMarkdown(conv, msgs)

	// 验证标题
	if !strings.Contains(got, "# 测试对话") {
		t.Errorf("Markdown 应包含标题 '# 测试对话'，实际: %q", got)
	}
	// 验证用户消息
	if !strings.Contains(got, "## 用户") {
		t.Errorf("Markdown 应包含 '## 用户'，实际: %q", got)
	}
	if !strings.Contains(got, "你好") {
		t.Errorf("Markdown 应包含用户消息内容，实际: %q", got)
	}
	// 验证助手消息
	if !strings.Contains(got, "## 助手") {
		t.Errorf("Markdown 应包含 '## 助手'，实际: %q", got)
	}
	if !strings.Contains(got, "你好！有什么可以帮你的吗？") {
		t.Errorf("Markdown 应包含助手消息内容，实际: %q", got)
	}
}

// TestExportMarkdown_EmptyMessages 验证空消息列表
func TestExportMarkdown_EmptyMessages(t *testing.T) {
	s := &Service{}
	conv := &store.Conversation{ID: "c1", Title: "空对话"}
	msgs := []*store.Message{}

	got := s.exportMarkdown(conv, msgs)

	if !strings.Contains(got, "# 空对话") {
		t.Errorf("即使消息为空，也应包含标题，实际: %q", got)
	}
}

// TestExportJSON 验证 JSON 导出格式
func TestExportJSON(t *testing.T) {
	s := &Service{}
	conv := &store.Conversation{ID: "c1", Title: "测试对话"}
	msgs := []*store.Message{
		{Role: "user", Content: "你好", CreatedAt: time.Now()},
		{Role: "assistant", Content: "你好！", CreatedAt: time.Now()},
	}

	got, err := s.exportJSON(conv, msgs)
	if err != nil {
		t.Fatalf("exportJSON 返回错误: %v", err)
	}

	// 验证是有效 JSON
	var parsed []map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("导出的不是有效 JSON: %v", err)
	}
	if len(parsed) != 2 {
		t.Errorf("应有 2 条消息，实际 %d", len(parsed))
	}
	if parsed[0]["role"] != "user" {
		t.Errorf("第一条消息 role 应为 user，实际: %v", parsed[0]["role"])
	}
	if parsed[1]["role"] != "assistant" {
		t.Errorf("第二条消息 role 应为 assistant，实际: %v", parsed[1]["role"])
	}
}

// TestExportJSON_EmptyMessages 验证空消息列表的 JSON 导出
func TestExportJSON_EmptyMessages(t *testing.T) {
	s := &Service{}
	conv := &store.Conversation{ID: "c1", Title: "空"}
	msgs := []*store.Message{}

	got, err := s.exportJSON(conv, msgs)
	if err != nil {
		t.Fatalf("exportJSON 返回错误: %v", err)
	}

	var parsed []map[string]any
	if err := json.Unmarshal([]byte(got), &parsed); err != nil {
		t.Fatalf("空列表应导出有效 JSON: %v", err)
	}
	if len(parsed) != 0 {
		t.Errorf("空消息应导出空数组，实际 %d 条", len(parsed))
	}
}

// TestExportPlainText 验证纯文本导出格式
func TestExportPlainText(t *testing.T) {
	s := &Service{}
	conv := &store.Conversation{ID: "c1", Title: "测试对话"}
	msgs := []*store.Message{
		{Role: "user", Content: "你好", CreatedAt: time.Now()},
		{Role: "assistant", Content: "你好！", CreatedAt: time.Now()},
	}

	got := s.exportPlainText(conv, msgs)

	// 验证标题
	if !strings.HasPrefix(got, "测试对话") {
		t.Errorf("纯文本应以标题开头，实际: %q", got[:min(len(got), 20)])
	}
	// 验证角色标记
	if !strings.Contains(got, "[用户]") {
		t.Errorf("应包含 [用户] 标记，实际: %q", got)
	}
	if !strings.Contains(got, "[助手]") {
		t.Errorf("应包含 [助手] 标记，实际: %q", got)
	}
}

// TestExportCSV 验证 CSV 导出格式
// CSV 格式：instruction,input,output
// 仅导出 user 消息，配对下一条 assistant 消息作为 output
func TestExportCSV(t *testing.T) {
	s := &Service{}
	conv := &store.Conversation{ID: "c1", Title: "测试"}
	msgs := []*store.Message{
		{Role: "user", Content: "你好", CreatedAt: time.Now()},
		{Role: "assistant", Content: "你好！", CreatedAt: time.Now()},
		{Role: "user", Content: "再见", CreatedAt: time.Now()},
		{Role: "assistant", Content: "再见！", CreatedAt: time.Now()},
	}

	got, err := s.exportCSV(conv, msgs)
	if err != nil {
		t.Fatalf("exportCSV 返回错误: %v", err)
	}

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// 1 行表头 + 2 行数据
	if len(lines) != 3 {
		t.Errorf("应有 3 行（1 表头 + 2 数据），实际 %d 行", len(lines))
	}
	// 验证表头
	if lines[0] != "instruction,input,output" {
		t.Errorf("表头应为 'instruction,input,output'，实际: %q", lines[0])
	}
}

// TestExportCSV_NoAssistant 验证 user 消息后无 assistant 配对时 output 为空
func TestExportCSV_NoAssistant(t *testing.T) {
	s := &Service{}
	conv := &store.Conversation{ID: "c1", Title: "测试"}
	msgs := []*store.Message{
		{Role: "user", Content: "你好", CreatedAt: time.Now()},
	}

	got, err := s.exportCSV(conv, msgs)
	if err != nil {
		t.Fatalf("exportCSV 返回错误: %v", err)
	}

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if len(lines) != 2 {
		t.Errorf("应有 2 行（1 表头 + 1 数据），实际 %d 行", len(lines))
	}
}

// TestExportCSV_ToolMessagesSkipped 验证 tool 消息被跳过
func TestExportCSV_ToolMessagesSkipped(t *testing.T) {
	s := &Service{}
	conv := &store.Conversation{ID: "c1", Title: "测试"}
	msgs := []*store.Message{
		{Role: "user", Content: "搜索天气", CreatedAt: time.Now()},
		{Role: "tool", Content: "搜索结果：晴天", CreatedAt: time.Now()},
		{Role: "assistant", Content: "今天天气晴朗", CreatedAt: time.Now()},
	}

	got, err := s.exportCSV(conv, msgs)
	if err != nil {
		t.Fatalf("exportCSV 返回错误: %v", err)
	}

	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	// 1 表头 + 1 数据（tool 消息被跳过，user 配对 assistant）
	if len(lines) != 2 {
		t.Errorf("应有 2 行（tool 被跳过），实际 %d 行", len(lines))
	}
}
