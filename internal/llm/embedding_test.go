package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEmbedding_SingleString(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("expected path /v1/embeddings, got %s", r.URL.Path)
		}
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		var req EmbeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		if req.Input != "Hello, world!" {
			t.Errorf("expected input 'Hello, world!', got %v", req.Input)
		}

		resp := EmbeddingResponse{
			Object: "list",
			Data: []Embedding{
				{Object: "embedding", Embedding: []float64{0.1, 0.2, 0.3}, Index: 0},
			},
			Model: "all-minilm",
			Usage:  Usage{PromptTokens: 4, TotalTokens: 4},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "")
	ctx := context.Background()

	req := &EmbeddingRequest{Input: "Hello, world!"}
	resp, err := client.Embedding(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 embedding, got %d", len(resp.Data))
	}
	if len(resp.Data[0].Embedding) != 3 {
		t.Errorf("expected 3 dimensions, got %d", len(resp.Data[0].Embedding))
	}
}

func TestEmbedding_MultipleStrings(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req EmbeddingRequest
		json.NewDecoder(r.Body).Decode(&req)

		inputs, ok := req.Input.([]interface{})
		if !ok {
			t.Errorf("expected array input, got %T", req.Input)
		}

		data := make([]Embedding, len(inputs))
		for i := range data {
			data[i] = Embedding{Object: "embedding", Embedding: []float64{float64(i) * 0.1}, Index: i}
		}

		resp := EmbeddingResponse{Object: "list", Data: data, Model: "all-minilm"}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "")
	ctx := context.Background()

	req := &EmbeddingRequest{Input: []string{"Hello", "World"}}
	resp, err := client.Embedding(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 embeddings, got %d", len(resp.Data))
	}
}

func TestEmbedding_HTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "")
	ctx := context.Background()

	req := &EmbeddingRequest{Input: "test"}
	_, err := client.Embedding(ctx, req)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestEmbedding_ModelFieldInRequest(t *testing.T) {
	var receivedModel string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req EmbeddingRequest
		json.NewDecoder(r.Body).Decode(&req)
		receivedModel = req.Model

		resp := EmbeddingResponse{
			Object: "list",
			Data:   []Embedding{{Object: "embedding", Embedding: []float64{0.1}, Index: 0}},
			Model:  "test-model",
			Usage:  Usage{PromptTokens: 1, TotalTokens: 1},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "")
	ctx := context.Background()

	// 测试：设置了 Model 字段时，请求中应包含 model
	req := &EmbeddingRequest{Input: "test", Model: "my-embedding-model"}
	_, err := client.Embedding(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if receivedModel != "my-embedding-model" {
		t.Errorf("expected model 'my-embedding-model', got %q", receivedModel)
	}
}

func TestEmbedding_ModelFieldEmpty(t *testing.T) {
	var rawBody map[string]interface{}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&rawBody)

		resp := EmbeddingResponse{
			Object: "list",
			Data:   []Embedding{{Object: "embedding", Embedding: []float64{0.1}, Index: 0}},
			Model:  "default",
			Usage:  Usage{PromptTokens: 1, TotalTokens: 1},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	client := NewClient(ts.URL, "")
	ctx := context.Background()

	// 测试：Model 为空时，JSON 中不应包含 model 字段（omitempty）
	req := &EmbeddingRequest{Input: "test"}
	_, err := client.Embedding(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := rawBody["model"]; ok {
		t.Errorf("expected 'model' key to be omitted when empty, but it was present: %v", rawBody["model"])
	}
}
