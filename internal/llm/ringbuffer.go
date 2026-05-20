package llm

import (
	"io"
	"strings"
	"sync"
)

type RingBuffer struct {
	mu     sync.Mutex
	lines  []string
	max    int
	writer io.Writer
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

func (rb *RingBuffer) TeeWriter(w io.Writer) *RingBuffer {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.writer = w
	return rb
}
