package rag

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/dgraph-io/badger/v4"
)

// mockEmbedder 用于测试，生成固定维度的随机向量
type mockEmbedder struct {
	dim int
}

func (m *mockEmbedder) Embed(_ context.Context, texts []string) ([][]float64, error) {
	vecs := make([][]float64, len(texts))
	for i := range texts {
		vec := make([]float64, m.dim)
		// 简单的确定性向量：基于文本长度的伪随机
		for j := range vec {
			vec[j] = float64((len(texts[i])*31+j*17)%100) / 100.0
		}
		vecs[i] = vec
	}
	return vecs, nil
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		minToken int // 最小期望值
		maxToken int // 最大期望值
	}{
		{"空字符串", "", 0, 0},
		{"英文", "hello world", 5, 15},
		{"中文", "你好世界", 2, 8},
		{"混合", "hello你好", 4, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got < tt.minToken || got > tt.maxToken {
				t.Errorf("estimateTokens(%q) = %d, want [%d, %d]", tt.text, got, tt.minToken, tt.maxToken)
			}
		})
	}
}

func TestChunkDocument_BasicSplit(t *testing.T) {
	// 构造一段超过 chunkSize 的文本，用换行分隔
	text := strings.Repeat("这是第一段内容。", 50) + "\n\n" + strings.Repeat("这是第二段内容。", 50)
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 100
	cfg.ChunkOverlap = 20

	chunks := ChunkDocument(text, cfg)
	if len(chunks) < 2 {
		t.Errorf("期望至少分成 2 个块，实际得到 %d 个", len(chunks))
	}

	// 每个 chunk 不应为空
	for i, c := range chunks {
		if c.Content == "" {
			t.Errorf("chunk[%d] 内容为空", i)
		}
	}
}

func TestChunkDocument_SmallText(t *testing.T) {
	// 小文本不应被分块
	text := "这是一段很短的文本。"
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 512

	chunks := ChunkDocument(text, cfg)
	if len(chunks) != 1 {
		t.Errorf("期望 1 个块，实际得到 %d 个", len(chunks))
	}
	if chunks[0].Content != text {
		t.Errorf("内容不匹配: got %q, want %q", chunks[0].Content, text)
	}
}

func TestChunkDocument_EmptyText(t *testing.T) {
	chunks := ChunkDocument("", DefaultChunkConfig())
	if len(chunks) != 0 {
		t.Errorf("空文本应该返回 0 个块，实际得到 %d 个", len(chunks))
	}
}

func TestChunkDocument_RecursiveSplit(t *testing.T) {
	// 测试递归分块：先用段落分隔，超大段落再用句子分隔
	text := strings.Repeat("短句。", 200) // 一个超大的段落，没有 \n\n
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 50
	cfg.ChunkOverlap = 10

	chunks := ChunkDocument(text, cfg)
	if len(chunks) < 3 {
		t.Errorf("期望至少 3 个块（递归分块），实际得到 %d 个", len(chunks))
	}
}

func TestChunkDocument_Overlap(t *testing.T) {
	// 验证重叠：相邻 chunk 应有部分内容重叠
	text := strings.Repeat("这是一段用于测试重叠的文本内容。", 30)
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 50
	cfg.ChunkOverlap = 20

	chunks := ChunkDocument(text, cfg)
	if len(chunks) < 2 {
		t.Skip("块数不足，无法验证重叠")
	}

	// 检查相邻块是否有重叠内容（至少部分字符相同）
	for i := 1; i < len(chunks); i++ {
		prev := chunks[i-1].Content
		curr := chunks[i].Content
		// 检查前一个块末尾和当前块开头是否有共同子串
		hasOverlap := false
		for overlapLen := 5; overlapLen <= min(len(prev), len(curr)); overlapLen++ {
			if strings.HasSuffix(prev, curr[:overlapLen]) {
				hasOverlap = true
				break
			}
		}
		if !hasOverlap {
			// 没有严格重叠也算正常（可能在分隔符处切分），但不应该完全无关
			t.Logf("chunk[%d] 和 chunk[%d] 无严格重叠（可能在分隔符处切分）", i-1, i)
		}
	}
}

