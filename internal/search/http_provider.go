// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package search

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"

	"douya/internal/apperror"
)

// BaseHTTPSearchProvider 为基于 HTTP+JSON 的搜索 Provider 提供通用骨架。
//
// 它在 BaseProvider（HTTP 请求骨架）之上，新增：
//   - apiKey 字段：所有 JSON 搜索 Provider 都需要 API Key 鉴权
//   - doSearchJSON 方法：封装 "marshal 请求 → 构造鉴权 headers → doSearch → unmarshal 响应" 通用流程
//
// 各 Provider 通过嵌入 BaseHTTPSearchProvider 复用通用流程，仅定义差异部分：
//   - 搜索 URL（Tavily: api.tavily.com/search，Ollama: ollama.com/api/web_search）
//   - 请求体结构（Tavily 含 max_results/include_answer 等字段，Ollama 仅 query）
//   - 响应结构（Tavily 含 answer/score，Ollama 仅 title/url/content）
//   - HTTP 超时（Tavily 30s 支持 advanced search，Ollama 20s）
//
// 生活类比：BaseHTTPSearchProvider 像"标准化快递柜"——
//
//	快递员（具体 Provider）只需知道"目的地"（URL）、"包裹内容"（请求体）、"取货格式"（响应结构），
//	寄件、运输、拆箱的通用流程由快递柜自动完成。
type BaseHTTPSearchProvider struct {
	BaseProvider
	apiKey string
}

// doSearchJSON 执行 JSON POST 搜索请求并解析 JSON 响应。
//
// 通用流程：
//  1. marshal reqBody 为 JSON
//  2. 构造鉴权 headers（Content-Type + Authorization Bearer）
//  3. 调用 BaseProvider.doSearch 发送请求
//  4. unmarshal 响应体到 respOut
//
// 安全说明：headers 中的 Authorization 由本方法构造，不进入 error 信息（doSearch 已脱敏）。
//
// 参数：
//   - ctx: 请求 context
//   - url: 搜索 API 端点
//   - reqBody: 请求体（任意可 marshal 为 JSON 的结构）
//   - respOut: 响应体解析目标（指针，由调用方定义具体结构）
//
// 返回：error 已包含 marshal/doSearch/unmarshal 各阶段上下文，调用方可直接 wrap provider 名前缀。
func (p *BaseHTTPSearchProvider) doSearchJSON(ctx context.Context, url string, reqBody any, respOut any) error {
	body, err := json.Marshal(reqBody)
	if err != nil {
		return apperror.Wrap(apperror.KindInternal, "marshal request", err)
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + p.apiKey,
	}

	respBody, err := p.doSearch(ctx, http.MethodPost, url, bytes.NewReader(body), headers)
	if err != nil {
		return err
	}

	if err := json.Unmarshal(respBody, respOut); err != nil {
		return apperror.Wrap(apperror.KindInternal, "unmarshal response", err)
	}

	return nil
}
