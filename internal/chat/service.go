// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"douya/internal/config"
	"douya/internal/llm"
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
	mutex             sync.Mutex
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
	lastPromptTokens   int // 最近一次实际 prompt_tokens（来自 llama-server usage）
	lastEstimatedTokens int // 对应的估算值
	tokenCalibMu       sync.RWMutex
	// 当前流式聊天的 completion ID，用于 /v1/chat/completions/control 实时控制
	currentCompletionID string
	completionIDMu      sync.RWMutex
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

func (s *Service) CurrentConvID() string {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.currentConvID
}

// IsGenerating 返回当前是否正在生成（currentConvID 非空表示正在生成）。
// 用于让 router 模式轮询在生成期间暂停，避免与生成请求争用 HTTP 连接。
func (s *Service) IsGenerating() bool {
	s.mutex.Lock()
	defer s.mutex.Unlock()
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

// getConfigSnapshot 在锁保护下获取配置快照，避免数据竞争。
// 生活类比：就像在图书馆查阅共享资料时，先借出（加锁）再阅读，避免别人同时修改。
func (s *Service) getConfigSnapshot() *config.Config {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.config
}

// getClientSnapshot 在锁保护下获取 LLM 客户端快照，避免数据竞争。
func (s *Service) getClientSnapshot() *llm.Client {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.llmClient
}

// getSearchChainSnapshot 在锁保护下获取搜索链快照，避免数据竞争。
func (s *Service) getSearchChainSnapshot() *search.SearchChain {
	s.mutex.Lock()
	defer s.mutex.Unlock()
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

// 测试导出函数
func StoreMsgToChat(m *store.Message) *Message { return storeMsgToChat(m) }    // Exported for testing
func GetDB(s *Service) *sql.DB                           { return s.db }                     // Exported for testing
func SetCurrentCancel(s *Service, fn context.CancelFunc) { s.currentCancel = fn }            // Exported for testing
