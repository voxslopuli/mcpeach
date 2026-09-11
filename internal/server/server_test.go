package server

import (
	"errors"
	"testing"
)

// TestStopReturnsErrNotRunning verifies Manager.Stop classifies "not running"
// with the typed sentinel so callers can use errors.Is instead of matching text.
func TestStopReturnsErrNotRunning(t *testing.T) {
	m := NewManager()
	m.Add(New("s"))

	err := m.Stop("s")
	if err == nil {
		t.Fatal("Stop on stopped server: want error, got nil")
	}
	if !errors.Is(err, ErrNotRunning) {
		t.Fatalf("Stop error = %v, want errors.Is(_, ErrNotRunning)", err)
	}
}

// TestInvalidTransitionSentinel verifies Server.transition classifies a rejected
// transition with the typed sentinel.
func TestInvalidTransitionSentinel(t *testing.T) {
	s := New("s") // Stopped

	err := s.transition(Stopped)
	if err == nil {
		t.Fatal("invalid transition: want error, got nil")
	}
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("transition error = %v, want errors.Is(_, ErrInvalidTransition)", err)
	}
}
