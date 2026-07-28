// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package rag

import (
	"bytes"
	"container/heap"
	"container/list"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"
)

// ErrCollectionExists is returned when creating a collection that already exists.
var ErrCollectionExists = errors.New("collection already exists")

// ErrCollectionNotFound is returned when operating on a non-existent collection.
var ErrCollectionNotFound = errors.New("collection not found")

// ErrVectorDimMismatch is returned when vector dimension does not match the collection.
var ErrVectorDimMismatch = errors.New("vector dimension does not match collection dimension")

// ErrEmptyVector is returned when a zero-length vector is provided.
var ErrEmptyVector = errors.New("vector is empty")

// ErrZeroDimension is returned when creating a collection with dim <= 0.
var ErrZeroDimension = errors.New("dimension must be positive")

// ErrEmptyCollectionName is returned when collection name is empty.
var ErrEmptyCollectionName = errors.New("collection name cannot be empty")

// ErrInvalidCollectionName is returned when collection name contains invalid characters.
var ErrInvalidCollectionName = errors.New("collection name contains invalid characters")

// ErrClosed is returned when operating on a closed vector store.
// 用于防止 Close 后并发 goroutine 对 nil map 写入导致 panic（任务 5）。
var ErrClosed = errors.New("vector store is closed")

// SearchResult represents a single search result.
type SearchResult struct {
	ID           string            // the stored vector ID
	Score        float64           // cosine similarity score (0-1, higher = more similar)
	ChunkContent string            // the original chunk text (loaded after search)
	Metadata     map[string]string // optional metadata from the chunk
}

// CollectionInfo holds summary information about a collection.
type CollectionInfo struct {
	Name        string `json:"name"`
	Dim         int    `json:"dim"`
	VectorCount int64  `json:"vector_count"`
}

// collectionMeta stores metadata for a collection.
type collectionMeta struct {
	Dim         int32 // vector dimension
	VectorCount int64 // number of vectors in this collection
}

// HNSWConfig is reserved for future HNSW index tuning.
// Currently we use an efficient in-memory brute-force index; HNSW will be
// wired in transparently when the go-hnsw dependency is available.
type HNSWConfig struct {
	M              int
	EFConstruction int
	EF             int
}

// DefaultHNSWConfig returns sensible defaults.
func DefaultHNSWConfig() HNSWConfig {
	return HNSWConfig{
		M:              16,
		EFConstruction: 200,
		EF:             100,
	}
}

// VectorStore provides a vector database backed by Badger (KV) and an
// in-memory index. It is safe for concurrent use.
type VectorStore struct {
	db          *badger.DB
	bm25Indexes map[string]*BM25Index // BM25 关键词检索索引（按 collection 隔离）

	// Per-collection state. Access is protected by the collection-level lock map.
	mu            sync.RWMutex            // 保护 locks / indexes / badgerIndexes
	locks         map[string]*sync.Mutex  // collection name → lock
	indexes       map[string]vectorIndex  // collection name → 索引（memIndex 或 badgerIndex，nil until first load）
	badgerIndexes map[string]*badgerIndex // collection name → badgerIndex 缓存（避免重复构造）

	// closed 标记 store 是否已 Close（任务 5）。
	// 用 atomic.Bool 使并发 goroutine 无需加锁即可安全读取，
	// 避免 Close 后对 nil map（locks/bm25Indexes 等）写入导致 panic。
	closed atomic.Bool
}

// vectorIndex 抽象向量索引的检索能力（任务 30）。
// memIndex 将全部向量常驻内存，检索快但内存占用高；
// badgerIndex 则分批扫描 Badger，检索较慢但避免 OOM。
// getOrLoadIndex 按 maxInMemoryVectors 阈值选择具体实现。
type vectorIndex interface {
	Search(ctx context.Context, vec []float64, k int) ([]SearchResult, error)
	Close() error
}

// memIndex is an in-memory vector index backed by Badger.
// It stores vectors as []float64 and searches by cosine similarity.
type memIndex struct {
	dim      int
	vecs     [][]float64 // indexed vectors, position i = id i
	ids      []string    // id for each position
	vecMu    sync.RWMutex
	ready    chan struct{} // 任务 3:构建完成信号,构建期间阻塞 Search;nil 表示无需等待
	buildErr error         // 任务 3:构建失败原因,构建完成后通过 waitReady 返回
}

// newMemIndex creates an empty in-memory index.
func newMemIndex(dim int) *memIndex {
	return &memIndex{
		dim:  dim,
		vecs: make([][]float64, 0),
		ids:  make([]string, 0),
	}
}

// insert adds a vector and its id to the index.
func (idx *memIndex) insert(id string, vec []float64) {
	idx.vecMu.Lock()
	defer idx.vecMu.Unlock()
	// Grow slices in chunks to reduce allocations.
	if len(idx.vecs) == cap(idx.vecs) {
		newCap := len(idx.vecs)*2 + 64
		newVecs := make([][]float64, len(idx.vecs), newCap)
		newIDs := make([]string, len(idx.ids), newCap)
		copy(newVecs, idx.vecs)
		copy(newIDs, idx.ids)
		idx.vecs, idx.ids = newVecs, newIDs
	}
	idx.vecs = append(idx.vecs, vec)
	idx.ids = append(idx.ids, id)
}

// Search 实现 vectorIndex 接口，返回 topK 最相似向量的 SearchResult。
// 直接返回包含 ID 的 SearchResult，调用方无需再映射位置。
// 在同一把 vecMu.RLock 内完成检索和 ID 映射，避免重复加锁。
// 任务 34:接受 ctx 参数,每 1000 次迭代检查 ctx.Err() 以支持超时取消。
func (idx *memIndex) Search(ctx context.Context, query []float64, k int) ([]SearchResult, error) {
	if err := idx.waitReady(); err != nil {
		return nil, fmt.Errorf("memIndex not ready: %w", err)
	}
	if len(query) == 0 {
		return nil, ErrEmptyVector
	}
	if k <= 0 {
		k = 10
	}

	idx.vecMu.RLock()
	defer idx.vecMu.RUnlock()

	qNorm := cosineNorm(query)
	if qNorm == 0 {
		return nil, nil
	}

	// 使用最小堆维护 topK 结果，避免全排序
	h := &minHeap{}
	heap.Init(h)

	for i, v := range idx.vecs {
		// 任务 34:每 1000 次检查 ctx,支持超时取消
		if i > 0 && i%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		score := cosineSimilarityPreNorm(query, v, qNorm)
		if h.Len() < k {
			heap.Push(h, scoredPos{pos: i, score: score})
		} else if score > h.Peek().score {
			heap.Pop(h)
			heap.Push(h, scoredPos{pos: i, score: score})
		}
	}

	// 从堆中提取结果，按分数降序排列
	result := make([]scoredPos, h.Len())
	for i := h.Len() - 1; i >= 0; i-- {
		result[i] = heap.Pop(h).(scoredPos)
	}

	out := make([]SearchResult, len(result))
	for i, r := range result {
		if r.pos >= 0 && r.pos < len(idx.ids) {
			out[i] = SearchResult{ID: idx.ids[r.pos], Score: r.score, Metadata: make(map[string]string)}
		}
	}
	return out, nil
}

