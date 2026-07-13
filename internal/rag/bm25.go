package rag

import (
	"context"
	"encoding/json"
	"maps"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/dgraph-io/badger/v4"
)

// BM25Index 实现轻量级 BM25 关键词检索
// 用于与向量检索混合，提升精确关键词匹配的召回率
type BM25Index struct {
	documents []bm25Doc          // 文档集合
	avgDL     float64            // 平均文档长度
	k1        float64            // 词频饱和参数（默认 1.5）
	b         float64            // 文档长度归一化参数（默认 0.75）
	idf       map[string]float64 // 逆文档频率
	mu        sync.RWMutex       // 保护 documents/avgDL/idf 的并发读写
}

type bm25Doc struct {
	id     string
	tokens []string
	tf     map[string]int // 词频
	dl     int            // 文档长度（token 数）
}

// NewBM25Index 创建空的 BM25 索引
func NewBM25Index() *BM25Index {
	return &BM25Index{
		k1:  1.5,
		b:   0.75,
		idf: make(map[string]float64),
	}
}

// tokenize 将文本分词：中文按字符 bigram，英文按空格/标点分词
func tokenize(text string) []string {
	text = strings.ToLower(text)
	var tokens []string

	// 按非字母数字字符分割
	runs := []rune(text)
	var current []rune

	for i := range runs {
		r := runs[i]
		if unicode.Is(unicode.Han, r) {
			// 中文字符：先输出当前积累的英文词
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = current[:0]
			}
			// 输出单字
			tokens = append(tokens, string(r))
			// 生成 bigram
			if i+1 < len(runs) && unicode.Is(unicode.Han, runs[i+1]) {
				tokens = append(tokens, string(r)+string(runs[i+1]))
			}
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			current = append(current, r)
		} else {
			// 标点/空格：分割
			if len(current) > 0 {
				tokens = append(tokens, string(current))
				current = current[:0]
			}
		}
	}
	if len(current) > 0 {
		tokens = append(tokens, string(current))
	}

	return tokens
}

// recomputeAvgDLAndIDF 重新计算 avgDL 和 IDF
// 安全实践（基于 B-1.22/B-1.23）：统一 AddDocument/AddDocuments/RemoveByPrefix/RemoveDocument/RemoveDocuments
// 五处重复的 avgDL 重算逻辑，避免维护时漏改某处导致数据不一致
// 调用前提：已持有 idx.mu 写锁
func (idx *BM25Index) recomputeAvgDLAndIDF() {
	if len(idx.documents) > 0 {
		var totalDL int
		for _, d := range idx.documents {
			totalDL += d.dl
		}
		idx.avgDL = float64(totalDL) / float64(len(idx.documents))
	} else {
		idx.avgDL = 0
	}
	idx.recomputeIDF()
}

// AddDocument 向 BM25 索引添加文档
// 注意：每次调用都会重算 avgDL 和 IDF，批量插入时请用 AddDocuments 以避免 O(N²)
func (idx *BM25Index) AddDocument(id string, text string) {
	idx.mu.Lock()
	defer idx.mu.Unlock()

	tokens := tokenize(text)
	tf := make(map[string]int)
	for _, t := range tokens {
		tf[t]++
	}

	idx.documents = append(idx.documents, bm25Doc{
		id:     id,
		tokens: tokens,
		tf:     tf,
		dl:     len(tokens),
	})

	// 重算 avgDL 和 IDF（统一调用 recomputeAvgDLAndIDF）
	idx.recomputeAvgDLAndIDF()
}

// BM25DocInput 批量添加文档的输入参数
type BM25DocInput struct {
	ID   string
	Text string
}

