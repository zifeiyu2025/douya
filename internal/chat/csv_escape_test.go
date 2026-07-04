// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"testing"
)

// TestCSVEscape_PlainText 验证纯文本原样返回
func TestCSVEscape_PlainText(t *testing.T) {
	input := "Hello World"
	got := csvEscape(input)
	if got != input {
		t.Errorf("纯文本应原样返回，期望: %q，实际: %q", input, got)
	}
}

// TestCSVEscape_ContainsQuotes 验证双引号被转义为两个双引号
func TestCSVEscape_ContainsQuotes(t *testing.T) {
	input := `他说"你好"`
	got := csvEscape(input)
	expected := `他说""你好""`
	if got != expected {
		t.Errorf("双引号应被转义为两个双引号，期望: %q，实际: %q", expected, got)
	}
}

// TestCSVEscape_CRLF 验证 \r\n 被替换为 \n
func TestCSVEscape_CRLF(t *testing.T) {
	input := "line1\r\nline2"
	got := csvEscape(input)
	expected := "line1\nline2"
	if got != expected {
		t.Errorf("\\r\\n 应被替换为 \\n，期望: %q，实际: %q", expected, got)
	}
}

// TestCSVEscape_LoneCR 验证单独的 \r 被替换为 \n
func TestCSVEscape_LoneCR(t *testing.T) {
	input := "line1\rline2"
	got := csvEscape(input)
	expected := "line1\nline2"
	if got != expected {
		t.Errorf("单独 \\r 应被替换为 \\n，期望: %q，实际: %q", expected, got)
	}
}

// TestCSVEscape_EmptyString 验证空字符串处理
func TestCSVEscape_EmptyString(t *testing.T) {
	got := csvEscape("")
	if got != "" {
		t.Errorf("空字符串应返回空，实际: %q", got)
	}
}

// TestCSVEscape_MixedContent 验证混合内容正确处理
func TestCSVEscape_MixedContent(t *testing.T) {
	input := "标题: \"测试\"\r\n内容: 第二行"
	got := csvEscape(input)
	expected := "标题: \"\"测试\"\"\n内容: 第二行"
	if got != expected {
		t.Errorf("混合内容处理错误，期望: %q，实际: %q", expected, got)
	}
}
