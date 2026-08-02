// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store

import (
	"crypto/rand"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"douya/internal/apperror"
)

// ===== 辅助函数 =====

// newValidEncKey 返回合法的 32 字节 AES-256 密钥
func newValidEncKey(t *testing.T) []byte {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}
	return key
}

// newInvalidEncKey 返回非法长度的密钥（1 字节，AES 不接受）
// 生活类比：用错误尺寸的钥匙开锁，锁芯会拒绝。
func newInvalidEncKey() []byte {
	return []byte{0x01}
}

// countConversations 统计数据库中的会话数量
func countConversations(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM conversations").Scan(&count); err != nil {
		t.Fatalf("统计会话失败: %v", err)
	}
	return count
}

// countMessages 统计数据库中的消息数量
func countMessages(t *testing.T, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM messages").Scan(&count); err != nil {
		t.Fatalf("统计消息失败: %v", err)
	}
	return count
}

// insertSimpleMessage 插入一条简单的测试消息（直接用 SQL，不经加密层）
// 与 message_test.go 的 insertSimpleMessage 区别：后者走 CreateMessage 加密流程，签名不同
func insertSimpleMessage(t *testing.T, db *sql.DB, id, convID, role, content string) {
	t.Helper()
	if _, err := db.Exec(
		"INSERT INTO messages (id, conversation_id, role, content, created_at) VALUES (?, ?, ?, ?, datetime('now'))",
		id, convID, role, content,
	); err != nil {
		t.Fatalf("插入消息 %s 失败: %v", id, err)
	}
}

// ===== CreateConversation 测试 =====

// TestCreateConversation_Success 验证正常创建会话
func TestCreateConversation_Success(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	conv := &Conversation{
		Title: "测试对话",
	}
	if err := CreateConversation(db, conv, encKey); err != nil {
		t.Fatalf("CreateConversation 失败: %v", err)
	}
	if conv.ID == "" {
		t.Error("期望 ID 被自动生成，实际为空")
	}
	if conv.CreatedAt.IsZero() {
		t.Error("期望 CreatedAt 被自动设置，实际为零值")
	}

	// 验证数据库中确实有这条记录
	if count := countConversations(t, db); count != 1 {
		t.Errorf("期望 1 条会话，实际 %d", count)
	}

	// 验证标题被加密（数据库中存储的是 enc: 前缀的密文）
	var storedTitle string
	if err := db.QueryRow("SELECT title FROM conversations WHERE id = ?", conv.ID).Scan(&storedTitle); err != nil {
		t.Fatalf("查询标题失败: %v", err)
	}
	if !strings.HasPrefix(storedTitle, "enc:") {
		t.Errorf("期望标题被加密（enc: 前缀），实际: %q", storedTitle)
	}
}

// TestCreateConversation_WithPresetID 验证带预设 ID 的创建
func TestCreateConversation_WithPresetID(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	conv := &Conversation{
		ID:    "preset-id-123",
		Title: "预设ID对话",
	}
	if err := CreateConversation(db, conv, encKey); err != nil {
		t.Fatalf("CreateConversation 失败: %v", err)
	}
	if conv.ID != "preset-id-123" {
		t.Errorf("期望 ID 保持预设值，实际: %s", conv.ID)
	}
}

// TestCreateConversation_EmptyTitle 验证空标题不加密（encryptField 对空串返回空串）
func TestCreateConversation_EmptyTitle(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	conv := &Conversation{Title: ""}
	if err := CreateConversation(db, conv, encKey); err != nil {
		t.Fatalf("CreateConversation 失败: %v", err)
	}

	var storedTitle string
	if err := db.QueryRow("SELECT title FROM conversations WHERE id = ?", conv.ID).Scan(&storedTitle); err != nil {
		t.Fatalf("查询标题失败: %v", err)
	}
	if storedTitle != "" {
		t.Errorf("期望空标题保持空，实际: %q", storedTitle)
	}
}

