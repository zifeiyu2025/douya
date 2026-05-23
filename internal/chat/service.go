// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/rs/zerolog/log"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/rag"
	"douya/internal/search"
	"douya/internal/store"
)

var searchToolDef = llm.ToolDefinition{
	Type: "function",
	Function: llm.FunctionDef{
		Name:        "search",
		Description: "Search the internet for up-to-date information. Use this tool when: (1) you need current news, latest data, or real-time information; (2) you are not confident about facts; (3) the user asks about recent events or specific URLs. For programming/code queries, the search engine will automatically use GitHub. Construct a concise and specific search query in the same language as the user's question. If the search returns no results, use your own knowledge to answer.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"query": map[string]interface{}{
					"type":        "string",
					"description": "The search query string. Should be concise and specific, in the same language as the user's question.",
				},
			},
			"required": []string{"query"},
		},
	},
}

type Service struct {
	llmClient         *llm.Client
	searchChain       *search.SearchChain
	db                *sql.DB
	config            *config.Config
	wailsCtx          context.Context
	currentCancel     context.CancelFunc
	currentConvID     string
	mutex             sync.Mutex
	modelCaps         llm.ModelCapabilities
	modelCapsMu       sync.RWMutex
	detectedModelName string
	sysPromptCache    string
	sysPromptDate     string
	sysPromptConfig   string
	// RAG
	ragVectorStore  *rag.VectorStore
	ragEmbedder    rag.Embedder
	ragCollection  string
	ragEnabled     bool
}

func NewService(llmClient *llm.Client, searchChain *search.SearchChain, db *sql.DB, cfg *config.Config) *Service {
	return &Service{
		llmClient:   llmClient,
		searchChain:  searchChain,
		db:           db,
		config:       cfg,
		modelCaps:    llm.ModelCapabilities{TextInput: true},
	}
}

func (s *Service) SetContext(ctx context.Context) {
	s.wailsCtx = ctx
}

func (s *Service) CurrentConvID() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.currentConvID
}

func (s *Service) UpdateClient(client *llm.Client) {
	s.llmClient = client
}

func (s *Service) UpdateSearchChain(chain *search.SearchChain) {
	s.searchChain = chain
}

func (s *Service) SetRAG(vs *rag.VectorStore, embedder rag.Embedder, collection string) {
	s.ragVectorStore = vs
	s.ragEmbedder = embedder
	s.ragCollection = collection
	s.ragEnabled = true
}


func (s *Service) DetectModelArchitecture() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	info, err := s.llmClient.GetModelInfo(ctx)
	if err != nil {
		return s.DetectModelArchitectureForModel("")
	}
	return s.DetectModelArchitectureForModel(info.Name)
}

func (s *Service) DetectModelArchitectureForModel(modelName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var info *llm.ModelInfo
	var err error
	if modelName != "" {
		info, err = s.llmClient.GetModelInfoByName(ctx, modelName)
	} else {
		info, err = s.llmClient.GetModelInfo(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to get model info: %w", err)
	}

	caps := llm.DetectCapabilities(*info)

	var supportsReasoning bool
	var mmprojLoaded bool

	props, propsErr := s.llmClient.GetServerProps(ctx, modelName)
	if propsErr == nil {
		log.Info().
			Bool("vision", props.Modalities.Vision).
			Bool("audio", props.Modalities.Audio).
			Interface("chat_template_caps", props.ChatTemplateCaps).
			Msg("[model] /props")

		mmprojLoaded = props.Modalities.Vision || props.Modalities.Audio
		caps.ImageInput = props.Modalities.Vision
		caps.AudioInput = props.Modalities.Audio

		if props.ChatTemplateCaps != nil {
			if v, ok := props.ChatTemplateCaps["supports_preserve_reasoning"]; ok && v {
				supportsReasoning = true
			}
		}
	} else {
		log.Warn().Err(propsErr).Msg("[model] /props failed, using /v1/models capabilities as fallback")
	}

	s.modelCapsMu.Lock()
	s.modelCaps = llm.ModelCapabilities{
		ImageInput:   caps.ImageInput,
		AudioInput:   caps.AudioInput,
		TextInput:    caps.TextInput,
		Reasoning:    supportsReasoning,
		MmprojLoaded: mmprojLoaded,
	}
	s.modelCapsMu.Unlock()
	// FIX: Only set detectedModelName when it's empty (called from DetectModelArchitecture without model name).
	// When called from SwitchModel, SetDetectedModelName() has already set the correct name.
	// Do NOT overwrite with info.Name, which may differ from the user-selected model name.
	if s.detectedModelName == "" {
		s.detectedModelName = info.Name
	}
	log.Info().
		Str("name", info.Name).
		Str("model", modelName).
		Interface("server_caps", info.Capabilities).
		Bool("image", caps.ImageInput).
		Bool("audio", caps.AudioInput).
		Bool("text", caps.TextInput).
		Bool("reasoning", supportsReasoning).
		Msg("[model] detected capabilities")

	return nil
}

func (s *Service) GetDetectedModelName() string {
	return s.detectedModelName
}

func (s *Service) SetDetectedModelName(name string) {
	s.detectedModelName = name
	s.InvalidatePromptCache()
}

func (s *Service) InvalidatePromptCache() {
	s.sysPromptCache = ""
	s.sysPromptDate = ""
	s.sysPromptConfig = ""
}

func (s *Service) GetModelCapabilities() llm.ModelCapabilities {
	s.modelCapsMu.RLock()
	defer s.modelCapsMu.RUnlock()
	return s.modelCaps
}
func (s *Service) modelNameForRequest() string {
	if s.detectedModelName != "" {
		return s.detectedModelName
	}
	return "default"
}


func (s *Service) emit(eventType string, content interface{}) {
	if s.wailsCtx != nil {
		runtime.EventsEmit(s.wailsCtx, "chat:stream", StreamEvent{
			Type:    eventType,
			Content: content,
		})
	}
}

func (s *Service) emitForConv(convID string, eventType string, content interface{}) {
	if s.wailsCtx != nil {
		runtime.EventsEmit(s.wailsCtx, "chat:stream", StreamEvent{
			Type:           eventType,
			Content:        content,
			ConversationID: convID,
		})
	}
}

func cleanThinkingContent(s string) string {
	if s == "" {
		return ""
	}
	var cleaned strings.Builder
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "<tool_call/>" {
			continue
		}
		if strings.HasPrefix(trimmed, "</tool_call") {
			continue
		}
		if cleaned.Len() > 0 {
			cleaned.WriteByte('\n')
		}
		cleaned.WriteString(line)
	}
	return strings.TrimSpace(cleaned.String())
}

