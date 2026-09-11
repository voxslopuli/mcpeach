// Package control implements the daemon's control-plane HTTP API over a unix
// socket. The TUI (and future web client) talks to this API.
package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

// ServerDetail is the GET /v0/servers/{name} response: the full server
// configuration plus its runtime state and tool count. Env values are the
// SOURCE REFERENCES from config (e.g. "keychain:...", "env:..."), never
// resolved secrets.
type ServerDetail struct {
	Name      string            `json:"name"`
	State     string            `json:"state"`
	Transport string            `json:"transport,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	URL       string            `json:"url,omitempty"`
	Enabled   bool              `json:"enabled"`
	Env       map[string]string `json:"env,omitempty"`
	ToolCount int               `json:"tool_count"`
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
	Enabled   bool              `json:"enabled,omitempty"`
}

// Handler serves the control-plane API.
type Handler struct {
	mu         sync.RWMutex
	mgr        *server.Manager
	gw         *gateway.Gateway
	cfg        atomic.Pointer[config.Config] // immutable snapshot; swapped atomically
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
	h := &Handler{mgr: mgr, gw: gw, configPath: config.Path(), log: obs.Default().With("pkg", "control"), res: secrets.NewResolver(secrets.NewKeyringStore())}
	h.cfg.Store(cfg)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v0/servers", h.listServers)
	mux.HandleFunc("POST /v0/servers", h.addServer)
	mux.HandleFunc("GET /v0/tools", h.listTools)
	mux.HandleFunc("GET /v0/metrics", h.metrics)
	mux.HandleFunc("GET /v0/logs", h.logs)
	mux.HandleFunc("GET /v0/processes", h.processes)
	mux.HandleFunc("GET /v0/servers/{name}", h.getServer)
	mux.HandleFunc("PUT /v0/servers/{name}", h.updateServer)
	mux.HandleFunc("DELETE /v0/servers/{name}", h.deleteServer)
	mux.HandleFunc("GET /v0/servers/{name}/logs", h.serverLogs)
	mux.HandleFunc("POST /v0/servers/{name}/start", h.startServer)
	mux.HandleFunc("POST /v0/servers/{name}/stop", h.stopServer)
	mux.HandleFunc("GET /v0/secrets", h.listSecrets)
	mux.HandleFunc("POST /v0/secrets/{server}/{variable}", h.storeSecret)
	mux.HandleFunc("DELETE /v0/secrets/{server}/{variable}", h.deleteSecret)
	h.mux = mux
	return h
}

func (h *Handler) listServers(w http.ResponseWriter, r *http.Request) {
	// Report the actual operational state from the manager where known.
	resp := ListServersResponse{Servers: []ServerInfo{}}
	cfg := h.cfg.Load()
	if cfg != nil {
		for name := range cfg.Servers {
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

// getServer returns the full detail for one server: config (with env as
// source references), runtime state, and the count of its registered tools.
func (h *Handler) getServer(w http.ResponseWriter, r *http.Request) {
	// The config is an atomic snapshot; no lock needed for this read-only path.
	cfg := h.cfg.Load()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "config not available")
		return
	}
	name := r.PathValue("name")
	sc, ok := cfg.Servers[name]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown server %q", name))
		return
	}
	state := "stopped"
	if h.mgr != nil {
		if s := h.mgr.Server(name); s != nil {
			state = s.State().String()
		}
	}
	transport := sc.Transport
	if transport == "" && sc.Command != "" {
		transport = "stdio"
	}
	toolCount := 0
	if h.gw != nil {
		for _, t := range h.gw.Tools() {
			if srv, _, ok := config.SplitCanonical(t.Name); ok && srv == name {
				toolCount++
			}
		}
	}
	writeJSON(w, http.StatusOK, ServerDetail{
		Name:      name,
		State:     state,
		Transport: transport,
		Command:   sc.Command,
		Args:      sc.Args,
		URL:       sc.URL,
		Enabled:   sc.Enabled,
		Env:       sc.Env,
		ToolCount: toolCount,
	})
}

// addServer adds a new server to the config and persists it. The mutation is
// transactional: a candidate copy is validated and persisted before being
// published to the active config, so a failed save leaves both disk and
// in-memory state unchanged. The whole sequence is guarded by the handler
// mutex so concurrent adds cannot race or lose updates.
// addServer adds a new server to the config and persists it. The mutation is
// transactional: a candidate copy is validated and persisted before being
// published to the active config, so a failed save leaves both disk and
// in-memory state unchanged. The whole sequence is guarded by the handler
// mutex so concurrent adds cannot race or lose updates. The lock is held
// across config.Save deliberately: releasing it would open a lost-update
// window between save and publish. The write is a small local YAML file on
// the owner-only control socket, so the brief reader block is acceptable.
func (h *Handler) addServer(w http.ResponseWriter, r *http.Request) {
	req, ok := parseServerRequest(w, r, "")
	if !ok {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	cfg := h.cfg.Load()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "config not available")
		return
	}
	if _, exists := cfg.Servers[req.Name]; exists {
		writeError(w, http.StatusConflict, "server already exists")
		return
	}
	// Build a candidate copy so a failed save leaves the active config
	// untouched. The explicit map rebuild keeps mutating candidate.Servers
	// from affecting the published snapshot.
	candidate := *cfg
	candidate.Servers = copyServers(cfg.Servers)
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
	if !h.publishCandidate(w, &candidate) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": req.Name})
}

// copyServers returns a fresh map with the same entries, so mutating the copy
// cannot affect the published snapshot.
func copyServers(src map[string]config.ServerConfig) map[string]config.ServerConfig {
	out := make(map[string]config.ServerConfig, len(src)+1)
	for k, v := range src {
		out[k] = v
	}
	return out
}

// publishCandidate persists the candidate atomically and, on success,
// publishes it (config snapshot + gateway). Returns false (after writing the
// error response) when the save fails. The caller must hold h.mu.
func (h *Handler) publishCandidate(w http.ResponseWriter, candidate *config.Config) bool {
	if err := config.Save(h.configPath, candidate); err != nil {
		writeError(w, http.StatusInternalServerError, "save config: "+err.Error())
		return false
	}
	// Publish only after save succeeds. The atomic Store swaps the whole
	// snapshot, so readers never observe a half-built config. The mutex stays
	// held across the sequence so two concurrent mutations cannot both build
	// from the same base and lose one update.
	h.cfg.Store(candidate)
	// Keep the gateway's snapshot in step so config changes are visible
	// without a daemon restart.
	if h.gw != nil {
		h.gw.SetConfig(candidate)
	}
	return true
}

// updateServer applies an edit to an existing server. It follows the same
// transactional pattern as addServer: build a candidate, validate, persist,
// then publish. A rename removes the old entry. If the edited server is
// running and runtime-affecting fields changed, the response flags
// restart_required so the client can decide whether to restart.
func (h *Handler) updateServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	req, ok := parseServerRequest(w, r, name)
	if !ok {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	cfg := h.cfg.Load()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "config not available")
		return
	}
	old, ok := cfg.Servers[name]
	if !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown server %q", name))
		return
	}
	if req.Name != name {
		if _, exists := cfg.Servers[req.Name]; exists {
			writeError(w, http.StatusConflict, fmt.Sprintf("server %q already exists", req.Name))
			return
		}
	}

	candidate := *cfg
	candidate.Servers = copyServers(cfg.Servers)
	newSC := serverConfigFromRequest(req)
	if req.Name != name {
		delete(candidate.Servers, name)
	}
	candidate.Servers[req.Name] = newSC
	if err := candidate.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !h.publishCandidate(w, &candidate) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"name": req.Name, "restart_required": restartNeeded(h.mgr, name, req.Name, old, newSC)})
}

// serverConfigFromRequest maps a request body onto a config entry.
func serverConfigFromRequest(req AddServerRequest) config.ServerConfig {
	return config.ServerConfig{
		Command:   req.Command,
		Args:      req.Args,
		Env:       req.Env,
		URL:       req.URL,
		Transport: req.Transport,
		Enabled:   req.Enabled,
	}
}

// restartNeeded reports whether a running server's runtime-affecting fields
// changed (or the server was renamed), so the client should restart it to
// pick up the edit.
func restartNeeded(mgr *server.Manager, name, newName string, old, new config.ServerConfig) bool {
	if mgr == nil {
		return false
	}
	s := mgr.Server(name)
	if s == nil || s.State().String() != "running" {
		return false
	}
	if name != newName {
		// A rename orphans the running process under the old name.
		return true
	}
	return old.Command != new.Command || old.URL != new.URL ||
		old.Transport != new.Transport || !equalStrings(old.Args, new.Args) ||
		!equalEnv(old.Env, new.Env)
}

// parseServerRequest decodes and validates a server request body. On failure
// it writes the error response and returns false. defaultName is used when
// the body omits the name (PUT keeps the existing name).
func parseServerRequest(w http.ResponseWriter, r *http.Request, defaultName string) (AddServerRequest, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	var req AddServerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	if req.Name == "" {
		req.Name = defaultName
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return req, false
	}
	if strings.Contains(req.Name, "__") {
		writeError(w, http.StatusBadRequest, "name cannot contain '__' (reserved for tool canonicalization)")
		return req, false
	}
	if req.Command != "" && req.URL != "" {
		writeError(w, http.StatusBadRequest, "cannot set both command and url")
		return req, false
	}
	if req.Command == "" && req.URL == "" {
		writeError(w, http.StatusBadRequest, "must set command or url")
		return req, false
	}
	return req, true
}

// equalStrings compares two string slices for equality.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// equalEnv compares two env maps for equality.
// deleteServer removes a server transactionally: stop it if running, remove
// its client and tools from the gateway, then persist the config without it.
// A failed save leaves the config and runtime untouched.
func (h *Handler) deleteServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	h.mu.Lock()
	defer h.mu.Unlock()

	cfg := h.cfg.Load()
	if cfg == nil {
		writeError(w, http.StatusInternalServerError, "config not available")
		return
	}
	if _, ok := cfg.Servers[name]; !ok {
		writeError(w, http.StatusNotFound, fmt.Sprintf("unknown server %q", name))
		return
	}

	candidate := *cfg
	candidate.Servers = copyServers(cfg.Servers)
	delete(candidate.Servers, name)
	if err := candidate.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !h.publishCandidate(w, &candidate) {
		return
	}

	// Runtime cleanup after the config is committed: stop the server, remove
	// its client + tools from the gateway, and deregister it from the manager.
	h.cleanupRuntime(name)
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "deleted": true})
}

// cleanupRuntime stops a server, removes its client and tools from the
// gateway, and deregisters it from the manager. Shared by the delete and
// update (rename) paths so stale state cannot survive a mutation.
func (h *Handler) cleanupRuntime(name string) {
	if h.mgr != nil {
		if s := h.mgr.Server(name); s != nil {
			_ = s.Stop()
		}
		h.mgr.Remove(name)
	}
	if h.gw != nil {
		if c := h.gw.RemoveServer(name); c != nil {
			if closer, ok := c.(interface{ Close() error }); ok {
				_ = closer.Close()
			}
		}
	}
	if h.syncTools != nil {
		h.syncTools()
	}
}

func equalEnv(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if bv, ok := b[k]; !ok || bv != v {
			return false
		}
	}
	return true
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
	cfg := h.cfg.Load()
	if cfg == nil {
		writeJSON(w, http.StatusOK, map[string]any{"processes": out})
		return
	}
	for name := range cfg.Servers {
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
	var ok bool
	cfg := h.cfg.Load()
	if cfg != nil {
		sc, ok = cfg.Servers[name]
	}
	if !ok {
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

	// Commit the lifecycle state before touching the gateway. On replacement
	// the server is already running; tolerate that as a no-op so restarting a
	// running server succeeds. Any other failure leaves the old client intact.
	if err := h.mgr.MarkRunning(name); err != nil && !errors.Is(err, server.ErrInvalidTransition) {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if h.gw != nil {
		// Atomically swap: remove the old client + old tools, then register the
		// new client + new tools. This runs only after MarkRunning succeeded,
		// so a state failure cannot destroy a previously working client.
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
	cfg := h.cfg.Load()
	if cfg != nil {
		if _, ok := cfg.Servers[name]; !ok {
			writeError(w, http.StatusNotFound, fmt.Sprintf("unknown server %q", name))
			return
		}
	}
	if h.mgr == nil {
		writeError(w, http.StatusInternalServerError, "manager not available")
		return
	}
	// A server the manager has never seen is not running: report the state
	// conflict without touching the gateway.
	if h.mgr.Server(name) == nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("server %q is not running", name))
		return
	}
	// Commit the lifecycle state before destructive cleanup. If this fails
	// (already stopped, or an unexpected error) the gateway stays intact.
	// MarkStopped only fails with state sentinels for a registered server, so
	// any error here is a state conflict, not a server fault.
	if err := h.mgr.MarkStopped(name); err != nil {
		writeError(w, http.StatusConflict, err.Error())
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
	if err := os.MkdirAll(filepath.Dir(s.sock), 0o700); err != nil {
		return err
	}
	// Probe for a live daemon before touching the socket. A successful dial
	// means another daemon is listening; refuse rather than unlinking its
	// socket and hijacking the endpoint. A failed dial (ECONNREFUSED or no
	// such file) means the socket is stale and safe to remove.
	if conn, derr := net.Dial("unix", s.sock); derr == nil {
		_ = conn.Close()
		return fmt.Errorf("daemon already running on %s", s.sock)
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
