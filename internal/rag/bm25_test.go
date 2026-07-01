package rag

import (
	"math"
	"testing"
)

// ========== tokenize 测试 ==========

func TestTokenize_Chinese(t *testing.T) {
	tokens := tokenize("你好世界")
	// 中文应该产生单字和 bigram
	hasSingle := false
	hasBigram := false
	for _, tok := range tokens {
		if len([]rune(tok)) == 1 {
			hasSingle = true
		}
		if len([]rune(tok)) == 2 {
			hasBigram = true
		}
	}
	if !hasSingle {
		t.Error("中文分词应包含单字")
	}
	if !hasBigram {
		t.Error("中文分词应包含 bigram")
	}
}

func TestTokenize_English(t *testing.T) {
	tokens := tokenize("hello world test")
	// 英文按空格分词
	found := map[string]bool{}
	for _, tok := range tokens {
		found[tok] = true
	}
	if !found["hello"] || !found["world"] || !found["test"] {
		t.Errorf("英文分词结果不正确: %v", tokens)
	}
}

func TestTokenize_Mixed(t *testing.T) {
	tokens := tokenize("你好hello世界")
	// 应同时包含中文和英文 token
	hasChinese := false
	hasEnglish := false
	for _, tok := range tokens {
		for _, r := range tok {
			if isChinese(r) {
				hasChinese = true
			}
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				hasEnglish = true
			}
		}
	}
	if !hasChinese {
		t.Error("混合文本应包含中文 token")
	}
	if !hasEnglish {
		t.Error("混合文本应包含英文 token")
	}
}

func isChinese(r rune) bool {
	return r >= 0x4e00 && r <= 0x9fff
}

// ========== BM25Index 测试 ==========

func TestBM25Index_Search(t *testing.T) {
	idx := NewBM25Index()

	// 添加足够多的文档，确保 IDF 为正
	idx.AddDocument("doc1", "机器学习是人工智能的一个重要分支，涉及监督学习和无监督学习")
	idx.AddDocument("doc2", "深度学习是机器学习的子领域，使用神经网络进行特征学习")
	idx.AddDocument("doc3", "自然语言处理是人工智能的分支，处理人类语言理解和生成")
	idx.AddDocument("doc4", "计算机视觉使用深度学习技术进行图像识别和目标检测")
	idx.AddDocument("doc5", "强化学习是机器学习的一种，通过奖励信号训练智能体")

	// 搜索"机器学习"
	results := idx.Search("机器学习", 5)
	if len(results) == 0 {
		t.Error("BM25 搜索应返回结果")
	}

	if len(results) > 0 {
		// doc1、doc2、doc5 包含"机器学习"关键词，应排在前面
		topIDs := make(map[string]bool)
		topN := min(len(results), 3)
		for i := range topN {
			topIDs[results[i].ID] = true
		}
		// 至少 2 个包含"机器学习"的文档应在前 3
		matchedCount := 0
		for _, id := range []string{"doc1", "doc2", "doc5"} {
			if topIDs[id] {
				matchedCount++
			}
		}
		if matchedCount < 2 {
			t.Errorf("包含'机器学习'的文档应排在前面，实际前3: %v", results[:topN])
		}
	}
}

func TestBM25Index_EmptyQuery(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocument("doc1", "测试文档")
	results := idx.Search("", 5)
	// 空查询不应崩溃，结果可能为空
	_ = results
}

func TestBM25Index_NoMatch(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocument("doc1", "苹果是水果")
	results := idx.Search("量子力学", 5)
	if len(results) != 0 {
		t.Errorf("不相关的查询应返回 0 个结果，实际返回 %d 个", len(results))
	}
}

func TestBM25Index_TopK(t *testing.T) {
	idx := NewBM25Index()
	for i := range 20 {
		idx.AddDocument(string(rune('A'+i%26)), "机器学习测试文档内容")
	}
	results := idx.Search("机器学习", 3)
	if len(results) > 3 {
		t.Errorf("topK=3 应最多返回 3 个结果，实际返回 %d 个", len(results))
	}
}

