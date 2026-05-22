package chat

import (
	"context"
	"strings"

	"github.com/rs/zerolog/log"
	"douya/internal/search"
	"douya/internal/store"
)

// isCodeRelated returns true if the query looks like a code-related question.
func isCodeRelated(query string) bool {
queryLower := strings.ToLower(query)

	// 1. 代码语法关键词（直接出现代码特征）
	codeSyntax := []string{
		"func ", "func(", "def ", "class ", "import ", "from ",
		"package ", "var ", "let ", "const ", "struct ", "interface ",
		"return ", "if ", "for ", "while ", "switch ", "try ", "catch ",
		"public ", "private ", "protected ", "static ", "void ",
		"#{", "/*", "/**", "*/",
	}
	for _, kw := range codeSyntax {
		if strings.Contains(query, kw) {
			return true
		}
	}

	// 2. 英文自然语言代码关键词
	enKeywords := []string{
		"python", "java", "go ", "golang", "rust", "c++", "c#", "javascript", "js ", "typescript", "ts ",
		"code", "function", "debug", "error", "bug", "programming", "algorithm", "data structure",
		"api", "sdk", "library", "framework", "dependency", "package", "module",
		"class", "object", "interface", "code review", "refactor",
		"database", "sql ", "query", "optimization",
		"docker", "kubernetes", "k8s", "compile", "build", "run", "test",
		"debugging", "troubleshoot", "fix", "crash", "exception",
		"web", "framework", "comparison", "python web",
	}
	for _, kw := range enKeywords {
		if strings.Contains(queryLower, kw) {
			return true
		}
	}

	// 3. 中文自然语言代码关键词
	zhKeywords := []string{
		"代码", "函数", "调试", "错误", "编程", "程序", "算法", "数据结构",
		"api", "sdk", "库", "框架", "依赖", "包", "模块",
		"类", "对象", "接口", "代码审查", "重构",
		"数据库", "编译", "构建", "运行", "测试", "报错", "bug",
		"微服务", "架构", "设计模式", "优化", "性能", "排障",
		"写代码", "写个", "写一个",
	}
	for _, kw := range zhKeywords {
		if strings.Contains(query, kw) {
			return true
		}
	}

	// 4. 编程字符计数（括号、分号等）
	progChars := 0
	for _, r := range query {
		if r == '{' || r == '}' || r == '(' || r == ')' || r == ';' || r == '=' {
			progChars++
		}
	}
	return progChars > 5
}

// detectLanguage returns "zh" if the text appears to be Chinese, otherwise "en".
func detectLanguage(content string) string {
	zhCount := 0
	for _, r := range content {
		if r >= 0x4e00 && r <= 0x9fff || r >= 0x3400 && r <= 0x4dbf {
			zhCount++
		}
	}
	// 只要有中文字符就认为是中文（混合文本也归中文）
	if zhCount > 0 {
		return "zh"
	}
	return "en"
}

// estimateTokensByLang estimates token count for a given text and language.
func estimateTokensByLang(text string, lang string) int {
	runes := []rune(text)
	if lang == "zh" {
		return (len(runes) + 1) / 2
	}
	return (len(text) + 3) / 4
}

// EstimateTokensByLang is the exported version for testing.
func EstimateTokensByLang(text string, lang string) int { return estimateTokensByLang(text, lang) }

// estimateMessageTokens estimates the token count for a stored message.
func estimateMessageTokens(m *store.Message) int {
	if m == nil {
		return 0
	}
	total := 0
	// Content
	if m.Content != "" {
		lang := detectLanguage(m.Content)
		total += estimateTokensByLang(m.Content, lang)
	}
	// ToolCalls
	if m.ToolCalls != "" {
		lang := detectLanguage(m.ToolCalls)
		total += estimateTokensByLang(m.ToolCalls, lang)
	}
	// Images: estimate 1500 per image
	if m.Images != "" {
		imgCount := 1
		if len(m.Images) >= 2 && m.Images[0] == '[' && m.Images[len(m.Images)-1] == ']' {
			imgCount = strings.Count(m.Images, ",") + 1
		} else if strings.Contains(m.Images, ",") {
			imgCount = strings.Count(m.Images, ",") + 1
		}
		total += imgCount * 1500
	}
	// SearchResults
	if m.SearchResults != "" {
		lang := detectLanguage(m.SearchResults)
		total += estimateTokensByLang(m.SearchResults, lang)
	}
	// ThinkingContent
	if m.ThinkingContent != "" {
		lang := detectLanguage(m.ThinkingContent)
		total += estimateTokensByLang(m.ThinkingContent, lang)
	}
	// Attachments: audio=500, image=1500
	if m.Attachments != "" {
		if strings.Contains(strings.ToLower(m.Attachments), "audio") {
			total += 500
		} else {
			total += 1500
		}
	}
	if total == 0 {
		total = 1
	}
	return total
}

// searchResultInstruction returns the instruction string for search results in the given language.
func searchResultInstruction(lang string) string {
	if lang == "zh" {
		return "请根据以上搜索结果回答用户问题。如果搜索结果有帮助请优先使用，并自然融入回答中，否则使用你自己的知识回答。"
	}
	return "Please answer the user's question based on the above search results. Prefer information from the search results if helpful, otherwise use your own knowledge."
}

// DetectLanguage is the exported version for testing.
func DetectLanguage(content string) string { return detectLanguage(content) }

// SearchResultInstruction is the exported version for testing.
func SearchResultInstruction(lang string) string { return searchResultInstruction(lang) }
func (s *Service) doSearch(ctx context.Context, query string) *search.SearchResponse {
	if s.searchChain == nil {
		log.Warn().Str("query", query).Msg("[chat] searchChain is nil, cannot search")
		return nil
	}
	category := "general"
	if isCodeRelated(query) {
		category = "code"
	}
	resp := s.searchChain.SearchWithCategory(ctx, query, category)
	if resp == nil {
		log.Warn().Str("query", query).Msg("[chat] search returned nil")
	}
	return resp
}
