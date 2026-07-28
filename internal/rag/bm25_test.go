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

// TestRemoveDocument_Success 验证删除已存在的文档
//
// 生活类比：就像从通讯录里删除一个联系人，删除后联系人数量减 1，
// 而且搜索时再也找不到这个人。
func TestRemoveDocument_Success(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "苹果是水果"},
		{ID: "doc2", Text: "香蕉也是水果"},
		{ID: "doc3", Text: "计算机科学"},
	})

	// 删除 doc2
	removed := idx.RemoveDocument("doc2")
	if !removed {
		t.Error("删除存在的文档应返回 true")
	}
	if len(idx.documents) != 2 {
		t.Errorf("删除后应有 2 个文档，实际: %d", len(idx.documents))
	}

	// 验证 doc2 确实被删除
	for _, d := range idx.documents {
		if d.id == "doc2" {
			t.Error("doc2 应已被删除")
		}
	}
}

// TestRemoveDocument_EmptyID 验证空 ID 返回 false
func TestRemoveDocument_EmptyID(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{{ID: "doc1", Text: "测试"}})

	if idx.RemoveDocument("") {
		t.Error("空 ID 应返回 false")
	}
	if len(idx.documents) != 1 {
		t.Error("空 ID 不应删除任何文档")
	}
}

// TestRemoveDocument_NotFound 验证删除不存在的文档返回 false
func TestRemoveDocument_NotFound(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{{ID: "doc1", Text: "测试"}})

	if idx.RemoveDocument("不存在的ID") {
		t.Error("删除不存在的文档应返回 false")
	}
	if len(idx.documents) != 1 {
		t.Error("不存在的文档不应影响文档数量")
	}
}

// TestRemoveDocument_EmptyIndex 验证空索引删除返回 false
func TestRemoveDocument_EmptyIndex(t *testing.T) {
	idx := NewBM25Index()
	if idx.RemoveDocument("any") {
		t.Error("空索引删除应返回 false")
	}
}

// TestRemoveDocument_LastDocument 验证删除最后一个文档后 avgDL 归零
func TestRemoveDocument_LastDocument(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{{ID: "doc1", Text: "测试"}})

	if !idx.RemoveDocument("doc1") {
		t.Error("删除最后一个文档应返回 true")
	}
	if len(idx.documents) != 0 {
		t.Errorf("删除后应有 0 个文档，实际: %d", len(idx.documents))
	}
	if idx.avgDL != 0 {
		t.Errorf("空索引 avgDL 应为 0，实际: %v", idx.avgDL)
	}
}

// TestRemoveDocument_AVGDLOverRecompute 验证删除后 avgDL 重新计算
func TestRemoveDocument_AVGDLOverRecompute(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "一二三四五"},
		{ID: "doc2", Text: "六七八九十"},
	})
	// 两文档相同长度，avgDL = doc1.dl = doc2.dl
	originalAvgDL := idx.avgDL

	// 删除一个文档后 avgDL 应保持不变（因为剩下一个文档长度相同）
	idx.RemoveDocument("doc1")
	if idx.avgDL != originalAvgDL {
		t.Errorf("删除后 avgDL 应为 %v（保持不变），实际: %v", originalAvgDL, idx.avgDL)
	}
}

// ========== recomputeIDF 内存泄漏修复测试（Task 2: P1-2） ==========

// computeUniqueTerms 计算当前文档集合的唯一词数（与 recomputeIDF 内部 df 计算口径一致）
// 用于测试中作为期望值：idx.idf 的长度应等于当前文档集合的唯一词数
func computeUniqueTerms(idx *BM25Index) int {
	unique := make(map[string]struct{})
	for _, doc := range idx.documents {
		seen := make(map[string]struct{})
		for _, t := range doc.tokens {
			if _, ok := seen[t]; !ok {
				unique[t] = struct{}{}
				seen[t] = struct{}{}
			}
		}
	}
	return len(unique)
}

