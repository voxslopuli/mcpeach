package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default()
	if c.Gateway.Addr != "127.0.0.1:8080" {
		t.Errorf("Default gateway addr = %q, want 127.0.0.1:8080", c.Gateway.Addr)
	}
	if c.Gateway.Name != "mcpeach" {
		t.Errorf("Default gateway name = %q, want mcpeach", c.Gateway.Name)
	}
	if c.LLM.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("Default llm base_url = %q, want http://localhost:11434/v1", c.LLM.BaseURL)
	}
	if c.LLM.Model != "llama3.2" {
		t.Errorf("Default llm model = %q, want llama3.2", c.LLM.Model)
	}
	if !c.LLM.EnrichDescriptions {
		t.Error("Default llm enrich_descriptions = false, want true")
	}
	if len(c.Servers) != 0 {
		t.Errorf("Default servers = %d entries, want 0", len(c.Servers))
	}
	if len(c.Groups) != 0 {
		t.Errorf("Default groups = %d entries, want 0", len(c.Groups))
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcpeach.yml")

	c := Default()
	c.Servers["github"] = ServerConfig{
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-github"},
		Env:     map[string]string{"GITHUB_PERSONAL_ACCESS_TOKEN": "secret"},
		Enabled: true,
		Tools: ToolConfig{
			Mode: "allow",
			List: []string{"create_or_update_file"},
		},
	}
	c.Servers["context7"] = ServerConfig{
		URL:       "https://mcp.context7.com/mcp",
		Transport: "streamable-http",
		Enabled:   true,
	}
	c.Groups["claude-tools"] = GroupConfig{
		Description:     "Curated set for Claude Desktop",
		IncludedServers: []string{"github"},
		IncludedTools:   []string{"context7__get-library-docs"},
		ExcludedTools:   []string{},
	}

	if err := Save(path, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Gateway.Addr != c.Gateway.Addr {
		t.Errorf("gateway addr = %q, want %q", got.Gateway.Addr, c.Gateway.Addr)
	}
	if got.LLM.Model != c.LLM.Model {
		t.Errorf("llm model = %q, want %q", got.LLM.Model, c.LLM.Model)
	}
	if len(got.Servers) != 2 {
		t.Fatalf("servers = %d, want 2", len(got.Servers))
	}
	gh := got.Servers["github"]
	if gh.Command != "npx" || len(gh.Args) != 2 || gh.Args[0] != "-y" {
		t.Errorf("github server = %+v, want npx -y", gh)
	}
	if gh.Env["GITHUB_PERSONAL_ACCESS_TOKEN"] != "secret" {
		t.Errorf("github env token = %q, want secret", gh.Env["GITHUB_PERSONAL_ACCESS_TOKEN"])
	}
	if gh.Tools.Mode != "allow" || len(gh.Tools.List) != 1 {
		t.Errorf("github tools = %+v, want allow [create_or_update_file]", gh.Tools)
	}
	c7 := got.Servers["context7"]
	if c7.URL != "https://mcp.context7.com/mcp" || c7.Transport != "streamable-http" {
		t.Errorf("context7 server = %+v, want streamable-http", c7)
	}
	if len(got.Groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(got.Groups))
	}
	grp := got.Groups["claude-tools"]
	if grp.Description != "Curated set for Claude Desktop" {
		t.Errorf("group description = %q", grp.Description)
	}
	if len(grp.IncludedServers) != 1 || grp.IncludedServers[0] != "github" {
		t.Errorf("group included_servers = %v", grp.IncludedServers)
	}
}

func TestLoadMissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "nope.yml"))
	if err == nil {
		t.Fatal("Load of missing file: want error, got nil")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"valid default", func(c *Config) {}, false},
		{"empty gateway addr", func(c *Config) { c.Gateway.Addr = "" }, true},
		{"empty llm base_url", func(c *Config) { c.LLM.BaseURL = "" }, true},
		{"empty llm model", func(c *Config) { c.LLM.Model = "" }, true},
		{"server no command and no url", func(c *Config) {
			c.Servers["bad"] = ServerConfig{Enabled: true}
		}, true},
		{"server both command and url", func(c *Config) {
			c.Servers["bad"] = ServerConfig{Command: "x", URL: "http://y", Enabled: true}
		}, true},
		{"server bad transport", func(c *Config) {
			c.Servers["bad"] = ServerConfig{URL: "http://y", Transport: "bogus", Enabled: true}
		}, true},
		{"server bad tool mode", func(c *Config) {
			c.Servers["bad"] = ServerConfig{Command: "x", Enabled: true, Tools: ToolConfig{Mode: "bogus"}}
		}, true},
		{"group references missing server", func(c *Config) {
			c.Groups["g"] = GroupConfig{IncludedServers: []string{"nope"}}
		}, true},
		{"group references missing server", func(c *Config) {
			c.Groups["g"] = GroupConfig{IncludedServers: []string{"nope"}}
		}, true},
		{"group references missing tool server prefix", func(c *Config) {
			c.Groups["g"] = GroupConfig{IncludedTools: []string{"nope__tool"}}
		}, true},
		{"group tool with valid server prefix", func(c *Config) {
			c.Servers["s"] = ServerConfig{Command: "x", Enabled: true}
			c.Groups["g"] = GroupConfig{IncludedTools: []string{"s__anytool"}}
		}, false},
		{"group valid", func(c *Config) {
			c.Servers["s"] = ServerConfig{Command: "x", Enabled: true}
			c.Groups["g"] = GroupConfig{IncludedServers: []string{"s"}}
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default()
			tt.mutate(c)
			err := c.Validate()
			if tt.wantErr && err == nil {
				t.Errorf("Validate: want error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("Validate: want nil, got %v", err)
			}
		})
	}
}

func TestPath(t *testing.T) {
	// XDG_CONFIG_HOME override
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	p := Path()
	want := filepath.Join(dir, "mcpeach", "mcpeach.yml")
	if p != want {
		t.Errorf("Path() = %q, want %q", p, want)
	}
}

func TestSaveCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "deep")
	path := filepath.Join(dir, "mcpeach.yml")
	if err := Save(path, Default()); err != nil {
		t.Fatalf("Save to nested dir: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("saved file not found: %v", err)
	}
}
