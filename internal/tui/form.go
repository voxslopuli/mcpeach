package tui

import (
	"context"
	"errors"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mcpeach/mcpeach/internal/client"
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
// collected values via the form's Value pointers. Field-level Validate
// callbacks give immediate feedback before the form closes.
func buildAddServerForm(f *addServerForm) *huh.Form {
	return huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Server name").
				Placeholder("e.g. github").
				Validate(func(s string) error {
					if s == "" {
						return errors.New("name is required")
					}
					if strings.Contains(s, "__") {
						return errors.New("name cannot contain '__'")
					}
					return nil
				}).
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
					huh.NewOption("sse", "sse"),
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

// submitAddServer sends the form values to the control plane. A bounded
// context prevents the TUI from hanging if the control plane is unresponsive.
func (m *Model) submitAddServer(f *addServerForm) tea.Cmd {
	return func() tea.Msg {
		if err := validateAddServer(f); err != nil {
			return addServerDoneMsg{err: err}
		}
		if m.client == nil {
			return addServerDoneMsg{}
		}
		var args []string
		if f.args != "" {
			args = splitArgs(f.args)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		err := m.client.AddServer(ctx, client.AddServerRequest{
			Name:      f.name,
			Command:   f.command,
			Args:      args,
			Env:       nil, // the form does not collect env vars yet
			URL:       f.url,
			Transport: f.transport,
		})
		return addServerDoneMsg{err: err}
	}
}

// validateAddServer checks transport-specific requirements before submission.
// stdio (or unset) servers need a command; remote transports need a URL.
func validateAddServer(f *addServerForm) error {
	if f.name == "" {
		return errors.New("name is required")
	}
	if f.transport == "" || f.transport == "stdio" {
		if f.command == "" {
			return errors.New("command is required for stdio servers")
		}
		return nil
	}
	if f.url == "" {
		return errors.New("url is required for remote servers")
	}
	return nil
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
