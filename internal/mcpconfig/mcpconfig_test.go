package mcpconfig

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mcpeach/mcpeach/internal/config"
)

func TestImport(t *testing.T) {
	// A Claude Code mcp config with a stdio and a remote server.
	raw := `{
		"mcpServers": {
			"github": {
				"command": "npx",
				"args": ["-y", "@modelcontextprotocol/server-github"],
				"env": {"GITHUB_PERSONAL_ACCESS_TOKEN": "secret"}
			},
			"context7": {
				"url": "https://mcp.context7.com/mcp",
				"type": "http"
			}
		}
	}`
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := config.Default()
	if err := Import(path, cfg); err != nil {
		t.Fatalf("Import: %v", err)
	}

	gh, ok := cfg.Servers["github"]
	if !ok {
		t.Fatal("github server not imported")
	}
	if gh.Command != "npx" || len(gh.Args) != 2 || gh.Args[0] != "-y" {
		t.Errorf("github = %+v, want npx -y", gh)
	}
	if gh.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "secret" {
		t.Errorf("github env = %v", gh.Env)
	}

	c7, ok := cfg.Servers["context7"]
	if !ok {
		t.Fatal("context7 server not imported")
	}
	if c7.URL != "https://mcp.context7.com/mcp" || c7.Transport != "streamable-http" {
		t.Errorf("context7 = %+v, want streamable-http", c7)
	}
}

func TestImportPreservesExisting(t *testing.T) {
	raw := `{"mcpServers": {"new": {"command": "echo"}}}`
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := config.Default()
	cfg.Servers["existing"] = config.ServerConfig{Command: "keep", Enabled: true}
	if err := Import(path, cfg); err != nil {
		t.Fatalf("Import: %v", err)
	}
	if _, ok := cfg.Servers["existing"]; !ok {
		t.Error("existing server was dropped")
	}
	if _, ok := cfg.Servers["new"]; !ok {
		t.Error("new server not imported")
	}
}

func TestImportMissingFile(t *testing.T) {
	cfg := config.Default()
	if err := Import(filepath.Join(t.TempDir(), "nope.json"), cfg); err == nil {
		t.Fatal("Import missing file: want error, got nil")
	}
}

func TestImportBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	cfg := config.Default()
	if err := Import(path, cfg); err == nil {
		t.Fatal("Import bad JSON: want error, got nil")
	}
}

func TestExport(t *testing.T) {
	cfg := config.Default()
	cfg.Servers["github"] = config.ServerConfig{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-github"},
		Env:     map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "secret"},
		Enabled: true,
	}
	cfg.Servers["context7"] = config.ServerConfig{
		URL:       "https://mcp.context7.com/mcp",
		Transport: "streamable-http",
		Enabled:   true,
	}

	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := Export(path, cfg); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Read back and verify the Claude Code format.
	// nosemgrep: go.filesystem.rule-fileread — path is a t.TempDir() test file,
	// not untrusted input.
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(doc.MCPServers) != 2 {
		t.Fatalf("mcpServers = %d, want 2", len(doc.MCPServers))
	}
}

func TestRoundTrip(t *testing.T) {
	// Import then export should be lossless for the fields Claude Code supports.
	raw := `{
		"mcpServers": {
			"a": {"command": "echo", "args": ["hi"], "env": {"K": "v"}},
			"b": {"url": "https://x/mcp", "type": "http"}
		}
	}`
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cfg := config.Default()
	if err := Import(path, cfg); err != nil {
		t.Fatalf("Import: %v", err)
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := Export(out, cfg); err != nil {
		t.Fatalf("Export: %v", err)
	}

	// Re-import the exported file and compare.
	cfg2 := config.Default()
	if err := Import(out, cfg2); err != nil {
		t.Fatalf("re-Import: %v", err)
	}
	if len(cfg2.Servers) != 2 {
		t.Fatalf("servers = %d, want 2", len(cfg2.Servers))
	}
	a := cfg2.Servers["a"]
	if a.Command != "echo" || len(a.Args) != 1 || a.Args[0] != "hi" {
		t.Errorf("server a = %+v", a)
	}
	if a.Env["K"] != "v" {
		t.Errorf("server a env = %v", a.Env)
	}
	b := cfg2.Servers["b"]
	if b.URL != "https://x/mcp" || b.Transport != "streamable-http" {
		t.Errorf("server b = %+v", b)
	}
}
