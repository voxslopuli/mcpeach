package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// fakeMCPPath returns the path to the fake-mcp test binary.
func fakeMCPPath(t *testing.T) string {
	t.Helper()
	return harness.BuildFakeMCP(t)
}

// TestLifecycleStartStop verifies the state-aware Space toggle starts and
// stops a server, and that the child process and tools follow.
func TestLifecycleStartStop(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	// enabled: true so the daemon auto-starts the server and registers its
	// tools (a disabled server's tools are not exposed even when manually
	// started).
	fx.WriteConfig(map[string]map[string]any{
		"fake": {
			"command": fake,
			"enabled": true,
		},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "lifecycle", binPath, nil, fx.Env(), 80, 24)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	// The server auto-starts running and its tool is registered.
	s.Expect("fake")
	s.Expect("running")
	harness.AssertAPI(t, d, "GET", "/v0/tools", "", 200, "fake__echo")

	// Space stops it (state-aware toggle).
	s.Key("space")
	s.Expect("stopped")
	harness.AssertProcessGone(t, fake)
	harness.AssertAPI(t, d, "GET", "/v0/tools", "", 200)

	// Space starts it again.
	s.Key("space")
	s.Expect("running")
	harness.AssertProcessAlive(t, fake)
	harness.AssertAPI(t, d, "GET", "/v0/tools", "", 200, "fake__echo")
}

// TestLifecycleEnterDoesNotStart verifies Enter opens the management screen
// without starting or stopping the server.
func TestLifecycleEnterDoesNotStart(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	// enabled: false so the server starts stopped; Enter must not start it.
	fx.WriteConfig(map[string]map[string]any{
		"fake": {"command": fake, "enabled": false},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "enter", binPath, nil, fx.Env(), 80, 24)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	s.Expect("fake")
	s.Expect("stopped")

	// Enter opens the management screen; state stays stopped.
	s.Key("enter")
	s.Expect("Server: fake")
	s.Expect("stopped")
	harness.AssertProcessGone(t, fake)
}
