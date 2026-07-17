// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package store_test

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"douya/internal/store"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")
	db, err := store.Init(dbPath, nil)
	if err != nil {
		t.Fatalf("failed to init test db: %v", err)
	}
	t.Cleanup(func() {
		db.Close()
	})
	return db
}

func TestCreateConversation(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "测试会话"}
	err := store.CreateConversation(db, conv, nil)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	if conv.ID == "" {
		t.Error("expected ID to be set after creation")
	}
	if conv.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set after creation")
	}
	if conv.UpdatedAt.IsZero() {
		t.Error("expected UpdatedAt to be set after creation")
	}
}

func TestGetConversation(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "获取测试"}
	store.CreateConversation(db, conv, nil)

	got, err := store.GetConversation(db, conv.ID, nil)
	if err != nil {
		t.Fatalf("GetConversation failed: %v", err)
	}

	if got.Title != "获取测试" {
		t.Errorf("expected title '获取测试', got '%s'", got.Title)
	}
	if got.ID != conv.ID {
		t.Errorf("expected ID '%s', got '%s'", conv.ID, got.ID)
	}
}

func TestListConversations(t *testing.T) {
	db := openTestDB(t)

	store.CreateConversation(db, &store.Conversation{Title: "会话1"}, nil)
	store.CreateConversation(db, &store.Conversation{Title: "会话2"}, nil)
	store.CreateConversation(db, &store.Conversation{Title: "会话3"}, nil)

	convs, err := store.ListConversations(db, nil)
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}

	if len(convs) != 3 {
		t.Errorf("expected 3 conversations, got %d", len(convs))
	}
}

func TestListConversations_OrderedByUpdatedAt(t *testing.T) {
	db := openTestDB(t)

	conv1 := &store.Conversation{Title: "第一"}
	store.CreateConversation(db, conv1, nil)

	time.Sleep(time.Millisecond * 10)

	conv2 := &store.Conversation{Title: "第二"}
	store.CreateConversation(db, conv2, nil)

	convs, err := store.ListConversations(db, nil)
	if err != nil {
		t.Fatalf("ListConversations failed: %v", err)
	}

	if len(convs) < 2 {
		t.Fatal("expected at least 2 conversations")
	}
	if convs[0].Title != "第二" {
		t.Errorf("expected first conversation '第二', got '%s'", convs[0].Title)
	}
}

func TestUpdateConversation(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "原始标题"}
	store.CreateConversation(db, conv, nil)

	conv.Title = "更新标题"
	err := store.UpdateConversation(db, conv, nil)
	if err != nil {
		t.Fatalf("UpdateConversation failed: %v", err)
	}

	got, _ := store.GetConversation(db, conv.ID, nil)
	if got.Title != "更新标题" {
		t.Errorf("expected title '更新标题', got '%s'", got.Title)
	}
}

func TestDeleteConversation(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "待删除"}
	store.CreateConversation(db, conv, nil)

	err := store.DeleteConversation(db, conv.ID)
	if err != nil {
		t.Fatalf("DeleteConversation failed: %v", err)
	}

	_, err = store.GetConversation(db, conv.ID, nil)
	if err == nil {
		t.Error("expected error after deletion, got nil")
	}
}

func TestDeleteConversation_CascadesMessages(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "级联删除测试"}
	store.CreateConversation(db, conv, nil)

	msg := &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "测试消息",
	}
	store.CreateMessage(db, msg, nil)

	store.DeleteConversation(db, conv.ID)

	msgs, _ := store.GetMessagesByConversation(db, conv.ID, nil)
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages after cascade delete, got %d", len(msgs))
	}
}

func TestCreateMessage(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "消息测试"}
	store.CreateConversation(db, conv, nil)

	msg := &store.Message{
		ConversationID:  conv.ID,
		Role:            "user",
		Content:         "你好世界",
		ThinkingContent: "思考中",
		SearchResults:   `[{"title":"test"}]`,
	}
	err := store.CreateMessage(db, msg, nil)
	if err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}

	if msg.ID == "" {
		t.Error("expected ID to be set after creation")
	}
	if msg.CreatedAt.IsZero() {
		t.Error("expected CreatedAt to be set after creation")
	}
}

func TestGetMessagesByConversation(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "消息列表测试"}
	store.CreateConversation(db, conv, nil)

	store.CreateMessage(db, &store.Message{ConversationID: conv.ID, Role: "user", Content: "第一条"}, nil)
	store.CreateMessage(db, &store.Message{ConversationID: conv.ID, Role: "assistant", Content: "第二条"}, nil)
	store.CreateMessage(db, &store.Message{ConversationID: conv.ID, Role: "user", Content: "第三条"}, nil)

	msgs, err := store.GetMessagesByConversation(db, conv.ID, nil)
	if err != nil {
		t.Fatalf("GetMessagesByConversation failed: %v", err)
	}

	if len(msgs) != 3 {
		t.Errorf("expected 3 messages, got %d", len(msgs))
	}

	if msgs[0].Content != "第一条" {
		t.Errorf("expected first message '第一条', got '%s'", msgs[0].Content)
	}
	if msgs[2].Content != "第三条" {
		t.Errorf("expected last message '第三条', got '%s'", msgs[2].Content)
	}
}

