package rag

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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

	err = vs.AddVectors(collectionName, ids, allVectors)
	if err != nil {
		return nil, fmt.Errorf("add vectors failed: %w", err)
	}

	for i, id := range ids {
		key := chunkKey(collectionName, id)
		err = vs.db.Update(func(txn *badger.Txn) error {
			return txn.Set(key, []byte(texts[i]))
		})
		if err != nil {
			log.Warn().Err(err).Str("id", id).Msg("[rag] failed to store chunk text")
		}
		if len(chunks[i].Metadata) > 0 {
			metaData, jsonErr := json.Marshal(chunks[i].Metadata)
			if jsonErr == nil {
				metaKey := chunkMetaKey(collectionName, id)
				metaErr := vs.db.Update(func(txn *badger.Txn) error {
					return txn.Set(metaKey, metaData)
				})
				if metaErr != nil {
					log.Warn().Err(metaErr).Str("id", id).Msg("[rag] failed to store chunk meta")
				}
			}
		}
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
		StoredChunks:   len(allVectors),
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
	re := regexp.MustCompile(`\n{3,}`)
	text = re.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
