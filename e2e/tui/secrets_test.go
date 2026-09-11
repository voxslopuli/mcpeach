package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestSecretsStoreAndList verifies the secrets workflow with an in-memory
// keychain store (injected via MCPEACH_KEYCHAIN_STORE=memory, so no test
// touches the real OS keychain).
func TestSecretsStoreAndList(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	// Configure a server with a keychain reference. enabled: false so the
	// daemon does not try to resolve the (not-yet-stored) secret at startup.
	fx.WriteConfig(map[string]map[string]any{
		"fake": {
			"command": fake,
			"enabled": false,
			"env":     map[string]string{"GITHUB_TOKEN": "keychain:mcpeach/fake/GITHUB_TOKEN"},
		},
	})
	// Start the daemon with the in-memory keychain store.
	env := append(fx.Env(), "MCPEACH_KEYCHAIN_STORE=memory")
	d := harness.StartDaemon(t, binPath, fx.Root, env)
	defer d.Stop()

	// Store a secret via the API.
	secret := "supersecretvalue123"
	harness.AssertAPI(t, d, "POST", "/v0/secrets/fake/GITHUB_TOKEN", `{"value":"`+secret+`"}`, 200)

	// The secrets listing shows the reference, never the value.
	_, body := d.API("GET", "/v0/secrets", "")
	harness.AssertNoSecret(t, body, secret)
	if !harness.Contains(body, "keychain:mcpeach/fake/GITHUB_TOKEN") {
		t.Errorf("secrets API missing reference: %s", body)
	}

	// The server detail shows the reference, never the value.
	_, body2 := d.API("GET", "/v0/servers/fake", "")
	harness.AssertNoSecret(t, body2, secret)

	// Delete the secret.
	harness.AssertAPI(t, d, "DELETE", "/v0/secrets/fake/GITHUB_TOKEN", "", 200)
}

// TestSecretsTUIView verifies the TUI secrets screen shows source references
// without leaking values.
func TestSecretsTUIView(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(map[string]map[string]any{
		"fake": {
			"command": fake,
			"enabled": false,
			"env":     map[string]string{"GITHUB_TOKEN": "keychain:mcpeach/fake/GITHUB_TOKEN"},
		},
	})
	env := append(fx.Env(), "MCPEACH_KEYCHAIN_STORE=memory")
	d := harness.StartDaemon(t, binPath, fx.Root, env)
	defer d.Stop()

	s := harness.NewSession(t, "secrets", binPath, nil, fx.Env(), 100, 30)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	// 's' opens the secrets screen.
	s.Expect("fake")
	s.Expect("stopped")
	s.Key("s")
	s.Expect("Secrets")
	s.Expect("GITHUB_TOKEN")
	// The source kind is shown (keychain), never a value.
	s.Expect("keychain")
	harness.AssertNoSecret(t, s.Text(), "supersecretvalue123")
}
