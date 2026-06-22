package chat

import (
	"context"
	"database/sql"
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
// 采用偏保守的估算系数，确保预防性裁剪在溢出前生效：
// - 中文：BPE tokenizer 约 1.5-2 token/字，取 2 偏保守
// - 英文：BPE tokenizer 约 0.75 token/byte，取 1/3 偏保守
func estimateTokensByLang(text string, lang string) int {
	runes := []rune(text)
	if lang == "zh" {
		return len(runes)*2 + 1
	}
	return (len(text) + 2) / 3
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

// EstimateAttachmentTokensWithData 根据附件类型和实际数据内容估算 token 数。
// 对于 text/PDF 附件，基于内容长度估算，避免返回 0 导致上下文溢出防御失效。
func EstimateAttachmentTokensWithData(attType, data string) int {
	switch strings.ToLower(attType) {
	case "image":
		return 3500
	case "video":
		return 5000
	case "audio":
		return 500
	case "text":
		// att.Data 为原始文本内容，直接按语言估算
		if data == "" {
			return 0
		}
		lang := detectLanguage(data)
		return estimateTokensByLang(data, lang) + 20 // +20 for attachment wrapper
	case "pdf":
		// att.Data 为 base64 编码的 PDF 二进制
		if data == "" {
			return 0
		}
		// base64 解码后字节数 ≈ len(data) * 3/4
		// PDF 提取的文本通常为二进制大小的 10-30%，取 25% 偏保守
		decodedBytes := len(data) * 3 / 4
		estimatedTextChars := decodedBytes / 4
		// 按混合语言估算（中文 2 token/字，英文 1/3 token/byte）
		return estimatedTextChars*3/2 + 20 // +20 for attachment wrapper
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
		Data string `json:"data"`
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
		total += EstimateAttachmentTokensWithData(att.Type, att.Data)
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
	// chat template 开销
	total += 10
	if total < 11 {
		total = 11
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

// enhanceErrorWithHint 为运行时错误添加设置调整建议
// 如果错误可以通过设置界面调整解决，追加提示；否则直接说明原因
func enhanceErrorWithHint(errMsg string) string {
	lower := strings.ToLower(errMsg)

	// 上下文溢出相关
	if strings.Contains(lower, "exceed") && (strings.Contains(lower, "context") || strings.Contains(lower, "ctx")) {
		return errMsg + "\n💡 可尝试：设置 → 增大上下文长度，或缩短对话/新建对话"
	}
	if strings.Contains(lower, "context length") || strings.Contains(lower, "context_size") {
		return errMsg + "\n💡 可尝试：设置 → 增大上下文长度，或缩短对话/新建对话"
	}

	// 模型加载/内存不足相关
	if strings.Contains(lower, "out of memory") || strings.Contains(lower, "oom") || strings.Contains(lower, "cuda") && strings.Contains(lower, "alloc") {
		return errMsg + "\n💡 可尝试：设置 → 减少 GPU 层数，或开启 Flash Attention，或使用更小的模型"
	}
	if strings.Contains(lower, "not enough memory") || strings.Contains(lower, "memory allocation") {
		return errMsg + "\n💡 可尝试：设置 → 减少 GPU 层数，或使用更小的模型"
	}
	if strings.Contains(lower, "mmproj") && (strings.Contains(lower, "failed") || strings.Contains(lower, "error") || strings.Contains(lower, "load")) {
		return errMsg + "\n💡 可尝试：设置 → 关闭「视觉投影卸载到 GPU」，或检查视觉模型文件是否完整"
	}

	// 连接/服务相关
	if strings.Contains(lower, "connection refused") || strings.Contains(lower, "connect: connection refused") {
		return "AI 服务未启动或已停止，请等待服务启动完成"
	}
	if strings.Contains(lower, "connection reset") || strings.Contains(lower, "broken pipe") {
		return "与服务器的连接中断，模型可能正在切换或服务已重启"
	}

	// 模型/参数不兼容
	if strings.Contains(lower, "flash_attn") && strings.Contains(lower, "not supported") {
		return errMsg + "\n💡 可尝试：设置 → 关闭 Flash Attention"
	}
	if strings.Contains(lower, "cache_type") && (strings.Contains(lower, "not supported") || strings.Contains(lower, "unknown")) {
		return errMsg + "\n💡 可尝试：设置 → 将 KV 缓存类型改为默认值（q8_0）"
	}
	if strings.Contains(lower, "grammar") && strings.Contains(lower, "reasoning") {
		return errMsg + "\n💡 可尝试：设置 → 关闭推理模式，或移除语法约束"
	}
	if strings.Contains(lower, "backend sampling") && strings.Contains(lower, "not compatible") {
		return errMsg + "\n💡 可尝试：设置 → 关闭「后端采样」"
	}
	if strings.Contains(lower, "speculative") || strings.Contains(lower, "draft") && strings.Contains(lower, "failed") {
		return errMsg + "\n💡 可尝试：设置 → 关闭推测解码（MTP），或检查模型是否支持"
	}

	// 请求格式相关
	if strings.Contains(lower, "invalid type") && strings.Contains(lower, "enable_thinking") {
		return errMsg + "\n💡 模型不支持 enable_thinking 参数，可尝试：设置 → 将推理模式设为「关闭」"
	}
	if strings.Contains(lower, "tool_call") && strings.Contains(lower, "not supported") {
		return errMsg + "\n💡 当前模型不支持工具调用，联网搜索将使用预搜索模式"
	}

	// 搜索 API 相关
	if strings.Contains(lower, "tavily") && (strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "api key")) {
		return "Tavily 搜索 API Key 无效或已过期\n💡 可尝试：设置 → 重新填写 Tavily API Key"
	}
	if strings.Contains(lower, "bing") && (strings.Contains(lower, "401") || strings.Contains(lower, "403")) {
		return "Bing 搜索 API 认证失败\n💡 可尝试：设置 → 检查 Bing API Key"
	}

	return errMsg
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
	// chat template 开销：每条消息的 <|im_start|>role\n...<|im_end|> 约占 3-10 tokens
	total += 10
	if total < 11 {
		total = 11
	}
	return total
}

// estimateMessagesTokens 估算消息列表的总 token 数
func estimateMessagesTokens(messages []llm.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += estimateChatMessageTokens(msg)
	}
	return total
}

// cleanToolCallPairs 保护 tool call 配对，清理孤立的 tool/assistant(tool_calls) 消息。
// 从 TrimMessagesToFit 中提取的独立清理逻辑，供 CompressContext 复用：
//  1. 如果裁剪后第一条消息是孤立的 tool 消息（对应的 assistant 被裁剪掉了），
//     继续删除直到遇到非 tool 消息，避免 API 报错
//  2. 移除孤立的 assistant 消息（带有 tool_calls 但后面没有对应的 tool 消息），
//     这种消息会导致 API 报错，需要清理
func cleanToolCallPairs(messages []llm.ChatMessage) []llm.ChatMessage {
	// 1. 删除开头的孤立 tool 消息
	for len(messages) > 0 && messages[0].Role == "tool" {
		messages = messages[1:]
	}

	// 2. 移除孤立的 assistant 消息（带 tool_calls 但后面没有对应的 tool 消息）
	for i := 0; i < len(messages); i++ {
		if messages[i].Role == "assistant" && len(messages[i].ToolCalls) > 0 {
			hasFollowingTool := false
			for j := i + 1; j < len(messages); j++ {
				if messages[j].Role == "tool" {
					hasFollowingTool = true
					break
				}
				// 遇到 user 或新的 assistant 消息，说明没有 tool 消息跟随
				if messages[j].Role == "user" || messages[j].Role == "assistant" {
					break
				}
			}
			if !hasFollowingTool {
				messages = append(messages[:i], messages[i+1:]...)
				i-- // 重新检查当前位置
			}
		}
	}

	return messages
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

	// 保护 tool call 配对（提取为独立函数 cleanToolCallPairs，供 CompressContext 复用）
	kept = cleanToolCallPairs(kept)

	// 确保以 user 消息开头（Jinja 模板要求）
	for len(kept) > 0 && kept[0].Role != "user" {
		kept = kept[1:]
	}

	var result []llm.ChatMessage
	if systemMsg != nil {
		result = append(result, *systemMsg)
	}
	result = append(result, kept...)
	result = append(result, lastMsg)

	// 确保结果中至少包含一条 user 消息（Jinja 模板要求）
	hasUser := false
	for _, msg := range result {
		if msg.Role == "user" {
			hasUser = true
			break
		}
	}
	if !hasUser {
		// 从原始消息中找到最后一条 user 消息，插入到 lastMsg 之前
		var lastUserMsg *llm.ChatMessage
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "user" {
				lastUserMsg = &messages[i]
				break
			}
		}
		if lastUserMsg != nil {
			result = append(result[:len(result)-1], *lastUserMsg, result[len(result)-1])
		}
	}

	return result
}

// CalcSlidingWindowSize 根据上下文大小动态计算滑动窗口消息数
func CalcSlidingWindowSize(contextSize int) int {
	if contextSize <= 0 {
		contextSize = 4096
	}
	switch {
	case contextSize <= 8192:
		return 6
	case contextSize < 32768:
		return 12
	default:
		return 20
	}
}

// CompressContextResult 是 CompressContext 的返回结果
type CompressContextResult struct {
	Messages        []llm.ChatMessage // 压缩后的消息列表
	TrimmedCount    int               // 被裁剪的消息数
	SummaryInserted bool              // 是否插入了摘要
}

// CompressContext 统一上下文压缩函数：滑动窗口裁剪 + 异步摘要
// 参数：
//   - messages: 原始消息列表（第一条可能是 system 消息）
//   - maxTokens: 上下文大小
//   - contextSize: 用于计算滑动窗口大小
//   - existingSummary: 已有的旧摘要（可为空）
//   - trimmedStoreMsgs: 被裁剪的原始 store.Message 列表（用于摘要生成）
//   - llmClient: LLM 客户端（用于异步摘要生成）
//   - convID: 对话ID（用于保存摘要到数据库）
//   - db: 数据库连接（用于保存摘要）
//
// 返回：压缩后的消息列表和裁剪信息
// 摘要生成是异步的，此函数立即返回
func CompressContext(
	messages []llm.ChatMessage,
	contextSize int,
	existingSummary string,
	trimmedStoreMsgs []*store.Message,
	llmClient *llm.Client,
	convID string,
	db *sql.DB,
) CompressContextResult {
	// 1. 分离 system 消息
	var systemMsg *llm.ChatMessage
	rest := messages
	if len(rest) > 0 && rest[0].Role == "system" {
		systemMsg = &rest[0]
		rest = rest[1:]
	}

	// 2. 计算滑动窗口大小
	windowSize := CalcSlidingWindowSize(contextSize)
	if windowSize > len(rest) {
		windowSize = len(rest)
	}

	// 3. 保留窗口内的最近消息
	windowStart := len(rest) - windowSize
	kept := rest[windowStart:]
	trimmed := rest[:windowStart] // 被裁剪的消息（ChatMessage 格式）

	// 4. 构建结果消息列表
	var result []llm.ChatMessage
	if systemMsg != nil {
		result = append(result, *systemMsg)
	}

	// 5. 如果有已有摘要，立即插入
	if existingSummary != "" {
		result = append(result, llm.ChatMessage{
			Role:    "system",
			Content: "[对话摘要] 以下是之前对话的摘要：" + existingSummary,
		})
	}

	// 6. 添加窗口内的消息
	result = append(result, kept...)

	// 7. 保护 tool call 配对（复用 TrimMessagesToFit 中的清理逻辑）
	result = cleanToolCallPairs(result)

	// 8. 确保以 user 消息开头（Jinja 模板要求）
	for len(result) > 0 && result[0].Role != "system" && result[0].Role != "user" {
		result = result[1:]
	}

	// 9. 异步生成摘要
	if len(trimmedStoreMsgs) >= 4 && llmClient != nil && convID != "" && db != nil {
		go func() {
			newSummary := summarizeMessages(llmClient, existingSummary, trimmedStoreMsgs)
			if newSummary != "" {
				if err := store.UpdateConversationSummary(db, convID, newSummary); err != nil {
					log.Warn().Err(err).Msg("[compress] 保存摘要失败")
				} else {
					log.Debug().Str("conv_id", convID).Msg("[compress] 摘要已异步保存")
				}
			}
		}()
	}

	return CompressContextResult{
		Messages:        result,
		TrimmedCount:    len(trimmed),
		SummaryInserted: existingSummary != "",
	}
}

func searchResultInstruction(lang string) string {
	if lang == "zh" {
		return "\n请仅基于以上信息回答用户的问题，无法确认时明确说明，不要使用[1][2]等编号引用格式。"
	}
	return "\nAnswer the user's question based strictly on the above information. If you cannot confirm, state it clearly. Do not use numbered citation formats like [1][2]."
}

// DetectLanguage is the exported version for testing.
func DetectLanguage(content string) string { return detectLanguage(content) }

// SearchResultInstruction is the exported version for testing.
func SearchResultInstruction(lang string) string { return searchResultInstruction(lang) }
func (s *Service) doSearch(ctx context.Context, query string) *search.SearchResponse {
	// 在锁保护下获取搜索链快照，避免数据竞争
	chain := s.getSearchChainSnapshot()
	if chain == nil {
		log.Warn().Str("query", query).Msg("[chat] searchChain is nil, cannot search")
		return nil
	}
	category := "general"
	if isCodeRelated(query) {
		category = "code"
	}
	resp := chain.SearchWithCategory(ctx, query, category, 10)
	if resp == nil {
		log.Warn().Str("query", query).Msg("[chat] search returned nil")
	}
	return resp
}
