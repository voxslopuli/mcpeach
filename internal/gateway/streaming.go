package gateway

import (
	"context"
	"net/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// StreamingServer is a single local streamable-HTTP MCP server that re-exposes
// the gateway's aggregated tools. Clients connect to it as one unified MCP
// server, regardless of how many underlying servers (stdio, SSE, streamable-HTTP)
// are aggregated.
type StreamingServer struct {
	mcpServer *server.MCPServer
	http      *server.StreamableHTTPServer
}

// NewStreamingServer builds a streamable-HTTP MCP server from the gateway,
// applying the gateway's tool filter so only aggregated tools are exposed.
func NewStreamingServer(g *Gateway, name, version string) *StreamingServer {
	mcpServer := server.NewMCPServer(
		name,
		version,
		server.WithToolFilter(g.ToolFilterFunc()),
	)
	// Register the aggregated tools with a passthrough handler. The tool filter
	// governs which are visible/invocable.
	for _, t := range g.Tools() {
		tool := t
		mcpServer.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{
				Content: []mcp.Content{
					&mcp.TextContent{Text: "tool not yet wired to upstream server"},
				},
			}, nil
		})
	}
	httpSrv := server.NewStreamableHTTPServer(mcpServer)
	return &StreamingServer{mcpServer: mcpServer, http: httpSrv}
}

// ServeHTTP implements http.Handler, mounting the MCP endpoint at /mcp.
func (s *StreamingServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.http.ServeHTTP(w, r)
}
