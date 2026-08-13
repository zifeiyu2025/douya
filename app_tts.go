package main

import (
	"context"
	"time"

	"douya/internal/tts"
)

// SynthesizeSpeech 在线合成语音（Edge TTS / 微软神经语音）。
// 返回 MP3 字节，Wails 会以 base64 字符串形式传给前端。
// 当在线 TTS 被关闭或网络不可用时返回错误，由前端自动回退到本地 Web Speech API。
//
// 参数：
//   - text：要合成的文本
//   - voice：设置页选的本地发音人名（如 "Microsoft Xiaoxiao"）；空 → 默认晓晓
//   - rate/pitch/volume：Web Speech 风格的相对倍率（1.0 = 正常），内部转为 Edge SSML 格式
func (a *App) SynthesizeSpeech(text string, voice string, rate float64, pitch float64, volume float64) ([]byte, error) {
	cfg := a.getConfig()
	if cfg == nil || !cfg.TtsOnline {
		return nil, tts.ErrOnlineDisabled
	}
	// 按设置页选的本地发音人映射到对应在线 Neural 音色；未选则默认晓晓
	onlineVoice := tts.ResolveOnlineVoice(voice)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return tts.SynthesizeOnline(ctx, text, onlineVoice, rate, pitch, volume)
}
