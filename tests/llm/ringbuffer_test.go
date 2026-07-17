package llm_test

import (
	"bytes"
	"strings"
	"testing"

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