func storeMsgToChat(m *store.Message) *Message {
	msg := &Message{
		ID:               m.ID,
		ConversationID:   m.ConversationID,
		Role:             m.Role,
		Content:          m.Content,
		ThinkingContent:  m.ThinkingContent,
		ThinkingDuration: m.ThinkingDuration,
		SearchResults:    m.SearchResults,
		Images:           m.Images,
		CreatedAt:        m.CreatedAt.Format(time.RFC3339),
	}
	if m.Attachments != "" {
		var atts []Attachment
		if err := json.Unmarshal([]byte(m.Attachments), &atts); err == nil {
			msg.Attachments = make([]AttachmentSummary, 0, len(atts))
			for _, a := range atts {
				msg.Attachments = append(msg.Attachments, AttachmentSummary{
					Type:     a.Type,
					Name:     a.Name,
					MimeType: a.MimeType,
				})
			}
		}
	}
	return msg
}

type StreamAccumulator struct {
	FullContent              string
	FullThinking             string
	FinishReason             string
	ToolCallMap              map[int]*llm.ToolCall
	EmitFn                   func(string, interface{})
	ConvID                   string
	EmitForConvFn            func(string, string, interface{})
	PendingBytes             string
	PendingThink             string
	LastSearchJSON           string
	ThinkingStartTime        time.Time
	ThinkingDuration         float64
	ThinkingDone             bool
	FirstRoundThinking       string
	FirstRoundThinkingDuration float64
}

func NewStreamAccumulator(convID string, emitFn func(string, interface{}), emitForConvFn func(string, string, interface{})) *StreamAccumulator {
	return &StreamAccumulator{
		ToolCallMap:   make(map[int]*llm.ToolCall),
		EmitFn:        emitFn,
		ConvID:        convID,
		EmitForConvFn: emitForConvFn,
	}
}

func (a *StreamAccumulator) callback() func(llm.SSEChunk) error {
	return func(chunk llm.SSEChunk) error {
		if len(chunk.Choices) == 0 {
			return nil
		}

		choice := chunk.Choices[0]
		deltaContent := choice.Delta.ContentString()
		if deltaContent != "" {
			if a.FullThinking != "" && !a.ThinkingDone && !a.ThinkingStartTime.IsZero() {
				a.ThinkingDuration = time.Since(a.ThinkingStartTime).Seconds()
				a.ThinkingDone = true
			}
			combined := a.PendingBytes + deltaContent
			valid, pending := llm.TruncateIncompleteUTF8(combined)
			a.PendingBytes = pending
			fixed := llm.FixUTF8(valid)
			a.FullContent += fixed
			a.EmitForConvFn(a.ConvID, "token", fixed)
		}

		if choice.Delta.ReasoningContent != "" {
			if a.ThinkingStartTime.IsZero() {
				a.ThinkingStartTime = time.Now()
			}
			combined := a.PendingThink + choice.Delta.ReasoningContent
			valid, pending := llm.TruncateIncompleteUTF8(combined)
			a.PendingThink = pending
			fixed := llm.FixUTF8(valid)
			a.FullThinking += fixed
			a.EmitForConvFn(a.ConvID, "thinking", fixed)
		}

		if len(choice.Delta.ToolCalls) > 0 {
			for _, tc := range choice.Delta.ToolCalls {
				idx := tc.Index
				if tc.ID != "" {
					if existing, ok := a.ToolCallMap[idx]; ok {
						existing.Function.Arguments += tc.Function.Arguments
					} else {
						newTC := llm.ToolCall{
							Index: tc.Index,
							ID:    tc.ID,
							Type:  tc.Type,
							Function: llm.FunctionCall{
								Name:      tc.Function.Name,
								Arguments: tc.Function.Arguments,
							},
						}
						a.ToolCallMap[idx] = &newTC
					}
				} else {
					if existing, ok := a.ToolCallMap[idx]; ok {
						existing.Function.Arguments += tc.Function.Arguments
					}
				}
			}
		}

		if choice.FinishReason != nil {
			if a.FullThinking != "" && !a.ThinkingDone && !a.ThinkingStartTime.IsZero() {
				a.ThinkingDuration = time.Since(a.ThinkingStartTime).Seconds()
				a.ThinkingDone = true
			}
			a.FinishReason = *choice.FinishReason
		}

		return nil
	}
}

