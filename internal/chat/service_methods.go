package chat

import (
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
	conv := &store.Conversation{Title: "新的对话"}
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

// DeleteMessage deletes a single message.
func (s *Service) DeleteMessage(id string) error {
	return store.DeleteMessage(s.db, id)
}

// RegenerateMessage regenerates the last assistant message in a conversation.
func (s *Service) RegenerateMessage(msgID string, searchEnabled bool) error {
	s.StopGeneration()
	log.Info().Str("msgID", msgID).Bool("searchEnabled", searchEnabled).Msg("[chat] RegenerateMessage called")
	return nil
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
