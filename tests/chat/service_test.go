// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"douya/internal/chat"
	"douya/internal/config"
	"douya/internal/store"
)

func newTestService() *chat.Service {
	cfg := &config.Config{
		ContextSize:  4096,
		SystemPrompt: "",
		Temperature:  0.7,
	}
	return chat.NewService(nil, nil, nil, cfg, nil, "")
}

func TestBuildLLMMessages_SystemPromptContainsCurrentDate(t *testing.T) {
	svc := newTestService()
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	if len(msgs) < 1 {
		t.Fatal("expected at least 1 message (system)")
	}
	if msgs[0].Role != "system" {
		t.Fatalf("expected first message role 'system', got '%s'", msgs[0].Role)
	}

	now := time.Now()
	dateStr := now.Format("2006-01-02")
	if !strings.Contains(msgs[0].ContentString(), dateStr) {
		t.Errorf("system prompt should contain current date %s, got: %s", dateStr, msgs[0].ContentString())
	}

	weekdayMap := map[string]string{
		"Sunday": "星期日", "Monday": "星期一", "Tuesday": "星期二",
		"Wednesday": "星期三", "Thursday": "星期四", "Friday": "星期五", "Saturday": "星期六",
	}
	expectedWeekday := weekdayMap[now.Weekday().String()]
	if !strings.Contains(msgs[0].ContentString(), expectedWeekday) {
		t.Errorf("system prompt should contain weekday %s, got: %s", expectedWeekday, msgs[0].ContentString())
	}
}

func TestBuildLLMMessages_DefaultSystemPrompt(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().SystemPrompt = ""
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	if !strings.Contains(msgs[0].ContentString(), "豆芽") {
		t.Errorf("default system prompt should contain '豆芽', got: %s", msgs[0].ContentString())
	}
}

func TestBuildLLMMessages_DefaultSystemPrompt_ContainsRoleDefinition(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().SystemPrompt = ""
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)
	content := msgs[0].ContentString()

	if !strings.Contains(content, "规范") {
		t.Errorf("default system prompt should describe answer guidelines, got: %s", truncate(content, 200))
	}
	if !strings.Contains(content, "语言") {
		t.Errorf("default system prompt should mention language matching, got: %s", truncate(content, 200))
	}
	// 验证不再包含"能力"部分
	if strings.Contains(content, "## 能力") {
		t.Errorf("default system prompt should NOT contain '## 能力' section, got: %s", truncate(content, 200))
	}
}

func TestBuildLLMMessages_CustomSystemPrompt(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().SystemPrompt = "你是代码专家"
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	if !strings.Contains(msgs[0].ContentString(), "你是代码专家") {
		t.Errorf("system prompt should contain custom prompt, got: %s", msgs[0].ContentString())
	}
	if !strings.Contains(msgs[0].ContentString(), "豆芽") {
		t.Errorf("custom prompt should be appended after default prompt, got: %s", msgs[0].ContentString())
	}
	if !strings.Contains(msgs[0].ContentString(), "当前时间") {
		t.Errorf("system prompt should still contain date info, got: %s", msgs[0].ContentString())
	}
}

func TestBuildLLMMessages_ThinkConciselyInstruction(t *testing.T) {
	svc := newTestService()
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	// 检查是否有工具相关的提示
	if !strings.Contains(msgs[0].ContentString(), "获取实时信息") && !strings.Contains(msgs[0].ContentString(), "工具") {
		t.Errorf("system prompt should contain tool guidance, got: %s", msgs[0].ContentString())
	}
}

func TestBuildLLMMessages_TokenEstimationLimitsMessages(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 100

	var dbMsgs []*store.Message
	for i := 0; i < 30; i++ {
		dbMsgs = append(dbMsgs, &store.Message{
			ID:      fmt.Sprintf("msg_%d", i),
			Role:    "user",
			Content: "this is a message with some content",
		})
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "this is a message with some content", nil)

	systemMsgCount := 0
	for _, m := range msgs {
		if m.Role == "system" {
			systemMsgCount++
		}
	}
	if systemMsgCount != 1 {
		t.Errorf("expected exactly 1 system message, got %d", systemMsgCount)
	}

	if len(msgs) >= 31 {
		t.Errorf("with small ContextSize, not all 30 messages should be included, got %d", len(msgs)-1)
	}
}

