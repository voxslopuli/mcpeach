// Command sse is a deterministic loopback SSE MCP server used by E2E tests.
// It exposes a single "echo" tool over SSE.
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

func main() {
	s := server.NewMCPServer("sse", "1.0.0")
	s.AddTool(mcp.NewTool("echo", mcp.WithString("text", mcp.Required(), mcp.Description("text to echo"))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			text := req.GetString("text", "")
			return mcp.NewToolResultText(text), nil
		})

	addr := os.Getenv("SSE_ADDR")
	if addr == "" {
		addr = "127.0.0.1:0"
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sse: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "sse listening on %s\n", ln.Addr().String())
	sseServer := server.NewSSEServer(s)
	if err := http.Serve(ln, sseServer); err != nil {
		fmt.Fprintf(os.Stderr, "sse: %v\n", err)
		os.Exit(1)
	}
}
