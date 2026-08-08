// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import "sync/atomic"

// contextTrimReason 上下文裁剪的触发类别。
// 用于统一 context_trimmed 事件中的 reason 取值，避免三处发送侧拼写不一致。
type contextTrimReason string

const (
	// trimReasonPreventive 主动预防性裁剪（估算接近上限时提前压缩）。
	trimReasonPreventive contextTrimReason = "preventive_trim"
	// trimReasonExceed 溢出后裁剪（模型实际返回 context exceeded 后重试）。
	trimReasonExceed contextTrimReason = "exceed_context_size"
	// trimReasonToolLoop tool call 循环中的预防性裁剪。
	trimReasonToolLoop contextTrimReason = "tool_call_loop_trim"
)

// ContextTrimEventContent 是 context_trimmed 事件的标准化 content 结构，
// 与前端 frontend/src/types/chat.ts 的 ContextTrimmedEvent 一一对应。
type ContextTrimEventContent struct {
	Reason        string `json:"reason"`
	PromptTokens  int    `json:"prompt_tokens,omitempty"`
	ContextSize   int    `json:"context_size,omitempty"`
	MessagesAfter int    `json:"messages_after,omitempty"`
}

// CompressionStats 记录本次运行中上下文压缩的累计数据。
// 内部使用原子计数，可在并发流式生成期间安全累加与读取。
// 生活类比：像厨房的计数器，每次压一次菜就记一下，方便看这周压了多少次。
type CompressionStats struct {
	preventive int64 // 预防性压缩次数
	exceed     int64 // 溢出后压缩次数
	toolLoop   int64 // tool 循环内压缩次数
}

func (s *CompressionStats) inc(why contextTrimReason) {
	switch why {
	case trimReasonPreventive:
		atomic.AddInt64(&s.preventive, 1)
	case trimReasonExceed:
		atomic.AddInt64(&s.exceed, 1)
	case trimReasonToolLoop:
		atomic.AddInt64(&s.toolLoop, 1)
	}
}

// Snapshot 是 CompressionStats 的可读快照，用于日志/前端展示。
type CompressionStatsSnapshot struct {
	PreventiveTrimmed int `json:"preventive_trimmed"`
	ExceedTrimmed     int `json:"exceed_trimmed"`
	ToolLoopTrimmed   int `json:"tool_loop_trimmed"`
	TotalTrimmed      int `json:"total_trimmed"`
}

func (s *CompressionStats) snapshot() CompressionStatsSnapshot {
	prev := atomic.LoadInt64(&s.preventive)
	exc := atomic.LoadInt64(&s.exceed)
	tool := atomic.LoadInt64(&s.toolLoop)
	return CompressionStatsSnapshot{
		PreventiveTrimmed: int(prev),
		ExceedTrimmed:     int(exc),
		ToolLoopTrimmed:   int(tool),
		TotalTrimmed:      int(prev + exc + tool),
	}
}
