// Package gateway aggregates tools from multiple MCP servers, canonicalizes
// their names, applies permission filters, and exposes group-filtered views.
package gateway

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/permission"
)

// Canonicalize returns the canonical "<server>__<tool>" name.
func Canonicalize(server, tool string) string {
	return server + "__" + tool
}

// Gateway aggregates tools from configured servers. It is safe for concurrent
// use: registration and lookups may happen across multiple goroutines.
type Gateway struct {
	mu      sync.RWMutex
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
	g.mu.Lock()
	g.tools[canonical] = tool
	g.mu.Unlock()
}

// Tools returns all aggregated tools, sorted by name.
func (g *Gateway) Tools() []mcp.Tool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return sortedTools(g.tools)
}

// GroupTools returns the tools exposed by a named group, or nil if unknown.
func (g *Gateway) GroupTools(name string) []mcp.Tool {
	grp, ok := g.cfg.Groups[name]
	if !ok {
		return nil
	}
	g.mu.RLock()
	defer g.mu.RUnlock()
	// Build the catalog of canonical tools per server in a single pass.
	catalog := map[string][]string{}
	for canonical := range g.tools {
		idx := strings.Index(canonical, "__")
		if idx < 0 {
			continue
		}
		srvName := canonical[:idx]
		if s, ok := g.cfg.Servers[srvName]; ok && s.Enabled {
			catalog[srvName] = append(catalog[srvName], canonical)
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

// sortedTools returns the values of a tool map sorted by name.
func sortedTools(tools map[string]mcp.Tool) []mcp.Tool {
	out := make([]mcp.Tool, 0, len(tools))
	for _, t := range tools {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
