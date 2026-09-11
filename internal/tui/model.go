// Package tui is the mcpeach terminal UI. It is a thin client over the
// control-plane API: it lists servers, toggles them, and shows logs, process
// stats, and metrics.
package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/mcpeach/mcpeach/internal/client"
)

// clientIface is the subset of the control-plane client the TUI needs.
type clientIface interface {
	ListServers(ctx context.Context) ([]client.ServerInfo, error)
	ListTools(ctx context.Context) ([]string, error)
	ServerLogs(ctx context.Context, name string) ([]string, error)
	StartServer(ctx context.Context, name string) error
	StopServer(ctx context.Context, name string) error
	AddServer(ctx context.Context, req client.AddServerRequest) error
	UpdateServer(ctx context.Context, name string, req client.AddServerRequest) error
	DeleteServer(ctx context.Context, name string) error
	GetServer(ctx context.Context, name string) (client.ServerDetail, error)
	ListSecrets(ctx context.Context) ([]client.ServerSecrets, error)
	Import(ctx context.Context, path, conflictsPolicy, secretsPolicy string) (client.ImportResult, error)
	Export(ctx context.Context, path, secrets string, allowPlaintext bool) error
}

// view identifies the active TUI screen.
type view int

const (
	viewList    view = iota // server list (root)
	viewLogs                // per-server log viewer
	viewTools               // aggregated tool list
	viewForm                // add-server form
	viewDetail              // server management screen
	viewSecrets             // secret sources per server
)

// Model is the Bubble Tea model for the mcpeach TUI.
type Model struct {
	client        clientIface
	servers       []client.ServerInfo
	tools         []string
	logLines      []string
	selected      int
	view          view
	form          *addServerForm
	err           string
	status        string // transient informational message (not an error)
	detailName    string
	detail        *client.ServerDetail
	detailErr     string
	confirmDelete string // non-empty => delete confirmation armed for this server
	secrets       []client.ServerSecrets
	secretsErr    string
}

// NewModel builds a TUI model backed by the given control-plane client.
func NewModel(c clientIface) *Model {
	return &Model{client: c}
}

// Init returns the initial command (load servers).
func (m *Model) Init() tea.Cmd {
	return func() tea.Msg { return loadServersMsg{} }
}

// loadServersMsg is sent after the initial load.
type loadServersMsg struct{}

// serversLoadedMsg carries the result of a ListServers call.
type serversLoadedMsg struct {
	servers []client.ServerInfo
	err     error
}

// serverActionMsg is sent after a start/stop completes.
type serverActionMsg struct {
	name    string
	err     error
	deleted bool // set when the action was a successful delete
}

// logsLoadedMsg carries the result of a ServerLogs call. name identifies the
// server the lines belong to so a slow response for a previously selected
// server can be dropped instead of overwriting the current server's logs.
type logsLoadedMsg struct {
	name  string
	lines []string
	err   error
}

// toolsLoadedMsg carries the result of a ListTools call. name is the server
// selected when the request was issued, used to drop stale responses.
type toolsLoadedMsg struct {
	name  string
	tools []string
	err   error
}

// addServerDoneMsg is sent after an add-server form submits.
type addServerDoneMsg struct {
	err error
}

// loadLogsCmd returns a tea.Cmd that fetches logs for the named server. The
// name is captured as a string so the command is safe even if the selection
// or server list changes while the command is in flight.
func (m *Model) loadLogsCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return logsLoadedMsg{name: name}
		}
		lines, err := m.client.ServerLogs(context.Background(), name)
		if err != nil {
			return logsLoadedMsg{name: name, err: err}
		}
		return logsLoadedMsg{name: name, lines: lines}
	}
}

// secretsLoadedMsg carries the secret-source listing for the secrets view.
type secretsLoadedMsg struct {
	servers []client.ServerSecrets
	err     error
}

// loadSecretsCmd returns a tea.Cmd that fetches per-server secret sources.
func (m *Model) loadSecretsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return secretsLoadedMsg{err: fmt.Errorf("not connected to daemon")}
		}
		s, err := m.client.ListSecrets(context.Background())
		return secretsLoadedMsg{servers: s, err: err}
	}
}

// loadToolsCmd returns a tea.Cmd that fetches the aggregated tool list.
func (m *Model) loadToolsCmd() tea.Cmd {
	name := m.selectedServerName()
	return func() tea.Msg {
		if m.client == nil {
			return toolsLoadedMsg{name: name}
		}
		tools, err := m.client.ListTools(context.Background())
		if err != nil {
			return toolsLoadedMsg{name: name, err: err}
		}
		return toolsLoadedMsg{name: name, tools: tools}
	}
}

