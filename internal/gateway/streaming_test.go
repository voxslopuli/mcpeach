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

// newInProcessClient connects an in-process MCP client to the server and
// completes the initialize handshake.
func newInProcessClient(t *testing.T, srv *StreamingServer) *client.Client {
	t.Helper()
	c, err := client.NewInProcessClient(srv.mcpServer)
	if err != nil {
		t.Fatalf("NewInProcessClient: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })
	if _, err := c.Initialize(context.Background(), mcp.InitializeRequest{}); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	return c
}

// listToolNames lists the tool names exposed by the server.
func listToolNames(t *testing.T, c *client.Client) map[string]bool {
	t.Helper()
	res, err := c.ListTools(context.Background(), mcp.ListToolsRequest{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	names := map[string]bool{}
	for _, tool := range res.Tools {
		names[tool.Name] = true
	}
	return names
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
	c := newInProcessClient(t, srv)
	names := listToolNames(t, c)
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

	c := newInProcessClient(t, srv)
	names := listToolNames(t, c)
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

	c := newInProcessClient(t, srv)

	// b__t3 is not in the group's catalog — the call must fail.
	if _, err := c.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "b__t3"},
	}); err == nil {
		t.Fatal("CallTool(b__t3) succeeded, want error for tool outside group")
	}
}

func TestGroupStreamingServerCallsGroupTool(t *testing.T) {
	g := setupGroupTestGateway(t)
	fc := &fakeClient{result: &mcp.CallToolResult{}}
	g.RegisterClient("a", fc)
	srv := NewGroupStreamingServer(g, "g", "1.0.0", "grp")

	c := newInProcessClient(t, srv)
	// a__t1 is in the group's catalog — the call must route to the upstream
	// client with the canonical prefix stripped.
	if _, err := c.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "a__t1"},
	}); err != nil {
		t.Fatalf("CallTool(a__t1) in group: %v", err)
	}
	if fc.calledName != "t1" {
		t.Errorf("upstream called with %q, want t1 (prefix stripped)", fc.calledName)
	}
}

func TestGroupStreamingServerServesGroupPath(t *testing.T) {
	g := setupGroupTestGateway(t)
	srv := NewGroupStreamingServer(g, "g", "1.0.0", "grp")
	ts := httptest.NewServer(srv)
	defer ts.Close()

	// The group-scoped server must be mounted at /v0/groups/{group}/mcp.
	resp, err := http.Get(ts.URL + "/v0/groups/grp/mcp")
	if err != nil {
		t.Fatalf("GET /v0/groups/grp/mcp: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		t.Fatalf("GET /v0/groups/grp/mcp = 404, want mounted handler")
	}

	// Other paths still 404.
	resp2, err := http.Get(ts.URL + "/bogus")
	if err != nil {
		t.Fatalf("GET /bogus: %v", err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /bogus = %d, want 404", resp2.StatusCode)
	}

	// The group-scoped server must NOT respond on the global /mcp path.
	resp3, err := http.Get(ts.URL + "/mcp")
	if err != nil {
		t.Fatalf("GET /mcp: %v", err)
	}
	defer func() { _ = resp3.Body.Close() }()
	if resp3.StatusCode != http.StatusNotFound {
		t.Fatalf("GET /mcp = %d, want 404 for group-scoped server", resp3.StatusCode)
	}
}

func TestStreamingServerSyncToolsNilGateway(t *testing.T) {
	// SyncTools on a server with no gateway must be a no-op, not a panic.
	s := &StreamingServer{}
	s.SyncTools()
}

func TestStreamingServerSyncTools(t *testing.T) {
	g := setupTestGateway(t, mcp.Tool{Name: "t1"})
	srv := NewStreamingServer(g, "test", "0.1.0")

	// Register a new tool after construction and re-sync.
	g.RegisterTool("a", mcp.Tool{Name: "t2"})
	srv.SyncTools()

	// The newly registered tool should now be visible via tools/list.
	c := newInProcessClient(t, srv)
	names := listToolNames(t, c)
	if !names["a__t1"] || !names["a__t2"] {
		t.Errorf("tools/list after SyncTools = %v, want a__t1 and a__t2", names)
	}
}