// waitReady 在 memIndex 构建期间阻塞调用方，直到构建完成或失败。
func (idx *memIndex) waitReady() error {
	if idx.ready != nil {
		<-idx.ready
		return idx.buildErr
	}
	return nil
}

// Close 实现 vectorIndex 接口，释放 memIndex 占用的内存。
// 清空 vecs 和 ids 切片，让 GC 回收底层向量数据。
func (idx *memIndex) Close() error {
	idx.vecMu.Lock()
	defer idx.vecMu.Unlock()
	idx.vecs = nil
	idx.ids = nil
	return nil
}

// scoredPos 用于堆排序的位置-分数对
type scoredPos struct {
	pos   int
	score float64
}

// minHeap 实现最小堆接口，用于维护 topK 结果
type minHeap []scoredPos

func (h minHeap) Len() int           { return len(h) }
func (h minHeap) Less(i, j int) bool { return h[i].score < h[j].score } // 最小堆
func (h minHeap) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeap) Push(x any)        { *h = append(*h, x.(scoredPos)) }
func (h *minHeap) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Peek 返回堆顶元素但不移除
func (h *minHeap) Peek() scoredPos {
	if len(*h) == 0 {
		return scoredPos{}
	}
	return (*h)[0]
}

// cosineNorm returns the L2 norm of a vector.
func cosineNorm(v []float64) float64 {
	sum := 0.0
	for _, x := range v {
		sum += x * x
	}
	return math.Sqrt(sum)
}

// cosineSimilarityPreNorm returns the cosine similarity between query and vec.
// queryNorm is pre-computed to avoid redundant calculation.
func cosineSimilarityPreNorm(query, vec []float64, queryNorm float64) float64 {
	dot := 0.0
	for i := range query {
		dot += query[i] * vec[i]
	}
	vecNorm := cosineNorm(vec)
	return dot / (queryNorm*vecNorm + 1e-12)
}

// ---------------------------------------------------------------------------
// badgerIndex —— 大集合的磁盘扫描型向量索引（任务 30）
// ---------------------------------------------------------------------------

// badgerIndex 不在内存常驻全部向量，而是每次 Search 时分批扫描 Badger
// 中 "vector:<collection>:" 前缀的键，逐批计算点积并用最小堆维护全局 top-K。
// 适用于向量数超过 maxInMemoryVectors 的场景：内存占用 O(K)（仅堆），
// 代价是检索延迟从 ms 级上升到 100ms 级（10 万向量、D=768 时约 200ms）。
// cacheBlockSize 是 badgerIndex 缓存的单个向量块大小(按 Badger key 字典序连续切片)。
// 4096 个向量/块兼顾缓存粒度与单次加载开销:D=768 时约 24MB/块。
const cacheBlockSize = 4096

// maxScanVectors 是 badgerIndex.Search 单次检索扫描的向量上限(任务 34)。
// 超过此阈值时中断扫描并告警,避免大集合检索耗时过长。
// 改为 var 便于测试中临时调小阈值验证截断逻辑(参考 maxInMemoryVectors)。
var maxScanVectors = 500000

// vectorEntry 缓存中单个向量条目:id 去掉了 collection 前缀。
type vectorEntry struct {
	id  string
	vec []float64
}

// vectorBlock 缓存的一个向量块:起始 key、末尾 key 与 entries。
// 起始 key 作为 LRU 的索引键,命中时直接用 endKey 通过 Seek 跳过整个块。
type vectorBlock struct {
	startKey []byte
	endKey   []byte
	entries  []vectorEntry
}

// vectorLRU 简单 LRU 缓存:双向链表 + map,按块的 startKey 字符串索引。
// 自实现避免引入 hashicorp/golang-lru 新依赖(任务 6 要求优先自实现)。
type vectorLRU struct {
	capacity int
	mu       sync.Mutex
	m        map[string]*list.Element
	ll       *list.List
}

type lruEntry struct {
	key   string
	block *vectorBlock
}

func newVectorLRU(capacity int) *vectorLRU {
	if capacity < 1 {
		capacity = 1
	}
	return &vectorLRU{
		capacity: capacity,
		m:        make(map[string]*list.Element),
		ll:       list.New(),
	}
}

// get 命中时把块移动到链表头并返回;未命中返回 nil, false。
func (l *vectorLRU) get(key string) (*vectorBlock, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.m[key]; ok {
		l.ll.MoveToFront(el)
		return el.Value.(*lruEntry).block, true
	}
	return nil, false
}

// put 写入块,超出容量时淘汰最久未使用的块。
func (l *vectorLRU) put(key string, block *vectorBlock) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if el, ok := l.m[key]; ok {
		el.Value.(*lruEntry).block = block
		l.ll.MoveToFront(el)
		return
	}
	el := l.ll.PushFront(&lruEntry{key: key, block: block})
	l.m[key] = el
	for l.ll.Len() > l.capacity {
		oldest := l.ll.Back()
		if oldest == nil {
			break
		}
		l.ll.Remove(oldest)
		delete(l.m, oldest.Value.(*lruEntry).key)
	}
}

// clear 清空缓存(任务 6:AddVectors/DeleteDocument 后整体失效 collection 缓存)。
func (l *vectorLRU) clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.m = make(map[string]*list.Element)
	l.ll = list.New()
}

// lruCapacity 是单个 badgerIndex LRU 最多缓存的块数。
// 256 块 * 4096 = 100 万向量,足够覆盖单 collection 的热数据。
const lruCapacity = 256

type badgerIndex struct {
	db     *badger.DB // 共享的 Badger 句柄(由 VectorStore.Close 统一关闭)
	prefix []byte     // "vector:<collection>:",迭代时用前缀过滤
	dim    int        // 向量维度,用于解析存储的字节流
	cache  *vectorLRU // 任务 6:按块缓存最近访问的向量,避免重复扫描 Badger
}

