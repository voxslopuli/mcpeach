package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestSecurityNoSecretLeak verifies that a configured secret value never
// appears in the TUI output or the control-plane API responses.
func TestSecurityNoSecretLeak(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	// Configure a server with a keychain reference (the value is stored in the
	// keychain, never in the config or API).
	fx.WriteConfig(map[string]map[string]any{
		"fake": {
			"command": fake,
			"enabled": true,
			"env":     map[string]string{"GITHUB_TOKEN": "keychain:mcpeach/fake/GITHUB_TOKEN"},
		},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "security", binPath, nil, fx.Env(), 100, 30)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	// The TUI shows the server; the secret value must never appear.
	s.Expect("fake")
	s.Expect("running")
	s.Key("enter")
	s.Expect("Server: fake")
	// The secret value must never appear in the TUI output.
	harness.AssertNoSecret(t, s.Text(), "supersecretvalue")

	// The API never returns the resolved value.
	_, body := d.API("GET", "/v0/servers/fake", "")
	harness.AssertNoSecret(t, body, "supersecretvalue")
	_, body2 := d.API("GET", "/v0/secrets", "")
	harness.AssertNoSecret(t, body2, "supersecretvalue")
}