// selectedServerName returns the name of the currently selected server, or ""
// when the list is empty.
func (m *Model) selectedServerName() string {
	if len(m.servers) == 0 || m.selected < 0 || m.selected >= len(m.servers) {
		return ""
	}
	return m.servers[m.selected].Name
}

// loadServersCmd returns a tea.Cmd that fetches the server list asynchronously.
func (m *Model) loadServersCmd() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return serversLoadedMsg{}
		}
		servers, err := m.client.ListServers(context.Background())
		if err != nil {
			return serversLoadedMsg{err: err}
		}
		return serversLoadedMsg{servers: servers}
	}
}

// startServerCmd returns a tea.Cmd that starts a server asynchronously.
func (m *Model) startServerCmd(name string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if m.client != nil {
			err = m.client.StartServer(context.Background(), name)
		}
		return serverActionMsg{name: name, err: err}
	}
}

// stopServerCmd returns a tea.Cmd that stops a server asynchronously.
func (m *Model) stopServerCmd(name string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if m.client != nil {
			err = m.client.StopServer(context.Background(), name)
		}
		return serverActionMsg{name: name, err: err}
	}
}

// deleteServerCmd returns a tea.Cmd that deletes a server asynchronously.
func (m *Model) deleteServerCmd(name string) tea.Cmd {
	return func() tea.Msg {
		var err error
		if m.client != nil {
			err = m.client.DeleteServer(context.Background(), name)
		}
		return serverActionMsg{name: name, err: err, deleted: err == nil}
	}
}

// loadServers fetches the server list synchronously (used in tests).
func (m *Model) loadServers() {
	if m.client == nil {
		return
	}
	servers, err := m.client.ListServers(context.Background())
	if err != nil {
		m.err = err.Error()
		return
	}
	m.servers = servers
	m.clampSelection()
}

// clampSelection keeps m.selected within the bounds of the current server
// list. A refresh that shrinks the list would otherwise leave it out of range.
func (m *Model) clampSelection() {
	if m.selected >= len(m.servers) {
		m.selected = len(m.servers) - 1
	}
	if m.selected < 0 {
		m.selected = 0
	}
}

// selectNext moves the selection down.
func (m *Model) selectNext() {
	if len(m.servers) == 0 {
		return
	}
	m.selected = (m.selected + 1) % len(m.servers)
}

// selectPrev moves the selection up.
func (m *Model) selectPrev() {
	if len(m.servers) == 0 {
		return
	}
	m.selected = (m.selected - 1 + len(m.servers)) % len(m.servers)
}

// startSelected starts the currently selected server asynchronously.
func (m *Model) startSelected() tea.Cmd {
	if m.client == nil || len(m.servers) == 0 {
		return nil
	}
	name := m.servers[m.selected].Name
	return m.startServerCmd(name)
}

// stopSelected stops the currently selected server asynchronously.
func (m *Model) stopSelected() tea.Cmd {
	if m.client == nil || len(m.servers) == 0 {
		return nil
	}
	name := m.servers[m.selected].Name
	return m.stopServerCmd(name)
}