// Search 实现 vectorIndex 接口，分批扫描 Badger 维护全局 top-K 最小堆。
// 任务 6:优先从 LRU 缓存读取向量块,未命中再从 Badger 加载并填充缓存。
// 任务 34:接受 ctx 参数,每 1000 次迭代检查 ctx.Err();扫描上限 maxScanVectors。
func (bi *badgerIndex) Search(ctx context.Context, query []float64, k int) ([]SearchResult, error) {
	if len(query) == 0 {
		return nil, ErrEmptyVector
	}
	if k <= 0 {
		k = 10
	}

	qNorm := cosineNorm(query)
	if qNorm == 0 {
		return nil, nil
	}

	// 使用最小堆维护 topK 结果，堆顶是当前最小分数
	h := &minHeapResult{}
	heap.Init(h)

	scanned := 0
	truncated := false

	err := bi.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = cacheBlockSize // 每批预取一个块的条目
		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(bi.prefix); it.ValidForPrefix(bi.prefix); {
			// 任务 34:块级检查 ctx 与扫描上限
			if err := ctx.Err(); err != nil {
				return err
			}
			if scanned >= maxScanVectors {
				truncated = true
				return nil
			}

			// 当前 key 作为块的起始 key
			curKey := it.Item().KeyCopy(nil)
			curKeyStr := string(curKey)

			// 任务 6:优先从 LRU 缓存读取块
			if block, ok := bi.cache.get(curKeyStr); ok {
				for _, entry := range block.entries {
					if scanned > 0 && scanned%1000 == 0 {
						if err := ctx.Err(); err != nil {
							return err
						}
						if scanned >= maxScanVectors {
							truncated = true
							return nil
						}
					}
					score := cosineSimilarityPreNorm(query, entry.vec, qNorm)
					if h.Len() < k {
						heap.Push(h, scoredResult{id: entry.id, score: score})
					} else if score > h.Peek().score {
						heap.Pop(h)
						heap.Push(h, scoredResult{id: entry.id, score: score})
					}
					scanned++
				}
				// 跳过已缓存的块:Seek 到 endKey 之后
				it.Seek(nextBadgerKey(block.endKey))
				continue
			}

			// 缓存未命中:从 Badger 加载一个块(cacheBlockSize 个向量)
			var entries []vectorEntry
			var lastKey []byte
			for i := 0; i < cacheBlockSize && it.ValidForPrefix(bi.prefix); i++ {
				if i > 0 && i%1000 == 0 {
					if err := ctx.Err(); err != nil {
						return err
					}
					if scanned >= maxScanVectors {
						truncated = true
						return nil
					}
				}
				item := it.Item()
				vecBytes, err := item.ValueCopy(nil)
				if err != nil {
					log.Warn().Err(err).Str("key", string(item.Key())).Msg("[rag] badgerIndex skipped malformed vector")
					it.Next()
					continue
				}
				vec, err := bytesToVector(vecBytes, bi.dim)
				if err != nil {
					log.Warn().Err(err).Str("key", string(item.Key())).Msg("[rag] badgerIndex skipped malformed vector")
					it.Next()
					continue
				}
				id := string(item.Key()[len(bi.prefix):])
				score := cosineSimilarityPreNorm(query, vec, qNorm)
				if h.Len() < k {
					heap.Push(h, scoredResult{id: id, score: score})
				} else if score > h.Peek().score {
					heap.Pop(h)
					heap.Push(h, scoredResult{id: id, score: score})
				}
				entries = append(entries, vectorEntry{id: id, vec: vec})
				lastKey = item.KeyCopy(nil)
				scanned++
				it.Next()
			}

			// 缓存加载的块
			if len(entries) > 0 {
				bi.cache.put(curKeyStr, &vectorBlock{
					startKey: curKey,
					endKey:   lastKey,
					entries:  entries,
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("badger index search: %w", err)
	}

	if truncated {
		log.Warn().Int("scanned", scanned).Int("limit", maxScanVectors).Msg("[rag] badgerIndex search truncated: reached scan limit")
	}

	// 从堆中提取结果，按分数降序排列
	result := make([]scoredResult, h.Len())
	for i := h.Len() - 1; i >= 0; i-- {
		result[i] = heap.Pop(h).(scoredResult)
	}

	out := make([]SearchResult, len(result))
	for i, r := range result {
		out[i] = SearchResult{ID: r.id, Score: r.score, Metadata: make(map[string]string)}
	}
	return out, nil
}

// nextBadgerKey 返回严格大于 key 的最小 Badger key(追加 \x00 字节)。
// 用于缓存命中后 Seek 跳过已缓存的块。
func nextBadgerKey(key []byte) []byte {
	result := make([]byte, len(key)+1)
	copy(result, key)
	result[len(key)] = 0x00
	return result
}

// Close 实现 vectorIndex 接口。badgerIndex 不持有需释放的资源
// （db 由 VectorStore.Close 统一管理），此处为 no-op。
func (bi *badgerIndex) Close() error {
	return nil
}

// scoredResult 用于 badgerIndex 堆排序的 id-分数对
type scoredResult struct {
	id    string
	score float64
}

// minHeapResult 实现最小堆接口，用于 badgerIndex 维护 topK 结果
type minHeapResult []scoredResult

func (h minHeapResult) Len() int           { return len(h) }
func (h minHeapResult) Less(i, j int) bool { return h[i].score < h[j].score } // 最小堆
func (h minHeapResult) Swap(i, j int)      { h[i], h[j] = h[j], h[i] }
func (h *minHeapResult) Push(x any) {
	*h = append(*h, x.(scoredResult))
}
func (h *minHeapResult) Pop() any {
	old := *h
	n := len(old)
	x := old[n-1]
	*h = old[:n-1]
	return x
}

// Peek 返回堆顶元素但不移除
func (h *minHeapResult) Peek() scoredResult {
	if len(*h) == 0 {
		return scoredResult{}
	}
	return (*h)[0]
}

// NewVectorStore opens (or creates) a Badger-backed vector store at dataDir.
// An empty dataDir creates an in-memory store (useful for testing).
func NewVectorStore(dataDir string) (*VectorStore, error) {
	opts := badger.DefaultOptions(dataDir)
	opts.Logger = &badgerLogAdapter{}

	if dataDir == "" {
		opts = badger.DefaultOptions("").WithInMemory(true)
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open badger: %w", err)
	}

	vs := &VectorStore{
		db:            db,
		bm25Indexes:   make(map[string]*BM25Index),
		locks:         make(map[string]*sync.Mutex),
		indexes:       make(map[string]vectorIndex),
		badgerIndexes: make(map[string]*badgerIndex),
	}

	// 从 Badger 中重建 BM25 索引（程序重启后保持混合检索能力）
	vs.rebuildBM25Index()

	return vs, nil
}

// rebuildBM25Index 从 Badger 中加载所有已存储的 chunk 文本，按 collection 分组重建 BM25 索引
// 每个 collection 拥有独立的 BM25Index，doc id 统一使用 "docID_chunkIdx" 格式（不含 collection 前缀），
// 从根上消除了两种 id 格式并存的问题，也避免了跨 collection 的 id 碰撞。
func (vs *VectorStore) rebuildBM25Index() {
	prefix := []byte("chunk:")
	// 按 collection 分组收集 (id, content)，便于后续对每个 collection 调用一次 AddDocuments
	grouped := make(map[string][]BM25DocInput)
	// 旧格式兜底检测：若发现历史残留的 "collection:docID_chunkIdx" 形态 id，记录警告
	legacyCount := 0
	err := vs.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			// key 形如 "chunk:<collection>:<docID_chunkIdx>"，去掉 "chunk:" 前缀后得到 "<collection>:<docID_chunkIdx>"
			rest := string(item.Key()[len(prefix):])
			// collection 名不含冒号（CreateCollection 已校验），按第一个冒号切分
			before, after, ok := strings.Cut(rest, ":")
			if !ok {
				legacyCount++
				continue
			}
			collection := before
			id := after
			val, err := item.ValueCopy(nil)
			if err != nil {
				continue
			}
			content := string(val)
			if content == "" {
				continue
			}
			grouped[collection] = append(grouped[collection], BM25DocInput{ID: id, Text: content})
		}
		return nil
	})
	if err != nil {
		log.Warn().Err(err).Msg("[rag] failed to rebuild BM25 index")
		return
	}
	// 旧格式残留告警（per-collection 重建后 BM25 内部不再有 collection 前缀，此处仅为数据观测）
	if legacyCount > 0 {
		log.Warn().Int("legacy", legacyCount).Msg("[rag] BM25 rebuild skipped keys without collection separator")
	}
	total := 0
	for collection, docs := range grouped {
		vs.getOrCreateBM25(collection).AddDocuments(docs)
		total += len(docs)
	}
	if total > 0 {
		log.Info().Int("count", total).Msg("[rag] BM25 index rebuilt from Badger")
	}
}

