// Package obs provides app-wide structured logging with a bounded in-memory
// ring buffer, so recent log lines are queryable by the control plane and TUI.
package obs

import (
	"log/slog"

	"github.com/mcpeach/mcpeach/internal/logs"
)

// ringCapacity is the number of log lines retained in memory.
const ringCapacity = 1000

// Logger wraps slog.Logger with a ring-buffer sink.
type Logger struct {
	*slog.Logger
	ring *logs.Ring
}

// NewLogger builds a Logger that writes JSON into a bounded ring buffer.
func NewLogger(component string) *Logger {
	ring := logs.NewRing(ringCapacity)
	h := slog.NewJSONHandler(ringWriter{ring: ring}, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &Logger{Logger: slog.New(h).With("component", component), ring: ring}
}

// Default returns a Logger for the "mcpeach" component.
func Default() *Logger {
	return NewLogger("mcpeach")
}

// With returns a child logger with the given key-value pairs attached.
func (l *Logger) With(args ...any) *Logger {
	return &Logger{Logger: l.Logger.With(args...), ring: l.ring}
}

// Lines returns the retained log lines in insertion order.
func (l *Logger) Lines() []string {
	return l.ring.Lines()
}

// ringWriter adapts a *logs.Ring to io.Writer.
type ringWriter struct{ ring *logs.Ring }

func (w ringWriter) Write(p []byte) (int, error) {
	w.ring.Write(string(p))
	return len(p), nil
}
