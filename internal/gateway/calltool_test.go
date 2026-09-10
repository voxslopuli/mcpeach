package gateway

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
)

// fakeClient is a minimal MCPClient that records calls.
type fakeClient struct {
	calledName string
	result     *mcp.CallToolResult
	err        error
}

func (f *fakeClient) CallTool(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	f.calledName = req.Params.Name
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func TestGatewayCallToolRoutes(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
			"b": {Enabled: true},
		},
	})
	fc := &fakeClient{result: &mcp.CallToolResult{}}
	g.RegisterClient("a", fc)

	// Call a canonical tool name; it should route to server "a" with the
	// original (non-canonical) tool name.
	_, err := g.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "a__t1"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if fc.calledName != "t1" {
		t.Errorf("upstream called with %q, want t1 (prefix stripped)", fc.calledName)
	}
}

func TestGatewayCallToolUnknownServer(t *testing.T) {
	g := New(&config.Config{})
	if _, err := g.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "nope__t1"},
	}); err == nil {
		t.Fatal("CallTool unknown server: want error, got nil")
	}
}

func TestGatewayCallToolNoClient(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
		},
	})
	// No client registered for "a".
	if _, err := g.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "a__t1"},
	}); err == nil {
		t.Fatal("CallTool with no client: want error, got nil")
	}
}

func TestGatewayCallToolUpstreamError(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
		},
	})
	fc := &fakeClient{err: errors.New("boom")}
	g.RegisterClient("a", fc)
	if _, err := g.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "a__t1"},
	}); err == nil {
		t.Fatal("CallTool upstream error: want error, got nil")
	}
}

func TestGatewayCallToolBadName(t *testing.T) {
	g := New(&config.Config{})
	if _, err := g.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "no-separator"},
	}); err == nil {
		t.Fatal("CallTool bad name: want error, got nil")
	}
}
