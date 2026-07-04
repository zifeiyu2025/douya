// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"encoding/json"
	"testing"

	"douya/internal/chat"
	"douya/internal/search"
)

// TestToolCallStartContent_ContainsToolCallID 验证 tool_call_start 事件 payload 包含 tool_call_id 字段。
//
// 业务场景：当 LLM 一次返回多个 tool call（parallel tool calls）时，前端需要通过 tool_call_id
// 区分每个 tool call 的开始/结果事件。修复前 payload 只有 tool 和 query，无法区分并发 tool call。
func TestToolCallStartContent_ContainsToolCallID(t *testing.T) {
	content := chat.ToolCallStartContent{
		ToolCallID: "tc-abc-123",
		Tool:       "search",
		Query:      "量子纠缠",
	}

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// 核心断言：payload 必须包含 tool_call_id
	if parsed["tool_call_id"] != "tc-abc-123" {
		t.Errorf("payload 应包含 tool_call_id='tc-abc-123'，实际: %v", parsed["tool_call_id"])
	}
	if parsed["tool"] != "search" {
		t.Errorf("payload 应包含 tool='search'，实际: %v", parsed["tool"])
	}
	if parsed["query"] != "量子纠缠" {
		t.Errorf("payload 应包含 query='量子纠缠'，实际: %v", parsed["query"])
	}
}

// TestSearchResultContent_ContainsToolCallID 验证 search_result 事件 payload 包含 tool_call_id 字段。
//
// 修复前 search_result 事件只发射 []search.SearchResult，前端无法知道结果属于哪个 tool call。
// 修复后使用 SearchResultContent struct，包含 tool_call_id + results，前端可正确关联。
func TestSearchResultContent_ContainsToolCallID(t *testing.T) {
	results := []search.SearchResult{
		{Title: "Test", URL: "http://example.com", Snippet: "Test snippet"},
	}
	content := chat.SearchResultContent{
		ToolCallID: "tc-xyz-789",
		Results:    results,
	}

	data, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	// 核心断言：payload 必须包含 tool_call_id
	if parsed["tool_call_id"] != "tc-xyz-789" {
		t.Errorf("payload 应包含 tool_call_id='tc-xyz-789'，实际: %v", parsed["tool_call_id"])
	}
	resultsArr, ok := parsed["results"].([]any)
	if !ok {
		t.Fatalf("payload 应包含 results 数组，实际类型: %T", parsed["results"])
	}
	if len(resultsArr) != 1 {
		t.Errorf("results 应有 1 条结果，实际: %d", len(resultsArr))
	}
}

// TestMergeSearchJSON 验证多个 tool call 的搜索结果能正确合并为一个 JSON 数组。
//
// 业务场景：LLM 一次返回 2 个 search tool call（查询 A 和查询 B），
// 修复前 LastSearchJSON 会被覆盖，最终只保存最后一个搜索结果。
// 修复后使用 MergeSearchJSON 聚合所有结果，持久化的 SearchResults 包含全部搜索结果。
func TestMergeSearchJSON(t *testing.T) {
	existing := `[{"title":"结果A1","url":"http://a1.com","snippet":"A1"},{"title":"结果A2","url":"http://a2.com","snippet":"A2"}]`
	newResults := `[{"title":"结果B1","url":"http://b1.com","snippet":"B1"}]`

	merged := chat.MergeSearchJSON(existing, newResults)

	var parsed []map[string]any
	if err := json.Unmarshal([]byte(merged), &parsed); err != nil {
		t.Fatalf("合并后的 JSON 应可解析为数组，解析失败: %v, merged=%s", err, merged)
	}

	if len(parsed) != 3 {
		t.Errorf("合并后应有 3 条结果（2+1），实际: %d", len(parsed))
	}

	// 验证包含所有原始结果
	titles := map[string]bool{}
	for _, r := range parsed {
		if title, ok := r["title"].(string); ok {
			titles[title] = true
		}
	}
	for _, expected := range []string{"结果A1", "结果A2", "结果B1"} {
		if !titles[expected] {
			t.Errorf("合并后应包含 %q，实际 titles: %v", expected, titles)
		}
	}
}

// TestMergeSearchJSON_EmptyExisting 验证 existing 为空时直接返回 new
func TestMergeSearchJSON_EmptyExisting(t *testing.T) {
	newResults := `[{"title":"B1","url":"http://b1.com","snippet":"B1"}]`

	merged := chat.MergeSearchJSON("", newResults)

	if merged != newResults {
		t.Errorf("existing 为空时应直接返回 new，期望: %s，实际: %s", newResults, merged)
	}
}

// TestMergeSearchJSON_EmptyNew 验证 new 为空时保持 existing 不变
func TestMergeSearchJSON_EmptyNew(t *testing.T) {
	existing := `[{"title":"A1","url":"http://a1.com","snippet":"A1"}]`

	merged := chat.MergeSearchJSON(existing, "")

	if merged != existing {
		t.Errorf("new 为空时应保持 existing 不变，期望: %s，实际: %s", existing, merged)
	}
}

// TestMergeSearchJSON_BothEmpty 验证两者都为空时返回空数组
func TestMergeSearchJSON_BothEmpty(t *testing.T) {
	merged := chat.MergeSearchJSON("", "")

	var parsed []map[string]any
	if err := json.Unmarshal([]byte(merged), &parsed); err != nil {
		t.Fatalf("两者都为空时应返回可解析的 JSON，解析失败: %v, merged=%s", err, merged)
	}
	if len(parsed) != 0 {
		t.Errorf("两者都为空时应返回空数组，实际长度: %d", len(parsed))
	}
}

// TestMergeSearchJSON_InvalidJSON 验证无效 JSON 输入的安全降级
func TestMergeSearchJSON_InvalidJSON(t *testing.T) {
	// existing 无效，new 有效 → 应返回 new
	merged := chat.MergeSearchJSON("invalid json", `[{"title":"B1"}]`)
	if merged != `[{"title":"B1"}]` {
		t.Errorf("existing 无效时应降级返回 new，实际: %s", merged)
	}

	// existing 有效，new 无效 → 应返回 existing
	merged = chat.MergeSearchJSON(`[{"title":"A1"}]`, "invalid json")
	if merged != `[{"title":"A1"}]` {
		t.Errorf("new 无效时应降级返回 existing，实际: %s", merged)
	}
}
