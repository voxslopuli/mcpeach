package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/mcpeach/mcpeach/internal/client"
	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/service"
	"github.com/spf13/cobra"
)

func TestStatusString(t *testing.T) {
	tests := []struct {
		st   service.Status
		want string
	}{
		{service.StatusRunning, "running"},
		{service.StatusStopped, "stopped"},
		{service.StatusUnknown, "unknown"},
	}
	for _, tt := range tests {
		if got := statusString(tt.st); got != tt.want {
			t.Errorf("statusString(%v) = %q, want %q", tt.st, got, tt.want)
		}
	}
}

func TestServeCmd(t *testing.T) {
	cmd := serveCmd()
	if cmd == nil {
		t.Fatal("serveCmd returned nil")
	}
	if cmd.Use != "serve" {
		t.Errorf("Use = %q, want serve", cmd.Use)
	}
}

func TestTuiCmd(t *testing.T) {
	cmd := tuiCmd()
	if cmd == nil {
		t.Fatal("tuiCmd returned nil")
	}
	if cmd.Use != "tui" {
		t.Errorf("Use = %q, want tui", cmd.Use)
	}
}

func TestImportCmd(t *testing.T) {
	cmd := importCmd()
	if cmd == nil {
		t.Fatal("importCmd returned nil")
	}
	if cmd.Use != "import <file>" {
		t.Errorf("Use = %q, want import <file>", cmd.Use)
	}
}

func TestExportCmd(t *testing.T) {
	cmd := exportCmd()
	if cmd == nil {
		t.Fatal("exportCmd returned nil")
	}
	if cmd.Use != "export <file>" {
		t.Errorf("Use = %q, want export <file>", cmd.Use)
	}
}

