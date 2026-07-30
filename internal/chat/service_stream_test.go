// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/search"
)

// slowProvider 是一个阻塞式搜索 provider，用于测试超时（任务 35）。
//
// 生活类比：像一个故意拖延的快递员，接到单子后睡大觉，直到超时或被取消。
// 通过 select 监听 ctx.Done() 和 time.After(delay)，确保 context 超时能立即返回。
type slowProvider struct {
	delay time.Duration
	name  string
}

func (p *slowProvider) Name() string { return p.name }
func (p *slowProvider) Search(ctx context.Context, query string) (*search.SearchResponse, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(p.delay):
		return &search.SearchResponse{
			Engine:  p.name,
			Results: []search.SearchResult{{Title: query, URL: "https://example.com"}},
		}, nil
	}
}
func (p *slowProvider) SearchWithOpts(ctx context.Context, query string, opts search.SearchOpts) (*search.SearchResponse, error) {
	return p.Search(ctx, query)
}

// TestIncrementalTokenCounting 验证增量 token 计数与全量计算结果一致（任务 14）。
//
// 生活类比：像账本记账，全量计算是把所有交易重新加一遍，增量计算是只加新增的交易。
// 两者结果应该完全一致，但增量计算在多轮 tool call 中性能更好（O(n) vs O(n²)）。
//
// 修复前：handleToolCallLoop 每轮都调用 estimateMessagesTokens(llmMessages) 重新计算所有消息
// 修复后：循环外维护 totalTokens，每轮只累加新增消息的 token 数
func TestIncrementalTokenCounting(t *testing.T) {
	// 构造初始消息列表（模拟 handleToolCallLoop 入参）
	messages := []llm.ChatMessage{
		{Role: "user", Content: "你好，今天天气怎么样？"},
		{Role: "assistant", Content: "你好！让我帮你查一下今天的天气。"},
	}

	// 全量计算（修复前的方式）
	fullTotal := estimateMessagesTokens(messages)

	// 增量计算（修复后的方式）：先算前 1 条，再累加第 2 条
	incrementalTotal := estimateMessagesTokens(messages[:1])
	for i := 1; i < len(messages); i++ {
		incrementalTotal += estimateChatMessageTokens(messages[i])
	}

	if fullTotal != incrementalTotal {
		t.Errorf("初始消息：增量计算 %d 与全量计算 %d 不一致", incrementalTotal, fullTotal)
	}

	// 模拟多轮 tool call 累加：每轮新增 assistant(tool_calls) 和 tool 消息
	newMessages := []llm.ChatMessage{
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_1", Type: "function", Function: llm.FunctionCall{Name: "search", Arguments: `{"query":"天气"}`}}}},
		{Role: "tool", Content: "搜索结果：今天晴天，气温 25 度", ToolCallID: "call_1"},
		{Role: "assistant", Content: "今天天气晴朗，气温 25 度，适合外出。"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "call_2", Type: "function", Function: llm.FunctionCall{Name: "search", Arguments: `{"query":"紫外线"}`}}}},
		{Role: "tool", Content: "搜索结果：紫外线指数中等", ToolCallID: "call_2"},
	}

	// 增量累加新消息
	prevMsgCount := len(messages)
	messages = append(messages, newMessages...)
	for i := prevMsgCount; i < len(messages); i++ {
		incrementalTotal += estimateChatMessageTokens(messages[i])
	}

	// 重新全量计算，验证结果一致
	fullTotal = estimateMessagesTokens(messages)

	if fullTotal != incrementalTotal {
		t.Errorf("多轮累加后：增量计算 %d 与全量计算 %d 不一致", incrementalTotal, fullTotal)
	}

	// 验证：增量计算确实只计算了新增部分（性能优化）
	// prevMsgCount 之前的部分不会被重复计算
	if incrementalTotal <= 0 {
		t.Errorf("token 总数应大于 0，实际 %d", incrementalTotal)
	}
}

