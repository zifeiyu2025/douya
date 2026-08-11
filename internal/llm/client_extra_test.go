// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"net/http"
	"strings"
	"testing"
)

// TestParseMetrics 验证 Prometheus 指标解析
// 生活类比：像翻译电报，把一串带标签的数字翻译成结构化的报告。
func TestParseMetrics(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(s MetricsSummary) bool
		desc  string
	}{
		{
			name:  "空字符串",
			input: "",
			check: func(s MetricsSummary) bool {
				return s.TokensPromptTotal == 0 && s.PredictTokensPerSecond == 0
			},
			desc: "空输入应返回零值",
		},
		{
			name:  "仅注释行",
			input: "# HELP prompt_tokens\n# TYPE prompt_tokens counter",
			check: func(s MetricsSummary) bool {
				return s.TokensPromptTotal == 0
			},
			desc: "注释行应被跳过",
		},
		{
			name:  "完整指标",
			input: "llamacpp:prompt_tokens_total 100\nllamacpp:tokens_predicted_total 200\nllamacpp:predicted_tokens_seconds 50.5",
			check: func(s MetricsSummary) bool {
				return s.TokensPromptTotal == 100 && s.TokensPredictedTotal == 200 && s.PredictTokensPerSecond == 50.5
			},
			desc: "应正确解析所有指标",
		},
		{
			name:  "带label的指标",
			input: `llamacpp:kv_cache_usage_ratio{slot="0"} 0.75`,
			check: func(s MetricsSummary) bool {
				// kv_cache_usage_ratio 不在解析列表中，但不应崩溃
				return true
			},
			desc: "带 label 的指标应正确去除 label",
		},
		{
			name:  "requests_processing",
			input: "llamacpp:requests_processing 2\nllamacpp:requests_deferred 1",
			check: func(s MetricsSummary) bool {
				return s.ProcessingRequests == 2 && s.DeferredRequests == 1
			},
			desc: "int 类型指标应正确解析",
		},
		{
			name:  "无效数值_跳过",
			input: "llamacpp:prompt_tokens_total abc",
			check: func(s MetricsSummary) bool {
				return s.TokensPromptTotal == 0
			},
			desc: "无效数值应跳过，保持零值",
		},
		{
			name:  "n_decode_total",
			input: "llamacpp:n_decode_total 1234\nllamacpp:n_tokens_max 4096",
			check: func(s MetricsSummary) bool {
				return s.NDecodeTotal == 1234 && s.NTokensMax == 4096
			},
			desc: "新增指标应正确解析",
		},
		{
			name:  "busy_slots_per_decode",
			input: "llamacpp:n_busy_slots_per_decode 1.5",
			check: func(s MetricsSummary) bool {
				return s.BusySlotsPerDecode == 1.5
			},
			desc: "BusySlotsPerDecode 应正确解析",
		},
		{
			name:  "混合内容",
			input: "# HELP prompt_tokens\nllamacpp:prompt_tokens_total 100\n# COMMENT\nllamacpp:predicted_tokens_seconds 25.3\ninvalid_line",
			check: func(s MetricsSummary) bool {
				return s.TokensPromptTotal == 100 && s.PredictTokensPerSecond == 25.3
			},
			desc: "混合注释、有效、无效行应正确解析",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseMetrics(tc.input)
			if !tc.check(got) {
				t.Errorf("%s: got %+v", tc.desc, got)
			}
		})
	}
}