// TestCreateConversation_EncryptFail 验证加密失败时返回错误且不写入数据库
// 生活类比：锁匠配钥匙时发现钥匙尺寸不对，就不应该继续开锁，而是直接报错。
func TestCreateConversation_EncryptFail(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	conv := &Conversation{Title: "测试"}
	// 使用非法长度密钥触发加密失败
	invalidKey := newInvalidEncKey()

	err := CreateConversation(db, conv, invalidKey)
	if err == nil {
		t.Fatal("期望加密失败返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "encrypt") {
		t.Errorf("期望错误包含 encrypt，实际: %v", err)
	}

	// 验证数据库中没有写入任何记录
	if count := countConversations(t, db); count != 0 {
		t.Errorf("加密失败时不应写入数据库，实际有 %d 条记录", count)
	}
}

// TestCreateConversation_DBFail 验证 DB 插入失败时返回错误
// 通过关闭 DB 连接触发 sql.ErrConnDone
func TestCreateConversation_DBFail(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	cleanup() // 提前关闭 DB

	conv := &Conversation{Title: "测试"}
	err := CreateConversation(db, conv, encKey)
	if err == nil {
		t.Fatal("期望 DB 关闭后返回错误，实际返回 nil")
	}
}

// TestCreateConversation_DuplicateID 验证重复 ID 插入失败
func TestCreateConversation_DuplicateID(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	conv1 := &Conversation{ID: "dup-id", Title: "第一个"}
	if err := CreateConversation(db, conv1, encKey); err != nil {
		t.Fatalf("第一次创建失败: %v", err)
	}

	conv2 := &Conversation{ID: "dup-id", Title: "第二个"}
	err := CreateConversation(db, conv2, encKey)
	if err == nil {
		t.Fatal("期望重复 ID 返回错误，实际返回 nil")
	}
}

// ===== GetConversation 测试 =====

// TestGetConversation_Success 验证正常获取会话（含解密）
func TestGetConversation_Success(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	// 创建一条会话
	original := &Conversation{Title: "测试获取"}
	if err := CreateConversation(db, original, encKey); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 获取并验证
	conv, err := GetConversation(db, original.ID, encKey)
	if err != nil {
		t.Fatalf("GetConversation 失败: %v", err)
	}
	if conv.Title != "测试获取" {
		t.Errorf("期望标题 '测试获取'，实际: %q", conv.Title)
	}
	if conv.ID != original.ID {
		t.Errorf("期望 ID %s，实际: %s", original.ID, conv.ID)
	}
}

// TestGetConversation_NotFound 验证不存在的 ID 返回 NotFound 错误
func TestGetConversation_NotFound(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	_, err := GetConversation(db, "nonexistent-id", encKey)
	if err == nil {
		t.Fatal("期望 NotFound 错误，实际返回 nil")
	}
	if !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("期望 apperror.ErrNotFound，实际: %v", err)
	}
}

// TestGetConversation_DecryptFail 验证解密失败时降级为占位符
// 生活类比：锁匠用错误的钥匙开锁，打不开但门还在，于是挂个"锁坏了"的牌子让人知道。
func TestGetConversation_DecryptFail(t *testing.T) {
	db, encKeyA, cleanup := newTestDB(t)
	defer cleanup()

	// 用密钥 A 加密创建会话
	conv := &Conversation{Title: "密钥A加密"}
	if err := CreateConversation(db, conv, encKeyA); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 用密钥 B 解密，应触发解密失败降级
	encKeyB := newValidEncKey(t)
	got, err := GetConversation(db, conv.ID, encKeyB)
	if err != nil {
		t.Fatalf("解密失败不应返回错误（降级为占位符），实际: %v", err)
	}
	if got.Title != "[解密失败]" {
		t.Errorf("期望标题降级为 '[解密失败]'，实际: %q", got.Title)
	}
}

// TestGetConversation_PlaintextCompat 验证旧版明文数据兼容
// 没有 enc: 前缀的标题应直接返回（不尝试解密）
func TestGetConversation_PlaintextCompat(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	// 直接插入明文标题（模拟旧版数据）
	insertTestConversation(t, db, "old-conv", "明文标题")

	conv, err := GetConversation(db, "old-conv", encKey)
	if err != nil {
		t.Fatalf("GetConversation 失败: %v", err)
	}
	if conv.Title != "明文标题" {
		t.Errorf("期望明文标题直接返回，实际: %q", conv.Title)
	}
}

// ===== ListConversations 测试 =====

// TestListConversations_Empty 验证空数据库返回空列表
func TestListConversations_Empty(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	convs, err := ListConversations(db, encKey)
	if err != nil {
		t.Fatalf("ListConversations 失败: %v", err)
	}
	if len(convs) != 0 {
		t.Errorf("期望空列表，实际 %d 条", len(convs))
	}
}

