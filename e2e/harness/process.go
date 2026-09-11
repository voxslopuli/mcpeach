package harness

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// PidOf returns the PID of a process by matching its command line. Returns 0
// if not found.
func PidOf(t *testing.T, match string) int {
	t.Helper()
	out, err := exec.Command("pgrep", "-f", match).Output() // NOSONAR S4036 fixed args, no shell
	if err != nil {
		return 0
	}
	lines := strings.Fields(string(out))
	if len(lines) == 0 {
		return 0
	}
	pid, _ := strconv.Atoi(lines[0])
	return pid
}

// AssertProcessGone asserts that no process matching match is alive, retrying
// for up to 5s (processes may take a moment to exit).
func AssertProcessGone(t *testing.T, match string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if PidOf(t, match) == 0 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("process matching %q still alive", match)
}

// AssertProcessAlive asserts that a process matching match is running.
func AssertProcessAlive(t *testing.T, match string) {
	t.Helper()
	if PidOf(t, match) == 0 {
		t.Fatalf("process matching %q not running", match)
	}
}

// Signal sends a signal to a process by PID.
func Signal(pid int, sig syscall.Signal) error {
	return syscall.Kill(pid, sig)
}

// FileExists reports whether a path exists.
func FileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
