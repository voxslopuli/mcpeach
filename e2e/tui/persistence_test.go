package tui

import (
	"testing"

	"github.com/voxslopuli/mcpeach/e2e/harness"
)

// TestPersistenceSurvivesRestart verifies a configured server survives a daemon
// restart (config is persisted to disk).
func TestPersistenceSurvivesRestart(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)

	// First daemon run.
	d1, _ := fx.Setup(t, binPath, harness.FakeServerConfig(fake), "persist1", 80, 24)
	harness.AssertAPI(t, d1, "GET", "/v0/servers", "", 200, "fake")
	d1.Stop()

	// Second daemon run against the same config.
	d2 := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d2.Stop()
	harness.AssertAPI(t, d2, "GET", "/v0/servers", "", 200, "fake")
	harness.AssertToolRegistered(t, d2)
}
