package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestPersistenceSurvivesRestart verifies a configured server survives a daemon
// restart (config is persisted to disk).
func TestPersistenceSurvivesRestart(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(map[string]map[string]any{
		"fake": {"command": fake, "enabled": true},
	})

	// First daemon run.
	d1 := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	harness.AssertAPI(t, d1, "GET", "/v0/servers", "", 200, "fake")
	d1.Stop()

	// Second daemon run against the same config.
	d2 := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d2.Stop()
	harness.AssertAPI(t, d2, "GET", "/v0/servers", "", 200, "fake")
	harness.AssertAPI(t, d2, "GET", "/v0/tools", "", 200, "fake__echo")
}
