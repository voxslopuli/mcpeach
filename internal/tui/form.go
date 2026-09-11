package tui

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mcpeach/mcpeach/internal/client"
)

// addServerForm is the huh form for adding a new MCP server. The same form
// powers edit mode: when editName is non-empty the form prepopulates from the
// server's detail and submits via UpdateServer instead of AddServer.
type addServerForm struct {
	name      string
	command   string
	args      string
	transport string
	url       string
	editName  string // non-empty => edit mode for this server
}

// validateServerName is the huh field-level validator for the server name.
func validateServerName(s string) error {
	if s == "" {
		return errors.New("name is required")
	}
	if strings.Contains(s, "__") {
		return errors.New("name cannot contain '__'")
	}
	return nil
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
				Validate(validateServerName).
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

// runEditServerForm opens the shared form in edit mode for the named server,
// prepopulating every field from its detail. Env values arrive as source
// references, never resolved secrets.
func (m *Model) runEditServerForm(name string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return addServerDoneMsg{err: errors.New("client unavailable")}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		d, err := m.client.GetServer(ctx, name)
		if err != nil {
			return addServerDoneMsg{err: err}
		}
		f := editFormFromDetail(&d, name)
		m.form = f
		form := buildAddServerForm(f)
		if err := form.Run(); err != nil {
			return addServerDoneMsg{err: err}
		}
		return m.submitAddServer(f)()
	}
}

// editFormFromDetail builds a prefilled edit-mode form from a server detail.
// Separated from runEditServerForm so tests can exercise the prefill without
// running the interactive huh form.
func editFormFromDetail(d *client.ServerDetail, originalName string) *addServerForm {
	return &addServerForm{
		name:      d.Name,
		command:   d.Command,
		args:      strings.Join(d.Args, " "),
		transport: d.Transport,
		url:       d.URL,
		editName:  originalName,
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
			parsed, err := splitArgs(f.args)
			if err != nil {
				return addServerDoneMsg{err: err}
			}
			args = parsed
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		req := client.AddServerRequest{
			Name:      f.name,
			Command:   f.command,
			Args:      args,
			Env:       nil, // the form does not collect env vars yet
			URL:       f.url,
			Transport: f.transport,
			Enabled:   true,
		}
		var err error
		if f.editName != "" {
			err = m.client.UpdateServer(ctx, f.editName, req)
		} else {
			err = m.client.AddServer(ctx, req)
		}
		return addServerDoneMsg{err: err}
	}
}

// validateAddServer checks transport-specific requirements before submission.
// stdio (or unset) servers need a command; remote transports need a URL.
func validateAddServer(f *addServerForm) error {
	if f.name == "" {
		return errors.New("name is required")
	}
	if f.command != "" && f.url != "" {
		return errors.New("cannot set both command and url")
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

// splitArgs splits s into arguments on unicode whitespace (spaces, tabs,
// newlines) outside quotes. Supported quoting is deliberately minimal: a
// double-quoted span groups whitespace into one argument and the quotes are
// stripped; adjacent quotes concatenate ("a""b" -> ab). Single quotes and
// escape sequences are not special. An unterminated double quote is an error.
func splitArgs(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inQuote := false
	started := false // cur holds an argument, possibly empty (e.g. "")
	for _, r := range s {
		switch {
		case r == '"':
			inQuote = !inQuote
			started = true
		case unicode.IsSpace(r) && !inQuote:
			if started {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
			started = true
		}
	}
	if inQuote {
		return nil, errors.New("unclosed quote in args")
	}
	if started {
		out = append(out, cur.String())
	}
	return out, nil
}
