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
	"github.com/rs/zerolog/log"
)

type Client struct {
	baseURL        string
	httpClient     *http.Client
	streamClient   *http.Client
}

func (c *Client) BaseURL() string {
	return c.baseURL
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:      strings.TrimRight(baseURL, "/"),
		httpClient:   &http.Client{Timeout: 300 * time.Second},
		streamClient: &http.Client{Timeout: 900 * time.Second},
	}
}

func (c *Client) StreamChat(ctx context.Context, req *ChatCompletionRequest, onToken func(chunk SSEChunk) error) error {
	req.Stream = true

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.streamClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(respBody))
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)
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

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
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

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
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
		resp, err := c.httpClient.Do(httpReq)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				body, err := io.ReadAll(resp.Body)
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

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	log.Info().Str("raw_response", string(body)).Msg("[client] /v1/models raw response")

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
	ChatTemplateCaps map[string]bool `json:"chat_template_caps"`
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

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("props endpoint returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	rawLog := string(body)
	if len(rawLog) > 500 {
		rawLog = rawLog[:500] + "..."
	}
	log.Info().Str("raw_response", rawLog).Msg("[client] /props raw response")

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

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("load model request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("load model returned status %d: %s", resp.StatusCode, string(respBody))
	}

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

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("unload model request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unload model returned status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

func (c *Client) GetModelsList(ctx context.Context) ([]ListedModel, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
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

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("reload models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("reload models returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

type ModelStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

func (c *Client) GetModelStatus(ctx context.Context, modelName string) (*ModelStatus, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var raw struct {
		Data []struct {
			ID     string `json:"id"`
			Status struct {
				Value string `json:"value"`
			} `json:"status"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	for _, d := range raw.Data {
		if d.ID == modelName {
			return &ModelStatus{
				Name:   d.ID,
				Status: d.Status.Value,
			}, nil
		}
	}

	return nil, fmt.Errorf("model %s not found in models list", modelName)
}

func (c *Client) WaitForModelLoaded(ctx context.Context, modelName string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	pollClient := &http.Client{Timeout: 3 * time.Second}

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

		resp, err := pollClient.Do(httpReq)
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK || readErr != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		var raw struct {
			Data []struct {
				ID     string `json:"id"`
				Status struct {
					Value  string `json:"value"`
					Failed bool   `json:"failed,omitempty"`
				} `json:"status"`
			} `json:"data"`
		}

		if json.Unmarshal(body, &raw) != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}

		for _, d := range raw.Data {
			if d.ID == modelName {
				switch d.Status.Value {
				case "loaded":
					return nil
				case "failed":
					return fmt.Errorf("model %s failed to load", modelName)
				}
				break
			}
		}

		time.Sleep(500 * time.Millisecond)
	}

	return fmt.Errorf("model %s did not become loaded within %v", modelName, timeout)
}
