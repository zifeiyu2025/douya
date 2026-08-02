// Copyright zifeiyu. All rights reserved.
// 豆芽本地AI

// Package modelruntime contains framework-independent state used to coordinate
// a local model runtime. Process management stays in the host layer while this
// package owns concurrent model-session transitions.
package modelruntime

import "sync"

// SessionSnapshot is a consistent view of the active model and any in-flight
// switch. It is intended for UI status, health checks, and diagnostics.
type SessionSnapshot struct {
	CurrentModel string
	Switching    bool
	SwitchingTo  string
}

// Session serializes model switch ownership. A switch is deliberately a
// single-writer operation because llama.cpp Router changes VRAM allocations and
// model state asynchronously.
type Session struct {
	mu           sync.RWMutex
	currentModel string
	switching    bool
	switchingTo  string
}

func NewSession() *Session { return &Session{} }

func (s *Session) SetCurrentModel(model string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.currentModel = model
}

func (s *Session) CurrentModel() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentModel
}

// BeginSwitch atomically reserves the session for a target model. It returns
// false if another switch (including its rollback) still owns the session.
func (s *Session) BeginSwitch(target string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.switching {
		return false
	}
	s.switching = true
	s.switchingTo = target
	return true
}

// ClearTarget keeps the switch reservation while hiding its target. This is
// used during rollback: another switch must not start until recovery ends.
func (s *Session) ClearTarget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.switchingTo = ""
}

// EndSwitch releases the switch reservation after success or rollback.
func (s *Session) EndSwitch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.switching = false
	s.switchingTo = ""
}

func (s *Session) Snapshot() SessionSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return SessionSnapshot{
		CurrentModel: s.currentModel,
		Switching:    s.switching,
		SwitchingTo:  s.switchingTo,
	}
}
