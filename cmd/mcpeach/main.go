// Package main is the mcpeach CLI entrypoint.
package main

import (
	"context"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
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
	if err := fang.Execute(context.Background(), root); err != nil {
		os.Exit(1)
	}
}
