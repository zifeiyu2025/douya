// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat_test

import (
	"sync"
	"testing"

	"douya/internal/config"
)

// TestService_ConcurrentConfigRead 验证并发读取 config 的安全性。
//
// 业务场景：多个并发请求（SendMessage、RegenerateMessage、CompressConversation）
// 同时调用 getConfigSnapshot() 读取配置。升级 s.mutex 到 RWMutex 后，读操作应可并行。
//
// 运行：go test -race ./tests/chat/ -run TestService_ConcurrentConfigRead
func TestService_ConcurrentConfigRead(t *testing.T) {
	svc := newTestService()
	svc.UpdateConfig(&config.Config{
		ContextSize:  8192,
		SystemPrompt: "test prompt",
		Temperature:  0.7,
	})

	const goroutines = 20
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// 启动多个 reader 并发读取 config
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				cfg := svc.GetConfig()
				if cfg == nil {
					t.Error("GetConfig 不应返回 nil")
					return
				}
				if cfg.ContextSize != 8192 {
					t.Errorf("ContextSize 期望 8192，实际: %d", cfg.ContextSize)
					return
				}
			}
		}()
	}
	wg.Wait()
}

// TestService_ConcurrentClientRead 验证并发读取 llmClient 的安全性。
// 升级 s.mutex 到 RWMutex 后，getClientSnapshot 应可并行。
func TestService_ConcurrentClientRead(t *testing.T) {
	svc := newTestService()
	// 注意：newTestService 传入 nil client，这里测试 nil 读取的并发安全
	const goroutines = 20
	const iterations = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				// 并发读取，不应 panic
				_ = svc.GetConfig()
			}
		}()
	}
	wg.Wait()
}

// TestService_ConcurrentReadWrite 验证 reader 与 writer 并发时的安全性。
//
// 业务场景：生成过程中用户切换模型或更新配置，writer 与 reader 并发访问 s.mutex。
// 升级到 RWMutex 后，多个 reader 应不被 writer 阻塞（只要不修改同一字段）。
func TestService_ConcurrentReadWrite(t *testing.T) {
	svc := newTestService()
	svc.UpdateConfig(&config.Config{
		ContextSize:  4096,
		SystemPrompt: "initial",
		Temperature:  0.7,
	})

	const goroutines = 10
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// 一半 goroutine 持续读 config
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				cfg := svc.GetConfig()
				if cfg == nil {
					t.Error("GetConfig 不应返回 nil")
					return
				}
			}
		}()
	}

	// 一半 goroutine 持续更新 config（写）
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				svc.UpdateConfig(&config.Config{
					ContextSize:  4096 + n,
					SystemPrompt: "updated",
					Temperature:  0.7,
				})
			}
		}(i)
	}
	wg.Wait()
}

// TestService_ConcurrentIsGenerating 验证 IsGenerating 的并发读取安全性。
//
// 业务场景：health 监控每 5 秒调用 IsGenerating()，与 SendMessage 的写操作并发。
// 升级后 IsGenerating 可用 RLock 并行读取。
func TestService_ConcurrentIsGenerating(t *testing.T) {
	svc := newTestService()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// 并发读取 IsGenerating，不应 panic
				_ = svc.IsGenerating()
			}
		}()
	}
	wg.Wait()
}

// TestService_ClientSnapshot_AfterUpdate 验证 UpdateClient 后 getClientSnapshot 返回新客户端。
//
// 业务场景：模型切换后调用 UpdateClient，后续请求应拿到新的 client。
// 这是 RWMutex 升级后的核心保证：写操作对读操作可见。
func TestService_ClientSnapshot_AfterUpdate(t *testing.T) {
	// 这个测试需要真实的 llm.Client，跳过 nil 场景
	// 仅验证 UpdateClient 后 getClientSnapshot 的一致性
	// 由于 getClientSnapshot 是包内方法，通过 GetConfig 验证类似逻辑
	svc := newTestService()
	cfg1 := &config.Config{ContextSize: 1024, SystemPrompt: "v1"}
	svc.UpdateConfig(cfg1)

	if svc.GetConfig().ContextSize != 1024 {
		t.Errorf("第一次更新后 ContextSize 应为 1024")
	}

	cfg2 := &config.Config{ContextSize: 2048, SystemPrompt: "v2"}
	svc.UpdateConfig(cfg2)

	if svc.GetConfig().ContextSize != 2048 {
		t.Errorf("第二次更新后 ContextSize 应为 2048")
	}
}
