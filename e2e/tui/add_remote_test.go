package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// buildStreamable builds the streamable fixture and returns its path.
func buildStreamable(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(os.TempDir(), "mcpeach-e2e-fixtures")
	_ = os.MkdirAll(dir, 0o755)
	bin := filepath.Join(dir, "streamable")
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	cmd := exec.Command("go", "build", "-o", bin, "github.com/mcpeach/mcpeach/e2e/fixtures/streamable")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build streamable: %v", err)
	}
	return bin
}

// TestRemoteServerLifecycle verifies a streamable-HTTP remote server can be
// configured, started, and its tools exposed.
func TestRemoteServerLifecycle(t *testing.T) {
	fx := harness.NewFixture(t)
	streamable := buildStreamable(t)
	// Start the streamable fixture and capture its address.
	cmd := exec.Command(streamable) // nosemgrep go_subproc_rule-subproc,go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd.Env = append(os.Environ(), "STREAMABLE_ADDR=127.0.0.1:0")
	stderr, err := os.CreateTemp("", "streamable-stderr")
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start streamable: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()

	// Wait for the address line.
	addr := harness.WaitForAddr(t, stderr.Name())
	if addr == "" {
		t.Fatal("streamable fixture did not report an address")
	}

	// Configure the remote server.
	fx.WriteConfig(map[string]map[string]any{
		"remote": {
			"url":       "http://" + addr + "/mcp",
			"transport": "streamable-http",
			"enabled":   true,
		},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	// The remote server's tool is exposed.
	harness.AssertAPI(t, d, "GET", "/v0/tools", "", 200, "remote__echo")
}