// AddDocuments 批量添加文档，仅在末尾单次重算 avgDL 和 IDF，避免 O(N²)
// 线程安全：内部加写锁，与 AddDocument 一致
func (idx *BM25Index) AddDocuments(docs []BM25DocInput) {
	if len(docs) == 0 {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()

	// 逐个分词并追加，期间不重算 IDF
	for _, d := range docs {
		tokens := tokenize(d.Text)
		tf := make(map[string]int, len(tokens))
		for _, t := range tokens {
			tf[t]++
		}
		idx.documents = append(idx.documents, bm25Doc{
			id:     d.ID,
			tokens: tokens,
			tf:     tf,
			dl:     len(tokens),
		})
	}

	// 单次重算 avgDL 和 IDF（统一调用 recomputeAvgDLAndIDF）
	idx.recomputeAvgDLAndIDF()
}

// RemoveByPrefix 按文档 id 前缀批量删除文档（用于 DeleteDocument/DeleteCollection 同步清理 BM25）
// 安全说明：BM25 的文档 id 不带 collection 前缀，调用方需确保 prefix 在 BM25 中只匹配目标 collection 的文档
func (idx *BM25Index) RemoveByPrefix(prefix string) int {
	if prefix == "" {
		return 0
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if len(idx.documents) == 0 {
		return 0
	}
	// 原地复用底层数组：安全，因为已持有 mu.Lock（独占访问），且遍历原数组、写入同底层数组头部，
	// 写入指针始终 <= 读取指针，不会覆盖未读元素。见安全审查 #30。
	kept := idx.documents[:0]
	removed := 0
	for _, d := range idx.documents {
		if strings.HasPrefix(d.id, prefix) {
			removed++
			continue
		}
		kept = append(kept, d)
	}
	idx.documents = kept
	if removed > 0 {
		// 重算 avgDL 和 IDF（统一调用 recomputeAvgDLAndIDF）
		idx.recomputeAvgDLAndIDF()
	}
	return removed
}

// RemoveDocument 按 id 精确删除单个文档（用于 DeleteDocument 同步清理 BM25）
func (idx *BM25Index) RemoveDocument(id string) bool {
	if id == "" {
		return false
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if len(idx.documents) == 0 {
		return false
	}
	for i, d := range idx.documents {
		if d.id == id {
			// 删除第 i 个元素
			idx.documents = append(idx.documents[:i], idx.documents[i+1:]...)
			// 重算 avgDL 和 IDF（统一调用 recomputeAvgDLAndIDF）
			idx.recomputeAvgDLAndIDF()
			return true
		}
	}
	return false
}

// RemoveDocuments 按 id 集合批量删除文档（用于 DeleteCollection 一次性清理 BM25）
// 一次性重算 avgDL 和 IDF，避免逐个删除时重复计算
func (idx *BM25Index) RemoveDocuments(idSet map[string]bool) int {
	if len(idSet) == 0 {
		return 0
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	if len(idx.documents) == 0 {
		return 0
	}
	kept := idx.documents[:0] // 原地复用底层数组
	removed := 0
	for _, d := range idx.documents {
		if idSet[d.id] {
			removed++
			continue
		}
		kept = append(kept, d)
	}
	idx.documents = kept
	if removed > 0 {
		// 重算 avgDL 和 IDF（统一调用 recomputeAvgDLAndIDF）
		idx.recomputeAvgDLAndIDF()
	}
	return removed
}

// recomputeIDF 重新计算所有词的 IDF
func (idx *BM25Index) recomputeIDF() {
	n := float64(len(idx.documents))
	df := make(map[string]int) // 文档频率
	for _, doc := range idx.documents {
		seen := make(map[string]bool)
		for _, t := range doc.tokens {
			if !seen[t] {
				df[t]++
				seen[t] = true
			}
		}
	}

	for t, freq := range df {
		// 使用 log(1 + ...) 形式的 IDF，保证始终为正
		// 避免高频词在文档数较少时 IDF 为负导致所有得分为 0
		idx.idf[t] = math.Log(1 + (n-float64(freq)+0.5)/(float64(freq)+0.5))
	}
}

// Search 使用 BM25 算法检索与查询最相关的 topK 个文档
func (idx *BM25Index) Search(query string, topK int) []BM25Result {
	if topK <= 0 {
		topK = 10
	}
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	if len(idx.documents) == 0 {
		return nil
	}

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
			// BM25 评分公式
			numerator := tf * (idx.k1 + 1)
			denominator := tf + idx.k1*(1-idx.b+idx.b*float64(doc.dl)/idx.avgDL)
			score += idf * numerator / denominator
		}
		if score > 0 {
			scores[doc.id] = score
		}
	}

	// 取 topK
	results := make([]BM25Result, 0, len(scores))
	for id, score := range scores {
		results = append(results, BM25Result{ID: id, Score: score})
	}

	// 按分数降序排序
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// BM25Result BM25 检索结果
type BM25Result struct {
	ID    string
	Score float64
}

// HybridSearchResult 混合检索结果
type HybridSearchResult struct {
	ID           string
	Score        float64 // RRF 融合分数
	VectorScore  float64 // 向量余弦相似度
	BM25Score    float64 // BM25 分数
	ChunkContent string
	Metadata     map[string]string
}

// HybridSearch 执行混合检索：向量检索 + BM25 关键词检索，RRF 融合
// ctx 用于传播取消信号到向量检索
func (vs *VectorStore) HybridSearch(ctx context.Context, collection string, query []float64, queryText string, topK int, minScore float64) ([]HybridSearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	// 1. 向量检索
	vectorResults, vecErr := vs.SearchWithThreshold(ctx, collection, query, topK*2, minScore)

	// 2. BM25 检索（按 collection 隔离，每个 collection 拥有独立索引）
	bm25Results := vs.getOrCreateBM25(collection).Search(queryText, topK*2)

	// 2.1 BM25 相对阈值过滤：只保留得分 >= 最高得分 10% 的结果
	if len(bm25Results) > 0 {
		maxBM25Score := bm25Results[0].Score // 已排序，第一个最高
		threshold := maxBM25Score * 0.1
		var filtered []BM25Result
		for _, r := range bm25Results {
			if r.Score >= threshold {
				filtered = append(filtered, r)
			}
		}
		bm25Results = filtered
	}

	// 3. RRF 融合
	const rrfK = 60.0 // RRF 常数，通常取 60
	scores := make(map[string]float64)
	vectorScores := make(map[string]float64)
	bm25Scores := make(map[string]float64)
	chunkContents := make(map[string]string)
	metadatas := make(map[string]map[string]string)

	// 向量检索排名
	for i, r := range vectorResults {
		scores[r.ID] += 1.0 / (rrfK + float64(i+1))
		vectorScores[r.ID] = r.Score
		chunkContents[r.ID] = r.ChunkContent
		metadatas[r.ID] = r.Metadata
	}

	// BM25 检索排名
	// 收集所有需要从 Badger 加载的 ID（BM25 找到但向量结果中不存在的），稍后批量加载
	var idsToLoad []string
	for i, r := range bm25Results {
		scores[r.ID] += 1.0 / (rrfK + float64(i+1))
		bm25Scores[r.ID] = r.Score
		if _, ok := chunkContents[r.ID]; !ok {
			idsToLoad = append(idsToLoad, r.ID)
		}
	}
	// 批量加载 chunk 内容和元数据，替代 N 次 loadChunkContent/loadChunkMeta 调用
	// 生活类比：与其每借一本书就跑一趟柜台（N 次事务），不如列好清单一次性借回（单事务）
	if len(idsToLoad) > 0 {
		loadedContents, loadedMetas, _ := vs.loadChunksBatch(collection, idsToLoad)
		maps.Copy(chunkContents, loadedContents)
		maps.Copy(metadatas, loadedMetas)
	}

	// 如果向量检索失败且 BM25 无结果，返回错误
	if vecErr != nil && len(bm25Results) == 0 {
		return nil, vecErr
	}

	// 按融合分数排序
	var results []HybridSearchResult
	for id, score := range scores {
		results = append(results, HybridSearchResult{
			ID:           id,
			Score:        score,
			VectorScore:  vectorScores[id],
			BM25Score:    bm25Scores[id],
			ChunkContent: chunkContents[id],
			Metadata:     metadatas[id],
		})
	}

	// 排序：使用 sort.Slice 替代 O(n^2) 冒泡排序，降序排列
	// 优化前为嵌套 for 循环（O(n^2)），在 RAG 检索结果较多时性能较差；
	// 改为 sort.Slice 后复杂度降为 O(n log n)，且 sort 包已在此文件 import
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// 最终过滤：只保留在向量或 BM25 中有有意义得分的结果
	// 向量得分 >= minScore 或 BM25 得分 > 0 的结果保留
	var filteredResults []HybridSearchResult
	for _, r := range results {
		if r.VectorScore >= minScore || r.BM25Score > 0 {
			filteredResults = append(filteredResults, r)
		}
	}
	results = filteredResults

	// 去重：合并同一文档的相邻 chunk（chunk_idx 连续）
	results = mergeAdjacentChunks(results)

	if len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// loadChunkContent 从 Badger 加载单个 chunk 的文本内容
func (vs *VectorStore) loadChunkContent(collection, id string) (string, error) {
	var content string
	err := vs.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(chunkKey(collection, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			content = string(val)
			return nil
		})
	})
	return content, err
}

// loadChunkMeta 从 Badger 加载单个 chunk 的元数据
func (vs *VectorStore) loadChunkMeta(collection, id string) (map[string]string, error) {
	var meta map[string]string
	err := vs.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(chunkMetaKey(collection, id))
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &meta)
		})
	})
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// cleanQueryText 清理查询文本，去除特殊字符
var cleanQueryRe = regexp.MustCompile(`[^\p{L}\p{N}\s]`)