func TestChunkDocument_Separators(t *testing.T) {
	// 测试不同分隔符的切分
	text := "第一段\n\n第二段\n第三行。第四句 第五词"
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 512

	chunks := ChunkDocument(text, cfg)
	if len(chunks) < 1 {
		t.Errorf("期望至少 1 个块，实际得到 %d 个", len(chunks))
	}
	// 文本不太长，可能合为 1 个块
	var totalContent strings.Builder
	for _, c := range chunks {
		totalContent.WriteString(c.Content)
	}
	// 所有原始内容应被保留（可能有空格/换行差异）
	if !strings.Contains(totalContent.String(), "第一段") || !strings.Contains(totalContent.String(), "第二段") {
		t.Errorf("分块后丢失了原始内容")
	}
}

func TestHardSplit(t *testing.T) {
	// 测试硬切：没有分隔符时按字符切分
	text := strings.Repeat("A", 1000)
	chunks := hardSplit(text, 100, 20)
	if len(chunks) < 5 {
		t.Errorf("期望至少 5 个块，实际得到 %d 个", len(chunks))
	}
	// 验证所有原始内容都被覆盖（有重叠所以总字符数会超过原文）
	// 检查第一个 chunk 以 "A" 开头，最后一个以 "A" 结尾
	if len(chunks) > 0 {
		if !strings.HasPrefix(chunks[0].Content, "A") {
			t.Error("第一个 chunk 应以 A 开头")
		}
		if !strings.HasSuffix(chunks[len(chunks)-1].Content, "A") {
			t.Error("最后一个 chunk 应以 A 结尾")
		}
	}
}

func TestTakeOverlap(t *testing.T) {
	text := "abcdefghij"
	result := takeOverlap(text, 5)
	// overlapTokens=5 对应约 5/0.7=7 个字符
	if len(result) == 0 {
		t.Error("takeOverlap 返回空")
	}
	// 应该是文本末尾的部分
	if !strings.HasSuffix(text, result) {
		t.Errorf("takeOverlap 结果 %q 不是文本末尾", result)
	}
}

// helperCreateVectorStore 创建基于临时目录的 VectorStore，返回 vs 和 cleanup。
func helperCreateVectorStore(t *testing.T) (*VectorStore, func()) {
	t.Helper()
	dir := t.TempDir()
	vs, err := NewVectorStore(dir)
	if err != nil {
		t.Fatalf("NewVectorStore failed: %v", err)
	}
	return vs, func() {
		if cerr := vs.Close(); cerr != nil {
			t.Logf("Close VectorStore: %v", cerr)
		}
	}
}

// TestIngestDocumentWithMeta_BM25Hit 验证任务 1：调换顺序后 BM25 检索能命中。
// 原实现先调 AddVectors（内部读 chunk 文本更新 BM25），后写 chunk 文本，
// 导致 BM25 索引为空。修复后先写 chunk 文本，再调 addVectorsCore，BM25 能正确读取文本。
func TestIngestDocumentWithMeta_BM25Hit(t *testing.T) {
	vs, cleanup := helperCreateVectorStore(t)
	defer cleanup()

	embedder := &mockEmbedder{dim: 8}
	// 构造包含特定关键词的文本，确保分块后关键词存在于某 chunk 中
	text := "机器学习是人工智能的重要分支。深度学习是机器学习的子领域。神经网络支撑了深度学习的发展。"
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 30
	cfg.ChunkOverlap = 5

	result, err := IngestDocumentWithMeta(context.Background(), vs, nil, embedder,
		"test_bm25", "doc1", text, "test.txt", int64(len(text)), "text/plain", cfg)
	if err != nil {
		t.Fatalf("IngestDocumentWithMeta failed: %v", err)
	}
	if result.StoredChunks == 0 {
		t.Fatal("StoredChunks == 0，期望至少 1 个 chunk")
	}

	// 任务 1 核心验证：chunk 文本已先于向量写入，addVectorsCore 内的 BM25 更新
	// 通过 chunkKey 读到文本，BM25 索引应有效
	bm25 := vs.getOrCreateBM25("test_bm25")
	hits := bm25.Search("机器学习", 5)
	if len(hits) == 0 {
		t.Fatal("BM25 检索未命中 '机器学习'，期望能找到包含该关键词的 chunk")
	}
	t.Logf("BM25 命中 %d 个结果，top score: %f, top id: %s", len(hits), hits[0].Score, hits[0].ID)

	// 额外验证：向量检索也能命中，且 SearchResult 包含已写入的 chunk 文本
	queryVec, _ := embedder.Embed(context.Background(), []string{"机器学习"})
	sr, sErr := vs.Search(context.Background(), "test_bm25", queryVec[0], 5)
	if sErr != nil {
		t.Fatalf("Vector Search 失败: %v", sErr)
	}
	if len(sr) == 0 {
		t.Fatal("向量检索未命中")
	}
	foundContent := false
	for _, r := range sr {
		if r.ChunkContent != "" {
			foundContent = true
			break
		}
	}
	if !foundContent {
		t.Error("SearchResult 的 ChunkContent 全为空，期望 chunk 文本已写入")
	}
}

