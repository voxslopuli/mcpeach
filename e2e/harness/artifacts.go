package harness

import (
	"os"
	"path/filepath"
	"testing"
)

// CollectArtifacts saves terminal text and API state after a failure so the
// failure is reproducible. Call via t.Cleanup in each test.
func CollectArtifacts(t *testing.T, s *Session, d *Daemon) {
	t.Helper()
	t.Cleanup(func() {
		if t.Failed() && s != nil {
			s.SaveArtifact("terminal.txt", s.Text())
			if d != nil {
				s.SaveArtifact("servers.json", d.APIJSON("GET", "/v0/servers", ""))
			}
		}
	})
}

// SaveFile writes content to a path under the session artifacts dir.
func SaveFile(t *testing.T, s *Session, name, content string) {
	t.Helper()
	p := filepath.Join(s.ArtifactsDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Logf("write artifact %s: %v", name, err)
	}
}
