// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/apperror"

	_ "github.com/mattn/go-sqlite3" // 注册 SQLite3 驱动（database/sql 需要）
)

func Init(dbPath string, encKey []byte) (*sql.DB, error) {
	log.Info().Str("path", dbPath).Msg("[store] 初始化数据库")
	dir := filepath.Dir(dbPath)
	// 注：数据库目录不收紧 ACL（icacls），SQLite WAL 模式需要目录写权限创建 -wal/-shm 文件。
	// 数据本身已用 AES-GCM 加密，目录权限收紧收益有限且可能导致 SQLite 功能异常。
	// 见安全审查 #22（已评估，风险可接受）。
	if err := os.MkdirAll(dir, 0o755); err != nil {
		log.Error().Err(err).Str("dir", dir).Msg("[store] 创建数据库目录失败")
		return nil, err
	}
	// PRAGMA 优化说明：
	// - _synchronous=NORMAL：WAL 模式下 NORMAL 足够安全（仅断电时可能丢失最后一条事务），性能提升 2-3 倍
	// - _cache_size=-65536：64MB 页缓存（负值表示 KB 单位），减少磁盘 I/O
	// - _mmap_size=268435456：256MB 内存映射，加速读取
	// - _wal_autocheckpoint=1000：显式控制 WAL checkpoint（默认值，避免 -wal 文件膨胀）
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000&_synchronous=NORMAL&_cache_size=-65536&_wal_autocheckpoint=1000&_mmap_size=268435456")
	if err != nil {
		log.Error().Err(err).Msg("[store] 打开数据库失败")
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)
	// 设置连接最大生命周期，避免长时间运行时连接老化
	db.SetConnMaxLifetime(time.Hour)
	if err := Migrate(db, encKey); err != nil {
		log.Error().Err(err).Msg("[store] 数据库迁移失败")
		db.Close()
		return nil, err
	}
	log.Info().Msg("[store] 数据库初始化完成")
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
		-- M-后2：SearchMessages 按 created_at DESC 排序扫描全表，无此索引时退化为全表扫描+排序。
		-- 加入后 ORDER BY created_at DESC 可直接走索引，避免大消息表的搜索卡顿。
		CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at DESC);
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

	// mmap_size 通过连接字符串设置不生效（go-sqlite3 驱动限制），需通过 PRAGMA 语句执行
	// 256MB 内存映射，加速读取
	if _, err := db.Exec("PRAGMA mmap_size = 268435456"); err != nil {
		log.Warn().Err(err).Msg("[db] failed to set mmap_size")
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
			// 安全说明（基于 GO-INJECT-001 #6）：col.name/col.typ 来自上方硬编码常量数组，不接受外部输入，
			// 故字符串拼接是安全的。SQLite ALTER TABLE 语句不支持 ? 占位符，必须使用字符串拼接。
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
		// P1-C1: 摘要分层管理 - 长期摘要 + 压缩计数
		// summary 字段保留作为"短期摘要"（每次压缩都更新）
		// long_summary 是"长期摘要"（每 N 次压缩合并一次，避免无限递归漂移）
		// compress_count 记录压缩次数，用于触发合并/重置
		{"long_summary", "TEXT"},
		{"compress_count", "INTEGER DEFAULT 0"},
	}

	for _, col := range convAddCols {
		if !convColumns[col.name] {
			// 安全说明（基于 GO-INJECT-001 #6）：col.name/col.typ 来自上方硬编码常量数组，不接受外部输入，
			// 故字符串拼接是安全的。SQLite ALTER TABLE 语句不支持 ? 占位符，必须使用字符串拼接。
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
		// 表名不在白名单内属于调用方传入非法参数，归为 InvalidInput
		return nil, apperror.Newf(apperror.KindInvalidInput, "table %q is not in allowed list", tableName)
	}
	// 安全说明（基于 GO-INJECT-001 #6）：tableName 已通过上方 allowedTables 白名单校验
	// （仅允许 "conversations" 和 "messages"），故字符串拼接是安全的。
	// SQLite PRAGMA 语句不支持 ? 占位符，必须使用字符串拼接。
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
		var dfltValue any
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
// 采用分批处理避免全量加载到内存
func migrateEncryptExistingData(db *sql.DB, encKey []byte) error {
	// 检查是否已完成迁移
	var migrationDone string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'encryption_migration_done'").Scan(&migrationDone)
	if err == nil && migrationDone == "yes" {
		return nil
	}

	log.Info().Msg("[db] starting encryption migration for existing plaintext data")

	// 加密 conversations.title（通常数量较少，可直接加载）
	if err := migrateConversations(db, encKey); err != nil {
		return apperror.Wrap(apperror.KindInternal, "migrate conversations", err)
	}

	// 加密 messages 的敏感字段（分批处理）
	if err := migrateMessages(db, encKey); err != nil {
		return apperror.Wrap(apperror.KindInternal, "migrate messages", err)
	}

	log.Info().Msg("[db] encryption migration completed")

	// 标记迁移完成
	_, err = db.Exec("INSERT OR REPLACE INTO settings (key, value) VALUES ('encryption_migration_done', 'yes')")
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "mark migration done", err)
	}

	return nil
}

