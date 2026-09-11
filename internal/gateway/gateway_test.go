package gateway

import (
	"context"
	"sync"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
)

func TestCanonicalize(t *testing.T) {
	tests := []struct {
		server, tool, want string
	}{
		{"github", "create_or_update_file", "github__create_or_update_file"},
		{"context7", "get-library-docs", "context7__get-library-docs"},
	}
	for _, tt := range tests {
		if got := Canonicalize(tt.server, tt.tool); got != tt.want {
			t.Errorf("Canonicalize(%q, %q) = %q, want %q", tt.server, tt.tool, got, tt.want)
		}
	}
}

func TestGatewayAggregatesTools(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
			"b": {Enabled: true},
		},
	})

	// Register tools from two servers.
	g.RegisterTool("a", mcp.Tool{Name: "t1", Description: "a t1"})
	g.RegisterTool("a", mcp.Tool{Name: "t2", Description: "a t2"})
	g.RegisterTool("b", mcp.Tool{Name: "t1", Description: "b t1"})

	tools := g.Tools()
	if len(tools) != 3 {
		t.Fatalf("Tools() = %d, want 3", len(tools))
	}
	// Canonicalized names.
	names := map[string]bool{}
	for _, t := range tools {
		names[t.Name] = true
	}
	for _, want := range []string{"a__t1", "a__t2", "b__t1"} {
		if !names[want] {
			t.Errorf("missing canonical tool %q in %v", want, names)
		}
	}
}

func TestGatewayDisabledServerExcluded(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
			"b": {Enabled: false},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("b", mcp.Tool{Name: "t1"})

	tools := g.Tools()
	if len(tools) != 1 || tools[0].Name != "a__t1" {
		t.Errorf("Tools() = %v, want [a__t1]", tools)
	}
}

func TestGatewayPermissionFilter(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {
				Enabled: true,
				Tools:   config.ToolConfig{Mode: "allow", List: []string{"a__t1"}},
			},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("a", mcp.Tool{Name: "t2"})

	tools := g.Tools()
	if len(tools) != 1 || tools[0].Name != "a__t1" {
		t.Errorf("Tools() = %v, want [a__t1]", tools)
	}
}

func TestGatewayGroupTools(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
			"b": {Enabled: true},
		},
		Groups: map[string]config.GroupConfig{
			"g": {
				IncludedServers: []string{"a"},
				ExcludedTools:   []string{"a__t2"},
			},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("a", mcp.Tool{Name: "t2"})
	g.RegisterTool("b", mcp.Tool{Name: "t1"})

	tools := g.GroupTools("g")
	if len(tools) != 1 || tools[0].Name != "a__t1" {
		t.Errorf("GroupTools(g) = %v, want [a__t1]", tools)
	}
}

func TestGatewayGroupToolsAppliesPermissionFilter(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {
				Enabled: true,
				Tools:   config.ToolConfig{Mode: "allow", List: []string{"a__t1"}},
			},
		},
		Groups: map[string]config.GroupConfig{
			"g": {IncludedServers: []string{"a"}},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("a", mcp.Tool{Name: "t2"})
	// Simulate a tool that slipped past registration (e.g. registered before
	// the filter was tightened): GroupTools must still exclude it.
	g.tools["a__t2"] = mcp.Tool{Name: "a__t2"}

	tools := g.GroupTools("g")
	if len(tools) != 1 || tools[0].Name != "a__t1" {
		t.Errorf("GroupTools(g) = %v, want [a__t1] (blocked a__t2 excluded)", tools)
	}
}

func TestGatewayGroupUnknown(t *testing.T) {
	g := New(&config.Config{})
	if tools := g.GroupTools("nope"); tools != nil {
		t.Errorf("GroupTools(nope) = %v, want nil", tools)
	}
}

func TestGatewayToolFilterFunc(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("a", mcp.Tool{Name: "t2"})

	// With no permission filter, both tools pass through.
	filter := g.ToolFilterFunc()
	ctx := context.Background()
	all := []mcp.Tool{
		{Name: "a__t1"},
		{Name: "a__t2"},
	}
	got := filter(ctx, all)
	if len(got) != 2 {
		t.Errorf("ToolFilterFunc() = %d tools, want 2", len(got))
	}
}

func TestGatewayToolFilterFuncBlocks(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {
				Enabled: true,
				Tools:   config.ToolConfig{Mode: "allow", List: []string{"a__t1"}},
			},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("a", mcp.Tool{Name: "t2"})

	// Only a__t1 is allowed, so the filter blocks a__t2.
	filter := g.ToolFilterFunc()
	ctx := context.Background()
	all := []mcp.Tool{
		{Name: "a__t1"},
		{Name: "a__t2"},
	}
	got := filter(ctx, all)
	if len(got) != 1 || got[0].Name != "a__t1" {
		t.Errorf("ToolFilterFunc() = %v, want [a__t1]", got)
	}
}