func TestGetMessagesByConversation_Order(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "排序测试"}
	store.CreateConversation(db, conv, nil)

	msg1 := &store.Message{ConversationID: conv.ID, Role: "user", Content: "早"}
	store.CreateMessage(db, msg1, nil)

	time.Sleep(time.Millisecond * 10)

	msg2 := &store.Message{ConversationID: conv.ID, Role: "assistant", Content: "晚"}
	store.CreateMessage(db, msg2, nil)

	msgs, _ := store.GetMessagesByConversation(db, conv.ID, nil)

	if len(msgs) < 2 {
		t.Fatal("expected at least 2 messages")
	}
	if msgs[0].Content != "早" {
		t.Errorf("expected first message '早', got '%s'", msgs[0].Content)
	}
	if msgs[1].Content != "晚" {
		t.Errorf("expected second message '晚', got '%s'", msgs[1].Content)
	}
}

func TestChineseContent(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "中文内容测试 🎉"}
	store.CreateConversation(db, conv, nil)

	chineseContent := "这是一段中文内容，包含特殊字符：<>&\"' 以及 emoji 🚀"
	msg := &store.Message{
		ConversationID:  conv.ID,
		Role:            "user",
		Content:         chineseContent,
		ThinkingContent: "思考中文内容",
	}
	store.CreateMessage(db, msg, nil)

	msgs, _ := store.GetMessagesByConversation(db, conv.ID, nil)
	if len(msgs) == 0 {
		t.Fatal("expected at least 1 message")
	}

	if msgs[0].Content != chineseContent {
		t.Errorf("expected content '%s', got '%s'", chineseContent, msgs[0].Content)
	}
	if msgs[0].ThinkingContent != "思考中文内容" {
		t.Errorf("expected thinking_content '思考中文内容', got '%s'", msgs[0].ThinkingContent)
	}
}

func TestConversationTitleWithChinese(t *testing.T) {
	db := openTestDB(t)

	title := "中文标题测试 🎊"
	conv := &store.Conversation{Title: title}
	store.CreateConversation(db, conv, nil)

	got, _ := store.GetConversation(db, conv.ID, nil)
	if got.Title != title {
		t.Errorf("expected title '%s', got '%s'", title, got.Title)
	}
}

func TestSearchMessages_BasicSearch(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "搜索测试"}
	store.CreateConversation(db, conv, nil)

	store.CreateMessage(db, &store.Message{ConversationID: conv.ID, Role: "user", Content: "Go programming language"}, nil)
	store.CreateMessage(db, &store.Message{ConversationID: conv.ID, Role: "user", Content: "Python data analysis"}, nil)

	msgs, err := store.SearchMessages(db, "Go programming", nil)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}

	if len(msgs) == 0 {
		t.Error("expected at least 1 search result for 'Go programming'")
	}
}

func TestCreateConversation_WithPresetID(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{ID: "custom-id-123", Title: "自定义ID"}
	err := store.CreateConversation(db, conv, nil)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}
	if conv.ID != "custom-id-123" {
		t.Errorf("expected ID to remain 'custom-id-123', got '%s'", conv.ID)
	}
}

func TestCreateConversation_WithPresetTimestamps(t *testing.T) {
	db := openTestDB(t)

	presetTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	conv := &store.Conversation{Title: "预设时间", CreatedAt: presetTime, UpdatedAt: presetTime}
	err := store.CreateConversation(db, conv, nil)
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	got, _ := store.GetConversation(db, conv.ID, nil)
	if !got.CreatedAt.Equal(presetTime) {
		t.Errorf("expected CreatedAt %v, got %v", presetTime, got.CreatedAt)
	}
}

func TestGetConversation_NotFound(t *testing.T) {
	db := openTestDB(t)

	_, err := store.GetConversation(db, "nonexistent-id", nil)
	if err == nil {
		t.Error("expected error for nonexistent conversation, got nil")
	}
}

func TestUpdateConversation_UpdatesTimestamp(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "时间更新测试"}
	store.CreateConversation(db, conv, nil)
	originalUpdatedAt := conv.UpdatedAt

	time.Sleep(time.Millisecond * 10)

	conv.Title = "更新后"
	store.UpdateConversation(db, conv, nil)

	got, _ := store.GetConversation(db, conv.ID, nil)
	if !got.UpdatedAt.After(originalUpdatedAt) {
		t.Errorf("expected UpdatedAt to be after original, got original=%v, new=%v", originalUpdatedAt, got.UpdatedAt)
	}
}

