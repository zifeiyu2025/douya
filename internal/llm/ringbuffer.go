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
	rb.mu.Lock()
	defer rb.mu.Unlock()

	text := string(p)
	newLines := strings.Split(strings.TrimRight(text, "\n"), "\n")

	for _, line := range newLines {
		if line == "" {
			continue
		}
		rb.lines = append(rb.lines, line)
		if len(rb.lines) > rb.max {
			rb.lines = rb.lines[len(rb.lines)-rb.max:]
		}
		// 触发回调（在锁内调用，回调实现需避免死锁，不能再次获取同一把锁）
		if rb.onChange != nil {
			rb.onChange(line)
		}
	}

	if rb.writer != nil {
		return rb.writer.Write(p)
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
