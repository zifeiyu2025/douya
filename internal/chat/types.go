// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"encoding/json"

	"douya/internal/search"
)

type Conversation struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type Message struct {
	ID               string              `json:"id"`
	ConversationID   string              `json:"conversation_id"`
	Role             string              `json:"role"`
	Content          string              `json:"content"`
	ThinkingContent  string              `json:"thinking_content,omitempty"`
	ThinkingDuration float64             `json:"thinking_duration,omitempty"`
	SearchResults    string              `json:"search_results"`
	Images           string              `json:"images,omitempty"`
	Attachments      []AttachmentSummary `json:"attachments,omitempty"`
	CreatedAt        string              `json:"created_at"`
	TokensPerSecond  float64             `json:"tokens_per_second,omitempty"` // 生成速度（tokens/s），仅事件传递，不存数据库
	PredictedN       int                 `json:"predicted_n,omitempty"`       // 生成的 token 数，仅事件传递，不存数据库
}

type AttachmentSummary struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
}

type Attachment struct {
	Type     string `json:"type"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type"`
	Data     string `json:"data"`
	Format   string `json:"format,omitempty"`
}

type SendMessageParams struct {
	ConversationID string       `json:"conversation_id"`
	Content        string       `json:"content"`
	SearchMode     string       `json:"search_mode"` // "off", "auto", "on"
	Images         []string     `json:"images,omitempty"`
	Attachments    []Attachment `json:"attachments,omitempty"`
}

type StreamEvent struct {
	Type           string `json:"type"`
	Content        any    `json:"content"`
	ConversationID string `json:"conversation_id"`
}

// ===== 事件类型常量（任务 31.4） =====
// 统一管理事件类型字符串，避免发送侧/接收侧拼写不一致。
// 与前端 frontend/src/types/chat.ts 的 StreamEvent 联合类型成员一一对应。
const (
	EventToken               = "token"                 // token 增量，content: string
	EventThinking            = "thinking"              // 思考增量，content: string
	EventToolCallStart       = "tool_call_start"       // 工具调用开始，content: ToolCallStartContent
	EventSearchStart         = "search_start"          // 搜索开始，content: string
	EventSearchResult        = "search_result"         // 搜索结果，content: SearchResultContent（C-7 起预搜索与 tool call 统一格式）
	EventSearchError         = "search_error"          // 搜索失败，content: string（用户友好的错误提示）
	EventTokenSpeed          = "token_speed"           // 生成速度，content: TokenSpeedContent
	EventPromptProgress      = "prompt_progress"       // 提示词进度，content: PromptProgressContent
	EventContextTrimmed      = "context_trimmed"       // 上下文裁剪，content: ContextTrimmedContent
	EventOutputTruncated     = "output_truncated"      // 输出截断（finish_reason=length），content: OutputTruncatedContent
	EventDone                = "done"                  // 生成完成，content: nil
	EventStopped             = "stopped"               // 生成停止，content: nil
	EventError               = "error"                 // 错误，content: string
	EventConversationCreated = "conversation_created"  // 会话创建，content: Conversation
	EventAssistantMessage    = "assistant_message"     // 助手消息，content: Message
	EventUserMessage         = "user_message"          // 用户消息，content: Message
	EventConversationUpdated = "conversation_updated"  // 会话更新，content: Conversation
	EventConversationDeleted = "conversation_deleted"  // 会话删除，content: string（会话 ID，C-7 起唯一格式）
	EventMessageDeleted      = "message_deleted"       // 消息删除，content: string（消息 ID，C-7 起唯一格式）
	EventToolApprovalRequest = "tool_approval_request" // 工具审批请求（Agent 模式硬门禁），content: ToolApprovalRequestContent
	EventToolCallEnd         = "tool_call_end"         // 单个工具执行结束，content: ToolCallEndContent
)

// ===== 类型化 Content struct（任务 31.4） =====
// 为 content 为复杂对象的事件提供类型化 struct，便于编译期校验与测试。
// 简单 content（string / nil / Conversation / Message）直接复用既有类型，不单独定义 struct。
// 注意：当前发送侧（service_stream.go）仍用 map[string]interface{} 发送，
// 此处 struct 作为类型契约文档与 Decode 目标，后续可逐步迁移发送侧使用 struct。

// ToolCallStartContent 工具调用开始事件的内容
type ToolCallStartContent struct {
	ToolCallID string `json:"tool_call_id"` // 工具调用 ID（用于并发 tool call 关联）
	Tool       string `json:"tool"`         // 工具名称
	Query      string `json:"query"`        // 查询参数
}

// ToolApprovalRequestContent 工具审批请求事件的内容（Agent 模式硬门禁）。
// 前端展示工具名、参数与风险等级，用户决定后调用 ResolveToolApproval 回传，
// 后端以 tool_call_id 关联解除阻塞；超时未响应视为拒绝。
type ToolApprovalRequestContent struct {
	ToolCallID  string `json:"tool_call_id"` // 工具调用 ID
	Tool        string `json:"tool"`         // 工具名称（如 exec_shell_command）
	DisplayName string `json:"display_name"` // 展示名（如 "Execute shell command"，来自引擎元数据）
	Risk        string `json:"risk"`         // 风险等级："write"（引擎声明写操作）/ "unknown"（未声明权限的 MCP 工具）/ "all"（审批模式为 always）
	Arguments   string `json:"arguments"`    // 原始 JSON 参数（前端格式化预览）
}

