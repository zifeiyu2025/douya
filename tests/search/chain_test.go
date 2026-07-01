// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search_test

import (
	"context"
	"douya/internal/search"
	"fmt"
	"sync"
	"testing"
	"time"
)

type mockProvider struct {
	name       string
	searchFunc func(ctx context.Context, query string) (*search.SearchResponse, error)
	mu         sync.Mutex
	callCount  int
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Search(ctx context.Context, query string) (*search.SearchResponse, error) {
	m.mu.Lock()
	m.callCount++
	m.mu.Unlock()
	if m.searchFunc != nil {
		return m.searchFunc(ctx, query)
	}
	return &search.SearchResponse{Engine: m.name}, nil
}

func (m *mockProvider) SearchWithOpts(ctx context.Context, query string, opts search.SearchOpts) (*search.SearchResponse, error) {
	return m.Search(ctx, query)
}

func (m *mockProvider) getCallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCount
}

func TestChainFirstProviderSucceeds(t *testing.T) {
	p1 := &mockProvider{
		name: "provider1",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "provider1",
				Results: []search.SearchResult{{Title: "result1", URL: "http://1", Snippet: "s1"}},
			}, nil
		},
	}
	p2 := &mockProvider{name: "provider2"}

	chain := search.NewSearchChain(p1, p2)
	resp := chain.Search(context.Background(), "test")

	if resp.Engine != "provider1" {
		t.Errorf("expected engine provider1, got %s", resp.Engine)
	}
	if resp.Error != "" {
		t.Errorf("expected no error, got %s", resp.Error)
	}
	if p1.getCallCount() != 1 {
		t.Errorf("expected p1 call count 1, got %d", p1.getCallCount())
	}
	if p2.getCallCount() != 0 {
		t.Errorf("expected p2 call count 0, got %d", p2.getCallCount())
	}
}

func TestChainFallback(t *testing.T) {
	p1 := &mockProvider{
		name: "provider1",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return nil, fmt.Errorf("provider1 failed")
		},
	}
	p2 := &mockProvider{
		name: "provider2",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "provider2",
				Results: []search.SearchResult{{Title: "result2", URL: "http://2", Snippet: "s2"}},
			}, nil
		},
	}

	chain := search.NewSearchChain(p1, p2)
	resp := chain.Search(context.Background(), "test")

	if resp.Engine != "provider2" {
		t.Errorf("expected engine provider2, got %s", resp.Engine)
	}
	if resp.Error != "" {
		t.Errorf("expected no error, got %s", resp.Error)
	}
	if p1.getCallCount() != 1 {
		t.Errorf("expected p1 call count 1, got %d", p1.getCallCount())
	}
	if p2.getCallCount() != 1 {
		t.Errorf("expected p2 call count 1, got %d", p2.getCallCount())
	}
}

func TestChainAllProvidersFail(t *testing.T) {
	failFunc := func(ctx context.Context, query string) (*search.SearchResponse, error) {
		return nil, fmt.Errorf("failed")
	}
	p1 := &mockProvider{name: "provider1", searchFunc: failFunc}
	p2 := &mockProvider{name: "provider2", searchFunc: failFunc}

	chain := search.NewSearchChain(p1, p2)
	resp := chain.Search(context.Background(), "test")

	if resp.Engine != "none" {
		t.Errorf("expected engine none, got %s", resp.Engine)
	}
	if resp.Error == "" {
		t.Error("expected error message, got empty")
	}
}

func TestCircuitBreakerOpenSkipsProvider(t *testing.T) {
	p := &mockProvider{
		name: "failProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return nil, fmt.Errorf("failed")
		},
	}

	chain := search.NewSearchChain(p)
	chain.Providers()[0].FailureThreshold = 3

	for range 3 {
		chain.Search(context.Background(), "test")
	}

	if chain.Providers()[0].State != search.CircuitOpen {
		t.Errorf("expected CircuitOpen after 3 failures, got %d", chain.Providers()[0].State)
	}

	callCountBefore := p.getCallCount()
	chain.Search(context.Background(), "test")
	callCountAfter := p.getCallCount()

	if callCountAfter != callCountBefore {
		t.Errorf("expected provider to be skipped in Open state, but call count changed from %d to %d", callCountBefore, callCountAfter)
	}
}