// toggleSelected starts or stops the selected server based on its current
// state: stopped/error → start, running → stop. Returns nil when there is
// nothing to do (no client, empty list, or a transitional state).
func (m *Model) toggleSelected() tea.Cmd {
	if m.client == nil || len(m.servers) == 0 || m.selected >= len(m.servers) {
		return nil
	}
	s := m.servers[m.selected]
	switch s.State {
	case "running":
		return m.stopServerCmd(s.Name)
	case "stopped", "error":
		return m.startServerCmd(s.Name)
	default:
		// Transitional or unknown state: no-op with visible status.
		m.status = fmt.Sprintf("server %s is %s; waiting", s.Name, s.State)
		return nil
	}
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadServersMsg:
		return m, m.loadServersCmd()
	case serversLoadedMsg:
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.servers = msg.servers
		m.clampSelection()
		return m, nil
	case serverActionMsg:
		// After a start/stop/delete, refresh the server list. Surface any error.
		if msg.err != nil {
			m.err = msg.err.Error()
		} else if msg.deleted {
			// A successful delete returns to the list.
			m.view = viewList
			m.detail = nil
			m.detailName = ""
		}
		return m, m.loadServersCmd()
	case logsLoadedMsg:
		// Drop a response for a server that is no longer selected BEFORE
		// surfacing its error, so a stale failure cannot appear for the
		// now-selected server.
		if msg.name != m.selectedServerName() {
			return m, nil
		}
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.logLines = msg.lines
		return m, nil
	case toolsLoadedMsg:
		// ListTools is aggregated and global, so the response is never stale
		// regardless of selection changes.
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		m.err = ""
		m.tools = msg.tools
		return m, nil
	case detailLoadMsg:
		return m, m.loadDetailCmd(m.detailName)
	case detailLoadedMsg:
		// Drop a response for a server that is no longer selected BEFORE
		// surfacing its error.
		if msg.name != m.detailName {
			return m, nil
		}
		if msg.err != nil {
			m.detailErr = msg.err.Error()
			return m, nil
		}
		m.detail = msg.detail
		m.detailErr = ""
		return m, nil
	case secretsLoadedMsg:
		if msg.err != nil {
			m.secretsErr = msg.err.Error()
			return m, nil
		}
		m.secrets = msg.servers
		m.secretsErr = ""
		return m, nil
	case importExportMsg:
		m.view = viewList
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		switch msg.mode {
		case ieImport:
			sum := fmt.Sprintf("imported %s from %s", formatImported(msg.res), msg.path)
			if len(msg.res.Conflicts) > 0 {
				sum += fmt.Sprintf(" (kept existing: %s)", strings.Join(msg.res.Conflicts, ", "))
			}
			m.status = sum
		case ieExport:
			m.status = "exported to " + msg.path + " (references preserved)"
		}
		return m, m.loadServersCmd()
	case addServerDoneMsg:
		m.view = viewList
		if msg.err != nil {
			m.err = msg.err.Error()
			return m, nil
		}
		// Refresh the server list after a successful add.
		return m, m.loadServersCmd()
	case tea.KeyPressMsg:
		switch msg.Code {
		case tea.KeyUp:
			m.selectPrev()
		case tea.KeyDown:
			m.selectNext()
		case uv.KeySpace:
			// State-aware lifecycle toggle: stopped/error → start, running → stop.
			if cmd := m.toggleSelected(); cmd != nil {
				return m, cmd
			}
		case uv.KeyEnter:
			// Opens the server management screen for the selected server.
			if m.view == viewList {
				if m.client == nil || len(m.servers) == 0 {
					return m, nil
				}
				// Capture the server NAME, not the index: the list may refresh
				// while the detail screen is open.
				m.detailName = m.servers[m.selected].Name
				m.detail = nil
				m.detailErr = ""
				m.view = viewDetail
				return m, func() tea.Msg { return detailLoadMsg{} }
			}
			return m, nil
		case 'l':
			m.confirmDelete = ""
			switch m.view {
			case viewList:
				m.view = viewLogs
				if len(m.servers) > 0 {
					return m, m.loadLogsCmd(m.servers[m.selected].Name)
				}
			case viewLogs:
				m.view = viewList
			}
		case 't':
			m.confirmDelete = ""
			switch m.view {
			case viewList:
				m.view = viewTools
				return m, m.loadToolsCmd()
			case viewTools:
				m.view = viewList
			}
		case 'n':
			m.confirmDelete = ""
			if m.view == viewList {
				m.view = viewForm
				m.form = &addServerForm{enabled: true}
				return m, m.runAddServerForm()
			}
		case 's':
			m.confirmDelete = ""
			if m.view == viewList {
				m.view = viewSecrets
				return m, m.loadSecretsCmd()
			}
			if m.view == viewSecrets {
				m.view = viewList
			}
		case 'i':
			m.confirmDelete = ""
			if m.view == viewList {
				return m, m.runImportExportForm(ieImport)
			}
		case 'x':
			m.confirmDelete = ""
			if m.view == viewList {
				return m, m.runImportExportForm(ieExport)
			}
		case 'e':
			// Edit the server shown on the detail screen.
			m.confirmDelete = ""
			if m.view == viewDetail && m.detailName != "" {
				m.view = viewForm
				return m, m.runEditServerForm(m.detailName)
			}
		case 'd':
			// Arm the delete confirmation for the server on the detail screen.
			if m.view == viewDetail && m.detailName != "" {
				m.confirmDelete = m.detailName
				return m, nil
			}
		case 'y':
			// Confirm the armed deletion.
			if m.confirmDelete != "" {
				name := m.confirmDelete
				m.confirmDelete = ""
				return m, m.deleteServerCmd(name)
			}
		case uv.KeyEscape, 'q':
			// While a delete confirmation is armed, only esc cancels it (the
			// prompt says [esc]); 'q' is inert so it cannot be mistaken for
			// confirmation or accidentally quit.
			if m.confirmDelete != "" {
				if msg.Code == uv.KeyEscape {
					m.confirmDelete = ""
					return m, nil
				}
				return m, nil
			}
			// The form owns text input: esc cancels it, but 'q' must be inert so
			if m.view == viewForm {
				if msg.Code == uv.KeyEscape {
					m.view = viewList
					return m, nil
				}
				return m, nil
			}
			// From a sub-view, esc/q returns to the list; from the list it quits.
			if m.view != viewList {
				m.view = viewList
				m.detail = nil
				m.detailErr = ""
				return m, nil
			}
			return m, tea.Quit
		}
		// Ctrl+C quits.
		if msg.Mod.Contains(uv.ModCtrl) && msg.Code == 'c' {
			return m, tea.Quit
		}
	}
	return m, nil
}

