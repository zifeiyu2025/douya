// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/mcp"
	"douya/internal/rag"
	"douya/internal/search"
	"douya/internal/secrets"
	"douya/internal/store"
)

type Service struct {
	llmClient         *llm.Client
	searchChain       *search.SearchChain
	db                *sql.DB
	config            *config.Config
	wailsCtx          context.Context
	appDir            string
	currentCancel     context.CancelFunc
	currentConvID     string
	mutex             sync.RWMutex
	modelCaps         llm.ModelCapabilities
	modelCapsMu       sync.RWMutex
	detectedModelName string
	detectedModelMu   sync.RWMutex
	cachedProps       *llm.ServerProps
	cachedPropsMu     sync.RWMutex
	sysPromptCache    string
	sysPromptDate     string
	sysPromptConfig   string
	promptMu          sync.RWMutex
	cipher            secrets.Cipher
	// RAG
	ragMu          sync.RWMutex
	ragVectorStore *rag.VectorStore
	ragDocStore    *rag.DocumentStore
	ragEmbedder    rag.Embedder
	ragCollection  string
	ragEnabled     bool
	// MCP（原生 MCP 客户端，连接外部 MCP server 获取工具）
	mcpManager *mcp.Manager
	// prompt_tokens 反馈校准
	lastPromptTokens    int // 最近一次实际 prompt_tokens（来自 llama-server usage）
	lastEstimatedTokens int // 对应的估算值
	tokenCalibMu        sync.RWMutex
	// 当前流式聊天的 completion ID，用于 /v1/chat/completions/control 实时控制
	currentCompletionID string
	completionIDMu      sync.RWMutex
	// slot 自动持久化：记录最后保存 KV 缓存的对话 ID
	// 仅在 SlotSaveEnabled=true 时启用，用于对话切换时跳过重复 prefill
	lastSavedConvID string
	lastSavedSlotMu sync.RWMutex
}

func NewService(llmClient *llm.Client, searchChain *search.SearchChain, db *sql.DB, cfg *config.Config, cipher secrets.Cipher, appDir string) *Service {
	return &Service{
		llmClient:   llmClient,
		searchChain: searchChain,
		db:          db,
		config:      cfg,
		cipher:      cipher,
		appDir:      appDir,
		modelCaps:   llm.ModelCapabilities{TextInput: true},
	}
}

func (s *Service) SetContext(ctx context.Context) {
	s.wailsCtx = ctx
}

// LastPromptTokens 返回最近一次请求的 prompt_tokens（来自 llama-server usage）。
// 这是真实的上下文已用 token 数（含系统提示词+历史消息+RAG+搜索结果等）。
// 用于前端持久化显示总上下文 token 用量。
func (s *Service) LastPromptTokens() int {
	s.tokenCalibMu.RLock()
	defer s.tokenCalibMu.RUnlock()
	return s.lastPromptTokens
}

func (s *Service) CurrentConvID() string {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.currentConvID
}

// IsGenerating 返回当前是否正在生成（currentConvID 非空表示正在生成）。
// 用于让 router 模式轮询在生成期间暂停，避免与生成请求争用 HTTP 连接。
func (s *Service) IsGenerating() bool {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.currentConvID != ""
}

func (s *Service) UpdateClient(client *llm.Client) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.llmClient = client
}

func (s *Service) UpdateSearchChain(chain *search.SearchChain) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.searchChain = chain
}

// getConfigSnapshot 在读锁保护下获取配置快照，避免数据竞争。
// 升级为 RWMutex 后，多个并发请求读取 config 可并行，不再串行化。
// 生活类比：就像在图书馆查阅共享资料时，多人可同时阅读（读锁），只有修改时才独占（写锁）。
func (s *Service) getConfigSnapshot() *config.Config {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.config
}

// getClientSnapshot 在读锁保护下获取 LLM 客户端快照，避免数据竞争。
func (s *Service) getClientSnapshot() *llm.Client {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.llmClient
}