func (a *StreamAccumulator) toolCalls() []llm.ToolCall {
	if len(a.ToolCallMap) == 0 {
		return nil
	}
	result := make([]llm.ToolCall, 0, len(a.ToolCallMap))
	for _, tc := range a.ToolCallMap {
		result = append(result, *tc)
	}
	return result
}

func (a *StreamAccumulator) resetForNextCall() {
	if a.FullThinking != "" {
		a.FirstRoundThinking = a.FullThinking
		a.FirstRoundThinkingDuration = a.ThinkingDuration
	}
	a.FullContent = ""
	a.FinishReason = ""
	a.ToolCallMap = make(map[int]*llm.ToolCall)
	a.PendingBytes = ""
	a.PendingThink = ""
	a.ThinkingStartTime = time.Time{}
	a.ThinkingDuration = 0
	a.ThinkingDone = false
}

func clampDuration(d float64) float64 {
	if d < 0 || d > 3600 {
		return 0
	}
	return d
}

func (s *Service) calcMaxTokens() int {
	ctxSize := s.config.ContextSize
	if ctxSize <= 0 {
		ctxSize = 4096
	}
	maxTokens := ctxSize / 2
	if maxTokens > 16384 {
		maxTokens = 16384
	}
	if maxTokens < 512 {
		maxTokens = 512
	}
	return maxTokens
}

func formatSearchResults(results []search.SearchResult) string {
	return formatSearchResultsWithLang(results, "zh")
}

func formatSearchResultsWithLang(results []search.SearchResult, lang string) string {
	contentLabel := "内容:"
	urlLabel := "链接:"
	if lang == "en" {
		contentLabel = "Content:"
		urlLabel = "URL:"
	}
	var sb strings.Builder
	for i, r := range results {
		sb.WriteString(fmt.Sprintf("[%d] %s\n%s %s\n%s %s\n\n", i+1, r.Title, urlLabel, r.URL, contentLabel, r.Snippet))
	}
	return sb.String()
}

func truncateSearchContext(searchContext string, ctxSize int) string {
	if ctxSize <= 0 {
		ctxSize = 4096
	}
	searchTokenEstimate := len([]rune(searchContext)) * 3
	maxSearchTokens := ctxSize / 3
	if searchTokenEstimate > maxSearchTokens {
		runes := []rune(searchContext)
		if maxSearchTokens/3 < len(runes) {
			searchContext = string(runes[:maxSearchTokens/3]) + "\n..."
		}
	}
	return searchContext
}

func ClampDuration(d float64) float64 { return clampDuration(d) } // Exported for testing
func CalcMaxTokens(s *Service) int    { return s.calcMaxTokens() } // Exported for testing
func FormatSearchResults(results []search.SearchResult) string { // Exported for testing
	return formatSearchResults(results)
}
func TruncateSearchContext(searchContext string, ctxSize int) string { // Exported for testing
	return truncateSearchContext(searchContext, ctxSize)
}
func StoreMsgToChat(m *store.Message) *Message { return storeMsgToChat(m) } // Exported for testing
func IsCodeRelated(query string) bool          { return isCodeRelated(query) } // Exported for testing
func BuildLLMMessages(s *Service, dbMsgs []*store.Message, currentUserContent string, currentAttachments []Attachment) ([]llm.ChatMessage, error) {
	return s.buildLLMMessages(dbMsgs, currentUserContent, currentAttachments, false)
}
func BuildLLMMessagesWithSearch(s *Service, dbMsgs []*store.Message, currentUserContent string, currentAttachments []Attachment, searchEnabled bool) ([]llm.ChatMessage, error) {
	return s.buildLLMMessages(dbMsgs, currentUserContent, currentAttachments, searchEnabled)
}

