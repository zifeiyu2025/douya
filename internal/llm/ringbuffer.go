package llm

import (
	"io"
	"strings"
	"sync"
)

type RingBuffer struct {
	mu       sync.Mutex
	lines    []string
	max      int
	writer   io.Writer
	onChange func(line string) // 每行写入时的回调（用于实时日志推送）
}

func NewRingBuffer(maxLines int) *RingBuffer {
	return &RingBuffer{
		max: maxLines,
	}
}

func (rb *RingBuffer) Write(p []byte) (n int, err error) {
	text := string(p)
	newLines := strings.SplitSeq(strings.TrimRight(text, "\n"), "\n")

	// 安全修复（S2）：原实现在持有 rb.mu 时调用 rb.onChange 和 rb.writer.Write，
	// 若回调或 writer 内部再次访问 RingBuffer 任何方法（如 String/Write）会死锁。
	// 修复：锁内只更新 lines 并收集待触发的回调与 writer，锁外触发。
	var pendingCallbacks []string
	var cb func(string)
	var writer io.Writer
	rb.mu.Lock()
	for line := range newLines {
		if line == "" {
			continue
		}
		rb.lines = append(rb.lines, line)
		if len(rb.lines) > rb.max {
			rb.lines = rb.lines[len(rb.lines)-rb.max:]
		}
		if rb.onChange != nil {
			pendingCallbacks = append(pendingCallbacks, line)
		}
	}
	cb = rb.onChange
	writer = rb.writer
	rb.mu.Unlock()

	// 锁外触发回调：回调内可安全调用 rb.String() / rb.Write() 等方法
	if cb != nil {
		for _, line := range pendingCallbacks {
			cb(line)
		}
	}

	// 锁外写入 tee writer，避免 writer 内部回调导致死锁
	if writer != nil {
		return writer.Write(p)
	}
	return len(p), nil
}

func (rb *RingBuffer) String() string {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return strings.Join(rb.lines, "\n")
}

// SetOnChange 设置行写入回调，每写入一行都会触发
func (rb *RingBuffer) SetOnChange(cb func(line string)) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.onChange = cb
}

func (rb *RingBuffer) TeeWriter(w io.Writer) *RingBuffer {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.writer = w
	return rb
}