// TestListConversations_WithDecrypt 验证列表查询含解密
func TestListConversations_WithDecrypt(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	titles := []string{"对话A", "对话B", "对话C"}
	for _, title := range titles {
		if err := CreateConversation(db, &Conversation{Title: title}, encKey); err != nil {
			t.Fatalf("创建失败: %v", err)
		}
	}

	convs, err := ListConversations(db, encKey)
	if err != nil {
		t.Fatalf("ListConversations 失败: %v", err)
	}
	if len(convs) != 3 {
		t.Fatalf("期望 3 条会话，实际 %d", len(convs))
	}

	// 验证所有标题都被正确解密
	titleSet := make(map[string]bool)
	for _, c := range convs {
		titleSet[c.Title] = true
	}
	for _, expected := range titles {
		if !titleSet[expected] {
			t.Errorf("期望包含标题 %q，实际未找到", expected)
		}
	}
}

// TestListConversations_DecryptFail 验证列表中解密失败降级
func TestListConversations_DecryptFail(t *testing.T) {
	db, encKeyA, cleanup := newTestDB(t)
	defer cleanup()

	// 用密钥 A 创建
	if err := CreateConversation(db, &Conversation{Title: "密钥A"}, encKeyA); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	// 用密钥 B 列表查询，应降级
	encKeyB := newValidEncKey(t)
	convs, err := ListConversations(db, encKeyB)
	if err != nil {
		t.Fatalf("ListConversations 失败: %v", err)
	}
	if len(convs) != 1 {
		t.Fatalf("期望 1 条会话，实际 %d", len(convs))
	}
	if convs[0].Title != "[解密失败]" {
		t.Errorf("期望降级为 '[解密失败]'，实际: %q", convs[0].Title)
	}
}

// TestListConversations_PlaintextCompat 验证列表查询兼容明文标题
func TestListConversations_PlaintextCompat(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	insertTestConversation(t, db, "plain-1", "明文标题1")
	insertTestConversation(t, db, "plain-2", "明文标题2")

	convs, err := ListConversations(db, encKey)
	if err != nil {
		t.Fatalf("ListConversations 失败: %v", err)
	}
	if len(convs) != 2 {
		t.Fatalf("期望 2 条会话，实际 %d", len(convs))
	}
	for _, c := range convs {
		if !strings.HasPrefix(c.Title, "明文标题") {
			t.Errorf("期望明文标题直接返回，实际: %q", c.Title)
		}
	}
}

// ===== UpdateConversation 测试 =====

// TestUpdateConversation_Success 验证正常更新
// 风险：UpdateConversation 未刷新 UpdatedAt，导致排序/过期判断错误。
// 注意：Windows 系统时钟精度约 15ms，两次连续 time.Now() 可能返回相同值，
// 需在创建后、更新前插入微小延迟，避免 flake（具名风险：wall-clock 精度）。
func TestUpdateConversation_Success(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	conv := &Conversation{Title: "旧标题"}
	if err := CreateConversation(db, conv, encKey); err != nil {
		t.Fatalf("创建失败: %v", err)
	}
	originalUpdatedAt := conv.UpdatedAt

	// 确保 time.Now() 推进，避免 Windows 时钟精度导致两次取值相同
	time.Sleep(20 * time.Millisecond)

	conv.Title = "新标题"
	if err := UpdateConversation(db, conv, encKey); err != nil {
		t.Fatalf("UpdateConversation 失败: %v", err)
	}

	// 验证更新后的标题
	got, err := GetConversation(db, conv.ID, encKey)
	if err != nil {
		t.Fatalf("GetConversation 失败: %v", err)
	}
	if got.Title != "新标题" {
		t.Errorf("期望标题 '新标题'，实际: %q", got.Title)
	}
	// UpdatedAt 应被刷新：必须严格大于原值，才能检测"忘记更新 UpdatedAt"的回归
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("期望 UpdatedAt 被刷新，原: %v 现: %v", originalUpdatedAt, got.UpdatedAt)
	}
}

// TestUpdateConversation_EncryptFail 验证加密失败时返回错误
func TestUpdateConversation_EncryptFail(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	defer cleanup()

	conv := &Conversation{Title: "原标题"}
	if err := CreateConversation(db, conv, encKey); err != nil {
		t.Fatalf("创建失败: %v", err)
	}

	conv.Title = "新标题"
	invalidKey := newInvalidEncKey()
	err := UpdateConversation(db, conv, invalidKey)
	if err == nil {
		t.Fatal("期望加密失败返回错误，实际返回 nil")
	}
	if !strings.Contains(err.Error(), "encrypt") {
		t.Errorf("期望错误包含 encrypt，实际: %v", err)
	}

	// 验证原标题未被修改
	got, _ := GetConversation(db, conv.ID, encKey)
	if got.Title != "原标题" {
		t.Errorf("加密失败时不应更新标题，期望 '原标题'，实际: %q", got.Title)
	}
}

