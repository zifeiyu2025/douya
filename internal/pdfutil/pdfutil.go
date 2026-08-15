// Package pdfutil 提供 PDF 文本提取的公共功能。
// 优先使用 ledongthuc/pdf 库提取（支持中文、编码流），
// 失败时回退到正则提取。
package pdfutil

import (
	"bytes"
	"regexp"
	"strings"
	"sync"

	pdf "github.com/ledongthuc/pdf"
	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
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
// 并行解析各页，按页码顺序合并结果。单页解析失败不中断，继续其他页。
// 注：ledongthuc/pdf 的 Reader 字段在 NewReader 后只读，resolve 每次创建独立
// buffer 和 SectionReader，并发访问不同页面是安全的。
func ExtractTextWithLib(data []byte) (string, error) {
	reader, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", apperror.Wrap(apperror.KindInternal, "pdf reader", err)
	}

	pageCount := reader.NumPage()
	if pageCount == 0 {
		return "", apperror.New(apperror.KindInternal, "no pages in PDF")
	}

	// 并行解析各页，结果存入按页码索引的数组以保证顺序。
	// M12 修复：用带缓冲的 channel 作为信号量限制并发 worker 数量，
	// 避免恶意声明大量页面的 PDF 一次性扇出海量 goroutine 导致内存/并发尖峰。
	pages := make([]string, pageCount)
	var wg sync.WaitGroup
	maxWorkers := 8
	if maxWorkers > pageCount {
		maxWorkers = pageCount
	}
	sem := make(chan struct{}, maxWorkers)
	for i := 1; i <= pageCount; i++ {
		wg.Add(1)
		sem <- struct{}{} // 获取 worker 槽位，满时阻塞等待
		go func(pageNum int) {
			defer wg.Done()
			defer func() { <-sem }() // 释放槽位
			defer func() {
				// 防御性 recover：单页 panic 不影响其他页，页面解析异常时保留空字符串
				if r := recover(); r != nil {
					log.Error().Interface("panic", r).Int("page", pageNum).
						Msg("[pdfutil] PDF 单页解析 panic，已跳过该页")
				}
			}()
			page := reader.Page(pageNum)
			if page.V.IsNull() {
				return
			}
			content, err := page.GetPlainText(nil)
			if err != nil {
				log.Warn().Err(err).Int("page", pageNum).
					Msg("[pdfutil] PDF 单页文本提取失败，已跳过该页")
				return
			}
			pages[pageNum-1] = strings.TrimSpace(content)
		}(i)
	}
	wg.Wait()

	// 按顺序合并非空页
	var buf strings.Builder
	for _, text := range pages {
		if text != "" {
			if buf.Len() > 0 {
				buf.WriteString("\n")
			}
			buf.WriteString(text)
		}
	}

	result := buf.String()
	if result == "" {
		return "", apperror.New(apperror.KindInternal, "no text extracted from PDF")
	}
	return result, nil
}

// extractWithRegex 使用正则提取 PDF 文本（fallback 方案）。
var (
	streamRe    = regexp.MustCompile(`(?s)stream\r?\n.*?\r?\nendstream`)
	binaryRe    = regexp.MustCompile(`[\x00-\x08\x0b\x0c\x0e-\x1f]`)
	parenTextRe = regexp.MustCompile(`\(([^)]*)\)`)
	arrayTextRe = regexp.MustCompile(`\[(.*?)\]`)
)

// maxRegexExtractSize 限制正则提取的输入大小（50MB）。
// 安全实践（SEC-001）：防止超大 PDF 导致内存放大或 ReDoS。
// 超过此大小的 PDF 回退到截断处理，仍能提取前 50MB 的文本。
const maxRegexExtractSize = 50 * 1024 * 1024

// maxRegexMatches 限制正则匹配结果数量，防止极端输入产生过多匹配导致内存膨胀。
const maxRegexMatches = 5000

func extractWithRegex(data []byte) string {
	// SEC-001: 超大 PDF 截断后再正则处理，防止内存放大
	if len(data) > maxRegexExtractSize {
		data = data[:maxRegexExtractSize]
	}
	text := string(data)

	text = streamRe.ReplaceAllString(text, "")
	text = binaryRe.ReplaceAllString(text, "")

	var texts []string
	matches := parenTextRe.FindAllStringSubmatch(text, maxRegexMatches)
	for _, m := range matches {
		if len(m) > 1 {
			cleaned := strings.TrimSpace(m[1])
			if len(cleaned) > 1 {
				texts = append(texts, cleaned)
			}
		}
	}

	arrayMatches := arrayTextRe.FindAllStringSubmatch(text, maxRegexMatches)
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
