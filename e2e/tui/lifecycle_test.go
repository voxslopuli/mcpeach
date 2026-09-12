package tui

import (
	"testing"

	"github.com/voxslopuli/mcpeach/e2e/harness"
)

// TestLifecycleStartStop verifies the state-aware Space toggle starts and
// stops a server, and that the child process and tools follow.
func TestLifecycleStartStop(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	// enabled: true so the daemon auto-starts the server and registers its
	// tools (a disabled server's tools are not exposed even when manually
	// started).
	d, s := fx.Setup(t, binPath, harness.FakeServerConfig(fake), "lifecycle", 80, 24)

	// The server auto-starts running and its tool is registered.
	s.ExpectServerRunning()
	harness.AssertToolRegistered(t, d)

	// Space stops it (state-aware toggle).
	s.Key("space")
	s.Expect("stopped")
	harness.AssertProcessGone(t, fake)
	harness.AssertNoTools(t, d)

	// Space starts it again.
	s.Key("space")
	s.Expect("running")
	harness.AssertProcessAlive(t, fake)
	harness.AssertToolRegistered(t, d)
}

// TestLifecycleEnterDoesNotStart verifies Enter opens the management screen
// without starting or stopping the server.
func TestLifecycleEnterDoesNotStart(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	// enabled: false so the server starts stopped; Enter must not start it.
	_, s := fx.Setup(t, binPath, map[string]map[string]any{
		"fake": {"command": fake, "enabled": false},
	}, "enter", 80, 24)

	s.Expect("fake")
	s.Expect("stopped")

	// Enter opens the management screen; state stays stopped.
	s.Key("enter")
	s.Expect("Server: fake")
	s.Expect("stopped")
	harness.AssertProcessGone(t, fake)
}
