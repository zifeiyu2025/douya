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
	decodedLen, ok := validateAttachment(att, 0)
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
	_, ok := validateAttachment(att, 0)
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
	_, ok := validateAttachment(att, 0)
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
	_, ok := validateAttachment(att, 0)
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
	decodedLen, ok := validateAttachment(att, 0)
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
	_, ok := validateAttachment(att, 0)
	if ok {
		t.Error("无效 base64 PDF 应被拒绝")
	}
}

// TestValidateAttachment_UnknownType 验证未知类型被拒绝（SEC-007: 默认拒绝策略）
func TestValidateAttachment_UnknownType(t *testing.T) {
	att := Attachment{
		Type:     "unknown",
		Name:     "unknown.txt",
		MimeType: "text/plain",
		Data:     "plain text data",
	}
	_, ok := validateAttachment(att, 0)
	if ok {
		t.Error("未知类型应被拒绝（SEC-007: 默认拒绝策略）")
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

// ===== 附件 Token 估算测试 =====
//
// 这些测试验证 EstimateAttachmentTokensWithData 和 estimateAttachmentTokensFromJSON
// 的正确性，确保上下文溢出防御不会因估算为 0 而失效。

// TestEstimateAttachmentTokensWithData_KnownTypes 验证已知附件类型返回正确的估算值
func TestEstimateAttachmentTokensWithData_KnownTypes(t *testing.T) {
	cases := []struct {
		name    string
		attType string
		data    string
		wantMin int // 期望最小值（大于0）
		wantMax int // 期望最大值（合理上界）
	}{
		{"image", "image", "base64data", 3500, 3500},
		{"IMAGE大写", "IMAGE", "base64data", 3500, 3500},
		{"video", "video", "base64data", 5000, 5000},
		{"audio", "audio", "base64data", 500, 500},
		{"text非空", "text", "hello world", 10, 100},
		{"pdf非空", "pdf", base64.StdEncoding.EncodeToString([]byte("%PDF-1.0 test")), 10, 500},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := EstimateAttachmentTokensWithData(c.attType, c.data)
			if got < c.wantMin {
				t.Errorf("EstimateAttachmentTokensWithData(%q, ...) = %d, 期望 >= %d", c.attType, got, c.wantMin)
			}
			if got > c.wantMax {
				t.Errorf("EstimateAttachmentTokensWithData(%q, ...) = %d, 期望 <= %d", c.attType, got, c.wantMax)
			}
		})
	}
}

// TestEstimateAttachmentTokensWithData_EmptyData text/pdf 空数据应返回0
// 注意：image/video/audio 即使 data 为空也返回估算值（data 可能是 URL 引用而非 base64）
func TestEstimateAttachmentTokensWithData_EmptyData(t *testing.T) {
	// text 和 pdf 的 data 为原始内容/base64，空时无内容应返回0
	cases := []string{"text", "pdf"}
	for _, attType := range cases {
		got := EstimateAttachmentTokensWithData(attType, "")
		if got != 0 {
			t.Errorf("EstimateAttachmentTokensWithData(%q, '') = %d, 期望 0", attType, got)
		}
	}
}

// TestEstimateAttachmentTokensWithData_UnknownType 验证未知附件类型返回保守估算值而非0
//
// Bug 场景：当附件类型不在已知列表中（如未来新增的 "spreadsheet" 类型），
// 旧代码返回 0，导致上下文溢出防御失效。应返回保守默认值。
//
// 生活类比：安检时遇到不认识的包裹，宁可多算一些重量（保守估算），
// 也不能当作没重量（返回0），否则飞机可能超载。
func TestEstimateAttachmentTokensWithData_UnknownType(t *testing.T) {
	got := EstimateAttachmentTokensWithData("spreadsheet", "some base64 data")
	if got <= 0 {
		t.Errorf("未知附件类型 'spreadsheet' 返回 %d, 期望 > 0（保守估算防止上下文溢出）", got)
	}
	// 应返回合理的保守值（与 invalid JSON 的 fallback 1500 一致）
	if got < 1000 {
		t.Errorf("未知附件类型返回 %d, 期望 >= 1000（足够保守以防溢出）", got)
	}
}

// TestEstimateAttachmentTokensFromJSON_ValidMultiple 验证多个附件的 JSON 正确累加
func TestEstimateAttachmentTokensFromJSON_ValidMultiple(t *testing.T) {
	json := `[{"type":"image","data":"abc"},{"type":"audio","data":"xyz"}]`
	got := estimateAttachmentTokensFromJSON(json)
	// image(3500) + audio(500) = 4000
	if got != 4000 {
		t.Errorf("多个附件累加 = %d, 期望 4000 (image 3500 + audio 500)", got)
	}
}

// TestEstimateAttachmentTokensFromJSON_Empty 空字符串返回0
func TestEstimateAttachmentTokensFromJSON_Empty(t *testing.T) {
	got := estimateAttachmentTokensFromJSON("")
	if got != 0 {
		t.Errorf("空字符串应返回 0, 实际 %d", got)
	}
}

// TestEstimateAttachmentTokensFromJSON_InvalidJSON 无效JSON走fallback
func TestEstimateAttachmentTokensFromJSON_InvalidJSON(t *testing.T) {
	// 无效 JSON 但包含 "video" 关键词
	got := estimateAttachmentTokensFromJSON("this is not json but has video keyword")
	if got != 5000 {
		t.Errorf("无效JSON含video应返回 5000, 实际 %d", got)
	}
	// 无效 JSON 且无任何已知关键词
	got = estimateAttachmentTokensFromJSON("totally garbage data")
	if got != 1500 {
		t.Errorf("无效JSON无关键词应返回 1500, 实际 %d", got)
	}
}
