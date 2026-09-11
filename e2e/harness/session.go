// Package harness provides the E2E test harness for mcpeach: it drives the
// real `mcpeach serve` daemon and the real `mcpeach` TUI through the
// `tui-test` CLI over a real PTY, against isolated XDG directories.
package harness

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TUITest is the path to the pinned tui-test executable.
var TUITest = "tui-test"

// Session wraps a named tui-test session and runs CLI operations without
// invoking a shell.
type Session struct {
	t         *testing.T
	name      string
	bin       string // path to the mcpeach binary under test
	root      string // isolated test root
	artifacts string
}

// NewSession starts a tui-test session running the given program (the mcpeach
// binary) with the given args, in an isolated environment.
func NewSession(t *testing.T, name, bin string, args []string, env []string, cols, rows int) *Session {
	t.Helper()
	s := &Session{t: t, name: name, bin: bin, root: t.TempDir()}
	s.artifacts = filepath.Join(s.root, "artifacts")
	_ = os.MkdirAll(s.artifacts, 0o755)

	cmdArgs := []string{"run", "--session", name, "--cols", fmt.Sprint(cols), "--rows", fmt.Sprint(rows), "--no-wait-ready", "--restart"}
	for _, e := range env {
		cmdArgs = append(cmdArgs, "--env", e)
	}
	cmdArgs = append(cmdArgs, bin)
	cmdArgs = append(cmdArgs, args...)

	out, err := s.runCLI(cmdArgs...)
	if err != nil {
		t.Fatalf("tui-test run: %v\n%s", err, out)
	}
	return s
}

// runCLI executes a tui-test command and returns its combined output.
func (s *Session) runCLI(args ...string) (string, error) {
	cmd := exec.Command(TUITest, args...) // nosemgrep go_subproc_rule-subproc // NOSONAR S4036
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// Key presses a key in the session.
func (s *Session) Key(key string) {
	s.t.Helper()
	out, err := s.runCLI("key", "press", "--session", s.name, key)
	if err != nil {
		s.t.Fatalf("key %s: %v\n%s", key, err, out)
	}
}

// Type types literal text into the session.
func (s *Session) Type(text string) {
	s.t.Helper()
	out, err := s.runCLI("type", "--session", s.name, text)
	if err != nil {
		s.t.Fatalf("type %q: %v\n%s", text, err, out)
	}
}

// Text returns the current terminal text.
func (s *Session) Text() string {
	s.t.Helper()
	out, err := s.runCLI("text", "--session", s.name)
	if err != nil {
		s.t.Fatalf("text: %v\n%s", err, out)
	}
	return out
}

// Expect asserts that the terminal text contains want, retrying until the
// timeout. It fails the test if the text never appears.
func (s *Session) Expect(want string) {
	s.t.Helper()
	out, err := s.runCLI("expect", "--session", s.name, "text", want)
	if err != nil {
		s.t.Fatalf("expect %q: %v\n--- current text ---\n%s", want, err, s.Text())
	}
	_ = out
}

// ExpectGone asserts that the terminal text does NOT contain want.
func (s *Session) ExpectGone(want string) {
	s.t.Helper()
	out, err := s.runCLI("expect", "--session", s.name, "text", want)
	if err == nil {
		s.t.Fatalf("expected %q to be absent, but it is present", want)
	}
	_ = out
}

// WaitIdle waits until the terminal is idle (no pending output).
func (s *Session) WaitIdle() {
	s.t.Helper()
	out, err := s.runCLI("wait", "--session", s.name, "idle")
	if err != nil {
		s.t.Fatalf("wait idle: %v\n%s", err, out)
	}
}

// Close closes the session.
func (s *Session) Close() {
	s.t.Helper()
	_, _ = s.runCLI("close", "--session", s.name)
}

// Root returns the isolated test root directory.
func (s *Session) Root() string { return s.root }

// ArtifactsDir returns the artifacts directory for this session.
func (s *Session) ArtifactsDir() string { return s.artifacts }

// SaveArtifact writes a named artifact (e.g. terminal text) into the session's
// artifacts directory.
func (s *Session) SaveArtifact(name, content string) {
	s.t.Helper()
	p := filepath.Join(s.artifacts, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		s.t.Logf("save artifact %s: %v", name, err)
	}
}

// EnvFor builds the environment for a session: the base environment plus the
// given overrides, with XDG paths pointed at the isolated root.
func (s *Session) EnvFor(overrides ...string) []string {
	env := []string{
		"XDG_CONFIG_HOME=" + filepath.Join(s.root, "config"),
		"XDG_RUNTIME_DIR=" + filepath.Join(s.root, "runtime"),
		"HOME=" + filepath.Join(s.root, "home"),
	}
	env = append(env, overrides...)
	return env
}

// SocketPath returns the expected control socket path for this session.
func (s *Session) SocketPath() string {
	return filepath.Join(s.root, "runtime", "mcpeach", "mcpeach.sock")
}

// ConfigPath returns the expected config path for this session.
func (s *Session) ConfigPath() string {
	return filepath.Join(s.root, "config", "mcpeach", "mcpeach.yml")
}

// Sanitize strips ANSI escape sequences from text for stable assertions.
func Sanitize(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		if r == '\x1b' {
			in = true
			continue
		}
		if in {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				in = false
			}
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
