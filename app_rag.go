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

// extToMIME 扩展名到 MIME 类型的映射，用于前端未传 mimeType（或仅传占位值）时兜底推断。
// 生活类比：就像快递单上没写内容物时，根据包裹外形（扩展名）判断里面是什么。
// 所有映射值都应在 allowedDocMIMETypes 中，确保推断后能通过 MIME 白名单校验；
// 代码/配置类文件统一映射为 text/plain，因为更精确的 x- 类型不在白名单中，避免误拒。
var extToMIME = map[string]string{
	".txt":  "text/plain",
	".md":   "text/markdown",
	".csv":  "text/csv",
	".json": "application/json",
	".xml":  "application/xml",
	".html": "text/html",
	".yaml": "application/x-yaml",
	".yml":  "application/x-yaml",
	// 文本类配置/日志文件统一归为 text/plain
	".toml": "text/plain",
	".ini":  "text/plain",
	".cfg":  "text/plain",
	".log":  "text/plain",
	".sql":  "text/plain",
	// 代码文件统一归为 text/plain
	".go":    "text/plain",
	".py":    "text/plain",
	".js":    "text/plain",
	".ts":    "text/plain",
	".java":  "text/plain",
	".c":     "text/plain",
	".cpp":   "text/plain",
	".h":     "text/plain",
	".rs":    "text/plain",
	".sh":    "text/plain",
	".rb":    "text/plain",
	".php":   "text/plain",
	".swift": "text/plain",
	".kt":    "text/plain",
	// 二进制文档
	".pdf":  "application/pdf",
	".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
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

	// MIME 类型校验与兜底推断
	// 生活类比：就像快递员送件时先看包裹标签（mimeType），如果标签缺失或只是"通用包裹"占位，
	// 就根据包装盒外形（扩展名）判断内容物；判断不出来则拒收。
	// 前端在浏览器无法识别文件类型时会传 "application/octet-stream"，同样视为需要推断。
	if mimeType == "" || mimeType == "application/octet-stream" {
		inferred, ok := extToMIME[ext]
		if !ok {
			return fmt.Errorf("无法识别文件类型")
		}
		mimeType = inferred
	}
	if !allowedDocMIMETypes[mimeType] {
		return fmt.Errorf("不支持的 MIME 类型: %s", mimeType)
	}

	// 验证文件大小
	decodedLen := base64.StdEncoding.DecodedLen(len(fileData))
	if decodedLen > maxUploadSize {
		return fmt.Errorf("文件大小超过限制（最大 %d MB）", maxUploadSize/(1024*1024))
	}

	// 安全实践（基于 GO-UPLOAD-001 #7）：对 PDF 增加 magic bytes 内容校验
	// 防止伪造 MIME 类型的文件通过校验后利用 PDF 解析库漏洞。
	// PDF 文件头固定为 "%PDF-"（5 字节），此处只需解码前 8 字节即可判断。
	if ext == ".pdf" {
		// base64 解码前 8 字节（base64 每 4 字符解码为 3 字节，前 8 字符可解码出前 6 字节，足够判断）
		minDecodeLen := 8
		if len(fileData) < minDecodeLen {
			minDecodeLen = len(fileData)
		}
		// base64 编码长度必须是 4 的倍数，截取到最近的 4 倍数边界
		minDecodeLen -= minDecodeLen % 4
		if minDecodeLen > 0 {
			decoded, err := base64.StdEncoding.DecodeString(fileData[:minDecodeLen])
			if err != nil || len(decoded) < 5 || string(decoded[:5]) != "%PDF-" {
				return fmt.Errorf("文件扩展名为 PDF 但内容不是有效的 PDF 文件（magic bytes 不匹配）")
			}
		}
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
	// 采用"复制→修改副本→替换指针"模式，避免直接修改 a.config 字段破坏快照语义
	a.configMu.Lock()
	newCfg := *a.config
	newCfg.RAGActiveKB = kbName
	cfg := &newCfg
	a.config = cfg
	a.configMu.Unlock()
	a.service.SetRAGCollection(kbName)
	// 保存前校验，失败记录日志但不阻塞保存（避免阻塞切换知识库功能）
	if err := cfg.Validate(); err != nil {
		zlog.Warn().Err(err).Msg("[SetActiveKnowledgeBase] 配置校验失败，仍保存")
	}
	return config.Save(filepath.Join(appDir(), "config.json"), cfg)
}

func (a *App) GetActiveKnowledgeBase() string {
	return a.getConfig().RAGActiveKB
}

func (a *App) SetRAGEnabled(enabled bool) {
	// 采用"复制→修改副本→替换指针"模式，避免直接修改 a.config 字段破坏快照语义
	a.configMu.Lock()
	newCfg := *a.config
	newCfg.RAGEnabled = enabled
	// RAG 开启时自动关闭联网搜索（两者互斥，RAG 优先级更高）
	if enabled && newCfg.SearchMode != "off" {
		newCfg.SearchMode = "off"
		// 通知前端搜索已自动关闭
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "search:autoDisabled", nil)
		}
	}
	cfg := &newCfg
	a.config = cfg
	a.configMu.Unlock()
	a.service.SetRAGEnabled(enabled)
	// 保存前校验，失败记录日志但不阻塞保存（避免阻塞 RAG 开关功能）
	if err := cfg.Validate(); err != nil {
		zlog.Warn().Err(err).Msg("[SetRAGEnabled] 配置校验失败，仍保存")
	}
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
