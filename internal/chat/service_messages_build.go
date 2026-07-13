// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/store"
)

// searchPreCallSeq 弱模型路径预搜索的 tool_call ID 自增序列
// 用原子计数器保证每次调用生成唯一 ID，避免多轮对话中 ID 重复
var searchPreCallSeq int64

// buildLLMMessages 构建发送给 LLM 的消息列表。
//
// 拆分说明：原 216 行函数按职责拆为调度器 + 6 子函数：
//   - validateAttachments: 校验附件与模型能力匹配
//   - resolveSystemContent: 构建系统提示词（含缓存）
//   - calculateContextBudget: 估算 token 并计算上下文预算
//   - buildOverflowMessages: 降级路径（上下文超限时的压缩处理）
//   - buildNormalMessages: 正常路径（历史消息构建 + 按需压缩）
//   - appendAuxiliaryContext: 追加 RAG 和搜索上下文
//
// 生活类比：就像准备一份会议材料——先检查设备（validateAttachments），写好开场白（resolveSystemContent），
// 估算材料厚度（calculateContextBudget），如果太厚就精简（buildOverflowMessages），否则正常整理（buildNormalMessages），
// 最后附上参考资料和搜索结果（appendAuxiliaryContext）。
func (s *Service) buildLLMMessages(ctx context.Context, convID string, dbMsgs []*store.Message, currentUserContent string, currentAttachments []Attachment, searchMode string, searchContext string) ([]llm.ChatMessage, bool, error) {
	cfg := s.getConfigSnapshot()
	maxContext := 0
	if cfg != nil {
		maxContext = cfg.ContextSize
	}
	if maxContext <= 0 {
		maxContext = 4096
	}

	s.modelCapsMu.RLock()
	caps := s.modelCaps
	s.modelCapsMu.RUnlock()

	// 校验附件与模型能力
	if err := validateAttachments(caps, currentAttachments); err != nil {
		return nil, false, err
	}

	now := time.Now()
	systemContent := s.resolveSystemContent(cfg, searchMode, caps, now)
	ragContext := s.retrieveRAGContext(ctx, cfg, currentUserContent)

	estimatedTokens, effectiveMax := s.calculateContextBudget(cfg, maxContext, systemContent, ragContext, dbMsgs, currentAttachments)

	// 降级路径：估算总 token 超限时，直接走压缩
	currentMsgTokens := estimateCurrentMessageTokens(dbMsgs, currentAttachments)
	var messages []llm.ChatMessage
	var overflow bool
	if estimatedTokens+currentMsgTokens > effectiveMax {
		var err error
		messages, err = s.buildOverflowMessages(systemContent, dbMsgs, currentUserContent, currentAttachments, maxContext, convID, effectiveMax)
		if err != nil {
			return nil, false, err
		}
		overflow = true
	} else {
		// 正常路径
		var err error
		messages, err = s.buildNormalMessages(systemContent, dbMsgs, currentUserContent, currentAttachments, caps, estimatedTokens, effectiveMax, maxContext, convID)
		if err != nil {
			return nil, false, err
		}
	}

	messages = appendAuxiliaryContext(messages, ragContext, searchContext, currentUserContent)
	// 模型不支持 system role 时（如 Gemma 系列），把 system 消息内容合并到第一条 user 消息前
	// 避免 llama.cpp 渲染模板时因不认识 system role 而报错
	if !caps.SupportsSystemRole {
		messages = mergeSystemIntoUser(messages)
	}
	return messages, overflow, nil
}

// mergeSystemIntoUser 把所有 system 消息的内容合并到第一条 user 消息前面
// 用于不支持 system role 的模型（如 Gemma 系列）
//
// 合并后的 user 消息格式：
//   <原 system 内容>
//
//   <原 user 内容>
//
// 若没有 user 消息，则把 system 内容转成一条 user 消息
func mergeSystemIntoUser(messages []llm.ChatMessage) []llm.ChatMessage {
	// 收集所有 system 消息内容
	var systemParts []string
	var nonSystem []llm.ChatMessage
	for _, m := range messages {
		if m.Role == "system" {
			if s, ok := m.Content.(string); ok && s != "" {
				systemParts = append(systemParts, s)
			}
		} else {
			nonSystem = append(nonSystem, m)
		}
	}
	if len(systemParts) == 0 {
		// 没有 system 内容可合并，但可能存在空 system 消息，需要移除
		if len(nonSystem) == len(messages) {
			return messages // 没有 system 消息，原样返回
		}
		return nonSystem // 移除空 system 消息
	}
	systemContent := strings.Join(systemParts, "\n\n")

	// 找第一条 user 消息，把 system 内容合并到前面
	result := make([]llm.ChatMessage, 0, len(nonSystem))
	merged := false
	for _, m := range nonSystem {
		if !merged && m.Role == "user" {
			if s, ok := m.Content.(string); ok {
				m.Content = systemContent + "\n\n" + s
			} else {
				// content 不是字符串（可能是 typed content），无法简单合并
				// 退而求其次：在 user 消息前插入一条 user 消息承载 system 内容
				result = append(result, llm.ChatMessage{Role: "user", Content: systemContent})
			}
			merged = true
		}
		result = append(result, m)
	}
	// 没有 user 消息，把 system 内容转成 user 消息
	if !merged {
		result = append(result, llm.ChatMessage{Role: "user", Content: systemContent})
	}
	return result
}