// TestUpdateConversation_DBFail 验证 DB 失败时返回错误
func TestUpdateConversation_DBFail(t *testing.T) {
	db, encKey, cleanup := newTestDB(t)
	cleanup() // 提前关闭

	conv := &Conversation{ID: "test", Title: "测试"}
	err := UpdateConversation(db, conv, encKey)
	if err == nil {
		t.Fatal("期望 DB 关闭后返回错误，实际返回 nil")
	}
}

// ===== DeleteConversation 测试（事务+回滚关键路径） =====

// TestDeleteConversation_Success 验证正常删除（含消息）
func TestDeleteConversation_Success(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	// 插入会话 + 消息
	insertTestConversation(t, db, "del-1", "删除")
	insertSimpleMessage(t, db, "msg-1", "del-1", "user", "内容")
	insertSimpleMessage(t, db, "msg-2", "del-1", "assistant", "回复")

	if count := countConversations(t, db); count != 1 {
		t.Fatalf("前置条件: 期望 1 条会话，实际 %d", count)
	}
	if count := countMessages(t, db); count != 2 {
		t.Fatalf("前置条件: 期望 2 条消息，实际 %d", count)
	}

	if err := DeleteConversation(db, "del-1"); err != nil {
		t.Fatalf("DeleteConversation 失败: %v", err)
	}

	if count := countConversations(t, db); count != 0 {
		t.Errorf("期望 0 条会话，实际 %d", count)
	}
	if count := countMessages(t, db); count != 0 {
		t.Errorf("期望 0 条消息（事务删除），实际 %d", count)
	}
}

// TestDeleteConversation_NonExist 验证删除不存在的 ID 不报错（SQLite DELETE 对不存在行不报错）
func TestDeleteConversation_NonExist(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	if err := DeleteConversation(db, "nonexistent"); err != nil {
		t.Errorf("删除不存在的 ID 不应报错，实际: %v", err)
	}
}

// TestDeleteConversation_DBFail 验证 DB 失败时返回错误（事务 BeginTx 失败）
// 生活类比：银行转账时系统突然断电，事务应该回滚，不能钱扣了对方没收到。
func TestDeleteConversation_DBFail(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	cleanup() // 提前关闭 DB

	err := DeleteConversation(db, "test")
	if err == nil {
		t.Fatal("期望 DB 关闭后返回错误，实际返回 nil")
	}
}

// TestDeleteConversation_WithMessages 验证删除会话时消息也被删除（事务原子性）
func TestDeleteConversation_WithMessages(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	// 插入 2 个会话，每个有消息
	insertTestConversation(t, db, "conv-a", "A")
	insertTestConversation(t, db, "conv-b", "B")
	insertSimpleMessage(t, db, "msg-a1", "conv-a", "user", "A1")
	insertSimpleMessage(t, db, "msg-a2", "conv-a", "assistant", "A2")
	insertSimpleMessage(t, db, "msg-b1", "conv-b", "user", "B1")

	// 删除 conv-a
	if err := DeleteConversation(db, "conv-a"); err != nil {
		t.Fatalf("DeleteConversation 失败: %v", err)
	}

	// 验证 conv-a 和其消息被删除，conv-b 和其消息保留
	if count := countConversations(t, db); count != 1 {
		t.Errorf("期望 1 条会话，实际 %d", count)
	}
	if count := countMessages(t, db); count != 1 {
		t.Errorf("期望 1 条消息（conv-b 的），实际 %d", count)
	}

	// 验证剩余的是 conv-b
	var id string
	if err := db.QueryRow("SELECT id FROM conversations").Scan(&id); err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if id != "conv-b" {
		t.Errorf("期望剩余 conv-b，实际: %s", id)
	}
}

// ===== DeleteConversationsBatch 事务回滚测试 =====

// TestDeleteConversationsBatch_DBFail 验证批量删除在 DB 失败时返回错误
func TestDeleteConversationsBatch_DBFail(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	cleanup() // 提前关闭

	err := DeleteConversationsBatch(db, []string{"id1", "id2"})
	if err == nil {
		t.Fatal("期望 DB 关闭后返回错误，实际返回 nil")
	}
}

// ===== 分层摘要函数测试 =====

