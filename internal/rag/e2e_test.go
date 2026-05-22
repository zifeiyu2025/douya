package rag

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// mockEmbedder is defined in document_pipeline_test.go — reuse it here.

// TestEndToEndRAG tests the full pipeline: create store → ingest → search → verify results.
func TestEndToEndRAG(t *testing.T) {
	// Use temp dir for Badger
	tmpDir := t.TempDir()
	vs, err := NewVectorStore(tmpDir, DefaultHNSWConfig())
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	defer vs.Close()

	embedder := &mockEmbedder{dim: 16}
	collection := "test-e2e"
	ctx := context.Background()

	// Step 1: Ingest a document
	text := "Go语言是由Google开发的静态强类型编译型语言。它具有垃圾回收、并发编程和简洁语法等特点。Go语言广泛应用于云原生、微服务和DevOps领域。"
	chunkCfg := ChunkConfig{
		ChunkSize:    50,
		ChunkOverlap: 10,
	}

	result, err := IngestDocument(ctx, vs, embedder, collection, text, chunkCfg)
	if err != nil {
		t.Fatalf("IngestDocument: %v", err)
	}
	if result.TotalChunks == 0 {
		t.Fatal("IngestDocument produced 0 chunks")
	}
	if result.StoredChunks != result.TotalChunks {
		t.Fatalf("StoredChunks=%d != TotalChunks=%d", result.StoredChunks, result.TotalChunks)
	}
	t.Logf("Ingested: %d chunks into collection %q", result.StoredChunks, collection)

	// Step 2: Search for relevant chunks
	query := "Go语言的特点"
	queryVecs, err := embedder.Embed(ctx, []string{query})
	if err != nil {
		t.Fatalf("Embed query: %v", err)
	}

	results, err := vs.Search(collection, queryVecs[0], 3)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search returned 0 results")
	}
	t.Logf("Search returned %d results", len(results))

	// Step 3: Verify chunk content is populated
	for i, r := range results {
		if r.ChunkContent == "" {
			t.Errorf("result[%d].ChunkContent is empty", i)
		} else {
			t.Logf("result[%d]: score=%.4f content=%q", i, r.Score, truncate(r.ChunkContent, 60))
		}
	}

	// Step 4: Ingest another document and search again
	text2 := "Python是一种解释型高级编程语言，由Guido van Rossum创建。Python支持多种编程范式，包括面向对象、命令式和函数式编程。"
	result2, err := IngestDocument(ctx, vs, embedder, collection, text2, chunkCfg)
	if err != nil {
		t.Fatalf("IngestDocument 2: %v", err)
	}
	t.Logf("Ingested 2nd doc: %d chunks", result2.StoredChunks)

	results2, err := vs.Search(collection, queryVecs[0], 5)
	if err != nil {
		t.Fatalf("Search 2: %v", err)
	}
	if len(results2) < len(results) {
		t.Errorf("after ingesting more docs, got %d results (expected >= %d)", len(results2), len(results))
	}
	t.Logf("After 2nd ingest, search returned %d results", len(results2))
}

// TestEndToEndRAG_Persistence tests that data persists after closing and reopening the store.
func TestEndToEndRAG_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	embedder := &mockEmbedder{dim: 16}
	collection := "test-persist"
	ctx := context.Background()

	// Phase 1: Write
	vs1, err := NewVectorStore(tmpDir, DefaultHNSWConfig())
	if err != nil {
		t.Fatalf("NewVectorStore 1: %v", err)
	}

	text := "Docker是一个开源的应用容器引擎，让开发者可以打包应用及依赖到一个可移植的容器中。"
	result, err := IngestDocument(ctx, vs1, embedder, collection, text, ChunkConfig{ChunkSize: 100, ChunkOverlap: 20})
	if err != nil {
		vs1.Close()
		t.Fatalf("IngestDocument: %v", err)
	}
	t.Logf("Phase 1: ingested %d chunks", result.StoredChunks)
	vs1.Close()

	// Phase 2: Reopen and search
	vs2, err := NewVectorStore(tmpDir, DefaultHNSWConfig())
	if err != nil {
		t.Fatalf("NewVectorStore 2: %v", err)
	}
	defer vs2.Close()

	queryVecs, err := embedder.Embed(ctx, []string{"容器技术"})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}

	results, err := vs2.Search(collection, queryVecs[0], 3)
	if err != nil {
		t.Fatalf("Search after reopen: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search after reopen returned 0 results — data did not persist!")
	}
	t.Logf("Phase 2: search found %d results after reopen", len(results))

	for i, r := range results {
		if r.ChunkContent == "" {
			t.Errorf("result[%d].ChunkContent is empty after persistence", i)
		} else {
			t.Logf("result[%d]: score=%.4f content=%q", i, r.Score, truncate(r.ChunkContent, 60))
		}
	}
}

// TestClientEmbedder_VerifyInterface verifies ClientEmbedder satisfies the Embedder interface.
func TestClientEmbedder_VerifyInterface(t *testing.T) {
	var _ Embedder = (*ClientEmbedder)(nil)
}

// TestEndToEndRAG_EmbedderAdapter tests that the adapter compiles and the interface is correct.
func TestEndToEndRAG_EmbedderAdapter(t *testing.T) {
	// We can't call the real LLM API in unit tests, so just verify the type system works.
	embedder := &ClientEmbedder{Client: nil, Model: "test-model"}
	if embedder.Model != "test-model" {
		t.Errorf("expected model 'test-model', got %q", embedder.Model)
	}
	// Embed should return error when client is nil
	_, err := embedder.Embed(context.Background(), []string{"test"})
	if err == nil {
		t.Error("expected error when client is nil")
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// TestEndToEndRAG_DataDir tests that the data directory is created automatically.
func TestEndToEndRAG_DataDir(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "nested", "rag", "data")
	vs, err := NewVectorStore(tmpDir, DefaultHNSWConfig())
	if err != nil {
		t.Fatalf("NewVectorStore with nested dir: %v", err)
	}
	defer vs.Close()

	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Errorf("data directory %q was not created", tmpDir)
	}
}
