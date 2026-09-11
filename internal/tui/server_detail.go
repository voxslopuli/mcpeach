package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/mcpeach/mcpeach/internal/client"
)

// detailLoadMsg triggers an async load of the detail screen's data.
type detailLoadMsg struct{}

// detailLoadedMsg carries the result of a GetServer call.
type detailLoadedMsg struct {
	detail *client.ServerDetail
	err    error
}

// loadDetailCmd returns a tea.Cmd that fetches the detail for the named
// server. The name is captured as a string so the command is safe even if the
// selection or server list changes while the command is in flight.
func (m *Model) loadDetailCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return detailLoadedMsg{}
		}
		d, err := m.client.GetServer(context.Background(), name)
		if err != nil {
			return detailLoadedMsg{err: err}
		}
		return detailLoadedMsg{detail: &d}
	}
}

// renderDetail renders the server detail screen.
func (m *Model) renderDetail(b *strings.Builder) {
	theme := DefaultTheme()
	if m.detail == nil {
		if m.detailErr != "" {
			b.WriteString(theme.Error.Render(m.detailErr) + "\n")
		} else {
			b.WriteString("Loading " + m.detailName + "…\n")
		}
		b.WriteString(theme.Help.Render("esc back") + "\n")
		return
	}
	d := m.detail
	state := theme.Stopped.Render(d.State)
	if d.State == "running" {
		state = theme.Running.Render(d.State)
	}
	b.WriteString(theme.Title.Render("Server: "+d.Name) + " " + state + "\n\n")

	row := func(label, value string) {
		if value == "" {
			return
		}
		b.WriteString(theme.Header.Render(padLabel(label)) + value + "\n")
	}
	row("Transport", d.Transport)
	if d.Command != "" {
		row("Command", d.Command)
		if len(d.Args) > 0 {
			row("Arguments", strings.Join(d.Args, " "))
		}
	}
	if d.URL != "" {
		row("Endpoint", d.URL)
	}
	row("Configured", enabledLabel(d.Enabled))
	row("Runtime", d.State)
	row("MCP tools", fmt.Sprint(d.ToolCount))
	for _, kv := range sortedEnv(d.Env) {
		b.WriteString(theme.Header.Render(padLabel("Env "+kv[0])) + kv[1] + "\n")
	}
	b.WriteString("\n" + theme.Help.Render("esc back") + "\n")
}

// enabledLabel renders the configured state.
func enabledLabel(enabled bool) string {
	if enabled {
		return "enabled"
	}
	return "disabled"
}

// sortedEnv returns env entries sorted by key for stable rendering.
func sortedEnv(env map[string]string) [][2]string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([][2]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, [2]string{k, env[k]})
	}
	return out
}

// padLabel pads a label to a fixed column width.
func padLabel(label string) string {
	const width = 16
	for len(label) < width {
		label += " "
	}
	return label
}
