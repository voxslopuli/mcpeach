package tui

import (
	"context"
	"reflect"
	"testing"

	"github.com/mcpeach/mcpeach/internal/client"
)

// addRecorder records AddServer requests for form tests.
type addRecorder struct {
	req client.AddServerRequest
}

func (r *addRecorder) ListServers(ctx context.Context) ([]client.ServerInfo, error) {
	return nil, nil
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
