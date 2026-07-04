// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"encoding/base64"
	"strings"
	"testing"

	"douya/internal/llm"
)

// TestValidateAttachment_ValidImage 验证合法图片通过校验
func TestValidateAttachment_ValidImage(t *testing.T) {
	att := Attachment{
		Type:     "image",
		Name:     "test.png",
		MimeType: "image/png",
		Data:     "data:image/png;base64,iVBORw0KGgo=",
	}
	decodedLen, ok := validateAttachment(att)
	if !ok {
		t.Error("合法图片应通过校验")
	}
	if decodedLen <= 0 {
		t.Errorf("decodedLen 应大于 0，实际: %d", decodedLen)
	}
}

// TestValidateAttachment_InvalidMime 验证非法 MIME 类型被拒绝
func TestValidateAttachment_InvalidMime(t *testing.T) {
	att := Attachment{
		Type:     "image",
		Name:     "malicious.exe",
		MimeType: "application/x-msdownload",
		Data:     "data:image/png;base64,iVBORw0KGgo=",
	}
	_, ok := validateAttachment(att)
	if ok {
		t.Error("非法 MIME 类型应被拒绝")
	}
}

// TestValidateAttachment_EmptyMime 验证空 MIME 允许通过（兼容旧前端）
func TestValidateAttachment_EmptyMime(t *testing.T) {
	att := Attachment{
		Type:     "image",
		Name:     "old_image",
		MimeType: "", // 空 MIME
		Data:     "data:image/png;base64,iVBORw0KGgo=",
	}
	_, ok := validateAttachment(att)
	if !ok {
		t.Error("空 MIME 应允许通过（兼容旧前端）")
	}
}

// TestValidateAttachment_OversizedImage 验证超大图片被拒绝
func TestValidateAttachment_OversizedImage(t *testing.T) {
	// 构造超过 200MB 的 base64 数据
	// 200MB = 200 * 1024 * 1024 = 209715200 字节
	// base64 编码后约 280MB，DecodedLen 会返回 > 200MB
	hugeData := strings.Repeat("A", 280*1024*1024)
	att := Attachment{
		Type:     "image",
		Name:     "huge.png",
		MimeType: "image/png",
		Data:     hugeData,
	}
	_, ok := validateAttachment(att)
	if ok {
		t.Error("超过 200MB 的图片应被拒绝")
	}
}

// TestValidateAttachment_ValidPDF 验证合法 PDF 通过校验
func TestValidateAttachment_ValidPDF(t *testing.T) {
	// 构造一个最小的 PDF base64 编码
	pdfContent := "%PDF-1.0\n1 0 obj<</Pages 2 0 R>>endobj\n2 0 obj<</Kids[]>>endobj\nxref\n0 3\n0000000000 65535 f \ntrailer<</Root 1 0 R>>\nstartxref\n0\n%%EOF"
	encoded := base64.StdEncoding.EncodeToString([]byte(pdfContent))
	att := Attachment{
		Type:     "pdf",
		Name:     "test.pdf",
		MimeType: "application/pdf",
		Data:     encoded,
	}
	decodedLen, ok := validateAttachment(att)
	if !ok {
		t.Error("合法 PDF 应通过校验")
	}
	if decodedLen <= 0 {
		t.Errorf("decodedLen 应大于 0，实际: %d", decodedLen)
	}
}

// TestValidateAttachment_InvalidBase64PDF 验证无效 base64 PDF 被拒绝
func TestValidateAttachment_InvalidBase64PDF(t *testing.T) {
	att := Attachment{
		Type:     "pdf",
		Name:     "corrupt.pdf",
		MimeType: "application/pdf",
		Data:     "这不是有效的base64数据!!!",
	}
	_, ok := validateAttachment(att)
	if ok {
		t.Error("无效 base64 PDF 应被拒绝")
	}
}

// TestValidateAttachment_UnknownType 验证未知类型返回 -1（非 base64）
func TestValidateAttachment_UnknownType(t *testing.T) {
	att := Attachment{
		Type:     "unknown",
		Name:     "unknown.txt",
		MimeType: "text/plain",
		Data:     "plain text data",
	}
	decodedLen, ok := validateAttachment(att)
	if !ok {
		t.Error("未知类型应通过校验（返回 -1）")
	}
	if decodedLen != -1 {
		t.Errorf("未知类型 decodedLen 应为 -1，实际: %d", decodedLen)
	}
}

// TestBuildMessageFromAttachments_ImageOnly 验证仅图片附件构建正确的 ChatMessage
func TestBuildMessageFromAttachments_ImageOnly(t *testing.T) {
	attachments := []Attachment{
		{
			Type:     "image",
			Name:     "test.png",
			MimeType: "image/png",
			Data:     "data:image/png;base64,iVBORw0KGgo=",
		},
	}
	msg := buildMessageFromAttachments("user", "看这张图", attachments)

	if msg.Role != "user" {
		t.Errorf("Role 期望 user，实际: %s", msg.Role)
	}
	// 应为 []llm.ContentPart（多模态内容）
	parts, ok := msg.Content.([]llm.ContentPart)
	if !ok {
		t.Fatalf("Content 应为 []llm.ContentPart，实际: %T", msg.Content)
	}
	// 至少应包含文本部分和图片部分
	hasImage := false
	for _, p := range parts {
		if p.Type == "image_url" && p.ImageURL != nil {
			hasImage = true
			break
		}
	}
	if !hasImage {
		t.Errorf("ContentParts 应包含 image_url 类型，实际: %+v", parts)
	}
}

// TestBuildMessageFromAttachments_NoAttachments 验证无附件时构建普通文本消息
func TestBuildMessageFromAttachments_NoAttachments(t *testing.T) {
	msg := buildMessageFromAttachments("user", "纯文本消息", nil)

	if msg.Role != "user" {
		t.Errorf("Role 期望 user，实际: %s", msg.Role)
	}
	if msg.Content != "纯文本消息" {
		t.Errorf("Content 期望 '纯文本消息'，实际: %v", msg.Content)
	}
}

// TestBuildMessageFromAttachments_RejectedAttachment 验证被拒绝附件生成文本提示
func TestBuildMessageFromAttachments_RejectedAttachment(t *testing.T) {
	attachments := []Attachment{
		{
			Type:     "image",
			Name:     "malicious.exe",
			MimeType: "application/x-msdownload", // 非法 MIME
			Data:     "data:image/png;base64,iVBORw0KGgo=",
		},
	}
	msg := buildMessageFromAttachments("user", "用户消息", attachments)

	// 被拒绝的附件应生成文本提示
	contentStr, ok := msg.Content.(string)
	if !ok {
		t.Fatalf("被拒绝附件时 Content 应为 string，实际: %T", msg.Content)
	}
	if !strings.Contains(contentStr, "附件被拒绝") {
		t.Errorf("Content 应包含'附件被拒绝'提示，实际: %s", contentStr)
	}
}