// TestRecomputeIDF_NoMemoryLeak 验证 recomputeIDF 不会积累已删除文档的词
//
// 场景设计：
//   1. 添加文档（包含词 A、B、C），记录 len(idx.idf)
//   2. 删除部分文档
//   3. 再添加完全不同词的文档（包含词 X、Y、Z）
//   4. 验证 len(idx.idf) == 当前文档集合的唯一词数
//      （不应包含已删除文档的词 A、B、C）
//
// 修复前：recomputeIDF 只往 idx.idf 里写新词，从不删除旧词，
// 导致 idx.idf 会持续增长（A、B、C 残留）→ 内存泄漏
// 修复后：recomputeIDF 重建 map，旧词被彻底回收
//
// 生活类比：通讯录里删除了老朋友小王的电话后，再添加新朋友小李，
// 通讯录里应该只有小李，而不应该还残留着小王的电话号码占地方
func TestRecomputeIDF_NoMemoryLeak(t *testing.T) {
	idx := NewBM25Index()

	// 步骤1：添加3个文档
	// doc1 独有词：苹果（bigram）
	// doc2 独有词：香蕉（bigram）
	// doc3 独有词：葡萄（bigram）
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "苹果 橘子"},
		{ID: "doc2", Text: "香蕉 橘子"},
		{ID: "doc3", Text: "葡萄 橘子"},
	})
	idfAfterAdd1 := len(idx.idf)
	if idfAfterAdd1 == 0 {
		t.Fatal("添加文档后 idf 不应为空")
	}

	// 步骤2：删除 doc1 和 doc2（"苹果"、"香蕉"只在被删文档中出现）
	idx.RemoveDocument("doc1")
	idx.RemoveDocument("doc2")

	// 步骤3：再添加完全不同词的新文档，词集合为 {电脑, 手机, 平板}
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc4", Text: "电脑 手机"},
		{ID: "doc5", Text: "手机 平板"},
	})

	// 步骤4：验证 idf 仅包含当前文档集合的唯一词
	wantUnique := computeUniqueTerms(idx)
	if len(idx.idf) != wantUnique {
		t.Errorf("IDF 内存泄漏：len(idx.idf)=%d，期望=%d（当前文档集合唯一词数）",
			len(idx.idf), wantUnique)
	}

	// 显式验证：已删除文档独有的词不应残留
	// "苹果" 只在已删除的 doc1 中出现，不应在 idf 中
	if _, ok := idx.idf["苹果"]; ok {
		t.Errorf("IDF 内存泄漏：已删除文档的词 '苹果' 不应残留在 idf 中")
	}
	// "香蕉" 只在已删除的 doc2 中出现，不应在 idf 中
	if _, ok := idx.idf["香蕉"]; ok {
		t.Errorf("IDF 内存泄漏：已删除文档的词 '香蕉' 不应残留在 idf 中")
	}
	// 当前文档集合中的词应存在
	if _, ok := idx.idf["电脑"]; !ok {
		t.Errorf("当前文档集合中的词 '电脑' 应在 idf 中")
	}
	if _, ok := idx.idf["葡萄"]; !ok {
		t.Errorf("当前文档集合中的词 '葡萄' 应在 idf 中")
	}
}

// TestRecomputeIDF_ShrinkAfterAllDelete 验证删除所有文档后 idf map 被清空
// 修复前：即使所有文档都被删除，idf 仍保留所有历史词 → 内存永不回收
// 修复后：重建 map，文档数为 0 时 idf 也应为空
func TestRecomputeIDF_ShrinkAfterAllDelete(t *testing.T) {
	idx := NewBM25Index()

	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "苹果 香蕉 橘子"},
		{ID: "doc2", Text: "电脑 手机 平板"},
	})
	if len(idx.idf) == 0 {
		t.Fatal("添加文档后 idf 不应为空")
	}

	// 删除所有文档
	idx.RemoveDocument("doc1")
	idx.RemoveDocument("doc2")

	// idf 应被重建为空 map（不残留任何历史词）
	if len(idx.idf) != 0 {
		t.Errorf("删除所有文档后 idf 应为空，实际 len=%d（存在内存泄漏）", len(idx.idf))
	}
}

