// Package mcpconfig converts between the Claude Code MCP config JSON format
// (`.mcp.json` / `claude_desktop_config.json`) and the mcpeach config.
package mcpconfig

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/mcpeach/mcpeach/internal/config"
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
// cfg, preserving existing servers. Conflicts (same name) are overwritten.
func Import(path string, cfg *config.Config) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var doc claudeDoc
	if err := json.Unmarshal(b, &doc); err != nil {
		return fmt.Errorf("parse %s: %w", path, err)
	}
	for name, cs := range doc.MCPServers {
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
		cfg.Servers[name] = sc
	}
	return nil
}

// Export writes cfg's servers to a Claude Code MCP config JSON file.
func Export(path string, cfg *config.Config) error {
	doc := claudeDoc{MCPServers: map[string]claudeServer{}}
	for name, sc := range cfg.Servers {
		cs := claudeServer{
			Command: sc.Command,
			Args:    sc.Args,
			Env:     sc.Env,
			URL:     sc.URL,
		}
		// Map mcpeach's transport back to the Claude Code type.
		switch sc.Transport {
		case "sse":
			cs.Type = "sse"
		default:
			cs.Type = "http"
		}
		doc.MCPServers[name] = cs
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
