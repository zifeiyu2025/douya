package chat

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/rs/zerolog/log"

	"douya/internal/llm"
	"douya/internal/store"
)

const (
	summaryMaxTokens  = 300 // 摘要生成的最大 token 数
	summaryMaxChars   = 500 // 摘要结果最大字符数（按 rune 计数，L-11 修复）
	summaryMinMsgs    = 4   // 少于该数量的消息不生成摘要
	summaryTimeoutSec = 30  // 摘要生成超时秒数
)

// truncateRunes 按 rune（字符）截断字符串，避免在多字节字符中间截断产生非法 UTF-8
// L-11 修复：原实现用 len()+[:] 按字节截断，中文每字符 3 字节，[:800] 可能切断字符。
func truncateRunes(s string, maxRunes int) string {
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes]) + "..."
}

// summarizeMessages 对被裁剪的消息生成摘要
// ctx: 调用方传入的上下文，用于控制超时和取消（通常是异步 goroutine 中的可取消 ctx）
// existingSummary: 已有的旧摘要（增量更新时传入），为空则首次生成
// messages: 被裁剪的消息列表
func summarizeMessages(ctx context.Context, client *llm.Client, existingSummary string, messages []*store.Message) string {
	if len(messages) < summaryMinMsgs {
		return ""
	}

	// 优先检查 ctx 是否已取消：若调用方已取消，直接返回空摘要，避免发起无意义的 LLM 请求
	if ctx.Err() != nil {
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
		// 截断过长的单条消息，避免摘要请求本身过大（L-11：按字符截断，避免切断中文）
		content = truncateRunes(content, 800)
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

	// 从调用方传入的 ctx 派生带超时的子 ctx，这样当父 ctx 被取消时，摘要请求也会被联动取消
	ctx, cancel := context.WithTimeout(ctx, summaryTimeoutSec*time.Second)
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
	// L-11：按字符截断，避免切断中文产生非法 UTF-8
	summary = truncateRunes(summary, summaryMaxChars)

	log.Info().Int("input_msgs", len(messages)).Int("summary_len", len(summary)).Msg("[summarizer] 摘要生成成功")
	return summary
}
