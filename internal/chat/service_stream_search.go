// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
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

	// 自动 restore：若上次保存的 KV 缓存属于同一对话，先恢复以跳过重复 prefill
	// 必须在构建请求之后、发送请求之前执行，否则 restore 来的 KV 不会被本次请求复用
	s.tryRestoreSlot(cancelCtx, convID)

	// 估算值接近上下文上限时，用 /tokenize 准确校准 MaxTokens，避免生成时溢出
	// 生活类比：行李目测接近限重时才上精准秤，避免每次托运都浪费时间称重
	s.tryRefineMaxTokens(cancelCtx, req, client, cfg)

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

	// 自动 save：生成成功完成后保存 KV 缓存，下次同对话的请求可跳过 prefill
	// 必须放在 executeStreamAndHandleErrors + finalizeStreamResult 之后，
	// 否则可能在 tool call 中途或生成失败时保存半截 KV
	s.trySaveSlot(cancelCtx, convID)

	// 更新会话标题
	s.updateConversationTitleIfNeeded(convID, titleContent)

	s.emitForConv(convID, "done", nil)
	return nil
}

// tryRefineMaxTokens 在估算 prompt token 数接近上下文上限时，用 /tokenize 准确校准。
//
// 触发条件：估算值 > contextSize * 75%
// 失败时保留估算值，不阻塞主流程（context-shift 兜底会处理溢出）
//
// 生活类比：像行李托运前先目测重量（估算），如果目测接近限重（75%+），
// 才放上精准秤称一次（/tokenize），避免超重被拒。大多数情况不需要称重。
func (s *Service) tryRefineMaxTokens(ctx context.Context, req *llm.ChatCompletionRequest, client *llm.Client, cfg *config.Config) {
	if client == nil || cfg == nil || len(req.Messages) == 0 {
		return
	}

	ctxSize := cfg.ContextSize
	if ctxSize <= 0 {
		ctxSize = 4096
	}

	// 估算 prompt token 数（含 tool schema 开销）
	estimated := estimateMessagesTokens(req.Messages)
	if len(req.Tools) > 0 {
		estimated += 250
	}

	// 估算值远低于上限，跳过校准（大多数情况走这条路径，零额外开销）
	// 阈值 75%：ctxSize * 3 / 4，用整数运算避免浮点
	if estimated < ctxSize*3/4 {
		return
	}

	// 估算值接近上限，用 /tokenize 准确计算
	// 先用 ApplyTemplate 把 messages 转成 prompt 文本
	tokenizeCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	text, err := client.ApplyTemplate(tokenizeCtx, req.Messages)
	if err != nil {
		// bug D 修复：此处仅在估算值已超上下文 75% 时才会到达，
		// 校准失败会直接影响后续 max_tokens 决策，Debug 级别在生产日志不可见，升级为 Warn
		log.Warn().Err(err).Msg("[tokenize] apply template failed, skip refinement")
		return
	}

	tokens, err := client.Tokenize(tokenizeCtx, text)
	if err != nil {
		log.Warn().Err(err).Msg("[tokenize] tokenize failed, skip refinement")
		return
	}

	accurateTokens := len(tokens)
	log.Info().
		Int("estimated", estimated).
		Int("accurate", accurateTokens).
		Int("context_size", ctxSize).
		Float64("deviation_pct", (float64(accurateTokens-estimated)/float64(max(estimated, 1)))*100).
		Msg("[tokenize] refined prompt token count")

	// 用准确值重新计算 MaxTokens
	req.MaxTokens = s.calcMaxTokens(accurateTokens)
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
		// C-7 协议唯一事实化：改用类型化 struct 发射，移除 tokens_per_second 重复字段
		s.emitForConv(convID, EventTokenSpeed, TokenSpeedContent{
			TokensPerSecond: timings.PredictedPerSecond,
			PredictedN:      timings.PredictedN,
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
	// 支持 tool call 的模型，提供 search + MCP 工具
	if caps.ToolCallSupport {
		includeSearch := searchMode == "auto" || searchMode == "on"
		tools := s.buildAvailableTools(includeSearch)
		if len(tools) > 0 {
			req.Tools = tools
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
		apperror.New(apperror.KindTimeout, "stream chat timeout"),
		"[chat] context exceeded, trimming and retrying",
		"stream chat (retry after context trim)",
		"stream chat",
	)
	if result == streamStopped {
		return err
	}
	return nil
}

// recordCalibration 记录 prompt_tokens 反馈校准数据。
// 普通回复与 tool call 循环结束路径均会调用，使校准能覆盖工具调用场景（此前的缺口）。
// 生活类比：每次结账时记录实际找零与预估的差距，下次好更准地估算金额。
func (s *Service) recordCalibration(acc *StreamAccumulator, llmMessages []llm.ChatMessage) {
	if acc.PromptTokens <= 0 {
		return
	}
	estimated := estimateMessagesTokens(llmMessages)
	s.tokenCalibMu.Lock()
	s.lastPromptTokens = acc.PromptTokens
	s.lastEstimatedTokens = estimated
	s.tokenCalibMu.Unlock()
	log.Debug().Int("actual", acc.PromptTokens).Int("estimated", estimated).Float64("ratio", float64(acc.PromptTokens)/float64(max(estimated, 1))).Msg("[chat] token estimation calibration")
}

// finalizeStreamResult 处理流式结果：tool call 循环或保存 AI 消息。
func (s *Service) finalizeStreamResult(cancelCtx context.Context, convID string, llmMessages []llm.ChatMessage, acc *StreamAccumulator) error {
	if acc.FinishReason == "tool_calls" && len(acc.toolCalls()) > 0 {
		return s.handleToolCallLoop(cancelCtx, convID, llmMessages, acc, 3)
	}

	// 记录 prompt_tokens 反馈校准数据（普通回复路径）
	s.recordCalibration(acc, llmMessages)

	// 空内容不保存，避免产生空 assistant 消息
	content := acc.FullContent.String()
	thinkingContent := acc.FullThinking.String()
	if content == "" && thinkingContent == "" {
		// 用户主动取消属正常路径：前端已收到 stopped 事件，静默跳过即可
		if cancelCtx.Err() != nil {
			log.Info().Str("convID", convID).Msg("[chat] 生成已取消，跳过空消息保存")
			return nil
		}
		// bug B 修复：异常空回复不再静默吞掉——记 Warn 日志便于排查，
		// 并向前端发 error 事件，避免出现"AI 已读不回"且无任何解释的情况
		log.Warn().
			Str("convID", convID).
			Str("finish_reason", acc.FinishReason).
			Int("prompt_tokens", acc.PromptTokens).
			Bool("first_token_sent", acc.FirstTokenSent).
			Msg("[chat] 模型返回空内容，未保存 assistant 消息")
		s.emitForConv(convID, "error", "模型返回了空回复，请重试；若持续出现，请检查模型是否加载完整或调整生成参数")
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
		// M6 修复：保存失败时不发送 assistant_message，否则 UI 显示的消息在刷新后消失。
		// 改为发送 error 事件，让前端知道该条回复未持久化。
		log.Error().Err(err).Msg("save ai message")
		s.emitForConv(convID, "error", enhanceErrorWithHint(fmt.Sprintf("保存回复失败，此条回复可能未持久化: %v", err)))
		return nil
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
	if (conv.Title == "新对话" || conv.Title == "新的对话") && titleContent != "" {
		title := generateConversationTitle(titleContent)
		conv.Title = title
		if err := store.UpdateConversation(s.db, conv, secrets.CipherKey(s.cipher)); err != nil {
			// M18 修复：DB 更新失败时不 emit 新标题，避免前端显示新标题但刷新后回退到旧标题
			// 生活类比：仓库标签打印机故障时，不能让前台显示新货位，否则客户找不到货
			log.Error().Err(err).Str("convID", convID).Msg("[chat] 更新会话标题失败")
			return
		}
	}
	s.emitForConv(convID, "conversation_updated", &Conversation{
		ID:        conv.ID,
		Title:     conv.Title,
		CreatedAt: conv.CreatedAt.Format(time.RFC3339),
		UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
	})
}
