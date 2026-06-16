// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	pdf "github.com/ledongthuc/pdf"
)

// extractPDFText 提取 PDF 文本内容，优先使用 pdf 库，失败时回退到正则提取
func extractPDFText(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return ""
	}

	// 优先使用 pdf 库提取（支持中文、编码流等）
	text, err := extractPDFTextWithLib(data)
	if err == nil && text != "" {
		return text
	}

	// 回退到正则提取
	return extractPDFTextWithRegex(data)
}

// extractPDFTextWithLib 使用 ledongthuc/pdf 库提取 PDF 文本
func extractPDFTextWithLib(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("pdf reader: %w", err)
	}

	var buf strings.Builder
	pageCount := reader.NumPage()
	for i := 1; i <= pageCount; i++ {
		page := reader.Page(i)
		if page.V.IsNull() {
			continue
		}
		content, err := page.GetPlainText(nil)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(content)
		if text != "" {
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(text)
		}
	}

	result := buf.String()
	if result == "" {
		return "", fmt.Errorf("no text extracted from PDF")
	}
	return result, nil
}

// extractPDFTextWithRegex 使用正则提取 PDF 文本（fallback）
func extractPDFTextWithRegex(data []byte) string {
	text := string(data)

	streamRe := regexp.MustCompile(`(?s)stream\r?\n.*?\r?\nendstream`)
	text = streamRe.ReplaceAllString(text, "")

	binaryRe := regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f]`)
	text = binaryRe.ReplaceAllString(text, "")

	parenTextRe := regexp.MustCompile(`\(([^)]*)\)`)
	var texts []string
	matches := parenTextRe.FindAllStringSubmatch(text, -1)
	for _, m := range matches {
		if len(m) > 1 {
			cleaned := strings.TrimSpace(m[1])
			if len(cleaned) > 1 {
				texts = append(texts, cleaned)
			}
		}
	}

	arrayTextRe := regexp.MustCompile(`\[(.*?)\]`)
	arrayMatches := arrayTextRe.FindAllStringSubmatch(text, -1)
	for _, m := range arrayMatches {
		if len(m) > 1 {
			parenMatches := parenTextRe.FindAllStringSubmatch(m[1], -1)
			for _, pm := range parenMatches {
				if len(pm) > 1 {
					cleaned := strings.TrimSpace(pm[1])
					if len(cleaned) > 1 {
						texts = append(texts, cleaned)
					}
				}
			}
		}
	}

	result := strings.Join(texts, "\n")
	result = strings.ReplaceAll(result, "\\n", "\n")
	result = strings.ReplaceAll(result, "\\r", "")
	result = strings.ReplaceAll(result, "\\t", " ")

	lines := strings.Split(result, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}

	if len(cleaned) == 0 {
		return "[PDF文件无法提取文本内容]"
	}

	return strings.Join(cleaned, "\n")
}
