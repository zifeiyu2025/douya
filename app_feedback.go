// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// reportEmail 开发者邮箱：用户举报的 AI 不当内容统一发往该地址。
// 微软商店政策 11.16「实时生成式人工智能内容」要求产品必须为用户提供
// 举报 AI 生成不当内容的途径，本方法即该合规入口的后端实现。
const reportEmail = "510144286@qq.com"

// ReportProblem 组装举报邮件并调用系统默认邮件客户端打开（mailto）。
// 豆芽是本地 AI 应用，不经过任何云端服务器，故举报通过用户本机邮件发出，
// 既能到达开发者邮箱，又不经手第三方服务。
//
// 参数：
//   - content：被举报的 AI 生成内容
//   - reason：举报原因（如"色情或不当内容"）
//   - remark：用户的补充说明（可为空）
//
// 说明：runtime.BrowserOpenURL 为尽力打开（void），若系统无默认邮件客户端
// 可能静默失败；前端已配套"复制举报内容"兜底按钮，保证举报内容不丢失。
func (a *App) ReportProblem(content string, reason string, remark string) error {
	// 组装邮件正文，固定结构方便开发者快速定位被举报内容
	var sb strings.Builder
	sb.WriteString("举报原因：" + reason)
	sb.WriteString("\n\n")
	if strings.TrimSpace(remark) != "" {
		sb.WriteString("补充说明：" + strings.TrimSpace(remark))
		sb.WriteString("\n\n")
	}
	sb.WriteString("被举报的 AI 内容：\n")
	sb.WriteString("----------------------------------------\n")
	sb.WriteString(content)
	sb.WriteString("\n----------------------------------------\n")

	mailto := fmt.Sprintf("mailto:%s?subject=%s&body=%s",
		reportEmail,
		url.QueryEscape("【豆芽】AI 内容举报"),
		url.QueryEscape(sb.String()),
	)

	runtime.BrowserOpenURL(a.ctx, mailto)
	return nil
}
