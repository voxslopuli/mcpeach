package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestSecurityNoSecretLeak verifies that a resolved secret value (from an
// env: reference) never appears in the TUI output or the control-plane API.
func TestSecurityNoSecretLeak(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	// Configure a server with an env: reference whose value is set in the
	// daemon's environment.
	fx.WriteConfig(map[string]map[string]any{
		"fake": {
			"command": fake,
			"enabled": true,
			"env":     map[string]string{"GITHUB_TOKEN": "env:MC_TEST_SECRET"},
		},
	})
	secret := "supersecretvalue123"
	d := harness.StartDaemon(t, binPath, fx.Root, append(fx.Env(), "MC_TEST_SECRET="+secret))
	defer d.Stop()

	s := harness.NewSession(t, "security", binPath, nil, fx.Env(), 100, 30)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	// The TUI shows the server; the resolved secret value must never appear.
	s.ExpectServerRunning()
	s.OpenManagement()
	harness.AssertNoSecret(t, s.Text(), secret)

	// The API returns the env reference, never the resolved value.
	_, body := d.API("GET", "/v0/servers/fake", "")
	harness.AssertNoSecret(t, body, secret)
	if !harness.Contains(body, "env:MC_TEST_SECRET") {
		t.Errorf("API should expose the env reference, got: %s", body)
	}
	// The secrets listing returns the reference, never the value.
	_, body2 := d.API("GET", "/v0/secrets", "")
	harness.AssertNoSecret(t, body2, secret)
}
