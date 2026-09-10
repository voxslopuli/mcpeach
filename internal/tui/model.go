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

// loadServers fetches the server list from the control plane.
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

// startSelected starts the currently selected server.
func (m *Model) startSelected() {
	if m.client == nil || len(m.servers) == 0 {
		return
	}
	name := m.servers[m.selected].Name
	m.client.StartServer(context.Background(), name)
	m.loadServers()
}

// stopSelected stops the currently selected server.
func (m *Model) stopSelected() {
	if m.client == nil || len(m.servers) == 0 {
		return
	}
	name := m.servers[m.selected].Name
	m.client.StopServer(context.Background(), name)
	m.loadServers()
}

// Update handles messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case loadServersMsg:
		m.loadServers()
		return m, nil
	case tea.KeyPressMsg:
		switch msg.Code {
		case tea.KeyUp:
			m.selectPrev()
		case tea.KeyDown:
			m.selectNext()
		case uv.KeyEnter:
			m.startSelected()
		case uv.KeySpace:
			m.stopSelected()
		case uv.KeyEscape:
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
