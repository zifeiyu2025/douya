// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"testing"
)

// TestStart_CleansUpOldCancel 验证重复调用 Start() 时，旧的 cancel 函数会被调用，避免资源泄漏
//
// 生活类比：想象 Server 是一台电视机，cancel 函数是关机遥控器。
// 每次开机（Start）都会拿到一个新的遥控器，但如果旧的遥控器没被按下（旧 cancel 没调用），
// 旧电视的某些后台进程还在偷偷耗电（资源泄漏）。所以开机前要先按一下旧遥控器关掉旧进程。
//
// 由于 Start() 会启动真实的 llama-server 子进程，无法在单元测试中直接调用，
// 因此提取了 replaceContext() 方法来封装"清理旧 cancel + 创建新 context"的逻辑，
// 这里直接测试 replaceContext() 方法。
func TestStart_CleansUpOldCancel(t *testing.T) {
	// 创建一个 Server 实例（使用空配置即可，因为不会真正启动进程）
	s := NewServer(&ServerConfig{})

	// 手动设置一个旧的 cancel 函数，模拟之前 Start() 调用留下的状态
	// oldCtx 会被 oldCancel 取消，通过检查 oldCtx.Err() 可以判断 cancel 是否被调用
	oldCtx, oldCancel := context.WithCancel(context.Background())
	s.ctx = oldCtx
	s.cancel = oldCancel

	// 调用 replaceContext，模拟 Start() 中创建新 context 前的清理逻辑
	s.replaceContext()

	// 验证旧 cancel 被调用：旧 ctx 应该处于已取消状态（Err() 返回非 nil）
	if oldCtx.Err() == nil {
		t.Error("旧 cancel 函数未被调用，存在资源泄漏隐患")
	}

	// 验证新 cancel 不为空（确保新 context 已正确创建）
	if s.cancel == nil {
		t.Error("新 cancel 函数未被设置")
	}

	// 验证新 ctx 不为空且处于活动状态
	if s.ctx == nil {
		t.Error("新 ctx 未被设置")
	}
	if s.ctx.Err() != nil {
		t.Error("新 ctx 不应处于已取消状态")
	}

	// 清理新创建的 cancel，避免测试本身产生泄漏
	if s.cancel != nil {
		s.cancel()
	}
}

// TestStart_ReplaceContext_NilOldCancel 验证当旧 cancel 为 nil 时（首次启动场景），
// replaceContext() 不会 panic
func TestStart_ReplaceContext_NilOldCancel(t *testing.T) {
	s := NewServer(&ServerConfig{})

	// 首次启动时 s.cancel 为 nil，replaceContext 不应 panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("replaceContext 在 cancel 为 nil 时不应 panic: %v", r)
		}
	}()

	s.replaceContext()

	if s.cancel == nil {
		t.Error("新 cancel 函数未被设置")
	}

	// 清理
	if s.cancel != nil {
		s.cancel()
	}
}
