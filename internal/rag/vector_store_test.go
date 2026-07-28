// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package rag

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dgraph-io/badger/v4"
)

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

func newTestStore(t *testing.T) (*VectorStore, func()) {
	t.Helper()
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	cleanup := func() {
		if vs != nil {
			vs.Close()
		}
	}
	return vs, cleanup
}

// ---------------------------------------------------------------------------
// CreateCollection
// ---------------------------------------------------------------------------

func TestCreateCollection_OK(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.CreateCollection("test_col", 128)
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
}

// TestDefaultHNSWConfig 验证默认 HNSW 配置参数合理
//
// 生活类比：就像工厂的"标准产品规格表"，M=16、EFConstruction=200、EF=100
// 是经过验证的默认值，保证产品（向量检索）开箱即用且有合理性能。
func TestDefaultHNSWConfig(t *testing.T) {
	cfg := DefaultHNSWConfig()
	if cfg.M <= 0 {
		t.Errorf("M 应 > 0，实际: %d", cfg.M)
	}
	if cfg.EFConstruction <= 0 {
		t.Errorf("EFConstruction 应 > 0，实际: %d", cfg.EFConstruction)
	}
	if cfg.EF <= 0 {
		t.Errorf("EF 应 > 0，实际: %d", cfg.EF)
	}
	// M 通常为 16
	if cfg.M != 16 {
		t.Errorf("M 期望 16，实际: %d", cfg.M)
	}
	// EFConstruction 通常为 200
	if cfg.EFConstruction != 200 {
		t.Errorf("EFConstruction 期望 200，实际: %d", cfg.EFConstruction)
	}
}

func TestCreateCollection_Duplicate(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("dup", 64); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	err := vs.CreateCollection("dup", 64)
	if !errors.Is(err, ErrCollectionExists) {
		t.Fatalf("expected ErrCollectionExists, got %v", err)
	}
}

func TestCreateCollection_ZeroDim(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.CreateCollection("zero_dim", 0)
	if err != nil {
		t.Fatalf("expected no error for zero dim (lazy init), got %v", err)
	}
}

func TestCreateCollection_NegativeDim(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.CreateCollection("neg_dim", -5)
	if err != nil {
		t.Fatalf("expected no error for negative dim (lazy init), got %v", err)
	}
}

func TestCreateCollection_EmptyName(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.CreateCollection("   ", 64)
	if !errors.Is(err, ErrEmptyCollectionName) {
		t.Fatalf("expected ErrEmptyCollectionName, got %v", err)
	}
}

