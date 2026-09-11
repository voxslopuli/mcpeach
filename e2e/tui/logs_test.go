package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestLogsView verifies the per-server log viewer shows captured log lines.
func TestLogsView(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	_, s := fx.Setup(t, binPath, map[string]map[string]any{
		"fake": {"command": fake, "enabled": true},
	}, "logs", 100, 30)

	s.Expect("fake")
	s.Expect("running")
	// 'l' toggles the log viewer for the selected server.
	s.Key("l")
	s.Expect("Logs for fake")
	// The fake-mcp emits a JSON log line on startup.
	s.Expect("fake-mcp started")
	// Esc returns to the list.
	s.Key("esc")
	s.Expect("fake")
}
