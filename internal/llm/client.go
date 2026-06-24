// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"douya/internal/httputil"

	"github.com/rs/zerolog/log"
)

const maxResponseBody = 50 * 1024 * 1024

// readBody 读取 HTTP 响应体，限制最大 50MB 防止内存耗尽。
func readBody(r io.Reader) ([]byte, error) {
	return httputil.ReadBodyLimited(r, maxResponseBody)
}

type Client struct {
	baseURL       string
	apiKey        string
	httpClient    *http.Client
	streamClient  *http.Client
	currentModel  string
	modelMu       sync.RWMutex
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

// HTTPClient 返回配置了超时的 HTTP 客户端实例，供外部包（如 app.go）发起 HTTP 请求时复用
func (c *Client) HTTPClient() *http.Client {
	return c.httpClient
}

// SetCurrentModel 设置当前加载的模型名称（v9744+ API 要求在请求中包含 model 字段）
func (c *Client) SetCurrentModel(modelName string) {
	c.modelMu.Lock()
	defer c.modelMu.Unlock()
	c.currentModel = modelName
}

// GetCurrentModel 获取当前加载的模型名称
func (c *Client) GetCurrentModel() string {
	c.modelMu.RLock()
	defer c.modelMu.RUnlock()
	return c.currentModel
}

func NewClient(baseURL string, apiKey string) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		apiKey:       apiKey,
		httpClient:   &http.Client{Timeout: 300 * time.Second},
		streamClient: &http.Client{Timeout: 900 * time.Second},
	}
}

