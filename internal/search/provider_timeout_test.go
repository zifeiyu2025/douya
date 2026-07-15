// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"testing"
	"time"
)

// TestTavilyProvider_Timeout 验证 TavilyProvider 的 HTTP 超时为 30s。
// 项目记忆规则要求 Tavily 超时 30s 以支持 advanced search 深度检索，
// 过短超时会导致正常请求被误杀。防回归：避免被误改回 15s。
func TestTavilyProvider_Timeout(t *testing.T) {
	p := NewTavilyProvider("test-key")
	got := p.httpClient.Timeout
	want := 30 * time.Second
	if got != want {
		t.Errorf("TavilyProvider HTTP 超时 = %v, 期望 %v（项目记忆规则要求 30s 支持 advanced search）", got, want)
	}
}

// TestDefaultSearchTimeout 验证全局搜索超时为 50s。
// 50s 兼容三级降级链（Tavily 30s + Ollama 5s + Bing 15s ≈ 50s），
// 步进式降级无并发，单端超时累加需留足余量，否则下游 provider 永远拿不到机会。
// 防回归：避免被误改回 20s（20s 无法覆盖降级链，导致 Bing 兜底永远不执行）。
func TestDefaultSearchTimeout(t *testing.T) {
	got := DefaultSearchTimeout
	want := 50 * time.Second
	if got != want {
		t.Errorf("DefaultSearchTimeout = %v, 期望 %v（兼容三级降级链 Tavily+Ollama+Bing）", got, want)
	}
}

// TestTimeoutChainCompatibility 验证降级链超时累加不超过全局超时。
// Tavily 30s + Ollama 5s + Bing 15s = 50s，应 <= DefaultSearchTimeout。
// 这是保证步进式降级链能完整执行的关键约束。
func TestTimeoutChainCompatibility(t *testing.T) {
	tavilyTimeout := 30 * time.Second
	// Ollama 和 Bing 的超时由各自 Provider 内部控制，此处用估值验证累加上限
	ollamaEstimate := 5 * time.Second
	bingEstimate := 15 * time.Second
	total := tavilyTimeout + ollamaEstimate + bingEstimate
	if total > DefaultSearchTimeout {
		t.Errorf("降级链超时累加 %v 超过全局超时 %v，下游 provider 无法完整执行", total, DefaultSearchTimeout)
	}
}
