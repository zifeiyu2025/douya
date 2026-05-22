// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package rag

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

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

// SearchResult represents a single search result.
type SearchResult struct {
	ID           string  // the stored vector ID
	Score        float64 // cosine similarity score (0-1, higher = more similar)
	ChunkContent string  // the original chunk text (loaded after search)
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
	db  *badger.DB
	cfg HNSWConfig

	// Per-collection state. Access is protected by the collection-level lock map.
	mu     sync.RWMutex
	locks  map[string]*sync.Mutex // collection name → lock
	indexes map[string]*memIndex // collection name → in-memory index (nil until first load)
}

// memIndex is an in-memory vector index backed by Badger.
// It stores vectors as []float64 and searches by cosine similarity.
type memIndex struct {
	dim   int
	vecs  [][]float64 // indexed vectors, position i = id i
	ids   []string    // id for each position
	vecMu sync.RWMutex
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

// search finds the topK most similar vectors to the query using cosine similarity.
// Returns positions and scores; callers map positions back to IDs.
func (idx *memIndex) search(query []float64, topK int) ([]int, []float64) {
	idx.vecMu.RLock()
	defer idx.vecMu.RUnlock()

	type scoredPos struct {
		pos   int
		score float64
	}
	hits := make([]scoredPos, 0, len(idx.vecs))

	qNorm := cosineNorm(query)
	if qNorm == 0 {
		return nil, nil
	}

	for i, v := range idx.vecs {
		score := cosineSimilarity(query, v, qNorm)
		hits = append(hits, scoredPos{pos: i, score: score})
	}

	// Partial sort to get topK.
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].score > hits[j].score
	})
	if topK > len(hits) {
		topK = len(hits)
	}
	hits = hits[:topK]

	positions := make([]int, topK)
	scores := make([]float64, topK)
	for i, h := range hits {
		positions[i] = h.pos
		scores[i] = h.score
	}
	return positions, scores
}

// cosineNorm returns the L2 norm of a vector.
func cosineNorm(v []float64) float64 {
	sum := 0.0
	for _, x := range v {
		sum += x * x
	}
	return math.Sqrt(sum)
}

// cosineSimilarity returns the cosine similarity between a and b (both normalised).
// Assumes a is already normalised; bNorm is pre-computed.
func cosineSimilarity(a, b []float64, bNorm float64) float64 {
	dot := 0.0
	for i := range a {
		dot += a[i] * b[i]
	}
	return dot / (cosineNorm(a) * bNorm + 1e-12)
}

// NewVectorStore opens (or creates) a Badger-backed vector store at dataDir.
// An empty dataDir creates an in-memory store (useful for testing).
func NewVectorStore(dataDir string, cfg HNSWConfig) (*VectorStore, error) {
	opts := badger.DefaultOptions(dataDir)
	opts.Logger = &badgerLogAdapter{}

	if dataDir == "" {
		opts = badger.DefaultOptions("").WithInMemory(true)
	}

	db, err := badger.Open(opts)
	if err != nil {
		return nil, fmt.Errorf("open badger: %w", err)
	}

	return &VectorStore{
		db:      db,
		cfg:     cfg,
		locks:   make(map[string]*sync.Mutex),
		indexes: make(map[string]*memIndex),
	}, nil
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
	buf := bytes.NewBuffer(nil)
	for _, f := range v {
		binary.Write(buf, binary.LittleEndian, f)
	}
	return buf.Bytes()
}

func bytesToVector(b []byte, dim int) ([]float64, error) {
	if len(b) != dim*8 {
		return nil, fmt.Errorf("expected %d bytes, got %d", dim*8, len(b))
	}
	v := make([]float64, dim)
	binary.Read(bytes.NewReader(b), binary.LittleEndian, v)
	return v, nil
}

// collectionLock returns the mutex for a collection, creating it if needed.
func (vs *VectorStore) collectionLock(name string) *sync.Mutex {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	if vs.locks[name] == nil {
		vs.locks[name] = &sync.Mutex{}
	}
	return vs.locks[name]
}

