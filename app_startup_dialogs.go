// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"time"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// ===== 启动期"用户二选一"对话框的前端化 =====
// 将原本的 runtime.MessageDialog（操作系统级对话框）改为前端风格：
//   1. 后端通过事件把"要问什么"推给前端（前端渲染对话框）
//   2. 后端在对应 channel 上阻塞等待
//   3. 前端用户选择后调用对应的 ResolveXxx RPC 把结果写回 channel，解除阻塞
//
// 生活类比：以前店家有事直接在店门口挂块木板大喊（操作系统弹窗），
// 现在改成把问题写进"意见本"（事件）交给前台，呆在柜台前等回执（channel），
// 顾客（前端）在漂亮的界面上作答后再把回执（RPC）交回来。

// BackendDownloadRequestPayload 是 EventBackendDownloadRequest 事件发给前端的数据。
// 前端据此在对话框中展示"检测到哪些显卡 / 推荐哪个后端 / 缺了哪些文件"。
type BackendDownloadRequestPayload struct {
	GPUName        string `json:"gpu_name"`         // 检测到的显卡名称（可能为空）
	BackendName    string `json:"backend_name"`     // 推荐后端的展示名（如 "CUDA（NVIDIA）"）
	BackendType    string `json:"backend_type"`     // 推荐后端类型字符串（如 "cuda"）
	MissingFiles   string `json:"missing_files"`    // 缺失文件清单（每行一条，已格式化）
	TimeoutSeconds int    `json:"timeout_seconds"`  // 超时秒数，前端可显示倒计时
	SourceURL      string `json:"source_url"`       // 下载来源说明
}

// backendChoiceTimeout 是"是否下载后端"对话框等待用户答复的最长时间。
// 超时后默认继续下载，保证开箱即用、不打断启动。
const backendChoiceTimeout = 60 * time.Second

// writeChoice 非阻塞地向 channel 写一个布尔结果，即使 channel 已超时/清理也不会 panic。
func writeChoice(ch chan bool, proceed bool) {
	if ch == nil {
		return
	}
	select {
	case ch <- proceed:
	default: // 接收方已走（超时/关闭），本次答复作废
	}
}

// ===== 1. 后端下载确认 =====

// ResolveBackendDownloadConfirm 由前端在用户对"是否下载后端"对话框作答后调用。
// proceed=true 表示"下载"（推荐），false 表示"不下载，退出应用"。
//
// 设计为非阻塞写入 channel，即使后端已超时走人也安全。
func (a *App) ResolveBackendDownloadConfirm(proceed bool) error {
	zlog.Info().Bool("proceed", proceed).Msg("[startup] 用户对后端下载询问作出答复")
	writeChoice(a.backendDownloadChan, proceed)
	return nil
}

// waitForBackendDownloadConfirm 发送 EventBackendDownloadRequest 通知前端弹"是否下载"对话框，
// 并阻塞等待用户答复。返回 true 表示"下载"，false 表示"不下载"。
//
// 超时兜底：前端可能未挂载（对话框没弹出来），此时默认返回 true（继续下载），
// 与原实现"对话框调用失败默认走下载路径"的行为保持一致，保证开箱即用。
func (a *App) waitForBackendDownloadConfirm(ctx context.Context, payload BackendDownloadRequestPayload) bool {
	ch := make(chan bool, 1)
	a.backendDownloadChan = ch
	defer func() { a.backendDownloadChan = nil }()

	zlog.Info().
		Str("backend", payload.BackendType).
		Msg("[startup] 通知前端询问是否下载推理后端")

	if ctx != nil {
		runtime.EventsEmit(ctx, EventBackendDownloadRequest, payload)
	}

	select {
	case proceed := <-ch:
		return proceed
	case <-time.After(backendChoiceTimeout):
		zlog.Warn().Msg("[startup] 等待下载确认超时，默认继续下载")
		return true
	case <-ctx.Done():
		zlog.Info().Msg("[startup] 应用关闭，取消等待下载确认")
		return true
	}
}