// getSearchChainSnapshot 在读锁保护下获取搜索链快照，避免数据竞争。
func (s *Service) getSearchChainSnapshot() *search.SearchChain {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.searchChain
}

func (s *Service) SetRAG(vs *rag.VectorStore, ds *rag.DocumentStore, embedder rag.Embedder, collection string, enabled bool) {
	s.ragMu.Lock()
	defer s.ragMu.Unlock()
	s.ragVectorStore = vs
	s.ragDocStore = ds
	s.ragEmbedder = embedder
	s.ragCollection = collection
	s.ragEnabled = enabled
}

func (s *Service) SetRAGCollection(collection string) {
	s.ragMu.Lock()
	defer s.ragMu.Unlock()
	s.ragCollection = collection
}

func (s *Service) SetRAGEnabled(enabled bool) {
	s.ragMu.Lock()
	defer s.ragMu.Unlock()
	s.ragEnabled = enabled
}

// SetMCPManager 设置 MCP 管理器（由 app 层在启动时注入）。
// 生活类比：前台装上了外卖对接系统，之后就能把各平台的菜品加入菜单了。
func (s *Service) SetMCPManager(mgr *mcp.Manager) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.mcpManager = mgr
}

// getMCPManager 在读锁保护下获取 MCP 管理器快照。
func (s *Service) getMCPManager() *mcp.Manager {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.mcpManager
}

func (s *Service) emit(eventType string, content any) {
	if s.wailsCtx != nil {
		runtime.EventsEmit(s.wailsCtx, "chat:stream", StreamEvent{
			Type:    eventType,
			Content: content,
		})
	}
}

