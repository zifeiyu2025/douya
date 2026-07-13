// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

// 真实网络回归测试：调用真实的 BingProvider，验证端到端能拿到干净结果。
// 这是验证单元测试通过后的"最后一公里"测试。

package search_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"douya/internal/search"
)

// TestBingProvider_RealNetwork 真实网络测试
// 验证：NewBingProvider → SearchWithOpts → 拿到干净结果（标题无域名污染）
func TestBingProvider_RealNetwork(t *testing.T) {
	// CI 环境下跳过真实网络测试
	if os.Getenv("CI") != "" {
		t.Skip("跳过真实网络测试（CI 环境）")
	}
	// 跳过短测试模式，只在正常 go test 时运行
	if testing.Short() {
		t.Skip("跳过真实网络测试（-short 模式）")
	}

	provider := search.NewBingProvider()
	if provider == nil {
		t.Fatal("NewBingProvider 返回 nil")
	}

	queries := []struct {
		name  string
		query string
	}{
		{"中文查询", "Go 语言教程"},
		{"英文查询", "golang tutorial"},
		{"时效性查询", "2026年最新AI模型"},
	}

	// 重置汇总文件
	os.WriteFile("bing_network_summary.txt", []byte{}, 0644)
	appendLine := func(s string) {
		f, _ := os.OpenFile("bing_network_summary.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			f.WriteString(s + "\n")
			f.Close()
		}
	}

	appendLine("========== Bing Provider 真实网络回归测试 ==========")

	for _, tc := range queries {
		t.Run(tc.name, func(t *testing.T) {
			appendLine("")
			appendLine(fmt.Sprintf("查询: %q", tc.query))

			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()

			resp, err := provider.SearchWithOpts(ctx, tc.query, search.SearchOpts{
				MaxResults:    5,
				IncludeAnswer: false,
			})
			if err != nil {
				appendLine(fmt.Sprintf("  ❌ 请求失败: %v", err))
				t.Fatalf("请求失败: %v", err)
			}

			appendLine(fmt.Sprintf("  引擎: %s, 结果数: %d", resp.Engine, len(resp.Results)))
			t.Logf("引擎: %s, 结果数: %d", resp.Engine, len(resp.Results))

			if len(resp.Results) == 0 {
				appendLine("  ❌ 未获取到任何结果")
				t.Fatal("未获取到任何结果")
			}

			// 验证每条结果的质量
			for i, r := range resp.Results {
				// 标题不能为空
				if r.Title == "" {
					appendLine(fmt.Sprintf("  ❌ 结果[%d] 标题为空", i+1))
					t.Errorf("结果[%d] 标题为空", i+1)
					continue
				}
				// URL 不能为空
				if r.URL == "" {
					appendLine(fmt.Sprintf("  ❌ 结果[%d] URL 为空", i+1))
					t.Errorf("结果[%d] URL 为空", i+1)
					continue
				}
				// 标题不能包含 http（说明被 cite 域名污染了）
				if strings.Contains(r.Title, "http://") || strings.Contains(r.Title, "https://") {
					appendLine(fmt.Sprintf("  ❌ 结果[%d] 标题被 URL 污染: %q", i+1, r.Title))
					t.Errorf("结果[%d] 标题被 URL 污染: %q", i+1, r.Title)
					continue
				}

				appendLine(fmt.Sprintf("  [%d] %s", i+1, r.Title))
				appendLine(fmt.Sprintf("      URL: %s", truncateURL(r.URL)))
				snippet := r.Snippet
				if len(snippet) > 100 {
					snippet = snippet[:100] + "..."
				}
				appendLine(fmt.Sprintf("      摘要: %s", snippet))
			}

			// 至少要有 3 条有效结果
			if len(resp.Results) < 3 {
				appendLine(fmt.Sprintf("  ⚠️ 有效结果数偏少（%d < 3）", len(resp.Results)))
				t.Errorf("有效结果数偏少: %d < 3", len(resp.Results))
			}
		})
	}
}

// TestBingProvider_RealNetwork_EnglishOnly 单独跑英文查询，验证英文场景
func TestBingProvider_RealNetwork_EnglishOnly(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("跳过真实网络测试（CI 环境）")
	}
	if testing.Short() {
		t.Skip("跳过真实网络测试（-short 模式）")
	}

	provider := search.NewBingProvider()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	resp, err := provider.Search(ctx, "rust programming language")
	if err != nil {
		t.Fatalf("请求失败: %v", err)
	}

	if len(resp.Results) == 0 {
		t.Fatal("未获取到任何结果")
	}

	// 英文查询应该能搜到 rust-lang.org 或类似权威源
	found := false
	for _, r := range resp.Results {
		t.Logf("标题: %s | URL: %s", r.Title, r.URL)
		if strings.Contains(strings.ToLower(r.URL), "rust-lang.org") ||
			strings.Contains(strings.ToLower(r.URL), "wikipedia.org") ||
			strings.Contains(strings.ToLower(r.URL), "github.com") {
			found = true
		}
	}
	if !found {
		t.Log("⚠️ 未找到 rust-lang.org/wikipedia.org/github.com 等权威源，但搜索本身成功")
	}
}

// truncateURL 截断 URL 便于显示
func truncateURL(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}
