package chat

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"douya/internal/apperror"
	"douya/internal/config"

	"douya/internal/secrets"
	"douya/internal/store"

	"github.com/rs/zerolog/log"
)

// StopGeneration stops the current generation (if any).
// 优先通过 DELETE /v1/stream/:conv_id 优雅停止（让 llama-server 立即停止推理并释放资源），
// 同时取消 context 作为兜底确保连接断开。
// 生活类比：就像挂断电话，先礼貌地说"再见"让对方停止说话，然后挂断线路
func (s *Service) StopGeneration() {
	// 快速加锁拷贝状态后立即释放，避免网络 I/O 阻塞 SendMessage 的 defer 清理
	s.mutex.Lock()
	convID := s.currentConvID
	cancelFn := s.currentCancel
	s.currentCancel = nil
	s.currentConvID = ""
	s.mutex.Unlock()

	// 优先调用 DELETE 端点优雅停止（基于 SSE Replay Buffer 功能）
	if convID != "" {
		client := s.getClientSnapshot()
		if client != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := client.DeleteStream(ctx, convID); err != nil {
				log.Debug().Err(err).Str("conv_id", convID).Msg("[chat] DeleteStream failed, falling back to context cancel")
			}
			cancel()
		}
	}

	// 兜底：取消 context 确保连接断开
	if cancelFn != nil {
		cancelFn()
	}
}

// GetCurrentCompletionID 返回当前流式聊天的 completion ID（用于 /v1/chat/completions/control）。
func (s *Service) GetCurrentCompletionID() string {
	s.completionIDMu.RLock()
	defer s.completionIDMu.RUnlock()
	return s.currentCompletionID
}

func (s *Service) setCurrentCompletionID(id string) {
	s.completionIDMu.Lock()
	defer s.completionIDMu.Unlock()
	s.currentCompletionID = id
}

// GetConversations returns all conversations.
func (s *Service) GetConversations() ([]*Conversation, error) {
	convs, err := store.ListConversations(s.db, secrets.CipherKey(s.cipher))
	if err != nil {
		log.Error().Err(err).Msg("[chat] GetConversations failed")
		return nil, err
	}
	result := make([]*Conversation, len(convs))
	for i, c := range convs {
		result[i] = &Conversation{
			ID:        c.ID,
			Title:     c.Title,
			CreatedAt: c.CreatedAt.Format(time.RFC3339),
			UpdatedAt: c.UpdatedAt.Format(time.RFC3339),
		}
	}
	return result, nil
}

// CreateConversation creates a new conversation.
func (s *Service) CreateConversation() (*Conversation, error) {
	conv := &store.Conversation{Title: "新对话"}
	err := store.CreateConversation(s.db, conv, secrets.CipherKey(s.cipher))
	if err != nil {
		log.Error().Err(err).Msg("[chat] CreateConversation failed")
		return nil, err
	}
	return &Conversation{
		ID:        conv.ID,
		Title:     conv.Title,
		CreatedAt: conv.CreatedAt.Format(time.RFC3339),
		UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
	}, nil
}

// RenameConversation renames a conversation.
func (s *Service) RenameConversation(id string, title string) error {
	if strings.TrimSpace(title) == "" {
		return apperror.New(apperror.KindInvalidInput, "title cannot be empty")
	}
	conv, err := store.GetConversation(s.db, id, secrets.CipherKey(s.cipher))
	if err != nil {
		return err
	}
	conv.Title = title
	return store.UpdateConversation(s.db, conv, secrets.CipherKey(s.cipher))
}

// DeleteConversation deletes a conversation and all its messages.
func (s *Service) DeleteConversation(id string) error {
	return store.DeleteConversation(s.db, id)
}

