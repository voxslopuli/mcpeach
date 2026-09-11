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
	dir := filepath.Join(os.TempDir(), "mcpeach-e2e-fake") // NOSONAR S5443 test-only temp dir
	_ = os.MkdirAll(dir, 0o755)                            // NOSONAR S5445 test-only
	bin := filepath.Join(dir, "fake-mcp")
	if _, err := os.Stat(bin); err == nil {
		return bin
	}
	cmd := exec.Command("go", "build", "-o", bin, "github.com/mcpeach/mcpeach/testdata/fake-mcp") // NOSONAR S4036 fixed args, no shell
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("build fake-mcp: %v", err)
	}
	return bin
}
