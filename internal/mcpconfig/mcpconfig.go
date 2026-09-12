// Package mcpconfig converts between the Claude Code MCP config JSON format
// (`.mcp.json` / `claude_desktop_config.json`) and the mcpeach config.
package mcpconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/voxslopuli/mcpeach/internal/config"
	"github.com/voxslopuli/mcpeach/internal/secrets"
)

// claudeServer is one entry in a Claude Code mcpServers map.
type claudeServer struct {
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
	Type    string            `json:"type,omitempty"`
}

// claudeDoc is the top-level Claude Code MCP config document.
type claudeDoc struct {
	MCPServers map[string]claudeServer `json:"mcpServers"`
}

// Import reads a Claude Code MCP config JSON file and merges its servers into
// cfg, preserving existing servers. Servers whose names already exist in cfg
// are overwritten; the returned slice names each overwritten server, sorted.
// The merge is transactional: the imported servers are validated against a
// candidate copy first, so a malformed import returns an error and leaves the
// live config untouched.
// Import parses a Claude Code MCP config JSON file and returns a candidate
// config with the imported servers merged in, plus the names of servers that
// already existed (conflicts). It does NOT mutate the passed-in config: the
// caller decides how to publish the candidate (save to disk, and if a gateway
// is live, publish via SetConfig so its filters stay in sync).
// SecretRef identifies a likely-plaintext secret found during a preview.
type SecretRef struct {
	Server string `json:"server"`
	Name   string `json:"name"`
}

// Preview parses an import file and reports what would be imported (server
// names), which already exist (conflicts), and which env values look like
// plaintext secrets — without mutating the passed config.
func Preview(path string, cfg *config.Config) (imports []string, conflicts []string, secretRefs []SecretRef, err error) {
	if cfg == nil {
		return nil, nil, nil, fmt.Errorf("config is nil")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, nil, err
	}
	var doc claudeDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	for name, cs := range doc.MCPServers {
		imports = append(imports, name)
		if _, exists := cfg.Servers[name]; exists {
			conflicts = append(conflicts, name)
		}
		for k, v := range cs.Env {
			if !secrets.IsSecretRef(v) && secrets.LooksLikeSecretName(k) {
				secretRefs = append(secretRefs, SecretRef{Server: name, Name: k})
			}
		}
	}
	sort.Strings(imports)
	sort.Strings(conflicts)
	return imports, conflicts, secretRefs, nil
}

func Import(path string, cfg *config.Config) (*config.Config, []string, error) {
	if cfg == nil {
		return nil, nil, fmt.Errorf("config is nil")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc claudeDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	// Build a candidate copy so a failed validation leaves the live config
	// untouched. The explicit map rebuild keeps mutating candidate.Servers
	// from affecting cfg.Servers.
	// candidate is a shallow copy; Servers is explicitly re-allocated below.
	candidate := *cfg
	candidate.Servers = make(map[string]config.ServerConfig, len(cfg.Servers)+len(doc.MCPServers))
	for name, sc := range cfg.Servers {
		candidate.Servers[name] = sc
	}
	var conflicts []string
	for name, cs := range doc.MCPServers {
		if _, exists := candidate.Servers[name]; exists {
			conflicts = append(conflicts, name)
		}
		sc := config.ServerConfig{
			Command: cs.Command,
			Args:    cs.Args,
			Env:     cs.Env,
			URL:     cs.URL,
			Enabled: true,
		}
		// Map the Claude Code transport type to mcpeach's transport, only for
		// remote (URL) servers. stdio servers have no transport.
		if cs.URL != "" {
			switch cs.Type {
			case "sse":
				sc.Transport = "sse"
			default:
				sc.Transport = "streamable-http"
			}
		}
		candidate.Servers[name] = sc
	}
	// Validate the merged candidate before returning, so a malformed import
	// cannot be published.
	if err := candidate.Validate(); err != nil {
		return nil, nil, err
	}
	sort.Strings(conflicts)
	return &candidate, conflicts, nil
}

// Export writes cfg's servers to a Claude Code MCP config JSON file.
func Export(path string, cfg *config.Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	doc := claudeDoc{MCPServers: map[string]claudeServer{}}
	for name, sc := range cfg.Servers {
		cs := claudeServer{
			Command: sc.Command,
			Args:    sc.Args,
			Env:     sc.Env,
			URL:     sc.URL,
		}
		// Map mcpeach's transport back to the Claude Code type. stdio servers
		// (no URL) omit the type field per the Claude Code standard.
		if sc.URL != "" {
			switch sc.Transport {
			case "sse":
				cs.Type = "sse"
			default:
				cs.Type = "http"
			}
		}
		doc.MCPServers[name] = cs
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
