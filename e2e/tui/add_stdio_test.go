package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestAddStdioServer verifies the 'n' new-server form adds a stdio server.
func TestAddStdioServer(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	// Start with no servers.
	fx.WriteConfig(map[string]map[string]any{})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "addstdio", binPath, nil, fx.Env(), 100, 30)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	// The empty list renders.
	s.Expect("mcpeach")
	// 'n' opens the new-server form (integrated into the TUI).
	s.Key("n")
	s.Expect("Server name")
	// Fill in the form and submit.
	s.FillAddServerForm("newserver", fake)
	// The new server appears in the list.
	s.Expect("newserver")
	// Verify via the API.
	harness.AssertAPI(t, d, "GET", "/v0/servers", "", 200, "newserver")
}
