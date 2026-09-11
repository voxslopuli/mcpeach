package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestCleanupStopKillsChild verifies stopping a server kills its child process.
func TestCleanupStopKillsChild(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(map[string]map[string]any{
		"fake": {"command": fake, "enabled": true},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "cleanup", binPath, nil, fx.Env(), 80, 24)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	s.Expect("fake")
	s.Expect("running")
	harness.AssertProcessAlive(t, fake)

	// Stop via the API and verify the child exits.
	d.API("POST", "/v0/servers/fake/stop", "")
	harness.AssertProcessGone(t, fake)
}

// TestCleanupDaemonShutdown verifies stopping the daemon cleans up its child
// processes.
func TestCleanupDaemonShutdown(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(map[string]map[string]any{
		"fake": {"command": fake, "enabled": true},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	harness.AssertProcessAlive(t, fake)

	// Stop the daemon; its children should be cleaned up.
	d.Stop()
	harness.AssertProcessGone(t, fake)
}
