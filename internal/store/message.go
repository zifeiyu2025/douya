// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"douya/internal/secrets"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// searchMaxScanRows 限制 SearchMessages 最多扫描的消息数量，避免全表扫描导致性能问题。
// 由于消息内容是加密的，无法在 SQL 层面做 LIKE 匹配，只能加载后解密再匹配，
// 因此该限制是在解密前截断，可能漏掉较旧的匹配结果，但对于搜索场景，最近的记录通常足够。
const searchMaxScanRows = 500

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

// encryptField 使用 AES-GCM 加密字段，返回 "enc:" 前缀的密文
// 如果 encKey 为 nil，则跳过加密直接返回明文
// 加密失败时返回错误，调用方应决定如何处理而非静默回退为明文
func encryptField(plaintext string, encKey []byte) (string, error) {
	if encKey == nil || plaintext == "" {
		return plaintext, nil
	}
	encrypted, err := secrets.Encrypt(plaintext, encKey)
	if err != nil {
		return "", fmt.Errorf("encrypt field failed: %w", err)
	}
	return "enc:" + encrypted, nil
}

// decryptField 解密 "enc:" 前缀的密文，兼容旧版明文数据
// 如果 encKey 为 nil，则跳过解密直接返回原值
func decryptField(ciphertext string, encKey []byte) string {
	if encKey == nil || ciphertext == "" {
		return ciphertext
	}
	if len(ciphertext) < 4 || ciphertext[:4] != "enc:" {
		return ciphertext
	}
	plaintext, err := secrets.Decrypt(ciphertext[4:], encKey)
	if err != nil {
		return ciphertext
	}
	return plaintext
}

// encryptMessage 加密消息中的敏感字段
func encryptMessage(msg *Message, encKey []byte) error {
	var err error
	if msg.Content, err = encryptField(msg.Content, encKey); err != nil {
		return err
	}
	if msg.ThinkingContent, err = encryptField(msg.ThinkingContent, encKey); err != nil {
		return err
	}
	if msg.SearchResults, err = encryptField(msg.SearchResults, encKey); err != nil {
		return err
	}
	if msg.Images, err = encryptField(msg.Images, encKey); err != nil {
		return err
	}
	if msg.Attachments, err = encryptField(msg.Attachments, encKey); err != nil {
		return err
	}
	if msg.ToolCalls, err = encryptField(msg.ToolCalls, encKey); err != nil {
		return err
	}
	return nil
}

// decryptMessage 解密消息中的敏感字段
func decryptMessage(msg *Message, encKey []byte) {
	msg.Content = decryptField(msg.Content, encKey)
	msg.ThinkingContent = decryptField(msg.ThinkingContent, encKey)
	msg.SearchResults = decryptField(msg.SearchResults, encKey)
	msg.Images = decryptField(msg.Images, encKey)
	msg.Attachments = decryptField(msg.Attachments, encKey)
	msg.ToolCalls = decryptField(msg.ToolCalls, encKey)
}

func CreateMessage(db *sql.DB, msg *Message, encKey []byte) error {
	// 复制结构体，避免加密修改调用方的原始数据
	saved := *msg

	if saved.ID == "" {
		saved.ID = uuid.New().String()
		// 同步回写 ID，调用方可能依赖自动生成的 ID
		msg.ID = saved.ID
	}
	if saved.CreatedAt.IsZero() {
		saved.CreatedAt = time.Now()
		msg.CreatedAt = saved.CreatedAt
	}
	// 加密复制的结构体，不修改原始 msg
	if err := encryptMessage(&saved, encKey); err != nil {
		return fmt.Errorf("encrypt message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := db.ExecContext(ctx,
		"INSERT INTO messages (id, conversation_id, role, content, thinking_content, thinking_duration, search_results, images, attachments, tool_calls, tool_call_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		saved.ID, saved.ConversationID, saved.Role, saved.Content, saved.ThinkingContent, saved.ThinkingDuration, saved.SearchResults, saved.Images, saved.Attachments, saved.ToolCalls, saved.ToolCallID, saved.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	return nil
}

func GetMessagesByConversation(db *sql.DB, convID string, encKey []byte) ([]*Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx,
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
		// 解密敏感字段
		decryptMessage(msg, encKey)
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return msgs, nil
}

// SearchMessages 在内存中搜索消息（支持加密内容）
// 加密后 FTS5 无法使用，改为加载最近 searchMaxScanRows 条消息解密后在内存中匹配。
// 由于消息内容是加密的，无法在 SQL 层面做 LIKE 匹配，只能加载后解密再匹配，
// 因此通过 LIMIT 限制扫描数量，避免对消息量大的应用造成性能瓶颈。
func SearchMessages(db *sql.DB, query string, encKey []byte) ([]*Message, error) {
	// 加载最近的消息（限制扫描数量，避免全表扫描）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rows, err := db.QueryContext(ctx,
		`SELECT id, conversation_id, role, content, thinking_content, thinking_duration, search_results, images, attachments, tool_calls, tool_call_id, created_at FROM messages ORDER BY created_at DESC LIMIT ?`,
		searchMaxScanRows,
	)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
	}
	defer rows.Close()

	lowerQuery := strings.ToLower(query)
	var msgs []*Message
	scanned := 0
	for rows.Next() {
		scanned++
		msg := &Message{}
		if err := rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ThinkingContent, &msg.ThinkingDuration, &msg.SearchResults, &msg.Images, &msg.Attachments, &msg.ToolCalls, &msg.ToolCallID, &msg.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		// 解密敏感字段
		decryptMessage(msg, encKey)
		// 在内存中匹配
		if strings.Contains(strings.ToLower(msg.Content), lowerQuery) {
			msgs = append(msgs, msg)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	// 如果扫描的行数达到上限，说明可能还有更早的匹配结果被截断，提示用户缩小搜索范围
	if scanned >= searchMaxScanRows {
		log.Warn().Int("scanned", scanned).Int("limit", searchMaxScanRows).Str("query", query).Msg("[store] SearchMessages reached scan limit, older matches may be truncated")
	}
	return msgs, nil
}

func GetMessage(db *sql.DB, id string, encKey []byte) (*Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var msg Message
	err := db.QueryRowContext(ctx,
		"SELECT id, conversation_id, role, content, thinking_content, thinking_duration, search_results, images, attachments, tool_calls, tool_call_id, created_at FROM messages WHERE id = ?",
		id,
	).Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ThinkingContent, &msg.ThinkingDuration, &msg.SearchResults, &msg.Images, &msg.Attachments, &msg.ToolCalls, &msg.ToolCallID, &msg.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("get message: %w", err)
	}
	// 解密敏感字段
	decryptMessage(&msg, encKey)
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
