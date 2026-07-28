// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"strings"
	"testing"
)

// TestRingBuffer_BasicWrite 验证 RingBuffer 基本写入和读取
// 生活类比：像环形跑道，跑满一圈后新的会覆盖最旧的。
func TestRingBuffer_BasicWrite(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Write([]byte("line1\n"))
	rb.Write([]byte("line2\n"))

	got := rb.String()
	if !strings.Contains(got, "line1") || !strings.Contains(got, "line2") {
		t.Errorf("应包含 line1 和 line2，实际: %q", got)
	}
}

// TestRingBuffer_Overflow 验证超过最大行数时旧行被丢弃
func TestRingBuffer_Overflow(t *testing.T) {
	rb := NewRingBuffer(2)
	rb.Write([]byte("line1\n"))
	rb.Write([]byte("line2\n"))
	rb.Write([]byte("line3\n"))

	got := rb.String()
	if strings.Contains(got, "line1") {
		t.Errorf("line1 应被丢弃，实际: %q", got)
	}
	if !strings.Contains(got, "line2") || !strings.Contains(got, "line3") {
		t.Errorf("应保留 line2 和 line3，实际: %q", got)
	}
}

// TestRingBuffer_MultiLineInOneWrite 验证一次写入多行
func TestRingBuffer_MultiLineInOneWrite(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("line1\nline2\nline3\n"))

	got := rb.String()
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Errorf("应有 3 行，实际 %d 行: %q", len(lines), got)
	}
}

// TestRingBuffer_EmptyLines 验证空行被跳过
func TestRingBuffer_EmptyLines(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("\n\nline1\n\n"))

	got := rb.String()
	if got != "line1" {
		t.Errorf("空行应被跳过，只保留 line1，实际: %q", got)
	}
}

// TestRingBuffer_OnChange 验证行写入回调
func TestRingBuffer_OnChange(t *testing.T) {
	rb := NewRingBuffer(10)
	var collected []string
	rb.SetOnChange(func(line string) {
		collected = append(collected, line)
	})

	rb.Write([]byte("line1\n"))
	rb.Write([]byte("line2\n"))

	if len(collected) != 2 {
		t.Errorf("回调应触发 2 次，实际 %d 次", len(collected))
	}
	if collected[0] != "line1" || collected[1] != "line2" {
		t.Errorf("回调应按顺序收到 line1, line2，实际: %v", collected)
	}
}

// TestRingBuffer_TeeWriter 验证 TeeWriter 同时写入外部 writer
func TestRingBuffer_TeeWriter(t *testing.T) {
	var teeBuf strings.Builder
	rb := NewRingBuffer(10).TeeWriter(&teeBuf)

	rb.Write([]byte("hello\n"))

	// 验证 RingBuffer 和 TeeWriter 都收到数据
	if !strings.Contains(rb.String(), "hello") {
		t.Errorf("RingBuffer 应包含 hello，实际: %q", rb.String())
	}
	if !strings.Contains(teeBuf.String(), "hello") {
		t.Errorf("TeeWriter 应包含 hello，实际: %q", teeBuf.String())
	}
}

// TestRingBuffer_CallbackCanCallString 验证回调内调用 String() 不会死锁
// 这是 S2 安全修复的核心测试
func TestRingBuffer_CallbackCanCallString(t *testing.T) {
	rb := NewRingBuffer(10)
	var callbackSawContent string
	rb.SetOnChange(func(line string) {
		// 回调内调用 String()，验证不会死锁
		callbackSawContent = rb.String()
	})

	rb.Write([]byte("test_line\n"))

	if !strings.Contains(callbackSawContent, "test_line") {
		t.Errorf("回调内应能读到已写入内容，实际: %q", callbackSawContent)
	}
}

// TestRingBuffer_NoTrailingNewline 验证无尾换行的内容也能写入
func TestRingBuffer_NoTrailingNewline(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("no newline"))

	got := rb.String()
	if got != "no newline" {
		t.Errorf("无尾换行应也能写入，实际: %q", got)
	}
}
