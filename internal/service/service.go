// Package service wraps kardianos/service to install, uninstall, and run
// mcpeach as a background OS service (launchd on macOS, systemd on Linux).
package service

import (
	"context"
	"sync"

	ks "github.com/kardianos/service"
)

// Status mirrors kardianos/service.Status.
type Status = ks.Status

// Status values.
const (
	StatusUnknown = ks.StatusUnknown
	StatusRunning = ks.StatusRunning
	StatusStopped = ks.StatusStopped
)

// Manager installs/uninstalls/queries the mcpeach service.
type Manager struct {
	svc ks.Service
}

// NewManager builds a service manager for the current executable. run is the
// daemon entrypoint invoked when the service starts; stop is called on
// shutdown; log receives any error the daemon returns (nil to discard).
func NewManager(run func(ctx context.Context) error, stop func(), log func(err error)) (*Manager, error) {
	cfg := &ks.Config{
		Name:        "mcpeach",
		DisplayName: "mcpeach MCP gateway",
		Description: "Local MCP gateway aggregating multiple MCP servers behind a single streamable-HTTP endpoint.",
		Arguments:   []string{"serve"},
	}
	prg := &Program{run: run, stop: stop, log: log}
	s, err := ks.New(prg, cfg)
	if err != nil {
		return nil, err
	}
	return &Manager{svc: s}, nil
}

// Run blocks until the service is stopped. It should be called from the
// service entrypoint (e.g. the serve command when running as a service).
func (m *Manager) Run() error {
	return m.svc.Run()
}

// Install registers the service with the OS service manager.
func (m *Manager) Install() error {
	return m.svc.Install()
}

// Uninstall removes the service from the OS service manager.
func (m *Manager) Uninstall() error {
	return m.svc.Uninstall()
}

// Status returns the current service status.
func (m *Manager) Status() (Status, error) {
	return m.svc.Status()
}

// Program implements kardianos/service.Interface. It runs the daemon until
// the service is stopped.
type Program struct {
	run  func(ctx context.Context) error
	stop func()
	log  func(err error)

	mu      sync.Mutex // guards the fields below
	cancel  context.CancelFunc
	started bool // a Start has occurred
	stopped bool // the current run's stop hook has fired
}

// Start runs the daemon in a goroutine. If a prior run is still active it is
// cancelled first, so reusing a Program does not leak the earlier context.
func (p *Program) Start(s ks.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.mu.Lock()
	if p.cancel != nil {
		p.cancel()
	}
	p.cancel = cancel
	p.started = true
	p.stopped = false
	p.mu.Unlock()
	go func() {
		if p.run != nil {
			if err := p.run(ctx); err != nil && p.log != nil {
				p.log(err)
			}
		}
	}()
	return nil
}

// Stop cancels the daemon context and invokes the caller's stop hook. It is
// safe to call concurrently and idempotent per run: the stop hook fires at
// most once per Start, and never before the first Start.
func (p *Program) Stop(s ks.Service) error {
	p.mu.Lock()
	if !p.started || p.stopped {
		p.mu.Unlock()
		return nil
	}
	p.stopped = true
	cancel := p.cancel
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if p.stop != nil {
		p.stop()
	}
	return nil
}
