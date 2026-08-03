package rag

import (
	"encoding/json"

	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"

	"douya/internal/apperror"
)

type DocumentMeta struct {
	ID         string            `json:"id"`
	Collection string            `json:"collection"`
	FileName   string            `json:"file_name"`
	FileSize   int64             `json:"file_size"`
	MimeType   string            `json:"mime_type"`
	ChunkCount int               `json:"chunk_count"`
	IngestedAt string            `json:"ingested_at"`
	Tags       map[string]string `json:"tags,omitempty"`
}

type DocumentStore struct {
	vs *VectorStore
}

// NewDocumentStore 创建文档元数据存储。
// 改为接收 *VectorStore 而非 *badger.DB，通过 WithTx 访问底层 Badger，
// 避免直接暴露 *badger.DB 句柄。
func NewDocumentStore(vs *VectorStore) *DocumentStore {
	return &DocumentStore{vs: vs}
}

func docMetaKey(collection, id string) []byte {
	return []byte("docmeta:" + collection + ":" + id)
}

func (ds *DocumentStore) Put(meta DocumentMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "marshal document meta", err)
	}
	key := docMetaKey(meta.Collection, meta.ID)
	// 通过 VectorStore.WithTx 访问 Badger，不再直接持有 *badger.DB
	err = ds.vs.WithTx(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return apperror.Wrapf(apperror.KindInternal, "put document meta %q/%q", err, meta.Collection, meta.ID)
	}
	log.Info().Str("collection", meta.Collection).Str("id", meta.ID).Msg("document meta stored")
	return nil
}

func (ds *DocumentStore) Get(collection, id string) (*DocumentMeta, error) {
	key := docMetaKey(collection, id)
	var meta DocumentMeta
	// 读操作也走 WithTx（读写事务），功能等价于原 db.View
	err := ds.vs.WithTx(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &meta)
		})
	})
	if err != nil {
		return nil, apperror.Wrapf(apperror.KindInternal, "get document meta %q/%q", err, collection, id)
	}
	return &meta, nil
}

func (ds *DocumentStore) List(collection string) ([]DocumentMeta, error) {
	prefix := []byte("docmeta:" + collection + ":")
	var result []DocumentMeta
	err := ds.vs.WithTx(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			var meta DocumentMeta
			err := item.Value(func(val []byte) error {
				return json.Unmarshal(val, &meta)
			})
			if err != nil {
				log.Warn().Err(err).Str("key", string(item.Key())).Msg("skipping malformed document meta")
				continue
			}
			result = append(result, meta)
		}
		return nil
	})
	if err != nil {
		return nil, apperror.Wrapf(apperror.KindInternal, "list document meta for %q", err, collection)
	}
	if result == nil {
		result = []DocumentMeta{}
	}
	return result, nil
}

func (ds *DocumentStore) Delete(collection, id string) error {
	key := docMetaKey(collection, id)
	err := ds.vs.WithTx(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	if err != nil {
		return apperror.Wrapf(apperror.KindInternal, "delete document meta %q/%q", err, collection, id)
	}
	log.Info().Str("collection", collection).Str("id", id).Msg("document meta deleted")
	return nil
}