// migrateConversations 加密会话标题
func migrateConversations(db *sql.DB, encKey []byte) error {
	rows, err := db.Query("SELECT id, title FROM conversations")
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "query conversations", err)
	}
	defer rows.Close()

	migrated := 0
	for rows.Next() {
		var id, title string
		if err := rows.Scan(&id, &title); err != nil {
			return apperror.Wrap(apperror.KindInternal, "scan conversation", err)
		}
		// 只加密非空且未加密的标题
		if title == "" || (len(title) >= 4 && title[:4] == "enc:") {
			continue
		}
		encTitle, err := encryptField(title, encKey)
		if err != nil {
			log.Error().Err(err).Str("id", id).Msg("[db] failed to encrypt conversation title during migration")
			continue
		}
		if _, err := db.Exec("UPDATE conversations SET title = ? WHERE id = ?", encTitle, id); err != nil {
			log.Error().Err(err).Str("id", id).Msg("[db] failed to update encrypted conversation title")
		}
		migrated++
	}

	log.Info().Int("migrated", migrated).Msg("[db] conversation title encryption done")
	return nil
}

// migrateMessages 分批加密消息的敏感字段
//
// 修复（M-后3）：原实现用 OFFSET 分页，每批 OFFSET N 会让 SQLite 重新扫描并跳过前 N 行，
// N 批迁移总成本 O(N²)。改为 keyset 分页：用上一批最后一条 id 作为游标（WHERE id > ?），
// 配合主键索引，每批都是 O(batchSize) 而非 O(offset+batchSize)。
//
// 修复（任务18）：原实现每条消息单独 db.Exec，N 条 = N 次独立事务，开销显著放大。
// 改为先收集本批所有待更新行，再用单个事务（db.Begin）批量 Exec，最后单次 Commit。
// 事务失败时 Rollback 并跳过该批，不阻塞后续批处理。
func migrateMessages(db *sql.DB, encKey []byte) error {
	const batchSize = 100
	lastID := "" // keyset 游标：上一批最后一条 id
	totalMigrated := 0

	// pendingUpdate 表示一条待更新的消息（已加密完成）
	type pendingUpdate struct {
		id                                      string
		encContent, encThinking, encSearch      string
		encImages, encAttachments, encToolCalls string
	}

	for {
		var rows *sql.Rows
		var err error
		if lastID == "" {
			rows, err = db.Query(
				"SELECT id, content, thinking_content, search_results, images, attachments, tool_calls FROM messages ORDER BY id LIMIT ?",
				batchSize,
			)
		} else {
			rows, err = db.Query(
				"SELECT id, content, thinking_content, search_results, images, attachments, tool_calls FROM messages WHERE id > ? ORDER BY id LIMIT ?",
				lastID, batchSize,
			)
		}
		if err != nil {
			return apperror.Wrap(apperror.KindInternal, "query messages batch", err)
		}

		// 收集本批所有需要更新的行（先加密，再统一写入事务）
		var updates []pendingUpdate
		batchCount := 0
		for rows.Next() {
			batchCount++
			var id string
			var content, thinkingContent, searchResults, images, attachments, toolCalls sql.NullString
			if err := rows.Scan(&id, &content, &thinkingContent, &searchResults, &images, &attachments, &toolCalls); err != nil {
				rows.Close()
				return apperror.Wrap(apperror.KindInternal, "scan message", err)
			}
			lastID = id // 更新游标为当前批最后一条 id

			// 检查 content 是否需要加密（只加密未加密的数据）
			if !content.Valid || content.String == "" || (len(content.String) >= 4 && content.String[:4] == "enc:") {
				continue
			}

			// 加密 content（主字段，错误时跳过该条消息）
			encContent, err := encryptField(content.String, encKey)
			if err != nil {
				log.Error().Err(err).Str("id", id).Msg("[db] failed to encrypt message content during migration, skipping")
				continue
			}

			// 加密其他字段（错误时保留原值，而非静默丢弃）
			updates = append(updates, pendingUpdate{
				id:             id,
				encContent:     encContent,
				encThinking:    encryptFieldWithFallback(thinkingContent, encKey),
				encSearch:      encryptFieldWithFallback(searchResults, encKey),
				encImages:      encryptFieldWithFallback(images, encKey),
				encAttachments: encryptFieldWithFallback(attachments, encKey),
				encToolCalls:   encryptFieldWithFallback(toolCalls, encKey),
			})
		}
		rows.Close()

		// 用单个事务批量执行 UPDATE，替代 N 次独立 db.Exec
		// 生活类比：与其每改一份文件就跑一趟档案室（N 次事务），不如攒齐一摞一次性提交（单事务）
		if len(updates) > 0 {
			tx, txErr := db.Begin()
			if txErr != nil {
				log.Error().Err(txErr).Msg("[db] begin tx for migration batch failed, skipping batch")
			} else {
				batchFailed := false
				for _, u := range updates {
					if _, execErr := tx.Exec(
						"UPDATE messages SET content = ?, thinking_content = ?, search_results = ?, images = ?, attachments = ?, tool_calls = ? WHERE id = ?",
						u.encContent, u.encThinking, u.encSearch, u.encImages, u.encAttachments, u.encToolCalls, u.id,
					); execErr != nil {
						log.Error().Err(execErr).Str("id", u.id).Msg("[db] failed to update in migration tx, rolling back batch")
						batchFailed = true
						break
					}
				}
				if batchFailed {
					// 整批回滚，跳过该批，继续处理下一批
					if rbErr := tx.Rollback(); rbErr != nil {
						log.Error().Err(rbErr).Msg("[db] rollback migration batch failed")
					}
				} else if commitErr := tx.Commit(); commitErr != nil {
					// Commit 失败也尝试 Rollback（虽然事务可能已无效）
					log.Error().Err(commitErr).Msg("[db] commit migration batch failed")
					_ = tx.Rollback()
				} else {
					totalMigrated += len(updates)
				}
			}
		}

		// 本批不足 batchSize 条，说明已处理完
		if batchCount < batchSize {
			break
		}
	}

	log.Info().Int("migrated", totalMigrated).Msg("[db] message encryption done")
	return nil
}

// encryptFieldWithFallback 加密 NullString 字段，失败时保留原值
func encryptFieldWithFallback(ns sql.NullString, encKey []byte) string {
	if !ns.Valid || ns.String == "" {
		return ns.String
	}
	if len(ns.String) >= 4 && ns.String[:4] == "enc:" {
		return ns.String
	}
	encrypted, err := encryptField(ns.String, encKey)
	if err != nil {
		log.Error().Err(err).Msg("[db] encrypt field failed, keeping original value")
		return ns.String // 保留原值而非返回空字符串
	}
	return encrypted
}
