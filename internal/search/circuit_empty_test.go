// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"context"
	"testing"
)

// TestCircuitBreaker_EmptyResultsNotFailure 验证：HTTP 成功但结果为空时，
// 不应视为失败触发熔断器。
//
// 业务场景：用户搜索冷门关键词（如"量子纠缠的烹饪方法"），搜索引擎正常响应
// 但没有匹配结果。这是正常的"无数据"响应，不是引擎故障。
//
// 修复前：连续 3 次空结果会熔断 Provider 30 秒，期间所有查询都被跳过
// 修复后：空结果仍调用 recordSuccess，熔断器保持 Closed
func TestCircuitBreaker_EmptyResultsNotFailure(t *testing.T) {
	// 构造一个始终返回 200 + 空结果的 mock provider
	mock := &mockEmptyResultProvider{name: "empty_engine"}
	chain := NewSearchChain(mock)

	// 连续调用 5 次（超过 FailureThreshold=3）
	for i := 0; i < 5; i++ {
		// 通过 SearchChain 调用，让 provider.go 的熔断逻辑生效
		_ = chain.Search(context.Background(), "冷门关键词")
	}

	// 关键断言：熔断器不应打开
	pw := chain.Providers()[0]
	if pw.IsOpen() {
		t.Errorf("空结果不应触发熔断：Failures=%d, State=%s", pw.Failures, pw.State.String())
	}
	if pw.Failures != 0 {
		t.Errorf("空结果不应累加失败计数：Failures=%d（应为 0）", pw.Failures)
	}
	if pw.State != CircuitClosed {
		t.Errorf("熔断器应保持 Closed，实际: %s", pw.State.String())
	}
}

// TestCircuitBreaker_ErrorStillTriggersFailure 验证：真正的 error 仍会触发熔断
// 确保修复空结果误判时，没有破坏对真实错误的熔断能力
func TestCircuitBreaker_ErrorStillTriggersFailure(t *testing.T) {
	mock := &mockErrorProvider{name: "error_engine"}
	chain := NewSearchChain(mock)

	// 连续 3 次错误应触发熔断
	for i := 0; i < 3; i++ {
		_ = chain.Search(context.Background(), "test")
	}

	pw := chain.Providers()[0]
	if !pw.IsOpen() {
		t.Errorf("连续 3 次 error 应触发熔断，State=%s, Failures=%d", pw.State.String(), pw.Failures)
	}
}

// TestCircuitBreaker_EmptyThenErrorNotTripFast 验证：空结果不累加失败计数
// 场景：2 次空结果 + 1 次错误，不应触发熔断（因为空结果不计入失败）
func TestCircuitBreaker_EmptyThenErrorNotTripFast(t *testing.T) {
	// 构造混合链：先空结果，后错误
	emptyMock := &mockEmptyResultProvider{name: "empty_engine"}
	errorMock := &mockErrorProvider{name: "error_engine"}
	chain := NewSearchChain(emptyMock, errorMock)

	// 2 次调用（每次都会尝试 empty_engine 空结果，然后 error_engine 错误）
	for i := 0; i < 2; i++ {
		_ = chain.Search(context.Background(), "test")
	}

	// 检查 empty_engine：2 次空结果不应触发熔断
	emptyPW := chain.Providers()[0]
	if emptyPW.IsOpen() {
		t.Errorf("2 次空结果不应熔断 empty_engine：Failures=%d", emptyPW.Failures)
	}
	if emptyPW.Failures != 0 {
		t.Errorf("empty_engine 失败计数应为 0（空结果不计入），实际: %d", emptyPW.Failures)
	}

	// 检查 error_engine：2 次错误（每次调用都降级到 error_engine）
	// FailureThreshold=3，2 次错误不应触发熔断
	errorPW := chain.Providers()[1]
	if errorPW.IsOpen() {
		t.Errorf("2 次错误不应熔断 error_engine（阈值 3）：Failures=%d", errorPW.Failures)
	}
}

// mockEmptyResultProvider 模拟 HTTP 200 + 空结果的 provider
type mockEmptyResultProvider struct {
	name string
}

func (m *mockEmptyResultProvider) Name() string { return m.name }
func (m *mockEmptyResultProvider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	return &SearchResponse{
		Engine:  m.name,
		Results: []SearchResult{}, // 空结果
	}, nil
}
func (m *mockEmptyResultProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	return m.Search(ctx, query)
}

// mockErrorProvider 模拟始终返回 error 的 provider
type mockErrorProvider struct {
	name string
}

func (m *mockErrorProvider) Name() string { return m.name }
func (m *mockErrorProvider) Search(ctx context.Context, query string) (*SearchResponse, error) {
	return nil, errMockSearchFailed
}
func (m *mockErrorProvider) SearchWithOpts(ctx context.Context, query string, opts SearchOpts) (*SearchResponse, error) {
	return m.Search(ctx, query)
}

var errMockSearchFailed = &mockSearchError{}

type mockSearchError struct{}

func (e *mockSearchError) Error() string { return "mock search failed" }
