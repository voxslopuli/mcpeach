package tui

import (
	"os"
	"os/exec"
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestRemoteServerLifecycle verifies a streamable-HTTP remote server can be
// configured, started, and its tools exposed.
func TestRemoteServerLifecycle(t *testing.T) {
	fx := harness.NewFixture(t)
	streamable := harness.BuildStreamable(t)
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
