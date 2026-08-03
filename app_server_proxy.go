package main

import (
	"context"

	"douya/internal/apperror"
)

// AnthropicMessages 代理 Anthropic Messages API
// 前端传入原始 JSON 请求体字符串，后端透传到 /v1/messages 端点，返回原始 JSON 响应体字符串
func (a *App) AnthropicMessages(body string) (string, error) {
	if a.getClient() == nil {
		return "", apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	if err := validateJSONBody(body); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), httpClientTimeout)
	defer cancel()
	respBody, err := a.getClient().AnthropicMessages(ctx, []byte(body))
	if err != nil {
		return "", err
	}
	return string(respBody), nil
}

// AnthropicCountTokens 代理 Anthropic token 计数
// 前端传入原始 JSON 请求体字符串，后端透传到 /v1/messages/count_tokens 端点，返回原始 JSON 响应体字符串
func (a *App) AnthropicCountTokens(body string) (string, error) {
	if a.getClient() == nil {
		return "", apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	if err := validateJSONBody(body); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), apiTimeoutMedium)
	defer cancel()
	respBody, err := a.getClient().AnthropicCountTokens(ctx, []byte(body))
	if err != nil {
		return "", err
	}
	return string(respBody), nil
}

// BuiltInTools 代理内置工具
// 前端传入原始 JSON 请求体字符串，后端透传到 /tools 端点，返回原始 JSON 响应体字符串
func (a *App) BuiltInTools(body string) (string, error) {
	if a.getClient() == nil {
		return "", apperror.New(apperror.KindUnavailable, "客户端未初始化")
	}
	if err := validateJSONBody(body); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(context.Background(), httpClientTimeout)
	defer cancel()
	respBody, err := a.getClient().BuiltInTools(ctx, []byte(body))
	if err != nil {
		return "", err
	}
	return string(respBody), nil
}