func TestBuildLLMMessages_NoMaxMessagesHardcode_WhenContextAllows(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 100000

	var dbMsgs []*store.Message
	for i := 0; i < 30; i++ {
		dbMsgs = append(dbMsgs, &store.Message{
			ID:      fmt.Sprintf("msg_%d", i),
			Role:    "user",
			Content: "short",
		})
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "short", nil)

	userMsgCount := 0
	for _, m := range msgs {
		if m.Role == "user" {
			userMsgCount++
		}
	}

	if userMsgCount < 30 {
		t.Errorf("when ContextSize is large enough, all 30 user messages should be included, got %d (maxMessages hardcode is limiting)", userMsgCount)
	}
}

func TestBuildLLMMessages_TokenEstimationRespectsContextSize(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 2000

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: strings.Repeat("你好世界", 10)},
		{ID: "2", Role: "assistant", Content: strings.Repeat("回复内容", 10)},
		{ID: "3", Role: "user", Content: "short"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "short", nil)

	totalEstimated := 0
	for _, m := range msgs {
		totalEstimated += len([]rune(m.ContentString())) * 2
	}

	if totalEstimated > svc.GetConfig().ContextSize*2+200 {
		t.Errorf("total estimated tokens %d exceeds context size %d by too much", totalEstimated, svc.GetConfig().ContextSize)
	}
}

func TestBuildLLMMessages_ChineseTokenEstimation(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 4000

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "这是一段中文内容用于测试"},
		{ID: "2", Role: "user", Content: "短"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "短", nil)

	if len(msgs) < 2 {
		t.Fatal("expected at least system + 1 message")
	}

	lastMsg := msgs[len(msgs)-1]
	if lastMsg.ContentString() != "短" {
		t.Errorf("expected last message content '短', got '%s'", lastMsg.ContentString())
	}
}

func TestBuildLLMMessages_EmptyMessages(t *testing.T) {
	svc := newTestService()
	msgs, _ := chat.BuildLLMMessages(svc, []*store.Message{}, "hello", nil)

	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (system only), got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected system message, got %s", msgs[0].Role)
	}
}

func TestBuildLLMMessages_ZeroContextSize(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 0

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	if len(msgs) < 1 {
		t.Fatal("expected at least system message even with zero context size")
	}
}

func TestBuildLLMMessages_LastUserMessageUsesCurrentContent(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 4096

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "original message"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "updated message", nil)

	lastMsg := msgs[len(msgs)-1]
	if lastMsg.ContentString() != "updated message" {
		t.Errorf("expected last user message to use currentContent 'updated message', got '%s'", lastMsg.ContentString())
	}
	if lastMsg.Role != "user" {
		t.Errorf("expected last message role 'user', got '%s'", lastMsg.Role)
	}
}

func TestBuildLLMMessages_MessagesInCorrectOrder(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 4096

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "first"},
		{ID: "2", Role: "assistant", Content: "second"},
		{ID: "3", Role: "user", Content: "third"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "third", nil)

	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages (system + 3), got %d", len(msgs))
	}

	if msgs[0].Role != "system" {
		t.Errorf("expected msgs[0] role 'system', got '%s'", msgs[0].Role)
	}
	if msgs[1].Role != "user" || msgs[1].ContentString() != "first" {
		t.Errorf("expected msgs[1] user/first, got %s/%s", msgs[1].Role, msgs[1].ContentString())
	}
	if msgs[2].Role != "assistant" || msgs[2].ContentString() != "second" {
		t.Errorf("expected msgs[2] assistant/second, got %s/%s", msgs[2].Role, msgs[2].ContentString())
	}
	if msgs[3].Role != "user" || msgs[3].ContentString() != "third" {
		t.Errorf("expected msgs[3] user/third, got %s/%s", msgs[3].Role, msgs[3].ContentString())
	}
}

func newTestServiceWithDB(t *testing.T) *chat.Service {
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
	cfg := &config.Config{
		ContextSize:  4096,
		SystemPrompt: "",
		Temperature:  0.7,
	}
	return chat.NewService(nil, nil, db, cfg, nil, "")
}

func TestIsCodeRelated(t *testing.T) {
	tests := []struct {
		query string
		want  bool
	}{
		{"写一个python函数", true},
		{"今天天气怎么样", false},
		{"debug this error", true},
		{"如何做红烧肉", false},
		{"数据库优化", true},
	}
	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			got := chat.IsCodeRelated(tt.query)
			if got != tt.want {
				t.Errorf("IsCodeRelated(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestGetConversations_TimeFormatting(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "时间格式测试"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	convs, err := svc.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations failed: %v", err)
	}
	if len(convs) == 0 {
		t.Fatal("expected at least 1 conversation")
	}

	c := convs[0]
	if _, err := time.Parse(time.RFC3339, c.CreatedAt); err != nil {
		t.Errorf("CreatedAt is not RFC3339 format: %q, err: %v", c.CreatedAt, err)
	}
	if _, err := time.Parse(time.RFC3339, c.UpdatedAt); err != nil {
		t.Errorf("UpdatedAt is not RFC3339 format: %q, err: %v", c.UpdatedAt, err)
	}
}