func TestDeleteConversation_Nonexistent(t *testing.T) {
	db := openTestDB(t)

	err := store.DeleteConversation(db, "nonexistent-id")
	if err != nil {
		t.Errorf("deleting nonexistent conversation should not error, got: %v", err)
	}
}

func TestCreateMessage_WithPresetID(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "消息ID测试"}
	store.CreateConversation(db, conv, nil)

	msg := &store.Message{
		ID:             "custom-msg-id",
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "test",
	}
	err := store.CreateMessage(db, msg, nil)
	if err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}
	if msg.ID != "custom-msg-id" {
		t.Errorf("expected ID 'custom-msg-id', got '%s'", msg.ID)
	}
}

func TestCreateMessage_WithAllFields(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "完整字段测试"}
	store.CreateConversation(db, conv, nil)

	msg := &store.Message{
		ConversationID:  conv.ID,
		Role:            "assistant",
		Content:         "完整回复内容",
		ThinkingContent: "这是思考过程",
		SearchResults:   `[{"title":"补充信息","url":"http://example.com","snippet":"摘要"}]`,
	}
	err := store.CreateMessage(db, msg, nil)
	if err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}

	msgs, _ := store.GetMessagesByConversation(db, conv.ID, nil)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}

	got := msgs[0]
	if got.Content != "完整回复内容" {
		t.Errorf("expected content '完整回复内容', got '%s'", got.Content)
	}
	if got.ThinkingContent != "这是思考过程" {
		t.Errorf("expected thinking_content '这是思考过程', got '%s'", got.ThinkingContent)
	}
	if got.SearchResults != `[{"title":"补充信息","url":"http://example.com","snippet":"摘要"}]` {
		t.Errorf("expected search_results, got '%s'", got.SearchResults)
	}
	if got.Role != "assistant" {
		t.Errorf("expected role 'assistant', got '%s'", got.Role)
	}
}

func TestGetMessagesByConversation_EmptyConversation(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "空会话"}
	store.CreateConversation(db, conv, nil)

	msgs, err := store.GetMessagesByConversation(db, conv.ID, nil)
	if err != nil {
		t.Fatalf("GetMessagesByConversation failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages, got %d", len(msgs))
	}
}

func TestGetMessagesByConversation_NonexistentConversation(t *testing.T) {
	db := openTestDB(t)

	msgs, err := store.GetMessagesByConversation(db, "nonexistent-id", nil)
	if err != nil {
		t.Fatalf("GetMessagesByConversation failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages for nonexistent conversation, got %d", len(msgs))
	}
}

func TestSearchMessages_NoResults(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "搜索无结果测试"}
	store.CreateConversation(db, conv, nil)

	store.CreateMessage(db, &store.Message{ConversationID: conv.ID, Role: "user", Content: "Go语言编程"}, nil)

	msgs, err := store.SearchMessages(db, "Rust语言", nil)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 results for 'Rust语言', got %d", len(msgs))
	}
}

func TestSearchMessages_SpecialCharacters(t *testing.T) {
	db := openTestDB(t)

	conv := &store.Conversation{Title: "特殊字符搜索"}
	store.CreateConversation(db, conv, nil)

	store.CreateMessage(db, &store.Message{ConversationID: conv.ID, Role: "user", Content: "test <script>alert('xss')</script>"}, nil)

	msgs, err := store.SearchMessages(db, "script", nil)
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("expected at least 1 result for 'script'")
	}
}

func TestMultipleConversationsIsolation(t *testing.T) {
	db := openTestDB(t)

	conv1 := &store.Conversation{Title: "会话1"}
	store.CreateConversation(db, conv1, nil)
	conv2 := &store.Conversation{Title: "会话2"}
	store.CreateConversation(db, conv2, nil)

	store.CreateMessage(db, &store.Message{ConversationID: conv1.ID, Role: "user", Content: "会话1消息"}, nil)
	store.CreateMessage(db, &store.Message{ConversationID: conv2.ID, Role: "user", Content: "会话2消息"}, nil)

	msgs1, _ := store.GetMessagesByConversation(db, conv1.ID, nil)
	msgs2, _ := store.GetMessagesByConversation(db, conv2.ID, nil)

	if len(msgs1) != 1 {
		t.Errorf("expected 1 message in conv1, got %d", len(msgs1))
	}
	if len(msgs2) != 1 {
		t.Errorf("expected 1 message in conv2, got %d", len(msgs2))
	}
	if msgs1[0].Content != "会话1消息" {
		t.Errorf("expected '会话1消息', got '%s'", msgs1[0].Content)
	}
	if msgs2[0].Content != "会话2消息" {
		t.Errorf("expected '会话2消息', got '%s'", msgs2[0].Content)
	}
}

