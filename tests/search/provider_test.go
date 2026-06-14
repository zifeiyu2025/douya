// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search_test

import (
	"context"
	"douya/internal/search"
	"testing"
)

func TestOllamaProvider_EmptyAPIKey(t *testing.T) {
	p := search.NewOllamaProvider("")
	if p == nil {
		t.Fatal("NewOllamaProvider should not return nil")
	}
	_, err := p.Search(context.Background(), "test query")
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
	if err.Error() != "ollama api key is empty" {
		t.Fatalf("expected error 'ollama api key is empty', got '%s'", err.Error())
	}
}

func TestTavilyProvider_EmptyAPIKey(t *testing.T) {
	p := search.NewTavilyProvider("")
	if p == nil {
		t.Fatal("NewTavilyProvider should not return nil")
	}
	_, err := p.Search(context.Background(), "test query")
	if err == nil {
		t.Fatal("expected error for empty API key, got nil")
	}
	if err.Error() != "tavily api key is empty" {
		t.Fatalf("expected error 'tavily api key is empty', got '%s'", err.Error())
	}
}