// ToolCallEndContent 单个工具执行结束事件的内容。
// 前端据此把时间线上对应条目从 running 翻转为成功/失败/被拒绝终态。
type ToolCallEndContent struct {
	ToolCallID string `json:"tool_call_id"` // 工具调用 ID
	OK         bool   `json:"ok"`           // 是否执行成功
	Denied     bool   `json:"denied"`       // 是否被用户拒绝
	Error      string `json:"error"`        // 失败/拒绝原因（ok=true 时为空）
	Preview    string `json:"preview"`      // 结果预览（截断，仅 UI 摘要展示）
}

// SearchResultContent 搜索结果事件的内容
// 修复前 search_result 事件只发射 []search.SearchResult，前端无法区分并发 tool call 的结果
// 修复后使用 struct 包含 tool_call_id，前端可正确关联结果与开始事件
type SearchResultContent struct {
	ToolCallID string                `json:"tool_call_id"` // 工具调用 ID（用于并发 tool call 关联）
	Results    []search.SearchResult `json:"results"`      // 搜索结果数组
}

// TokenSpeedContent 生成速度事件的内容
// C-7 协议唯一事实化：移除 tokens_per_second 重复字段（原为兼容 generation_speed 消费者）
type TokenSpeedContent struct {
	TokensPerSecond float64 `json:"tokensPerSecond"` // 生成速度（tokens/s）
	PredictedN      int     `json:"predictedN"`      // 已生成 token 数
}

// PromptProgressContent 提示词处理进度事件的内容
type PromptProgressContent struct {
	Total     int `json:"total"`     // 总 token 数
	Cache     int `json:"cache"`     // 缓存命中 token 数
	Processed int `json:"processed"` // 已处理 token 数
	TimeMs    int `json:"timeMs"`    // 处理耗时（毫秒）
}

// OutputTruncatedContent 输出截断事件的内容。
// 场景：模型回复因触及 max_tokens 上限（finish_reason=length）而中途停止，
// 用户看到的回复戛然而止却不知原因——显式上报截断事件，
// 避免"静默截断只写日志"的反面模式。
type OutputTruncatedContent struct {
	Reason string `json:"reason"` // 截断原因，当前固定为 "length"
}

// ContextTrimmedContent 上下文裁剪事件的内容
type ContextTrimmedContent struct {
	Reason        string `json:"reason"`         // 裁剪原因
	PromptTokens  int    `json:"prompt_tokens"`  // 裁剪前 prompt token 数
	ContextSize   int    `json:"context_size"`   // 上下文长度
	MessagesAfter int    `json:"messages_after"` // 裁剪后消息数
}

// DeletedContent 删除事件的内容（conversation_deleted / message_deleted 通用）
// 后端可能发送裸 string（ID）或 {"id": "..."} 对象
type DeletedContent struct {
	ID string `json:"id"`
}

// ===== Decode 方法（任务 31.4） =====
// 将 StreamEvent.Content（interface{}）解码为具体类型，便于消费侧类型安全访问。
// 实现：先 Marshal 再 Unmarshal，兼容发送侧 map[string]interface{} 与裸 string 两种形态。
// 注意：当前为类型契约文档，发送侧尚未强制使用 struct，后续渐进迁移。

// decodeContent 将 Content 经 JSON 往返解码到目标 struct 指针
func decodeContent(content any, target any) error {
	data, err := json.Marshal(content)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

// DecodeToolCallStart 解码工具调用开始事件内容
func (e *StreamEvent) DecodeToolCallStart() (ToolCallStartContent, error) {
	var c ToolCallStartContent
	return c, decodeContent(e.Content, &c)
}

// DecodeTokenSpeed 解码生成速度事件内容
func (e *StreamEvent) DecodeTokenSpeed() (TokenSpeedContent, error) {
	var c TokenSpeedContent
	return c, decodeContent(e.Content, &c)
}

// DecodePromptProgress 解码提示词进度事件内容
func (e *StreamEvent) DecodePromptProgress() (PromptProgressContent, error) {
	var c PromptProgressContent
	return c, decodeContent(e.Content, &c)
}

// DecodeContextTrimmed 解码上下文裁剪事件内容
func (e *StreamEvent) DecodeContextTrimmed() (ContextTrimmedContent, error) {
	var c ContextTrimmedContent
	return c, decodeContent(e.Content, &c)
}

// DecodeConversation 解码会话事件内容（conversation_created / conversation_updated）
func (e *StreamEvent) DecodeConversation() (Conversation, error) {
	var c Conversation
	return c, decodeContent(e.Content, &c)
}

// DecodeMessage 解码消息事件内容（assistant_message / user_message）
func (e *StreamEvent) DecodeMessage() (Message, error) {
	var c Message
	return c, decodeContent(e.Content, &c)
}

// DecodeDeleted 解码删除事件内容（conversation_deleted / message_deleted）
// 兼容裸 string（ID）和 {"id": "..."} 两种形态
func (e *StreamEvent) DecodeDeleted() (DeletedContent, error) {
	// 裸 string 形态：直接作为 ID
	if s, ok := e.Content.(string); ok {
		return DeletedContent{ID: s}, nil
	}
	var c DeletedContent
	return c, decodeContent(e.Content, &c)
}

// DecodeString 解码 string 类型内容（token / thinking / error / search_start）
func (e *StreamEvent) DecodeString() (string, error) {
	if s, ok := e.Content.(string); ok {
		return s, nil
	}
	// 兜底：经 JSON 往返（例如 Content 为 []byte 等情况）
	var s string
	return s, decodeContent(e.Content, &s)
}

type AbnormalConversation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}
