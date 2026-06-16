package rag

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"unicode"

	"github.com/dgraph-io/badger/v4"
)

// BM25Index 实现轻量级 BM25 关键词检索
// 用于与向量检索混合，提升精确关键词匹配的召回率
type BM25Index struct {
	documents []bm25Doc    // 文档集合
	avgDL     float64      // 平均文档长度
	k1        float64      // 词频饱和参数（默认 1.5）
	b         float64      // 文档长度归一化参数（默认 0.75）
	idf       map[string]float64 // 逆文档频率
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

	for i := 0; i < len(runs); i++ {
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

// AddDocument 向 BM25 索引添加文档
func (idx *BM25Index) AddDocument(id string, text string) {
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

	// 重新计算 avgDL
	var totalDL int
	for _, d := range idx.documents {
		totalDL += d.dl
	}
	idx.avgDL = float64(totalDL) / float64(len(idx.documents))

	// 重新计算 IDF
	idx.recomputeIDF()
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

	// 简单排序（结果集通常不大）
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

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
	Score        float64           // RRF 融合分数
	VectorScore  float64           // 向量余弦相似度
	BM25Score    float64           // BM25 分数
	ChunkContent string
	Metadata     map[string]string
}

// HybridSearch 执行混合检索：向量检索 + BM25 关键词检索，RRF 融合
func (vs *VectorStore) HybridSearch(collection string, query []float64, queryText string, topK int, minScore float64) ([]HybridSearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	// 1. 向量检索
	vectorResults, vecErr := vs.SearchWithThreshold(collection, query, topK*2, minScore)

	// 2. BM25 检索
	bm25Results := vs.bm25Index.Search(queryText, topK*2)

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
	for i, r := range bm25Results {
		scores[r.ID] += 1.0 / (rrfK + float64(i+1))
		bm25Scores[r.ID] = r.Score
		// 如果 BM25 找到的文档不在向量结果中，需要从 Badger 加载 chunk 内容
		if _, ok := chunkContents[r.ID]; !ok {
			if content, err := vs.loadChunkContent(collection, r.ID); err == nil {
				chunkContents[r.ID] = content
			}
			if meta, err := vs.loadChunkMeta(collection, r.ID); err == nil {
				metadatas[r.ID] = meta
			}
		}
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

	// 排序
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].Score > results[i].Score {
				results[i], results[j] = results[j], results[i]
			}
		}
	}

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
