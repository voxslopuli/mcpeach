// Command fake-mcp is a minimal MCP server used in tests. It exposes a single
// tool ("echo") over stdio so the connect layer and gateway can be exercised
// end-to-end. It also emits a JSON log line on startup (captured by the server
// manager) so spawn/capture/stop lifecycle can be verified.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func main() {
	// Emit a JSON log line to stderr (the manager captures stderr), including
	// any args so tests can verify they reach the subprocess.
	fmt.Fprintf(os.Stderr, `{"level":"info","msg":"fake-mcp started","args":%q,"time":%q}`+"\n", os.Args[1:], time.Now().Format(time.RFC3339))

	s := server.NewMCPServer("fake-mcp", "1.0.0")
	s.AddTool(mcp.NewTool("echo", mcp.WithString("text", mcp.Required(), mcp.Description("text to echo"))),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			text := req.GetString("text", "")
			return mcp.NewToolResultText(text), nil
		})

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "fake-mcp: %v\n", err)
		os.Exit(1)
	}
}