// validateAttachments 校验附件类型是否被当前模型支持。
func validateAttachments(caps llm.ModelCapabilities, attachments []Attachment) error {
	for _, att := range attachments {
		if att.Type == "image" && !caps.ImageInput {
			return fmt.Errorf("当前模型不支持图片输入，请加载支持视觉的模型（如 llava 系列）")
		}
		if att.Type == "audio" && !caps.AudioInput {
			return fmt.Errorf("当前模型不支持音频输入，请加载支持音频的模型（如 whisper 系列）")
		}
	}
	return nil
}

// resolveSystemContent 构建系统提示词，支持基于日期和配置的缓存。
// 当日期变化或配置变更时重建缓存，否则复用缓存。
func (s *Service) resolveSystemContent(cfg *config.Config, searchMode string, caps llm.ModelCapabilities, now time.Time) string {
	today := now.Format("2006-01-02")
	configPrompt := ""
	systemPromptMode := "append"
	if cfg != nil {
		configPrompt = cfg.SystemPrompt
		systemPromptMode = cfg.SystemPromptMode
	}

	// 检查缓存是否命中
	s.promptMu.RLock()
	cacheHit := s.sysPromptCache != "" && s.sysPromptDate == today && s.sysPromptConfig == configPrompt
	cachedPrompt := s.sysPromptCache
	s.promptMu.RUnlock()

	if !cacheHit {
		s.detectedModelMu.RLock()
		modelName := s.detectedModelName
		s.detectedModelMu.RUnlock()
		if modelName == "" {
			modelName = "本地模型"
		}
		base := buildBaseSystemPrompt(modelName, configPrompt, systemPromptMode)
		s.promptMu.Lock()
		s.sysPromptCache = base
		s.sysPromptDate = today
		s.sysPromptConfig = configPrompt
		s.promptMu.Unlock()
		cachedPrompt = base
	}

	return applyDynamicSystemPrompt(cachedPrompt, searchMode, caps, now)
}

// calculateContextBudget 估算系统提示词和 RAG 的 token 数，并计算有效上下文上限。
// 返回 (estimatedTokens, effectiveMax)。
// effectiveMax = maxContext - reserve，其中 reserve 取 max(maxContext/10, 512) 和主动压缩预留的较大值。
func (s *Service) calculateContextBudget(cfg *config.Config, maxContext int, systemContent string, ragContext string, dbMsgs []*store.Message, currentAttachments []Attachment) (int, int) {
	estimatedTokens := estimateTokensByLang(systemContent, detectLanguage(systemContent)) + 10
	if ragContext != "" {
		estimatedTokens += estimateTokensByLang(ragContext, detectLanguage(ragContext)) + 10
	}

	// 利用历史 prompt_tokens 反馈校准估算系数
	s.tokenCalibMu.RLock()
	calibActual := s.lastPromptTokens
	calibEstimated := s.lastEstimatedTokens
	s.tokenCalibMu.RUnlock()
	calibRatio := 1.0
	if calibEstimated > 0 && calibActual > 0 {
		calibRatio = float64(calibActual) / float64(calibEstimated)
		if calibRatio < 1.0 {
			calibRatio = 1.0
		} else if calibRatio > 3.0 {
			calibRatio = 3.0
		}
		estimatedTokens = int(float64(estimatedTokens) * calibRatio)
	}

	reserve := max(maxContext/10, 512)
	// P1-A1: 主动压缩阈值 - 当估算接近上限时提前压缩，避免到溢出边缘才动。
	proactiveThreshold := cfg.ProactiveCompressThreshold
	if proactiveThreshold <= 0 || proactiveThreshold > 0.95 {
		proactiveThreshold = 0.8
	}
	proactiveReserve := int(float64(maxContext) * (1.0 - proactiveThreshold))
	if proactiveReserve > reserve {
		reserve = proactiveReserve
	}
	effectiveMax := maxContext - reserve
	return estimatedTokens, effectiveMax
}

// estimateCurrentMessageTokens 估算当前消息（最后一条历史消息 + 附件）的 token 数。
func estimateCurrentMessageTokens(dbMsgs []*store.Message, currentAttachments []Attachment) int {
	if len(dbMsgs) == 0 {
		return 0
	}
	currentMsgTokens := estimateMessageTokens(dbMsgs[len(dbMsgs)-1])
	if currentMsgTokens == 0 {
		currentMsgTokens = 1
	}
	for _, att := range currentAttachments {
		currentMsgTokens += EstimateAttachmentTokensWithData(att.Type, att.Data)
	}
	return currentMsgTokens
}

