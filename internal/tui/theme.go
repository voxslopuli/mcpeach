package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
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

// serverNameWidth is the display width reserved for the server name column.
const serverNameWidth = 20

// renderServerRow renders a single server row with the theme.
func (t Theme) renderServerRow(name, state string, selected bool) string {
	marker := "  "
	style := lipgloss.NewStyle()
	if selected {
		marker = "> "
		style = t.Selected
	}
	name = truncateWidth(name, serverNameWidth)
	nameStyled := style.Render(name)
	stateStyled := t.Stopped.Render(state)
	if state == "running" {
		stateStyled = t.Running.Render(state)
	}
	pad := serverNameWidth - runewidth.StringWidth(name)
	if pad < 0 {
		pad = 0
	}
	return marker + nameStyled + strings.Repeat(" ", pad) + stateStyled
}

// truncateWidth truncates s to at most width display cells, appending an
// ellipsis when it does not fit. It is rune-width aware so wide (CJK) runes
// do not break column alignment.
func truncateWidth(s string, width int) string {
	if runewidth.StringWidth(s) <= width {
		return s
	}
	remaining := width - 1 // reserve one cell for the ellipsis
	var b strings.Builder
	for _, r := range s {
		rw := runewidth.RuneWidth(r)
		if remaining-rw < 0 {
			break
		}
		b.WriteRune(r)
		remaining -= rw
	}
	return b.String() + "…"
}