func InjectSearchContext(messages []llm.ChatMessage, searchContext string, instruction string) []llm.ChatMessage {
	lastUserIdx := -1
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}
	if lastUserIdx == -1 {
		return append(messages, llm.NewTextMessage("user", fmt.Sprintf("[补充信息]\n%s\n%s", searchContext, instruction)))
	}

	switch content := messages[lastUserIdx].Content.(type) {
	case []llm.ContentPart:
		modifiedParts := make([]llm.ContentPart, 0, len(content))
		for _, part := range content {
			if part.Type == "text" {
				modified := fmt.Sprintf("%s\n\n[补充信息]\n%s\n%s", part.Text, searchContext, instruction)
				modifiedParts = append(modifiedParts, llm.ContentPart{Type: "text", Text: modified})
			} else {
				modifiedParts = append(modifiedParts, part)
			}
		}
		messages[lastUserIdx] = llm.ChatMessage{Role: "user", Content: modifiedParts}
	default:
		original := messages[lastUserIdx].ContentString()
		modified := fmt.Sprintf("%s\n\n[补充信息]\n%s\n%s", original, searchContext, instruction)
		messages[lastUserIdx] = llm.NewTextMessage("user", modified)
	}
	return messages
}
func DoSearch(s *Service, ctx context.Context, query string) *search.SearchResponse { // Exported for testing
	return s.doSearch(ctx, query)
}
func ResetForNextCall(a *StreamAccumulator) { a.resetForNextCall() } // Exported for testing
func GetFirstRoundThinking(a *StreamAccumulator) string  { return a.FirstRoundThinking }
func GetDB(s *Service) *sql.DB              { return s.db } // Exported for testing
func SetCurrentCancel(s *Service, fn context.CancelFunc) { s.currentCancel = fn } // Exported for testing
func EstimateMessageTokens(m *store.Message) int { return estimateMessageTokens(m) } // Exported for testing
func CleanThinkingContent(s string) string       { return cleanThinkingContent(s) } // Exported for testing

func (s *Service) handleToolCallLoop(cancelCtx context.Context, convID string, llmMessages []llm.ChatMessage, acc *StreamAccumulator, maxRounds int) error {
	hitMaxRounds := false
	for round := 0; round < maxRounds; round++ {
		hitMaxRounds = round == maxRounds-1

		accumulatedToolCalls := acc.toolCalls()
		if acc.FinishReason != "tool_calls" || len(accumulatedToolCalls) == 0 {
			break
		}

		type toolCallResult struct {
			tc          llm.ToolCall
			toolContent string
			searchJSON  string
		}

		var toolResults []toolCallResult
		var toolMu sync.Mutex
		var toolWg sync.WaitGroup

		for _, tc := range accumulatedToolCalls {
			if tc.Function.Name != "search" {
				continue
			}
			s.emitForConv(convID, "search_start", tc.Function.Arguments)
			toolWg.Add(1)
			go func(tc llm.ToolCall) {
				defer toolWg.Done()
				var result toolCallResult
				result.tc = tc

				var args struct {
					Query string `json:"query"`
				}
				if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
					result.toolContent = fmt.Sprintf("Error: invalid arguments format. Expected JSON with \"query\" field. Got: %s. Please correct your arguments and try again.", tc.Function.Arguments)
				} else {
					s.emitForConv(convID, "tool_call_start", map[string]string{"tool": tc.Function.Name, "query": args.Query})
					searchResp := s.doSearch(cancelCtx, args.Query)
					if searchResp != nil && len(searchResp.Results) > 0 {
						s.emitForConv(convID, "search_result", searchResp.Results)
						sj, _ := json.Marshal(searchResp.Results)
						result.searchJSON = string(sj)
						result.toolContent = formatSearchResultsWithLang(searchResp.Results, detectLanguage(args.Query))
					} else {
						result.toolContent = fmt.Sprintf("No search results found for \"%s\". Please use your own knowledge to answer the user's question.", args.Query)
					}
				}

				toolMu.Lock()
				toolResults = append(toolResults, result)
				toolMu.Unlock()
			}(tc)
		}
		toolWg.Wait()

		for _, tr := range toolResults {
			assistantToolCallJSON, _ := json.Marshal([]llm.ToolCall{tr.tc})
			if err := store.CreateMessage(s.db, &store.Message{
				ConversationID:   convID,
				Role:             "assistant",
				Content:          "",
				ToolCalls:        string(assistantToolCallJSON),
				ThinkingContent:  acc.FirstRoundThinking,
				ThinkingDuration: clampDuration(acc.FirstRoundThinkingDuration),
			}); err != nil {
				log.Error().Err(err).Msg("save assistant tool call message")
			}

			llmMessages = append(llmMessages, llm.ChatMessage{
				Role:      "assistant",
				Content:   "",
				ToolCalls: []llm.ToolCall{tr.tc},
			})

			if tr.searchJSON != "" {
				acc.LastSearchJSON = tr.searchJSON
			}

			if err := store.CreateMessage(s.db, &store.Message{
				ConversationID: convID,
				Role:           "tool",
				Content:        tr.toolContent,
				ToolCallID:     tr.tc.ID,
			}); err != nil {
				log.Error().Err(err).Msg("save tool result message")
			}

			llmMessages = append(llmMessages, llm.ChatMessage{
				Role:       "tool",
				Content:    tr.toolContent,
				ToolCallID: tr.tc.ID,
			})
		}

		req := &llm.ChatCompletionRequest{
			Model:         s.modelNameForRequest(),
			Messages:      llmMessages,
			MaxTokens:     s.calcMaxTokens(),
			Temperature:   s.config.Temperature,
			TopP:          s.config.TopP,
			TopK:          s.config.TopK,
			RepeatPenalty: s.config.RepeatPenalty,
		}
		if !hitMaxRounds {
			req.Tools = []llm.ToolDefinition{searchToolDef}
		}

		acc.resetForNextCall()
		toolCtx, toolCancel := context.WithTimeout(cancelCtx, 300*time.Second)
		err := s.llmClient.StreamChat(toolCtx, req, acc.callback())
		toolCancel()
		if err != nil {
			if cancelCtx.Err() == context.Canceled {
				s.emitForConv(convID, "stopped", nil)
				return nil
			}
			if toolCtx.Err() == context.DeadlineExceeded {
				s.emitForConv(convID, "error", "工具调用生成超时")
				return fmt.Errorf("tool call stream timeout")
			}
			s.emitForConv(convID, "error", err.Error())
			return fmt.Errorf("stream chat after search: %w", err)
		}

		if acc.FinishReason != "tool_calls" {
			break
		}
	}

	aiMsg := &store.Message{
		ConversationID:   convID,
		Role:             "assistant",
		Content:          acc.FullContent,
		ThinkingContent:  acc.FullThinking,
		ThinkingDuration: clampDuration(acc.ThinkingDuration),
	}
	if acc.LastSearchJSON != "" {
		aiMsg.SearchResults = acc.LastSearchJSON
	}
	if acc.FinishReason == "tool_calls" && hitMaxRounds {
		aiMsg.Content += "\n\n[工具调用已达最大轮次限制，部分搜索结果可能未完全处理]"
	}
	if err := store.CreateMessage(s.db, aiMsg); err != nil {
		log.Error().Err(err).Msg("save ai message")
	}
	s.emitForConv(convID, "assistant_message", storeMsgToChat(aiMsg))
	return nil
}

