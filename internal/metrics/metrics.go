// Package metrics provides lightweight, in-process observability for the
// gateway: per-server tool-call counters and latency. It is intentionally
// dependency-free (no Prometheus/OTel) — a small, testable surface that the
// control plane and TUI can query.
package metrics

import (
	"sync"
	"time"
)

// ServerStats aggregates tool-call metrics for one server.
type ServerStats struct {
	Total        int           `json:"total"`
	Success      int           `json:"success"`
	Failure      int           `json:"failure"`
	TotalLatency time.Duration `json:"total_latency_ns"`
}

// Snapshot is a point-in-time view of all server metrics.
type Snapshot struct {
	Servers map[string]ServerStats `json:"servers"`
}

// Metrics records per-server tool-call counters and latency. Safe for
// concurrent use.
type Metrics struct {
	mu      sync.RWMutex
	servers map[string]ServerStats
}

// New returns an empty Metrics.
func New() *Metrics {
	return &Metrics{servers: map[string]ServerStats{}}
}

// RecordToolCall records a tool call for a server, its latency, and whether it
// succeeded (err == nil).
func (m *Metrics) RecordToolCall(server, tool string, latency time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.servers[server]
	s.Total++
	s.TotalLatency += latency
	if err != nil {
		s.Failure++
	} else {
		s.Success++
	}
	m.servers[server] = s
}

// Snapshot returns a copy of the current metrics.
func (m *Metrics) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := Snapshot{Servers: make(map[string]ServerStats, len(m.servers))}
	for k, v := range m.servers {
		out.Servers[k] = v
	}
	return out
}