// ========== parseChunkID 测试 ==========

func TestParseChunkID(t *testing.T) {
	tests := []struct {
		id      string
		wantDoc string
		wantIdx int
	}{
		{"doc_1234567890_000001", "doc_1234567890", 1},
		{"doc_1234567890_000002", "doc_1234567890", 2},
		{"doc_999_000010", "doc_999", 10},
		{"nodocument", "nodocument", 0}, // 无下划线
	}
	for _, tt := range tests {
		docID, chunkIdx := parseChunkID(tt.id)
		if docID != tt.wantDoc || chunkIdx != tt.wantIdx {
			t.Errorf("parseChunkID(%q) = (%q, %d), want (%q, %d)",
				tt.id, docID, chunkIdx, tt.wantDoc, tt.wantIdx)
		}
	}
}

// ========== mergeAdjacentChunks 测试 ==========

func TestMergeAdjacentChunks(t *testing.T) {
	results := []HybridSearchResult{
		{ID: "doc1_000001", ChunkContent: "第一段", Score: 0.8},
		{ID: "doc1_000002", ChunkContent: "第二段", Score: 0.7},
		{ID: "doc2_000001", ChunkContent: "其他文档", Score: 0.6},
	}

	merged := mergeAdjacentChunks(results)
	// doc1 的两个相邻 chunk 应被合并
	doc1Count := 0
	for _, r := range merged {
		if parseDocID(r.ID) == "doc1" {
			doc1Count++
		}
	}
	if doc1Count != 1 {
		t.Errorf("doc1 的相邻 chunk 应合并为 1 个，实际 %d 个", doc1Count)
	}

	// 合并后的内容应包含两段
	for _, r := range merged {
		if parseDocID(r.ID) == "doc1" {
			if !containsStr(r.ChunkContent, "第一段") || !containsStr(r.ChunkContent, "第二段") {
				t.Errorf("合并后的内容应包含两段，实际: %q", r.ChunkContent)
			}
		}
	}
}

func TestMergeAdjacentChunks_NoAdjacent(t *testing.T) {
	// 不相邻的 chunk 不应被合并
	results := []HybridSearchResult{
		{ID: "doc1_000001", ChunkContent: "第一段", Score: 0.8},
		{ID: "doc1_000003", ChunkContent: "第三段", Score: 0.6}, // 跳过了 000002
	}

	merged := mergeAdjacentChunks(results)
	if len(merged) != 2 {
		t.Errorf("不相邻的 chunk 不应合并，期望 2 个，实际 %d 个", len(merged))
	}
}

func TestMergeAdjacentChunks_DifferentDocs(t *testing.T) {
	// 不同文档的 chunk 不应被合并
	results := []HybridSearchResult{
		{ID: "doc1_000001", ChunkContent: "文档1", Score: 0.8},
		{ID: "doc2_000002", ChunkContent: "文档2", Score: 0.7},
	}

	merged := mergeAdjacentChunks(results)
	if len(merged) != 2 {
		t.Errorf("不同文档的 chunk 不应合并，期望 2 个，实际 %d 个", len(merged))
	}
}

// ========== cleanQueryText 测试 ==========

func TestCleanQueryText(t *testing.T) {
	tests := []struct {
		input  string
		expect string
	}{
		{"hello world", "hello world"},
		{"你好，世界！", "你好 世界 "},
		{"test@#$%123", "test    123"},
	}
	for _, tt := range tests {
		got := cleanQueryText(tt.input)
		if got != tt.expect {
			t.Errorf("cleanQueryText(%q) = %q, want %q", tt.input, got, tt.expect)
		}
	}
}

// 辅助函数
func parseDocID(id string) string {
	docID, _ := parseChunkID(id)
	return docID
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 ||
		anyMatch(s, sub))
}

