package tui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/mcpeach/mcpeach/internal/client"
)

// fakeClient is a minimal control-plane client for tests.
type fakeClient struct {
	servers []client.ServerInfo
	tools   []string
	logs    map[string][]string
	started map[string]bool
	stopped map[string]bool
}

func (f *fakeClient) ListServers(ctx context.Context) ([]client.ServerInfo, error) {
	return f.servers, nil
}
func (f *fakeClient) ListTools(ctx context.Context) ([]string, error) {
	return f.tools, nil
}
func (f *fakeClient) ServerLogs(ctx context.Context, name string) ([]string, error) {
	return f.logs[name], nil
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
func (f *fakeClient) AddServer(ctx context.Context, req client.AddServerRequest) error {
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

func TestModelViewLogs(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a"}}, logs: map[string][]string{"a": {"line1"}}}
	m := NewModel(fc)
	m.loadServers()
	m.view = viewLogs
	m.logLines = []string{"line1"}
	v := m.View()
	if !strings.Contains(v.Content, "Logs for a") {
		t.Errorf("View logs missing header: %q", v.Content)
	}
	if !strings.Contains(v.Content, "line1") {
		t.Errorf("View logs missing line: %q", v.Content)
	}
}

func TestModelViewTools(t *testing.T) {
	fc := &fakeClient{tools: []string{"a__tool1"}}
	m := NewModel(fc)
	m.view = viewTools
	m.tools = []string{"a__tool1"}
	v := m.View()
	if !strings.Contains(v.Content, "Tools") {
		t.Errorf("View tools missing header: %q", v.Content)
	}
	if !strings.Contains(v.Content, "a__tool1") {
		t.Errorf("View tools missing tool: %q", v.Content)
	}
}

func TestModelViewForm(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)
	m.view = viewForm
	v := m.View()
	if !strings.Contains(v.Content, "Add server form") {
		t.Errorf("View form missing header: %q", v.Content)
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

func TestSpaceStartsStoppedServer(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: "stopped"}}}
	m := NewModel(fc)
	m.loadServers()

	_, cmd := m.Update(tea.KeyPressMsg{Code: uv.KeySpace})
	if cmd == nil {
		t.Fatal("Space on stopped: want start cmd, got nil")
	}
	cmd()
	if !fc.started["a"] {
		t.Error("Space on stopped did not start server")
	}
	if fc.stopped["a"] {
		t.Error("Space on stopped must not stop the server")
	}
}

func TestSpaceStartsErroredServer(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: "error"}}}
	m := NewModel(fc)
	m.loadServers()

	_, cmd := m.Update(tea.KeyPressMsg{Code: uv.KeySpace})
	if cmd == nil {
		t.Fatal("Space on error: want start cmd, got nil")
	}
	cmd()
	if !fc.started["a"] {
		t.Error("Space on error did not start server")
	}
}

func TestSpaceStopsRunningServer(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: "running"}}}
	m := NewModel(fc)
	m.loadServers()

	_, cmd := m.Update(tea.KeyPressMsg{Code: uv.KeySpace})
	if cmd == nil {
		t.Fatal("Space on running: want stop cmd, got nil")
	}
	cmd()
	if !fc.stopped["a"] {
		t.Error("Space on running did not stop server")
	}
	if fc.started["a"] {
		t.Error("Space on running must not start the server")
	}
}

func TestSpaceNoOpOnUnknownState(t *testing.T) {
	// Unknown/transitional states must not trigger a lifecycle command, but
	// should surface a visible status message.
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: "weird"}}}
	m := NewModel(fc)
	m.loadServers()

	_, cmd := m.Update(tea.KeyPressMsg{Code: uv.KeySpace})
	if cmd != nil {
		t.Error("Space on unknown state: want nil cmd")
	}
	if m.status == "" {
		t.Error("Space on unknown state: want visible status, got empty")
	}
}

func TestEnterDoesNotStartOrStop(t *testing.T) {
	for _, state := range []string{"stopped", "running", "error"} {
		fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: state}}}
		m := NewModel(fc)
		m.loadServers()

		_, cmd := m.Update(tea.KeyPressMsg{Code: uv.KeyEnter})
		if cmd != nil {
			t.Errorf("Enter on %s: want nil cmd, got %v", state, cmd)
		}
		if fc.started["a"] || fc.stopped["a"] {
			t.Errorf("Enter on %s issued a lifecycle command", state)
		}
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

