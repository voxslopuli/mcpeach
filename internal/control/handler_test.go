package control

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/gateway"
	"github.com/mcpeach/mcpeach/internal/secrets"
	"github.com/mcpeach/mcpeach/internal/server"
	"github.com/mcpeach/mcpeach/internal/testutil"
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

// newFakeServerHandler builds a handler backed by one enabled fake MCP server.
// It returns the handler plus the manager, gateway, and config so tests can
// inspect or mutate them.
func newFakeServerHandler(t *testing.T) (*Handler, *server.Manager, *gateway.Gateway, *config.Config) {
	t.Helper()
	bin := buildFakeServer(t)
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"fake": {Command: bin, Enabled: true},
		},
	}
	mgr := server.NewManager()
	mgr.Add(server.New("fake"))
	gw := gateway.New(cfg)
	return NewHandler(mgr, gw, cfg), mgr, gw, cfg
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

func TestMetricsNilGateway(t *testing.T) {
	h := NewHandler(server.NewManager(), nil, &config.Config{})
	req := httptest.NewRequest(http.MethodGet, "/v0/metrics", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (nil gateway)", rec.Code)
	}
}

func TestSetSyncTools(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"fake": {Command: buildFakeServer(t), Enabled: true},
		},
	}
	mgr := server.NewManager()
	mgr.Add(server.New("fake"))
	h := NewHandler(mgr, gateway.New(cfg), cfg)
	called := false
	h.SetSyncTools(func() { called = true })
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/fake/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if !called {
		t.Error("SetSyncTools callback not invoked on start")
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
	cfg := config.Default()
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

func TestAddServerInvalidBody(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{}}
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	body := `{not json`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (invalid body)", rec.Code)
	}
}

func TestAddServerNameDoubleUnderscore(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{}}
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	body := `{"name":"a__b","command":"echo"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (name contains '__')", rec.Code)
	}
}

func TestAddServerBothCommandAndURL(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{}}
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	body := `{"name":"a","command":"echo","url":"http://x"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (both command and url)", rec.Code)
	}
}

func TestAddServerNeitherCommandNorURL(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{}}
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	body := `{"name":"a"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (neither command nor url)", rec.Code)
	}
}

func TestAddServerOversizedBody(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{}}
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	// Body larger than the 1 MiB MaxBytesReader limit.
	body := `{"name":"a","command":"echo","args":["` + strings.Repeat("x", 1<<20+1) + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (oversized body)", rec.Code)
	}
}

func TestAddServerSaveFailLeavesConfigUnchanged(t *testing.T) {
	cfg := config.Default()
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	// Point configPath at a path where a file blocks the parent dir creation.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	h.configPath = filepath.Join(blocker, "mcpeach.yml")
	body := `{"name":"a","command":"echo"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (save fail)", rec.Code)
	}
	// The in-memory config must NOT contain the new server: a failed save
	// must leave the active daemon state unchanged.
	if _, ok := cfg.Servers["a"]; ok {
		t.Error("server 'a' present in in-memory config after failed save (nontransactional)")
	}
}

func TestAddServerInvalidTransport(t *testing.T) {
	cfg := config.Default()
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	body := `{"name":"a","url":"http://x/mcp","transport":"bogus"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (invalid transport)", rec.Code)
	}
	if _, ok := cfg.Servers["a"]; ok {
		t.Error("server 'a' present in in-memory config after rejected add")
	}
}

