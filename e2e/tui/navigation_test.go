package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestNavigationEnterOpensManagement verifies Enter opens the management screen
// and shows the server's sections.
func TestNavigationEnterOpensManagement(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	_, s := fx.Setup(t, binPath, map[string]map[string]any{
		"fake": {"command": fake, "enabled": true},
	}, "nav", 100, 30)

	s.Expect("fake")
	s.Expect("running")
	s.Key("enter")
	s.Expect("Server: fake")
	s.Expect("running")
	// The management screen shows config + process + MCP tools sections.
	s.Expect("Command")
	s.Expect("MCP tools")
	s.Expect("Process")
	// Esc returns to the list.
	s.Key("esc")
	s.Expect("fake")
}

// TestNavigationEscBack verifies esc returns from a sub-view to the list.
func TestNavigationEscBack(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(map[string]map[string]any{
		"fake": {"command": fake, "enabled": true},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "navback", binPath, nil, fx.Env(), 100, 30)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	s.Expect("fake")
	s.Key("enter")
	s.Expect("Server: fake")
	s.Key("esc")
	s.Expect("fake")
}
