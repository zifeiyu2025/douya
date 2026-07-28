package llm

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"
)

// === 以下用例合并自 tests/llm/ringbuffer_test.go（黑盒测试） ===

func TestRingBuffer_WritesAndReads(t *testing.T) {
	rb := NewRingBuffer(3)
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
	rb := NewRingBuffer(2)
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
	rb := NewRingBuffer(3)
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
	rb := NewRingBuffer(3)
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
	rb := NewRingBuffer(10)
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

// === 以下为白盒测试用例 ===

// TestRingBufferTruncateNoMemoryLeak 验证 RingBuffer 截断后底层数组容量不持续增长。
// 修复 PERF-5: 原代码 rb.lines = rb.lines[len-rb.max:] 只移动切片头指针，
// 底层数组前半部分仍被引用无法 GC，长时间运行内存会持续增长。
// 修复方案：显式 copy 到新切片，释放原底层数组引用。
func TestRingBufferTruncateNoMemoryLeak(t *testing.T) {
	const maxLines = 100
	rb := NewRingBuffer(maxLines)

	// 写入大量数据（远超 maxLines），触发多次截断
	const totalLines = 10000
	for i := 0; i < totalLines; i++ {
		// 每行写入一定长度的内容，模拟真实日志输出
		line := "log line " + strings.Repeat("x", 50) + "\n"
		if _, err := rb.Write([]byte(line)); err != nil {
			t.Fatalf("写入第 %d 行出错: %v", i, err)
		}
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	// 验证 1: 行数被正确截断到 maxLines
	if len(rb.lines) != maxLines {
		t.Fatalf("期望 len(rb.lines)=%d, 实际=%d", maxLines, len(rb.lines))
	}

	// 验证 2: 底层数组容量不应超过 maxLines*2
	// 修复前: 截断只移动头指针，cap 会持续增长到 totalLines 级别（约 10000）
	// 修复后: 显式 copy 到新切片，cap 应该保持在合理范围（max 或 2*max 附近）
	if cap(rb.lines) > maxLines*2 {
		t.Fatalf("底层数组容量泄漏: cap(rb.lines)=%d, 期望不超过 %d (maxLines*2)",
			cap(rb.lines), maxLines*2)
	}

	t.Logf("修复后: len=%d, cap=%d (maxLines=%d)", len(rb.lines), cap(rb.lines), maxLines)
}

// TestRingBufferTruncateContentCorrect 验证截断后保留的是最后 maxLines 行内容
func TestRingBufferTruncateContentCorrect(t *testing.T) {
	const maxLines = 5
	rb := NewRingBuffer(maxLines)

	// 写入 10 行，应保留最后 5 行（6-10）
	for i := 1; i <= 10; i++ {
		line := "line-" + strconv.Itoa(i) + "\n"
		if _, err := rb.Write([]byte(line)); err != nil {
			t.Fatalf("写入第 %d 行出错: %v", i, err)
		}
	}

	got := rb.String()
	want := "line-6\nline-7\nline-8\nline-9\nline-10"
	if got != want {
		t.Fatalf("截断后内容不正确\n期望: %q\n实际: %q", want, got)
	}
}

// TestRingBufferNoTruncateUnderMax 验证未超过 max 时不触发截断
func TestRingBufferNoTruncateUnderMax(t *testing.T) {
	const maxLines = 100
	rb := NewRingBuffer(maxLines)

	// 写入不超过 max 的行数
	for i := 0; i < 50; i++ {
		_, _ = rb.Write([]byte("short\n"))
	}

	rb.mu.Lock()
	defer rb.mu.Unlock()

	if len(rb.lines) != 50 {
		t.Fatalf("期望 len=50, 实际=%d", len(rb.lines))
	}
}