// getOrCreateBM25 返回指定 collection 的 BM25 索引，不存在则创建。
// 访问 vs.bm25Indexes 受 vs.mu 保护；BM25Index 自身的并发安全由其内部 mu 保证。
func (vs *VectorStore) getOrCreateBM25(name string) *BM25Index {
	vs.mu.RLock()
	idx, ok := vs.bm25Indexes[name]
	vs.mu.RUnlock()
	if ok {
		return idx
	}
	vs.mu.Lock()
	defer vs.mu.Unlock()
	// 双重检查：避免并发时重复创建
	if idx, ok := vs.bm25Indexes[name]; ok {
		return idx
	}
	idx = NewBM25Index()
	vs.bm25Indexes[name] = idx
	return idx
}

// ---------------------------------------------------------------------------
// Key helpers
// ---------------------------------------------------------------------------

func collectionKey(name string) []byte { return []byte("collection:" + name) }
func hnswKey(name string) []byte       { return []byte("hnsw:" + name) }
func vectorKey(collection, id string) []byte {
	return []byte("vector:" + collection + ":" + id)
}

func chunkKey(collection, id string) []byte {
	return []byte("chunk:" + collection + ":" + id)
}

func chunkMetaKey(collection, id string) []byte {
	return []byte("chunkmeta:" + collection + ":" + id)
}

func metaToBytes(m collectionMeta) ([]byte, error) {
	buf := bytes.NewBuffer(nil)
	if err := binary.Write(buf, binary.LittleEndian, m.Dim); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.LittleEndian, m.VectorCount); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func bytesToMeta(b []byte) (collectionMeta, error) {
	r := bytes.NewReader(b)
	var m collectionMeta
	if err := binary.Read(r, binary.LittleEndian, &m.Dim); err != nil {
		return m, err
	}
	if err := binary.Read(r, binary.LittleEndian, &m.VectorCount); err != nil {
		return m, err
	}
	return m, nil
}

func vectorToBytes(v []float64) []byte {
	buf := make([]byte, len(v)*8)
	for i, f := range v {
		binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(f))
	}
	return buf
}

func bytesToVector(b []byte, dim int) ([]float64, error) {
	if len(b) != dim*8 {
		return nil, fmt.Errorf("expected %d bytes, got %d", dim*8, len(b))
	}
	v := make([]float64, dim)
	if err := binary.Read(bytes.NewReader(b), binary.LittleEndian, v); err != nil {
		return nil, fmt.Errorf("failed to decode vector: %w", err)
	}
	return v, nil
}

// collectionLock returns the mutex for a collection, creating it if needed.
// 任务 5：若 store 已 Close（vs.locks 为 nil），写入会 panic。
// 此时返回一个新的临时 mutex（不写入 map），调用方加锁/解锁仍能正常工作，
// 后续对 vs.db 的操作会因 db 已关闭而返回错误，从而安全失败而非 panic。
func (vs *VectorStore) collectionLock(name string) *sync.Mutex {
	if vs.closed.Load() {
		return &sync.Mutex{}
	}
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if vs.locks == nil {
		// 双重检查：Close 可能在 atomic.Load 之后、加锁之前发生
		return &sync.Mutex{}
	}
	if vs.locks[name] == nil {
		vs.locks[name] = &sync.Mutex{}
	}
	return vs.locks[name]
}

// maxInMemoryVectors 是内存索引的上限（任务 30）。
// 超过此阈值时切换为 badgerIndex（分批扫描 Badger，不在内存常驻全部向量），
// 避免 RAG 子系统内存随知识库线性增长导致 OOM。D=768 时 50000 向量约 300MB。
// 改为 var 便于测试中临时调低阈值以触发 badgerIndex 路径。
var maxInMemoryVectors = 50000

// getOrLoadIndex returns the live index for a collection, building it from
// Badger on first access. 按 maxInMemoryVectors 阈值选择实现（任务 30）：
//   - 向量数 > 阈值：返回 badgerIndex（磁盘扫描，避免 OOM）
//   - 向量数 <= 阈值：返回 memIndex（全量常驻内存，检索快）
//
// 任务 3:两阶段加锁 —— Phase 1 在 vs.mu 内仅注册占位 memIndex 并释放锁,
// Phase 2 在不持有 vs.mu 的情况下扫描 Badger 填充 memIndex,
// Phase 3 标记就绪。这样 collection A 构建索引时不会阻塞 collection B 的检索。
func (vs *VectorStore) getOrLoadIndex(name string) (vectorIndex, error) {
	vs.mu.RLock()
	idx, ok := vs.indexes[name]
	vs.mu.RUnlock()
	if ok {
		return idx, nil
	}

	vs.mu.Lock()

	// Double-check with write lock.
	if idx, ok := vs.indexes[name]; ok {
		vs.mu.Unlock()
		return idx, nil
	}

	// Load collection metadata to get dimension and vector count.
	meta, err := vs.getCollectionMeta(name)
	if err != nil {
		vs.mu.Unlock()
		return nil, err
	}

	// 任务 30：超过阈值时改用 badgerIndex，避免全量加载导致 OOM
	if meta.VectorCount > int64(maxInMemoryVectors) {
		log.Info().
			Str("collection", name).
			Int64("vector_count", meta.VectorCount).
			Int("threshold", maxInMemoryVectors).
			Msg("[rag] using badgerIndex (disk scan) to avoid high memory usage")
		bi := vs.getOrCreateBadgerIndex(name, int(meta.Dim))
		vs.indexes[name] = bi
		vs.mu.Unlock()
		return bi, nil
	}

	// 任务 3:Phase 1 —— 创建空 memIndex 占位并注册到 map,释放 vs.mu
	mi := newMemIndex(int(meta.Dim))
	mi.ready = make(chan struct{})
	vs.indexes[name] = mi
	vs.mu.Unlock()

	// 任务 3:Phase 2 —— 在不持有 vs.mu 的情况下扫描 Badger 填充 memIndex
	prefix := []byte("vector:" + name + ":")
	buildErr := vs.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			vecBytes, err := item.ValueCopy(nil)
			if err != nil {
				log.Warn().Err(err).Str("key", string(item.Key())).Msg("skipping malformed vector")
				continue
			}
			vec, err := bytesToVector(vecBytes, int(meta.Dim))
			if err != nil {
				log.Warn().Err(err).Str("key", string(item.Key())).Msg("skipping malformed vector")
				continue
			}
			id := string(item.Key()[len(prefix):])
			mi.insert(id, vec)
		}
		return nil
	})

	// 任务 3:Phase 3 —— 标记构建完成,唤醒所有 waitReady 等待者
	mi.buildErr = buildErr
	close(mi.ready)

	if buildErr != nil {
		// 构建失败:从 map 中移除占位,允许后续重试
		vs.mu.Lock()
		delete(vs.indexes, name)
		vs.mu.Unlock()
		return nil, fmt.Errorf("rebuild index from badger: %w", buildErr)
	}

	return mi, nil
}

