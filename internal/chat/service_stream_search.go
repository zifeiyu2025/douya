// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/search"
	"douya/internal/secrets"
	"douya/internal/store"
)

// streamWithSearch 执行带搜索功能的流式聊天。
//
// 拆分说明：原 186 行函数按流程拆为调度器 + 5 子函数：
//   - newStreamAccumulatorWithCallbacks: 创建累加器并设置降频回调
//   - buildChatStreamRequest: 构建聊天请求（含 tool call 支持）
//   - executeStreamAndHandleErrors: 执行流式请求并处理错误（取消/超时/重试）
//   - finalizeStreamResult: 处理流结果（tool call 循环或保存消息）
//   - updateConversationTitleIfNeeded: 更新会话标题
//
// 生活类比：就像一场直播——先架好摄像机（accumulator），准备好脚本（request），
// 开始直播（execute），直播结束后整理录像（finalize），最后更新节目单（updateTitle）。
func (s *Service) streamWithSearch(cancelCtx context.Context, convID string, llmMessages []llm.ChatMessage, searchMode string, _ string, titleContent string, searchResp *search.SearchResponse) error {
	acc := s.newStreamAccumulatorWithCallbacks(convID, searchResp)

	caps := s.GetModelCapabilities()
	cfg := s.getConfigSnapshot()
	client := s.getClientSnapshot()

	req := s.buildChatStreamRequest(llmMessages, searchMode, caps, cfg)

	streamCtx, streamCancel := context.WithTimeout(cancelCtx, streamRequestTimeout)
	defer streamCancel()
	defer s.setCurrentCompletionID("") // 流结束后清除 completion ID

	// 执行流式请求并处理错误
	if err := s.executeStreamAndHandleErrors(streamCtx, cancelCtx, convID, client, req, acc, cfg); err != nil {
		return err
	}

	// 处理流结果（tool call 或保存消息）
	if err := s.finalizeStreamResult(cancelCtx, convID, llmMessages, acc); err != nil {
		return err
	}

	// 更新会话标题
	s.updateConversationTitleIfNeeded(convID, titleContent)

	s.emitForConv(convID, "done", nil)
	return nil
}

// newStreamAccumulatorWithCallbacks 创建流式累加器并设置降频回调。
// 降频控制：token 速度每 500ms 发射一次，prompt 进度每 200ms 发射一次，
// 高速生成（100+ t/s）时 IPC 开销降低 98%。
func (s *Service) newStreamAccumulatorWithCallbacks(convID string, searchResp *search.SearchResponse) *StreamAccumulator {
	acc := NewStreamAccumulator(convID, s.emit, s.emitForConv)

	var lastSpeedEmit time.Time
	const speedEmitInterval = 500 * time.Millisecond
	var lastProgressEmit time.Time
	const progressEmitInterval = 200 * time.Millisecond

	// 设置 timings 回调：合并原 token_speed + generation_speed 为单一事件
	acc.OnTimings = func(timings llm.SSETimings) {
		now := time.Now()
		if now.Sub(lastSpeedEmit) < speedEmitInterval {
			return
		}
		lastSpeedEmit = now
		s.emitForConv(convID, "token_speed", map[string]any{
			"tokensPerSecond":   timings.PredictedPerSecond,
			"predictedN":        timings.PredictedN,
			"tokens_per_second": timings.PredictedPerSecond,
		})
	}
	acc.OnPromptProgress = func(progress llm.SSEPromptProgress) {
		if progress.Processed <= 0 {
			return
		}
		now := time.Now()
		if now.Sub(lastProgressEmit) < progressEmitInterval {
			return
		}
		lastProgressEmit = now
		s.emitForConv(convID, "prompt_progress", map[string]any{
			"total":     progress.Total,
			"cache":     progress.Cache,
			"processed": progress.Processed,
			"timeMs":    progress.TimeMs,
		})
	}

	if searchResp != nil && len(searchResp.Results) > 0 {
		sj, _ := json.Marshal(searchResp.Results)
		acc.LastSearchJSON = string(sj)
	}

	return acc
}