func TestImportCmdRunE(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Write a minimal config.
	cfg := config.Default()
	if err := config.Save(config.Path(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// Write a Claude Code MCP config JSON to import.
	src := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(src, []byte(`{"mcpServers":{"github":{"command":"npx","args":["-y","@modelcontextprotocol/server-github"]}}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	cmd := importCmd()
	if err := cmd.RunE(cmd, []string{src}); err != nil {
		t.Fatalf("import RunE: %v", err)
	}

	// Verify the server was imported into the config.
	loaded, err := config.Load(config.Path())
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if _, ok := loaded.Servers["github"]; !ok {
		t.Error("import did not add github server")
	}
}

func TestExportCmdRunE(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Write a config with one server.
	cfg := config.Default()
	cfg.Servers = map[string]config.ServerConfig{
		"github": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-github"}, Enabled: true},
	}
	if err := config.Save(config.Path(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}

	dst := filepath.Join(dir, "out.json")
	cmd := exportCmd()
	if err := cmd.RunE(cmd, []string{dst}); err != nil {
		t.Fatalf("export RunE: %v", err)
	}

	// Verify the file was written and contains the server.
	b, err := os.ReadFile(dst) // nosemgrep: go_filesystem_rule-fileread — dst is a t.TempDir() path (trusted test input)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(b), "github") {
		t.Errorf("export file missing github: %s", b)
	}
}

func TestInstallCmd(t *testing.T) {
	cmd := installCmd()
	if cmd == nil {
		t.Fatal("installCmd returned nil")
	}
	if cmd.Use != "install" {
		t.Errorf("Use = %q, want install", cmd.Use)
	}
}

func TestUninstallCmd(t *testing.T) {
	cmd := uninstallCmd()
	if cmd == nil {
		t.Fatal("uninstallCmd returned nil")
	}
	if cmd.Use != "uninstall" {
		t.Errorf("Use = %q, want uninstall", cmd.Use)
	}
}

func TestStatusCmd(t *testing.T) {
	cmd := statusCmd()
	if cmd == nil {
		t.Fatal("statusCmd returned nil")
	}
	if cmd.Use != "status" {
		t.Errorf("Use = %q, want status", cmd.Use)
	}
}

func TestNewServiceManager(t *testing.T) {
	m, err := newServiceManager()
	if err != nil {
		t.Fatalf("newServiceManager: %v", err)
	}
	if m == nil {
		t.Fatal("newServiceManager returned nil")
	}
}

// fakeManager is a minimal service.Manager for testing the command RunE bodies.
type fakeManager struct {
	installed   bool
	uninstalled bool
	status      service.Status
}

func (f *fakeManager) Install() error                  { f.installed = true; return nil }
func (f *fakeManager) Uninstall() error                { f.uninstalled = true; return nil }
func (f *fakeManager) Status() (service.Status, error) { return f.status, nil }
func (f *fakeManager) Run() error                      { return nil }

func TestInstallCmdRunE(t *testing.T) {
	fm := &fakeManager{}
	serviceManagerFactory = func() (serviceManager, error) { return fm, nil }
	defer func() { serviceManagerFactory = newServiceManager }()

	cmd := installCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("install RunE: %v", err)
	}
	if !fm.installed {
		t.Error("install did not call Install")
	}
}

func TestUninstallCmdRunE(t *testing.T) {
	fm := &fakeManager{}
	serviceManagerFactory = func() (serviceManager, error) { return fm, nil }
	defer func() { serviceManagerFactory = newServiceManager }()

	cmd := uninstallCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("uninstall RunE: %v", err)
	}
	if !fm.uninstalled {
		t.Error("uninstall did not call Uninstall")
	}
}

func TestStatusCmdRunE(t *testing.T) {
	fm := &fakeManager{status: service.StatusRunning}
	serviceManagerFactory = func() (serviceManager, error) { return fm, nil }
	defer func() { serviceManagerFactory = newServiceManager }()

	cmd := statusCmd()
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("status RunE: %v", err)
	}
}

func TestInstallCmdRunEError(t *testing.T) {
	serviceManagerFactory = func() (serviceManager, error) { return nil, fmt.Errorf("boom") }
	defer func() { serviceManagerFactory = newServiceManager }()

	cmd := installCmd()
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("install RunE with factory error: want error, got nil")
	}
}

// setupTestConfig writes a config to a temp XDG dir and returns the dir.
func setupTestConfig(t *testing.T, mutate func(*config.Config)) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	cfg := config.Default()
	cfg.Gateway.Addr = "127.0.0.1:0"
	if mutate != nil {
		mutate(cfg)
	}
	if err := config.Save(config.Path(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	return dir
}

func TestRunDaemon(t *testing.T) {
	// Point XDG dirs at a temp dir so config + socket don't touch the real home.
	setupTestConfig(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// runDaemon blocks until ctx is cancelled; run it in a goroutine and
	// cancel after a short delay to verify it starts cleanly.
	errCh := make(chan error, 1)
	go func() {
		errCh <- runDaemon(ctx)
	}()

	// Give it a moment to bind, then cancel.
	time.Sleep(200 * time.Millisecond)
	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runDaemon: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runDaemon did not return after cancel")
	}
}

func TestRunDaemonNoConfig(t *testing.T) {
	// No config file → runDaemon should return a load error.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)

	err := runDaemon(context.Background())
	if err == nil {
		t.Fatal("runDaemon with no config: want error, got nil")
	}
}

func TestRunDaemonRemoteConnectFail(t *testing.T) {
	// A remote server that fails to connect should make runDaemon return an error.
	setupTestConfig(t, func(cfg *config.Config) {
		cfg.Servers = map[string]config.ServerConfig{
			"remote": {URL: "http://127.0.0.1:1/mcp", Transport: "streamable-http", Enabled: true},
		}
	})

	err := runDaemon(context.Background())
	if err == nil {
		t.Fatal("runDaemon with failing remote: want error, got nil")
	}
}

// runDaemonAndWaitForTool starts runDaemon with the given config and polls the
// control plane until the named tool appears (or the deadline passes).
func runDaemonAndWaitForTool(t *testing.T, cfg *config.Config, wantTool string) {
	t.Helper()
	// short XDG dir for the unix socket path limit
	dir := filepath.Join(os.TempDir(), "mcpeach-daemon-test")
	_ = os.RemoveAll(dir)
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	if err := config.Save(config.Path(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- runDaemon(ctx) }()
	// Poll for the tool with a bounded deadline (no fixed sleep).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c := client.NewUnix(config.SocketPath())
		tools, err := c.ListTools(context.Background())
		if err == nil {
			for _, tool := range tools {
				if tool == wantTool {
					cancel()
					select {
					case err := <-errCh:
						if err != nil {
							t.Fatalf("runDaemon: %v", err)
						}
					case <-time.After(5 * time.Second):
						t.Fatal("runDaemon did not return after cancel")
					}
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	t.Fatalf("tool %q did not appear within deadline", wantTool)
}

func TestRunDaemonRemoteServer(t *testing.T) {
	// A remote streamable-http server that connects successfully should be
	// registered and its client closed on shutdown (no leak).
	// Spin up a real streamable-http MCP server on a free port.
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

	cfg := config.Default()
	cfg.Gateway.Addr = "127.0.0.1:0"
	cfg.Servers = map[string]config.ServerConfig{
		"remote": {URL: "http://" + ln.Addr().String() + "/mcp", Transport: "streamable-http", Enabled: true},
	}
	runDaemonAndWaitForTool(t, cfg, "remote__echo")
}

// runEWithNoConfig asserts that running cmd with args against an empty XDG
// config dir returns a load error.
func runEWithNoConfig(t *testing.T, cmd *cobra.Command, args []string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if err := cmd.RunE(cmd, args); err == nil {
		t.Fatal("RunE with no config: want error, got nil")
	}
}

// runEWithFactoryError asserts that running cmd with args returns an error
// when the service manager factory fails.
func runEWithFactoryError(t *testing.T, cmd *cobra.Command, args []string) {
	t.Helper()
	serviceManagerFactory = func() (serviceManager, error) { return nil, fmt.Errorf("boom") }
	defer func() { serviceManagerFactory = newServiceManager }()
	if err := cmd.RunE(cmd, args); err == nil {
		t.Fatal("RunE with factory error: want error, got nil")
	}
}

func TestImportCmdRunEError(t *testing.T) {
	// No config file -> import RunE should return a load error.
	runEWithNoConfig(t, importCmd(), []string{filepath.Join(t.TempDir(), "mcp.json")})
}

func TestExportCmdRunEError(t *testing.T) {
	// No config file -> export RunE should return a load error.
	runEWithNoConfig(t, exportCmd(), []string{filepath.Join(t.TempDir(), "out.json")})
}

func TestUninstallCmdRunEError(t *testing.T) {
	runEWithFactoryError(t, uninstallCmd(), nil)
}

func TestStatusCmdRunEError(t *testing.T) {
	runEWithFactoryError(t, statusCmd(), nil)
}

func TestRunDaemonPopulatesGateway(t *testing.T) {
	// Build the fake MCP server binary.
	bin := filepath.Join(t.TempDir(), "fake-mcp")
	cmd := exec.Command("go", "build", "-o", bin, "../../testdata/fake-mcp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake server: %v\n%s", err, out)
	}

	// Write a config with one enabled stdio server pointing at fake-mcp.
	cfg := config.Default()
	cfg.Gateway.Addr = "127.0.0.1:0"
	cfg.Servers = map[string]config.ServerConfig{
		"fake": {Command: bin, Enabled: true},
	}
	runDaemonAndWaitForTool(t, cfg, "fake__echo")
}
