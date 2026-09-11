package control

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/mcpconfig"
)

// writeClaudeConfig writes a Claude Code MCP config file with the given servers.
func writeClaudeConfig(t *testing.T, servers map[string]map[string]any) string {
	t.Helper()
	doc := map[string]any{"mcpServers": servers}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "claude.json")
	if err := os.WriteFile(p, b, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestImportServer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h, store := newSecretsHandler(t)
	src := writeClaudeConfig(t, map[string]map[string]any{
		"new":    {"command": "npx", "args": []string{"-y", "foo"}},
		"github": {"command": "npx", "args": []string{"-y", "github"}},
	})
	// 'github' already exists in the handler config.
	cfg := h.cfg.Load()
	cfg.Servers["github"] = config.ServerConfig{Command: "old"}
	h.cfg.Store(cfg)

	body, _ := json.Marshal(map[string]string{"path": src, "conflicts_policy": "replace", "secrets_policy": "keep"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/import", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var res map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &res)
	if !strings.Contains(rec.Body.String(), `"new"`) {
		t.Errorf("imported list missing 'new': %s", rec.Body.String())
	}
	// The existing github server was replaced (conflicts_policy=replace).
	cfg = h.cfg.Load()
	if cfg.Servers["github"].Command != "npx" {
		t.Errorf("github not replaced: %+v", cfg.Servers["github"])
	}
	if cfg.Servers["new"].Command != "npx" {
		t.Errorf("new server not imported: %+v", cfg.Servers["new"])
	}
	_ = store
}

func TestImportKeepConflicts(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h, _ := newSecretsHandler(t)
	src := writeClaudeConfig(t, map[string]map[string]any{
		"github": {"command": "npx", "args": []string{"-y", "github"}},
	})
	cfg := h.cfg.Load()
	cfg.Servers["github"] = config.ServerConfig{Command: "old"}
	h.cfg.Store(cfg)

	body, _ := json.Marshal(map[string]string{"path": src, "conflicts_policy": "keep"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/import", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	// With keep, the existing github server is preserved.
	cfg = h.cfg.Load()
	if cfg.Servers["github"].Command != "old" {
		t.Errorf("github overwritten despite keep policy: %+v", cfg.Servers["github"])
	}
}

func TestImportMigratesPlaintextSecret(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h, store := newSecretsHandler(t)
	src := writeClaudeConfig(t, map[string]map[string]any{
		"new": {"command": "npx", "env": map[string]string{"GITHUB_TOKEN": "plainsecret"}},
	})

	body, _ := json.Marshal(map[string]string{"path": src, "secrets_policy": "keychain"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/import", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	// The plaintext secret was migrated to a keychain ref.
	cfg := h.cfg.Load()
	got := cfg.Servers["new"].Env["GITHUB_TOKEN"]
	if !strings.HasPrefix(got, "keychain:") {
		t.Errorf("env not migrated: %q", got)
	}
	// The value is in the keychain store.
	if store.values["mcpeach/new/GITHUB_TOKEN"] != "plainsecret" {
		t.Errorf("keychain store missing secret: %v", store.values)
	}
}

func TestExportServer(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h, _ := newSecretsHandler(t)
	cfg := h.cfg.Load()
	cfg.Servers["github"].Env["TOKEN"] = "keychain:mcpeach/github/TOKEN"
	h.cfg.Store(cfg)

	dst := filepath.Join(t.TempDir(), "out.json")
	body, _ := json.Marshal(map[string]string{"path": dst, "secrets": "references"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/export", bytes.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	b, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	// The keychain reference is preserved; the value is never resolved.
	if !strings.Contains(string(b), "keychain:mcpeach/github/TOKEN") {
		t.Errorf("export missing keychain ref: %s", b)
	}
	if strings.Contains(string(b), "resolved-secret") {
		t.Error("export leaked a resolved value")
	}
}

func TestExportPlaintextRequiresFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	h, store := newSecretsHandler(t)
	store.values["mcpeach/github/TOKEN"] = "supersecret"
	cfg := h.cfg.Load()
	sc := cfg.Servers["github"]
	sc.Env = map[string]string{"TOKEN": "keychain:mcpeach/github/TOKEN"}
	cfg.Servers["github"] = sc
	h.cfg.Store(cfg)

	dst := filepath.Join(t.TempDir(), "out.json")
	// Without allow_plaintext, plaintext export is refused.
	body, _ := json.Marshal(map[string]string{"path": dst, "secrets": "plaintext"})
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v0/export", bytes.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 without allow_plaintext", rec.Code)
	}
	// With allow_plaintext, the value is written.
	body2, _ := json.Marshal(map[string]any{"path": dst, "secrets": "plaintext", "allow_plaintext": true})
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, httptest.NewRequest(http.MethodPost, "/v0/export", bytes.NewReader(body2)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec2.Code, rec2.Body.String())
	}
	b, _ := os.ReadFile(dst)
	if !strings.Contains(string(b), "supersecret") {
		t.Errorf("plaintext export missing resolved value: %s", b)
	}
}

func TestPreview(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	cfg := config.Default()
	cfg.Servers["github"] = config.ServerConfig{Command: "old"}
	src := writeClaudeConfig(t, map[string]map[string]any{
		"github": {"command": "npx"},
		"new":    {"command": "npx", "env": map[string]string{"API_KEY": "abc"}},
	})
	imports, conflicts, secrets, err := mcpconfig.Preview(src, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(imports) != 2 {
		t.Errorf("imports = %v", imports)
	}
	if len(conflicts) != 1 || conflicts[0] != "github" {
		t.Errorf("conflicts = %v", conflicts)
	}
	if len(secrets) != 1 || secrets[0].Server != "new" || secrets[0].Name != "API_KEY" {
		t.Errorf("secrets = %+v", secrets)
	}
}
