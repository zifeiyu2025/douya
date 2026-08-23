// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
)

// searchMaxScanRows 限制 SearchMessages 最多扫描的消息数量，避免全表扫描导致性能问题。
// 由于消息内容是加密的，无法在 SQL 层面做 LIKE 匹配，只能加载后解密再匹配，
// 因此该限制是在解密前截断，可能漏掉较旧的匹配结果，但对于搜索场景，最近的记录通常足够。
const searchMaxScanRows = 500

// dbOpTimeout 是 store 包所有 DB 操作的默认超时时间。
// 提取为常量便于统一调整，避免魔法数字散落各处。
const dbOpTimeout = 10 * time.Second

// messageColumns 是 messages 表的列名列表，保持 INSERT/SELECT 语句一致。
// 生活类比：像表格的表头清单，确保每次填写和读取都按同一顺序，不会错位。
const messageColumns = "id, conversation_id, role, content, thinking_content, thinking_duration, search_results, images, attachments, tool_calls, tool_call_id, created_at"

// scanMessage 将 rows 当前行扫描到 msg 结构体，统一 12 个字段的 Scan 逻辑。
// 生活类比：像快递扫码枪，按固定顺序逐一扫描包裹的 12 个标签贴到对应字段上。
func scanMessage(rows *sql.Rows, msg *Message) error {
	return rows.Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ThinkingContent, &msg.ThinkingDuration, &msg.SearchResults, &msg.Images, &msg.Attachments, &msg.ToolCalls, &msg.ToolCallID, &msg.CreatedAt)
}

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
// 安全实践（基于 B-1.13/B-1.14）：已统一到 crypto.go 的 encryptWithPrefix，此处仅保留薄包装层
// 保留原函数名是为了不破坏 message.go 内部调用的可读性
func encryptField(plaintext string, encKey []byte) (string, error) {
	return encryptWithPrefix(plaintext, encKey)
}

// decryptField 解密 "enc:" 前缀的密文，兼容旧版明文数据
// 安全实践（基于 B-1.13/B-1.14）：已统一到 crypto.go 的 decryptWithPrefix，此处仅保留薄包装层
func decryptField(ciphertext string, encKey []byte) (string, error) {
	return decryptWithPrefix(ciphertext, encKey)
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
// 任何一个字段解密失败都会立即返回 error，避免部分解密导致的数据不一致
// 调用方应将解密失败的消息视为不可读，向上层报告错误而非展示半解密内容
func decryptMessage(msg *Message, encKey []byte) error {
	var err error
	if msg.Content, err = decryptField(msg.Content, encKey); err != nil {
		return apperror.Wrap(apperror.KindInternal, "decrypt content", err)
	}
	if msg.ThinkingContent, err = decryptField(msg.ThinkingContent, encKey); err != nil {
		return apperror.Wrap(apperror.KindInternal, "decrypt thinking_content", err)
	}
	if msg.SearchResults, err = decryptField(msg.SearchResults, encKey); err != nil {
		return apperror.Wrap(apperror.KindInternal, "decrypt search_results", err)
	}
	if msg.Images, err = decryptField(msg.Images, encKey); err != nil {
		return apperror.Wrap(apperror.KindInternal, "decrypt images", err)
	}
	if msg.Attachments, err = decryptField(msg.Attachments, encKey); err != nil {
		return apperror.Wrap(apperror.KindInternal, "decrypt attachments", err)
	}
	if msg.ToolCalls, err = decryptField(msg.ToolCalls, encKey); err != nil {
		return apperror.Wrap(apperror.KindInternal, "decrypt tool_calls", err)
	}
	return nil
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
		return apperror.Wrap(apperror.KindInternal, "encrypt message", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	if err := insertMessage(ctx, db, &saved); err != nil {
		return err
	}
	return nil
}

// CreateMessagesTx 在单个数据库事务内批量创建消息。
// 原子性：一组消息（例如 tool_call 的 assistant+tool 成对消息）要么全部落库，
// 要么全部回滚，避免失败时留下仅有一半的孤儿数据。
// CreatedAt/ID 为空的字段会在写入时自动生成并回写到原 slice。
//
// 生活类比：同一次调度的多条报表必须一起归档，绝不会出现"只有客户回单、
// 没有送达记录"的对不上账的情况。
func CreateMessagesTx(db *sql.DB, msgs []*Message, encKey []byte) error {
	// 空列表直接返回，避免启动无意义的空事务
	if len(msgs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "begin messages transaction", err)
	}

	for _, msg := range msgs {
		saved := *msg
		if saved.ID == "" {
			saved.ID = uuid.New().String()
			msg.ID = saved.ID
		}
		if saved.CreatedAt.IsZero() {
			saved.CreatedAt = time.Now()
			msg.CreatedAt = saved.CreatedAt
		}
		if err := encryptMessage(&saved, encKey); err != nil {
			_ = tx.Rollback()
			return apperror.Wrap(apperror.KindInternal, "encrypt message", err)
		}
		if err := insertMessage(ctx, tx, &saved); err != nil {
			_ = tx.Rollback()
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return apperror.Wrap(apperror.KindInternal, "commit messages transaction", err)
	}
	return nil
}

// insertMessage 执行单条消息 INSERT，execer 可为 *sql.DB 或 *sql.Tx。
// 由 CreateMessage / CreateMessagesTx 复用，避免 SQL 语句重复。
func insertMessage(ctx context.Context, execer execer, saved *Message) error {
	if _, err := execer.ExecContext(ctx,
		"INSERT INTO messages ("+messageColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		saved.ID, saved.ConversationID, saved.Role, saved.Content, saved.ThinkingContent, saved.ThinkingDuration, saved.SearchResults, saved.Images, saved.Attachments, saved.ToolCalls, saved.ToolCallID, saved.CreatedAt,
	); err != nil {
		return apperror.Wrap(apperror.KindInternal, "create message", err)
	}
	return nil
}

// execer 抽象 *sql.DB 和 *sql.Tx 共有的执行接口，供 insertMessage 复用。
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func GetMessagesByConversation(db *sql.DB, convID string, encKey []byte) ([]*Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx,
		"SELECT "+messageColumns+" FROM messages WHERE conversation_id = ? ORDER BY created_at ASC",
		convID,
	)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "get messages by conversation", err)
	}
	defer rows.Close()
	var msgs []*Message
	for rows.Next() {
		msg := &Message{}
		if err := scanMessage(rows, msg); err != nil {
			return nil, apperror.Wrap(apperror.KindInternal, "scan message", err)
		}
		// 解密敏感字段
		if err := decryptMessage(msg, encKey); err != nil {
			return nil, apperror.Wrapf(apperror.KindInternal, "decrypt message %s", err, msg.ID)
		}
		msgs = append(msgs, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "iterate messages", err)
	}
	return msgs, nil
}