func TestCircuitBreakerHalfOpenAfterResetTimeout(t *testing.T) {
	p := &mockProvider{
		name: "failProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return nil, fmt.Errorf("failed")
		},
	}

	chain := search.NewSearchChain(p)
	chain.Providers()[0].FailureThreshold = 3
	chain.Providers()[0].ResetTimeout = 10 * time.Millisecond

	for range 3 {
		chain.Search(context.Background(), "test")
	}

	if chain.Providers()[0].State != search.CircuitOpen {
		t.Errorf("expected CircuitOpen, got %d", chain.Providers()[0].State)
	}

	time.Sleep(20 * time.Millisecond)

	callCountBefore := p.getCallCount()
	chain.Search(context.Background(), "test")
	callCountAfter := p.getCallCount()

	if callCountAfter <= callCountBefore {
		t.Errorf("expected provider to be attempted in HalfOpen state, but call count didn't increase (before=%d, after=%d)", callCountBefore, callCountAfter)
	}
}

func TestHalfOpenSuccessRestoresClosed(t *testing.T) {
	var attempt int
	p := &mockProvider{
		name: "provider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			attempt++
			if attempt <= 3 {
				return nil, fmt.Errorf("failed")
			}
			return &search.SearchResponse{
				Engine:  "provider",
				Results: []search.SearchResult{{Title: "ok", URL: "http://ok", Snippet: "ok"}},
			}, nil
		},
	}

	chain := search.NewSearchChain(p)
	chain.Providers()[0].FailureThreshold = 3
	chain.Providers()[0].ResetTimeout = 10 * time.Millisecond

	for range 3 {
		chain.Search(context.Background(), "test")
	}

	time.Sleep(20 * time.Millisecond)

	resp := chain.Search(context.Background(), "test")

	if resp.Engine != "provider" {
		t.Errorf("expected engine provider, got %s", resp.Engine)
	}
	if chain.Providers()[0].State != search.CircuitClosed {
		t.Errorf("expected CircuitClosed after HalfOpen success, got %d", chain.Providers()[0].State)
	}
	if chain.Providers()[0].Failures != 0 {
		t.Errorf("expected Failures to be 0 after recovery, got %d", chain.Providers()[0].Failures)
	}
}

func TestHalfOpenFailureBackToOpen(t *testing.T) {
	p := &mockProvider{
		name: "provider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return nil, fmt.Errorf("failed")
		},
	}

	chain := search.NewSearchChain(p)
	chain.Providers()[0].FailureThreshold = 3
	chain.Providers()[0].ResetTimeout = 10 * time.Millisecond

	for range 3 {
		chain.Search(context.Background(), "test")
	}

	if chain.Providers()[0].State != search.CircuitOpen {
		t.Errorf("expected CircuitOpen, got %d", chain.Providers()[0].State)
	}

	time.Sleep(20 * time.Millisecond)

	chain.Search(context.Background(), "test")

	if chain.Providers()[0].State != search.CircuitOpen {
		t.Errorf("expected CircuitOpen after HalfOpen failure, got %d", chain.Providers()[0].State)
	}
}

func TestCategoryFiltering(t *testing.T) {
	codeProvider := &mockProvider{
		name: "codeProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "codeProvider",
				Results: []search.SearchResult{{Title: "code result", URL: "http://code", Snippet: "code"}},
			}, nil
		},
	}
	generalProvider := &mockProvider{
		name: "generalProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "generalProvider",
				Results: []search.SearchResult{{Title: "general result", URL: "http://general", Snippet: "general"}},
			}, nil
		},
	}

	chain := search.NewCategorizedSearchChain([]search.CategorizedProvider{
		{Provider: generalProvider, Categories: []string{"general"}},
		{Provider: codeProvider, Categories: []string{"code"}},
	})

	resp := chain.SearchWithCategory(context.Background(), "test", "code")

	if resp.Engine != "codeProvider" {
		t.Errorf("expected engine codeProvider, got %s", resp.Engine)
	}
	if generalProvider.getCallCount() != 0 {
		t.Errorf("expected generalProvider not to be called for category=code, but call count is %d", generalProvider.getCallCount())
	}
	if codeProvider.getCallCount() != 1 {
		t.Errorf("expected codeProvider call count 1, got %d", codeProvider.getCallCount())
	}
}

