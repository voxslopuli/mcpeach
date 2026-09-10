package testutil

import (
	"net/http"
	"os"
	"strings"
	"testing"
)

// TestBuildFakeServer exercises BuildFakeServer: it must compile the fake MCP
// binary and return a path to an existing file. Without this test the helper is
// never instrumented, since it is otherwise only referenced from _test.go files
// in other packages.
func TestBuildFakeServer(t *testing.T) {
	bin := BuildFakeServer(t)
	if bin == "" {
		t.Fatal("BuildFakeServer returned empty path")
	}
	info, err := os.Stat(bin)
	if err != nil {
		t.Fatalf("stat fake server binary %q: %v", bin, err)
	}
	if info.IsDir() {
		t.Fatalf("fake server path %q is a directory, want a file", bin)
	}
}

// TestStartRemoteMCP exercises StartRemoteMCP: it must return a localhost
// http:// URL backed by a reachable streamable-http server. A GET to /mcp is
// the session handshake; any HTTP response (even 404/405) proves the listener
// is up, so only a transport error is a failure.
func TestStartRemoteMCP(t *testing.T) {
	url := StartRemoteMCP(t)
	if !strings.HasPrefix(url, "http://") {
		t.Fatalf("StartRemoteMCP URL = %q, want http:// prefix", url)
	}

	resp, err := http.Get(url + "/mcp") // NOSONAR: S5145 — test-only localhost helper, not production traffic
	if err != nil {
		t.Fatalf("GET %s/mcp: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 0 {
		t.Fatal("empty status code")
	}
}
