// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"douya/internal/apperror"
	"douya/internal/config"
	"douya/internal/llm"
	"douya/internal/rag"
	"douya/internal/search"
	"douya/internal/secrets"
	"douya/internal/store"
)

type Service struct {
	llmClient   *llm.Client
	searchChain *search.SearchChain
	db          *sql.DB
	config      *config.Config
	// events is the output port for UI-facing stream events. Keeping this as an
	// interface prevents chat use cases from depending on a specific desktop UI
	// framework (Wails today, another host tomorrow).
	events EventPublisher
	// hostCtx is the application lifecycle context. It is deliberately separate
	// from events: command cancellation is a use-case concern, while publishing
	// is an infrastructure concern.
	hostCtx       context.Context
	appDir        string
	currentCancel context.CancelFunc
	currentConvID string
	// genToken 单调递增，标识"当前槽位属于第几代生成"。见 beginGeneration 竞态修复。
	genToken          uint64
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
	// MCP 工具缓存：由 llama-server 通过 GET /tools 端点提供（含内置工具 + MCP 工具）。
	// 懒加载：第一次 buildAvailableTools 时拉取，之后用缓存；
	// 用户改 MCP 配置 + llama-server 重启后可通过 RefreshMcpToolsCache 强制刷新。
	// mcpToolsInitialized 区分"未初始化"和"已初始化但为空"，避免空缓存反复触发拉取。
	// mcpToolsOnce 确保并发 getMcpTools 只触发一次拉取（M2: 防止启动阶段重复拉取）。
	mcpToolsCache       []llm.ToolDefinition
	mcpToolsCacheMu     sync.RWMutex
	mcpToolsInitialized bool
	mcpToolsOnce        sync.Once
	// 上下文压缩累计统计（并发安全）
	compressionStats CompressionStats
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

// SetEventPublisher sets the adapter used to forward stream events outside the
// chat application layer. Passing nil intentionally disables event delivery,
// which is useful for headless callers and tests.
func (s *Service) SetEventPublisher(events EventPublisher) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.events = events
}

// SetHostContext sets the host lifecycle context used as the parent for
// long-running chat operations. It does not imply a particular UI framework.
func (s *Service) SetHostContext(ctx context.Context) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.hostCtx = ctx
}

// getEventPublisherSnapshot returns the current event adapter under a read
// lock, so replacing it during lifecycle changes cannot race with streaming.
func (s *Service) getEventPublisherSnapshot() EventPublisher {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.events
}

// getHostContextSnapshot returns the host lifecycle context, if one has been
// supplied. Callers fall back to context.Background for headless use and tests.
func (s *Service) getHostContextSnapshot() context.Context {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	return s.hostCtx
}

// GetCompressionStats 返回上下文压缩累计统计快照，供日志/前端展示。
// 原子读取，无需调用方加锁。
func (s *Service) GetCompressionStats() CompressionStatsSnapshot {
	return s.compressionStats.snapshot()
}

