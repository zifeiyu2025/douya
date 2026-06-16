package rag

import (
	"testing"
)

func newTestDocStore(t *testing.T) (*DocumentStore, func()) {
	t.Helper()
	vs, cleanup := newTestStore(t)
	ds := NewDocumentStore(vs.DB())
	return ds, cleanup
}

func TestDocumentStore_PutAndGet(t *testing.T) {
	ds, cleanup := newTestDocStore(t)
	defer cleanup()

	meta := DocumentMeta{
		ID:         "doc-001",
		Collection: "test-col",
		FileName:   "test.txt",
		FileSize:   1024,
		MimeType:   "text/plain",
		ChunkCount: 3,
		IngestedAt: "2026-01-01T00:00:00Z",
	}

	err := ds.Put(meta)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := ds.Get("test-col", "doc-001")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.ID != "doc-001" {
		t.Errorf("expected ID 'doc-001', got %q", got.ID)
	}
	if got.FileName != "test.txt" {
		t.Errorf("expected FileName 'test.txt', got %q", got.FileName)
	}
	if got.FileSize != 1024 {
		t.Errorf("expected FileSize 1024, got %d", got.FileSize)
	}
	if got.ChunkCount != 3 {
		t.Errorf("expected ChunkCount 3, got %d", got.ChunkCount)
	}
}

func TestDocumentStore_GetNotFound(t *testing.T) {
	ds, cleanup := newTestDocStore(t)
	defer cleanup()

	_, err := ds.Get("nonexistent", "no-such-doc")
	if err == nil {
		t.Error("expected error for nonexistent document, got nil")
	}
}

func TestDocumentStore_List(t *testing.T) {
	ds, cleanup := newTestDocStore(t)
	defer cleanup()

	// 空集合应返回空列表
	docs, err := ds.List("empty-col")
	if err != nil {
		t.Fatalf("List empty collection failed: %v", err)
	}
	if len(docs) != 0 {
		t.Errorf("expected 0 docs for empty collection, got %d", len(docs))
	}

	// 添加两个文档
	for _, id := range []string{"doc-a", "doc-b"} {
		err := ds.Put(DocumentMeta{
			ID:         id,
			Collection: "my-col",
			FileName:   id + ".txt",
			ChunkCount: 1,
		})
		if err != nil {
			t.Fatalf("Put %s failed: %v", id, err)
		}
	}

	docs, err = ds.List("my-col")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(docs) != 2 {
		t.Errorf("expected 2 docs, got %d", len(docs))
	}
}

func TestDocumentStore_Delete(t *testing.T) {
	ds, cleanup := newTestDocStore(t)
	defer cleanup()

	meta := DocumentMeta{
		ID:         "doc-del",
		Collection: "del-col",
		FileName:   "delete-me.txt",
	}

	err := ds.Put(meta)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	err = ds.Delete("del-col", "doc-del")
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err = ds.Get("del-col", "doc-del")
	if err == nil {
		t.Error("expected error after delete, got nil")
	}
}

func TestDocumentStore_DeleteNotFound(t *testing.T) {
	ds, cleanup := newTestDocStore(t)
	defer cleanup()

	// 删除不存在的文档不应报错（幂等）
	err := ds.Delete("no-col", "no-doc")
	if err != nil {
		t.Errorf("Delete nonexistent should not error, got: %v", err)
	}
}

func TestDocumentStore_PutOverwrite(t *testing.T) {
	ds, cleanup := newTestDocStore(t)
	defer cleanup()

	meta := DocumentMeta{
		ID:         "doc-ov",
		Collection: "ov-col",
		FileName:   "original.txt",
		ChunkCount: 1,
	}
	ds.Put(meta)

	// 覆盖写入
	meta.FileName = "updated.txt"
	meta.ChunkCount = 5
	ds.Put(meta)

	got, err := ds.Get("ov-col", "doc-ov")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.FileName != "updated.txt" {
		t.Errorf("expected FileName 'updated.txt', got %q", got.FileName)
	}
	if got.ChunkCount != 5 {
		t.Errorf("expected ChunkCount 5, got %d", got.ChunkCount)
	}
}