func (s *Service) SendMessage(ctx context.Context, params SendMessageParams) error {
	s.mutex.Lock()
	var oldCancel context.CancelFunc
	var oldConvID string
	if s.currentCancel != nil {
		oldCancel = s.currentCancel
		oldConvID = s.currentConvID
	}
	cancelCtx, cancel := context.WithCancel(ctx)
	s.currentCancel = cancel
	s.currentConvID = params.ConversationID
	s.mutex.Unlock()
	if oldCancel != nil {
		oldCancel()
		if oldConvID != "" {
			s.emitForConv(oldConvID, "stopped", nil)
		}
	}
	defer func() {
		s.mutex.Lock()
		s.currentCancel = nil
		s.currentConvID = ""
		s.mutex.Unlock()
	}()

	convID := params.ConversationID
	if convID == "" {
		conv := &store.Conversation{Title: "新对话"}
		if err := store.CreateConversation(s.db, conv); err != nil {
			s.emitForConv("", "error", fmt.Sprintf("create conversation: %v", err))
			return fmt.Errorf("create conversation: %w", err)
		}
		convID = conv.ID
		s.mutex.Lock()
		s.currentConvID = convID
		s.mutex.Unlock()
		s.emitForConv(convID, "conversation_created", &Conversation{
			ID:        conv.ID,
			Title:     conv.Title,
			CreatedAt: conv.CreatedAt.Format(time.RFC3339),
			UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
		})
	}

	userContent := params.Content

	userMsg := &store.Message{
		ConversationID: convID,
		Role:           "user",
		Content:        params.Content,
	}
	if len(params.Images) > 0 {
		imgJSON, _ := json.Marshal(params.Images)
		userMsg.Images = string(imgJSON)
	}
	if len(params.Attachments) > 0 {
		attJSON, _ := json.Marshal(params.Attachments)
		userMsg.Attachments = string(attJSON)
	}
	if err := store.CreateMessage(s.db, userMsg); err != nil {
		s.emitForConv(convID, "error", fmt.Sprintf("save user message: %v", err))
		return fmt.Errorf("save user message: %w", err)
	}
	emitMsg := &Message{
		ID:             userMsg.ID,
		ConversationID: userMsg.ConversationID,
		Role:           userMsg.Role,
		Content:        userMsg.Content,
		Images:         userMsg.Images,
		CreatedAt:      userMsg.CreatedAt.Format(time.RFC3339),
	}
	if len(params.Attachments) > 0 {
		emitMsg.Attachments = make([]AttachmentSummary, 0, len(params.Attachments))
		for _, a := range params.Attachments {
			emitMsg.Attachments = append(emitMsg.Attachments, AttachmentSummary{
				Type:     a.Type,
				Name:     a.Name,
				MimeType: a.MimeType,
			})
		}
	}
	s.emitForConv(convID, "user_message", emitMsg)

	dbMsgs, err := store.GetMessagesByConversation(s.db, convID)
	if err != nil {
		s.emitForConv(convID, "error", fmt.Sprintf("load messages: %v", err))
		return fmt.Errorf("load messages: %w", err)
	}

	llmMessages, err := s.buildLLMMessages(dbMsgs, userContent, params.Attachments, params.SearchEnabled)
	if err != nil {
		s.emitForConv(convID, "error", err.Error())
		return err
	}

	return s.streamWithSearch(cancelCtx, convID, llmMessages, params.SearchEnabled, params.Content, params.Content)
}

