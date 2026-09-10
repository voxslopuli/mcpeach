package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mcpeach/mcpeach/internal/control"
)

// newTestClient builds a client pointed at an in-memory handler.
func newTestClient(t *testing.T) *Client {
	t.Helper()
	h := control.NewHandler(nil, nil, nil)
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(srv.URL)
}

func TestListServers(t *testing.T) {
	c := newTestClient(t)
	servers, err := c.ListServers(context.Background())
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if servers == nil {
		t.Fatal("ListServers returned nil")
	}
}

func TestListTools(t *testing.T) {
	c := newTestClient(t)
	tools, err := c.ListTools(context.Background())
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if tools == nil {
		t.Fatal("ListTools returned nil")
	}
}

func TestStartServer(t *testing.T) {
	c := newTestClient(t)
	err := c.StartServer(context.Background(), "echo")
	if err == nil {
		t.Fatal("StartServer unknown: want error, got nil")
	}
}

func TestStopServer(t *testing.T) {
	c := newTestClient(t)
	err := c.StopServer(context.Background(), "echo")
	if err == nil {
		t.Fatal("StopServer unknown: want error, got nil")
	}
}

func TestClientError(t *testing.T) {
	// Point at a server that returns 404 for everything.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]string{"error": "nope"})
	}))
	t.Cleanup(srv.Close)
	c := New(srv.URL)
	if _, err := c.ListServers(context.Background()); err == nil {
		t.Fatal("ListServers on 404: want error, got nil")
	}
}
