// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/llm"
	"douya/internal/search"
	"douya/internal/secrets"
	"douya/internal/store"
)

// L-15：streamRequestTimeout 是业务层流式请求超时，通过 context.WithTimeout 包裹请求，
// 优先生效于 client.go 的 streamTimeout=900s 兜底超时。长输出（>300s）会被此值截断。
const streamRequestTimeout = 300 * time.Second // 业务层流式请求超时

// toolCallSearchTimeout 单个 tool call 搜索的独立超时时间（任务 35）
// 用 var 而非 const，便于单元测试临时调小以快速验证超时逻辑
// 生活类比：每个快递员有 30 秒配送时限，超时就标记失败让其他人继续工作
var toolCallSearchTimeout = 30 * time.Second

var searchToolDef = llm.ToolDefinition{
	Type: "function",
	Function: llm.FunctionDef{
		Name:        "search",
		Description: "搜索互联网获取实时信息。当用户问题涉及以下情况时调用：1.时事新闻、最新动态；2.具体数据、统计、价格等时效性信息；3.你不确定或可能已变化的事实；4.需要验证的信息。无需调用的情况：数学计算、代码编写、文学创作、闲聊问候等。调用是内部流程，不要在回答中提及。",
		Parameters: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"query": map[string]any{
					"type":        "string",
					"description": "精简搜索词，语言与用户问题一致",
				},
			},
			"required": []string{"query"},
		},
	},
}

type StreamAccumulator struct {
	FullContent                strings.Builder
	FullThinking               strings.Builder
	FinishReason               string
	ToolCallMap                map[int]*llm.ToolCall
	EmitFn                     func(string, any)
	ConvID                     string
	EmitForConvFn              func(string, string, any)
	PendingBytes               string
	PendingThink               string
	LastSearchJSON             string
	ThinkingStartTime          time.Time
	ThinkingDuration           float64
	ThinkingDone               bool
	FirstRoundThinking         string
	FirstRoundThinkingDuration float64
	PromptTokens               int                                  // 来自 SSE 流式响应的 usage 字段
	CompletionID               string                               // 来自 SSE 流式响应的 id 字段，用于 /v1/chat/completions/control
	TokensPerSecond            float64                              // 来自 SSE 流式响应的 timings.predicted_per_second
	PredictedN                 int                                  // 来自 SSE 流式响应的 timings.predicted_n
	OnTimings                  func(timings llm.SSETimings)         // 当收到 timings 数据时的回调，用于实时推送速度
	OnPromptProgress           func(progress llm.SSEPromptProgress) // 当收到 prompt_progress 数据时的回调
}

// 流式响应缓冲区最大大小（10MB）
const maxStreamBufferSize = 10 * 1024 * 1024

func NewStreamAccumulator(convID string, emitFn func(string, any), emitForConvFn func(string, string, any)) *StreamAccumulator {
	return &StreamAccumulator{
		ToolCallMap:   make(map[int]*llm.ToolCall),
		EmitFn:        emitFn,
		ConvID:        convID,
		EmitForConvFn: emitForConvFn,
	}
}