// getOrCreateBadgerIndex 返回指定 collection 的 badgerIndex（无缓存则创建）。
// badgerIndex 不持有向量数据，仅持有 db 引用和前缀，创建开销极低；
// 缓存在 vs.badgerIndexes 中，DeleteCollection 时统一清除。
// 调用方需持有 vs.mu 写锁。
func (vs *VectorStore) getOrCreateBadgerIndex(name string, dim int) *badgerIndex {
	if idx, ok := vs.badgerIndexes[name]; ok {
		return idx
	}
	idx := &badgerIndex{
		db:     vs.db,
		prefix: []byte("vector:" + name + ":"),
		dim:    dim,
		cache:  newVectorLRU(lruCapacity), // 任务 6:初始化 LRU 缓存
	}
	vs.badgerIndexes[name] = idx
	return idx
}

func (vs *VectorStore) getCollectionMeta(name string) (collectionMeta, error) {
	var meta collectionMeta
	err := vs.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(collectionKey(name))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrCollectionNotFound
		}
		if err != nil {
			return err
		}
		b, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		meta, err = bytesToMeta(b)
		return err
	})
	return meta, err
}

// updateCollectionDim updates the dimension of an existing collection.
// 任务 8（P2-4）：校验 dim > 0，防止 0 或负数被写入 collectionMeta，
// 否则后续 bytesToVector(dim*8) 会按错误维度解码，导致检索结果错乱。
func (vs *VectorStore) updateCollectionDim(name string, dim int32) error {
	if dim <= 0 {
		return ErrZeroDimension
	}
	return vs.db.Update(func(txn *badger.Txn) error {
		item, err := txn.Get(collectionKey(name))
		if err != nil {
			return err
		}
		b, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		meta, err := bytesToMeta(b)
		if err != nil {
			return err
		}
		meta.Dim = dim
		data, err := metaToBytes(meta)
		if err != nil {
			return err
		}
		return txn.Set(collectionKey(name), data)
	})
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// CreateCollection creates a new named collection with the given vector dimension.
// Returns ErrCollectionExists if the collection already exists.
func (vs *VectorStore) CreateCollection(name string, dim int) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrEmptyCollectionName
	}
	// 拒绝包含键前缀分隔符的字符（冒号用于 Badger KV 键前缀，斜杠可能导致歧义）
	if strings.ContainsAny(name, ":/\\") {
		return ErrInvalidCollectionName
	}

	err := vs.db.Update(func(txn *badger.Txn) error {
		_, err := txn.Get(collectionKey(name))
		if err == nil {
			return ErrCollectionExists
		}
		if !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		meta := collectionMeta{Dim: int32(dim), VectorCount: 0}
		data, err := metaToBytes(meta)
		if err != nil {
			return fmt.Errorf("serialise meta: %w", err)
		}
		return txn.Set(collectionKey(name), data)
	})
	if err != nil {
		return fmt.Errorf("create collection %q: %w", name, err)
	}

	log.Info().Str("collection", name).Int("dim", dim).Msg("collection created")
	return nil
}

// AddVectors inserts vectors into the named collection.
// ids and vectors must have the same length. Each id should be unique within
// the collection. Returns ErrCollectionNotFound, ErrVectorDimMismatch, or
// ErrEmptyVector.
//
// 安全说明（任务 13）：本方法获取 collectionLock 后委托给 addVectorsCore，
// 以保证 memIndex 插入与并发 Search/DeleteDocument 互斥。IngestDocumentWithMeta
// 在已持有 collectionLock 的场景下应直接调用 addVectorsCore，避免重复加锁导致死锁。
func (vs *VectorStore) AddVectors(collection string, ids []string, vectors [][]float64) error {
	// 任务 5：Close 后 locks/bm25Indexes 等已被置 nil，直接操作会 panic。
	if vs.closed.Load() {
		return ErrClosed
	}
	mu := vs.collectionLock(collection)
	mu.Lock()
	defer mu.Unlock()
	return vs.addVectorsCore(collection, ids, vectors)
}

// addVectorsCore 执行 AddVectors 的全部实际逻辑（校验、Badger 写入、memIndex 插入、BM25 更新），
// 但不获取 collectionLock，调用方必须自行持有 collectionLock。
// 抽取此 helper 是为了让 IngestDocumentWithMeta 能在已持有 collectionLock 的情况下
// 复用本逻辑，将整个文档摄入纳入单次锁保护，避免与并发 DeleteDocument 产生孤立数据（任务 13）。
func (vs *VectorStore) addVectorsCore(collection string, ids []string, vectors [][]float64) error {
	if len(ids) != len(vectors) {
		return fmt.Errorf("ids (%d) and vectors (%d) must have the same length", len(ids), len(vectors))
	}
	if len(ids) == 0 {
		return nil
	}

	meta, err := vs.getCollectionMeta(collection)
	if err != nil {
		return err
	}
	dim := int(meta.Dim)

	// 如果集合维度为 0（旧数据或初始化时未确定维度），自动更新为实际向量维度
	if dim == 0 && len(vectors) > 0 && len(vectors[0]) > 0 {
		dim = len(vectors[0])
		if err := vs.updateCollectionDim(collection, int32(dim)); err != nil {
			return fmt.Errorf("update collection dim: %w", err)
		}
	}

	// Validate all vectors before any write.
	for i, v := range vectors {
		if len(v) == 0 {
			return fmt.Errorf("%w: vector at index %d is empty", ErrEmptyVector, i)
		}
		if len(v) != dim {
			return fmt.Errorf("%w: vector at index %d has dim %d, expected %d",
				ErrVectorDimMismatch, i, len(v), dim)
		}
	}

	// Persist to Badger.
	err = vs.db.Update(func(txn *badger.Txn) error {
		for i, id := range ids {
			if err := txn.Set(vectorKey(collection, id), vectorToBytes(vectors[i])); err != nil {
				return fmt.Errorf("store vector %q: %w", id, err)
			}
		}
		meta.VectorCount += int64(len(vectors))
		data, err := metaToBytes(meta)
		if err != nil {
			return err
		}
		return txn.Set(collectionKey(collection), data)
	})
	if err != nil {
		return fmt.Errorf("badger update: %w", err)
	}

	// 更新内存索引（仅 memIndex 模式；badgerIndex 模式下数据已通过 db.Update 写入 Badger）
	idx, err := vs.getOrLoadIndex(collection)
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}
	// 注意：此处不再获取 collectionLock，由调用方（AddVectors 或 IngestDocumentWithMeta）持有
	if mi, ok := idx.(*memIndex); ok {
		// 任务 3:等待 memIndex 构建完成后再插入,避免与并发构建产生重复向量
		if err := mi.waitReady(); err != nil {
			return fmt.Errorf("memIndex build failed: %w", err)
		}
		for i := range ids {
			mi.insert(ids[i], vectors[i])
		}
	}
	// 任务 6:badgerIndex 模式下,新增向量使缓存失效,下次 Search 重新从 Badger 加载
	if bi, ok := idx.(*badgerIndex); ok && bi.cache != nil {
		bi.cache.clear()
	}

	// 同步更新 BM25 索引：用一个 db.View 批量读取所有 chunk 文本，再调用 AddDocuments 一次性写入
	// 修复（M-后4）：原实现循环调用 loadChunkContent，每次都开启独立 db.View 事务，
	// N 个向量 = N 次独立事务，开销显著放大。改为单事务批量读取。
	// 修复（任务4）：原实现循环调用 AddDocument，每次都 O(N) 重算 IDF，退化为 O(N²)；
	// 改为收集到切片后调用 AddDocuments，仅在末尾单次重算 avgDL 和 IDF。
	bm25Docs := make([]BM25DocInput, 0, len(ids))
	err = vs.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			item, err := txn.Get(chunkKey(collection, id))
			if err != nil {
				if errors.Is(err, badger.ErrKeyNotFound) {
					continue // 无 chunk 文本的向量（如纯向量检索场景），跳过
				}
				continue // 其他错误也跳过，避免阻塞向量写入主流程
			}
			if err := item.Value(func(val []byte) error {
				content := string(val)
				if content != "" {
					bm25Docs = append(bm25Docs, BM25DocInput{ID: id, Text: content})
				}
				return nil
			}); err != nil {
				continue
			}
		}
		return nil
	})
	if err != nil {
		// BM25 更新失败不影响向量写入主流程，仅记录日志
		log.Warn().Err(err).Str("collection", collection).Msg("[rag] batch load chunk content for BM25 failed")
	}
	// 批量写入 BM25（doc id 统一为 "docID_chunkIdx"，与 collection 隔离索引配合）
	if len(bm25Docs) > 0 {
		vs.getOrCreateBM25(collection).AddDocuments(bm25Docs)
	}

	log.Info().Str("collection", collection).Int("count", len(vectors)).Msg("vectors added")
	return nil
}

