// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"douya/internal/rag"
)

// newMinimalApp 构造一个仅初始化 rootCtx/g 的最小 App 实例，
// 用于测试 shutdownInternal 的逻辑分支，避免依赖 Wails runtime / db / server 等。
func newMinimalApp() *App {
	a := &App{}
	a.rootCtx, a.rootCancel = context.WithCancel(context.Background())
	return a
}

// TestShutdown_NilResourcesNoPanic 验证：资源均为 nil 时 shutdownInternal 安全跳过各关闭步骤，
// 且 stopOnce 保证 PrepareShutdown + shutdown 多次调用不会重复进入关闭逻辑（无 panic）。
func TestShutdown_NilResourcesNoPanic(t *testing.T) {
	a := newMinimalApp()

	// 资源均为 nil，shutdownInternal 应安全跳过 service/server/ragVS/db 的关闭
	a.PrepareShutdown()
	a.shutdown(context.Background()) // stopOnce 应跳过
	a.shutdown(context.Background()) // 再次调用，确保幂等

	// rootCtx 应已被取消（shutdownInternal 调用了 rootCancel）
	select {
	case <-a.rootCtx.Done():
		// 预期行为
	default:
		t.Fatal("shutdownInternal 应调用 rootCancel 使 rootCtx 取消")
	}
}

// TestShutdown_StopOnceIdempotent 验证 stopOnce 保证关闭逻辑只执行一次。
// shutdownInternal 依赖 stopOnce 实现幂等性（PrepareShutdown 后再 shutdown 不重复关闭资源），
// 本测试直接验证该机制。
func TestShutdown_StopOnceIdempotent(t *testing.T) {
	a := newMinimalApp()

	var closeCount atomic.Int32
	// 模拟 shutdownInternal 内 stopOnce.Do 的行为：多次 Do 只执行一次
	a.stopOnce.Do(func() { closeCount.Add(1) })
	a.stopOnce.Do(func() { closeCount.Add(1) })
	a.stopOnce.Do(func() { closeCount.Add(1) })

	if got := closeCount.Load(); got != 1 {
		t.Fatalf("stopOnce 应保证只执行一次，实际执行 %d 次", got)
	}
}

// TestShutdown_PrepareThenShutdown_NoDoubleClose 验证 PrepareShutdown 后再调用 shutdown
// 不会重复关闭资源（stopOnce 幂等）。用真实 ragVS 观察第一次关闭生效、第二次为 no-op。
func TestShutdown_PrepareThenShutdown_NoDoubleClose(t *testing.T) {
	a := newMinimalApp()

	dir := t.TempDir()
	vs, err := rag.NewVectorStore(filepath.Join(dir, "rag"))
	if err != nil {
		t.Fatalf("创建 rag VectorStore 失败: %v", err)
	}
	a.ragVS = vs

	// 关闭前 ListCollections 应正常工作（空库返回 nil error）
	if _, err := a.ragVS.ListCollections(); err != nil {
		t.Fatalf("关闭前 ListCollections 应成功，实际: %v", err)
	}

	// 第一次：PrepareShutdown 应关闭 ragVS
	a.PrepareShutdown()

	// 关闭后 ListCollections 应失败（db 已关闭）
	if _, err := a.ragVS.ListCollections(); err == nil {
		t.Fatal("PrepareShutdown 后 ListCollections 应失败（db 已关闭），但未返回错误")
	}

	// 第二次：shutdown 应被 stopOnce 跳过，不重复关闭（不 panic 即可）
	a.shutdown(context.Background())
}

// TestShutdown_TrackedGoroutineExitsOnCancel 验证 shutdownInternal 先 rootCancel 再 g.Wait，
// 被跟踪的长生命周期 goroutine 能在 rootCtx 取消后退出，shutdown 不会永久阻塞。
// 生活类比：下班时先广播"下班了"（rootCancel），再在门口等所有人出来（g.Wait）才锁门。
func TestShutdown_TrackedGoroutineExitsOnCancel(t *testing.T) {
	a := newMinimalApp()

	started := make(chan struct{})
	exited := make(chan struct{})
	// 启动一个被跟踪的 goroutine，模拟 watcher/health 监听：阻塞至 rootCtx 取消
	a.trackedGo(func() {
		close(started)
		<-a.rootCtx.Done()
		close(exited)
	})

	// 确保被跟踪 goroutine 已进入运行状态
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("被跟踪 goroutine 未能在 1s 内启动")
	}

	// 在独立 goroutine 中调用 shutdown，避免测试因 g.Wait 阻塞而超时卡死
	done := make(chan struct{})
	go func() {
		a.shutdown(context.Background()) // 内部 rootCancel + g.Wait
		close(done)
	}()

	select {
	case <-done:
		// shutdown 返回，说明被跟踪 goroutine 已退出，g.Wait 已完成
	case <-time.After(2 * time.Second):
		// 兜底：即使测试失败也释放 rootCtx，避免 goroutine 泄漏卡住测试进程
		if a.rootCancel != nil {
			a.rootCancel()
		}
		t.Fatal("shutdown 超时，被跟踪 goroutine 未在 rootCtx 取消后退出")
	}

	select {
	case <-exited:
		// 被跟踪 goroutine 已退出
	default:
		t.Fatal("被跟踪 goroutine 未退出")
	}
}
