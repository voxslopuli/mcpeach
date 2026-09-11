package tui

import (
	"context"
	"errors"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/mcpeach/mcpeach/internal/client"
)

// importExportMode distinguishes the two directions of the shared form.
type importExportMode int

const (
	ieImport importExportMode = iota
	ieExport
)

// importExportForm collects a single file path for import or export.
type importExportForm struct {
	path string
	mode importExportMode
}

// importExportMsg carries the result of an import/export operation.
type importExportMsg struct {
	mode importExportMode
	path string
	res  client.ImportResult
	err  error
}

// runImportExportForm returns a tea.Cmd that collects a path via a huh form,
// runs the daemon import/export, and emits the result.
func (m *Model) runImportExportForm(mode importExportMode) tea.Cmd {
	f := &importExportForm{mode: mode}
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("File path").
				Placeholder("/path/to/config.json").
				Validate(func(s string) error {
					if s == "" {
						return errors.New("path is required")
					}
					return nil
				}).
				Value(&f.path),
		),
	)

	return func() tea.Msg {
		if m.client == nil {
			return importExportMsg{mode: mode, err: errors.New("not connected to daemon")}
		}
		if err := form.Run(); err != nil {
			return importExportMsg{mode: mode, err: err}
		}
		return m.executeImportExport(f.path, mode)
	}
}

// executeImportExport runs the daemon import/export for a path and returns the
// result message. Split from runImportExportForm so it is testable without a
// TTY (the huh form needs one).
func (m *Model) executeImportExport(path string, mode importExportMode) tea.Msg {
	if m.client == nil {
		return importExportMsg{mode: mode, err: errors.New("not connected to daemon")}
	}
	ctx := context.Background()
	if mode == ieExport {
		// Safe default: preserve references, never resolve plaintext.
		err := m.client.Export(ctx, path, "references", false)
		return importExportMsg{mode: mode, path: path, err: err}
	}
	// ieImport: review conflicts, migrate likely secrets to keychain.
	res, err := m.client.Import(ctx, path, "review", "keychain")
	return importExportMsg{mode: mode, path: path, res: res, err: err}
}
