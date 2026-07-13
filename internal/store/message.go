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
	"github.com/rs/zerolog/log"
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
		return fmt.Errorf("decrypt content: %w", err)
	}
	if msg.ThinkingContent, err = decryptField(msg.ThinkingContent, encKey); err != nil {
		return fmt.Errorf("decrypt thinking_content: %w", err)
	}
	if msg.SearchResults, err = decryptField(msg.SearchResults, encKey); err != nil {
		return fmt.Errorf("decrypt search_results: %w", err)
	}
	if msg.Images, err = decryptField(msg.Images, encKey); err != nil {
		return fmt.Errorf("decrypt images: %w", err)
	}
	if msg.Attachments, err = decryptField(msg.Attachments, encKey); err != nil {
		return fmt.Errorf("decrypt attachments: %w", err)
	}
	if msg.ToolCalls, err = decryptField(msg.ToolCalls, encKey); err != nil {
		return fmt.Errorf("decrypt tool_calls: %w", err)
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
		return fmt.Errorf("encrypt message: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	_, err := db.ExecContext(ctx,
		"INSERT INTO messages ("+messageColumns+") VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		saved.ID, saved.ConversationID, saved.Role, saved.Content, saved.ThinkingContent, saved.ThinkingDuration, saved.SearchResults, saved.Images, saved.Attachments, saved.ToolCalls, saved.ToolCallID, saved.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create message: %w", err)
	}
	return nil
}

func GetMessagesByConversation(db *sql.DB, convID string, encKey []byte) ([]*Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx,
		"SELECT "+messageColumns+" FROM messages WHERE conversation_id = ? ORDER BY created_at ASC",
		convID,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages by conversation: %w", err)
	}
	defer rows.Close()
	var msgs []*Message
	for rows.Next() {
		msg := &Message{}
		if err := scanMessage(rows, msg); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		// 解密敏感字段
		if err := decryptMessage(msg, encKey); err != nil {
			return nil, fmt.Errorf("decrypt message %s: %w", msg.ID, err)
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	rows, err := db.QueryContext(ctx,
		`SELECT `+messageColumns+` FROM messages ORDER BY created_at DESC LIMIT ?`,
		searchMaxScanRows,
	)
	if err != nil {
		return nil, fmt.Errorf("search messages: %w", err)
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
			return nil, fmt.Errorf("scan message: %w", err)
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
		haystack := strings.ToLower(msg.Content + " " + msg.ThinkingContent + " " + msg.SearchResults + " " + msg.ToolCalls)
		if strings.Contains(haystack, lowerQuery) {
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
		return nil, fmt.Errorf("get message: %w", err)
	}
	// 解密敏感字段
	if err := decryptMessage(&msg, encKey); err != nil {
		return nil, fmt.Errorf("decrypt message %s: %w", msg.ID, err)
	}
	return &msg, nil
}

func DeleteMessage(db *sql.DB, id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), dbOpTimeout)
	defer cancel()
	_, err := db.ExecContext(ctx, "DELETE FROM messages WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete message: %w", err)
	}
	return nil
}

// DeleteMessagesBatch 批量删除消息，修复 N+1 问题。
// 单事务内执行 DELETE WHERE id IN (...)，比循环调用 DeleteMessage 性能提升 N 倍。
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
		return fmt.Errorf("delete messages batch: %w", err)
	}
	return nil
}
