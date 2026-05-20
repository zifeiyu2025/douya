// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

var fts5Available bool

func Init(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := Migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func Migrate(db *sql.DB) error {
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
	`)
	if err != nil {
		return err
	}

	if err := migrateAddColumns(db); err != nil {
		return err
	}

	logStartupSchema(db)

	fts5Available = checkFTS5(db)
	if fts5Available {
		_, err = db.Exec(`
			CREATE VIRTUAL TABLE IF NOT EXISTS messages_fts USING fts5(
				content,
				content=messages,
				content_rowid=rowid
			);
			CREATE TRIGGER IF NOT EXISTS messages_ai AFTER INSERT ON messages BEGIN
				INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
			END;
			CREATE TRIGGER IF NOT EXISTS messages_ad AFTER DELETE ON messages BEGIN
				INSERT INTO messages_fts(messages_fts, rowid, content) VALUES('delete', old.rowid, old.content);
			END;
			CREATE TRIGGER IF NOT EXISTS messages_au AFTER UPDATE ON messages BEGIN
				INSERT INTO messages_fts(messages_fts, rowid, content) VALUES('delete', old.rowid, old.content);
				INSERT INTO messages_fts(rowid, content) VALUES (new.rowid, new.content);
			END;
		`)
		if err != nil {
			log.Printf("[db] FTS5 setup failed: %v, cleaning up FTS5 artifacts", err)
			fts5Available = false
			dropFTS5Artifacts(db)
		}
	} else {
		dropFTS5Artifacts(db)
	}

	return nil
}

func migrateAddColumns(db *sql.DB) error {
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

func checkFTS5(db *sql.DB) bool {
	var ok bool
	row := db.QueryRow("SELECT 1 FROM pragma_compile_options WHERE compile_options = 'ENABLE_FTS5'")
	if err := row.Scan(&ok); err != nil {
		_, err := db.Exec("CREATE VIRTUAL TABLE IF NOT EXISTS _fts5_test USING fts5(x)")
		if err != nil {
			return false
		}
		db.Exec("DROP TABLE IF EXISTS _fts5_test")
		return true
	}
	return true
}

func FTS5Available() bool {
	return fts5Available
}

func logStartupSchema(db *sql.DB) {
	cols, err := GetTableColumns(db, "messages")
	if err != nil {
		log.Printf("[db] failed to get messages schema: %v", err)
		return
	}
	var names []string
	for k := range cols {
		names = append(names, k)
	}
	log.Printf("[db] messages table columns: %s", strings.Join(names, ", "))
	log.Printf("[db] FTS5 available: %v", fts5Available)
}

func dropFTS5Artifacts(db *sql.DB) {
	_, _ = db.Exec("DROP TRIGGER IF EXISTS messages_ai")
	_, _ = db.Exec("DROP TRIGGER IF EXISTS messages_ad")
	_, _ = db.Exec("DROP TRIGGER IF EXISTS messages_au")
	_, err := db.Exec("DROP TABLE IF EXISTS messages_fts")
	if err != nil {
		log.Printf("[db] could not drop messages_fts table (FTS5 module unavailable): %v", err)
	}
	log.Printf("[db] cleaned up FTS5 triggers and virtual table")
}
