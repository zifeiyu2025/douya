// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

package chat

// EventPublisher is the output port used by chat use cases to notify their
// host about streaming progress. Adapters may forward events to Wails, an HTTP
// SSE connection, a CLI, or a test collector.
//
// Implementations must be safe for concurrent calls: a generation can emit
// from background goroutines while lifecycle code replaces infrastructure.
type EventPublisher interface {
	Publish(StreamEvent)
}