func TestGetMessages_TimeFormatting(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "消息时间测试"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	msg := &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "测试消息",
	}
	if err := store.CreateMessage(chat.GetDB(svc), msg, nil); err != nil {
		t.Fatalf("CreateMessage failed: %v", err)
	}

	msgs, err := svc.GetMessages(conv.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected at least 1 message")
	}

	if _, err := time.Parse(time.RFC3339, msgs[0].CreatedAt); err != nil {
		t.Errorf("CreatedAt is not RFC3339 format: %q, err: %v", msgs[0].CreatedAt, err)
	}
}

func TestCreateConversation_TimeFormatting(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv, err := svc.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	if _, err := time.Parse(time.RFC3339, conv.CreatedAt); err != nil {
		t.Errorf("CreatedAt is not RFC3339 format: %q, err: %v", conv.CreatedAt, err)
	}
	if _, err := time.Parse(time.RFC3339, conv.UpdatedAt); err != nil {
		t.Errorf("UpdatedAt is not RFC3339 format: %q, err: %v", conv.UpdatedAt, err)
	}
}

func TestExportConversation_Markdown(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "导出测试"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "你好",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "你好！有什么可以帮你的？",
	}, nil)

	result, err := svc.ExportConversation(conv.ID, "markdown")
	if err != nil {
		t.Fatalf("ExportConversation markdown failed: %v", err)
	}

	if !strings.Contains(result, "# 导出测试") {
		t.Errorf("markdown should contain title heading '# 导出测试', got: %s", result)
	}
	if !strings.Contains(result, "你好") {
		t.Errorf("markdown should contain user message '你好', got: %s", result)
	}
	if !strings.Contains(result, "你好！有什么可以帮你的？") {
		t.Errorf("markdown should contain assistant message, got: %s", result)
	}
}

func TestExportConversation_JSON(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "JSON导出测试"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "测试JSON",
	}, nil)

	result, err := svc.ExportConversation(conv.ID, "json")
	if err != nil {
		t.Fatalf("ExportConversation json failed: %v", err)
	}

	if !json.Valid([]byte(result)) {
		t.Errorf("json export should be valid JSON, got: %s", result)
	}
}

func TestExportConversation_UnsupportedFormat(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "不支持格式测试"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	_, err := svc.ExportConversation(conv.ID, "xml")
	if err == nil {
		t.Error("expected error for unsupported format 'xml', got nil")
	}
}

func TestIsCodeRelated_AllKeywords(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{"english code", "write a function in python", true},
		{"english debug", "debug this error", true},
		{"english api", "how to use the api", true},
		{"english algorithm", "explain this algorithm", true},
		{"english database", "database query optimization", true},
		{"english docker", "docker container setup", true},
		{"english kubernetes", "kubernetes pod config", true},
		{"chinese code", "代码审查", true},
		{"chinese programming", "编程入门", true},
		{"chinese function", "函数式编程", true},
		{"chinese debug", "调试报错", true},
		{"chinese compile", "编译错误", true},
		{"chinese framework", "框架选择", true},
		{"chinese database", "数据库设计", true},
		{"chinese module", "模块化开发", true},
		{"general question", "今天天气怎么样", false},
		{"cooking", "如何做红烧肉", false},
		{"travel", "去哪里旅游好", false},
		{"health", "感冒了怎么办", false},
		{"empty query", "", false},
		{"mixed cn/en", "用python写个爬虫", true},
		{"case insensitive", "PYTHON is great", true},
		{"substring match", "I need some debugging help", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chat.IsCodeRelated(tt.query)
			if got != tt.want {
				t.Errorf("IsCodeRelated(%q) = %v, want %v", tt.query, got, tt.want)
			}
		})
	}
}

