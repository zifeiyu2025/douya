// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/secrets"
	"douya/internal/store"
)

// toolCallResult 单个 tool call 的执行结果。
// 从原 handleToolCallLoop 的局部类型提取为包级类型，便于跨子函数传递。
type toolCallResult struct {
	tc          llm.ToolCall
	toolContent string
	searchJSON  string
}

// toolCallLoopState 封装 tool call 循环中的可变状态。
// 生活类比：像账本，记录当前余额（totalTokens）和是否到账期（hitMaxRounds），
// 避免把一堆零散变量在子函数间来回传递。
type toolCallLoopState struct {
	totalTokens  int  // 增量累加的 token 总数（避免每轮 O(n) 重算）
	hitMaxRounds bool // 是否已到达最大轮次
}

// updateTokenCount 增量累加本轮新增消息的 token 数。
// 生活类比：账本上只追加新交易，老账目不动，O(n²) 退化为 O(n)。
func (st *toolCallLoopState) updateTokenCount(llmMessages []llm.ChatMessage, prevMsgCount int) {
	for i := prevMsgCount; i < len(llmMessages); i++ {
		st.totalTokens += estimateChatMessageTokens(llmMessages[i])
	}
}

// handleToolCallLoop 执行 tool call 循环：模型返回 tool_calls 后，并发执行搜索、
// 将结果追加到消息列表、构建下一轮请求，直到模型不再返回 tool_calls 或达到最大轮次。
//
// 拆分说明：原 266 行函数按职责拆为调度器 + 6 子函数：
//   - executeToolCallsConcurrently: 并发执行所有 search tool calls
//   - executeSingleToolCall:        执行单个 tool call（goroutine 内调用）
//   - appendToolCallMessages:       将结果追加到 llmMessages 并持久化到 DB
//   - compressContextIfNeeded:      预防性裁剪上下文
//   - buildToolCallStreamRequest:   构建下一轮流式请求
//   - executeToolCallStream:        执行下一轮流式请求并处理错误
//   - saveToolCallFinalMessage:     保存最终 AI 消息
//
// 生活类比：像一个工作流水线——调度器是车间主任，子函数是各工位工人，
// 主任负责按顺序协调各工位，工人只管自己手头的活。
func (s *Service) handleToolCallLoop(cancelCtx context.Context, convID string, llmMessages []llm.ChatMessage, acc *StreamAccumulator, maxRounds int) error {
	// 在函数入口获取快照，避免循环中反复加锁和数据竞争
	cfg := s.getConfigSnapshot()
	client := s.getClientSnapshot()
	defer s.setCurrentCompletionID("") // 工具调用循环结束后清除 completion ID

	state := &toolCallLoopState{
		totalTokens: estimateMessagesTokens(llmMessages),
	}

	for round := range maxRounds {
		state.hitMaxRounds = round == maxRounds-1

		accumulatedToolCalls := acc.toolCalls()
		if acc.FinishReason != "tool_calls" || len(accumulatedToolCalls) == 0 {
			break
		}

		// 1. 并发执行所有 tool calls
		results := s.executeToolCallsConcurrently(cancelCtx, convID, accumulatedToolCalls, cfg)

		// 2. 将结果追加到 llmMessages 并持久化
		prevMsgCount := len(llmMessages)
		// M17 修复：DB 写入失败时中断 tool call 循环，避免上下文与 DB 不一致
		var appendErr error
		llmMessages, appendErr = s.appendToolCallMessages(convID, llmMessages, acc, results)
		if appendErr != nil {
			return appendErr
		}
		state.updateTokenCount(llmMessages, prevMsgCount)

		// 3. 预防性裁剪上下文
		llmMessages = s.compressContextIfNeeded(convID, llmMessages, state, cfg, client)

		// 4. 构建并执行下一轮请求
		req := s.buildToolCallStreamRequest(llmMessages, cfg, state)
		acc.resetForNextCall()

		// 5. 执行流式请求；done=true 表示整个循环应结束（用户取消或不可恢复错误）
		done, err := s.executeToolCallStream(cancelCtx, convID, client, req, acc, round, cfg)
		if err != nil {
			return err
		}
		if done {
			// 用户取消：已通过 savePartialContentIfAny 保存部分内容，直接返回
			return nil
		}

		if acc.FinishReason != "tool_calls" {
			break
		}
	}

	return s.saveToolCallFinalMessage(convID, acc, state, llmMessages)
}

