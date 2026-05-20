package search_test

import (
	"context"
	"douya/internal/search"
	"testing"
)

func TestNewDuckDuckGoProvider(t *testing.T) {
	p := search.NewDuckDuckGoProvider()
	if p == nil {
		t.Fatal("NewDuckDuckGoProvider should not return nil")
	}
}

func TestDuckDuckGoProvider_Name(t *testing.T) {
	p := search.NewDuckDuckGoProvider()
	if p.Name() != "duckduckgo" {
		t.Fatalf("expected name 'duckduckgo', got '%s'", p.Name())
	}
}

func TestDuckDuckGoProvider_SearchWithOpts(t *testing.T) {
	p := search.NewDuckDuckGoProvider()
	opts := search.SearchOpts{MaxResults: 5}
	resp, err := p.SearchWithOpts(context.Background(), "test query", opts)
	if err != nil {
		t.Logf("SearchWithOpts returned error (expected for network call in test): %v", err)
		return
	}
	if resp == nil {
		t.Fatal("SearchWithOpts should not return nil response when no error")
	}
	if resp.Engine != "duckduckgo" {
		t.Fatalf("expected engine 'duckduckgo', got '%s'", resp.Engine)
	}
}
