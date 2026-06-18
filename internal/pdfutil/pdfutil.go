// Package pdfutil 提供 PDF 文本提取的公共功能。
// 优先使用 ledongthuc/pdf 库提取（支持中文、编码流），
// 失败时回退到正则提取。
package pdfutil

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	pdf "github.com/ledongthuc/pdf"
)

// ExtractText 提取 PDF 文本内容。
// 空输入或非 PDF 文件返回空字符串。
// 库解析失败时回退到正则提取，正则也失败时返回 fallbackText。
func ExtractText(data []byte) string {
	return ExtractTextWithFallback(data, "[PDF文件无法提取文本内容]")
}

// ExtractTextWithFallback 提取 PDF 文本，允许自定义失败时的回退文本。
// 传入空字符串作为 fallback 时，失败返回空字符串。
func ExtractTextWithFallback(data []byte, fallback string) string {
	if len(data) == 0 {
		return ""
	}

	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return ""
	}

	// 优先使用 pdf 库提取（支持中文、编码流等）
	text, err := ExtractTextWithLib(data)
	if err == nil && text != "" {
		return text
	}

	// 回退到正则提取
	result := extractWithRegex(data)
	if result == "" {
		return fallback
	}
	return result
}

// ExtractTextWithLib 使用 ledongthuc/pdf 库提取 PDF 文本。
// 单页解析失败不中断，继续下一页。
func ExtractTextWithLib(data []byte) (string, error) {
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
			// 单页解析失败不中断，继续下一页
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

// extractWithRegex 使用正则提取 PDF 文本（fallback 方案）。
var (
	streamRe   = regexp.MustCompile(`(?s)stream\r?\n.*?\r?\nendstream`)
	binaryRe   = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f]`)
	parenTextRe = regexp.MustCompile(`\(([^)]*)\)`)
	arrayTextRe = regexp.MustCompile(`\[(.*?)\]`)
)

func extractWithRegex(data []byte) string {
	text := string(data)

	text = streamRe.ReplaceAllString(text, "")
	text = binaryRe.ReplaceAllString(text, "")

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

	return strings.Join(cleaned, "\n")
}
