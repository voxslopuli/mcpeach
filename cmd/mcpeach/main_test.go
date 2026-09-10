package main

import (
	"context"
	"testing"
	"time"

	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/service"
)

func TestStatusString(t *testing.T) {
	tests := []struct {
		st   service.Status
		want string
	}{
		{service.StatusRunning, "running"},
		{service.StatusStopped, "stopped"},
		{service.StatusUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := statusString(tt.st); got != tt.want {
			t.Errorf("statusString(%v) = %q, want %q", tt.st, got, tt.want)
		}
	}
}

func TestServeCmd(t *testing.T) {
	cmd := serveCmd()
	if cmd == nil {
		t.Fatal("serveCmd returned nil")
	}
	if cmd.Use != "serve" {
		t.Errorf("Use = %q, want serve", cmd.Use)
	}
}

func TestInstallCmd(t *testing.T) {
	cmd := installCmd()
	if cmd == nil {
		t.Fatal("installCmd returned nil")
	}
	if cmd.Use != "install" {
		t.Errorf("Use = %q, want install", cmd.Use)
	}
}

func TestUninstallCmd(t *testing.T) {
	cmd := uninstallCmd()
	if cmd == nil {
		t.Fatal("uninstallCmd returned nil")
	}
	if cmd.Use != "uninstall" {
		t.Errorf("Use = %q, want uninstall", cmd.Use)
	}
}

func TestStatusCmd(t *testing.T) {
	cmd := statusCmd()
	if cmd == nil {
		t.Fatal("statusCmd returned nil")
	}
	if cmd.Use != "status" {
		t.Errorf("Use = %q, want status", cmd.Use)
	}
}

func TestNewServiceManager(t *testing.T) {
	m, err := newServiceManager()
	if err != nil {
		t.Fatalf("newServiceManager: %v", err)
	}
	if m == nil {
		t.Fatal("newServiceManager returned nil")
	}
}

func TestRunDaemon(t *testing.T) {
	// Point XDG dirs at a temp dir so config + socket don't touch the real home.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)

	// Write a minimal config with no servers and a free gateway addr.
	cfg := config.Default()
	cfg.Gateway.Addr = "127.0.0.1:0"
	if err := config.Save(config.Path(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// runDaemon blocks until ctx is cancelled; run it in a goroutine and
	// cancel after a short delay to verify it starts cleanly.
	errCh := make(chan error, 1)
	go func() {
		errCh <- runDaemon(ctx)
	}()

	// Give it a moment to bind, then cancel.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runDaemon: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runDaemon did not return after cancel")
	}
}

func TestRunDaemonNoConfig(t *testing.T) {
	// No config file → runDaemon should return a load error.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)

	err := runDaemon(context.Background())
	if err == nil {
		t.Fatal("runDaemon with no config: want error, got nil")
	}
}
