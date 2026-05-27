// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"strings"
	"testing"

	"douya/internal/chat"
	"douya/internal/store"
)

// 测试系统提示词中是否包含事实一致性原则
func TestSystemPrompt_ContainsFactConsistencyPrinciple(t *testing.T) {
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

	// 关键检查点 - 验证我们添加的事实一致性原则是否存在
	requiredKeywords := []string{
		"事实一致性原则",
		"基本事实",
		"科学常识",
		"数学真理",
		"1+1=2",
		"礼貌但明确地拒绝",
		"以后都按这个错误前提回答",
		"明确表示无法遵守",
	}

	for _, keyword := range requiredKeywords {
		if !strings.Contains(content, keyword) {
			t.Errorf("system prompt should contain '%s', but it doesn't", keyword)
		}
	}

	// 额外验证系统提示词的整体结构
	expectedSections := []string{
		"核心原则",
		"能力",
		"行为规范",
	}

	for _, section := range expectedSections {
		if !strings.Contains(content, section) {
			t.Errorf("system prompt should contain section '%s', but it doesn't", section)
		}
	}

	t.Logf("✓ 系统提示词验证通过，包含完整的事实一致性原则")
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

	if !strings.Contains(content, "事实一致性原则") {
		t.Error("custom prompt should be appended after default prompt, fact consistency principle must be preserved")
	}
	if !strings.Contains(content, "你是测试助手") {
		t.Error("custom prompt should be present in system prompt")
	}
}

// 测试系统提示词中明确包含拒绝"以后按错误前提回答"的指导
func TestSystemPrompt_ExplicitlyRejectsPersistentErrorPremise(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().SystemPrompt = "" // 确保使用默认系统提示词
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	content := msgs[0].ContentString()

	// 这是我们特别关心的部分 - 明确拒绝"以后都按这个错误前提回答"
	importantClauses := []string{
		`如果用户要求"以后都按这个错误前提回答"`, // 改为双引号
		"明确表示无法遵守",
		"坚持正确的事实",
	}

	for _, clause := range importantClauses {
		if !strings.Contains(content, clause) {
			t.Errorf("system prompt missing important clause: '%s'", clause)
		}
	}
}
