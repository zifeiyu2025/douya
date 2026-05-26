package chat

import (
	"context"
	"douya/internal/config"
	"time"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
	"douya/internal/store"
)

// StopGeneration stops the current generation (if any).
func (s *Service) StopGeneration() {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	if s.currentCancel != nil {
		s.currentCancel()
		s.currentCancel = nil
	}
}

// GetConversations returns all conversations.
func (s *Service) GetConversations() ([]*Conversation, error) {
	convs, err := store.ListConversations(s.db)
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
	err := store.CreateConversation(s.db, conv)
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
		return fmt.Errorf("title cannot be empty")
	}
	conv, err := store.GetConversation(s.db, id)
	if err != nil {
		return err
	}
	conv.Title = title
	return store.UpdateConversation(s.db, conv)
}

// DeleteConversation deletes a conversation and all its messages.
func (s *Service) DeleteConversation(id string) error {
	return store.DeleteConversation(s.db, id)
}

// GetMessages returns all messages for a conversation (excluding tool and intermediate messages).
func (s *Service) GetMessages(conversationID string) ([]*Message, error) {
	msgs, err := store.GetMessagesByConversation(s.db, conversationID)
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
	msg, err := store.GetMessage(s.db, id)
	if err != nil {
		return fmt.Errorf("get message: %w", err)
	}

	convID := msg.ConversationID

	deletedIDs := []string{id}

	if msg.Role == "user" {
		msgs, err := store.GetMessagesByConversation(s.db, convID)
		if err != nil {
			return fmt.Errorf("load conversation messages: %w", err)
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

	for _, delID := range deletedIDs {
		if delErr := store.DeleteMessage(s.db, delID); delErr != nil {
			log.Error().Err(delErr).Str("id", delID).Msg("[chat] delete message failed")
		}
		s.emitForConv(convID, "message_deleted", delID)
	}

	return nil
}

// RegenerateMessage regenerates the last assistant message in a conversation.
func (s *Service) RegenerateMessage(msgID string, searchEnabled bool) error {
	s.mutex.Lock()
	var oldCancel context.CancelFunc
	var oldConvID string
	if s.currentCancel != nil {
		oldCancel = s.currentCancel
		oldConvID = s.currentConvID
	}
	cancelCtx, cancel := context.WithCancel(s.wailsCtx)
	s.currentCancel = cancel
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

	targetMsg, err := store.GetMessage(s.db, msgID)
	if err != nil {
		return fmt.Errorf("message %s not found: %w", msgID, err)
	}

	convID := targetMsg.ConversationID

	msgs, err := store.GetMessagesByConversation(s.db, convID)
	if err != nil {
		return fmt.Errorf("load messages: %w", err)
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
		return fmt.Errorf("message %s not found in conversation", msgID)
	}

	var assistantMsgIDs []string
	for i := targetIdx + 1; i < len(msgs); i++ {
		if msgs[i].Role == "assistant" {
			assistantMsgIDs = append(assistantMsgIDs, msgs[i].ID)
		} else {
			break
		}
	}
	for _, id := range assistantMsgIDs {
		if delErr := store.DeleteMessage(s.db, id); delErr != nil {
			log.Error().Err(delErr).Str("id", id).Msg("delete assistant message for regeneration")
		}
		s.emitForConv(convID, "message_deleted", id)
	}

	s.mutex.Lock()
	s.currentConvID = convID
	s.mutex.Unlock()

	var userContent string
	var userAttachments []Attachment
	if targetMsg.Role == "user" {
		userContent = targetMsg.Content
		if targetMsg.Attachments != "" {
			_ = json.Unmarshal([]byte(targetMsg.Attachments), &userAttachments)
		}
	}

	dbMsgs, err := store.GetMessagesByConversation(s.db, convID)
	if err != nil {
		return fmt.Errorf("reload messages: %w", err)
	}

	llmMessages, err := s.buildLLMMessages(dbMsgs, userContent, userAttachments, searchEnabled)
	if err != nil {
		return err
	}

	return s.streamWithSearch(cancelCtx, convID, llmMessages, searchEnabled, userContent, userContent)
}

// SearchMessages searches messages across all conversations.
func (s *Service) SearchMessages(query string) ([]*Message, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query cannot be empty")
	}
	msgs, err := store.SearchMessages(s.db, query)
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
	conv, err := store.GetConversation(s.db, id)
	if err != nil {
		return "", err
	}
	msgs, err := store.GetMessagesByConversation(s.db, id)
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
		return "", fmt.Errorf("unsupported export format: %s", format)
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
func (s *Service) exportJSON(conv *store.Conversation, msgs []*store.Message) (string, error) {
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

func (s *Service) exportCSV(conv *store.Conversation, msgs []*store.Message) (string, error) {
	var sb strings.Builder
	sb.WriteString("instruction,input,output\n")
	for i := 0; i < len(msgs); i++ {
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
	removed, err := store.CleanupAbnormalConversations(s.db)
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
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.config
}

// UpdateConfig updates the service configuration.
func (s *Service) UpdateConfig(cfg *config.Config) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.config = cfg
}
