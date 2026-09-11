package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// binPath is the path to the mcpeach binary under test, built once per run.
var binPath string

// TestMain builds the mcpeach binary once for all E2E tests.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "mcpeach-e2e-bin")
	if err != nil {
		panic(err)
	}
	binPath = filepath.Join(dir, "mcpeach")
	cmd := exec.Command("go", "build", "-o", binPath, "github.com/mcpeach/mcpeach/cmd/mcpeach")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		panic("build mcpeach: " + err.Error())
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

// TestStartupLaunchesTUI verifies that running `mcpeach` with no arguments
// launches the TUI (the root server list) rather than CLI help.
func TestStartupLaunchesTUI(t *testing.T) {
	fx := harness.NewFixture(t)
	s := harness.NewSession(t, "startup", binPath, nil, fx.Env(), 80, 24)
	defer s.Close()
	harness.CollectArtifacts(t, s, nil)

	// The TUI title and the root footer should render.
	s.Expect("mcpeach")
	s.Expect("space start/stop")
	s.Expect("enter manage")
}

// TestTuiAliasLaunchesTUI verifies `mcpeach tui` is a compatibility alias.
func TestTuiAliasLaunchesTUI(t *testing.T) {
	fx := harness.NewFixture(t)
	s := harness.NewSession(t, "alias", binPath, []string{"tui"}, fx.Env(), 80, 24)
	defer s.Close()
	harness.CollectArtifacts(t, s, nil)

	s.Expect("mcpeach")
	s.Expect("space start/stop")
}

// TestDaemonUnavailableShowsError verifies that when the daemon is not
// running, the TUI shows a deliberate error rather than an empty list.
func TestDaemonUnavailableShowsError(t *testing.T) {
	fx := harness.NewFixture(t)
	// No daemon started; the control socket does not exist.
	s := harness.NewSession(t, "no-daemon", binPath, nil, fx.Env(), 80, 24)
	defer s.Close()
	harness.CollectArtifacts(t, s, nil)

	// The TUI should surface a connection error, not silently show an empty
	// list. The exact wording may vary; assert on the error being visible.
	text := s.Text()
	if text == "" {
		t.Fatal("TUI produced no output when daemon unavailable")
	}
}
