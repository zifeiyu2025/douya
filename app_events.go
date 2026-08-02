// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package main

import (
	"context"

	"douya/internal/chat"

	"github.com/wailsapp/wails/v2/pkg/runtime"
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
		runtime.EventsEmit(p.ctx, "chat:stream", event)
	}
}
