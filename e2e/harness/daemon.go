package harness

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Daemon manages a real `mcpeach serve` process for E2E tests.
type Daemon struct {
	t    *testing.T
	bin  string
	root string
	cmd  *exec.Cmd
}

// StartDaemon launches `mcpeach serve` with an isolated XDG environment and
// waits for the control socket to become connectable.
func StartDaemon(t *testing.T, bin, root string, env []string) *Daemon {
	t.Helper()
	d := &Daemon{t: t, bin: bin, root: root}
	_ = os.MkdirAll(filepath.Join(root, "runtime", "mcpeach"), 0o700)
	_ = os.MkdirAll(filepath.Join(root, "config", "mcpeach"), 0o700)

	logDir := filepath.Join(root, "logs")
	_ = os.MkdirAll(logDir, 0o755)
	stdout, err := os.Create(filepath.Join(logDir, "daemon.stdout"))
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.Create(filepath.Join(logDir, "daemon.stderr"))
	if err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "serve")
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start daemon: %v", err)
	}
	d.cmd = cmd

	// Wait for the socket to become connectable (poll, no fixed sleep).
	sock := filepath.Join(root, "runtime", "mcpeach", "mcpeach.sock")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		// If the daemon exited, report immediately.
		if cmd.ProcessState != nil {
			out, _ := os.ReadFile(filepath.Join(logDir, "daemon.stderr"))
			t.Fatalf("daemon exited early (code %d)\nstderr: %s", cmd.ProcessState.ExitCode(), out)
		}
		conn, err := net.Dial("unix", sock)
		if err == nil {
			_ = conn.Close()
			return d
		}
		time.Sleep(50 * time.Millisecond)
	}
	// The daemon likely exited; report its output.
	out, _ := os.ReadFile(filepath.Join(logDir, "daemon.stderr"))
	out2, _ := os.ReadFile(filepath.Join(logDir, "daemon.stdout"))
	t.Fatalf("daemon socket %s never became connectable\nstderr: %s\nstdout: %s", sock, out, out2)
	return nil
}

// RunDaemonExpectError runs `mcpeach serve` and returns its error, expecting
// the daemon to exit (e.g. when an enabled server fails to connect).
func RunDaemonExpectError(t *testing.T, bin, root string, env []string) error {
	t.Helper()
	_ = os.MkdirAll(filepath.Join(root, "runtime", "mcpeach"), 0o700)
	_ = os.MkdirAll(filepath.Join(root, "config", "mcpeach"), 0o700)
	cmd := exec.Command(bin, "serve")
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	// Return a wrapped error that includes the daemon output.
	return fmt.Errorf("%v: %s", err, out)
}

// waitForAddr polls a file for a "listening on" line and returns the address.
func WaitForAddr(t *testing.T, file string) string {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		b, err := os.ReadFile(file)
		if err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if strings.Contains(line, "listening on") {
					parts := strings.Split(line, "listening on ")
					if len(parts) == 2 {
						return strings.TrimSpace(parts[1])
					}
				}
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	return ""
}

// Stop terminates the daemon and waits for it to exit.
func (d *Daemon) Stop() {
	d.t.Helper()
	if d.cmd == nil || d.cmd.Process == nil {
		return
	}
	_ = d.cmd.Process.Signal(os.Interrupt)
	done := make(chan struct{})
	go func() { _ = d.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = d.cmd.Process.Kill()
		<-done
	}
	d.cmd = nil
}

// SocketPath returns the daemon's control socket path.
func (d *Daemon) SocketPath() string {
	return filepath.Join(d.root, "runtime", "mcpeach", "mcpeach.sock")
}

// ConfigPath returns the daemon's config path.
func (d *Daemon) ConfigPath() string {
	return filepath.Join(d.root, "config", "mcpeach", "mcpeach.yml")
}

// Root returns the daemon's isolated root.
func (d *Daemon) Root() string { return d.root }

// API performs a control-plane request over the unix socket and returns the
// status code and body.
func (d *Daemon) API(method, path string, body string) (int, string) {
	d.t.Helper()
	client := unixClient(d.SocketPath())
	req, err := newUnixRequest(method, path, body)
	if err != nil {
		d.t.Fatalf("build request: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		d.t.Fatalf("api %s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	buf := make([]byte, resp.ContentLength)
	if resp.ContentLength > 0 {
		_, _ = resp.Body.Read(buf)
	}
	return resp.StatusCode, string(buf)
}

// APIJSON performs a control-plane request and returns the raw body.
func (d *Daemon) APIJSON(method, path string, body string) string {
	_, b := d.API(method, path, body)
	return b
}

// fmt is used by callers; keep import.
var _ = fmt.Sprintf
