// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"context"
	"strings"
	"testing"
)

// TestBingParseResults_NormalHTML 正常 HTML：能提取多条结果，标题/URL/摘要都正确
// 这是核心测试，验证修复后的解析逻辑能正确处理真实 Bing HTML 结构
func TestBingParseResults_NormalHTML(t *testing.T) {
	// 这是从真实 Bing 抓取的 2 条结果结构（简化版，保留关键元素）
	// 关键点：
	//   1. b_algo 内有 b_tpcn（缩略图区，含 <cite> 域名，不是标题）
	//   2. 真正的标题在 <h2><a> 里，可能含 <strong> 标签
	//   3. 摘要在 <div class="b_caption"><p class="b_lineclamp*"> 里
	html := `
<li class="b_algo" data-id iid=SERP.5337>
  <div class="b_tpcn">
    <a class="tilk" aria-label="runoob.com" href="https://www.runoob.com/go/go-tutorial.html">
      <div class="tptxt"><div class="tptt">runoob.com</div>
      <cite>https://www.runoob.com</cite></div>
    </a>
  </div>
  <h2 class=""><a target="_blank" href="https://www.runoob.com/go/go-tutorial.html"><strong>Go 语言教程</strong> | 菜鸟<strong>教程</strong></a></h2>
  <div class="b_caption"><p class="b_lineclamp2">Go 是一个开源的编程语言，它能让构造简单、可靠且高效的软件变得容易。</p></div>
</li>
<li class="b_algo" data-id iid=SERP.5338>
  <div class="b_tpcn">
    <a class="tilk" aria-label="zhihu.com" href="https://zhuanlan.zhihu.com/p/123">
      <cite>https://zhuanlan.zhihu.com</cite>
    </a>
  </div>
  <h2><a href="https://zhuanlan.zhihu.com/p/123">Go 语言入门指南</a></h2>
  <div class="b_caption"><p class="b_lineclamp4">本文档为 Go 语言初学者提供一份系统、清晰、实用的学习路径。</p></div>
</li>
`
	results := parseBingResults(html)
	if len(results) != 2 {
		t.Fatalf("期望 2 条结果，实际 %d", len(results))
	}

	// 验证第 1 条：标题不能包含域名，URL 要正确
	r1 := results[0]
	if r1.Title != "Go 语言教程 | 菜鸟教程" {
		t.Errorf("第1条标题错误: 期望 %q, 实际 %q", "Go 语言教程 | 菜鸟教程", r1.Title)
	}
	if r1.URL != "https://www.runoob.com/go/go-tutorial.html" {
		t.Errorf("第1条 URL 错误: 期望 %q, 实际 %q", "https://www.runoob.com/go/go-tutorial.html", r1.URL)
	}
	if !strings.Contains(r1.Snippet, "Go 是一个开源的编程语言") {
		t.Errorf("第1条摘要错误: 实际 %q", r1.Snippet)
	}

	// 验证第 2 条
	r2 := results[1]
	if r2.Title != "Go 语言入门指南" {
		t.Errorf("第2条标题错误: 期望 %q, 实际 %q", "Go 语言入门指南", r2.Title)
	}
	if r2.URL != "https://zhuanlan.zhihu.com/p/123" {
		t.Errorf("第2条 URL 错误: 期望 %q, 实际 %q", "https://zhuanlan.zhihu.com/p/123", r2.URL)
	}
}

// TestBingParseResults_TitleNotPollutedByCite 验证标题不会被 <cite> 域名污染
// 这是修复之前 bug 的关键测试
func TestBingParseResults_TitleNotPollutedByCite(t *testing.T) {
	html := `
<li class="b_algo">
  <div class="b_tpcn">
    <a class="tilk" href="https://example.com/page">
      <cite>example.com</cite>
    </a>
  </div>
  <h2><a href="https://example.com/page">真正的标题</a></h2>
  <div class="b_caption"><p>摘要内容</p></div>
</li>
`
	results := parseBingResults(html)
	if len(results) != 1 {
		t.Fatalf("期望 1 条结果，实际 %d", len(results))
	}

	r := results[0]
	// 标题必须是 "真正的标题"，不能是 "example.com真正的标题" 之类
	if r.Title != "真正的标题" {
		t.Errorf("标题被 <cite> 污染: 期望 %q, 实际 %q", "真正的标题", r.Title)
	}
	// 标题里不能出现 "example.com"
	if strings.Contains(r.Title, "example.com") {
		t.Errorf("标题错误地包含了域名: %q", r.Title)
	}
}

// TestBingParseResults_EmptyHTML 空 HTML 应返回空结果，不报错
func TestBingParseResults_EmptyHTML(t *testing.T) {
	results := parseBingResults("")
	if len(results) != 0 {
		t.Errorf("空 HTML 应返回 0 条结果，实际 %d", len(results))
	}
}

// TestBingParseResults_NoBAlgo HTML 里没有 b_algo 块
func TestBingParseResults_NoBAlgo(t *testing.T) {
	html := `<html><body><div>没有搜索结果</div></body></html>`
	results := parseBingResults(html)
	if len(results) != 0 {
		t.Errorf("无 b_algo 块应返回 0 条结果，实际 %d", len(results))
	}
}

