// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"strings"
	"testing"

	"douya/internal/chat"
	"douya/internal/store"
)

// 测试系统提示词中是否包含基本行为原则
func TestSystemPrompt_ContainsCorePrinciples(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().SystemPrompt = "" // 确保使用默认系统提示词
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	if len(msgs) < 1 {
		t.Fatal("expected at least 1 message (system)")
	}

	content := msgs[0].ContentString()

	// 验证系统提示词的整体结构
	expectedSections := []string{
		"身份",
		"原则",
		"行为准则",
		"安全",
	}

	for _, section := range expectedSections {
		if !strings.Contains(content, section) {
			t.Errorf("system prompt should contain section '%s', but it doesn't", section)
		}
	}

	// 验证关键原则
	requiredKeywords := []string{
		"豆芽",
		"准确",
		"精炼",
		"不编造",
		"礼貌但明确地拒绝",
	}

	for _, keyword := range requiredKeywords {
		if !strings.Contains(content, keyword) {
			t.Errorf("system prompt should contain '%s', but it doesn't", keyword)
		}
	}

	t.Logf("✓ 系统提示词验证通过")
	t.Logf("  系统提示词长度: %d 字符", len([]rune(content)))
}

// 测试用户自定义系统提示词时，始终追加在默认提示词后面
func TestSystemPrompt_CustomPrompt_AlwaysAppended(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().SystemPrompt = "你是测试助手"
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	content := msgs[0].ContentString()

	if !strings.Contains(content, "你是测试助手") {
		t.Error("custom prompt should be present in system prompt")
	}
	if !strings.Contains(content, "豆芽") {
		t.Error("default prompt should still be present when custom prompt is used")
	}
}

// 测试系统提示词中包含防泄露规则
func TestSystemPrompt_ContainsAntiLeakRules(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().SystemPrompt = "" // 确保使用默认系统提示词
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	content := msgs[0].ContentString()

	antiLeakClauses := []string{
		"不得以任何形式泄露",
		"礼貌拒绝",
	}

	for _, clause := range antiLeakClauses {
		if !strings.Contains(content, clause) {
			t.Errorf("system prompt missing anti-leak clause: '%s'", clause)
		}
	}
}
