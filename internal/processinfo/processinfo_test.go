package processinfo

import (
	"context"
	"os"
	"testing"
)

func TestCollectSelf(t *testing.T) {
	// Collect stats for the current test process — it must exist and have a PID.
	info, err := Collect(context.Background(), int32(os.Getpid()))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if info.PID != int32(os.Getpid()) {
		t.Errorf("PID = %d, want %d", info.PID, os.Getpid())
	}
	// RSS should be non-zero for a running process.
	if info.RSS == 0 {
		t.Error("RSS = 0, want non-zero")
	}
}

func TestCollectGone(t *testing.T) {
	// A non-existent PID should return a zero Info with no error.
	info, err := Collect(context.Background(), 99999999)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if info.PID != 0 {
		t.Errorf("PID = %d, want 0 (gone)", info.PID)
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		b    uint64
		want string
	}{
		{0, "0 B"},
		{500, "500 B"},
		{1024, "1.0 KiB"},
		{5 * 1024 * 1024, "5.0 MiB"},
		{3 * 1024 * 1024 * 1024, "3.0 GiB"},
	}
	for _, tt := range tests {
		if got := FormatBytes(tt.b); got != tt.want {
			t.Errorf("FormatBytes(%d) = %q, want %q", tt.b, got, tt.want)
		}
	}
}

func TestFormatPercent(t *testing.T) {
	tests := []struct {
		p    float64
		want string
	}{
		{0, "0.0%"},
		{12.5, "12.5%"},
		{99.99, "100.0%"},
		{3.14159, "3.1%"},
	}
	for _, tt := range tests {
		if got := FormatPercent(tt.p); got != tt.want {
			t.Errorf("FormatPercent(%v) = %q, want %q", tt.p, got, tt.want)
		}
	}
}

func TestInfoZero(t *testing.T) {
	// A zero Info should render without panicking.
	i := Info{}
	if i.CPUPercent != 0 || i.RSS != 0 || len(i.Ports) != 0 {
		t.Errorf("zero Info = %+v", i)
	}
}
