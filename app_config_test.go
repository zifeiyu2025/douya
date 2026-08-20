// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"strings"
	"testing"
)

// TestGenerateAPIKeyString 验证一键生成 API Key 的格式与随机性。
//
// 契约：sk-douya- 前缀（业界通用格式）+ 48 位 base62 字符（≈285 bits 熵），
// 两次生成结果不同（crypto/rand 随机源）。
func TestGenerateAPIKeyString(t *testing.T) {
	const charset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"

	key1, err := generateAPIKeyString()
	if err != nil {
		t.Fatalf("生成 API Key 失败: %v", err)
	}

	// 格式：sk-douya- 前缀 + 48 字符，总长 57
	if !strings.HasPrefix(key1, "sk-douya-") {
		t.Errorf("key 应以 sk-douya- 开头，实际 %q", key1)
	}
	if wantLen := len("sk-douya-") + 48; len(key1) != wantLen {
		t.Errorf("key 总长应为 %d，实际 %d（key=%q）", wantLen, len(key1), key1)
	}

	// 字符集：随机部分只含 base62，不含符号/空白
	randomPart := strings.TrimPrefix(key1, "sk-douya-")
	for i, c := range randomPart {
		if !strings.ContainsRune(charset, c) {
			t.Errorf("随机段第 %d 字符 %q 不在 base62 字符集内", i, c)
		}
	}

	// 唯一性：连续生成 100 次应互不相同（碰撞概率可忽略）
	seen := make(map[string]bool, 100)
	for i := 0; i < 100; i++ {
		k, err := generateAPIKeyString()
		if err != nil {
			t.Fatalf("第 %d 次生成失败: %v", i, err)
		}
		if seen[k] {
			t.Fatalf("第 %d 次生成的 key 与之前重复: %q", i, k)
		}
		seen[k] = true
	}
}

// TestGenerateAPIKeyString_EntropySanity 熵量健全性抽查：
// 随机段应同时覆盖大写、小写、数字三类字符（纯巧合概率 < 2^-50）。
func TestGenerateAPIKeyString_EntropySanity(t *testing.T) {
	key, err := generateAPIKeyString()
	if err != nil {
		t.Fatalf("生成失败: %v", err)
	}
	part := strings.TrimPrefix(key, "sk-douya-")
	var hasUpper, hasLower, hasDigit bool
	for _, c := range part {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	if !hasUpper || !hasLower || !hasDigit {
		t.Errorf("随机段应覆盖三类字符（大写=%v 小写=%v 数字=%v），key=%q", hasUpper, hasLower, hasDigit, key)
	}
}