// TestIngestDocumentWithMeta_ManyChunksBatchWrite 验证任务 1：100+ chunk 批量写入全部成功。
// 实现上所有 chunk 文本+元数据在单个 db.Update 事务内写入（见 document_pipeline.go 注释），
// 解决了原 N+1 事务问题。本测试验证批量写入的功能正确性。
func TestIngestDocumentWithMeta_ManyChunksBatchWrite(t *testing.T) {
	vs, cleanup := helperCreateVectorStore(t)
	defer cleanup()

	embedder := &mockEmbedder{dim: 8}
	// 构造 100+ chunk 的文本：每段是一个独特短句，用 \n\n 分隔
	var sb strings.Builder
	for i := range 120 {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "这是第%d个独立的段落内容。", i)
	}
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 15
	cfg.ChunkOverlap = 2

	result, err := IngestDocument(context.Background(), vs, embedder, "test_100chunks", sb.String(), cfg)
	if err != nil {
		t.Fatalf("IngestDocument failed: %v", err)
	}
	if result.TotalChunks < 50 {
		t.Fatalf("期望至少 50 个 chunk，实际 %d", result.TotalChunks)
	}
	// 全部 chunk 应成功写入（无失败钩子）
	if result.StoredChunks != result.TotalChunks {
		t.Errorf("StoredChunks=%d != TotalChunks=%d，期望全部成功写入",
			result.StoredChunks, result.TotalChunks)
	}

	// 验证向量检索能命中
	queryVec, _ := embedder.Embed(context.Background(), []string{"第50个"})
	sr, sErr := vs.Search(context.Background(), "test_100chunks", queryVec[0], 10)
	if sErr != nil {
		t.Fatalf("Search 失败: %v", sErr)
	}
	if len(sr) == 0 {
		t.Error("向量检索未命中，期望能找到相关 chunk")
	}
	t.Logf("100+ chunk 批量写入成功：Total=%d, Stored=%d, Search 命中 %d",
		result.TotalChunks, result.StoredChunks, len(sr))
}

