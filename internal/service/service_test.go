package service

import (
	"context"
	"testing"
	"time"

	ks "github.com/kardianos/service"
)

// fakeSvc is a fake kardianos/service.Service for tests.
type fakeSvc struct {
	ks.Service  // embed to satisfy the full interface
	installed   bool
	uninstalled bool
	started     bool
	stopped     bool
	status      Status
}

func (f *fakeSvc) Install() error                              { f.installed = true; return nil }
func (f *fakeSvc) Uninstall() error                            { f.uninstalled = true; return nil }
func (f *fakeSvc) Start() error                                { f.started = true; return nil }
func (f *fakeSvc) Stop() error                                 { f.stopped = true; return nil }
func (f *fakeSvc) Restart() error                              { return nil }
func (f *fakeSvc) Run() error                                  { return nil }
func (f *fakeSvc) Status() (Status, error)                     { return f.status, nil }
func (f *fakeSvc) Logger(errs chan<- error) (ks.Logger, error) { return nil, nil }
func (f *fakeSvc) Platform() string                            { return "test" }
func (f *fakeSvc) String() string                              { return "mcpeach" }

func TestInstall(t *testing.T) {
	fs := &fakeSvc{}
	m := &Manager{svc: fs}
	if err := m.Install(); err != nil {
		t.Fatalf("Install: %v", err)
	}
	if !fs.installed {
		t.Error("Install did not call svc.Install")
	}
}

func TestUninstall(t *testing.T) {
	fs := &fakeSvc{}
	m := &Manager{svc: fs}
	if err := m.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !fs.uninstalled {
		t.Error("Uninstall did not call svc.Uninstall")
	}
}

func TestStatus(t *testing.T) {
	fs := &fakeSvc{status: StatusRunning}
	m := &Manager{svc: fs}
	st, err := m.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st != StatusRunning {
		t.Errorf("Status = %v, want running", st)
	}
}

func TestNewManager(t *testing.T) {
	m, err := NewManager()
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestProgramStartStop(t *testing.T) {
	started := make(chan struct{})
	stopped := make(chan struct{})
	p := &Program{
		run: func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return nil
		},
		stop: func() { close(stopped) },
	}
	if err := p.Start(nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Start did not invoke run")
	}
	if err := p.Stop(nil); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not invoke stop")
	}
}