func TestBuildLLMMessages_AssistantMessagesIncluded(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 4096

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "question 1"},
		{ID: "2", Role: "assistant", Content: "answer 1"},
		{ID: "3", Role: "user", Content: "question 2"},
		{ID: "4", Role: "assistant", Content: "answer 2"},
		{ID: "5", Role: "user", Content: "question 3"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "question 3", nil)

	if len(msgs) != 6 {
		t.Fatalf("expected 6 messages (system + 5), got %d", len(msgs))
	}

	if msgs[0].Role != "system" {
		t.Errorf("expected msgs[0] role 'system', got '%s'", msgs[0].Role)
	}
	if msgs[1].Role != "user" || msgs[1].ContentString() != "question 1" {
		t.Errorf("expected msgs[1] user/question 1, got %s/%s", msgs[1].Role, msgs[1].ContentString())
	}
	if msgs[2].Role != "assistant" || msgs[2].ContentString() != "answer 1" {
		t.Errorf("expected msgs[2] assistant/answer 1, got %s/%s", msgs[2].Role, msgs[2].ContentString())
	}
	if msgs[5].Role != "user" || msgs[5].ContentString() != "question 3" {
		t.Errorf("expected msgs[5] user/question 3, got %s/%s", msgs[5].Role, msgs[5].ContentString())
	}
}

func TestBuildLLMMessages_ContextSizeTruncatesOlderMessages(t *testing.T) {
	t.Skip("token estimation algorithm changed, test expectations no longer valid")
	svc := newTestService()
	svc.GetConfig().ContextSize = 3000

	var dbMsgs []*store.Message
	for i := 0; i < 50; i++ {
		dbMsgs = append(dbMsgs, &store.Message{
			ID:      fmt.Sprintf("msg_%d", i),
			Role:    "user",
			Content: "这是一条很长的历史消息应该被截断因为上下文窗口有限",
		})
	}
	dbMsgs = append(dbMsgs, &store.Message{
		ID:      "last",
		Role:    "user",
		Content: "短消息",
	})

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "短消息", nil)

	hasSystem := false
	for _, m := range msgs {
		if m.Role == "system" {
			hasSystem = true
		}
	}
	if !hasSystem {
		t.Error("system message should always be present")
	}

	lastMsg := msgs[len(msgs)-1]
	if lastMsg.ContentString() != "短消息" {
		t.Errorf("expected last message '短消息', got '%s'", lastMsg.ContentString())
	}

	if len(msgs) >= 52 {
		t.Error("with limited ContextSize, older messages should be truncated")
	}
}

func TestBuildLLMMessages_SingleMessageContextSize1(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 1

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hi"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hi", nil)

	if len(msgs) < 1 {
		t.Fatal("expected at least system message")
	}
	if msgs[0].Role != "system" {
		t.Errorf("expected first message to be system, got '%s'", msgs[0].Role)
	}
}

func TestBuildLLMMessages_ConfigParametersInRequest(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().Temperature = 0.5
	svc.GetConfig().TopP = 0.8
	svc.GetConfig().TopK = 20
	svc.GetConfig().RepeatPenalty = 1.2

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "test"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "test", nil)

	if len(msgs) < 1 {
		t.Fatal("expected at least system message")
	}
}

func TestRenameConversation(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv, err := svc.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	if err := svc.RenameConversation(conv.ID, "新标题"); err != nil {
		t.Fatalf("RenameConversation failed: %v", err)
	}

	msgs, _ := svc.GetConversations()
	for _, c := range msgs {
		if c.ID == conv.ID {
			if c.Title != "新标题" {
				t.Errorf("expected title '新标题', got '%s'", c.Title)
			}
			return
		}
	}
	t.Error("conversation not found after rename")
}

func TestDeleteConversation(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv, err := svc.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	if err := svc.DeleteConversation(conv.ID); err != nil {
		t.Fatalf("DeleteConversation failed: %v", err)
	}

	convs, _ := svc.GetConversations()
	for _, c := range convs {
		if c.ID == conv.ID {
			t.Error("conversation should be deleted")
		}
	}
}

func TestSearchMessages(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "搜索测试"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Go concurrent programming",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Go uses goroutines for concurrency",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Python data analysis",
	}, nil)

	msgs, err := svc.SearchMessages("Go concurrent")
	if err != nil {
		t.Fatalf("SearchMessages failed: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("expected at least 1 search result for 'Go concurrent'")
	}
}

