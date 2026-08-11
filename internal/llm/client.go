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
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"douya/internal/apperror"
	"douya/internal/httputil"

	"github.com/rs/zerolog/log"
)

const maxResponseBody = 50 * 1024 * 1024

const (
	httpClientTimeout = 300 * time.Second // 普通 HTTP 请求超时
	// L-15：streamTimeout 是 http.Client 层面的兜底超时（连接+读取总上限）。
	// 业务层在 service_stream.go 用 streamRequestTimeout=300s 通过 context.WithTimeout
	// 包裹请求，context 必然先于此 900s 触发。900s 仅作为防御性兜底，防止
	// 业务层忘记设置 context 超时时请求永久挂起。两处常量语义不同，不要混淆。
	streamTimeout     = 900 * time.Second      // 流式请求兜底超时（业务层 300s 优先生效）
	pollTimeout       = 3 * time.Second        // 轮询超时
	pollRetryInterval = 300 * time.Millisecond // 轮询重试间隔
)

// readBody 读取 HTTP 响应体，限制最大 50MB 防止内存耗尽。
func readBody(r io.Reader) ([]byte, error) {
	return httputil.ReadBodyLimited(r, maxResponseBody)
}

type Client struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	streamClient *http.Client
	pollClient   *http.Client // 复用的轮询客户端，用于 WaitForModelLoaded 等轮询场景
	currentModel string
	modelMu      sync.RWMutex
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
		httpClient:   &http.Client{Timeout: httpClientTimeout},
		streamClient: &http.Client{Timeout: streamTimeout},
		pollClient:   &http.Client{Timeout: pollTimeout},
	}
}

// setAuthHeader 为请求设置认证 header（如果配置了 API Key）。
// M1 修复：内部 API Key 是给本机 llama-server 用的启动凭据，绝不应当发送到
// 不可信主机。只有当 api_base 解析为 loopback（127.0.0.1 / localhost / ::1）时才
// 附加该 Key，防止配置被改写（如配置文件的 api_base 指向远程地址）后 Key 随请求外泄。
func (c *Client) setAuthHeader(req *http.Request) {
	if c.apiKey != "" && c.baseIsLoopback() {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
}

// baseIsLoopback 判断 api_base 的 host 是否为本机回环地址。
// hostname 解析失败、IP 未知或非回环一律返回 false（保守：不发送内部 Key）。
func (c *Client) baseIsLoopback() bool {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

// SetAuthHeader 公开方法，供外部包（如 app.go）设置认证 header
func (c *Client) SetAuthHeader(req *http.Request) {
	c.setAuthHeader(req)
}

// doSimpleJSONRequest 执行简单的 JSON 请求，返回响应体（成功时）或错误（失败时）。
// 用于 LoadModel/UnloadModel/DeleteModel/DownloadModel/ReloadModels/ReloadPresets/StopThinking/healthCheckOnce 等场景：
// 这些请求的共同模式是 "创建请求 → setAuthHeader → Do → Close → 检查状态码"。
// 生活类比：就像寄挂号信，只关心是否签收（状态码 200），不关心回信内容（除非调用方需要响应体）。
//
// 参数：
//   - method: HTTP 方法（GET/POST/DELETE 等）
//   - reqURL: 完整 URL
//   - body: 请求体（nil 表示无请求体，GET 请求通常传 nil）
//   - actionDesc: 操作描述（如 "load model"），用于错误信息
//   - wantBody: 是否读取成功响应体（L-16 修复：false 时跳过读取，减少无谓 IO）
//
// 返回：
//   - 成功：wantBody=true 时返回响应体字节，wantBody=false 时返回 nil
//   - 失败：nil 和错误（错误信息包含状态码和错误响应体，限制 1MB）
func (c *Client) doSimpleJSONRequest(ctx context.Context, method, reqURL string, body []byte, actionDesc string, wantBody bool) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, method, reqURL, bodyReader)
	if err != nil {
		return nil, apperror.Wrapf(apperror.KindInternal, "failed to create %s request", err, actionDesc)
	}
	if body != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, apperror.Wrapf(apperror.KindUnavailable, "%s request failed", err, actionDesc)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, httputil.ReadErrorBody(resp, actionDesc+" returned")
	}

	// L-16：仅在调用方需要响应体时读取，避免忽略 body 的调用方产生无谓 IO
	if !wantBody {
		return nil, nil
	}
	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, apperror.Wrapf(apperror.KindInternal, "failed to read %s response", err, actionDesc)
	}
	return respBody, nil
}

