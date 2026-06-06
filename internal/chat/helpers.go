package chat

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"douya/internal/llm"
	"douya/internal/search"
	"douya/internal/store"

	"github.com/rs/zerolog/log"
)

// 预编译正则表达式，避免每次调用时重复编译
var (
	rePromptTokens  = regexp.MustCompile(`n_prompt_tokens["\s:=]+(\d+)`)
	reNCtx          = regexp.MustCompile(`n_ctx["\s:=]+(\d+)`)
	reRequestTokens = regexp.MustCompile(`request \((\d+) tokens\)`)
	reAvailCtxSize  = regexp.MustCompile(`available context size \((\d+) tokens\)`)
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
		"写代码", "项目仓库",
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

func EstimateTokensByLang(text string, lang string) int { return estimateTokensByLang(text, lang) }

func EstimateAttachmentTokens(attType string) int {
	switch strings.ToLower(attType) {
	case "image":
		return 3500
	case "video":
		return 5000
	case "audio":
		return 500
	default:
		return 0
	}
}

func estimateAttachmentTokensFromJSON(attachmentsJSON string) int {
	if attachmentsJSON == "" {
		return 0
	}
	var atts []struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal([]byte(attachmentsJSON), &atts); err != nil {
		if strings.Contains(strings.ToLower(attachmentsJSON), "video") {
			return 5000
		}
		if strings.Contains(strings.ToLower(attachmentsJSON), "audio") {
			return 500
		}
		if strings.Contains(strings.ToLower(attachmentsJSON), "image") {
			return 3500
		}
		return 1500
	}
	total := 0
	for _, att := range atts {
		total += EstimateAttachmentTokens(att.Type)
	}
	return total
}

func estimateMessageTokens(m *store.Message) int {
	if m == nil {
		return 0
	}
	total := 0
	// 对同一消息只检测一次语言，各字段复用
	lang := detectLanguage(m.Content)
	if m.Content != "" {
		total += estimateTokensByLang(m.Content, lang)
	}
	if m.ToolCalls != "" {
		total += estimateTokensByLang(m.ToolCalls, lang)
	}
	if m.Images != "" {
		imgCount := 1
		if len(m.Images) >= 2 && m.Images[0] == '[' && m.Images[len(m.Images)-1] == ']' {
			imgCount = strings.Count(m.Images, ",") + 1
		} else if strings.Contains(m.Images, ",") {
			imgCount = strings.Count(m.Images, ",") + 1
		}
		total += imgCount * 3500
	}
	if m.SearchResults != "" {
		total += estimateTokensByLang(m.SearchResults, lang)
	}
	if m.ThinkingContent != "" {
		total += estimateTokensByLang(m.ThinkingContent, lang)
	}
	if m.Attachments != "" {
		total += estimateAttachmentTokensFromJSON(m.Attachments)
	}
	if total == 0 {
		total = 1
	}
	return total
}

type ExceedContextInfo struct {
	PromptTokens int
	ContextSize  int
	Exceeded     bool
}

func ParseExceedContextError(err error) *ExceedContextInfo {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if !strings.Contains(msg, "exceed_context_size_error") && !strings.Contains(msg, "exceeds available context size") && !strings.Contains(msg, "context size exceeded") {
		return nil
	}
	info := &ExceedContextInfo{Exceeded: true}
	if m := rePromptTokens.FindStringSubmatch(msg); len(m) > 1 {
		if v, e := strconv.Atoi(m[1]); e == nil {
			info.PromptTokens = v
		}
	}
	if m := reNCtx.FindStringSubmatch(msg); len(m) > 1 {
		if v, e := strconv.Atoi(m[1]); e == nil {
			info.ContextSize = v
		}
	}
	if info.PromptTokens == 0 {
		if m := reRequestTokens.FindStringSubmatch(msg); len(m) > 1 {
			if v, e := strconv.Atoi(m[1]); e == nil {
				info.PromptTokens = v
			}
		}
	}
	if info.ContextSize == 0 {
		if m := reAvailCtxSize.FindStringSubmatch(msg); len(m) > 1 {
			if v, e := strconv.Atoi(m[1]); e == nil {
				info.ContextSize = v
			}
		}
	}
	return info
}

func estimateChatMessageTokens(msg llm.ChatMessage) int {
	total := 0
	contentStr := msg.ContentString()
	if contentStr != "" {
		lang := detectLanguage(contentStr)
		total += estimateTokensByLang(contentStr, lang)
	}
	switch v := msg.Content.(type) {
	case []llm.ContentPart:
		for _, part := range v {
			if part.Type == "image_url" {
				total += 3500
			}
			if part.Type == "input_audio" {
				total += 500
			}
		}
	case []interface{}:
		for _, item := range v {
			if part, ok := item.(map[string]interface{}); ok {
				if part["type"] == "image_url" {
					total += 3500
				}
				if part["type"] == "input_audio" {
					total += 500
				}
			}
		}
	}
	for _, tc := range msg.ToolCalls {
		b, _ := json.Marshal(tc)
		lang := detectLanguage(string(b))
		total += estimateTokensByLang(string(b), lang)
	}
	if total == 0 {
		total = 1
	}
	return total
}

func TrimMessagesToFit(messages []llm.ChatMessage, maxTokens int, reserve int) []llm.ChatMessage {
	if len(messages) <= 2 {
		return messages
	}
	effectiveMax := maxTokens - reserve
	if effectiveMax < 100 {
		effectiveMax = 100
	}

	total := 0
	for _, msg := range messages {
		total += estimateChatMessageTokens(msg)
	}
	if total <= effectiveMax {
		return messages
	}

	var systemMsg *llm.ChatMessage
	startIdx := 0
	if len(messages) > 0 && messages[0].Role == "system" {
		systemMsg = &messages[0]
		startIdx = 1
	}

	rest := messages[startIdx:]
	if len(rest) == 0 {
		return messages
	}

	lastMsg := rest[len(rest)-1]
	rest = rest[:len(rest)-1]

	systemTokens := 0
	if systemMsg != nil {
		systemTokens = estimateChatMessageTokens(*systemMsg)
	}
	lastTokens := estimateChatMessageTokens(lastMsg)

	remaining := effectiveMax - systemTokens - lastTokens
	if remaining < 0 {
		if systemMsg != nil {
			return []llm.ChatMessage{*systemMsg, lastMsg}
		}
		return []llm.ChatMessage{lastMsg}
	}

	var kept []llm.ChatMessage
	acc := 0
	for i := len(rest) - 1; i >= 0; i-- {
		t := estimateChatMessageTokens(rest[i])
		if acc+t > remaining {
			break
		}
		acc += t
		kept = append([]llm.ChatMessage{rest[i]}, kept...)
	}

	for len(kept) > 0 && kept[0].Role != "user" {
		kept = kept[1:]
	}

	var result []llm.ChatMessage
	if systemMsg != nil {
		result = append(result, *systemMsg)
	}
	result = append(result, kept...)
	result = append(result, lastMsg)
	return result
}

func searchResultInstruction(lang string) string {
	if lang == "zh" {
		return "以下是为回答用户问题提供的补充参考信息。请先消化吸收这些内容，然后像运用自身知识一样自然地融入回答。不要提及或暗示存在'搜索'或'补充信息'这一过程。"
	}
	return "Below is supplementary reference information for answering the user's question. Absorb and internalize this content, then naturally integrate it into your response as if it were your own knowledge. Do not mention or allude to the existence of a 'search' or 'supplementary information' process."
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
