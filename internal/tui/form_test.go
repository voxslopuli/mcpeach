package tui

import (
	"context"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/mcpeach/mcpeach/internal/client"
)

// addRecorder records AddServer requests for form tests.
type addRecorder struct {
	req client.AddServerRequest
}

func (r *addRecorder) ListServers(ctx context.Context) ([]client.ServerInfo, error) {
	return nil, nil
}
func (r *addRecorder) ListSecrets(ctx context.Context) ([]client.ServerSecrets, error) {
	return nil, nil
}

func (r *addRecorder) Import(ctx context.Context, path, c, s string) (client.ImportResult, error) {
	return client.ImportResult{}, nil
}

func (r *addRecorder) Export(ctx context.Context, path, s string, a bool) error {
	return nil
}

func (r *addRecorder) ListTools(ctx context.Context) ([]string, error) {
	return nil, nil
}
func (r *addRecorder) ServerLogs(ctx context.Context, name string) ([]string, error) {
	return nil, nil
}
func (r *addRecorder) StartServer(ctx context.Context, name string) error { return nil }
func (r *addRecorder) StopServer(ctx context.Context, name string) error  { return nil }
func (r *addRecorder) AddServer(ctx context.Context, req client.AddServerRequest) error {
	r.req = req
	return nil
}
func (r *addRecorder) UpdateServer(ctx context.Context, name string, req client.AddServerRequest) error {
	r.req = req
	return nil
}
func (r *addRecorder) DeleteServer(ctx context.Context, name string) error {
	return nil
}
func (r *addRecorder) GetServer(ctx context.Context, name string) (client.ServerDetail, error) {
	return client.ServerDetail{}, nil
}

func TestFormSubmit(t *testing.T) {
	tests := []struct {
		name    string
		form    *addServerForm
		wantErr bool
		wantReq client.AddServerRequest
	}{
		{
			name:    "stdio",
			form:    &addServerForm{name: "srv", command: "echo", transport: "stdio", enabled: true},
			wantReq: client.AddServerRequest{Name: "srv", Command: "echo", Transport: "stdio", Enabled: true},
		},
		{
			name:    "remote",
			form:    &addServerForm{name: "srv", transport: "sse", url: "https://mcp.example.com/mcp", enabled: true},
			wantReq: client.AddServerRequest{Name: "srv", URL: "https://mcp.example.com/mcp", Transport: "sse", Enabled: true},
		},
		{
			name:    "remote missing url",
			form:    &addServerForm{name: "srv", transport: "streamable-http"},
			wantErr: true,
		},
		{
			name:    "stdio missing command",
			form:    &addServerForm{name: "srv", transport: "stdio"},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := &addRecorder{}
			m := NewModel(rec)
			msg := m.submitAddServer(tt.form)()
			done, ok := msg.(addServerDoneMsg)
			if !ok {
				t.Fatalf("got %T, want addServerDoneMsg", msg)
			}
			if tt.wantErr {
				if done.err == nil {
					t.Fatal("err = nil, want validation error")
				}
				if rec.req.Name != "" {
					t.Errorf("request = %+v, want no submission", rec.req)
				}
				return
			}
			if done.err != nil {
				t.Fatalf("err = %v, want nil", done.err)
			}
			if !reflect.DeepEqual(rec.req, tt.wantReq) {
				t.Errorf("request = %+v, want %+v", rec.req, tt.wantReq)
			}
		})
	}
}

func TestValidateServerName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"github", false},
		{"", true},
		{"a__b", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateServerName(tt.name)
			if tt.wantErr && err == nil {
				t.Fatalf("validateServerName(%q) = nil, want error", tt.name)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("validateServerName(%q) = %v, want nil", tt.name, err)
			}
		})
	}
}

