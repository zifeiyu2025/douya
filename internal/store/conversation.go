// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

type Conversation struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Summary   string    `json:"summary,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// withDBTimeout 在带超时的 context 中执行数据库操作。
// 生活类比：像给每个数据库操作都装上"闹钟"，超时自动响铃，避免操作卡住永远不返回。
func withDBTimeout(fn func(ctx context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	return fn(ctx)
}

func CreateConversation(db *sql.DB, conv *Conversation, encKey []byte) error {
	if conv.ID == "" {
		conv.ID = uuid.New().String()
	}
	now := time.Now()
	if conv.CreatedAt.IsZero() {
		conv.CreatedAt = now
	}
	if conv.UpdatedAt.IsZero() {
		conv.UpdatedAt = now
	}
	// 加密标题
	encryptedTitle, err := encryptField(conv.Title, encKey)
	if err != nil {
		return fmt.Errorf("encrypt conversation title: %w", err)
	}

	err = withDBTimeout(func(ctx context.Context) error {
		_, err := db.ExecContext(ctx,
			"INSERT INTO conversations (id, title, created_at, updated_at) VALUES (?, ?, ?, ?)",
			conv.ID, encryptedTitle, conv.CreatedAt, conv.UpdatedAt,
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("create conversation: %w", err)
	}
	return nil
}

func GetConversation(db *sql.DB, id string, encKey []byte) (*Conversation, error) {
	conv := &Conversation{}
	var summary sql.NullString
	err := withDBTimeout(func(ctx context.Context) error {
		return db.QueryRowContext(ctx,
			"SELECT id, title, summary, created_at, updated_at FROM conversations WHERE id = ?",
			id,
		).Scan(&conv.ID, &conv.Title, &summary, &conv.CreatedAt, &conv.UpdatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	// 解密标题
	conv.Title = decryptField(conv.Title, encKey)
	if summary.Valid {
		conv.Summary = summary.String
	}
	return conv, nil
}

func ListConversations(db *sql.DB, encKey []byte) ([]*Conversation, error) {
	var rows *sql.Rows
	err := withDBTimeout(func(ctx context.Context) error {
		var err error
		rows, err = db.QueryContext(ctx,
			"SELECT id, title, created_at, updated_at FROM conversations ORDER BY updated_at DESC",
		)
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list conversations: %w", err)
	}
	defer rows.Close()
	var convs []*Conversation
	for rows.Next() {
		conv := &Conversation{}
		if err := rows.Scan(&conv.ID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan conversation: %w", err)
		}
		// 解密标题
		conv.Title = decryptField(conv.Title, encKey)
		convs = append(convs, conv)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate conversations: %w", err)
	}
	return convs, nil
}

func UpdateConversation(db *sql.DB, conv *Conversation, encKey []byte) error {
	conv.UpdatedAt = time.Now()
	// 加密标题
	encryptedTitle, err := encryptField(conv.Title, encKey)
	if err != nil {
		return fmt.Errorf("encrypt conversation title: %w", err)
	}

	err = withDBTimeout(func(ctx context.Context) error {
		_, err := db.ExecContext(ctx,
			"UPDATE conversations SET title = ?, updated_at = ? WHERE id = ?",
			encryptedTitle, conv.UpdatedAt, conv.ID,
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("update conversation: %w", err)
	}
	return nil
}

func DeleteConversation(db *sql.DB, id string) error {
	err := withDBTimeout(func(ctx context.Context) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM messages WHERE conversation_id = ?", id)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("delete messages: %w", err)
		}
		_, err = tx.ExecContext(ctx, "DELETE FROM conversations WHERE id = ?", id)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("delete conversation: %w", err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit transaction: %w", err)
		}
		return nil
	})
	return err
}

type AbnormalConversation struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Reason string `json:"reason"`
}

func FindAbnormalConversations(db *sql.DB, encKey []byte) ([]*AbnormalConversation, error) {
	rows, err := db.Query(`
		SELECT c.id, c.title
		FROM conversations c
		LEFT JOIN messages m ON m.conversation_id = c.id AND m.role = 'user'
		WHERE m.id IS NULL AND c.created_at < datetime('now', '-5 minutes')
		ORDER BY c.updated_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("find abnormal conversations: %w", err)
	}
	defer rows.Close()

	var abnormal []*AbnormalConversation
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return nil, fmt.Errorf("scan abnormal conversation: %w", err)
		}
		abnormal = append(abnormal, &AbnormalConversation{
			ID:     id,
			Title:  decryptField(title, encKey),
			Reason: "no_user_messages",
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate abnormal conversations: %w", err)
	}
	return abnormal, nil
}

func CleanupAbnormalConversations(db *sql.DB, encKey []byte) ([]*AbnormalConversation, error) {
	abnormal, err := FindAbnormalConversations(db, encKey)
	if err != nil {
		return nil, err
	}

	if len(abnormal) == 0 {
		return nil, nil
	}

	var removed []*AbnormalConversation
	for _, ac := range abnormal {
		if err := DeleteConversation(db, ac.ID); err != nil {
			log.Error().Err(err).Str("id", ac.ID).Msg("[cleanup] failed to delete abnormal conversation")
			continue
		}
		log.Info().Str("id", ac.ID).Str("title", ac.Title).Str("reason", ac.Reason).Msg("[cleanup] removed abnormal conversation")
		removed = append(removed, ac)
	}

	return removed, nil
}

// UpdateConversationSummary 只更新对话的摘要字段
func UpdateConversationSummary(db *sql.DB, id string, summary string) error {
	err := withDBTimeout(func(ctx context.Context) error {
		_, err := db.ExecContext(ctx,
			"UPDATE conversations SET summary = ? WHERE id = ?",
			summary, id,
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("update conversation summary: %w", err)
	}
	return nil
}

// GetConversationSummary 只获取对话的摘要字段
func GetConversationSummary(db *sql.DB, id string) (string, error) {
	var summary sql.NullString
	err := withDBTimeout(func(ctx context.Context) error {
		return db.QueryRowContext(ctx, "SELECT summary FROM conversations WHERE id = ?", id).Scan(&summary)
	})
	if err != nil {
		return "", fmt.Errorf("get conversation summary: %w", err)
	}
	if summary.Valid {
		return summary.String, nil
	}
	return "", nil
}
