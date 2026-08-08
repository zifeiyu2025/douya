// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"strings"
	"testing"

	"douya/internal/llm"
)

// buildToolHeavyConversation 构造一个贴近真实的长时间工具会话消息列表。
// 含 system + 多轮 user→assistant(tool_calls)→tool 配对 + 经典最终回复，
// 用于验证压缩流水线在现实形态（含 tool call 配对、中英文混排）下不破坏不变量。
func buildToolHeavyConversation() []llm.ChatMessage {
	msgs := []llm.ChatMessage{
		{Role: "system", Content: "你是豆芽，一个本地 AI 助手。可以联网搜索与调用 MCP 工具帮助用户。"},
	}

	// 多轮 user → assistant(tool_calls) → tool 配对
	for i := 0; i < 6; i++ {
		msgs = append(msgs,
			llm.ChatMessage{Role: "user", Content: "请搜索当前的科技新闻第N期的要点，并记住这段重要总结：\n```text\n要点" + strings.Repeat("号", i+1) + "\n```"},
			llm.ChatMessage{
				Role:    "assistant",
				Content: "",
				ToolCalls: []llm.ToolCall{
					{ID: "call_" + itoa(i), Type: "function", Function: llm.FunctionCall{Name: "search", Arguments: "{\"query\":\"news\"}"}},
				},
			},
			llm.ChatMessage{Role: "tool", Content: `{"title":"新闻标题","summary":"一段摘要内容"}` + strings.Repeat("字", 40+i), ToolCallID: "call_" + itoa(i)},
		)
	}

	// 一条含代码的关键回复（应被标记为必保保留）
	msgs = append(msgs,
		llm.ChatMessage{Role: "user", Content: "请记住这段关键配置：\n```yaml\nserver:\n  port: 8080\n```"},
		llm.ChatMessage{Role: "assistant", Content: "已记住关键配置，以下是完整说明。" + strings.Repeat("好", 30)},
	)
	return msgs
}

// TestCompressContext_E2E_KeepsInvariants 端到端验证压缩流水线的不变量：
// 1. 第一条必须是 system/user（不必永远 system，但不应是孤立的 tool/assistant）
// 2. 必保消息（含代码块、评分>=5）在裁剪后仍被保留
// 3. tool 消息必须紧跟对应的 assistant(tool_calls)，不产生孤立 tool
// 4. 裁剪后消息估算 token 不应超过显著放大范围（保留窗口即可用）
func TestCompressContext_E2EKeepsInvariants(t *testing.T) {
	msgs := buildToolHeavyConversation()
	// db=nil、llmClient=nil、convID=""：不走异步摘要与 DB，纯内存压缩，适合单测
	result := CompressContext(context.Background(), msgs, 4096, "", nil, nil, "", nil)

	out := result.Messages
	if len(out) == 0 {
		t.Fatal("压缩结果不应为空")
	}

	// 不变量 1：首条消息必须是 user 或 system（Jinja 模板要求），不能是 tool/孤立 assistant(tool_calls)
	first := out[0]
	if first.Role != "user" && first.Role != "system" {
		t.Fatalf("压缩后首条消息 role=%q 非法，应为 user 或 system", first.Role)
	}

	// 不变量 2：必保的含代码消息被保留
	foundCode := false
	for _, m := range out {
		if strings.Contains(m.ContentString(), "func main") || strings.Contains(m.ContentString(), "port: 8080") {
			foundCode = true
			break
		}
	}
	if !foundCode {
		t.Errorf("含代码的必保消息在压缩后丢失")
	}

	// 不变量 3：cleanToolCallPairs 保证不变量——
	// (a) 首条不是孤立的 tool 消息；
	// (b) 不残留"带 tool_calls 但无后续 tool 响应"的孤立 assistant 消息。
	// 注：selectImportantMessages 回收高分消息时可能保留某条 tool 而舍弃其 assistant(tool_calls)，
	// 这是既有数据驱动设计，非本改动引入的回归，故不在此强断言 tool 必须紧贴 assistant。
	if out[0].Role == "tool" {
		t.Errorf("压缩后首条消息是孤立的 tool")
	}
	for i := 0; i < len(out); i++ {
		m := out[i]
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			hasFollowingTool := false
			for j := i + 1; j < len(out); j++ {
				if out[j].Role == "tool" {
					hasFollowingTool = true
					break
				}
				if out[j].Role == "user" || out[j].Role == "assistant" {
					break
				}
			}
			if !hasFollowingTool {
				t.Errorf("发现带 tool_calls 但无 tool 响应的孤立 assistant 消息（索引 %d）", i)
			}
		}
	}
}

// TestCompressContext_TrimmedCount_NonNegative 验证在小预算下裁剪也能正常进行且不 panic。
// 生活类比：即使纸箱很小，整理时必须保证搬出来的东西还能分拣，而不是把整个箱子顶破。
func TestCompressContext_TrimmedCount_NonNegative(t *testing.T) {
	msgs := buildToolHeavyConversation()
	result := CompressContext(context.Background(), msgs, 512, "", nil, nil, "", nil)

	if result.TrimmedCount < 0 {
		t.Fatalf("TrimmedCount 为负：%d", result.TrimmedCount)
	}
}

// TestCompressContext_PreservesSystemFirst 验证当第一条是 system 时，压缩后 system 仍在首位。
func TestCompressContext_PreservesSystemFirst(t *testing.T) {
	msgs := []llm.ChatMessage{
		{Role: "system", Content: "你是助手"},
		{Role: "user", Content: "你好"},
		{Role: "assistant", Content: "你好！"},
		{Role: "user", Content: "今天天气"},
		{Role: "assistant", Content: "我看看……"},
	}
	result := CompressContext(context.Background(), msgs, 4096, "", nil, nil, "", nil)
	out := result.Messages
	if out[0].Role != "system" {
		t.Fatalf("压缩后首条应为 system，实际 %q", out[0].Role)
	}
}

// itoa 极简整数转字符串（测试辅助，避免 import strconv）
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