// GetMessages returns all messages for a conversation (excluding tool and intermediate messages).
func (s *Service) GetMessages(conversationID string) ([]*Message, error) {
	msgs, err := store.GetMessagesByConversation(s.db, conversationID, secrets.CipherKey(s.cipher))
	if err != nil {
		log.Error().Err(err).Str("convID", conversationID).Msg("[chat] GetMessages failed")
		return nil, err
	}
	// 过滤掉：
	// 1. tool 消息
	// 2. 触发 tool call 的助手消息（中间消息，不显示在聊天界面）
	filtered := make([]*Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "tool" {
			continue
		}
		// 检查 store.Message 的 ToolCalls 字段（转换前）
		if m.Role == "assistant" && strings.TrimSpace(m.ToolCalls) != "" {
			// 这是触发 tool call 的中间消息，不显示在聊天界面
			continue
		}
		filtered = append(filtered, storeMsgToChat(m))
	}
	return filtered, nil
}

// DeleteMessage deletes a message and, if it's a user message, also deletes
// the subsequent assistant reply in the same conversation.
func (s *Service) DeleteMessage(id string) error {
	msg, err := store.GetMessage(s.db, id, secrets.CipherKey(s.cipher))
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "get message", err)
	}

	convID := msg.ConversationID

	deletedIDs := []string{id}

	if msg.Role == "user" {
		msgs, err := store.GetMessagesByConversation(s.db, convID, secrets.CipherKey(s.cipher))
		if err != nil {
			return apperror.Wrap(apperror.KindInternal, "load conversation messages", err)
		}
		found := false
		for _, m := range msgs {
			if m.ID == id {
				found = true
				continue
			}
			if found && m.Role == "assistant" {
				deletedIDs = append(deletedIDs, m.ID)
			}
			if found && m.Role != "assistant" {
				break
			}
		}
	}

	// 批量删除消息（修复 N+1 问题：原实现循环调用 DeleteMessage，每条独立事务）
	// M16 修复：删除失败时不 emit 事件，避免前端 UI 移除消息但 DB 仍保留导致刷新后"复活"
	// 生活类比：仓库销毁清单执行失败时，不能让前台把货品从目录划掉，否则客户下单会找不到货
	if len(deletedIDs) > 0 {
		if delErr := store.DeleteMessagesBatch(s.db, deletedIDs); delErr != nil {
			log.Error().Err(delErr).Msg("[chat] batch delete messages failed")
			return apperror.Wrap(apperror.KindInternal, "batch delete messages", delErr)
		}
		for _, delID := range deletedIDs {
			s.emitForConv(convID, "message_deleted", delID)
		}
	}

	return nil
}

// EditMessage 更新指定消息的正文内容（消息编辑链路的 service 层入口）。
// 仅落库新 content；"截断后续消息 + 重新生成"的编排由前端驱动（见改进计划 C-4），
// 本方法保持单一职责，便于独立测试与复用。
func (s *Service) EditMessage(messageID string, newContent string) error {
	if strings.TrimSpace(newContent) == "" {
		return apperror.New(apperror.KindInvalidInput, "编辑后的内容不能为空")
	}
	if err := store.UpdateMessageContent(s.db, messageID, newContent, secrets.CipherKey(s.cipher)); err != nil {
		log.Error().Err(err).Str("messageID", messageID).Msg("[chat] EditMessage failed")
		return err
	}
	return nil
}

