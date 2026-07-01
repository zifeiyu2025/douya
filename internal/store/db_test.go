package store

import (
	"crypto/rand"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"douya/internal/secrets"
)

// TestSettingReadWrite 测试普通设置的读写
func TestSettingReadWrite(t *testing.T) {
	// 创建临时目录和数据库
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Init(dbPath, nil)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 写入设置
	key := "test_key"
	value := "test_value"
	if err := SetSetting(db, key, value); err != nil {
		t.Fatalf("写入设置失败: %v", err)
	}

	// 读取并验证
	got, err := GetSetting(db, key)
	if err != nil {
		t.Fatalf("读取设置失败: %v", err)
	}
	if got != value {
		t.Errorf("读取到的值不匹配: 期望 %q, 实际 %q", value, got)
	}
}

// TestEncryptedSettingReadWrite 测试加密设置的读写
func TestEncryptedSettingReadWrite(t *testing.T) {
	// 创建临时目录和数据库
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// 生成32字节随机加密密钥
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatalf("生成加密密钥失败: %v", err)
	}

	db, err := Init(dbPath, encKey)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 写入加密设置
	key := "encrypted_key"
	value := "secret_value"
	if err := SetEncryptedSetting(db, key, value, encKey); err != nil {
		t.Fatalf("写入加密设置失败: %v", err)
	}

	// 读取并验证解密后的值
	got, err := GetEncryptedSetting(db, key, encKey)
	if err != nil {
		t.Fatalf("读取加密设置失败: %v", err)
	}
	if got != value {
		t.Errorf("解密后的值不匹配: 期望 %q, 实际 %q", value, got)
	}

	// 验证数据库中存储的值有 "enc:" 前缀
	var rawValue string
	err = db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&rawValue)
	if err != nil {
		t.Fatalf("查询原始值失败: %v", err)
	}
	if len(rawValue) < 4 || rawValue[:4] != "enc:" {
		t.Errorf("数据库中存储的值缺少 'enc:' 前缀, 实际值: %q", rawValue)
	}
}

// TestGetSettingNonExistent 测试读取不存在的设置
func TestGetSettingNonExistent(t *testing.T) {
	// 创建临时目录和数据库
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Init(dbPath, nil)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 读取不存在的设置，应返回空字符串和 nil 错误
	got, err := GetSetting(db, "nonexistent_key")
	if err != nil {
		t.Errorf("读取不存在的设置不应返回错误, 实际错误: %v", err)
	}
	if got != "" {
		t.Errorf("读取不存在的设置应返回空字符串, 实际值: %q", got)
	}
}

// TestGetTableColumns 测试获取表列信息
func TestGetTableColumns(t *testing.T) {
	// 创建临时目录和数据库
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Init(dbPath, nil)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 获取 messages 表的列信息
	columns, err := GetTableColumns(db, "messages")
	if err != nil {
		t.Fatalf("获取表列信息失败: %v", err)
	}

	// 验证 messages 表包含预期的列
	expectedColumns := []string{
		"id", "conversation_id", "role", "content",
		"thinking_content", "search_results", "created_at",
	}
	for _, col := range expectedColumns {
		if !columns[col] {
			t.Errorf("messages 表缺少预期的列: %q", col)
		}
	}

	// 验证不允许查询不在白名单中的表
	_, err = GetTableColumns(db, "settings")
	if err == nil {
		t.Error("查询不在白名单中的表应返回错误, 但返回了 nil")
	}
}

// 测试 secrets.LoadOrCreateKey 也能正常工作
func TestEncryptedSettingWithLoadOrCreateKey(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	keyPath := filepath.Join(tmpDir, "enc.key")

	// 使用 secrets.LoadOrCreateKey 生成密钥
	encKey, err := secrets.LoadOrCreateKey(keyPath)
	if err != nil {
		t.Fatalf("加载或创建密钥失败: %v", err)
	}

	db, err := Init(dbPath, encKey)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 写入并读取加密设置
	key := "api_key"
	value := "sk-1234567890abcdef"
	if err := SetEncryptedSetting(db, key, value, encKey); err != nil {
		t.Fatalf("写入加密设置失败: %v", err)
	}

	got, err := GetEncryptedSetting(db, key, encKey)
	if err != nil {
		t.Fatalf("读取加密设置失败: %v", err)
	}
	if got != value {
		t.Errorf("解密后的值不匹配: 期望 %q, 实际 %q", value, got)
	}
}

// TestEncryptedSettingEmptyValue 测试加密设置写入空值
func TestEncryptedSettingEmptyValue(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatalf("生成加密密钥失败: %v", err)
	}

	db, err := Init(dbPath, encKey)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 写入空值
	key := "empty_key"
	if err := SetEncryptedSetting(db, key, "", encKey); err != nil {
		t.Fatalf("写入空值失败: %v", err)
	}

	// 读取空值
	got, err := GetEncryptedSetting(db, key, encKey)
	if err != nil {
		t.Fatalf("读取空值失败: %v", err)
	}
	if got != "" {
		t.Errorf("空值不匹配: 期望空字符串, 实际 %q", got)
	}

	// 验证数据库中存储的空值没有 "enc:" 前缀
	var rawValue string
	err = db.QueryRow("SELECT value FROM settings WHERE key = ?", key).Scan(&rawValue)
	if err != nil {
		t.Fatalf("查询原始值失败: %v", err)
	}
	if rawValue != "" {
		t.Errorf("空值在数据库中应为空字符串, 实际: %q", rawValue)
	}
}

