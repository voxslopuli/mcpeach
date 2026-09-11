package tui

import (
	"testing"

	"github.com/mcpeach/mcpeach/e2e/harness"
)

// TestResizeNarrow verifies the TUI renders a deliberate compact view at a
// narrow width without crashing.
func TestResizeNarrow(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	fx.WriteConfig(harness.FakeServerConfig(fake))
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	// Start at a narrow width.
	s := harness.NewSession(t, "resize", binPath, nil, fx.Env(), 40, 20)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	// The TUI still renders the server list.
	s.ExpectServerRunning()
}

// TestResizeLongServerName verifies a long server name does not panic the
// renderer.
func TestResizeLongServerName(t *testing.T) {
	fx := harness.NewFixture(t)
	fake := harness.BuildFakeMCP(t)
	longName := "averylongservernamethatmightoverflowthelayoutboundaries"
	fx.WriteConfig(map[string]map[string]any{
		longName: {"command": fake, "enabled": true},
	})
	d := harness.StartDaemon(t, binPath, fx.Root, fx.Env())
	defer d.Stop()

	s := harness.NewSession(t, "longname", binPath, nil, fx.Env(), 80, 24)
	defer s.Close()
	harness.CollectArtifacts(t, s, d)

	// The long name renders (truncated with an ellipsis) without crashing.
	s.Expect("averylongservername")
}
