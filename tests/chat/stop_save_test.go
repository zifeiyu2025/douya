// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"douya/internal/chat"
	"douya/internal/store"
)

// TestStopSave_PartialContentSaved 验证：当用户停止生成时，已生成的部分内容应该被保存到数据库。
//
// 生活类比：就像你在用录音机录音，中途按了停止键，录音机应该把已经录到的声音保存下来，
// 而不是直接丢弃。这个测试就是检查"停止生成"后，已经生成的那部分文字是否被保存了。
//
// 修复前：点击停止后，已生成内容丢失（不保存），测试会失败。
// 修复后：点击停止后，已生成内容被保存，测试通过。
func TestStopSave_PartialContentSaved(t *testing.T) {
	// 创建一个慢速流式服务器：先发送部分内容，然后等待（让我们有机会取消）
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("ResponseWriter 不支持 Flush")
		}

		w.Header().Set("Content-Type", "text/event-stream")

		// 先发送部分内容（这部分应该被保存）
		fmt.Fprint(w, "data: "+makeContentChunk("这是已经生成的部分内容")+"\n\n")
		flusher.Flush()

		// 等待足够长时间，让测试有机会调用 StopGeneration
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	// 在 goroutine 中启动 SendMessage（因为它会阻塞直到流结束或被取消）
	done := make(chan error, 1)
	go func() {
		done <- svc.SendMessage(context.Background(), chat.SendMessageParams{
			Content:    "测试停止保存",
			SearchMode: "off",
		})
	}()

	// 等待一下，让流式开始并接收到部分内容
	time.Sleep(200 * time.Millisecond)

	// 用户点击"停止生成"
	svc.StopGeneration()

	// 等待 SendMessage 返回
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendMessage 返回了错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SendMessage 在 5 秒内没有返回")
	}

	// 检查数据库中是否有保存的部分内容
	convs, _ := store.ListConversations(chat.GetDB(svc), nil)
	if len(convs) == 0 {
		t.Fatal("期望至少有 1 个会话")
	}

	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID, nil)

	// 查找非工具调用的 assistant 消息（即最终回复）
	var assistantMsg *store.Message
	for _, m := range msgs {
		if m.Role == "assistant" && m.ToolCalls == "" {
			assistantMsg = m
			break
		}
	}

	if assistantMsg == nil {
		t.Fatal("期望停止生成后，已生成的部分内容被保存为 assistant 消息，但没找到")
	}

	if !strings.Contains(assistantMsg.Content, "这是已经生成的部分内容") {
		t.Errorf("assistant 消息应该包含部分内容，实际得到: %s", assistantMsg.Content)
	}
}

// TestStopSave_EmptyContentNotSaved 验证：当用户停止生成时，如果还没有生成任何内容，
// 不应该创建空的 assistant 消息。
//
// 生活类比：就像录音机刚启动你就按了停止，没录到任何声音，就不应该保存一个空录音文件。
//
// 修复前后都应该通过：空内容时不创建消息。
func TestStopSave_EmptyContentNotSaved(t *testing.T) {
	// 创建一个服务器：不发送任何内容，直接等待（让我们在内容生成前取消）
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// 不发送任何内容，等待取消
		time.Sleep(2 * time.Second)
	}))
	defer server.Close()

	svc := newInteractionTestService(t, server, nil)

	// 在 goroutine 中启动 SendMessage
	done := make(chan error, 1)
	go func() {
		done <- svc.SendMessage(context.Background(), chat.SendMessageParams{
			Content:    "测试空内容停止",
			SearchMode: "off",
		})
	}()

	// 等待一下让请求开始
	time.Sleep(100 * time.Millisecond)

	// 用户点击"停止生成"（此时还没有任何内容生成）
	svc.StopGeneration()

	// 等待 SendMessage 返回
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendMessage 返回了错误: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SendMessage 在 5 秒内没有返回")
	}

	// 检查数据库中不应该有 assistant 消息（因为内容为空）
	convs, _ := store.ListConversations(chat.GetDB(svc), nil)
	if len(convs) == 0 {
		// 没有会话也没问题（理论上应该有 user 消息，但如果没有也说明没有 assistant 消息）
		return
	}

	msgs, _ := store.GetMessagesByConversation(chat.GetDB(svc), convs[0].ID, nil)

	for _, m := range msgs {
		if m.Role == "assistant" && m.ToolCalls == "" {
			t.Errorf("期望空内容时不创建 assistant 消息，但找到了: content=%q", m.Content)
		}
	}
}