func TestCategoryFiltering_GeneralCategory(t *testing.T) {
	codeProvider := &mockProvider{
		name: "codeProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "codeProvider",
				Results: []search.SearchResult{{Title: "code result", URL: "http://code", Snippet: "code"}},
			}, nil
		},
	}
	generalProvider := &mockProvider{
		name: "generalProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "generalProvider",
				Results: []search.SearchResult{{Title: "general result", URL: "http://general", Snippet: "general"}},
			}, nil
		},
	}

	chain := search.NewCategorizedSearchChain([]search.CategorizedProvider{
		{Provider: generalProvider, Categories: []string{"general"}},
		{Provider: codeProvider, Categories: []string{"code"}},
	})

	resp := chain.SearchWithCategory(context.Background(), "test", "general")

	if resp.Engine != "generalProvider" {
		t.Errorf("expected engine generalProvider, got %s", resp.Engine)
	}
	if codeProvider.getCallCount() != 0 {
		t.Errorf("expected codeProvider not to be called for category=general, but call count is %d", codeProvider.getCallCount())
	}
}

func TestCategoryFiltering_NoCategoryMatchesAll(t *testing.T) {
	generalProvider := &mockProvider{
		name: "generalProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "generalProvider",
				Results: []search.SearchResult{{Title: "result", URL: "http://r", Snippet: "s"}},
			}, nil
		},
	}

	chain := search.NewSearchChain(generalProvider)

	resp := chain.SearchWithCategory(context.Background(), "test", "code")

	if resp.Engine != "generalProvider" {
		t.Errorf("expected engine generalProvider (no category = matches all), got %s", resp.Engine)
	}
}

func TestSearchWithCategory_NoProviderForCategory(t *testing.T) {
	codeProvider := &mockProvider{
		name: "codeProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "codeProvider",
				Results: []search.SearchResult{{Title: "code", URL: "http://c", Snippet: "c"}},
			}, nil
		},
	}

	chain := search.NewCategorizedSearchChain([]search.CategorizedProvider{
		{Provider: codeProvider, Categories: []string{"code"}},
	})

	resp := chain.SearchWithCategory(context.Background(), "test", "general")

	if resp.Engine != "none" {
		t.Errorf("expected engine 'none' when no provider matches category, got %s", resp.Engine)
	}
}

func TestCircuitBreaker_FailureThreshold2(t *testing.T) {
	p := &mockProvider{
		name: "failProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return nil, fmt.Errorf("failed")
		},
	}

	chain := search.NewSearchChain(p)
	chain.Providers()[0].FailureThreshold = 2

	chain.Search(context.Background(), "test")
	if chain.Providers()[0].State != search.CircuitClosed {
		t.Errorf("expected CircuitClosed after 1 failure (threshold=2), got %d", chain.Providers()[0].State)
	}

	chain.Search(context.Background(), "test")
	if chain.Providers()[0].State != search.CircuitOpen {
		t.Errorf("expected CircuitOpen after 2 failures (threshold=2), got %d", chain.Providers()[0].State)
	}
}

