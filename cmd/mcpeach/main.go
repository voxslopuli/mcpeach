// Package main is the mcpeach CLI entrypoint.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/mcpeach/mcpeach/internal/service"
)

// version is set at build time via -ldflags.
var version = "dev"

func main() {
	root := &cobra.Command{
		Use:   "mcpeach",
		Short: "A local MCP gateway with a TUI",
		Long: `mcpeach aggregates multiple MCP servers (stdio + remote) behind a
single streamable-HTTP endpoint, permissions tools, and exposes curated
tool groups to clients.`,
		Version: version,
	}

	root.AddCommand(installCmd(), uninstallCmd(), statusCmd())

	if err := fang.Execute(context.Background(), root); err != nil {
		os.Exit(1)
	}
}

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install mcpeach as a background service",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := service.NewManager()
			if err != nil {
				return err
			}
			if err := m.Install(); err != nil {
				return err
			}
			fmt.Println("mcpeach service installed")
			return nil
		},
	}
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Remove the mcpeach background service",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := service.NewManager()
			if err != nil {
				return err
			}
			if err := m.Uninstall(); err != nil {
				return err
			}
			fmt.Println("mcpeach service uninstalled")
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the mcpeach service status",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := service.NewManager()
			if err != nil {
				return err
			}
			st, err := m.Status()
			if err != nil {
				return err
			}
			fmt.Printf("mcpeach service: %s\n", statusString(st))
			return nil
		},
	}
}

// statusString renders a service status for display.
func statusString(st service.Status) string {
	switch st {
	case service.StatusRunning:
		return "running"
	case service.StatusStopped:
		return "stopped"
	default:
		return "unknown"
	}
}
