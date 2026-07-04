// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"douya/internal/chat"
	"douya/internal/llm"
	"douya/internal/search"
	"douya/internal/store"
)

func TestClampDuration_Normal(t *testing.T) {
	if chat.ClampDuration(5.5) != 5.5 {
		t.Errorf("expected 5.5, got %f", chat.ClampDuration(5.5))
	}
}

func TestClampDuration_Negative(t *testing.T) {
	if chat.ClampDuration(-1) != 0 {
		t.Errorf("expected 0 for negative, got %f", chat.ClampDuration(-1))
	}
}

func TestClampDuration_ExceedsMax(t *testing.T) {
	if chat.ClampDuration(5000) != 0 {
		t.Errorf("expected 0 for >3600, got %f", chat.ClampDuration(5000))
	}
}

func TestClampDuration_Zero(t *testing.T) {
	if chat.ClampDuration(0) != 0 {
		t.Errorf("expected 0, got %f", chat.ClampDuration(0))
	}
}

func TestClampDuration_Boundary3600(t *testing.T) {
	if chat.ClampDuration(3600) != 3600 {
		t.Errorf("expected 3600 at boundary, got %f", chat.ClampDuration(3600))
	}
}

func TestClampDuration_JustOver3600(t *testing.T) {
	if chat.ClampDuration(3600.1) != 0 {
		t.Errorf("expected 0 for >3600, got %f", chat.ClampDuration(3600.1))
	}
}

func TestResetForNextCall_ClearsAllFields(t *testing.T) {
	acc := chat.NewStreamAccumulator("", func(string, any) {}, func(string, string, any) {})
	acc.FullContent.WriteString("hello")
	acc.FullThinking.WriteString("think")
	acc.FinishReason = "stop"
	acc.ToolCallMap[0] = &llm.ToolCall{Index: 0, ID: "tc1"}
	acc.PendingBytes = "pen"
	acc.PendingThink = "pt"
	acc.LastSearchJSON = `{"results":[]}`
	acc.ThinkingStartTime = time.Now()
	acc.ThinkingDuration = 42.5
	acc.ThinkingDone = true

	chat.ResetForNextCall(acc)

	if acc.FullContent.String() != "" {
		t.Errorf("fullContent not reset, got %q", acc.FullContent.String())
	}
	if acc.FullThinking.String() != "think" {
		t.Errorf("fullThinking should NOT be reset (it accumulates across calls), got %q", acc.FullThinking.String())
	}
	if acc.FinishReason != "" {
		t.Errorf("finishReason not reset, got %q", acc.FinishReason)
	}
	if len(acc.ToolCallMap) != 0 {
		t.Errorf("toolCallMap not reset, got %d entries", len(acc.ToolCallMap))
	}
	if acc.PendingBytes != "" {
		t.Errorf("pendingBytes not reset, got %q", acc.PendingBytes)
	}
	if acc.PendingThink != "" {
		t.Errorf("pendingThink not reset, got %q", acc.PendingThink)
	}
	if acc.LastSearchJSON != `{"results":[]}` {
		t.Errorf("lastSearchJSON should NOT be reset (it persists across calls), got %q", acc.LastSearchJSON)
	}
	if !acc.ThinkingStartTime.IsZero() {
		t.Errorf("thinkingStartTime not reset, got %v", acc.ThinkingStartTime)
	}
	if acc.ThinkingDuration != 0 {
		t.Errorf("thinkingDuration not reset, got %f", acc.ThinkingDuration)
	}
	if acc.ThinkingDone {
		t.Errorf("thinkingDone not reset, got %v", acc.ThinkingDone)
	}
}

func TestResetForNextCall_PreservesFirstRoundThinking(t *testing.T) {
	acc := chat.NewStreamAccumulator("", func(string, any) {}, func(string, string, any) {})
	acc.FullThinking.WriteString("first round deep thought")
	acc.ThinkingDuration = 7.5

	chat.ResetForNextCall(acc)

	if chat.GetFirstRoundThinking(acc) != "first round deep thought" {
		t.Errorf("FirstRoundThinking should be %q, got %q", "first round deep thought", chat.GetFirstRoundThinking(acc))
	}
	if acc.FirstRoundThinkingDuration != 7.5 {
		t.Errorf("FirstRoundThinkingDuration should be 7.5, got %f", acc.FirstRoundThinkingDuration)
	}
}

func TestResetForNextCall_EmptyThinkingDoesNotOverwriteFirstRound(t *testing.T) {
	acc := chat.NewStreamAccumulator("", func(string, any) {}, func(string, string, any) {})
	acc.FirstRoundThinking = "original thinking"
	acc.FirstRoundThinkingDuration = 3.0

	chat.ResetForNextCall(acc)

	if chat.GetFirstRoundThinking(acc) != "original thinking" {
		t.Errorf("FirstRoundThinking should remain %q when FullThinking is empty, got %q", "original thinking", chat.GetFirstRoundThinking(acc))
	}
	if acc.FirstRoundThinkingDuration != 3.0 {
		t.Errorf("FirstRoundThinkingDuration should remain 3.0 when FullThinking is empty, got %f", acc.FirstRoundThinkingDuration)
	}
}