// beginGeneration 统一处理"开始新一轮生成"的锁与取消逻辑，消除 SendMessage 和 RegenerateMessage 的重复代码。
// 流程：加锁 → 读取旧 cancel/convID → 创建新 cancelCtx → 赋值 currentCancel/currentConvID → 解锁 →
// 取消旧 cancel + emit stopped → 返回 cancelCtx 和 cleanup（defer 调用清空 currentCancel/currentConvID）
// initialConvID: 立即设置的 currentConvID；传 "" 表示延迟设置（RegenerateMessage 场景，由调用方后续赋值）
// 生活类比：调度中心接新单时的标准流程——记录旧单、派发新单号、通知旧单停止、清理台面
func (s *Service) beginGeneration(parentCtx context.Context, initialConvID string) (context.Context, func()) {
	s.mutex.Lock()
	var oldCancel context.CancelFunc
	var oldConvID string
	if s.currentCancel != nil {
		oldCancel = s.currentCancel
		oldConvID = s.currentConvID
	}
	// 为本次生成分配唯一令牌：cleanup 只清空"仍属于本代"的槽位，
	// 避免旧代收尾时误清新一代的 currentCancel/currentConvID（竞态修复）。
	s.genToken++
	token := s.genToken
	cancelCtx, cancel := context.WithCancel(parentCtx)
	s.currentCancel = cancel
	s.currentConvID = initialConvID
	s.mutex.Unlock()

	if oldCancel != nil {
		oldCancel()
		if oldConvID != "" {
			s.emitForConv(oldConvID, "stopped", nil)
		}
	}

	return cancelCtx, func() {
		s.mutex.Lock()
		if s.genToken == token {
			s.currentCancel = nil
			s.currentConvID = ""
		}
		s.mutex.Unlock()
	}
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

// defaultConfigSnapshot 是 config.Config 的零值单例，用于 s.config 为 nil 时的兜底返回。
// 零值 Config 会被各处默认逻辑处理（如 ContextSize=0 → 4096），不会导致 panic。
// 不可变对象，多处共享安全。
var defaultConfigSnapshot = &config.Config{}

// getConfigSnapshot 在读锁保护下获取配置快照，避免数据竞争。
// 升级为 RWMutex 后，多个并发请求读取 config 可并行，不再串行化。
// H1/H2 修复：永不返回 nil，s.config 为 nil 时返回零值单例，避免调用方 nil panic。
// 生活类比：就像在图书馆查阅共享资料时，多人可同时阅读（读锁），只有修改时才独占（写锁）。
// 即使资料架上暂时没有书（s.config 为 nil），也会给一本空白手册（零值单例），让大家不至于空手发呆。
func (s *Service) getConfigSnapshot() *config.Config {
	s.mutex.RLock()
	defer s.mutex.RUnlock()
	if s.config == nil {
		return defaultConfigSnapshot
	}
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

func (s *Service) emit(eventType string, content any) {
	if events := s.getEventPublisherSnapshot(); events != nil {
		events.Publish(StreamEvent{
			Type:    eventType,
			Content: content,
		})
	}
}

func (s *Service) emitForConv(convID string, eventType string, content any) {
	if events := s.getEventPublisherSnapshot(); events != nil {
		events.Publish(StreamEvent{
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
		return nil, apperror.New(apperror.KindInvalidInput, "会话 ID 不能为空")
	}

	// 1. 加载会话所有 DB 消息
	dbMsgs, err := store.GetMessagesByConversation(s.db, convID, secrets.CipherKey(s.cipher))
	if err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "加载消息失败", err)
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
		return nil, apperror.New(apperror.KindUnavailable, "LLM 客户端未初始化，请等待模型加载完成")
	}

	// 6. 同步生成新短期摘要
	// 超时设为 summaryTimeoutSec*2+10s，与 CompressContext 一致，留出短期+长期两次 LLM 调用时间
	// M15 修复：用 hostCtx 作为父 context，应用关闭时能立即取消，避免阻塞退出最长 70+ 秒
	// 生活类比：加班任务要挂在公司总机下，公司下班时能一起切断，而不是独立电话线永远占线
	parentCtx := s.getHostContextSnapshot()
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	ctx, cancel := context.WithTimeout(parentCtx, time.Duration(summaryTimeoutSec*2+10)*time.Second)
	defer cancel()

	// P3-C2: 判断是否触发周期性重置（与 CompressContext 保持一致）
	// 重置优先级高于合并，触发时跳过 shouldMergeLongSummary
	var newShortSummary, newLongSummary string
	if ShouldResetSummary(compressCount) {
		// 重置模式：从当前所有被裁剪消息重新生成摘要，丢弃旧摘要
		newShortSummary = resetSummary(ctx, client, trimmedMsgs)
		if newShortSummary == "" {
			return nil, apperror.New(apperror.KindInternal, "摘要重置失败，请稍后重试或检查模型是否正常")
		}
		newLongSummary = "" // 清空长期摘要，下次 mergeLongSummary 会重新积累
	} else {
		// 原有流程：增量短期摘要 + 每 5 次合并长期摘要
		newShortSummary = summarizeMessages(ctx, client, shortSummary, trimmedMsgs)
		if newShortSummary == "" {
			return nil, apperror.New(apperror.KindInternal, "摘要生成失败，请稍后重试或检查模型是否正常")
		}
		if shouldMergeLongSummary(compressCount) {
			newLongSummary = mergeLongSummary(ctx, client, longSummary, newShortSummary)
		}
	}

	// 8. 保存分层摘要到 DB（短期每次更新，长期仅在合并时有值）
	if err := store.UpdateConversationLayeredSummary(s.db, convID, newShortSummary, newLongSummary); err != nil {
		return nil, apperror.Wrap(apperror.KindInternal, "保存摘要失败", err)
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