func (c *Client) StreamChat(ctx context.Context, req *ChatCompletionRequest, onToken func(chunk SSEChunk) error) error {
	return c.StreamChatWithConvID(ctx, req, "", onToken)
}

// StreamChatWithConvID 发起流式聊天请求，可选通过 convID 启用 SSE Replay Buffer
// 当 convID 非空时，设置 X-Conversation-Id header，llama-server 会缓冲 SSE 字节，
// 客户端断线后可通过 GET /v1/stream/:conv_id 重放恢复（TTL 5分钟，缓冲区 4MB）
// 生活类比：就像看视频时开启"断点续播"功能，网络断了也能从上次的位置继续
func (c *Client) StreamChatWithConvID(ctx context.Context, req *ChatCompletionRequest, convID string, onToken func(chunk SSEChunk) error) error {
	req.Stream = true
	req.StreamOptions = &StreamOptions{IncludeUsage: true}

	body, err := json.Marshal(req)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "failed to marshal request", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "failed to create request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	// 启用 SSE Replay Buffer：服务端缓冲 SSE 字节，支持断线重连
	if convID != "" {
		httpReq.Header.Set("X-Conversation-Id", convID)
	}
	c.setAuthHeader(httpReq)

	resp, err := c.streamClient.Do(httpReq)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "failed to send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return httputil.ReadErrorBody(resp, "unexpected status code")
	}

	scanner := bufio.NewScanner(resp.Body)
	// SSE 单行最大 10MB，超过此限制的行会被截断并记录警告
	const maxSSELineSize = 10 * 1024 * 1024
	scanner.Buffer(make([]byte, 0, 1024*1024), maxSSELineSize)
	for scanner.Scan() {
		// 在处理每行前检查 ctx 是否已取消，与 WatchModelLoadProgress 保持一致
		// 生活类比：就像服务员每上一道菜前先看一眼顾客是否已经离席，避免把菜端给空座位
		// 虽然 http.NewRequestWithContext 会在 ctx 取消时关闭底层连接，
		// 但 scanner 已缓冲的数据仍会被处理，这里主动检查可避免取消后继续 emit token
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
			break
		}

		var chunk SSEChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			continue
		}

		if onToken != nil {
			if err := onToken(chunk); err != nil {
				return apperror.Wrap(apperror.KindInternal, "onToken callback error", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return apperror.Wrap(apperror.KindInternal, "error reading SSE stream", err)
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
	if s == "" {
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
		return nil, apperror.Wrap(apperror.KindInternal, "failed to marshal request", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "failed to create request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	result, _, err := httputil.DoAndUnmarshal[ChatCompletionResponse](c.httpClient, httpReq, maxResponseBody)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// proxyRequest 代理透传请求：将原始请求体转发到指定端点，返回原始响应体。
// 用于 AnthropicMessages/AnthropicCountTokens/BuiltInTools 等代理场景。
// 生活类比：就像快递中转站，原封不动地把包裹从发件人转给收件人，不拆包不改装。
func (c *Client) proxyRequest(ctx context.Context, endpoint, actionDesc string, body []byte) ([]byte, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, apperror.Wrapf(apperror.KindInternal, "failed to create %s request", err, actionDesc)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, apperror.Wrapf(apperror.KindUnavailable, "%s request failed", err, actionDesc)
	}
	defer resp.Body.Close()

	respBody, err := readBody(resp.Body)
	if err != nil {
		return nil, apperror.Wrapf(apperror.KindInternal, "failed to read %s response", err, actionDesc)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, apperror.Newf(apperror.KindUnavailable, "%s returned status %d: %s", actionDesc, resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// AnthropicMessages 代理 Anthropic Messages API 请求
// 将原始请求体转发到 /v1/messages 端点，返回原始响应体
func (c *Client) AnthropicMessages(ctx context.Context, body []byte) ([]byte, error) {
	return c.proxyRequest(ctx, "/v1/messages", "anthropic messages", body)
}

// AnthropicCountTokens 代理 Anthropic token 计数请求
// 将原始请求体转发到 /v1/messages/count_tokens 端点，返回原始响应体
func (c *Client) AnthropicCountTokens(ctx context.Context, body []byte) ([]byte, error) {
	return c.proxyRequest(ctx, "/v1/messages/count_tokens", "anthropic count tokens", body)
}

// BuiltInTools 代理内置工具请求
// 将原始请求体转发到 /tools 端点，返回原始响应体
func (c *Client) BuiltInTools(ctx context.Context, body []byte) ([]byte, error) {
	return c.proxyRequest(ctx, "/tools", "built-in tools", body)
}

// GetToolsList 拉取 llama-server 当前暴露的所有工具列表（含内置工具 + MCP 工具）。
// 用于在 /v1/chat/completions 请求的 tools 字段中注入可用工具。
// 生活类比：去餐厅后厨问"今天有哪些菜可选"，把菜单拿回来给顾客看。
//
// 返回的原始 JSON 形如：
//
//	[{"type":"function","function":{"name":"...","description":"...","parameters":{...}}}, ...]
func (c *Client) GetToolsList(ctx context.Context) ([]byte, error) {
	reqURL := c.baseURL + "/tools"
	respBody, err := c.doSimpleJSONRequest(ctx, http.MethodGet, reqURL, nil, "list tools", true)
	if err != nil {
		return nil, err
	}
	return respBody, nil
}

// CallTool 通过 POST /tools 端点调用指定工具，返回原始响应体。
// 用于 MCP 工具调用（llama-server 内部转发到对应 MCP server 子进程）。
// toolName 形如 "echo_echo"（<server>_<tool> 格式，由 llama-server 自动加前缀）。
// 生活类比：把订单送到外卖调度中心，调度中心再分发给对应平台。
func (c *Client) CallTool(ctx context.Context, toolName string, params json.RawMessage) ([]byte, error) {
	body := map[string]any{
		"tool":   toolName,
		"params": json.RawMessage(params),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "failed to marshal tool call request", err)
	}
	return c.proxyRequest(ctx, "/tools", "call tool "+toolName, bodyBytes)
}

// Embedding sends a request to /v1/embeddings and returns vector embeddings.
// input can be a string or []string.
func (c *Client) Embedding(ctx context.Context, req *EmbeddingRequest) (*EmbeddingResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "failed to marshal request", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "failed to create request", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.setAuthHeader(httpReq)

	result, _, err := httputil.DoAndUnmarshal[EmbeddingResponse](c.httpClient, httpReq, maxResponseBody)
	if err != nil {
		return nil, err
	}

	return result, nil
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
	_, err := c.doSimpleJSONRequest(ctx, http.MethodGet, c.baseURL+endpoint, nil, "health check", false)
	return err
}

// GetMetrics 获取 llama-server /metrics 端点的 Prometheus 格式指标文本。
// 前提：llama-server 启动时已通过 --metrics 参数启用指标端点。
// 生活类比：去医院体检，/metrics 就像是打印出来的体检报告原始数据，
// 后续由调用方解析提取关心的指标（如血压、心率）展示给用户。
//
// 参数 modelName：在 router 模式下为必填，路由器需要根据 model 参数
// 将请求代理到对应的子进程；非 router 模式下可传空字符串。
// 豆芽默认使用 router 模式，因此调用方应传入当前已加载的模型名。
// 返回原始 Prometheus 文本（text/plain; version=0.0.4）。
func (c *Client) GetMetrics(ctx context.Context, modelName string) (string, error) {
	metricsURL := c.baseURL + "/metrics"
	// router 模式下 /metrics 走 proxy_get，必须带 model 查询参数，
	// 否则 router_validate_model 返回 400 "model name is missing from the request"
	if modelName != "" {
		metricsURL += "?model=" + url.QueryEscape(modelName)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, http.NoBody)
	if err != nil {
		return "", err
	}
	c.setAuthHeader(httpReq)
	// /metrics 端点返回 text/plain，不是 JSON，无需设置 Accept: application/json

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", apperror.Wrap(apperror.KindUnavailable, "failed to fetch /metrics", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", apperror.Newf(apperror.KindUnavailable, "metrics endpoint returned status %d", resp.StatusCode)
	}

	body, err := readBody(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

// MetricsSummary 是从 Prometheus /metrics 文本中提取的关键指标摘要。
// 生活类比：体检报告里挑出最重要的几项指标做成一页摘要，方便快速了解健康状况。
//
// 字段对应 llama.cpp tools/server/server-context.cpp 中 get_metrics 生成的指标，
// 指标名带 "llamacpp:" 前缀（注意是冒号，不是下划线）。
type MetricsSummary struct {
	TokensPromptTotal      float64 `json:"tokens_prompt_total"`       // 已处理的 prompt token 总数（llamacpp:prompt_tokens_total）
	PromptSecondsTotal     float64 `json:"prompt_seconds_total"`      // 处理 prompt 总耗时（秒）（llamacpp:prompt_seconds_total）
	TokensPredictedTotal   float64 `json:"tokens_predicted_total"`    // 已生成的 token 总数（llamacpp:tokens_predicted_total）
	PredictedSecondsTotal  float64 `json:"predicted_seconds_total"`   // 生成 token 总耗时（秒）（llamacpp:tokens_predicted_seconds_total）
	NDecodeTotal           float64 `json:"n_decode_total"`            // llama_decode() 调用总次数（llamacpp:n_decode_total）
	NTokensMax             float64 `json:"n_tokens_max"`              // 观察到的最大 n_tokens（llamacpp:n_tokens_max）
	PromptTokensPerSecond  float64 `json:"prompt_tokens_per_second"`  // prompt 处理速度（token/s，gauge，llamacpp:prompt_tokens_seconds）
	PredictTokensPerSecond float64 `json:"predict_tokens_per_second"` // 生成速度（token/s，gauge，llamacpp:predicted_tokens_seconds）
	ProcessingRequests     int     `json:"processing_requests"`       // 处理中的请求数（llamacpp:requests_processing）
	DeferredRequests       int     `json:"deferred_requests"`         // 排队中的请求数（llamacpp:requests_deferred）
	BusySlotsPerDecode     float64 `json:"busy_slots_per_decode"`     // 每次 decode 平均繁忙 slot 数（llamacpp:n_busy_slots_per_decode）

	// 推测解码指标（llama.cpp b10287 / PR #26389 引入，命名对齐 vLLM）
	// 推测解码未启用时这些计数器恒为 0；启用后用于评估命中率（accepted/draft）
	SpecDraftTokensTotal    float64 `json:"spec_draft_tokens_total"`    // 草稿模型生成的 token 总数（llamacpp:spec_decode_num_draft_tokens_total）
	SpecAcceptedTokensTotal float64 `json:"spec_accepted_tokens_total"` // 被目标模型接受的草稿 token 总数（llamacpp:spec_decode_num_accepted_tokens_total）
	SpecDraftsTotal         float64 `json:"spec_drafts_total"`          // 推测解码验证步骤总数（llamacpp:spec_decode_num_drafts_total）
	// 按位置的接受草稿 token 数（llama.cpp b10355 新增：spec_decode_num_accepted_tokens_per_pos_total）。
	// 每个位置 label 对应一个计数器；DSpark 等推测解码可用它评估各 draft 位置的命中分布。
	SpecAcceptedTokensPerPosTotal float64 `json:"spec_accepted_tokens_per_pos_total"` // llamacpp:spec_decode_num_accepted_tokens_per_pos_total（按位置累计）
}

// ParseMetrics 解析 Prometheus 格式文本，提取关键指标。
// 生活类比：从一堆体检数据中挑出关心的几项，填到摘要表格里。
// 解析逻辑：逐行扫描，匹配 "metric_name value" 格式的行（忽略 # 开头的注释行）。
//
// llama.cpp 生成的指标名带 "llamacpp:" 前缀（冒号是 Prometheus 的 namespace 分隔符），
// 例如 "llamacpp:tokens_predicted_total 150"。
// 解析时直接用完整名匹配，不做前缀处理，避免误匹配其他指标。
func ParseMetrics(text string) MetricsSummary {
	var s MetricsSummary
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Prometheus 格式：metric_name [labels] value
		// 简化解析：按空白分割，第一段是名称（可能含 label），最后一段是数值
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := parts[0]
		// 去掉 label 部分：如 llamacpp:kv_cache_usage_ratio{slot="0"} -> llamacpp:kv_cache_usage_ratio
		if idx := strings.Index(name, "{"); idx >= 0 {
			name = name[:idx]
		}
		valueStr := parts[len(parts)-1]
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}
		switch name {
		case "llamacpp:prompt_tokens_total":
			s.TokensPromptTotal = value
		case "llamacpp:prompt_seconds_total":
			s.PromptSecondsTotal = value
		case "llamacpp:tokens_predicted_total":
			s.TokensPredictedTotal = value
		case "llamacpp:tokens_predicted_seconds_total":
			s.PredictedSecondsTotal = value
		case "llamacpp:n_decode_total":
			s.NDecodeTotal = value
		case "llamacpp:n_tokens_max":
			s.NTokensMax = value
		case "llamacpp:prompt_tokens_seconds":
			s.PromptTokensPerSecond = value
		case "llamacpp:predicted_tokens_seconds":
			s.PredictTokensPerSecond = value
		case "llamacpp:requests_processing":
			s.ProcessingRequests = int(value)
		case "llamacpp:requests_deferred":
			s.DeferredRequests = int(value)
		case "llamacpp:n_busy_slots_per_decode":
			s.BusySlotsPerDecode = value
		case "llamacpp:spec_decode_num_draft_tokens_total":
			s.SpecDraftTokensTotal = value
		case "llamacpp:spec_decode_num_accepted_tokens_total":
			s.SpecAcceptedTokensTotal = value
		case "llamacpp:spec_decode_num_drafts_total":
			s.SpecDraftsTotal = value
		case "llamacpp:spec_decode_num_accepted_tokens_per_pos_total":
			// 同一指标按 position label 有多个序列，累加得到总接受数
			s.SpecAcceptedTokensPerPosTotal += value
		}
	}
	return s
}

// StopThinking 发送 POST /v1/chat/completions/control 请求，强制结束当前思考块。
// 用于实时推理控制：用户在流式推理过程中点击"直接回答"按钮时调用。
// 请求体格式（v9744+）：{"id": "chatcmpl-xxx", "action": "reasoning_end", "model": "xxx"}
// 前提：原始聊天请求必须带 reasoning_control: true，否则此端点无效。
func (c *Client) StopThinking(ctx context.Context, completionID string) error {
	body, err := json.Marshal(map[string]any{
		"id":     completionID,
		"action": "reasoning_end",
		"model":  c.GetCurrentModel(),
	})
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "failed to marshal stop thinking request", err)
	}

	if _, err := c.doSimpleJSONRequest(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions/control", body, "stop thinking", false); err != nil {
		return err
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
		return nil, apperror.Wrap(apperror.KindInternal, "failed to marshal rerank request", err)
	}

	respBody, err := c.doSimpleJSONRequest(ctx, http.MethodPost, c.baseURL+"/v1/rerank", body, "rerank", true)
	if err != nil {
		return nil, err
	}

	var result RerankResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "failed to unmarshal rerank response", err)
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
		switch lc {
		case "vision":
			caps.ImageInput = true
		case "audio", "speech":
			caps.AudioInput = true
		case "video":
			caps.VideoInput = true
		case "multimodal":
			caps.ImageInput = true
		}
	}

	for _, m := range info.InputModalities {
		lm := strings.ToLower(m)
		switch lm {
		case "image":
			caps.ImageInput = true
		case "audio":
			caps.AudioInput = true
		case "video":
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
	FType     string  `json:"ftype"` // 模型量化类型名（如 "Q4_K - Medium"），由 llama.cpp fdb1db877+ 提供
}

func (c *Client) GetModelInfo(ctx context.Context) (*ModelInfo, error) {
	return c.GetModelInfoByName(ctx, "")
}

func parseCapabilitiesRaw(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	var strArr []string
	if err := json.Unmarshal(raw, &strArr); err == nil {
		return strArr
	}

	var objArr []map[string]any
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
	ChatTemplateToolUse string           `json:"chat_template_tool_use"`
	BuildInfo           string           `json:"build_info,omitempty"`
	IsSleeping          bool             `json:"is_sleeping,omitempty"`
	CorsProxyEnabled    bool             `json:"cors_proxy_enabled,omitempty"`
}

// ChatTemplateCaps 对应 llama.cpp 最新版 /props 返回的 chat_template_caps 字段
// 包含模型模板能力声明，用于判断工具调用、推理保留等能力
type ChatTemplateCaps struct {
	SupportsTools             bool `json:"supports_tools"`
	SupportsToolCalls         bool `json:"supports_tool_calls"`
	SupportsSystemRole        bool `json:"supports_system_role"`
	SupportsParallelToolCalls bool `json:"supports_parallel_tool_calls"`
	SupportsPreserveReasoning bool `json:"supports_preserve_reasoning"`
	SupportsStringContent     bool `json:"supports_string_content"`
	SupportsTypedContent      bool `json:"supports_typed_content"`
	SupportsObjectArguments   bool `json:"supports_object_arguments"`
}

func (c *Client) GetServerProps(ctx context.Context, modelName string) (*ServerProps, error) {
	propsURL := c.baseURL + "/props"
	if modelName != "" {
		propsURL += "?model=" + url.QueryEscape(modelName)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, propsURL, http.NoBody)
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
		return nil, apperror.Newf(apperror.KindUnavailable, "props endpoint returned status %d", resp.StatusCode)
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
		return apperror.Wrap(apperror.KindInternal, "failed to marshal load model request", err)
	}

	respBody, err := c.doSimpleJSONRequest(ctx, http.MethodPost, c.baseURL+"/models/load", body, "load model", true)
	if err != nil {
		// llama-server 在模型已加载/正在加载时返回包含 "already running" 或 "already loaded" 的错误
		// 这里集中识别并类型化为 Conflict，上层可用 errors.Is(err, apperror.ErrConflict) 精准判断
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "already running") || strings.Contains(errMsg, "already loaded") {
			return apperror.Wrap(apperror.KindConflict, "模型已正在运行: "+modelName, err)
		}
		return err
	}

	// 安全实践：响应体可能含模型加载状态/内部路径，降级为 Debug 并截断到 500 字符，与 GetServerProps 保持一致
	bodySnippet := string(respBody)
	if len(bodySnippet) > 500 {
		bodySnippet = bodySnippet[:500] + "...(truncated)"
	}
	log.Debug().Str("model", modelName).Str("body", bodySnippet).Msg("[client] LoadModel response")
	return nil
}

// WatchModelLoadProgress 通过 /models/sse 端点实时监听模型加载进度
// 当模型状态变为 "loaded" 或上下文被取消时返回
//
// llama.cpp 最新版本将事件名统一为 "model_status"（之前为 "status_change"/"download_finished"）
// 本方法兼容新旧两种事件名，确保跨版本稳定性
func (c *Client) WatchModelLoadProgress(ctx context.Context, modelName string, onProgress func(event ModelLoadEvent)) error {
	reqURL := fmt.Sprintf("%s/models/sse", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, http.NoBody)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "create request failed", err)
	}
	c.setAuthHeader(req)

	resp, err := c.streamClient.Do(req)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "request failed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return httputil.ReadErrorBody(resp, "unexpected status")
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
		return apperror.Wrap(apperror.KindInternal, "error reading SSE stream", err)
	}

	return nil
}

