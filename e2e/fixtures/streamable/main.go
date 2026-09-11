// Command streamable is a deterministic loopback streamable-HTTP MCP server
// used by E2E tests. It exposes a single "echo" tool over streamable HTTP.
package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// netListen binds a TCP listener on the given address.
func netListen(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

func main() {
	s := server.NewMCPServer("streamable", "1.0.0")
	s.AddTool(mcp.NewTool("echo", mcp.WithString("text", mcp.Required(), mcp.Description("text to echo"))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			text := req.GetString("text", "")
			return mcp.NewToolResultText(text), nil
		})

	addr := os.Getenv("STREAMABLE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	httpSrv := server.NewStreamableHTTPServer(s)
	ln, err := netListen(addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "streamable: %v\n", err)
		os.Exit(1)
	}
	// Print the bound address so tests can discover it.
	fmt.Fprintf(os.Stderr, "streamable listening on %s\n", ln.Addr().String())
	if err := http.Serve(ln, httpSrv); err != nil {
		fmt.Fprintf(os.Stderr, "streamable: %v\n", err)
		os.Exit(1)
	}
}
