package main

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestApp_ExitingFlag_InitialState(t *testing.T) {
	app := NewApp()
	if app.exiting.Load() {
		t.Error("exiting flag should be false initially")
	}
}

func TestApp_BeforeClose_ReturnsFalseWhenExiting(t *testing.T) {
	app := NewApp()
	app.exiting.Store(true)

	result := app.shouldPreventClose()
	if result {
		t.Error("beforeClose should return false (allow close) when exiting is true, but shouldPreventClose returned true")
	}
}

func TestApp_BeforeClose_ReturnsTrueWhenNotExiting(t *testing.T) {
	app := NewApp()

	result := app.shouldPreventClose()
	if !result {
		t.Error("beforeClose should return true (prevent close) when exiting is false, but shouldPreventClose returned false")
	}
}

func TestApp_GracefulExit_SetsExitingToTrue(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()

	started := app.tryStartExit()
	if !started {
		t.Error("tryStartExit should return true on first call")
	}
	if !app.exiting.Load() {
		t.Error("exiting flag should be true after tryStartExit succeeds")
	}
}

func TestApp_GracefulExit_DoesNotStartTwice(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()

	first := app.tryStartExit()
	second := app.tryStartExit()

	if !first {
		t.Error("first tryStartExit should return true")
	}
	if second {
		t.Error("second tryStartExit should return false (already exiting)")
	}
}

func TestApp_GracefulExit_DoesNotStartWithNilContext(t *testing.T) {
	app := NewApp()
	app.ctx = nil

	started := app.tryStartExit()
	if started {
		t.Error("tryStartExit should return false when ctx is nil")
	}
	if app.exiting.Load() {
		t.Error("exiting flag should remain false when ctx is nil")
	}
}

func TestApp_BeforeClose_TransitionsWhenExitingChanges(t *testing.T) {
	app := NewApp()

	if !app.shouldPreventClose() {
		t.Error("should prevent close initially")
	}

	app.exiting.Store(true)

	if app.shouldPreventClose() {
		t.Error("should allow close after exiting is set")
	}
}

func TestApp_HiddenFlag_InitialState(t *testing.T) {
	app := NewApp()
	if app.hidden.Load() {
		t.Error("hidden flag should be false initially")
	}
}

func TestApp_ExitingFlag_IsAtomic(t *testing.T) {
	app := NewApp()
	app.ctx = context.Background()

	var successes atomic.Int32
	const goroutines = 100

	done := make(chan struct{})
	for range goroutines {
		go func() {
			if app.tryStartExit() {
				successes.Add(1)
			}
			done <- struct{}{}
		}()
	}

	for range goroutines {
		<-done
	}

	if successes.Load() != 1 {
		t.Errorf("expected exactly 1 successful tryStartExit, got %d", successes.Load())
	}
}