// SearchMessages 在内存中搜索消息（支持加密内容）
// 加密后 FTS5 无法使用，改为加载最近 searchMaxScanRows 条消息解密后在内存中匹配。
// 由于消息内容是加密的，无法在 SQL 层面做 LIKE 匹配，只能加载后解密再匹配，
// 因此通过 LIMIT 限制扫描数量，避免对消息量大的应用造成性能瓶颈。
func SearchMessages(db *sql.DB, query string, encKey []byte) ([]*Message, error) {
	// 加载最近的消息（限制扫描数量，避免全表扫描）
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx,
		`SELECT `+messageColumns+` FROM messages ORDER BY created_at DESC LIMIT ?`,
		searchMaxScanRows,
	)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "search messages", err)
	}
	defer rows.Close()

	lowerQuery := strings.ToLower(query)
	var msgs []*Message
	scanned := 0
	skipCount := 0
	for rows.Next() {
		scanned++
		msg := &Message{}
		if err := scanMessage(rows, msg); err != nil {
			return nil, apperror.Wrap(apperror.KindInternal, "scan message", err)
		}
		// 解密敏感字段
		if err := decryptMessage(msg, encKey); err != nil {
			// 解密失败（密钥不匹配或密文损坏）：跳过该消息而非中断整个搜索
			// 这样即使个别消息损坏，用户仍能搜索到其他正常消息
			skipCount++
			continue
		}
		// 在内存中匹配：扩展匹配字段到 content / thinking_content / search_results / tool_calls，
		// 让搜索能命中思考过程、RAG 检索结果、工具调用等扩展内容，而非仅限正文。
		// 生活类比：以前只在"正文"里找关键词，现在也会翻"草稿纸（思考）"、"参考资料（RAG）"、"工具记录"一起找。
		//
		// 性能优化（PERF-1）：原实现先把 4 个字段拼接成一个大字符串再做 ToLower，
		// 每条消息产生 1 次大字符串分配 + 1 次完整小写化拷贝。
		// 现改为逐字段单独 Contains 并短路退出，命中任一字段即停止后续字段检查，
		// 避免大字符串拼接，同时减少不必要的 ToLower 拷贝（空字段直接跳过）。
		// 注意：空 query 时 strings.Contains(_, "") 永远返回 true，保持原有"匹配所有消息"的行为。
		matched := false
		for _, field := range []string{msg.Content, msg.ThinkingContent, msg.SearchResults, msg.ToolCalls} {
			if field == "" {
				continue
			}
			if strings.Contains(strings.ToLower(field), lowerQuery) {
				matched = true
				break
			}
		}
		if matched {
			msgs = append(msgs, msg)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "iterate messages", err)
	}
	// 如果扫描的行数达到上限，说明可能还有更早的匹配结果被截断，提示用户缩小搜索范围
	if scanned >= searchMaxScanRows {
		log.Warn().Int("scanned", scanned).Int("limit", searchMaxScanRows).Str("query", query).Msg("[store] SearchMessages reached scan limit, older matches may be truncated")
	}
	if skipCount > 0 {
		log.Warn().Int("skipped", skipCount).Str("query", query).Msg("[store] SearchMessages skipped messages with decryption errors")
	}
	return msgs, nil
}

