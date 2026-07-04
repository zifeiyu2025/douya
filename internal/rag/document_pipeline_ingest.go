// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package rag

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"
)

// IngestDocumentWithMeta 将文档分块、向量化并写入向量库和文档元数据存储。
//
// 拆分说明：原 197 行函数按流水线阶段拆为调度器 + 6 子函数：
//   - prepareChunksForIngest: 文档分块 + 元数据注入 + 数量校验
//   - generateChunkEmbeddings: 批量向量化
//   - writeChunksTransaction: 单事务批量写入 chunk 文本+元数据
//   - rollbackChunkWrites: 回滚已写入的 chunk（失败恢复）
//   - writeVectorsForWrittenChunks: 只对成功写入的 chunk 写入向量
//   - storeDocumentMetadata: 存储文档级元数据
//
// 生活类比：就像图书入库流程——主厨（本函数）只负责按流程分派任务：
// 切分（prepare）→ 编码（embed）→ 上架（writeChunks）→ 建索引（writeVectors）→ 登记造册（storeMeta），
// 每一步出错都有对应的回滚机制（rollback）。
func IngestDocumentWithMeta(ctx context.Context, vs *VectorStore, ds *DocumentStore, embedder Embedder,
	collectionName string, docID string, text string, fileName string, fileSize int64, mimeType string, chunkCfg ChunkConfig) (*IngestResult, error) {

	// 安全实践（基于 GO-PATH-001 #8）：过滤 fileName 中的控制字符并限制长度
	fileName = sanitizeFileName(fileName)
	if docID == "" {
		docID = fmt.Sprintf("doc_%d", time.Now().UnixNano())
	}

	// 阶段 1：分块 + 元数据注入 + 数量校验
	chunks, err := prepareChunksForIngest(text, fileName, docID, chunkCfg, collectionName)
	if err != nil {
		return nil, err
	}

	// 阶段 2：批量向量化
	allVectors, err := generateChunkEmbeddings(ctx, embedder, chunks, collectionName, docID)
	if err != nil {
		return nil, err
	}

	// 阶段 3：创建集合（已存在则忽略）
	dim := len(allVectors[0])
	if err := vs.CreateCollection(collectionName, dim); err != nil && !errors.Is(err, ErrCollectionExists) {
		return nil, fmt.Errorf("create collection failed: %w", err)
	}

	// 生成 chunk ID
	ids := make([]string, len(chunks))
	for i := range chunks {
		ids[i] = fmt.Sprintf("%s_%06d", docID, i)
	}

	// 任务 13：整个文档摄入纳入单次 collectionLock 保护，避免与并发 DeleteDocument
	// 产生孤立数据（向量已写但 chunk 文本未写，或反之）。
	mu := vs.collectionLock(collectionName)
	mu.Lock()
	defer mu.Unlock()

	// 阶段 4：单事务批量写入 chunk 文本+元数据
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}
	writtenIDs, err := writeChunksTransaction(vs, collectionName, ids, chunks, texts)
	if err != nil {
		return nil, err
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
		rollbackChunkWrites(vs, collectionName, writtenIDs)
		return nil, fmt.Errorf("chunk write failure ratio %.2f exceeds threshold %.2f (%d/%d failed)",
			float64(failedCount)/float64(len(ids)), chunkWriteFailureThreshold, failedCount, len(ids))
	}

	// 阶段 5：只对成功写入 chunk 文本的 id 写入向量
	if err := writeVectorsForWrittenChunks(vs, collectionName, writtenIDs, ids, allVectors); err != nil {
		// 向量写入失败，回滚已写的 chunk 文本+元数据，避免孤立数据
		log.Warn().Err(err).Msg("[rag] addVectorsCore failed, rolling back chunk writes")
		rollbackChunkWrites(vs, collectionName, writtenIDs)
		return nil, fmt.Errorf("add vectors failed: %w", err)
	}

	// 阶段 6：存储文档级元数据
	storeDocumentMetadata(ds, collectionName, docID, fileName, fileSize, mimeType, len(chunks))

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

// prepareChunksForIngest 负责文档分块、元数据注入和数量校验。
// 返回带元数据的 chunks，供后续向量化使用。
func prepareChunksForIngest(text string, fileName string, docID string, chunkCfg ChunkConfig, collectionName string) ([]Chunk, error) {
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

	// 为每个 chunk 注入元数据（source/doc_id/chunk_idx）
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

	return chunks, nil
}

// generateChunkEmbeddings 将 chunks 批量向量化。
// 使用 embedBatchSize 分批调用 embedder，避免单次请求过大。
func generateChunkEmbeddings(ctx context.Context, embedder Embedder, chunks []Chunk, collectionName string, docID string) ([][]float64, error) {
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	var allVectors [][]float64
	for i := 0; i < len(texts); i += embedBatchSize {
		end := min(i+embedBatchSize, len(texts))
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

	return allVectors, nil
}

// writeChunksTransaction 在单次 badger 事务中批量写入 chunk 文本和元数据。
// 返回成功写入的 id 列表（writtenIDs），失败的 id 会被跳过并记录日志。
//
// 任务 1：先写 chunk 文本+元数据（单事务批量写入，解决 N+1 事务问题），
// 再调用 addVectorsCore。这样 addVectorsCore 内部的 BM25 更新能通过 chunkKey
// 正确读取已写入的 chunk 文本，修复了原来顺序依赖导致的 BM25 索引为空的问题。
func writeChunksTransaction(vs *VectorStore, collectionName string, ids []string, chunks []Chunk, texts []string) ([]string, error) {
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
	return writtenIDs, nil
}

// rollbackChunkWrites 回滚已写入的 chunk 文本和元数据。
// 用于 chunk 写入失败比例超阈值或向量写入失败时的恢复。
func rollbackChunkWrites(vs *VectorStore, collectionName string, writtenIDs []string) {
	_ = vs.db.Update(func(txn *badger.Txn) error {
		for _, id := range writtenIDs {
			_ = txn.Delete(chunkKey(collectionName, id))
			_ = txn.Delete(chunkMetaKey(collectionName, id))
		}
		return nil
	})
}

// writeVectorsForWrittenChunks 只对成功写入 chunk 文本的 id 写入向量，保持向量与文本的一致性。
// 调用 addVectorsCore（不加锁版本），外层已持锁。
func writeVectorsForWrittenChunks(vs *VectorStore, collectionName string, writtenIDs []string, ids []string, allVectors [][]float64) error {
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
	return vs.addVectorsCore(collectionName, writtenIDs, writtenVectors)
}

// storeDocumentMetadata 存储文档级元数据（文件名、大小、MIME、chunk 数等）。
// ds 为 nil 时跳过（测试场景可能不传 DocumentStore）。
func storeDocumentMetadata(ds *DocumentStore, collectionName string, docID string, fileName string, fileSize int64, mimeType string, chunkCount int) {
	if ds == nil {
		return
	}
	meta := DocumentMeta{
		ID:         docID,
		Collection: collectionName,
		FileName:   fileName,
		FileSize:   fileSize,
		MimeType:   mimeType,
		ChunkCount: chunkCount,
		IngestedAt: time.Now().Format(time.RFC3339),
	}
	if putErr := ds.Put(meta); putErr != nil {
		log.Warn().Err(putErr).Str("docID", docID).Msg("[rag] failed to store document meta")
	}
}
