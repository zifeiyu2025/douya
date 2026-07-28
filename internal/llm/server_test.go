// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
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

// TestStart_ExposeServerValidation 验证开启局域网暴露时强制要求 API Key 的安全校验
//
// 生活类比：就像开店时如果要把后门也打开（暴露到局域网），就必须配一把后门钥匙（API Key），
// 否则任何人都能从后门进出，太危险了。这个测试就是检查"开后门但没配钥匙"的情况会被拒绝。
//
// 覆盖三种配置组合：
//   - 暴露 + 已启用 API Key + 有密钥 → 校验通过（不返回校验错误）
//   - 暴露 + 未启用 API Key → 校验失败
//   - 暴露 + 已启用 API Key + 无密钥 → 校验失败
//   - 不暴露 → 校验通过（不返回校验错误）
func TestStart_ExposeServerValidation(t *testing.T) {
	// 校验失败时应返回明确错误的错误信息片段
	const validationErrSnippet = "开启局域网暴露"

	// 用例 1：暴露 + 已启用 API Key + 有密钥 → 应通过校验（不返回校验错误）
	// 注意：Start() 通过校验后会尝试启动进程，因 ServerPath 不存在会返回其他错误，
	// 这里只验证返回的错误不是校验错误
	s1 := NewServer(&ServerConfig{
		ExposeServer:        true,
		ServerAPIKeyEnabled: true,
		APIKey:              "test-key-123",
		ServerPath:          "nonexistent_llama_server.exe",
	})
	err1 := s1.Start()
	if err1 != nil && strings.Contains(err1.Error(), validationErrSnippet) {
		t.Errorf("用例1（暴露+有Key）：不应返回校验错误，但得到: %v", err1)
	}

	// 用例 2：暴露 + 未启用 API Key → 应返回校验错误
	s2 := NewServer(&ServerConfig{
		ExposeServer:        true,
		ServerAPIKeyEnabled: false,
		APIKey:              "test-key-123",
	})
	err2 := s2.Start()
	if err2 == nil || !strings.Contains(err2.Error(), validationErrSnippet) {
		t.Errorf("用例2（暴露+未启用Key）：应返回校验错误，但得到: %v", err2)
	}

	// 用例 3：暴露 + 已启用 API Key + 无密钥 → 应返回校验错误
	s3 := NewServer(&ServerConfig{
		ExposeServer:        true,
		ServerAPIKeyEnabled: true,
		APIKey:              "",
	})
	err3 := s3.Start()
	if err3 == nil || !strings.Contains(err3.Error(), validationErrSnippet) {
		t.Errorf("用例3（暴露+无密钥）：应返回校验错误，但得到: %v", err3)
	}

	// 用例 4：不暴露 → 应通过校验（不返回校验错误）
	s4 := NewServer(&ServerConfig{
		ExposeServer:        false,
		ServerAPIKeyEnabled: false,
		APIKey:              "",
		ServerPath:          "nonexistent_llama_server.exe",
	})
	err4 := s4.Start()
	if err4 != nil && strings.Contains(err4.Error(), validationErrSnippet) {
		t.Errorf("用例4（不暴露）：不应返回校验错误，但得到: %v", err4)
	}
}

// TestWatchWithCallback_PermanentFailure 验证服务器反复崩溃后进入永久失败状态
//
// 生活类比：就像一台自动售货机如果连续卡货好几次，管理员会把它切换到"故障停用"模式，
// 不再自动重试，而是等人工检修。这个测试就是验证售货机在连续失败后会正确停机。
//
// 测试策略：配置极小的重试次数和退避时间，使用不存在的 ServerPath 让 Start() 快速失败，
// 验证 WatchWithCallback 在达到上限后退出 goroutine 且 permanentFailure==true。
func TestWatchWithCallback_PermanentFailure(t *testing.T) {
	s := NewServer(&ServerConfig{
		ServerPath: "nonexistent_llama_server.exe",
	})
	// 配置极小参数便于快速测试（默认 10 次 * 2s 退避太久）
	s.maxRestartAttempts = 3
	s.initialBackoff = 1 * time.Millisecond
	s.pollInterval = 1 * time.Millisecond

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	done := make(chan struct{})
	go func() {
		s.WatchWithCallback(ctx, nil, nil)
		close(done)
	}()

	select {
	case <-done:
		// WatchWithCallback 已正常返回
	case <-time.After(10 * time.Second):
		t.Fatal("WatchWithCallback 未在 10 秒内返回，可能未正确进入永久失败状态")
	}

	if !s.IsPermanentFailure() {
		t.Error("连续启动失败后 permanentFailure 应为 true")
	}
}

