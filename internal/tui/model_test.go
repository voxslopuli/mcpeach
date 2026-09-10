package tui

import (
	"context"
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/mcpeach/mcpeach/internal/client"
)

// fakeClient is a minimal control-plane client for tests.
type fakeClient struct {
	servers []client.ServerInfo
	tools   []string
	started map[string]bool
	stopped map[string]bool
}

func (f *fakeClient) ListServers(ctx context.Context) ([]client.ServerInfo, error) {
	return f.servers, nil
}
func (f *fakeClient) ListTools(ctx context.Context) ([]string, error) {
	return f.tools, nil
}
func (f *fakeClient) StartServer(ctx context.Context, name string) error {
	if f.started == nil {
		f.started = map[string]bool{}
	}
	f.started[name] = true
	return nil
}
func (f *fakeClient) StopServer(ctx context.Context, name string) error {
	if f.stopped == nil {
		f.stopped = map[string]bool{}
	}
	f.stopped[name] = true
	return nil
}

func TestNewModel(t *testing.T) {
	m := NewModel(&fakeClient{})
	if m == nil {
		t.Fatal("NewModel returned nil")
	}
	if m.client == nil {
		t.Error("client not set")
	}
}

func TestModelInit(t *testing.T) {
	m := NewModel(&fakeClient{})
	if m.Init() == nil {
		t.Error("Init returned nil cmd, want a load command")
	}
}

func TestModelLoadServers(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: "running"}}}
	m := NewModel(fc)
	m.loadServers()
	if len(m.servers) != 1 || m.servers[0].Name != "a" {
		t.Errorf("servers = %+v, want [a]", m.servers)
	}
}

func TestModelSelectServer(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a"}, {Name: "b"}}}
	m := NewModel(fc)
	m.loadServers()
	m.selectNext()
	if m.selected != 1 {
		t.Errorf("selected = %d, want 1", m.selected)
	}
	m.selectPrev()
	if m.selected != 0 {
		t.Errorf("selected = %d, want 0", m.selected)
	}
}

func TestModelStartStop(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: "stopped"}}}
	m := NewModel(fc)
	m.loadServers()
	cmd := m.startSelected()
	if cmd == nil {
		t.Fatal("startSelected returned nil cmd")
	}
	cmd() // run the cmd to trigger the client call
	if !fc.started["a"] {
		t.Error("startSelected did not call StartServer")
	}
	cmd = m.stopSelected()
	if cmd == nil {
		t.Fatal("stopSelected returned nil cmd")
	}
	cmd()
	if !fc.stopped["a"] {
		t.Error("stopSelected did not call StopServer")
	}
}

func TestModelView(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: "running"}}}
	m := NewModel(fc)
	m.loadServers()
	v := m.View()
	if v.Content == "" {
		t.Error("View returned empty string")
	}
}

func TestModelUpdateKeyDown(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a"}, {Name: "b"}}}
	m := NewModel(fc)
	m.loadServers()

	// KeyDown moves selection to index 1.
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.selected != 1 {
		t.Errorf("selected = %d, want 1", m.selected)
	}
	// KeyUp moves back to 0.
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.selected != 0 {
		t.Errorf("selected = %d, want 0", m.selected)
	}
}

func TestModelUpdateStartStop(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: "stopped"}}}
	m := NewModel(fc)
	m.loadServers()

	_, cmd := m.Update(tea.KeyPressMsg{Code: uv.KeyEnter})
	if cmd == nil {
		t.Fatal("Enter: want start cmd, got nil")
	}
	cmd()
	if !fc.started["a"] {
		t.Error("Enter did not start server")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: uv.KeySpace})
	if cmd == nil {
		t.Fatal("Space: want stop cmd, got nil")
	}
	cmd()
	if !fc.stopped["a"] {
		t.Error("Space did not stop server")
	}
}

func TestModelUpdateQuit(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)

	// Escape quits.
	_, cmd := m.Update(tea.KeyPressMsg{Code: uv.KeyEscape})
	if cmd == nil {
		t.Error("Escape: want quit cmd, got nil")
	}
}

func TestModelLoadServersError(t *testing.T) {
	// A client that errors on ListServers should not panic.
	m := NewModel(&errClient{})
	m.loadServers()
	if len(m.servers) != 0 {
		t.Errorf("servers = %d, want 0 on error", len(m.servers))
	}
}

func TestModelEmptyServerNavigation(t *testing.T) {
	// With no servers, navigation and start/stop should be no-ops.
	m := NewModel(&fakeClient{})
	m.selectNext()
	m.selectPrev()
	if cmd := m.startSelected(); cmd != nil {
		t.Error("startSelected with no servers: want nil cmd")
	}
	if cmd := m.stopSelected(); cmd != nil {
		t.Error("stopSelected with no servers: want nil cmd")
	}
	if m.selected != 0 {
		t.Errorf("selected = %d, want 0", m.selected)
	}
}

func TestModelInitCmd(t *testing.T) {
	m := NewModel(&fakeClient{})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("Init returned nil cmd")
	}
	// The cmd should produce a loadServersMsg.
	msg := cmd()
	if _, ok := msg.(loadServersMsg); !ok {
		t.Errorf("Init cmd produced %T, want loadServersMsg", msg)
	}
}

func TestModelUpdateLoadServers(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a"}}}
	m := NewModel(fc)
	// loadServersMsg triggers the async load cmd; run it and handle the result.
	_, cmd := m.Update(loadServersMsg{})
	if cmd == nil {
		t.Fatal("loadServersMsg: want load cmd, got nil")
	}
	msg := cmd()
	loaded, ok := msg.(serversLoadedMsg)
	if !ok {
		t.Fatalf("load cmd produced %T, want serversLoadedMsg", msg)
	}
	updated, _ := m.Update(loaded)
	if len(updated.(*Model).servers) != 1 {
		t.Errorf("servers = %d, want 1 after load", len(updated.(*Model).servers))
	}
}

// errClient returns an error from every method.
type errClient struct{}

func (e *errClient) ListServers(ctx context.Context) ([]client.ServerInfo, error) {
	return nil, fmt.Errorf("boom")
}
func (e *errClient) ListTools(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("boom")
}
func (e *errClient) StartServer(ctx context.Context, name string) error {
	return fmt.Errorf("boom")
}
func (e *errClient) StopServer(ctx context.Context, name string) error {
	return fmt.Errorf("boom")
}