func (a *StreamAccumulator) callback() func(llm.SSEChunk) error {
	return func(chunk llm.SSEChunk) error {
		// 提取 usage 信息（llama-server 在流结束时返回）
		if chunk.Usage != nil && chunk.Usage.PromptTokens > 0 {
			a.PromptTokens = chunk.Usage.PromptTokens
		}

		// 提取 timings 信息（llama-server 在流结束时返回，包含生成速度）
		if chunk.Timings != nil && chunk.Timings.PredictedPerSecond > 0 {
			a.TokensPerSecond = chunk.Timings.PredictedPerSecond
			a.PredictedN = chunk.Timings.PredictedN
			// 实时推送速度数据到前端
			if a.OnTimings != nil {
				a.OnTimings(*chunk.Timings)
			}
		}

		// 提取 prompt_progress 信息（llama-server 在 prompt 处理阶段返回）
		if chunk.PromptProgress != nil && chunk.PromptProgress.Processed > 0 {
			if a.OnPromptProgress != nil {
				a.OnPromptProgress(*chunk.PromptProgress)
			}
		}

		// 追踪 completion ID（用于 /v1/chat/completions/control 实时控制）
		if chunk.ID != "" && a.CompletionID == "" {
			a.CompletionID = chunk.ID
		}

		if len(chunk.Choices) == 0 {
			return nil
		}

		// 检查缓冲区大小，防止内存无限增长
		if a.FullContent.Len()+a.FullThinking.Len() > maxStreamBufferSize {
			log.Warn().Msgf("[stream] buffer size exceeded %dMB, truncating", maxStreamBufferSize/1024/1024)
			return fmt.Errorf("response exceeds maximum buffer size (%dMB)", maxStreamBufferSize/1024/1024)
		}

		choice := chunk.Choices[0]
		deltaContent := choice.Delta.ContentString()
		if deltaContent != "" {
			if a.FullThinking.Len() > 0 && !a.ThinkingDone && !a.ThinkingStartTime.IsZero() {
				a.ThinkingDuration = time.Since(a.ThinkingStartTime).Seconds()
				a.ThinkingDone = true
			}
			combined := a.PendingBytes + deltaContent
			valid, pending := llm.TruncateIncompleteUTF8(combined)
			a.PendingBytes = pending
			fixed := llm.FixUTF8(valid)
			a.FullContent.WriteString(fixed)
			a.EmitForConvFn(a.ConvID, "token", fixed)
		}

		if choice.Delta.ReasoningContent != "" {
			if a.ThinkingStartTime.IsZero() {
				a.ThinkingStartTime = time.Now()
			}
			combined := a.PendingThink + choice.Delta.ReasoningContent
			valid, pending := llm.TruncateIncompleteUTF8(combined)
			a.PendingThink = pending
			fixed := llm.FixUTF8(valid)
			a.FullThinking.WriteString(fixed)
			a.EmitForConvFn(a.ConvID, "thinking", fixed)
		}

		if len(choice.Delta.ToolCalls) > 0 {
			for _, tc := range choice.Delta.ToolCalls {
				idx := tc.Index
				if tc.ID != "" {
					if existing, ok := a.ToolCallMap[idx]; ok {
						existing.Function.Arguments += tc.Function.Arguments
					} else {
						newTC := llm.ToolCall{
							Index: tc.Index,
							ID:    tc.ID,
							Type:  tc.Type,
							Function: llm.FunctionCall{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						}
						a.ToolCallMap[idx] = &newTC
					}
				} else {
					if existing, ok := a.ToolCallMap[idx]; ok {
						existing.Function.Arguments += tc.Function.Arguments
					}
				}
			}
		}

		if choice.FinishReason != nil {
			if a.FullThinking.Len() > 0 && !a.ThinkingDone && !a.ThinkingStartTime.IsZero() {
				a.ThinkingDuration = time.Since(a.ThinkingStartTime).Seconds()
				a.ThinkingDone = true
			}

			a.FinishReason = *choice.FinishReason

			// 思考完成但正文为空：记录日志，方便排查是模型行为还是截断问题
			if a.FullThinking.Len() > 0 && a.FullContent.Len() == 0 {
				log.Warn().Msgf("[stream] thinking completed but content is empty (finish_reason=%s, thinking_len=%d)", a.FinishReason, a.FullThinking.Len())
			}
		}

		return nil
	}
}

func (a *StreamAccumulator) toolCalls() []llm.ToolCall {
	if len(a.ToolCallMap) == 0 {
		return nil
	}
	result := make([]llm.ToolCall, 0, len(a.ToolCallMap))
	for _, tc := range a.ToolCallMap {
		result = append(result, *tc)
	}
	return result
}

func (a *StreamAccumulator) resetForNextCall() {
	if a.FullThinking.Len() > 0 {
		a.FirstRoundThinking = a.FullThinking.String()
		a.FirstRoundThinkingDuration = a.ThinkingDuration
	}
	a.FullContent.Reset()
	a.FinishReason = ""
	a.ToolCallMap = make(map[int]*llm.ToolCall)
	a.PendingBytes = ""
	a.PendingThink = ""
	a.ThinkingStartTime = time.Time{}
	a.ThinkingDuration = 0
	a.ThinkingDone = false
	a.CompletionID = "" // 重置 completion ID，让下一轮流式请求能捕获新的 ID
}

func clampDuration(d float64) float64 {
	if d < 0 || d > 3600 {
		return 0
	}
	return d
}

func (s *Service) calcMaxTokens(promptTokens int) int {
	ctxSize := 0
	if cfg := s.getConfigSnapshot(); cfg != nil {
		ctxSize = cfg.ContextSize
	}
	if ctxSize <= 0 {
		ctxSize = 4096
	}
	// 可用生成空间 = 上下文大小 - prompt 占用
	maxTokens := max(min(ctxSize-promptTokens, 16384), 512)
	return maxTokens
}

// savePartialContentIfAny 在用户停止生成时，若有已生成内容则保存为 assistant 消息。
//
// 生活类比：就像录音机中途被按下停止键，已经录到的声音仍然要保存下来。
// 如果还没录到任何内容（空内容），就不保存，避免产生空录音。
func (s *Service) savePartialContentIfAny(convID string, acc *StreamAccumulator) {
	content := acc.FullContent.String()
	thinkingContent := acc.FullThinking.String()
	if content == "" && thinkingContent == "" {
		return
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
		log.Error().Err(err).Msg("save partial ai message on stop")
	}
	s.emitForConv(convID, "assistant_message", storeMsgToChat(aiMsg))
}

// retryStreamAfterContextExceeded 在上下文溢出时裁剪消息并重试流式请求。
// 生活类比：像行李超重时，先扔掉一些不重要的东西（裁剪消息），再重新办托运（重试请求）。
//
// 该函数统一封装了 streamWithSearch 和 handleToolCallLoop 中重复的上下文溢出重试逻辑。
// 处理流程：
//  1. 用 ParseExceedContextError 解析 origErr
//  2. 若是上下文溢出：裁剪消息 → emit context_trimmed → 用 retryConvID 重试
//     - 重试成功：返回 (false, nil)，调用方继续往下执行
//     - 重试失败（用户取消）：emit stopped + savePartialContentIfAny，返回 (true, nil)
//     - 重试失败（其他错误）：emit error，返回 (true, fmt.Errorf(retryErrFmt, retryErr))
//  3. 若不是上下文溢出：emit error，返回 (true, fmt.Errorf(nonExceedErrFmt, origErr))
//
// 关于 retryCancel：原 handleToolCallLoop 在 for 循环内立即调用 retryCancel() 避免 defer 累积泄漏；
// 提取为独立函数后，每次调用都会同步返回，defer retryCancel() 不会累积，因此统一使用 defer 更安全
// （panic 时也能释放 retryCtx），且与原行为等价（StreamChatWithConvID 为同步调用，返回后 retryCtx 不再使用）。
func (s *Service) retryStreamAfterContextExceeded(
	cancelCtx context.Context,
	convID string,
	retryConvID string,
	client *llm.Client,
	req *llm.ChatCompletionRequest,
	fallbackCtxSize int,
	origErr error,
	acc *StreamAccumulator,
	logMsg string,
	retryErrFmt string,
	nonExceedErrFmt string,
) (handled bool, err error) {
	exceedInfo := ParseExceedContextError(origErr)
	if exceedInfo == nil || !exceedInfo.Exceeded {
		// 非上下文溢出错误：emit error 并返回
		s.emitForConv(convID, "error", enhanceErrorWithHint(origErr.Error()))
		return true, fmt.Errorf(nonExceedErrFmt, origErr)
	}

	// 上下文溢出：裁剪消息后重试
	actualCtx := exceedInfo.ContextSize
	if actualCtx <= 0 {
		actualCtx = fallbackCtxSize
	}
	reserve := max(actualCtx/10, 512)
	trimmed := TrimMessagesToFit(req.Messages, actualCtx, reserve)
	req.Messages = trimmed

	log.Info().Int("prompt_tokens", exceedInfo.PromptTokens).Int("context_size", actualCtx).Int("messages_after_trim", len(trimmed)).Msg(logMsg)

	s.emitForConv(convID, "context_trimmed", map[string]any{
		"reason":         "exceed_context_size",
		"prompt_tokens":  exceedInfo.PromptTokens,
		"context_size":   actualCtx,
		"messages_after": len(trimmed),
	})

	retryCtx, retryCancel := context.WithTimeout(cancelCtx, streamRequestTimeout)
	defer retryCancel()

	retryErr := client.StreamChatWithConvID(retryCtx, req, retryConvID, acc.callback())
	if retryErr == nil {
		// 重试成功，调用方继续往下执行
		return false, nil
	}

	// 重试失败：区分用户取消和其他错误
	if cancelCtx.Err() == context.Canceled {
		s.savePartialContentIfAny(convID, acc)
		s.emitForConv(convID, "stopped", nil)
		return true, nil
	}
	s.emitForConv(convID, "error", enhanceErrorWithHint("上下文过长，裁剪后仍无法生成，请尝试缩短对话或新建对话"))
	return true, fmt.Errorf(retryErrFmt, retryErr)
}

func (s *Service) handleToolCallLoop(cancelCtx context.Context, convID string, llmMessages []llm.ChatMessage, acc *StreamAccumulator, maxRounds int) error {
	// 在函数入口获取快照，避免循环中反复加锁和数据竞争
	cfg := s.getConfigSnapshot()
	client := s.getClientSnapshot()
	defer s.setCurrentCompletionID("") // 工具调用循环结束后清除 completion ID
	hitMaxRounds := false

	// 在循环外维护增量 token 计数器，避免每轮都重新计算所有消息的 token 数（O(n²) 退化为 O(n)）
	// 生活类比：像记账，每笔新交易只需加上新增金额，不需要把之前每笔交易重新加一遍。
	// 注意：当 CompressContext 修改 llmMessages 时，需重新计算 totalTokens。
	totalTokens := estimateMessagesTokens(llmMessages)

	for round := range maxRounds {
		hitMaxRounds = round == maxRounds-1

		accumulatedToolCalls := acc.toolCalls()
		if acc.FinishReason != "tool_calls" || len(accumulatedToolCalls) == 0 {
			break
		}

		type toolCallResult struct {
			tc          llm.ToolCall
			toolContent string
			searchJSON  string
		}

		// 预分配结果切片，按 tool call 在原切片中的索引写入，
		// 避免 goroutine 并发完成后 append 导致结果乱序。
		// 不同 goroutine 写入不同 idx，无需加锁；最终读取前用 WaitGroup 等待全部完成。
		// 生活类比：像给每个快递员编好编号，按编号放回对应格子，避免谁先回来谁先放导致顺序乱。
		toolResults := make([]toolCallResult, len(accumulatedToolCalls))
		var toolWg sync.WaitGroup

		// 记录添加新消息前的位置，用于增量 token 累加（任务 14）
		prevMsgCount := len(llmMessages)

		for idx, tc := range accumulatedToolCalls {
			if tc.Function.Name != "search" {
				continue
			}
			s.emitForConv(convID, "search_start", tc.Function.Arguments)
			toolWg.Add(1)
			go func(idx int, tc llm.ToolCall) {
				defer toolWg.Done()
				var result toolCallResult
				result.tc = tc

				var args struct {
					Query string `json:"query"`
				}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					result.toolContent = fmt.Sprintf("Error: invalid arguments format. Expected JSON with \"query\" field. Got: %s. Please correct your arguments and try again.", tc.Function.Arguments)
				} else {
					s.emitForConv(convID, "tool_call_start", map[string]string{"tool": tc.Function.Name, "query": args.Query})

					// 为单个 tool call 设置独立超时（30s），避免某个慢搜索阻塞整个循环（任务 35）
					// 生活类比：每个快递员有自己的配送时限，不会因为一个人迟到让整个团队等他。
					toolCtx, toolCancel := context.WithTimeout(cancelCtx, toolCallSearchTimeout)
					defer toolCancel() // 确保资源释放

					searchResp := s.doSearch(toolCtx, args.Query)

					// 检查是否超时：doSearch 不返回 error，需通过 toolCtx.Err() 判断
					if toolCtx.Err() == context.DeadlineExceeded {
						result.toolContent = "搜索超时（30s），请稍后重试"
					} else if searchResp != nil && len(searchResp.Results) > 0 {
						s.emitForConv(convID, "search_result", searchResp.Results)
						sj, _ := json.Marshal(searchResp.Results)
						result.searchJSON = string(sj)
						lang := detectLanguage(args.Query)
						toolContent := formatSearchResultsWithLang(searchResp.Results, lang) + searchResultInstruction(lang)
						// 截断搜索结果，防止上下文膨胀
						ctxSize := 0
						if cfg := s.getConfigSnapshot(); cfg != nil {
							ctxSize = cfg.ContextSize
						}
						result.toolContent = truncateSearchContext(toolContent, ctxSize)
					} else {
						result.toolContent = "No results found. Use your own knowledge."
					}
				}

				// 按 idx 写入预分配的切片，不同 goroutine 写不同位置，无需加锁
				toolResults[idx] = result
			}(idx, tc)
		}
		toolWg.Wait()

		// 遍历时跳过未填充的条目（非 search 类 tool call 的位置为零值）
		for i := range toolResults {
			tr := toolResults[i]
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
				acc.LastSearchJSON = tr.searchJSON
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

		// 增量累加本轮新增消息的 token 数，避免重新计算整个 llmMessages（任务 14）
		// 生活类比：账本上只追加新交易，老账目不动，O(n²) 退化为 O(n)
		for i := prevMsgCount; i < len(llmMessages); i++ {
			totalTokens += estimateChatMessageTokens(llmMessages[i])
		}

		// 预防性裁剪：tool call 多轮累积可能导致上下文溢出
		estimatedTotal := totalTokens + 250 // +250 for tool schema
		contextLimit := cfg.ContextSize
		if contextLimit <= 0 {
			contextLimit = 4096
		}
		if estimatedTotal > contextLimit*80/100 {
			// 调用 CompressContext 进行统一压缩
			// tool call 路径中的消息是 llm.ChatMessage 格式，没有对应的 store.Message
			// 传 nil 给 trimmedStoreMsgs，此时不会生成新摘要但会保留已有摘要
			existingSummary := ""
			if convID != "" {
				existingSummary, _ = store.GetConversationSummary(s.db, convID)
			}
			result := CompressContext(llmMessages, contextLimit, existingSummary, nil, client, convID, s.db)
			llmMessages = result.Messages
			// 压缩后 llmMessages 发生变化，重新计算 totalTokens 以保持准确（任务 14）
			totalTokens = estimateMessagesTokens(llmMessages)
			log.Info().Int("estimated", estimatedTotal).Int("context_size", contextLimit).Int("messages_after", len(llmMessages)).Msg("[chat] tool call preventive trim")

			s.emitForConv(convID, "context_trimmed", map[string]any{
				"reason":         "tool_call_preventive_trim",
				"estimated":      estimatedTotal,
				"context_size":   contextLimit,
				"messages_after": len(llmMessages),
			})
		}

		req := &llm.ChatCompletionRequest{
			Model:           s.modelNameForRequest(),
			Messages:        llmMessages,
			MaxTokens:       s.calcMaxTokens(totalTokens + 250), // +250 for tool schema（任务 14：复用增量计数）
			Temperature:     cfg.Temperature,
			TopP:            cfg.TopP,
			TopK:            cfg.TopK,
			RepeatPenalty:   cfg.RepeatPenalty,
			TimingsPerToken: true,
			ReturnProgress:  true,
			StreamOptions:   &llm.StreamOptions{IncludeUsage: true},
		}
		if !hitMaxRounds {
			req.Tools = []llm.ToolDefinition{searchToolDef}
		}

		s.applyThinkingControl(req)
		s.applySamplingParams(req)

		acc.resetForNextCall()
		toolCtx, toolCancel := context.WithTimeout(cancelCtx, streamRequestTimeout)
		// tool call 每轮使用独立的 convID，避免 SSE Replay Buffer 冲突
		toolConvID := fmt.Sprintf("%s::round%d", convID, round)
		// 包装 callback：在收到 completion ID 时同步到 Service，供 StopThinking 使用
		toolInner := acc.callback()
		toolWrapped := func(chunk llm.SSEChunk) error {
			err := toolInner(chunk)
			if err != nil {
				return err
			}
			if acc.CompletionID != "" {
				s.setCurrentCompletionID(acc.CompletionID)
			}
			return nil
		}
		err := client.StreamChatWithConvID(toolCtx, req, toolConvID, toolWrapped)
		toolCancel()
		if err != nil {
			if cancelCtx.Err() == context.Canceled {
				s.savePartialContentIfAny(convID, acc)
				s.emitForConv(convID, "stopped", nil)
				return nil
			}
			if toolCtx.Err() == context.DeadlineExceeded {
				s.emitForConv(convID, "error", enhanceErrorWithHint("工具调用生成超时"))
				return fmt.Errorf("tool call stream timeout")
			}

			// 上下文溢出重试：截断消息后重新请求
			retryConvID := fmt.Sprintf("%s::round%d::retry", convID, round)
			handled, retryErr := s.retryStreamAfterContextExceeded(
				cancelCtx, convID, retryConvID, client, req, cfg.ContextSize, err, acc,
				"[chat] tool call context exceeded, trimming and retrying",
				"tool call stream (retry after context trim): %w",
				"stream chat after search: %w",
			)
			if handled {
				return retryErr
			}
		}

		if acc.FinishReason != "tool_calls" {
			break
		}
	}

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
	if acc.FinishReason == "tool_calls" && hitMaxRounds {
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

func (s *Service) SendMessage(ctx context.Context, params SendMessageParams) error {
	s.mutex.Lock()
	var oldCancel context.CancelFunc
	var oldConvID string
	if s.currentCancel != nil {
		oldCancel = s.currentCancel
		oldConvID = s.currentConvID
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	s.currentCancel = cancel
	s.currentConvID = params.ConversationID
	s.mutex.Unlock()
	if oldCancel != nil {
		oldCancel()
		if oldConvID != "" {
			s.emitForConv(oldConvID, "stopped", nil)
		}
	}
	defer func() {
		s.mutex.Lock()
		s.currentCancel = nil
		s.currentConvID = ""
		s.mutex.Unlock()
	}()

	convID := params.ConversationID
	if convID == "" {
		conv := &store.Conversation{Title: "新对话"}
		if err := store.CreateConversation(s.db, conv, secrets.CipherKey(s.cipher)); err != nil {
			s.emitForConv("", "error", enhanceErrorWithHint(fmt.Sprintf("创建对话失败: %v", err)))
			return fmt.Errorf("create conversation: %w", err)
		}
		convID = conv.ID
		s.mutex.Lock()
		s.currentConvID = convID
		s.mutex.Unlock()
		s.emitForConv(convID, "conversation_created", &Conversation{
			ID:        conv.ID,
			Title:     conv.Title,
			CreatedAt: conv.CreatedAt.Format(time.RFC3339),
			UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
		})
	}

	userContent := params.Content

	userMsg := &store.Message{
		ConversationID: convID,
		Role:           "user",
		Content:        params.Content,
	}
	if len(params.Images) > 0 {
		imgJSON, _ := json.Marshal(params.Images)
		userMsg.Images = string(imgJSON)
	}
	if len(params.Attachments) > 0 {
		attJSON, _ := json.Marshal(params.Attachments)
		userMsg.Attachments = string(attJSON)
	}
	if err := store.CreateMessage(s.db, userMsg, secrets.CipherKey(s.cipher)); err != nil {
		s.emitForConv(convID, "error", enhanceErrorWithHint(fmt.Sprintf("保存消息失败: %v", err)))
		return fmt.Errorf("save user message: %w", err)
	}
	emitMsg := &Message{
		ID:             userMsg.ID,
		ConversationID: userMsg.ConversationID,
		Role:           userMsg.Role,
		Content:        userMsg.Content,
		Images:         userMsg.Images,
		CreatedAt:      userMsg.CreatedAt.Format(time.RFC3339),
	}
	if len(params.Attachments) > 0 {
		emitMsg.Attachments = make([]AttachmentSummary, 0, len(params.Attachments))
		for _, a := range params.Attachments {
			emitMsg.Attachments = append(emitMsg.Attachments, AttachmentSummary{
				Type:     a.Type,
				Name:     a.Name,
				MimeType: a.MimeType,
			})
		}
	}
	s.emitForConv(convID, "user_message", emitMsg)

	dbMsgs, err := store.GetMessagesByConversation(s.db, convID, secrets.CipherKey(s.cipher))
	if err != nil {
		s.emitForConv(convID, "error", enhanceErrorWithHint(fmt.Sprintf("加载消息失败: %v", err)))
		return fmt.Errorf("load messages: %w", err)
	}

	var searchContext string
	var searchResp *search.SearchResponse
	caps := s.GetModelCapabilities()
	cfg := s.getConfigSnapshot()
	// 不支持 tool call 的模型，在 "auto" 和 "on" 模式下都预搜索
	if (params.SearchMode == "auto" || params.SearchMode == "on") && !caps.ToolCallSupport {
		s.emitForConv(convID, "search_start", userContent)
		searchResp = s.doSearch(cancelCtx, userContent)
		if searchResp != nil && len(searchResp.Results) > 0 {
			s.emitForConv(convID, "search_result", searchResp.Results)
			searchContext = formatSearchResultsWithLang(searchResp.Results, detectLanguage(userContent))
			ctxSize := 0
			if cfg != nil {
				ctxSize = cfg.ContextSize
			}
			searchContext = truncateSearchContext(searchContext, ctxSize)
		} else {
			s.emitForConv(convID, "search_result", []search.SearchResult{})
			if searchResp != nil && searchResp.Error != "" && len(searchResp.Results) == 0 {
				log.Info().Str("error", searchResp.Error).Msg("[search] 搜索未返回结果")
			}
		}
	}

	llmMessages, trimmed, err := s.buildLLMMessages(cancelCtx, convID, dbMsgs, userContent, params.Attachments, params.SearchMode, searchContext)
	if err != nil {
		s.emitForConv(convID, "error", enhanceErrorWithHint(err.Error()))
		return err // L-13：修正缩进，原 return 与 if 体不对齐易误读为函数顶层
	}

	if trimmed {
		s.emitForConv(convID, "context_trimmed", map[string]any{
			"reason": "preventive_trim",
		})
	}

	return s.streamWithSearch(cancelCtx, convID, llmMessages, params.SearchMode, params.Content, params.Content, searchResp)
}

func (s *Service) streamWithSearch(cancelCtx context.Context, convID string, llmMessages []llm.ChatMessage, searchMode string, _ string, titleContent string, searchResp *search.SearchResponse) error {
	acc := NewStreamAccumulator(convID, s.emit, s.emitForConv)
	// 降频控制：token 速度每 500ms 发射一次，prompt 进度每 200ms 发射一次
	// 生活类比：就像汽车仪表盘的速度表，不需要每毫秒刷新，每 500ms 跳一下数字人眼完全看不出来差异
	// 但能把每 token 一次 IPC 降低到每秒 2 次，高速生成（100+ t/s）时开销降低 98%
	var lastSpeedEmit time.Time
	const speedEmitInterval = 500 * time.Millisecond
	var lastProgressEmit time.Time
	const progressEmitInterval = 200 * time.Millisecond

	// 设置 timings 回调：合并原 token_speed + generation_speed 为单一事件
	// payload 同时包含两种字段命名，前端 handleTokenSpeed 一次性更新会话级和全局速度
	acc.OnTimings = func(timings llm.SSETimings) {
		now := time.Now()
		if now.Sub(lastSpeedEmit) < speedEmitInterval {
			return
		}
		lastSpeedEmit = now
		s.emitForConv(convID, "token_speed", map[string]any{
			"tokensPerSecond":   timings.PredictedPerSecond,
			"predictedN":        timings.PredictedN,
			"tokens_per_second": timings.PredictedPerSecond, // 兼容原 generation_speed 消费者
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

	caps := s.GetModelCapabilities()
	// 在函数入口获取快照，避免数据竞争
	cfg := s.getConfigSnapshot()
	client := s.getClientSnapshot()

	if searchResp != nil && len(searchResp.Results) > 0 {
		sj, _ := json.Marshal(searchResp.Results)
		acc.LastSearchJSON = string(sj)
	}

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
	}
	// 支持 tool call 的模型，在 "auto" 和 "on" 模式下提供工具
	if (searchMode == "auto" || searchMode == "on") && caps.ToolCallSupport {
		req.Tools = []llm.ToolDefinition{searchToolDef}
		// tool schema 定义约占 250 tokens，需计入上下文估算
		req.MaxTokens = s.calcMaxTokens(estimateMessagesTokens(llmMessages) + 250)
	}

	req.Messages = llmMessages

	s.applyThinkingControl(req)
	s.applySamplingParams(req)

	streamCtx, streamCancel := context.WithTimeout(cancelCtx, streamRequestTimeout)
	defer streamCancel()
	defer s.setCurrentCompletionID("") // 流结束后清除 completion ID

	// 包装 callback，在收到 completion ID 时同步到 Service（供 StopThinking 使用）
	innerCallback := acc.callback()
	wrappedCallback := func(chunk llm.SSEChunk) error {
		// 先调用 innerCallback 让它缓存 chunk.ID 到 acc.CompletionID
		err := innerCallback(chunk)
		if err != nil {
			return err
		}
		// 再同步到 Service，确保首个带 ID 的 chunk 也能立即被 StopThinking 使用
		if acc.CompletionID != "" {
			s.setCurrentCompletionID(acc.CompletionID)
		}
		return nil
	}

	// 启用 SSE Replay Buffer：传入 convID 让 llama-server 缓冲 SSE 字节
	// 当 HTTP 连接断开但 llama-server 仍在运行时，可通过 GET /v1/stream/:conv_id 恢复
	err := client.StreamChatWithConvID(streamCtx, req, convID, wrappedCallback)

	if err != nil {
		if cancelCtx.Err() == context.Canceled {
			s.savePartialContentIfAny(convID, acc)
			s.emitForConv(convID, "stopped", nil)
			return nil
		}
		if streamCtx.Err() == context.DeadlineExceeded {
			s.emitForConv(convID, "error", enhanceErrorWithHint("生成超时，请重试"))
			return fmt.Errorf("stream chat timeout")
		}

		retryConvID := convID + "::retry"
		handled, retryErr := s.retryStreamAfterContextExceeded(
			cancelCtx, convID, retryConvID, client, req, cfg.ContextSize, err, acc,
			"[chat] context exceeded, trimming and retrying",
			"stream chat (retry after context trim): %w",
			"stream chat: %w",
		)
		if handled {
			return retryErr
		}
	}

	if acc.FinishReason == "tool_calls" && len(acc.toolCalls()) > 0 {
		if err := s.handleToolCallLoop(cancelCtx, convID, llmMessages, acc, 3); err != nil {
			return err
		}
	} else {
		// 记录 prompt_tokens 反馈校准数据
		if acc.PromptTokens > 0 {
			estimated := estimateMessagesTokens(llmMessages)
			s.tokenCalibMu.Lock()
			s.lastPromptTokens = acc.PromptTokens
			s.lastEstimatedTokens = estimated
			s.tokenCalibMu.Unlock()
			log.Debug().Int("actual", acc.PromptTokens).Int("estimated", estimated).Float64("ratio", float64(acc.PromptTokens)/float64(max(estimated, 1))).Msg("[chat] token estimation calibration")
		}

		// 空内容不保存（与 savePartialContentIfAny 保持一致），避免产生空 assistant 消息
		// 生活类比：录音机没录到任何声音，就不保存空录音文件
		content := acc.FullContent.String()
		thinkingContent := acc.FullThinking.String()
		if content != "" || thinkingContent != "" {
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
		}
	}

	conv, err := store.GetConversation(s.db, convID, secrets.CipherKey(s.cipher))
	if err != nil {
		log.Error().Err(err).Str("convID", convID).Msg("[chat] 无法获取会话以更新标题")
	} else if conv != nil {
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

	s.emitForConv(convID, "done", nil)
	return nil
}

func generateConversationTitle(content string) string {
	// 去除首尾空白
	content = strings.TrimSpace(content)

	// 如果内容为空，返回默认标题
	if content == "" {
		return "新对话"
	}

	// 过滤掉无意义的纯标点/表情符号
	hasMeaningfulChar := false
	for _, r := range content {
		// 检查是否是有意义的字符（字母、数字、汉字等）
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			(r >= 0x4e00 && r <= 0x9fff) { // 汉字范围
			hasMeaningfulChar = true
			break
		}
	}

	if !hasMeaningfulChar {
		return "新对话"
	}

	// 将最大长度从30增加到50
	maxLen := 50
	runes := []rune(content)

	if len(runes) <= maxLen {
		return content
	}

	// 尝试在合适的位置截断（空格、标点符号处）
	truncateAt := maxLen

	// 从后向前搜索合适的截断点（在前40-50字符范围内）
	// 修复（M-后1）：原代码 i++ 导致从位置50向后递增搜索，与"从后向前"语义相反，
	// 截断点可能落在50字符之后使标题更长。改为 i-- 从50向40递减搜索最近的分隔符。
	for i := maxLen; i >= 40 && i < len(runes); i-- {
		r := runes[i]
		// 检查是否是适合截断的字符
		if r == ' ' || r == '，' || r == ',' || r == '。' || r == '.' ||
			r == '！' || r == '!' || r == '？' || r == '?' ||
			r == '；' || r == ';' || r == '：' || r == ':' ||
			r == '\n' || r == '\t' {
			truncateAt = i
			break
		}
	}

	// 提取截断前的内容并添加省略号
	title := string(runes[:truncateAt])
	title = strings.TrimSpace(title)

	// 确保我们不会返回空字符串
	if title == "" {
		title = string(runes[:maxLen])
	}

	return title + "…"
}

// 测试导出函数
func ClampDuration(d float64) float64                { return clampDuration(d) }              // Exported for testing
func CalcMaxTokens(s *Service, promptTokens int) int { return s.calcMaxTokens(promptTokens) } // Exported for testing
func DoSearch(s *Service, ctx context.Context, query string) *search.SearchResponse { // Exported for testing
	return s.doSearch(ctx, query)
}
func ResetForNextCall(a *StreamAccumulator)             { a.resetForNextCall() } // Exported for testing
func GetFirstRoundThinking(a *StreamAccumulator) string { return a.FirstRoundThinking }
