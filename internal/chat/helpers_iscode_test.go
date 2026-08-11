// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"encoding/json"
	"testing"

	"douya/internal/llm"
)

// TestIsCodeRelated 验证代码相关问题检测
// 生活类比：像图书管理员判断读者问的是不是技术书，通过关键词和"代码味"来识别。
func TestIsCodeRelated(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		// 代码语法关键词
		{"函数定义", "func main() {}", true},
		{"Python定义", "def hello():", true},
		{"类定义", "class Foo:", true},
		{"import语句", "import os", true},
		{"return语句", "return result", true},
		{"注释块", "/* 这是注释 */", true},

		// 英文关键词
		{"Python关键词", "how to use python", true},
		{"JavaScript关键词", "javascript error", true},
		{"debug关键词", "help me debug this", true},
		{"API关键词", "design an api", true},
		{"数据库关键词", "sql query optimization", true},
		{"Docker关键词", "docker build failed", true},

		// 中文关键词
		{"中文代码", "帮我写一段代码", true},
		{"中文函数", "这个函数有问题", true},
		{"中文调试", "调试一下程序", true},
		{"中文报错", "运行报错了", true},
		{"中文框架", "用什么框架好", true},

		// 编程字符计数
		{"编程字符多", "a() { b(); c(); d(); }", true},

		// 非代码
		{"日常问题", "今天天气怎么样", false},
		{"闲聊", "你好，你是谁", false},
		{"空字符串", "", false},
		{"纯文本", "讲个故事吧", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isCodeRelated(tc.query)
			if got != tc.want {
				t.Errorf("isCodeRelated(%q) = %v, want %v", tc.query, got, tc.want)
			}
		})
	}
}

// TestDetectLanguage 验证语言检测
func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"纯中文", "你好世界", "zh"},
		{"纯英文", "Hello World", "en"},
		{"混合中英文", "Hello 你好", "zh"},
		{"空字符串", "", "en"},
		{"日文_非中文", "こんにちは", "en"}, // 日文不在检测范围
		{" emoji", "😀😁😂", "en"},
		{"中文标点", "，。！？", "en"}, // 标点不是中文字符
		{"混合代码", "func() 你好", "zh"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := detectLanguage(tc.content)
			if got != tc.want {
				t.Errorf("detectLanguage(%q) = %q, want %q", tc.content, got, tc.want)
			}
		})
	}
}

// TestCalcSlidingWindowSize 验证滑动窗口大小计算
// 不同 contextSize 对应不同的窗口大小
func TestCalcSlidingWindowSize(t *testing.T) {
	tests := []struct {
		name        string
		contextSize int
		want        int
	}{
		{"零_使用默认4096", 0, 6},
		{"负数_使用默认4096", -1, 6},
		{"极小值", 100, 6},
		{"4096", 4096, 6},
		{"8192_边界", 8192, 6},
		{"8193_进入中档", 8193, 12},
		{"16384", 16384, 12},
		{"32767_中档上限", 32767, 12},
		{"32768_进入高档", 32768, 20},
		{"65536", 65536, 20},
		{"超大规模", 1000000, 20},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := CalcSlidingWindowSize(tc.contextSize)
			if got != tc.want {
				t.Errorf("CalcSlidingWindowSize(%d) = %d, want %d", tc.contextSize, got, tc.want)
			}
		})
	}
}

// TestCleanToolCallPairs 验证工具调用消息对的清理
// 生活类比：像整理对话记录，把没有回应的工具提问和没有提问的工具回答都清理掉。
func TestCleanToolCallPairs(t *testing.T) {
	tests := []struct {
		name     string
		messages []llm.ChatMessage
		wantLen  int
		desc     string
	}{
		{
			name:     "空列表",
			messages: []llm.ChatMessage{},
			wantLen:  0,
			desc:     "空列表原样返回",
		},
		{
			name: "正常对话_无工具调用",
			messages: []llm.ChatMessage{
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "你好！"},
			},
			wantLen: 2,
			desc:    "无工具调用，原样返回",
		},
		{
			name: "开头的孤立tool消息_被删除",
			messages: []llm.ChatMessage{
				{Role: "tool", Content: "孤立工具结果"},
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "你好！"},
			},
			wantLen: 2,
			desc:    "开头的 tool 消息应被删除",
		},
		{
			name: "多个开头孤立tool_全删除",
			messages: []llm.ChatMessage{
				{Role: "tool", Content: "结果1"},
				{Role: "tool", Content: "结果2"},
				{Role: "user", Content: "你好"},
			},
			wantLen: 1,
			desc:    "多个开头 tool 消息全删除",
		},
		{
			name: "完整工具调用对_保留",
			messages: []llm.ChatMessage{
				{Role: "user", Content: "搜索天气"},
				{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Function: llm.FunctionCall{Name: "search"}}}},
				{Role: "tool", Content: "晴天"},
				{Role: "assistant", Content: "今天天气晴朗"},
			},
			wantLen: 4,
			desc:    "完整的工具调用对应保留",
		},
		{
			name: "孤立assistant带toolcalls_被删除",
			messages: []llm.ChatMessage{
				{Role: "user", Content: "你好"},
				{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Function: llm.FunctionCall{Name: "search"}}}},
				{Role: "user", Content: "下一个问题"},
			},
			wantLen: 2,
			desc:    "孤立 assistant+toolcalls 应被删除",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := cleanToolCallPairs(tc.messages)
			if len(got) != tc.wantLen {
				t.Errorf("cleanToolCallPairs: %s；got len=%d, want %d；messages=%+v",
					tc.desc, len(got), tc.wantLen, got)
			}
		})
	}
}

