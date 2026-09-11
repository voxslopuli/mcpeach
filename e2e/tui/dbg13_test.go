package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

func TestDbgDaemonLsof(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(map[string]map[string]any{
		"fake": {"command": fake, "enabled": true},
	})
	cmd := exec.Command(binPath, "serve")
	cmd.Env = append(os.Environ(), fx.Env()...)
	logDir := filepath.Join(fx.Root, "logs")
	os.MkdirAll(logDir, 0o755)
	stderr, _ := os.Create(filepath.Join(logDir, "d.stderr"))
	cmd.Stderr = stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	time.Sleep(2 * time.Second)
	t.Logf("daemon pid=%d alive=%v", cmd.Process.Pid, cmd.ProcessState == nil)
	// lsof the daemon to find its unix sockets
	out, _ := exec.Command("lsof", "-p", "0").Output()
	_ = out
	// Check the daemon's actual socket via lsof -p
	out2, _ := exec.Command("lsof", "-p", "0").Output()
	_ = out2
	// Use lsof to list the daemon's files
	cmd2 := exec.Command("lsof", "-p", "0")
	_ = cmd2
	// Just check the fixture socket and the daemon's stderr
	b, _ := os.ReadFile(filepath.Join(logDir, "d.stderr"))
	t.Logf("stderr=%s", b)
	cmd.Process.Kill()
}