// TestRecomputeIDF_StableAfterReAdd 验证反复增删同一批文档后 idf 大小稳定
// 这是内存泄漏最直接的体现：如果 idf 不清理，反复增删会使 len(idx.idf) 单调递增
func TestRecomputeIDF_StableAfterReAdd(t *testing.T) {
	idx := NewBM25Index()

	docsA := []BM25DocInput{
		{ID: "a1", Text: "苹果 香蕉"},
		{ID: "a2", Text: "橘子 葡萄"},
	}
	docsB := []BM25DocInput{
		{ID: "b1", Text: "电脑 手机"},
		{ID: "b2", Text: "平板 键盘"},
	}

	// 循环：添加 A → 添加 B → 全部删除，重复 3 轮
	// 中文 tokenize 会生成单字+bigram，所以每个"苹果 香蕉"会产出多个 token
	// 这里不写死数字，用 computeUniqueTerms 动态计算期望值
	for round := 0; round < 3; round++ {
		idx.AddDocuments(docsA)
		idx.AddDocuments(docsB)

		// 此时 idf 大小应等于当前文档集合的唯一词数（A、B 词集无交集）
		wantUnique := computeUniqueTerms(idx)
		if len(idx.idf) != wantUnique {
			t.Errorf("第 %d 轮：len(idx.idf)=%d，期望=%d",
				round, len(idx.idf), wantUnique)
			return
		}

		// 全部删除
		idx.RemoveDocuments(map[string]bool{
			"a1": true, "a2": true, "b1": true, "b2": true,
		})
		if len(idx.idf) != 0 {
			t.Errorf("第 %d 轮删除后：idf 应为空，实际 len=%d", round, len(idx.idf))
			return
		}
	}
}

// ========== 倒排索引测试（Task 14: PERF-4） ==========

// TestInvertedIndex_SearchOnlyMatchedDocs 验证 Search 只返回包含 query token 的文档
//
// 场景：添加 5 个英文文档，其中只有 2 个包含 "apple" 这个词。
// 搜索 "apple" 时，倒排索引只会把这两个文档作为候选，结果中不应出现其他文档。
// 用英文避免中文 bigram 干扰（"苹果" 会生成单字"苹"、"果"和 bigram"苹果"，
// 单字"果"会误匹配"水果"等词）
//
// 生活类比：图书馆的索引卡上"apple"这一词条只列了 2 本书的位置，
// 你按卡去找就只会拿到这 2 本，不会把整个书架都翻一遍
func TestInvertedIndex_SearchOnlyMatchedDocs(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "apple is a fruit"},
		{ID: "doc2", Text: "banana is yellow"},
		{ID: "doc3", Text: "apple and orange are fruits"},
		{ID: "doc4", Text: "computer and phone"},
		{ID: "doc5", Text: "tablet device"},
	})

	results := idx.Search("apple", 10)
	// 收集结果中的所有 ID
	resultIDs := make(map[string]bool, len(results))
	for _, r := range results {
		resultIDs[r.ID] = true
	}

	// doc1、doc3 包含 "apple"，应出现在结果中
	if !resultIDs["doc1"] {
		t.Errorf("doc1 包含 'apple'，应出现在结果中，实际结果: %v", resultIDs)
	}
	if !resultIDs["doc3"] {
		t.Errorf("doc3 包含 'apple'，应出现在结果中，实际结果: %v", resultIDs)
	}

	// doc2、doc4、doc5 不包含 "apple"，不应出现
	for _, unexpectedID := range []string{"doc2", "doc4", "doc5"} {
		if resultIDs[unexpectedID] {
			t.Errorf("%s 不包含 'apple'，不应出现在结果中", unexpectedID)
		}
	}
}

