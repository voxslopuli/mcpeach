package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/voxslopuli/mcpeach/e2e/harness"
)

// TestImportExport verifies the daemon-owned import/export workflow with an
// in-memory keychain store.
func TestImportExport(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	// Start with no servers.
	fx.WriteConfig(map[string]map[string]any{})
	env := append(fx.Env(), "MCPEACH_KEYCHAIN_STORE=memory")
	d := harness.StartDaemon(t, binPath, fx.Root, env)
	defer d.Stop()

	// Write a Claude Code MCP config to import.
	src := filepath.Join(fx.Root, "claude.json")
	doc := `{"mcpServers":{"new":{"command":"` + fake + `","env":{"GITHUB_TOKEN":"plainsecret"}}}}`
	if err := os.WriteFile(src, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}

	// Import with keychain migration (default).
	harness.AssertAPI(t, d, "POST", "/v0/import",
		`{"path":"`+src+`","conflicts_policy":"replace","secrets_policy":"keychain"}`, 200, "new")

	// The server is imported and its plaintext secret migrated to a keychain ref.
	_, body := d.API("GET", "/v0/servers/new", "")
	if !harness.Contains(body, "keychain:mcpeach/new/GITHUB_TOKEN") {
		t.Errorf("imported server missing keychain ref: %s", body)
	}
	harness.AssertNoSecret(t, body, "plainsecret")

	// Export with references preserved.
	dst := filepath.Join(fx.Root, "out.json")
	harness.AssertAPI(t, d, "POST", "/v0/export",
		`{"path":"`+dst+`","secrets":"references"}`, 200)
	out, err := os.ReadFile(dst) // nosemgrep go_filesystem_rule-fileread
	if err != nil {
		t.Fatal(err)
	}
	harness.AssertNoSecret(t, string(out), "plainsecret")
	if !harness.Contains(string(out), "keychain:mcpeach/new/GITHUB_TOKEN") {
		t.Errorf("export missing keychain ref: %s", out)
	}
}

// TestImportPlaintextRequiresFlag verifies plaintext export is gated.
func TestImportPlaintextRequiresFlag(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(map[string]map[string]any{
		"fake": {"command": fake, "enabled": false, "env": map[string]string{"GITHUB_TOKEN": "keychain:mcpeach/fake/GITHUB_TOKEN"}},
	})
	env := append(fx.Env(), "MCPEACH_KEYCHAIN_STORE=memory")
	d := harness.StartDaemon(t, binPath, fx.Root, env)
	defer d.Stop()

	// Store a secret.
	harness.AssertAPI(t, d, "POST", "/v0/secrets/fake/GITHUB_TOKEN", `{"value":"supersecret"}`, 200)

	// Plaintext export without the flag is refused.
	dst := filepath.Join(fx.Root, "out.json")
	harness.AssertAPI(t, d, "POST", "/v0/export",
		`{"path":"`+dst+`","secrets":"plaintext"}`, 400)

	// With the flag, the value is written.
	harness.AssertAPI(t, d, "POST", "/v0/export",
		`{"path":"`+dst+`","secrets":"plaintext","allow_plaintext":true}`, 200)
	out, err := os.ReadFile(dst) // nosemgrep go_filesystem_rule-fileread
	if err != nil {
		t.Fatal(err)
	}
	if !harness.Contains(string(out), "supersecret") {
		t.Errorf("plaintext export missing value: %s", out)
	}
}
