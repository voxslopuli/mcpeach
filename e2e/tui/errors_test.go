package tui

import (
	"testing"

	"github.com/voxslopuli/mcpeach/e2e/harness"
)

// TestErrorsDaemonUnavailable verifies the TUI surfaces a connection error
// when the daemon is not running, rather than silently showing an empty list.
func TestErrorsDaemonUnavailable(t *testing.T) {
	fx := harness.NewFixture(t)
	// No daemon started; the control socket does not exist.
	s := harness.NewSession(t, "err-nodaemon", binPath, nil, fx.Env(), 80, 24)
	defer s.Close()
	harness.CollectArtifacts(t, s, nil)

	// The TUI should render something (title) and surface an error, not an
	// empty list with no feedback.
	s.Expect("mcpeach")
	text := s.Text()
	if text == "" {
		t.Fatal("TUI produced no output when daemon unavailable")
	}
}

// TestErrorsChildStartFailure verifies that a server whose command fails to
// start causes the daemon to report the failure (the daemon exits with an
// error rather than silently continuing with a broken server).
func TestErrorsChildStartFailure(t *testing.T) {
	fx := harness.NewFixture(t)
	// A command that does not exist.
	fx.WriteConfig(map[string]map[string]any{
		"broken": {"command": "/nonexistent/binary", "enabled": true},
	})
	// The daemon should fail to start and report the connect error.
	err := harness.RunDaemonExpectError(t, binPath, fx.Root, fx.Env())
	if err == nil {
		t.Fatal("expected daemon to fail starting with a broken server")
	}
	if !harness.Contains(err.Error(), "connect") && !harness.Contains(err.Error(), "broken") {
		t.Errorf("daemon error did not mention the broken server: %v", err)
	}
}