func TestSplitArgs(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    []string
		wantErr bool
	}{
		{"empty", "", nil, false},
		{"simple", "a b c", []string{"a", "b", "c"}, false},
		{"whitespace", "a\tb\nc", []string{"a", "b", "c"}, false},
		{"quoted", `"a b" c`, []string{"a b", "c"}, false},
		{"adjacent quotes", `a""b`, []string{"ab"}, false},
		{"unclosed quote", `"unclosed`, nil, true},
		{"cjk and emoji", "日本語 🍑 arg", []string{"日本語", "🍑", "arg"}, false},
		{"quoted cjk", `"日本 語"`, []string{"日本 語"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitArgs(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("splitArgs(%q) = %v, want error", tt.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitArgs(%q) unexpected error: %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitArgs(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestSubmitAddServerUnclosedQuote(t *testing.T) {
	m := NewModel(&fakeClient{})
	msg := m.submitAddServer(&addServerForm{name: "srv", command: "echo", args: `"oops`})()
	done, ok := msg.(addServerDoneMsg)
	if !ok {
		t.Fatalf("got %T, want addServerDoneMsg", msg)
	}
	if done.err == nil {
		t.Error("unclosed quote: want submission error, got nil")
	}
}

func TestFormEditModePrefills(t *testing.T) {
	d := &client.ServerDetail{
		Name: "srv", Command: "npx", Args: []string{"-y", "pkg"}, Transport: "stdio", Enabled: true,
	}
	f := editFormFromDetail(d, "srv")
	if f.name != "srv" || f.command != "npx" || f.args != "-y pkg" || f.editName != "srv" {
		t.Errorf("form = %+v, want prefilled from detail", f)
	}
}

func TestFormEditSubmitsUpdate(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)
	f := &addServerForm{name: "srv", command: "npx", editName: "srv"}
	msg := m.submitAddServer(f)()
	done, ok := msg.(addServerDoneMsg)
	if !ok {
		t.Fatalf("submit produced %T, want addServerDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("submit: %v", done.err)
	}
	if !fc.updated["srv"] {
		t.Error("edit mode did not call UpdateServer")
	}
	if fc.added {
		t.Error("edit mode must not call AddServer")
	}
}

func TestFormRejectsCommandAndURL(t *testing.T) {
	f := &addServerForm{name: "x", command: "npx", url: "https://mcp.example.com/mcp"}
	if err := validateAddServer(f); err == nil {
		t.Error("command+url both set: want error, got nil")
	}
}

func TestFormEditPreservesEnvAndEnabled(t *testing.T) {
	d := &client.ServerDetail{
		Name: "srv", Command: "npx", Transport: "stdio",
		Env:     map[string]string{"TOKEN": "keychain:mcpeach/srv/TOKEN"},
		Enabled: false,
	}
	f := editFormFromDetail(d, "srv")
	if f.env["TOKEN"] != "keychain:mcpeach/srv/TOKEN" {
		t.Errorf("env = %v, want source reference preserved", f.env)
	}
	if f.enabled {
		t.Error("enabled = true, want false (carried from detail)")
	}
}

func TestFormIntegrationCompletes(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)
	// 'n' opens the form (integrated into the model).
	m.Update(tea.KeyPressMsg{Code: 'n'})
	if m.view != viewForm || m.huhForm == nil {
		t.Fatalf("view=%v huhForm=%v, want viewForm + form", m.view, m.huhForm != nil)
	}
	// The form renders.
	if !strings.Contains(m.View().Content, "Server name") {
		t.Errorf("form view missing fields: %s", m.View().Content)
	}
	// Esc aborts and returns to the list.
	m.Update(tea.KeyPressMsg{Code: uv.KeyEscape})
	if m.view != viewList || m.huhForm != nil {
		t.Errorf("view=%v huhForm=%v, want viewList + nil after esc", m.view, m.huhForm != nil)
	}
}

func TestFormIntegrationSubmit(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)
	m.Update(tea.KeyPressMsg{Code: 'n'})
	// Simulate form completion: set the state to completed and verify the
	// submit path returns a cmd and returns to the list.
	m.huhForm.State = huh.StateCompleted
	// updateForm checks State after Update; a completed form's Update returns
	// early, so the state check triggers the submit path.
	_, cmd := m.updateForm(tea.KeyPressMsg{Code: 'x'})
	if m.view != viewList {
		t.Errorf("view=%v, want viewList after submit", m.view)
	}
	if cmd == nil {
		t.Error("expected submit cmd after form completion")
	}
}

func TestRunEditServerForm(t *testing.T) {
	fc := &fakeClient{
		details: map[string]client.ServerDetail{
			"fake": {Name: "fake", Command: "/bin/echo", Transport: "stdio", Enabled: true},
		},
	}
	m := NewModel(fc)
	cmd := m.runEditServerForm("fake")
	if cmd == nil {
		t.Fatal("expected edit cmd")
	}
	if m.huhForm == nil {
		t.Error("huhForm not initialized for edit")
	}
	if m.form == nil || m.form.editName != "fake" {
		t.Errorf("form editName = %v, want fake", m.form)
	}
}

func TestRunEditServerFormNoClient(t *testing.T) {
	m := NewModel(nil)
	msg := m.runEditServerForm("fake")()
	if msg == nil {
		t.Fatal("expected msg")
	}
	done, ok := msg.(addServerDoneMsg)
	if !ok || done.err == nil {
		t.Errorf("expected error msg, got %#v", msg)
	}
}

func TestUpdateFormNil(t *testing.T) {
	m := NewModel(&fakeClient{})
	m.view = viewForm
	m.huhForm = nil
	_, cmd := m.updateForm(tea.KeyPressMsg{Code: 'x'})
	if m.view != viewList {
		t.Errorf("view=%v, want viewList", m.view)
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}

func TestUpdateFormCompletionPath(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)
	m.Update(tea.KeyPressMsg{Code: 'n'})
	// Drive the form to completion by setting its state to completed, then
	// call updateForm with a non-esc message to hit the post-Update branch.
	m.huhForm.State = huh.StateCompleted
	// The pre-check catches it and submits.
	_, cmd := m.updateForm(tea.KeyPressMsg{Code: 'x'})
	if cmd == nil {
		t.Error("expected submit cmd")
	}
	if m.view != viewList {
		t.Errorf("view=%v, want viewList", m.view)
	}
}

func TestFinishFormCompleted(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)
	m.Update(tea.KeyPressMsg{Code: 'n'})
	m.huhForm.State = huh.StateCompleted
	_, cmd := m.finishForm()
	if m.view != viewList || m.huhForm != nil {
		t.Errorf("view=%v huhForm=%v, want viewList + nil", m.view, m.huhForm != nil)
	}
	if cmd == nil {
		t.Error("expected submit cmd for completed form")
	}
}

func TestFinishFormAborted(t *testing.T) {
	fc := &fakeClient{}
	m := NewModel(fc)
	m.Update(tea.KeyPressMsg{Code: 'n'})
	m.huhForm.State = huh.StateAborted
	_, cmd := m.finishForm()
	if m.view != viewList || m.huhForm != nil {
		t.Errorf("view=%v huhForm=%v, want viewList + nil", m.view, m.huhForm != nil)
	}
	if cmd != nil {
		t.Error("expected nil cmd for aborted form")
	}
}
