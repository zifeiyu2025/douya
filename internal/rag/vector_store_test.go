// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package rag

import (
	"errors"
	"fmt"
	"math"
	"testing"
)

// ---------------------------------------------------------------------------
// Test fixtures
// ---------------------------------------------------------------------------

func newTestStore(t *testing.T) (*VectorStore, func()) {
	t.Helper()
	dir := t.TempDir()
	cfg := HNSWConfig{
		M:              8,
		EFConstruction: 50,
		EF:             50,
	}
	vs, err := NewVectorStore(dir, cfg)
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
	results, err := vs.Search("search_col", []float64{1.0, 0.0, 0.0, 0.0}, 2)
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

	_, err := vs.Search("non_existent", []float64{1.0, 2.0}, 5)
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

	_, err := vs.Search("search_dim_mismatch", []float64{1.0, 2.0, 3.0}, 5) // dim 3 vs expected 4
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

	_, err := vs.Search("empty_query_col", []float64{}, 5)
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

	// Request topK=100 on a collection with only 1 vector — should succeed and
	// return all available vectors (capped at min(topK, len(vectors))).
	results, err := vs.Search("small_col", []float64{1.0, 0.0, 0.0, 0.0}, 100)
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
	cfg := HNSWConfig{M: 8, EFConstruction: 50, EF: 50}

	// First session: create collection and add vectors.
	vs1, err := NewVectorStore(dir, cfg)
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
	vs2, err := NewVectorStore(dir, cfg)
	if err != nil {
		t.Fatalf("NewVectorStore session 2: %v", err)
	}
	defer vs2.Close()

	results, err := vs2.Search("persistent", []float64{1.0, 0.0, 0.0, 0.0}, 2)
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

	results, err := vs.Search("similarity_test", vecs[0], 5)
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
	allResults, err := vs.Search("thresh-test", query, 5)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}
	if len(allResults) == 0 {
		t.Fatal("expected at least 1 search result")
	}

	// 用最高分数 + 0.01 作为阈值，应该过滤掉所有结果
	highestScore := allResults[0].Score
	results, err := vs.SearchWithThreshold("thresh-test", query, 5, highestScore+0.01)
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
	results, err := vs.SearchWithThreshold("thresh-accept", query, 5, 0.5)
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
	_, err := vs.SearchWithThreshold("nonexistent", query, 5, 0.5)
	if err == nil {
		t.Error("expected error for nonexistent collection, got nil")
	}
}
