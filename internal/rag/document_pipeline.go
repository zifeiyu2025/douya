package rag

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"
)

type Chunk struct {
	Content  string
	Metadata map[string]string
}

type ChunkConfig struct {
	ChunkSize    int
	ChunkOverlap int
	Separators   []string
}

func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		ChunkSize:    512,
		ChunkOverlap: 64,
		Separators:   []string{"\n\n", "\n", "。", ".", "！", "？", "；", ";", " "},
	}
}

// estimateTokens 粗略估算文本的 token 数
// 中文约 1 字 ≈ 1.5 token，英文约 1 词 ≈ 1.3 token
// 简化估算：rune 数 × 0.7 作为 token 近似值
func estimateTokens(text string) int {
	return int(float64(utf8.RuneCountInString(text)) * 0.7)
}

type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

type IngestResult struct {
	CollectionName string
	DocumentID     string
	TotalChunks    int
	StoredChunks   int
}

const embedBatchSize = 32

// maxChunksPerDocument 限制单文档切分后的最大 chunk 数，避免因超大文档
// 一次性产生过多 chunk 导致内存暴涨或写入耗时过长（任务 33）。
const maxChunksPerDocument = 10000

// chunkWriteFailureThreshold 是 chunk 写入失败比例的容忍上限。
// 超过该阈值时 IngestDocumentWithMeta 会回滚已写入的数据并返回错误（任务 5）。
const chunkWriteFailureThreshold = 0.1

// chunkWriteErrorHook 仅供测试使用：若非 nil，则在每个 chunk 写入前调用，
// 返回 error 时该 chunk 视为写入失败。生产环境为 nil，零开销。
var chunkWriteErrorHook func(collection, id string) error

// ChunkDocument 使用递归字符分块策略将文本拆分为块。
// 策略：按分隔符优先级递归切分 —— 先用高级分隔符(段落)切分，
// 超过 chunkSize 的块再用低级分隔符(句子)继续切分，依此类推。
// 每个块保留 chunkOverlap 的重叠以保证上下文连贯。
func ChunkDocument(text string, cfg ChunkConfig) []Chunk {
	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 512
	}
	if cfg.ChunkOverlap < 0 {
		cfg.ChunkOverlap = 64
	}
	if cfg.ChunkOverlap >= cfg.ChunkSize {
		cfg.ChunkOverlap = cfg.ChunkSize / 4
	}
	if len(cfg.Separators) == 0 {
		cfg.Separators = []string{"\n\n", "\n", "。", ".", "！", "？", "；", ";", " "}
	}

	if text == "" {
		return nil
	}

	text = cleanText(text)
	return recursiveChunk(text, cfg.Separators, cfg.ChunkSize, cfg.ChunkOverlap)
}

// recursiveChunk 递归字符分块核心逻辑
func recursiveChunk(text string, separators []string, chunkSize, chunkOverlap int) []Chunk {
	// 没有更多分隔符可用时，按字符硬切
	if len(separators) == 0 {
		return hardSplit(text, chunkSize, chunkOverlap)
	}

	sep := separators[0]
	remainingSeps := separators[1:]
	parts := strings.Split(text, sep)

	var chunks []Chunk
	var current strings.Builder

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 当前块加入此部分后是否超过大小限制
		combinedLen := estimateTokens(current.String()) + estimateTokens(part)
		if current.Len() > 0 && combinedLen > chunkSize {
			// 当前块已满，先保存
			chunks = append(chunks, Chunk{Content: current.String()})

			// 保留重叠部分
			overlapText := takeOverlap(current.String(), chunkOverlap)
			current.Reset()
			current.WriteString(overlapText)
		}

		// 检查单个部分是否就超过大小限制
		if estimateTokens(part) > chunkSize && len(remainingSeps) > 0 {
			// 先把当前积累的内容保存
			if current.Len() > 0 {
				chunks = append(chunks, Chunk{Content: current.String()})
				current.Reset()
			}
			// 递归用下一级分隔符切分
			subChunks := recursiveChunk(part, remainingSeps, chunkSize, chunkOverlap)
			chunks = append(chunks, subChunks...)
			continue
		}

		if current.Len() > 0 {
			current.WriteString(sep)
		}
		current.WriteString(part)
	}

	if current.Len() > 0 {
		chunks = append(chunks, Chunk{Content: current.String()})
	}

	return chunks
}

// takeOverlap 从文本末尾取约 overlapTokens 个 token 的重叠文本
func takeOverlap(text string, overlapTokens int) string {
	runes := []rune(text)
	// overlapTokens 对应的字符数约 overlapTokens / 0.7
	overlapChars := int(float64(overlapTokens) / 0.7)
	if overlapChars >= len(runes) {
		return text
	}
	start := len(runes) - overlapChars
	return string(runes[start:])
}