// buildChatStreamRequest 构建流式聊天请求，根据模型能力决定是否提供搜索工具。
func (s *Service) buildChatStreamRequest(llmMessages []llm.ChatMessage, searchMode string, caps llm.ModelCapabilities, cfg *config.Config) *llm.ChatCompletionRequest {
	req := &llm.ChatCompletionRequest{
		Model:           s.modelNameForRequest(),
		MaxTokens:       s.calcMaxTokens(estimateMessagesTokens(llmMessages)),
		Temperature:     cfg.Temperature,
		TopP:            cfg.TopP,
		TopK:            cfg.TopK,
		RepeatPenalty:   cfg.RepeatPenalty,
		TimingsPerToken: true,
		ReturnProgress:  true,
		StreamOptions:   &llm.StreamOptions{IncludeUsage: true},
		SsePingInterval: &defaultSsePingInterval,
	}
	// 支持 tool call 的模型，在 "auto" 和 "on" 模式下提供工具
	if (searchMode == "auto" || searchMode == "on") && caps.ToolCallSupport {
		req.Tools = []llm.ToolDefinition{searchToolDef}
		// tool schema 定义约占 250 tokens，需计入上下文估算
		req.MaxTokens = s.calcMaxTokens(estimateMessagesTokens(llmMessages) + 250)
		// searchMode="on" 为强制搜索，用 tool_choice="required" 确保模型一定调用工具
		// searchMode="auto" 不设置（默认 "auto"），让模型自主决定
		if searchMode == "on" {
			req.ToolChoice = "required"
		}
		// 显式声明是否允许并发 tool call
		// SupportsParallelToolCalls=true 时允许（提升多查询场景效率）
		// SupportsParallelToolCalls=false 时禁用（避免不支持并发的模型出错）
		parallel := caps.SupportsParallelToolCalls
		req.ParallelToolCalls = &parallel
	}

	req.Messages = llmMessages
	s.applyThinkingControl(req)
	s.applySamplingParams(req)
	return req
}

// executeStreamAndHandleErrors 执行流式请求并处理各类错误。
// 处理的错误类型：用户取消、超时、上下文溢出（自动重试）。
// 返回 nil 表示流式请求已完成（成功或用户取消），返回 error 表示不可恢复的错误。
func (s *Service) executeStreamAndHandleErrors(streamCtx context.Context, cancelCtx context.Context, convID string, client *llm.Client, req *llm.ChatCompletionRequest, acc *StreamAccumulator, cfg *config.Config) error {
	// 统一调用 runStreamWithStandardErrors 处理流式请求 + 三类标准错误（取消/超时/重试）
	// 安全实践（基于 B-1.1+B-1.2+B-1.3）：消除与 executeToolCallStream 之间的重复逻辑
	result, err := s.runStreamWithStandardErrors(
		streamCtx, cancelCtx, convID, convID, client, req, acc, cfg,
		"生成超时，请重试",
		fmt.Errorf("stream chat timeout"),
		"[chat] context exceeded, trimming and retrying",
		"stream chat (retry after context trim): %w",
		"stream chat: %w",
	)
	if result == streamStopped {
		return err
	}
	return nil
}

// finalizeStreamResult 处理流式结果：tool call 循环或保存 AI 消息。
func (s *Service) finalizeStreamResult(cancelCtx context.Context, convID string, llmMessages []llm.ChatMessage, acc *StreamAccumulator) error {
	if acc.FinishReason == "tool_calls" && len(acc.toolCalls()) > 0 {
		return s.handleToolCallLoop(cancelCtx, convID, llmMessages, acc, 3)
	}

	// 记录 prompt_tokens 反馈校准数据
	if acc.PromptTokens > 0 {
		estimated := estimateMessagesTokens(llmMessages)
		s.tokenCalibMu.Lock()
		s.lastPromptTokens = acc.PromptTokens
		s.lastEstimatedTokens = estimated
		s.tokenCalibMu.Unlock()
		log.Debug().Int("actual", acc.PromptTokens).Int("estimated", estimated).Float64("ratio", float64(acc.PromptTokens)/float64(max(estimated, 1))).Msg("[chat] token estimation calibration")
	}

	// 空内容不保存，避免产生空 assistant 消息
	content := acc.FullContent.String()
	thinkingContent := acc.FullThinking.String()
	if content == "" && thinkingContent == "" {
		return nil
	}

	aiMsg := &store.Message{
		ConversationID:   convID,
		Role:             "assistant",
		Content:          content,
		ThinkingContent:  thinkingContent,
		ThinkingDuration: clampDuration(acc.ThinkingDuration),
	}
	if aiMsg.ThinkingContent != "" && aiMsg.ThinkingDuration == 0 && acc.FirstRoundThinkingDuration > 0 {
		aiMsg.ThinkingDuration = clampDuration(acc.FirstRoundThinkingDuration)
	}
	if acc.LastSearchJSON != "" {
		aiMsg.SearchResults = acc.LastSearchJSON
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

// updateConversationTitleIfNeeded 在新对话首次交互后自动生成标题。
func (s *Service) updateConversationTitleIfNeeded(convID string, titleContent string) {
	conv, err := store.GetConversation(s.db, convID, secrets.CipherKey(s.cipher))
	if err != nil {
		log.Error().Err(err).Str("convID", convID).Msg("[chat] 无法获取会话以更新标题")
		return
	}
	if conv == nil {
		return
	}
	if (conv.Title == "新对话" || conv.Title == "新的对话") && len(titleContent) > 0 {
		title := generateConversationTitle(titleContent)
		conv.Title = title
		if err := store.UpdateConversation(s.db, conv, secrets.CipherKey(s.cipher)); err != nil {
			log.Error().Err(err).Str("convID", convID).Msg("[chat] 更新会话标题失败")
		}
	}
	s.emitForConv(convID, "conversation_updated", &Conversation{
		ID:        conv.ID,
		Title:     conv.Title,
		CreatedAt: conv.CreatedAt.Format(time.RFC3339),
		UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
	})
}