func TestStreamAccumulator_ThinkingDuration_ZeroStartTimeNoPanic(t *testing.T) {
	acc := chat.NewStreamAccumulator("", func(string, any) {}, func(string, string, any) {})

	if !acc.ThinkingStartTime.IsZero() {
		t.Fatal("thinkingStartTime should be zero initially")
	}
	if acc.ThinkingDuration != 0 {
		t.Errorf("thinkingDuration should be 0 with zero startTime, got %f", acc.ThinkingDuration)
	}
}

func TestStoreMsgToChat_ThinkingDurationAndImages(t *testing.T) {
	now := time.Now()
	storeMsg := &store.Message{
		ID:               "msg-1",
		ConversationID:   "conv-1",
		Role:             "assistant",
		Content:          "hello",
		ThinkingContent:  "thinking",
		ThinkingDuration: 12.5,
		SearchResults:    `{"results":[]}`,
		Images:           `["img1.png"]`,
		CreatedAt:        now,
	}

	chatMsg := chat.StoreMsgToChat(storeMsg)

	if chatMsg.ThinkingDuration != 12.5 {
		t.Errorf("ThinkingDuration not mapped, got %f", chatMsg.ThinkingDuration)
	}
	if chatMsg.Images != `["img1.png"]` {
		t.Errorf("Images not mapped, got %q", chatMsg.Images)
	}
}

func TestCalcMaxTokens_DefaultContextSize(t *testing.T) {
	svc := newTestService()
	// 默认 ctxSize=4096, promptTokens=0 → maxTokens=4096 (capped to 16384)
	result := chat.CalcMaxTokens(svc, 0)
	if result != 4096 {
		t.Errorf("expected 4096 (4096-0), got %d", result)
	}
}

func TestCalcMaxTokens_LargeContextSize(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 65536
	// ctxSize=65536, promptTokens=0 → 65536, capped to 16384
	result := chat.CalcMaxTokens(svc, 0)
	if result != 16384 {
		t.Errorf("expected 16384 (capped), got %d", result)
	}
}

func TestCalcMaxTokens_SmallContextSize(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 600
	// ctxSize=600, promptTokens=0 → 600
	result := chat.CalcMaxTokens(svc, 0)
	if result != 600 {
		t.Errorf("expected 600 (600-0), got %d", result)
	}
}

func TestCalcMaxTokens_ZeroContextSize(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 0
	// default ctxSize=4096, promptTokens=0 → 4096
	result := chat.CalcMaxTokens(svc, 0)
	if result != 4096 {
		t.Errorf("expected 4096 (default 4096-0), got %d", result)
	}
}

func TestCalcMaxTokens_WithPromptTokens(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 8192
	// ctxSize=8192, promptTokens=6000 → 2192
	result := chat.CalcMaxTokens(svc, 6000)
	if result != 2192 {
		t.Errorf("expected 2192 (8192-6000), got %d", result)
	}
}

func TestStopGeneration_NilCancelIsSafe(t *testing.T) {
	svc := newTestService()
	chat.SetCurrentCancel(svc, nil)
	svc.StopGeneration()
}

func TestStopGeneration_CancelIsCalled(t *testing.T) {
	svc := newTestService()
	cancelled := false
	chat.SetCurrentCancel(svc, func() { cancelled = true })
	svc.StopGeneration()
	if !cancelled {
		t.Error("expected cancel to be called")
	}
}

func TestFormatSearchResults_Format(t *testing.T) {
	results := []search.SearchResult{
		{Title: "AI技术突破", URL: "https://example.com/ai", Snippet: "2026年AI技术最新突破"},
		{Title: "AI趋势报告", URL: "https://example.com/trends", Snippet: "多模态和Agent是主要趋势"},
	}
	out := chat.FormatSearchResults(results)
	if !strings.Contains(out, "AI技术突破") {
		t.Errorf("should contain 'AI技术突破', got: %s", out)
	}
	// URL 被有意移除：url 对回答内容无帮助，仅占 token（见 service_messages.go formatSearchResultsWithLang）
	if strings.Contains(out, "https://example.com/ai") {
		t.Errorf("should NOT contain URL (removed to save tokens), got: %s", out)
	}
	if !strings.Contains(out, "2026年AI技术最新突破") {
		t.Errorf("should contain '2026年AI技术最新突破', got: %s", out)
	}
	if strings.Contains(out, "Search results for") {
		t.Errorf("should NOT contain old format 'Search results for', got: %s", out)
	}
	if !strings.Contains(out, "<search_results>") {
		t.Errorf("should contain XML tag <search_results>, got: %s", out)
	}
	if !strings.Contains(out, "<result>") {
		t.Errorf("should contain XML tag <result>, got: %s", out)
	}
}

func TestFormatSearchResults_Empty(t *testing.T) {
	out := chat.FormatSearchResults(nil)
	if out != "<search_results>\n</search_results>" {
		t.Errorf("expected XML wrapper for nil results, got: %q", out)
	}
	out = chat.FormatSearchResults([]search.SearchResult{})
	if out != "<search_results>\n</search_results>" {
		t.Errorf("expected XML wrapper for empty results, got: %q", out)
	}
}

func TestTruncateSearchContext_ShortContext(t *testing.T) {
	short := "hello world"
	result := chat.TruncateSearchContext(short, 4096)
	if result != short {
		t.Errorf("short context should not be truncated, got: %q", result)
	}
}

