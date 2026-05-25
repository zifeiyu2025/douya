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
		Separators:   []string{"\n\n", "\n", "。", ". ", " "},
	}
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
		cfg.Separators = []string{"\n\n", "\n", "。", ". ", " "}
	}

	if text == "" {
		return nil
	}

	chunks := make([]Chunk, 0)

	for _, sep := range cfg.Separators {
		parts := strings.Split(text, sep)
		if len(parts) <= 1 {
			continue
		}

		var current strings.Builder
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			if current.Len() > 0 && current.Len()+len(sep)+utf8.RuneCountInString(part) > cfg.ChunkSize {
				chunks = append(chunks, Chunk{Content: current.String()})
				overlapRunes := []rune(current.String())
				overlapStart := len(overlapRunes) - cfg.ChunkOverlap
				if overlapStart < 0 {
					overlapStart = 0
				}
				current.Reset()
				current.WriteString(string(overlapRunes[overlapStart:]))
			}

			if current.Len() > 0 {
				current.WriteString(sep)
			}
			current.WriteString(part)
		}

		if current.Len() > 0 {
			chunks = append(chunks, Chunk{Content: current.String()})
		}

		if len(chunks) > 0 {
			break
		}
	}

	if len(chunks) == 0 {
		runes := []rune(text)
		for i := 0; i < len(runes); i += cfg.ChunkSize - cfg.ChunkOverlap {
			end := i + cfg.ChunkSize
			if end > len(runes) {
				end = len(runes)
			}
			chunks = append(chunks, Chunk{Content: string(runes[i:end])})
			if end >= len(runes) {
				break
			}
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