// setAuthHeader 为请求设置认证 header（如果配置了 API Key）
func (c *Client) setAuthHeader(req *http.Request) {
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// SetAuthHeader 公开方法，供外部包（如 app.go）设置认证 header
func (c *Client) SetAuthHeader(req *http.Request) {
	c.setAuthHeader(req)
}

func (c *Client) StreamChat(ctx context.Context, req *ChatCompletionRequest, onToken func(chunk SSEChunk) error) error {
	req.Stream = true
	req.StreamOptions = &StreamOptions{IncludeUsage: true}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.streamClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	// SSE 单行最大 10MB，超过此限制的行会被截断并记录警告
	const maxSSELineSize = 10 * 1024 * 1024
	scanner.Buffer(make([]byte, 0, 1024*1024), maxSSELineSize)
	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		if data == "[DONE]" {
			break
		}

		var chunk SSEChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if onToken != nil {
			if err := onToken(chunk); err != nil {
				return fmt.Errorf("onToken callback error: %w", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading SSE stream: %w", err)
	}

	return nil
}

func FixUTF8(s string) string {
	if utf8.ValidString(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteString("\uFFFD")
		} else {
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

func TruncateIncompleteUTF8(s string) (valid string, pending string) {
	if len(s) == 0 {
		return "", ""
	}
	for i := len(s) - 1; i >= 0 && i >= len(s)-4; i-- {
		if utf8.RuneStart(s[i]) {
			if utf8.FullRuneInString(s[i:]) {
				return s, ""
			}
			return s[:i], s[i:]
		}
	}
	return s, ""
}

func (c *Client) Chat(ctx context.Context, req *ChatCompletionRequest) (*ChatCompletionResponse, error) {
	req.Stream = false

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var result ChatCompletionResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

// AnthropicMessages 代理 Anthropic Messages API 请求
// 将原始请求体转发到 /v1/messages 端点，返回原始响应体
// 生活类比：就像快递中转站，原封不动地把包裹从发件人转给收件人，不拆包不改装
func (c *Client) AnthropicMessages(ctx context.Context, body []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic messages request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic messages request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read anthropic messages response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic messages returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// AnthropicCountTokens 代理 Anthropic token 计数请求
// 将原始请求体转发到 /v1/messages/count_tokens 端点，返回原始响应体
func (c *Client) AnthropicCountTokens(ctx context.Context, body []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages/count_tokens", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create anthropic count tokens request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic count tokens request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read anthropic count tokens response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("anthropic count tokens returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// BuiltInTools 代理内置工具请求
// 将原始请求体转发到 /tools 端点，返回原始响应体
func (c *Client) BuiltInTools(ctx context.Context, body []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tools", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create built-in tools request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("built-in tools request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read built-in tools response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("built-in tools returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// Embedding sends a request to /v1/embeddings and returns vector embeddings.
// input can be a string or []string.
func (c *Client) Embedding(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	var result EmbeddingResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &result, nil
}

func (c *Client) HealthCheck(ctx context.Context) error {
	// 优先请求 /v1/health 端点（新版 llama-server 推荐端点）
	// 如果失败（404 或连接错误），回退到 /health 端点
	if err := c.healthCheckOnce(ctx, "/v1/health"); err == nil {
		return nil
	}
	return c.healthCheckOnce(ctx, "/health")
}

// healthCheckOnce 向指定端点发起健康检查请求
func (c *Client) healthCheckOnce(ctx context.Context, endpoint string) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+endpoint, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	return nil
}

// StopThinking 发送 POST /v1/chat/completions/control 请求，强制结束当前思考块。
// 用于实时推理控制：用户在流式推理过程中点击"直接回答"按钮时调用。
// 请求体格式（v9744+）：{"id": "chatcmpl-xxx", "action": "reasoning_end", "model": "xxx"}
// 前提：原始聊天请求必须带 reasoning_control: true，否则此端点无效。
func (c *Client) StopThinking(ctx context.Context, completionID string) error {
	body, err := json.Marshal(map[string]interface{}{
		"id":     completionID,
		"action": "reasoning_end",
		"model":  c.GetCurrentModel(),
	})
	if err != nil {
		return fmt.Errorf("failed to marshal stop thinking request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions/control", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create stop thinking request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("stop thinking request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return fmt.Errorf("stop thinking returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Info().Str("completion_id", completionID).Msg("[client] StopThinking: reasoning_end sent successfully")
	return nil
}

// Rerank 调用 /v1/rerank 端点对文档进行重排序。
// query: 查询文本；documents: 候选文档列表；topN: 返回的 top-N 结果数。
// 返回重排序后的文档索引和相关性分数（按分数降序排列）。
func (c *Client) Rerank(ctx context.Context, query string, documents []string, topN int) ([]RerankResult, error) {
	if len(documents) == 0 {
		return nil, nil
	}

	req := RerankRequest{
		Query:     query,
		Documents: documents,
		TopN:      topN,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal rerank request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/rerank", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create rerank request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("rerank request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read rerank response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("rerank returned status %d: %s", resp.StatusCode, string(respBody))
	}

	var result RerankResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal rerank response: %w", err)
	}

	log.Info().Int("results", len(result.Results)).Msg("[client] Rerank: success")
	return result.Results, nil
}

type ModelInfo struct {
	Name            string    `json:"name"`
	Capabilities    []string  `json:"capabilities"`
	InputModalities []string  `json:"input_modalities"`
	Meta            ModelMeta `json:"meta"`
}

func DetectCapabilities(info ModelInfo) ModelCapabilities {
	caps := ModelCapabilities{
		TextInput: true,
	}

	for _, c := range info.Capabilities {
		lc := strings.ToLower(c)
		switch {
		case lc == "vision":
			caps.ImageInput = true
		case lc == "audio", lc == "speech":
			caps.AudioInput = true
		case lc == "video":
			caps.VideoInput = true
		case lc == "multimodal":
			caps.ImageInput = true
		}
	}

	for _, m := range info.InputModalities {
		lm := strings.ToLower(m)
		switch {
		case lm == "image":
			caps.ImageInput = true
		case lm == "audio":
			caps.AudioInput = true
		case lm == "video":
			caps.VideoInput = true
		}
	}

	return caps
}

type ModelMeta struct {
	VocabType int     `json:"vocab_type"`
	NVocab    int     `json:"n_vocab"`
	NCtxTrain int     `json:"n_ctx_train"`
	NEmbd     int     `json:"n_embd"`
	NParams   float64 `json:"n_params"`
	Size      int64   `json:"size"`
}

func (c *Client) GetModelInfo(ctx context.Context) (*ModelInfo, error) {
	return c.GetModelInfoByName(ctx, "")
}

func (c *Client) GetModelInfoByName(ctx context.Context, modelName string) (*ModelInfo, error) {
	// If specific model requested, try direct endpoint first (avoids fetching all models)
	if modelName != "" {
		directURL := c.baseURL + "/v1/models/" + url.PathEscape(modelName)
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, directURL, nil)
		if err != nil {
			return nil, err
		}
		c.setAuthHeader(httpReq)
		resp, err := c.httpClient.Do(httpReq)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body, err := readBody(resp.Body)
				if err == nil {
					var target struct {
						ID           string    `json:"id"`
						Capabilities []string  `json:"capabilities"`
						Meta         ModelMeta `json:"meta"`
						Architecture struct {
							InputModalities  []string `json:"input_modalities"`
							OutputModalities []string `json:"output_modalities"`
						} `json:"architecture"`
					}
					if json.Unmarshal(body, &target) == nil && target.ID != "" {
						caps := target.Capabilities
						if len(caps) == 0 {
							var rawGeneric struct {
								Capabilities json.RawMessage `json:"capabilities"`
							}
							if json.Unmarshal(body, &rawGeneric) == nil {
								caps = parseCapabilitiesRaw(rawGeneric.Capabilities)
							}
						}
						log.Info().Str("model", target.ID).Strs("caps", caps).Msg("[client] GetModelInfoByName: direct hit")
						return &ModelInfo{
							Name:            target.ID,
							Capabilities:    caps,
							InputModalities: target.Architecture.InputModalities,
							Meta:            target.Meta,
						}, nil
					}
				}
			}
		}
		// Direct endpoint not available or failed, fall through to full list
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := readBody(resp.Body)
		return nil, fmt.Errorf("models endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Debug().Str("raw_response", string(body)).Msg("[client] /v1/models raw response")

	var raw struct {
		Data []struct {
			ID           string    `json:"id"`
			Capabilities []string  `json:"capabilities"`
			Meta         ModelMeta `json:"meta"`
			Architecture struct {
				InputModalities  []string `json:"input_modalities"`
				OutputModalities []string `json:"output_modalities"`
			} `json:"architecture"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	if len(raw.Data) == 0 {
		return nil, fmt.Errorf("no models returned")
	}

	target := &raw.Data[0]
	if modelName != "" {
		for i := range raw.Data {
			if raw.Data[i].ID == modelName {
				target = &raw.Data[i]
				break
			}
		}
	}

	caps := target.Capabilities
	if len(caps) == 0 {
		var rawGeneric struct {
			Data []struct {
				ID           string          `json:"id"`
				Capabilities json.RawMessage `json:"capabilities"`
			} `json:"data"`
		}
		if err := json.Unmarshal(body, &rawGeneric); err == nil {
			for i := range rawGeneric.Data {
				if rawGeneric.Data[i].ID == target.ID {
					caps = parseCapabilitiesRaw(rawGeneric.Data[i].Capabilities)
					break
				}
			}
		}
	}

	log.Info().Str("model", modelName).Str("target_id", target.ID).Strs("caps", caps).Int("raw_data_count", len(raw.Data)).Msg("[client] GetModelInfoByName")

	return &ModelInfo{
		Name:            target.ID,
		Capabilities:    caps,
		InputModalities: target.Architecture.InputModalities,
		Meta:            target.Meta,
	}, nil
}

func parseCapabilitiesRaw(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	var strArr []string
	if err := json.Unmarshal(raw, &strArr); err == nil {
		return strArr
	}

	var objArr []map[string]interface{}
	if err := json.Unmarshal(raw, &objArr); err == nil {
		var result []string
		for _, obj := range objArr {
			if name, ok := obj["name"].(string); ok {
				result = append(result, name)
			}
			if t, ok := obj["type"].(string); ok {
				result = append(result, t)
			}
		}
		return result
	}

	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return []string{single}
	}

	return nil
}

type ListedModel struct {
	ID           string   `json:"id"`
	Capabilities []string `json:"capabilities"`
	Status       string   `json:"status"`
}

type ServerProps struct {
	Modalities struct {
		Vision bool `json:"vision"`
		Audio  bool `json:"audio"`
		Video  bool `json:"video"`
	} `json:"modalities"`
	ChatTemplateCaps    ChatTemplateCaps `json:"chat_template_caps"`
	ChatTemplateToolUse string          `json:"chat_template_tool_use"`
	BuildInfo           string          `json:"build_info,omitempty"`
	IsSleeping          bool            `json:"is_sleeping,omitempty"`
	CorsProxyEnabled    bool            `json:"cors_proxy_enabled,omitempty"`
}

// ChatTemplateCaps 对应 llama.cpp 最新版 /props 返回的 chat_template_caps 字段
// 包含模型模板能力声明，用于判断工具调用、推理保留等能力
type ChatTemplateCaps struct {
	SupportsTools              bool `json:"supports_tools"`
	SupportsToolCalls          bool `json:"supports_tool_calls"`
	SupportsSystemRole         bool `json:"supports_system_role"`
	SupportsParallelToolCalls  bool `json:"supports_parallel_tool_calls"`
	SupportsPreserveReasoning  bool `json:"supports_preserve_reasoning"`
	SupportsStringContent      bool `json:"supports_string_content"`
	SupportsTypedContent       bool `json:"supports_typed_content"`
	SupportsObjectArguments    bool `json:"supports_object_arguments"`
}

func (c *Client) GetServerProps(ctx context.Context, modelName string) (*ServerProps, error) {
	propsURL := c.baseURL + "/props"
	if modelName != "" {
		propsURL += "?model=" + url.QueryEscape(modelName)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, propsURL, nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("props endpoint returned status %d", resp.StatusCode)
	}

	body, err := readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	rawLog := string(body)
	if len(rawLog) > 500 {
		rawLog = rawLog[:500] + "..."
	}
	log.Debug().Str("raw_response", rawLog).Msg("[client] /props raw response")

	var props ServerProps
	if err := json.Unmarshal(body, &props); err != nil {
		return nil, err
	}

	return &props, nil
}

func (c *Client) LoadModel(ctx context.Context, modelName string) error {
	body, err := json.Marshal(map[string]string{"model": modelName})
	if err != nil {
		return fmt.Errorf("failed to marshal load model request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/models/load", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create load model request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("load model request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := readBody(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("load model returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Info().Str("model", modelName).Int("status", resp.StatusCode).Str("body", string(respBody)).Msg("[client] LoadModel response")

	return nil
}

// WatchModelLoadProgress 通过 /models/sse 端点实时监听模型加载进度
// 当模型状态变为 "loaded" 或上下文被取消时返回
//
// llama.cpp 最新版本将事件名统一为 "model_status"（之前为 "status_change"/"download_finished"）
// 本方法兼容新旧两种事件名，确保跨版本稳定性
func (c *Client) WatchModelLoadProgress(ctx context.Context, modelName string, onProgress func(event ModelLoadEvent)) error {
	url := fmt.Sprintf("%s/models/sse", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request failed: %w", err)
	}
	c.setAuthHeader(req)

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	// SSE 单行最大 10MB，与 StreamChat 保持一致
	const maxSSELineSize = 10 * 1024 * 1024
	scanner.Buffer(make([]byte, 0, 1024*1024), maxSSELineSize)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			return nil
		}

		var event ModelLoadEvent
		if err := json.Unmarshal([]byte(data), &event); err != nil {
			continue
		}

		// 只关注目标模型的事件
		if event.Model != modelName {
			continue
		}

		// 从嵌套的 data 字段推导 status 和 progress
		// 新版 SSE 格式: {"model":"xxx", "event":"model_status", "data":{"status":"loading", "progress":{"stages":[...], "current":"text_model", "value":0.35}}}
		// 旧版 SSE 格式: {"model":"xxx", "event":"status_change", "data":{"status":"loading", "progress":{"value":0.35}}}
		// 兼容处理：无论事件名是 model_status、status_change 还是 download_finished，都统一解析 data 字段
		if event.Data.Status != "" {
			event.Status = event.Data.Status
		}
		if event.Data.Progress != nil {
			event.ProgressPercent = event.Data.Progress.Value * 100 // 0-1 → 0-100
		}

		if onProgress != nil {
			onProgress(event)
		}

		// 模型加载完成（llama.cpp 最新版状态名称：loaded，旧版为 running）
		if event.Status == "loaded" || event.Status == "running" {
			return nil
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading SSE stream: %w", err)
	}

	return nil
}

// ReloadPresets 通知路由器重新加载 preset 文件
// 在修改了 router-preset.ini 后调用，使路由器感知到配置变化
// 原版 llama.cpp 使用 GET /v1/models?reload 来触发 preset 重载
func (c *Client) ReloadPresets(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models?reload=1", nil)
	if err != nil {
		return fmt.Errorf("failed to create reload presets request: %w", err)
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("reload presets request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return fmt.Errorf("reload presets returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Info().Msg("[client] ReloadPresets: presets reloaded successfully")
	return nil
}

func (c *Client) UnloadModel(ctx context.Context, modelName string) error {
	body, err := json.Marshal(map[string]string{"model": modelName})
	if err != nil {
		return fmt.Errorf("failed to marshal unload model request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/models/unload", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create unload model request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("unload model request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return fmt.Errorf("unload model returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) GetModelsList(ctx context.Context) ([]ListedModel, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := readBody(resp.Body)
		return nil, fmt.Errorf("models endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Data []struct {
			ID           string   `json:"id"`
			Capabilities []string `json:"capabilities"`
			Status       struct {
				Value string `json:"value"`
			} `json:"status"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	models := make([]ListedModel, 0, len(raw.Data))
	for _, d := range raw.Data {
		models = append(models, ListedModel{
			ID:           d.ID,
			Capabilities: d.Capabilities,
			Status:       d.Status.Value,
		})
	}

	return models, nil
}

func (c *Client) ReloadModels(ctx context.Context) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models?reload", nil)
	if err != nil {
		return err
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("reload models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := readBody(resp.Body)
		return fmt.Errorf("reload models returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// DeleteModel 调用 DELETE /models 删除模型（从列表中移除并卸载）
func (c *Client) DeleteModel(ctx context.Context, modelName string) error {
	body, err := json.Marshal(map[string]string{"model": modelName})
	if err != nil {
		return fmt.Errorf("failed to marshal delete model request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, c.baseURL+"/models", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create delete model request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("delete model request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return fmt.Errorf("delete model returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Info().Str("model", modelName).Msg("[client] DeleteModel: model deleted")
	return nil
}

// DownloadModel 触发模型下载（非阻塞，进度通过 /models/sse 跟踪）
// 模型名格式：HF 仓库格式，如 "ggml-org/gemma-3-4b-it-GGUF:Q4_K_M"
func (c *Client) DownloadModel(ctx context.Context, modelName string) error {
	body, err := json.Marshal(map[string]string{"model": modelName})
	if err != nil {
		return fmt.Errorf("failed to marshal download model request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/models", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create download model request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("download model request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return fmt.Errorf("download model returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Info().Str("model", modelName).Msg("[client] DownloadModel: download started")
	return nil
}

// CountTokens 调用 /v1/chat/completions/input_tokens 估算消息的 token 数量
func (c *Client) CountTokens(ctx context.Context, messages []ChatMessage) (int, error) {
	// v9744+ 要求在请求体中包含 model 字段
	reqBody := map[string]interface{}{
		"messages": messages,
		"model":    c.GetCurrentModel(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal count tokens request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions/input_tokens", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create count tokens request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("count tokens request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return 0, fmt.Errorf("count tokens returned status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := readBody(resp.Body)
	if err != nil {
		return 0, err
	}

	var result struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, fmt.Errorf("failed to parse count tokens response: %w", err)
	}

	return result.InputTokens, nil
}

// CountTokensViaInputTokens 通过 /v1/chat/completions/input_tokens 端点获取精确 token 计数
// 比 /tokenize 更精确，因为会经过完整的 chat template 处理
func (c *Client) CountTokensViaInputTokens(ctx context.Context, messages []ChatMessage) (int, error) {
	body, err := json.Marshal(map[string]interface{}{
		"messages": messages,
	})
	if err != nil {
		return 0, fmt.Errorf("failed to marshal input tokens request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions/input_tokens", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("failed to create input tokens request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return 0, fmt.Errorf("input tokens request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read input tokens response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("input tokens request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Object      string `json:"object"`
		InputTokens int    `json:"input_tokens"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, fmt.Errorf("failed to parse input tokens response: %w", err)
	}

	return result.InputTokens, nil
}

// GetLoraAdapters 调用 GET /lora-adapters 获取 LoRA 适配器列表
func (c *Client) GetLoraAdapters(ctx context.Context) ([]LoraAdapter, error) {
	// v9744+ 要求在查询参数中包含 model 字段
	reqURL := c.baseURL + "/lora-adapters"
	if model := c.GetCurrentModel(); model != "" {
		reqURL += "?model=" + url.QueryEscape(model)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create get lora adapters request: %w", err)
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("get lora adapters request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := readBody(resp.Body)
		return nil, fmt.Errorf("get lora adapters returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	var adapters []LoraAdapter
	if err := json.Unmarshal(body, &adapters); err != nil {
		return nil, fmt.Errorf("failed to parse lora adapters response: %w", err)
	}

	return adapters, nil
}

// SetLoraAdapters 调用 POST /lora-adapters 设置 LoRA 适配器（运行时热切换）
func (c *Client) SetLoraAdapters(ctx context.Context, adapters []LoraAdapter) error {
	body, err := json.Marshal(adapters)
	if err != nil {
		return fmt.Errorf("failed to marshal set lora adapters request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/lora-adapters", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create set lora adapters request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("set lora adapters request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return fmt.Errorf("set lora adapters returned status %d: %s", resp.StatusCode, string(respBody))
	}

	log.Info().Int("adapters", len(adapters)).Msg("[client] SetLoraAdapters: lora adapters updated")
	return nil
}

// GetSlots 调用 GET /slots 获取所有 slot 的状态信息
func (c *Client) GetSlots(ctx context.Context) ([]SlotInfo, error) {
	// v9744+ 要求在查询参数中包含 model 字段
	reqURL := c.baseURL + "/slots"
	if model := c.GetCurrentModel(); model != "" {
		reqURL += "?model=" + url.QueryEscape(model)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create get slots request: %w", err)
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("get slots request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := readBody(resp.Body)
		return nil, fmt.Errorf("get slots returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	var slots []SlotInfo
	if err := json.Unmarshal(body, &slots); err != nil {
		return nil, fmt.Errorf("failed to parse slots response: %w", err)
	}

	return slots, nil
}

// Tokenize 调用 /tokenize 对文本进行分词，返回 token ID 列表
func (c *Client) Tokenize(ctx context.Context, text string) ([]int, error) {
	// v9744+ 要求在请求体中包含 model 字段
	reqBody := map[string]interface{}{
		"content": text,
		"model":   c.GetCurrentModel(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal tokenize request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/tokenize", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create tokenize request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("tokenize request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return nil, fmt.Errorf("tokenize returned status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tokens []int `json:"tokens"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse tokenize response: %w", err)
	}

	return result.Tokens, nil
}

// ApplyTemplate 调用 /apply-template 应用聊天模板，返回格式化后的 prompt
func (c *Client) ApplyTemplate(ctx context.Context, messages []ChatMessage) (string, error) {
	// v9744+ 要求在请求体中包含 model 字段
	reqBody := map[string]interface{}{
		"messages": messages,
		"model":    c.GetCurrentModel(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal apply template request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/apply-template", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to create apply template request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("apply template request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := readBody(resp.Body)
		return "", fmt.Errorf("apply template returned status %d: %s", resp.StatusCode, string(respBody))
	}

	respBody, err := readBody(resp.Body)
	if err != nil {
		return "", err
	}

	var result struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to parse apply template response: %w", err)
	}

	return result.Prompt, nil
}

type ModelStatus struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	Failed   bool   `json:"failed,omitempty"`
	ExitCode int    `json:"exit_code,omitempty"`
}

func (c *Client) GetModelStatus(ctx context.Context, modelName string) (*ModelStatus, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := readBody(resp.Body)
		return nil, fmt.Errorf("models endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := readBody(resp.Body)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Data []struct {
			ID     string `json:"id"`
			Status struct {
				Value    string `json:"value"`
				Failed   bool   `json:"failed,omitempty"`
				ExitCode int    `json:"exit_code,omitempty"`
			} `json:"status"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	for _, d := range raw.Data {
		if d.ID == modelName || FuzzyMatchModelID(d.ID, modelName) {
			return &ModelStatus{
				Name:     d.ID,
				Status:   d.Status.Value,
				Failed:   d.Status.Failed,
				ExitCode: d.Status.ExitCode,
			}, nil
		}
	}

	return nil, fmt.Errorf("model %s not found in models list", modelName)
}

// FuzzyMatchModelID 模糊匹配模型 ID
// 处理 llama-server 返回的模型 ID 与派生名不一致的情况
// 例如: ID="default", name="Qwen3.6-35B-A3B-UD" 无法模糊匹配（"default" 太通用）
// 例如: ID="Qwen3.6-35B-A3B-UD-Q4_K_XL", name="Qwen3.6-35B-A3B-UD" 可以匹配
func FuzzyMatchModelID(id, name string) bool {
	idLower := strings.ToLower(id)
	nameLower := strings.ToLower(name)
	// 互相包含（但排除 "default" 这种太通用的 ID）
	if idLower != "default" && nameLower != "default" {
		if strings.Contains(idLower, nameLower) || strings.Contains(nameLower, idLower) {
			return true
		}
	}
	return false
}

func (c *Client) WaitForModelLoaded(ctx context.Context, modelName string, timeout time.Duration, onProgress ...func(pollCount int, status string)) error {
	deadline := time.Now().Add(timeout)
	pollClient := &http.Client{Timeout: 3 * time.Second}
	pollCount := 0

	// 稳定性检查：模型变为 loaded/sleeping 后，再确认几次状态保持稳定
	// 防止子进程短暂 loaded 后崩溃导致误判
	stableCount := 0
	const requiredStablePolls = 2 // 连续 2 次轮询状态稳定才认为真正就绪
	const stableInterval = 500 * time.Millisecond

	// 详细日志：加载超过 30 秒时每 30 秒记录一次状态
	startTime := time.Now()
	lastDetailedLogTime := startTime
	const detailedLogInterval = 30 * time.Second

	// 子进程崩溃检测状态
	// VRAM 释放监控：检测子进程崩溃后 VRAM 被操作系统回收
	vramSeenOccupied := false      // 是否曾经检测到 VRAM 被占用
	vramReleaseCount := 0          // VRAM 释放确认计数（避免单次抖动误判）
	const vramReleaseThreshold = 3 // 连续 3 次检测到 VRAM 释放才确认崩溃（约 1 秒）
	// 模型消失快速失败：检测模型从列表中消失
	modelSeenBefore := false // 模型是否曾经在 /v1/models 列表中

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
		if err != nil {
			return err
		}
		c.setAuthHeader(httpReq)

		resp, err := pollClient.Do(httpReq)
		if err != nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}

		defer resp.Body.Close()
		body, readErr := readBody(resp.Body)

		if resp.StatusCode != http.StatusOK || readErr != nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}

		var raw struct {
			Data []struct {
				ID     string `json:"id"`
				Status struct {
					Value    string `json:"value"`
					Failed   bool   `json:"failed,omitempty"`
					ExitCode int    `json:"exit_code,omitempty"`
				} `json:"status"`
			} `json:"data"`
		}

		if json.Unmarshal(body, &raw) != nil {
			time.Sleep(300 * time.Millisecond)
			continue
		}

		pollCount++

		// 首次轮询时记录所有模型 ID 和状态，帮助诊断模型名不匹配问题
		if pollCount == 1 {
			allModels := make([]string, 0, len(raw.Data))
			for _, d := range raw.Data {
				allModels = append(allModels, fmt.Sprintf("%s=%s", d.ID, d.Status.Value))
			}
			log.Info().Str("target", modelName).Strs("models", allModels).Msg("[client] WaitForModelLoaded: first poll result")
		}

		// 进度回调：通知调用者当前轮询状态
		if len(onProgress) > 0 && onProgress[0] != nil {
			statusValue := "polling"
			for _, d := range raw.Data {
				if d.ID == modelName || FuzzyMatchModelID(d.ID, modelName) {
					statusValue = d.Status.Value
					break
				}
			}
			onProgress[0](pollCount, statusValue)

			// 详细日志：加载超过 30 秒时每 30 秒记录一次状态
			now := time.Now()
			if now.Sub(lastDetailedLogTime) >= detailedLogInterval {
				log.Info().
					Str("model", modelName).
					Str("status", statusValue).
					Int("polls", pollCount).
					Dur("elapsed", now.Sub(startTime)).
					Msg("[client] WaitForModelLoaded: long-running load")
				lastDetailedLogTime = now
			}
		}
		found := false
		modelLoaded := false
		for _, d := range raw.Data {
			if d.ID == modelName || FuzzyMatchModelID(d.ID, modelName) {
				found = true
				modelSeenBefore = true
				// 检测 failed 字段：子进程崩溃后路由器可能将状态设为 unloaded+failed
				// 也可能卡在 loading 状态但 failed 已被设置
				if d.Status.Failed {
					return fmt.Errorf("model %s failed to load (exit_code=%d)", modelName, d.Status.ExitCode)
				}
				// 每 10 次轮询记录一次状态，帮助排查加载卡住的问题
				if pollCount%10 == 1 {
					log.Debug().Str("model", modelName).Str("status", d.Status.Value).Int("poll", pollCount).Msg("[client] WaitForModelLoaded: polling")
				}
				switch d.Status.Value {
				case "loaded", "sleeping":
					// loaded = 模型已加载就绪
					// sleeping = 模型已加载但处于休眠状态（仍在 VRAM 中，新请求会自动唤醒）
					modelLoaded = true
					stableCount++
					if stableCount >= requiredStablePolls {
						log.Info().Str("model", modelName).Str("status", d.Status.Value).Int("polls", pollCount).Msg("[client] WaitForModelLoaded: model is stable")
						return nil
					}
					log.Debug().Str("model", modelName).Str("status", d.Status.Value).Int("stable", stableCount).Int("required", requiredStablePolls).Msg("[client] WaitForModelLoaded: stability check")
					// 稳定性检查期间使用较长间隔
					time.Sleep(stableInterval)
				case "failed":
					return fmt.Errorf("model %s failed to load", modelName)
				case "unloaded":
					// unloaded 可能是子进程崩溃（exit_code != 0），也可能是初始状态
					// 如果 exit_code 非零，说明子进程确实崩溃了
					if d.Status.ExitCode != 0 {
						return fmt.Errorf("model %s crashed during loading (exit_code=%d)", modelName, d.Status.ExitCode)
					}
					// 模型曾经加载后又卸载了（子进程崩溃），重置稳定性计数
					if stableCount > 0 {
						log.Warn().Str("model", modelName).Int("previous_stable", stableCount).Msg("[client] WaitForModelLoaded: model became unloaded after being loaded (child process crash?)")
						stableCount = 0
					}
					// 使用正常轮询间隔
					time.Sleep(300 * time.Millisecond)
				default:
					// loading 等其他状态，继续等待
					// 注意：不再提前终止 loading 状态，只依赖总超时保护
					// 子进程崩溃会通过 status=failed 或 status=unloaded+exit_code≠0 检测
					// 或通过下方的 VRAM 释放监控检测
					if stableCount > 0 {
						log.Debug().Str("model", modelName).Str("status", d.Status.Value).Int("previous_stable", stableCount).Msg("[client] WaitForModelLoaded: model left loaded state, resetting stability")
						stableCount = 0
					}
					time.Sleep(300 * time.Millisecond)
				}
				break
			}
		}

		// VRAM 释放检测：子进程崩溃后 VRAM 会被操作系统回收
		// 只在模型未 loaded 时检测，避免正常加载后误判
		if !modelLoaded {
			vramFree, vramErr := checkVRAMFree()
			if vramErr == nil {
				if !vramFree {
					// VRAM 被占用，子进程在运行
					if !vramSeenOccupied {
						log.Debug().Str("model", modelName).Msg("[client] WaitForModelLoaded: VRAM occupied detected")
					}
					vramSeenOccupied = true
					vramReleaseCount = 0
				} else if vramSeenOccupied {
					// VRAM 从占用变为空闲，可能子进程崩溃
					vramReleaseCount++
					log.Warn().Str("model", modelName).Int("release_count", vramReleaseCount).Int("threshold", vramReleaseThreshold).Msg("[client] WaitForModelLoaded: VRAM released after being occupied (possible crash)")
					if vramReleaseCount >= vramReleaseThreshold {
						return fmt.Errorf("model %s crashed during loading (VRAM released after being occupied)", modelName)
					}
				}
			}
		}

		if !found {
			// 模型曾经在列表中，现在消失了，子进程崩溃
			if modelSeenBefore {
				log.Warn().Str("model", modelName).Msg("[client] WaitForModelLoaded: model disappeared from list (process crashed)")
				return fmt.Errorf("model %s disappeared from model list (process crashed)", modelName)
			}
			// 模型名称未匹配，记录日志帮助调试
			if pollCount <= 5 {
				ids := make([]string, 0, len(raw.Data))
				for _, d := range raw.Data {
					ids = append(ids, d.ID)
				}
				log.Debug().Str("model", modelName).Strs("available_ids", ids).Int("poll", pollCount).Msg("[client] WaitForModelLoaded: model not found in response, retrying")
			}
			time.Sleep(300 * time.Millisecond)
		}
	}

	return fmt.Errorf("model %s did not become loaded within %v", modelName, timeout)
}