// executeToolCallsConcurrently 并发执行所有 tool calls（search + MCP 工具）。

// executeToolCallsConcurrently 并发执行所有 tool calls（search + MCP 工具）。
// 预分配结果切片，按 tool call 在原切片中的索引写入，
// 避免 goroutine 并发完成后 append 导致结果乱序。
// 生活类比：像给每个快递员编好编号，按编号放回对应格子，避免谁先回来谁先放导致顺序乱。
//
// MCP 工具的判定基于缓存的工具列表（由 llama-server /tools 端点提供）。
// 工具名形如 "<server>_<tool>"，由 llama-server 自动加前缀避免命名冲突。
func (s *Service) executeToolCallsConcurrently(cancelCtx context.Context, convID string, toolCalls []llm.ToolCall, cfg *config.Config) []toolCallResult {
	toolResults := make([]toolCallResult, len(toolCalls))
	var toolWg sync.WaitGroup

	for idx, tc := range toolCalls {
		// 判断工具类型：search、MCP 工具、未知工具
		isSearch := tc.Function.Name == "search"
		isMCP := !isSearch && s.isMCPTool(tc.Function.Name)
		if !isSearch && !isMCP {
			continue // 跳过未知工具
		}

		if isSearch {
			s.emitForConv(convID, "search_start", tc.Function.Arguments)
		}
		toolWg.Add(1)
		go func(idx int, tc llm.ToolCall) {
			defer toolWg.Done()
			// C-1 修复：goroutine 内加 recover 防护，避免 panic 导致整个进程崩溃
			// 生活类比：流水线工位配备应急停止按钮，单个工位出故障不会让整条生产线停摆
			defer func() {
				if r := recover(); r != nil {
					log.Error().Interface("panic", r).Str("tool", tc.Function.Name).Int("idx", idx).Msg("[chat] executeSingleToolCall panic recovered")
					toolResults[idx] = toolCallResult{
						tc:          tc,
						toolContent: fmt.Sprintf(`{"error":"工具执行内部错误:%v"}`, r),
						searchJSON:  "",
					}
				}
			}()
			result := s.executeSingleToolCall(cancelCtx, convID, tc, cfg)
			// 按 idx 写入预分配的切片，不同 goroutine 写不同位置，无需加锁
			toolResults[idx] = result
		}(idx, tc)
	}
	toolWg.Wait()
	return toolResults
}

// executeSingleToolCall 执行单个 tool call（在 goroutine 内调用）。
// 根据工具名路由：search 走原搜索逻辑，MCP 工具走 HTTP 调用 llama-server /tools 端点。
// 生活类比：前台接到订单后，看是自家的菜（search）还是外卖平台的菜（MCP），
// 分别送到对应的后厨。
func (s *Service) executeSingleToolCall(cancelCtx context.Context, convID string, tc llm.ToolCall, cfg *config.Config) toolCallResult {
	// search 工具：走原有搜索逻辑
	if tc.Function.Name == "search" {
		return s.executeSearchToolCall(cancelCtx, convID, tc, cfg)
	}
	// MCP 工具：通过 llama-server /tools 端点调用
	if s.isMCPTool(tc.Function.Name) {
		return s.executeMCPToolCall(cancelCtx, convID, tc)
	}
	// 未知工具，返回错误信息
	var result toolCallResult
	result.tc = tc
	result.toolContent = fmt.Sprintf("Error: unknown tool %q. Please use only the provided tools.", tc.Function.Name)
	return result
}