// buildOverflowMessages 降级路径：上下文严重超限时，调用 CompressContext 进行统一压缩。
// 摘要作为独立 system 消息插入（不拼到 system prompt 末尾）。
func (s *Service) buildOverflowMessages(systemContent string, dbMsgs []*store.Message, currentUserContent string, currentAttachments []Attachment, maxContext int, convID string, effectiveMax int) ([]llm.ChatMessage, error) {
	var lastMsg llm.ChatMessage
	hasLastMsg := false
	if len(dbMsgs) > 0 {
		dbLastMsg := dbMsgs[len(dbMsgs)-1]
		content := currentUserContent
		if content == "" && (dbLastMsg.Images != "" || dbLastMsg.Attachments != "") {
			content = "请描述这张图片"
		}
		if len(currentAttachments) > 0 {
			lastMsg = buildMessageFromAttachments(dbLastMsg.Role, content, currentAttachments)
		} else {
			lastMsg = llm.NewTextMessage(dbLastMsg.Role, content)
		}
		hasLastMsg = true
	}

	baseMessages := []llm.ChatMessage{
		{Role: "system", Content: systemContent},
	}
	if hasLastMsg {
		baseMessages = append(baseMessages, lastMsg)
	}

	existingSummary := ""
	if convID != "" {
		existingSummary, _ = store.GetConversationSummary(s.db, convID)
	}
	client := s.getClientSnapshot()
	result := CompressContext(baseMessages, maxContext, existingSummary, dbMsgs, client, convID, s.db)
	messages := result.Messages

	// 如果 CompressContext 返回的消息仍然超限（极端情况），fallback 到只保留 system + 最后一条消息
	if estimateMessagesTokens(messages) > effectiveMax {
		messages = baseMessages
		log.Warn().Int("effective_max", effectiveMax).Msg("[buildLLMMessages] 降级路径压缩后仍超限，fallback 到最小消息")
	}

	log.Info().Int("trimmed_count", result.TrimmedCount).Bool("summary_inserted", result.SummaryInserted).Str("convID", convID).Msg("[buildLLMMessages] 降级路径上下文已压缩")
	return messages, nil
}

// buildNormalMessages 正常路径：构建历史消息，如有裁剪则压缩。
func (s *Service) buildNormalMessages(systemContent string, dbMsgs []*store.Message, currentUserContent string, currentAttachments []Attachment, caps llm.ModelCapabilities, estimatedTokens int, effectiveMax int, maxContext int, convID string) ([]llm.ChatMessage, error) {
	history, trimmedMsgs := s.buildHistoryFromDB(dbMsgs, currentUserContent, currentAttachments, caps, estimatedTokens, effectiveMax)
	history = cleanHistoryMessages(history)

	baseMessages := []llm.ChatMessage{
		{Role: "system", Content: systemContent},
	}
	baseMessages = append(baseMessages, history...)

	// 如果有消息被裁剪，调用 CompressContext 进行统一压缩
	var messages []llm.ChatMessage
	if len(trimmedMsgs) > 0 && convID != "" {
		existingSummary, _ := store.GetConversationSummary(s.db, convID)
		client := s.getClientSnapshot()
		result := CompressContext(baseMessages, maxContext, existingSummary, trimmedMsgs, client, convID, s.db)
		messages = result.Messages
		log.Info().Int("trimmed_count", result.TrimmedCount).Bool("summary_inserted", result.SummaryInserted).Str("convID", convID).Msg("[buildLLMMessages] 上下文已压缩")
	} else {
		messages = baseMessages
	}
	return messages, nil
}

// appendAuxiliaryContext 追加 RAG 参考资料和搜索结果到消息列表。
// RAG 上下文作为独立 system 消息插入（在所有 system 消息之后、history 之前）。
// 搜索结果以 assistant(tool_call) + tool(搜索结果) 格式追加（模拟工具响应流）。
func appendAuxiliaryContext(messages []llm.ChatMessage, ragContext string, searchContext string, currentUserContent string) []llm.ChatMessage {
	// 将 RAG 参考资料作为独立的 system 上下文消息
	if ragContext != "" {
		insertIdx := 0
		for i, m := range messages {
			if m.Role != "system" {
				insertIdx = i
				break
			}
			insertIdx = i + 1
		}
		ragMsg := llm.ChatMessage{Role: "system", Content: ragContext}
		messages = append(messages[:insertIdx], append([]llm.ChatMessage{ragMsg}, messages[insertIdx:]...)...)
	}

	// 搜索结果以 assistant(tool_call) + tool(搜索结果) 格式追加
	if searchContext != "" {
		// 使用原子计数器生成唯一 ID，避免多轮对话中 ID 重复
		// 注：不用 time.Now().UnixNano()，因为 Windows 系统时间精度低，连续调用可能返回相同值
		toolCallID := fmt.Sprintf("search_pre_%d", atomic.AddInt64(&searchPreCallSeq, 1))
		messages = append(messages, llm.ChatMessage{
			Role:    "assistant",
			Content: "",
			ToolCalls: []llm.ToolCall{{
				ID:   toolCallID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      "search",
					Arguments: fmt.Sprintf(`{"query":%q}`, currentUserContent),
				},
			}},
		})
		lang := detectLanguage(currentUserContent)
		messages = append(messages, llm.ChatMessage{
			Role:       "tool",
			Content:    searchContext + searchResultInstruction(lang),
			ToolCallID: toolCallID,
		})
	}

	return messages
}
