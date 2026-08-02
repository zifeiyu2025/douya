package chat

import (
	"sync"
	"testing"
)

type eventCollector struct {
	mu     sync.Mutex
	events []StreamEvent
}

func (c *eventCollector) Publish(event StreamEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
}

func TestServicePublishesEventsThroughOutputPort(t *testing.T) {
	s := NewService(nil, nil, nil, nil, nil, "")
	collector := &eventCollector{}
	s.SetEventPublisher(collector)

	s.emit("token", "hello")
	s.emitForConv("conv-1", "done", nil)

	collector.mu.Lock()
	defer collector.mu.Unlock()
	if len(collector.events) != 2 {
		t.Fatalf("got %d events, want 2", len(collector.events))
	}
	if got := collector.events[0]; got.Type != "token" || got.Content != "hello" {
		t.Fatalf("first event = %#v, want token event", got)
	}
	if got := collector.events[1]; got.Type != "done" || got.ConversationID != "conv-1" {
		t.Fatalf("second event = %#v, want conversation event", got)
	}
}
