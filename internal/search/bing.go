// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

// Bing HTML 搜索 Provider（免 API Key 兜底搜索）
//
// 设计说明：
//   - 使用 cn.bing.com 国内版，国内访问稳定
//   - 通过解析 HTML 提取搜索结果，不需要 API Key
//   - 作为 Tavily/Ollama 之后的兜底搜索引擎
//
// 解析逻辑（基于真实抓取的 HTML 结构）：
//   <li class="b_algo">
//     <div class="b_tpcn">  <!-- 缩略图区，含 <cite> 域名，不是标题，跳过 -->
//       <a class="tilk" href="..."><cite>example.com</cite></a>
//     </div>
//     <h2><a href="真实URL"><strong>标题</strong>文本</a></h2>  <!-- 真正的标题 -->
//     <div class="b_caption"><p class="b_lineclamp*">摘要</p></div>  <!-- 摘要 -->
//   </li>
//
// 关键修复：标题只从 <h2><a> 提取，避免被 b_tpcn 里的 <cite> 域名污染

package search

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"

	"douya/internal/apperror"
)

// bingEndpoint Bing 国内版搜索端点
const bingEndpoint = "https://cn.bing.com/search"

// BingProvider Bing HTML 搜索 Provider（免 API Key）
type BingProvider struct {
	BaseProvider
}

// NewBingProvider 创建 Bing Provider
// 超时设为 15s：与 Tavily 一致，避免兜底搜索引擎拖慢整体响应
func NewBingProvider() *BingProvider {
	return &BingProvider{
		BaseProvider: BaseProvider{
			httpClient: newSearchHTTPClient(15 * time.Second),
		},
	}
}

// Name 返回 Provider 名称
func (p *BingProvider) Name() string {
	return "bing"
}

// Search 以默认参数执行搜索（maxResults=5）
func (p *BingProvider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	return p.SearchWithOpts(ctx, query, SearchOpts{MaxResults: 5})
}

// SearchWithOpts 执行带选项的搜索
// 免 API Key，直接 GET cn.bing.com/search?q=xxx&cc=cn&setlang=zh-Hans
func (p *BingProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	if strings.TrimSpace(query) == "" {
		return nil, apperror.New(apperror.KindInvalidInput, "bing: query is empty")
	}

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 5
	}

	targetURL := buildBingQueryURL(query)

	// 设置浏览器 UA，避免被基本反爬拦截
	headers := map[string]string{
		"User-Agent":      "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
		"Accept":          "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
		"Accept-Language": "zh-CN,zh;q=0.9,en;q=0.8",
	}

	respBody, err := p.doSearch(ctx, http.MethodGet, targetURL, nil, headers)
	if err != nil {
		return nil, apperror.Wrap(apperror.KindUnavailable, "bing", err)
	}

	results := parseBingResults(string(respBody))

	// 限制结果数量
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	// 去重和过滤（复用通用工具函数）
	results = dedupAndFilterResults(results)

	searchResp := &SearchResponse{
		Engine:  p.Name(),
		Results: results,
	}

	return searchResp, nil
}

// buildBingQueryURL 构造 Bing 搜索 URL
// 中文查询加 cc=cn&setlang=zh-Hans 参数（项目惯例，优化中文结果）
// 中文/CJK 查询自动加双引号强制精确匹配，避免 Bing 拆字（项目规则：
// Chinese search queries must be wrapped in double quotes to force exact
// phrase matching and avoid word splitting）
func buildBingQueryURL(query string) string {
	q := query
	// 含 CJK 字符的查询加双引号，避免被搜索引擎按词拆分
	// 例："2026年最新AI模型" 不加引号会被拆成 "2026年" + "最新" + "AI模型" 分别匹配
	if containsCJK(query) {
		q = `"` + query + `"`
	}
	params := url.Values{}
	params.Set("q", q)
	params.Set("cc", "cn")
	params.Set("setlang", "zh-Hans")
	return bingEndpoint + "?" + params.Encode()
}

// containsCJK 检查字符串是否包含 CJK（中日韩）字符
// CJK 统一表意文字范围：U+4E00 ~ U+9FFF（常用汉字）
// CJK 扩展 A：U+3400 ~ U+4DBF（生僻字）
// 日文假名：U+3040 ~ U+30FF（平假名+片假名）
// 韩文音节：U+AC00 ~ U+D7AF
// 全角字符：U+FF00 ~ U+FFEF
func containsCJK(s string) bool {
	for _, r := range s {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF: // CJK 统一表意文字
			return true
		case r >= 0x3400 && r <= 0x4DBF: // CJK 扩展 A
			return true
		case r >= 0x3040 && r <= 0x30FF: // 日文假名
			return true
		case r >= 0xAC00 && r <= 0xD7AF: // 韩文音节
			return true
		case r >= 0xFF00 && r <= 0xFFEF: // 全角字符
			return true
		}
	}
	return false
}

// maxBingHTMLSize 限制 Bing HTML 响应体的最大大小（5MB）。
// 安全实践（SEC-002）：纵深防御，防止异常大响应导致 html.Parse 内存暴涨。
// doSearch 已限制 10MB，此处再收紧到 5MB 作为独立函数级自我保护。
const maxBingHTMLSize = 5 * 1024 * 1024