// ReloadPresets 通知路由器重新加载 preset 文件
// 在修改了 router-preset.ini 后调用，使路由器感知到配置变化
// 原版 llama.cpp 使用 GET /v1/models?reload 来触发 preset 重载
func (c *Client) ReloadPresets(ctx context.Context) error {
	if _, err := c.doSimpleJSONRequest(ctx, http.MethodGet, c.baseURL+"/v1/models?reload=1", nil, "reload presets", false); err != nil {
		return err
	}
	log.Info().Msg("[client] ReloadPresets: presets reloaded successfully")
	return nil
}

func (c *Client) UnloadModel(ctx context.Context, modelName string) error {
	body, err := json.Marshal(map[string]string{"model": modelName})
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "failed to marshal unload model request", err)
	}
	_, err = c.doSimpleJSONRequest(ctx, http.MethodPost, c.baseURL+"/models/unload", body, "unload model", false)
	return err
}

func (c *Client) GetModelsList(ctx context.Context) ([]ListedModel, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", http.NoBody)
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
		return nil, httputil.ReadErrorBody(resp, "models endpoint returned")
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
	_, err := c.doSimpleJSONRequest(ctx, http.MethodGet, c.baseURL+"/models?reload", nil, "reload models", false)
	return err
}

// DeleteModel 调用 DELETE /models 删除模型（从列表中移除并卸载）
func (c *Client) DeleteModel(ctx context.Context, modelName string) error {
	body, err := json.Marshal(map[string]string{"model": modelName})
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "failed to marshal delete model request", err)
	}
	if _, err := c.doSimpleJSONRequest(ctx, http.MethodDelete, c.baseURL+"/models", body, "delete model", false); err != nil {
		return err
	}
	log.Info().Str("model", modelName).Msg("[client] DeleteModel: model deleted")
	return nil
}

