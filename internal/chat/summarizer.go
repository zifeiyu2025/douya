package chat

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"douya/internal/llm"
	"douya/internal/store"
)

const (
	summaryMaxTokens  = 300 // 摘要生成的最大 token 数
	summaryMaxChars   = 500 // 摘要结果最大字符数
	summaryMinMsgs    = 4   // 少于该数量的消息不生成摘要
	summaryTimeoutSec = 30  // 摘要生成超时秒数
)

// summarizeMessages 对被裁剪的消息生成摘要
// existingSummary: 已有的旧摘要（增量更新时传入），为空则首次生成
// messages: 被裁剪的消息列表
func summarizeMessages(client *llm.Client, existingSummary string, messages []*store.Message) string {
	if len(messages) < summaryMinMsgs {
		return ""
	}

	// 格式化消息为纯文本
	var sb strings.Builder
	if existingSummary != "" {
		sb.WriteString("【之前的对话摘要】\n")
		sb.WriteString(existingSummary)
		sb.WriteString("\n\n【后续对话内容】\n")
	}
	for _, m := range messages {
		role := m.Role
		switch role {
		case "user":
			role = "用户"
		case "assistant":
			role = "助手"
		case "system":
			role = "系统"
		}
		content := m.Content
		// 截断过长的单条消息，避免摘要请求本身过大
		if len(content) > 800 {
			content = content[:800] + "..."
		}
		sb.WriteString(fmt.Sprintf("%s: %s\n", role, content))
	}

	dialogText := sb.String()
	// 如果对话文本本身就很短，不需要摘要
	if len(dialogText) < 100 {
		return ""
	}

	// 构造摘要请求
	req := &llm.ChatCompletionRequest{
		Messages: []llm.ChatMessage{
			{
				Role: "system",
				Content: "你是一个对话摘要助手。请用2-3句话概括以下对话的关键信息，" +
					"包括讨论的主题、重要结论和未解决的问题。" +
					"只输出摘要内容，不要输出其他内容。",
			},
			{
				Role:    "user",
				Content: dialogText,
			},
		},
		MaxTokens:   summaryMaxTokens,
		Temperature: 0.3, // 低温度保证摘要稳定
	}

	ctx, cancel := context.WithTimeout(context.Background(), summaryTimeoutSec*time.Second)
	defer cancel()

	resp, err := client.Chat(ctx, req)
	if err != nil {
		log.Warn().Err(err).Msg("[summarizer] 摘要生成失败，降级为直接丢弃")
		return ""
	}

	if len(resp.Choices) == 0 {
		log.Warn().Msg("[summarizer] 摘要生成返回空结果")
		return ""
	}

	summary := strings.TrimSpace(resp.Choices[0].Message.ContentString())
	if len(summary) > summaryMaxChars {
		summary = summary[:summaryMaxChars] + "..."
	}

	log.Info().Int("input_msgs", len(messages)).Int("summary_len", len(summary)).Msg("[summarizer] 摘要生成成功")
	return summary
}
