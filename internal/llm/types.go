// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

type ContentPart struct {
	Type       string      `json:"type"`
	Text       string      `json:"text"`
	ImageURL   *ImageURL   `json:"image_url,omitempty"`
	InputAudio *InputAudio `json:"input_audio,omitempty"`
	InputVideo *InputVideo `json:"input_video,omitempty"`
}

type ImageURL struct {
	URL string `json:"url"`
}

type InputAudio struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

// InputVideo 视频输入（llama.cpp 新增独立类型，替代之前用 image_url 传递视频的方式）
// 支持两种格式：
//  1. URL 格式（data URL 或文件路径）：仅设置 URL 字段
//  2. 分离格式：设置 Data（base64）和 Format（如 "mp4"）
type InputVideo struct {
	URL    string `json:"url,omitempty"`
	Data   string `json:"data,omitempty"`
	Format string `json:"format,omitempty"`
}

type ChatMessage struct {
	Role             string     `json:"role"`
	Content          any        `json:"content"`
	ReasoningContent string     `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string     `json:"tool_call_id,omitempty"`
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
	case []any:
		for _, item := range v {
			if part, ok := item.(map[string]any); ok {
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

// NewVideoMessage 构造仅含视频输入的消息（llama.cpp 新增 input_video 独立类型）
func NewVideoMessage(role, text string, videos []InputVideo) ChatMessage {
	parts := make([]ContentPart, 0, len(videos)+1)
	if text == "" {
		text = "."
	}
	parts = append(parts, ContentPart{Type: "text", Text: text})
	for _, video := range videos {
		parts = append(parts, ContentPart{
			Type:       "input_video",
			InputVideo: &InputVideo{URL: video.URL, Data: video.Data, Format: video.Format},
		})
	}
	return ChatMessage{Role: role, Content: parts}
}

// NewMultimodalMessage 构造包含图像、音频、视频的多模态消息
func NewMultimodalMessage(role, text string, imageURLs []string, audios []InputAudio, videos []InputVideo) ChatMessage {
	parts := make([]ContentPart, 0, len(imageURLs)+len(audios)+len(videos)+1)
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
	for _, video := range videos {
		parts = append(parts, ContentPart{
			Type:       "input_video",
			InputVideo: &InputVideo{URL: video.URL, Data: video.Data, Format: video.Format},
		})
	}
	return ChatMessage{Role: role, Content: parts}
}

type ToolCall struct {
	Index    int          `json:"index"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolDefinition struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

type FunctionDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ChatCompletionRequest struct {
	Model            string        `json:"model"`
	Messages         []ChatMessage `json:"messages"`
	Stream           bool          `json:"stream"`
	MaxTokens        int           `json:"max_tokens,omitempty"`
	Temperature      float64       `json:"temperature,omitempty"`
	TopP             float64       `json:"top_p,omitempty"`
	TopK             int           `json:"top_k,omitempty"`
	RepeatPenalty    float64       `json:"repeat_penalty,omitempty"`
	Reasoning        string        `json:"reasoning,omitempty"`
	ReasoningBudget  int           `json:"reasoning_budget_tokens,omitempty"`
	ReasoningControl bool          `json:"reasoning_control,omitempty"`
	// 请求级 reasoning 扩展字段（v9744+）
	ReasoningFormat         string `json:"reasoning_format,omitempty"`           // 请求级覆盖思考格式：none/deepseek/deepseek-legacy
	ReasoningBudgetStartTag string `json:"reasoning_budget_start_tag,omitempty"` // 思考预算区间起始标记
	ReasoningBudgetEndTag   string `json:"reasoning_budget_end_tag,omitempty"`   // 思考预算区间结束标记
	ReasoningInContent      *bool  `json:"reasoning_in_content,omitempty"`       // deepseek-legacy 流式时是否在 content 中保留 think 标签
	// OAI reasoning_effort 扩展（避免与 llama.cpp 服务器级 --reasoning 冲突）
	// llama.cpp #26045：仅 "none" 有语义（设置 enable_thinking=false，按请求关闭推理），
	// 其余值由模型自行解释、插件可忽略。豆芽复用 reasoning=off 状态写入 "none"；
	// 非 none 的思考强度通过 chat_template_kwargs.reasoning_effort 透传给模板（见 service_model.go）。
	ReasoningEffort    string           `json:"reasoning_effort,omitempty"`
	TimingsPerToken    bool             `json:"timings_per_token,omitempty"` // 每个 token 返回 timings 数据，用于实时速度显示
	ReturnProgress     bool             `json:"return_progress,omitempty"`   // 在流式响应中返回 prompt 处理进度
	Tools              []ToolDefinition `json:"tools,omitempty"`
	ToolChoice         any              `json:"tool_choice,omitempty"`         // 工具选择策略："auto"（默认）/"none"/"required" 或 {"type":"function","function":{"name":...}}
	ParallelToolCalls  *bool            `json:"parallel_tool_calls,omitempty"` // 是否允许并发工具调用，nil=服务端默认
	ChatTemplateKwargs map[string]any   `json:"chat_template_kwargs,omitempty"`
	StreamOptions      *StreamOptions   `json:"stream_options,omitempty"`
	// llama.cpp 新增请求参数
	NCacheReuse          int              `json:"n_cache_reuse,omitempty"`          // 请求级 KV 缓存复用块大小
	TMaxPredictMs        int              `json:"t_max_predict_ms,omitempty"`       // 预测时间限制（毫秒）
	Echo                 bool             `json:"echo,omitempty"`                   // 是否回显输入
	GrammarLazy          bool             `json:"grammar_lazy,omitempty"`           // 懒惰语法（仅在需要时应用 grammar）
	GrammarTriggers      []GrammarTrigger `json:"grammar_triggers,omitempty"`       // 语法触发器
	ContinueFinalMessage bool             `json:"continue_final_message,omitempty"` // 继续最终消息
	GenerationPrompt     string           `json:"generation_prompt,omitempty"`      // 生成提示
	PostSamplingProbs    bool             `json:"post_sampling_probs,omitempty"`    // 后采样概率
	// 请求级采样扩展字段（llama.cpp 新增）
	Samplers       []string `json:"samplers,omitempty"`        // 自定义采样器顺序（如 ["top_k","top_p","temperature"]）
	IgnoreEos      bool     `json:"ignore_eos,omitempty"`      // 忽略 EOS 继续生成
	Verbose        bool     `json:"verbose,omitempty"`         // 响应中包含调试信息
	AdaptiveTarget float64  `json:"adaptive_target,omitempty"` // 请求级自适应采样目标（0=禁用）
	AdaptiveDecay  float64  `json:"adaptive_decay,omitempty"`  // 请求级自适应采样衰减（0=禁用）
	// Anthropic 风格 thinking 参数兼容（llama.cpp 自动转换为 reasoning_budget_tokens）
	Thinking *AnthropicThinking `json:"thinking,omitempty"`
	// SSE 流式响应静默 ping 间隔（秒，fdb1db877+ 起为请求级字段）
	// 防止大上下文 prefill 慢时连接被误断：静默超过此间隔时发送 ping，3s 后才 kick
	// nil=用 server 默认 30s，正值启用 ping，-1 禁用
	SsePingInterval *int `json:"sse_ping_interval,omitempty"`
}

// AnthropicThinking Anthropic 风格的思考参数，llama-server 会自动转换为 reasoning_budget_tokens
type AnthropicThinking struct {
	Type         string `json:"type"`          // "enabled" 或 "disabled"
	BudgetTokens int    `json:"budget_tokens"` // 思考 token 预算
}

// StreamOptions 控制 SSE 流式响应的附加选项
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

// GrammarTrigger 语法触发器，用于 grammar_lazy 模式下按条件激活 grammar
type GrammarTrigger struct {
	Type  string `json:"type"`            // 触发类型（如 "word_start"）
	Value string `json:"value,omitempty"` // 触发值
	Token int    `json:"token,omitempty"` // 触发 token ID
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
	ID             string             `json:"id"`
	Choices        []SSEChoice        `json:"choices"`
	Usage          *SSEUsage          `json:"usage,omitempty"`
	Timings        *SSETimings        `json:"timings,omitempty"`
	PromptProgress *SSEPromptProgress `json:"prompt_progress,omitempty"`
}

type SSEUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// SSETimings 解析 llama-server 返回的 timings 字段（包含生成速度等信息）
type SSETimings struct {
	PromptN             int     `json:"prompt_n"`
	PromptMs            float64 `json:"prompt_ms"`
	PromptPerSecond     float64 `json:"prompt_per_second"`
	PredictedN          int     `json:"predicted_n"`
	PredictedMs         float64 `json:"predicted_ms"`
	PredictedPerTokenMs float64 `json:"predicted_per_token_ms"`
	PredictedPerSecond  float64 `json:"predicted_per_second"`
}

// SSEPromptProgress 解析 llama-server 返回的 prompt_progress 字段（包含 prompt 处理进度信息）
type SSEPromptProgress struct {
	Total     int     `json:"total"`
	Cache     int     `json:"cache"`
	Processed int     `json:"processed"`
	TimeMs    float64 `json:"time_ms"`
}

type SSEChoice struct {
	Index        int         `json:"index"`
	Delta        ChatMessage `json:"delta"`
	FinishReason *string     `json:"finish_reason"`
}

type ServerStatus struct {
	Running      bool               `json:"running"`
	ModelReady   bool               `json:"model_ready,omitempty"`
	Error        string             `json:"error,omitempty"`
	Switching    bool               `json:"switching,omitempty"`
	SwitchingTo  string             `json:"switching_to,omitempty"`
	CurrentModel string             `json:"current_model,omitempty"`
	Capabilities *ModelCapabilities `json:"capabilities,omitempty"`
}

const (
	ThinkingModeNone      = "none"
	ThinkingModeTemplate  = "template"
	ThinkingModeReasoning = "reasoning"
)

type ModelCapabilities struct {
	ImageInput        bool   `json:"image_input"`
	AudioInput        bool   `json:"audio_input"`
	VideoInput        bool   `json:"video_input"`
	TextInput         bool   `json:"text_input"`
	Reasoning         bool   `json:"reasoning"`
	MmprojLoaded      bool   `json:"mmproj_loaded"`
	HasMTP            bool   `json:"has_mtp"`
	ThinkingMode      string `json:"thinking_mode"`
	SoftSwitchSupport bool   `json:"soft_switch_support"` // 是否支持 /think /no_think 软开关（目前仅 Qwen3）
	// DefaultThinkingAuto 表示模板默认自主思考档位（enable_thinking 未设置时的默认行为）。
	// 取值："on"（默认自主思考）、"off"（默认不思考）、""（无法判定）。
	// 仅作为能力元数据展示；auto 模式下豆芽不据此干预，尊重模型模板自身的默认行为。
	DefaultThinkingAuto       string  `json:"default_thinking_auto"`
	NParams                   float64 `json:"n_params"`
	ToolCallSupport           bool    `json:"tool_call_support"`            // 模型是否支持 tool call
	SupportsPreserveReasoning bool    `json:"supports_preserve_reasoning"`  // 模型是否支持 --reasoning-preserve
	SupportsParallelToolCalls bool    `json:"supports_parallel_tool_calls"` // 模型是否支持并发工具调用
	SupportsSystemRole        bool    `json:"supports_system_role"`         // 模型是否支持 system role（Gemma 等不支持，需合并到 user）
}

// EmbeddingRequest represents a request to /v1/embeddings
type EmbeddingRequest struct {
	Model          string `json:"model,omitempty"`
	Input          any    `json:"input"`                     // string or []string
	EncodingFormat string `json:"encoding_format,omitempty"` // "float" or "base64"
}

// EmbeddingResponse represents a response from /v1/embeddings
type EmbeddingResponse struct {
	Object string      `json:"object"`
	Data   []Embedding `json:"data"`
	Model  string      `json:"model"`
	Usage  Usage       `json:"usage"`
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

// RerankRequest 表示 /v1/rerank 端点的请求体
type RerankRequest struct {
	Model     string   `json:"model,omitempty"` // 可选，指定 reranker 模型
	Query     string   `json:"query"`           // 查询文本
	Documents []string `json:"documents"`       // 候选文档列表
	TopN      int      `json:"top_n,omitempty"` // 返回的 top-N 结果数
}

// RerankResult 表示单个重排序结果
type RerankResult struct {
	Index          int     `json:"index"`           // 原始文档列表中的索引
	RelevanceScore float64 `json:"relevance_score"` // 相关性分数（越高越相关）
	Document       struct {
		Text string `json:"text"` // 文档文本
	} `json:"document"`
}

// RerankResponse 表示 /v1/rerank 端点的响应体
type RerankResponse struct {
	Model   string         `json:"model"`
	Results []RerankResult `json:"results"`
	Usage   struct {
		PromptTokens int `json:"prompt_tokens"`
		TotalTokens  int `json:"total_tokens"`
	} `json:"usage"`
}

// LoraAdapter LoRA 适配器信息（用于 /lora-adapters 端点）
type LoraAdapter struct {
	ID    int     `json:"id"`
	Path  string  `json:"path"`
	Scale float64 `json:"scale"`
}

// ModelLoadEvent 解析 /models/sse 端点返回的模型加载进度事件
// 实际 SSE 数据格式（新版 llama.cpp 统一事件名为 model_status）：
//
//	{"model":"xxx", "event":"model_status", "data":{"status":"loading", "progress":{"stages":[...], "current":"text_model", "value":0.35}}}
//
// 旧版事件名 status_change / download_finished 仍兼容解析
// value 范围 0-1，需乘以 100 转为百分比
type ModelLoadEvent struct {
	Model           string             `json:"model"`
	Event           string             `json:"event"`
	Data            ModelLoadEventData `json:"data"`
	Status          string             // 从 Data.Status 或顶层 status 推导
	ProgressPercent float64            // 0-100 百分比，从 Data.Progress.Value 转换
}

// ModelLoadEventData /models/sse 事件中的 data 字段
type ModelLoadEventData struct {
	Status   string             `json:"status"`
	Progress *ModelLoadProgress `json:"progress"`
}

// ModelLoadProgress 加载进度信息
type ModelLoadProgress struct {
	Stages  []string `json:"stages"`
	Current string   `json:"current"`
	Value   float64  `json:"value"` // 0-1 范围
}

// SlotInfo slot 状态信息（用于 /slots 端点）
type SlotInfo struct {
	ID           int    `json:"id"`
	Task         string `json:"task"`
	NPrompt      int    `json:"n_prompt"`
	NPredicted   int    `json:"n_predicted"`
	NGpuLayers   int    `json:"n_gpu_layers"`
	Model        string `json:"model"`
	NCacheTokens int    `json:"n_cache_tokens"`
	CacheShift   bool   `json:"cache_shift"`
}
