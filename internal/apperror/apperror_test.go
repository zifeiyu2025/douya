// apperror 包的单元测试
package apperror

import (
	"errors"
	"fmt"
	"testing"
)

// TestError_Error 验证 Error.Error() 的字符串输出格式
func TestError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "无底层错误",
			err:  New(KindNotFound, "模型文件不存在"),
			want: "NotFound: 模型文件不存在",
		},
		{
			name: "有底层错误",
			err:  Wrap(KindInvalidConfig, "读取配置失败", errors.New("文件不存在")),
			want: "InvalidConfig: 读取配置失败: 文件不存在",
		},
		{
			name: "格式化新错误",
			err:  Newf(KindInvalidInput, "不支持的文件类型: %s", ".exe"),
			want: "InvalidInput: 不支持的文件类型: .exe",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestErrorKind_String 验证 ErrorKind.String() 的输出
func TestErrorKind_String(t *testing.T) {
	tests := []struct {
		kind ErrorKind
		want string
	}{
		{KindNotFound, "NotFound"},
		{KindAlreadyExists, "AlreadyExists"},
		{KindInvalidInput, "InvalidInput"},
		{KindInvalidConfig, "InvalidConfig"},
		{KindTimeout, "Timeout"},
		{KindUnavailable, "Unavailable"},
		{KindPermission, "Permission"},
		{KindConflict, "Conflict"},
		{KindInternal, "Internal"},
		{KindUnknown, "Unknown"},
		{ErrorKind(99), "Unknown"}, // 未知值
	}
	for _, tt := range tests {
		if got := tt.kind.String(); got != tt.want {
			t.Errorf("Kind(%d).String() = %q, want %q", tt.kind, got, tt.want)
		}
	}
}

// TestError_Unwrap 验证 Unwrap 能返回底层错误，支持错误链
func TestError_Unwrap(t *testing.T) {
	baseErr := errors.New("原始错误")
	wrapped := Wrap(KindInternal, "上层错误", baseErr)

	if unwrapped := wrapped.Unwrap(); unwrapped != baseErr {
		t.Errorf("Unwrap() = %v, want %v", unwrapped, baseErr)
	}

	// 无底层错误时 Unwrap 返回 nil
	noCause := New(KindNotFound, "无底层错误")
	if unwrapped := noCause.Unwrap(); unwrapped != nil {
		t.Errorf("Unwrap() = %v, want nil", unwrapped)
	}
}

// TestError_Is 验证 errors.Is 自定义比较逻辑
//
// 这是 apperror 包的核心机制：任何 Kind=KindNotFound 的 *Error
// 都应该与哨兵错误 ErrNotFound 匹配。
func TestError_Is(t *testing.T) {
	baseErr := errors.New("文件不存在")

	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{
			name:   "New创建的错误匹配同Kind哨兵",
			err:    New(KindNotFound, "模型文件不存在"),
			target: ErrNotFound,
			want:   true,
		},
		{
			name:   "Wrap创建的错误匹配同Kind哨兵",
			err:    Wrap(KindInvalidConfig, "读取失败", baseErr),
			target: ErrInvalidConfig,
			want:   true,
		},
		{
			name:   "不同Kind不匹配",
			err:    New(KindNotFound, "不存在"),
			target: ErrTimeout,
			want:   false,
		},
		{
			name:   "与普通error比较返回false",
			err:    New(KindNotFound, "不存在"),
			target: errors.New("普通错误"),
			want:   false,
		},
		{
			name:   "Newf创建的错误匹配同Kind哨兵",
			err:    Newf(KindInvalidInput, "不支持的类型: %s", "exe"),
			target: ErrInvalidInput,
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errors.Is(tt.err, tt.target); got != tt.want {
				t.Errorf("errors.Is(%v, %v) = %v, want %v", tt.err, tt.target, got, tt.want)
			}
		})
	}
}