func TestTruncateSearchContext_LongContext(t *testing.T) {
	long := strings.Repeat("a", 5000)
	result := chat.TruncateSearchContext(long, 4096)
	if result == long {
		t.Error("long context should be truncated")
	}
	if !strings.HasSuffix(result, "\n...") {
		t.Errorf("truncated context should end with '\\n...', got suffix: %q", result[len(result)-10:])
	}
}

func TestTruncateSearchContext_ZeroCtxSize(t *testing.T) {
	short := "hello"
	result := chat.TruncateSearchContext(short, 0)
	if result != short {
		t.Errorf("short context with zero ctxSize should not be truncated, got: %q", result)
	}
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"chinese", "今天天气怎么样", "zh"},
		{"english", "What is the weather today?", "en"},
		{"mixed", "用python写个爬虫", "zh"},
		{"empty", "", "en"},
		{"numbers only", "12345", "en"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chat.DetectLanguage(tt.input)
			if got != tt.want {
				t.Errorf("DetectLanguage(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSearchResultInstruction(t *testing.T) {
	zh := chat.SearchResultInstruction("zh")
	// 防幻觉措辞：应包含"仅基于以上信息"，不应包含"用你自己的话"
	if !strings.Contains(zh, "仅基于以上信息") {
		t.Errorf("Chinese instruction should contain '仅基于以上信息', got: %s", zh)
	}
	if strings.Contains(zh, "用你自己的话") {
		t.Errorf("Chinese instruction should not contain '用你自己的话' (anti-hallucination), got: %s", zh)
	}
	en := chat.SearchResultInstruction("en")
	if !strings.Contains(en, "strictly") && !strings.Contains(en, "based on") {
		t.Errorf("English instruction should contain 'based strictly on', got: %s", en)
	}
}

func TestInjectSearchContext_AppendsToLastUserMessage(t *testing.T) {
	messages := []llm.ChatMessage{
		llm.NewTextMessage("system", "you are helpful"),
		llm.NewTextMessage("user", "what is AI?"),
	}

	result := chat.InjectSearchContext(messages, "[1] AI article\nhttps://example.com\n", "自然融入你的回答中")

	lastMsg := result[len(result)-1]
	if lastMsg.Role != "user" {
		t.Fatalf("expected last message role 'user', got '%s'", lastMsg.Role)
	}
	if !strings.Contains(lastMsg.ContentString(), "what is AI?") {
		t.Errorf("last user message should still contain original question, got: %s", lastMsg.ContentString())
	}
	if !strings.Contains(lastMsg.ContentString(), "[1] AI article") {
		t.Errorf("last user message should contain search context, got: %s", lastMsg.ContentString())
	}
	if !strings.Contains(lastMsg.ContentString(), "自然融入你的回答中") {
		t.Errorf("last user message should contain instruction, got: %s", lastMsg.ContentString())
	}
}

func TestInjectSearchContext_DoesNotAddExtraMessages(t *testing.T) {
	messages := []llm.ChatMessage{
		llm.NewTextMessage("system", "you are helpful"),
		llm.NewTextMessage("user", "what is AI?"),
	}

	result := chat.InjectSearchContext(messages, "search results", "instruction")

	if len(result) != len(messages) {
		t.Errorf("expected %d messages (same count), got %d - should modify last user message, not add new ones", len(messages), len(result))
	}
}

func TestInjectSearchContext_WithHistory(t *testing.T) {
	messages := []llm.ChatMessage{
		llm.NewTextMessage("system", "you are helpful"),
		llm.NewTextMessage("user", "hello"),
		llm.NewTextMessage("assistant", "hi there"),
		llm.NewTextMessage("user", "what is AI?"),
	}

	result := chat.InjectSearchContext(messages, "search results", "instruction")

	if len(result) != 4 {
		t.Errorf("expected 4 messages, got %d", len(result))
	}

	lastMsg := result[len(result)-1]
	if lastMsg.Role != "user" {
		t.Fatalf("expected last message role 'user', got '%s'", lastMsg.Role)
	}
	if !strings.Contains(lastMsg.ContentString(), "what is AI?") {
		t.Errorf("should contain original question, got: %s", lastMsg.ContentString())
	}
	if !strings.Contains(lastMsg.ContentString(), "search results") {
		t.Errorf("should contain search results, got: %s", lastMsg.ContentString())
	}
}

func TestInjectSearchContext_NoUserMessage_NoPanic(t *testing.T) {
	messages := []llm.ChatMessage{
		llm.NewTextMessage("system", "you are helpful"),
	}

	result := chat.InjectSearchContext(messages, "search results", "instruction")

	if len(result) != 2 {
		t.Errorf("expected 2 messages (system + new user), got %d", len(result))
	}
	if result[1].Role != "user" {
		t.Errorf("expected new message role 'user', got '%s'", result[1].Role)
	}
}

func TestBuildLLMMessages_SearchEnabled_NoSearchToolInstruction(t *testing.T) {
	svc := newTestService()
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessagesWithSearch(svc, dbMsgs, "hello", nil, "on")

	if strings.Contains(msgs[0].ContentString(), "search工具") || strings.Contains(msgs[0].ContentString(), "内置工具") {
		t.Errorf("when searchEnabled=true, system prompt should NOT mention search tool, got: %s", msgs[0].ContentString())
	}
}

func TestBuildLLMMessages_SearchDisabled_HasSearchToolInstruction(t *testing.T) {
	svc := newTestService()
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "hello"},
	}
	msgs, _ := chat.BuildLLMMessagesWithSearch(svc, dbMsgs, "hello", nil, "off")

	// SearchMode 为 off 时，系统提示词不应包含搜索工具说明
	if strings.Contains(msgs[0].ContentString(), "search 工具") {
		t.Errorf("when SearchMode=off, system prompt should NOT contain search tool guidance, got: %s", msgs[0].ContentString())
	}
}

