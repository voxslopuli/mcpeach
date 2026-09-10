package tui

import (
	"context"
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

func TestFormSubmitStdio(t *testing.T) {
	rec := &addRecorder{}
	m := NewModel(rec)
	f := &addServerForm{name: "srv", command: "echo", transport: "stdio"}
	msg := m.submitAddServer(f)()
	done, ok := msg.(addServerDoneMsg)
	if !ok {
		t.Fatalf("got %T, want addServerDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("err = %v, want nil", done.err)
	}
	if rec.req.Name != "srv" || rec.req.Command != "echo" || rec.req.URL != "" || rec.req.Transport != "stdio" {
		t.Errorf("request = %+v, want name=srv command=echo url= transport=stdio", rec.req)
	}
}

func TestFormSubmitRemote(t *testing.T) {
	rec := &addRecorder{}
	m := NewModel(rec)
	f := &addServerForm{name: "srv", transport: "sse", url: "https://mcp.example.com/mcp"}
	msg := m.submitAddServer(f)()
	done, ok := msg.(addServerDoneMsg)
	if !ok {
		t.Fatalf("got %T, want addServerDoneMsg", msg)
	}
	if done.err != nil {
		t.Fatalf("err = %v, want nil", done.err)
	}
	if rec.req.URL != "https://mcp.example.com/mcp" || rec.req.Transport != "sse" {
		t.Errorf("request = %+v, want url+transport set", rec.req)
	}
}

func TestFormSubmitRemoteMissingURL(t *testing.T) {
	rec := &addRecorder{}
	m := NewModel(rec)
	f := &addServerForm{name: "srv", transport: "streamable-http"}
	msg := m.submitAddServer(f)()
	done, ok := msg.(addServerDoneMsg)
	if !ok {
		t.Fatalf("got %T, want addServerDoneMsg", msg)
	}
	if done.err == nil {
		t.Fatal("err = nil, want validation error")
	}
	if rec.req.Name != "" {
		t.Errorf("request = %+v, want no submission", rec.req)
	}
}

func TestFormSubmitStdioMissingCommand(t *testing.T) {
	rec := &addRecorder{}
	m := NewModel(rec)
	f := &addServerForm{name: "srv", transport: "stdio"}
	msg := m.submitAddServer(f)()
	done, ok := msg.(addServerDoneMsg)
	if !ok {
		t.Fatalf("got %T, want addServerDoneMsg", msg)
	}
	if done.err == nil {
		t.Fatal("err = nil, want validation error")
	}
	if rec.req.Name != "" {
		t.Errorf("request = %+v, want no submission", rec.req)
	}
}
