package tui

import (
	"testing"

	"github.com/voxslopuli/mcpeach/e2e/harness"
)

// TestCleanupStopKillsChild verifies stopping a server kills its child process.
func TestCleanupStopKillsChild(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	d, s := fx.Setup(t, binPath, harness.FakeServerConfig(fake), "cleanup", 80, 24)

	s.ExpectServerRunning()
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
	fx.WriteConfig(harness.FakeServerConfig(fake))
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	harness.AssertProcessAlive(t, fake)

	// Stop the daemon; its children should be cleaned up.
	d.Stop()
	harness.AssertProcessGone(t, fake)
}
