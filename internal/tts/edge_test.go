package tts

import (
	"context"
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
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

// mockEdgeTTSServer 模拟 Edge TTS 的 WebSocket 协议：
//   - 读取客户端发来的两条文本（speech.config、ssml）
//   - 下发一个二进制音频帧（header 含 Path:audio）
//   - 再下发一个二进制结束帧（header 含 Path:turn.end）
//   - 之后保持连接空闲（真实服务端不会立即关闭），以检验客户端是否靠 turn.end 及时结束
func mockEdgeTTSServer(t *testing.T, audioPayload []byte) *httptest.Server {
	t.Helper()
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		// 读取 speech.config 与 ssml 两条文本
		for i := 0; i < 2; i++ {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
		writeBinary := func(header string, payload []byte) {
			buf := make([]byte, 2)
			binary.BigEndian.PutUint16(buf, uint16(len(header)))
			buf = append(buf, []byte(header)...)
			buf = append(buf, payload...)
			_ = conn.WriteMessage(websocket.BinaryMessage, buf)
		}
		writeBinary("Path:audio\r\nX-StreamId:1\r\n", audioPayload)
		writeBinary("Path:turn.end\r\n", nil)
		// 保持空闲，等待客户端主动关闭（若客户端没识别 turn.end 会卡在读超时）
		_, _, _ = conn.ReadMessage()
	}))
}

func TestSynthesize_EndsOnBinaryTurnEnd(t *testing.T) {
	want := []byte("FAKE-MP3-AUDIO-DATA")
	srv := mockEdgeTTSServer(t, want)
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	done := make(chan struct{})
	var got []byte
	var synthErr error
	go func() {
		got, synthErr = synthesizeStream(ctx, wsURL, "你好", DefaultVoice, "+0%", "+0Hz", "+100%")
		close(done)
	}()

	select {
	case <-done:
		// 正常：收到 turn.end 后及时结束
	case <-time.After(15 * time.Second):
		t.Fatal("synthesizeStream 未在 15s 内结束：未正确识别二进制帧里的 Path:turn.end，会阻塞到读超时（约 30s）")
	}

	if synthErr != nil {
		t.Fatalf("synthesizeStream 返回错误: %v", synthErr)
	}
	if string(got) != string(want) {
		t.Fatalf("音频内容不符：got %q, want %q", got, want)
	}
}
