// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"
	"time"

	zlog "github.com/rs/zerolog/log"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// StartupError 是一次启动期致命错误的描述，通过 EventStartupError 推送给前端，
// 前端据此在启动屏上渲染"启动错误卡"（标题 + 简述 + 详情 + 退出按钮）。
//
// 生活类比：店门口的红灯牌，正面写"暂停营业"，背面写明具体原因
// （详情），顾客看完按"我知道了"（退出）店家才落闸关门。
type StartupError struct {
	Title  string `json:"title"`  // 简短标题（前端卡片标题）
	Brief  string `json:"brief"`  // 一句话简述（前端卡片副标题）
	Detail string `json:"detail"` // 多行详情（前端卡片正文，含原因 / 建议）
}

// startupErrorAckTimeout 是启动致命错误等待前端确认的最大时长。
// 达到超时后不再等待（前端可能未渲染出来），直接 forceQuit 兜底退出，避免进程悬挂。
// 生活类比：等顾客看告示，最多等这些秒；顾客一直没来，就先关门，防止一直耗着。
const startupErrorAckTimeout = 30 * time.Second

// confirmStartupError 是 ConfirmStartupError RPC 的内部实现，非阻塞唤醒等待者。
// 即使 channel 已关闭或未初始化也不会 panic。
func (a *App) confirmStartupError() {
	a.startupErrorMu.Lock()
	ch := a.startupErrorChan
	a.startupErrorMu.Unlock()
	if ch != nil {
		select {
		case ch <- struct{}{}:
		default: // 已有人确认过，丢弃本次
		}
	}
}

// ConfirmStartupError 由前端在用户点击"启动错误卡"的退出按钮后调用，
// 通知后端"用户已看到错误信息，可以退出了"，解除 emitFatalError 的阻塞等待。
//
// 生活类比：顾客看完"暂停营业"告示，按下确认键通知店家可以关门了。
func (a *App) ConfirmStartupError() error {
	a.confirmStartupError()
	return nil
}

// GetStartupError 由前端启动挂载后兜底查询"当前是否有待确认的启动致命错误"。
//
// 为什么需要它：后端在 WebView 尚未挂载完时就可能触发 EventStartupError，
// 前端若恰好错过事件推送，错误卡就不会出现，只剩后端空白等待 30 秒后退出。
// 前端 onMounted 后调用本方法，若返回非空则主动展示错误卡，避免信息丢失。
//
// 生活类比：顾客进店时告示可能已经贴了很久（事件错过了），但店家还要再口头
// 确认一遍"您看到暂停营业的原因了吗"——保证每个顾客都不会错过。
func (a *App) GetStartupError() *StartupError {
	a.startupErrorMu.Lock()
	defer a.startupErrorMu.Unlock()
	if a.startupError == nil {
		return nil
	}
	cp := *a.startupError
	return &cp
}

// emitFatalError 统一处理启动期致命错误：记录状态 → 推送到前端 → 等待前端确认后退出。
//
// 与旧的 runtime.MessageDialog 不同：错误信息先通过事件交给前端，
// 在启动屏上以"前端风格"的错误卡展示（而非操作系统级对话框），
// 等待用户点击退出按钮后再 forceQuit。
//
// 设计要点：
//  1. 前端可能尚未渲染（WebView 加载中），此时 ErrorCard 未必能立刻显示，
//     因此必须有超时兜底——达到 startupErrorAckTimeout 后仍无人确认则直接退出。
//  2. forceQuit 会触发 runtime.Quit / systray.Quit / os.Exit，见 app_lifecycle.go。
//  3. 调用方在 emitFatalError 返回后应直接 return（不再继续启动流程）。
func (a *App) emitFatalError(ctx context.Context, title, brief, detail string) {
	err := &StartupError{Title: title, Brief: brief, Detail: detail}

	// 1. 记录状态（前端可通过 GetStartupError 兜底查询，或直接订阅事件）
	a.startupErrorMu.Lock()
	a.startupError = err
	ch := make(chan struct{}, 1)
	a.startupErrorChan = ch
	a.startupErrorMu.Unlock()

	// 2. 推送事件给前端
	if ctx != nil {
		runtime.EventsEmit(ctx, EventStartupError, err)
	}

	// 3. 阻塞等待前端确认，或超时兜底退出
	select {
	case <-ch:
		zlog.Info().Str("title", title).Msg("[startup] 用户已确认启动错误，退出")
	case <-time.After(startupErrorAckTimeout):
		zlog.Warn().Str("title", title).Msg("[startup] 等待启动错误确认超时，强制退出")
	case <-ctx.Done():
		zlog.Info().Str("title", title).Msg("[startup] 应用关闭，取消等待启动错误确认")
	}

	// 清理 channel，避免泄漏
	a.startupErrorMu.Lock()
	a.startupErrorChan = nil
	a.startupErrorMu.Unlock()

	a.forceQuit()
}