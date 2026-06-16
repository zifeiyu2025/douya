package rag

import (
	"archive/zip"
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	pdf "github.com/ledongthuc/pdf"
)

var textExtensions = map[string]bool{
	".txt":  true,
	".md":   true,
	".csv":  true,
	".json": true,
	".xml":  true,
	".html": true,
	".yaml": true,
	".yml":  true,
	".toml": true,
	".ini":  true,
	".cfg":  true,
	".log":  true,
	".sql":  true,
}

var codeExtensions = map[string]bool{
	".go":   true,
	".py":   true,
	".js":   true,
	".ts":   true,
	".java": true,
	".c":    true,
	".cpp":  true,
	".h":    true,
	".rs":   true,
	".sh":   true,
	".rb":   true,
	".php":  true,
	".swift": true,
	".kt":   true,
}

func ParseFileFromBytes(data []byte, fileName string) (string, error) {
	ext := strings.ToLower(filepath.Ext(fileName))

	if textExtensions[ext] || codeExtensions[ext] {
		return parseAsText(data)
	}

	switch ext {
	case ".pdf":
		text, err := extractPDFTextWithLib(data)
		if err != nil {
			// 库解析失败时回退到正则提取
			text = extractPDFTextFromBytes(data)
		}
		if text == "" {
			return "", fmt.Errorf("failed to extract text from PDF: %s", fileName)
		}
		return text, nil
	case ".docx":
		return parseDOCX(data)
	default:
		return parseAsText(data)
	}
}

func parseAsText(data []byte) (string, error) {
	if !utf8.Valid(data) {
		return "", fmt.Errorf("file content is not valid UTF-8")
	}
	return string(data), nil
}

// extractPDFTextWithLib 使用 ledongthuc/pdf 库提取 PDF 文本，支持中文和编码流
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

func extractPDFTextFromBytes(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	if !bytes.HasPrefix(data, []byte("%PDF")) {
		return ""
	}

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
		return ""
	}

	return strings.Join(cleaned, "\n")
}

func parseDOCX(data []byte) (string, error) {
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("failed to open docx as zip: %w", err)
	}

	var xmlContent []byte
	for _, f := range reader.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			if err != nil {
				return "", fmt.Errorf("failed to open word/document.xml: %w", err)
			}
			buf := new(bytes.Buffer)
			if _, err := buf.ReadFrom(rc); err != nil {
				rc.Close()
				return "", fmt.Errorf("failed to read word/document.xml: %w", err)
			}
			rc.Close()
			xmlContent = buf.Bytes()
			break
		}
	}

	if xmlContent == nil {
		return "", fmt.Errorf("word/document.xml not found in docx")
	}

	text := stripXMLTags(string(xmlContent))
	text = cleanWhitespace(text)

	if text == "" {
		return "", fmt.Errorf("no text content extracted from docx")
	}

	return text, nil
}

var xmlTagRe = regexp.MustCompile(`<[^>]+>`)

func stripXMLTags(xml string) string {
	text := xmlTagRe.ReplaceAllString(xml, " ")
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", "\"")
	text = strings.ReplaceAll(text, "&apos;", "'")
	text = strings.ReplaceAll(text, "&#10;", "\n")
	text = strings.ReplaceAll(text, "&#13;", "\r")
	text = strings.ReplaceAll(text, "&#9;", "\t")
	return text
}

func cleanWhitespace(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	multiSpaceRe := regexp.MustCompile(`[^\S\n]+`)
	text = multiSpaceRe.ReplaceAllString(text, " ")
	multiNewlineRe := regexp.MustCompile(`\n{3,}`)
	text = multiNewlineRe.ReplaceAllString(text, "\n\n")
	lines := strings.Split(text, "\n")
	var cleaned []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, "\n")
}
