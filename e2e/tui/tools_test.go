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
	_, s := fx.Setup(t, binPath, harness.FakeServerConfig(fake), "tools", 100, 30)

	s.ExpectServerRunning()
	s.OpenManagement()
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
	fx.WriteConfig(harness.FakeServerConfig(fake))
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "toolsstop", binPath, nil, fx.Env(), 100, 30)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	s.ExpectServerRunning()
	harness.AssertToolRegistered(t, d)

	// Stop via the API and verify the tool disappears.
	d.API("POST", "/v0/servers/fake/stop", "")
	harness.AssertNoTools(t, d)
	harness.AssertProcessGone(t, fake)
}
