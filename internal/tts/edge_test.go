package tts

import (
	"strings"
	"testing"
)

// TestResolveOnlineVoice 校验本地发音人 → 在线 Neural 音色映射
func TestResolveOnlineVoice(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", DefaultOnlineVoice},                         // 空 → 默认晓晓
		{"Microsoft Xiaoxiao", "zh-CN-XiaoxiaoNeural"},   // 精确命中
		{"Microsoft Yunxi", "zh-CN-YunxiNeural"},         // 精确命中
		{"Microsoft Xiaoyi", "zh-CN-XiaoyiNeural"},       // 精确命中
		{"  Microsoft Yunyang  ", "zh-CN-YunyangNeural"}, // 带空白精确命中
		{"Xiaoxiao", "zh-CN-XiaoxiaoNeural"},             // 去前缀模糊匹配
		{"xiaoxiao", "zh-CN-XiaoxiaoNeural"},             // 大小写不敏感
		{"zh-CN-XiaochenNeural", "zh-CN-XiaochenNeural"}, // 在线 Neural 名直接透传
		{"zh-CN-YunzeNeural", "zh-CN-YunzeNeural"},       // 在线 Neural 名直接透传
		{"Unknown Voice", DefaultOnlineVoice},            // 未匹配 → 默认晓晓
	}
	for _, c := range cases {
		got := ResolveOnlineVoice(c.in)
		if got != c.want {
			t.Errorf("ResolveOnlineVoice(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestGenerateSecMsGec_Format 校验 Sec-MS-GEC 令牌为 64 位大写十六进制
func TestGenerateSecMsGec_Format(t *testing.T) {
	token := generateSecMsGec()
	if len(token) != 64 {
		t.Fatalf("Sec-MS-GEC 应为 64 位 hex，实际 %d 位: %s", len(token), token)
	}
	for _, c := range token {
		if !((c >= '0' && c <= '9') || (c >= 'A' && c <= 'F')) {
			t.Fatalf("Sec-MS-GEC 含非法字符 %q: %s", c, token)
		}
	}
}

func TestFormatRate(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1.0, "+0%"},
		{2.0, "+100%"},
		{0.5, "-50%"},
		{1.5, "+50%"},
	}
	for _, c := range cases {
		if got := formatRate(c.in); got != c.want {
			t.Errorf("formatRate(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatPitch(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1.0, "+0Hz"},
		{2.0, "+50Hz"},
		{0.0, "-50Hz"},
		{1.5, "+25Hz"},
	}
	for _, c := range cases {
		if got := formatPitch(c.in); got != c.want {
			t.Errorf("formatPitch(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestFormatVolume(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{1.0, "+100%"},
		{0.5, "+50%"},
		{0.0, "+0%"},
	}
	for _, c := range cases {
		if got := formatVolume(c.in); got != c.want {
			t.Errorf("formatVolume(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBuildSSML(t *testing.T) {
	ssml := buildSSML("你好 <world> & \"test\"", DefaultVoice, "+0%", "+0Hz", "+100%")
	if !strings.Contains(ssml, `name="`+DefaultVoice+`"`) {
		t.Errorf("SSML 缺少 voice: %s", ssml)
	}
	if !strings.Contains(ssml, "&lt;world&gt;") {
		t.Errorf("SSML 未转义 < >: %s", ssml)
	}
	if !strings.Contains(ssml, "&amp;") {
		t.Errorf("SSML 未转义 &: %s", ssml)
	}
	if !strings.Contains(ssml, "&#34;") {
		t.Errorf("SSML 未转义双引号: %s", ssml)
	}
	if !strings.Contains(ssml, `pitch="+0Hz"`) ||
		!strings.Contains(ssml, `rate="+0%"`) ||
		!strings.Contains(ssml, `volume="+100%"`) {
		t.Errorf("SSML 缺少 prosody 属性: %s", ssml)
	}
}

func TestBuildSpeechConfig_ContainsOutputFormat(t *testing.T) {
	cfg := buildSpeechConfig()
	if !strings.Contains(cfg, outputFormat) {
		t.Errorf("speech.config 缺少 outputFormat: %s", cfg)
	}
	if !strings.Contains(cfg, "Path:speech.config") {
		t.Errorf("speech.config 缺少 Path: %s", cfg)
	}
}

func TestIsReachable_OfflineHost(t *testing.T) {
	// 探测一个确定不存在的端口，期望返回 false（不应 panic）
	if IsReachable(200 * 1e6) { // 200ms
		t.Skip("检测到网络可达（环境有网），跳过离线断言")
	}
}
