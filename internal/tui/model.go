// Package tui is the mcpeach terminal UI. It is a thin client over the
// control-plane API: it lists servers, toggles them, and shows logs, process
// stats, and metrics.
package tui

import (
	"context"
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
	AddServer(ctx context.Context, name, command string, args []string, env map[string]string) error
}

// Model is the Bubble Tea model for the mcpeach TUI.
type Model struct {
	client    clientIface
	servers   []client.ServerInfo
	tools     []string
	logLines  []string
	selected  int
	showLogs  bool
	showTools bool
	showForm  bool
	form      *addServerForm
	width     int
	height    int
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
}

// serverActionMsg is sent after a start/stop completes.
type serverActionMsg struct {
	name string
}

// logsLoadedMsg carries the result of a ServerLogs call.
type logsLoadedMsg struct {
	lines []string
}

// toolsLoadedMsg carries the result of a ListTools call.
type toolsLoadedMsg struct {
	tools []string
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
			return logsLoadedMsg{}
		}
		lines, err := m.client.ServerLogs(context.Background(), name)
		if err != nil {
			return logsLoadedMsg{}
		}
		return logsLoadedMsg{lines: lines}
	}
}

// loadToolsCmd returns a tea.Cmd that fetches the aggregated tool list.
func (m *Model) loadToolsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return toolsLoadedMsg{}
		}
		tools, err := m.client.ListTools(context.Background())
		if err != nil {
			return toolsLoadedMsg{}
		}
		return toolsLoadedMsg{tools: tools}
	}
}

// loadServersCmd returns a tea.Cmd that fetches the server list asynchronously.
func (m *Model) loadServersCmd() tea.Cmd {
	return func() tea.Msg {
		if m.client == nil {
			return serversLoadedMsg{}
		}
		servers, err := m.client.ListServers(context.Background())
		if err != nil {
			return serversLoadedMsg{}
		}
		return serversLoadedMsg{servers: servers}
	}
}

// startServerCmd returns a tea.Cmd that starts a server asynchronously.
func (m *Model) startServerCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if m.client != nil {
			m.client.StartServer(context.Background(), name)
		}
		return serverActionMsg{name: name}
	}
}

// stopServerCmd returns a tea.Cmd that stops a server asynchronously.
func (m *Model) stopServerCmd(name string) tea.Cmd {
	return func() tea.Msg {
		if m.client != nil {
			m.client.StopServer(context.Background(), name)
		}
		return serverActionMsg{name: name}
	}
}

// loadServers fetches the server list synchronously (used in tests).
func (m *Model) loadServers() {
	if m.client == nil {
		return
	}
	servers, err := m.client.ListServers(context.Background())
	if err != nil {
		return
	}
	m.servers = servers
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

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadServersMsg:
		return m, m.loadServersCmd()
	case serversLoadedMsg:
		m.servers = msg.servers
		return m, nil
	case serverActionMsg:
		// After a start/stop, refresh the server list.
		return m, m.loadServersCmd()
	case logsLoadedMsg:
		m.logLines = msg.lines
		return m, nil
	case toolsLoadedMsg:
		m.tools = msg.tools
		return m, nil
	case addServerDoneMsg:
		m.showForm = false
		if msg.err == nil {
			// Refresh the server list after a successful add.
			return m, m.loadServersCmd()
		}
		return m, nil
	case tea.KeyPressMsg:
		switch msg.Code {
		case tea.KeyUp:
			m.selectPrev()
		case tea.KeyDown:
			m.selectNext()
		case uv.KeyEnter:
			return m, m.startSelected()
		case uv.KeySpace:
			return m, m.stopSelected()
		case 'l':
			m.showLogs = !m.showLogs
			if m.showLogs && len(m.servers) > 0 {
				return m, m.loadLogsCmd(m.servers[m.selected].Name)
			}
		case 't':
			m.showTools = !m.showTools
			if m.showTools {
				return m, m.loadToolsCmd()
			}
		case 'a':
			m.showForm = !m.showForm
			if m.showForm {
				m.form = &addServerForm{}
				return m, m.runAddServerForm()
			}
		case uv.KeyEscape, 'q':
			// If a sub-view is active, esc/q toggles back to the list.
			if m.showLogs || m.showTools || m.showForm {
				m.showLogs = false
				m.showTools = false
				m.showForm = false
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

	if m.showForm {
		b.WriteString(theme.Header.Render("Add server form") + "\n\n")
		b.WriteString("Fill in the fields and press enter to submit.\n")
		b.WriteString(theme.Help.Render("esc/q back") + "\n")
		return tea.NewView(b.String())
	}

	if m.showLogs {
		if len(m.servers) == 0 || m.selected >= len(m.servers) {
			b.WriteString(theme.Help.Render("No server selected") + "\n\n")
			b.WriteString(theme.Help.Render("l toggle logs · esc/q back") + "\n")
			return tea.NewView(b.String())
		}
		b.WriteString(theme.Header.Render("Logs for "+m.servers[m.selected].Name) + "\n\n")
		for _, line := range m.logLines {
			b.WriteString(line + "\n")
		}
		b.WriteString("\n" + theme.Help.Render("l toggle logs · esc/q back") + "\n")
		return tea.NewView(b.String())
	}

	if m.showTools {
		b.WriteString(theme.Header.Render("Tools") + "\n\n")
		for _, tool := range m.tools {
			b.WriteString(tool + "\n")
		}
		b.WriteString("\n" + theme.Help.Render("t toggle tools · esc/q back") + "\n")
		return tea.NewView(b.String())
	}

	for i, s := range m.servers {
		b.WriteString(theme.renderServerRow(s.Name, s.State, i == m.selected) + "\n")
	}
	b.WriteString("\n" + theme.Help.Render("↑/↓ select · enter start · space stop · l logs · t tools · a add · q quit"))
	return tea.NewView(b.String())
}