// getOrLoadIndex returns the live index for a collection, building it from
// Badger on first access.
func (vs *VectorStore) getOrLoadIndex(name string) (*memIndex, error) {
	vs.mu.RLock()
	idx, ok := vs.indexes[name]
	vs.mu.RUnlock()
	if ok {
		return idx, nil
	}

	vs.mu.Lock()
	defer vs.mu.Unlock()

	// Double-check with write lock.
	if idx, ok := vs.indexes[name]; ok {
		return idx, nil
	}

	// Load collection metadata to get dimension.
	meta, err := vs.getCollectionMeta(name)
	if err != nil {
		return nil, err
	}

	idx = newMemIndex(int(meta.Dim))
	prefix := []byte("vector:" + name + ":")
	err = vs.db.View(func(txn *badger.Txn) error {
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
			idx.insert(id, vec)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("rebuild index from badger: %w", err)
	}

	vs.indexes[name] = idx
	return idx, nil
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
	if dim <= 0 {
		return ErrZeroDimension
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
func (vs *VectorStore) AddVectors(collection string, ids []string, vectors [][]float64) error {
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

	// Insert into in-memory index.
	idx, err := vs.getOrLoadIndex(collection)
	if err != nil {
		return fmt.Errorf("load index: %w", err)
	}
	mu := vs.collectionLock(collection)
	mu.Lock()
	defer mu.Unlock()
	for i := range ids {
		idx.insert(ids[i], vectors[i])
	}

	log.Info().Str("collection", collection).Int("count", len(vectors)).Msg("vectors added")
	return nil
}

// Search finds the topK most similar vectors to the query in the collection.
// Uses cosine similarity; higher scores (closer to 1.0) mean more similar.
// Returns ErrCollectionNotFound or ErrVectorDimMismatch.
func (vs *VectorStore) Search(collection string, query []float64, topK int) ([]SearchResult, error) {
	if len(query) == 0 {
		return nil, ErrEmptyVector
	}
	if topK <= 0 {
		topK = 10
	}

	meta, err := vs.getCollectionMeta(collection)
	if err != nil {
		return nil, err
	}
	if len(query) != int(meta.Dim) {
		return nil, fmt.Errorf("%w: query dim %d, expected %d", ErrVectorDimMismatch, len(query), meta.Dim)
	}

	idx, err := vs.getOrLoadIndex(collection)
	if err != nil {
		return nil, err
	}

	mu := vs.collectionLock(collection)
	mu.Lock()
	defer mu.Unlock()

	positions, scores := idx.search(query, topK)

	out := make([]SearchResult, len(positions))
	for i, pos := range positions {
		out[i] = SearchResult{ID: idx.ids[pos], Score: scores[i]}
	}

	// Load chunk contents from Badger
	for i := range out {
		var buf bytes.Buffer
		readErr := vs.db.View(func(txn *badger.Txn) error {
			item, err := txn.Get(chunkKey(collection, out[i].ID))
			if err != nil {
				return nil // not found, skip
			}
			return item.Value(func(val []byte) error {
				buf.Reset()
				buf.Write(val)
				return nil
			})
		})
		if readErr == nil {
			out[i].ChunkContent = buf.String()
		}
	}

	log.Debug().Str("collection", collection).Int("topK", topK).Int("found", len(out)).Msg("search complete")
	return out, nil
}

// DeleteCollection removes a collection and all its data from Badger.
// The in-memory index entry is also cleared.
func (vs *VectorStore) DeleteCollection(name string) error {
	_, err := vs.getCollectionMeta(name)
	if err != nil {
		return err
	}

	// Remove from memory.
	vs.mu.Lock()
	delete(vs.indexes, name)
	delete(vs.locks, name)
	vs.mu.Unlock()

	// Remove all keys from Badger.
	err = vs.db.Update(func(txn *badger.Txn) error {
		if err := txn.Delete(collectionKey(name)); err != nil {
			return err
		}
		if err := txn.Delete(hnswKey(name)); err != nil && !errors.Is(err, badger.ErrKeyNotFound) {
			return err
		}
		prefix := []byte("vector:" + name + ":")
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		var keys [][]byte
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			keys = append(keys, it.Item().KeyCopy(nil))
		}
		for _, k := range keys {
			if err := txn.Delete(k); err != nil {
				return fmt.Errorf("delete key: %w", err)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete collection %q: %w", name, err)
	}

	log.Info().Str("collection", name).Msg("collection deleted")
	return nil
}

// Close closes the underlying Badger database.
func (vs *VectorStore) Close() error {
	vs.mu.Lock()
	defer vs.mu.Unlock()
	vs.indexes = nil
	vs.locks = nil
	return vs.db.Close()
}

// ---------------------------------------------------------------------------
// Badger log adapter — routes Badger logs into zerolog.
// ---------------------------------------------------------------------------

type badgerLogAdapter struct{}

func (*badgerLogAdapter) Errorf(f string, v ...interface{})   { log.Error().Msgf("[badger] "+f, v...) }
func (*badgerLogAdapter) Warningf(f string, v ...interface{}) { log.Warn().Msgf("[badger] "+f, v...) }
func (*badgerLogAdapter) Infof(f string, v ...interface{})     { log.Info().Msgf("[badger] "+f, v...) }
func (*badgerLogAdapter) Debugf(f string, v ...interface{})    { log.Debug().Msgf("[badger] "+f, v...) }
