package control

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/gateway"
	"github.com/mcpeach/mcpeach/internal/secrets"
	"github.com/mcpeach/mcpeach/internal/server"
)

// newTestHandler builds a handler backed by a fresh manager + gateway.
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	return newHandlerWithConfig(t, &config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Enabled: true},
		},
	})
}

// newHandlerWithConfig builds a handler with a fresh manager + gateway and the
// given config.
func newHandlerWithConfig(t *testing.T, cfg *config.Config) http.Handler {
	t.Helper()
	mgr := server.NewManager()
	gw := gateway.New(cfg)
	return NewHandler(mgr, gw, cfg)
}

func TestListServers(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/servers", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp ListServersResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Servers) != 1 || resp.Servers[0].Name != "a" {
		t.Errorf("servers = %+v, want [a]", resp.Servers)
	}
}

func TestListTools(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/tools", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp ListToolsResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Tools) != 0 {
		t.Errorf("tools = %d, want 0", len(resp.Tools))
	}
}

func TestMetrics(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp["servers"]; !ok {
		t.Errorf("metrics response missing 'servers' key: %v", resp)
	}
}

func TestLogs(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/logs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// An empty log buffer is valid; just verify the endpoint responds.
	_ = resp.Lines
}

func TestServerLogs(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"a": {Command: "echo", Enabled: true},
		},
	}
	mgr := server.NewManager()
	mgr.Add(server.New("a"))
	h := NewHandler(mgr, gateway.New(cfg), cfg)
	req := httptest.NewRequest(http.MethodGet, "/v0/servers/a/logs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Lines []string `json:"lines"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	_ = resp.Lines
}

func TestServerLogsUnknown(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/servers/nope/logs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestServerLogsNilManager(t *testing.T) {
	gw := gateway.New(&config.Config{})
	h := NewHandler(nil, gw, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/v0/servers/a/logs", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (nil manager)", rec.Code)
	}
}

func TestAddServer(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{}}
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)

	body := `{"name":"new","command":"echo","args":["hi"]}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if _, ok := cfg.Servers["new"]; !ok {
		t.Error("server 'new' not added to config")
	}
}

func TestAddServerDuplicate(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{"a": {}}}
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)

	body := `{"name":"a","command":"echo"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (duplicate)", rec.Code)
	}
}

func TestAddServerNoName(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{}}
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)

	body := `{"command":"echo"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (no name)", rec.Code)
	}
}

func TestAddServerNilConfig(t *testing.T) {
	h := NewHandler(server.NewManager(), gateway.New(&config.Config{}), nil)
	body := `{"name":"a","command":"echo"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (nil config)", rec.Code)
	}
}

func TestProcesses(t *testing.T) {
	mgr := server.NewManager()
	gw := gateway.New(&config.Config{})
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"echo": {Command: "echo", Enabled: true},
		},
	}
	h := NewHandler(mgr, gw, cfg)

	// Start the server so it has a PID, then query /v0/processes.
	srv := server.New("echo")
	mgr.Add(srv)
	if err := mgr.Start(context.Background(), "echo", "echo", nil, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = mgr.Stop("echo") }()

	req := httptest.NewRequest(http.MethodGet, "/v0/processes", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var resp struct {
		Processes map[string]any `json:"processes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := resp.Processes["echo"]; !ok {
		t.Errorf("processes = %v, want echo entry", resp.Processes)
	}
}

func TestProcessesNilManager(t *testing.T) {
	gw := gateway.New(&config.Config{})
	h := NewHandler(nil, gw, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/v0/processes", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (nil manager)", rec.Code)
	}
}

func TestStartStopServer(t *testing.T) {
	bin := buildFakeServer(t)
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"fake": {Command: bin, Enabled: true},
		},
	}
	mgr := server.NewManager()
	srv := server.New("fake")
	mgr.Add(srv)
	h := NewHandler(mgr, gateway.New(cfg), cfg)

	// Start.
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/fake/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	// Stop.
	req = httptest.NewRequest(http.MethodPost, "/v0/servers/fake/stop", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want 200", rec.Code)
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

func TestStartUnknownServer(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/nope/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestStartServerNoCommand(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"remote": {URL: "http://x", Enabled: true},
		},
	}
	h := newHandlerWithConfig(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/remote/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (no command)", rec.Code)
	}
}

func TestStopUnknownServer(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/nope/stop", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestStopNotRunning(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"echo": {Command: "echo", Enabled: true},
		},
	}
	h := newHandlerWithConfig(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/echo/stop", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (not running)", rec.Code)
	}
}

func TestStartServerFailsToStart(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"bad": {Command: "/nonexistent/binary", Enabled: true},
		},
	}
	h := newHandlerWithConfig(t, cfg)
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/bad/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (start failed)", rec.Code)
	}
}

func TestStartServerNilManager(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"echo": {Command: "echo", Enabled: true},
		},
	}
	gw := gateway.New(cfg)
	h := NewHandler(nil, gw, cfg)
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/echo/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (nil manager)", rec.Code)
	}
}

func TestResolveEnv(t *testing.T) {
	h := &Handler{res: secrets.NewResolver(nil)}
	t.Setenv("MY_TOKEN", "s3cret")
	got, err := h.resolveEnv(map[string]string{"TOKEN": "env:MY_TOKEN", "PLAIN": "x"})
	if err != nil {
		t.Fatalf("resolveEnv: %v", err)
	}
	seen := map[string]bool{}
	for _, e := range got {
		seen[e] = true
	}
	if !seen["TOKEN=s3cret"] {
		t.Errorf("resolveEnv = %v, want TOKEN=s3cret", got)
	}
	if !seen["PLAIN=x"] {
		t.Errorf("resolveEnv = %v, want PLAIN=x", got)
	}
}

func TestResolveEnvUnset(t *testing.T) {
	h := &Handler{res: secrets.NewResolver(nil)}
	if _, err := h.resolveEnv(map[string]string{"TOKEN": "env:DOES_NOT_EXIST_XYZ"}); err == nil {
		t.Fatal("resolveEnv unset: want error, got nil")
	}
}

func TestSocketPath(t *testing.T) {
	s := NewServer("/tmp/test.sock", nil)
	if got := s.SocketPath(); got != "/tmp/test.sock" {
		t.Errorf("SocketPath = %q, want /tmp/test.sock", got)
	}
}

func TestUnknownRoute(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/v0/bogus", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
}

func TestServeUnixSocket(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "mcpeach.sock")

	mgr := server.NewManager()
	gw := gateway.New(&config.Config{})
	srv := NewServer(sock, NewHandler(mgr, gw, &config.Config{}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Connect over the unix socket.
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return net.Dial("unix", sock)
			},
		},
	}
	// "http://unix" is a unix-socket transport placeholder, not a real HTTP
	// URL; no TLS is involved.
	resp, err := client.Get("http://unix/v0/servers") // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.http-request.http-request
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