func TestSendMessage_ImageAttachment_NoDuplicate(t *testing.T) {
	// 需要视觉模型，跳过
	t.Skip("needs vision model")
	var receivedReq *llm.ChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		receivedReq = &req

		sseData := makeSSEData([]string{
			makeContentChunk("这是一张图片"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	imageData := "data:image/png;base64,iVBORw0KGgo="

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:    "描述这张图片",
		SearchMode: "off",
		Images:     []string{imageData},
		Attachments: []chat.Attachment{
			{Type: "image", Name: "test.png", MimeType: "image/png", Data: imageData},
		},
	})
	if err != nil {
		t.Fatalf("SendMessage with image attachment failed: %v", err)
	}

	if receivedReq == nil {
		t.Fatal("expected LLM request")
	}

	lastMsg := receivedReq.Messages[len(receivedReq.Messages)-1]
	if lastMsg.Role != "user" {
		t.Fatalf("expected last message role 'user', got '%s'", lastMsg.Role)
	}

	contentParts, ok := lastMsg.Content.([]any)
	if !ok {
		t.Fatalf("expected content to be []interface{} for image message, got %T", lastMsg.Content)
	}

	imageCount := 0
	for _, part := range contentParts {
		if partMap, ok := part.(map[string]any); ok {
			if partMap["type"] == "image_url" {
				imageCount++
			}
		}
	}

	if imageCount != 1 {
		t.Errorf("expected exactly 1 image_url in message, got %d (duplicate bug)", imageCount)
	}
}

func TestSendMessage_PDFAttachment_SentAsDataURL(t *testing.T) {
	var receivedReq *llm.ChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		receivedReq = &req

		sseData := makeSSEData([]string{
			makeContentChunk("这是一个PDF"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	pdfBase64 := "JVBERi0xLjQKMSAwIG9iago8PAovVHlwZSAvQ2F0YWxvZwovUGFnZXMgMiAwIFIKPj4KZW5kb2Jq"

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:    "总结这个PDF",
		SearchMode: "off",
		Attachments: []chat.Attachment{
			{Type: "pdf", Name: "test.pdf", MimeType: "application/pdf", Data: pdfBase64},
		},
	})
	if err != nil {
		t.Fatalf("SendMessage with PDF attachment failed: %v", err)
	}

	if receivedReq == nil {
		t.Fatal("expected LLM request")
	}

	lastMsg := receivedReq.Messages[len(receivedReq.Messages)-1]
	if lastMsg.Role != "user" {
		t.Fatalf("expected last message role 'user', got '%s'", lastMsg.Role)
	}

	contentStr := lastMsg.ContentString()

	if strings.Contains(contentStr, pdfBase64) {
		t.Error("PDF attachment should NOT be sent as raw base64 text in message content")
	}

	hasPDFImageURL := false
	contentParts, ok := lastMsg.Content.([]any)
	if ok {
		for _, part := range contentParts {
			if partMap, ok := part.(map[string]any); ok {
				if partMap["type"] == "image_url" {
					if imageURL, ok := partMap["image_url"].(map[string]any); ok {
						if url, ok := imageURL["url"].(string); ok && strings.HasPrefix(url, "data:application/pdf") {
							hasPDFImageURL = true
						}
					}
				}
			}
		}
	}

	if hasPDFImageURL {
		t.Error("PDF attachment should NOT be sent as image_url because llama-server may not support it")
	}

	if !strings.Contains(contentStr, "test.pdf") {
		t.Error("PDF attachment should include file name as text hint for the model")
	}
}

func TestSendMessage_AttachmentsPersistedToDB(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sseData := makeSSEData([]string{
			makeContentChunk("回答"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:    "带附件的消息",
		SearchMode: "off",
		Attachments: []chat.Attachment{
			{Type: "text", Name: "note.txt", MimeType: "text/plain", Data: "hello world"},
		},
	})
	if err != nil {
		t.Fatalf("SendMessage with text attachment failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc), nil)
	if len(convs) == 0 {
		t.Fatal("expected at least 1 conversation")
	}

	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID, nil)
	var userMsg *store.Message
	for _, m := range msgs {
		if m.Role == "user" {
			userMsg = m
		}
	}

	if userMsg == nil {
		t.Fatal("expected user message")
	}

	if userMsg.Attachments == "" {
		t.Fatal("user message should have Attachments persisted to DB")
	}

	var dbAttachments []chat.Attachment
	if err := json.Unmarshal([]byte(userMsg.Attachments), &dbAttachments); err != nil {
		t.Fatalf("Attachments should be valid JSON: %v", err)
	}

	if len(dbAttachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(dbAttachments))
	}

	if dbAttachments[0].Type != "text" || dbAttachments[0].Data != "hello world" {
		t.Errorf("attachment data mismatch, got type=%s data=%s", dbAttachments[0].Type, dbAttachments[0].Data)
	}
}

func TestSendMessage_HistoryAttachmentsRestored(t *testing.T) {
	// 需要视觉模型，跳过
	t.Skip("needs vision model")
	callCount := 0
	var secondRoundMessages []llm.ChatMessage

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		var req llm.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)

		if callCount == 1 {
			sseData := makeSSEData([]string{
				makeContentChunk("这是一张图片"),
				makeFinishChunk("stop"),
			})
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, sseData)
			return
		}

		secondRoundMessages = req.Messages
		sseData := makeSSEData([]string{
			makeContentChunk("继续"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	imageData := "data:image/png;base64,iVBORw0KGgo="

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:    "描述这张图片",
		SearchMode: "off",
		Images:     []string{imageData},
		Attachments: []chat.Attachment{
			{Type: "image", Name: "test.png", MimeType: "image/png", Data: imageData},
		},
	})
	if err != nil {
		t.Fatalf("first SendMessage failed: %v", err)
	}

	convs, _ := store.ListConversations(chat.GetDB(svc), nil)
	convID := convs[0].ID

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		secondRoundMessages = req.Messages

		sseData := makeSSEData([]string{
			makeContentChunk("继续"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server2.Close()
	svc.UpdateClient(llm.NewClient(server2.URL, ""))

	err = svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:        "继续",
		ConversationID: convID,
		SearchMode:     "off",
	})
	if err != nil {
		t.Fatalf("second SendMessage failed: %v", err)
	}

	hasImageInHistory := false
	for _, m := range secondRoundMessages {
		if m.Role == "user" {
			if parts, ok := m.Content.([]any); ok {
				for _, part := range parts {
					if partMap, ok := part.(map[string]any); ok {
						if partMap["type"] == "image_url" {
							hasImageInHistory = true
						}
					}
				}
			}
		}
	}

	if !hasImageInHistory {
		t.Error("second round should include image from first round's attachments stored in DB")
	}
}

func TestSendMessage_ImageOnly_AlwaysHasTextContentPart(t *testing.T) {
	// 需要视觉模型，跳过
	t.Skip("needs vision model")
	var receivedReq *llm.ChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		receivedReq = &req

		sseData := makeSSEData([]string{
			makeContentChunk("这是一张图片"),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	imageData := "data:image/png;base64,iVBORw0KGgo="

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:    "",
		SearchMode: "off",
		Images:     []string{imageData},
		Attachments: []chat.Attachment{
			{Type: "image", Name: "test.png", MimeType: "image/png", Data: imageData},
		},
	})
	if err != nil {
		t.Fatalf("SendMessage with image-only (no text) failed: %v", err)
	}

	if receivedReq == nil {
		t.Fatal("expected LLM request")
	}

	lastMsg := receivedReq.Messages[len(receivedReq.Messages)-1]
	if lastMsg.Role != "user" {
		t.Fatalf("expected last message role 'user', got '%s'", lastMsg.Role)
	}

	contentParts, ok := lastMsg.Content.([]any)
	if !ok {
		t.Fatalf("expected content to be []interface{} for image message, got %T", lastMsg.Content)
	}

	hasTextPart := false
	for _, part := range contentParts {
		if partMap, ok := part.(map[string]any); ok {
			if partMap["type"] == "text" {
				hasTextPart = true
			}
		}
	}

	if !hasTextPart {
		t.Error("image-only message must contain a text ContentPart to avoid 'No user query found' Jinja error")
	}
}

func TestSendMessage_TextAttachment_HasFileLabel(t *testing.T) {
	var receivedReq *llm.ChatCompletionRequest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req llm.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &req)
		receivedReq = &req

		sseData := makeSSEData([]string{
			makeContentChunk("文件内容是..."),
			makeFinishChunk("stop"),
		})
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, sseData)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	err := svc.SendMessage(context.Background(), chat.SendMessageParams{
		Content:    "文件里面有什么内容",
		SearchMode: "off",
		Attachments: []chat.Attachment{
			{Type: "text", Name: "note.txt", MimeType: "text/plain", Data: "你好\nhello\n我爱你\nhhhh"},
		},
	})
	if err != nil {
		t.Fatalf("SendMessage with text attachment failed: %v", err)
	}

	if receivedReq == nil {
		t.Fatal("expected LLM request")
	}

	lastMsg := receivedReq.Messages[len(receivedReq.Messages)-1]
	contentStr := lastMsg.ContentString()

	if !strings.Contains(contentStr, "--- 附件: note.txt (text/plain) ---") {
		t.Errorf("text attachment should have file label, got: %s", contentStr)
	}

	if !strings.Contains(contentStr, "你好") {
		t.Errorf("text attachment content should be included, got: %s", contentStr)
	}

	if !strings.Contains(contentStr, "--- 附件结束 ---") {
		t.Errorf("text attachment should have end marker, got: %s", contentStr)
	}
}

func TestInjectSearchContext_PreservesImageContent(t *testing.T) {
	imageURL := "data:image/png;base64,iVBORw0KGgo="
	messages := []llm.ChatMessage{
		llm.NewTextMessage("system", "you are helpful"),
		llm.NewVisionMessage("user", "describe this image", []string{imageURL}),
	}

	result := chat.InjectSearchContext(messages, "search results here", "please use search results")

	lastMsg := result[len(result)-1]
	if lastMsg.Role != "user" {
		t.Fatalf("expected last message role 'user', got '%s'", lastMsg.Role)
	}

	textContent := lastMsg.ContentString()
	if !strings.Contains(textContent, "describe this image") {
		t.Errorf("should preserve original text, got: %s", textContent)
	}
	if !strings.Contains(textContent, "search results here") {
		t.Errorf("should contain search context, got: %s", textContent)
	}

	parts, ok := lastMsg.Content.([]llm.ContentPart)
	if !ok {
		t.Fatalf("expected content to be []ContentPart for multimodal message, got %T", lastMsg.Content)
	}

	hasImage := false
	for _, part := range parts {
		if part.Type == "image_url" && part.ImageURL != nil {
			hasImage = true
			if part.ImageURL.URL != imageURL {
				t.Errorf("image URL should be preserved, got: %s", part.ImageURL.URL)
			}
		}
	}
	if !hasImage {
		t.Error("multimodal message should still contain image_url after InjectSearchContext")
	}
}

func TestInjectSearchContext_PreservesAudioContent(t *testing.T) {
	audios := []llm.InputAudio{{Data: "base64audiodata", Format: "wav"}}
	messages := []llm.ChatMessage{
		llm.NewTextMessage("system", "you are helpful"),
		llm.NewAudioMessage("user", "transcribe this", audios),
	}

	result := chat.InjectSearchContext(messages, "search results", "instruction")

	lastMsg := result[len(result)-1]
	parts, ok := lastMsg.Content.([]llm.ContentPart)
	if !ok {
		t.Fatalf("expected content to be []ContentPart, got %T", lastMsg.Content)
	}

	hasAudio := false
	for _, part := range parts {
		if part.Type == "input_audio" {
			hasAudio = true
		}
	}
	if !hasAudio {
		t.Error("multimodal message should still contain input_audio after InjectSearchContext")
	}
}

func TestEstimateMessageTokens_ImageUsesFixedEstimate(t *testing.T) {
	msg := &store.Message{
		Role:    "user",
		Content: "hello",
		Images:  `["data:image/png;base64,iVBORw0KGgo="]`,
	}
	tokens := chat.EstimateMessageTokens(msg)

	if tokens > 10000 {
		t.Errorf("image token estimate should use fixed value, not base64 length * 3; got %d (likely using base64 length)", tokens)
	}

	if tokens < 1500 {
		t.Errorf("image token estimate should be at least 1500 per image, got %d", tokens)
	}
}

func TestEstimateMessageTokens_MultipleImages(t *testing.T) {
	msg := &store.Message{
		Role:    "user",
		Content: "compare these",
		Images:  `["data:image/png;base64,aaa","data:image/png;base64,bbb"]`,
	}
	tokens := chat.EstimateMessageTokens(msg)

	expectedMin := chat.EstimateTokensByLang(msg.Content, "en") + 2*1500
	if tokens < expectedMin {
		t.Errorf("2 images should estimate at least %d tokens, got %d", expectedMin, tokens)
	}
}

func TestEstimateMessageTokens_AttachmentsImageFixedEstimate(t *testing.T) {
	msg := &store.Message{
		Role:        "user",
		Content:     "hello",
		Attachments: `[{"type":"image","name":"test.png","mime_type":"image/png","data":"data:image/png;base64,verylongbase64"}]`,
	}
	tokens := chat.EstimateMessageTokens(msg)

	if tokens > 10000 {
		t.Errorf("image attachment token estimate should use fixed value, not base64 length * 3; got %d", tokens)
	}
}

func TestEstimateMessageTokens_AttachmentsAudioFixedEstimate(t *testing.T) {
	msg := &store.Message{
		Role:        "user",
		Content:     "hello",
		Attachments: `[{"type":"audio","name":"test.wav","mime_type":"audio/wav","data":"base64audio","format":"wav"}]`,
	}
	tokens := chat.EstimateMessageTokens(msg)

	expectedMin := chat.EstimateTokensByLang(msg.Content, "en") + 500
	if tokens < expectedMin {
		t.Errorf("audio attachment should add at least 500 tokens, got %d", tokens)
	}
}

func TestEstimateMessageTokens_VideoAttachmentHigherThanImage(t *testing.T) {
	msg := &store.Message{
		Role:        "user",
		Content:     "hello",
		Attachments: `[{"type":"video","name":"test.mp4","mime_type":"video/mp4","data":"base64video"}]`,
	}
	tokens := chat.EstimateMessageTokens(msg)

	if tokens < 3000 {
		t.Errorf("video attachment should estimate at least 3000 tokens, got %d", tokens)
	}
}

func TestEstimateMessageTokens_ImageAttachmentAccurateEstimate(t *testing.T) {
	msg := &store.Message{
		Role:        "user",
		Content:     "hello",
		Attachments: `[{"type":"image","name":"test.png","mime_type":"image/png","data":"base64data"}]`,
	}
	tokens := chat.EstimateMessageTokens(msg)

	if tokens < 2500 {
		t.Errorf("image attachment should estimate at least 2500 tokens (llama.cpp uses ~2000-6000 per image), got %d", tokens)
	}
}

func TestEstimateMessageTokens_MultipleImageAttachments(t *testing.T) {
	msg := &store.Message{
		Role:        "user",
		Content:     "compare these",
		Attachments: `[{"type":"image","name":"a.png","mime_type":"image/png","data":"d1"},{"type":"image","name":"b.png","mime_type":"image/png","data":"d2"}]`,
	}
	tokens := chat.EstimateMessageTokens(msg)

	if tokens < 7000 {
		t.Errorf("2 image attachments should estimate at least 7000 tokens (2*3500), got %d", tokens)
	}
}

func TestEstimateAttachmentTokens_ImageType(t *testing.T) {
	tokens := chat.EstimateAttachmentTokens("image")
	if tokens < 3500 {
		t.Errorf("image attachment type should estimate at least 3500, got %d", tokens)
	}
}

func TestEstimateAttachmentTokens_VideoType(t *testing.T) {
	tokens := chat.EstimateAttachmentTokens("video")
	if tokens < 5000 {
		t.Errorf("video attachment type should estimate at least 5000, got %d", tokens)
	}
}

func TestEstimateAttachmentTokens_AudioType(t *testing.T) {
	tokens := chat.EstimateAttachmentTokens("audio")
	if tokens < 500 {
		t.Errorf("audio attachment type should estimate at least 500, got %d", tokens)
	}
}

func TestEstimateAttachmentTokens_TextType(t *testing.T) {
	tokens := chat.EstimateAttachmentTokens("text")
	if tokens != 0 {
		t.Errorf("text attachment type should estimate 0 (content is inlined), got %d", tokens)
	}
}

func TestBuildLLMMessages_CurrentAttachmentsTokenEstimate(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 4096

	longContent := strings.Repeat("这是一段很长的中文内容用于填充token。", 100)
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: longContent},
		{ID: "2", Role: "assistant", Content: "好的"},
		{ID: "3", Role: "user", Content: "hello"},
	}

	msgsNoAtt, _ := chat.BuildLLMMessages(svc, dbMsgs, "hello", nil)

	svc2 := newTestService()
	svc2.GetConfig().ContextSize = 4096
	imageAttachments := []chat.Attachment{
		{Type: "image", Name: "test.png", MimeType: "image/png", Data: "base64data"},
	}
	msgsWithAtt, err := chat.BuildLLMMessages(svc2, dbMsgs, "hello", imageAttachments)

	if err != nil {
		t.Logf("BuildLLMMessages with image attachment returned error (model may not support vision): %v", err)
	}

	if err == nil && len(msgsWithAtt) > len(msgsNoAtt) {
		t.Errorf("with image attachment consuming ~3500 tokens, history should be trimmed more; got %d msgs with att vs %d without", len(msgsWithAtt), len(msgsNoAtt))
	}
}

