package tui

import (
	"context"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
)

// addServerForm is the huh form for adding a new MCP server.
type addServerForm struct {
	name      string
	command   string
	args      string
	transport string
	url       string
}

// buildAddServerForm constructs the huh form. On submit it returns the
// collected values via the form's Value pointers.
func buildAddServerForm(f *addServerForm) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Server name").
				Placeholder("e.g. github").
				Value(&f.name),
			huh.NewInput().
				Title("Command").
				Placeholder("e.g. npx -y @modelcontextprotocol/server-github").
				Value(&f.command),
			huh.NewInput().
				Title("Args (space-separated)").
				Placeholder("optional").
				Value(&f.args),
			huh.NewSelect[string]().
				Title("Transport").
				Options(
					huh.NewOption("stdio", "stdio"),
					huh.NewOption("streamable-http", "streamable-http"),
				).
				Value(&f.transport),
			huh.NewInput().
				Title("URL (for remote)").
				Placeholder("https://mcp.example.com/mcp").
				Value(&f.url),
		),
	)
}

// runAddServerForm returns a tea.Cmd that runs the huh form and submits the
// result to the control plane.
func (m *Model) runAddServerForm() tea.Cmd {
	return func() tea.Msg {
		if m.form == nil {
			m.form = &addServerForm{}
		}
		form := buildAddServerForm(m.form)
		if err := form.Run(); err != nil {
			return addServerDoneMsg{err: err}
		}
		return m.submitAddServer(m.form)()
	}
}

// submitAddServer sends the form values to the control plane.
func (m *Model) submitAddServer(f *addServerForm) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return addServerDoneMsg{}
		}
		var args []string
		if f.args != "" {
			args = splitArgs(f.args)
		}
		err := m.client.AddServer(context.Background(), f.name, f.command, args, nil)
		return addServerDoneMsg{err: err}
	}
}

// splitArgs splits a space-separated string into args, respecting quotes.
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	inQuote := false
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
		case r == ' ' && !inQuote:
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}
