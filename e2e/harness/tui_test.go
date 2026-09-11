package harness

import (
	"os/exec"
	"strings"
	"testing"
)

// TestTUITestAvailable validates that the pinned tui-test executable is
// installed and reports the expected version. This is a harness precondition,
// not a product test.
func TestTUITestAvailable(t *testing.T) {
	out, err := exec.Command(TUITest, "--version").Output()
	if err != nil {
		t.Fatalf("tui-test not available: %v (run scripts/install-tui-test.sh)", err)
	}
	v := strings.TrimSpace(string(out))
	if !strings.HasPrefix(v, "tui-test 0.1.0-beta") {
		t.Fatalf("unexpected tui-test version %q; expected 0.1.0-beta.x", v)
	}
	t.Logf("tui-test version: %s", v)
}