func TestBuildLLMMessages_SmallContextWithImage_NoOverflow(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 8192
	svc.SetModelCapabilities(llm.ModelCapabilities{TextInput: true, ImageInput: true})

	longContent := strings.Repeat("这是一段很长的中文内容用于填充token。", 50)
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: longContent},
		{ID: "2", Role: "assistant", Content: longContent},
		{ID: "3", Role: "user", Content: longContent},
		{ID: "4", Role: "assistant", Content: longContent},
		{ID: "5", Role: "user", Content: "请描述这张图片"},
	}

	imageAttachments := []chat.Attachment{
		{Type: "image", Name: "test.png", MimeType: "image/png", Data: "base64data"},
	}

	msgs, err := chat.BuildLLMMessages(svc, dbMsgs, "请描述这张图片", imageAttachments)
	if err != nil {
		t.Fatalf("BuildLLMMessages should not error with vision model: %v", err)
	}

	if len(msgs) < 2 {
		t.Errorf("should have at least system + current message, got %d", len(msgs))
	}

	if len(msgs) >= len(dbMsgs)+1 {
		t.Errorf("with 8192 context and large history, some messages should be trimmed; got %d messages for %d db messages", len(msgs), len(dbMsgs))
	}
}

func TestBuildLLMMessages_CurrentMessageNearLimit_NoHistory(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 4096
	svc.SetModelCapabilities(llm.ModelCapabilities{TextInput: true, ImageInput: true})

	longContent := strings.Repeat("这是一段很长的中文内容用于填充token。", 200)
	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: longContent},
		{ID: "2", Role: "assistant", Content: longContent},
		{ID: "3", Role: "user", Content: "描述图片"},
	}

	imageAttachments := []chat.Attachment{
		{Type: "image", Name: "test.png", MimeType: "image/png", Data: "base64data"},
	}

	msgs, err := chat.BuildLLMMessages(svc, dbMsgs, "描述图片", imageAttachments)
	if err != nil {
		t.Fatalf("BuildLLMMessages should not error with vision model: %v", err)
	}

	if len(msgs) > 2 {
		t.Errorf("when current message + system prompt near limit, should not load history; got %d messages (expected <= 2: system + current)", len(msgs))
	}
}