// View renders the TUI.
func (m *Model) View() tea.View {
	theme := DefaultTheme()
	var b strings.Builder
	b.WriteString(theme.Title.Render("mcpeach") + "\n")

	if m.view == viewDetail {
		m.renderDetail(&b)
		return tea.NewView(b.String())
	}

	if m.view == viewForm {
		b.WriteString(theme.Header.Render("Add server form") + "\n\n")
		b.WriteString("Fill in the fields and press enter to submit.\n")
		b.WriteString(theme.Help.Render("esc/q back") + "\n")
		return tea.NewView(b.String())
	}

	if m.view == viewLogs {
		if len(m.servers) == 0 || m.selected >= len(m.servers) {
			b.WriteString(theme.Help.Render("No server selected") + "\n\n")
			b.WriteString(theme.Help.Render("l toggle logs · esc/q back") + "\n")
			return tea.NewView(b.String())
		}
		b.WriteString(theme.Header.Render("Logs for "+m.servers[m.selected].Name) + "\n\n")
		for _, line := range m.logLines {
			b.WriteString(line + "\n")
		}
		m.renderError(&b, theme)
		b.WriteString("\n" + theme.Help.Render("l toggle logs · esc/q back") + "\n")
		return tea.NewView(b.String())
	}

	if m.view == viewTools {
		b.WriteString(theme.Header.Render("Tools") + "\n\n")
		for _, tool := range m.tools {
			b.WriteString(tool + "\n")
		}
		m.renderError(&b, theme)
		b.WriteString("\n" + theme.Help.Render("t toggle tools · esc/q back") + "\n")
		return tea.NewView(b.String())
	}

	if m.view == viewSecrets {
		b.WriteString(theme.Header.Render("Secrets") + "\n\n")
		if m.secretsErr != "" {
			b.WriteString(theme.Error.Render(m.secretsErr) + "\n")
		} else {
			for _, ss := range m.secrets {
				b.WriteString(theme.Title.Render(ss.Server) + "\n")
				if len(ss.Sources) == 0 {
					b.WriteString("  (no environment variables)\n")
				}
				for _, src := range ss.Sources {
					b.WriteString("  " + src.Name + "  " + theme.Help.Render(sourceStatus(src)) + "\n")
				}
			}
		}
		m.renderError(&b, theme)
		b.WriteString("\n" + theme.Help.Render("s toggle secrets · esc/q back") + "\n")
		return tea.NewView(b.String())
	}

	for i, s := range m.servers {
		b.WriteString(theme.renderServerRow(s.Name, s.State, i == m.selected) + "\n")
	}
	m.renderError(&b, theme)
	if m.status != "" {
		b.WriteString("\n" + theme.Help.Render(m.status) + "\n")
	}
	b.WriteString("\n" + theme.Help.Render("↑/↓ select · space start/stop · enter manage · n new · i import · x export · s secrets · l logs · q quit"))
	return tea.NewView(b.String())
}

// formatImported renders the imported server names for a status line.
func formatImported(res client.ImportResult) string {
	if len(res.Imported) == 0 {
		return "no servers"
	}
	return strings.Join(res.Imported, ", ")
}

// sourceStatus describes an env source's kind and resolvability without
// leaking its value.
func sourceStatus(src client.SecretSource) string {
	switch {
	case src.Reference == "":
		return "literal"
	case strings.HasPrefix(src.Reference, "keychain:"):
		if src.Resolvable {
			return "keychain"
		}
		return "keychain (missing)"
	case strings.HasPrefix(src.Reference, "env:"):
		if src.Resolvable {
			return "env"
		}
		return "env (missing)"
	default:
		return "literal"
	}
}

// renderError appends the current error line to the view buffer, if any.
func (m *Model) renderError(b *strings.Builder, theme Theme) {
	if m.err != "" {
		b.WriteString("\n" + theme.Error.Render(m.err) + "\n")
	}
}