// executeSearchToolCall 执行 search 工具调用（原 executeSingleToolCall 逻辑）。
// 处理流程：解析参数 → 发起搜索 → 处理超时/空结果 → 格式化搜索结果。
func (s *Service) executeSearchToolCall(cancelCtx context.Context, convID string, tc llm.ToolCall, cfg *config.Config) toolCallResult {
	var result toolCallResult
	result.tc = tc

	var args struct {
		Query string `json:"query"`
	}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		result.toolContent = fmt.Sprintf("Error: invalid arguments format. Expected JSON with \"query\" field. Got: %s. Please correct your arguments and try again.", tc.Function.Arguments)
		return result
	}

	s.emitForConv(convID, EventToolCallStart, ToolCallStartContent{
		ToolCallID: tc.ID,
		Tool:       tc.Function.Name,
		Query:      args.Query,
	})

	// 为单个 tool call 设置独立超时（30s），避免某个慢搜索阻塞整个循环
	// 生活类比：每个快递员有自己的配送时限，不会因为一个人迟到让整个团队等他。
	toolCtx, toolCancel := context.WithTimeout(cancelCtx, toolCallSearchTimeout)
	defer toolCancel() // 确保资源释放

	searchResp := s.doSearch(toolCtx, args.Query)

	// 检查是否超时：doSearch 不返回 error，需通过 toolCtx.Err() 判断
	if toolCtx.Err() == context.DeadlineExceeded {
		result.toolContent = "搜索超时（30s），请稍后重试"
		s.emitForConv(convID, EventSearchError, "搜索超时（30s），请稍后重试")
		return result
	}

	if searchResp == nil || len(searchResp.Results) == 0 {
		result.toolContent = "No results found. Use your own knowledge."
		// 搜索失败时把实际原因通过 search_error 事件推给前端，让用户看到具体问题
		// 区分"无结果"和"出错"：searchResp.Error 非空表示 provider 出错，空表示正常无匹配
		if searchResp != nil && searchResp.Error != "" {
			if hint := formatSearchErrorHint(searchResp.Error); hint != "" {
				s.emitForConv(convID, EventSearchError, hint)
			}
		}
		return result
	}

	s.emitForConv(convID, EventSearchResult, SearchResultContent{
		ToolCallID: tc.ID,
		Results:    searchResp.Results,
	})
	sj, _ := json.Marshal(searchResp.Results)
	result.searchJSON = string(sj)
	lang := detectLanguage(args.Query)
	toolContent := formatSearchResultsWithLang(searchResp.Results, lang) + searchResultInstruction(lang)
	// M7: 截断搜索结果，防止上下文膨胀
	// tool call 循环中无法准确估算剩余预算，用 ctxSize/6 作为保守预算（与原行为一致）
	// 生活类比：tool call 循环像快餐店出餐——按固定份量盛饭，不计算剩余空间，保证不出错
	ctxSize := 0
	if cfg != nil {
		ctxSize = cfg.ContextSize
	}
	availableBudget := ctxSize / 6
	result.toolContent = truncateSearchContext(toolContent, availableBudget)
	return result
}

// executeMCPToolCall 执行 MCP 工具调用（在 goroutine 内调用）。
// 通过 llama-server 的 /tools 端点 HTTP 调用，由 llama-server 内部转发到对应 MCP server 子进程。
// 生活类比：前台把订单发到外卖调度中心（llama-server），调度中心再分发给对应外卖平台（MCP server）。
//
// 工具名形如 "echo_echo"（<server>_<tool> 格式，由 llama-server 自动加前缀）。
// 超时复用 toolCallSearchTimeout（30s），与 search 工具保持一致。
func (s *Service) executeMCPToolCall(cancelCtx context.Context, convID string, tc llm.ToolCall) toolCallResult {
	var result toolCallResult
	result.tc = tc

	// 解析参数（MCP 工具参数是任意 JSON 对象）
	var args map[string]any
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		result.toolContent = fmt.Sprintf("Error: invalid arguments format for tool %q. Got: %s", tc.Function.Name, tc.Function.Arguments)
		return result
	}

	// 推送工具调用开始事件（复用现有前端事件机制）
	s.emitForConv(convID, EventToolCallStart, ToolCallStartContent{
		ToolCallID: tc.ID,
		Tool:       tc.Function.Name,
		Query:      tc.Function.Arguments,
	})

	log.Info().Str("tool", tc.Function.Name).Str("convID", convID).Msg("[chat] 调用 MCP 工具")

	// 获取 LLM 客户端快照
	client := s.getClientSnapshot()
	if client == nil {
		result.toolContent = fmt.Sprintf("Error: LLM client not initialized, cannot call tool %q", tc.Function.Name)
		return result
	}

	// 设置独立超时（复用 toolCallSearchTimeout=30s），避免某个慢 MCP 工具阻塞整个循环
	toolCtx, toolCancel := context.WithTimeout(cancelCtx, toolCallSearchTimeout)
	defer toolCancel()

	// 序列化参数为 JSON RawMessage
	paramsBytes, err := json.Marshal(args)
	if err != nil {
		result.toolContent = fmt.Sprintf("Error: failed to marshal arguments for tool %q: %v", tc.Function.Name, err)
		return result
	}

	// 通过 llama-server /tools 端点调用工具（HTTP 代理到 MCP server 子进程）
	respBody, err := client.CallTool(toolCtx, tc.Function.Name, paramsBytes)
	if err != nil {
		log.Warn().Err(err).Str("tool", tc.Function.Name).Msg("[chat] MCP 工具调用失败")
		result.toolContent = fmt.Sprintf("Error: tool %q call failed: %v", tc.Function.Name, err)
		return result
	}

	// 从响应中提取文本内容（兼容 MCP 标准格式和纯文本）
	content := extractToolResponseContent(respBody)
	if content == "" {
		content = "(工具未返回内容)"
	}

	// SEC-005: 限制 MCP 工具响应大小，防止超大响应撑爆上下文
	// 上限 100KB：与搜索结果 snippet 限制同量级，足够容纳工具返回的结构化数据
	const maxToolContentRunes = 100 * 1024 / 2 // 100KB 按 UTF-8 平均 2 字节/字符估算
	if runes := []rune(content); len(runes) > maxToolContentRunes {
		content = string(runes[:maxToolContentRunes]) + "\n[... 工具响应过长，已截断 ...]"
		log.Warn().Str("tool", tc.Function.Name).Int("orig_len", len(runes)).Msg("[chat] MCP 工具响应已截断")
	}

	result.toolContent = content
	log.Info().Str("tool", tc.Function.Name).Int("content_len", len(content)).Msg("[chat] MCP 工具调用完成")
	return result
}

