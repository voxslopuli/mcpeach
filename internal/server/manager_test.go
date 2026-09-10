package server

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/mcpeach/mcpeach/internal/testutil"
)

// buildFakeServer compiles the testdata fake MCP server binary.
func buildFakeServer(t *testing.T) string {
	t.Helper()
	return testutil.BuildFakeServer(t)
}

func TestManagerStartStop(t *testing.T) {
	bin := buildFakeServer(t)
	m := NewManager()
	srv := New("fake")
	m.Add(srv)

	if err := m.Start(context.Background(), "fake", bin, nil, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if srv.State() != Running {
		t.Fatalf("state = %s, want running", srv.State())
	}

	if err := m.Stop("fake"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if srv.State() != Stopped {
		t.Fatalf("state = %s, want stopped", srv.State())
	}
}

func TestManagerStartUnknown(t *testing.T) {
	m := NewManager()
	if err := m.Start(context.Background(), "nope", "echo", nil, nil); err == nil {
		t.Fatal("Start unknown server: want error, got nil")
	}
}

func TestManagerStopNotRunning(t *testing.T) {
	m := NewManager()
	srv := New("fake")
	m.Add(srv)
	if err := m.Stop("fake"); err == nil {
		t.Fatal("Stop not-running server: want error, got nil")
	}
}

func TestManagerRestart(t *testing.T) {
	bin := buildFakeServer(t)
	m := NewManager()
	srv := New("fake")
	m.Add(srv)

	if err := m.Start(context.Background(), "fake", bin, nil, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop("fake"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := m.Start(context.Background(), "fake", bin, nil, nil); err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if srv.State() != Running {
		t.Fatalf("state = %s, want running after restart", srv.State())
	}
}

func TestManagerCapturesOutput(t *testing.T) {
	bin := buildFakeServer(t)
	m := NewManager()
	srv := New("fake")
	m.Add(srv)

	if err := m.Start(context.Background(), "fake", bin, nil, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop("fake") }()

	// Give the fake server time to emit a log line.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.Logs("fake")) > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if len(m.Logs("fake")) == 0 {
		t.Fatal("expected captured log output, got none")
	}
}

func TestManagerCaptureLogs(t *testing.T) {
	m := NewManager()
	m.Add(New("fake"))

	// Feed a line into the ring via CaptureLogs and verify it's retrievable.
	m.CaptureLogs("fake", strings.NewReader("hello\n"))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if len(m.Logs("fake")) > 0 {
			if m.Logs("fake")[0] != "hello" {
				t.Fatalf("log = %q, want hello", m.Logs("fake")[0])
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("CaptureLogs did not feed the ring")
}

func TestManagerCaptureLogsUnknown(t *testing.T) {
	// Capturing logs for an unknown server should be a no-op (no panic).
	m := NewManager()
	m.CaptureLogs("nope", strings.NewReader("x\n"))
}

func TestManagerServer(t *testing.T) {
	m := NewManager()
	m.Add(New("fake"))
	if s := m.Server("fake"); s == nil {
		t.Fatal("Server(fake) = nil, want non-nil")
	}
	if s := m.Server("nope"); s != nil {
		t.Fatalf("Server(nope) = %v, want nil", s)
	}
}

func TestManagerPID(t *testing.T) {
	bin := buildFakeServer(t)
	m := NewManager()
	m.Add(New("fake"))

	// Not running → PID 0.
	if pid := m.PID("fake"); pid != 0 {
		t.Fatalf("PID not running = %d, want 0", pid)
	}

	if err := m.Start(context.Background(), "fake", bin, nil, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop("fake") }()

	if pid := m.PID("fake"); pid == 0 {
		t.Fatal("PID running = 0, want non-zero")
	}
}

func TestManagerMarkRunningStopped(t *testing.T) {
	m := NewManager()
	m.Add(New("fake"))

	// MarkRunning transitions to running.
	if err := m.MarkRunning("fake"); err != nil {
		t.Fatalf("MarkRunning: %v", err)
	}
	if s := m.Server("fake"); s.State() != Running {
		t.Fatalf("state = %s, want running", s.State())
	}

	// MarkStopped transitions back to stopped.
	if err := m.MarkStopped("fake"); err != nil {
		t.Fatalf("MarkStopped: %v", err)
	}
	if s := m.Server("fake"); s.State() != Stopped {
		t.Fatalf("state = %s, want stopped", s.State())
	}
}

func TestManagerMarkUnknown(t *testing.T) {
	m := NewManager()
	if err := m.MarkRunning("nope"); err == nil {
		t.Fatal("MarkRunning unknown: want error, got nil")
	}
	if err := m.MarkStopped("nope"); err == nil {
		t.Fatal("MarkStopped unknown: want error, got nil")
	}
}

func TestManagerPassesArgs(t *testing.T) {
	bin := buildFakeServer(t)
	m := NewManager()
	srv := New("fake")
	m.Add(srv)

	// Start with args and verify they reach the subprocess via the log line.
	if err := m.Start(context.Background(), "fake", bin, []string{"--flag", "value"}, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop("fake") }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, line := range m.Logs("fake") {
			if strings.Contains(line, "--flag") && strings.Contains(line, "value") {
				return // args reached the subprocess
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("args not passed to subprocess; logs: %v", m.Logs("fake"))
}

func TestManagerStartInheritsEnv(t *testing.T) {
	bin := buildFakeServer(t)
	t.Setenv("MC_TEST_VAR", "inherited")
	m := NewManager()
	srv := New("fake")
	m.Add(srv)

	// Start with a configured env var; the child must see BOTH the inherited
	// MC_TEST_VAR and the configured MC_CONFIG_VAR.
	if err := m.Start(context.Background(), "fake", bin, nil, []string{"MC_CONFIG_VAR=configured"}); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = m.Stop("fake") }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, line := range m.Logs("fake") {
			if strings.Contains(line, `"MC_TEST_VAR":"inherited"`) && strings.Contains(line, `"MC_CONFIG_VAR":"configured"`) {
				return // child saw both inherited and configured env
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("child env missing inherited/configured vars; logs: %v", m.Logs("fake"))
}

func TestManagerLogsUnknown(t *testing.T) {
	m := NewManager()
	if logs := m.Logs("nope"); logs != nil {
		t.Fatalf("Logs unknown server = %v, want nil", logs)
	}
}
