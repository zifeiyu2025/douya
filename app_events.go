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
	// 模型下载（内置下载器，来源 ModelScope / HF 镜像）事件。
	EventModelDownloadProgress = "model:downloadProgress"
	EventModelDownloadComplete = "model:downloadComplete"
	EventWindowCloseRequest      = "window:closeRequest"
	EventShutdownProgress        = "shutdown:progress"
	EventSearchAutoDisabled      = "search:autoDisabled"
	// EventStartupError: 启动期致命错误。后端遇到无法继续启动的错误时推送，
	// 前端据此在启动屏上展示错误卡（标题/简述/详情），用户确认后后端才退出。
	// 生活类比：店门口亮起"暂停营业"红灯，顾客先看到原因再关门。
	EventStartupError = "startup:error"
	// EventBackendDownloadRequest: 后端在 runtime 缺失且需要下载时推送，通知前端
	// 弹"是否下载后端"对话框。后端在 channel 上阻塞等待，前端确认后调
	// ResolveBackendDownloadConfirm 放行。
	// 生活类比：店里的发动机仓库缺货，店家广播问顾客"要不要帮您订购"，
	// 顾客答复（写 channel）后店家才决定下单（下载）还是关门。
	EventBackendDownloadRequest = "startup:backendDownloadRequest"
	// EventStartupRagDisabled: 知识库（RAG）初始化失败时推送，提示前端用非阻塞
	// 的方式告知用户"知识库已禁用，但基本对话不受影响"，不打断启动流程。
	EventStartupRagDisabled = "startup:ragDisabled"
	// EventStartupModelNotice: 没有可用的模型时推送，前端用非阻塞提示展示
	// 一段"如何下载模型"的引导文案，用户看完可正常进入界面。
	EventStartupModelNotice = "startup:modelNotice"
	// P3.6 修复：EventUpdateCheck 已被移除——前端使用同步 CheckUpdate RPC，
	// 无事件监听者，保留 EventUpdateProgress（前端有订阅）。
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
