// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package llm

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestDetectOOMInStderr 验证 OOM 模式检测：显存/内存不足应命中，普通日志不命中。
func TestDetectOOMInStderr(t *testing.T) {
	cases := []struct {
		name    string
		stderr  string
		wantOOM bool
	}{
		{name: "空输出", stderr: "", wantOOM: false},
		{name: "CUDA OOM", stderr: "CUDA error: out of memory\n", wantOOM: true},
		{name: "cuda_error_out_of_memory", stderr: "cuda_error_out_of_memory during alloc", wantOOM: true},
		{name: "显存分配失败", stderr: "failed to allocate CUDA memory", wantOOM: true},
		{name: "not enough gpu memory", stderr: "not enough gpu memory", wantOOM: true},
		{name: "vram 关键词", stderr: "VRAM insufficient 24576 MiB needed", wantOOM: true},
		{name: "std::bad_alloc", stderr: "std::bad_alloc", wantOOM: true},
		{name: "mmap failed", stderr: "mmap failed: cannot allocate memory", wantOOM: true},
		{name: "普通加载日志", stderr: "load model ok, backend cuda, ngl=99", wantOOM: false},
		{name: "普通启动日志", stderr: "llama server listening at 127.0.0.1:8080", wantOOM: false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DetectOOMInStderr(tc.stderr); got != tc.wantOOM {
				t.Errorf("期望 detectOOMInStderr=%v，实际 %v（stderr=%q）", tc.wantOOM, got, tc.stderr)
			}
		})
	}
}

// TestLastStartTimeConcurrentAccess 验证 lastStartTime 的并发读写无数据竞争。
// 配合 `go test -race` 运行可检测竞争：多个 goroutine 同时 Store/Load，
// 若 lastStartTime 不是 atomic 类型，race detector 会报告竞争。
//
// P1-1 修复回归测试：lastStartTime 已从 time.Time 改为 atomic.Int64，
// WatchWithCallback 中无锁读取与 Start 中持锁写入之间不再有竞争。
func TestLastStartTimeConcurrentAccess(t *testing.T) {
	s := &Server{}
	// 预置一个非零起始值，模拟 Start 中已写入
	s.lastStartTime.Store(time.Now().UnixNano())

	var stop atomic.Bool
	var wg sync.WaitGroup

	// 写入 goroutine：模拟 Start() 中 s.lastStartTime.Store(...)
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				s.lastStartTime.Store(time.Now().UnixNano())
			}
		}()
	}

	// 读取 goroutine：模拟 WatchWithCallback 中无锁读取
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for !stop.Load() {
				ns := s.lastStartTime.Load()
				// 确保能还原为 time.Time 并计算差值（与生产代码读取方式一致）
				startTime := time.Unix(0, ns)
				_ = time.Since(startTime)
			}
		}()
	}

	time.Sleep(100 * time.Millisecond)
	stop.Store(true)
	wg.Wait()
}

// TestEnhanceExitError_ExitCodeFormats 验证不同格式的 exit_code 字符串均能正确匹配。
//
// P2-3 修复回归测试：原实现用 strings.Contains 硬匹配 "exit_code=-1073740791"，
// 正数形式（3221226507）或分隔符变化会漏匹配。改用正则提取数字后按数值匹配。
func TestEnhanceExitError_ExitCodeFormats(t *testing.T) {
	tests := []struct {
		name    string
		errMsg  string
		port    int
		wantSub string // 期望返回值包含的关键子串
	}{
		// 0xC0000409 STATUS_STACK_BUFFER_OVERRUN
		{name: "stack_buffer_overrun_negative_equals", errMsg: "server exited with error: exit_code=-1073740791", port: 8080, wantSub: "0xC0000409"},
		{name: "stack_buffer_overrun_positive_colon", errMsg: "exit_code: 3221226507", port: 8080, wantSub: "0xC0000409"},
		{name: "stack_buffer_overrun_positive_equals", errMsg: "exit_code=3221226507", port: 8080, wantSub: "0xC0000409"},
		// 0xC00000FD STATUS_STACK_OVERFLOW
		{name: "stack_overflow_negative_equals", errMsg: "exit_code=-1073741571", port: 8080, wantSub: "0xC00000FD"},
		{name: "stack_overflow_positive_colon_space", errMsg: "exit_code: 3221225725", port: 8080, wantSub: "0xC00000FD"},
		// 0xC0000005 STATUS_ACCESS_VIOLATION
		{name: "access_violation_negative_equals", errMsg: "exit_code=-1073741819", port: 8080, wantSub: "0xC0000005"},
		{name: "access_violation_positive_colon", errMsg: "exit_code: 3221225477", port: 8080, wantSub: "0xC0000005"},
		{name: "access_violation_space_separator", errMsg: "error occurred exit_code 3221225477 done", port: 8080, wantSub: "0xC0000005"},
		{name: "access_violation_equals_with_space", errMsg: "exit_code= -1073741819", port: 8080, wantSub: "0xC0000005"},
		// 嵌在更长文本中
		{name: "embedded_in_long_text", errMsg: "llama-server crashed, exit_code=-1073741819, see log for details", port: 8080, wantSub: "0xC0000005"},
		// 端口占用
		{name: "port_in_use", errMsg: "address already in use", port: 8080, wantSub: "端口 8080 被占用"},
		// 未知退出码：应原样返回
		{name: "unknown_exit_code_unchanged", errMsg: "exit_code=1 some error", port: 8080, wantSub: "exit_code=1 some error"},
		{name: "no_exit_code_unchanged", errMsg: "some random error", port: 8080, wantSub: "some random error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := enhanceExitError(tt.errMsg, tt.port)
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("enhanceExitError(%q, %d) = %q, 期望包含子串 %q", tt.errMsg, tt.port, got, tt.wantSub)
			}
		})
	}
}
