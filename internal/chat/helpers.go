package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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

// 附件 token 估算值。图片用变量（可通过 SetImageTokenEstimate 根据 --image-max-tokens 动态调整），
// 视频/音频保持常量（llama-server 无对应参数，估算值固定）。
// 生活类比：像尺子上的刻度，统一标定后所有测量都用同一标准。图片刻度可按需调整。
const (
	videoTokenEstimate = 5000 // 视频附件 token 估算值
	audioTokenEstimate = 500  // 音频附件 token 估算值

	// Q2: 消息裁剪相关的魔法数字抽为常量，提升可读性
	minEffectiveTokens    = 100  // TrimMessagesToFit 的最小有效 token 数，防止 reserve 过大导致负数
	maxImportantBudget    = 1000 // 高分历史消息预算上限
	minImportantBudget    = 200  // 高分历史消息预算下限
	importantBudgetRatio  = 5    // 高分历史消息预算 = contextSize / importantBudgetRatio
)

// imageTokenEstimate 图片附件 token 估算值，默认 3500（覆盖多数模型默认值）。
// 通过 SetImageTokenEstimate 在模型加载后根据 --image-max-tokens 动态调整，
// 让估算值与 llama-server 实际行为一致，避免 MaxTokens 计算偏差。
// 并发安全：写入在模型切换时（单线程），读取在请求构建时（单线程），无竞争。
var imageTokenEstimate = 3500

// defaultImageTokenEstimate 是未设置 --image-max-tokens 时的保守估算值
const defaultImageTokenEstimate = 3500

// SetImageTokenEstimate 根据用户配置的 --image-max-tokens 更新图片 token 估算值。
// imageMaxTokens <= 0 时恢复默认值（llama-server 默认行为，token 数取决于图片分辨率）。
// 在模型加载后调用，确保估算值与 llama-server 实际行为一致。
func SetImageTokenEstimate(imageMaxTokens int) {
	if imageMaxTokens > 0 {
		imageTokenEstimate = imageMaxTokens
	} else {
		imageTokenEstimate = defaultImageTokenEstimate
	}
}

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
		return imageTokenEstimate
	case "video":
		return videoTokenEstimate
	case "audio":
		return audioTokenEstimate
	default:
		return 0
	}
}

