// Package control implements the daemon's control-plane HTTP API over a unix
// socket. The TUI (and future web client) talks to this API.
package control

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/connect"
	"github.com/mcpeach/mcpeach/internal/gateway"
	"github.com/mcpeach/mcpeach/internal/obs"
	"github.com/mcpeach/mcpeach/internal/processinfo"
	"github.com/mcpeach/mcpeach/internal/secrets"
	"github.com/mcpeach/mcpeach/internal/server"
)

// ServerInfo is a server's state as exposed by the API.
type ServerInfo struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

// ListServersResponse is the /v0/servers response.
type ListServersResponse struct {
	Servers []ServerInfo `json:"servers"`
}

// ListToolsResponse is the /v0/tools response.
type ListToolsResponse struct {
	Tools []string `json:"tools"`
}

// AddServerRequest is the POST /v0/servers request body.
type AddServerRequest struct {
	Name      string            `json:"name"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Transport string            `json:"transport,omitempty"`
}

// Handler serves the control-plane API.
type Handler struct {
	mu         sync.RWMutex
	mgr        *server.Manager
	gw         *gateway.Gateway
	cfg        *config.Config
	configPath string
	log        *obs.Logger
	res        *secrets.Resolver
	syncTools  func()
	mux        *http.ServeMux
}

// SetSyncTools registers a callback invoked after the gateway topology changes
// (e.g. a server is started) so the streaming server re-exposes new tools.
func (h *Handler) SetSyncTools(fn func()) {
	h.syncTools = fn
}

// ServeHTTP implements http.Handler, dispatching to the control-plane routes.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

// NewHandler builds a control-plane handler. The config is saved to
// config.Path() when servers are added.
func NewHandler(mgr *server.Manager, gw *gateway.Gateway, cfg *config.Config) *Handler {
	h := &Handler{mgr: mgr, gw: gw, cfg: cfg, configPath: config.Path(), log: obs.Default().With("pkg", "control"), res: secrets.NewResolver(secrets.NewKeyringStore())}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v0/servers", h.listServers)
	mux.HandleFunc("POST /v0/servers", h.addServer)
	mux.HandleFunc("GET /v0/tools", h.listTools)
	mux.HandleFunc("GET /v0/metrics", h.metrics)
	mux.HandleFunc("GET /v0/logs", h.logs)
	mux.HandleFunc("GET /v0/processes", h.processes)
	mux.HandleFunc("GET /v0/servers/{name}/logs", h.serverLogs)
	mux.HandleFunc("POST /v0/servers/{name}/start", h.startServer)
	mux.HandleFunc("POST /v0/servers/{name}/stop", h.stopServer)
	h.mux = mux
	return h
}

func (h *Handler) listServers(w http.ResponseWriter, r *http.Request) {
	// Report the actual operational state from the manager where known.
	resp := ListServersResponse{Servers: []ServerInfo{}}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.cfg != nil {
		for name := range h.cfg.Servers {
			state := "stopped"
			if h.mgr != nil {
				if s := h.mgr.Server(name); s != nil {
					state = s.State().String()
				}
			}
			resp.Servers = append(resp.Servers, ServerInfo{Name: name, State: state})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// addServer adds a new server to the config and persists it.
func (h *Handler) addServer(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.cfg == nil {
		writeError(w, http.StatusInternalServerError, "config not available")
		return
	}
	// Bound the request body to prevent memory exhaustion on the control socket.
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req AddServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if strings.Contains(req.Name, "__") {
		writeError(w, http.StatusBadRequest, "name cannot contain '__' (reserved for tool canonicalization)")
		return
	}
	if req.Command != "" && req.URL != "" {
		writeError(w, http.StatusBadRequest, "cannot set both command and url")
		return
	}
	if req.Command == "" && req.URL == "" {
		writeError(w, http.StatusBadRequest, "must set command or url")
		return
	}
	if _, exists := h.cfg.Servers[req.Name]; exists {
		writeError(w, http.StatusConflict, "server already exists")
		return
	}
	// Build a candidate copy so a failed save leaves the active config
	// untouched. The explicit map rebuild keeps mutating candidate.Servers
	// from affecting h.cfg.Servers.
	candidate := *h.cfg
	candidate.Servers = make(map[string]config.ServerConfig, len(h.cfg.Servers)+1)
	for k, v := range h.cfg.Servers {
		candidate.Servers[k] = v
	}
	candidate.Servers[req.Name] = config.ServerConfig{
		Command:   req.Command,
		Args:      req.Args,
		Env:       req.Env,
		URL:       req.URL,
		Transport: req.Transport,
		Enabled:   true,
	}
	if err := candidate.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := config.Save(h.configPath, &candidate); err != nil {
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return
	}
	// Publish only after save succeeds.
	h.cfg.Servers = candidate.Servers
	writeJSON(w, http.StatusOK, map[string]any{"name": req.Name})
}

func (h *Handler) listTools(w http.ResponseWriter, r *http.Request) {
	names := []string{}
	if h.gw != nil {
		tools := h.gw.Tools()
		names = make([]string, 0, len(tools))
		for _, t := range tools {
			names = append(names, t.Name)
		}
	}
	writeJSON(w, http.StatusOK, ListToolsResponse{Tools: names})
}

// metrics exposes the gateway's tool-call metrics.
func (h *Handler) metrics(w http.ResponseWriter, r *http.Request) {
	if h.gw == nil {
		writeError(w, http.StatusInternalServerError, "gateway not available")
		return
	}
	writeJSON(w, http.StatusOK, h.gw.Metrics())
}

// logs exposes the app-wide structured log ring buffer.
func (h *Handler) logs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"lines": h.log.Lines()})
}

// processes exposes per-server process resource usage (CPU, RAM, ports).
func (h *Handler) processes(w http.ResponseWriter, r *http.Request) {
	if h.mgr == nil {
		writeError(w, http.StatusInternalServerError, "manager not available")
		return
	}
	out := map[string]processinfo.Info{}
	h.mu.RLock()
	defer h.mu.RUnlock()
	for name := range h.cfg.Servers {
		pid := h.mgr.PID(name)
		if pid == 0 {
			continue
		}
		info, err := processinfo.Collect(r.Context(), pid)
		if err != nil {
			h.log.Warn("process collect failed", "server", name, "pid", pid, "err", err)
			continue
		}
		out[name] = info
	}
	writeJSON(w, http.StatusOK, map[string]any{"processes": out})
}

// serverLogs exposes the captured log lines for a single server.
func (h *Handler) serverLogs(w http.ResponseWriter, r *http.Request) {
	if h.mgr == nil {
		writeError(w, http.StatusInternalServerError, "manager not available")
		return
	}
	name := r.PathValue("name")
	if h.mgr.Server(name) == nil {
		writeError(w, http.StatusNotFound, "unknown server")
		return
	}
	lines := h.mgr.Logs(name)
	if lines == nil {
		lines = []string{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"lines": lines})
}

func (h *Handler) startServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var sc config.ServerConfig
	h.mu.RLock()
	if h.cfg != nil {
		sc = h.cfg.Servers[name]
	}
	h.mu.RUnlock()
	if sc.Command == "" {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown server %q", name))
		return
	}
	// Ensure the server is registered with the manager.
	if h.mgr == nil {
		writeError(w, http.StatusInternalServerError, "manager not available")
		return
	}
	if h.mgr.Server(name) == nil {
		h.mgr.Add(server.New(name))
	}
	env, err := h.resolveEnv(sc.Env)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Connect the replacement FIRST so a failed connect leaves the existing
	// client untouched. The caller owns closing the returned client.
	caller, tools, err := connect.Connect(r.Context(), sc, env)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Cleanup guard: close the new client exactly once on any post-connect
	// failure, so a partial swap does not leak it.
	closed := false
	closeNew := func() {
		if !closed {
			closed = true
			if closer, ok := caller.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		}
	}
	defer closeNew()

	if h.gw != nil {
		// Atomically swap: remove the old client + old tools, then register the
		// new client + new tools.
		if old := h.gw.RemoveServer(name); old != nil {
			if closer, ok := old.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		}
		h.gw.RegisterClient(name, caller)
		for _, t := range tools {
			h.gw.RegisterTool(name, t)
		}
	}
	if h.mgr != nil {
		// On replacement the server is already running; tolerate that as a
		// no-op so restarting a running server succeeds.
		if err := h.mgr.MarkRunning(name); err != nil && !strings.Contains(err.Error(), "invalid transition") {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	// The swap succeeded; the new client is now owned by the gateway.
	closed = true
	if h.syncTools != nil {
		h.syncTools()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// resolveEnv resolves any env:/keychain: references in the server's env vars.
func (h *Handler) resolveEnv(env map[string]string) ([]string, error) {
	return h.res.ResolveEnv(env)
}

func (h *Handler) stopServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	h.mu.RLock()
	if h.cfg != nil {
		if _, ok := h.cfg.Servers[name]; !ok {
			h.mu.RUnlock()
			writeError(w, http.StatusNotFound, fmt.Sprintf("unknown server %q", name))
			return
		}
	}
	h.mu.RUnlock()
	if h.mgr == nil {
		writeError(w, http.StatusInternalServerError, "manager not available")
		return
	}
	if h.gw != nil {
		// Atomically remove the client and its tools, then close the client.
		if c := h.gw.RemoveServer(name); c != nil {
			if closer, ok := c.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		}
	}
	if h.mgr != nil {
		if err := h.mgr.MarkStopped(name); err != nil && !strings.Contains(err.Error(), "not running") {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if h.syncTools != nil {
		h.syncTools()
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// Server serves the control-plane API over a unix socket.
type Server struct {
	sock    string
	handler http.Handler
	httpSrv *http.Server
}

// NewServer builds a unix-socket server.
func NewServer(sock string, handler http.Handler) *Server {
	return &Server{sock: sock, handler: handler}
}

// Start binds the unix socket and serves in a background goroutine until ctx
// is cancelled. It returns once the listener is bound.
func (s *Server) Start(ctx context.Context) error {
	if err := os.MkdirAll(filepath.Dir(s.sock), 0o755); err != nil {
		return err
	}
	// Remove a stale socket file if present.
	_ = os.Remove(s.sock)
	ln, err := net.Listen("unix", s.sock)
	if err != nil {
		return err
	}
	// Restrict the socket to the owner: it can start/stop processes.
	if err := os.Chmod(s.sock, 0o600); err != nil {
		_ = ln.Close()
		return err
	}
	s.httpSrv = &http.Server{
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	go func() {
		<-ctx.Done()
		_ = s.httpSrv.Close()
		_ = os.Remove(s.sock)
	}()
	go func() { _ = s.httpSrv.Serve(ln) }()
	return nil
}

// SocketPath returns the unix socket path.
func (s *Server) SocketPath() string { return s.sock }
