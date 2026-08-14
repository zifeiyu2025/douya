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

// MicrosoftChineseVoices 微软全部中文（zh-CN）发音人清单。
//   - Local=true：Windows Web Speech API 提供本地音色（离线可用）
//   - Online=true：Edge TTS 提供 Neural 在线音色（见 OnlineName）
// 本地与在线音色一一对应，音质一致。晓晓（Xiaoxiao）为默认在线音色。
//
//	本地名             在线 Neural 音色          性别   本地  在线
//	Microsoft Xiaoxiao  zh-CN-XiaoxiaoNeural    女     ✔     ✔   (默认在线)
//	Microsoft Yunxi     zh-CN-YunxiNeural       男     ✔     ✔
//	Microsoft Yunyang   zh-CN-YunyangNeural     男     ✔     ✔
//	Microsoft Xiaoyi    zh-CN-XiaoyiNeural      女     ✔     ✔
//	Microsoft Huihui    zh-CN-HuihuiNeural      女     ✔     ✔
//	Microsoft Yaoyao    zh-CN-YaoyaoNeural      女     ✔     ✔
//	Microsoft Kangkang  zh-CN-KangkangNeural    男     ✔     ✔
//	— 以下仅在线 Neural 版（Windows 本地 Web Speech 不提供）—
//	（无本地名）         zh-CN-XiaochenNeural     男     ✘     ✔
//	（无本地名）         zh-CN-XiaohanNeural      女     ✘     ✔
//	（无本地名）         zh-CN-XiaomengNeural     女     ✘     ✔
//	（无本地名）         zh-CN-XiaomoNeural       女     ✘     ✔
//	（无本地名）         zh-CN-XiaoqiuNeural      女     ✘     ✔
//	（无本地名）         zh-CN-XiaoruiNeural      女     ✘     ✔
//	（无本地名）         zh-CN-XiaoshuangNeural   女     ✘     ✔
//	（无本地名）         zh-CN-XiaoyouNeural      女     ✘     ✔
//	（无本地名）         zh-CN-XiaozhenNeural     女     ✘     ✔
//	（无本地名）         zh-CN-YunfengNeural      男     ✘     ✔
//	（无本地名）         zh-CN-YunhaoNeural       男     ✘     ✔
//	（无本地名）         zh-CN-YunjianNeural      男     ✘     ✔
//	（无本地名）         zh-CN-YunxiaNeural       男     ✘     ✔
//	（无本地名）         zh-CN-YunyeNeural        男     ✘     ✔
//	（无本地名）         zh-CN-YunzeNeural        男     ✘     ✔
//
// 结论：本应用内置的 7 个本地发音人【全部】都有对应在线 Neural 版；
// 另有多达 15 个在线专属 Neural 音色可选（需直接指定在线名）。
var MicrosoftChineseVoices = []struct {
	LocalName  string
	OnlineName string
	Gender     string
	Local      bool
	Online     bool
}{
	{"Microsoft Xiaoxiao", "zh-CN-XiaoxiaoNeural", "女", true, true},
	{"Microsoft Yunxi", "zh-CN-YunxiNeural", "男", true, true},
	{"Microsoft Yunyang", "zh-CN-YunyangNeural", "男", true, true},
	{"Microsoft Xiaoyi", "zh-CN-XiaoyiNeural", "女", true, true},
	{"Microsoft Huihui", "zh-CN-HuihuiNeural", "女", true, true},
	{"Microsoft Yaoyao", "zh-CN-YaoyaoNeural", "女", true, true},
	{"Microsoft Kangkang", "zh-CN-KangkangNeural", "男", true, true},
	{"", "zh-CN-XiaochenNeural", "男", false, true},
	{"", "zh-CN-XiaohanNeural", "女", false, true},
	{"", "zh-CN-XiaomengNeural", "女", false, true},
	{"", "zh-CN-XiaomoNeural", "女", false, true},
	{"", "zh-CN-XiaoqiuNeural", "女", false, true},
	{"", "zh-CN-XiaoruiNeural", "女", false, true},
	{"", "zh-CN-XiaoshuangNeural", "女", false, true},
	{"", "zh-CN-XiaoyouNeural", "女", false, true},
	{"", "zh-CN-XiaozhenNeural", "女", false, true},
	{"", "zh-CN-YunfengNeural", "男", false, true},
	{"", "zh-CN-YunhaoNeural", "男", false, true},
	{"", "zh-CN-YunjianNeural", "男", false, true},
	{"", "zh-CN-YunxiaNeural", "女", false, true},
	{"", "zh-CN-YunyeNeural", "男", false, true},
	{"", "zh-CN-YunzeNeural", "男", false, true},
}