func TestParseExceedContextError_VariousFormats(t *testing.T) {
	tests := []struct {
		name         string
		errMsg       string
		wantExceeded bool
		wantPrompt   int
		wantCtx      int
	}{
		{
			name:         "llama_cpp_format",
			errMsg:       `unexpected status code 400: {"error":{"code":400,"message":"request (8345 tokens) exceeds available context size (8192 tokens)","type":"exceed_context_size_error","n_prompt_tokens":8345,"n_ctx":8192}}`,
			wantExceeded: true,
			wantPrompt:   8345,
			wantCtx:      8192,
		},
		{
			name:         "n_prompt_tokens_format",
			errMsg:       `exceed_context_size_error: n_prompt_tokens=10000, n_ctx=8192`,
			wantExceeded: true,
			wantPrompt:   10000,
			wantCtx:      8192,
		},
		{
			name:         "available_context_format",
			errMsg:       `request (5000 tokens) exceeds available context size (4096 tokens)`,
			wantExceeded: true,
			wantPrompt:   5000,
			wantCtx:      4096,
		},
		{
			name:         "context_size_exceeded",
			errMsg:       `context size exceeded: prompt has 12000 tokens but n_ctx is 8192`,
			wantExceeded: true,
			wantPrompt:   0,
			wantCtx:      0,
		},
		{
			name:         "non_context_error",
			errMsg:       `connection refused`,
			wantExceeded: false,
			wantPrompt:   0,
			wantCtx:      0,
		},
		{
			name:         "nil_error",
			errMsg:       "",
			wantExceeded: false,
			wantPrompt:   0,
			wantCtx:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var err error
			if tt.errMsg != "" {
				err = fmt.Errorf("%s", tt.errMsg)
			}
			info := chat.ParseExceedContextError(err)
			if tt.wantExceeded {
				if info == nil || !info.Exceeded {
					t.Fatal("expected exceeded=true")
				}
				if info.PromptTokens != tt.wantPrompt {
					t.Errorf("PromptTokens = %d, want %d", info.PromptTokens, tt.wantPrompt)
				}
				if info.ContextSize != tt.wantCtx {
					t.Errorf("ContextSize = %d, want %d", info.ContextSize, tt.wantCtx)
				}
			} else {
				if info != nil {
					t.Errorf("expected nil, got %+v", info)
				}
			}
		})
	}
}