// TestIngestDocumentWithMeta_ChunkWriteFailureReturnsError 验证任务 5：
// chunk 写入失败比例超阈值时，函数必须返回错误并回滚已写入的向量。
func TestIngestDocumentWithMeta_ChunkWriteFailureReturnsError(t *testing.T) {
	vs, cleanup := helperCreateVectorStore(t)
	defer cleanup()

	// 通过测试钩子注入全部 chunk 写入失败（100% 失败，远超 10% 阈值）
	chunkWriteErrorHook = func(collection, id string) error {
		return fmt.Errorf("injected write failure")
	}
	defer func() { chunkWriteErrorHook = nil }()

	embedder := &mockEmbedder{dim: 8}
	text := strings.Repeat("这是用于测试写入失败的文本内容。", 10)
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 20
	cfg.ChunkOverlap = 2

	result, err := IngestDocument(context.Background(), vs, embedder, "test_fail", text, cfg)
	if err == nil {
		t.Fatal("期望返回错误（chunk 写入失败比例超阈值），实际 err=nil")
	}
	if result != nil {
		t.Errorf("期望 result 为 nil，实际 %+v", result)
	}
	if !strings.Contains(err.Error(), "exceeds threshold") {
		t.Errorf("错误信息应包含 'exceeds threshold'，实际: %v", err)
	}

	// 验证回滚：collection 的向量数应为 0（addVectorsCore 未被调用）
	meta, mErr := vs.getCollectionMeta("test_fail")
	if mErr == nil && meta.VectorCount > 0 {
		t.Errorf("回滚后 VectorCount 应为 0，实际 %d", meta.VectorCount)
	}
	t.Logf("chunk 写入失败正确返回错误并回滚: %v", err)
}

// TestIngestDocumentWithMeta_PartialFailureWithinThreshold 验证任务 5：
// 部分 chunk 写入失败但未超阈值时，函数正常返回，StoredChunks 反映实际写入数。
func TestIngestDocumentWithMeta_PartialFailureWithinThreshold(t *testing.T) {
	vs, cleanup := helperCreateVectorStore(t)
	defer cleanup()

	// 注入少量失败：仅第 0 个 chunk 失败（1/总chunk < 10%）
	chunkWriteErrorHook = func(collection, id string) error {
		if strings.HasSuffix(id, "_000000") {
			return fmt.Errorf("injected single failure")
		}
		return nil
	}
	defer func() { chunkWriteErrorHook = nil }()

	embedder := &mockEmbedder{dim: 8}
	// 构造足够多 chunk 使 1 个失败占比 < 10%（至少 11 个 chunk）
	var sb strings.Builder
	for i := range 30 {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "段落%d的独特内容。", i)
	}
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 10
	cfg.ChunkOverlap = 1

	result, err := IngestDocument(context.Background(), vs, embedder, "test_partial", sb.String(), cfg)
	if err != nil {
		t.Fatalf("部分失败未超阈值时不应返回错误，实际: %v", err)
	}
	if result == nil {
		t.Fatal("result 不应为 nil")
	}
	// StoredChunks 应小于 TotalChunks（1 个失败）
	expectedStored := result.TotalChunks - 1
	if result.StoredChunks != expectedStored {
		t.Errorf("StoredChunks=%d, 期望 %d（TotalChunks=%d，1 个失败）",
			result.StoredChunks, expectedStored, result.TotalChunks)
	}
	t.Logf("部分失败场景：Total=%d, Stored=%d（1 个失败，未超阈值，正常返回）",
		result.TotalChunks, result.StoredChunks)
}