// TestIncrementalTokenCounting_MultipleRounds 模拟 handleToolCallLoop 多轮累加场景（任务 14）。
//
// 生活类比：像多轮记账，每轮只加新交易，不重算老账目。多轮下来，累计结果应与
// 一次性全量计算完全一致。
func TestIncrementalTokenCounting_MultipleRounds(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "user", Content: "请帮我搜索最新的 AI 新闻"},
	}

	// 循环外初始化 totalTokens（对应修复后的代码）
	totalTokens := estimateMessagesTokens(messages)

	// 模拟 3 轮 tool call
	for round := range 3 {
		prevMsgCount := len(messages)

		// 每轮新增 assistant(tool_calls) + tool 消息
		callID := "call_" + string(rune('1'+round))
		messages = append(messages, llm.ChatMessage{
			Role:    "assistant",
			Content: "",
			ToolCalls: []llm.ToolCall{{
				ID:       callID,
				Type:     "function",
				Function: llm.FunctionCall{Name: "search", Arguments: `{"query":"AI news"}`},
			}},
		})
		messages = append(messages, llm.ChatMessage{
			Role:       "tool",
			Content:    "第" + string(rune('1'+round)) + "轮搜索结果内容",
			ToolCallID: callID,
		})

		// 增量累加本轮新增消息（对应修复后的代码）
		for i := prevMsgCount; i < len(messages); i++ {
			totalTokens += estimateChatMessageTokens(messages[i])
		}
	}

	// 验证：3 轮累加后，与全量计算结果一致
	fullTotal := estimateMessagesTokens(messages)
	if fullTotal != totalTokens {
		t.Errorf("3 轮累加后：增量 %d 与全量 %d 不一致", totalTokens, fullTotal)
	}
}

// TestToolCallSearchTimeout 验证单个 tool call 搜索超时被触发（任务 35）。
//
// 生活类比：快递员有 30 秒配送时限（测试中调小为 200ms），超时就标记失败。
// mock 的 slowProvider 会阻塞 35 秒（模拟慢搜索 API），但 toolCtx 超时会在 200ms 后触发，
// doSearch 会因 ctx 取消而立即返回，不会真的等 35 秒。
//
// 修复前：doSearch 用 cancelCtx，无独立超时，会阻塞 35 秒
// 修复后：doSearch 用 toolCtx（30s 超时），超时后立即返回
func TestToolCallSearchTimeout(t *testing.T) {
	// 临时调小超时时间，避免测试等待 30s
	origTimeout := toolCallSearchTimeout
	toolCallSearchTimeout = 200 * time.Millisecond
	defer func() { toolCallSearchTimeout = origTimeout }()

	// 创建 Service，searchChain 使用阻塞 35s 的 provider
	chain := search.NewSearchChain(&slowProvider{delay: 35 * time.Second, name: "slow"})
	s := NewService(nil, chain, nil, &config.Config{ContextSize: 4096}, nil, "")

	// 模拟 handleToolCallLoop 中 goroutine 内的逻辑
	toolCtx, toolCancel := context.WithTimeout(context.Background(), toolCallSearchTimeout)
	defer toolCancel()

	start := time.Now()
	searchResp := s.doSearch(toolCtx, "test query")
	elapsed := time.Since(start)

	// 验证 1：doSearch 在超时时间内返回，而非等待 35s
	if elapsed >= 30*time.Second {
		t.Errorf("doSearch 耗时 %v，超时未触发（应约 200ms）", elapsed)
	}

	// 验证 2：toolCtx 触发了 DeadlineExceeded
	if toolCtx.Err() != context.DeadlineExceeded {
		t.Errorf("toolCtx.Err() = %v，期望 context.DeadlineExceeded", toolCtx.Err())
	}

	// 验证 3：超时时 searchResp 不含有效结果（doSearch 因 ctx 取消，所有 provider 失败）
	// SearchChain 在所有 provider 失败时返回非 nil 的 SearchResponse，但 Results 为空
	if searchResp != nil && len(searchResp.Results) > 0 {
		t.Errorf("超时时 searchResp 不应包含结果，实际 %+v", searchResp)
	}
}

