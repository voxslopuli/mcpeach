package server

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// buildFakeServer compiles the testdata fake MCP server binary.
func buildFakeServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-mcp")
	cmd := exec.Command("go", "build", "-o", bin, "../../testdata/fake-mcp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake server: %v\n%s", err, out)
	}
	return bin
}

func TestManagerStartStop(t *testing.T) {
	bin := buildFakeServer(t)
	m := NewManager()
	srv := New("fake")
	m.Add(srv)

	if err := m.Start(context.Background(), "fake", bin, nil); err != nil {
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
	if err := m.Start(context.Background(), "nope", "echo", nil); err == nil {
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

	if err := m.Start(context.Background(), "fake", bin, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := m.Stop("fake"); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := m.Start(context.Background(), "fake", bin, nil); err != nil {
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

	if err := m.Start(context.Background(), "fake", bin, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer m.Stop("fake")

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

func TestManagerLogsUnknown(t *testing.T) {
	m := NewManager()
	if logs := m.Logs("nope"); logs != nil {
		t.Fatalf("Logs unknown server = %v, want nil", logs)
	}
}
