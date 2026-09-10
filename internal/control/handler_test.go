package control

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/gateway"
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

func TestStartStopServer(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"echo": {Command: "echo", Enabled: true},
		},
	}
	mgr := server.NewManager()
	srv := server.New("echo")
	mgr.Add(srv)
	h := NewHandler(mgr, gateway.New(cfg), cfg)

	// Start.
	req := httptest.NewRequest(http.MethodPost, "/v0/servers/echo/start", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if srv.State() != server.Running {
		t.Fatalf("state = %s, want running", srv.State())
	}

	// Stop.
	req = httptest.NewRequest(http.MethodPost, "/v0/servers/echo/stop", nil)
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stop status = %d, want 200", rec.Code)
	}
	if srv.State() != server.Stopped {
		t.Fatalf("state = %s, want stopped", srv.State())
	}
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

func TestEnvSlice(t *testing.T) {
	got := envSlice(map[string]string{"A": "1", "B": "2"})
	if len(got) != 2 {
		t.Fatalf("envSlice = %v, want 2 entries", got)
	}
	seen := map[string]bool{}
	for _, e := range got {
		seen[e] = true
	}
	if !seen["A=1"] || !seen["B=2"] {
		t.Errorf("envSlice = %v, want A=1 and B=2", got)
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
	// nosemgrep: http-request — "http://unix" is a unix-socket transport
	// placeholder, not a real HTTP URL; no TLS is involved.
	resp, err := client.Get("http://unix/v0/servers")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
