// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

import (
	"context"
	"sync"
	"testing"
	"time"

	"douya/internal/config"
	"douya/internal/rag"
)

// TestConfigSnapshot_NoDataRace 验证并发读写 s.config 时不会发生数据竞争。
//
// 生活类比：想象 config 是一个共享的笔记本，UpdateConfig 是有人在写新内容，
// 而 GetThinkingSoftSwitch / calcMaxTokens 是有人在读内容。如果读写不同步，
// 读者可能读到写了一半的内容（数据竞争）。快照方法就像让读者在锁保护下
// 快速复印一份再读，避免和写者冲突。
//
// 此测试用 -race 标志运行时：
//   - 修复前：检测到数据竞争（GetThinkingSoftSwitch / calcMaxTokens 无锁读取 s.config）
//   - 修复后：通过（这些方法通过 getConfigSnapshot() 在锁保护下读取）
func TestConfigSnapshot_NoDataRace(t *testing.T) {
	// 创建一个 Service 实例，config 非 nil
	s := NewService(nil, nil, nil, &config.Config{
		ContextSize:        4096,
		Temperature:        0.7,
		ThinkingEnabled:    true,
		ThinkingSoftSwitch: "auto",
	}, nil, "")

	var wg sync.WaitGroup
	n := 200

	// 写者 goroutine：反复调用 UpdateConfig 更新配置
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < n; j++ {
				s.UpdateConfig(&config.Config{
					ContextSize:        4096 + j,
					Temperature:        0.5 + float64(j)*0.01,
					ThinkingEnabled:    j%2 == 0,
					ThinkingSoftSwitch: "auto",
				})
			}
		}()
	}

	// 读者 goroutine：反复调用读取 s.config 的方法
	// 修复前：GetThinkingSoftSwitch 和 calcMaxTokens 直接读取 s.config（无锁）
	// 修复后：它们通过 getConfigSnapshot() 在锁保护下读取
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < n; j++ {
				// 这两个方法在修复前会无锁读取 s.config 的多个字段
				_ = s.GetThinkingSoftSwitch()
				_ = s.calcMaxTokens(100)
			}
		}()
	}

	wg.Wait()
}

// blockingEmbedder 是一个阻塞式 embedder，仅在 context 取消或超时 10 秒后返回。
// 用于测试 buildLLMMessages 是否正确将 context 传播到 RAG 嵌入调用。
type blockingEmbedder struct{}

func (e *blockingEmbedder) Embed(ctx context.Context, texts []string) ([][]float64, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(10 * time.Second):
		return [][]float64{{0.1, 0.2, 0.3}}, nil
	}
}

// TestBuildLLMMessages_ContextCancellation 验证 buildLLMMessages 接收 context 参数，
// 并且当 context 被取消时，RAG 嵌入调用能立即返回，而不是等待 5 秒超时。
//
// 生活类比：就像你在餐厅点餐后等待上菜，如果你突然有事要离开（取消），
// 服务员应该立刻停止准备（context 传播），而不是继续做完再告诉你。
//
// 修复前：buildLLMMessages 不接收 ctx 参数（测试无法编译）
// 修复后：buildLLMMessages 接收 ctx，RAG 嵌入使用 ctx，取消能立即生效
func TestBuildLLMMessages_ContextCancellation(t *testing.T) {
	s := NewService(nil, nil, nil, &config.Config{
		ContextSize: 4096,
	}, nil, "")

	// 设置 RAG：使用阻塞式 embedder，VectorStore 用空对象即可
	// （embedder.Embed 会先阻塞，不会走到 HybridSearch）
	s.SetRAG(&rag.VectorStore{}, nil, &blockingEmbedder{}, "test-collection", true)

	ctx, cancel := context.WithCancel(context.Background())

	// 在另一个 goroutine 中延迟取消 context
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	// 调用 buildLLMMessages，传入可取消的 ctx
	// 修复前：此行无法编译（buildLLMMessages 不接收 ctx 参数）
	_, _, _ = s.buildLLMMessages(ctx, nil, "测试查询内容", nil, "off", "")
	elapsed := time.Since(start)

	// 验证：context 取消后应立即返回，不应等待 5 秒超时
	// 留 2 秒余量（100ms 取消 + 处理时间）
	if elapsed > 2*time.Second {
		t.Errorf("buildLLMMessages 在 context 取消后未及时返回（耗时 %v），context 未正确传播到 RAG 嵌入调用", elapsed)
	}
}
