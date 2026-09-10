// Package service wraps kardianos/service to install, uninstall, and run
// mcpeach as a background OS service (launchd on macOS, systemd on Linux).
package service

import (
	"context"

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
// daemon entrypoint invoked when the service starts; stop is called on shutdown.
func NewManager(run func(ctx context.Context) error, stop func()) (*Manager, error) {
	cfg := &ks.Config{
		Name:        "mcpeach",
		DisplayName: "mcpeach MCP gateway",
		Description: "Local MCP gateway aggregating multiple MCP servers behind a single streamable-HTTP endpoint.",
		Arguments:   []string{"serve"},
	}
	prg := &Program{run: run, stop: stop}
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
	run    func(ctx context.Context) error
	stop   func()
	cancel context.CancelFunc
	log    func(err error)
}

// Start runs the daemon in a goroutine.
func (p *Program) Start(s ks.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go func() {
		if p.run != nil {
			if err := p.run(ctx); err != nil && p.log != nil {
				p.log(err)
			}
		}
	}()
	return nil
}

// Stop cancels the daemon context and invokes the caller's stop hook.
func (p *Program) Stop(s ks.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.stop != nil {
		p.stop()
	}
	return nil
}