func TestGatewayRemoveServer(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
			"b": {Enabled: true},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("a", mcp.Tool{Name: "t2"})
	g.RegisterTool("b", mcp.Tool{Name: "t1"})
	fc := &fakeToolCaller{}
	g.RegisterClient("a", fc)

	removed := g.RemoveServer("a")
	if removed != fc {
		t.Errorf("RemoveServer returned %v, want the registered client", removed)
	}
	// The caller owns closing the returned client.
	if closer, ok := removed.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	if !fc.closed {
		t.Error("caller should close the returned client")
	}
	// Only server b's tool remains.
	tools := g.Tools()
	if len(tools) != 1 || tools[0].Name != "b__t1" {
		t.Errorf("Tools() = %v, want [b__t1]", tools)
	}
}

func TestGatewayRemoveServerUnknown(t *testing.T) {
	g := New(&config.Config{})
	if got := g.RemoveServer("nope"); got != nil {
		t.Errorf("RemoveServer(nope) = %v, want nil", got)
	}
}

func TestGatewayRemoveServerPrefixSafety(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a":  {Enabled: true},
			"ab": {Enabled: true},
		},
	})
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("ab", mcp.Tool{Name: "t1"})

	g.RemoveServer("a")

	tools := g.Tools()
	if len(tools) != 1 || tools[0].Name != "ab__t1" {
		t.Errorf("Tools() = %v, want [ab__t1] (server a's tools removed, ab's survive)", tools)
	}
}

func TestGatewayClose(t *testing.T) {
	g := New(&config.Config{})
	fc := &fakeToolCaller{}
	g.RegisterClient("a", fc)

	g.Close()

	if !fc.closed {
		t.Error("Close did not close the registered client")
	}
	if len(g.clients) != 0 {
		t.Errorf("clients after Close = %d, want 0", len(g.clients))
	}
}

func TestGatewayCloseClient(t *testing.T) {
	g := New(&config.Config{})
	// Register a client that tracks Close.
	fc := &fakeToolCaller{}
	g.RegisterClient("a", fc)
	g.CloseClient("a")
	if !fc.closed {
		t.Error("CloseClient did not close the client")
	}
	// Closing again is a no-op.
	g.CloseClient("a")
}

func TestGatewayMetrics(t *testing.T) {
	g := New(&config.Config{})
	// Metrics on an empty gateway should not panic.
	_ = g.Metrics()
}

// fakeToolCaller is a ToolCaller that records Close.
type fakeToolCaller struct {
	closed bool
}

func (f *fakeToolCaller) CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("ok"), nil
}
func (f *fakeToolCaller) Close() error { f.closed = true; return nil }

func TestGatewayConcurrent(t *testing.T) {
	g := New(&config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
			"b": {Enabled: true},
		},
	})

	// Concurrent registration and reads must not race.
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			g.RegisterTool("a", mcp.Tool{Name: "t" + string(rune('a'+n%26))})
			g.RegisterTool("b", mcp.Tool{Name: "t" + string(rune('a'+n%26))})
			_ = g.Tools()
			_ = g.GroupTools("g")
		}(i)
	}
	wg.Wait()
}

// TestGatewayConcurrentConfigSwap hammers the gateway's read paths while
// another goroutine swaps the config snapshot. The gateway used to read a
// *config.Config that the control plane mutated in place; run with -race.
func TestGatewayConcurrentConfigSwap(t *testing.T) {
	newCfg := func() *config.Config {
		return &config.Config{
			Servers: map[string]config.ServerConfig{
				"a": {Enabled: true},
				"b": {Enabled: true},
			},
			Groups: map[string]config.GroupConfig{
				"g": {IncludedServers: []string{"a", "b"}},
			},
		}
	}
	g := New(newCfg())
	g.RegisterTool("a", mcp.Tool{Name: "t1"})
	g.RegisterTool("b", mcp.Tool{Name: "t1"})

	stop := make(chan struct{})
	var writers, readers sync.WaitGroup

	writers.Add(1)
	go func() {
		defer writers.Done()
		for i := 0; i < 500; i++ {
			if i%2 == 0 {
				g.cfg.Store(newCfg())
			} else {
				g.SetConfig(newCfg())
			}
		}
	}()

	for i := 0; i < 8; i++ {
		readers.Add(1)
		go func() {
			defer readers.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				g.RegisterTool("a", mcp.Tool{Name: "t1"})
				g.RegisterTool("b", mcp.Tool{Name: "t2"})
				_ = g.Tools()
				_ = g.GroupTools("g")
			}
		}()
	}

	writers.Wait()
	close(stop)
	readers.Wait()
}

func TestSetConfigRebuildsFilters(t *testing.T) {
	// A server added via SetConfig must get a filter; RegisterTool for it
	// must not panic on a nil filter (regression for the stale-filters bug).
	cfg := config.Default()
	cfg.Servers = map[string]config.ServerConfig{
		"old": {Command: "echo", Enabled: true},
	}
	gw := New(cfg)

	// Publish a config with a new server.
	cfg2 := config.Default()
	cfg2.Servers = map[string]config.ServerConfig{
		"old": {Command: "echo", Enabled: true},
		"new": {Command: "echo", Enabled: true},
	}
	gw.SetConfig(cfg2)

	// RegisterTool for the new server must not panic.
	gw.RegisterTool("new", mcp.Tool{Name: "echo", Description: "echo"})
	found := false
	for _, t := range gw.Tools() {
		if t.Name == "new__echo" {
			found = true
		}
	}
	if !found {
		t.Error("tool new__echo not registered after SetConfig")
	}
}
