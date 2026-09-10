// Package obs provides app-wide structured logging with a bounded in-memory
// ring buffer, so recent log lines are queryable by the control plane and TUI.
package obs

import (
	charmlog "github.com/charmbracelet/log"
	"github.com/mcpeach/mcpeach/internal/logs"
)

// ringCapacity is the number of log lines retained in memory.
const ringCapacity = 1000

// Logger wraps charmbracelet/log with a ring-buffer sink.
type Logger struct {
	*charmlog.Logger
	ring *logs.Ring
}

// NewLogger builds a Logger that writes JSON into a bounded ring buffer.
func NewLogger(component string) *Logger {
	ring := logs.NewRing(ringCapacity)
	l := charmlog.NewWithOptions(ringWriter{ring: ring}, charmlog.Options{
		Level:           charmlog.InfoLevel,
		ReportTimestamp: true,
	})
	l.SetFormatter(charmlog.JSONFormatter)
	return &Logger{Logger: l.With("component", component), ring: ring}
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
