// Package testutil provides shared helpers for tests across packages.
package testutil

import (
	"context"
	"net"
	"net/http"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

// BuildFakeServer compiles the testdata fake MCP server binary and returns its
// path. It is shared by the server, connect, and control test suites.
func BuildFakeServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-mcp")
	cmd := exec.Command("go", "build", "-o", bin, "../../testdata/fake-mcp") // NOSONAR: S2076 — bin is a t.TempDir() path (trusted test input), not user input.
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake server: %v\n%s", err, out)
	}
	return bin
}

// StartRemoteMCP spins up a streamable-http MCP server exposing an "echo" tool
// on a free port and returns its base URL (e.g. "http://127.0.0.1:PORT").
func StartRemoteMCP(t *testing.T) string {
	t.Helper()
	ms := mcpserver.NewMCPServer("remote", "1.0.0")
	ms.AddTool(mcp.NewTool("echo", mcp.WithString("text", mcp.Required())),
		func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return mcp.NewToolResultText(req.GetString("text", "")), nil
		})
	httpSrv := mcpserver.NewStreamableHTTPServer(ms)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	go func() { _ = http.Serve(ln, httpSrv) }()
	t.Cleanup(func() { _ = ln.Close() })
	return "http://" + ln.Addr().String() // NOSONAR: S5145 — test-only localhost helper, not production traffic
}