// RegenerateMessage regenerates the last assistant message in a conversation.
func (s *Service) RegenerateMessage(msgID string, searchMode string) error {
	// C-7 修复：用 beginGeneration 统一锁/取消逻辑，消除与 SendMessage 的重复代码
	// M14 修复：hostCtx 可能为 nil（初始化期间或测试环境），用 context.Background() 兜底
	// 生活类比：快递员找不到调度总机时，用离线模式继续工作，而不是当场罢工
	parentCtx := s.getHostContextSnapshot()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	// M3 修复：initialConvID 传 ""，立即清空 currentConvID，避免"获取 targetMsg 前的窗口期"内
	// StopGeneration 读到旧 convID 向错误会话发 "stopped" 事件。
	// 获取到 targetMsg 后再设置正确值（见下方 s.currentConvID = convID）。
	cancelCtx, cleanup := s.beginGeneration(parentCtx, "")
	defer cleanup()

	targetMsg, err := store.GetMessage(s.db, msgID, secrets.CipherKey(s.cipher))
	if err != nil {
		return apperror.Wrapf(apperror.KindNotFound, "message %s not found", err, msgID)
	}

	convID := targetMsg.ConversationID

	// M3 修复：尽早设置 currentConvID，缩短窗口期
	// 此后任何错误返回前，StopGeneration 都能正确针对当前会话操作
	s.mutex.Lock()
	s.currentConvID = convID
	s.mutex.Unlock()

	msgs, err := store.GetMessagesByConversation(s.db, convID, secrets.CipherKey(s.cipher))
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "load messages", err)
	}

	var targetIdx int
	found := false
	for i, m := range msgs {
		if m.ID == msgID {
			targetIdx = i
			found = true
			break
		}
	}
	if !found {
		return apperror.Newf(apperror.KindNotFound, "message %s not found in conversation", msgID)
	}

	var assistantMsgIDs []string
	for i := targetIdx + 1; i < len(msgs); i++ {
		if msgs[i].Role == "assistant" {
			assistantMsgIDs = append(assistantMsgIDs, msgs[i].ID)
		} else {
			break
		}
	}
	// 批量删除消息（修复 N+1 问题：原实现循环调用 DeleteMessage，每条独立事务）
	// M16 修复：删除失败时返回错误，避免后续基于错误状态继续生成
	deletedSet := make(map[string]bool, len(assistantMsgIDs))
	if len(assistantMsgIDs) > 0 {
		if delErr := store.DeleteMessagesBatch(s.db, assistantMsgIDs); delErr != nil {
			log.Error().Err(delErr).Msg("batch delete assistant messages for regeneration")
			return apperror.Wrap(apperror.KindInternal, "batch delete assistant messages", delErr)
		}
		for _, id := range assistantMsgIDs {
			deletedSet[id] = true
			s.emitForConv(convID, "message_deleted", id)
		}
	}

	var userContent string
	var userAttachments []Attachment
	if targetMsg.Role == "user" {
		userContent = targetMsg.Content
		if targetMsg.Attachments != "" {
			if err := json.Unmarshal([]byte(targetMsg.Attachments), &userAttachments); err != nil {
				log.Warn().Err(err).Msg("parse attachments for regeneration")
			}
		}
	}

	// M5: 复用第一次查询结果，本地过滤已删除的 assistant 消息，省掉一次数据库查询
	dbMsgs := make([]*store.Message, 0, len(msgs))
	for _, m := range msgs {
		if !deletedSet[m.ID] {
			dbMsgs = append(dbMsgs, m)
		}
	}

	llmMessages, _, err := s.buildLLMMessages(cancelCtx, convID, dbMsgs, userContent, userAttachments, searchMode, "")
	if err != nil {
		return err
	}

	return s.streamWithSearch(cancelCtx, convID, llmMessages, searchMode, userContent, userContent, nil)
}

// SearchMessages searches messages across all conversations.
func (s *Service) SearchMessages(query string) ([]*Message, error) {
	if strings.TrimSpace(query) == "" {
		return nil, apperror.New(apperror.KindInvalidInput, "query cannot be empty")
	}
	msgs, err := store.SearchMessages(s.db, query, secrets.CipherKey(s.cipher))
	if err != nil {
		log.Error().Err(err).Str("query", query).Msg("[chat] SearchMessages failed")
		return nil, err
	}
	result := make([]*Message, len(msgs))
	for i, m := range msgs {
		result[i] = storeMsgToChat(m)
	}
	return result, nil
}

// ExportConversation exports a conversation.
func (s *Service) ExportConversation(id string, format string) (string, error) {
	conv, err := store.GetConversation(s.db, id, secrets.CipherKey(s.cipher))
	if err != nil {
		return "", err
	}
	msgs, err := store.GetMessagesByConversation(s.db, id, secrets.CipherKey(s.cipher))
	if err != nil {
		return "", err
	}
	// 过滤 tool 消息
	filtered := make([]*store.Message, 0, len(msgs))
	for _, m := range msgs {
		if m.Role != "tool" {
			filtered = append(filtered, m)
		}
	}
	switch strings.ToLower(format) {
	case "markdown", "md":
		return s.exportMarkdown(conv, filtered), nil
	case "json":
		return s.exportJSON(conv, filtered)
	case "txt", "plain", "plaintext":
		return s.exportPlainText(conv, filtered), nil
	case "csv":
		return s.exportCSV(conv, filtered)
	default:
		return "", apperror.Newf(apperror.KindInvalidInput, "unsupported export format: %s", format)
	}
}

