package main

import (
	"testing"

	"douya/internal/llm"
)

// TestFindWindowsAsset 测试 Windows 安装包资产匹配逻辑
// 覆盖当前发布命名（windows.zip）与历史命名（windows-amd64.zip），
// 防止因上游资产命名变化导致"检查更新"误报 NotFound。
func TestFindWindowsAsset(t *testing.T) {
	tests := []struct {
		name   string
		assets []llm.GitHubAsset
		want   string // 期望的 BrowserDownloadURL，空串表示不命中
	}{
		{
			name: "当前发布命名 windows.zip",
			assets: []llm.GitHubAsset{
				{Name: "Douya-v0.11.6-windows.zip", BrowserDownloadURL: "https://example.com/windows.zip"},
			},
			want: "https://example.com/windows.zip",
		},
		{
			name: "历史命名 windows-amd64.zip",
			assets: []llm.GitHubAsset{
				{Name: "Douya-v0.10.0-windows-amd64.zip", BrowserDownloadURL: "https://example.com/windows-amd64.zip"},
			},
			want: "https://example.com/windows-amd64.zip",
		},
		{
			name: "同时存在时优先 amd64",
			assets: []llm.GitHubAsset{
				{Name: "Douya-v0.11.6-windows.zip", BrowserDownloadURL: "https://example.com/windows.zip"},
				{Name: "Douya-v0.11.6-windows-amd64.zip", BrowserDownloadURL: "https://example.com/windows-amd64.zip"},
			},
			want: "https://example.com/windows-amd64.zip",
		},
		{
			name: "混合资产中命中 windows",
			assets: []llm.GitHubAsset{
				{Name: "Douya-v0.11.6-linux.zip", BrowserDownloadURL: "https://example.com/linux.zip"},
				{Name: "Douya-v0.11.6-macos.zip", BrowserDownloadURL: "https://example.com/macos.zip"},
				{Name: "Douya-v0.11.6-windows.zip", BrowserDownloadURL: "https://example.com/windows.zip"},
			},
			want: "https://example.com/windows.zip",
		},
		{
			name: "无 Windows 资产返回空",
			assets: []llm.GitHubAsset{
				{Name: "Douya-v0.11.6-linux.zip", BrowserDownloadURL: "https://example.com/linux.zip"},
			},
			want: "",
		},
		{
			name:   "空资产返回空",
			assets: nil,
			want:   "",
		},
		{
			name: "无关命名不误匹配",
			assets: []llm.GitHubAsset{
				{Name: "source-code.zip", BrowserDownloadURL: "https://example.com/source.zip"},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := findWindowsAsset(tt.assets)
			if got != tt.want {
				t.Errorf("findWindowsAsset() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestCompareVersions 测试版本号比较逻辑
// 生活类比：就像比较两个学生的成绩单——先比第一位（主版本），
// 相同再比第二位（次版本），再相同比第三位（补丁号）。
func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int // 1: a>b, -1: a<b, 0: a==b
	}{
		// 基本比较
		{"相等", "0.11.2", "0.11.2", 0},
		{"补丁号不同(小)", "0.11.2", "0.11.3", -1},
		{"补丁号不同(大)", "0.11.3", "0.11.2", 1},
		{"次版本号不同", "0.10.5", "0.11.0", -1},
		{"主版本号不同", "0.11.0", "1.0.0", -1},

		// 关键边界：数字比较 vs 字符串比较
		// 这是 v0.11.2 vs v0.11.10 的核心用例：字符串比较 "2" > "10" 是错误的
		{"补丁号10vs2(数字比较)", "0.11.10", "0.11.2", 1},
		{"补丁号2vs10(数字比较)", "0.11.2", "0.11.10", -1},

		// 多位数
		{"两位数补丁号", "0.11.99", "0.11.100", -1},
		{"两位数次版本号", "0.99.0", "0.100.0", -1},

		// 位数不同
		{"a多一位(补丁号缺省为0)", "0.11.2.1", "0.11.2", 1},
		{"b多一位(补丁号缺省为0)", "0.11.2", "0.11.2.1", -1},

		// 大版本号
		{"大版本号", "10.0.0", "9.99.99", 1},

		// 异常格式（Atoi 失败返回 0）
		{"非数字段(当0处理)", "0.11.abc", "0.11.0", 0},
		{"空字符串", "", "", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