func GetMessage(db *sql.DB, id string, encKey []byte) (*Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	var msg Message
	err := db.QueryRowContext(ctx,
		"SELECT "+messageColumns+" FROM messages WHERE id = ?",
		id,
	).Scan(&msg.ID, &msg.ConversationID, &msg.Role, &msg.Content, &msg.ThinkingContent, &msg.ThinkingDuration, &msg.SearchResults, &msg.Images, &msg.Attachments, &msg.ToolCalls, &msg.ToolCallID, &msg.CreatedAt)
	if err != nil {
		// sql.ErrNoRows 转为统一的 NotFound 错误
		if errors.Is(err, sql.ErrNoRows) {
			return nil, apperror.Wrap(apperror.KindNotFound, "消息不存在: "+id, err)
		}
		return nil, apperror.Wrap(apperror.KindInternal, "get message", err)
	}
	// 解密敏感字段
	if err := decryptMessage(&msg, encKey); err != nil {
		return nil, apperror.Wrapf(apperror.KindInternal, "decrypt message %s", err, msg.ID)
	}
	return &msg, nil
}

// DeleteMessagesBatch 批量删除消息，修复 N+1 问题。
// 单事务内执行 DELETE WHERE id IN (...)，比逐条删除性能提升 N 倍。
// 部分ID不存在时不影响其他ID的删除（SQL DELETE 对不存在的行静默忽略）。
// 安全限制：单批最多 500 个 ID，防止 SQL 占位符过多。
func DeleteMessagesBatch(db *sql.DB, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	// 限制单批大小，防止 SQL 占位符过多
	const maxBatchSize = 500
	if len(ids) > maxBatchSize {
		// 分批处理
		for i := 0; i < len(ids); i += maxBatchSize {
			end := i + maxBatchSize
			if end > len(ids) {
				end = len(ids)
			}
			if err := deleteMessagesBatchInternal(db, ids[i:end]); err != nil {
				return err
			}
		}
		return nil
	}
	return deleteMessagesBatchInternal(db, ids)
}

func deleteMessagesBatchInternal(db *sql.DB, ids []string) error {
	// 构造占位符：?,?,?,?
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	query := "DELETE FROM messages WHERE id IN (" + strings.Join(placeholders, ",") + ")"

	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	_, err := db.ExecContext(ctx, query, args...)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "delete messages batch", err)
	}
	return nil
}

// UpdateMessageContent 更新指定消息的正文内容（消息编辑功能的存储层入口）。
// 新内容经既有加密管线（encryptField → "enc:" 前缀密文）落库，与 CreateMessage 的加密策略保持一致。
// 通过 RowsAffected 判断目标消息是否存在：不存在时返回 KindNotFound，便于上层给出准确提示。
// 注意：仅更新 content 字段，其余字段（思考内容/附件/工具调用等）不受影响。
func UpdateMessageContent(db *sql.DB, id string, newContent string, encKey []byte) error {
	encrypted, err := encryptField(newContent, encKey)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "encrypt message content", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	res, err := db.ExecContext(ctx, "UPDATE messages SET content = ? WHERE id = ?", encrypted, id)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "update message content", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		return apperror.New(apperror.KindNotFound, "消息不存在: "+id)
	}
	return nil
}
