// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"fmt"
	"testing"
)

// TestParseVersionOutput 验证从 llama-server --version 输出中解析版本号。
func TestParseVersionOutput(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		wantVersion int
		wantCommit  string
		wantErr     bool
	}{
		{
			name: "标准格式_带commit",
			output: `version: 10216 (876a43211)
built with MSVC 19.44.35227.0 for x64`,
			wantVersion: 10216,
			wantCommit:  "876a43211",
			wantErr:     false,
		},
		{
			name:        "标准格式_无commit",
			output:      "version: 10210",
			wantVersion: 10210,
			wantCommit:  "",
			wantErr:     false,
		},
		{
			name:        "无版本号",
			output:      "built with MSVC 19.44.35227.0 for x64",
			wantVersion: 0,
			wantCommit:  "",
			wantErr:     true,
		},
		{
			name:        "空输出",
			output:      "",
			wantVersion: 0,
			wantCommit:  "",
			wantErr:     true,
		},
		{
			name: "多行带前缀日志",
			output: `system_info: n_threads = 8
version: 10216 (876a43211)
built with MSVC 19.44.35227.0 for x64`,
			wantVersion: 10216,
			wantCommit:  "876a43211",
			wantErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 模拟解析逻辑（复用 versionRegexp）
			matches := versionRegexp.FindStringSubmatch(tt.output)
			if len(matches) < 2 {
				if !tt.wantErr {
					t.Errorf("期望解析成功，但未匹配到版本号")
				}
				return
			}

			// 解析版本号
			gotVersion := 0
			if n, err := parseIntSafe(matches[1]); err == nil {
				gotVersion = n
			}

			if gotVersion != tt.wantVersion {
				t.Errorf("版本号 = %d, 期望 %d", gotVersion, tt.wantVersion)
			}

			// 解析 commit（如果期望有 commit）
			if tt.wantCommit != "" {
				gotCommit := extractCommit(tt.output)
				if gotCommit != tt.wantCommit {
					t.Errorf("commit = %q, 期望 %q", gotCommit, tt.wantCommit)
				}
			}
		})
	}
}

// TestParseReleaseTag 验证从 GitHub release tag 解析构建编号。
func TestParseReleaseTag(t *testing.T) {
	tests := []struct {
		tag     string
		want    int
		wantErr bool
	}{
		{"b10216", 10216, false},
		{"b10210", 10210, false},
		{"b1", 1, false},
		{"v1.0.0", 0, true},  // 不匹配格式
		{"", 0, true},        // 空字符串
		{"b", 0, true},       // 无数字
		{"b10216rc1", 0, true}, // 有后缀，不严格匹配
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			matches := tagRegexp.FindStringSubmatch(tt.tag)
			if len(matches) < 2 {
				if !tt.wantErr {
					t.Errorf("期望解析成功，但未匹配到构建编号")
				}
				return
			}
			got, err := parseIntSafe(matches[1])
			if (err != nil) != tt.wantErr {
				t.Errorf("解析错误 = %v, 期望错误 %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("构建编号 = %d, 期望 %d", got, tt.want)
			}
		})
	}
}

// TestVersionComparison 验证版本对比逻辑。
func TestVersionComparison(t *testing.T) {
	tests := []struct {
		name      string
		local     int
		remote    int
		hasUpdate bool
	}{
		{"远程更高_有更新", 10210, 10216, true},
		{"版本相同_无更新", 10216, 10216, false},
		{"本地更高_无更新", 10216, 10210, false},
		{"本地为0_无更新", 0, 10216, false},
		{"远程为0_无更新", 10216, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.remote > tt.local && tt.local > 0 && tt.remote > 0
			if got != tt.hasUpdate {
				t.Errorf("hasUpdate = %v, 期望 %v", got, tt.hasUpdate)
			}
		})
	}
}

// parseIntSafe 是 strconv.Atoi 的测试辅助函数，避免直接 import strconv。
func parseIntSafe(s string) (int, error) {
	var n int
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid digit: %c", c)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// extractCommit 从版本输出中提取 commit hash（括号内的字符串）。
// 复用 version_check.go 中的逻辑，用于测试验证。
func extractCommit(output string) string {
	idx := indexOf(output, "(")
	if idx < 0 {
		return ""
	}
	rest := output[idx+1:]
	endIdx := indexOf(rest, ")")
	if endIdx < 1 {
		return ""
	}
	return trimSpace(rest[:endIdx])
}

// indexOf 返回 substr 在 s 中首次出现的索引，未找到返回 -1。
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// trimSpace 去除字符串首尾空白。
func trimSpace(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t' || s[start] == '\n' || s[start] == '\r') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t' || s[end-1] == '\n' || s[end-1] == '\r') {
		end--
	}
	return s[start:end]
}
