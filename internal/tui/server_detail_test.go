package tui

import (
	"strings"
	"testing"

	"github.com/mcpeach/mcpeach/internal/client"
)

func TestRenderDetailProcessAndTools(t *testing.T) {
	m := NewModel(&fakeClient{})
	m.view = viewDetail
	m.detailName = "srv"
	m.detail = &client.ServerDetail{
		Name:      "srv",
		State:     "running",
		Transport: "stdio",
		Command:   "npx",
		ToolCount: 2,
		Tools:     []string{"srv__read", "srv__write"},
		Process:   &client.ProcessInfo{PID: 42173, CPUPercent: 2.5, RSSBytes: 86402662, Ports: []string{"127.0.0.1:49152"}},
	}
	v := m.View().Content
	for _, want := range []string{"PID", "42173", "2.5%", "MiB", "srv__read", "srv__write", "Ports"} {
		if !strings.Contains(v, want) {
			t.Errorf("detail missing %q: %s", want, v)
		}
	}
	// MCP tools and process info must be distinct sections.
	if !strings.Contains(v, "MCP Tools") || !strings.Contains(v, "Process") {
		t.Errorf("detail missing section headers: %s", v)
	}
}

func TestRenderDetailRemoteNoProcess(t *testing.T) {
	m := NewModel(&fakeClient{})
	m.view = viewDetail
	m.detailName = "ctx"
	m.detail = &client.ServerDetail{Name: "ctx", State: "running", Transport: "streamable-http", URL: "https://x/mcp"}
	v := m.View().Content
	if !strings.Contains(v, "remote or no local process") {
		t.Errorf("detail should note remote has no local process: %s", v)
	}
}

func TestFormatBytes(t *testing.T) {
	cases := map[uint64]string{
		512:        "512 B",
		2048:       "2.0 KiB",
		2 << 20:    "2.0 MiB",
		3 << 30:    "3.0 GiB",
	}
	for n, want := range cases {
		if got := formatBytes(n); got != want {
			t.Errorf("formatBytes(%d) = %s, want %s", n, got, want)
		}
	}
}
