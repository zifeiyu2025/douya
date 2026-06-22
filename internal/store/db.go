// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"database/sql"
	"fmt"
	"os"

	"github.com/rs/zerolog/log"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

func Init(dbPath string, encKey []byte) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	if err := Migrate(db, encKey); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(db *sql.DB, encKey []byte) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS conversations (
			id TEXT PRIMARY KEY,
			title TEXT,
			created_at DATETIME,
			updated_at DATETIME
		);
		CREATE TABLE IF NOT EXISTS messages (
			id TEXT PRIMARY KEY,
			conversation_id TEXT,
			role TEXT,
			content TEXT,
			thinking_content TEXT,
			search_results TEXT,
			tool_calls TEXT,
			tool_call_id TEXT,
			created_at DATETIME,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id);
		CREATE INDEX IF NOT EXISTS idx_messages_conversation_created ON messages(conversation_id, created_at);
		CREATE INDEX IF NOT EXISTS idx_conversations_updated_at ON conversations(updated_at);
		CREATE TABLE IF NOT EXISTS settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	if err != nil {
		return err
	}

	if err := migrateAddColumns(db); err != nil {
		return err
	}

	// 移除 FTS5（内容加密后 FTS5 无法使用）
	dropFTS5Artifacts(db)

	logStartupSchema(db)

	// 迁移旧版明文数据为加密数据
	if encKey != nil {
		if err := migrateEncryptExistingData(db, encKey); err != nil {
			log.Error().Err(err).Msg("[db] failed to migrate encrypt existing data")
		}
	}

	return nil
}

