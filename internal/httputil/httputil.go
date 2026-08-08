// Package httputil 提供 HTTP 相关的公共工具函数。
package httputil

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"douya/internal/apperror"
)

// ErrBodyTooLarge 表示响应体超过 maxBytes 上限而被安全截断。
// 用于让调用方感知"读到的内容不完整"，避免把被截断的合法大响应误判为解析失败/熔断。
var ErrBodyTooLarge = errors.New("response body exceeds limit")

// ReadBodyLimited 读取 r 的内容，最多读取 maxBytes 字节。
// 若实际内容超过 maxBytes，返回已读取的前 maxBytes 字节并附带 ErrBodyTooLarge，
// 让调用方明确"数据被截断"，而不是静默得到一个不完整的 body。
func ReadBodyLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	// 多读 1 字节以检测是否还有更多内容，从而判断是否发生截断。
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return data[:maxBytes], ErrBodyTooLarge
	}
	return data, nil
}

// DoAndUnmarshal 执行 HTTP 请求并将响应体反序列化到目标类型 T。
// 消除 readBody + StatusCode 检查 + Unmarshal 的重复模式。
// 返回：反序列化结果、原始响应体、错误（错误时 body 仍返回供调用方诊断）
func DoAndUnmarshal[T any](client *http.Client, req *http.Request, maxBodySize int64) (*T, []byte, error) {
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, apperror.Wrap(apperror.KindUnavailable, "request failed", err)
	}
	defer resp.Body.Close()

	body, err := ReadBodyLimited(resp.Body, maxBodySize)
	if err != nil && !errors.Is(err, ErrBodyTooLarge) {
		return nil, nil, apperror.Wrap(apperror.KindUnavailable, "read body failed", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, body, apperror.Newf(apperror.KindUnavailable, "unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, body, apperror.Wrap(apperror.KindUnavailable, "unmarshal failed", err)
	}
	return &result, body, nil
}

// ReadErrorBody 读取非 200 响应的 body 并格式化错误信息。
// 消除重复的错误响应处理模式。
func ReadErrorBody(resp *http.Response, prefix string) error {
	body, _ := ReadBodyLimited(resp.Body, 64*1024) // 错误响应体限制 64KB
	return apperror.Newf(apperror.KindUnavailable, "%s, status %d: %s", prefix, resp.StatusCode, string(body))
}
