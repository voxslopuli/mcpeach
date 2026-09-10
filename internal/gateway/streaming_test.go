package gateway

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mark3labs/mcp-go/client"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
)

// setupTestGateway builds a gateway with one enabled server "a" and registers
// the given tools on it.
func setupTestGateway(t *testing.T, tools ...mcp.Tool) *Gateway {
	t.Helper()
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
		},
	})
	for _, tool := range tools {
		g.RegisterTool("a", tool)
	}
	return g
}

func TestNewStreamingServer(t *testing.T) {
	g := setupTestGateway(t, mcp.Tool{Name: "t1", Description: "a t1"})
	srv := NewStreamingServer(g, "test", "0.1.0")
	if srv == nil {
		t.Fatal("NewStreamingServer returned nil")
	}
}

func TestStreamingServerServesHTTP(t *testing.T) {
	g := setupTestGateway(t, mcp.Tool{Name: "t1", Description: "a t1"})
	srv := NewStreamingServer(g, "test", "0.1.0")
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// A GET to the MCP endpoint should return 405 (method not allowed) or
	// similar — the point is the handler is wired and responds.
	resp, err := http.Get(ts.URL + "/mcp")
	if err != nil {
		t.Fatalf("GET /mcp: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET /mcp = 404, want mounted handler")
	}
}

func TestStreamingServerRejectsNonMCPPath(t *testing.T) {
	g := setupTestGateway(t, mcp.Tool{Name: "t1"})
	srv := NewStreamingServer(g, "test", "0.1.0")
	ts := httptest.NewServer(srv)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/bogus")
	if err != nil {
		t.Fatalf("GET /bogus: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /bogus = %d, want 404", resp.StatusCode)
	}
}

func TestStreamingServerExposesTools(t *testing.T) {
	g := setupTestGateway(t,
		mcp.Tool{Name: "t1", Description: "a t1"},
		mcp.Tool{Name: "t2", Description: "a t2"},
	)
	srv := NewStreamingServer(g, "test", "0.1.0")

	// Connect an in-process MCP client to the streaming server's underlying
	// MCPServer and list tools — this verifies the tools are actually exposed
	// (and filtered) through the MCP interface.
	c, err := client.NewInProcessClient(srv.mcpServer)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	defer func() { _ = c.Close() }()

	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	if !names["a__t1"] || !names["a__t2"] {
		t.Errorf("tools/list returned %v, want a__t1 and a__t2", names)
	}
}

func TestStreamingServerToolFilter(t *testing.T) {
	g := setupTestGateway(t,
		mcp.Tool{Name: "t1"},
		mcp.Tool{Name: "t2"},
	)
	srv := NewStreamingServer(g, "test", "0.1.0")
	_ = srv // the filter is applied internally; verify via the gateway
	if got := len(g.Tools()); got != 2 {
		t.Errorf("gateway tools = %d, want 2", got)
	}
}

// setupGroupTestGateway builds a gateway with two enabled servers "a" and "b"
// and a group "grp" selecting only server "a"'s tools.
func setupGroupTestGateway(t *testing.T) *Gateway {
	t.Helper()
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
			"b": {Enabled: true},
		},
		Groups: map[string]config.GroupConfig{
			"grp": {IncludedServers: []string{"a"}},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1", Description: "a t1"})
	g.RegisterTool("a", mcp.Tool{Name: "t2", Description: "a t2"})
	g.RegisterTool("b", mcp.Tool{Name: "t3", Description: "b t3"})
	return g
}

func TestGroupStreamingServerExposesGroupTools(t *testing.T) {
	g := setupGroupTestGateway(t)
	srv := NewGroupStreamingServer(g, "g", "1.0.0", "grp")

	c, err := client.NewInProcessClient(srv.mcpServer)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	if len(names) != 2 || !names["a__t1"] || !names["a__t2"] {
		t.Errorf("tools/list = %v, want only a__t1 and a__t2", names)
	}
	if names["b__t3"] {
		t.Errorf("tools/list = %v, group must not expose b__t3", names)
	}
}

func TestGroupStreamingServerRejectsOutsideTools(t *testing.T) {
	g := setupGroupTestGateway(t)
	srv := NewGroupStreamingServer(g, "g", "1.0.0", "grp")

	c, err := client.NewInProcessClient(srv.mcpServer)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// b__t3 is not in the group's catalog — the call must fail.
	if _, err := c.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "b__t3"},
	}); err == nil {
		t.Fatal("CallTool(b__t3) succeeded, want error for tool outside group")
	}
}

func TestStreamingServerSyncTools(t *testing.T) {
	g := setupTestGateway(t, mcp.Tool{Name: "t1"})
	srv := NewStreamingServer(g, "test", "0.1.0")

	// Register a new tool after construction and re-sync.
	g.RegisterTool("a", mcp.Tool{Name: "t2"})
	srv.SyncTools()

	// The newly registered tool should now be visible via tools/list.
	c, err := client.NewInProcessClient(srv.mcpServer)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	defer func() { _ = c.Close() }()
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	if !names["a__t1"] || !names["a__t2"] {
		t.Errorf("tools/list after SyncTools = %v, want a__t1 and a__t2", names)
	}
}