// isMCPTool 判断工具名是否为已缓存的 MCP 工具。
// 通过比对 buildAvailableTools 拉取的工具列表（来自 llama-server /tools 端点）。
// 生活类比：检查这道菜是不是外卖平台提供的——查一下自家菜单里有没有。
func (s *Service) isMCPTool(name string) bool {
	if name == "" || name == "search" {
		return false
	}
	s.mcpToolsCacheMu.RLock()
	defer s.mcpToolsCacheMu.RUnlock()
	for _, t := range s.mcpToolsCache {
		if t.Function.Name == name {
			return true
		}
	}
	return false
}

// extractToolResponseContent 从 llama-server /tools POST 响应中提取工具返回的文本内容。
// 兼容三种格式（llama-server 实际响应格式取决于工具类型）：
// 1. 纯文本格式（最常见）：{"plain_text_response":"Echo: hello"}
// 2. MCP 标准格式（结构化）：{"content":[{"type":"text","text":"..."}],"isError":false}
// 3. 兜底容错：直接返回原始字符串
// 生活类比：拆快递——标准包裹有标签有内容（按标签找文本），散装包裹直接看里面装了什么。
func extractToolResponseContent(body []byte) string {
	// 优先尝试 llama-server 常用的 plain_text_response 字段
	var plainResp struct {
		PlainTextResponse string `json:"plain_text_response"`
	}
	if err := json.Unmarshal(body, &plainResp); err == nil && plainResp.PlainTextResponse != "" {
		return plainResp.PlainTextResponse
	}

	// 尝试 MCP 标准格式（content 数组）
	var mcpResp struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(body, &mcpResp); err == nil && len(mcpResp.Content) > 0 {
		var parts []string
		for _, c := range mcpResp.Content {
			if c.Type == "text" && c.Text != "" {
				parts = append(parts, c.Text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	// 容错：直接返回原始文本（可能是非标准响应）
	return string(body)
}

// appendToolCallMessages 将 tool call 结果追加到 llmMessages 并持久化到 DB。
// 每个结果生成两条消息：assistant（含 tool_calls）和 tool（含搜索结果）。
// 跳过未填充的条目（非 search 类 tool call 的位置为零值）。
// M17 修复：DB 写入失败时返回 error，避免 LLM 上下文与 DB 不一致（刷新后 tool call 历史丢失）
// 生活类比：仓库录入失败时停止后续入库，而不是让账本和实物对不上
func (s *Service) appendToolCallMessages(convID string, llmMessages []llm.ChatMessage, acc *StreamAccumulator, results []toolCallResult) ([]llm.ChatMessage, error) {
	// M7 修复：将该轮所有 assistant(tool_calls) + tool 成对消息汇总后以单个事务写入，
	// 保证 assistant 与对应 tool 结果要么全部落库、要么全部回滚，避免孤儿/半截数据。
	var toWrite []*store.Message
	for _, tr := range results {
		if tr.tc.Function.Name == "" {
			continue
		}
		assistantToolCallJSON, _ := json.Marshal([]llm.ToolCall{tr.tc})
		toWrite = append(toWrite, &store.Message{
			ConversationID:   convID,
			Role:             "assistant",
			Content:          "",
			ToolCalls:        string(assistantToolCallJSON),
			ThinkingContent:  acc.FirstRoundThinking,
			ThinkingDuration: clampDuration(acc.FirstRoundThinkingDuration),
		})

		llmMessages = append(llmMessages, llm.ChatMessage{
			Role:      "assistant",
			Content:   "",
			ToolCalls: []llm.ToolCall{tr.tc},
		})

		if tr.searchJSON != "" {
			// 聚合所有 tool call 的搜索结果，而非覆盖（修复多 tool call 结果丢失问题）
			acc.LastSearchJSON = MergeSearchJSON(acc.LastSearchJSON, tr.searchJSON)
		}

		toWrite = append(toWrite, &store.Message{
			ConversationID: convID,
			Role:           "tool",
			Content:        tr.toolContent,
			ToolCallID:     tr.tc.ID,
		})

		llmMessages = append(llmMessages, llm.ChatMessage{
			Role:       "tool",
			Content:    tr.toolContent,
			ToolCallID: tr.tc.ID,
		})
	}

	if len(toWrite) > 0 {
		if err := store.CreateMessagesTx(s.db, toWrite, secrets.CipherKey(s.cipher)); err != nil {
			log.Error().Err(err).Msg("save tool call messages")
			return nil, apperror.Wrap(apperror.KindInternal, "save tool call messages", err)
		}
	}
	return llmMessages, nil
}

// compressContextIfNeeded 预防性裁剪上下文。
// tool call 多轮累积可能导致上下文溢出，当估算 token 数超过上下文上限 80% 时触发压缩。
// 压缩后 llmMessages 发生变化，需重新计算 totalTokens 以保持准确。
func (s *Service) compressContextIfNeeded(convID string, llmMessages []llm.ChatMessage, state *toolCallLoopState, cfg *config.Config, client *llm.Client) []llm.ChatMessage {
	estimatedTotal := state.totalTokens + 250 // +250 for tool schema
	contextLimit := cfg.ContextSize
	if contextLimit <= 0 {
		contextLimit = 4096
	}
	if estimatedTotal <= contextLimit*80/100 {
		return llmMessages
	}

	// 调用 CompressContext 进行统一压缩
	// tool call 路径中的消息是 llm.ChatMessage 格式，没有对应的 store.Message
	// 传 nil 给 trimmedStoreMsgs，此时不会生成新摘要但会保留已有摘要
	existingSummary := ""
	if convID != "" {
		existingSummary, _ = store.GetConversationSummary(s.db, convID)
	}
	// M13 修复：hostCtx 用 snapshot 读取避免数据竞争
	hostCtx := s.getHostContextSnapshot()
	result := CompressContext(hostCtx, llmMessages, contextLimit, existingSummary, nil, client, convID, s.db)
	llmMessages = result.Messages
	// 压缩后 llmMessages 发生变化，重新计算 totalTokens 以保持准确
	state.totalTokens = estimateMessagesTokens(llmMessages)
	log.Info().Int("estimated", estimatedTotal).Int("context_size", contextLimit).Int("messages_after", len(llmMessages)).Msg("[chat] tool call preventive trim")

	s.compressionStats.inc(trimReasonToolLoop)
	s.emitForConv(convID, "context_trimmed", ContextTrimEventContent{
		Reason:        string(trimReasonToolLoop),
		ContextSize:   contextLimit,
		MessagesAfter: len(llmMessages),
	})
	return llmMessages
}

// buildToolCallStreamRequest 构建下一轮 tool call 流式请求。
// hitMaxRounds 时不提供工具，强制模型生成最终回复。
func (s *Service) buildToolCallStreamRequest(llmMessages []llm.ChatMessage, cfg *config.Config, state *toolCallLoopState) *llm.ChatCompletionRequest {
	req := &llm.ChatCompletionRequest{
		Model:           s.modelNameForRequest(),
		Messages:        llmMessages,
		MaxTokens:       s.calcMaxTokens(state.totalTokens + 250), // +250 for tool schema（复用增量计数）
		Temperature:     cfg.Temperature,
		TopP:            cfg.TopP,
		TopK:            cfg.TopK,
		RepeatPenalty:   cfg.RepeatPenalty,
		TimingsPerToken: true,
		ReturnProgress:  true,
		StreamOptions:   &llm.StreamOptions{IncludeUsage: true},
		SsePingInterval: &defaultSsePingInterval,
	}
	if !state.hitMaxRounds {
		req.Tools = s.buildAvailableTools(true)
	}

	s.applyThinkingControl(req)
	s.applySamplingParams(req)
	return req
}

// executeToolCallStream 执行下一轮 tool call 流式请求并处理错误。
// 返回 (done, err)：
//   - done=true, err=nil:  用户取消，调用方应直接返回（已保存部分内容）
//   - done=true, err!=nil: 不可恢复错误，调用方应返回 err
//   - done=false, err=nil: 继续循环（成功或重试成功）
func (s *Service) executeToolCallStream(cancelCtx context.Context, convID string, client *llm.Client, req *llm.ChatCompletionRequest, acc *StreamAccumulator, round int, cfg *config.Config) (bool, error) {
	toolCtx, toolCancel := context.WithTimeout(cancelCtx, streamRequestTimeout)
	defer toolCancel()
	// tool call 每轮使用独立的 convID，避免 SSE Replay Buffer 冲突
	toolConvID := fmt.Sprintf("%s::round%d", convID, round)

	// 统一调用 runStreamWithStandardErrors 处理流式请求 + 三类标准错误（取消/超时/重试）
	// 安全实践（基于 B-1.1+B-1.2+B-1.3）：消除与 executeStreamAndHandleErrors 之间的重复逻辑
	result, err := s.runStreamWithStandardErrors(
		toolCtx, cancelCtx, convID, toolConvID, client, req, acc, cfg,
		"工具调用生成超时",
		apperror.New(apperror.KindTimeout, "tool call stream timeout"),
		"[chat] tool call context exceeded, trimming and retrying",
		"tool call stream (retry after context trim)",
		"stream chat after search",
	)
	if result == streamStopped {
		return true, err
	}
	return false, nil
}

// saveToolCallFinalMessage 保存 tool call 循环结束后的最终 AI 消息。
// 若达到最大轮次仍有 tool_calls，追加提示信息。
func (s *Service) saveToolCallFinalMessage(convID string, acc *StreamAccumulator, state *toolCallLoopState, llmMessages []llm.ChatMessage) error {
	// 记录 prompt_tokens 反馈校准数据（tool call 循环路径）
	// 覆盖工具调用场景，避免校准只在普通回复路径更新
	s.recordCalibration(acc, llmMessages)

	aiMsg := &store.Message{
		ConversationID:   convID,
		Role:             "assistant",
		Content:          acc.FullContent.String(),
		ThinkingContent:  acc.FullThinking.String(),
		ThinkingDuration: clampDuration(acc.ThinkingDuration),
	}
	if aiMsg.ThinkingContent != "" && aiMsg.ThinkingDuration == 0 && acc.FirstRoundThinkingDuration > 0 {
		aiMsg.ThinkingDuration = clampDuration(acc.FirstRoundThinkingDuration)
	}
	if acc.LastSearchJSON != "" {
		aiMsg.SearchResults = acc.LastSearchJSON
	}
	if acc.FinishReason == "tool_calls" && state.hitMaxRounds {
		aiMsg.Content += "\n\n[工具调用已达最大轮次限制，部分搜索结果可能未完全处理]"
	}
	if err := store.CreateMessage(s.db, aiMsg, secrets.CipherKey(s.cipher)); err != nil {
		// M6 修复：保存失败时返回错误而非仍发送 assistant_message。
		// 上游 handleToolCallLoop 会把该错误转为 error 事件，避免 UI 显示未持久化的回复。
		log.Error().Err(err).Msg("save ai message")
		return apperror.Wrap(apperror.KindInternal, "保存工具调用回复失败", err)
	}
	chatMsg := storeMsgToChat(aiMsg)
	chatMsg.TokensPerSecond = acc.TokensPerSecond
	chatMsg.PredictedN = acc.PredictedN
	s.emitForConv(convID, "assistant_message", chatMsg)
	return nil
}