// DownloadModel 触发模型下载（非阻塞，进度通过 /models/sse 跟踪）
// 模型名格式：HF 仓库格式，如 "ggml-org/gemma-3-4b-it-GGUF:Q4_K_M"
func (c *Client) DownloadModel(ctx context.Context, modelName string) error {
	body, err := json.Marshal(map[string]string{"model": modelName})
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "failed to marshal download model request", err)
	}
	if _, err := c.doSimpleJSONRequest(ctx, http.MethodPost, c.baseURL+"/models", body, "download model", false); err != nil {
		return err
	}
	log.Info().Str("model", modelName).Msg("[client] DownloadModel: download started")
	return nil
}

// CountTokens 调用 /v1/chat/completions/input_tokens 估算消息的 token 数量
func (c *Client) CountTokens(ctx context.Context, messages []ChatMessage) (int, error) {
	// v9744+ 要求在请求体中包含 model 字段
	reqBody := map[string]any{
		"messages": messages,
		"model":    c.GetCurrentModel(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return 0, apperror.Wrap(apperror.KindInternal, "failed to marshal count tokens request", err)
	}

	respBody, err := c.doSimpleJSONRequest(ctx, http.MethodPost, c.baseURL+"/v1/chat/completions/input_tokens", body, "count tokens", true)
	if err != nil {
		return 0, err
	}

	var result struct {
		InputTokens int `json:"input_tokens"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return 0, apperror.Wrap(apperror.KindInternal, "failed to parse count tokens response", err)
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
	body, err := c.doSimpleJSONRequest(ctx, http.MethodGet, reqURL, nil, "get lora adapters", true)
	if err != nil {
		return nil, err
	}

	var adapters []LoraAdapter
	if err := json.Unmarshal(body, &adapters); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "failed to parse lora adapters response", err)
	}

	return adapters, nil
}

// SetLoraAdapters 调用 POST /lora-adapters 设置 LoRA 适配器（运行时热切换）
func (c *Client) SetLoraAdapters(ctx context.Context, adapters []LoraAdapter) error {
	body, err := json.Marshal(adapters)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "failed to marshal set lora adapters request", err)
	}

	if _, err := c.doSimpleJSONRequest(ctx, http.MethodPost, c.baseURL+"/lora-adapters", body, "set lora adapters", false); err != nil {
		return err
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
	body, err := c.doSimpleJSONRequest(ctx, http.MethodGet, reqURL, nil, "get slots", true)
	if err != nil {
		return nil, err
	}

	var slots []SlotInfo
	if err := json.Unmarshal(body, &slots); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "failed to parse slots response", err)
	}

	return slots, nil
}

// OperateSlot 调用 POST /slots/{id}?action=save|restore|erase 执行 slot 操作。
// 生活类比：像图书馆的"存包/取包/丢弃"三个按钮，背后是同一个储物柜系统，只是动作不同。
//
// 参数：
//   - slotID: slot 编号（默认单 slot 模式下为 0）
//   - action: "save"（保存 KV 缓存到磁盘）、"restore"（从磁盘恢复）、"erase"（删除磁盘文件）
//
// 失败时返回错误，调用方自行决定是否记录日志或忽略。
func (c *Client) OperateSlot(ctx context.Context, slotID int, action string) error {
	// 白名单校验，避免 action 参数被注入到 URL 查询串
	switch action {
	case "save", "restore", "erase":
	default:
		return apperror.Newf(apperror.KindInvalidInput, "invalid slot action: %s (only save/restore/erase)", action)
	}
	query := url.Values{"action": {action}}.Encode()
	reqURL := fmt.Sprintf("%s/slots/%d?%s", c.baseURL, slotID, query)
	// slot 操作无响应体需求，wantBody=false 节省一次 Body 读取
	_, err := c.doSimpleJSONRequest(ctx, http.MethodPost, reqURL, nil, "slot "+action, false)
	return err
}

// Tokenize 调用 /tokenize 对文本进行分词，返回 token ID 列表
func (c *Client) Tokenize(ctx context.Context, text string) ([]int, error) {
	// v9744+ 要求在请求体中包含 model 字段
	reqBody := map[string]any{
		"content": text,
		"model":   c.GetCurrentModel(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "failed to marshal tokenize request", err)
	}

	respBody, err := c.doSimpleJSONRequest(ctx, http.MethodPost, c.baseURL+"/tokenize", body, "tokenize", true)
	if err != nil {
		return nil, err
	}

	var result struct {
		Tokens []int `json:"tokens"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "failed to parse tokenize response", err)
	}

	return result.Tokens, nil
}

