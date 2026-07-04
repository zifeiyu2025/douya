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
	ID      string `json:"id"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"` // 短期摘要（每次压缩都更新）
	// P1-C1: 摘要分层管理
	// LongSummary 长期摘要：每 N 次压缩合并一次（N=5），保留跨多次压缩的关键事实/决策/实体
	// CompressCount 压缩次数计数：用于触发长期摘要合并和重置（C2）
	LongSummary   string    `json:"long_summary,omitempty"`
	CompressCount int       `json:"compress_count,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
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
	var summary, longSummary sql.NullString
	err := withDBTimeout(func(ctx context.Context) error {
		return db.QueryRowContext(ctx,
			"SELECT id, title, summary, long_summary, compress_count, created_at, updated_at FROM conversations WHERE id = ?",
			id,
		).Scan(&conv.ID, &conv.Title, &summary, &longSummary, &conv.CompressCount, &conv.CreatedAt, &conv.UpdatedAt)
	})
	if err != nil {
		return nil, fmt.Errorf("get conversation: %w", err)
	}
	// 解密标题：失败时降级为占位符，保证会话仍可列出
	decryptedTitle, decErr := decryptField(conv.Title, encKey)
	if decErr != nil {
		conv.Title = "[解密失败]"
	} else {
		conv.Title = decryptedTitle
	}
	if summary.Valid {
		conv.Summary = summary.String
	}
	if longSummary.Valid {
		conv.LongSummary = longSummary.String
	}
	return conv, nil
}

func ListConversations(db *sql.DB, encKey []byte) ([]*Conversation, error) {
	var convs []*Conversation
	err := withDBTimeout(func(ctx context.Context) error {
		// 整个查询+遍历必须放在同一个 context 内
		// 原因：withDBTimeout 的 defer cancel() 会在 fn 返回后取消 context，
		// 若 rows.Next() 在 context 取消后才执行，会返回 "context canceled" 错误
		rows, err := db.QueryContext(ctx,
			"SELECT id, title, created_at, updated_at FROM conversations ORDER BY updated_at DESC",
		)
		if err != nil {
			return fmt.Errorf("list conversations: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			conv := &Conversation{}
			if err := rows.Scan(&conv.ID, &conv.Title, &conv.CreatedAt, &conv.UpdatedAt); err != nil {
				return fmt.Errorf("scan conversation: %w", err)
			}
			// 解密标题：失败时降级为占位符
			decryptedTitle, decErr := decryptField(conv.Title, encKey)
			if decErr != nil {
				conv.Title = "[解密失败]"
			} else {
				conv.Title = decryptedTitle
			}
			convs = append(convs, conv)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate conversations: %w", err)
		}
		return nil
	})
	return convs, err
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

// DeleteConversationsBatch 在单事务内批量删除多个对话及其消息。
// 相比循环调用 DeleteConversation，避免了 N 次独立事务的开启/提交开销。
// 生活类比：像去快递站一次取走所有包裹，而不是每个包裹跑一趟。
//
// 注意：单事务内删除过多记录可能撑大 SQLite WAL 文件，调用方应控制批量大小。
func DeleteConversationsBatch(db *sql.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	err := withDBTimeout(func(ctx context.Context) error {
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin transaction: %w", err)
		}
		// 失败时统一回滚，保证批量删除的原子性
		success := false
		defer func() {
			if !success {
				tx.Rollback()
			}
		}()
		for _, id := range ids {
			if _, err := tx.ExecContext(ctx, "DELETE FROM messages WHERE conversation_id = ?", id); err != nil {
				return fmt.Errorf("delete messages for %s: %w", id, err)
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM conversations WHERE id = ?", id); err != nil {
				return fmt.Errorf("delete conversation %s: %w", id, err)
			}
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit transaction: %w", err)
		}
		success = true
		return nil
	})
	return err
}

type AbnormalConversation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}

func FindAbnormalConversations(db *sql.DB, encKey []byte) ([]*AbnormalConversation, error) {
	// L-12：改用 withDBTimeout 包装，与包内其他查询保持一致（10s 超时保护）
	var abnormal []*AbnormalConversation
	err := withDBTimeout(func(ctx context.Context) error {
		rows, err := db.QueryContext(ctx, `
			SELECT c.id, c.title
			FROM conversations c
			LEFT JOIN messages m ON m.conversation_id = c.id AND m.role = 'user'
			WHERE m.id IS NULL AND c.created_at < datetime('now', '-5 minutes')
			ORDER BY c.updated_at DESC
		`)
		if err != nil {
			return fmt.Errorf("find abnormal conversations: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id, title string
			if err := rows.Scan(&id, &title); err != nil {
				return fmt.Errorf("scan abnormal conversation: %w", err)
			}
			// 解密标题：失败时降级为占位符
			decryptedTitle, decErr := decryptField(title, encKey)
			displayTitle := decryptedTitle
			if decErr != nil {
				displayTitle = "[解密失败]"
			}
			abnormal = append(abnormal, &AbnormalConversation{
				ID:     id,
				Title:  displayTitle,
				Reason: "no_user_messages",
			})
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate abnormal conversations: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
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

	// 改用批量删除接口：单事务内删除所有异常对话，避免 N 次独立事务开销
	ids := make([]string, 0, len(abnormal))
	for _, ac := range abnormal {
		ids = append(ids, ac.ID)
	}
	if err := DeleteConversationsBatch(db, ids); err != nil {
		// 批量删除失败：记录错误并回退到逐个删除，保留容错能力
		log.Error().Err(err).Msg("[cleanup] batch delete failed, falling back to individual deletes")
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

	// 批量删除成功：记录每个被删除的对话
	for _, ac := range abnormal {
		log.Info().Str("id", ac.ID).Str("title", ac.Title).Str("reason", ac.Reason).Msg("[cleanup] removed abnormal conversation")
	}
	return abnormal, nil
}

// UpdateConversationSummary 只更新对话的短期摘要字段（向后兼容旧调用）
func UpdateConversationSummary(db *sql.DB, id string, summary string) error {
	err := withDBTimeout(func(ctx context.Context) error {
		_, err := db.ExecContext(ctx,
			"UPDATE conversations SET summary = ?, compress_count = compress_count + 1 WHERE id = ?",
			summary, id,
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("update conversation summary: %w", err)
	}
	return nil
}

// UpdateConversationLayeredSummary P1-C1: 同时更新短期摘要、长期摘要和压缩计数。
// 每次压缩都会调用：shortSummary 更新为最新摘要，compress_count + 1。
// longSummary 仅在触发合并时（由调用方判断 compress_count % 5 == 0）传入新值，否则传空串保持不变。
func UpdateConversationLayeredSummary(db *sql.DB, id string, shortSummary, longSummary string) error {
	err := withDBTimeout(func(ctx context.Context) error {
		var query string
		var args []any
		if longSummary != "" {
			// 同时更新短期+长期摘要
			query = "UPDATE conversations SET summary = ?, long_summary = ?, compress_count = compress_count + 1 WHERE id = ?"
			args = []any{shortSummary, longSummary, id}
		} else {
			// 仅更新短期摘要（长期摘要保持不变）
			query = "UPDATE conversations SET summary = ?, compress_count = compress_count + 1 WHERE id = ?"
			args = []any{shortSummary, id}
		}
		_, err := db.ExecContext(ctx, query, args...)
		return err
	})
	if err != nil {
		return fmt.Errorf("update conversation layered summary: %w", err)
	}
	return nil
}

// GetConversationLayeredSummary P1-C1: 获取分层摘要和压缩计数。
// 返回 shortSummary, longSummary, compressCount。
func GetConversationLayeredSummary(db *sql.DB, id string) (shortSummary, longSummary string, compressCount int, err error) {
	var short, long sql.NullString
	err = withDBTimeout(func(ctx context.Context) error {
		return db.QueryRowContext(ctx,
			"SELECT summary, long_summary, compress_count FROM conversations WHERE id = ?",
			id,
		).Scan(&short, &long, &compressCount)
	})
	if err != nil {
		return "", "", 0, fmt.Errorf("get conversation layered summary: %w", err)
	}
	if short.Valid {
		shortSummary = short.String
	}
	if long.Valid {
		longSummary = long.String
	}
	return shortSummary, longSummary, compressCount, nil
}

// ResetConversationSummary P2-C4: 重置会话摘要（清空短期+长期+计数归零）。
// 用户在摘要面板点击"重置"按钮时调用。
// 与 UpdateConversationLayeredSummary 的区别：
//   - Update 会 compress_count+1（增量）
//   - Reset 把 compress_count 归零（完全清除）
func ResetConversationSummary(db *sql.DB, id string) error {
	err := withDBTimeout(func(ctx context.Context) error {
		_, err := db.ExecContext(ctx,
			"UPDATE conversations SET summary = '', long_summary = '', compress_count = 0 WHERE id = ?",
			id,
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("reset conversation summary: %w", err)
	}
	return nil
}

// SetConversationSummaryManual P2-C4: 手动设置会话摘要（用户编辑后保存）。
// 与 UpdateConversationLayeredSummary 的区别：
//   - Update 会 compress_count+1（自动/手动压缩时调用）
//   - Set 不改 compress_count（用户编辑视为修正，不触发压缩计数）
func SetConversationSummaryManual(db *sql.DB, id string, shortSummary, longSummary string) error {
	err := withDBTimeout(func(ctx context.Context) error {
		_, err := db.ExecContext(ctx,
			"UPDATE conversations SET summary = ?, long_summary = ? WHERE id = ?",
			shortSummary, longSummary, id,
		)
		return err
	})
	if err != nil {
		return fmt.Errorf("set conversation summary manual: %w", err)
	}
	return nil
}

// GetConversationSummary 只获取对话的短期摘要字段（向后兼容旧调用）
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
