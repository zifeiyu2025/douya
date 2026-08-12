package tts

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// Edge TTS（微软在线神经语音）WebSocket 客户端。
// 直连 speech.platform.bing.com，免 API Key，声音含 zh-CN-XiaoyiNeural（晓伊）。
// 协议细节参照 edge-tts 开源实现（含 Sec-MS-GEC 握手令牌）。

const (
	trustedClientToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
	secMsGecVersion    = "1-143.0.3650.75"
	wssBaseURL         = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1?TrustedClientToken=" + trustedClientToken
	outputFormat       = "audio-24khz-48kbitrate-mono-mp3"
	userAgent          = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"
	origin             = "chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"
	// DefaultVoice 在线 TTS 默认发音人（微软晓伊）
	DefaultVoice = "zh-CN-XiaoyiNeural"
	// winEpoch Windows 纪元与 Unix 纪元的差值（秒），用于 Sec-MS-GEC 令牌计算
	winEpoch = 11644473600
)

// ErrOnlineDisabled 在线 TTS 被用户关闭
var ErrOnlineDisabled = fmt.Errorf("online TTS disabled")

// generateSecMsGec 根据当前 UTC 时间生成 Sec-MS-GEC 令牌（每 5 分钟一个窗口）。
func generateSecMsGec() string {
	now := time.Now().UTC().Unix()
	ticks := now + winEpoch
	ticks -= ticks % 300 // 向下取整到 5 分钟
	ticks = int64(float64(ticks) * (1e9 / 100))
	sum := sha256.Sum256([]byte(fmt.Sprintf("%d%s", ticks, trustedClientToken)))
	return strings.ToUpper(fmt.Sprintf("%x", sum))
}

func randomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	const hexd = "0123456789abcdef"
	buf := make([]byte, n*2)
	for i := 0; i < n; i++ {
		buf[i*2] = hexd[b[i]>>4]
		buf[i*2+1] = hexd[b[i]&0x0f]
	}
	return string(buf)
}

func connectionID() string { return randomHex(16) }

func wsURL() string {
	return fmt.Sprintf("%s&ConnectionId=%s&Sec-MS-GEC=%s&Sec-MS-GEC-Version=%s",
		wssBaseURL, connectionID(), generateSecMsGec(), secMsGecVersion)
}

func wsHeaders() http.Header {
	h := http.Header{}
	h.Set("Pragma", "no-cache")
	h.Set("Cache-Control", "no-cache")
	h.Set("Origin", origin)
	h.Set("User-Agent", userAgent)
	h.Set("Cookie", "muid="+connectionID())
	return h
}

// dateString 生成与 edge-tts 一致的 X-Timestamp 格式
func dateString() string {
	return time.Now().Format("Mon Jan 02 2006 15:04:05 GMT-0700 (MST)")
}

func xmlEscape(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

func buildSSML(text, voice, rate, pitch, volume string) string {
	voiceName := voice
	if voiceName == "" {
		voiceName = DefaultVoice
	}
	return fmt.Sprintf(`<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="zh-CN">`+
		`<voice name="%s"><prosody pitch="%s" rate="%s" volume="%s">%s</prosody></voice></speak>`,
		voiceName, pitch, rate, volume, xmlEscape(text))
}

func buildSpeechConfig() string {
	cfg := fmt.Sprintf(`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"false"},"outputFormat":"%s"}}}}`, outputFormat)
	return "X-Timestamp:" + dateString() + "\r\n" +
		"Content-Type:application/json; charset=utf-8\r\n" +
		"Path:speech.config\r\n\r\n" + cfg
}

func buildSSMLRequest(ssml string) string {
	return "X-RequestId:" + connectionID() + "\r\n" +
		"Content-Type:application/ssml+xml\r\n" +
		"X-Timestamp:" + dateString() + "\r\n" +
		"Path:ssml\r\n\r\n" + ssml
}

// Synthesize 通过 Edge TTS WebSocket 合成语音，返回拼接后的 MP3 字节。
func Synthesize(ctx context.Context, text, voice, rate, pitch, volume string) ([]byte, error) {
	if strings.TrimSpace(text) == "" {
		return nil, fmt.Errorf("tts: empty text")
	}

	dialer := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	conn, _, err := dialer.DialContext(ctx, wsURL(), wsHeaders())
	if err != nil {
		return nil, fmt.Errorf("tts: dial edge tts: %w", err)
	}
	defer conn.Close()

	if err := conn.WriteMessage(websocket.TextMessage, []byte(buildSpeechConfig())); err != nil {
		return nil, fmt.Errorf("tts: send speech.config: %w", err)
	}
	ssml := buildSSML(text, voice, rate, pitch, volume)
	if err := conn.WriteMessage(websocket.TextMessage, []byte(buildSSMLRequest(ssml))); err != nil {
		return nil, fmt.Errorf("tts: send ssml: %w", err)
	}

	var audio bytes.Buffer
	for {
		// 周期性刷新读截止时间，避免长文本合成时永久阻塞；同时响应 ctx 取消。
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		select {
		case <-ctx.Done():
			if audio.Len() > 0 {
				return audio.Bytes(), nil
			}
			return nil, ctx.Err()
		default:
		}

		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if audio.Len() > 0 {
				return audio.Bytes(), nil // 已收部分音频，尽力返回
			}
			return nil, fmt.Errorf("tts: read: %w", err)
		}
		if msgType == websocket.TextMessage {
			if strings.Contains(string(data), "Path:turn.end") {
				break
			}
			continue
		}
		// BinaryMessage：前 2 字节大端为 header 长度，其后为 header，再后为音频数据
		if len(data) < 2 {
			continue
		}
		headerLen := int(binary.BigEndian.Uint16(data[:2]))
		if 2+headerLen > len(data) {
			continue
		}
		header := string(data[2 : 2+headerLen])
		if strings.Contains(header, "Path:audio") {
			audio.Write(data[2+headerLen:])
		}
	}

	if audio.Len() == 0 {
		return nil, fmt.Errorf("tts: no audio received")
	}
	return audio.Bytes(), nil
}