// Search finds the topK most similar vectors to the query in the collection.
// Uses cosine similarity; higher scores (closer to 1.0) mean more similar.
// Returns ErrCollectionNotFound or ErrVectorDimMismatch.
// ctx 用于传播取消信号（如用户关闭应用、取消请求），检索仍受 5 秒上限保护
func (vs *VectorStore) Search(ctx context.Context, collection string, query []float64, topK int) ([]SearchResult, error) {
	// 任务 5：Close 后 collectionLock/getOrLoadIndex 会因 nil map panic。
	if vs.closed.Load() {
		return nil, ErrClosed
	}
	if len(query) == 0 {
		return nil, ErrEmptyVector
	}
	if topK <= 0 {
		topK = 10
	}

	// 优先检查 ctx 是否已取消，避免无谓的锁获取和索引加载
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	meta, err := vs.getCollectionMeta(collection)
	if err != nil {
		return nil, err
	}
	if len(query) != int(meta.Dim) {
		return nil, fmt.Errorf("%w: query dim %d, expected %d", ErrVectorDimMismatch, len(query), meta.Dim)
	}

	// 修复 #12: 在调用 getOrLoadIndex 之前获取 collectionLock，消除
	//   "getOrLoadIndex 释放 vs.mu 后、collectionLock.Lock 前" 的竞态窗口
	//   （并发 AddVectors 可能使索引引用指向旧对象）。
	// 修复 #9:  锁内只执行 idx.Search 获取 ID 列表，db.View 批量读取
	//   chunk 内容移出 collectionLock，避免锁持有时间过长。
	// 锁顺序 collectionLock → vs.mu 与 AddVectors/DeleteDocument 一致，无死锁风险。
	mu := vs.collectionLock(collection)
	mu.Lock()

	idx, err := vs.getOrLoadIndex(collection)
	if err != nil {
		mu.Unlock()
		return nil, err
	}

	// 任务 34:为索引检索设置 5 秒超时,防止单次检索耗时过长
	// 修复 C1: 用传入的 ctx 派生超时 ctx，使取消信号能传播到索引检索
	searchCtx, searchCancel := context.WithTimeout(ctx, 5*time.Second)
	// 通过 vectorIndex 接口检索，memIndex 和 badgerIndex 均返回带 ID 的 SearchResult
	out, err := idx.Search(searchCtx, query, topK)
	searchCancel()
	mu.Unlock()

	if err != nil {
		return nil, fmt.Errorf("search index: %w", err)
	}

	// 批量读取所有 chunk 内容和 metadata，避免 N+1 查询（已移出 collectionLock）。
	// 生活类比：先在锁保护下从索引柜查到书目编号清单，再带着清单去书架慢慢取书，
	// 不必一直占着索引柜。结果按 out 的原顺序回填，scores 保持对齐。
	if len(out) > 0 {
		ids := make([]string, len(out))
		for i := range out {
			ids[i] = out[i].ID
		}
		contents, metas, loadErr := vs.loadChunksBatch(collection, ids)
		if loadErr == nil {
			for i := range out {
				if content, ok := contents[out[i].ID]; ok {
					out[i].ChunkContent = content
				}
				if m, ok := metas[out[i].ID]; ok {
					out[i].Metadata = m
				}
			}
		}
	}

	log.Debug().Str("collection", collection).Int("topK", topK).Int("found", len(out)).Msg("search complete")
	return out, nil
}

// loadChunksBatch 在单个 db.View 事务内批量加载多个 chunk 的文本内容和元数据。
// 生活类比：像去图书馆一次性借一摞书，而不是每借一本就跑一趟柜台。
// 返回的 maps 以 id 为 key；未找到的 id 不会出现在返回的 map 中（不视为错误）。
// 用于替代 HybridSearch 中 N 次 loadChunkContent/loadChunkMeta 调用，减少事务开销。
func (vs *VectorStore) loadChunksBatch(collection string, ids []string) (contents map[string]string, metas map[string]map[string]string, err error) {
	contents = make(map[string]string, len(ids))
	metas = make(map[string]map[string]string, len(ids))
	if len(ids) == 0 {
		return contents, metas, nil
	}
	err = vs.db.View(func(txn *badger.Txn) error {
		for _, id := range ids {
			// 读取 chunk 文本
			if item, getErr := txn.Get(chunkKey(collection, id)); getErr == nil {
				if val, valErr := item.ValueCopy(nil); valErr == nil {
					contents[id] = string(val)
				}
			} else if !errors.Is(getErr, badger.ErrKeyNotFound) {
				// 非 "键不存在" 的错误记录日志，但不中断批量读取
				log.Debug().Err(getErr).Str("collection", collection).Str("id", id).Msg("[rag] loadChunksBatch: read chunk content failed")
			}
			// 读取 chunk 元数据
			if item, getErr := txn.Get(chunkMetaKey(collection, id)); getErr == nil {
				if val, valErr := item.ValueCopy(nil); valErr == nil && len(val) > 0 {
					var m map[string]string
					if jsonErr := json.Unmarshal(val, &m); jsonErr == nil {
						metas[id] = m
					}
				}
			}
		}
		return nil
	})
	return contents, metas, err
}