func (s *Service) streamWithSearch(cancelCtx context.Context, convID string, llmMessages []llm.ChatMessage, searchEnabled bool, searchQuery string, titleContent string) error {
	acc := NewStreamAccumulator(convID, s.emit, s.emitForConv)

	var searchResp *search.SearchResponse

	if searchEnabled {
		s.emitForConv(convID, "search_start", searchQuery)

		if s.searchChain != nil {
			category := "general"
			if isCodeRelated(searchQuery) {
				category = "code"
			}
			searchCtx, searchCancel := context.WithTimeout(cancelCtx, 10*time.Second)
			searchResp = s.searchChain.SearchWithCategory(searchCtx, searchQuery, category)
			searchCancel()
		}

		if searchResp != nil && len(searchResp.Results) > 0 {
			s.emitForConv(convID, "search_result", searchResp.Results)

			sj, _ := json.Marshal(searchResp.Results)
			acc.LastSearchJSON = string(sj)

			searchContext := formatSearchResultsWithLang(searchResp.Results, detectLanguage(searchQuery))
			searchContext = truncateSearchContext(searchContext, s.config.ContextSize)

			lang := detectLanguage(searchQuery)
			instruction := searchResultInstruction(lang)

			llmMessages = InjectSearchContext(llmMessages, searchContext, instruction)
		} else {
			s.emitForConv(convID, "search_result", []search.SearchResult{})
		}
		if searchResp != nil && searchResp.Error != "" && len(searchResp.Results) == 0 {
			log.Info().Str("error", searchResp.Error).Msg("[search] 搜索未返回结果")
		}
	}

	req := &llm.ChatCompletionRequest{
		Model:         s.modelNameForRequest(),
		MaxTokens:     s.calcMaxTokens(),
		Temperature:   s.config.Temperature,
		TopP:          s.config.TopP,
		TopK:          s.config.TopK,
		RepeatPenalty: s.config.RepeatPenalty,
	}
	if !searchEnabled {
		req.Tools = []llm.ToolDefinition{searchToolDef}
	}

	req.Messages = llmMessages

	streamCtx, streamCancel := context.WithTimeout(cancelCtx, 300*time.Second)
	defer streamCancel()

	err := s.llmClient.StreamChat(streamCtx, req, acc.callback())

	if err != nil {
		if cancelCtx.Err() == context.Canceled {
			s.emitForConv(convID, "stopped", nil)
			return nil
		}
		if streamCtx.Err() == context.DeadlineExceeded {
			s.emitForConv(convID, "error", "生成超时，请重试")
			return fmt.Errorf("stream chat timeout")
		}
		s.emitForConv(convID, "error", err.Error())
		return fmt.Errorf("stream chat: %w", err)
	}

	if acc.FinishReason == "tool_calls" && len(acc.toolCalls()) > 0 {
		if err := s.handleToolCallLoop(cancelCtx, convID, llmMessages, acc, 3); err != nil {
			return err
		}
	} else {
		aiMsg := &store.Message{
			ConversationID:   convID,
			Role:             "assistant",
			Content:          acc.FullContent,
			ThinkingContent:  acc.FullThinking,
			ThinkingDuration: clampDuration(acc.ThinkingDuration),
		}
		if acc.LastSearchJSON != "" {
			aiMsg.SearchResults = acc.LastSearchJSON
		}
		if err := store.CreateMessage(s.db, aiMsg); err != nil {
			log.Error().Err(err).Msg("save ai message")
		}
		s.emitForConv(convID, "assistant_message", storeMsgToChat(aiMsg))
	}

	conv, _ := store.GetConversation(s.db, convID)
	if conv != nil {
		if conv.Title == "新对话" && len(titleContent) > 0 {
			title := strings.ToValidUTF8(titleContent, "\uFFFD")
			runeTitle := []rune(title)
			if len(runeTitle) > 20 {
				title = string(runeTitle[:20]) + "..."
			}
			conv.Title = title
		}
		store.UpdateConversation(s.db, conv)
		s.emitForConv(convID, "conversation_updated", &Conversation{
			ID:        conv.ID,
			Title:     conv.Title,
			CreatedAt: conv.CreatedAt.Format(time.RFC3339),
			UpdatedAt: conv.UpdatedAt.Format(time.RFC3339),
		})
	}

	s.emitForConv(convID, "done", nil)
	return nil
}

func buildMessageFromAttachments(role, content string, attachments []Attachment) llm.ChatMessage {
	var imageUrls []string
	var audios []llm.InputAudio
	var textParts []string

	for _, att := range attachments {
		switch att.Type {
		case "image":
			imageUrls = append(imageUrls, att.Data)
		case "audio":
			audios = append(audios, llm.InputAudio{Data: att.Data, Format: att.Format})
		case "pdf":
			// Phase2: Use existing extractPDFText (defined in pdf.go)
			pdfText := extractPDFText([]byte(att.Data))
			textParts = append(textParts, fmt.Sprintf("--- 附件: %s (%s) ---\n%s\n--- 附件结束 ---", att.Name, att.MimeType, pdfText))
		case "text":
			textParts = append(textParts, fmt.Sprintf("--- 附件: %s (%s) ---\n%s\n--- 附件结束 ---", att.Name, att.MimeType, att.Data))
		}
	}

	extraText := strings.Join(textParts, "\n\n")
	fullContent := content
	if extraText != "" {
		if fullContent == "" {
			fullContent = extraText
		} else {
			fullContent = extraText + "\n\n" + content
		}
	}

	if fullContent == "" {
		if len(imageUrls) > 0 {
			fullContent = "请描述这张图片"
		} else if len(audios) > 0 {
			fullContent = "请转录这段音频"
		}
	}

	if len(imageUrls) > 0 && len(audios) > 0 {
		return llm.NewMultimodalMessage(role, fullContent, imageUrls, audios)
	} else if len(imageUrls) > 0 {
		return llm.NewVisionMessage(role, fullContent, imageUrls)
	} else if len(audios) > 0 {
		return llm.NewAudioMessage(role, fullContent, audios)
	}
	return llm.NewTextMessage(role, fullContent)
}



