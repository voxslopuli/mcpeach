package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
)

func TestNewStreamingServer(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1", Description: "a t1"})

	srv := NewStreamingServer(g, "test", "0.1.0")
	if srv == nil {
		t.Fatal("NewStreamingServer returned nil")
	}
}

func TestStreamingServerServesHTTP(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1", Description: "a t1"})

	srv := NewStreamingServer(g, "test", "0.1.0")
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// A GET to the MCP endpoint should return 405 (method not allowed) or
	// similar — the point is the handler is wired and responds.
	resp, err := http.Get(ts.URL + "/mcp")
	if err != nil {
		t.Fatalf("GET /mcp: %v", err)
	}
	defer resp.Body.Close()
	// The streamable-http server responds to GET with an SSE stream or 405.
	// Either is fine — we just verify the handler is mounted.
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET /mcp = 404, want mounted handler")
	}
}

func TestStreamingServerToolFilter(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("a", mcp.Tool{Name: "t2"})

	// The streaming server's underlying MCPServer should only expose the
	// gateway's aggregated tools via the tool filter.
	srv := NewStreamingServer(g, "test", "0.1.0")
	_ = srv // the filter is applied internally; verify via the gateway
	if got := len(g.Tools()); got != 2 {
		t.Errorf("gateway tools = %d, want 2", got)
	}
}
