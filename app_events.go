// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"

	"douya/internal/chat"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// 前后端事件名常量
// 生活类比：事件名是前后端之间的"电报频道号"——双方约定好频道号才能互相收发消息。
// 全部集中在这里定义，避免 60+ 处硬编码字符串散落各处，防止"频道号写错"导致的通信故障。
const (
	EventChatStream              = "chat:stream"
	EventChatAbnormalCleanup     = "chat:abnormal_cleanup"
	EventServerStatus            = "server:status"
	EventServerLog               = "server:log"
	EventServerTerminal          = "server:terminal"
	EventServerWarning           = "server:warning"
	EventServerSwitchProgress    = "server:switchProgress"
	EventServerMmprojUnavailable = "server:mmprojUnavailable"
	EventModelLoadProgress       = "modelLoadProgress"
	EventBackendSwitched         = "backend:switched"
	EventBackendDownloadStart    = "backend:downloadStart"
	EventBackendDownloadProgress = "backend:downloadProgress"
	EventBackendDownloadComplete = "backend:downloadComplete"
	EventWindowCloseRequest      = "window:closeRequest"
	EventShutdownProgress        = "shutdown:progress"
	EventSearchAutoDisabled      = "search:autoDisabled"
	// P3.6 修复：EventUpdateCheck 已被移除——前端使用同步 CheckUpdate RPC，
	// 无事件监听者，emit 是死代码。保留 EventUpdateProgress（前端有订阅）。
	EventUpdateProgress = "update:progress"
)

// wailsChatEventPublisher is the Wails adapter for the chat.EventPublisher
// output port. It intentionally lives in the application host layer so the
// chat package remains usable by non-Wails hosts.
type wailsChatEventPublisher struct {
	ctx context.Context
}

func newWailsChatEventPublisher(ctx context.Context) chat.EventPublisher {
	return wailsChatEventPublisher{ctx: ctx}
}

func (p wailsChatEventPublisher) Publish(event chat.StreamEvent) {
	if p.ctx != nil {
		runtime.EventsEmit(p.ctx, EventChatStream, event)
	}
}