func TestAddServerSaveFail(t *testing.T) {
	cfg := config.Default()
	h := NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	// Point configPath at a path where a file blocks the parent dir creation.
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	h.configPath = filepath.Join(blocker, "mcpeach.yml")
	body := `{"name":"a","command":"echo"}`
	req := httptest.NewRequest(http.MethodPost, "/v0/servers", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500 (save fail)", rec.Code)
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

func TestProcessesCollectError(t *testing.T) {
	// A server with a PID that processinfo.Collect fails on (nonexistent PID)
	// should be skipped, not error the whole response.
	mgr := server.NewManager()
	srv := server.New("ghost")
	mgr.Add(srv)
	// Force a bogus PID by starting a process then killing it.
	bin := buildFakeServer(t)
	if err := mgr.Start(context.Background(), "ghost", bin, nil, nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	_ = mgr.Stop("ghost")

	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"ghost": {Command: bin, Enabled: true},
		},
	}
	h := NewHandler(mgr, gateway.New(cfg), cfg)
	req := httptest.NewRequest(http.MethodGet, "/v0/processes", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
}

func TestStopServerRemovesTools(t *testing.T) {
	h, _, gw, _ := newFakeServerHandler(t)

	// Start: the fake server's tool should be registered.
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/fake/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if len(gw.Tools()) != 1 {
		t.Fatalf("after start Tools() = %d, want 1", len(gw.Tools()))
	}

	// Stop: the tool must be removed from the gateway.
	req = httptest.NewRequest(http.MethodPost, "/v0/servers/fake/stop", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if len(gw.Tools()) != 0 {
		t.Errorf("after stop Tools() = %d, want 0 (stale tools remain)", len(gw.Tools()))
	}
}

func TestStartServerReplacementFailure(t *testing.T) {
	h, _, gw, cfg := newFakeServerHandler(t)

	// Start a healthy server.
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/fake/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if len(gw.Tools()) != 1 {
		t.Fatalf("after start Tools() = %d, want 1", len(gw.Tools()))
	}

	// Attempt a replacement whose connect fails (bad command). The original
	// client and tool must survive.
	cfg.Servers["fake"] = config.ServerConfig{Command: "/nonexistent/binary", Enabled: true}
	req = httptest.NewRequest(http.MethodPost, "/v0/servers/fake/start", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("replacement status = %d, want 500 (body %s)", rec.Code, rec.Body.String())
	}
	if len(gw.Tools()) != 1 {
		t.Errorf("after failed replacement Tools() = %d, want 1 (original destroyed)", len(gw.Tools()))
	}
	// The original client must still route calls (not destroyed).
	res, err := gw.CallTool(context.Background(), mcp.CallToolRequest{
		Params: mcp.CallToolParams{Name: "fake__echo", Arguments: map[string]any{"text": "hi"}},
	})
	if err != nil {
		t.Errorf("CallTool after failed replacement: %v (original client destroyed)", err)
	} else if res == nil {
		t.Error("CallTool after failed replacement returned nil result")
	}
}

func TestStartServerReplacementSuccess(t *testing.T) {
	h, _, gw, _ := newFakeServerHandler(t)

	// Start once.
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/fake/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	// Start again (replacement). Must still work with no client leak.
	req = httptest.NewRequest(http.MethodPost, "/v0/servers/fake/start", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("replacement status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if len(gw.Tools()) != 1 {
		t.Errorf("after replacement Tools() = %d, want 1", len(gw.Tools()))
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
	return testutil.BuildFakeServer(t)
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

func TestServerStartSetsTimeouts(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "mcpeach.sock")
	srv := NewServer(sock, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if srv.httpSrv == nil {
		t.Fatal("httpSrv not initialized")
	}
	if srv.httpSrv.ReadHeaderTimeout != 10*time.Second {
		t.Errorf("ReadHeaderTimeout = %v, want 10s", srv.httpSrv.ReadHeaderTimeout)
	}
	if srv.httpSrv.ReadTimeout != 30*time.Second {
		t.Errorf("ReadTimeout = %v, want 30s", srv.httpSrv.ReadTimeout)
	}
	if srv.httpSrv.WriteTimeout != 30*time.Second {
		t.Errorf("WriteTimeout = %v, want 30s", srv.httpSrv.WriteTimeout)
	}
	if srv.httpSrv.IdleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout = %v, want 60s", srv.httpSrv.IdleTimeout)
	}
}