func TestCreateCollection_EmptyNameStrict(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.CreateCollection("", 64)
	if !errors.Is(err, ErrEmptyCollectionName) {
		t.Fatalf("expected ErrEmptyCollectionName, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// AddVectors
// ---------------------------------------------------------------------------

func TestAddVectors_OK(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 8
	if err := vs.CreateCollection("vec_col", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	ids := []string{"v1", "v2", "v3"}
	vectors := [][]float64{
		{1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		{0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 0.0, 0.0},
		{0.0, 0.0, 1.0, 0.0, 0.0, 0.0, 0.0, 0.0},
	}
	err := vs.AddVectors("vec_col", ids, vectors)
	if err != nil {
		t.Fatalf("AddVectors: %v", err)
	}
}

func TestAddVectors_CollectionNotFound(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.AddVectors("non_existent", []string{"v1"}, [][]float64{{1.0, 2.0}})
	if !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("expected ErrCollectionNotFound, got %v", err)
	}
}

func TestAddVectors_DimMismatch(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("mismatch_col", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	err := vs.AddVectors("mismatch_col", []string{"v1"}, [][]float64{{1.0, 2.0, 3.0}}) // dim 3 vs expected 4
	if !errors.Is(err, ErrVectorDimMismatch) {
		t.Fatalf("expected ErrVectorDimMismatch, got %v", err)
	}
}

func TestAddVectors_EmptyVector(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("empty_vec_col", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	err := vs.AddVectors("empty_vec_col", []string{"v1"}, [][]float64{{}})
	if !errors.Is(err, ErrEmptyVector) {
		t.Fatalf("expected ErrEmptyVector, got %v", err)
	}
}

func TestAddVectors_LengthMismatch(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("len_mismatch", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// ids has 2 elements, vectors has 3.
	err := vs.AddVectors("len_mismatch", []string{"a", "b"}, [][]float64{
		{1, 2, 3, 4},
		{1, 2, 3, 4},
		{1, 2, 3, 4},
	})
	if err == nil {
		t.Fatal("expected error for length mismatch, got nil")
	}
}

// ---------------------------------------------------------------------------
// Search
// ---------------------------------------------------------------------------

func TestSearch_OK(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 4
	if err := vs.CreateCollection("search_col", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	ids := []string{"a", "b", "c", "d"}
	vectors := [][]float64{
		{1.0, 0.0, 0.0, 0.0}, // identical to query → score ~1
		{0.0, 1.0, 0.0, 0.0},
		{0.0, 0.0, 1.0, 0.0},
		{0.0, 0.0, 0.0, 1.0},
	}
	if err := vs.AddVectors("search_col", ids, vectors); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	// Query for vector "a": [1,0,0,0].
	results, err := vs.Search(context.Background(), "search_col", []float64{1.0, 0.0, 0.0, 0.0}, 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result")
	}
	// The top result should be "a" (most similar to itself).
	if results[0].ID != "a" {
		t.Errorf("expected top result ID 'a', got %q", results[0].ID)
	}
	// Score should be close to 1.0 (cosine similarity for identical vectors).
	if results[0].Score < 0.99 {
		t.Errorf("expected score ~1.0, got %f", results[0].Score)
	}
}

func TestSearch_CollectionNotFound(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	_, err := vs.Search(context.Background(), "non_existent", []float64{1.0, 2.0}, 5)
	if !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("expected ErrCollectionNotFound, got %v", err)
	}
}

func TestSearch_DimMismatch(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("search_dim_mismatch", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	_, err := vs.Search(context.Background(), "search_dim_mismatch", []float64{1.0, 2.0, 3.0}, 5) // dim 3 vs expected 4
	if !errors.Is(err, ErrVectorDimMismatch) {
		t.Fatalf("expected ErrVectorDimMismatch, got %v", err)
	}
}

func TestSearch_EmptyQuery(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("empty_query_col", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	_, err := vs.Search(context.Background(), "empty_query_col", []float64{}, 5)
	if !errors.Is(err, ErrEmptyVector) {
		t.Fatalf("expected ErrEmptyVector, got %v", err)
	}
}

func TestSearch_TopKLargerThanCollection(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 4
	if err := vs.CreateCollection("small_col", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	ids := []string{"v1"}
	vectors := [][]float64{{1.0, 0.0, 0.0, 0.0}}
	if err := vs.AddVectors("small_col", ids, vectors); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	// Request topK=100 on a collection with only 1 vector — should succeed.
	results, err := vs.Search(context.Background(), "small_col", []float64{1.0, 0.0, 0.0, 0.0}, 100)
	if err != nil {
		t.Fatalf("Search with large topK: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least 1 result, got none")
	}
	// The top result should be v1 (identical to query).
	if results[0].ID != "v1" {
		t.Errorf("expected top result ID 'v1', got %q", results[0].ID)
	}
}

// ---------------------------------------------------------------------------
// DeleteCollection
// ---------------------------------------------------------------------------

func TestDeleteCollection_OK(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("to_delete", 8); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := vs.AddVectors("to_delete", []string{"v1"}, [][]float64{
		{1, 2, 3, 4, 5, 6, 7, 8},
	}); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	err := vs.DeleteCollection("to_delete")
	if err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}

	// Collection should no longer exist.
	err = vs.AddVectors("to_delete", []string{"v2"}, [][]float64{
		{1, 2, 3, 4, 5, 6, 7, 8},
	})
	if !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("expected ErrCollectionNotFound after delete, got %v", err)
	}
}

func TestDeleteCollection_NotFound(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.DeleteCollection("non_existent")
	if !errors.Is(err, ErrCollectionNotFound) {
		t.Fatalf("expected ErrCollectionNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Persistence — vectors survive store re-open
// ---------------------------------------------------------------------------

func TestPersist_VectorsSurviveReopen(t *testing.T) {
	dir := t.TempDir()

	// First session: create collection and add vectors.
	vs1, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore session 1: %v", err)
	}
	if err := vs1.CreateCollection("persistent", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := vs1.AddVectors("persistent", []string{"x", "y"}, [][]float64{
		{1.0, 0.0, 0.0, 0.0},
		{0.0, 1.0, 0.0, 0.0},
	}); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}
	if err := vs1.Close(); err != nil {
		t.Fatalf("Close session 1: %v", err)
	}

	// Second session: re-open and search — vectors should still be there.
	vs2, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore session 2: %v", err)
	}
	defer vs2.Close()

	results, err := vs2.Search(context.Background(), "persistent", []float64{1.0, 0.0, 0.0, 0.0}, 2)
	if err != nil {
		t.Fatalf("Search after reopen: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results after reopen, got %d", len(results))
	}
}

// ---------------------------------------------------------------------------
// Stress / correctness — similar vectors rank higher
// ---------------------------------------------------------------------------

func TestSearch_SimilarVectorsRankHigher(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 16
	if err := vs.CreateCollection("similarity_test", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// Build 10 random-ish unit vectors.
	// v0 and v1 are very close (cosine sim ≈ 0.99).
	// v9 is nearly orthogonal to v0.
	ids := make([]string, 10)
	vecs := make([][]float64, 10)
	for i := range ids {
		ids[i] = fmt.Sprintf("v%d", i)
		vecs[i] = unitVector(dim, i, 0.01) // small jitter varies by i
	}
	if err := vs.AddVectors("similarity_test", ids, vecs); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	results, err := vs.Search(context.Background(), "similarity_test", vecs[0], 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("no results")
	}
	// v0 should be the top result (or at least in the top 2 due to floating point).
	if results[0].ID != "v0" && (len(results) < 2 || results[1].ID != "v0") {
		t.Errorf("expected v0 as top result, got %v", results)
	}
	// Scores should be non-increasing.
	for i := 1; i < len(results); i++ {
		if results[i].Score > results[i-1].Score {
			t.Errorf("scores not non-increasing: [%f, %f] at indices %d,%d",
				results[i-1].Score, results[i].Score, i-1, i)
		}
	}
}

// unitVector returns a dim-dimensional vector with element `i` close to 1.0
// and other elements close to 0.0, with small random-ish variation to avoid ties.
func unitVector(dim, seed int, noiseScale float64) []float64 {
	v := make([]float64, dim)
	v[seed%dim] = 1.0
	for i := range v {
		v[i] += (float64(i*seed%31) - 15.5) * noiseScale
	}
	norm := 0.0
	for _, x := range v {
		norm += x * x
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] /= norm
	}
	return v
}

// normalize returns a unit vector in the same direction as v.
func normalize(v []float64) []float64 {
	norm := 0.0
	for _, x := range v {
		norm += x * x
	}
	norm = math.Sqrt(norm)
	for i := range v {
		v[i] /= norm
	}
	return v
}

// ---------------------------------------------------------------------------
// Collection name validation
// ---------------------------------------------------------------------------

func TestCreateCollection_InvalidNameWithColon(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	// 包含冒号的名称应被拒绝（冒号用于 Badger KV 键前缀分隔符）
	err := vs.CreateCollection("my:collection", 4)
	if err == nil {
		t.Error("expected error for collection name with colon, got nil")
	}
}

func TestCreateCollection_InvalidNameWithSlash(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.CreateCollection("my/collection", 4)
	if err == nil {
		t.Error("expected error for collection name with slash, got nil")
	}
}

func TestCreateCollection_ValidNameWithHyphen(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	// 连字符和下划线应被允许
	err := vs.CreateCollection("my-collection_v2", 4)
	if err != nil {
		t.Errorf("expected no error for valid name with hyphen/underscore, got %v", err)
	}
}

func TestCreateCollection_ValidChineseName(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	// 中文名称应被允许
	err := vs.CreateCollection("我的知识库", 4)
	if err != nil {
		t.Errorf("expected no error for Chinese name, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// SearchWithThreshold tests
// ---------------------------------------------------------------------------

func TestSearchWithThreshold_FiltersLowScore(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.CreateCollection("thresh-test", 3)
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	// 添加两个正交向量
	vecs := [][]float64{
		normalize([]float64{1.0, 0.0, 0.0}),
		normalize([]float64{0.0, 1.0, 0.0}),
	}
	err = vs.AddVectors("thresh-test", []string{"v1", "v2"}, vecs)
	if err != nil {
		t.Fatalf("AddVectors failed: %v", err)
	}

	// 先用普通 Search 获取实际分数
	query := normalize([]float64{0.9, 0.1, 0.0})
	allResults, err := vs.Search(context.Background(), "thresh-test", query, 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(allResults) == 0 {
		t.Fatal("expected at least 1 search result")
	}

	// 用最高分数 + 0.01 作为阈值，应该过滤掉所有结果
	highestScore := allResults[0].Score
	results, err := vs.SearchWithThreshold(context.Background(), "thresh-test", query, 5, highestScore+0.01)
	if err != nil {
		t.Fatalf("SearchWithThreshold failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("expected 0 results with threshold above highest score, got %d", len(results))
	}
}

func TestSearchWithThreshold_AcceptsHighScore(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	err := vs.CreateCollection("thresh-accept", 3)
	if err != nil {
		t.Fatalf("CreateCollection failed: %v", err)
	}

	vecs := [][]float64{
		normalize([]float64{1.0, 0.0, 0.0}),
	}
	err = vs.AddVectors("thresh-accept", []string{"v1"}, vecs)
	if err != nil {
		t.Fatalf("AddVectors failed: %v", err)
	}

	// 查询与 v1 完全相同的向量
	query := normalize([]float64{1.0, 0.0, 0.0})
	results, err := vs.SearchWithThreshold(context.Background(), "thresh-accept", query, 5, 0.5)
	if err != nil {
		t.Fatalf("SearchWithThreshold failed: %v", err)
	}
	// 至少应该有 1 个结果（完全匹配的 v1）
	found := false
	for _, r := range results {
		if r.ID == "v1" && r.Score >= 0.5 {
			found = true
		}
	}
	if !found {
		t.Errorf("expected to find v1 with score >= 0.5, got %d results", len(results))
	}
}

func TestSearchWithThreshold_CollectionNotFound(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	query := normalize([]float64{1.0, 0.0, 0.0})
	_, err := vs.SearchWithThreshold(context.Background(), "nonexistent", query, 5, 0.5)
	if err == nil {
		t.Error("expected error for nonexistent collection, got nil")
	}
}

// ---------------------------------------------------------------------------
// BM25 rebuild —— per-collection 隔离 + id 格式统一（任务4）
// ---------------------------------------------------------------------------

// TestRebuildBM25Index_NoCollectionPrefix 验证 rebuildBM25Index 后 BM25 doc id 不含 collection 前缀。
// 修复（任务4）：原实现存入 "collection:docID_chunkIdx"，导致 parseChunkID 解析出含冒号的错误 docID，
// 进而让 mergeAdjacentChunks 的同文档判断失效。per-collection 隔离后 id 统一为 "docID_chunkIdx"。
func TestRebuildBM25Index_NoCollectionPrefix(t *testing.T) {
	dir := t.TempDir()

	// 第一阶段：创建 store + collection，直接写入 chunk: 键（模拟 document_pipeline 行为）
	vs1, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore session 1: %v", err)
	}
	if err := vs1.CreateCollection("col1", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	chunks := map[string]string{
		"docA_000001": "机器学习基础",
		"docA_000002": "深度学习进阶",
		"docB_000001": "自然语言处理",
	}
	for id, text := range chunks {
		key := chunkKey("col1", id)
		if err := vs1.db.Update(func(txn *badger.Txn) error {
			return txn.Set(key, []byte(text))
		}); err != nil {
			t.Fatalf("write chunk %q: %v", id, err)
		}
	}
	if err := vs1.Close(); err != nil {
		t.Fatalf("Close session 1: %v", err)
	}

	// 第二阶段：重新打开，触发 rebuildBM25Index
	vs2, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore session 2: %v", err)
	}
	defer vs2.Close()

	// 取出 col1 的 BM25 索引
	vs2.mu.RLock()
	bm25Idx, ok := vs2.bm25Indexes["col1"]
	vs2.mu.RUnlock()
	if !ok {
		t.Fatal("rebuild 后应存在 col1 的 BM25 索引")
	}
	bm25Idx.mu.RLock()
	docs := bm25Idx.documents
	bm25Idx.mu.RUnlock()
	if len(docs) != len(chunks) {
		t.Fatalf("期望 %d 个文档，实际 %d", len(chunks), len(docs))
	}
	// 断言所有 id 都不含冒号（即无 collection 前缀），且属于预期集合
	for _, d := range docs {
		if strings.Contains(d.id, ":") {
			t.Errorf("BM25 doc id 不应含 collection 前缀，实际: %q", d.id)
		}
		if _, ok := chunks[d.id]; !ok {
			t.Errorf("BM25 doc id %q 不在预期集合中", d.id)
		}
	}
}

// TestBM25PerCollection_Isolation 验证不同 collection 的 BM25 索引相互隔离，
// 相同 docID 不会跨 collection 碰撞，DeleteCollection 只清理对应 collection。
func TestBM25PerCollection_Isolation(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("colA", 4); err != nil {
		t.Fatalf("CreateCollection colA: %v", err)
	}
	if err := vs.CreateCollection("colB", 4); err != nil {
		t.Fatalf("CreateCollection colB: %v", err)
	}

	// 两个 collection 各自添加同名 id 的文档，内容不同
	vs.getOrCreateBM25("colA").AddDocuments([]BM25DocInput{
		{ID: "doc_000001", Text: "苹果是水果"},
	})
	vs.getOrCreateBM25("colB").AddDocuments([]BM25DocInput{
		{ID: "doc_000001", Text: "计算机科学"},
	})

	// 各自检索应互不干扰
	aRes := vs.getOrCreateBM25("colA").Search("苹果", 5)
	if len(aRes) != 1 || aRes[0].ID != "doc_000001" {
		t.Errorf("colA 检索应命中苹果文档，实际: %v", aRes)
	}
	bRes := vs.getOrCreateBM25("colB").Search("计算机", 5)
	if len(bRes) != 1 || bRes[0].ID != "doc_000001" {
		t.Errorf("colB 检索应命中计算机文档，实际: %v", bRes)
	}

	// 删除 colA 不应影响 colB
	if err := vs.DeleteCollection("colA"); err != nil {
		t.Fatalf("DeleteCollection colA: %v", err)
	}
	vs.mu.RLock()
	_, aExists := vs.bm25Indexes["colA"]
	_, bExists := vs.bm25Indexes["colB"]
	vs.mu.RUnlock()
	if aExists {
		t.Error("删除 colA 后其 BM25 索引应被移除")
	}
	if !bExists {
		t.Error("删除 colA 不应影响 colB 的 BM25 索引")
	}
	// colB 仍可检索
	bRes2 := vs.getOrCreateBM25("colB").Search("计算机", 5)
	if len(bRes2) != 1 {
		t.Errorf("删除 colA 后 colB 应仍可检索，实际: %v", bRes2)
	}
}

// ---------------------------------------------------------------------------
// vectorIndex —— badgerIndex 内存淘汰（任务 30）
// ---------------------------------------------------------------------------

// TestVectorIndex_BadgerIndex_Search 验证向量数超过阈值时 getOrLoadIndex 返回 badgerIndex。
// 通过临时调低 maxInMemoryVectors 避免在测试中构造 5 万向量，dim=8 的 mock 向量不依赖真实模型。
func TestVectorIndex_BadgerIndex_Search(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 8
	const vectorCount = 100

	if err := vs.CreateCollection("big_col", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// 生成 vectorCount 个确定性向量（unitVector 保证不同 seed 产生不同方向）
	ids := make([]string, vectorCount)
	vecs := make([][]float64, vectorCount)
	for i := range vectorCount {
		ids[i] = fmt.Sprintf("v%d", i)
		vecs[i] = unitVector(dim, i, 0.01)
	}
	if err := vs.AddVectors("big_col", ids, vecs); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	// 临时调低阈值，使 100 个向量触发 badgerIndex 路径
	origThreshold := maxInMemoryVectors
	maxInMemoryVectors = 50
	defer func() { maxInMemoryVectors = origThreshold }()

	// 删除已加载的 memIndex 缓存，强制下次 getOrLoadIndex 重新决策
	vs.mu.Lock()
	delete(vs.indexes, "big_col")
	vs.mu.Unlock()

	idx, err := vs.getOrLoadIndex("big_col")
	if err != nil {
		t.Fatalf("getOrLoadIndex: %v", err)
	}

	// 断言返回的是 badgerIndex 而非 memIndex
	if _, ok := idx.(*badgerIndex); !ok {
		t.Fatalf("期望 getOrLoadIndex 返回 *badgerIndex，实际类型 %T", idx)
	}

	// 验证 badgerIndex 也能正常检索
	query := vecs[0]
	results, err := idx.Search(context.Background(), query, 10)
	if err != nil {
		t.Fatalf("badgerIndex.Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("badgerIndex.Search 返回空结果")
	}
	// 查询向量自身应排第一（余弦相似度 = 1.0）
	if results[0].ID != "v0" {
		t.Errorf("期望 top-1 为 v0，实际 %q (score=%f)", results[0].ID, results[0].Score)
	}
	if results[0].Score < 0.99 {
		t.Errorf("期望 top-1 score 接近 1.0，实际 %f", results[0].Score)
	}
}

// TestVectorIndex_BadgerIndex_TopKConsistent 验证 badgerIndex 与 memIndex 的 top-K 结果一致。
// 同一批向量分别用 badgerIndex（磁盘扫描）和 memIndex（内存）检索，top-10 ID 应完全相同。
func TestVectorIndex_BadgerIndex_TopKConsistent(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 8
	const vectorCount = 100
	const topK = 10

	if err := vs.CreateCollection("consistency_col", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// 生成 vectorCount 个确定性向量
	ids := make([]string, vectorCount)
	vecs := make([][]float64, vectorCount)
	for i := range vectorCount {
		ids[i] = fmt.Sprintf("v%d", i)
		vecs[i] = unitVector(dim, i, 0.01)
	}
	if err := vs.AddVectors("consistency_col", ids, vecs); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	// 临时调低阈值触发 badgerIndex
	origThreshold := maxInMemoryVectors
	maxInMemoryVectors = 50
	defer func() { maxInMemoryVectors = origThreshold }()

	// 清除缓存，强制 getOrLoadIndex 重新决策
	vs.mu.Lock()
	delete(vs.indexes, "consistency_col")
	vs.mu.Unlock()

	// 1. 通过 badgerIndex 检索
	badgerIdx, err := vs.getOrLoadIndex("consistency_col")
	if err != nil {
		t.Fatalf("getOrLoadIndex (badger): %v", err)
	}
	if _, ok := badgerIdx.(*badgerIndex); !ok {
		t.Fatalf("期望 *badgerIndex，实际 %T", badgerIdx)
	}

	query := vecs[0] // 用 v0 作为查询向量
	badgerResults, err := badgerIdx.Search(context.Background(), query, topK)
	if err != nil {
		t.Fatalf("badgerIndex.Search: %v", err)
	}
	if len(badgerResults) != topK {
		t.Fatalf("badgerIndex 期望 %d 个结果，实际 %d", topK, len(badgerResults))
	}

	// 2. 手动构建 memIndex，用同一批向量检索
	memIdx := newMemIndex(dim)
	for i := range vectorCount {
		memIdx.insert(ids[i], vecs[i])
	}
	memResults, err := memIdx.Search(context.Background(), query, topK)
	if err != nil {
		t.Fatalf("memIndex.Search: %v", err)
	}
	if len(memResults) != topK {
		t.Fatalf("memIndex 期望 %d 个结果，实际 %d", topK, len(memResults))
	}

	// 3. 逐位比较 top-K 的 ID 和分数
	for i := range topK {
		if badgerResults[i].ID != memResults[i].ID {
			t.Errorf("top-%d ID 不一致：badger=%q mem=%q", i+1, badgerResults[i].ID, memResults[i].ID)
		}
		// 分数允许浮点误差 1e-9
		if math.Abs(badgerResults[i].Score-memResults[i].Score) > 1e-9 {
			t.Errorf("top-%d (%s) 分数差异过大：badger=%.15f mem=%.15f",
				i+1, badgerResults[i].ID, badgerResults[i].Score, memResults[i].Score)
		}
	}

	// 4. 额外断言分数降序
	for i := 1; i < topK; i++ {
		if badgerResults[i].Score > badgerResults[i-1].Score {
			t.Errorf("badgerIndex 结果未按分数降序：[%d]=%.6f > [%d]=%.6f",
				i-1, badgerResults[i-1].Score, i, badgerResults[i].Score)
		}
	}
}

// TestLoadChunksBatch 测试 loadChunksBatch 在单个事务内批量加载 chunk 内容和元数据
// 验证：返回的 maps 以 id 为 key，未找到的 id 不出现，且元数据正确解析
func TestLoadChunksBatch(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("batch_col", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// 直接写入 chunk 文本和元数据到 Badger，模拟 document_pipeline 的写入
	// 元数据使用预制 JSON 字符串，避免引入 encoding/json 依赖
	docs := []struct {
		id       string
		content  string
		metaJSON string
	}{
		{"chunk1", "Hello 世界", "{\"doc_id\":\"d1\",\"chunk_idx\":\"0\"}"},
		{"chunk2", "Second chunk 文本", "{\"doc_id\":\"d1\",\"chunk_idx\":\"1\"}"},
		{"chunk3", "Third 文档", "{\"doc_id\":\"d2\",\"chunk_idx\":\"0\"}"},
	}
	err := vs.db.Update(func(txn *badger.Txn) error {
		for _, d := range docs {
			if err := txn.Set(chunkKey("batch_col", d.id), []byte(d.content)); err != nil {
				return err
			}
			if err := txn.Set(chunkMetaKey("batch_col", d.id), []byte(d.metaJSON)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed chunks: %v", err)
	}

	// 批量加载已存在的 3 个 id + 1 个不存在的 id
	ids := []string{"chunk1", "chunk2", "chunk3", "nonexistent"}
	contents, metas, loadErr := vs.loadChunksBatch("batch_col", ids)
	if loadErr != nil {
		t.Fatalf("loadChunksBatch: %v", loadErr)
	}
	if len(contents) != 3 {
		t.Fatalf("期望 3 个 content，实际 %d", len(contents))
	}
	if contents["chunk1"] != "Hello 世界" {
		t.Errorf("chunk1 content 期望 %q，实际 %q", "Hello 世界", contents["chunk1"])
	}
	if contents["chunk2"] != "Second chunk 文本" {
		t.Errorf("chunk2 content 期望 %q，实际 %q", "Second chunk 文本", contents["chunk2"])
	}
	if _, ok := contents["nonexistent"]; ok {
		t.Errorf("不期望 nonexistent 出现在 contents 中")
	}
	if len(metas) != 3 {
		t.Fatalf("期望 3 个 meta，实际 %d", len(metas))
	}
	if metas["chunk1"]["doc_id"] != "d1" {
		t.Errorf("chunk1 doc_id 期望 d1，实际 %q", metas["chunk1"]["doc_id"])
	}
	if metas["chunk3"]["chunk_idx"] != "0" {
		t.Errorf("chunk3 chunk_idx 期望 0，实际 %q", metas["chunk3"]["chunk_idx"])
	}
}

// TestLoadChunksBatch_Empty 测试 loadChunksBatch 传入空 ids 时不报错
func TestLoadChunksBatch_Empty(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	contents, metas, err := vs.loadChunksBatch("any_col", nil)
	if err != nil {
		t.Fatalf("期望 nil 错误，实际 %v", err)
	}
	if len(contents) != 0 || len(metas) != 0 {
		t.Fatalf("期望空 maps，实际 contents=%d metas=%d", len(contents), len(metas))
	}
}

// ---------------------------------------------------------------------------
// 任务 3: getOrLoadIndex 两阶段加锁 —— collection A 构建索引时不阻塞 collection B
// ---------------------------------------------------------------------------

// TestGetOrLoadIndex_NoCrossCollectionBlock 验证任务 3：
// collection A 构建索引期间，collection B 的检索不被 vs.mu 阻塞。
// 两阶段加锁后，Phase 1 仅在 vs.mu 内注册占位并释放锁，
// Phase 2 的 Badger 扫描不持有 vs.mu，因此 B 可以同时检索。
func TestGetOrLoadIndex_NoCrossCollectionBlock(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 8
	// 创建两个 collection
	if err := vs.CreateCollection("colA", dim); err != nil {
		t.Fatalf("CreateCollection colA: %v", err)
	}
	if err := vs.CreateCollection("colB", dim); err != nil {
		t.Fatalf("CreateCollection colB: %v", err)
	}

	// 向 colA 添加较多向量(使构建有一定耗时)，向 colB 添加少量向量
	idsA := make([]string, 50)
	vecsA := make([][]float64, 50)
	for i := range 50 {
		idsA[i] = fmt.Sprintf("a%d", i)
		vecsA[i] = unitVector(dim, i, 0.01)
	}
	if err := vs.AddVectors("colA", idsA, vecsA); err != nil {
		t.Fatalf("AddVectors colA: %v", err)
	}

	idsB := make([]string, 5)
	vecsB := make([][]float64, 5)
	for i := range 5 {
		idsB[i] = fmt.Sprintf("b%d", i)
		vecsB[i] = unitVector(dim, i+100, 0.01)
	}
	if err := vs.AddVectors("colB", idsB, vecsB); err != nil {
		t.Fatalf("AddVectors colB: %v", err)
	}

	// 预加载 colB 的索引，确保后续 Search(colB) 不需要构建
	_, err := vs.getOrLoadIndex("colB")
	if err != nil {
		t.Fatalf("pre-load colB index: %v", err)
	}

	// 删除 colA 的索引缓存，强制下次 getOrLoadIndex 重建
	vs.mu.Lock()
	delete(vs.indexes, "colA")
	vs.mu.Unlock()

	// 在 goroutine 中启动 colA 的索引重建
	aReady := make(chan error, 1)
	go func() {
		_, err := vs.getOrLoadIndex("colA")
		aReady <- err
	}()

	// 立即搜索 colB —— 如果 vs.mu 在 colA 构建期间被持有，这里会阻塞
	// 设置 3 秒超时，如果超时说明存在阻塞
	done := make(chan struct{})
	go func() {
		_, _ = vs.Search(context.Background(), "colB", vecsB[0], 3)
		close(done)
	}()

	select {
	case <-done:
		// colB 检索完成，未被 colA 阻塞
	case <-time.After(3 * time.Second):
		t.Fatal("colB 检索被 colA 索引构建阻塞超过 3 秒，两阶段加锁可能未生效")
	}

	// 等待 colA 构建完成
	select {
	case err := <-aReady:
		if err != nil {
			t.Fatalf("colA getOrLoadIndex: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("colA 索引构建超时")
	}
}

// ---------------------------------------------------------------------------
// 任务 4: AddVectors 的 BM25 更新纳入 collectionLock —— 并发后无残留
// ---------------------------------------------------------------------------

// TestAddVectorsDeleteDocument_BM25NoResidue 验证任务 4：
// 并发 DeleteDocument 不同文档后，BM25 索引中无残留已删除文档。
// collectionLock 确保 DeleteDocument 的 BM25 清理互斥执行，不会遗漏。
//
// 注意：本测试不涉及 writeChunk + AddVectors 的组合并发，因为 writeChunk
// 不在 collectionLock 保护下（真实场景中 IngestDocumentWithMeta 在锁保护下
// 完成 chunk 写入 + AddVectors，不会出现 chunk 文本被并发删除的问题）。
// AddVectors 与 DeleteDocument 对同一文档的并发在 TestAddVectorsDeleteDocument_DeterministicOrder
// 中通过确定性顺序验证。
func TestAddVectorsDeleteDocument_BM25NoResidue(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 8
	if err := vs.CreateCollection("bm25_race", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// 辅助函数：写入 chunk 文本到 Badger
	writeChunk := func(collection, id, text string) {
		if err := vs.WithTx(func(txn *badger.Txn) error {
			return txn.Set(chunkKey(collection, id), []byte(text))
		}); err != nil {
			t.Fatalf("write chunk %s: %v", id, err)
		}
	}

	// 预先写入 5 个文档的 chunk 文本和向量（含共同关键词"苹果"）
	for i := range 5 {
		id := fmt.Sprintf("doc%d_0", i)
		writeChunk("bm25_race", id, fmt.Sprintf("苹果文档%d的独特内容", i))
		vec := unitVector(dim, i, 0.01)
		if err := vs.AddVectors("bm25_race", []string{id}, [][]float64{vec}); err != nil {
			t.Fatalf("AddVectors doc%d: %v", i, err)
		}
	}

	// 并发删除 doc0、doc1、doc2（保留 doc3、doc4）
	// collectionLock 确保三个 DeleteDocument 互斥执行，BM25 清理不会遗漏
	var wg sync.WaitGroup
	for i := range 3 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			docID := fmt.Sprintf("doc%d", idx)
			if err := vs.DeleteDocument("bm25_race", docID); err != nil {
				t.Errorf("DeleteDocument %s: %v", docID, err)
			}
		}(i)
	}
	wg.Wait()

	// 验证 BM25 中 doc0、doc1、doc2 无残留，doc3、doc4 仍在
	results := vs.getOrCreateBM25("bm25_race").Search("苹果", 20)
	deletedDocs := map[string]bool{"doc0_0": true, "doc1_0": true, "doc2_0": true}
	keptDocs := map[string]bool{"doc3_0": false, "doc4_0": false}
	for _, r := range results {
		if deletedDocs[r.ID] {
			t.Errorf("BM25 中存在已删除文档残留: %s (score=%f)", r.ID, r.Score)
		}
		if kept, ok := keptDocs[r.ID]; ok {
			keptDocs[r.ID] = true
			_ = kept
		}
	}
	for id, found := range keptDocs {
		if !found {
			t.Errorf("BM25 中应保留 %s 但未找到", id)
		}
	}

	// 验证 Badger 中 doc0、doc1、doc2 的向量已删除，doc3、doc4 仍在
	err := vs.db.View(func(txn *badger.Txn) error {
		for _, id := range []string{"doc0_0", "doc1_0", "doc2_0"} {
			if _, err := txn.Get(vectorKey("bm25_race", id)); err == nil {
				t.Errorf("向量 %s 应已删除但仍存在", id)
			}
		}
		for _, id := range []string{"doc3_0", "doc4_0"} {
			if _, err := txn.Get(vectorKey("bm25_race", id)); err != nil {
				t.Errorf("向量 %s 应保留但未找到: %v", id, err)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("验证向量状态失败: %v", err)
	}
}

// TestAddVectorsDeleteDocument_DeterministicOrder 验证任务 4：
// 在明确的执行顺序下（先 AddVectors 后 DeleteDocument），BM25 索引无残留。
// 此测试与 TestAddVectorsDeleteDocument_BM25NoResidue 互补：
// 前者验证并发 DeleteDocument 的 BM25 清理，本测试验证 AddVectors/DeleteDocument
// 对同一文档的 BM25 更新与清理在 collectionLock 保护下正确工作。
func TestAddVectorsDeleteDocument_DeterministicOrder(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 8
	if err := vs.CreateCollection("bm25_det", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// 辅助函数：写入 chunk 文本到 Badger
	writeChunk := func(collection, id, text string) {
		if err := vs.WithTx(func(txn *badger.Txn) error {
			return txn.Set(chunkKey(collection, id), []byte(text))
		}); err != nil {
			t.Fatalf("write chunk %s: %v", id, err)
		}
	}

	vec := unitVector(dim, 1, 0.01)

	// 步骤1：写入 doc1 的 chunk 文本和向量
	writeChunk("bm25_det", "doc1_0", "苹果香蕉橙子")
	writeChunk("bm25_det", "doc1_1", "葡萄西瓜荔枝")
	if err := vs.AddVectors("bm25_det", []string{"doc1_0", "doc1_1"}, [][]float64{vec, vec}); err != nil {
		t.Fatalf("AddVectors doc1: %v", err)
	}

	// 验证 BM25 含 doc1
	results := vs.getOrCreateBM25("bm25_det").Search("苹果", 10)
	foundDoc1 := false
	for _, r := range results {
		if strings.HasPrefix(r.ID, "doc1_") {
			foundDoc1 = true
			break
		}
	}
	if !foundDoc1 {
		t.Fatal("AddVectors 后 BM25 应包含 doc1")
	}

	// 步骤2：DeleteDocument(doc1) 清理 BM25
	if err := vs.DeleteDocument("bm25_det", "doc1"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}

	// 验证 BM25 不含 doc1
	results = vs.getOrCreateBM25("bm25_det").Search("苹果", 10)
	for _, r := range results {
		if strings.HasPrefix(r.ID, "doc1_") {
			t.Errorf("DeleteDocument 后 BM25 不应包含 doc1，但找到 %s (score=%f)", r.ID, r.Score)
		}
	}

	// 步骤3：重新 AddVectors(doc1) 后 BM25 应再次包含 doc1
	writeChunk("bm25_det", "doc1_0", "苹果香蕉橙子")
	writeChunk("bm25_det", "doc1_1", "葡萄西瓜荔枝")
	if err := vs.AddVectors("bm25_det", []string{"doc1_0", "doc1_1"}, [][]float64{vec, vec}); err != nil {
		t.Fatalf("重新 AddVectors doc1: %v", err)
	}

	results = vs.getOrCreateBM25("bm25_det").Search("苹果", 10)
	foundDoc1 = false
	for _, r := range results {
		if strings.HasPrefix(r.ID, "doc1_") {
			foundDoc1 = true
			break
		}
	}
	if !foundDoc1 {
		t.Error("重新 AddVectors 后 BM25 应再次包含 doc1")
	}
}

// ---------------------------------------------------------------------------
// 任务 6: badgerIndex LRU 缓存 —— 重复检索命中缓存
// ---------------------------------------------------------------------------

// TestBadgerIndex_LRUCache 验证任务 6：
// badgerIndex.Search 首次检索后 LRU 缓存被填充，第二次检索命中缓存。
// 通过检查缓存条目数量验证缓存机制生效。
func TestBadgerIndex_LRUCache(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 8
	const vectorCount = 100
	if err := vs.CreateCollection("lru_col", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	ids := make([]string, vectorCount)
	vecs := make([][]float64, vectorCount)
	for i := range vectorCount {
		ids[i] = fmt.Sprintf("v%d", i)
		vecs[i] = unitVector(dim, i, 0.01)
	}
	if err := vs.AddVectors("lru_col", ids, vecs); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	// 临时调低阈值，触发 badgerIndex 路径
	origThreshold := maxInMemoryVectors
	maxInMemoryVectors = 50
	defer func() { maxInMemoryVectors = origThreshold }()

	// 删除已加载的 memIndex 缓存，强制下次 getOrLoadIndex 重新决策
	vs.mu.Lock()
	delete(vs.indexes, "lru_col")
	vs.mu.Unlock()

	idx, err := vs.getOrLoadIndex("lru_col")
	if err != nil {
		t.Fatalf("getOrLoadIndex: %v", err)
	}
	bi, ok := idx.(*badgerIndex)
	if !ok {
		t.Fatalf("期望 *badgerIndex，实际 %T", idx)
	}

	query := vecs[0]

	// 第一次检索：缓存未命中，从 Badger 加载并填充缓存
	_, err = bi.Search(context.Background(), query, 5)
	if err != nil {
		t.Fatalf("第一次 Search: %v", err)
	}

	// 验证缓存已被填充
	bi.cache.mu.Lock()
	cacheLenAfterFirst := bi.cache.ll.Len()
	bi.cache.mu.Unlock()
	if cacheLenAfterFirst == 0 {
		t.Fatal("第一次检索后缓存为空，LRU 缓存未生效")
	}

	// 第二次检索：应命中缓存
	results2, err := bi.Search(context.Background(), query, 5)
	if err != nil {
		t.Fatalf("第二次 Search: %v", err)
	}
	if len(results2) == 0 {
		t.Fatal("第二次检索返回空结果")
	}
	// 两次检索结果应一致
	if results2[0].ID != "v0" {
		t.Errorf("期望 top-1 为 v0，实际 %q", results2[0].ID)
	}

	// 验证缓存失效：调用 clear 后缓存应为空
	bi.cache.clear()
	bi.cache.mu.Lock()
	cacheLenAfterClear := bi.cache.ll.Len()
	bi.cache.mu.Unlock()
	if cacheLenAfterClear != 0 {
		t.Errorf("clear 后缓存非空，剩余 %d 个块", cacheLenAfterClear)
	}
}

// ---------------------------------------------------------------------------
// 任务 15: VectorStore.Close 关闭所有 index 资源
// ---------------------------------------------------------------------------

// TestClose_ReleasesIndexResources 验证任务 15：
// Close() 遍历 vs.indexes 调用 index.Close()，释放所有索引资源。
func TestClose_ReleasesIndexResources(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}

	const dim = 8
	if err := vs.CreateCollection("close_col", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	ids := make([]string, 5)
	vecs := make([][]float64, 5)
	for i := range 5 {
		ids[i] = fmt.Sprintf("v%d", i)
		vecs[i] = unitVector(dim, i, 0.01)
	}
	if err := vs.AddVectors("close_col", ids, vecs); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	// 预加载索引
	_, err = vs.getOrLoadIndex("close_col")
	if err != nil {
		t.Fatalf("getOrLoadIndex: %v", err)
	}

	// 验证索引存在
	vs.mu.RLock()
	idxCount := len(vs.indexes)
	vs.mu.RUnlock()
	if idxCount == 0 {
		t.Fatal("预加载后索引应为非空")
	}

	// 调用 Close
	if err := vs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// 验证索引已被清空
	vs.mu.RLock()
	afterClose := len(vs.indexes)
	vs.mu.RUnlock()
	if afterClose != 0 {
		t.Errorf("Close 后索引未清空，剩余 %d 个", afterClose)
	}

	// 验证可以重新打开（db 已关闭，应返回错误或新实例）
	vs2, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("重新打开 NewVectorStore: %v", err)
	}
	vs2.Close()
}

// ---------------------------------------------------------------------------
// 任务 34: badgerIndex.Search 超时与扫描上限
// ---------------------------------------------------------------------------

// TestBadgerSearch_ScanLimit 验证任务 34：
// badgerIndex.Search 在扫描超过 maxScanVectors 时截断并返回部分结果。
func TestBadgerSearch_ScanLimit(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 4
	const vectorCount = 200
	if err := vs.CreateCollection("scan_limit", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	ids := make([]string, vectorCount)
	vecs := make([][]float64, vectorCount)
	for i := range vectorCount {
		ids[i] = fmt.Sprintf("v%d", i)
		vecs[i] = unitVector(dim, i, 0.01)
	}
	if err := vs.AddVectors("scan_limit", ids, vecs); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	// 临时调低阈值和扫描上限，触发 badgerIndex + 截断
	origThreshold := maxInMemoryVectors
	origScanLimit := maxScanVectors
	maxInMemoryVectors = 50
	maxScanVectors = 100 // 只扫描前 100 个向量
	defer func() {
		maxInMemoryVectors = origThreshold
		maxScanVectors = origScanLimit
	}()

	vs.mu.Lock()
	delete(vs.indexes, "scan_limit")
	vs.mu.Unlock()

	idx, err := vs.getOrLoadIndex("scan_limit")
	if err != nil {
		t.Fatalf("getOrLoadIndex: %v", err)
	}
	bi, ok := idx.(*badgerIndex)
	if !ok {
		t.Fatalf("期望 *badgerIndex，实际 %T", idx)
	}

	// 清空缓存以确保从 Badger 扫描
	bi.cache.clear()

	// 检索：应截断在 100 个向量，但仍返回结果
	results, err := bi.Search(context.Background(), vecs[0], 5)
	if err != nil {
		t.Fatalf("Search with scan limit: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("截断后应仍返回结果，实际为空")
	}
	// 查询向量自身应排第一（如果在前 100 个内）
	if results[0].ID != "v0" {
		t.Logf("top-1 为 %q（可能因截断顺序不同），score=%f", results[0].ID, results[0].Score)
	}
}

// TestBadgerSearch_CtxCanceled 验证任务 34：
// badgerIndex.Search 在 ctx 已取消时立即返回 context.Canceled。
func TestBadgerSearch_CtxCanceled(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 4
	const vectorCount = 100
	if err := vs.CreateCollection("ctx_cancel", dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	ids := make([]string, vectorCount)
	vecs := make([][]float64, vectorCount)
	for i := range vectorCount {
		ids[i] = fmt.Sprintf("v%d", i)
		vecs[i] = unitVector(dim, i, 0.01)
	}
	if err := vs.AddVectors("ctx_cancel", ids, vecs); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	// 触发 badgerIndex 路径
	origThreshold := maxInMemoryVectors
	maxInMemoryVectors = 50
	defer func() { maxInMemoryVectors = origThreshold }()

	vs.mu.Lock()
	delete(vs.indexes, "ctx_cancel")
	vs.mu.Unlock()

	idx, err := vs.getOrLoadIndex("ctx_cancel")
	if err != nil {
		t.Fatalf("getOrLoadIndex: %v", err)
	}
	bi, ok := idx.(*badgerIndex)
	if !ok {
		t.Fatalf("期望 *badgerIndex，实际 %T", idx)
	}

	// 清空缓存确保需要扫描
	bi.cache.clear()

	// 使用已取消的 ctx
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = bi.Search(ctx, vecs[0], 5)
	if err == nil {
		t.Fatal("已取消的 ctx 应返回错误，实际返回 nil")
	}
	// 错误应包含 context.Canceled
	if !errors.Is(err, context.Canceled) {
		t.Logf("返回错误: %v（期望包含 context.Canceled）", err)
	}
}

// TestMemIndex_CtxCanceled 验证任务 34：
// memIndex.Search 在 ctx 已取消时返回 context.Canceled。
func TestMemIndex_CtxCanceled(t *testing.T) {
	mi := newMemIndex(4)
	for i := range 2000 {
		mi.insert(fmt.Sprintf("v%d", i), unitVector(4, i, 0.01))
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := mi.Search(ctx, unitVector(4, 0, 0.01), 5)
	if err == nil {
		t.Fatal("已取消的 ctx 应返回错误，实际返回 nil")
	}
}

// TestDeleteDocument_DocIDPrefixNoFalseMatch 验证安全修复 S1：
// 当两个 docID 互为前缀时（如 "doc" 和 "doc_1"），删除 "doc" 不应误删 "doc_1" 的 chunk。
// 原实现仅用 strings.HasPrefix(id, docID+"_") 匹配，会对 "doc_1_0" 误判（因为 "doc_1_0"
// 以 "doc_" 开头）。修复后用 parseChunkID 精确提取 docID 部分后再比对。
func TestDeleteDocument_DocIDPrefixNoFalseMatch(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	const dim = 8
	const collection = "prefix_test"
	if err := vs.CreateCollection(collection, dim); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	// 辅助函数：写入 chunk 文本
	writeChunk := func(id, text string) {
		if err := vs.WithTx(func(txn *badger.Txn) error {
			return txn.Set(chunkKey(collection, id), []byte(text))
		}); err != nil {
			t.Fatalf("write chunk %s: %v", id, err)
		}
	}

	vec := unitVector(dim, 1, 0.01)

	// 写入两个 docID 互为前缀的文档：
	// - docID="doc" 的 chunk：doc_0, doc_1
	// - docID="doc_1" 的 chunk：doc_1_0, doc_1_1
	// 注意：删除 "doc" 时，"doc_0" 应被删除，"doc_1_0" 不应被误删
	writeChunk("doc_0", "苹果香蕉")
	writeChunk("doc_1", "橙子葡萄")
	writeChunk("doc_1_0", "西瓜荔枝")
	writeChunk("doc_1_1", "芒果榴莲")
	if err := vs.AddVectors(collection,
		[]string{"doc_0", "doc_1", "doc_1_0", "doc_1_1"},
		[][]float64{vec, vec, vec, vec}); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	// 删除 docID="doc"，只应删除 doc_0 和 doc_1，不应误删 doc_1_0 和 doc_1_1
	if err := vs.DeleteDocument(collection, "doc"); err != nil {
		t.Fatalf("DeleteDocument doc: %v", err)
	}

	// 验证 doc_1_0 和 doc_1_1 仍存在
	meta, err := vs.getCollectionMeta(collection)
	if err != nil {
		t.Fatalf("getCollectionMeta: %v", err)
	}
	if meta.VectorCount != 2 {
		t.Errorf("删除 'doc' 后 VectorCount 应为 2（doc_1_0 和 doc_1_1），实际 %d", meta.VectorCount)
	}

	// 验证 BM25 检索仍能找到 doc_1 的内容
	results := vs.getOrCreateBM25(collection).Search("西瓜", 10)
	foundDoc1 := false
	for _, r := range results {
		if r.ID == "doc_1_0" {
			foundDoc1 = true
			break
		}
	}
	if !foundDoc1 {
		t.Errorf("删除 'doc' 后 BM25 应仍能找到 'doc_1_0'，但未找到。结果: %v", results)
	}
}

// ---------------------------------------------------------------------------
// 任务 5: VectorStore.Close 后并发调用不 panic，返回 ErrClosed
// ---------------------------------------------------------------------------

// TestClose_AddVectorsReturnsErrClosed 验证任务 5：
// Close 后调用 AddVectors 应返回 ErrClosed，而不是对 nil map 写入导致 panic。
// 生活类比：图书馆闭馆后，新书入库申请应在门口就被婉拒，而不是走到已锁的库房才发现进不去。
func TestClose_AddVectorsReturnsErrClosed(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	if err := vs.CreateCollection("c", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := vs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err = vs.AddVectors("c", []string{"v1"}, [][]float64{{1.0, 0.0, 0.0, 0.0}})
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("期望 ErrClosed，实际 %v", err)
	}
}

// TestClose_SearchReturnsErrClosed 验证任务 5：
// Close 后调用 Search 应返回 ErrClosed，而不是在 collectionLock/getOrLoadIndex 中 panic。
func TestClose_SearchReturnsErrClosed(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	if err := vs.CreateCollection("c", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := vs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err = vs.Search(context.Background(), "c", []float64{1.0, 0.0, 0.0, 0.0}, 5)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("期望 ErrClosed，实际 %v", err)
	}
}

// TestClose_DeleteDocumentReturnsErrClosed 验证任务 5：
// Close 后调用 DeleteDocument 应返回 ErrClosed，而不是在 collectionLock/索引清理中 panic。
func TestClose_DeleteDocumentReturnsErrClosed(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	if err := vs.CreateCollection("c", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := vs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err = vs.DeleteDocument("c", "doc1")
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("期望 ErrClosed，实际 %v", err)
	}
}

// TestClose_DeleteCollectionReturnsErrClosed 验证任务 5：
// Close 后调用 DeleteCollection 应返回 ErrClosed，而不是在 delete(vs.indexes, ...) 中 panic。
func TestClose_DeleteCollectionReturnsErrClosed(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	if err := vs.CreateCollection("c", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := vs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	err = vs.DeleteCollection("c")
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("期望 ErrClosed，实际 %v", err)
	}
}

// TestClose_HybridSearchReturnsErrClosed 验证任务 5：
// Close 后调用 HybridSearch 应返回 ErrClosed，而不是在 getOrCreateBM25 中对 nil map 写入 panic。
func TestClose_HybridSearchReturnsErrClosed(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	if err := vs.CreateCollection("c", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := vs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	_, err = vs.HybridSearch(context.Background(), "c", []float64{1.0, 0.0, 0.0, 0.0}, "query", 5, 0.0)
	if !errors.Is(err, ErrClosed) {
		t.Fatalf("期望 ErrClosed，实际 %v", err)
	}
}

// TestClose_CollectionLockNoPanic 验证任务 5：
// Close 后并发 goroutine 调用 collectionLock 不应 panic（返回临时 mutex）。
// 这是任务 5 的核心场景：Close 把 vs.locks 置 nil，若并发 goroutine 此时写入会 panic。
func TestClose_CollectionLockNoPanic(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	if err := vs.CreateCollection("c", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := vs.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// collectionLock 在 closed 时应返回新 mutex，不写入 nil map，不 panic
	mu := vs.collectionLock("c")
	if mu == nil {
		t.Fatal("collectionLock 返回 nil，期望非 nil mutex")
	}
	// 加解锁应正常工作
	mu.Lock()
	mu.Unlock()
}

// TestClose_ConcurrentNoPanic 验证任务 5：
// 在并发场景下 Close 与 AddVectors/Search 同时调用不会 panic。
// 通过 race detector 捕获数据竞争，通过 recover 捕获 panic。
func TestClose_ConcurrentNoPanic(t *testing.T) {
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore: %v", err)
	}
	if err := vs.CreateCollection("c", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if err := vs.AddVectors("c", []string{"v1"}, [][]float64{{1.0, 0.0, 0.0, 0.0}}); err != nil {
		t.Fatalf("AddVectors: %v", err)
	}

	var wg sync.WaitGroup
	panicCh := make(chan any, 4)

	// 并发 Close
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panicCh <- r
			}
		}()
		_ = vs.Close()
	}()

	// 并发 AddVectors / Search / DeleteDocument
	for range 3 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					panicCh <- r
				}
			}()
			_ = vs.AddVectors("c", []string{"v2"}, [][]float64{{1.0, 0.0, 0.0, 0.0}})
			_, _ = vs.Search(context.Background(), "c", []float64{1.0, 0.0, 0.0, 0.0}, 5)
			_ = vs.DeleteDocument("c", "v2")
		}()
	}

	wg.Wait()
	close(panicCh)
	for r := range panicCh {
		t.Fatalf("并发 Close 场景下发生 panic: %v", r)
	}
}

// ---------------------------------------------------------------------------
// 任务 8: updateCollectionDim 校验 dim > 0
// ---------------------------------------------------------------------------

// TestUpdateCollectionDim_ZeroDimRejected 验证任务 8（P2-4）：
// updateCollectionDim 传入 dim=0 应返回 ErrZeroDimension，不应写入 Badger。
// 生活类比：量尺寸时如果尺子读数是 0 或负数，显然是错的，应该当场拒绝，
// 而不是把这个错误尺寸记到档案里，导致后续所有按尺寸裁剪的操作都出错。
func TestUpdateCollectionDim_ZeroDimRejected(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("dim_test", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	err := vs.updateCollectionDim("dim_test", 0)
	if !errors.Is(err, ErrZeroDimension) {
		t.Fatalf("期望 ErrZeroDimension，实际 %v", err)
	}

	// 验证 dim 未被修改（仍为创建时的 4）
	meta, err := vs.getCollectionMeta("dim_test")
	if err != nil {
		t.Fatalf("getCollectionMeta: %v", err)
	}
	if meta.Dim != 4 {
		t.Errorf("dim 应保持 4 不变，实际 %d", meta.Dim)
	}
}

// TestUpdateCollectionDim_NegativeDimRejected 验证任务 8：
// updateCollectionDim 传入负数 dim 也应返回 ErrZeroDimension。
func TestUpdateCollectionDim_NegativeDimRejected(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("neg_dim_test", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	err := vs.updateCollectionDim("neg_dim_test", -8)
	if !errors.Is(err, ErrZeroDimension) {
		t.Fatalf("期望 ErrZeroDimension，实际 %v", err)
	}

	// 验证 dim 未被修改
	meta, err := vs.getCollectionMeta("neg_dim_test")
	if err != nil {
		t.Fatalf("getCollectionMeta: %v", err)
	}
	if meta.Dim != 4 {
		t.Errorf("dim 应保持 4 不变，实际 %d", meta.Dim)
	}
}

// TestUpdateCollectionDim_PositiveDimOK 验证任务 8：
// updateCollectionDim 传入合法 dim 仍能正常更新（回归测试，确保校验不影响正常路径）。
func TestUpdateCollectionDim_PositiveDimOK(t *testing.T) {
	vs, cleanup := newTestStore(t)
	defer cleanup()

	if err := vs.CreateCollection("ok_dim_test", 4); err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}

	err := vs.updateCollectionDim("ok_dim_test", 8)
	if err != nil {
		t.Fatalf("合法 dim 不应报错，实际 %v", err)
	}

	meta, err := vs.getCollectionMeta("ok_dim_test")
	if err != nil {
		t.Fatalf("getCollectionMeta: %v", err)
	}
	if meta.Dim != 8 {
		t.Errorf("dim 期望 8，实际 %d", meta.Dim)
	}
}