func (s *Service) emitForConv(convID string, eventType string, content any) {
	if s.wailsCtx != nil {
		runtime.EventsEmit(s.wailsCtx, "chat:stream", StreamEvent{
			Type:           eventType,
			Content:        content,
			ConversationID: convID,
		})
	}
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

// CompressResult 是手动压缩操作的返回结果
type CompressResult struct {
	ShortSummary string `json:"shortSummary"` // 新生成的短期摘要
	LongSummary  string `json:"longSummary"`  // 新生成的长期摘要（可能为空，仅在触发合并时有值）
	TrimmedCount int    `json:"trimmedCount"` // 被裁剪的早期消息数
	Message      string `json:"message"`      // 状态提示信息（如"压缩成功"/"消息较少，无需压缩"）
}

// CompressConversation 手动触发对话压缩（P2-A3）。
//
// 用户点击 TokenCounter 旁的"立即压缩"按钮时调用。
// 行为：按滑动窗口裁剪早期消息，同步生成新摘要并保存到 DB。
// 与 CompressContext 的区别：
//   - CompressContext 是构建 LLM 消息列表时的同步压缩（返回压缩后的消息供立即使用，摘要异步生成）
//   - CompressConversation 是用户主动触发（同步生成摘要并返回，不返回消息列表）
//
// 复用的核心函数：CalcSlidingWindowSize / summarizeMessages / shouldMergeLongSummary / mergeLongSummary
func (s *Service) CompressConversation(convID string) (*CompressResult, error) {
	if convID == "" {
		return nil, fmt.Errorf("会话 ID 不能为空")
	}

	// 1. 加载会话所有 DB 消息
	dbMsgs, err := store.GetMessagesByConversation(s.db, convID, secrets.CipherKey(s.cipher))
	if err != nil {
		return nil, fmt.Errorf("加载消息失败: %w", err)
	}

	// 2. 读取上下文大小（用于计算滑动窗口）
	cfg := s.getConfigSnapshot()
	maxContext := 0
	if cfg != nil {
		maxContext = cfg.ContextSize
	}
	if maxContext <= 0 {
		maxContext = 4096
	}

	// 3. 按滑动窗口大小分离 kept 和 trimmed
	// 生活类比：像整理书架，最近看过的书留在桌上，旧书归档到箱子里
	windowSize := CalcSlidingWindowSize(maxContext)
	if windowSize >= len(dbMsgs) {
		// 消息较少，无需压缩
		short, long, _, _ := store.GetConversationLayeredSummary(s.db, convID)
		return &CompressResult{
			ShortSummary: short,
			LongSummary:  long,
			TrimmedCount: 0,
			Message:      "消息较少，无需压缩",
		}, nil
	}
	trimmedMsgs := dbMsgs[:len(dbMsgs)-windowSize]

	// 被裁剪消息过少时不生成摘要（summarizeMessages 要求 >=4 条）
	if len(trimmedMsgs) < 4 {
		short, long, _, _ := store.GetConversationLayeredSummary(s.db, convID)
		return &CompressResult{
			ShortSummary: short,
			LongSummary:  long,
			TrimmedCount: len(trimmedMsgs),
			Message:      "被裁剪消息过少，无需生成摘要",
		}, nil
	}

	// 4. 读取现有分层摘要（用于增量更新）
	shortSummary, longSummary, compressCount, _ := store.GetConversationLayeredSummary(s.db, convID)

	// 5. 获取 LLM 客户端
	client := s.getClientSnapshot()
	if client == nil {
		return nil, fmt.Errorf("LLM 客户端未初始化，请等待模型加载完成")
	}

	// 6. 同步生成新短期摘要
	// 超时设为 summaryTimeoutSec*2+10s，与 CompressContext 一致，留出短期+长期两次 LLM 调用时间
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(summaryTimeoutSec*2+10)*time.Second)
	defer cancel()

	// P3-C2: 判断是否触发周期性重置（与 CompressContext 保持一致）
	// 重置优先级高于合并，触发时跳过 shouldMergeLongSummary
	var newShortSummary, newLongSummary string
	if ShouldResetSummary(compressCount) {
		// 重置模式：从当前所有被裁剪消息重新生成摘要，丢弃旧摘要
		newShortSummary = resetSummary(ctx, client, trimmedMsgs)
		if newShortSummary == "" {
			return nil, fmt.Errorf("摘要重置失败，请稍后重试或检查模型是否正常")
		}
		newLongSummary = "" // 清空长期摘要，下次 mergeLongSummary 会重新积累
	} else {
		// 原有流程：增量短期摘要 + 每 5 次合并长期摘要
		newShortSummary = summarizeMessages(ctx, client, shortSummary, trimmedMsgs)
		if newShortSummary == "" {
			return nil, fmt.Errorf("摘要生成失败，请稍后重试或检查模型是否正常")
		}
		if shouldMergeLongSummary(compressCount) {
			newLongSummary = mergeLongSummary(ctx, client, longSummary, newShortSummary)
		}
	}

	// 8. 保存分层摘要到 DB（短期每次更新，长期仅在合并时有值）
	if err := store.UpdateConversationLayeredSummary(s.db, convID, newShortSummary, newLongSummary); err != nil {
		return nil, fmt.Errorf("保存摘要失败: %w", err)
	}

	totalTrimmed := len(trimmedMsgs)
	msg := "压缩成功"
	if ShouldResetSummary(compressCount) {
		msg = "压缩成功，已重置摘要（周期性重述）"
	} else if newLongSummary != "" {
		msg = "压缩成功，已合并长期记忆"
	}

	return &CompressResult{
		ShortSummary: newShortSummary,
		LongSummary:  newLongSummary,
		TrimmedCount: totalTrimmed,
		Message:      msg,
	}, nil
}

// 测试导出函数
func StoreMsgToChat(m *store.Message) *Message           { return storeMsgToChat(m) } // Exported for testing
func GetDB(s *Service) *sql.DB                           { return s.db }              // Exported for testing
func SetCurrentCancel(s *Service, fn context.CancelFunc) { s.currentCancel = fn }     // Exported for testing
