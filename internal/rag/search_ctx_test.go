// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package rag

import (
	"context"
	"testing"
	"time"
)

// TestSearch_CancelPropagation 验证：当父 ctx 取消时，Search 应立即返回，
// 而不是等到 5 秒超时。这是 ctx 传播的关键保障：
//   - 用户关闭应用时，rootCancel 传播到 RAG 检索，避免阻塞 shutdown
//   - 用户取消请求时，检索能立即停止，释放资源
func TestSearch_CancelPropagation(t *testing.T) {
	vs, err := NewVectorStore("")
	if err != nil {
		t.Fatalf("NewVectorStore 失败: %v", err)
	}
	defer vs.Close()

	// 创建集合并添加向量
	collection := "cancel_test"
	dim := 4
	if err := vs.CreateCollection(collection, dim); err != nil {
		t.Fatalf("CreateCollection 失败: %v", err)
	}

	ids := []string{"v1", "v2"}
	vectors := [][]float64{
		{1.0, 0.0, 0.0, 0.0},
		{0.0, 1.0, 0.0, 0.0},
	}
	if err := vs.AddVectors(collection, ids, vectors); err != nil {
		t.Fatalf("AddVectors 失败: %v", err)
	}

	// 创建一个已取消的 ctx
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// 调用 Search，应立即返回（而不是等 5 秒）
	start := time.Now()
	_, _ = vs.Search(ctx, collection, []float64{1.0, 0.0, 0.0, 0.0}, 2)
	elapsed := time.Since(start)

	// 关键断言：必须在 1 秒内返回（远小于 5 秒超时）
	// 如果 ctx 没有传播，Search 会用 context.Background() 派生 5 秒超时，无法快速返回
	if elapsed > 1*time.Second {
		t.Errorf("ctx 取消后 Search 应快速返回，实际耗时 %v（应 < 1s）", elapsed)
	}
}

// TestSearchWithThreshold_CancelPropagation 验证 SearchWithThreshold 的 ctx 传播
func TestSearchWithThreshold_CancelPropagation(t *testing.T) {
	vs, err := NewVectorStore("")
	if err != nil {
		t.Fatalf("NewVectorStore 失败: %v", err)
	}
	defer vs.Close()

	collection := "thresh_cancel_test"
	dim := 4
	if err := vs.CreateCollection(collection, dim); err != nil {
		t.Fatalf("CreateCollection 失败: %v", err)
	}

	ids := []string{"v1"}
	vectors := [][]float64{{1.0, 0.0, 0.0, 0.0}}
	if err := vs.AddVectors(collection, ids, vectors); err != nil {
		t.Fatalf("AddVectors 失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, _ = vs.SearchWithThreshold(ctx, collection, []float64{1.0, 0.0, 0.0, 0.0}, 2, 0.0)
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("ctx 取消后 SearchWithThreshold 应快速返回，实际耗时 %v", elapsed)
	}
}

// TestHybridSearch_CancelPropagation 验证 HybridSearch 的 ctx 传播
func TestHybridSearch_CancelPropagation(t *testing.T) {
	vs, err := NewVectorStore("")
	if err != nil {
		t.Fatalf("NewVectorStore 失败: %v", err)
	}
	defer vs.Close()

	collection := "hybrid_cancel_test"
	dim := 4
	if err := vs.CreateCollection(collection, dim); err != nil {
		t.Fatalf("CreateCollection 失败: %v", err)
	}

	ids := []string{"v1"}
	vectors := [][]float64{{1.0, 0.0, 0.0, 0.0}}
	if err := vs.AddVectors(collection, ids, vectors); err != nil {
		t.Fatalf("AddVectors 失败: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	_, _ = vs.HybridSearch(ctx, collection, []float64{1.0, 0.0, 0.0, 0.0}, "测试", 2, 0.0)
	elapsed := time.Since(start)

	if elapsed > 1*time.Second {
		t.Errorf("ctx 取消后 HybridSearch 应快速返回，实际耗时 %v", elapsed)
	}
}

// TestSearch_NormalContextStillWorks 验证正常 ctx 下 Search 仍能正常工作
func TestSearch_NormalContextStillWorks(t *testing.T) {
	vs, err := NewVectorStore("")
	if err != nil {
		t.Fatalf("NewVectorStore 失败: %v", err)
	}
	defer vs.Close()

	collection := "normal_ctx_test"
	dim := 4
	if err := vs.CreateCollection(collection, dim); err != nil {
		t.Fatalf("CreateCollection 失败: %v", err)
	}

	ids := []string{"v1"}
	vectors := [][]float64{{1.0, 0.0, 0.0, 0.0}}
	if err := vs.AddVectors(collection, ids, vectors); err != nil {
		t.Fatalf("AddVectors 失败: %v", err)
	}

	// 正常 ctx 应能返回结果
	results, err := vs.Search(context.Background(), collection, []float64{1.0, 0.0, 0.0, 0.0}, 2)
	if err != nil {
		t.Errorf("正常 ctx 下 Search 不应报错: %v", err)
	}
	if len(results) == 0 {
		t.Error("正常 ctx 下 Search 应返回结果")
	}
}