// TestSettingOverwrite 测试设置的覆盖写入
func TestSettingOverwrite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Init(dbPath, nil)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	key := "overwrite_key"

	// 第一次写入
	if err := SetSetting(db, key, "value1"); err != nil {
		t.Fatalf("第一次写入失败: %v", err)
	}

	// 覆盖写入
	if err := SetSetting(db, key, "value2"); err != nil {
		t.Fatalf("覆盖写入失败: %v", err)
	}

	// 验证读取到的是最新的值
	got, err := GetSetting(db, key)
	if err != nil {
		t.Fatalf("读取设置失败: %v", err)
	}
	if got != "value2" {
		t.Errorf("覆盖后的值不匹配: 期望 %q, 实际 %q", "value2", got)
	}
}

// TestGetTableColumnsDisallowed 测试不允许查询的表
func TestGetTableColumnsDisallowed(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Init(dbPath, nil)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// settings 表不在白名单中，应返回错误
	_, err = GetTableColumns(db, "settings")
	if err == nil {
		t.Error("查询不在白名单中的表应返回错误")
	}
}

// TestMultipleSettings 测试多个设置的读写
func TestMultipleSettings(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := Init(dbPath, nil)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 写入多个设置
	settings := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}
	for k, v := range settings {
		if err := SetSetting(db, k, v); err != nil {
			t.Fatalf("写入设置 %s 失败: %v", k, err)
		}
	}

	// 逐个读取并验证
	for k, expected := range settings {
		got, err := GetSetting(db, k)
		if err != nil {
			t.Fatalf("读取设置 %s 失败: %v", k, err)
		}
		if got != expected {
			t.Errorf("设置 %s 的值不匹配: 期望 %q, 实际 %q", k, expected, got)
		}
	}
}

// TestEncryptedSettingCompatibility 测试加密设置对旧版明文数据的兼容性
func TestEncryptedSettingCompatibility(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatalf("生成加密密钥失败: %v", err)
	}

	db, err := Init(dbPath, encKey)
	if err != nil {
		t.Fatalf("初始化数据库失败: %v", err)
	}
	defer db.Close()

	// 直接写入明文数据（模拟旧版数据）
	key := "legacy_key"
	plainValue := "legacy_plain_value"
	if err := SetSetting(db, key, plainValue); err != nil {
		t.Fatalf("写入明文设置失败: %v", err)
	}

	// 使用 GetEncryptedSetting 读取，应兼容返回明文值
	got, err := GetEncryptedSetting(db, key, encKey)
	if err != nil {
		t.Fatalf("读取旧版明文设置失败: %v", err)
	}
	if got != plainValue {
		t.Errorf("旧版明文值不匹配: 期望 %q, 实际 %q", plainValue, got)
	}
}

// TestMigrateMessages_Batch 测试 migrateMessages 批量事务加密
// 验证：超过 batchSize 的多条未加密消息被分批加密，且全部以 "enc:" 前缀存储
func TestMigrateMessages_Batch(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// 生成 32 字节加密密钥
	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}

	// 用 nil encKey 初始化，避免 Init 时自动触发迁移
	db, err := Init(dbPath, nil)
	if err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	defer db.Close()

	// 插入一个 conversation（messages 表有外键约束）
	_, err = db.Exec("INSERT INTO conversations (id, title, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))", "conv1", "Test")
	if err != nil {
		t.Fatalf("插入会话失败: %v", err)
	}

	// 插入 250 条未加密消息（超过 batchSize=100，覆盖 3 批）
	const total = 250
	for i := range total {
		id := fmt.Sprintf("msg-%03d", i)
		content := fmt.Sprintf("明文消息内容 %d", i)
		if _, err := db.Exec("INSERT INTO messages (id, conversation_id, role, content, created_at) VALUES (?, ?, ?, ?, datetime('now'))",
			id, "conv1", "user", content); err != nil {
			t.Fatalf("插入消息 %d 失败: %v", i, err)
		}
	}

	// 调用 migrateMessages
	if err := migrateMessages(db, encKey); err != nil {
		t.Fatalf("migrateMessages 失败: %v", err)
	}

	// 验证所有消息的 content 都被加密（以 "enc:" 开头）
	rows, err := db.Query("SELECT id, content FROM messages")
	if err != nil {
		t.Fatalf("查询消息失败: %v", err)
	}
	encryptedCount := 0
	for rows.Next() {
		var id, content string
		if err := rows.Scan(&id, &content); err != nil {
			rows.Close()
			t.Fatalf("扫描失败: %v", err)
		}
		if !strings.HasPrefix(content, "enc:") {
			t.Errorf("消息 %s 未被加密: %q", id, content)
		} else {
			encryptedCount++
		}
	}
	rows.Close()
	if encryptedCount != total {
		t.Errorf("期望 %d 条被加密，实际 %d", total, encryptedCount)
	}
}

