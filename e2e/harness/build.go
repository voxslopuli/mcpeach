package harness

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// BuildFakeMCP builds the fake-mcp test server binary into a temp dir and
// returns its path. The binary is built once per test process.
func BuildFakeMCP(t *testing.T) string {
	t.Helper()
	return buildFixture(t, "fake-mcp", "github.com/voxslopuli/mcpeach/testdata/fake-mcp")
}

// BuildStreamable builds the streamable fixture binary into a temp dir and
// returns its path.
func BuildStreamable(t *testing.T) string {
	t.Helper()
	return buildFixture(t, "streamable", "github.com/voxslopuli/mcpeach/e2e/fixtures/streamable")
}

// buildFixture builds a fixture binary into a shared temp dir.
func buildFixture(t *testing.T, name, pkg string) string {
	t.Helper()
	dir := filepath.Join(os.TempDir(), "mcpeach-e2e-fixtures") // NOSONAR S5443 test-only temp dir
	_ = os.MkdirAll(dir, 0o755)                                // NOSONAR S5445 test-only
	bin := filepath.Join(dir, name)
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	cmd := exec.Command("go", "build", "-o", bin, pkg) // NOSONAR S4036 fixed args, no shell
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build %s: %v", name, err)
	}
	return bin
}