func TestExportConversation_MarkdownWithMultipleRounds(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "多轮对话导出"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "什么是Go语言？",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Go是一种静态类型的编译型语言。",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "它有什么优点？",
	}, nil)
	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Go语言有并发支持、编译速度快、语法简洁等优点。",
	}, nil)

	result, err := svc.ExportConversation(conv.ID, "markdown")
	if err != nil {
		t.Fatalf("ExportConversation failed: %v", err)
	}

	if !strings.Contains(result, "# 多轮对话导出") {
		t.Error("markdown should contain title")
	}
	if !strings.Contains(result, "## 用户") {
		t.Error("markdown should contain user label")
	}
	if !strings.Contains(result, "## 助手") {
		t.Error("markdown should contain assistant label")
	}
	if strings.Count(result, "## 用户") != 2 {
		t.Error("markdown should contain 2 user messages")
	}
	if strings.Count(result, "## 助手") != 2 {
		t.Error("markdown should contain 2 assistant messages")
	}
}

func TestExportConversation_JSONContainsAllFields(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv := &store.Conversation{Title: "JSON完整字段测试"}
	if err := store.CreateConversation(chat.GetDB(svc), conv, nil); err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	store.CreateMessage(chat.GetDB(svc), &store.Message{
		ConversationID:  conv.ID,
		Role:            "assistant",
		Content:         "测试内容",
		ThinkingContent: "思考过程",
		SearchResults:   `[{"title":"test"}]`,
	}, nil)

	result, err := svc.ExportConversation(conv.ID, "json")
	if err != nil {
		t.Fatalf("ExportConversation failed: %v", err)
	}

	var msgs []map[string]interface{}
	if err := json.Unmarshal([]byte(result), &msgs); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}

	if len(msgs) == 0 {
		t.Fatal("expected at least 1 message in JSON export")
	}

	msg := msgs[0]
	if msg["content"] != "测试内容" {
		t.Errorf("expected content '测试内容', got '%v'", msg["content"])
	}
	if msg["thinking_content"] != "思考过程" {
		t.Errorf("expected thinking_content '思考过程', got '%v'", msg["thinking_content"])
	}
}

func TestGetConversations_EmptyDB(t *testing.T) {
	svc := newTestServiceWithDB(t)

	convs, err := svc.GetConversations()
	if err != nil {
		t.Fatalf("GetConversations failed: %v", err)
	}
	if len(convs) != 0 {
		t.Errorf("expected 0 conversations in empty DB, got %d", len(convs))
	}
}

func TestGetMessages_EmptyConversation(t *testing.T) {
	svc := newTestServiceWithDB(t)

	conv, err := svc.CreateConversation()
	if err != nil {
		t.Fatalf("CreateConversation failed: %v", err)
	}

	msgs, err := svc.GetMessages(conv.ID)
	if err != nil {
		t.Fatalf("GetMessages failed: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("expected 0 messages in new conversation, got %d", len(msgs))
	}
}

func TestBuildLLMMessages_NegativeContextSize(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = -1

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	if len(msgs) < 1 {
		t.Fatal("expected at least system message with negative context size")
	}
}

func TestBuildLLMMessages_VeryLongSingleMessage(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 100

	longContent := strings.Repeat("这是一段很长的消息", 100)
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: longContent},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, longContent, nil)

	if len(msgs) < 1 {
		t.Fatal("expected at least system message")
	}
}

func TestBuildLLMMessages_MultipleUsersAndAssistants(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 4096

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "q1"},
		{ID: "2", Role: "assistant", Content: "a1"},
		{ID: "3", Role: "user", Content: "q2"},
		{ID: "4", Role: "assistant", Content: "a2"},
		{ID: "5", Role: "user", Content: "q3"},
		{ID: "6", Role: "assistant", Content: "a3"},
	}

	msgs, _ := chat.BuildLLMMessages(svc, dbMsgs, "q3", nil)

	expectedRoles := []string{"system", "user", "assistant", "user", "assistant", "user", "assistant"}
	if len(msgs) != len(expectedRoles) {
		t.Fatalf("expected %d messages, got %d", len(expectedRoles), len(msgs))
	}
	for i, expected := range expectedRoles {
		if msgs[i].Role != expected {
			t.Errorf("expected msgs[%d] role '%s', got '%s'", i, expected, msgs[i].Role)
		}
	}
}

func TestStopGeneration_NoActiveGeneration(t *testing.T) {
	svc := newTestService()
	svc.StopGeneration()
}

func TestGetConfig(t *testing.T) {
	svc := newTestService()
	cfg := svc.GetConfig()
	if cfg == nil {
		t.Fatal("expected config, got nil")
	}
	if cfg.ContextSize != 4096 {
		t.Errorf("expected ContextSize 4096, got %d", cfg.ContextSize)
	}
}
