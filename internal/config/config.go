// Package config defines the mcpeach YAML configuration schema, its
// load/save/validate lifecycle, and XDG path resolution.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/adrg/xdg"
	"gopkg.in/yaml.v3"
)

// Config is the root of the mcpeach configuration.
type Config struct {
	Gateway GatewayConfig           `yaml:"gateway"`
	LLM     LLMConfig               `yaml:"llm"`
	Servers map[string]ServerConfig `yaml:"servers"`
	Groups  map[string]GroupConfig  `yaml:"groups"`
}

// GatewayConfig configures the MCP streamable-HTTP endpoint.
type GatewayConfig struct {
	Addr    string `yaml:"addr"`
	Name    string `yaml:"name"`
	Version string `yaml:"version"`
}

// LLMConfig configures the OpenAI-compatible tool-finder endpoint.
type LLMConfig struct {
	BaseURL            string `yaml:"base_url"`
	Model              string `yaml:"model"`
	APIKey             string `yaml:"api_key"`
	EnrichDescriptions bool   `yaml:"enrich_descriptions"`
}

// ServerConfig describes a single MCP server: either a stdio subprocess
// (Command/Args/Env) or a remote endpoint (URL/Transport).
type ServerConfig struct {
	Command   string            `yaml:"command,omitempty"`
	Args      []string          `yaml:"args,omitempty"`
	Env       map[string]string `yaml:"env,omitempty"`
	URL       string            `yaml:"url,omitempty"`
	Transport string            `yaml:"transport,omitempty"`
	Enabled   bool              `yaml:"enabled"`
	Tools     ToolConfig        `yaml:"tools,omitempty"`
}

// ToolConfig is the per-server allow/block permission filter.
type ToolConfig struct {
	Mode string   `yaml:"mode,omitempty"` // "allow" | "block"
	List []string `yaml:"list,omitempty"`
}

// GroupConfig curates a subset of tools exposed at /v0/groups/{name}/mcp.
type GroupConfig struct {
	Description     string   `yaml:"description,omitempty"`
	IncludedServers []string `yaml:"included_servers,omitempty"`
	IncludedTools   []string `yaml:"included_tools,omitempty"`
	ExcludedTools   []string `yaml:"excluded_tools,omitempty"`
}

// Default returns a config with sensible defaults.
func Default() *Config {
	return &Config{
		Gateway: GatewayConfig{
			Addr:    "127.0.0.1:8080",
			Name:    "mcpeach",
			Version: "0.1.0",
		},
		LLM: LLMConfig{
			BaseURL:            "http://localhost:11434/v1",
			Model:              "qwen2.5-coder:7b",
			EnrichDescriptions: true,
		},
		Servers: map[string]ServerConfig{},
		Groups:  map[string]GroupConfig{},
	}
}

// Path returns the config file path under the XDG config dir.
// XDG_CONFIG_HOME is read at call time (not cached) so tests can override it.
func Path() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		base = xdg.ConfigHome
	}
	return filepath.Join(base, "mcpeach", "mcpeach.yml")
}

// SocketPath returns the control-plane unix socket path under the XDG runtime
// dir (falling back to the config dir). XDG_RUNTIME_DIR is read at call time.
func SocketPath() string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.Getenv("XDG_CONFIG_HOME")
		if base == "" {
			base = xdg.ConfigHome
		}
	}
	return filepath.Join(base, "mcpeach", "mcpeach.sock")
}

// Load reads and parses the config file at path.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := Default()
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Servers == nil {
		c.Servers = map[string]ServerConfig{}
	}
	if c.Groups == nil {
		c.Groups = map[string]GroupConfig{}
	}
	return c, nil
}

// Save writes the config to path atomically: it marshals to a temporary file
// in the same directory and renames it over path, so an interrupted write can
// never leave a truncated config behind. Parent directories are created.
func Save(path string, c *Config) error {
	if c == nil {
		return errors.New("config is nil")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// Validate checks the config for structural errors.
func (c *Config) Validate() error {
	if c.Gateway.Addr == "" {
		return errors.New("gateway.addr is required")
	}
	if c.LLM.BaseURL == "" {
		return errors.New("llm.base_url is required")
	}
	if c.LLM.Model == "" {
		return errors.New("llm.model is required")
	}
	for name, s := range c.Servers {
		if err := validateServer(name, s); err != nil {
			return err
		}
	}
	for name, g := range c.Groups {
		if err := c.validateGroup(name, g); err != nil {
			return err
		}
	}
	return nil
}

func validateServer(name string, s ServerConfig) error {
	if strings.Contains(name, "__") {
		return fmt.Errorf("server %q: name cannot contain '__' (reserved for tool canonicalization)", name)
	}
	hasCmd := s.Command != ""
	hasURL := s.URL != ""
	if !hasCmd && !hasURL {
		return fmt.Errorf("server %q: must set command or url", name)
	}
	if hasCmd && hasURL {
		return fmt.Errorf("server %q: cannot set both command and url", name)
	}
	if hasURL {
		switch s.Transport {
		case "", "streamable-http", "sse":
		default:
			return fmt.Errorf("server %q: unknown transport %q", name, s.Transport)
		}
	}
	switch s.Tools.Mode {
	case "", "allow", "block":
	default:
		return fmt.Errorf("server %q: unknown tools.mode %q", name, s.Tools.Mode)
	}
	return nil
}

func (c *Config) validateGroup(name string, g GroupConfig) error {
	for _, s := range g.IncludedServers {
		if _, ok := c.Servers[s]; !ok {
			return fmt.Errorf("group %q: included_server %q not found", name, s)
		}
	}
	if err := c.validateToolList(name, g.IncludedTools); err != nil {
		return err
	}
	return c.validateToolList(name, g.ExcludedTools)
}

// validateToolList checks that each canonical "<server>__<tool>" name
// references a known server.
func (c *Config) validateToolList(groupName string, tools []string) error {
	for _, t := range tools {
		server, _, err := splitTool(t)
		if err != nil {
			return fmt.Errorf("group %q: %w", groupName, err)
		}
		if _, ok := c.Servers[server]; !ok {
			return fmt.Errorf("group %q: tool %q references unknown server %q", groupName, t, server)
		}
	}
	return nil
}

// SplitCanonical splits a canonical "<server>__<tool>" name on the FIRST
// "__" separator, returning the server and tool parts. It returns ok=false
// when the name has no separator or either part is empty.
func SplitCanonical(name string) (server, tool string, ok bool) {
	server, tool, ok = strings.Cut(name, "__")
	if !ok || server == "" || tool == "" {
		return "", "", false
	}
	return server, tool, true
}

// splitTool parses a canonical "<server>__<tool>" name.
func splitTool(name string) (string, string, error) {
	server, tool, ok := SplitCanonical(name)
	if !ok {
		return "", "", fmt.Errorf("invalid tool name %q (want <server>__<tool>)", name)
	}
	return server, tool, nil
}
