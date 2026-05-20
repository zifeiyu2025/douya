// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"bytes"
	"regexp"
	"strings"
)

func extractPDFText(data []byte) string {
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
		return "[PDF文件无法提取文本内容]"
	}

	return strings.Join(cleaned, "\n")
}
