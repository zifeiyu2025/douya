package chat

import (
	"strings"

	"douya/internal/llm"
)

// 重要性评分阈值：评分 >= importanceMustKeep 的消息视为"必保"，不会被裁剪
const importanceMustKeep = 5

// 用户明确指令关键词（中英文）：包含这些词的消息优先保留
// 例如："请记住我的项目用 Go 1.22"、"重要决策：..." 等
var importantKeywords = []string{
	// 中文
	"记住", "重要", "务必", "关键", "注意", "请保存", "记一下", "别忘了",
	// 英文
	"remember", "important", "key", "don't forget", "note that",
}

// ScoreChatMessage 对单条 ChatMessage 计算重要性分数（0-10）。
// 评分规则（启发式，无 LLM 调用，零成本）：
//   - 含代码块（```）+3
//   - 含用户明确指令关键词 +2
//   - 是 tool 角色消息（结构化工具结果）+2
//   - assistant 带 tool_calls（即将调用工具）+2
//   - 是 assistant 的最终回复（非 thinking 推理）+1
//   - 是 user 消息（用户意图）+1
//   - system 消息默认 +0（已在 systemMsg 中单独保护，不需要评分）
//
// 注意：由于 ChatMessage 没有 CreatedAt 等元数据，时间衰减由调用方在排序时按位置处理。
func ScoreChatMessage(msg llm.ChatMessage) int {
	if msg.Role == "system" {
		return 0 // system 消息由调用方单独保护
	}

	score := 0
	content := msg.ContentString()

	// 1. 含代码块（```）+3：代码片段通常包含关键决策
	if strings.Contains(content, "```") {
		score += 3
	}

	// 2. 含用户明确指令关键词 +2：用户明确要求保留的信息
	contentLower := strings.ToLower(content)
	for _, kw := range importantKeywords {
		if strings.Contains(contentLower, kw) {
			score += 2
			break // 只加一次
		}
	}

	// 3. 角色相关加分
	switch msg.Role {
	case "tool":
		// tool 角色消息（工具调用结果，通常含结构化数据）
		score += 2
	case "assistant":
		if len(msg.ToolCalls) > 0 {
			// assistant 带 tool_calls（即将调用工具，是 tool call 链的起点）
			score += 2
		} else if msg.ReasoningContent == "" {
			// assistant 的最终回复（非 thinking 推理内容）
			score++
		}
		// assistant 的 thinking 内容（ReasoningContent）默认 0 分，可被裁剪
	case "user":
		// user 消息代表用户意图，比 assistant 的中间回复更重要
		score++
	}

	// 分数上限 10
	if score > 10 {
		score = 10
	}
	return score
}

// ScoreChatMessages 批量计算消息评分，返回"消息索引 -> 评分"的映射。
// 调用方（如 TrimMessagesToFit）可用此映射决定裁剪优先级：
//   - 评分 >= importanceMustKeep 的消息视为"必保"，不应被裁剪
//   - 其他消息按"评分升序 + 索引升序"（低分+旧的优先）依次淘汰
func ScoreChatMessages(msgs []llm.ChatMessage) map[int]int {
	scores := make(map[int]int, len(msgs))
	for i := range msgs {
		scores[i] = ScoreChatMessage(msgs[i])
	}
	return scores
}

// IsMustKeep 判断指定评分的消息是否为"必保"消息（不应被裁剪）。
// 调用方在裁剪时应跳过必保消息，从其他消息中淘汰。
func IsMustKeep(score int) bool {
	return score >= importanceMustKeep
}
