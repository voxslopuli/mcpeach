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

	"github.com/mcpeach/mcpeach/internal/config"
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

// Handler serves the control-plane API.
type Handler struct {
	mgr *server.Manager
	gw  *gateway.Gateway
	cfg *config.Config
	log *obs.Logger
	res *secrets.Resolver
}

// NewHandler builds a control-plane handler.
func NewHandler(mgr *server.Manager, gw *gateway.Gateway, cfg *config.Config) http.Handler {
	h := &Handler{mgr: mgr, gw: gw, cfg: cfg, log: obs.Default().With("pkg", "control"), res: secrets.NewResolver(secrets.NewKeyringStore())}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v0/servers", h.listServers)
	mux.HandleFunc("GET /v0/tools", h.listTools)
	mux.HandleFunc("GET /v0/metrics", h.metrics)
	mux.HandleFunc("GET /v0/logs", h.logs)
	mux.HandleFunc("GET /v0/processes", h.processes)
	mux.HandleFunc("POST /v0/servers/{name}/start", h.startServer)
	mux.HandleFunc("POST /v0/servers/{name}/stop", h.stopServer)
	return mux
}

func (h *Handler) listServers(w http.ResponseWriter, r *http.Request) {
	// Report the actual operational state from the manager where known.
	resp := ListServersResponse{Servers: []ServerInfo{}}
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

func (h *Handler) startServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var sc config.ServerConfig
	if h.cfg != nil {
		sc = h.cfg.Servers[name]
	}
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
	if err := h.mgr.Start(r.Context(), name, sc.Command, env); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "started"})
}

// resolveEnv resolves any env:/keychain: references in the server's env vars.
func (h *Handler) resolveEnv(env map[string]string) ([]string, error) {
	out := make([]string, 0, len(env))
	for k, v := range env {
		resolved, err := h.res.Resolve(v)
		if err != nil {
			return nil, fmt.Errorf("env %s: %w", k, err)
		}
		out = append(out, k+"="+resolved)
	}
	return out, nil
}

func (h *Handler) stopServer(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if h.cfg != nil {
		if _, ok := h.cfg.Servers[name]; !ok {
			writeError(w, http.StatusNotFound, fmt.Sprintf("unknown server %q", name))
			return
		}
	}
	if h.mgr == nil {
		writeError(w, http.StatusInternalServerError, "manager not available")
		return
	}
	if err := h.mgr.Stop(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stopped"})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
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
	os.Remove(s.sock)
	ln, err := net.Listen("unix", s.sock)
	if err != nil {
		return err
	}
	// Restrict the socket to the owner: it can start/stop processes.
	if err := os.Chmod(s.sock, 0o600); err != nil {
		ln.Close()
		return err
	}
	s.httpSrv = &http.Server{Handler: s.handler}
	go func() {
		<-ctx.Done()
		s.httpSrv.Close()
		os.Remove(s.sock)
	}()
	go s.httpSrv.Serve(ln)
	return nil
}

// SocketPath returns the unix socket path.
func (s *Server) SocketPath() string { return s.sock }