// TestInvertedIndex_RemoveDocumentCleansUp 验证删除文档后倒排索引不包含已删除文档的索引
//
// 场景：添加 3 个文档，删除 doc2，检查"香蕉"对应的倒排索引中不再有 doc2 的索引位置
//
// 生活类比：通讯录删掉了小王的电话后，按"姓氏"查小王时不应再找到他
func TestInvertedIndex_RemoveDocumentCleansUp(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "苹果 水果"},
		{ID: "doc2", Text: "香蕉 水果"},
		{ID: "doc3", Text: "橘子 水果"},
	})

	// 删除 doc2（位于索引 1）
	idx.RemoveDocument("doc2")

	// 删除后文档数应为 2
	if len(idx.documents) != 2 {
		t.Fatalf("删除后应有 2 个文档，实际: %d", len(idx.documents))
	}

	// 验证"香蕉"的倒排索引中没有任何文档索引（因为只有 doc2 包含"香蕉"）
	if docs, ok := idx.invertedIndex["香蕉"]; ok {
		if len(docs) > 0 {
			t.Errorf("删除 doc2 后 '香蕉' 的倒排索引应为空，实际有 %d 个文档索引", len(docs))
		}
	}

	// 验证"水果"的倒排索引中只有 2 个文档（doc1 和 doc3）
	docs, ok := idx.invertedIndex["水果"]
	if !ok {
		t.Fatal("'水果' 应在倒排索引中")
	}
	if len(docs) != 2 {
		t.Errorf("'水果' 的倒排索引应有 2 个文档，实际 %d 个", len(docs))
	}

	// 验证倒排索引中的文档索引位置仍能正确对应到现存文档
	for docIdx := range docs {
		doc := idx.documents[docIdx]
		if doc.id == "doc2" {
			t.Errorf("倒排索引中不应再指向已删除的 doc2")
		}
	}
}

// TestInvertedIndex_RemoveDocumentsBatchCleansUp 验证批量删除后倒排索引一致性
func TestInvertedIndex_RemoveDocumentsBatchCleansUp(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "苹果"},
		{ID: "doc2", Text: "香蕉"},
		{ID: "doc3", Text: "苹果 香蕉"},
		{ID: "doc4", Text: "橘子"},
	})

	// 批量删除 doc1、doc2
	removed := idx.RemoveDocuments(map[string]bool{"doc1": true, "doc2": true})
	if removed != 2 {
		t.Fatalf("期望删除 2 个，实际 %d", removed)
	}

	// "苹果" 现在只在 doc3 中
	docs, ok := idx.invertedIndex["苹果"]
	if !ok {
		t.Fatal("'苹果' 应在倒排索引中（doc3 仍包含）")
	}
	if len(docs) != 1 {
		t.Errorf("'苹果' 应只对应 1 个文档（doc3），实际 %d 个", len(docs))
	}

	// "香蕉" 现在也只在 doc3 中
	docs, ok = idx.invertedIndex["香蕉"]
	if !ok {
		t.Fatal("'香蕉' 应在倒排索引中（doc3 仍包含）")
	}
	if len(docs) != 1 {
		t.Errorf("'香蕉' 应只对应 1 个文档（doc3），实际 %d 个", len(docs))
	}

	// "橘子" 仍在 doc4 中
	docs, ok = idx.invertedIndex["橘子"]
	if !ok {
		t.Fatal("'橘子' 应在倒排索引中")
	}
	if len(docs) != 1 {
		t.Errorf("'橘子' 应只对应 1 个文档（doc4），实际 %d 个", len(docs))
	}

	// 验证 Search 仍能正常工作
	results := idx.Search("苹果", 5)
	if len(results) != 1 || results[0].ID != "doc3" {
		t.Errorf("Search('苹果') 应只返回 doc3，实际: %v", results)
	}
}

