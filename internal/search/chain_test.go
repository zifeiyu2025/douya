package search

import (
	"context"
	"testing"
	"time"
)

type mockProvider struct {
	name    string
	results []SearchResult
	err     error
}

func (m *mockProvider) Name() string { return m.name }
func (m *mockProvider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &SearchResponse{Engine: m.name, Results: m.results}, nil
}
func (m *mockProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	return m.Search(ctx, query)
}

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"https basic", "https://example.com", "https://example.com/"},
		{"http to https", "http://example.com", "https://example.com/"},
		{"remove www", "https://www.example.com", "https://example.com/"},
		{"remove fragment", "https://example.com/page#section", "https://example.com/page"},
		{"remove query", "https://example.com/page?foo=bar", "https://example.com/page"},
		{"trailing slash", "https://example.com/page/", "https://example.com/page"},
		{"combined", "http://www.example.com/path/?q=1#top", "https://example.com/path"},
		{"https with www", "https://www.example.com/path", "https://example.com/path"},
		{"no scheme", "example.com", "https://example.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeURL(tt.input)
			if got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestCircuitBreaker(t *testing.T) {
	p := &ProviderWithCircuit{
		State:            CircuitClosed,
		FailureThreshold: 3,
		ResetTimeout:     100 * time.Millisecond,
	}

	if !p.canAttempt() {
		t.Fatal("closed circuit should allow attempt")
	}

	p.recordFailure()
	p.recordFailure()
	if p.State != CircuitClosed {
		t.Fatalf("after 2 failures, state = %v, want Closed", p.State)
	}
	if !p.canAttempt() {
		t.Fatal("2 failures should still allow attempt")
	}

	p.recordFailure()
	if p.State != CircuitOpen {
		t.Fatalf("after 3 failures, state = %v, want Open", p.State)
	}
	if p.canAttempt() {
		t.Fatal("open circuit should block attempt immediately")
	}

	time.Sleep(150 * time.Millisecond)
	if !p.canAttempt() {
		t.Fatal("after reset timeout, should allow attempt (HalfOpen)")
	}
	if p.State != CircuitHalfOpen {
		t.Fatalf("after reset timeout, state = %v, want HalfOpen", p.State)
	}

	p.recordSuccess()
	if p.State != CircuitClosed {
		t.Fatalf("after success in HalfOpen, state = %v, want Closed", p.State)
	}
	if p.Failures != 0 {
		t.Fatalf("after success, failures = %d, want 0", p.Failures)
	}
}

func TestCircuitBreakerFailureCounting(t *testing.T) {
	p := &ProviderWithCircuit{
		State:            CircuitClosed,
		FailureThreshold: 5,
		ResetTimeout:     10 * time.Second,
	}
	for i := 0; i < 4; i++ {
		p.recordFailure()
	}
	if p.State != CircuitClosed {
		t.Fatalf("after 4/5 failures, state = %v, want Closed", p.State)
	}
	p.recordFailure()
	if p.State != CircuitOpen {
		t.Fatalf("after 5/5 failures, state = %v, want Open", p.State)
	}
}

func TestNewSearchChain(t *testing.T) {
	p1 := &mockProvider{name: "p1"}
	p2 := &mockProvider{name: "p2"}
	p3 := &mockProvider{name: "p3"}

	chain := NewSearchChain(p1, p2, p3)
	if chain == nil {
		t.Fatal("NewSearchChain returned nil")
	}

	providers := chain.Providers()
	if len(providers) != 3 {
		t.Fatalf("got %d providers, want 3", len(providers))
	}
	for i, pw := range providers {
		if pw.Provider == nil {
			t.Fatalf("provider %d is nil", i)
		}
		if pw.State != CircuitClosed {
			t.Fatalf("provider %d initial state = %v, want Closed", i, pw.State)
		}
		if pw.FailureThreshold != 3 {
			t.Fatalf("provider %d threshold = %d, want 3", i, pw.FailureThreshold)
		}
	}
}

func TestNewSearchChainEmpty(t *testing.T) {
	chain := NewSearchChain()
	if len(chain.Providers()) != 0 {
		t.Fatalf("empty chain has %d providers, want 0", len(chain.Providers()))
	}
}