func (s *Service) buildLLMMessages(dbMsgs []*store.Message, currentUserContent string, currentAttachments []Attachment, searchEnabled bool) ([]llm.ChatMessage, error) {
	maxContext := s.config.ContextSize
	if maxContext <= 0 {
		maxContext = 4096
	}

	s.modelCapsMu.RLock()
	caps := s.modelCaps
	s.modelCapsMu.RUnlock()

	for _, att := range currentAttachments {
		if att.Type == "image" && !caps.ImageInput {
			return nil, fmt.Errorf("当前模型不支持图片输入，请加载支持视觉的模型（如 llava 系列）")
		}
		if att.Type == "audio" && !caps.AudioInput {
			return nil, fmt.Errorf("当前模型不支持音频输入，请加载支持音频的模型（如 whisper 系列）")
		}
	}

	now := time.Now()
	today := now.Format("2006-01-02")
	configPrompt := s.config.SystemPrompt

	// Rebuild cache if date changed or config changed
	if s.sysPromptCache == "" || s.sysPromptDate != today || s.sysPromptConfig != configPrompt {
		systemContent := configPrompt
		if systemContent == "" {
			systemContent = `你叫豆芽，是一个运行在本地的智能AI助手，基于开源大语言模型和llama.cpp推理引擎，为用户提供私密、高效、可靠的服务。

## 核心身份
- 你的品牌名称始终是「豆芽」（DouYa），底层运行着不同的开源模型。当用户问"你是谁"时回答"我叫豆芽"；当用户问"你是什么模型"时，回答"我叫豆芽，当前运行的底层模型是 [模型名，见下文]"
- 你不是任何商业产品，你是豆芽——一个自由的本地AI助手
- 你运行在用户的个人电脑上，注重隐私保护和离线可用性

## 能力范围
- 拥有截止到模型训练日期的广泛知识储备
- 具备联网搜索能力，可获取实时信息和最新数据
- 擅长编程、写作、翻译、分析、推理、创意等各类任务
- 支持图像理解和多模态输入（取决于所加载的模型）
- 能够处理长文档和复杂多轮对话

## 回答规范
- 准确优先：不确定的事实应明确说明不确定，绝不编造
- 结构清晰：复杂问题分步骤、分要点回答，善用标题和列表
- 代码规范：提供完整可运行的代码示例，标注语言类型，附简要说明
- 引用规范：使用外部信息时以[1][2]形式标注来源，自然地融入行文，不提及信息来源（如不说"根据搜索结果"）
- 语言一致：始终使用与用户相同的语言回复
- 保持中立：对争议话题客观陈述各方观点，不预设立场

## 交互风格
- 友好耐心但专业精炼，不啰嗦不敷衍
- 用户提出模糊需求时主动追问澄清
- 善于举一反三，提供超出问题本身的深层见解`
		}
		if s.detectedModelName != "" {
			systemContent += fmt.Sprintf("\n\n你当前加载的底层模型是 %s。当用户询问你的模型名称、版本或型号时，应基于此信息回答。", s.detectedModelName)
		}
		s.sysPromptCache = systemContent
		s.sysPromptDate = today
		s.sysPromptConfig = configPrompt
	}

	// Append dynamic date/time each request
	weekday := ""
	switch now.Weekday() {
	case time.Sunday:
		weekday = "星期日"
	case time.Monday:
		weekday = "星期一"
	case time.Tuesday:
		weekday = "星期二"
	case time.Wednesday:
		weekday = "星期三"
	case time.Thursday:
		weekday = "星期四"
	case time.Friday:
		weekday = "星期五"
	case time.Saturday:
		weekday = "星期六"
	}
	systemContent := s.sysPromptCache + fmt.Sprintf("\n\n当前日期时间: %s %s", now.Format("2006-01-02 15:04:05"), weekday)
	if !searchEnabled {
		systemContent += "\n\n需要最新信息、不确定事实或用户明确要求时，使用search工具进行搜索。常识性问题直接回答即可。"
	} else {
		systemContent += "\n\n如果对话中包含了补充资料，请严格按引用规范标注来源，自然地融入回答。"
	}

	estimatedTokens := len([]rune(systemContent)) * 3

	var history []llm.ChatMessage

	for i := len(dbMsgs) - 1; i >= 0; i-- {
		m := dbMsgs[i]

		estimated := estimateMessageTokens(m)
		if estimated == 0 {
			estimated = 1
		}
		if estimatedTokens+estimated > maxContext {
			break
		}
		estimatedTokens += estimated

		if m.Role == "tool" {
			msg := llm.ChatMessage{
				Role:       "tool",
				Content:    m.Content,
				ToolCallID: m.ToolCallID,
			}
			history = append([]llm.ChatMessage{msg}, history...)
			continue
		}

		if m.Role == "assistant" && m.ToolCalls != "" {
			var toolCalls []llm.ToolCall
			if err := json.Unmarshal([]byte(m.ToolCalls), &toolCalls); err == nil && len(toolCalls) > 0 {
				msg := llm.ChatMessage{
					Role:      "assistant",
					Content:   m.Content,
					ToolCalls: toolCalls,
				}
				history = append([]llm.ChatMessage{msg}, history...)
				continue
			}
		}

		content := m.Content
		if m.Role == "user" {
			if m.ID == dbMsgs[len(dbMsgs)-1].ID {
				content = currentUserContent
			}
			if content == "" && (m.Images != "" || m.Attachments != "") {
				content = "请描述这张图片"
			}
		}

		var msg llm.ChatMessage
		if m.Role == "user" && m.ID == dbMsgs[len(dbMsgs)-1].ID && len(currentAttachments) > 0 {
			msg = buildMessageFromAttachments(m.Role, content, currentAttachments)
		} else if m.Role == "user" && m.Attachments != "" {
			var dbAttachments []Attachment
			if err := json.Unmarshal([]byte(m.Attachments), &dbAttachments); err == nil && len(dbAttachments) > 0 {
				// Phase1: Check model capabilities for historical attachments
				supportsAll := true
				for _, att := range dbAttachments {
					if att.Type == "image" && !caps.ImageInput {
						supportsAll = false
						break
					}
					if att.Type == "audio" && !caps.AudioInput {
						supportsAll = false
						break
					}
				}
				if supportsAll {
					msg = buildMessageFromAttachments(m.Role, content, dbAttachments)
				} else {
					msg = llm.NewTextMessage(m.Role, content)
				}
			} else if m.Images != "" {
				if caps.ImageInput {
					var imageUrls []string
					if err := json.Unmarshal([]byte(m.Images), &imageUrls); err == nil && len(imageUrls) > 0 {
						msg = llm.NewVisionMessage(m.Role, content, imageUrls)
					} else {
						msg = llm.NewTextMessage(m.Role, content)
					}
				} else {
					msg = llm.NewTextMessage(m.Role, content)
				}
			} else {
				msg = llm.NewTextMessage(m.Role, content)
			}
		} else if m.Role == "user" && m.Images != "" {
		if caps.ImageInput {
				var imageUrls []string
				if err := json.Unmarshal([]byte(m.Images), &imageUrls); err == nil && len(imageUrls) > 0 {
					msg = llm.NewVisionMessage(m.Role, content, imageUrls)
				} else {
					msg = llm.NewTextMessage(m.Role, content)
				}
			} else {
				msg = llm.NewTextMessage(m.Role, content)
			}
		} else {
			msg = llm.NewTextMessage(m.Role, content)
		}
		history = append([]llm.ChatMessage{msg}, history...)
	}

	for len(history) > 0 && history[0].Role != "user" && history[0].Role != "system" {
		history = history[1:]
	}

	var cleaned []llm.ChatMessage
	for i := 0; i < len(history); i++ {
		msg := history[i]
		if msg.Role == "tool" {
			if len(cleaned) > 0 && cleaned[len(cleaned)-1].Role == "assistant" && len(cleaned[len(cleaned)-1].ToolCalls) > 0 {
				cleaned = append(cleaned, msg)
			}
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			hasFollowingTool := false
			for j := i + 1; j < len(history); j++ {
				if history[j].Role == "tool" && history[j].ToolCallID != "" {
					for _, tc := range msg.ToolCalls {
						if tc.ID == history[j].ToolCallID {
							hasFollowingTool = true
							break
						}
					}
				}
				if hasFollowingTool {
					break
				}
			}
			if hasFollowingTool {
				cleaned = append(cleaned, msg)
			}
			continue
		}
		cleaned = append(cleaned, msg)
	}
	history = cleaned

	cleaned = nil
	for _, m := range history {
		if len(cleaned) > 0 && cleaned[len(cleaned)-1].Role == "assistant" && m.Role == "assistant" {
			cleaned[len(cleaned)-1] = m
		} else {
			cleaned = append(cleaned, m)
		}
	}
	history = cleaned

	// RAG: inject relevant document chunks into system prompt before building messages
	if s.ragEnabled && s.ragVectorStore != nil && s.ragEmbedder != nil && s.ragCollection != "" && currentUserContent != "" {
		ctxRag, cancelRag := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancelRag()
		vecs, err := s.ragEmbedder.Embed(ctxRag, []string{currentUserContent})
		if err == nil && len(vecs) > 0 && len(vecs[0]) > 0 {
			results, err2 := s.ragVectorStore.Search(s.ragCollection, vecs[0], 3)
			if err2 == nil && len(results) > 0 {
				var parts []string
				for _, r := range results {
					if r.ChunkContent != "" {
						parts = append(parts, r.ChunkContent)
					}
				}
				if len(parts) > 0 {
					systemContent += "\n\n## 参考资料\n" + strings.Join(parts, "\n---\n")
				}
			}
		}
	}

	var messages []llm.ChatMessage
	messages = append(messages, llm.ChatMessage{
		Role:    "system",
		Content: systemContent,
	})
	messages = append(messages, history...)


	return messages, nil
}