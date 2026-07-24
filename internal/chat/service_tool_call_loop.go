// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/mcp"
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
		llmMessages = s.appendToolCallMessages(convID, llmMessages, acc, results)
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

	return s.saveToolCallFinalMessage(convID, acc, state)
}

// executeToolCallsConcurrently 并发执行所有 tool calls（search + MCP 工具）。
// 预分配结果切片，按 tool call 在原切片中的索引写入，
// 避免 goroutine 并发完成后 append 导致结果乱序。
// 生活类比：像给每个快递员编好编号，按编号放回对应格子，避免谁先回来谁先放导致顺序乱。
func (s *Service) executeToolCallsConcurrently(cancelCtx context.Context, convID string, toolCalls []llm.ToolCall, cfg *config.Config) []toolCallResult {
	toolResults := make([]toolCallResult, len(toolCalls))
	var toolWg sync.WaitGroup

	mgr := s.getMCPManager()

	for idx, tc := range toolCalls {
		// 判断工具类型：search、MCP 工具、未知工具
		isSearch := tc.Function.Name == "search"
		isMCP := !isSearch && mgr != nil && mgr.HasTool(tc.Function.Name)
		if !isSearch && !isMCP {
			continue // 跳过未知工具
		}

		if isSearch {
			s.emitForConv(convID, "search_start", tc.Function.Arguments)
		}
		toolWg.Add(1)
		go func(idx int, tc llm.ToolCall) {
			defer toolWg.Done()
			result := s.executeSingleToolCall(cancelCtx, convID, tc, cfg)
			// 按 idx 写入预分配的切片，不同 goroutine 写不同位置，无需加锁
			toolResults[idx] = result
		}(idx, tc)
	}
	toolWg.Wait()
	return toolResults
}

// executeSingleToolCall 执行单个 tool call（在 goroutine 内调用）。
// 根据工具名路由：search 走原搜索逻辑，MCP 工具走 MCP 调用。
// 生活类比：前台接到订单后，看是自家的菜（search）还是外卖平台的菜（MCP），
// 分别送到对应的后厨。
func (s *Service) executeSingleToolCall(cancelCtx context.Context, convID string, tc llm.ToolCall, cfg *config.Config) toolCallResult {
	// search 工具：走原有搜索逻辑
	if tc.Function.Name == "search" {
		return s.executeSearchToolCall(cancelCtx, convID, tc, cfg)
	}
	// MCP 工具：走 MCP 客户端调用
	mgr := s.getMCPManager()
	if mgr != nil && mgr.HasTool(tc.Function.Name) {
		return s.executeMCPToolCall(cancelCtx, convID, tc, mgr)
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
		return result
	}

	if searchResp == nil || len(searchResp.Results) == 0 {
		result.toolContent = "No results found. Use your own knowledge."
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
	// 截断搜索结果，防止上下文膨胀
	ctxSize := 0
	if cfg != nil {
		ctxSize = cfg.ContextSize
	}
	result.toolContent = truncateSearchContext(toolContent, ctxSize)
	return result
}

// executeMCPToolCall 执行 MCP 工具调用（在 goroutine 内调用）。
// 处理流程：解析参数 → 调用 MCP server → 返回结果文本。
// 生活类比：前台把订单发到对应的外卖平台，等平台返回结果。
func (s *Service) executeMCPToolCall(cancelCtx context.Context, convID string, tc llm.ToolCall, mgr *mcp.Manager) toolCallResult {
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

	// 调用 MCP 工具（Manager 内部已有超时控制）
	content, err := mgr.CallTool(cancelCtx, tc.Function.Name, args)
	if err != nil {
		log.Warn().Err(err).Str("tool", tc.Function.Name).Msg("[chat] MCP 工具调用失败")
		result.toolContent = fmt.Sprintf("Error: tool %q call failed: %v", tc.Function.Name, err)
		return result
	}

	if content == "" {
		content = "(工具未返回内容)"
	}

	result.toolContent = content
	log.Info().Str("tool", tc.Function.Name).Int("content_len", len(content)).Msg("[chat] MCP 工具调用完成")
	return result
}

// appendToolCallMessages 将 tool call 结果追加到 llmMessages 并持久化到 DB。
// 每个结果生成两条消息：assistant（含 tool_calls）和 tool（含搜索结果）。
// 跳过未填充的条目（非 search 类 tool call 的位置为零值）。
func (s *Service) appendToolCallMessages(convID string, llmMessages []llm.ChatMessage, acc *StreamAccumulator, results []toolCallResult) []llm.ChatMessage {
	for _, tr := range results {
		if tr.tc.Function.Name == "" {
			continue
		}
		assistantToolCallJSON, _ := json.Marshal([]llm.ToolCall{tr.tc})
		if err := store.CreateMessage(s.db, &store.Message{
			ConversationID:   convID,
			Role:             "assistant",
			Content:          "",
			ToolCalls:        string(assistantToolCallJSON),
			ThinkingContent:  acc.FirstRoundThinking,
			ThinkingDuration: clampDuration(acc.FirstRoundThinkingDuration),
		}, secrets.CipherKey(s.cipher)); err != nil {
			log.Error().Err(err).Msg("save assistant tool call message")
		}

		llmMessages = append(llmMessages, llm.ChatMessage{
			Role:      "assistant",
			Content:   "",
			ToolCalls: []llm.ToolCall{tr.tc},
		})

		if tr.searchJSON != "" {
			// 聚合所有 tool call 的搜索结果，而非覆盖（修复多 tool call 结果丢失问题）
			acc.LastSearchJSON = MergeSearchJSON(acc.LastSearchJSON, tr.searchJSON)
		}

		if err := store.CreateMessage(s.db, &store.Message{
			ConversationID: convID,
			Role:           "tool",
			Content:        tr.toolContent,
			ToolCallID:     tr.tc.ID,
		}, secrets.CipherKey(s.cipher)); err != nil {
			log.Error().Err(err).Msg("save tool result message")
		}

		llmMessages = append(llmMessages, llm.ChatMessage{
			Role:       "tool",
			Content:    tr.toolContent,
			ToolCallID: tr.tc.ID,
		})
	}
	return llmMessages
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
	result := CompressContext(s.wailsCtx, llmMessages, contextLimit, existingSummary, nil, client, convID, s.db)
	llmMessages = result.Messages
	// 压缩后 llmMessages 发生变化，重新计算 totalTokens 以保持准确
	state.totalTokens = estimateMessagesTokens(llmMessages)
	log.Info().Int("estimated", estimatedTotal).Int("context_size", contextLimit).Int("messages_after", len(llmMessages)).Msg("[chat] tool call preventive trim")

	s.emitForConv(convID, "context_trimmed", map[string]any{
		"reason":         "tool_call_preventive_trim",
		"estimated":      estimatedTotal,
		"context_size":   contextLimit,
		"messages_after": len(llmMessages),
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
		"tool call stream (retry after context trim): %w",
		"stream chat after search: %w",
	)
	if result == streamStopped {
		return true, err
	}
	return false, nil
}

// saveToolCallFinalMessage 保存 tool call 循环结束后的最终 AI 消息。
// 若达到最大轮次仍有 tool_calls，追加提示信息。
func (s *Service) saveToolCallFinalMessage(convID string, acc *StreamAccumulator, state *toolCallLoopState) error {
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
		log.Error().Err(err).Msg("save ai message")
	}
	chatMsg := storeMsgToChat(aiMsg)
	chatMsg.TokensPerSecond = acc.TokensPerSecond
	chatMsg.PredictedN = acc.PredictedN
	s.emitForConv(convID, "assistant_message", chatMsg)
	return nil
}
