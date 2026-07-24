// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
	"douya/internal/config"
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

// defaultSsePingInterval 流式请求默认 SSE ping 间隔（秒）
// 防止大上下文 prefill 慢时连接被误断：静默超过此间隔时发送 ping，3s 后才 kick
// 5 秒间隔平衡稳定性和开销（WebUI 用 1s，默认 30s 太长）
var defaultSsePingInterval = 5

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

// buildAvailableTools 构建当前可用的工具列表（search + MCP 工具）。
// 生活类比：把自家的招牌菜（search）和各外卖平台的菜品（MCP 工具）合并到一张菜单上。
// includeSearch: 是否包含 search 工具（在 tool call 循环中始终包含，首次请求取决于 searchMode）
func (s *Service) buildAvailableTools(includeSearch bool) []llm.ToolDefinition {
	var tools []llm.ToolDefinition

	// 添加 search 工具
	if includeSearch {
		tools = append(tools, searchToolDef)
	}

	// 添加 MCP 工具（如果 MCP Manager 已连接）
	mgr := s.getMCPManager()
	if mgr != nil {
		for _, t := range mgr.ListTools() {
			tools = append(tools, llm.ToolDefinition{
				Type: "function",
				Function: llm.FunctionDef{
					Name:        t.Name,
					Description: t.Description,
					Parameters:  t.InputSchema,
				},
			})
		}
	}

	return tools
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
	TokenBuf                   strings.Builder                      // token 批量化累积缓冲（减少 IPC 频率）
	LastTokenEmit              time.Time                            // 上次 token 发射时间（用于时间触发 flush）
}

// 流式响应缓冲区最大大小（10MB）
const maxStreamBufferSize = 10 * 1024 * 1024

// token 批量化降频阈值
// 首 token 立即发送（消除首字延迟）；后续累积到 tokenBatchSize 字符或 tokenBatchInterval 间隔再发射
// 减少高频 IPC 跨进程调用，避免长内容时前端 IPC 队列积压导致 token 到达节奏不均匀
const (
	tokenBatchSize     = 48                    // 累积字符数阈值（约 1-2 个中文句子，视觉上仍是逐句流式）
	tokenBatchInterval = 16 * time.Millisecond // 时间间隔阈值（对齐 60fps，保证每帧有 token 到达）
)

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
			return apperror.Newf(apperror.KindInvalidInput, "response exceeds maximum buffer size (%dMB)", maxStreamBufferSize/1024/1024)
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
			// 批量化降频：首 token 立即发送，后续累积到阈值或时间间隔再发射
			a.emitTokenBatched(fixed)
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
			// 流式结束前 flush 残留的 token 缓冲，确保最终内容完整送达
			a.flushTokenBuffer()
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

// emitTokenBatched 批量化发射 token 事件
// 首 token 立即发送（消除首字延迟）；后续累积到 tokenBatchSize 字符或 tokenBatchInterval 间隔再发射
// 减少高频 IPC 跨进程调用，避免长内容时前端 IPC 队列积压
func (a *StreamAccumulator) emitTokenBatched(fixed string) {
	// 首 token：FullContent 刚写入，长度等于本次 fixed 长度（首字延迟是用户感知最强的卡顿源，不降频）
	if a.FullContent.Len() == len(fixed) {
		a.EmitForConvFn(a.ConvID, "token", fixed)
		a.LastTokenEmit = time.Now()
		return
	}
	a.TokenBuf.WriteString(fixed)
	// 累积到阈值或时间间隔：发射缓冲区内容
	if a.TokenBuf.Len() >= tokenBatchSize || time.Since(a.LastTokenEmit) >= tokenBatchInterval {
		a.EmitForConvFn(a.ConvID, "token", a.TokenBuf.String())
		a.TokenBuf.Reset()
		a.LastTokenEmit = time.Now()
	}
}

