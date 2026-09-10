// Package processinfo provides lightweight process observability (CPU, memory,
// listening ports) for the TUI's process viewer. It wraps gopsutil.
package processinfo

import (
	"context"
	"fmt"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

// Info is a point-in-time snapshot of a process's resource usage.
type Info struct {
	PID        int32    `json:"pid"`
	CPUPercent float64  `json:"cpu_percent"`
	RSS        uint64   `json:"rss_bytes"`
	Ports      []string `json:"ports"`
}

// Collect gathers CPU%, RSS, and listening ports for the given PID. It returns
// a zero Info (no error) if the process no longer exists.
func Collect(ctx context.Context, pid int32) (Info, error) {
	p, err := process.NewProcess(pid)
	if err != nil {
		return Info{}, nil // process gone
	}

	info := Info{PID: pid}

	// Sample CPU over a short interval so the reading is accurate (a fresh
	// Process with interval 0 always returns 0).
	if cpu, err := p.PercentWithContext(ctx, 100*time.Millisecond); err == nil {
		info.CPUPercent = cpu
	}
	if mem, err := p.MemoryInfoWithContext(ctx); err == nil {
		info.RSS = mem.RSS
	}
	if conns, err := p.ConnectionsWithContext(ctx); err == nil {
		for _, c := range conns {
			if c.Laddr.Port > 0 && c.Status == "LISTEN" {
				info.Ports = append(info.Ports, fmt.Sprintf("%s:%d", c.Laddr.IP, c.Laddr.Port))
			}
		}
	}
	return info, nil
}

// FormatBytes renders a byte count in human-readable units.
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(b)/float64(div), "KMGTPE"[exp])
}

// FormatPercent renders a percentage to one decimal place. It does not cap at
// 100% because a process can exceed 100% CPU on multi-core systems.
func FormatPercent(p float64) string {
	return fmt.Sprintf("%.1f%%", p)
}