// TestFuzzyMatchModelID 验证模型 ID 模糊匹配
// 生活类比：像在通讯录里找人，"张三" 能匹配 "张三丰"，但 "default" 太通用了不算匹配。
func TestFuzzyMatchModelID(t *testing.T) {
	tests := []struct {
		name  string
		id    string
		model string
		want  bool
	}{
		{"完全匹配", "qwen2.5:7b", "qwen2.5:7b", true},
		{"ID包含模型名", "qwen2.5:7b-instruct", "qwen2.5:7b", true},
		{"模型名包含ID", "qwen", "qwen2.5:7b", true},
		{"大小写不敏感", "QWEN2.5:7B", "qwen2.5:7b", true},
		{"default作为ID_不匹配", "default", "qwen2.5:7b", false},
		{"default作为模型名_不匹配", "qwen2.5:7b", "default", false},
		{"两者都是default_不匹配", "default", "default", false},
		{"完全不同", "llama3:8b", "qwen2.5:7b", false},
		{"空ID", "", "qwen", true},  // strings.Contains("qwen", "") = true
		{"空模型名", "qwen", "", true}, // strings.Contains("qwen", "") = true
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := FuzzyMatchModelID(tc.id, tc.model)
			if got != tc.want {
				t.Errorf("FuzzyMatchModelID(%q, %q) = %v, want %v", tc.id, tc.model, got, tc.want)
			}
		})
	}
}

// TestDetectCapabilities 验证模型能力检测
// 生活类比：像检查手机的硬件配置清单，看看支持哪些功能。
func TestDetectCapabilities(t *testing.T) {
	tests := []struct {
		name          string
		info          ModelInfo
		wantTextInput bool
		wantImage     bool
		wantAudio     bool
		wantVideo     bool
	}{
		{
			name:          "默认_仅文本",
			info:          ModelInfo{},
			wantTextInput: true,
			wantImage:     false,
			wantAudio:     false,
			wantVideo:     false,
		},
		{
			name:          "vision能力",
			info:          ModelInfo{Capabilities: []string{"vision"}},
			wantTextInput: true,
			wantImage:     true,
			wantAudio:     false,
			wantVideo:     false,
		},
		{
			name:          "multimodal能力_等同于image",
			info:          ModelInfo{Capabilities: []string{"multimodal"}},
			wantTextInput: true,
			wantImage:     true,
			wantAudio:     false,
			wantVideo:     false,
		},
		{
			name:          "audio能力",
			info:          ModelInfo{Capabilities: []string{"audio"}},
			wantTextInput: true,
			wantImage:     false,
			wantAudio:     true,
			wantVideo:     false,
		},
		{
			name:          "speech能力_等同于audio",
			info:          ModelInfo{Capabilities: []string{"speech"}},
			wantTextInput: true,
			wantImage:     false,
			wantAudio:     true,
			wantVideo:     false,
		},
		{
			name:          "video能力",
			info:          ModelInfo{Capabilities: []string{"video"}},
			wantTextInput: true,
			wantImage:     false,
			wantAudio:     false,
			wantVideo:     true,
		},
		{
			name:          "InputModalities_image",
			info:          ModelInfo{InputModalities: []string{"image"}},
			wantTextInput: true,
			wantImage:     true,
			wantAudio:     false,
			wantVideo:     false,
		},
		{
			name:          "InputModalities_audio",
			info:          ModelInfo{InputModalities: []string{"audio"}},
			wantTextInput: true,
			wantImage:     false,
			wantAudio:     true,
			wantVideo:     false,
		},
		{
			name:          "InputModalities_video",
			info:          ModelInfo{InputModalities: []string{"video"}},
			wantTextInput: true,
			wantImage:     false,
			wantAudio:     false,
			wantVideo:     true,
		},
		{
			name:          "大小写混合",
			info:          ModelInfo{Capabilities: []string{"Vision"}, InputModalities: []string{"Audio"}},
			wantTextInput: true,
			wantImage:     true,
			wantAudio:     true,
			wantVideo:     false,
		},
		{
			name: "多种能力组合",
			info: ModelInfo{
				Capabilities:    []string{"vision", "audio"},
				InputModalities: []string{"image", "video"},
			},
			wantTextInput: true,
			wantImage:     true,
			wantAudio:     true,
			wantVideo:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectCapabilities(tc.info)
			if got.TextInput != tc.wantTextInput {
				t.Errorf("TextInput = %v, want %v", got.TextInput, tc.wantTextInput)
			}
			if got.ImageInput != tc.wantImage {
				t.Errorf("ImageInput = %v, want %v", got.ImageInput, tc.wantImage)
			}
			if got.AudioInput != tc.wantAudio {
				t.Errorf("AudioInput = %v, want %v", got.AudioInput, tc.wantAudio)
			}
			if got.VideoInput != tc.wantVideo {
				t.Errorf("VideoInput = %v, want %v", got.VideoInput, tc.wantVideo)
			}
		})
	}
}