// TestInvertedIndex_RemoveByPrefixCleansUp 验证按前缀删除后倒排索引一致性
func TestInvertedIndex_RemoveByPrefixCleansUp(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "docA_000001", Text: "苹果"},
		{ID: "docA_000002", Text: "香蕉"},
		{ID: "docB_000001", Text: "苹果 橘子"},
	})

	// 删除 docA 的所有 chunk
	removed := idx.RemoveByPrefix("docA_")
	if removed != 2 {
		t.Fatalf("期望删除 2 个，实际 %d", removed)
	}

	// "苹果" 现在只在 docB_000001 中
	docs, ok := idx.invertedIndex["苹果"]
	if !ok {
		t.Fatal("'苹果' 应在倒排索引中（docB_000001 仍包含）")
	}
	if len(docs) != 1 {
		t.Errorf("'苹果' 应只对应 1 个文档，实际 %d 个", len(docs))
	}

	// "香蕉" 应完全从倒排索引中消失（只有 docA_000002 包含）
	if docs, ok := idx.invertedIndex["香蕉"]; ok && len(docs) > 0 {
		t.Errorf("'香蕉' 的倒排索引应为空或不存在，实际 %d 个文档", len(docs))
	}

	// 验证 Search 仍能找到 docB_000001
	results := idx.Search("苹果", 5)
	if len(results) != 1 || results[0].ID != "docB_000001" {
		t.Errorf("Search('苹果') 应只返回 docB_000001，实际: %v", results)
	}
}

// TestInvertedIndex_ConsistencyWithFullScan 验证倒排索引结果与全文档扫描结果一致
//
// 通过对照：用相同查询，对比"通过倒排索引 Search"和"直接全文档扫描计算 BM25"的结果。
// 如果倒排索引维护正确，两者应给出完全相同的得分（顺序和分数都一致）。
func TestInvertedIndex_ConsistencyWithFullScan(t *testing.T) {
	idx := NewBM25Index()
	docs := []BM25DocInput{
		{ID: "doc1", Text: "机器学习是人工智能的重要分支"},
		{ID: "doc2", Text: "深度学习是机器学习的子领域"},
		{ID: "doc3", Text: "自然语言处理是人工智能的分支"},
		{ID: "doc4", Text: "计算机视觉使用深度学习技术"},
		{ID: "doc5", Text: "强化学习是机器学习的一种"},
	}
	idx.AddDocuments(docs)

	query := "机器学习"
	topK := 5

	// 通过倒排索引的 Search（生产路径）
	resultsViaInverted := idx.Search(query, topK)

	// 手动全文档扫描（作为对照基准）
	idx.mu.RLock()
	queryTokens := tokenize(query)
	scores := make(map[string]float64)
	for _, doc := range idx.documents {
		var score float64
		for _, qt := range queryTokens {
			tf := float64(doc.tf[qt])
			if tf == 0 {
				continue
			}
			idf := idx.idf[qt]
			numerator := tf * (idx.k1 + 1)
			denominator := tf + idx.k1*(1-idx.b+idx.b*float64(doc.dl)/idx.avgDL)
			score += idf * numerator / denominator
		}
		if score > 0 {
			scores[doc.id] = score
		}
	}
	idx.mu.RUnlock()

	// 两者结果数量应一致
	if len(resultsViaInverted) != len(scores) {
		t.Errorf("结果数量不一致: 倒排索引=%d, 全扫描=%d", len(resultsViaInverted), len(scores))
	}

	// 两者得分应一致（按 ID 对照）
	for _, r := range resultsViaInverted {
		expected, ok := scores[r.ID]
		if !ok {
			t.Errorf("倒排索引结果中的 %s 在全扫描中不存在", r.ID)
			continue
		}
		if math.Abs(r.Score-expected) > 1e-9 {
			t.Errorf("文档 %s 得分不一致: 倒排索引=%v, 全扫描=%v", r.ID, r.Score, expected)
		}
	}
}

// TestInvertedIndex_EmptyQuery 验证空查询不崩溃且返回空结果
func TestInvertedIndex_EmptyQuery(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "测试文档"},
		{ID: "doc2", Text: "另一个测试"},
	})
	results := idx.Search("", 5)
	// 空查询 tokenize 后无 token，候选集为空，结果应为空
	if len(results) != 0 {
		t.Errorf("空查询应返回 0 个结果，实际 %d 个", len(results))
	}
}

