// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"crypto/rand"
	"database/sql"
	"path/filepath"
	"testing"
)

// newTestDB 创建用于 conversation 测试的临时数据库
func newTestDB(t *testing.T) (*sql.DB, []byte, func()) {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	db, err := Init(dbPath, nil)
	if err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	cleanup := func() {
		db.Close()
	}
	return db, encKey, cleanup
}

// insertTestConversation 插入一条测试用对话（不带 user 消息，用于构造异常对话）
func insertTestConversation(t *testing.T, db *sql.DB, id, title string) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO conversations (id, title, created_at, updated_at) VALUES (?, ?, datetime('now','-10 minutes'), datetime('now','-10 minutes'))",
		id, title,
	); err != nil {
		t.Fatalf("插入对话 %s 失败: %v", id, err)
	}
}

// TestDeleteConversationsBatchEmpty 测试空切片不会报错
func TestDeleteConversationsBatchEmpty(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	if err := DeleteConversationsBatch(db, nil); err != nil {
		t.Errorf("DeleteConversationsBatch(nil) 不应报错, 实际: %v", err)
	}
	if err := DeleteConversationsBatch(db, []string{}); err != nil {
		t.Errorf("DeleteConversationsBatch([]) 不应报错, 实际: %v", err)
	}
}

// TestDeleteConversationsBatchBasic 测试批量删除对话及其消息
func TestDeleteConversationsBatchBasic(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	// 插入 3 个对话
	for _, id := range []string{"batch-1", "batch-2", "batch-3"} {
		insertTestConversation(t, db, id, "title")
		// 给每个对话插入一条 assistant 消息（无 user 消息即为异常）
		if _, err := db.Exec(
			"INSERT INTO messages (id, conversation_id, role, content, created_at) VALUES (?, ?, ?, ?, datetime('now'))",
			"msg-"+id, id, "assistant", "test",
		); err != nil {
			t.Fatalf("插入消息失败: %v", err)
		}
	}

	// 批量删除前：3 个对话 + 3 条消息
	var convCount, msgCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&convCount); err != nil {
		t.Fatalf("统计对话失败: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&msgCount); err != nil {
		t.Fatalf("统计消息失败: %v", err)
	}
	if convCount != 3 || msgCount != 3 {
		t.Fatalf("前置条件不满足: 对话=%d 消息=%d", convCount, msgCount)
	}

	// 批量删除
	if err := DeleteConversationsBatch(db, []string{"batch-1", "batch-2", "batch-3"}); err != nil {
		t.Fatalf("DeleteConversationsBatch 失败: %v", err)
	}

	// 验证：对话和消息都被删除
	if err := db.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&convCount); err != nil {
		t.Fatalf("统计对话失败: %v", err)
	}
	if err := db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&msgCount); err != nil {
		t.Fatalf("统计消息失败: %v", err)
	}
	if convCount != 0 {
		t.Errorf("批量删除后对话应为 0, 实际 %d", convCount)
	}
	if msgCount != 0 {
		t.Errorf("批量删除后消息应为 0, 实际 %d", msgCount)
	}
}

// TestDeleteConversationsBatchPartial 测试批量删除部分对话，未删除的保留
func TestDeleteConversationsBatchPartial(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	insertTestConversation(t, db, "keep-1", "保留")
	insertTestConversation(t, db, "del-1", "删除")
	insertTestConversation(t, db, "del-2", "删除")

	if err := DeleteConversationsBatch(db, []string{"del-1", "del-2"}); err != nil {
		t.Fatalf("DeleteConversationsBatch 失败: %v", err)
	}

	var convCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&convCount); err != nil {
		t.Fatalf("统计对话失败: %v", err)
	}
	if convCount != 1 {
		t.Errorf("部分删除后应剩 1 个对话, 实际 %d", convCount)
	}

	// 验证剩余的是 keep-1
	var id string
	if err := db.QueryRow("SELECT id FROM conversations").Scan(&id); err != nil {
		t.Fatalf("查询剩余对话失败: %v", err)
	}
	if id != "keep-1" {
		t.Errorf("剩余对话应为 keep-1, 实际 %s", id)
	}
}

// TestDeleteConversationsBatchAtomic 测试批量删除的原子性：
// 若其中一个 id 不存在（DELETE 不报错），其他 id 仍应被正常删除。
// SQLite 的 DELETE 对不存在的行不报错，因此整批应成功。
func TestDeleteConversationsBatchAtomic(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	insertTestConversation(t, db, "exist-1", "存在")

	// 包含一个不存在的 id
	if err := DeleteConversationsBatch(db, []string{"exist-1", "not-exist"}); err != nil {
		t.Fatalf("DeleteConversationsBatch 失败: %v", err)
	}

	var convCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&convCount); err != nil {
		t.Fatalf("统计对话失败: %v", err)
	}
	if convCount != 0 {
		t.Errorf("批量删除后对话应为 0, 实际 %d", convCount)
	}
}

// TestCleanupAbnormalConversationsBatchPath 测试 CleanupAbnormalConversations
// 使用批量删除路径清理异常对话
func TestCleanupAbnormalConversationsBatchPath(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	// 插入 2 个异常对话（无 user 消息，且 created_at 早于 5 分钟前）
	insertTestConversation(t, db, "abn-1", "异常1")
	insertTestConversation(t, db, "abn-2", "异常2")

	// 插入 1 个正常对话（有 user 消息）
	insertTestConversation(t, db, "norm-1", "正常")
	if _, err := db.Exec(
		"INSERT INTO messages (id, conversation_id, role, content, created_at) VALUES (?, ?, ?, ?, datetime('now'))",
		"msg-norm", "norm-1", "user", "用户消息",
	); err != nil {
		t.Fatalf("插入消息失败: %v", err)
	}

	removed, err := CleanupAbnormalConversations(db, encKey)
	if err != nil {
		t.Fatalf("CleanupAbnormalConversations 失败: %v", err)
	}
	if len(removed) != 2 {
		t.Fatalf("期望清理 2 个异常对话, 实际 %d", len(removed))
	}

	// 验证异常对话已删除，正常对话保留
	var convCount int
	if err := db.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&convCount); err != nil {
		t.Fatalf("统计对话失败: %v", err)
	}
	if convCount != 1 {
		t.Errorf("清理后应剩 1 个正常对话, 实际 %d", convCount)
	}

	var id string
	if err := db.QueryRow("SELECT id FROM conversations").Scan(&id); err != nil {
		t.Fatalf("查询剩余对话失败: %v", err)
	}
	if id != "norm-1" {
		t.Errorf("剩余对话应为 norm-1, 实际 %s", id)
	}
}

// TestCleanupAbnormalConversationsEmpty 测试无异常对话时返回 nil
func TestCleanupAbnormalConversationsEmpty(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	// 只插入正常对话
	insertTestConversation(t, db, "norm-1", "正常")
	if _, err := db.Exec(
		"INSERT INTO messages (id, conversation_id, role, content, created_at) VALUES (?, ?, ?, ?, datetime('now'))",
		"msg-norm", "norm-1", "user", "用户消息",
	); err != nil {
		t.Fatalf("插入消息失败: %v", err)
	}

	removed, err := CleanupAbnormalConversations(db, encKey)
	if err != nil {
		t.Fatalf("CleanupAbnormalConversations 失败: %v", err)
	}
	if removed != nil {
		t.Errorf("无异常对话时返回应为 nil, 实际长度 %d", len(removed))
	}
}