// parseBingResults 从 Bing HTML 响应中解析搜索结果
// 核心逻辑：找 <li class="b_algo">，在每个块内提取 <h2><a> 的标题和 URL，
// 以及 <div class="b_caption"><p> 的摘要。
//
// 关键：标题只从 <h2> 内的 <a> 提取，跳过 b_tpcn 里的缩略图链接（避免 <cite> 域名污染）
func parseBingResults(htmlStr string) []SearchResult {
	if strings.TrimSpace(htmlStr) == "" {
		return nil
	}
	// SEC-002: 纵深防御，超大响应直接返回空结果
	if len(htmlStr) > maxBingHTMLSize {
		return nil
	}

	doc, err := html.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return nil
	}

	var results []SearchResult
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		// 找 <li class="b_algo">
		if n.Type == html.ElementNode && n.Data == "li" && hasAttrClass(n, "b_algo") {
			if r, ok := parseBingAlgoBlock(n); ok {
				results = append(results, r)
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	return results
}

// parseBingAlgoBlock 解析单个 b_algo 块，返回一条搜索结果
// 返回 ok=false 表示该块无效（无标题/无 URL/广告链接），应跳过
func parseBingAlgoBlock(li *html.Node) (SearchResult, bool) {
	var r SearchResult

	// 1. 找 <h2>，再在 <h2> 内找 <a>，取标题和 URL
	h2 := findFirstByTag(li, "h2")
	if h2 == nil {
		return r, false
	}
	a := findFirstByTag(h2, "a")
	if a == nil {
		return r, false
	}

	href := getAttrValue(a, "href")
	if href == "" {
		return r, false
	}

	// 跳过 Bing 内部广告链接（aclk）和微软跳转链接
	if isBingInternalLink(href) {
		return r, false
	}

	// 跳过非 http 链接（如 javascript:、# 等）
	if !strings.HasPrefix(href, "http://") && !strings.HasPrefix(href, "https://") {
		return r, false
	}

	r.URL = href
	// 标题：收集 <a> 内所有文本（含 <strong> 里的字）
	r.Title = strings.TrimSpace(collectNodeText(a))
	if r.Title == "" {
		return r, false
	}

	// 2. 找摘要：在 b_algo 内找 <div class="b_caption">，取里面第一个 <p> 的文本
	r.Snippet = extractBingSnippet(li)

	return r, true
}

// extractBingSnippet 从 b_algo 块中提取摘要
// Bing 摘要结构：<div class="b_caption"><p class="b_lineclamp*">摘要文本</p></div>
// 兜底：若无 b_caption，取块内第一个 <p> 文本
func extractBingSnippet(li *html.Node) string {
	var snippet string
	var captionDiv *html.Node

	// 找 b_caption div
	var findCaption func(*html.Node)
	findCaption = func(n *html.Node) {
		if captionDiv != nil {
			return
		}
		if n.Type == html.ElementNode && n.Data == "div" && hasAttrClass(n, "b_caption") {
			captionDiv = n
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			findCaption(c)
		}
	}
	findCaption(li)

	// 在 b_caption 内找第一个非空 <p>
	if captionDiv != nil {
		snippet = strings.TrimSpace(collectNodeText(findFirstByTag(captionDiv, "p")))
	}

	// 兜底：若 b_caption 里没有 <p>，在整个 b_algo 内找第一个 <p>
	if snippet == "" {
		if p := findFirstByTag(li, "p"); p != nil {
			snippet = strings.TrimSpace(collectNodeText(p))
		}
	}

	return snippet
}

// isBingInternalLink 判断是否为 Bing 内部广告/跳转链接，应跳过
func isBingInternalLink(href string) bool {
	// Bing 广告链接格式：https://www.bing.com/aclk?...
	// 微软跳转链接：https://go.microsoft.com/...
	// Bing 内部链接：https://www.bing.com/...
	lower := strings.ToLower(href)
	return strings.Contains(lower, "bing.com/aclk") ||
		strings.Contains(lower, "go.microsoft.com") ||
		strings.HasPrefix(lower, "https://www.bing.com/bingsearch")
}

// ---------------------------------------------------------------------------
// HTML 解析辅助函数（Bing Provider 专用，避免与测试文件中的同名函数冲突）
// ---------------------------------------------------------------------------

// hasAttrClass 检查节点的 class 属性是否包含指定值
func hasAttrClass(n *html.Node, class string) bool {
	classes := strings.Fields(getAttrValue(n, "class"))
	for _, c := range classes {
		if c == class {
			return true
		}
	}
	return false
}

// getAttrValue 获取节点的指定属性值
func getAttrValue(n *html.Node, key string) string {
	for _, attr := range n.Attr {
		if attr.Key == key {
			return attr.Val
		}
	}
	return ""
}

// findFirstByTag 在节点子树中找第一个指定标签的元素
func findFirstByTag(n *html.Node, tag string) *html.Node {
	var found *html.Node
	var search func(*html.Node)
	search = func(node *html.Node) {
		if found != nil {
			return
		}
		if node.Type == html.ElementNode && node.Data == tag {
			found = node
			return
		}
		for c := node.FirstChild; c != nil; c = c.NextSibling {
			search(c)
			if found != nil {
				return
			}
		}
	}
	search(n)
	return found
}

// collectNodeText 收集节点及其所有子节点的文本内容
// 注意：html.Parse 会自动解码 HTML 实体（如 &amp; → &），所以这里直接收集 Data 即可
func collectNodeText(n *html.Node) string {
	if n.Type == html.TextNode {
		return n.Data
	}
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sb.WriteString(collectNodeText(c))
	}
	return sb.String()
}