func anyMatch(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ========== AddDocuments 批量接口测试（任务4） ==========

// TestAddDocuments_IDFOnce 验证批量添加的 IDF/avgDL 结果与逐个添加完全一致。
// AddDocuments 仅在末尾单次重算 avgDL 和 IDF，结果等价于逐个 AddDocument（每次重算）。
func TestAddDocuments_IDFOnce(t *testing.T) {
	docs := []BM25DocInput{
		{ID: "doc1", Text: "机器学习是人工智能的一个重要分支"},
		{ID: "doc2", Text: "深度学习是机器学习的子领域"},
		{ID: "doc3", Text: "自然语言处理是人工智能的分支"},
		{ID: "doc4", Text: "计算机视觉使用深度学习技术"},
		{ID: "doc5", Text: "强化学习是机器学习的一种"},
	}

	// 批量添加（仅末尾单次重算）
	batchIdx := NewBM25Index()
	batchIdx.AddDocuments(docs)

	// 逐个添加（每次都重算）
	seqIdx := NewBM25Index()
	for _, d := range docs {
		seqIdx.AddDocument(d.ID, d.Text)
	}

	// 文档数应一致
	if len(batchIdx.documents) != len(seqIdx.documents) {
		t.Fatalf("文档数不一致: batch=%d, seq=%d", len(batchIdx.documents), len(seqIdx.documents))
	}
	// avgDL 应一致
	if math.Abs(batchIdx.avgDL-seqIdx.avgDL) > 1e-9 {
		t.Errorf("avgDL 不一致: batch=%f, seq=%f", batchIdx.avgDL, seqIdx.avgDL)
	}
	// idf 词表大小应一致
	if len(batchIdx.idf) != len(seqIdx.idf) {
		t.Fatalf("idf 词表大小不一致: batch=%d, seq=%d", len(batchIdx.idf), len(seqIdx.idf))
	}
	// 每个词的 idf 值应一致
	for term, v := range seqIdx.idf {
		bv, ok := batchIdx.idf[term]
		if !ok {
			t.Errorf("batch idf 缺少词 %q", term)
			continue
		}
		if math.Abs(bv-v) > 1e-9 {
			t.Errorf("idf[%q] 不一致: batch=%f, seq=%f", term, bv, v)
		}
	}
}

// TestAddDocuments_Empty 空切片不应 panic 或改变索引状态
func TestAddDocuments_Empty(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments(nil)
	idx.AddDocuments([]BM25DocInput{})
	if len(idx.documents) != 0 {
		t.Errorf("空 AddDocuments 不应添加文档，实际 %d", len(idx.documents))
	}
}

// ========== parseChunkID 无前缀格式测试（任务4） ==========

// TestParseChunkID_NoPrefix 验证无 collection 前缀的 id 解析正确
// 统一 BM25 doc id 为 "docID_chunkIdx" 后，parseChunkID 不再遇到含冒号的错误 docID
func TestParseChunkID_NoPrefix(t *testing.T) {
	docID, chunkIdx := parseChunkID("doc_123_000001")
	if docID != "doc_123" || chunkIdx != 1 {
		t.Errorf("parseChunkID(\"doc_123_000001\") = (%q, %d), want (\"doc_123\", 1)", docID, chunkIdx)
	}
}

// ========== RemoveByPrefix 单格式测试（任务4） ==========

// TestRemoveByPrefix_SingleFormat 验证按 "docID_" 前缀清理单格式 id 的文档
func TestRemoveByPrefix_SingleFormat(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "docA_000001", Text: "苹果是水果"},
		{ID: "docA_000002", Text: "香蕉也是水果"},
		{ID: "docB_000001", Text: "计算机科学"},
	})
	// 删除 docA 的所有 chunk
	removed := idx.RemoveByPrefix("docA_")
	if removed != 2 {
		t.Fatalf("期望删除 2 个，实际 %d", removed)
	}
	// 剩余应为 docB_000001
	if len(idx.documents) != 1 {
		t.Fatalf("期望剩余 1 个文档，实际 %d", len(idx.documents))
	}
	if idx.documents[0].id != "docB_000001" {
		t.Errorf("剩余文档 id 不正确: %q", idx.documents[0].id)
	}
	// 验证仍能正常检索
	results := idx.Search("计算机", 5)
	if len(results) == 0 {
		t.Error("删除后应仍能检索到 docB")
	}
}