// TestEnsureStartsWithUserOrSystem 验证消息列表以 user 或 system 开头
// 源码实现：删除开头的非 user/system 消息，直到遇到 user 或 system
func TestEnsureStartsWithUserOrSystem(t *testing.T) {
	tests := []struct {
		name      string
		messages  []llm.ChatMessage
		wantFirst string
		wantLen   int
	}{
		{
			name:      "空列表",
			messages:  []llm.ChatMessage{},
			wantFirst: "",
			wantLen:   0,
		},
		{
			name: "已以system开头",
			messages: []llm.ChatMessage{
				{Role: "system", Content: "系统提示"},
				{Role: "user", Content: "你好"},
			},
			wantFirst: "system",
			wantLen:   2,
		},
		{
			name: "已以user开头",
			messages: []llm.ChatMessage{
				{Role: "user", Content: "你好"},
			},
			wantFirst: "user",
			wantLen:   1,
		},
		{
			name: "以assistant开头_删除到空",
			messages: []llm.ChatMessage{
				{Role: "assistant", Content: "你好"},
			},
			wantFirst: "",
			wantLen:   0,
		},
		{
			name: "以tool开头_删除到遇到user",
			messages: []llm.ChatMessage{
				{Role: "tool", Content: "结果"},
				{Role: "user", Content: "你好"},
			},
			wantFirst: "user",
			wantLen:   1,
		},
		{
			name: "多个非user开头_全删除",
			messages: []llm.ChatMessage{
				{Role: "tool", Content: "结果1"},
				{Role: "assistant", Content: "回复"},
				{Role: "user", Content: "你好"},
			},
			wantFirst: "user",
			wantLen:   1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureStartsWithUserOrSystem(tc.messages)
			if len(got) != tc.wantLen {
				t.Errorf("len = %d, want %d", len(got), tc.wantLen)
				return
			}
			if tc.wantFirst != "" && len(got) > 0 && got[0].Role != tc.wantFirst {
				t.Errorf("first role = %q, want %q", got[0].Role, tc.wantFirst)
			}
		})
	}
}

// TestEstimateChatMessageTokens_ToolCallNotZero 验证带 ToolCall 的消息估算结果不为 0
// PERF-3: 优化后不再使用 json.Marshal，需确保仍能正确给出非零估算值
// 生活类比：快递单上即使只写了物品名称没写包装，称重时也不能显示 0 克
func TestEstimateChatMessageTokens_ToolCallNotZero(t *testing.T) {
	msg := llm.ChatMessage{
		Role:    "assistant",
		Content: "",
		ToolCalls: []llm.ToolCall{
			{ID: "call_1", Type: "function", Function: llm.FunctionCall{Name: "search", Arguments: `{"query":"今天天气怎么样"}`}},
		},
	}
	got := estimateChatMessageTokens(msg)
	if got <= 0 {
		t.Fatalf("带 ToolCall 的消息估算应 > 0，实际: %d", got)
	}
	// 至少应包含 chat template 开销(10) + ToolCall 结构开销(8) + name/arguments 的 token
	if got < 18 {
		t.Errorf("带 ToolCall 的消息估算过小: %d，期望至少 18", got)
	}
}

// TestEstimateChatMessageTokens_ToolCallCloseToMarshal 验证字段长度估算与原 JSON Marshal 估算结果接近
// PERF-3: 优化前后估算结果差异应 < 20%，确保精度不会因去掉 Marshal 而大幅偏离
// 生活类比：用清单估算的重量和实际打包后称的重量不能差太多，否则估算就失去意义
func TestEstimateChatMessageTokens_ToolCallCloseToMarshal(t *testing.T) {
	// 构造若干典型 ToolCall 场景：英文参数、中文参数、长参数
	cases := []struct {
		name string
		tc   llm.ToolCall
	}{
		{
			name: "英文短参数",
			tc:   llm.ToolCall{ID: "call_1", Type: "function", Function: llm.FunctionCall{Name: "search", Arguments: `{"query":"weather"}`}},
		},
		{
			name: "中文参数",
			tc:   llm.ToolCall{ID: "call_2", Type: "function", Function: llm.FunctionCall{Name: "search", Arguments: `{"query":"今天天气怎么样"}`}},
		},
		{
			name: "长参数",
			tc:   llm.ToolCall{ID: "call_3", Type: "function", Function: llm.FunctionCall{Name: "execute", Arguments: `{"code":"print('hello world')\nfor i in range(10):\n    print(i)"}`}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 优化前：用 json.Marshal 估算
			b, _ := json.Marshal(c.tc)
			oldLang := detectLanguage(string(b))
			oldTokens := estimateTokensByLang(string(b), oldLang)

			// 优化后：用字段拼接模拟 JSON Marshal 输出（与 estimateChatMessageTokens 实现一致）
			text := `{"index":0,"id":"` + c.tc.ID + `","type":"` + c.tc.Type + `","function":{"name":"` + c.tc.Function.Name + `","arguments":"` + c.tc.Function.Arguments + `"}}`
			newLang := detectLanguage(text)
			newTokens := estimateTokensByLang(text, newLang)

			if oldTokens == 0 {
				t.Fatalf("oldTokens 不应为 0")
			}
			// 差异比例（取绝对值）
			var diff float64
			if newTokens > oldTokens {
				diff = float64(newTokens-oldTokens) / float64(oldTokens)
			} else {
				diff = float64(oldTokens-newTokens) / float64(oldTokens)
			}
			if diff > 0.20 {
				t.Errorf("估算差异过大: old=%d, new=%d, diff=%.2f（期望 < 20%%）", oldTokens, newTokens, diff)
			}
		})
	}
}
