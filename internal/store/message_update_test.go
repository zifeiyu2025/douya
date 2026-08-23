// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"errors"
	"strings"
	"testing"

	"douya/internal/apperror"
)

// TestUpdateMessageContent_Success 验证编辑消息正文的核心链路：
// 新内容能正确加密落库，读回时自动解密为明文，且其余字段不受影响。
func TestUpdateMessageContent_Success(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	msg := insertTestMessage(t, db, encKey, "conv-edit-1", "user",
		"原始内容", "思考草稿保持不变", "", "")

	newContent := "这是编辑后的新内容"
	if err := UpdateMessageContent(db, msg.ID, newContent, encKey); err != nil {
		t.Fatalf("UpdateMessageContent 失败: %v", err)
	}

	got, err := GetMessage(db, msg.ID, encKey)
	if err != nil {
		t.Fatalf("GetMessage 失败: %v", err)
	}
	if got.Content != newContent {
		t.Errorf("编辑后内容不匹配，期望 %q，实际 %q", newContent, got.Content)
	}
	// 编辑只允许影响 content 字段，其他字段必须原样保留
	if got.ThinkingContent != "思考草稿保持不变" {
		t.Errorf("ThinkingContent 被意外修改，实际: %q", got.ThinkingContent)
	}
	if got.Role != "user" || got.ConversationID != "conv-edit-1" {
		t.Errorf("Role/ConversationID 被意外修改，实际 role=%s conv=%s", got.Role, got.ConversationID)
	}
}

// TestUpdateMessageContent_EncryptedAtRest 验证静态加密：
// 直接读取数据库原始 content 列，必须是 "enc:" 前缀的密文，不能出现明文。
// 这是与 CreateMessage 一致的安全底线。
func TestUpdateMessageContent_EncryptedAtRest(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	msg := insertTestMessage(t, db, encKey, "conv-edit-2", "user", "原始内容", "", "", "")

	newContent := "落库前必须加密的敏感内容"
	if err := UpdateMessageContent(db, msg.ID, newContent, encKey); err != nil {
		t.Fatalf("UpdateMessageContent 失败: %v", err)
	}

	var raw string
	err := db.QueryRow("SELECT content FROM messages WHERE id = ?", msg.ID).Scan(&raw)
	if err != nil {
		t.Fatalf("读取原始密文失败: %v", err)
	}
	if !strings.HasPrefix(raw, "enc:") {
		t.Errorf("落库内容缺少 enc: 前缀，疑似明文存储: %q", raw)
	}
	if strings.Contains(raw, newContent) {
		t.Errorf("落库内容包含明文，加密失效: %q", raw)
	}
}

// TestUpdateMessageContent_NotFound 验证目标消息不存在时返回统一的 NotFound 错误，
// 便于上层给出准确提示而不是笼统的"更新失败"。
func TestUpdateMessageContent_NotFound(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	err := UpdateMessageContent(db, "no-such-message-id", "任意内容", encKey)
	if err == nil {
		t.Fatal("更新不存在的消息应返回错误，实际返回 nil")
	}
	if !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("期望 apperror.ErrNotFound，实际: %v", err)
	}
}

// TestUpdateMessageContent_RepeatedEdits 验证同一消息可被多次编辑，
// 每次都以最新内容为准（幂等覆盖语义）。
func TestUpdateMessageContent_RepeatedEdits(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	msg := insertTestMessage(t, db, encKey, "conv-edit-3", "user", "第一版", "", "", "")

	for i, content := range []string{"第二版", "第三版", "最终版"} {
		if err := UpdateMessageContent(db, msg.ID, content, encKey); err != nil {
			t.Fatalf("第 %d 次编辑失败: %v", i+2, err)
		}
		got, err := GetMessage(db, msg.ID, encKey)
		if err != nil {
			t.Fatalf("第 %d 次读回失败: %v", i+2, err)
		}
		if got.Content != content {
			t.Errorf("第 %d 次编辑后内容不匹配，期望 %q，实际 %q", i+2, content, got.Content)
		}
	}
}
