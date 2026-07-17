// Package apperror 提供项目统一的错误类型体系。
//
// 设计目标：
//   - 用错误类型（ErrorKind）替代字符串匹配，让上层能用 errors.Is/As 精准判断错误
//   - 保留错误链（%w），让错误可追溯
//   - 提供哨兵错误（Sentinel errors）作为常见的、可判断的错误值
//
// 生活类比：
//
// 想象医院的分诊系统。每个病人（错误）都有一个分诊标签（ErrorKind）：
// "NotFound"、"Timeout"、"Invalid"... 医生（调用方）看标签就知道该挂什么科，
// 不需要听病人描述完症状（字符串匹配）再猜。
//
// 同时，病人会带着完整的病历（错误链）流转，上一个科室的诊断结果不会丢失。
package apperror

import (
	"errors"
	"fmt"
)

// ErrorKind 错误类型枚举
//
// 生活类比：分诊标签，用类型而非文字描述来分类错误。
type ErrorKind int

const (
	// KindUnknown 未知错误（默认值，不应主动使用）
	KindUnknown ErrorKind = iota
	// KindNotFound 资源不存在（文件、模型、会话等）
	KindNotFound
	// KindAlreadyExists 资源已存在（重复创建）
	KindAlreadyExists
	// KindInvalidInput 输入参数无效（格式、范围、类型等）
	KindInvalidInput
	// KindInvalidConfig 配置无效或缺失
	KindInvalidConfig
	// KindTimeout 操作超时
	KindTimeout
	// KindUnavailable 服务不可用（服务未启动、连接失败等）
	KindUnavailable
	// KindPermission 权限不足（文件、目录访问等）
	KindPermission
	// KindConflict 状态冲突（并发修改、版本冲突等）
	KindConflict
	// KindInternal 内部错误（不应暴露给用户的系统级错误）
	KindInternal
)

// String 返回错误类型的字符串表示，便于日志输出
func (k ErrorKind) String() string {
	switch k {
	case KindNotFound:
		return "NotFound"
	case KindAlreadyExists:
		return "AlreadyExists"
	case KindInvalidInput:
		return "InvalidInput"
	case KindInvalidConfig:
		return "InvalidConfig"
	case KindTimeout:
		return "Timeout"
	case KindUnavailable:
		return "Unavailable"
	case KindPermission:
		return "Permission"
	case KindConflict:
		return "Conflict"
	case KindInternal:
		return "Internal"
	default:
		return "Unknown"
	}
}

// Error 是项目统一的错误类型，实现了 error 接口。
//
// 生活类比：一张带分诊标签的病历单。
// - Kind: 分诊标签（错误类型）
// - Message: 病历摘要（人类可读的错误描述）
// - Cause: 转诊来源（底层错误，保留错误链）
//
// 使用方式：
//
//	// 创建新错误（无底层 err）
//	err := apperror.New(apperror.KindNotFound, "模型文件不存在")
//
//	// 包装底层错误（保留错误链）
//	err := apperror.Wrap(apperror.KindInvalidConfig, "读取配置失败", originalErr)
//
//	// 上层判断
//	if errors.Is(err, apperror.ErrNotFound) { ... }
//	if apperror.KindOf(err) == apperror.KindTimeout { ... }
type Error struct {
	// Kind 错误类型
	Kind ErrorKind
	// Message 人类可读的错误描述
	Message string
	// Cause 底层错误（可能为 nil）
	Cause error
}

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %s: %v", e.Kind, e.Message, e.Cause)
	}
	return fmt.Sprintf("%s: %s", e.Kind, e.Message)
}

// Unwrap 支持 errors.Is 和 errors.As 递归解包
func (e *Error) Unwrap() error {
	return e.Cause
}

// Is 实现 errors.Is 的自定义比较逻辑。
//
// 默认 errors.Is 只比较指针相等，但我们的使用场景是：
// 用 New/Wrap 创建的 *Error 应该能与同 Kind 的哨兵错误匹配。
//
// 生活类比：两个病人都有"NotFound"的分诊标签，即使病历号不同，
// 也应该被分到同一个科室。这里比较的是"标签"（Kind）而非"病历号"（指针）。
//
// 这样上层可以写：
//
//	if errors.Is(err, apperror.ErrNotFound) { ... }
//
// 只要 err 是任何 Kind=KindNotFound 的 *Error，都会返回 true。
func (e *Error) Is(target error) bool {
	if t, ok := target.(*Error); ok {
		return e.Kind == t.Kind
	}
	return false
}