// TestError_Is_NestedErrors 验证嵌套错误链中的 errors.Is
//
// 错误链：apperror.Wrap(KindInternal, "外层", apperror.Wrap(KindNotFound, "内层", baseErr))
//
// Go 的 errors.Is 会递归整个错误链查找匹配项，类似 os.Open 包装 fs.ErrNotExist 后，
// errors.Is(err, fs.ErrNotExist) 仍然返回 true。
//
// 因此外层错误应该：
//   - 匹配 ErrInternal（外层自己的 Kind）
//   - 也匹配 ErrNotFound（内层的 Kind，通过 Unwrap 递归找到）
//   - 也匹配 baseErr（最底层的原始错误）
func TestError_Is_NestedErrors(t *testing.T) {
	baseErr := errors.New("文件不存在")
	inner := Wrap(KindNotFound, "内层错误", baseErr)
	outer := Wrap(KindInternal, "外层错误", inner)

	// 外层匹配 Internal（自己的 Kind）
	if !errors.Is(outer, ErrInternal) {
		t.Error("外层应匹配 ErrInternal")
	}
	// 外层也匹配 NotFound（内层的 Kind，递归查找）
	if !errors.Is(outer, ErrNotFound) {
		t.Error("外层应通过递归匹配到内层的 ErrNotFound")
	}
	// 外层能解到底层 baseErr
	if !errors.Is(outer, baseErr) {
		t.Error("外层应能解到底层 baseErr")
	}
	// KindOf 返回最外层的 Kind
	if got := KindOf(outer); got != KindInternal {
		t.Errorf("KindOf(外层) = %v, want Internal（最外层优先）", got)
	}
}

// TestKindOf 验证 KindOf 函数
func TestKindOf(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want ErrorKind
	}{
		{"nil错误", nil, KindUnknown},
		{"New创建的NotFound", New(KindNotFound, "不存在"), KindNotFound},
		{"Wrap创建的Timeout", Wrap(KindTimeout, "超时", errors.New("cause")), KindTimeout},
		{"嵌套错误取外层Kind", Wrap(KindInternal, "外层", New(KindNotFound, "内层")), KindInternal},
		{"普通error返回Unknown", errors.New("普通"), KindUnknown},
		{"fmt.Errorf包装的返回Unknown", fmt.Errorf("包装: %w", New(KindNotFound, "不存在")), KindNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := KindOf(tt.err); got != tt.want {
				t.Errorf("KindOf(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

// TestIs 验证包级别的 Is 函数
func TestIs_Func(t *testing.T) {
	if !Is(New(KindTimeout, "超时"), KindTimeout) {
		t.Error("Is(Timeout错误, KindTimeout) 应返回 true")
	}
	if Is(New(KindTimeout, "超时"), KindNotFound) {
		t.Error("Is(Timeout错误, KindNotFound) 应返回 false")
	}
	if Is(nil, KindNotFound) {
		t.Error("Is(nil, KindNotFound) 应返回 false")
	}
}

// TestWrapf 验证 Wrapf 格式化包装
func TestWrapf(t *testing.T) {
	baseErr := errors.New("磁盘已满")
	wrapped := Wrapf(KindInternal, "写入文件 %s 失败", baseErr, "/tmp/config.json")

	if wrapped.Kind != KindInternal {
		t.Errorf("Kind = %v, want Internal", wrapped.Kind)
	}
	if wrapped.Message != "写入文件 /tmp/config.json 失败" {
		t.Errorf("Message = %q, want %q", wrapped.Message, "写入文件 /tmp/config.json 失败")
	}
	if wrapped.Cause != baseErr {
		t.Errorf("Cause = %v, want %v", wrapped.Cause, baseErr)
	}
	if !errors.Is(wrapped, baseErr) {
		t.Error("应能解到底层错误")
	}
}

// TestSentinelErrors 验证所有哨兵错误的 Kind
func TestSentinelErrors(t *testing.T) {
	sentinels := []struct {
		name     string
		err      error
		wantKind ErrorKind
	}{
		{"ErrNotFound", ErrNotFound, KindNotFound},
		{"ErrAlreadyExists", ErrAlreadyExists, KindAlreadyExists},
		{"ErrInvalidInput", ErrInvalidInput, KindInvalidInput},
		{"ErrInvalidConfig", ErrInvalidConfig, KindInvalidConfig},
		{"ErrTimeout", ErrTimeout, KindTimeout},
		{"ErrUnavailable", ErrUnavailable, KindUnavailable},
		{"ErrPermission", ErrPermission, KindPermission},
		{"ErrConflict", ErrConflict, KindConflict},
		{"ErrInternal", ErrInternal, KindInternal},
	}
	for _, s := range sentinels {
		t.Run(s.name, func(t *testing.T) {
			if got := KindOf(s.err); got != s.wantKind {
				t.Errorf("%s.Kind = %v, want %v", s.name, got, s.wantKind)
			}
			// 哨兵错误应能与自己匹配
			if !errors.Is(s.err, s.err) {
				t.Errorf("%s 应能与自身匹配", s.name)
			}
		})
	}
}