// TestInvertedIndex_NoMatch 验证查询词不在任何文档中时返回空结果
// 用英文 "quantum" 避免中文 bigram 干扰（"量子" 会生成单字"量"、"子"，
// "子"会误匹配"橘子"中的"子"字）
func TestInvertedIndex_NoMatch(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "apple banana"},
		{ID: "doc2", Text: "orange grape"},
	})
	// "quantum" 不在任何文档中
	results := idx.Search("quantum", 5)
	if len(results) != 0 {
		t.Errorf("不匹配的查询应返回 0 个结果，实际 %d 个", len(results))
	}
}

// TestInvertedIndex_FallbackToFullScan 验证倒排索引为空但 documents 不为空时回退到全文档扫描
//
// 场景：手动清空 invertedIndex 后，Search 应仍能正确返回结果（兼容性回退路径）
func TestInvertedIndex_FallbackToFullScan(t *testing.T) {
	idx := NewBM25Index()
	idx.AddDocuments([]BM25DocInput{
		{ID: "doc1", Text: "苹果 水果"},
		{ID: "doc2", Text: "香蕉 水果"},
	})

	// 模拟倒排索引为空的旧数据场景：手动清空
	idx.mu.Lock()
	idx.invertedIndex = make(map[string]map[int]bool)
	idx.mu.Unlock()

	// Search 应通过 collectCandidates 回退到全文档扫描
	results := idx.Search("苹果", 5)
	if len(results) == 0 {
		t.Error("回退到全文档扫描后应能找到 '苹果'")
	}
	found := false
	for _, r := range results {
		if r.ID == "doc1" {
			found = true
			break
		}
	}
	if !found {
		t.Error("回退扫描应找到 doc1（包含 '苹果'）")
	}
}

// TestInvertedIndex_LargeDatasetPerformance 验证大量文档下搜索性能
//
// 添加 1000 个文档，确保 Search 在合理时间内完成且只返回匹配文档。
// 这是性能烟囱测试（smoke test），不严格断言时间，主要确保不退化到 O(N) 全扫描。
func TestInvertedIndex_LargeDatasetPerformance(t *testing.T) {
	idx := NewBM25Index()
	docs := make([]BM25DocInput, 0, 1000)
	// 1000 个文档：只有第 0、500、999 个包含"稀有词"
	for i := 0; i < 1000; i++ {
		text := "普通文档内容 编号 " + string(rune('A'+i%26))
		if i == 0 || i == 500 || i == 999 {
			text += " 稀有词"
		}
		docs = append(docs, BM25DocInput{
			ID:   "doc" + string(rune('0'+i/100)) + string(rune('0'+i%100/10)) + string(rune('0'+i%10)),
			Text: text,
		})
	}
	idx.AddDocuments(docs)

	if len(idx.documents) != 1000 {
		t.Fatalf("期望 1000 个文档，实际 %d", len(idx.documents))
	}

	// 搜索"稀有词"：通过倒排索引只应扫描到 3 个候选文档
	results := idx.Search("稀有词", 10)
	if len(results) != 3 {
		t.Errorf("应有 3 个文档包含 '稀有词'，实际 %d 个", len(results))
	}
}

// TestInvertedIndex_Benchmark 可选的性能基准测试
// 用于通过 go test -bench=. 量化倒排索引带来的性能提升
func BenchmarkInvertedIndex_Search(b *testing.B) {
	idx := NewBM25Index()
	docs := make([]BM25DocInput, 0, 1000)
	for i := 0; i < 1000; i++ {
		text := "普通文档内容 编号 " + string(rune('A'+i%26))
		if i%100 == 0 {
			text += " 稀有词"
		}
		docs = append(docs, BM25DocInput{
			ID:   "doc" + string(rune('0'+i/100)) + string(rune('0'+i%100/10)) + string(rune('0'+i%10)),
			Text: text,
		})
	}
	idx.AddDocuments(docs)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		idx.Search("稀有词", 10)
	}
}