// --- 哨兵错误（Sentinel Errors）---
//
// 生活类比：急救电话号码。你想判断"这是不是资源不存在的错误"，
// 直接 errors.Is(err, apperror.ErrNotFound) 即可，不需要描述完整症状。
//
// 哨兵错误本身不携带具体信息，只用于类型判断。
// 实际返回错误时应该用 New/Wrap 创建带详细信息的 Error。
var (
	// ErrNotFound 资源不存在的哨兵错误
	ErrNotFound = &Error{Kind: KindNotFound, Message: "not found"}
	// ErrAlreadyExists 资源已存在的哨兵错误
	ErrAlreadyExists = &Error{Kind: KindAlreadyExists, Message: "already exists"}
	// ErrInvalidInput 输入无效的哨兵错误
	ErrInvalidInput = &Error{Kind: KindInvalidInput, Message: "invalid input"}
	// ErrInvalidConfig 配置无效的哨兵错误
	ErrInvalidConfig = &Error{Kind: KindInvalidConfig, Message: "invalid config"}
	// ErrTimeout 操作超时的哨兵错误
	ErrTimeout = &Error{Kind: KindTimeout, Message: "timeout"}
	// ErrUnavailable 服务不可用的哨兵错误
	ErrUnavailable = &Error{Kind: KindUnavailable, Message: "unavailable"}
	// ErrPermission 权限不足的哨兵错误
	ErrPermission = &Error{Kind: KindPermission, Message: "permission denied"}
	// ErrConflict 状态冲突的哨兵错误
	ErrConflict = &Error{Kind: KindConflict, Message: "conflict"}
	// ErrInternal 内部错误的哨兵错误
	ErrInternal = &Error{Kind: KindInternal, Message: "internal error"}
)

// --- 构造函数 ---

// New 创建一个新错误（无底层 err）
//
// 用法：
//
//	err := apperror.New(apperror.KindNotFound, "模型文件不存在: " + path)
func New(kind ErrorKind, message string) *Error {
	return &Error{Kind: kind, Message: message}
}

// Newf 创建一个带格式化的新错误（无底层 err）
//
// 用法：
//
//	err := apperror.Newf(apperror.KindInvalidInput, "不支持的文件类型: %s", ext)
func Newf(kind ErrorKind, format string, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

// Wrap 包装一个底层错误，保留错误链
//
// 用法：
//
//	err := apperror.Wrap(apperror.KindInvalidConfig, "读取配置失败", originalErr)
//	// 上层可以用 errors.Is(err, originalErr) 拿到原始错误
func Wrap(kind ErrorKind, message string, cause error) *Error {
	return &Error{Kind: kind, Message: message, Cause: cause}
}

// Wrapf 包装一个底层错误，message 支持格式化
//
// 用法：
//
//	err := apperror.Wrapf(apperror.KindInternal, "解密消息 %s 失败", msgID, originalErr)
func Wrapf(kind ErrorKind, format string, cause error, args ...any) *Error {
	return &Error{Kind: kind, Message: fmt.Sprintf(format, args...), Cause: cause}
}

// --- 查询函数 ---

// KindOf 返回错误的 ErrorKind。
//
// 如果错误是 *Error 类型，返回其 Kind；
// 如果错误通过 errors.As 能解包到 *Error，返回对应的 Kind；
// 否则返回 KindUnknown。
//
// 用法：
//
//	switch apperror.KindOf(err) {
//	case apperror.KindNotFound:
//	    // 处理不存在的情况
//	case apperror.KindTimeout:
//	    // 处理超时
//	}
func KindOf(err error) ErrorKind {
	if err == nil {
		return KindUnknown
	}
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindUnknown
}

// Is 判断错误是否属于指定的 ErrorKind。
//
// 用法：
//
//	if apperror.Is(err, apperror.KindTimeout) {
//	    // 超时了，可以重试
//	}
func Is(err error, kind ErrorKind) bool {
	return KindOf(err) == kind
}