func TestModelToggleLogViewer(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a"}}, logs: map[string][]string{"a": {"line1", "line2"}}}
	m := NewModel(fc)
	m.loadServers()

	// 'l' opens the log viewer.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'l'})
	if cmd == nil {
		t.Fatal("'l': want load-logs cmd, got nil")
	}
	msg := cmd()
	logsMsg, ok := msg.(logsLoadedMsg)
	if !ok {
		t.Fatalf("load-logs cmd produced %T, want logsLoadedMsg", msg)
	}
	m.Update(logsMsg)
	if m.view != viewLogs {
		t.Errorf("view = %v, want viewLogs after 'l'", m.view)
	}
	if len(m.logLines) != 2 {
		t.Errorf("logLines = %d, want 2", len(m.logLines))
	}

	// 'l' again returns to the list.
	m.Update(tea.KeyPressMsg{Code: 'l'})
	if m.view != viewList {
		t.Errorf("view = %v, want viewList after second 'l'", m.view)
	}
}

func TestModelEscBackFromSubView(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a"}}}
	m := NewModel(fc)
	m.loadServers()

	// Enter the log viewer.
	m.Update(tea.KeyPressMsg{Code: 'l'})
	if m.view != viewLogs {
		t.Fatal("view = viewLogs, got other")
	}

	// esc toggles back to the list, does not quit.
	_, cmd := m.Update(tea.KeyPressMsg{Code: uv.KeyEscape})
	if cmd != nil {
		t.Errorf("esc in sub-view: want nil cmd (back), got %v", cmd)
	}
	if m.view != viewList {
		t.Errorf("view = %v, want viewList after esc", m.view)
	}

	// Same for the tools view.
	m.Update(tea.KeyPressMsg{Code: 't'})
	if m.view != viewTools {
		t.Fatal("view = viewTools, got other")
	}
	_, cmd = m.Update(tea.KeyPressMsg{Code: 'q'})
	if cmd != nil {
		t.Errorf("q in tools view: want nil cmd (back), got %v", cmd)
	}
	if m.view != viewList {
		t.Errorf("view = %v, want viewList after q", m.view)
	}
}

func TestListFooterShowsNewKeybindings(t *testing.T) {
	m := NewModel(&fakeClient{})
	v := m.View()
	for _, want := range []string{"space start/stop", "enter manage", "n new", "l logs", "t tools", "q quit"} {
		if !strings.Contains(v.Content, want) {
			t.Errorf("list footer missing %q: %q", want, v.Content)
		}
	}
	if strings.Contains(v.Content, "a add") {
		t.Error("list footer still advertises old 'a add' binding")
	}
}

func TestEnterShowsComingSoonStatus(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a", State: "running"}}}
	m := NewModel(fc)
	m.loadServers()
	m.Update(tea.KeyPressMsg{Code: uv.KeyEnter})
	v := m.View()
	if !strings.Contains(v.Content, "coming soon") {
		t.Errorf("View missing 'coming soon' status after Enter: %q", v.Content)
	}
}

func TestModelQuitFromList(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)

	// esc in the list view quits.
	_, cmd := m.Update(tea.KeyPressMsg{Code: uv.KeyEscape})
	if cmd == nil {
		t.Error("esc in list: want quit cmd, got nil")
	}
}

func TestModelViewLogsNoServer(t *testing.T) {
	// logs view with no servers should not panic.
	m := NewModel(&fakeClient{})
	m.view = viewLogs
	v := m.View()
	if !strings.Contains(v.Content, "No server selected") {
		t.Errorf("View logs no-server missing message: %q", v.Content)
	}
}

func TestModelToggleTools(t *testing.T) {
	fc := &fakeClient{tools: []string{"a__tool1", "a__tool2"}}
	m := NewModel(fc)

	// 't' opens the tools view.
	_, cmd := m.Update(tea.KeyPressMsg{Code: 't'})
	if cmd == nil {
		t.Fatal("'t': want load-tools cmd, got nil")
	}
	msg := cmd()
	toolsMsg, ok := msg.(toolsLoadedMsg)
	if !ok {
		t.Fatalf("load-tools cmd produced %T, want toolsLoadedMsg", msg)
	}
	m.Update(toolsMsg)
	if m.view != viewTools {
		t.Errorf("view = %v, want viewTools after 't'", m.view)
	}
	if len(m.tools) != 2 {
		t.Errorf("tools = %d, want 2", len(m.tools))
	}
}

func TestNOpensForm(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)

	// 'n' opens the add-server form.
	m.Update(tea.KeyPressMsg{Code: 'n'})
	if m.view != viewForm {
		t.Errorf("view = %v, want viewForm after 'n'", m.view)
	}
	if m.form == nil {
		t.Error("form not initialized after 'n'")
	}

	// esc returns to the list.
	m.Update(tea.KeyPressMsg{Code: uv.KeyEscape})
	if m.view != viewList {
		t.Errorf("view = %v, want viewList after esc", m.view)
	}

	// 'a' is a no-op.
	m.Update(tea.KeyPressMsg{Code: 'a'})
	if m.view != viewList {
		t.Errorf("'a' changed view to %v, want viewList (no-op)", m.view)
	}
}