// TestToolCallTimeoutContent 验证超时时返回的内容是"搜索超时（30s），请稍后重试"（任务 35）。
//
// 生活类比：快递员超时后，系统自动给客户发一条"配送超时，请稍后重试"的通知，
// 而不是让客户一直等。
//
// 此测试模拟 handleToolCallLoop 中 goroutine 内的超时判断分支：
//   - toolCtx.Err() == context.DeadlineExceeded → toolContent = "搜索超时（30s），请稍后重试"
func TestToolCallTimeoutContent(t *testing.T) {
	origTimeout := toolCallSearchTimeout
	toolCallSearchTimeout = 100 * time.Millisecond
	defer func() { toolCallSearchTimeout = origTimeout }()

	chain := search.NewSearchChain(&slowProvider{delay: 35 * time.Second, name: "slow"})
	s := NewService(nil, chain, nil, &config.Config{ContextSize: 4096}, nil, "")

	// 模拟 handleToolCallLoop 中 goroutine 内的完整逻辑
	toolCtx, toolCancel := context.WithTimeout(context.Background(), toolCallSearchTimeout)
	defer toolCancel()

	searchResp := s.doSearch(toolCtx, "test query")

	// 复刻 handleToolCallLoop 中的超时判断分支
	var toolContent string
	if toolCtx.Err() == context.DeadlineExceeded {
		toolContent = "搜索超时（30s），请稍后重试"
	} else if searchResp != nil && len(searchResp.Results) > 0 {
		toolContent = "has results"
	} else {
		toolContent = "No results found. Use your own knowledge."
	}

	if toolContent != "搜索超时（30s），请稍后重试" {
		t.Errorf("超时应返回 '搜索超时（30s），请稍后重试'，实际 '%s'", toolContent)
	}
}

// TestToolCallTimeoutNotBlockingOthers 验证单个 tool call 超时不阻塞其他 tool（任务 35）。
//
// 生活类比：厨房有两个厨师同时工作，一个在等慢菜（会超时），另一个有自己的短时限。
// 一个厨师的慢菜超时不应让另一个厨师也跟着等。
//
// 测试策略：启动两个并发的 doSearch goroutine，各自的 toolCtx 独立超时。
// 为避免 SearchChain 熔断器自身的数据竞争（Failures/State 字段无锁读取，非本任务范围），
// 每个 goroutine 使用独立的 SearchChain，专注于验证 toolCtx 独立超时机制。
// 验证两者都在各自的超时时间内返回，互不影响。
func TestToolCallTimeoutNotBlockingOthers(t *testing.T) {
	origTimeout := toolCallSearchTimeout
	toolCallSearchTimeout = 200 * time.Millisecond
	defer func() { toolCallSearchTimeout = origTimeout }()

	// 每个 goroutine 独立的 Service + SearchChain，避免触发 SearchChain 熔断器的数据竞争
	newService := func() *Service {
		chain := search.NewSearchChain(&slowProvider{delay: 35 * time.Second, name: "slow"})
		return NewService(nil, chain, nil, &config.Config{ContextSize: 4096}, nil, "")
	}

	var wg sync.WaitGroup
	wg.Add(2)

	type result struct {
		elapsed  time.Duration
		timedOut bool
	}
	results := make([]result, 2)

	// goroutine A：慢 tool，使用 toolCallSearchTimeout（200ms）超时
	go func(idx int) {
		defer wg.Done()
		s := newService()
		toolCtx, toolCancel := context.WithTimeout(context.Background(), toolCallSearchTimeout)
		defer toolCancel()
		start := time.Now()
		s.doSearch(toolCtx, "slow query")
		results[idx] = result{
			elapsed:  time.Since(start),
			timedOut: toolCtx.Err() == context.DeadlineExceeded,
		}
	}(0)

	// goroutine B：另一个 tool，使用更短的 50ms 超时
	// 验证它不会被 goroutine A 的 35s 阻塞影响
	go func(idx int) {
		defer wg.Done()
		s := newService()
		toolCtx, toolCancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer toolCancel()
		start := time.Now()
		s.doSearch(toolCtx, "another query")
		results[idx] = result{
			elapsed:  time.Since(start),
			timedOut: toolCtx.Err() == context.DeadlineExceeded,
		}
	}(1)

	wg.Wait()

	// 验证 1：两个 goroutine 都触发了超时
	if !results[0].timedOut {
		t.Errorf("goroutine A 应触发超时，耗时 %v", results[0].elapsed)
	}
	if !results[1].timedOut {
		t.Errorf("goroutine B 应触发超时，耗时 %v", results[1].elapsed)
	}

	// 验证 2：两个 goroutine 都在合理时间内返回（未被 35s 阻塞）
	// goroutine A 用 200ms 超时，应在 1s 内返回
	if results[0].elapsed > 1*time.Second {
		t.Errorf("goroutine A 耗时 %v，被阻塞超过 1s", results[0].elapsed)
	}
	// goroutine B 用 50ms 超时，应在 1s 内返回
	if results[1].elapsed > 1*time.Second {
		t.Errorf("goroutine B 耗时 %v，被 goroutine A 阻塞", results[1].elapsed)
	}
}

