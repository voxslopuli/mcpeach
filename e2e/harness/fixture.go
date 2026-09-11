package harness

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// Fixture creates an isolated XDG root for a test and returns the paths.
type Fixture struct {
	t    *testing.T
	Root string
}

// NewFixture creates a unique isolated root for a test. The root uses a short
// path under /tmp because the unix control socket path must stay under the
// ~108-byte AF_UNIX limit (os.TempDir() on macOS is a long /var/folders path).
func NewFixture(t *testing.T) *Fixture {
	t.Helper()
	root := filepath.Join("/tmp", "mcpeach-e2e", sanitizeTestName(t.Name()))
	_ = os.RemoveAll(root)
	for _, d := range []string{
		filepath.Join(root, "config", "mcpeach"),
		filepath.Join(root, "runtime", "mcpeach"),
		filepath.Join(root, "home"),
		filepath.Join(root, "logs"),
		filepath.Join(root, "artifacts"),
	} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	return &Fixture{t: t, Root: root}
}

// sanitizeTestName makes a test name safe for use as a directory segment.
func sanitizeTestName(name string) string {
	re := regexp.MustCompile(`[^A-Za-z0-9_-]+`)
	return re.ReplaceAllString(name, "_")
}

// Env returns the XDG-isolated environment for this fixture.
func (f *Fixture) Env(overrides ...string) []string {
	env := []string{
		"XDG_CONFIG_HOME=" + filepath.Join(f.Root, "config"),
		"XDG_RUNTIME_DIR=" + filepath.Join(f.Root, "runtime"),
		"HOME=" + filepath.Join(f.Root, "home"),
	}
	return append(env, overrides...)
}

// FakeServerConfig returns the common config map for a single enabled fake
// stdio server, used by most tests.
func FakeServerConfig(fakeBin string) map[string]map[string]any {
	return map[string]map[string]any{
		"fake": {"command": fakeBin, "enabled": true},
	}
}

// Setup starts a daemon and a TUI session against this fixture with the given
// config, returning both. It registers cleanup so tests do not repeat the
// boilerplate.
func (f *Fixture) Setup(t *testing.T, bin string, servers map[string]map[string]any, sessionName string, cols, rows int) (*Daemon, *Session) {
	t.Helper()
	f.WriteConfig(servers)
	d := StartDaemon(t, bin, f.Root, f.Env())
	t.Cleanup(d.Stop)
	s := NewSession(t, sessionName, bin, nil, f.Env(), cols, rows)
	t.Cleanup(s.Close)
	CollectArtifacts(t, s, d)
	return d, s
}

// ConfigPath returns the config file path.
func (f *Fixture) ConfigPath() string {
	return filepath.Join(f.Root, "config", "mcpeach", "mcpeach.yml")
}

// SocketPath returns the control socket path.
func (f *Fixture) SocketPath() string {
	return filepath.Join(f.Root, "runtime", "mcpeach", "mcpeach.sock")
}

// WriteConfig writes a YAML config with the given servers.
func (f *Fixture) WriteConfig(servers map[string]map[string]any) {
	f.t.Helper()
	var b []byte
	b = append(b, []byte("gateway:\n  port: 8080\nllm:\n  model: qwen2.5-coder:7b\nservers:\n")...)
	for name, sc := range servers {
		b = append(b, []byte(fmt.Sprintf("  %s:\n", name))...)
		for k, v := range sc {
			switch val := v.(type) {
			case string:
				b = append(b, []byte(fmt.Sprintf("    %s: %q\n", k, val))...)
			case bool:
				b = append(b, []byte(fmt.Sprintf("    %s: %v\n", k, val))...)
			case []string:
				b = append(b, []byte(fmt.Sprintf("    %s:\n", k))...)
				for _, item := range val {
					b = append(b, []byte(fmt.Sprintf("      - %q\n", item))...)
				}
			case map[string]string:
				b = append(b, []byte(fmt.Sprintf("    %s:\n", k))...)
				for ek, ev := range val {
					b = append(b, []byte(fmt.Sprintf("      %s: %q\n", ek, ev))...)
				}
			}
		}
	}
	if err := os.WriteFile(f.ConfigPath(), b, 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// WriteFile writes a file under the fixture root.
func (f *Fixture) WriteFile(rel, content string) string {
	f.t.Helper()
	p := filepath.Join(f.Root, rel)
	_ = os.MkdirAll(filepath.Dir(p), 0o700)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
	return p
}