// ApplyTemplate 调用 /apply-template 应用聊天模板，返回格式化后的 prompt
func (c *Client) ApplyTemplate(ctx context.Context, messages []ChatMessage) (string, error) {
	// v9744+ 要求在请求体中包含 model 字段
	reqBody := map[string]any{
		"messages": messages,
		"model":    c.GetCurrentModel(),
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "failed to marshal apply template request", err)
	}

	respBody, err := c.doSimpleJSONRequest(ctx, http.MethodPost, c.baseURL+"/apply-template", body, "apply template", true)
	if err != nil {
		return "", err
	}

	var result struct {
		Prompt string `json:"prompt"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "failed to parse apply template response", err)
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
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/v1/models", http.NoBody)
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
		return nil, httputil.ReadErrorBody(resp, "models endpoint returned")
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

	return nil, apperror.Newf(apperror.KindNotFound, "model %s not found in models list", modelName)
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

// DeleteStream 调用 DELETE /v1/stream/:conv_id 停止指定会话的流式生成
// 基于 llama.cpp SSE Replay Buffer 功能（b9809+）
// 生活类比：就像挂断电话，不仅自己不听，还告诉对方也不用继续说了
// 返回 nil 表示成功停止（包括会话不存在的情况，幂等操作）
func (c *Client) DeleteStream(ctx context.Context, convID string) error {
	if convID == "" {
		return nil
	}
	reqURL := c.baseURL + "/v1/stream/" + url.PathEscape(convID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, http.NoBody)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "failed to create delete stream request", err)
	}
	c.setAuthHeader(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return apperror.Wrap(apperror.KindUnavailable, "delete stream request failed", err)
	}
	defer resp.Body.Close()

	// 204 No Content 表示成功；404 表示会话不存在（幂等，视为成功）
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := readBody(resp.Body)
		return apperror.Newf(apperror.KindUnavailable, "delete stream returned status %d: %s", resp.StatusCode, string(body))
	}

	log.Debug().Str("conv_id", convID).Msg("[client] DeleteStream: stream stopped")
	return nil
}