// hardSplit 按字符硬切（最后兜底）
func hardSplit(text string, chunkSize, chunkOverlap int) []Chunk {
	runes := []rune(text)
	// chunkSize token 对应的字符数
	charSize := int(float64(chunkSize) / 0.7)
	charOverlap := int(float64(chunkOverlap) / 0.7)
	if charOverlap >= charSize {
		charOverlap = charSize / 4
	}

	var chunks []Chunk
	for i := 0; i < len(runes); i += charSize - charOverlap {
		end := i + charSize
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, Chunk{Content: string(runes[i:end])})
		if end >= len(runes) {
			break
		}
	}
	return chunks
}

func IngestDocument(ctx context.Context, vs *VectorStore, embedder Embedder, collectionName string, text string, chunkCfg ChunkConfig) (*IngestResult, error) {
	return IngestDocumentWithMeta(ctx, vs, nil, embedder, collectionName, "", text, "", 0, "", chunkCfg)
}

func IngestDocumentWithMeta(ctx context.Context, vs *VectorStore, ds *DocumentStore, embedder Embedder,
	collectionName string, docID string, text string, fileName string, fileSize int64, mimeType string, chunkCfg ChunkConfig) (*IngestResult, error) {

	if docID == "" {
		docID = fmt.Sprintf("doc_%d", time.Now().UnixNano())
	}

	chunks := ChunkDocument(text, chunkCfg)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks produced from document")
	}

	// 任务 33：单文档 chunk 数量上限保护，避免超大文档导致内存暴涨或写入耗时过长
	if len(chunks) > maxChunksPerDocument {
		log.Warn().
			Str("collection", collectionName).
			Str("docID", docID).
			Int("chunks", len(chunks)).
			Int("limit", maxChunksPerDocument).
			Msg("[rag] document exceeds max chunks limit, rejecting ingestion")
		return nil, fmt.Errorf("document produced %d chunks, exceeds max limit %d", len(chunks), maxChunksPerDocument)
	}

	for i := range chunks {
		if chunks[i].Metadata == nil {
			chunks[i].Metadata = make(map[string]string)
		}
		chunks[i].Metadata["source"] = fileName
		chunks[i].Metadata["doc_id"] = docID
		chunks[i].Metadata["chunk_idx"] = fmt.Sprintf("%d", i)
	}

	log.Info().
		Str("collection", collectionName).
		Str("docID", docID).
		Int("chunks", len(chunks)).
		Msg("[rag] document chunked")

	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	var allVectors [][]float64
	for i := 0; i < len(texts); i += embedBatchSize {
		end := i + embedBatchSize
		if end > len(texts) {
			end = len(texts)
		}
		batch := texts[i:end]
		batchVecs, err := embedder.Embed(ctx, batch)
		if err != nil {
			return nil, fmt.Errorf("embedding batch %d-%d failed: %w", i, end, err)
		}
		allVectors = append(allVectors, batchVecs...)
	}

	if len(allVectors) != len(chunks) {
		return nil, fmt.Errorf("embedding count mismatch: got %d vectors for %d chunks", len(allVectors), len(chunks))
	}

	dim := len(allVectors[0])

	err := vs.CreateCollection(collectionName, dim)
	if err != nil {
		if !errors.Is(err, ErrCollectionExists) {
			return nil, fmt.Errorf("create collection failed: %w", err)
		}
	}

	ids := make([]string, len(chunks))
	for i := range chunks {
		ids[i] = fmt.Sprintf("%s_%06d", docID, i)
	}

	// 任务 13：整个文档摄入纳入单次 collectionLock 保护，避免与并发 DeleteDocument
	// 产生孤立数据（向量已写但 chunk 文本未写，或反之）。
	// 锁内调用 addVectorsCore（不加锁版本）避免重复加锁导致死锁。
	mu := vs.collectionLock(collectionName)
	mu.Lock()
	defer mu.Unlock()

	// 任务 1：先写 chunk 文本+元数据（单事务批量写入，解决 N+1 事务问题），
	// 再调用 addVectorsCore。这样 addVectorsCore 内部的 BM25 更新能通过 chunkKey
	// 正确读取已写入的 chunk 文本，修复了原来顺序依赖导致的 BM25 索引为空的问题。
	writtenIDs := make([]string, 0, len(ids))
	writeErr := vs.db.Update(func(txn *badger.Txn) error {
		for i, id := range ids {
			// 任务 5：测试钩子，用于注入 chunk 写入失败场景
			if chunkWriteErrorHook != nil {
				if hookErr := chunkWriteErrorHook(collectionName, id); hookErr != nil {
					log.Warn().Err(hookErr).Str("id", id).Msg("[rag] chunk write skipped by hook")
					continue
				}
			}
			key := chunkKey(collectionName, id)
			if setErr := txn.Set(key, []byte(texts[i])); setErr != nil {
				log.Warn().Err(setErr).Str("id", id).Msg("[rag] failed to store chunk text")
				continue
			}
			if len(chunks[i].Metadata) > 0 {
				metaData, jsonErr := json.Marshal(chunks[i].Metadata)
				if jsonErr == nil {
					metaKey := chunkMetaKey(collectionName, id)
					if metaSetErr := txn.Set(metaKey, metaData); metaSetErr != nil {
						log.Warn().Err(metaSetErr).Str("id", id).Msg("[rag] failed to store chunk meta")
					}
				}
			}
			writtenIDs = append(writtenIDs, id)
		}
		return nil
	})
	if writeErr != nil {
		// 单事务整体失败（非逐条 continue），直接返回错误，无需回滚（事务原子失败）
		return nil, fmt.Errorf("batch write chunks failed: %w", writeErr)
	}

	// 任务 5：检查 chunk 写入失败比例，超阈值则回滚已写入数据并返回错误
	failedCount := len(ids) - len(writtenIDs)
	if len(ids) > 0 && float64(failedCount)/float64(len(ids)) > chunkWriteFailureThreshold {
		log.Warn().
			Str("collection", collectionName).
			Str("docID", docID).
			Int("total", len(ids)).
			Int("failed", failedCount).
			Float64("ratio", float64(failedCount)/float64(len(ids))).
			Msg("[rag] chunk write failure ratio exceeds threshold, rolling back")
		_ = vs.db.Update(func(txn *badger.Txn) error {
			for _, id := range writtenIDs {
				_ = txn.Delete(chunkKey(collectionName, id))
				_ = txn.Delete(chunkMetaKey(collectionName, id))
			}
			return nil
		})
		return nil, fmt.Errorf("chunk write failure ratio %.2f exceeds threshold %.2f (%d/%d failed)",
			float64(failedCount)/float64(len(ids)), chunkWriteFailureThreshold, failedCount, len(ids))
	}

	// 只对成功写入 chunk 文本的 id 写入向量，保持向量与文本的一致性
	vecByID := make(map[string][]float64, len(ids))
	for i, id := range ids {
		vecByID[id] = allVectors[i]
	}
	writtenVectors := make([][]float64, len(writtenIDs))
	for i, id := range writtenIDs {
		writtenVectors[i] = vecByID[id]
	}

	// 任务 1：chunk 文本已写入，现在调用 addVectorsCore（不加锁版本，外层已持锁）
	// addVectorsCore 内部的 BM25 更新会通过 chunkKey 读取已写入的 chunk 文本，顺序正确。
	err = vs.addVectorsCore(collectionName, writtenIDs, writtenVectors)
	if err != nil {
		// 向量写入失败，回滚已写的 chunk 文本+元数据，避免孤立数据
		log.Warn().Err(err).Msg("[rag] addVectorsCore failed, rolling back chunk writes")
		_ = vs.db.Update(func(txn *badger.Txn) error {
			for _, id := range writtenIDs {
				_ = txn.Delete(chunkKey(collectionName, id))
				_ = txn.Delete(chunkMetaKey(collectionName, id))
			}
			return nil
		})
		return nil, fmt.Errorf("add vectors failed: %w", err)
	}

	if ds != nil {
		meta := DocumentMeta{
			ID:         docID,
			Collection: collectionName,
			FileName:   fileName,
			FileSize:   fileSize,
			MimeType:   mimeType,
			ChunkCount: len(chunks),
			IngestedAt: time.Now().Format(time.RFC3339),
		}
		if putErr := ds.Put(meta); putErr != nil {
			log.Warn().Err(putErr).Str("docID", docID).Msg("[rag] failed to store document meta")
		}
	}

	result := &IngestResult{
		CollectionName: collectionName,
		DocumentID:     docID,
		TotalChunks:    len(chunks),
		StoredChunks:   len(writtenIDs),
	}

	log.Info().
		Str("collection", collectionName).
		Str("docID", docID).
		Int("stored", result.StoredChunks).
		Msg("[rag] document ingested")

	return result, nil
}

func IngestFile(ctx context.Context, vs *VectorStore, ds *DocumentStore, embedder Embedder,
	collectionName string, fileName string, fileData []byte, mimeType string, chunkCfg ChunkConfig) (*IngestResult, error) {

	text, err := ParseFileFromBytes(fileData, fileName)
	if err != nil {
		return nil, fmt.Errorf("parse file %q: %w", fileName, err)
	}
	if text == "" {
		return nil, fmt.Errorf("file %q produced no text content", fileName)
	}

	return IngestDocumentWithMeta(ctx, vs, ds, embedder, collectionName, "", text, fileName, int64(len(fileData)), mimeType, chunkCfg)
}

func IngestFileFromBase64(ctx context.Context, vs *VectorStore, ds *DocumentStore, embedder Embedder,
	collectionName string, fileName string, base64Data string, mimeType string, chunkCfg ChunkConfig) (*IngestResult, error) {

	data, err := base64.StdEncoding.DecodeString(base64Data)
	if err != nil {
		return nil, fmt.Errorf("decode base64 for %q: %w", fileName, err)
	}

	return IngestFile(ctx, vs, ds, embedder, collectionName, fileName, data, mimeType, chunkCfg)
}

func cleanText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = multiNewlineRe.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