// exportMarkdown exports as Markdown.
func (s *Service) exportMarkdown(conv *store.Conversation, msgs []*store.Message) string {
	var sb strings.Builder
	sb.WriteString("# ")
	sb.WriteString(conv.Title)
	sb.WriteString("\n\n")
	for _, m := range msgs {
		role := "用户"
		if m.Role == "assistant" {
			role = "助手"
		}
		sb.WriteString("## ")
		sb.WriteString(role)
		sb.WriteString("\n\n")
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

// exportJSON exports as JSON (array of messages).
func (s *Service) exportJSON(_ *store.Conversation, msgs []*store.Message) (string, error) {
	type jsonMsg struct {
		Role            string `json:"role"`
		Content         string `json:"content"`
		ThinkingContent string `json:"thinking_content,omitempty"`
		SearchResults   string `json:"search_results,omitempty"`
		CreatedAt       string `json:"created_at"`
	}
	export := make([]jsonMsg, len(msgs))
	for i, m := range msgs {
		export[i] = jsonMsg{
			Role:            m.Role,
			Content:         m.Content,
			ThinkingContent: m.ThinkingContent,
			SearchResults:   m.SearchResults,
			CreatedAt:       m.CreatedAt.Format(time.RFC3339),
		}
	}
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Service) exportPlainText(conv *store.Conversation, msgs []*store.Message) string {
	var sb strings.Builder
	sb.WriteString(conv.Title)
	sb.WriteString("\n\n")
	for _, m := range msgs {
		role := "用户"
		if m.Role == "assistant" {
			role = "助手"
		}
		sb.WriteString("[")
		sb.WriteString(role)
		sb.WriteString("]\n\n")
		sb.WriteString(m.Content)
		sb.WriteString("\n\n")
	}
	return sb.String()
}

func (s *Service) exportCSV(_ *store.Conversation, msgs []*store.Message) (string, error) {
	var sb strings.Builder
	sb.WriteString("instruction,input,output\n")
	for i := range msgs {
		if msgs[i].Role != "user" {
			continue
		}
		instruction := ""
		input := csvEscape(msgs[i].Content)
		output := ""
		if i+1 < len(msgs) && msgs[i+1].Role == "assistant" {
			output = csvEscape(msgs[i+1].Content)
		}
		sb.WriteString("\"")
		sb.WriteString(instruction)
		sb.WriteString("\",\"")
		sb.WriteString(input)
		sb.WriteString("\",\"")
		sb.WriteString(output)
		sb.WriteString("\"\n")
	}
	return sb.String(), nil
}

func csvEscape(s string) string {
	s = strings.ReplaceAll(s, "\"", "\"\"")
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return s
}

// CleanupAbnormalConversations removes abnormal conversations.
func (s *Service) CleanupAbnormalConversations() []*AbnormalConversation {
	removed, err := store.CleanupAbnormalConversations(s.db, secrets.CipherKey(s.cipher))
	if err != nil {
		log.Error().Err(err).Msg("[chat] CleanupAbnormalConversations failed")
		return nil
	}
	if len(removed) == 0 {
		return nil
	}
	result := make([]*AbnormalConversation, len(removed))
	for i, ac := range removed {
		result[i] = &AbnormalConversation{
			ID:     ac.ID,
			Title:  ac.Title,
			Reason: ac.Reason,
		}
		s.emitForConv(ac.ID, "conversation_deleted", ac.ID)
	}
	log.Info().Int("count", len(result)).Msg("[chat] cleaned up abnormal conversations")
	return result
}

// GetConfig returns the current service configuration.
func (s *Service) GetConfig() *config.Config {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.config
}

// UpdateConfig updates the service configuration.
func (s *Service) UpdateConfig(cfg *config.Config) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.config = cfg
}
