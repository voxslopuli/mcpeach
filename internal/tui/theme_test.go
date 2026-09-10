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
