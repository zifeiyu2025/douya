// Package pdfutil 提供 PDF 文本提取的公共功能。
package pdfutil

import (
	"testing"
)

// TestExtractTextEmptyInput 空输入返回空字符串
func TestExtractTextEmptyInput(t *testing.T) {
	got := ExtractText(nil)
	if got != "" {
		t.Fatalf("expected empty string for nil input, got %q", got)
	}
	got = ExtractText([]byte{})
	if got != "" {
		t.Fatalf("expected empty string for empty input, got %q", got)
	}
}

// TestExtractTextNonPDF 非PDF文件返回空字符串
func TestExtractTextNonPDF(t *testing.T) {
	got := ExtractText([]byte("not a pdf file"))
	if got != "" {
		t.Fatalf("expected empty string for non-PDF input, got %q", got)
	}
}

// TestExtractTextInvalidPDF 无效PDF（有%PDF头但内容损坏）返回提示或空
func TestExtractTextInvalidPDF(t *testing.T) {
	// 有 %PDF 头但内容损坏，库解析失败，回退到正则
	data := []byte("%PDF-1.4\nbroken content")
	got := ExtractText(data)
	// 正则提取失败时返回提示文本或空，不应 panic
	// 这里只验证不 panic 且返回字符串
	_ = got
}

// TestExtractTextWithLibInvalid 无效数据返回错误
func TestExtractTextWithLibInvalid(t *testing.T) {
	_, err := ExtractTextWithLib([]byte("invalid"))
	if err == nil {
		t.Fatal("expected error for invalid PDF data, got nil")
	}
}