// EstimateAttachmentTokensWithData 根据附件类型和实际数据内容估算 token 数。
// 对于 text/PDF 附件，基于内容长度估算，避免返回 0 导致上下文溢出防御失效。
// 未知类型且有数据时返回保守默认值（1500），防止上下文溢出防御失效。
func EstimateAttachmentTokensWithData(attType, data string) int {
	switch strings.ToLower(attType) {
	case "image":
		return imageTokenEstimate
	case "video":
		return videoTokenEstimate
	case "audio":
		return audioTokenEstimate
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
		// 未知类型且有数据：返回保守默认值，防止上下文溢出
		// 生活类比：安检遇到不认识的包裹，宁可多算重量也不能当作没重量
		if data == "" {
			return 0
		}
		return 1500
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
			return videoTokenEstimate
		}
		if strings.Contains(strings.ToLower(attachmentsJSON), "audio") {
			return audioTokenEstimate
		}
		if strings.Contains(strings.ToLower(attachmentsJSON), "image") {
			return imageTokenEstimate
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
		total += imgCount * imageTokenEstimate
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

// errorHintRule 定义错误提示匹配规则。
// 生活类比：像快递分拣规则表，每条规则说明"包裹上有哪些关键词就归入哪一类"。
type errorHintRule struct {
	allKeywords []string // 必须全部包含的关键词（AND）
	anyKeywords []string // 至少包含其一的关键词（与 allKeywords 为 AND 关系），为空表示不检查
	errCode     string   // 错误码，空字符串表示不带错误码前缀
	hint        string   // 设置建议（含 💡），为空表示仅返回 errMsg
	replaceMsg  string   // 非空时直接返回此消息（优先级最高），用于完全替换场景
}

// errorHintRules 错误提示匹配规则表（B-3.4 表驱动化）
// 按顺序匹配，第一个命中的规则生效。复杂 OR 条件已拆为多条规则（返回相同结果）。
var errorHintRules = []errorHintRule{
	// 上下文溢出
	{allKeywords: []string{"exceed"}, anyKeywords: []string{"context", "ctx"}, errCode: ErrCodeContextOverflow, hint: "💡 可尝试：设置 → 增大上下文长度，或缩短对话/新建对话"},
	{anyKeywords: []string{"context length", "context_size"}, errCode: ErrCodeContextOverflow, hint: "💡 可尝试：设置 → 增大上下文长度，或缩短对话/新建对话"},

	// OOM（显存/内存不足）
	{anyKeywords: []string{"out of memory", "oom"}, errCode: ErrCodeOOM, hint: "💡 可尝试：设置 → 减少 GPU 层数，或开启 Flash Attention，或使用更小的模型"},
	{allKeywords: []string{"cuda", "alloc"}, errCode: ErrCodeOOM, hint: "💡 可尝试：设置 → 减少 GPU 层数，或开启 Flash Attention，或使用更小的模型"},
	{anyKeywords: []string{"not enough memory", "memory allocation"}, errCode: ErrCodeOOM, hint: "💡 可尝试：设置 → 减少 GPU 层数，或使用更小的模型"},

	// mmproj 加载失败（无错误码，保持原行为）
	{allKeywords: []string{"mmproj"}, anyKeywords: []string{"failed", "error", "load"}, hint: "💡 可尝试：设置 → 关闭「视觉投影卸载到 GPU」，或检查视觉模型文件是否完整"},

	// DLL 缺失
	{allKeywords: []string{"dll"}, anyKeywords: []string{"not found", "could not be found", "缺失"}, errCode: ErrCodeDLLMissing, hint: "💡 可尝试：检查 runtime/ 目录是否包含所有必要的 DLL 文件"},

	// 引擎程序缺失
	{allKeywords: []string{"llama-server"}, anyKeywords: []string{"not found", "不存在", "could not find"}, errCode: ErrCodeEngineMissing, hint: "💡 可尝试：检查 runtime/ 目录下是否存在 llama-server.exe"},
	{allKeywords: []string{"引擎程序"}, anyKeywords: []string{"not found", "不存在", "could not find"}, errCode: ErrCodeEngineMissing, hint: "💡 可尝试：检查 runtime/ 目录下是否存在 llama-server.exe"},

	// 模型文件缺失（原复杂 OR 条件拆为 4 条规则，返回相同结果）
	{allKeywords: []string{"no models found", "gguf"}, errCode: ErrCodeModelMissing, hint: "💡 可尝试：将 GGUF 模型文件放入 models/ 目录"},
	{allKeywords: []string{"未找到任何", "gguf"}, errCode: ErrCodeModelMissing, hint: "💡 可尝试：将 GGUF 模型文件放入 models/ 目录"},
	{allKeywords: []string{"model", "not found", "gguf"}, errCode: ErrCodeModelMissing, hint: "💡 可尝试：将 GGUF 模型文件放入 models/ 目录"},
	{allKeywords: []string{"模型文件", "未找到"}, errCode: ErrCodeModelMissing, hint: "💡 可尝试：将 GGUF 模型文件放入 models/ 目录"},

	// 永久失败（无 hint，仅加错误码前缀）
	{allKeywords: []string{"permanent", "failure"}, errCode: ErrCodePermanentFailure},

	// 超时
	{anyKeywords: []string{"timeout", "timed out"}, errCode: ErrCodeTimeout, hint: "💡 可尝试：检查网络连接或稍后重试"},

	// 连接相关（完全替换为用户友好消息）
	{anyKeywords: []string{"connection refused"}, replaceMsg: "AI 服务未启动或已停止，请等待服务启动完成"},
	{anyKeywords: []string{"connection reset", "broken pipe"}, replaceMsg: "与服务器的连接中断，模型可能正在切换或服务已重启"},

	// 模型/参数不兼容
	{allKeywords: []string{"flash_attn", "not supported"}, hint: "💡 可尝试：设置 → 关闭 Flash Attention"},
	{allKeywords: []string{"cache_type"}, anyKeywords: []string{"not supported", "unknown"}, hint: "💡 可尝试：设置 → 将 KV 缓存类型改为默认值（q8_0）"},
	{allKeywords: []string{"grammar", "reasoning"}, hint: "💡 可尝试：设置 → 关闭推理模式，或移除语法约束"},
	{allKeywords: []string{"backend sampling", "not compatible"}, hint: "💡 可尝试：设置 → 关闭「后端采样」"},
	{anyKeywords: []string{"speculative"}, hint: "💡 可尝试：设置 → 关闭推测解码（MTP），或检查模型是否支持"},
	{allKeywords: []string{"draft", "failed"}, hint: "💡 可尝试：设置 → 关闭推测解码（MTP），或检查模型是否支持"},

	// 请求格式相关
	{allKeywords: []string{"invalid type", "enable_thinking"}, hint: "💡 模型不支持 enable_thinking 参数，可尝试：设置 → 将推理模式设为「关闭」"},
	{allKeywords: []string{"tool_call", "not supported"}, hint: "💡 当前模型不支持工具调用，联网搜索将使用预搜索模式"},

	// 搜索 API 认证失败
	{allKeywords: []string{"tavily"}, anyKeywords: []string{"401", "unauthorized", "api key"}, replaceMsg: "Tavily 搜索 API Key 无效或已过期\n💡 可尝试：设置 → 重新填写 Tavily API Key"},
	{allKeywords: []string{"bing"}, anyKeywords: []string{"401", "403"}, replaceMsg: "Bing 搜索 API 认证失败\n💡 可尝试：设置 → 检查 Bing API Key"},
}

// matchErrorHintRule 检查小写错误消息是否匹配规则（allKeywords 全包含 且 anyKeywords 至少包含其一）
func matchErrorHintRule(lower string, rule errorHintRule) bool {
	for _, kw := range rule.allKeywords {
		if !strings.Contains(lower, kw) {
			return false
		}
	}
	if len(rule.anyKeywords) > 0 {
		matched := false
		for _, kw := range rule.anyKeywords {
			if strings.Contains(lower, kw) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

// applyErrorHintRule 根据规则构造返回消息
func applyErrorHintRule(errMsg string, rule errorHintRule) string {
	if rule.replaceMsg != "" {
		return rule.replaceMsg
	}
	if rule.errCode != "" {
		if rule.hint != "" {
			return formatErrCode(rule.errCode, errMsg+"\n"+rule.hint)
		}
		return formatErrCode(rule.errCode, errMsg)
	}
	if rule.hint != "" {
		return errMsg + "\n" + rule.hint
	}
	return errMsg
}

// enhanceErrorWithHint 为运行时错误添加设置调整建议
// 如果错误可以通过设置界面调整解决，追加提示；否则直接说明原因
//
// 对于能映射到统一错误码（见 errorcodes.go）的错误，会在提示信息前加上
// "[ERR_CODE]" 前缀，便于前端 classifyError 精确匹配分类；其他错误保持
// 原有字符串匹配逻辑，向后兼容。
func enhanceErrorWithHint(errMsg string) string {
	lower := strings.ToLower(errMsg)
	for _, rule := range errorHintRules {
		if matchErrorHintRule(lower, rule) {
			return applyErrorHintRule(errMsg, rule)
		}
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
				total += imageTokenEstimate
			}
			if part.Type == "input_audio" {
				total += audioTokenEstimate
			}
		}
	case []any:
		for _, item := range v {
			if part, ok := item.(map[string]any); ok {
				if part["type"] == "image_url" {
					total += imageTokenEstimate
				}
				if part["type"] == "input_audio" {
					total += audioTokenEstimate
				}
			}
		}
	}
	for _, tc := range msg.ToolCalls {
		// PERF-3: 用字段拼接模拟 JSON Marshal 输出，避免反射+序列化开销
		// 生活类比：以前为了知道一个快递有多重，把整箱打包好再称；现在按清单格式直接拼出"等效重量"。
		// 保留 JSON 结构字符（如 {"index":0,"id":"...","function":{...}}）以保持与 json.Marshal 近似的结果
		text := `{"index":0,"id":"` + tc.ID + `","type":"` + tc.Type + `","function":{"name":"` + tc.Function.Name + `","arguments":"` + tc.Function.Arguments + `"}}`
		lang := detectLanguage(text)
		total += estimateTokensByLang(text, lang)
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
// ensureStartsWithUserOrSystem 跳过开头的非 system/user 消息，确保消息列表以 system 或 user 开头。
// Jinja 模板要求消息列表不能以 assistant/tool 开头，否则会触发模板渲染错误。
// C-10 修复：提取 TrimMessagesToFit 和 CompressContext 中重复的"确保以 user 开头"逻辑
// 生活类比：快递装车时，第一件必须是发件单（system）或客户委托（user），不能是快递员备注（assistant/tool）
func ensureStartsWithUserOrSystem(messages []llm.ChatMessage) []llm.ChatMessage {
	for len(messages) > 0 && messages[0].Role != "system" && messages[0].Role != "user" {
		messages = messages[1:]
	}
	return messages
}

// computeMustKeepBudget 计算必保消息的索引和总 token 数。
// 必保消息（评分>=5，如含代码块或用户明确指令）强制保留，即使超预算（丢失重要决策代价更大）。
// C-12 修复：提取 TrimMessagesToFit 和 selectImportantMessages 中重复的必保预算计算逻辑
// 生活类比：装车时先标记必须发的重要件并计算总重量，剩余重量再分配给普通件
// 返回：mustKeepIndices（必保消息在原切片中的索引，升序），mustKeepTokens（必保消息总 token 数）
func computeMustKeepBudget(msgs []llm.ChatMessage) (mustKeepIndices []int, mustKeepTokens int) {
	scores := ScoreChatMessages(msgs)
	for i := range msgs {
		if IsMustKeep(scores[i]) {
			mustKeepIndices = append(mustKeepIndices, i)
			mustKeepTokens += estimateChatMessageTokens(msgs[i])
		}
	}
	return mustKeepIndices, mustKeepTokens
}

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
	effectiveMax := max(maxTokens-reserve, minEffectiveTokens)

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

	// P1-B1: 引入消息重要性评分，避免机械裁剪丢失关键决策/代码。
	// 策略：必保消息（评分>=5，如含代码块或用户明确指令）强制保留；
	//       非必保消息按原"从后向前 break"逻辑填充至剩余预算。
	// C-12 修复：复用 computeMustKeepBudget 计算必保消息，消除重复逻辑
	mustKeepIndices, mustKeepTokens := computeMustKeepBudget(rest)
	keepDecision := make([]bool, len(rest))
	for _, idx := range mustKeepIndices {
		keepDecision[idx] = true
	}
	// 非必保消息的预算 = 总剩余预算 - 必保消息已占用
	// 注意：必保消息即使超预算也保留（丢失重要决策的代价 >> 超预算的代价）
	nonMustKeepBudget := remaining - mustKeepTokens
	acc := 0
	for i := len(rest) - 1; i >= 0; i-- {
		if keepDecision[i] {
			continue
		}
		t := estimateChatMessageTokens(rest[i])
		if acc+t > nonMustKeepBudget {
			break
		}
		keepDecision[i] = true
		acc += t
	}
	// 按原顺序构造 kept，保持消息时序合法性
	var kept []llm.ChatMessage
	for i, msg := range rest {
		if keepDecision[i] {
			kept = append(kept, msg)
		}
	}

	// 保护 tool call 配对（提取为独立函数 cleanToolCallPairs，供 CompressContext 复用）
	kept = cleanToolCallPairs(kept)

	// 确保以 user 消息开头（Jinja 模板要求）
	// C-10 修复：复用 ensureStartsWithUserOrSystem，消除重复逻辑
	kept = ensureStartsWithUserOrSystem(kept)

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

// selectImportantMessages P3-B2: 从被裁剪消息中按评分挑选高价值消息填充预算。
//
// 策略（与 TrimMessagesToFit 的"必保+按分填充"逻辑一致，但用于已裁剪列表的回收）：
//  1. 计算每条消息评分（复用 ScoreChatMessage）
//  2. 必保消息（评分>=5）强制保留，即使超预算也保留
//  3. 非必保消息按分数降序+原索引升序，累加 token 填充预算
//  4. 选中的消息按原索引升序返回，保持时序合法性
//
// 生活类比：像整理旧书架，先把"必藏经典"（高评分）全部留下，
// 再用剩余空间按"推荐指数"挑几本"值得一读"的书，最后按出版年份排好。
//
// 参数：
//   - msgs: 被裁剪的消息列表（按时间顺序，slice 位置即时间顺序）
//   - budget: token 预算（必保消息可能超预算，调用方应预留余量）
//
// 返回：选中的消息列表（按原时间顺序）
func selectImportantMessages(msgs []llm.ChatMessage, budget int) []llm.ChatMessage {
	if len(msgs) == 0 || budget <= 0 {
		return nil
	}

	// 1. 计算每条消息评分和 token
	// C-12 修复：复用 computeMustKeepBudget 计算必保消息，消除重复逻辑
	type msgMeta struct {
		index    int
		score    int
		tokens   int
		mustKeep bool
	}
	mustKeepIndices, mustKeepTokens := computeMustKeepBudget(msgs)
	metas := make([]msgMeta, len(msgs))
	for i := range msgs {
		score := ScoreChatMessage(msgs[i])
		tokens := estimateChatMessageTokens(msgs[i])
		metas[i] = msgMeta{
			index:    i,
			score:    score,
			tokens:   tokens,
			mustKeep: IsMustKeep(score),
		}
	}

	// 2. 必保消息全部选中（即使超预算，丢失重要决策代价更大）
	selected := make([]bool, len(msgs))
	for _, idx := range mustKeepIndices {
		selected[idx] = true
	}

	// 3. 非必保消息按"分数降序+索引升序"排序后贪心填充剩余预算
	remainingBudget := budget - mustKeepTokens
	if remainingBudget > 0 {
		// 复制一份 metas 用于排序（不破坏原顺序）
		sortedMetas := make([]msgMeta, 0, len(metas))
		for i := range metas {
			if !metas[i].mustKeep {
				sortedMetas = append(sortedMetas, metas[i])
			}
		}
		// 排序：分数降序，同分按索引升序（旧的优先）
		sort.Slice(sortedMetas, func(i, j int) bool {
			if sortedMetas[i].score != sortedMetas[j].score {
				return sortedMetas[i].score > sortedMetas[j].score
			}
			return sortedMetas[i].index < sortedMetas[j].index
		})
		// 贪心填充（continue 而非 break：后面的消息可能更小能放下）
		acc := 0
		for _, m := range sortedMetas {
			if acc+m.tokens > remainingBudget {
				continue
			}
			selected[m.index] = true
			acc += m.tokens
		}
	}

	// 4. 按原索引升序构造结果，保持时序合法性
	var result []llm.ChatMessage
	for i, msg := range msgs {
		if selected[i] {
			result = append(result, msg)
		}
	}
	return result
}

// CompressContext 统一上下文压缩函数：滑动窗口裁剪 + 异步摘要
// 参数：
//   - parentCtx: 父上下文，用于异步摘要 goroutine 的生命周期跟踪。应用关闭时取消此 ctx，
//     goroutine 内的 LLM 调用会自动取消并退出，避免在 db 关闭后仍访问 db（H1 修复）。
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
	parentCtx context.Context,
	messages []llm.ChatMessage,
	contextSize int,
	existingSummary string,
	trimmedStoreMsgs []*store.Message,
	llmClient *llm.Client,
	convID string,
	db *sql.DB,
) CompressContextResult {
	// H1 修复：parentCtx 为 nil 时兜底为 Background，避免 context.WithTimeout(nil) panic
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	// 1. 分离 system 消息
	var systemMsg *llm.ChatMessage
	rest := messages
	if len(rest) > 0 && rest[0].Role == "system" {
		systemMsg = &rest[0]
		rest = rest[1:]
	}

	// 2. 计算滑动窗口大小
	windowSize := min(CalcSlidingWindowSize(contextSize), len(rest))

	// 3. 保留窗口内的最近消息
	windowStart := len(rest) - windowSize
	kept := rest[windowStart:]
	trimmed := rest[:windowStart] // 被裁剪的消息（ChatMessage 格式）

	// P3-B2: 3a. 从被裁剪消息中回收高分历史消息
	// 高分历史预算 = 上下文大小的 20%（上限 1000，下限 200），避免挤占最近窗口
	// 生活类比：像从旧书堆里挑几本"必藏经典"放到书桌显眼处，而不是全扔进仓库
	importantBudget := max(min(contextSize/importantBudgetRatio, maxImportantBudget), minImportantBudget)
	importantMsgs := selectImportantMessages(trimmed, importantBudget)

	// 4. 构建结果消息列表
	var result []llm.ChatMessage
	if systemMsg != nil {
		result = append(result, *systemMsg)
	}

	// P1-C1: 5. 如果有分层摘要（长期+短期），合并注入
	// 优先从 DB 读取分层摘要；若失败则回退到 existingSummary 参数（向后兼容）
	var existingLongSummary string
	var compressCount int
	if db != nil && convID != "" {
		if short, long, count, err := store.GetConversationLayeredSummary(db, convID); err == nil {
			if existingSummary == "" {
				existingSummary = short // DB 读取的短期摘要作为 fallback
			}
			existingLongSummary = long
			compressCount = count
		}
	}
	// 构造注入的摘要文本
	summaryParts := []string{}
	if existingLongSummary != "" {
		summaryParts = append(summaryParts, "长期记忆："+existingLongSummary)
	}
	if existingSummary != "" {
		summaryParts = append(summaryParts, "近期对话："+existingSummary)
	}
	if len(summaryParts) > 0 {
		result = append(result, llm.ChatMessage{
			Role:    "system",
			Content: "[对话摘要] " + strings.Join(summaryParts, "；"),
		})
	}

	// P3-B2: 5a. 添加高分历史消息（在摘要之后、最近窗口之前，时序合法）
	result = append(result, importantMsgs...)

	// 6. 添加窗口内的消息
	result = append(result, kept...)

	// 7. 保护 tool call 配对（复用 TrimMessagesToFit 中的清理逻辑）
	result = cleanToolCallPairs(result)

	// 8. 确保以 user 消息开头（Jinja 模板要求）
	// C-10 修复：复用 ensureStartsWithUserOrSystem，消除重复逻辑
	result = ensureStartsWithUserOrSystem(result)

	// P1-C1: 9. 异步生成分层摘要（短期 + 长期）
	// 短期摘要：每次压缩都更新（基于被裁剪的消息）
	// 长期摘要：每 N 次压缩合并一次（避免无限递归漂移）
	if len(trimmedStoreMsgs) >= 4 && llmClient != nil && convID != "" && db != nil {
		// 捕获分层摘要状态，避免在 goroutine 中访问共享变量
		capturedExistingSummary := existingSummary
		capturedLongSummary := existingLongSummary
		capturedCompressCount := compressCount
		// H1 修复：用 parentCtx 作为父上下文，应用关闭时 parentCtx 取消会自动传播到
		// summarizeCtx，使 goroutine 内的 LLM 调用立即取消并退出。
		// 这样无需 trackedGo 也能保证 goroutine 不在 db 关闭后访问 db。
		// 仍保留 defer recover() 防 panic 崩溃进程。
		go func() {
			// 防止 panic 导致整个进程崩溃（异步 goroutine 的 panic 无法被外层 recover 捕获）
			defer func() {
				if r := recover(); r != nil {
					log.Warn().Interface("panic", r).Str("conv_id", convID).Msg("[compress] 异步摘要生成 panic")
				}
			}()
			// 超时设为 summaryTimeoutSec*2+10s，留出足够时间给短期+长期两次 LLM 调用
			// 父 ctx（parentCtx）取消时会自动传播取消，应用关闭时 goroutine 立即退出
			summarizeCtx, cancel := context.WithTimeout(parentCtx, (summaryTimeoutSec*2+10)*time.Second)
			defer cancel()

			// P3-C2: 周期性摘要重置判断（每 10 次压缩重置一次）
			// 与 shouldMergeLongSummary（每 5 次）错开周期，重置优先级更高，触发时跳过合并
			var newShortSummary, newLongSummary string
			if ShouldResetSummary(capturedCompressCount) {
				// 重置模式：从当前所有被裁剪消息重新生成摘要，丢弃旧摘要
				// 生活类比：像定期把笔记本撕掉重写，而不是在旧笔记上涂涂改改
				newShortSummary = resetSummary(summarizeCtx, llmClient, trimmedStoreMsgs)
				if newShortSummary == "" {
					return
				}
				newLongSummary = "" // 清空长期摘要，下次 mergeLongSummary 会重新积累
				log.Info().Int("compress_count", capturedCompressCount+1).Str("conv_id", convID).Msg("[compress] 触发周期性摘要重置")
			} else {
				// 原有流程：增量短期摘要 + 每 5 次合并长期摘要
				newShortSummary = summarizeMessages(summarizeCtx, llmClient, capturedExistingSummary, trimmedStoreMsgs)
				if newShortSummary == "" {
					return
				}
				if shouldMergeLongSummary(capturedCompressCount) {
					// 合并长期摘要：旧长期 + 新短期 → 新长期
					newLongSummary = mergeLongSummary(summarizeCtx, llmClient, capturedLongSummary, newShortSummary)
					log.Info().Int("compress_count", capturedCompressCount+1).Str("conv_id", convID).Msg("[compress] 触发长期摘要合并")
				}
			}

			// 9c. 保存分层摘要（短期+长期）和递增的压缩计数
			if err := store.UpdateConversationLayeredSummary(db, convID, newShortSummary, newLongSummary); err != nil {
				log.Warn().Err(err).Msg("[compress] 保存分层摘要失败")
			} else {
				log.Debug().Str("conv_id", convID).Int("compress_count", capturedCompressCount+1).Bool("long_merged", newLongSummary != "").Bool("reset", ShouldResetSummary(capturedCompressCount)).Msg("[compress] 分层摘要已异步保存")
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
		return "\n请仅基于以上信息回答用户的问题，无法确认时明确说明，将信息提炼总结后组织成自然连贯的回答呈现。"
	}
	return "\nAnswer the user's question based strictly on the above information. If you cannot confirm, state it clearly. Summarize the key points and present them as a natural, coherent response."
}

// DetectLanguage is the exported version for testing.
func DetectLanguage(content string) string { return detectLanguage(content) }

// SearchResultInstruction is the exported version for testing.
func SearchResultInstruction(lang string) string { return searchResultInstruction(lang) }
func IsCodeRelated(query string) bool            { return isCodeRelated(query) }     // Exported for testing
func EstimateMessageTokens(m *store.Message) int { return estimateMessageTokens(m) } // Exported for testing
// formatSearchErrorHint 把 SearchChain 返回的 Error 字段转成用户友好的中文提示。
//
// SearchChain 在所有 provider 都失败时会返回形如：
//   "all search providers failed: tavily: <err>; ollama: <err>"
// 或在无可用 provider 时返回：
//   "no providers available"
//
// 本函数解析这些错误，归类为以下几种用户可理解的情况：
//   1. 未配置任何 API Key（搜索链为空）
//   2. Tavily / Ollama API Key 认证失败（401/403）
//   3. 网络超时
//   4. 网络连接错误
//   5. 其他未知错误（回退到原始错误摘要）
//
// 生活类比：像快递客服把一堆物流异常码翻译成客户能听懂的话——
// "DN02" 变成 "地址无人签收"，而不是让客户看原始编码。
func formatSearchErrorHint(errMsg string) string {
	if errMsg == "" {
		return ""
	}
	lower := strings.ToLower(errMsg)

	// 1. 无可用 provider：搜索链为空（两个 API Key 都未配置）
	if strings.Contains(lower, "no providers available") {
		return "未配置搜索 API Key，请在设置中配置 Tavily 或 Ollama API Key 后重试"
	}

	// 收集每个 provider 的具体错误，分别给出针对性提示
	var hints []string

	// 2. Tavily 相关错误
	if strings.Contains(lower, "tavily") {
		switch {
		case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "api key"):
			hints = append(hints, "Tavily API Key 无效或已过期，请在设置中重新填写")
		case strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
			hints = append(hints, "Tavily 搜索超时，请稍后重试")
		case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "dial"):
			hints = append(hints, "Tavily 网络连接失败，请检查网络后重试")
		case strings.Contains(lower, "empty results"):
			// 空结果不算错误，不提示
		default:
			hints = append(hints, "Tavily 搜索失败")
		}
	}

	// 3. Ollama 相关错误
	if strings.Contains(lower, "ollama") {
		switch {
		case strings.Contains(lower, "401") || strings.Contains(lower, "unauthorized") || strings.Contains(lower, "api key"):
			hints = append(hints, "Ollama API Key 无效或已过期，请在设置中重新填写")
		case strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
			hints = append(hints, "Ollama 搜索超时，请稍后重试")
		case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "dial"):
			hints = append(hints, "Ollama 网络连接失败，请检查网络后重试")
		case strings.Contains(lower, "empty results"):
			// 空结果不算错误，不提示
		default:
			hints = append(hints, "Ollama 搜索失败")
		}
	}

	// 4. 兜底：未能识别具体 provider，检查通用错误类型
	if len(hints) == 0 {
		switch {
		case strings.Contains(lower, "timeout") || strings.Contains(lower, "timed out"):
			return "搜索超时，请检查网络连接后重试"
		case strings.Contains(lower, "connection refused") || strings.Contains(lower, "no such host") || strings.Contains(lower, "dial"):
			return "搜索网络连接失败，请检查网络后重试"
		default:
			// 截断原始错误，避免过长
			snippet := errMsg
			if len(snippet) > 120 {
				snippet = snippet[:120] + "..."
			}
			return "搜索失败：" + snippet
		}
	}

	return strings.Join(hints, "；")
}

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
	// 请求条数从 10 降到 6：formatSearchResultsWithLang 只注入前 5 条到 prompt，
	// 多请求 1 条作为冗余。减少返回数据量可加快 HTTP 响应，也减少后续处理体积。
	resp := chain.SearchWithCategory(ctx, query, category, 6)
	if resp == nil {
		log.Warn().Str("query", query).Msg("[chat] search returned nil")
	}
	return resp
}