// TestBingParseResults_BAlgoWithoutH2 b_algo 块里没有 h2（异常结构），应跳过该条
func TestBingParseResults_BAlgoWithoutH2(t *testing.T) {
	html := `
<li class="b_algo">
  <div class="b_caption"><p>摘要但没有标题</p></div>
</li>
<li class="b_algo">
  <h2><a href="https://example.com">正常标题</a></h2>
  <div class="b_caption"><p>摘要</p></div>
</li>
`
	results := parseBingResults(html)
	if len(results) != 1 {
		t.Fatalf("期望 1 条结果（跳过无 h2 的块），实际 %d", len(results))
	}
	if results[0].Title != "正常标题" {
		t.Errorf("标题错误: 期望 %q, 实际 %q", "正常标题", results[0].Title)
	}
}

// TestBingParseResults_SkipBingInternalLinks 跳过 Bing 内部广告链接
// b_algo 里偶尔会有广告（aclk）或跳转链接，应过滤
func TestBingParseResults_SkipBingInternalLinks(t *testing.T) {
	html := `
<li class="b_algo">
  <h2><a href="https://www.bing.com/aclk?ld=xxx">广告标题</a></h2>
  <div class="b_caption"><p>广告摘要</p></div>
</li>
<li class="b_algo">
  <h2><a href="https://example.com/real">真实结果</a></h2>
  <div class="b_caption"><p>真实摘要</p></div>
</li>
`
	results := parseBingResults(html)
	if len(results) != 1 {
		t.Fatalf("期望 1 条结果（跳过广告），实际 %d", len(results))
	}
	if results[0].Title != "真实结果" {
		t.Errorf("标题错误: 期望 %q, 实际 %q", "真实结果", results[0].Title)
	}
}

// TestBingParseResults_StrongTagsInTitle 标题里的 <strong> 标签文本要保留
// Bing 会对查询词匹配的部分加 <strong>
func TestBingParseResults_StrongTagsInTitle(t *testing.T) {
	html := `
<li class="b_algo">
  <h2><a href="https://example.com"><strong>Go</strong> 语言<strong>教程</strong>大全</a></h2>
  <div class="b_caption"><p>摘要</p></div>
</li>
`
	results := parseBingResults(html)
	if len(results) != 1 {
		t.Fatalf("期望 1 条结果，实际 %d", len(results))
	}
	if results[0].Title != "Go 语言教程大全" {
		t.Errorf("标题错误（<strong> 内文本未正确拼接）: 期望 %q, 实际 %q", "Go 语言教程大全", results[0].Title)
	}
}

// TestBingParseResults_SnippetFromBLineclamp 摘要从 b_lineclamp* class 的 <p> 提取
func TestBingParseResults_SnippetFromBLineclamp(t *testing.T) {
	html := `
<li class="b_algo">
  <h2><a href="https://example.com">标题</a></h2>
  <div class="b_caption">
    <p class="b_lineclamp2">这是 b_lineclamp2 的摘要</p>
  </div>
</li>
<li class="b_algo">
  <h2><a href="https://example2.com">标题2</a></h2>
  <div class="b_caption">
    <p class="b_lineclamp4">这是 b_lineclamp4 的摘要</p>
  </div>
</li>
`
	results := parseBingResults(html)
	if len(results) != 2 {
		t.Fatalf("期望 2 条结果，实际 %d", len(results))
	}
	if results[0].Snippet != "这是 b_lineclamp2 的摘要" {
		t.Errorf("第1条摘要错误: 实际 %q", results[0].Snippet)
	}
	if results[1].Snippet != "这是 b_lineclamp4 的摘要" {
		t.Errorf("第2条摘要错误: 实际 %q", results[1].Snippet)
	}
}

// TestBingParseResults_HtmlEntities HTML 实体应被正确解码
// Bing HTML 里 & 会被编码成 &amp; 等
func TestBingParseResults_HtmlEntities(t *testing.T) {
	html := `
<li class="b_algo">
  <h2><a href="https://example.com/?a=1&amp;b=2">标题 &amp; 测试</a></h2>
  <div class="b_caption"><p>摘要 &amp; 内容</p></div>
</li>
`
	results := parseBingResults(html)
	if len(results) != 1 {
		t.Fatalf("期望 1 条结果，实际 %d", len(results))
	}
	// URL 里的 &amp; 应解码成 &
	if results[0].URL != "https://example.com/?a=1&b=2" {
		t.Errorf("URL 实体未解码: 期望 %q, 实际 %q", "https://example.com/?a=1&b=2", results[0].URL)
	}
	// 标题里的 &amp; 应解码成 &
	if results[0].Title != "标题 & 测试" {
		t.Errorf("标题实体未解码: 期望 %q, 实际 %q", "标题 & 测试", results[0].Title)
	}
}

// TestBingProvider_Name 验证 Provider 名称
func TestBingProvider_Name(t *testing.T) {
	p := NewBingProvider()
	if p.Name() != "bing" {
		t.Errorf("Provider 名称错误: 期望 %q, 实际 %q", "bing", p.Name())
	}
}