func TestDeleteConversation_OnlyDeletesOwnMessages(t *testing.T) {
	db := openTestDB(t)

	conv1 := &store.Conversation{Title: "保留会话"}
	store.CreateConversation(db, conv1, nil)
	conv2 := &store.Conversation{Title: "删除会话"}
	store.CreateConversation(db, conv2, nil)

	store.CreateMessage(db, &store.Message{ConversationID: conv1.ID, Role: "user", Content: "保留消息"}, nil)
	store.CreateMessage(db, &store.Message{ConversationID: conv2.ID, Role: "user", Content: "删除消息"}, nil)

	store.DeleteConversation(db, conv2.ID)

	msgs1, _ := store.GetMessagesByConversation(db, conv1.ID, nil)
	if len(msgs1) != 1 {
		t.Errorf("expected conv1 messages to remain, got %d", len(msgs1))
	}
}

func TestListConversations_MultipleUpdates(t *testing.T) {
	db := openTestDB(t)

	conv1 := &store.Conversation{Title: "第一"}
	store.CreateConversation(db, conv1, nil)

	time.Sleep(time.Millisecond * 10)

	conv2 := &store.Conversation{Title: "第二"}
	store.CreateConversation(db, conv2, nil)

	time.Sleep(time.Millisecond * 10)

	conv1.Title = "更新第一"
	store.UpdateConversation(db, conv1, nil)

	convs, _ := store.ListConversations(db, nil)
	if len(convs) < 2 {
		t.Fatal("expected at least 2 conversations")
	}
	if convs[0].ID != conv1.ID {
		t.Errorf("expected first conversation to be conv1 (recently updated), got '%s'", convs[0].Title)
	}
}

func TestInit_CreatesDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "subdir", "test.db")

	db, err := store.Init(dbPath, nil)
	if err != nil {
		t.Fatalf("Init failed to create directory: %v", err)
	}
	db.Close()
}

func TestMigrate_AddsMissingColumns(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`
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
			created_at DATETIME,
			FOREIGN KEY (conversation_id) REFERENCES conversations(id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_messages_conversation_id ON messages(conversation_id);
	`)
	if err != nil {
		t.Fatalf("create old schema: %v", err)
	}

	_, err = db.Exec("INSERT INTO conversations (id, title, created_at, updated_at) VALUES ('conv1', 'test', '2024-01-01', '2024-01-01')")
	if err != nil {
		t.Fatalf("insert conversation: %v", err)
	}
	_, err = db.Exec("INSERT INTO messages (id, conversation_id, role, content, thinking_content, search_results, created_at) VALUES ('msg1', 'conv1', 'user', 'hello', '', '', '2024-01-01')")
	if err != nil {
		t.Fatalf("insert message: %v", err)
	}

	if err := store.Migrate(db, nil); err != nil {
		t.Fatalf("migrate with missing columns: %v", err)
	}

	columns, err := store.GetTableColumns(db, "messages")
	if err != nil {
		t.Fatalf("GetTableColumns: %v", err)
	}

	if !columns["tool_calls"] {
		t.Error("expected tool_calls column to exist after migration")
	}
	if !columns["tool_call_id"] {
		t.Error("expected tool_call_id column to exist after migration")
	}

	_, err = db.Exec("INSERT INTO messages (id, conversation_id, role, content, thinking_content, search_results, tool_calls, tool_call_id, created_at) VALUES ('msg2', 'conv1', 'tool', 'result', '', '', '[]', 'call_1', '2024-01-01')")
	if err != nil {
		t.Fatalf("insert message with tool_calls after migration: %v", err)
	}

	rows, err := db.Query("SELECT tool_calls, tool_call_id FROM messages WHERE id = 'msg2'")
	if err != nil {
		t.Fatalf("select tool_calls after migration: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected row")
	}
	var toolCalls, toolCallID string
	if err := rows.Scan(&toolCalls, &toolCallID); err != nil {
		t.Fatalf("scan tool_calls: %v", err)
	}
	if toolCalls != "[]" {
		t.Errorf("expected tool_calls '[]', got '%s'", toolCalls)
	}
	if toolCallID != "call_1" {
		t.Errorf("expected tool_call_id 'call_1', got '%s'", toolCallID)
	}
}

func TestMigrate_Idempotent(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := store.Init(dbPath, nil)
	if err != nil {
		t.Fatalf("first Init: %v", err)
	}
	db.Close()

	db, err = store.Init(dbPath, nil)
	if err != nil {
		t.Fatalf("second Init (idempotent): %v", err)
	}
	db.Close()
}
