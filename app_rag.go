package main

import (
	"encoding/base64"
	"fmt"
	"path/filepath"
	"strings"

	"douya/internal/config"
	"douya/internal/rag"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func (a *App) ListKnowledgeBases() ([]rag.CollectionInfo, error) {
	if a.ragVS == nil {
		return nil, fmt.Errorf("知识库未初始化")
	}
	return a.ragVS.ListCollections()
}

func (a *App) CreateKnowledgeBase(name string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	if name == "" {
		return fmt.Errorf("知识库名称不能为空")
	}
	return a.ragVS.CreateCollection(name, 0)
}

func (a *App) DeleteKnowledgeBase(name string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	if name == "default" {
		return fmt.Errorf("不能删除默认知识库")
	}
	return a.ragVS.DeleteCollection(name)
}

// 上传文档允许的文件扩展名
var allowedDocExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true,
	".xml": true, ".html": true, ".yaml": true, ".yml": true,
	".toml": true, ".ini": true, ".cfg": true, ".log": true,
	".sql": true,
	".go":  true, ".py": true, ".js": true, ".ts": true,
	".java": true, ".c": true, ".cpp": true, ".h": true,
	".rs": true, ".sh": true, ".rb": true, ".php": true,
	".swift": true, ".kt": true,
	".pdf": true, ".docx": true,
}

// 上传文档允许的 MIME 类型
var allowedDocMIMETypes = map[string]bool{
	"text/plain": true, "text/markdown": true, "text/csv": true,
	"application/json": true, "application/xml": true, "text/xml": true,
	"text/html": true, "text/yaml": true, "application/x-yaml": true,
	"application/pdf": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document": true,
}

const maxUploadSize = 200 * 1024 * 1024 // 200MB

func (a *App) UploadDocument(kbName string, fileName string, fileData string, mimeType string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	if !a.serverReady.Load() {
		return fmt.Errorf("AI 服务未启动，无法生成嵌入向量")
	}

	// 验证文件扩展名
	ext := strings.ToLower(filepath.Ext(fileName))
	if !allowedDocExts[ext] {
		return fmt.Errorf("不支持的文件类型: %s", ext)
	}

	// 验证 MIME 类型（如果前端提供了）
	if mimeType != "" && !allowedDocMIMETypes[mimeType] {
		return fmt.Errorf("不支持的 MIME 类型: %s", mimeType)
	}

	// 验证文件大小
	decodedLen := base64.StdEncoding.DecodedLen(len(fileData))
	if decodedLen > maxUploadSize {
		return fmt.Errorf("文件大小超过限制（最大 %d MB）", maxUploadSize/(1024*1024))
	}

	embedder := a.ragEmbedder
	if embedder == nil {
		return fmt.Errorf("知识库未初始化")
	}
	cfg := a.getConfig()
	chunkCfg := rag.ChunkConfig{
		ChunkSize:    cfg.RAGChunkSize,
		ChunkOverlap: cfg.RAGChunkOverlap,
	}
	if chunkCfg.ChunkSize <= 0 {
		chunkCfg.ChunkSize = 512
	}
	if chunkCfg.ChunkOverlap <= 0 {
		chunkCfg.ChunkOverlap = 64
	}
	_, err := rag.IngestFileFromBase64(a.ctx, a.ragVS, a.ragDS, embedder, kbName, fileName, fileData, mimeType, chunkCfg)
	if err != nil {
		return fmt.Errorf("上传文档失败: %w", err)
	}
	return nil
}

func (a *App) ListDocuments(kbName string) ([]rag.DocumentMeta, error) {
	if a.ragDS == nil {
		return nil, fmt.Errorf("知识库未初始化")
	}
	return a.ragDS.List(kbName)
}

func (a *App) DeleteDocument(kbName string, docID string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	if a.ragDS != nil {
		if err := a.ragDS.Delete(kbName, docID); err != nil {
			zlog.Error().Err(err).Msg("[rag] delete document meta failed")
		}
	}
	return a.ragVS.DeleteDocument(kbName, docID)
}

func (a *App) SetActiveKnowledgeBase(kbName string) error {
	if a.ragVS == nil {
		return fmt.Errorf("知识库未初始化")
	}
	a.configMu.Lock()
	a.config.RAGActiveKB = kbName
	cfg := a.config
	a.configMu.Unlock()
	a.service.SetRAGCollection(kbName)
	return config.Save(filepath.Join(appDir(), "config.json"), cfg)
}

func (a *App) GetActiveKnowledgeBase() string {
	return a.getConfig().RAGActiveKB
}

func (a *App) SetRAGEnabled(enabled bool) {
	a.configMu.Lock()
	a.config.RAGEnabled = enabled
	// RAG 开启时自动关闭联网搜索（两者互斥，RAG 优先级更高）
	if enabled && a.config.SearchMode != "off" {
		a.config.SearchMode = "off"
		// 通知前端搜索已自动关闭
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "search:autoDisabled", nil)
		}
	}
	cfg := a.config
	a.configMu.Unlock()
	a.service.SetRAGEnabled(enabled)
	if err := config.Save(filepath.Join(appDir(), "config.json"), cfg); err != nil {
		zlog.Error().Err(err).Msg("[rag] save config failed")
	}
}

func (a *App) IsRAGEnabled() bool {
	return a.getConfig().RAGEnabled
}

// RerankEnabled 返回是否配置了 reranker 模型（用于前端判断是否启用重排序功能）。
func (a *App) RerankEnabled() bool {
	cfg := a.getConfig()
	return cfg.RerankerModelPath != ""
}
