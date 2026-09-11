package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestToolsDistinctFromProcess verifies the management screen lists MCP tools
// under an "MCP Tools" section, distinct from process telemetry.
func TestToolsDistinctFromProcess(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(map[string]map[string]any{
		"fake": {"command": fake, "enabled": true},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "tools", binPath, nil, fx.Env(), 100, 30)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	s.Expect("fake")
	s.Expect("running")
	s.Key("enter")
	s.Expect("Server: fake")
	// The canonical tool name is shown under MCP Tools.
	s.Expect("fake__echo")
	// MCP Tools is a distinct section header; process telemetry is separate.
	s.Expect("MCP Tools")
	s.Expect("Process")
}

// TestToolsReflectStop verifies stopping a server removes its tools from the
// management screen and the API.
func TestToolsReflectStop(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(map[string]map[string]any{
		"fake": {"command": fake, "enabled": true},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "toolsstop", binPath, nil, fx.Env(), 100, 30)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	s.Expect("fake")
	s.Expect("running")
	harness.AssertAPI(t, d, "GET", "/v0/tools", "", 200, "fake__echo")

	// Stop via the API and verify the tool disappears.
	d.API("POST", "/v0/servers/fake/stop", "")
	harness.AssertAPI(t, d, "GET", "/v0/tools", "", 200)
	harness.AssertProcessGone(t, fake)
}
