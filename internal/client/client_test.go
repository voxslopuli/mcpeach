package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/control"
	"github.com/mcpeach/mcpeach/internal/gateway"
	"github.com/mcpeach/mcpeach/internal/server"
)

// newTestClient builds a client pointed at an in-memory handler.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	h := control.NewHandler(nil, nil, nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func TestListServers(t *testing.T) {
	c := newTestClient(t)
	servers, err := c.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if servers == nil {
		t.Fatal("ListServers returned nil")
	}
}

func TestListTools(t *testing.T) {
	c := newTestClient(t)
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if tools == nil {
		t.Fatal("ListTools returned nil")
	}
}

func TestServerLogs(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{"a": {}}}
	mgr := server.NewManager()
	mgr.Add(server.New("a"))
	h := control.NewHandler(mgr, gateway.New(cfg), cfg)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	lines, err := c.ServerLogs(context.Background(), "a")
	if err != nil {
		t.Fatalf("ServerLogs: %v", err)
	}
	if lines == nil {
		t.Fatal("ServerLogs returned nil")
	}
}

func TestServerLogsUnknown(t *testing.T) {
	c := newTestClient(t)
	_, err := c.ServerLogs(context.Background(), "nope")
	if err == nil {
		t.Fatal("ServerLogs unknown: want error, got nil")
	}
}

func TestStartServer(t *testing.T) {
	c := newTestClient(t)
	err := c.StartServer(context.Background(), "echo")
	if err == nil {
		t.Fatal("StartServer unknown: want error, got nil")
	}
}

func TestStopServer(t *testing.T) {
	c := newTestClient(t)
	err := c.StopServer(context.Background(), "echo")
	if err == nil {
		t.Fatal("StopServer unknown: want error, got nil")
	}
}

func TestAddServer(t *testing.T) {
	cfg := config.Default()
	h := control.NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	err := c.AddServer(context.Background(), AddServerRequest{Name: "new", Command: "echo", Args: []string{"hi"}})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	// The handler publishes an immutable config snapshot; verify the new
	// server is visible through the API rather than the stale pointer.
	servers, err := c.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	found := false
	for _, s := range servers {
		if s.Name == "new" {
			found = true
		}
	}
	if !found {
		t.Error("server 'new' not listed after add")
	}
}

func TestAddServerDuplicate(t *testing.T) {
	cfg := &config.Config{Servers: map[string]config.ServerConfig{"a": {}}}
	h := control.NewHandler(server.NewManager(), gateway.New(cfg), cfg)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	err := c.AddServer(context.Background(), AddServerRequest{Name: "a", Command: "echo"})
	if err == nil {
		t.Fatal("AddServer duplicate: want error, got nil")
	}
}

func TestAddServerFull(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"name": "remote"})
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	err := c.AddServer(context.Background(), AddServerRequest{
		Name:      "remote",
		URL:       "https://mcp.example.com/mcp",
		Transport: "sse",
	})
	if err != nil {
		t.Fatalf("AddServer: %v", err)
	}
	if got["name"] != "remote" {
		t.Errorf("body name = %v, want remote", got["name"])
	}
	if got["url"] != "https://mcp.example.com/mcp" {
		t.Errorf("body url = %v, want https://mcp.example.com/mcp", got["url"])
	}
	if got["transport"] != "sse" {
		t.Errorf("body transport = %v, want sse", got["transport"])
	}
}

func TestClientError(t *testing.T) {
	// Point at a server that returns 404 for everything.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "nope"})
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)
	if _, err := c.ListServers(context.Background()); err == nil {
		t.Fatal("ListServers on 404: want error, got nil")
	}
}

func TestNewUnix(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "mcpeach.sock")

	// Start a control server on the unix socket.
	mgr := server.NewManager()
	gw := gateway.New(&config.Config{})
	ctrl := control.NewServer(sock, control.NewHandler(mgr, gw, &config.Config{}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := ctrl.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// The client should talk to the unix socket.
	c := NewUnix(sock)
	servers, err := c.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers over unix socket: %v", err)
	}
	if servers == nil {
		t.Fatal("ListServers returned nil")
	}
}

func TestNewUnixContextCancelled(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "mcpeach.sock")
	c := NewUnix(sock)

	// Cancel before dialing: the connect phase must honor the request context
	// and fail immediately rather than hanging.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		_, err := c.ListServers(ctx)
		done <- err
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("ListServers with cancelled context: want error, got nil")
		}
		if !errors.Is(err, context.Canceled) {
			t.Errorf("error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("ListServers with cancelled context hung")
	}
}

func TestGetServerDetail(t *testing.T) {
	cfg := &config.Config{
		Servers: map[string]config.ServerConfig{
			"github": {
				Command: "npx",
				Args:    []string{"-y", "@modelcontextprotocol/server-github"},
				Env:     map[string]string{"TOKEN": "env:GITHUB_TOKEN"},
				Enabled: true,
			},
		},
	}
	mgr := server.NewManager()
	mgr.Add(server.New("github"))
	h := control.NewHandler(mgr, gateway.New(cfg), cfg)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c := New(srv.URL)

	d, err := c.GetServer(context.Background(), "github")
	if err != nil {
		t.Fatalf("GetServer: %v", err)
	}
	if d.Name != "github" {
		t.Errorf("name = %q, want github", d.Name)
	}
	if d.Command != "npx" {
		t.Errorf("command = %q, want npx", d.Command)
	}
	if len(d.Args) != 2 || d.Args[1] != "@modelcontextprotocol/server-github" {
		t.Errorf("args = %v, want round-tripped args", d.Args)
	}
	if d.Env["TOKEN"] != "env:GITHUB_TOKEN" {
		t.Errorf("env = %v, want source reference env:GITHUB_TOKEN", d.Env)
	}
	if d.Enabled != true {
		t.Error("enabled = false, want true")
	}
}

func TestGetServerDetailUnknown(t *testing.T) {
	c := newTestClient(t)
	if _, err := c.GetServer(context.Background(), "nope"); err == nil {
		t.Fatal("GetServer unknown: want error, got nil")
	}
}