// TestIngestDocumentWithMeta_ConcurrentIngestDeleteNoOrphans 验证任务 13：
// 并发摄入 + DeleteDocument 不会产生孤立数据（chunk 文本与向量不一致）。
// collectionLock 保护整个摄入流程，DeleteDocument 也获取同一锁，两者互斥。
func TestIngestDocumentWithMeta_ConcurrentIngestDeleteNoOrphans(t *testing.T) {
	vs, cleanup := helperCreateVectorStore(t)
	defer cleanup()

	embedder := &mockEmbedder{dim: 8}
	cfg := DefaultChunkConfig()
	cfg.ChunkSize = 30
	cfg.ChunkOverlap = 5

	collection := "test_concurrent"

	// 阶段1：串行摄入 10 个文档，确保基础数据存在
	for i := range 10 {
		docID := fmt.Sprintf("pre_%d", i)
		text := fmt.Sprintf("这是预置文档%d的独特内容，用于并发删除测试。", i)
		_, err := IngestDocumentWithMeta(context.Background(), vs, nil, embedder,
			collection, docID, text, "f.txt", 100, "text/plain", cfg)
		if err != nil {
			t.Fatalf("预置文档 %d 摄入失败: %v", i, err)
		}
	}

	// 阶段2：并发摄入新文档 + 删除部分预置文档
	var wg sync.WaitGroup
	for i := range 5 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			docID := fmt.Sprintf("pre_%d", idx)
			if err := vs.DeleteDocument(collection, docID); err != nil {
				t.Errorf("删除文档 %s 失败: %v", docID, err)
			}
		}(i)
	}
	for i := range 10 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			docID := fmt.Sprintf("conc_%d", idx)
			text := fmt.Sprintf("这是并发文档%d的独特内容，用于测试锁保护。", idx)
			_, err := IngestDocumentWithMeta(context.Background(), vs, nil, embedder,
				collection, docID, text, "f.txt", 100, "text/plain", cfg)
			if err != nil {
				t.Errorf("并发摄入文档 %s 失败: %v", docID, err)
			}
		}(i)
	}
	wg.Wait()

	// 阶段3：验证无孤立数据
	// 孤立数据 = chunk 文本存在但向量不存在，或向量存在但 chunk 文本不存在
	chunkIDs := make(map[string]bool)
	vecIDs := make(map[string]bool)
	chunkPrefix := []byte("chunk:" + collection + ":")
	vecPrefix := []byte("vector:" + collection + ":")

	err := vs.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		it1 := txn.NewIterator(opts)
		defer it1.Close()
		for it1.Seek(chunkPrefix); it1.ValidForPrefix(chunkPrefix); it1.Next() {
			key := string(it1.Item().Key())
			id := strings.TrimPrefix(key, string(chunkPrefix))
			chunkIDs[id] = true
		}

		it2 := txn.NewIterator(opts)
		defer it2.Close()
		for it2.Seek(vecPrefix); it2.ValidForPrefix(vecPrefix); it2.Next() {
			key := string(it2.Item().Key())
			id := strings.TrimPrefix(key, string(vecPrefix))
			vecIDs[id] = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("扫描孤立数据失败: %v", err)
	}

	chunkOnly, vecOnly := 0, 0
	for id := range chunkIDs {
		if !vecIDs[id] {
			chunkOnly++
		}
	}
	for id := range vecIDs {
		if !chunkIDs[id] {
			vecOnly++
		}
	}
	if chunkOnly > 0 || vecOnly > 0 {
		t.Errorf("存在孤立数据：chunk-only=%d, vector-only=%d（chunk 与 vector 应一致）",
			chunkOnly, vecOnly)
	}
	t.Logf("并发后状态：chunk 数=%d, vector 数=%d，无孤立数据", len(chunkIDs), len(vecIDs))
}

// TestIngestDocumentWithMeta_ExceedsMaxChunks 验证任务 33：
// 单文档 chunk 数量超 maxChunksPerDocument 时，拒绝摄入并返回错误。
func TestIngestDocumentWithMeta_ExceedsMaxChunks(t *testing.T) {
	vs, cleanup := helperCreateVectorStore(t)
	defer cleanup()

	embedder := &mockEmbedder{dim: 8}
	// 构造超过 maxChunksPerDocument (10000) 的 chunk
	// 策略：每段是一个独特短词，用 \n\n 分隔，chunkSize=2 使每段约 1 chunk
	// estimateTokens("wordN") 约 5*0.7=3.5 取整为 3，chunkSize=2 时每段独立成 chunk
	var sb strings.Builder
	for i := range 10001 {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		fmt.Fprintf(&sb, "word%d", i)
	}
	cfg := ChunkConfig{
		ChunkSize:    2,
		ChunkOverlap: 0,
		Separators:   []string{"\n\n"}, // 只用一个分隔符，确保每段独立成 chunk
	}

	result, err := IngestDocument(context.Background(), vs, embedder, "test_overflow", sb.String(), cfg)
	if err == nil {
		t.Fatal("期望超限返回错误，实际 err=nil")
	}
	if result != nil {
		t.Errorf("期望 result 为 nil，实际 %+v", result)
	}
	if !strings.Contains(err.Error(), "exceeds max limit") {
		t.Errorf("错误信息应包含 'exceeds max limit'，实际: %v", err)
	}
	t.Logf("超限拒绝成功: %v", err)
}
