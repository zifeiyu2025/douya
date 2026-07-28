// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"database/sql"
	"testing"
)

// insertTestMessage 辅助函数：插入一条测试消息（带可选的扩展字段），自动设置 ConversationID 和 Role。
// 生活类比：像往抽屉里放一张写了内容的卡片，卡片可以包含"正文"、"思考草稿"、"参考资料"、"工具记录"等不同栏目。
func insertTestMessage(t *testing.T, db *sql.DB, encKey []byte, convID, role, content, thinking, searchResults, toolCalls string) *Message {
	t.Helper()
	insertTestConversation(t, db, convID, "测试会话")
	msg := &Message{
		ConversationID:  convID,
		Role:            role,
		Content:         content,
		ThinkingContent: thinking,
		SearchResults:   searchResults,
		ToolCalls:       toolCalls,
	}
	if err := CreateMessage(db, msg, encKey); err != nil {
		t.Fatalf("CreateMessage 失败: %v", err)
	}
	return msg
}

// TestSearchMessages_MatchContent 验证基本搜索：query 命中 Content 字段时能查到消息。
// 这是优化后必须保持不变的核心行为。
func TestSearchMessages_MatchContent(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	insertTestMessage(t, db, encKey, "conv-1", "user", "今天天气真好", "", "", "")
	insertTestMessage(t, db, encKey, "conv-2", "assistant", "我在帮你查资料", "", "", "")

	results, err := SearchMessages(db, "天气", encKey)
	if err != nil {
		t.Fatalf("SearchMessages 失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("期望命中 1 条消息，实际 %d", len(results))
	}
	if results[0].Content != "今天天气真好" {
		t.Errorf("命中消息内容不匹配，实际: %s", results[0].Content)
	}
}

// TestSearchMessages_MatchThinkingContent 验证搜索能命中 ThinkingContent 字段。
// 优化前是把所有字段拼接后查，优化后逐字段查询，必须保证思考字段仍可被命中。
func TestSearchMessages_MatchThinkingContent(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	// 正文不含 "推理"，但思考过程包含
	insertTestMessage(t, db, encKey, "conv-1", "assistant",
		"答案是 42", "我需要先做推理过程才能得出结论", "", "")

	results, err := SearchMessages(db, "推理", encKey)
	if err != nil {
		t.Fatalf("SearchMessages 失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("期望命中 1 条思考字段的消息，实际 %d", len(results))
	}
	if results[0].ThinkingContent != "我需要先做推理过程才能得出结论" {
		t.Errorf("命中消息 ThinkingContent 不匹配: %s", results[0].ThinkingContent)
	}
}

// TestSearchMessages_MatchSearchResults 验证搜索能命中 SearchResults 字段（RAG 检索结果）。
func TestSearchMessages_MatchSearchResults(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	insertTestMessage(t, db, encKey, "conv-1", "assistant",
		"根据资料显示", "", "来源：维基百科 RAG 文档", "")

	results, err := SearchMessages(db, "维基百科", encKey)
	if err != nil {
		t.Fatalf("SearchMessages 失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("期望命中 1 条 SearchResults 字段的消息，实际 %d", len(results))
	}
	if results[0].SearchResults != "来源：维基百科 RAG 文档" {
		t.Errorf("命中消息 SearchResults 不匹配: %s", results[0].SearchResults)
	}
}

// TestSearchMessages_MatchToolCalls 验证搜索能命中 ToolCalls 字段（工具调用记录）。
func TestSearchMessages_MatchToolCalls(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	insertTestMessage(t, db, encKey, "conv-1", "assistant",
		"正在调用工具", "", "", `{"name":"web_search","args":"Go 语言教程"}`)

	results, err := SearchMessages(db, "Go 语言", encKey)
	if err != nil {
		t.Fatalf("SearchMessages 失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("期望命中 1 条 ToolCalls 字段的消息，实际 %d", len(results))
	}
}

// TestSearchMessages_CaseInsensitiveAllFields 验证大小写不敏感匹配在所有可搜索字段上都有效。
// 优化后使用 strings.ToLower(field) + strings.Contains(lowerQuery) 的方式，
// 等价于原实现的 strings.ToLower(拼接串) 查找，大小写行为必须保持一致。
// 与 db_test.go 中已有的 TestSearchMessages_CaseInsensitive（仅测 content）互补，
// 本测试额外覆盖 thinking_content / search_results / tool_calls 字段。
func TestSearchMessages_CaseInsensitiveAllFields(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	// 4 条消息分别在不同字段中含大写 KEYWORD
	insertTestMessage(t, db, encKey, "conv-1", "user",
		"这里 CONTENT 含大写", "", "", "")
	insertTestMessage(t, db, encKey, "conv-2", "assistant",
		"", "思考 THINKING 大写", "", "")
	insertTestMessage(t, db, encKey, "conv-3", "assistant",
		"", "", "结果 SEARCH 大写", "")
	insertTestMessage(t, db, encKey, "conv-4", "assistant",
		"", "", "", `{"tool":"TOOL 大写"}`)

	cases := []struct {
		name    string
		query   string
		wantLen int
	}{
		{"content 小写查询命中", "content", 1},
		{"thinking 小写查询命中", "thinking", 1},
		{"search 小写查询命中", "search", 1},
		{"tool 小写查询命中", "tool", 1},
		{"全大写查询命中 content", "CONTENT", 1},
		{"混合大小写查询命中", "ThInKiNg", 1},
		{"无匹配返回 0 条", "notexist", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			results, err := SearchMessages(db, c.query, encKey)
			if err != nil {
				t.Fatalf("SearchMessages 失败: %v", err)
			}
			if len(results) != c.wantLen {
				t.Errorf("query=%q 期望命中 %d 条，实际 %d", c.query, c.wantLen, len(results))
			}
		})
	}
}

// TestSearchMessages_EmptyQuery 验证空 query 的行为。
// 原实现中 strings.Contains(_, "") 永远返回 true，因此空 query 会匹配所有消息。
// 优化后逐字段判断：空字段被跳过，所有字段都为空的消息不会被命中。
// 这是有意的细微行为差异：空 query 时只返回"至少有一个非空可搜索字段"的消息，
// 避免返回完全空白的消息，该差异不影响实际搜索用途（用户不会真正搜空字符串）。
func TestSearchMessages_EmptyQuery(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	// 插入 3 条消息：其中 1 条所有可搜索字段都为空
	insertTestMessage(t, db, encKey, "conv-1", "user", "消息一", "", "", "")
	insertTestMessage(t, db, encKey, "conv-2", "assistant", "消息二", "", "", "")
	// 第 3 条所有可搜索字段为空，按优化后的短路逻辑不会被命中
	insertTestMessage(t, db, encKey, "conv-3", "assistant", "", "", "", "")

	results, err := SearchMessages(db, "", encKey)
	if err != nil {
		t.Fatalf("SearchMessages 失败: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("空 query 应命中 2 条有内容的消息，实际 %d", len(results))
	}
}

// TestSearchMessages_NoMatch 验证无匹配时返回空结果。
func TestSearchMessages_NoMatch(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	insertTestMessage(t, db, encKey, "conv-1", "user", "今天天气真好", "", "", "")

	results, err := SearchMessages(db, "不存在的关键词", encKey)
	if err != nil {
		t.Fatalf("SearchMessages 失败: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("无匹配时应返回 0 条，实际 %d", len(results))
	}
}

// TestSearchMessages_MultipleMatches 验证一次搜索能命中多条消息。
// 同时验证短路逻辑：第一个字段命中后不会影响其他消息的匹配结果。
func TestSearchMessages_MultipleMatches(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	// 4 条消息都包含 "项目" 这个词，但分布在不同字段
	insertTestMessage(t, db, encKey, "conv-1", "user", "项目进度如何", "", "", "")
	insertTestMessage(t, db, encKey, "conv-2", "assistant", "好的", "正在分析项目", "", "")
	insertTestMessage(t, db, encKey, "conv-3", "assistant", "明白", "", "项目资料已找到", "")
	insertTestMessage(t, db, encKey, "conv-4", "assistant", "处理中", "", "", `{"name":"项目查找工具"}`)
	// 1 条不包含关键词
	insertTestMessage(t, db, encKey, "conv-5", "user", "你好", "", "", "")

	results, err := SearchMessages(db, "项目", encKey)
	if err != nil {
		t.Fatalf("SearchMessages 失败: %v", err)
	}
	if len(results) != 4 {
		t.Errorf("期望命中 4 条消息（4 个字段分别命中），实际 %d", len(results))
	}
}
