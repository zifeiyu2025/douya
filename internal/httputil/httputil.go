// Package httputil 提供 HTTP 相关的公共工具函数。
package httputil

import (
	"io"
)

// ReadBodyLimited 读取 r 的内容，最多读取 maxBytes 字节。
// 超出部分被静默截断，用于防止读取过大的响应体导致内存耗尽。
func ReadBodyLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxBytes))
}
