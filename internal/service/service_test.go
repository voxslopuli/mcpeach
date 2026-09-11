package service

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
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
	m, err := NewManager(func(ctx context.Context) error { return nil }, func() {}, nil)
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if m == nil {
		t.Fatal("NewManager returned nil")
	}
}

func TestManagerRun(t *testing.T) {
	fs := &fakeSvc{}
	m := &Manager{svc: fs}
	if err := m.Run(); err != nil {
		t.Fatalf("Run: %v", err)
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

func TestProgramConcurrentStopIdempotent(t *testing.T) {
	var stopCalls int32
	p := &Program{
		run: func(ctx context.Context) error {
			<-ctx.Done()
			return nil
		},
		stop: func() { atomic.AddInt32(&stopCalls, 1) },
	}
	if err := p.Start(nil); err != nil {
		t.Fatalf("Start: %v", err)
	}

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = p.Stop(nil)
		}()
	}
	wg.Wait()

	if got := atomic.LoadInt32(&stopCalls); got != 1 {
		t.Errorf("stop hook calls = %d, want 1 (idempotent shutdown)", got)
	}
}

func TestProgramReuse(t *testing.T) {
	ctxs := make(chan context.Context, 2)
	var stopCalls int32
	p := &Program{
		run: func(ctx context.Context) error {
			ctxs <- ctx
			<-ctx.Done()
			return nil
		},
		stop: func() { atomic.AddInt32(&stopCalls, 1) },
	}

	for i := 1; i <= 2; i++ {
		if err := p.Start(nil); err != nil {
			t.Fatalf("Start %d: %v", i, err)
		}
		<-ctxs // wait for the run goroutine to begin
		if err := p.Stop(nil); err != nil {
			t.Fatalf("Stop %d: %v", i, err)
		}
	}

	if got := atomic.LoadInt32(&stopCalls); got != 2 {
		t.Errorf("stop hook calls = %d, want 2 across a reused Program", got)
	}
}

func TestProgramStartCancelsPriorRun(t *testing.T) {
	ctxs := make(chan context.Context, 2)
	p := &Program{
		run: func(ctx context.Context) error {
			ctxs <- ctx
			<-ctx.Done()
			return nil
		},
	}
	t.Cleanup(func() { _ = p.Stop(nil) })

	if err := p.Start(nil); err != nil {
		t.Fatalf("Start 1: %v", err)
	}
	first := <-ctxs
	if err := p.Start(nil); err != nil {
		t.Fatalf("Start 2: %v", err)
	}
	<-ctxs
	select {
	case <-first.Done():
	case <-time.After(time.Second):
		t.Fatal("Start did not cancel the prior run's context")
	}
}

func TestStopBeforeStart(t *testing.T) {
	var stopCalls int32
	p := &Program{
		run:  func(ctx context.Context) error { return nil },
		stop: func() { atomic.AddInt32(&stopCalls, 1) },
	}
	if err := p.Stop(nil); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
	if got := atomic.LoadInt32(&stopCalls); got != 0 {
		t.Errorf("stop hook calls = %d, want 0 before any Start", got)
	}
}

func TestProgramLogsErrors(t *testing.T) {
	wantErr := errors.New("daemon boom")
	logged := make(chan error, 1)
	p := &Program{
		run: func(ctx context.Context) error { return wantErr },
		log: func(err error) { logged <- err },
	}
	if err := p.Start(nil); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case err := <-logged:
		if !errors.Is(err, wantErr) {
			t.Errorf("logged error = %v, want %v", err, wantErr)
		}
	case <-time.After(time.Second):
		t.Fatal("log callback was not called with the run error")
	}
}
