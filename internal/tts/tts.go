package tts

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
)

// ErrOffline 网络不可达，调用方应回退到本地 TTS
var ErrOffline = errors.New("online TTS unavailable: network offline")

// probeHost 网络可达性探测目标（微软 Edge TTS 接入点）
const probeHost = "speech.platform.bing.com:443"

// DefaultOnlineVoice 未指定发音人时使用的默认在线音色（微软晓晓，与本地语音优先级一致）
const DefaultOnlineVoice = "zh-CN-XiaoxiaoNeural"

// localToOnlineVoice 把本地 Web Speech 发音人名（如 "Microsoft Xiaoxiao"）
// 映射到微软在线 Neural 音色（如 "XiaoxiaoNeural"）。
// 这样用户即使在设置页选了本地"晓晓"，在线合成时也用对应在线音色，音质一致。
var localToOnlineVoice = map[string]string{
	"Microsoft Xiaoxiao": "zh-CN-XiaoxiaoNeural",
	"Microsoft Yunxi":    "zh-CN-YunxiNeural",
	"Microsoft Yunyang":  "zh-CN-YunyangNeural",
	"Microsoft Xiaoyi":   "zh-CN-XiaoyiNeural",
	"Microsoft Huihui":   "zh-CN-HuihuiNeural",
	"Microsoft Yaoyao":   "zh-CN-YaoyaoNeural",
	"Microsoft Kangkang": "zh-CN-KangkangNeural",
}

// ResolveOnlineVoice 将设置页传入的本地发音人名解析为在线 Neural 音色名。
//   - 空 → 默认晓伊 zh-CN-XiaoyiNeural
//   - 命中映射表 → 对应在线音色
//   - 去掉 "Microsoft " 前缀后模糊匹配（如 "Xiaoxiao"）
//   - 都不匹配 → 默认晓伊
func ResolveOnlineVoice(localName string) string {
	localName = strings.TrimSpace(localName)
	if localName == "" {
		return DefaultOnlineVoice
	}
	if v, ok := localToOnlineVoice[localName]; ok {
		return v
	}
	key := strings.TrimSpace(strings.TrimPrefix(localName, "Microsoft "))
	for k, v := range localToOnlineVoice {
		short := strings.TrimPrefix(k, "Microsoft ")
		if strings.EqualFold(short, key) {
			return v
		}
	}
	return DefaultOnlineVoice
}

// IsReachable 快速 TCP 探测 Edge TTS 服务是否可达
func IsReachable(timeout time.Duration) bool {
	if timeout <= 0 {
		timeout = 800 * time.Millisecond
	}
	d := net.Dialer{Timeout: timeout}
	conn, err := d.Dial("tcp", probeHost)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// SynthesizeOnline 在线合成：先探测网络，不通返回 ErrOffline；通则合成 MP3。
// rate/pitch/volume 为相对倍数（与 Web Speech API 一致：1.0 为正常），内部转为 Edge SSML 格式。
func SynthesizeOnline(ctx context.Context, text, voice string, rate, pitch, volume float64) ([]byte, error) {
	if !IsReachable(800 * time.Millisecond) {
		return nil, ErrOffline
	}
	return Synthesize(ctx, text, voice, formatRate(rate), formatPitch(pitch), formatVolume(volume))
}

// formatRate 将 0.5~2.0 倍速转为 Edge "+N%"/"-N%"
func formatRate(r float64) string {
	return fmt.Sprintf("%+d%%", int(math.Round((r-1)*100)))
}

// formatPitch 将 0~2.0 音调转为 Edge "+NHz"/"-NHz"（范围约 ±50Hz）
func formatPitch(p float64) string {
	return fmt.Sprintf("%+dHz", int(math.Round((p-1)*50)))
}

// formatVolume 将 0~1.0 音量转为 Edge "N%"
func formatVolume(v float64) string {
	return fmt.Sprintf("%+d%%", int(math.Round(v*100)))
}
