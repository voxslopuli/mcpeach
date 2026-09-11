package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/mcpeach/mcpeach/internal/client"
	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/service"
	"github.com/mcpeach/mcpeach/internal/testutil"
	"github.com/mcpeach/mcpeach/internal/tui"
	"github.com/spf13/cobra"
)

// fakeTuiRunner satisfies tuiRunner without running a real TUI.
type fakeTuiRunner struct{}

func (fakeTuiRunner) Run() (tea.Model, error) { return nil, nil }

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

func TestRootCommandLaunchesTUI(t *testing.T) {
	// Root command with no args must run the same TUI factory as tuiCmd.
	var called int
	orig := tuiProgramFactory
	tuiProgramFactory = func(m *tui.Model) tuiRunner {
		called++
		return fakeTuiRunner{}
	}
	defer func() { tuiProgramFactory = orig }()

	root := newRootCommand()
	if err := root.RunE(root, nil); err != nil {
		t.Fatalf("root RunE: %v", err)
	}
	if called != 1 {
		t.Errorf("root RunE invoked TUI factory %d times, want 1", called)
	}

	// The tui subcommand uses the same factory.
	if err := tuiCmd().RunE(tuiCmd(), nil); err != nil {
		t.Fatalf("tui RunE: %v", err)
	}
	if called != 2 {
		t.Errorf("tui RunE invoked TUI factory %d times total, want 2", called)
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

func TestStreamingServerNoWriteTimeout(t *testing.T) {
	// The MCP streamable HTTP transport serves SSE-style long-lived streams;
	// a bounded WriteTimeout would terminate healthy streams mid-flight.
	srv := newStreamingServer("127.0.0.1:0", http.NewServeMux())
	if srv.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %v, want 0 (no timeout for SSE streams)", srv.WriteTimeout)
	}
	// Slowloris and idle-connection protection must remain in place.
	if srv.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 10s", srv.ReadHeaderTimeout)
	}
	if srv.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v, want 60s", srv.IdleTimeout)
	}
}

