// Package logs provides a bounded ring buffer for capturing server output.
package logs

import "sync"

// Ring is a fixed-capacity, thread-safe ring buffer of strings.
type Ring struct {
	mu   sync.Mutex
	buf  []string
	next int
	full bool
}

// NewRing returns a Ring that retains at most capacity entries.
func NewRing(capacity int) *Ring {
	return &Ring{buf: make([]string, capacity)}
}

// Write appends a line, evicting the oldest when full.
func (r *Ring) Write(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.buf[r.next] = line
	r.next = (r.next + 1) % len(r.buf)
	if r.next == 0 {
		r.full = true
	}
}

// Lines returns the buffered lines in insertion order.
func (r *Ring) Lines() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return append([]string(nil), r.buf[:r.next]...)
	}
	out := make([]string, len(r.buf))
	copy(out, r.buf[r.next:])
	copy(out[len(r.buf)-r.next:], r.buf[:r.next])
	return out
}
