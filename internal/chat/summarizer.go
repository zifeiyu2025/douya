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
	// P1-C1: 长期摘要合并触发频率 - 每 N 次压缩合并一次
	// N=5 是平衡点：太频繁（N=2）会导致 LLM 调用过多；太稀疏（N=10）会导致短期摘要累积过长
	longSummaryMergeInterval = 5
	// P1-C1: 长期摘要最大字符数（比短期摘要稍长，容纳更多关键事实）
	longSummaryMaxChars = 800
	// P3-C2: 摘要重置触发周期 - 每 N 次压缩重置一次（从当前窗口重新生成，丢弃旧摘要）
	// N=10 与 longSummaryMergeInterval=5 错开：第 5 次合并长期、第 10 次重置全部、第 15 次再合并…
	// 避免两个机制同时触发冲突
	summaryResetInterval = 10
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
	// P0-C3: 优化摘要提示词，明确"必须保留"与"可以丢弃"两类，避免弱模型摘要漂移。
	// 输出结构化（用户目标/关键决策/重要实体/待办事项），便于后续分层摘要合并时保留关键信息。
	req := &llm.ChatCompletionRequest{
		Messages: []llm.ChatMessage{
			{
				Role: "system",
				Content: "你是对话摘要助手。请按以下规则生成摘要：\n\n" +
					"【必须保留】\n" +
					"1. 用户的明确要求、偏好、决策\n" +
					"2. 关键代码片段的文件名和用途（不需要完整代码）\n" +
					"3. 重要实体（人名/项目/技术栈/版本号）\n" +
					"4. 错误和解决方案\n\n" +
					"【可以丢弃】\n" +
					"1. 寒暄、感谢、客套话\n" +
					"2. 重复的提问\n" +
					"3. 中间探索性的失败尝试\n\n" +
					"输出格式：\n" +
					"- 用户目标：...\n" +
					"- 关键决策：...\n" +
					"- 重要实体：...\n" +
					"- 待办事项：...\n" +
					"（不超过300字，只输出摘要，不要其他内容）",
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

// mergeLongSummary P1-C1: 合并长期摘要。
// 把旧的长期摘要 + 累积的短期摘要合并为新的长期摘要，避免无限递归漂移。
// 生活类比：像定期整理书桌，把散落的便签（短期摘要）整理到一本"年度记事本"（长期摘要）里。
//
// 参数：
//   - ctx: 调用方上下文（用于超时和取消）
//   - client: LLM 客户端
//   - oldLong: 旧的长期摘要（可为空，首次合并时为空）
//   - accumulatedShort: 累积的短期摘要（最近 N 次压缩的短期摘要）
//
// 返回：合并后的新长期摘要。失败时返回 oldLong（保留旧值，不丢数据）。
func mergeLongSummary(ctx context.Context, client *llm.Client, oldLong, accumulatedShort string) string {
	// 首次合并：旧长期摘要为空，直接用短期摘要作为长期摘要
	if oldLong == "" {
		return truncateRunes(accumulatedShort, longSummaryMaxChars)
	}
	// 短期摘要为空，保留旧长期摘要
	if accumulatedShort == "" {
		return oldLong
	}

	// 优先检查 ctx 是否已取消
	if ctx.Err() != nil {
		return oldLong
	}

	// 构造合并请求
	req := &llm.ChatCompletionRequest{
		Messages: []llm.ChatMessage{
			{
				Role: "system",
				Content: "你是记忆合并助手。请把【长期记忆】和【近期记忆】合并为一份新的长期记忆。\n\n" +
					"合并规则：\n" +
					"1. 保留所有关键事实、决策、实体（人名/项目/技术栈/版本号）\n" +
					"2. 去除重复信息\n" +
					"3. 若有冲突，以【近期记忆】为准（更新的事实覆盖旧的）\n" +
					"4. 保持简洁，不超过 500 字\n\n" +
					"输出格式：\n" +
					"- 用户目标：...\n" +
					"- 关键决策：...\n" +
					"- 重要实体：...\n" +
					"- 待办事项：...\n" +
					"（只输出合并后的记忆，不要其他内容）",
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("【长期记忆】\n%s\n\n【近期记忆】\n%s", oldLong, accumulatedShort),
			},
		},
		MaxTokens:   600, // 长期摘要比短期摘要稍长
		Temperature: 0.3,
	}

	ctx, cancel := context.WithTimeout(ctx, summaryTimeoutSec*time.Second)
	defer cancel()

	resp, err := client.Chat(ctx, req)
	if err != nil {
		log.Warn().Err(err).Msg("[summarizer] 长期摘要合并失败，保留旧值")
		return oldLong
	}
	if len(resp.Choices) == 0 {
		log.Warn().Msg("[summarizer] 长期摘要合并返回空结果，保留旧值")
		return oldLong
	}

	merged := strings.TrimSpace(resp.Choices[0].Message.ContentString())
	merged = truncateRunes(merged, longSummaryMaxChars)

	log.Info().Int("old_len", len(oldLong)).Int("short_len", len(accumulatedShort)).Int("merged_len", len(merged)).Msg("[summarizer] 长期摘要合并成功")
	return merged
}

// shouldMergeLongSummary P1-C1: 判断是否应该触发长期摘要合并。
// 触发条件：(当前压缩次数 + 1) % longSummaryMergeInterval == 0
// 即每 N 次压缩合并一次（首次合并在第 N 次压缩时触发）
func shouldMergeLongSummary(currentCompressCount int) bool {
	return (currentCompressCount+1)%longSummaryMergeInterval == 0
}

// ShouldResetSummary P3-C2: 判断是否应该触发周期性摘要重置（导出函数，便于测试）。
//
// 触发条件：currentCompressCount > 0 且 (currentCompressCount+1) % summaryResetInterval == 0
// 即每 N 次压缩重置一次（首次重置在第 N 次压缩时触发，N=10）。
//
// 与 shouldMergeLongSummary 的协调：
//   - 第 5 次压缩：shouldMergeLongSummary=true, ShouldResetSummary=false → 合并长期
//   - 第 10 次压缩：shouldMergeLongSummary=true, ShouldResetSummary=true → 重置全部（优先级更高，跳过合并）
//   - 第 15 次压缩：shouldMergeLongSummary=true, ShouldResetSummary=false → 合并长期
//   - 第 20 次压缩：shouldMergeLongSummary=true, ShouldResetSummary=true → 重置全部
//
// 注意：currentCompressCount=0 时不触发重置，避免首次压缩误触发。
// 调用方在重置时应跳过 shouldMergeLongSummary，避免两个机制同时执行。
func ShouldResetSummary(currentCompressCount int) bool {
	if currentCompressCount <= 0 {
		return false
	}
	return (currentCompressCount+1)%summaryResetInterval == 0
}

// resetSummary P3-C2: 完整重述 - 基于当前所有被裁剪消息重新生成摘要，避免无限递归漂移。
//
// 触发条件：ShouldResetSummary 返回 true（每 10 次压缩触发一次）
//
// 与 summarizeMessages 的区别：
//   - summarizeMessages: 增量模式，输入"旧摘要+新消息"，输出基于旧摘要的新摘要
//   - resetSummary: 重置模式，输入"完整对话窗口"，输出全新摘要，丢弃旧摘要
//
// 实现方式：复用 summarizeMessages，但 existingSummary 传空串，
// 这样 summarizeMessages 内部会跳过"【之前的对话摘要】"段，直接基于完整对话生成新摘要。
//
// 生活类比：像定期把笔记本撕掉重写，而不是在旧笔记上涂涂改改。
// 旧笔记涂改多了会看不清（漂移），重写一遍更清晰准确。
//
// 参数：
//   - ctx: 调用方上下文（用于超时和取消）
//   - client: LLM 客户端
//   - messages: 当前被裁剪的完整消息列表（全部，非增量）
//
// 返回：全新的短期摘要。失败时返回空串（调用方保留旧摘要，不丢数据）。
func resetSummary(ctx context.Context, client *llm.Client, messages []*store.Message) string {
	log.Info().Int("msg_count", len(messages)).Msg("[summarizer] 触发周期性摘要重置，丢弃旧摘要，从当前窗口重新生成")
	// 重置模式：existingSummary 传空串，summarizeMessages 内部会跳过"【之前的对话摘要】"段
	return summarizeMessages(ctx, client, "", messages)
}