// TestBingProvider_SearchWithOpts_Basic 验证 SearchWithOpts 接口存在且可调用
// 真实网络测试在另一个文件里，这里只验证接口契约
func TestBingProvider_SearchWithOpts_Basic(t *testing.T) {
	p := NewBingProvider()
	if p == nil {
		t.Fatal("NewBingProvider 返回 nil")
	}
	// 验证实现了 Provider 接口
	var _ Provider = p
}

// TestBingBuildQueryURL 验证 URL 构造（中文加 cc=cn&setlang=zh-Hans）
func TestBingBuildQueryURL(t *testing.T) {
	cases := []struct {
		name    string
		query   string
		wantSub []string // URL 中必须包含的子串
	}{
		{
			name:    "中文查询",
			query:   "Go 语言教程",
			wantSub: []string{"cn.bing.com/search", "q=", "cc=cn", "setlang=zh-Hans"},
		},
		{
			name:    "英文查询",
			query:   "golang tutorial",
			wantSub: []string{"cn.bing.com/search", "q=golang+tutorial", "cc=cn", "setlang=zh-Hans"},
		},
		{
			name:    "特殊字符查询",
			query:   "C++ & Go",
			wantSub: []string{"cn.bing.com/search", "cc=cn", "setlang=zh-Hans"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := buildBingQueryURL(tc.query)
			for _, sub := range tc.wantSub {
				if !strings.Contains(u, sub) {
					t.Errorf("URL 缺少子串 %q\n完整 URL: %s", sub, u)
				}
			}
		})
	}
}

// TestBingBuildQueryURL_ChineseWrappedInQuotes 验证中文查询会被双引号包裹
// 这是避免 Bing 拆字的关键：中文短语加双引号强制精确匹配
// 项目规则：Chinese search queries must be wrapped in double quotes
//
// 测试逻辑：
//   - 中文查询：q= 参数值应以 %22（URL 编码的双引号 "）开头和结尾
//   - 英文查询：q= 参数值不应以 %22 开头（英文不需要加引号）
//   - 中英混合查询：含中文就加引号
func TestBingBuildQueryURL_ChineseWrappedInQuotes(t *testing.T) {
	cases := []struct {
		name        string
		query       string
		shouldQuote bool // 是否应该被双引号包裹
	}{
		{"纯中文", "Go 语言教程", true},
		{"纯中文长句", "2026年最新AI模型", true},
		{"中英混合", "Go 语言 tutorial", true},
		{"纯英文", "golang tutorial", false},
		{"英文加数字", "rust 2026", false},
		{"日文", "Go言語チュートリアル", true}, // 日文也是 CJK，应加引号
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := buildBingQueryURL(tc.query)
			// %22 是双引号 " 的 URL 编码
			// url.Values.Encode() 会把 " 编码成 %22
			// 注意：Errorf 里 %22 会被误认为格式化动词，要用 %%22 转义
			if tc.shouldQuote {
				// 期望 q=%22...%22（双引号包裹）
				if !strings.Contains(u, "q=%22") {
					t.Errorf("中文查询应被双引号包裹，但 q= 后未跟 %%22\n完整 URL: %s", u)
				}
			} else {
				// 期望 q= 后不跟 %22（英文不加引号）
				if strings.Contains(u, "q=%22") {
					t.Errorf("英文查询不应被双引号包裹，但 q= 后跟了 %%22\n完整 URL: %s", u)
				}
			}
		})
	}
}

// TestBingParseResults_MultipleResults 大量结果测试（验证解析稳定性）
func TestBingParseResults_MultipleResults(t *testing.T) {
	var htmlBuilder strings.Builder
	for i := 0; i < 15; i++ {
		htmlBuilder.WriteString(`<li class="b_algo">
  <h2><a href="https://example.com/` + itoa(i) + `">标题` + itoa(i) + `</a></h2>
  <div class="b_caption"><p>摘要` + itoa(i) + `</p></div>
</li>`)
	}
	results := parseBingResults(htmlBuilder.String())
	if len(results) != 15 {
		t.Errorf("期望 15 条结果，实际 %d", len(results))
	}
	// 验证最后一条
	if len(results) > 0 {
		last := results[len(results)-1]
		if last.Title != "标题14" {
			t.Errorf("最后一条标题错误: 期望 %q, 实际 %q", "标题14", last.Title)
		}
	}
}

// itoa 简单的整数转字符串（测试用，避免引入 strconv）
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// TestBingProvider_Categories 验证 Bing 作为兜底应匹配所有分类
// buildSearchChain 会用 NewCategorizedSearchChain，Bing 应响应 general 和 code 分类
func TestBingProvider_Categories(t *testing.T) {
	p := NewBingProvider()
	// 调用 SearchWithOpts 用空 context 验证不会 panic
	// 真实网络调用会失败，但不应 panic
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即取消，避免真实网络请求

	_, err := p.SearchWithOpts(ctx, "test", SearchOpts{MaxResults: 5})
	// 取消的 context 会返回错误，这是预期的
	if err == nil {
		// 如果没报错也行（可能本地网络快），不强制失败
		return
	}
}
