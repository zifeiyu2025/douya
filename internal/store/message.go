// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID               string    `json:"id"`
	ConversationID   string    `json:"conversation_id"`
	Role             string    `json:"role"`
	Content          string    `json:"content"`
	ThinkingContent  string    `json:"thinking_content"`
	ThinkingDuration float64   `json:"thinking_duration"`
	SearchResults    string    `json:"search_results"`
	Images           string    `json:"images"`
	Attachments      string    `json:"attachments"`
	ToolCalls        string    `json:"tool_calls"`
	ToolCallID       string    `json:"tool_call_id"`
	CreatedAt        time.Time `json:"created_at"`
}

func CreateMessage(db *sql.DB, msg *Message) error {
	if msg.ID == "" {
		msg.ID = uuid.New().String()
	}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx,
		"INSERT INTO messages (id, conversation_id, role, content, thinking_content, thinking_duration, search_results, images, attachments, tool_calls, tool_call_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		msg.ID, msg.ConversationID, msg.Role, msg.Content, msg.ThinkingContent, msg.ThinkingDuration, msg.SearchResults, msg.Images, msg.Attachments, msg.ToolCalls, msg.ToolCallID, msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	return nil
}

func GetMessagesByConversation(db *sql.DB, convID string) ([]*Message, error) {
	rows, err := db.Query(
		"SELECT id, conversation_id, role, content, thinking_content, thinking_duration, search_results, images, attachments, tool_calls, tool_call_id, created_at FROM messages WHERE conversation_id = ? ORDER BY created_at ASC",
		convID,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages by conversation: %w", err)
	}
	defer rows.Close()
	var msgs []*Message
	for rows.Next() {
		msg := &Message{}
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ThinkingContent, &msg.ThinkingDuration, &msg.SearchResults, &msg.Images, &msg.Attachments, &msg.ToolCalls, &msg.ToolCallID, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return msgs, nil
}

func escapeLikeWildcards(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, "%", "\\%")
	s = strings.ReplaceAll(s, "_", "\\_")
	return s
}

func escapeFTS5Query(query string) string {
	return `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
}

func SearchMessages(db *sql.DB, query string) ([]*Message, error) {
	var rows *sql.Rows
	var err error

	if FTS5Available() {
		rows, err = db.Query(
			`SELECT m.id, m.conversation_id, m.role, m.content, m.thinking_content, m.thinking_duration, m.search_results, m.images, m.attachments, m.tool_calls, m.tool_call_id, m.created_at
			 FROM messages_fts f
			 JOIN messages m ON m.rowid = f.rowid
			 WHERE messages_fts MATCH ?
			 ORDER BY rank`,
			escapeFTS5Query(query),
		)
	} else {
		rows, err = db.Query(
			`SELECT id, conversation_id, role, content, thinking_content, thinking_duration, search_results, images, attachments, tool_calls, tool_call_id, created_at
			 FROM messages
			 WHERE content LIKE ?
			 ORDER BY created_at DESC`,
			"%"+escapeLikeWildcards(query)+"%",
		)
	}
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()
	var msgs []*Message
	for rows.Next() {
		msg := &Message{}
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ThinkingContent, &msg.ThinkingDuration, &msg.SearchResults, &msg.Images, &msg.Attachments, &msg.ToolCalls, &msg.ToolCallID, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return msgs, nil
}

func GetMessage(db *sql.DB, id string) (*Message, error) {
	var msg Message
	err := db.QueryRow(
		"SELECT id, conversation_id, role, content, thinking_content, thinking_duration, search_results, images, attachments, tool_calls, tool_call_id, created_at FROM messages WHERE id = ?",
		id,
	).Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ThinkingContent, &msg.ThinkingDuration, &msg.SearchResults, &msg.Images, &msg.Attachments, &msg.ToolCalls, &msg.ToolCallID, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	return &msg, nil
}

func DeleteMessage(db *sql.DB, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx, "DELETE FROM messages WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}
