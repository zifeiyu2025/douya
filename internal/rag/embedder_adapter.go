package rag

import (
	"context"
	"fmt"

	"douya/internal/llm"
)

// ClientEmbedder adapts llm.Client to the Embedder interface.
type ClientEmbedder struct {
	Client *llm.Client
	Model  string // embedding model name (optional, defaults to server default)
}

// Embed calls the LLM embedding API and returns float64 vectors.
func (e *ClientEmbedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if e.Client == nil {
		return nil, fmt.Errorf("rag: embedder client is nil")
	}

	req := &llm.EmbeddingRequest{
		Input: texts,
	}
	if e.Model != "" {
		req.Model = e.Model
	}

	resp, err := e.Client.Embedding(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("rag: embedding API call failed: %w", err)
	}

	vectors := make([][]float64, len(resp.Data))
	for i, d := range resp.Data {
		vectors[i] = make([]float64, len(d.Embedding))
		for j, v := range d.Embedding {
			vectors[i][j] = float64(v)
		}
	}

	return vectors, nil
}