// DeleteCollection removes a collection and all its data from Badger.
// The in-memory index entry and BM25 documents are also cleared.
//
// 安全修复（M-后5）：原实现只删除 vector: 和 collection:/hnsw: 键，
// 遗漏了 chunk: 和 chunkmeta: 前缀的键，导致 RAG 文本块和元数据残留；
// 同时未清理 BM25 索引中对应文档，造成关键词检索返回已删除内容。

// deleteByPrefix 在事务内删除所有匹配指定前缀的键
// 安全实践（基于 B-1.21）：统一 DeleteCollection 中 vector:/chunk:/chunkmeta: 三处重复的前缀键删除逻辑
// 返回删除的键数量，便于调用方日志记录
// 调用前提：已持有 txn 写事务
func deleteByPrefix(txn *badger.Txn, prefix []byte) (int, error) {
	it := txn.NewIterator(badger.DefaultIteratorOptions)
	var keys [][]byte
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		keys = append(keys, it.Item().KeyCopy(nil))
	}
	it.Close()
	for _, k := range keys {
		if err := txn.Delete(k); err != nil {
			return 0, fmt.Errorf("delete key with prefix %q: %w", string(prefix), err)
		}
	}
	return len(keys), nil
}

func (vs *VectorStore) DeleteCollection(name string) error {
	// 任务 5：Close 后 delete(vs.indexes, ...) 等会因 nil map panic。
	if vs.closed.Load() {
		return ErrClosed
	}
	_, err := vs.getCollectionMeta(name)
	if err != nil {
		return err
	}

	// Remove from memory.
	// BM25 索引按 collection 隔离，直接丢弃整个索引即可，无需逐个清理 doc id。
	// badgerIndex 缓存也需清除，避免引用已删除 collection 的前缀（任务 30）。
	vs.mu.Lock()
	delete(vs.indexes, name)
	delete(vs.locks, name)
	delete(vs.bm25Indexes, name)
	delete(vs.badgerIndexes, name)
	vs.mu.Unlock()

	// Remove all keys from Badger.
	err = vs.db.Update(func(txn *badger.Txn) error {
		if err := txn.Delete(collectionKey(name)); err != nil {
			return err
		}
		if err := txn.Delete(hnswKey(name)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}

		// 删除 vector:/chunk:/chunkmeta: 前缀键（统一调用 deleteByPrefix）
		// 安全实践（基于 B-1.21）：消除三处重复的"Seek→收集 keys→删除"代码块
		if _, err := deleteByPrefix(txn, []byte("vector:"+name+":")); err != nil {
			return err
		}
		if _, err := deleteByPrefix(txn, []byte("chunk:"+name+":")); err != nil {
			return err
		}
		if _, err := deleteByPrefix(txn, []byte("chunkmeta:"+name+":")); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete collection %q: %w", name, err)
	}

	// BM25 索引已在内存清理阶段整体丢弃（per-collection 隔离），此处无需再处理

	log.Info().Str("collection", name).Msg("collection deleted")
	return nil
}

// ListCollections returns summary information for all collections.
func (vs *VectorStore) ListCollections() ([]CollectionInfo, error) {
	var result []CollectionInfo
	prefix := []byte("collection:")
	err := vs.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			name := string(item.Key()[len(prefix):])
			b, err := item.ValueCopy(nil)
			if err != nil {
				log.Warn().Err(err).Str("collection", name).Msg("skipping malformed collection meta")
				continue
			}
			meta, err := bytesToMeta(b)
			if err != nil {
				log.Warn().Err(err).Str("collection", name).Msg("skipping malformed collection meta")
				continue
			}
			result = append(result, CollectionInfo{
				Name:        name,
				Dim:         int(meta.Dim),
				VectorCount: meta.VectorCount,
			})
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list collections: %w", err)
	}
	if result == nil {
		result = []CollectionInfo{}
	}
	return result, nil
}

// SearchWithThreshold finds the topK most similar vectors to the query,
// but only returns results with a cosine similarity score >= minScore.
// ctx 用于传播取消信号
func (vs *VectorStore) SearchWithThreshold(ctx context.Context, collection string, query []float64, topK int, minScore float64) ([]SearchResult, error) {
	// 任务 5：Close 后委托的 Search 会返回 ErrClosed，此处前置检查避免无谓调用。
	if vs.closed.Load() {
		return nil, ErrClosed
	}
	all, err := vs.Search(ctx, collection, query, topK)
	if err != nil {
		return nil, err
	}
	var filtered []SearchResult
	for _, r := range all {
		if r.Score >= minScore {
			filtered = append(filtered, r)
		}
	}
	if filtered == nil {
		filtered = []SearchResult{}
	}
	return filtered, nil
}

