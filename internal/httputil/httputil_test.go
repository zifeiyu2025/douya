// Package httputil 提供 HTTP 相关的公共工具函数。
package httputil

import (
	"io"
	"testing"
	"strings"
)

// TestReadBodyLimited 正常读取小于限制的内容
func TestReadBodyLimited(t *testing.T) {
	input := "hello world"
	r := strings.NewReader(input)
	got, err := ReadBodyLimited(r, 1024)
	if err != nil {
		t.Fatalf("ReadBodyLimited returned error: %v", err)
	}
	if string(got) != input {
		t.Fatalf("expected %q, got %q", input, string(got))
	}
}

// TestReadBodyLimitedTruncates 超过限制的内容应被截断
func TestReadBodyLimitedTruncates(t *testing.T) {
	// 生成 100 字节内容，限制为 10 字节
	input := strings.Repeat("a", 100)
	r := strings.NewReader(input)
	got, err := ReadBodyLimited(r, 10)
	if err != nil {
		t.Fatalf("ReadBodyLimited returned error: %v", err)
	}
	if len(got) != 10 {
		t.Fatalf("expected 10 bytes, got %d", len(got))
	}
}

// TestReadBodyLimitedEmpty 空输入返回空字节
func TestReadBodyLimitedEmpty(t *testing.T) {
	r := strings.NewReader("")
	got, err := ReadBodyLimited(r, 1024)
	if err != nil {
		t.Fatalf("ReadBodyLimited returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d bytes", len(got))
	}
}

// 确保 io 包被使用（避免未使用导入）
var _ = io.EOF
