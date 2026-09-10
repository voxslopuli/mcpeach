// Package testutil provides shared helpers for tests across packages.
package testutil

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// BuildFakeServer compiles the testdata fake MCP server binary and returns its
// path. It is shared by the server, connect, and control test suites.
func BuildFakeServer(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-mcp")
	cmd := exec.Command("go", "build", "-o", bin, "../../testdata/fake-mcp")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fake server: %v\n%s", err, out)
	}
	return bin
}
