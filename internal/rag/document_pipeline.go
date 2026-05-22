package rag

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"
)

// Chunk represents a text chunk extracted from a document.
type Chunk struct {
	Content  string // the text content
	Metadata map[string]string // optional metadata (source, page, etc.)
}

// ChunkConfig controls how documents are split into chunks.
type ChunkConfig struct {
	ChunkSize    int  // max characters per chunk (default 512)
	ChunkOverlap int  // overlap characters between chunks (default 64)
	Separators   []string // split priority (default: "\n\n", "\n", ". ", " ")
}

// DefaultChunkConfig returns sensible defaults.
func DefaultChunkConfig() ChunkConfig {
	return ChunkConfig{
		ChunkSize:    512,
		ChunkOverlap: 64,
		Separators:   []string{"\n\n", "\n", ". ", " "},
	}
}

// Embedder is an interface for text embedding.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([][]float64, error)
}

// IngestResult holds the result of ingesting a document.
type IngestResult struct {
	CollectionName string
	TotalChunks    int
	StoredChunks   int
}

// ChunkDocument splits text into chunks based on the config.
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
		cfg.Separators = []string{"\n\n", "\n", ". ", " "}
	}

	if text == "" {
		return nil
	}

	chunks := make([]Chunk, 0)

	// Try splitting by each separator in priority order
	for _, sep := range cfg.Separators {
		parts := strings.Split(text, sep)
		if len(parts) <= 1 {
			continue
		}

		// Build chunks by joining parts up to ChunkSize
		var current strings.Builder
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}

			if current.Len() > 0 && current.Len()+len(sep)+utf8.RuneCountInString(part) > cfg.ChunkSize {
				// Flush current chunk
				chunks = append(chunks, Chunk{Content: current.String()})
				// Keep overlap
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

		// Flush remaining
		if current.Len() > 0 {
			chunks = append(chunks, Chunk{Content: current.String()})
		}

		// If we got reasonable chunks, use them
		if len(chunks) > 0 {
			break
		}
	}

	// Fallback: hard split by character count
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

// IngestDocument ingests a document into the vector store.
// It chunks the text, embeds each chunk, and stores the vectors.
func IngestDocument(ctx context.Context, vs *VectorStore, embedder Embedder, collectionName string, text string, chunkCfg ChunkConfig) (*IngestResult, error) {
	// 1. Chunk the document
	chunks := ChunkDocument(text, chunkCfg)
	if len(chunks) == 0 {
		return nil, fmt.Errorf("no chunks produced from document")
	}

	log.Info().
		Str("collection", collectionName).
		Int("chunks", len(chunks)).
		Msg("[rag] document chunked")

	// 2. Embed all chunks
	texts := make([]string, len(chunks))
	for i, c := range chunks {
		texts[i] = c.Content
	}

	vectors, err := embedder.Embed(ctx, texts)
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}

	if len(vectors) != len(chunks) {
		return nil, fmt.Errorf("embedding count mismatch: got %d vectors for %d chunks", len(vectors), len(chunks))
	}

	// 3. Determine dimension from first vector
	dim := len(vectors[0])

	// 4. Create collection (idempotent: ok if exists)
	err = vs.CreateCollection(collectionName, dim)
	if err != nil {
		// Collection may already exist, that's ok
		if err != ErrCollectionExists {
			return nil, fmt.Errorf("create collection failed: %w", err)
		}
	}

	// 5. Generate IDs and store
	ids := make([]string, len(chunks))
	for i := range chunks {
		ids[i] = fmt.Sprintf("chunk_%06d", i)
	}

	err = vs.AddVectors(collectionName, ids, vectors)
	if err != nil {
		return nil, fmt.Errorf("add vectors failed: %w", err)
	}

	// Store chunk texts so they can be retrieved after search
	for i, id := range ids {
		key := chunkKey(collectionName, id)
		err = vs.db.Update(func(txn *badger.Txn) error {
			return txn.Set(key, []byte(texts[i]))
		})
		if err != nil {
			log.Warn().Err(err).Str("id", id).Msg("[rag] failed to store chunk text")
		}
	}

	result := &IngestResult{
		CollectionName: collectionName,
		TotalChunks:    len(chunks),
		StoredChunks:   len(vectors),
	}

	log.Info().
		Str("collection", collectionName).
		Int("stored", result.StoredChunks).
		Msg("[rag] document ingested")

	return result, nil
}

// cleanText removes excessive whitespace and normalizes line endings.
func cleanText(text string) string {
	// Replace Windows line endings
	text = strings.ReplaceAll(text, "\r\n", "\n")
	// Remove excessive blank lines
	re := regexp.MustCompile(`\n{3,}`)
	text = re.ReplaceAllString(text, "\n\n")
	return strings.TrimSpace(text)
}
