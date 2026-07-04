// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"database/sql"
	"testing"
)

// TestDeleteMessagesBatch_BatchDelete 验证批量删除消息功能。
//
// 业务场景：删除多条消息时，原实现循环调用 DeleteMessage（N+1 问题），
// 每条消息独立事务，性能差。批量化后单事务内 DELETE WHERE id IN (...)，性能提升 N 倍。
func TestDeleteMessagesBatch_BatchDelete(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	// 准备：创建 1 个会话 + 5 条消息
	const convID = "conv-batch-1"
	insertTestConversation(t, db, convID, "测试会话")
	msgIDs := []string{}
	for i := 0; i < 5; i++ {
		msg := &Message{
			ConversationID: convID,
			Role:           "user",
			Content:        "消息内容",
		}
		if err := CreateMessage(db, msg, encKey); err != nil {
			t.Fatalf("CreateMessage 失败: %v", err)
		}
		msgIDs = append(msgIDs, msg.ID)
	}

	// 验证：删除前有 5 条消息
	count := countMessagesInConv(t, db, convID)
	if count != 5 {
		t.Fatalf("删除前应有 5 条消息，实际: %d", count)
	}

	// 执行批量删除
	err := DeleteMessagesBatch(db, msgIDs)
	if err != nil {
		t.Fatalf("DeleteMessagesBatch 失败: %v", err)
	}

	// 验证：删除后有 0 条消息
	count = countMessagesInConv(t, db, convID)
	if count != 0 {
		t.Errorf("删除后应有 0 条消息，实际: %d", count)
	}
}

// TestDeleteMessagesBatch_EmptyList 验证空列表不报错
func TestDeleteMessagesBatch_EmptyList(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	err := DeleteMessagesBatch(db, []string{})
	if err != nil {
		t.Errorf("空列表不应返回错误，实际: %v", err)
	}

	// nil 也应安全
	err = DeleteMessagesBatch(db, nil)
	if err != nil {
		t.Errorf("nil 列表不应返回错误，实际: %v", err)
	}
}

// TestDeleteMessagesBatch_PartialExist 验证部分 ID 不存在时不影响其他删除
func TestDeleteMessagesBatch_PartialExist(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	const convID = "conv-partial-1"
	insertTestConversation(t, db, convID, "测试会话")

	// 创建 3 条消息
	realIDs := []string{}
	for i := 0; i < 3; i++ {
		msg := &Message{
			ConversationID: convID,
			Role:           "user",
			Content:        "消息",
		}
		if err := CreateMessage(db, msg, encKey); err != nil {
			t.Fatalf("CreateMessage 失败: %v", err)
		}
		realIDs = append(realIDs, msg.ID)
	}

	// 混合真实 ID 和不存在的 ID
	mixedIDs := []string{realIDs[0], "nonexistent-1", realIDs[1], "nonexistent-2", realIDs[2]}

	err := DeleteMessagesBatch(db, mixedIDs)
	if err != nil {
		t.Fatalf("部分存在时不应返回错误，实际: %v", err)
	}

	// 验证：真实 ID 对应的 3 条消息都被删除
	count := countMessagesInConv(t, db, convID)
	if count != 0 {
		t.Errorf("部分存在时真实 ID 应被删除，剩余: %d", count)
	}
}

// countMessagesInConv 统计指定会话的消息数（测试辅助函数）
func countMessagesInConv(t *testing.T, db *sql.DB, convID string) int {
	t.Helper()
	var count int
	err := db.QueryRow("SELECT COUNT(*) FROM messages WHERE conversation_id = ?", convID).Scan(&count)
	if err != nil {
		t.Fatalf("统计消息数失败: %v", err)
	}
	return count
}