// TestMigrateMessages_Idempotent 测试 migrateMessages 的幂等性
// 验证：已加密的消息（content 以 "enc:" 开头）不会被重复加密
func TestMigrateMessages_Idempotent(t *testing.T) {
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
	defer db.Close()

	_, err = db.Exec("INSERT INTO conversations (id, title, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))", "conv1", "Test")
	if err != nil {
		t.Fatalf("插入会话失败: %v", err)
	}

	// 插入一条已加密的消息（content 以 "enc:" 开头）
	encryptedContent := "enc:already-encrypted-data"
	if _, err := db.Exec("INSERT INTO messages (id, conversation_id, role, content, created_at) VALUES (?, ?, ?, ?, datetime('now'))",
		"msg-enc", "conv1", "user", encryptedContent); err != nil {
		t.Fatalf("插入已加密消息失败: %v", err)
	}

	// 调用 migrateMessages
	if err := migrateMessages(db, encKey); err != nil {
		t.Fatalf("migrateMessages 失败: %v", err)
	}

	// 验证 content 未被改变（仍然是原来的值）
	var content string
	err = db.QueryRow("SELECT content FROM messages WHERE id = ?", "msg-enc").Scan(&content)
	if err != nil {
		t.Fatalf("查询失败: %v", err)
	}
	if content != encryptedContent {
		t.Errorf("已加密消息不应被重复加密：期望 %q，实际 %q", encryptedContent, content)
	}
}

// TestSearchMessages_CrossField 测试 SearchMessages 跨字段匹配
// 验证：搜索关键词能命中 thinking_content / search_results / tool_calls 字段，而非仅限 content
func TestSearchMessages_CrossField(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}

	db, err := Init(dbPath, encKey)
	if err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO conversations (id, title, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))", "conv1", "Test")
	if err != nil {
		t.Fatalf("插入会话失败: %v", err)
	}

	// 插入 4 条消息：前 3 条 content 不含关键词，但分别在 thinking_content / search_results / tool_calls 中含 "keyword"
	// 第 4 条所有字段都不含关键词（不应被匹配）
	msgs := []*Message{
		{ConversationID: "conv1", Role: "assistant", Content: "普通回答一", ThinkingContent: "我在思考 keyword 这个词"},
		{ConversationID: "conv1", Role: "assistant", Content: "普通回答二", SearchResults: "搜索结果包含 keyword"},
		{ConversationID: "conv1", Role: "assistant", Content: "普通回答三", ToolCalls: "[{\"name\":\"keyword_tool\"}]"},
		{ConversationID: "conv1", Role: "user", Content: "用户消息不含关键词"},
	}
	for i, m := range msgs {
		if err := CreateMessage(db, m, encKey); err != nil {
			t.Fatalf("CreateMessage[%d] 失败: %v", i, err)
		}
	}

	// 搜索 "keyword"，应命中前 3 条
	results, err := SearchMessages(db, "keyword", encKey)
	if err != nil {
		t.Fatalf("SearchMessages 失败: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("期望 3 条跨字段匹配，实际 %d", len(results))
	}

	// 搜索 "普通回答"，应命中 content 字段的 3 条
	results2, err := SearchMessages(db, "普通回答", encKey)
	if err != nil {
		t.Fatalf("SearchMessages(content) 失败: %v", err)
	}
	if len(results2) != 3 {
		t.Fatalf("期望 3 条 content 匹配，实际 %d", len(results2))
	}

	// 搜索不存在的关键词，应返回 0 条
	results3, err := SearchMessages(db, "不存在的关键词xyz", encKey)
	if err != nil {
		t.Fatalf("SearchMessages(none) 失败: %v", err)
	}
	if len(results3) != 0 {
		t.Fatalf("期望 0 条匹配，实际 %d", len(results3))
	}
}

// TestSearchMessages_CaseInsensitive 测试 SearchMessages 大小写不敏感
func TestSearchMessages_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	encKey := make([]byte, 32)
	if _, err := rand.Read(encKey); err != nil {
		t.Fatalf("生成密钥失败: %v", err)
	}

	db, err := Init(dbPath, encKey)
	if err != nil {
		t.Fatalf("Init 失败: %v", err)
	}
	defer db.Close()

	_, err = db.Exec("INSERT INTO conversations (id, title, created_at, updated_at) VALUES (?, ?, datetime('now'), datetime('now'))", "conv1", "Test")
	if err != nil {
		t.Fatalf("插入会话失败: %v", err)
	}

	// content 含大写 KEYWORD，用小写 keyword 搜索应命中
	msg := &Message{
		ConversationID: "conv1",
		Role:           "user",
		Content:        "这里有 KEYWORD 大写",
	}
	if err := CreateMessage(db, msg, encKey); err != nil {
		t.Fatalf("CreateMessage 失败: %v", err)
	}

	results, err := SearchMessages(db, "keyword", encKey)
	if err != nil {
		t.Fatalf("SearchMessages 失败: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("期望大小写不敏感匹配 1 条，实际 %d", len(results))
	}
}