func TestTrimMessagesToFit_BasicTrimming(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: strings.Repeat("hello world ", 100)},
		{Role: "assistant", Content: strings.Repeat("response text ", 100)},
		{Role: "user", Content: strings.Repeat("another question ", 100)},
		{Role: "assistant", Content: strings.Repeat("another answer ", 100)},
		{Role: "user", Content: "final question"},
	}

	trimmed := chat.TrimMessagesToFit(messages, 200, 50)

	if len(trimmed) >= len(messages) {
		t.Errorf("expected trimming, got %d messages (same as original %d)", len(trimmed), len(messages))
	}

	if len(trimmed) < 2 {
		t.Fatalf("expected at least system + last message, got %d", len(trimmed))
	}

	if trimmed[0].Role != "system" {
		t.Error("first message should be system")
	}

	if trimmed[len(trimmed)-1].Content.(string) != "final question" {
		t.Error("last message should be preserved")
	}
}

func TestTrimMessagesToFit_PreservesSystemAndLast(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "question 1"},
		{Role: "assistant", Content: "answer 1"},
		{Role: "user", Content: "question 2"},
	}

	trimmed := chat.TrimMessagesToFit(messages, 50, 10)

	if len(trimmed) < 2 {
		t.Fatalf("expected at least 2 messages, got %d", len(trimmed))
	}

	if trimmed[0].Role != "system" {
		t.Error("first message should be system")
	}
	if trimmed[len(trimmed)-1].Content.(string) != "question 2" {
		t.Error("last message should be the final user message")
	}
}