func TestRunDaemon(t *testing.T) {
	// Point XDG dirs at a temp dir so config + socket don't touch the real home.
	setupTestConfig(t, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// runDaemon blocks until ctx is cancelled; run it in a goroutine and
	// cancel once the control socket is ready to verify it starts cleanly.
	errCh := make(chan error, 1)
	go func() {
		errCh <- runDaemon(ctx)
	}()

	// Wait for the control socket to become connectable (bounded poll, no
	// fixed sleep), then cancel.
	if err := waitForSocket(config.SocketPath(), 5*time.Second); err != nil {
		t.Fatalf("runDaemon: %v", err)
	}
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

func TestRunDaemonInvalidConfig(t *testing.T) {
	// A config that fails structural validation (a group referencing a missing
	// server) must make runDaemon return an error BEFORE any listener or
	// subprocess starts. Without the Validate() call, runDaemon would start
	// cleanly because it never inspects groups.
	setupTestConfig(t, func(cfg *config.Config) {
		cfg.Groups = map[string]config.GroupConfig{
			"g": {IncludedServers: []string{"missing"}},
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := runDaemon(ctx)
	if err == nil {
		t.Fatal("runDaemon with invalid config: want error, got nil")
	}
	if !strings.Contains(err.Error(), "validate config") {
		t.Errorf("runDaemon error = %q, want it to mention 'validate config'", err)
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

// bootDaemon writes the config to a short XDG dir and starts runDaemon in a
// goroutine, returning a cancel func and the daemon's error channel.
func bootDaemon(t *testing.T, cfg *config.Config) (context.CancelFunc, chan error) {
	t.Helper()
	// Short XDG dir for the unix socket path limit; MkdirTemp keeps the name
	// unique so parallel test runs cannot collide on the same path.
	dir, err := os.MkdirTemp(os.TempDir(), "mcpeach-daemon-test")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)
	if err := config.Save(config.Path(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- runDaemon(ctx) }()
	return cancel, errCh
}

// waitForSocket polls for the control socket to become connectable, up to
// timeout. Returns an error if it never becomes ready.
func waitForSocket(sock string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("unix", sock)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("socket %s not ready within %s", sock, timeout)
}

// waitDaemonExit asserts that runDaemon returns nil shortly after cancel.
func waitDaemonExit(t *testing.T, errCh chan error) {
	t.Helper()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("runDaemon: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("runDaemon did not return after cancel")
	}
}

// runDaemonAndWaitForTool starts runDaemon with the given config and polls the
// control plane until the named tool appears (or the deadline passes).
func runDaemonAndWaitForTool(t *testing.T, cfg *config.Config, wantTool string) {
	t.Helper()
	cancel, errCh := bootDaemon(t, cfg)
	defer cancel()
	// Poll for the tool with a bounded deadline (no fixed sleep).
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		c := client.NewUnix(config.SocketPath())
		tools, err := c.ListTools(context.Background())
		if err == nil {
			for _, tool := range tools {
				if tool == wantTool {
					cancel()
					waitDaemonExit(t, errCh)
					return
				}
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	t.Fatalf("tool %q did not appear within deadline", wantTool)
}

// runDaemonAndWaitForURL starts runDaemon with the given config and polls the
// given HTTP URL until it responds (bounded deadline, no fixed sleep). A 404
// means the handler is not mounted, so it keeps polling.
func runDaemonAndWaitForURL(t *testing.T, cfg *config.Config, url string) {
	t.Helper()
	cancel, errCh := bootDaemon(t, cfg)
	defer cancel()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusNotFound {
				continue
			}
			cancel()
			waitDaemonExit(t, errCh)
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	t.Fatalf("URL %q did not respond within deadline", url)
}

func TestRunDaemonMountsGroupEndpoints(t *testing.T) {
	// Build the fake MCP server binary.
	bin := filepath.Join(t.TempDir(), "fake-mcp")
	cmd := exec.Command("go", "build", "-o", bin, "../../testdata/fake-mcp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake server: %v\n%s", err, out)
	}

	// Pick a concrete free port so the test can reach the group endpoint.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	cfg := config.Default()
	cfg.Gateway.Addr = fmt.Sprintf("127.0.0.1:%d", port)
	cfg.Servers = map[string]config.ServerConfig{
		"fake": {Command: bin, Enabled: true},
	}
	cfg.Groups = map[string]config.GroupConfig{
		"grp": {IncludedServers: []string{"fake"}},
	}

	// Boot the daemon and poll the group MCP endpoint until it responds.
	url := fmt.Sprintf("http://127.0.0.1:%d/v0/groups/grp/mcp", port)
	runDaemonAndWaitForURL(t, cfg, url)
}

func TestRunDaemonRemoteServer(t *testing.T) {
	// A remote streamable-http server that connects successfully should be
	// registered and its client closed on shutdown (no leak).
	url := testutil.StartRemoteMCP(t)

	cfg := config.Default()
	cfg.Gateway.Addr = "127.0.0.1:0"
	cfg.Servers = map[string]config.ServerConfig{
		"remote": {URL: url + "/mcp", Transport: "streamable-http", Enabled: true},
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
		"fake":     {Command: bin, Enabled: true},
		"disabled": {Command: bin, Enabled: false},
	}
	runDaemonAndWaitForTool(t, cfg, "fake__echo")
}

// TestServeHandlesSIGTERM is an end-to-end check of the daemon's signal path:
// it spawns the real mcpeach binary, waits for the control socket, sends
// SIGTERM, and asserts the process exits promptly, removes the socket, and
// reaps the stdio MCP subprocess. It exercises the fang.WithNotifySignal
// wiring in main(), which in-process tests cannot reach.
func TestServeHandlesSIGTERM(t *testing.T) {
	if testing.Short() {
		t.Skip("spawns a real process; skipped in -short mode")
	}

	bin := buildMcpeachBinary(t)
	fakeBin := testutil.BuildFakeServer(t)
	cmd, waitCh, dump, sock := spawnServe(t, bin, fakeBin)

	// Wait for the control socket to accept connections, failing fast if the
	// daemon dies during startup.
	waitForSocketReady(t, sock, waitCh, dump, 15*time.Second)

	// The daemon connects to the stdio server before it binds the socket, so
	// the fake-mcp child is running by now. Record its PID to prove it is
	// reaped on shutdown.
	childPID := waitForChildPID(t, sock, "fake", 5*time.Second)
	if childPID == 0 {
		t.Fatalf("fake-mcp child never appeared\n%s", dump())
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("SIGTERM: %v", err)
	}

	select {
	case err := <-waitCh:
		if err != nil {
			t.Fatalf("serve exited with error after SIGTERM: %v\n%s", err, dump())
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("serve did not exit within 5s of SIGTERM\n%s", dump())
	}

	// The control-plane shutdown goroutine removes the socket on ctx.Done.
	if _, err := os.Stat(sock); !os.IsNotExist(err) {
		t.Errorf("control socket %s still present after shutdown (stat err = %v)", sock, err)
	}

	// The stdio subprocess must be reaped (gw.Close closes the client).
	if !waitForProcessGone(childPID, 5*time.Second) {
		t.Errorf("orphaned fake-mcp process %d still running after shutdown", childPID)
	}
}

// spawnServe writes a config with one fake stdio server and starts the
// mcpeach serve binary, returning the process handle, its wait channel, a
// stdout/stderr dump helper, and the control socket path.
func spawnServe(t *testing.T, bin, fakeBin string) (*exec.Cmd, chan error, func() string, string) {
	t.Helper()
	// Short XDG dir: the control socket path must stay under the ~108-byte
	// unix-socket limit, so avoid t.TempDir() (too long on macOS).
	dir := shortXDGDir(t, "mcpeach-sigterm")
	t.Setenv("XDG_CONFIG_HOME", dir)
	t.Setenv("XDG_RUNTIME_DIR", dir)

	cfg := config.Default()
	cfg.Gateway.Addr = "127.0.0.1:0"
	cfg.Servers = map[string]config.ServerConfig{
		"fake": {Command: fakeBin, Enabled: true},
	}
	if err := config.Save(config.Path(), cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	sock := config.SocketPath()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(filepath.Clean(bin), "serve") // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command, go_subproc_rule-subproc — bin is a t.TempDir() build path (trusted test input); exec.Command does not invoke a shell.
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start serve: %v", err)
	}
	dump := func() string { return "stdout:\n" + stdout.String() + "\nstderr:\n" + stderr.String() }
	waitCh := make(chan error, 1)
	go func() { waitCh <- cmd.Wait() }()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		select {
		case <-waitCh:
		default:
		}
	})
	return cmd, waitCh, dump, sock
}

// waitForSocketReady polls until the unix socket accepts connections, failing
// the test if the daemon exits first or the deadline passes.
func waitForSocketReady(t *testing.T, sock string, waitCh chan error, dump func() string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	ready := false
	for time.Now().Before(deadline) {
		if conn, err := net.Dial("unix", sock); err == nil {
			_ = conn.Close()
			ready = true
			break
		}
		select {
		case err := <-waitCh:
			t.Fatalf("serve exited before the socket was ready: %v\n%s", err, dump())
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !ready {
		t.Fatalf("control socket %s not ready within %s\n%s", sock, timeout, dump())
	}
}

// buildMcpeachBinary compiles the mcpeach CLI from the current package into a
// temp dir and returns the binary path.
func buildMcpeachBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "mcpeach")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mcpeach: %v\n%s", err, out)
	}
	return bin
}

// shortXDGDir creates a short XDG base dir under os.TempDir(). The unix socket
// path derived from it must fit the ~108-byte sockaddr_un limit, which
// t.TempDir() paths on macOS do not.
func shortXDGDir(t *testing.T, name string) string {
	t.Helper()
	// MkdirTemp keeps the name unique so parallel test runs cannot collide;
	// the short prefix keeps the unix socket path under the ~108-byte limit.
	dir, err := os.MkdirTemp(os.TempDir(), name)
	if err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// childPIDFromLogs fetches the fake-mcp child PID from the daemon's log ring
// (fake-mcp emits its PID in the startup JSON line). Returns 0 if not found.
func childPIDFromLogs(t *testing.T, sock, name string) int {
	t.Helper()
	c := client.NewUnix(sock)
	lines, err := c.ServerLogs(context.Background(), name)
	if err != nil {
		return 0
	}
	for _, line := range lines {
		var entry struct {
			PID int `json:"pid"`
		}
		if err := json.Unmarshal([]byte(line), &entry); err == nil && entry.PID > 0 {
			return entry.PID
		}
	}
	return 0
}

// processAlive reports whether a process with the given PID exists, using a
// portable signal-0 probe (no pgrep dependency).
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return p.Signal(syscall.Signal(0)) == nil
}

// waitForChildPID polls until the fake-mcp child PID appears in the daemon
// logs, returning it, or 0 on timeout.
func waitForChildPID(t *testing.T, sock, name string, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if pid := childPIDFromLogs(t, sock, name); pid > 0 {
			return pid
		}
		if time.Now().After(deadline) {
			return 0
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// waitForProcessGone polls until the process with pid no longer exists,
// returning true when it is gone.
func waitForProcessGone(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for {
		if !processAlive(pid) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestExecuteRoot(t *testing.T) {
	// executeRoot wires SIGINT/SIGTERM cancellation into fang.Execute. With
	// `--help` it prints help and returns nil without launching the TUI (the
	// bare-command path now runs the TUI, which needs a TTY); the point is
	// exercising the signal-wiring line in-process (the E2E SIGTERM test
	// covers the actual signal path via a spawned binary).
	root := newRootCommand()
	root.SetArgs([]string{"--help"})
	if err := executeRoot(root); err != nil {
		t.Fatalf("executeRoot: %v", err)
	}
}
