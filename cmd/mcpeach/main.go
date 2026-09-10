// Package main is the mcpeach CLI entrypoint.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"

	"github.com/mcpeach/mcpeach/internal/config"
	"github.com/mcpeach/mcpeach/internal/control"
	"github.com/mcpeach/mcpeach/internal/gateway"
	"github.com/mcpeach/mcpeach/internal/secrets"
	"github.com/mcpeach/mcpeach/internal/server"
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

	root.AddCommand(serveCmd(), installCmd(), uninstallCmd(), statusCmd())

	if err := fang.Execute(context.Background(), root); err != nil {
		os.Exit(1)
	}
}

// serveCmd runs the mcpeach daemon: it loads config, wires the gateway and
// control plane, and serves until the context is cancelled.
func serveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "serve",
		Short: "Run the mcpeach daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDaemon(cmd.Context())
		},
	}
}

// runDaemon wires and runs the mcpeach daemon until ctx is cancelled.
func runDaemon(ctx context.Context) error {
	cfg, err := config.Load(config.Path())
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	mgr := server.NewManager()
	res := secrets.NewResolver(secrets.NewKeyringStore())
	for name, sc := range cfg.Servers {
		mgr.Add(server.New(name))
		if sc.Enabled {
			env, err := res.ResolveEnv(sc.Env)
			if err != nil {
				return fmt.Errorf("resolve env for %s: %w", name, err)
			}
			if err := mgr.Start(ctx, name, sc.Command, sc.Args, env); err != nil {
				return fmt.Errorf("start %s: %w", name, err)
			}
		}
	}

	gw := gateway.New(cfg)
	streaming := gateway.NewStreamingServer(gw, cfg.Gateway.Name, cfg.Gateway.Version)

	// Mount the MCP streaming endpoint and the control plane.
	mux := http.NewServeMux()
	mux.Handle("/mcp", streaming)
	controlHandler := control.NewHandler(mgr, gw, cfg)
	ctrl := control.NewServer(config.SocketPath(), controlHandler)

	if err := ctrl.Start(ctx); err != nil {
		return fmt.Errorf("control plane: %w", err)
	}

	// Serve the MCP endpoint on the gateway address.
	srv := &http.Server{Addr: cfg.Gateway.Addr, Handler: mux}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Install mcpeach as a background service",
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := serviceManagerFactory()
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
			m, err := serviceManagerFactory()
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
			m, err := serviceManagerFactory()
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

// serviceManager is the subset of service.Manager the CLI commands use.
type serviceManager interface {
	Install() error
	Uninstall() error
	Status() (service.Status, error)
}

// newServiceManager builds a service manager wired to the daemon.
func newServiceManager() (serviceManager, error) {
	return service.NewManager(runDaemon, func() {})
}

// serviceManagerFactory is overridable in tests to inject a fake manager.
var serviceManagerFactory = newServiceManager

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
