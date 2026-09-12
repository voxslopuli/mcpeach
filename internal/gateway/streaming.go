package gateway

import (
	"context"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/voxslopuli/mcpeach/internal/obs"
)

// StreamingServer is a single local streamable-HTTP MCP server that re-exposes
// the gateway's aggregated tools. Clients connect to it as one unified MCP
// server, regardless of how many underlying servers (stdio, SSE, streamable-HTTP)
// are aggregated.
type StreamingServer struct {
	mcpServer *server.MCPServer
	http      *server.StreamableHTTPServer
	gw        *Gateway
	group     string // non-empty for group-scoped servers
}

// NewStreamingServer builds a streamable-HTTP MCP server from the gateway,
// applying the gateway's tool filter so only aggregated tools are exposed.
func NewStreamingServer(g *Gateway, name, version string) *StreamingServer {
	mcpServer := server.NewMCPServer(
		name,
		version,
		server.WithToolFilter(g.ToolFilterFunc()),
	)
	s := &StreamingServer{mcpServer: mcpServer, gw: g}
	s.SyncTools()
	httpSrv := server.NewStreamableHTTPServer(mcpServer)
	s.http = httpSrv
	return s
}

// NewGroupStreamingServer builds a streamable-HTTP MCP server exposing only the
// tools in the named group's resolved catalog. It is mounted at
// /v0/groups/{group}/mcp by the daemon.
func NewGroupStreamingServer(g *Gateway, name, version, group string) *StreamingServer {
	mcpServer := server.NewMCPServer(name, version)
	s := &StreamingServer{mcpServer: mcpServer, gw: g, group: group}
	s.SyncTools()
	httpSrv := server.NewStreamableHTTPServer(mcpServer)
	s.http = httpSrv
	return s
}

// SyncTools re-registers the gateway's current tools on the MCP server. It is
// called at construction and should be called after the gateway topology
// changes (e.g. a server is started at runtime) so newly discovered tools are
// exposed without rebuilding the streaming server. Group-scoped servers only
// expose the group's resolved catalog.
func (s *StreamingServer) SyncTools() {
	if s.gw == nil {
		return
	}
	var tools []mcp.Tool
	if s.group != "" {
		tools = s.gw.GroupTools(s.group)
	} else {
		tools = s.gw.Tools()
	}
	st := make([]server.ServerTool, 0, len(tools))
	for _, t := range tools {
		tool := t
		st = append(st, server.ServerTool{
			Tool: tool,
			Handler: func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				// SetTools already routes only registered tools; the gateway
				// resolves the canonical name and forwards upstream.
				return s.gw.CallTool(ctx, req)
			},
		})
	}
	s.mcpServer.SetTools(st...)
}

// ServeHTTP implements http.Handler, mounting the MCP endpoint at /mcp (or at
// /v0/groups/{group}/mcp for group-scoped servers). Other paths return 404.
func (s *StreamingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if s.group != "" {
		if r.URL.Path != s.groupPath() {
			http.NotFound(w, r)
			return
		}
	} else if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	obs.Default().Info("mcp request", "method", r.Method, "path", r.URL.Path, "group", s.group)
	s.http.ServeHTTP(w, r)
}

// groupPath returns the mount path for a group-scoped server, or "" for the
// global server.
func (s *StreamingServer) groupPath() string {
	if s.group == "" {
		return ""
	}
	return "/v0/groups/" + s.group + "/mcp"
}
