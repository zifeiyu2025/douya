package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"douya/internal/tts"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := tts.SynthesizeOnline(ctx, "你好，这是微软在线语音合成测试。", "", 1.0, 1.0, 1.0)
	if err != nil {
		fmt.Println("ERROR:", err)
		os.Exit(1)
	}
	n := 4
	if len(data) < n {
		n = len(data)
	}
	fmt.Printf("OK bytes=%d head=% x\n", len(data), data[:n])
	_ = os.WriteFile("d:/AI/tts_test.mp3", data, 0644)
	fmt.Println("saved d:/AI/tts_test.mp3")
}