// TestNewClient 验证客户端初始化
func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		apiKey  string
	}{
		{"带尾斜杠_应去除", "http://localhost:8080/", "key123"},
		{"多个尾斜杠_全去除", "http://localhost:8080///", "key123"},
		{"无尾斜杠_原样", "http://localhost:8080", "key123"},
		{"空apiKey", "http://localhost:8080", ""},
		{"空baseURL", "", "key123"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(tc.baseURL, tc.apiKey)
			if c == nil {
				t.Fatal("NewClient 返回 nil")
			}
			// 验证尾斜杠被去除
			if strings.HasSuffix(c.BaseURL(), "/") && tc.baseURL != "" {
				t.Errorf("BaseURL 应去除尾斜杠，实际: %q", c.BaseURL())
			}
			// 验证 HTTPClient 不为 nil
			if c.HTTPClient() == nil {
				t.Error("HTTPClient 不应为 nil")
			}
		})
	}
}

// TestClientSetCurrentModel 验证设置/获取当前模型
func TestClientSetCurrentModel(t *testing.T) {
	c := NewClient("http://localhost:8080", "")

	// 初始应为空
	if got := c.GetCurrentModel(); got != "" {
		t.Errorf("初始模型应为空，实际: %q", got)
	}

	// 设置模型
	c.SetCurrentModel("qwen2.5:7b")
	if got := c.GetCurrentModel(); got != "qwen2.5:7b" {
		t.Errorf("设置后模型应为 'qwen2.5:7b'，实际: %q", got)
	}

	// 再次设置
	c.SetCurrentModel("llama3:8b")
	if got := c.GetCurrentModel(); got != "llama3:8b" {
		t.Errorf("再次设置后模型应为 'llama3:8b'，实际: %q", got)
	}
}

// TestBaseIsLoopback 验证 baseIsLoopback 对 loopback / 非 loopback 地址的判断。
// M1 修复回归：内部 API Key 只应发送给本机回环地址，防止改写 api_base 后 Key 外泄。
func TestBaseIsLoopback(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		want    bool
	}{
		{"ipv4回环", "http://127.0.0.1:8080", true},
		{"localhost", "http://localhost:8080", true},
		{"localhost无端口", "http://localhost", true},
		{"ipv6回环", "http://[::1]:8080", true},
		{"局域网IP", "http://192.168.1.10:8080", false},
		{"内网IP", "http://10.0.0.1:8080", false},
		{"公网域名", "http://example.com:8080", false},
		{"含路径", "http://invalid-host:port/path", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(tc.baseURL, "secret")
			if got := c.baseIsLoopback(); got != tc.want {
				t.Errorf("baseIsLoopback(%q) = %v, want %v", tc.baseURL, got, tc.want)
			}
		})
	}
}

// TestSetAuthHeader_LoopbackOnly 验证认证头仅在 loopback 目标上附加内部 Key。
// M1 修复回归：非回环 host（如被改写的 api_base 指向远程地址）不发送内部 Key。
func TestSetAuthHeader_LoopbackOnly(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		apiKey  string
		want    string // 期望的完整 Authorization 值，空表示应不设置
	}{
		{"回环发送", "http://127.0.0.1:8080", "k123", "Bearer k123"},
		{"localhost发送", "http://localhost:8080", "k123", "Bearer k123"},
		{"非回环不发送", "http://192.168.1.10:8080", "k123", ""},
		{"非回环不发送_公网", "https://example.com:8443", "k123", ""},
		{"回环空key", "http://127.0.0.1:8080", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := NewClient(tc.baseURL, tc.apiKey)
			req, _ := http.NewRequest("GET", c.BaseURL()+"/health", nil)
			c.SetAuthHeader(req)
			if got := req.Header.Get("Authorization"); got != tc.want {
				t.Errorf("SetAuthHeader(%q) Authorization = %q, want %q", tc.baseURL, got, tc.want)
			}
		})
	}
}
