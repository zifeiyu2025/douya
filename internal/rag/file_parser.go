package rag

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"douya/internal/pdfutil"
)

// maxDOCXUncompressedSize 限制 DOCX 单个条目解压后的最大字节数（100MB）。
// 用于防御 zip bomb（高压缩比恶意文件）导致的 OOM 崩溃。
const maxDOCXUncompressedSize int64 = 100 * 1024 * 1024

// limitedReadAll 从 rc 读取内容，最多读取 limit 字节后停止。
// 返回读取到的字节切片（若内容超过 limit，则被截断到 limit 字节）。
// 调用者应检查返回的字节长度是否达到 limit，以判断内容是否超过限制。
// 这样可以防止恶意高压缩比 ZIP（zip bomb）在解压时占用过多内存。
func limitedReadAll(rc io.Reader, limit int64) ([]byte, error) {
	buf := new(bytes.Buffer)
	// io.LimitReader 会在读取到 limit 字节后返回 EOF，避免无限读取
	if _, err := io.Copy(buf, io.LimitReader(rc, limit)); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

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
		text := pdfutil.ExtractTextWithFallback(data, "")
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
			// 使用 limitedReadAll 限制解压后大小，防御 zip bomb 攻击
			xmlContent, err = limitedReadAll(rc, maxDOCXUncompressedSize)
			rc.Close()
			if err != nil {
				return "", fmt.Errorf("failed to read word/document.xml: %w", err)
			}
			// 检查读取到的字节数是否达到上限，若达到则说明内容超过 100MB 限制
			if int64(len(xmlContent)) >= maxDOCXUncompressedSize {
				return "", fmt.Errorf("DOCX 解压内容超过 100MB 限制，可能为恶意文件")
			}
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

var (
	xmlTagRe       = regexp.MustCompile(`<[^>]+>`)
	multiSpaceRe   = regexp.MustCompile(`[^\S\n]+`)
	multiNewlineRe = regexp.MustCompile(`\n{3,}`)
)

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
	text = multiSpaceRe.ReplaceAllString(text, " ")
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
