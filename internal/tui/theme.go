package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme holds the lipgloss styles for the mcpeach TUI.
type Theme struct {
	Title    lipgloss.Style
	Selected lipgloss.Style
	Running  lipgloss.Style
	Stopped  lipgloss.Style
	Help     lipgloss.Style
	Header   lipgloss.Style
	Error    lipgloss.Style
}

// DefaultTheme returns the mcpeach color theme.
func DefaultTheme() Theme {
	return Theme{
		Title:    lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")).PaddingBottom(1),
		Selected: lipgloss.NewStyle().Foreground(lipgloss.Color("205")).Bold(true),
		Running:  lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		Stopped:  lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Help:     lipgloss.NewStyle().Foreground(lipgloss.Color("240")),
		Header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("205")),
		Error:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
	}
}

// renderServerRow renders a single server row with the theme.
func (t Theme) renderServerRow(name, state string, selected bool) string {
	marker := "  "
	style := lipgloss.NewStyle()
	if selected {
		marker = "> "
		style = t.Selected
	}
	nameStyled := style.Render(name)
	stateStyled := t.Stopped.Render(state)
	if state == "running" {
		stateStyled = t.Running.Render(state)
	}
	return marker + nameStyled + strings.Repeat(" ", 20-len(name)) + stateStyled
}
