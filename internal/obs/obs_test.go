package obs

import (
	"strings"
	"testing"
)

func TestNewLogger(t *testing.T) {
	l := NewLogger("test")
	if l == nil {
		t.Fatal("NewLogger returned nil")
	}
	if l.Logger == nil {
		t.Fatal("Logger is nil")
	}
}

func TestLoggerCaptures(t *testing.T) {
	l := NewLogger("test")
	l.Info("hello", "key", "value")
	l.Warn("warning", "k", "v")
	l.Error("oops", "err", "boom")

	lines := l.Lines()
	if len(lines) != 3 {
		t.Fatalf("Lines = %d, want 3", len(lines))
	}
	// Each line should be JSON (slog JSON handler) containing the message.
	if !strings.Contains(lines[0], "hello") {
		t.Errorf("line[0] = %q, want contains hello", lines[0])
	}
	if !strings.Contains(lines[1], "warning") {
		t.Errorf("line[1] = %q, want contains warning", lines[1])
	}
	if !strings.Contains(lines[2], "oops") {
		t.Errorf("line[2] = %q, want contains oops", lines[2])
	}
}

func TestLoggerRingBounded(t *testing.T) {
	l := NewLogger("test")
	// Write more than the ring capacity.
	for i := 0; i < 2000; i++ {
		l.Info("line", "i", i)
	}
	lines := l.Lines()
	if len(lines) > 1000 {
		t.Errorf("Lines = %d, want <= 1000 (ring bounded)", len(lines))
	}
}

func TestLoggerWith(t *testing.T) {
	l := NewLogger("test")
	child := l.With("server", "github")
	child.Info("started")
	lines := l.Lines()
	if len(lines) != 1 {
		t.Fatalf("Lines = %d, want 1", len(lines))
	}
	if !strings.Contains(lines[0], "github") {
		t.Errorf("line = %q, want contains github", lines[0])
	}
}

func TestDefaultLogger(t *testing.T) {
	l := Default()
	if l == nil {
		t.Fatal("Default returned nil")
	}
}
