// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

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
	SearchEnabled  bool         `json:"search_enabled"`
	Images         []string     `json:"images,omitempty"`
	Attachments    []Attachment `json:"attachments,omitempty"`
}

type StreamEvent struct {
	Type           string      `json:"type"`
	Content        interface{} `json:"content"`
	ConversationID string      `json:"conversation_id"`
}

type AbnormalConversation struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Reason string `json:"reason"`
}