// TestUpdateConversationLayeredSummary_ShortOnly 验证仅更新短期摘要
func TestUpdateConversationLayeredSummary_ShortOnly(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	insertTestConversation(t, db, "sum-1", "测试")

	if err := UpdateConversationLayeredSummary(db, "sum-1", "短期摘要", ""); err != nil {
		t.Fatalf("UpdateConversationLayeredSummary 失败: %v", err)
	}

	short, long, count, err := GetConversationLayeredSummary(db, "sum-1")
	if err != nil {
		t.Fatalf("GetConversationLayeredSummary 失败: %v", err)
	}
	if short != "短期摘要" {
		t.Errorf("期望短期摘要 '短期摘要'，实际: %q", short)
	}
	if long != "" {
		t.Errorf("期望长期摘要为空，实际: %q", long)
	}
	if count != 1 {
		t.Errorf("期望压缩计数 1，实际: %d", count)
	}
}

// TestUpdateConversationLayeredSummary_Both 验证同时更新短期+长期摘要
func TestUpdateConversationLayeredSummary_Both(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	insertTestConversation(t, db, "sum-2", "测试")

	if err := UpdateConversationLayeredSummary(db, "sum-2", "短期", "长期"); err != nil {
		t.Fatalf("UpdateConversationLayeredSummary 失败: %v", err)
	}

	short, long, count, err := GetConversationLayeredSummary(db, "sum-2")
	if err != nil {
		t.Fatalf("GetConversationLayeredSummary 失败: %v", err)
	}
	if short != "短期" {
		t.Errorf("期望短期 '短期'，实际: %q", short)
	}
	if long != "长期" {
		t.Errorf("期望长期 '长期'，实际: %q", long)
	}
	if count != 1 {
		t.Errorf("期望计数 1，实际: %d", count)
	}
}

// TestUpdateConversationLayeredSummary_Multiple 验证多次更新累加计数
func TestUpdateConversationLayeredSummary_Multiple(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	insertTestConversation(t, db, "sum-3", "测试")

	for i := 0; i < 3; i++ {
		if err := UpdateConversationLayeredSummary(db, "sum-3", "摘要", ""); err != nil {
			t.Fatalf("第 %d 次更新失败: %v", i, err)
		}
	}

	_, _, count, err := GetConversationLayeredSummary(db, "sum-3")
	if err != nil {
		t.Fatalf("GetConversationLayeredSummary 失败: %v", err)
	}
	if count != 3 {
		t.Errorf("期望计数 3，实际: %d", count)
	}
}

// TestUpdateConversationLayeredSummary_DBFail 验证 DB 失败返回错误
func TestUpdateConversationLayeredSummary_DBFail(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	cleanup()

	err := UpdateConversationLayeredSummary(db, "sum", "short", "")
	if err == nil {
		t.Fatal("期望 DB 关闭后返回错误，实际返回 nil")
	}
}

// TestGetConversationLayeredSummary_NotFound 验证不存在的 ID 返回 NotFound
func TestGetConversationLayeredSummary_NotFound(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	_, _, _, err := GetConversationLayeredSummary(db, "nonexistent")
	if err == nil {
		t.Fatal("期望 NotFound 错误，实际返回 nil")
	}
	if !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("期望 apperror.ErrNotFound，实际: %v", err)
	}
}

// TestGetConversationSummary_Success 验证获取短期摘要
func TestGetConversationSummary_Success(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	insertTestConversation(t, db, "sum-4", "测试")
	if err := UpdateConversationLayeredSummary(db, "sum-4", "摘要内容", ""); err != nil {
		t.Fatalf("更新失败: %v", err)
	}

	summary, err := GetConversationSummary(db, "sum-4")
	if err != nil {
		t.Fatalf("GetConversationSummary 失败: %v", err)
	}
	if summary != "摘要内容" {
		t.Errorf("期望 '摘要内容'，实际: %q", summary)
	}
}

// TestGetConversationSummary_Empty 验证无摘要时返回空字符串
func TestGetConversationSummary_Empty(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	insertTestConversation(t, db, "sum-5", "测试")

	summary, err := GetConversationSummary(db, "sum-5")
	if err != nil {
		t.Fatalf("GetConversationSummary 失败: %v", err)
	}
	if summary != "" {
		t.Errorf("期望空摘要，实际: %q", summary)
	}
}

// TestGetConversationSummary_NotFound 验证不存在的 ID 返回 NotFound
func TestGetConversationSummary_NotFound(t *testing.T) {
	db, _, cleanup := newTestDB(t)
	defer cleanup()

	_, err := GetConversationSummary(db, "nonexistent")
	if err == nil {
		t.Fatal("期望 NotFound 错误，实际返回 nil")
	}
	if !errors.Is(err, apperror.ErrNotFound) {
		t.Errorf("期望 apperror.ErrNotFound，实际: %v", err)
	}
}
