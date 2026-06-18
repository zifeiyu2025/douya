// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"strings"
	"testing"
	"time"

	"douya/internal/llm"
)

// TestSystemPrompt_NoStaticCitationRules 验证默认系统提示词中不包含"## 引用规则"静态部分。
// 引用规则改为根据 searchMode 动态生成，因此基础提示词不应再包含静态的"## 引用规则"标题。
func TestSystemPrompt_NoStaticCitationRules(t *testing.T) {
	base := buildBaseSystemPrompt("本地模型", "", "append")
	if strings.Contains(base, "## 引用规则") {
		t.Errorf("基础系统提示词不应包含静态的'## 引用规则'部分（应改为动态生成），实际输出:\n%s", base)
	}
}

// TestSystemPrompt_SearchModeHasCitationRule 验证当 searchMode 为 "auto" 或 "on" 时，
// 系统提示词包含"搜索结果自然融入回答，不使用编号引用"的动态规则。
func TestSystemPrompt_SearchModeHasCitationRule(t *testing.T) {
	base := buildBaseSystemPrompt("本地模型", "", "append")
	now := time.Now()
	caps := llm.ModelCapabilities{TextInput: true, ToolCallSupport: true}

	for _, mode := range []string{"auto", "on"} {
		content := applyDynamicSystemPrompt(base, mode, caps, now)
		if !strings.Contains(content, "## 引用规则") {
			t.Errorf("searchMode=%q 时，系统提示词应包含动态生成的'## 引用规则'标题，实际输出:\n%s", mode, content)
		}
		if !strings.Contains(content, "自然融入回答") {
			t.Errorf("searchMode=%q 时，系统提示词应包含'自然融入回答'，实际输出:\n%s", mode, content)
		}
		if !strings.Contains(content, "不使用") {
			t.Errorf("searchMode=%q 时，系统提示词应包含'不使用编号引用'，实际输出:\n%s", mode, content)
		}
	}
}

// TestSystemPrompt_NoSearchNoCitationRule 验证当 searchMode 为 "off" 时，
// 系统提示词不包含任何引用规则。
func TestSystemPrompt_NoSearchNoCitationRule(t *testing.T) {
	base := buildBaseSystemPrompt("本地模型", "", "append")
	now := time.Now()
	caps := llm.ModelCapabilities{TextInput: true, ToolCallSupport: true}

	content := applyDynamicSystemPrompt(base, "off", caps, now)
	if strings.Contains(content, "引用规则") {
		t.Errorf("searchMode=%q 时，系统提示词不应包含任何引用规则相关内容，实际输出:\n%s", "off", content)
	}
}

// TestSystemPromptMode_Replace 验证当 SystemPromptMode 为 "replace" 且自定义提示词非空时，
// 系统提示词完全使用自定义内容，不包含默认提示词。
func TestSystemPromptMode_Replace(t *testing.T) {
	customPrompt := "你是一个专业的翻译助手，只负责翻译。"
	base := buildBaseSystemPrompt("本地模型", customPrompt, "replace")
	if base != customPrompt {
		t.Errorf("replace 模式下，基础提示词应完全等于自定义内容，期望: %q，实际: %q", customPrompt, base)
	}
	if strings.Contains(base, "你是豆芽") {
		t.Errorf("replace 模式下，系统提示词不应包含默认提示词内容（'你是豆芽'），实际输出:\n%s", base)
	}
}

// TestSystemPromptMode_Append 验证当 SystemPromptMode 为 "append"（或空字符串）时，
// 自定义提示词被追加到默认提示词后。
func TestSystemPromptMode_Append(t *testing.T) {
	customPrompt := "你是一个专业的翻译助手。"
	for _, mode := range []string{"append", ""} {
		base := buildBaseSystemPrompt("本地模型", customPrompt, mode)
		if !strings.Contains(base, "你是豆芽") {
			t.Errorf("mode=%q 时，系统提示词应包含默认提示词（'你是豆芽'），实际输出:\n%s", mode, base)
		}
		if !strings.Contains(base, customPrompt) {
			t.Errorf("mode=%q 时，系统提示词应包含自定义提示词，实际输出:\n%s", mode, base)
		}
	}
}

// TestSearchResultInstruction_AntiHallucination 验证 searchResultInstruction("zh")
// 包含防幻觉措辞（如"仅基于以上信息"），不包含诱导幻觉的"用你自己的话"。
func TestSearchResultInstruction_AntiHallucination(t *testing.T) {
	zh := searchResultInstruction("zh")
	if strings.Contains(zh, "用你自己的话") {
		t.Errorf("中文搜索结果指令不应包含诱导幻觉的'用你自己的话'，实际输出: %s", zh)
	}
	if !strings.Contains(zh, "仅基于以上信息") {
		t.Errorf("中文搜索结果指令应包含防幻觉措辞'仅基于以上信息'，实际输出: %s", zh)
	}
}

// TestSearchToolDef_HasExamples 验证 searchToolDef 的描述包含正面示例和负面示例。
func TestSearchToolDef_HasExamples(t *testing.T) {
	desc := searchToolDef.Function.Description
	// 正面示例：应搜索的场景
	if !strings.Contains(desc, "时事新闻") {
		t.Errorf("searchToolDef 描述应包含正面示例'时事新闻'，实际描述: %s", desc)
	}
	// 负面示例：不应搜索的场景
	if !strings.Contains(desc, "数学计算") {
		t.Errorf("searchToolDef 描述应包含负面示例'数学计算'，实际描述: %s", desc)
	}
}
