package tui

import (
	"testing"

	"github.com/voxslopuli/mcpeach/e2e/harness"
)

// TestEditServer verifies the 'e' edit flow on the management screen updates a
// server's configuration.
func TestEditServer(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	d, s := fx.Setup(t, binPath, harness.FakeServerConfig(fake), "edit", 100, 30)

	// Open the management screen and press 'e' to edit.
	s.ExpectServerRunning()
	s.OpenManagement()
	s.Key("e")
	s.Expect("Server name")

	// The form is prefilled with the server name. Change the command.
	s.Key("enter") // move to command (name is prefilled)
	s.WaitIdle()
	// Clear the command and type a new one.
	s.Key("ctrl+a")
	s.Type("/bin/echo")
	s.Key("enter")
	s.WaitIdle()
	// Args — leave empty.
	s.Key("enter")
	s.WaitIdle()
	// Transport — open select, select stdio, move to URL.
	s.Key("enter")
	s.WaitIdle()
	s.Key("enter")
	s.WaitIdle()
	s.Key("enter")
	s.WaitIdle()
	// URL — leave empty, submit.
	s.Key("enter")
	s.WaitIdle()
	s.Key("enter")
	s.WaitIdle()

	// The server is updated (command changed).
	harness.AssertAPI(t, d, "GET", "/v0/servers/fake", "", 200, "/bin/echo")
}