// TestIsValidCacheType_AllowedTypes 验证所有允许的 cache 类型返回 true
//
// 生活类比：就像安检口的"允许携带物品清单"，清单上的东西（f32、q8_0 等）可以放心通过。
// 这个测试确保清单完整，不会误拦合法物品。
func TestIsValidCacheType_AllowedTypes(t *testing.T) {
	allowed := []string{"f32", "f16", "bf16", "q8_0", "q4_0", "q4_1", "iq4_nl", "q5_0", "q5_1", "nvfp4"}
	for _, ct := range allowed {
		if !isValidCacheType(ct) {
			t.Errorf("isValidCacheType(%q) 期望 true，实际 false", ct)
		}
	}
}

// TestIsValidCacheType_CaseInsensitive 验证大小写不敏感
// 用户可能输入大写（如 "Q8_0"），应被接受
func TestIsValidCacheType_CaseInsensitive(t *testing.T) {
	cases := []string{"Q8_0", "F32", "BF16", "Q4_0", "IQ4_NL", "Q5_1", "NVFP4"}
	for _, ct := range cases {
		if !isValidCacheType(ct) {
			t.Errorf("isValidCacheType(%q) 大小写不敏感应返回 true，实际 false", ct)
		}
	}
}

// TestIsValidCacheType_RemovedTypes 验证已删除的 cache 类型返回 false
// llama-server 9631 版本删除了 q2_k, q3_k, q4_k, q5_k, q6_k, iq4_xs
// 如果误接受这些类型，会导致 llama-server 启动失败
func TestIsValidCacheType_RemovedTypes(t *testing.T) {
	removed := []string{"q2_k", "q3_k", "q4_k", "q5_k", "q6_k", "iq4_xs"}
	for _, ct := range removed {
		if isValidCacheType(ct) {
			t.Errorf("isValidCacheType(%q) 已删除的类型应返回 false，实际 true", ct)
		}
	}
}

// TestIsValidCacheType_UnknownAndEmpty 验证未知类型和空字符串返回 false
func TestIsValidCacheType_UnknownAndEmpty(t *testing.T) {
	cases := []string{"", "unknown", "q9_0", "f64", "int8"}
	for _, ct := range cases {
		if isValidCacheType(ct) {
			t.Errorf("isValidCacheType(%q) 未知/空类型应返回 false，实际 true", ct)
		}
	}
}

// TestEnhanceStartError_Nil 验证 nil 错误返回 nil
func TestEnhanceStartError_Nil(t *testing.T) {
	got := enhanceStartError(nil)
	if got != nil {
		t.Errorf("enhanceStartError(nil) 期望 nil，实际 %v", got)
	}
}

// TestEnhanceStartError_DLLMissing 验证 DLL 缺失错误返回中文提示
//
// 生活类比：就像翻译官，把 Windows 的英文报错翻译成用户能懂的中文，
// 并告诉用户去哪里找问题（runtime/ 目录）。
func TestEnhanceStartError_DLLMissing(t *testing.T) {
	cases := []struct {
		name    string
		errMsg  string
		wantSub string
	}{
		{"module not found", "The specified module could not be found", "DLL 文件缺失"},
		{"dll not found", "foo.dll not found", "DLL 文件缺失"},
		{".dll + not found", "bar.dll: not found in path", "DLL 文件缺失"},
		{".dll + cannot find", "cannot find baz.dll", "DLL 文件缺失"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			orig := errors.New(c.errMsg)
			got := enhanceStartError(orig)
			if got == nil {
				t.Fatalf("enhanceStartError 不应返回 nil")
			}
			if !strings.Contains(got.Error(), c.wantSub) {
				t.Errorf("应包含 %q，实际 %q", c.wantSub, got.Error())
			}
			// 应保留原始错误信息
			if !strings.Contains(got.Error(), c.errMsg) {
				t.Errorf("应保留原始错误 %q，实际 %q", c.errMsg, got.Error())
			}
		})
	}
}

// TestEnhanceStartError_EngineMissing 验证引擎文件不存在错误返回中文提示
func TestEnhanceStartError_EngineMissing(t *testing.T) {
	cases := []struct {
		name    string
		errMsg  string
		wantSub string
	}{
		{"file not specified", "The system cannot find the file specified", "引擎程序文件不存在"},
		{"no such file", "no such file or directory", "引擎程序文件不存在"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			orig := errors.New(c.errMsg)
			got := enhanceStartError(orig)
			if got == nil {
				t.Fatalf("enhanceStartError 不应返回 nil")
			}
			if !strings.Contains(got.Error(), c.wantSub) {
				t.Errorf("应包含 %q，实际 %q", c.wantSub, got.Error())
			}
		})
	}
}

// TestEnhanceStartError_UnknownError 验证未知错误原样返回
func TestEnhanceStartError_UnknownError(t *testing.T) {
	orig := fmt.Errorf("some unknown startup error")
	got := enhanceStartError(orig)
	if got != orig {
		t.Errorf("未知错误应原样返回，期望 %v，实际 %v", orig, got)
	}
}
