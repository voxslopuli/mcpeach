package connect

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
)

// fakeCaller is a minimal ToolCaller for tests.
type fakeCaller struct {
	tools []mcp.Tool
}

func (f *fakeCaller) CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("ok"), nil
}

func (f *fakeCaller) ListTools(ctx context.Context, request mcp.ListToolsRequest) (*mcp.ListToolsResult, error) {
	return mcp.NewListToolsResult(f.tools, ""), nil
}

func TestDiscoverTools(t *testing.T) {
	caller := &fakeCaller{tools: []mcp.Tool{{Name: "a"}, {Name: "b"}}}
	tools, err := DiscoverTools(context.Background(), caller)
	if err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("tools = %d, want 2", len(tools))
	}
	if tools[0].Name != "a" || tools[1].Name != "b" {
		t.Errorf("tools = %v, want [a b]", tools)
	}
}

func TestConnectStdio(t *testing.T) {
	// A stdio server that doesn't exist should return an error.
	sc := config.ServerConfig{Command: "/nonexistent/binary", Args: []string{"x"}}
	_, _, err := Connect(context.Background(), sc)
	if err == nil {
		t.Fatal("Connect stdio nonexistent: want error, got nil")
	}
}

func TestConnectRemoteNoURL(t *testing.T) {
	// A remote config with no URL should error.
	sc := config.ServerConfig{Transport: "streamable-http"}
	_, _, err := Connect(context.Background(), sc)
	if err == nil {
		t.Fatal("Connect remote no URL: want error, got nil")
	}
}

func TestConnectNoCommandNoURL(t *testing.T) {
	sc := config.ServerConfig{}
	_, _, err := Connect(context.Background(), sc)
	if err == nil {
		t.Fatal("Connect no command/url: want error, got nil")
	}
}

func TestConnectUnknownTransport(t *testing.T) {
	sc := config.ServerConfig{URL: "http://x", Transport: "bogus"}
	_, _, err := Connect(context.Background(), sc)
	if err == nil {
		t.Fatal("Connect unknown transport: want error, got nil")
	}
}

func TestConnectRemoteBadURL(t *testing.T) {
	// A streamable-http URL that doesn't respond should error on initialize.
	sc := config.ServerConfig{URL: "http://127.0.0.1:1/mcp", Transport: "streamable-http"}
	_, _, err := Connect(context.Background(), sc)
	if err == nil {
		t.Fatal("Connect bad remote URL: want error, got nil")
	}
}

func TestDiscoverToolsNoListTools(t *testing.T) {
	// A caller that doesn't implement ListTools should error.
	_, err := DiscoverTools(context.Background(), &noListCaller{})
	if err == nil {
		t.Fatal("DiscoverTools no ListTools: want error, got nil")
	}
}

// noListCaller implements ToolCaller but not ListTools.
type noListCaller struct{}

func (n *noListCaller) CallTool(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText("ok"), nil
}

func TestConnectFakeMCP(t *testing.T) {
	bin := buildFakeServer(t)
	sc := config.ServerConfig{Command: bin}
	caller, tools, err := Connect(context.Background(), sc)
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() {
		if c, ok := caller.(interface{ Close() error }); ok {
			_ = c.Close()
		}
	}()

	// The fake server exposes one tool named "echo".
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %v, want [echo]", tools)
	}
}

// buildFakeServer compiles the testdata fake MCP server binary.
func buildFakeServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-mcp")
	cmd := exec.Command("go", "build", "-o", bin, "../../testdata/fake-mcp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake server: %v\n%s", err, out)
	}
	return bin
}
