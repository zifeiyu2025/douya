package rag

import (
	"context"
	"fmt"
	"sync"

	"douya/internal/llm"
)

// ClientEmbedder adapts llm.Client to the Embedder interface.
type ClientEmbedder struct {
	Client         *llm.Client
	model          string // embedding model name (protected by mu)
	mu             sync.RWMutex
	currentModelFn func() string // 获取当前聊天模型名的回调（model 为空时使用）
}

// SetModel updates the embedding model name (safe for concurrent use).
func (e *ClientEmbedder) SetModel(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.model = name
}

// SetCurrentModelFn sets a callback to get the current chat model name (used when model is empty).
func (e *ClientEmbedder) SetCurrentModelFn(fn func() string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.currentModelFn = fn
}

// GetModel returns the current embedding model name (safe for concurrent use).
func (e *ClientEmbedder) GetModel() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.model
}

// Embed calls the LLM embedding API and returns float64 vectors.
func (e *ClientEmbedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	if e.Client == nil {
		return nil, fmt.Errorf("rag: embedder client is nil")
	}

	e.mu.RLock()
	modelName := e.model
	fn := e.currentModelFn
	e.mu.RUnlock()

	// 专用嵌入模型为空时，回退到当前聊天模型
	if modelName == "" && fn != nil {
		modelName = fn()
	}

	req := &llm.EmbeddingRequest{
		Input: texts,
	}
	if modelName != "" {
		req.Model = modelName
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
