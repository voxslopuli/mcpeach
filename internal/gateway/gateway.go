// Package gateway aggregates tools from multiple MCP servers, canonicalizes
// their names, applies permission filters, and exposes group-filtered views.
package gateway

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/metrics"
	"github.com/mcpeach/mcpeach/internal/obs"
	"github.com/mcpeach/mcpeach/internal/permission"
)

// Canonicalize returns the canonical "<server>__<tool>" name.
func Canonicalize(server, tool string) string {
	return server + "__" + tool
}

// ToolCaller is the minimal interface the gateway needs to route a tool call
// to an upstream server. It is satisfied by mcp-go's client.MCPClient.
type ToolCaller interface {
	CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error)
}

// Gateway aggregates tools from configured servers. It is safe for concurrent
// use: registration and lookups may happen across multiple goroutines.
type Gateway struct {
	mu      sync.RWMutex
	cfg     *config.Config
	filters map[string]permission.Filter // per-server permission filter
	tools   map[string]mcp.Tool          // canonical name -> tool
	clients map[string]ToolCaller        // server name -> upstream client
	metrics *metrics.Metrics             // tool-call observability
	log     *obs.Logger                  // structured logging
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
		clients: map[string]ToolCaller{},
		metrics: metrics.New(),
		log:     obs.Default().With("pkg", "gateway"),
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
		srvName, _, ok := config.SplitCanonical(canonical)
		if !ok {
			continue
		}
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
	return func(_ context.Context, tools []mcp.Tool) []mcp.Tool {
		// Read the allowed set at call time so tools registered after the
		// streaming server was constructed are still exposed.
		allowed := map[string]bool{}
		for _, t := range g.Tools() {
			allowed[t.Name] = true
		}
		out := make([]mcp.Tool, 0, len(tools))
		for _, t := range tools {
			if allowed[t.Name] {
				out = append(out, t)
			}
		}
		return out
	}
}

// RegisterClient associates an upstream client with a server, enabling tool
// call routing.
func (g *Gateway) RegisterClient(server string, c ToolCaller) {
	g.mu.Lock()
	g.clients[server] = c
	g.mu.Unlock()
}

// RemoveServer atomically removes a server's client and all canonical tools
// owned by that server. Returns the removed client (if any) so the caller can
// close it after the removal is published.
func (g *Gateway) RemoveServer(server string) ToolCaller {
	g.mu.Lock()
	c := g.clients[server]
	delete(g.clients, server)
	for name := range g.tools {
		srv, _, ok := config.SplitCanonical(name)
		if ok && srv == server {
			delete(g.tools, name)
		}
	}
	g.mu.Unlock()
	return c
}

// Close closes all registered upstream clients. It is called on daemon
// shutdown so clients added via the control plane (which register directly on
// the gateway) are not leaked.
func (g *Gateway) Close() {
	g.mu.Lock()
	clients := make([]ToolCaller, 0, len(g.clients))
	for _, c := range g.clients {
		clients = append(clients, c)
	}
	g.clients = map[string]ToolCaller{}
	g.mu.Unlock()
	for _, c := range clients {
		if closer, ok := c.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}
}

// CloseClient closes and removes the upstream client for a server, if any.
func (g *Gateway) CloseClient(server string) {
	if c := g.RemoveServer(server); c != nil {
		if closer, ok := c.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}
}

// CallTool routes a call for a canonical "<server>__<tool>" name to the
// originating server's client, stripping the server prefix before forwarding.
// It records the call in the gateway's metrics.
func (g *Gateway) CallTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := req.Params.Name
	server, tool, ok := config.SplitCanonical(name)
	if !ok {
		return nil, fmt.Errorf("invalid tool name %q (want <server>__<tool>)", name)
	}

	g.mu.RLock()
	c, ok := g.clients[server]
	g.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("no client for server %q", server)
	}

	// Forward with the original (non-canonical) tool name, recording latency.
	req.Params.Name = tool
	start := time.Now()
	result, err := c.CallTool(ctx, req)
	latency := time.Since(start)
	g.metrics.RecordToolCall(server, tool, latency, err)
	if err != nil {
		g.log.Error("tool call failed", "server", server, "tool", tool, "latency", latency, "err", err)
	} else {
		g.log.Info("tool call", "server", server, "tool", tool, "latency", latency)
	}
	return result, err
}

// Metrics returns the gateway's tool-call metrics snapshot.
func (g *Gateway) Metrics() metrics.Snapshot {
	return g.metrics.Snapshot()
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
