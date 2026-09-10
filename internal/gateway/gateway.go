// Package gateway aggregates tools from multiple MCP servers, canonicalizes
// their names, applies permission filters, and exposes group-filtered views.
package gateway

import (
	"context"
	"sort"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/permission"
)

// Canonicalize returns the canonical "<server>__<tool>" name.
func Canonicalize(server, tool string) string {
	return server + "__" + tool
}

// Gateway aggregates tools from configured servers.
type Gateway struct {
	cfg     *config.Config
	filters map[string]permission.Filter // per-server permission filter
	tools   map[string]mcp.Tool          // canonical name -> tool
}

// New builds a Gateway from config.
func New(cfg *config.Config) *Gateway {
	filters := make(map[string]permission.Filter, len(cfg.Servers))
	for name, s := range cfg.Servers {
		filters[name] = permission.FromConfig(s.Tools)
	}
	return &Gateway{
		cfg:     cfg,
		filters: filters,
		tools:   map[string]mcp.Tool{},
	}
}

// RegisterTool adds a tool from a server, canonicalizing its name and applying
// the server's permission filter. Disabled servers are ignored.
func (g *Gateway) RegisterTool(server string, tool mcp.Tool) {
	s, ok := g.cfg.Servers[server]
	if !ok || !s.Enabled {
		return
	}
	canonical := Canonicalize(server, tool.Name)
	if !g.filters[server].Allows(canonical) {
		return
	}
	tool.Name = canonical
	g.tools[canonical] = tool
}

// Tools returns all aggregated tools, sorted by name.
func (g *Gateway) Tools() []mcp.Tool {
	out := make([]mcp.Tool, 0, len(g.tools))
	for _, t := range g.tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// GroupTools returns the tools exposed by a named group, or nil if unknown.
func (g *Gateway) GroupTools(name string) []mcp.Tool {
	grp, ok := g.cfg.Groups[name]
	if !ok {
		return nil
	}
	// Build the catalog of canonical tools per server.
	catalog := map[string][]string{}
	for server, s := range g.cfg.Servers {
		if !s.Enabled {
			continue
		}
		for canonical := range g.tools {
			if strings.HasPrefix(canonical, server+"__") {
				catalog[server] = append(catalog[server], canonical)
			}
		}
	}
	selected := permission.ResolveGroup(grp, catalog)
	out := make([]mcp.Tool, 0, len(selected))
	for _, name := range selected {
		if t, ok := g.tools[name]; ok {
			out = append(out, t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ToolFilterFunc returns an mcp-go ToolFilterFunc that exposes only the
// aggregated tools (used to mount the gateway as an MCP server).
func (g *Gateway) ToolFilterFunc() func(ctx context.Context, tools []mcp.Tool) []mcp.Tool {
	allowed := map[string]bool{}
	for _, t := range g.Tools() {
		allowed[t.Name] = true
	}
	return func(_ context.Context, tools []mcp.Tool) []mcp.Tool {
		out := make([]mcp.Tool, 0, len(tools))
		for _, t := range tools {
			if allowed[t.Name] {
				out = append(out, t)
			}
		}
		return out
	}
}
