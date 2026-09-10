package tui

import (
	"strings"
	"testing"
)

func TestDefaultTheme(t *testing.T) {
	th := DefaultTheme()
	if th.Title.Render("x") == "" {
		t.Error("Title style renders empty")
	}
	if th.Selected.Render("x") == "" {
		t.Error("Selected style renders empty")
	}
	if th.Running.Render("x") == "" {
		t.Error("Running style renders empty")
	}
	if th.Stopped.Render("x") == "" {
		t.Error("Stopped style renders empty")
	}
	if th.Help.Render("x") == "" {
		t.Error("Help style renders empty")
	}
}

func TestRenderServerRow(t *testing.T) {
	th := DefaultTheme()

	// Selected row has a "> " marker.
	row := th.renderServerRow("github", "running", true)
	if !strings.HasPrefix(row, "> ") {
		t.Errorf("selected row = %q, want '> ' prefix", row)
	}

	// Unselected row has a "  " marker.
	row = th.renderServerRow("github", "stopped", false)
	if !strings.HasPrefix(row, "  ") {
		t.Errorf("unselected row = %q, want '  ' prefix", row)
	}
}

func TestRenderServerRowLongName(t *testing.T) {
	th := DefaultTheme()
	row := th.renderServerRow(strings.Repeat("x", 30), "stopped", false)
	if !strings.Contains(row, "…") {
		t.Errorf("long name not truncated with ellipsis: %q", row)
	}
	// State column stays at 2 (marker) + 20 (name column).
	if idx := strings.Index(row, "stopped"); idx != 22 {
		t.Errorf("state at %d, want 22: %q", idx, row)
	}
}

func TestRenderServerRowWideUnicode(t *testing.T) {
	th := DefaultTheme()
	row := th.renderServerRow("日本語のサーバー", "stopped", false)
	if row == "" {
		t.Fatal("wide unicode row rendered empty")
	}
	// 日本語のサーバー is 16 display cells wide; padding fills to 20.
	if idx := strings.Index(row, "stopped"); idx != 22 {
		t.Errorf("state at %d, want 22: %q", idx, row)
	}
}
