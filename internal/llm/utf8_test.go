// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"strings"
	"testing"
)

// TestFixUTF8_ValidString 验证合法 UTF8 字符串原样返回
func TestFixUTF8_ValidString(t *testing.T) {
	input := "你好，世界！Hello, World! 123"
	got := FixUTF8(input)
	if got != input {
		t.Errorf("合法 UTF8 应原样返回，期望: %q，实际: %q", input, got)
	}
}

// TestFixUTF8_InvalidBytes 验证无效字节被替换为 U+FFFD
func TestFixUTF8_InvalidBytes(t *testing.T) {
	// 构造包含无效字节的字符串：\xff\xfe 不是合法 UTF8 序列
	input := "abc\xff\xfe123"
	got := FixUTF8(input)

	// 应包含替换字符 U+FFFD
	if !strings.Contains(got, "\uFFFD") {
		t.Errorf("无效字节应被替换为 U+FFFD，实际: %q", got)
	}
	// 合法部分应保留
	if !strings.Contains(got, "abc") {
		t.Errorf("合法部分 'abc' 应保留，实际: %q", got)
	}
	if !strings.Contains(got, "123") {
		t.Errorf("合法部分 '123' 应保留，实际: %q", got)
	}
}

// TestFixUTF8_EmptyString 验证空字符串处理
func TestFixUTF8_EmptyString(t *testing.T) {
	got := FixUTF8("")
	if got != "" {
		t.Errorf("空字符串应返回空，实际: %q", got)
	}
}

// TestFixUTF8_ChineseMixed 验证中英文混合字符串正确处理
func TestFixUTF8_ChineseMixed(t *testing.T) {
	input := "你好Hello世界World"
	got := FixUTF8(input)
	if got != input {
		t.Errorf("中英文混合合法 UTF8 应原样返回，期望: %q，实际: %q", input, got)
	}
}

// TestFixUTF8_PartialMultibyte 验证被截断的多字节序列被替换
func TestFixUTF8_PartialMultibyte(t *testing.T) {
	// "你" 的 UTF8 编码是 0xE4 0xBD 0xA0（3字节）
	// 截断最后一个字节，变成无效序列
	input := "abc" + string([]byte{0xE4, 0xBD}) + "def"
	got := FixUTF8(input)

	// 被截断的序列应被替换为 U+FFFD
	if !strings.Contains(got, "\uFFFD") {
		t.Errorf("截断的多字节序列应被替换为 U+FFFD，实际: %q", got)
	}
	// 合法部分应保留
	if !strings.Contains(got, "abc") {
		t.Errorf("合法部分 'abc' 应保留，实际: %q", got)
	}
	if !strings.Contains(got, "def") {
		t.Errorf("合法部分 'def' 应保留，实际: %q", got)
	}
}

// TestTruncateIncompleteUTF8_CompleteString 验证完整字符串全部返回 valid
func TestTruncateIncompleteUTF8_CompleteString(t *testing.T) {
	input := "你好，世界！"
	valid, pending := TruncateIncompleteUTF8(input)

	if valid != input {
		t.Errorf("valid 期望 %q，实际: %q", input, valid)
	}
	if pending != "" {
		t.Errorf("pending 期望空，实际: %q", pending)
	}
}

// TestTruncateIncompleteUTF8_IncompleteTrailing 验证尾部不完整序列被分离
func TestTruncateIncompleteUTF8_IncompleteTrailing(t *testing.T) {
	// "你" 的 UTF8 编码是 0xE4 0xBD 0xA0（3字节）
	// 只保留前 2 字节，形成不完整序列
	incomplete := string([]byte{0xE4, 0xBD}) // 2 字节，"你" 的前2字节
	input := "prefix" + incomplete

	valid, pending := TruncateIncompleteUTF8(input)

	// valid 应包含 "prefix"
	if !strings.Contains(valid, "prefix") {
		t.Errorf("valid 应包含 'prefix'，实际: %q", valid)
	}
	// pending 应为不完整序列
	if pending != incomplete {
		t.Errorf("pending 期望 %q，实际: %q", incomplete, pending)
	}
	// valid + pending 应能重组为 input
	if valid+pending != input {
		t.Errorf("valid+pending 应等于 input，valid=%q pending=%q input=%q", valid, pending, input)
	}
}

// TestTruncateIncompleteUTF8_EmptyString 验证空字符串处理
func TestTruncateIncompleteUTF8_EmptyString(t *testing.T) {
	valid, pending := TruncateIncompleteUTF8("")
	if valid != "" {
		t.Errorf("valid 期望空，实际: %q", valid)
	}
	if pending != "" {
		t.Errorf("pending 期望空，实际: %q", pending)
	}
}

// TestTruncateIncompleteUTF8_ASCIIOnly 验证纯 ASCII 全部返回 valid
func TestTruncateIncompleteUTF8_ASCIIOnly(t *testing.T) {
	input := "Hello, World! 123"
	valid, pending := TruncateIncompleteUTF8(input)

	if valid != input {
		t.Errorf("valid 期望 %q，实际: %q", input, valid)
	}
	if pending != "" {
		t.Errorf("pending 期望空，实际: %q", pending)
	}
}

// TestTruncateIncompleteUTF8_AllIncomplete 验证整个字符串都是不完整序列的情况
func TestTruncateIncompleteUTF8_AllIncomplete(t *testing.T) {
	// 只有 1 字节，且是 UTF8 多字节序列的起始字节
	input := string([]byte{0xE4}) // "你" 的第一字节

	valid, pending := TruncateIncompleteUTF8(input)

	// 整个字符串都是不完整序列，valid 应为空
	if valid != "" {
		t.Errorf("全不完整时 valid 期望空，实际: %q", valid)
	}
	if pending != input {
		t.Errorf("pending 期望 %q，实际: %q", input, pending)
	}
}