func TestCircuitBreaker_SuccessResetsFailures(t *testing.T) {
	var attempt int
	p := &mockProvider{
		name: "provider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			attempt++
			if attempt == 2 {
				return &search.SearchResponse{
					Engine:  "provider",
					Results: []search.SearchResult{{Title: "ok", URL: "http://ok", Snippet: "ok"}},
				}, nil
			}
			return nil, fmt.Errorf("failed")
		},
	}

	chain := search.NewSearchChain(p)
	chain.Providers()[0].FailureThreshold = 3

	chain.Search(context.Background(), "test")
	if chain.Providers()[0].Failures != 1 {
		t.Errorf("expected 1 failure, got %d", chain.Providers()[0].Failures)
	}

	chain.Search(context.Background(), "test")
	if chain.Providers()[0].Failures != 0 {
		t.Errorf("expected 0 failures after success, got %d", chain.Providers()[0].Failures)
	}
	if chain.Providers()[0].State != search.CircuitClosed {
		t.Errorf("expected CircuitClosed after success, got %d", chain.Providers()[0].State)
	}

	chain.Search(context.Background(), "test")
	if chain.Providers()[0].Failures != 1 {
		t.Errorf("expected 1 failure after new failure, got %d", chain.Providers()[0].Failures)
	}
}

func TestChainFallback_WithCircuitBreaker(t *testing.T) {
	p1 := &mockProvider{
		name: "failProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return nil, fmt.Errorf("failed")
		},
	}
	p2 := &mockProvider{
		name: "successProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "successProvider",
				Results: []search.SearchResult{{Title: "ok", URL: "http://ok", Snippet: "ok"}},
			}, nil
		},
	}

	chain := search.NewSearchChain(p1, p2)
	chain.Providers()[0].FailureThreshold = 2

	for range 2 {
		resp := chain.Search(context.Background(), "test")
		if resp.Engine != "successProvider" {
			t.Errorf("expected fallback to successProvider, got %s", resp.Engine)
		}
	}

	if chain.Providers()[0].State != search.CircuitOpen {
		t.Errorf("expected CircuitOpen after 2 failures, got %d", chain.Providers()[0].State)
	}

	resp := chain.Search(context.Background(), "test")
	if resp.Engine != "successProvider" {
		t.Errorf("expected fallback to successProvider after circuit open, got %s", resp.Engine)
	}
}

func TestSearch_DefaultCategoryIsGeneral(t *testing.T) {
	codeProvider := &mockProvider{
		name: "codeProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "codeProvider",
				Results: []search.SearchResult{{Title: "code", URL: "http://c", Snippet: "c"}},
			}, nil
		},
	}
	generalProvider := &mockProvider{
		name: "generalProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "generalProvider",
				Results: []search.SearchResult{{Title: "general", URL: "http://g", Snippet: "g"}},
			}, nil
		},
	}

	chain := search.NewCategorizedSearchChain([]search.CategorizedProvider{
		{Provider: generalProvider, Categories: []string{"general"}},
		{Provider: codeProvider, Categories: []string{"code"}},
	})

	resp := chain.Search(context.Background(), "test")

	if resp.Engine != "generalProvider" {
		t.Errorf("expected Search() to use 'general' category by default, got %s", resp.Engine)
	}
}

func TestProviderWithMultipleCategories(t *testing.T) {
	multiProvider := &mockProvider{
		name: "multiProvider",
		searchFunc: func(ctx context.Context, query string) (*search.SearchResponse, error) {
			return &search.SearchResponse{
				Engine:  "multiProvider",
				Results: []search.SearchResult{{Title: "result", URL: "http://r", Snippet: "s"}},
			}, nil
		},
	}

	chain := search.NewCategorizedSearchChain([]search.CategorizedProvider{
		{Provider: multiProvider, Categories: []string{"general", "code"}},
	})

	respGeneral := chain.SearchWithCategory(context.Background(), "test", "general")
	if respGeneral.Engine != "multiProvider" {
		t.Errorf("expected multiProvider for 'general', got %s", respGeneral.Engine)
	}

	respCode := chain.SearchWithCategory(context.Background(), "test", "code")
	if respCode.Engine != "multiProvider" {
		t.Errorf("expected multiProvider for 'code', got %s", respCode.Engine)
	}
}

func TestChainEmptyProviders(t *testing.T) {
	chain := search.NewSearchChain()

	resp := chain.Search(context.Background(), "test")

	if resp.Engine != "none" {
		t.Errorf("expected engine 'none' for empty chain, got %s", resp.Engine)
	}
	if resp.Error == "" {
		t.Error("expected error for empty chain")
	}
}
