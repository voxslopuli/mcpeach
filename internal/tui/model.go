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
	StartServer(ctx context.Context, name string) error
	StopServer(ctx context.Context, name string) error
}

// Model is the Bubble Tea model for the mcpeach TUI.
type Model struct {
	client   clientIface
	servers  []client.ServerInfo
	tools    []string
	selected int
	width    int
	height   int
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
		case uv.KeyEscape, 'q':
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
	var b strings.Builder
	b.WriteString("mcpeach\n\n")
	for i, s := range m.servers {
		marker := " "
		if i == m.selected {
			marker = ">"
		}
		fmt.Fprintf(&b, "%s %-20s %s\n", marker, s.Name, s.State)
	}
	b.WriteString("\n↑/↓ select · enter start · space stop · q quit")
	return tea.NewView(b.String())
}