// flushTokenBuffer 发射缓冲区中残留的 token（流式结束/中断时调用，确保最终内容完整送达）
func (a *StreamAccumulator) flushTokenBuffer() {
	if a.TokenBuf.Len() > 0 {
		a.EmitForConvFn(a.ConvID, "token", a.TokenBuf.String())
		a.TokenBuf.Reset()
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

// streamExecResult 描述流式请求执行后的状态，供 runStreamWithStandardErrors 返回。
//
// 生活类比：流水线工人完成任务后，向车间主任汇报"继续生产"、"停线检修"或"停线下班"。
type streamExecResult int

const (
	// streamContinue 表示流式请求成功完成，调用方应继续后续流程（如保存消息、tool call 循环等）。
	streamContinue streamExecResult = iota
	// streamStopped 表示因用户取消或不可恢复错误而终止，调用方应直接返回（不再执行后续流程）。
	// 配合返回的 err：err == nil 表示用户主动取消；err != nil 表示不可恢复错误。
	streamStopped
)

// wrapCallbackWithCompletionID 包装 accumulator 的 callback，在收到 completion ID 时同步到 Service。
//
// 抽取原因（基于 B-1.1）：executeStreamAndHandleErrors 与 executeToolCallStream 中
// 存在完全相同的 callback 包装代码，提取为公共函数消除重复。
//
// 生活类比：就像给快递员配一个对讲机——遇到包裹（completion ID）就立即上报给调度中心（Service），
// 调度中心记录下来供后续操作（如 StopThinking）使用。
func (s *Service) wrapCallbackWithCompletionID(acc *StreamAccumulator) func(llm.SSEChunk) error {
	inner := acc.callback()
	return func(chunk llm.SSEChunk) error {
		if err := inner(chunk); err != nil {
			return err
		}
		if acc.CompletionID != "" {
			s.setCurrentCompletionID(acc.CompletionID)
		}
		return nil
	}
}

// runStreamWithStandardErrors 执行流式请求并统一处理三类标准错误：
// 用户取消、流式超时、上下文溢出（自动裁剪重试）。
//
// 抽取原因（基于 B-1.1+B-1.2+B-1.3）：executeStreamAndHandleErrors 与 executeToolCallStream
// 中存在近乎相同的错误处理流程（约 30 行重复代码），提取为公共函数统一维护，
// 避免一处改漏导致两处行为不一致。
//
// 参数说明：
//   - streamCtx：流式请求的 context（含超时）
//   - cancelCtx：用户取消 context（用于检测主动取消）
//   - convID：会话 ID（用于 emit 事件、保存部分内容）
//   - streamConvID：流式请求实际使用的 convID（tool call 每轮使用独立 convID 避免 SSE Replay Buffer 冲突）
//   - timeoutMsg：超时时的用户提示文案
//   - timeoutErr：超时时返回的 error（直接返回，不做格式化）
//   - logMsg/retryErrFmt/nonExceedErrFmt：透传给 retryStreamAfterContextExceeded
//
// 返回值约定：
//   - streamContinue, nil：流式请求成功，调用方继续后续流程
//   - streamStopped, nil：用户主动取消，调用方应直接返回 nil
//   - streamStopped, err：不可恢复错误（超时/重试失败/非溢出错误），调用方应返回 err
//
// 生活类比：像一个标准化的快递配送流程——不管哪个站点，遇到"客户取消"、"超时未送达"、
// "包裹太大需重新打包"这三种情况都按统一规则处理，站点只需填好对应的提示文案和错误信息。
func (s *Service) runStreamWithStandardErrors(
	streamCtx context.Context,
	cancelCtx context.Context,
	convID string,
	streamConvID string,
	client *llm.Client,
	req *llm.ChatCompletionRequest,
	acc *StreamAccumulator,
	cfg *config.Config,
	timeoutMsg string,
	timeoutErr error,
	logMsg string,
	retryErrFmt string,
	nonExceedErrFmt string,
) (streamExecResult, error) {
	wrapped := s.wrapCallbackWithCompletionID(acc)
	err := client.StreamChatWithConvID(streamCtx, req, streamConvID, wrapped)
	if err == nil {
		return streamContinue, nil
	}

	// 用户主动取消：保存已生成内容，emit stopped，返回 nil（视为正常结束）
	if cancelCtx.Err() == context.Canceled {
		s.savePartialContentIfAny(convID, acc)
		s.emitForConv(convID, "stopped", nil)
		return streamStopped, nil
	}

	// 流式超时：emit error 并返回超时错误
	if streamCtx.Err() == context.DeadlineExceeded {
		s.emitForConv(convID, "error", enhanceErrorWithHint(timeoutMsg))
		return streamStopped, timeoutErr
	}

	// 上下文溢出：自动裁剪并重试
	// retryConvID 由 streamConvID 派生，确保与原流式请求使用不同的 SSE Replay Buffer
	retryConvID := streamConvID + "::retry"
	handled, retryErr := s.retryStreamAfterContextExceeded(
		cancelCtx, convID, retryConvID, client, req, cfg.ContextSize, err, acc,
		logMsg, retryErrFmt, nonExceedErrFmt,
	)
	if handled {
		return streamStopped, retryErr
	}
	// 重试成功，调用方继续后续流程
	return streamContinue, nil
}

func (s *Service) SendMessage(ctx context.Context, params SendMessageParams) error {
	log.Info().
		Str("convID", params.ConversationID).
		Str("searchMode", params.SearchMode).
		Int("attachments", len(params.Attachments)).
		Int("images", len(params.Images)).
		Msg("[chat] SendMessage 入口")
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
			return apperror.Wrap(apperror.KindInternal, "create conversation", err)
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
		return apperror.Wrap(apperror.KindInternal, "save user message", err)
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
		return apperror.Wrap(apperror.KindInternal, "load messages", err)
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
