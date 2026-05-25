package rag

import (
	"encoding/json"
	"fmt"

	"github.com/dgraph-io/badger/v4"
	"github.com/rs/zerolog/log"
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
	db *badger.DB
}

func NewDocumentStore(db *badger.DB) *DocumentStore {
	return &DocumentStore{db: db}
}

func docMetaKey(collection, id string) []byte {
	return []byte("docmeta:" + collection + ":" + id)
}

func (ds *DocumentStore) Put(meta DocumentMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal document meta: %w", err)
	}
	key := docMetaKey(meta.Collection, meta.ID)
	err = ds.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, data)
	})
	if err != nil {
		return fmt.Errorf("put document meta %q/%q: %w", meta.Collection, meta.ID, err)
	}
	log.Info().Str("collection", meta.Collection).Str("id", meta.ID).Msg("document meta stored")
	return nil
}

func (ds *DocumentStore) Get(collection, id string) (*DocumentMeta, error) {
	key := docMetaKey(collection, id)
	var meta DocumentMeta
	err := ds.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			return json.Unmarshal(val, &meta)
		})
	})
	if err != nil {
		return nil, fmt.Errorf("get document meta %q/%q: %w", collection, id, err)
	}
	return &meta, nil
}

func (ds *DocumentStore) List(collection string) ([]DocumentMeta, error) {
	prefix := []byte("docmeta:" + collection + ":")
	var result []DocumentMeta
	err := ds.db.View(func(txn *badger.Txn) error {
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
		return nil, fmt.Errorf("list document meta for %q: %w", collection, err)
	}
	if result == nil {
		result = []DocumentMeta{}
	}
	return result, nil
}

func (ds *DocumentStore) Delete(collection, id string) error {
	key := docMetaKey(collection, id)
	err := ds.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
	if err != nil {
		return fmt.Errorf("delete document meta %q/%q: %w", collection, id, err)
	}
	log.Info().Str("collection", collection).Str("id", id).Msg("document meta deleted")
	return nil
}
