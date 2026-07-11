// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import "testing"

// TestContentString_StringContent 验证 string 类型 Content 直接返回
//
// 生活类比：Content 就像一个快递包裹，可能是简单的一张纸条（string），
// 也可能是一个多层收纳盒（[]ContentPart）。如果是纸条，直接读上面的字就行。
func TestContentString_StringContent(t *testing.T) {
	msg := &ChatMessage{Role: "user", Content: "你好世界"}
	got := msg.ContentString()
	if got != "你好世界" {
		t.Errorf("string Content 期望 '你好世界'，实际 %q", got)
	}
}

// TestContentString_ContentPartsWithText 验证 []ContentPart 类型返回第一个 text part
func TestContentString_ContentPartsWithText(t *testing.T) {
	parts := []ContentPart{
		{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,abc"}},
		{Type: "text", Text: "这是图片说明"},
	}
	msg := &ChatMessage{Role: "user", Content: parts}
	got := msg.ContentString()
	if got != "这是图片说明" {
		t.Errorf("[]ContentPart 期望 '这是图片说明'，实际 %q", got)
	}
}

// TestContentString_ContentPartsWithoutText 验证 []ContentPart 无 text part 时返回空字符串
func TestContentString_ContentPartsWithoutText(t *testing.T) {
	parts := []ContentPart{
		{Type: "image_url", ImageURL: &ImageURL{URL: "data:image/png;base64,abc"}},
		{Type: "input_audio", InputAudio: &InputAudio{Data: "base64audio"}},
	}
	msg := &ChatMessage{Role: "user", Content: parts}
	got := msg.ContentString()
	if got != "" {
		t.Errorf("无 text part 的 []ContentPart 期望 ''，实际 %q", got)
	}
}

// TestContentString_AnySliceWithText 验证 []any 类型（JSON 反序列化场景）返回 text
// 当 ChatMessage 从 JSON 反序列化时，[]ContentPart 会变成 []any，每个元素是 map[string]any
func TestContentString_AnySliceWithText(t *testing.T) {
	// 模拟 JSON 反序列化后的结构
	anySlice := []any{
		map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": "data:image/png;base64,abc",
			},
		},
		map[string]any{
			"type": "text",
			"text": "从 JSON 解析的文本",
		},
	}
	msg := &ChatMessage{Role: "user", Content: anySlice}
	got := msg.ContentString()
	if got != "从 JSON 解析的文本" {
		t.Errorf("[]any 期望 '从 JSON 解析的文本'，实际 %q", got)
	}
}

// TestContentString_AnySliceWithoutText 验证 []any 无 text part 时返回空字符串
func TestContentString_AnySliceWithoutText(t *testing.T) {
	anySlice := []any{
		map[string]any{
			"type": "image_url",
			"image_url": map[string]any{"url": "abc"},
		},
	}
	msg := &ChatMessage{Role: "user", Content: anySlice}
	got := msg.ContentString()
	if got != "" {
		t.Errorf("无 text 的 []any 期望 ''，实际 %q", got)
	}
}

// TestContentString_NilContent 验证 nil Content 返回空字符串
func TestContentString_NilContent(t *testing.T) {
	msg := &ChatMessage{Role: "user", Content: nil}
	got := msg.ContentString()
	if got != "" {
		t.Errorf("nil Content 期望 ''，实际 %q", got)
	}
}

// TestContentString_NonStringContent 验证非 string/非 slice 类型返回空字符串
// Content 字段是 any 类型，可能被误赋值为 int 等，应优雅返回 ""
func TestContentString_NonStringContent(t *testing.T) {
	cases := []any{42, 3.14, true, nil}
	for _, c := range cases {
		msg := &ChatMessage{Role: "user", Content: c}
		got := msg.ContentString()
		if got != "" {
			t.Errorf("非 string/非 slice Content (%v) 期望 ''，实际 %q", c, got)
		}
	}
}

// TestNewTextMessage 验证 NewTextMessage 构造正确的文本消息
func TestNewTextMessage(t *testing.T) {
	msg := NewTextMessage("assistant", "回复内容")
	if msg.Role != "assistant" {
		t.Errorf("Role 期望 'assistant'，实际 %q", msg.Role)
	}
	if msg.Content != "回复内容" {
		t.Errorf("Content 期望 '回复内容'，实际 %v", msg.Content)
	}
}

// TestNewVisionMessage 验证 NewVisionMessage 构造正确的多模态消息
// 空文本应被替换为 "."（避免 llama-server 拒绝空 content）
func TestNewVisionMessage(t *testing.T) {
	// 有文本
	msg := NewVisionMessage("user", "看这张图", []string{"url1", "url2"})
	parts, ok := msg.Content.([]ContentPart)
	if !ok {
		t.Fatalf("Content 应为 []ContentPart，实际 %T", msg.Content)
	}
	if len(parts) != 3 { // 1 text + 2 image
		t.Errorf("应有 3 个 part（1 text + 2 image），实际 %d", len(parts))
	}
	if parts[0].Type != "text" || parts[0].Text != "看这张图" {
		t.Errorf("第一个 part 应为 text '看这张图'，实际 %+v", parts[0])
	}

	// 空文本应被替换为 "."
	msg2 := NewVisionMessage("user", "", []string{"url1"})
	parts2, _ := msg2.Content.([]ContentPart)
	if parts2[0].Text != "." {
		t.Errorf("空文本应替换为 '.'，实际 %q", parts2[0].Text)
	}
}