func migrateAddColumns(db *sql.DB) error {
	// messages 表列迁移
	existingColumns, err := GetTableColumns(db, "messages")
	if err != nil {
		return err
	}

	addCols := []struct {
		name string
		typ  string
	}{
		{"tool_calls", "TEXT"},
		{"tool_call_id", "TEXT"},
		{"thinking_duration", "REAL DEFAULT 0"},
		{"images", "TEXT"},
		{"attachments", "TEXT"},
	}

	for _, col := range addCols {
		if !existingColumns[col.name] {
			_, err := db.Exec("ALTER TABLE messages ADD COLUMN " + col.name + " " + col.typ)
			if err != nil {
				return err
			}
		}
	}

	// conversations 表列迁移
	convColumns, err := GetTableColumns(db, "conversations")
	if err != nil {
		return err
	}

	convAddCols := []struct {
		name string
		typ  string
	}{
		{"summary", "TEXT"},
	}

	for _, col := range convAddCols {
		if !convColumns[col.name] {
			_, err := db.Exec("ALTER TABLE conversations ADD COLUMN " + col.name + " " + col.typ)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

var allowedTables = map[string]bool{
	"conversations": true,
	"messages":      true,
}

func GetTableColumns(db *sql.DB, tableName string) (map[string]bool, error) {
	if !allowedTables[tableName] {
		return nil, fmt.Errorf("table %q is not in allowed list", tableName)
	}
	rows, err := db.Query("PRAGMA table_info(" + tableName + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull int
		var dfltValue interface{}
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dfltValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func logStartupSchema(db *sql.DB) {
	cols, err := GetTableColumns(db, "messages")
	if err != nil {
		log.Error().Err(err).Msg("[db] failed to get messages schema")
		return
	}
	var names []string
	for k := range cols {
		names = append(names, k)
	}
	log.Info().Str("columns", strings.Join(names, ", ")).Msg("[db] messages table columns")
}

func dropFTS5Artifacts(db *sql.DB) {
	_, _ = db.Exec("DROP TRIGGER IF EXISTS messages_ai")
	_, _ = db.Exec("DROP TRIGGER IF EXISTS messages_ad")
	_, _ = db.Exec("DROP TRIGGER IF EXISTS messages_au")
	_, err := db.Exec("DROP TABLE IF EXISTS messages_fts")
	if err != nil {
		log.Error().Err(err).Msg("[db] could not drop messages_fts table (FTS5 module unavailable)")
	}
	log.Info().Msg("[db] cleaned up FTS5 triggers and virtual table")
}

// migrateEncryptExistingData 将旧版明文数据加密
// 检查是否已有加密数据标记，如果没有则批量加密
func migrateEncryptExistingData(db *sql.DB, encKey []byte) error {
	// 检查是否已完成迁移
	var migrationDone string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'encryption_migration_done'").Scan(&migrationDone)
	if err == nil && migrationDone == "yes" {
		return nil
	}

	log.Info().Msg("[db] starting encryption migration for existing plaintext data")

	// 加密 conversations.title
	rows, err := db.Query("SELECT id, title FROM conversations")
	if err != nil {
		return fmt.Errorf("migrate conversations: %w", err)
	}
	var convUpdates []struct {
		id    string
		title string
	}
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			rows.Close()
			return fmt.Errorf("scan conversation: %w", err)
		}
		// 只加密非空且未加密的标题
		if title != "" && (len(title) < 4 || title[:4] != "enc:") {
			encTitle, err := encryptField(title, encKey)
			if err != nil {
				log.Error().Err(err).Str("id", id).Msg("[db] failed to encrypt conversation title during migration")
				continue
			}
			convUpdates = append(convUpdates, struct {
				id    string
				title string
			}{id, encTitle})
		}
	}
	rows.Close()

	for _, u := range convUpdates {
		_, err := db.Exec("UPDATE conversations SET title = ? WHERE id = ?", u.title, u.id)
		if err != nil {
			log.Error().Err(err).Str("id", u.id).Msg("[db] failed to encrypt conversation title")
		}
	}

	// 加密 messages 的敏感字段
	msgRows, err := db.Query("SELECT id, content, thinking_content, search_results, images, attachments, tool_calls FROM messages")
	if err != nil {
		return fmt.Errorf("migrate messages: %w", err)
	}
	type msgUpdate struct {
		id               string
		content          string
		thinkingContent  string
		searchResults    string
		images           string
		attachments      string
		toolCalls        string
	}
	var msgUpdates []msgUpdate
	for msgRows.Next() {
		var id string
		var content, thinkingContent, searchResults, images, attachments, toolCalls sql.NullString
		if err := msgRows.Scan(&id, &content, &thinkingContent, &searchResults, &images, &attachments, &toolCalls); err != nil {
			msgRows.Close()
			return fmt.Errorf("scan message: %w", err)
		}
		// 检查 content 是否需要加密（只加密未加密的数据）
		needsEncrypt := false
		if content.Valid && content.String != "" && (len(content.String) < 4 || content.String[:4] != "enc:") {
			needsEncrypt = true
		}
		if !needsEncrypt {
			continue
		}
		encContent, err := encryptField(content.String, encKey)
		if err != nil {
			log.Error().Err(err).Str("id", id).Msg("[db] failed to encrypt message content during migration")
			continue
		}
		encThinking, _ := encryptField(thinkingContent.String, encKey)
		encSearch, _ := encryptField(searchResults.String, encKey)
		encImages, _ := encryptField(images.String, encKey)
		encAttachments, _ := encryptField(attachments.String, encKey)
		encToolCalls, _ := encryptField(toolCalls.String, encKey)
		msgUpdates = append(msgUpdates, msgUpdate{
			id:              id,
			content:         encContent,
			thinkingContent: encThinking,
			searchResults:   encSearch,
			images:          encImages,
			attachments:     encAttachments,
			toolCalls:       encToolCalls,
		})
	}
	msgRows.Close()

	for _, u := range msgUpdates {
		_, err := db.Exec(
			"UPDATE messages SET content = ?, thinking_content = ?, search_results = ?, images = ?, attachments = ?, tool_calls = ? WHERE id = ?",
			u.content, u.thinkingContent, u.searchResults, u.images, u.attachments, u.toolCalls, u.id,
		)
		if err != nil {
			log.Error().Err(err).Str("id", u.id).Msg("[db] failed to encrypt message")
		}
	}

	log.Info().Int("conversations", len(convUpdates)).Int("messages", len(msgUpdates)).Msg("[db] encryption migration completed")

	// 标记迁移完成
	_, err = db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('encryption_migration_done', 'yes')")
	if err != nil {
		return fmt.Errorf("mark migration done: %w", err)
	}

	return nil
}