func cleanQueryText(query string) string {
	return cleanQueryRe.ReplaceAllString(query, " ")
}

// mergeAdjacentChunks 合并同一文档的相邻 chunk
// chunk ID 格式为 "docID_000001"，如果同一文档的连续 chunk 都被检索到，
// 合并为一个更大的上下文块，避免返回内容碎片化
func mergeAdjacentChunks(results []HybridSearchResult) []HybridSearchResult {
	if len(results) <= 1 {
		return results
	}

	// 解析每个结果的 docID 和 chunkIdx
	type chunkInfo struct {
		docID    string
		chunkIdx int
	}
	infos := make([]chunkInfo, len(results))
	for i, r := range results {
		infos[i].docID, infos[i].chunkIdx = parseChunkID(r.ID)
	}

	// 标记哪些结果需要被合并（被合并到前一个的结果标记为跳过）
	skip := make(map[int]bool)
	for i := 1; i < len(results); i++ {
		if skip[i] {
			continue
		}
		// 向前查找最近的未被跳过的同文档结果
		for j := i - 1; j >= 0; j-- {
			if skip[j] {
				continue
			}
			if infos[i].docID == infos[j].docID && infos[i].docID != "" {
				// 同一文档，检查是否相邻
				if infos[i].chunkIdx == infos[j].chunkIdx+1 {
					// 合并 i 到 j
					results[j].ChunkContent += "\n" + results[i].ChunkContent
					results[j].Score += results[i].Score * 0.5 // 合并后分数适当提升
					skip[i] = true
				}
			}
			break
		}
	}

	// 过滤被跳过的结果
	var merged []HybridSearchResult
	for i, r := range results {
		if !skip[i] {
			merged = append(merged, r)
		}
	}
	return merged
}

// parseChunkID 解析 chunk ID，格式为 "docID_chunkIdx"
func parseChunkID(id string) (docID string, chunkIdx int) {
	// ID 格式: doc_1234567890_000001
	lastUnderscore := strings.LastIndex(id, "_")
	if lastUnderscore < 0 {
		return id, 0
	}
	docID = id[:lastUnderscore]
	idxStr := id[lastUnderscore+1:]
	// 解析 chunk 编号
	var idx int
	for _, c := range idxStr {
		if c >= '0' && c <= '9' {
			idx = idx*10 + int(c-'0')
		} else {
			break
		}
	}
	return docID, idx
}
