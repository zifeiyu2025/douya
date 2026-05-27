// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import "strings"

type ContentPart struct {
	Type       string      `json:"type"`
	Text       string      `json:"text"`
	ImageURL   *ImageURL   `json:"image_url,omitempty"`
	InputAudio *InputAudio `json:"input_audio,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type InputAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

type ChatMessage struct {
	Role             string        `json:"role"`
	Content          interface{}   `json:"content"`
	ReasoningContent string        `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall    `json:"tool_calls,omitempty"`
	ToolCallID       string        `json:"tool_call_id,omitempty"`
}

func (m *ChatMessage) ContentString() string {
	switch v := m.Content.(type) {
	case string:
		return v
	case []ContentPart:
		for _, part := range v {
			if part.Type == "text" {
				return part.Text
			}
		}
	case []interface{}:
		for _, item := range v {
			if part, ok := item.(map[string]interface{}); ok {
				if part["type"] == "text" {
					if text, ok := part["text"].(string); ok {
						return text
					}
				}
			}
		}
	}
	return ""
}

func NewTextMessage(role, content string) ChatMessage {
	return ChatMessage{Role: role, Content: content}
}

func NewVisionMessage(role, text string, imageURLs []string) ChatMessage {
	parts := make([]ContentPart, 0, len(imageURLs)+1)
	if text == "" {
		text = "."
	}
	parts = append(parts, ContentPart{Type: "text", Text: text})
	for _, url := range imageURLs {
		parts = append(parts, ContentPart{
			Type:     "image_url",
			ImageURL: &ImageURL{URL: url},
		})
	}
	return ChatMessage{Role: role, Content: parts}
}

func NewAudioMessage(role, text string, audios []InputAudio) ChatMessage {
	parts := make([]ContentPart, 0, len(audios)+1)
	if text == "" {
		text = "."
	}
	parts = append(parts, ContentPart{Type: "text", Text: text})
	for _, audio := range audios {
		parts = append(parts, ContentPart{
			Type:       "input_audio",
			InputAudio: &InputAudio{Data: audio.Data, Format: audio.Format},
		})
	}
	return ChatMessage{Role: role, Content: parts}
}

func NewMultimodalMessage(role, text string, imageURLs []string, audios []InputAudio) ChatMessage {
	parts := make([]ContentPart, 0, len(imageURLs)+len(audios)+1)
	if text == "" {
		text = "."
	}
	parts = append(parts, ContentPart{Type: "text", Text: text})
	for _, url := range imageURLs {
		parts = append(parts, ContentPart{
			Type:     "image_url",
			ImageURL: &ImageURL{URL: url},
		})
	}
	for _, audio := range audios {
		parts = append(parts, ContentPart{
			Type:       "input_audio",
			InputAudio: &InputAudio{Data: audio.Data, Format: audio.Format},
		})
	}
	return ChatMessage{Role: role, Content: parts}
}

type ToolCall struct {
	Index   int         `json:"index"`
	ID      string      `json:"id"`
	Type    string      `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Type     string       `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type ChatCompletionRequest struct {
	Model         string           `json:"model"`
	Messages      []ChatMessage    `json:"messages"`
	Stream        bool             `json:"stream"`
	MaxTokens     int              `json:"max_tokens,omitempty"`
	Temperature   float64          `json:"temperature,omitempty"`
	TopP          float64          `json:"top_p,omitempty"`
	TopK          int              `json:"top_k,omitempty"`
	RepeatPenalty float64          `json:"repeat_penalty,omitempty"`
	Reasoning     string           `json:"reasoning,omitempty"`
	ReasoningBudget int            `json:"reasoning_budget,omitempty"`
	Tools         []ToolDefinition       `json:"tools,omitempty"`
	ChatTemplateKwargs map[string]interface{} `json:"chat_template_kwargs,omitempty"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason"`
}

type SSEChunk struct {
	ID      string      `json:"id"`
	Choices []SSEChoice `json:"choices"`
}

type SSEChoice struct {
	Index        int         `json:"index"`
	Delta        ChatMessage `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type ServerStatus struct {
	Running        bool               `json:"running"`
	Error          string             `json:"error,omitempty"`
	Switching      bool               `json:"switching,omitempty"`
	SwitchingTo    string             `json:"switching_to,omitempty"`
	CurrentModel   string             `json:"current_model,omitempty"`
	Capabilities   *ModelCapabilities `json:"capabilities,omitempty"`
}

const (
	ThinkingModeNone     = "none"
	ThinkingModeTemplate = "template"
	ThinkingModeReasoning = "reasoning"
)

type ModelCapabilities struct {
	ImageInput   bool    `json:"image_input"`
	AudioInput   bool    `json:"audio_input"`
	TextInput    bool    `json:"text_input"`
	Reasoning    bool    `json:"reasoning"`
	MmprojLoaded bool    `json:"mmproj_loaded"`
	HasMTP       bool    `json:"has_mtp"`
	ThinkingMode string  `json:"thinking_mode"`
	NParams      float64 `json:"n_params"`
}

// EmbeddingRequest represents a request to /v1/embeddings
type EmbeddingRequest struct {
	Model          string      `json:"model,omitempty"`
	Input          interface{} `json:"input"` // string or []string
	EncodingFormat string      `json:"encoding_format,omitempty"` // "float" or "base64"
}

// EmbeddingResponse represents a response from /v1/embeddings
type EmbeddingResponse struct {
	Object string     `json:"object"`
	Data   []Embedding `json:"data"`
	Model string     `json:"model"`
	Usage  Usage      `json:"usage"`
}

// Embedding represents a single embedding vector
type Embedding struct {
	Object    string    `json:"object"`
	Embedding []float64 `json:"embedding"`
	Index     int       `json:"index"`
}

// Usage represents token usage in the response
type Usage struct {
	PromptTokens int `json:"prompt_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// IsWeakModel 判断当前加载的模型是否为弱模型。
// 弱模型判定标准：
//   - 模型名中包含 MoE 特征关键词（如 "a3b", "a2b", "a1b", "moe", "mixture"），
//     MoE 模型激活参数少，工具调用能力弱
//   - 模型总参数量 NParams 低于 200 亿（20e9）
func IsWeakModel(caps ModelCapabilities, modelName string) bool {
	lowerName := strings.ToLower(modelName)
	moelKeywords := []string{"a3b", "a2b", "a1b", "moe", "mixture"}
	for _, kw := range moelKeywords {
		if strings.Contains(lowerName, kw) {
			return true
		}
	}
	if caps.NParams < 20e9 {
		return true
	}
	return false
}
