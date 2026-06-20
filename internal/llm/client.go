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
	baseURL        string
	apiKey         string
	httpClient     *http.Client
	streamClient   *http.Client
}

func (c *Client) BaseURL() string {
	return c.baseURL
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
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
	VocabType   int     `json:"vocab_type"`
	NVocab      int     `json:"n_vocab"`
	NCtxTrain   int     `json:"n_ctx_train"`
	NEmbd       int     `json:"n_embd"`
	NParams     float64 `json:"n_params"`
	Size        int64   `json:"size"`
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
	ChatTemplateCaps    map[string]bool `json:"chat_template_caps"`
	ChatTemplateToolUse string          `json:"chat_template_tool_use"`
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
			ID           string `json:"id"`
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

type ModelStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Failed  bool   `json:"failed,omitempty"`
	ExitCode int   `json:"exit_code,omitempty"`
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
		for _, d := range raw.Data {
			if d.ID == modelName || FuzzyMatchModelID(d.ID, modelName) {
				found = true
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
					// 但如果 loading 状态持续太久（90秒），可能是子进程崩溃但路由器未更新状态
					// 这种情况发生在子进程因 mmproj 不兼容等原因崩溃，但路由器的监控线程卡住
					elapsed := time.Since(startTime)
					if d.Status.Value == "loading" && elapsed > 90*time.Second {
						log.Warn().Str("model", modelName).Dur("elapsed", elapsed).Msg("[client] WaitForModelLoaded: model stuck in loading state, likely child process crashed")
						return fmt.Errorf("model %s appears stuck in loading state after %v (child process may have crashed)", modelName, elapsed.Round(time.Second))
					}
					if stableCount > 0 {
						log.Debug().Str("model", modelName).Str("status", d.Status.Value).Int("previous_stable", stableCount).Msg("[client] WaitForModelLoaded: model left loaded state, resetting stability")
						stableCount = 0
					}
					time.Sleep(300 * time.Millisecond)
				}
				break
			}
		}

		if !found {
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