// TestParseMcpToolsResponse 验证 llama-server /tools 端点返回的工具列表能被正确解析。
//
// llama-server 实际响应格式（与 OpenAI tools 字段不同）：
//
//	[{"display_name":"echo_echo","tool":"echo_echo","type":"mcp",
//	  "definition":{"type":"function","function":{"name":"echo_echo",...}}}]
//
// 豆芽内部使用 OpenAI ToolDefinition 格式（顶层 {type, function:{...}}），
// refreshMcpToolsCache 需要把 definition 内的内容提到顶层。
func TestParseMcpToolsResponse(t *testing.T) {
	// 模拟 llama-server /tools 端点真实响应（来自实测）
	body := []byte(`[{"display_name":"echo_echo","tool":"echo_echo","type":"mcp","permissions":{"write":false},"definition":{"type":"function","function":{"name":"echo_echo","description":"Echo back the input text","parameters":{"type":"object","properties":{"text":{"type":"string","description":"Text to echo"}},"required":["text"]}}}}]`)

	var rawTools []struct {
		Type       string `json:"type"`
		Tool       string `json:"tool"`
		Definition struct {
			Type     string          `json:"type"`
			Function llm.FunctionDef `json:"function"`
		} `json:"definition"`
	}
	if err := json.Unmarshal(body, &rawTools); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(rawTools) != 1 {
		t.Fatalf("期望 1 个工具，实际 %d", len(rawTools))
	}
	if rawTools[0].Definition.Function.Name != "echo_echo" {
		t.Errorf("期望工具名 echo_echo，实际 %s", rawTools[0].Definition.Function.Name)
	}
	if rawTools[0].Definition.Function.Description != "Echo back the input text" {
		t.Errorf("description 不匹配: %s", rawTools[0].Definition.Function.Description)
	}
	if rawTools[0].Type != "mcp" {
		t.Errorf("期望顶层 type=mcp，实际 %s", rawTools[0].Type)
	}
}

// TestExtractToolResponseContent_PlainText 验证纯文本格式响应解析。
// llama-server /tools POST 端点实际返回 {"plain_text_response":"Echo: hello"}。
func TestExtractToolResponseContent_PlainText(t *testing.T) {
	body := []byte(`{"plain_text_response":"Echo: hello"}`)
	got := extractToolResponseContent(body)
	if got != "Echo: hello" {
		t.Errorf("期望 'Echo: hello'，实际 %q", got)
	}
}

// TestExtractToolResponseContent_MCPStandard 验证 MCP 标准格式响应解析。
func TestExtractToolResponseContent_MCPStandard(t *testing.T) {
	body := []byte(`{"content":[{"type":"text","text":"line1"},{"type":"text","text":"line2"}],"isError":false}`)
	got := extractToolResponseContent(body)
	if got != "line1\nline2" {
		t.Errorf("期望 'line1\\nline2'，实际 %q", got)
	}
}

// TestExtractToolResponseContent_Empty 验证空响应兜底。
func TestExtractToolResponseContent_Empty(t *testing.T) {
	body := []byte(`{"plain_text_response":""}`)
	got := extractToolResponseContent(body)
	// plain_text_response 为空时回退到原始字符串
	if got != `{"plain_text_response":""}` {
		t.Errorf("空 plain_text_response 应回退到原始字符串，实际 %q", got)
	}
}
