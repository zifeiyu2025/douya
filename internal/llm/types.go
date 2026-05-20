// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

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
	TopNSigma     float64          `json:"top_nsigma,omitempty"`
	AdaptiveTarget float64         `json:"adaptive_target,omitempty"`
	AdaptiveDecay float64          `json:"adaptive_decay,omitempty"`
	Tools         []ToolDefinition `json:"tools,omitempty"`
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

type ModelCapabilities struct {
	ImageInput bool `json:"image_input"`
	AudioInput bool `json:"audio_input"`
	TextInput  bool `json:"text_input"`
	Reasoning  bool `json:"reasoning"`
}
