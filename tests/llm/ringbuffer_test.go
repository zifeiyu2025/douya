package llm_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"douya/internal/llm"
)

func TestRingBuffer_WritesAndReads(t *testing.T) {
	rb := llm.NewRingBuffer(3)
	rb.Write([]byte("line1\n"))
	rb.Write([]byte("line2\n"))
	rb.Write([]byte("line3\n"))

	content := rb.String()
	if !strings.Contains(content, "line3") {
		t.Errorf("expected ring buffer to contain 'line3', got: %s", content)
	}
	if !strings.Contains(content, "line2") {
		t.Errorf("expected ring buffer to contain 'line2', got: %s", content)
	}
}

func TestRingBuffer_Overwrite(t *testing.T) {
	rb := llm.NewRingBuffer(2)
	rb.Write([]byte("line1\n"))
	rb.Write([]byte("line2\n"))
	rb.Write([]byte("line3\n"))

	content := rb.String()
	if strings.Contains(content, "line1") {
		t.Errorf("expected ring buffer to have overwritten 'line1', got: %s", content)
	}
	if !strings.Contains(content, "line2") {
		t.Errorf("expected ring buffer to contain 'line2', got: %s", content)
	}
	if !strings.Contains(content, "line3") {
		t.Errorf("expected ring buffer to contain 'line3', got: %s", content)
	}
}

func TestRingBuffer_Tee(t *testing.T) {
	var buf bytes.Buffer
	rb := llm.NewRingBuffer(3)
	tee := rb.TeeWriter(&buf)

	tee.Write([]byte("hello\n"))

	if !strings.Contains(buf.String(), "hello") {
		t.Errorf("expected tee writer to forward to underlying writer, got: %s", buf.String())
	}
	if !strings.Contains(rb.String(), "hello") {
		t.Errorf("expected ring buffer to capture output, got: %s", rb.String())
	}
}

func TestRingBuffer_Empty(t *testing.T) {
	rb := llm.NewRingBuffer(3)
	content := rb.String()
	if content != "" {
		t.Errorf("expected empty ring buffer, got: %s", content)
	}
}

// TestRingBuffer_OnChangeNoDeadlock 验证安全修复 S2：
// onChange 回调内调用 rb.String() 不应死锁。
// 原实现在持有 rb.mu 时调用回调，回调内再获取 rb.mu 会死锁。
// 修复后回调在锁外触发，回调内可安全调用 rb 任何方法。
func TestRingBuffer_OnChangeNoDeadlock(t *testing.T) {
	rb := llm.NewRingBuffer(10)
	deadline := make(chan struct{})
	go func() {
		defer close(deadline)
		// 设置一个会调用 rb.String() 的回调
		rb.SetOnChange(func(line string) {
			// 回调内读取 rb 内容——原实现会死锁
			_ = rb.String()
		})
		rb.Write([]byte("line1\nline2\n"))
	}()

	select {
	case <-deadline:
		// 测试通过
	case <-time.After(3 * time.Second):
		t.Fatal("onChange 回调内调用 rb.String() 死锁，修复 S2 失效")
	}

	// 验证 lines 正确写入
	if content := rb.String(); !strings.Contains(content, "line1") {
		t.Errorf("RingBuffer 应包含 line1，实际: %s", content)
	}
}
