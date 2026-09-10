package server

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"sync"

	"github.com/mcpeach/mcpeach/internal/logs"
)

// logCapacity is the number of log lines retained per server.
const logCapacity = 1000

// Manager owns the lifecycle of all managed servers.
type Manager struct {
	mu      sync.Mutex
	servers map[string]*Server
	procs   map[string]*proc
	logs    map[string]*logs.Ring
}

// proc tracks a running subprocess.
type proc struct {
	cmd  *exec.Cmd
	stop context.CancelFunc
}

// NewManager returns an empty Manager.
func NewManager() *Manager {
	return &Manager{
		servers: map[string]*Server{},
		procs:   map[string]*proc{},
		logs:    map[string]*logs.Ring{},
	}
}

// Add registers a server with the manager.
func (m *Manager) Add(s *Server) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.servers[s.Name()] = s
	m.logs[s.Name()] = logs.NewRing(logCapacity)
}

// Server returns the registered server by name, or nil if unknown.
func (m *Manager) Server(name string) *Server {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.servers[name]
}

// Start launches a stdio subprocess for the named server and captures its
// stderr into the server's log ring.
func (m *Manager) Start(ctx context.Context, name, command string, env []string) error {
	m.mu.Lock()
	srv, ok := m.servers[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown server %q", name)
	}
	if srv.State() == Running {
		m.mu.Unlock()
		return fmt.Errorf("server %q already running", name)
	}
	m.mu.Unlock()

	// command is a user-authored MCP server binary path from the user's own
	// config file, not untrusted input. exec.CommandContext does not invoke a
	// shell, so there is no shell-metacharacter injection vector.
	cmd := exec.CommandContext(ctx, command) // nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command, go_subproc_rule-subproc
	cmd.Env = env
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		srv.Fail()
		return fmt.Errorf("start %q: %w", name, err)
	}

	ring := m.logs[name]
	go scanLines(stderr, ring)

	m.mu.Lock()
	m.procs[name] = &proc{cmd: cmd, stop: func() { cmd.Process.Kill() }}
	m.mu.Unlock()

	if err := srv.Start(); err != nil {
		return err
	}
	return nil
}

// Stop terminates a running server.
func (m *Manager) Stop(name string) error {
	m.mu.Lock()
	srv, ok := m.servers[name]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("unknown server %q", name)
	}
	p, running := m.procs[name]
	if !running {
		m.mu.Unlock()
		return fmt.Errorf("server %q is not running", name)
	}
	delete(m.procs, name)
	m.mu.Unlock()

	if p.stop != nil {
		p.stop()
	}
	p.cmd.Wait()
	return srv.Stop()
}

// Logs returns the captured log lines for a server, or nil if unknown.
func (m *Manager) Logs(name string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ring, ok := m.logs[name]
	if !ok {
		return nil
	}
	return ring.Lines()
}

// scanLines reads lines from r and appends them to the ring.
func scanLines(r io.Reader, ring *logs.Ring) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		ring.Write(sc.Text())
	}
}
