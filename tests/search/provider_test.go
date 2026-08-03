// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search_test

import (
	"context"
	"strings"
	"testing"

	"douya/internal/apperror"
	"douya/internal/search"
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
	// 用错误类型判断替代字符串精确匹配，apperror 改造后 Error() 格式为 "Permission: ollama api key is empty"
	if apperror.KindOf(err) != apperror.KindPermission {
		t.Fatalf("expected KindPermission, got %s, err=%v", apperror.KindOf(err), err)
	}
	if !strings.Contains(err.Error(), "ollama api key is empty") {
		t.Fatalf("expected error containing 'ollama api key is empty', got '%s'", err.Error())
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
	if apperror.KindOf(err) != apperror.KindPermission {
		t.Fatalf("expected KindPermission, got %s, err=%v", apperror.KindOf(err), err)
	}
	if !strings.Contains(err.Error(), "tavily api key is empty") {
		t.Fatalf("expected error containing 'tavily api key is empty', got '%s'", err.Error())
	}
}
