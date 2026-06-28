// Package httputil 提供 HTTP 相关的公共工具函数。
package httputil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// ReadBodyLimited 读取 r 的内容，最多读取 maxBytes 字节。
// 超出部分被静默截断，用于防止读取过大的响应体导致内存耗尽。
func ReadBodyLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, maxBytes))
}

// DoAndUnmarshal 执行 HTTP 请求并将响应体反序列化到目标类型 T。
// 消除 readBody + StatusCode 检查 + Unmarshal 的重复模式。
// 返回：反序列化结果、原始响应体、错误（错误时 body 仍返回供调用方诊断）
func DoAndUnmarshal[T any](client *http.Client, req *http.Request, maxBodySize int64) (*T, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := ReadBodyLimited(resp.Body, maxBodySize)
	if err != nil {
		return nil, nil, fmt.Errorf("read body failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, body, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, body, fmt.Errorf("unmarshal failed: %w", err)
	}
	return &result, body, nil
}

// ReadErrorBody 读取非 200 响应的 body 并格式化错误信息。
// 消除重复的错误响应处理模式。
func ReadErrorBody(resp *http.Response, prefix string) error {
	body, _ := ReadBodyLimited(resp.Body, 64*1024) // 错误响应体限制 64KB
	return fmt.Errorf("%s, status %d: %s", prefix, resp.StatusCode, string(body))
}
