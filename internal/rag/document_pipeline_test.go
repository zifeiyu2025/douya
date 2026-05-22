package rag

import (
	"context"
	"fmt"
	"math"
	"testing"
)

// mockEmbedder returns deterministic vectors for testing.
type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	vectors := make([][]float64, len(texts))
	for i, text := range texts {
		vec := make([]float64, m.dim)
		// Generate deterministic vector based on text content
		for j := 0; j < m.dim && j < len(text); j++ {
			vec[j] = float64(text[j]) / 256.0
		}
		// Normalize
		norm := 0.0
		for _, v := range vec {
			norm += v * v
		}
		norm = math.Sqrt(norm)
		if norm > 0 {
			for j := range vec {
				vec[j] /= norm
			}
		}
		vectors[i] = vec
	}
	return vectors, nil
}

func TestChunkDocument_Basic(t *testing.T) {
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 50
	cfg.ChunkOverlap = 10

	text := "Hello world. This is a test document. It has multiple sentences. We want to see how it chunks."

	chunks := ChunkDocument(text, cfg)

	if len(chunks) == 0 {
		t.Fatal("expected at least 1 chunk")
	}

	for i, c := range chunks {
		if len(c.Content) == 0 {
			t.Errorf("chunk %d is empty", i)
		}
		if len(c.Content) > cfg.ChunkSize+10 { // allow some slack
			t.Errorf("chunk %d too large: %d chars", i, len(c.Content))
		}
	}
}

func TestChunkDocument_Empty(t *testing.T) {
	cfg := DefaultChunkConfig()
	chunks := ChunkDocument("", cfg)
	if chunks != nil {
		t.Errorf("expected nil for empty text, got %d chunks", len(chunks))
	}
}

func TestChunkDocument_LongParagraph(t *testing.T) {
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 20
	cfg.ChunkOverlap = 5

	// Generate a long string without separators
	text := "abcdefghij" // 10 chars * 5 = 50 chars
	longText := text + text + text + text + text

	chunks := ChunkDocument(longText, cfg)

	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
}

func TestChunkDocument_ParagraphSeparators(t *testing.T) {
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 40 // small enough to force split

	text := "First paragraph with some text here now.\n\nSecond paragraph with more text here.\n\nThird paragraph here now."

	chunks := ChunkDocument(text, cfg)

	// Should split on paragraph boundaries
	if len(chunks) < 2 {
		t.Errorf("expected at least 2 chunks for paragraphs, got %d", len(chunks))
	}

	// First chunk should contain first paragraph
	if len(chunks) > 0 && !contains(chunks[0].Content, "First paragraph") {
		t.Error("first chunk should contain first paragraph")
	}
}

func TestCleanText(t *testing.T) {
	input := "Hello\r\n\r\n\r\nWorld"
	output := cleanText(input)
	if output != "Hello\n\nWorld" {
		t.Errorf("cleanText failed: got %q", output)
	}
}

func TestIngestDocument_Basic(t *testing.T) {
	vs, cleanup := newTestVectorStore(t)
	defer cleanup()

	embedder := &mockEmbedder{dim: 8}
	text := "This is a test document for ingestion. It contains multiple sentences to test the chunking and embedding pipeline. We want to make sure everything works correctly."

	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 50

	result, err := IngestDocument(context.Background(), vs, embedder, "test_ingest", text, cfg)
	if err != nil {
		t.Fatalf("IngestDocument failed: %v", err)
	}

	if result.TotalChunks == 0 {
		t.Error("expected some chunks")
	}

	if result.StoredChunks != result.TotalChunks {
		t.Errorf("stored %d chunks but produced %d", result.StoredChunks, result.TotalChunks)
	}

	// Verify search works - use embedder to generate a real query vector
	queryVecs, _ := embedder.Embed(context.Background(), []string{"test document"})
	results, err := vs.Search("test_ingest", queryVecs[0], 3)
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("search should return results after ingestion")
	}
}

func TestIngestDocument_EmptyText(t *testing.T) {
	vs, cleanup := newTestVectorStore(t)
	defer cleanup()

	embedder := &mockEmbedder{dim: 8}

	_, err := IngestDocument(context.Background(), vs, embedder, "empty_test", "", DefaultChunkConfig())
	if err == nil {
		t.Error("expected error for empty text")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && indexOf(s, substr) >= 0
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestChunkDocument_PreservesMetadata(t *testing.T) {
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 50

	text := "Some content here."
	chunks := ChunkDocument(text, cfg)

	// Metadata should be nil (not set by ChunkDocument)
	for i, c := range chunks {
		if c.Metadata != nil {
			t.Errorf("chunk %d: expected nil metadata", i)
		}
	}
}

func TestChunkDocument_ChineseText(t *testing.T) {
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 20
	cfg.ChunkOverlap = 5

	text := "这是一段中文测试文本。它包含多个句子。我们想要看看分块效果如何。"

	chunks := ChunkDocument(text, cfg)
	if len(chunks) == 0 {
		t.Fatal("expected chunks for Chinese text")
	}

	for i, c := range chunks {
		if len(c.Content) == 0 {
			t.Errorf("chunk %d is empty", i)
		}
	}
}

func BenchmarkChunkDocument_ShortText(b *testing.B) {
	cfg := DefaultChunkConfig()
	text := "Short text."
	for i := 0; i < b.N; i++ {
		ChunkDocument(text, cfg)
	}
}

func BenchmarkChunkDocument_LongText(b *testing.B) {
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 512
	// Build a long text
	text := fmt.Sprintf("This is sentence number %d. ", 1)
	for i := 2; i <= 1000; i++ {
		text += fmt.Sprintf("This is sentence number %d. ", i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ChunkDocument(text, cfg)
	}
}

// newTestVectorStore creates a VectorStore with a temp directory for testing.
func newTestVectorStore(t *testing.T) (*VectorStore, func()) {
	t.Helper()
	dir := t.TempDir()
	vs, err := NewVectorStore(dir, DefaultHNSWConfig())
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	cleanup := func() {
		vs.Close()
	}
	return vs, cleanup
}
