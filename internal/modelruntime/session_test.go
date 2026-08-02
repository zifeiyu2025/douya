package modelruntime

import "testing"

func TestSessionSerializesSwitchesAndProvidesSnapshot(t *testing.T) {
	s := NewSession()
	s.SetCurrentModel("model-a")
	if !s.BeginSwitch("model-b") {
		t.Fatal("first switch should acquire session")
	}
	if s.BeginSwitch("model-c") {
		t.Fatal("second switch must be rejected while first is active")
	}
	if got := s.Snapshot(); got.CurrentModel != "model-a" || !got.Switching || got.SwitchingTo != "model-b" {
		t.Fatalf("snapshot = %#v", got)
	}
	s.ClearTarget()
	if got := s.Snapshot(); !got.Switching || got.SwitchingTo != "" {
		t.Fatalf("rollback snapshot = %#v", got)
	}
	s.SetCurrentModel("model-a")
	s.EndSwitch()
	if got := s.Snapshot(); got.Switching || got.CurrentModel != "model-a" {
		t.Fatalf("final snapshot = %#v", got)
	}
}