func TestTrimMessagesToFit_NoTrimNeeded(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "bye"},
	}

	trimmed := chat.TrimMessagesToFit(messages, 10000, 100)

	if len(trimmed) != len(messages) {
		t.Errorf("no trimming needed, expected %d messages, got %d", len(messages), len(trimmed))
	}
}

func TestTrimMessagesToFit_TwoMessagesNoTrim(t *testing.T) {
	messages := []llm.ChatMessage{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "hi"},
	}

	trimmed := chat.TrimMessagesToFit(messages, 10, 5)

	if len(trimmed) != 2 {
		t.Errorf("2 messages should not be trimmed, got %d", len(trimmed))
	}
}

func TestBuildLLMMessages_WithSearchContext(t *testing.T) {
	svc := newTestService()
	svc.GetConfig().ContextSize = 8192
	svc.SetModelCapabilities(llm.ModelCapabilities{TextInput: true, ImageInput: true})

	dbMsgs := []*store.Message{
		{ID: "1", Role: "user", Content: "你好"},
		{ID: "2", Role: "assistant", Content: "你好！"},
		{ID: "3", Role: "user", Content: "搜索一下最新新闻"},
	}

	msgs, err := chat.BuildLLMMessagesWithSearch(svc, dbMsgs, "搜索一下最新新闻", nil, "on")
	if err != nil {
		t.Fatalf("BuildLLMMessagesWithSearch failed: %v", err)
	}

	if len(msgs) < 2 {
		t.Fatalf("expected at least system + user message, got %d", len(msgs))
	}

	if msgs[0].Role != "system" {
		t.Error("first message should be system")
	}
}

func TestEstimateChatMessageTokens_VisionMessage(t *testing.T) {
	msg := llm.NewVisionMessage("user", "describe this", []string{"data:image/png;base64,abc"})
	tokens := chat.EstimateTokensByLang(msg.ContentString(), chat.DetectLanguage(msg.ContentString()))

	if tokens <= 0 {
		t.Error("vision message text should have positive token estimate")
	}
}

func TestEstimateChatMessageTokens_TextMessage(t *testing.T) {
	msg := llm.NewTextMessage("user", "这是一段中文测试内容")
	tokens := chat.EstimateTokensByLang(msg.ContentString(), chat.DetectLanguage(msg.ContentString()))

	if tokens <= 0 {
		t.Error("text message should have positive token estimate")
	}
}