// localToOnlineVoice 把本地 Web Speech 发音人名（如 "Microsoft Xiaoxiao"）
// 映射到微软在线 Neural 音色（如 "XiaoxiaoNeural"）。
// 这样用户即使在设置页选了本地"晓晓"，在线合成时也用对应在线音色，音质一致。
// 7 个本地发音人【全部】有在线版（见 MicrosoftChineseVoices）。
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
//   - 空 → 默认晓晓 zh-CN-XiaoxiaoNeural（DefaultOnlineVoice）
//   - 命中映射表 → 对应在线音色
//   - 去掉 "Microsoft " 前缀后模糊匹配（如 "Xiaoxiao"）
//   - 都不匹配 → 默认晓晓
func ResolveOnlineVoice(localName string) string {
	localName = strings.TrimSpace(localName)
	if localName == "" {
		return DefaultOnlineVoice
	}
	// 已经是微软在线 Neural 音色名（如 "zh-CN-XiaochenNeural"）→ 直接透传，
	// 这样前端可直接指定仅在线版（本地无对应发音人）的音色。
	if strings.HasSuffix(strings.ToLower(localName), "neural") {
		return localName
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

// SynthesizeOnline 在线合成：默认发音人为晓晓（zh-CN-XiaoxiaoNeural）。
// 先快速探测网络，可达则直接合成；探测未通过不等于真离线（部分环境会拦截裸 TCP 探测
// 但允许 TLS 连接），因此会再尝试一次真实 WebSocket 合成（限时 3s），确保服务可达时
// 一定走在线版（晓晓），仅当确属不可达时才返回 ErrOffline 由前端回退本地。
// rate/pitch/volume 为相对倍数（与 Web Speech API 一致：1.0 为正常），内部转为 Edge SSML 格式。
func SynthesizeOnline(ctx context.Context, text, voice string, rate, pitch, volume float64) ([]byte, error) {
	// 未指定发音人时默认使用晓晓在线版（与本地首选一致）
	if strings.TrimSpace(voice) == "" {
		voice = DefaultOnlineVoice
	}

	rateStr := formatRate(rate)

	// 指定音色已被云端移除（如部分"仅在线"Neural 音色）：静默回退默认晓晓重试，
	// 保证在线合成仍能成功，避免前端因在线失败而回退本地触发"第一句重复"缺陷。
	fallbackIfUnsupported := func(err error) ([]byte, error) {
		if err != nil && strings.Contains(err.Error(), "Unsupported voice") {
			return Synthesize(ctx, text, DefaultOnlineVoice, rateStr, formatPitch(pitch), formatVolume(volume))
		}
		return nil, err
	}

	doSynth := func(c context.Context) ([]byte, error) {
		return Synthesize(c, text, voice, rateStr, formatPitch(pitch), formatVolume(volume))
	}

	// 先快速探测：可达则直接合成（默认发音人已是晓晓）。
	if IsReachable(800 * time.Millisecond) {
		audio, err := doSynth(ctx)
		if err == nil {
			return audio, nil
		}
		return fallbackIfUnsupported(err)
	}
	// 探测未通过：再尝试一次真实 WebSocket 合成（限时 3s），避免误判离线而漏掉在线版。
	attemptCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	audio, err := doSynth(attemptCtx)
	if err == nil {
		return audio, nil
	}
	if fb, fbErr := fallbackIfUnsupported(err); fbErr == nil {
		return fb, nil
	}
	return nil, ErrOffline
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
