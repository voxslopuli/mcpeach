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

func TestFormSubmit(t *testing.T) {
	tests := []struct {
		name    string
		form    *addServerForm
		wantErr bool
		wantReq client.AddServerRequest
	}{
		{
			name:    "stdio",
			form:    &addServerForm{name: "srv", command: "echo", transport: "stdio"},
			wantReq: client.AddServerRequest{Name: "srv", Command: "echo", Transport: "stdio"},
		},
		{
			name:    "remote",
			form:    &addServerForm{name: "srv", transport: "sse", url: "https://mcp.example.com/mcp"},
			wantReq: client.AddServerRequest{Name: "srv", URL: "https://mcp.example.com/mcp", Transport: "sse"},
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