func TestStaleLogsResponseIgnored(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{{Name: "a"}, {Name: "b"}}}
	m := NewModel(fc)
	m.loadServers()
	m.view = viewLogs
	m.logLines = []string{"a-line"}

	// A response for server "b" while "a" is selected must be dropped.
	m.Update(logsLoadedMsg{name: "b", lines: []string{"b-line"}})
	if len(m.logLines) != 1 || m.logLines[0] != "a-line" {
		t.Errorf("logLines = %v, want [a-line] (stale response ignored)", m.logLines)
	}

	// A response for the selected server is applied.
	m.Update(logsLoadedMsg{name: "a", lines: []string{"fresh"}})
	if len(m.logLines) != 1 || m.logLines[0] != "fresh" {
		t.Errorf("logLines = %v, want [fresh]", m.logLines)
	}
}

func TestSelectionClampedOnShrink(t *testing.T) {
	fc := &fakeClient{servers: []client.ServerInfo{
		{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}, {Name: "e"},
	}}
	m := NewModel(fc)
	m.loadServers()
	m.selected = 4

	// Refresh returns fewer servers; selection must be clamped, not panic.
	updated, _ := m.Update(serversLoadedMsg{servers: []client.ServerInfo{{Name: "a"}, {Name: "b"}}})
	got := updated.(*Model)
	if got.selected != 1 {
		t.Errorf("selected = %d, want 1", got.selected)
	}
	// Accessing the selected server must not panic.
	_ = got.servers[got.selected]
}

func TestLoadErrorSurfaced(t *testing.T) {
	m := NewModel(&fakeClient{})
	updated, _ := m.Update(serversLoadedMsg{err: errors.New("daemon unreachable")})
	got := updated.(*Model)
	if !strings.Contains(got.View().Content, "daemon unreachable") {
		t.Errorf("View does not surface load error: %q", got.View().Content)
	}
}

func TestBuildAddServerForm(t *testing.T) {
	f := &addServerForm{}
	form := buildAddServerForm(f)
	if form == nil {
		t.Fatal("buildAddServerForm returned nil")
	}
}

func TestSubmitAddServer(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)
	f := &addServerForm{name: "new", command: "echo", args: "a b"}
	cmd := m.submitAddServer(f)
	if cmd == nil {
		t.Fatal("submitAddServer returned nil cmd")
	}
	msg := cmd()
	done, ok := msg.(addServerDoneMsg)
	if !ok {
		t.Fatalf("submitAddServer produced %T, want addServerDoneMsg", msg)
	}
	if done.err != nil {
		t.Errorf("submitAddServer err = %v, want nil", done.err)
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
func (e *errClient) ServerLogs(ctx context.Context, name string) ([]string, error) {
	return nil, fmt.Errorf("boom")
}
func (e *errClient) StartServer(ctx context.Context, name string) error {
	return fmt.Errorf("boom")
}
func (e *errClient) StopServer(ctx context.Context, name string) error {
	return fmt.Errorf("boom")
}
func (e *errClient) AddServer(ctx context.Context, req client.AddServerRequest) error {
	return fmt.Errorf("boom")
}

func TestLoadCommandsSurfaceErrors(t *testing.T) {
	m := NewModel(&errClient{})
	m.servers = []client.ServerInfo{{Name: "srv"}}
	m.selected = 0

	// Each load command must return a msg carrying the error.
	if msg := m.loadServersCmd()(); !strings.Contains(msg.(serversLoadedMsg).err.Error(), "boom") {
		t.Errorf("loadServersCmd err = %v, want boom", msg.(serversLoadedMsg).err)
	}
	if msg := m.loadLogsCmd("srv")(); !strings.Contains(msg.(logsLoadedMsg).err.Error(), "boom") {
		t.Errorf("loadLogsCmd err = %v, want boom", msg.(logsLoadedMsg).err)
	}
	if msg := m.loadToolsCmd()(); !strings.Contains(msg.(toolsLoadedMsg).err.Error(), "boom") {
		t.Errorf("loadToolsCmd err = %v, want boom", msg.(toolsLoadedMsg).err)
	}
}

func TestQInertInFormView(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)
	m.view = viewForm

	// 'q' must not quit or navigate while the form owns text input.
	updated, cmd := m.Update(tea.KeyPressMsg{Code: 'q'})
	if cmd != nil {
		t.Error("'q' in form view: want nil cmd")
	}
	if updated.(*Model).view != viewForm {
		t.Errorf("view = %v, want viewForm ('q' must be inert in form)", updated.(*Model).view)
	}

	// esc still cancels the form.
	m.Update(tea.KeyPressMsg{Code: uv.KeyEscape})
	if m.view != viewList {
		t.Errorf("view = %v, want viewList after esc in form", m.view)
	}
}