// DeleteDocument removes all vectors and chunk data for a specific document
// from a collection. The docID prefix is used to identify the document's chunks.
//
// 安全修复（M-后6）：
//  1. 原实现在 db.Update 事务内调用 vs.getCollectionMeta（内部又开启 db.View），
//     嵌套事务会触发 Badger OCC 冲突导致 VectorCount 漂移。改为在同一事务内
//     用 txn.Get 直接读取 collection meta，使读写参与同一冲突重试周期。
//  2. 原实现遗漏 chunkmeta: 前缀键的删除，导致 chunk 元数据残留。
//  3. 原实现未清理 BM25 索引，导致关键词检索返回已删除文档内容。
//  4. 安全修复（S1）：原实现仅用 strings.HasPrefix(id, docID+"_") 匹配，当某 docID
//     是另一 docID 的前缀时（如 "doc" 和 "doc_1"），删除 "doc" 会误删 "doc_1" 的
//     所有 chunk。改为用 parseChunkID 精确提取 chunk ID 所属的 docID 后再比对。
func (vs *VectorStore) DeleteDocument(collection string, docID string) error {
	// 任务 5：Close 后 collectionLock/索引清理会因 nil map panic。
	if vs.closed.Load() {
		return ErrClosed
	}
	// 任务 13：获取 collectionLock，与 IngestDocumentWithMeta 互斥，
	// 避免并发摄入与删除产生孤立数据（向量无对应文本 或 文本无对应向量）
	mu := vs.collectionLock(collection)
	mu.Lock()
	defer mu.Unlock()

	_, err := vs.getCollectionMeta(collection)
	if err != nil {
		return err
	}

	var deletedCount int64
	// 收集被删除的 vector id（用于事务后清理 BM25 索引）
	var deletedIDs []string

	// matchDocID 精确判断 chunk ID 是否属于指定 docID。
	// 用 parseChunkID 提取最后一个 "_" 之前的部分作为 docID，避免前缀冲突误删
	// （如 docID="doc" 不应匹配 "doc_1_0"，因为 parseChunkID("doc_1_0") = "doc_1"）。
	matchDocID := func(id string) bool {
		parsedDocID, _ := parseChunkID(id)
		return parsedDocID == docID
	}

	err = vs.db.Update(func(txn *badger.Txn) error {
		vecPrefix := []byte("vector:" + collection + ":")
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		var vecKeys [][]byte
		for it.Seek(vecPrefix); it.ValidForPrefix(vecPrefix); it.Next() {
			item := it.Item()
			key := item.Key()
			id := key[len(vecPrefix):]
			if matchDocID(string(id)) {
				vecKeys = append(vecKeys, item.KeyCopy(nil))
				deletedIDs = append(deletedIDs, string(id))
			}
		}
		it.Close()
		for _, k := range vecKeys {
			if err := txn.Delete(k); err != nil {
				return fmt.Errorf("delete vector key: %w", err)
			}
			deletedCount++
		}

		chunkPrefix := []byte("chunk:" + collection + ":")
		it2 := txn.NewIterator(badger.DefaultIteratorOptions)
		var chunkKeys [][]byte
		for it2.Seek(chunkPrefix); it2.ValidForPrefix(chunkPrefix); it2.Next() {
			item := it2.Item()
			key := item.Key()
			id := key[len(chunkPrefix):]
			if matchDocID(string(id)) {
				chunkKeys = append(chunkKeys, item.KeyCopy(nil))
			}
		}
		it2.Close()
		for _, k := range chunkKeys {
			if err := txn.Delete(k); err != nil {
				return fmt.Errorf("delete chunk key: %w", err)
			}
		}

		// 删除 chunkmeta: 前缀键（原实现遗漏，导致 chunk 元数据残留）
		chunkMetaPrefix := []byte("chunkmeta:" + collection + ":")
		it3 := txn.NewIterator(badger.DefaultIteratorOptions)
		var metaKeys [][]byte
		for it3.Seek(chunkMetaPrefix); it3.ValidForPrefix(chunkMetaPrefix); it3.Next() {
			item := it3.Item()
			key := item.Key()
			id := key[len(chunkMetaPrefix):]
			if matchDocID(string(id)) {
				metaKeys = append(metaKeys, item.KeyCopy(nil))
			}
		}
		it3.Close()
		for _, k := range metaKeys {
			if err := txn.Delete(k); err != nil {
				return fmt.Errorf("delete chunkmeta key: %w", err)
			}
		}

		// 在同一事务内读取 collection meta，避免嵌套事务引发 OCC 冲突（M-后6）
		item, err := txn.Get(collectionKey(collection))
		if errors.Is(err, badger.ErrKeyNotFound) {
			return ErrCollectionNotFound
		}
		if err != nil {
			return err
		}
		b, err := item.ValueCopy(nil)
		if err != nil {
			return err
		}
		meta, err := bytesToMeta(b)
		if err != nil {
			return err
		}
		meta.VectorCount -= deletedCount
		if meta.VectorCount < 0 {
			meta.VectorCount = 0
		}
		data, err := metaToBytes(meta)
		if err != nil {
			return err
		}
		return txn.Set(collectionKey(collection), data)
	})
	if err != nil {
		return fmt.Errorf("delete document %q from %q: %w", docID, collection, err)
	}

	// 事务成功后清理 BM25 索引
	// BM25 按 collection 隔离且 doc id 统一为 "docID_chunkIdx"（无 collection 前缀），
	// 因此只需单格式 idSet，直接在该 collection 的索引上删除。
	if len(deletedIDs) > 0 {
		idSet := make(map[string]bool, len(deletedIDs))
		for _, id := range deletedIDs {
			idSet[id] = true
		}
		removed := vs.getOrCreateBM25(collection).RemoveDocuments(idSet)
		if removed > 0 {
			log.Debug().Str("collection", collection).Str("docID", docID).Int("bm25_removed", removed).Msg("BM25 docs cleaned up")
		}
	}

	vs.mu.Lock()
	if idx, ok := vs.indexes[collection]; ok {
		// 仅 memIndex 需要更新内存数组；badgerIndex 无内存缓存，下次 Search 自动从 Badger 读取最新数据
		if mi, ok := idx.(*memIndex); ok {
			mi.vecMu.Lock()
			var newVecs [][]float64
			var newIDs []string
			for i, id := range mi.ids {
				if !matchDocID(id) {
					newVecs = append(newVecs, mi.vecs[i])
					newIDs = append(newIDs, id)
				}
			}
			mi.vecs = newVecs
			mi.ids = newIDs
			mi.vecMu.Unlock()
		}
		// 任务 6:badgerIndex 模式下,删除向量使缓存失效,下次 Search 重新从 Badger 加载
		if bi, ok := idx.(*badgerIndex); ok && bi.cache != nil {
			bi.cache.clear()
		}
	}
	vs.mu.Unlock()

	log.Info().Str("collection", collection).Str("docID", docID).Int64("deleted", deletedCount).Msg("document deleted")
	return nil
}

// WithTx 在 Badger 读写事务中执行 fn，委托给 db.Update。
// 外部调用方（如 DocumentStore）应通过此方法访问 Badger，
// 避免直接暴露 *badger.DB 句柄导致封装被绕过。
// 生活类比：像银行柜员代办业务，你把要办的事（fn）交给柜员，
// 由柜员在保险库（事务）里操作，你不需要自己拿保险库钥匙。
func (vs *VectorStore) WithTx(fn func(*badger.Txn) error) error {
	return vs.db.Update(fn)
}

func (vs *VectorStore) Close() error {
	// 任务 5：先标记 closed，使并发 goroutine 在 collectionLock / 公共方法入口
	// 检测到 closed 后短路返回，避免对随后被置 nil 的 map 写入导致 panic。
	vs.closed.Store(true)
	vs.mu.Lock()
	// 任务 15:关闭所有 index 资源(memIndex 无外部资源,badgerIndex 也为 no-op,
	// 但通过接口统一调用 Close 确保未来扩展时资源不泄漏)
	for _, idx := range vs.indexes {
		if idx != nil {
			_ = idx.Close()
		}
	}
	vs.indexes = nil
	vs.locks = nil
	vs.bm25Indexes = nil
	vs.badgerIndexes = nil
	vs.mu.Unlock()
	return vs.db.Close()
}

// ---------------------------------------------------------------------------
// Badger log adapter — routes Badger logs into zerolog.
// ---------------------------------------------------------------------------

type badgerLogAdapter struct{}

func (*badgerLogAdapter) Errorf(f string, v ...any)   { log.Error().Msgf("[badger] "+f, v...) }
func (*badgerLogAdapter) Warningf(f string, v ...any) { log.Warn().Msgf("[badger] "+f, v...) }
func (*badgerLogAdapter) Infof(f string, v ...any)    { log.Info().Msgf("[badger] "+f, v...) }
func (*badgerLogAdapter) Debugf(f string, v ...any)   { log.Debug().Msgf("[badger] "+f, v...) }
