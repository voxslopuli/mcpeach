// Package main is the mcpeach CLI entrypoint.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/fang"
	"github.com/mark3labs/mcp-go/client"
	"github.com/spf13/cobra"

	mcclient "github.com/voxslopuli/mcpeach/internal/client"
	"github.com/voxslopuli/mcpeach/internal/config"
	"github.com/voxslopuli/mcpeach/internal/connect"
	"github.com/voxslopuli/mcpeach/internal/control"
	"github.com/voxslopuli/mcpeach/internal/gateway"
	"github.com/voxslopuli/mcpeach/internal/mcpconfig"
	"github.com/voxslopuli/mcpeach/internal/secrets"
	"github.com/voxslopuli/mcpeach/internal/server"
	"github.com/voxslopuli/mcpeach/internal/service"
	"github.com/voxslopuli/mcpeach/internal/tui"
)

// version is set at build time via -ldflags.
var version = "dev"

// debug enables verbose output and raw error messages (set via --debug).
var debug bool

// #pragma: no cover — entrypoint; calling it would os.Exit the test process
func main() {
	root := newRootCommand()

	// executeRoot runs the CLI with signal-aware context cancellation.
	// WithNotifySignal installs a signal.NotifyContext over ctx so SIGINT and
	// SIGTERM cancel the command context. Without it, every shutdown goroutine
	// keyed on ctx.Done() (control-socket cleanup, gateway.Close) is dead code
	// and stdio subprocesses leak on Ctrl-C or a launchd/systemd stop.
	if err := executeRoot(root); err != nil {
		os.Exit(1)
	}
}

// newRootCommand builds the root CLI command with all subcommands attached.
// Bare `mcpeach` launches the TUI; `mcpeach tui` stays as a compatibility
// alias.
func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "mcpeach",
		Short: "A local MCP gateway with a TUI",
		Long: `mcpeach aggregates multiple MCP servers (stdio + remote) behind a
single streamable-HTTP endpoint, permissions tools, and exposes curated
tool groups to clients.`,
		Version: version,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(debug)
		},
	}
	root.PersistentFlags().BoolVar(&debug, "debug", false, "show raw errors and verbose output")
	root.AddCommand(serveCmd(), tuiCmd(), importCmd(), exportCmd(), installCmd(), uninstallCmd(), statusCmd())
	return root
}

// executeRoot runs the root command with SIGINT/SIGTERM cancelling the
// command context. Extracted from main so tests can exercise the signal
// wiring without spawning a subprocess.
func executeRoot(root *cobra.Command) error {
	return fang.Execute(context.Background(), root, fang.WithNotifySignal(notifySignals...))
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
	if err := cfg.Validate(); err != nil {
		return fmt.Errorf("validate config: %w", err)
	}

	mgr := server.NewManager()
	res := secrets.NewResolver(secrets.NewStoreFromEnv())
	gw := gateway.New(cfg)
	// All upstream clients (stdio + remote, including any added via the
	// control plane) are closed on daemon shutdown so a partial startup
	// failure does not leak subprocesses or SSE/streamable-HTTP connections.
	defer gw.Close()
	for name, sc := range cfg.Servers {
		mgr.Add(server.New(name))
		if !sc.Enabled {
			continue
		}
		env, err := res.ResolveEnv(sc.Env)
		if err != nil {
			return fmt.Errorf("resolve env for %s: %w", name, err)
		}
		// For stdio servers, the connect layer owns the subprocess (via mcp-go's
		// stdio client) so the gateway can route calls. Feed its stderr into the
		// manager's log ring so the TUI log viewer still works.
		if sc.Command != "" {
			caller, tools, err := connect.Connect(ctx, sc, env)
			if err != nil {
				return fmt.Errorf("connect %s: %w", name, err)
			}
			// Register the cleanup guard immediately after connect succeeds so a
			// partial failure (e.g. MarkRunning) does not leak the client.
			gw.RegisterClient(name, caller)
			for _, t := range tools {
				gw.RegisterTool(name, t)
			}
			if err := mgr.MarkRunning(name); err != nil {
				return fmt.Errorf("mark running %s: %w", name, err)
			}
			if c, ok := caller.(*client.Client); ok {
				if stderr, ok := client.GetStderr(c); ok {
					mgr.CaptureLogs(name, stderr)
				}
			}
		} else {
			// Remote server: connect and register.
			caller, tools, err := connect.Connect(ctx, sc, nil)
			if err != nil {
				return fmt.Errorf("connect %s: %w", name, err)
			}
			gw.RegisterClient(name, caller)
			for _, t := range tools {
				gw.RegisterTool(name, t)
			}
		}
	}

	streaming := gateway.NewStreamingServer(gw, cfg.Gateway.Name, cfg.Gateway.Version)

	// Mount the MCP streaming endpoint and the control plane.
	mux := http.NewServeMux()
	mux.Handle("/mcp", streaming)
	// Mount a group-scoped MCP endpoint per configured group, exposing only
	// that group's resolved tool catalog.
	allStreaming := []*gateway.StreamingServer{streaming}
	for groupName := range cfg.Groups {
		gs := gateway.NewGroupStreamingServer(gw, "mcpeach-"+groupName, cfg.Gateway.Version, groupName)
		mux.Handle("/v0/groups/"+groupName+"/mcp", gs)
		allStreaming = append(allStreaming, gs)
	}
	controlHandler := control.NewHandler(mgr, gw, cfg)
	controlHandler.SetSyncTools(func() {
		for _, s := range allStreaming {
			s.SyncTools()
		}
	})
	ctrl := control.NewServer(config.SocketPath(), controlHandler)

	if err := ctrl.Start(ctx); err != nil {
		return fmt.Errorf("control plane: %w", err)
	}

	// Serve the MCP endpoint on the gateway address.
	srv := newStreamingServer(cfg.Gateway.Addr, mux)
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	// Announce readiness so `mcpeach serve` gives the user a clear signal.
	fmt.Printf("mcpeach: control plane listening on %s\n", config.SocketPath())
	fmt.Printf("mcpeach: MCP gateway listening on %s\n", cfg.Gateway.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// newStreamingServer builds the HTTP server that serves the MCP streamable
// HTTP endpoints. WriteTimeout is disabled (0) because SSE-style streams are
// long-lived: a bounded write deadline would cut healthy streams off after
// 30s. ReadHeaderTimeout and IdleTimeout still bound slowloris and idle
// connections.
func newStreamingServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0,
		IdleTimeout:       60 * time.Second,
	}
}

// tuiCmd launches the Bubble Tea TUI over the control-plane unix socket.
// Kept as a compatibility alias for the bare `mcpeach` launch.
func tuiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tui",
		Short: "Launch the mcpeach TUI (alias: run mcpeach with no arguments)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runTUI(debug)
		},
	}
}

// tuiRunner is the subset of the Bubble Tea program the TUI launch needs.
type tuiRunner interface {
	Run() (tea.Model, error)
}

// tuiProgramFactory builds the TUI program. Overridable in tests.
var tuiProgramFactory = func(m *tui.Model) tuiRunner {
	return tea.NewProgram(m)
}

// runTUI builds and runs the TUI program. Shared by the root command and the
// `tui` alias. debug shows raw errors instead of friendly messages.
func runTUI(debug bool) error {
	c := mcclient.NewUnix(config.SocketPath())
	var m *tui.Model
	if debug {
		m = tui.NewModelWithDebug(c)
	} else {
		m = tui.NewModel(c)
	}
	_, err := tuiProgramFactory(m).Run()
	return err
}

// importCmd imports servers from a Claude Code MCP config JSON file.
func importCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "import <file>",
		Short: "Import servers from a Claude Code MCP config JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.Path())
			if err != nil {
				return err
			}
			candidate, _, err := mcpconfig.Import(args[0], cfg)
			if err != nil {
				return err
			}
			return config.Save(config.Path(), candidate)
		},
	}
}

// exportCmd writes the mcpeach servers to a Claude Code MCP config JSON file.
func exportCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "export <file>",
		Short: "Export servers to a Claude Code MCP config JSON file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(config.Path())
			if err != nil {
				return err
			}
			return mcpconfig.Export(args[0], cfg)
		},
	}
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

// newServiceManager builds a service manager wired to the daemon. Errors the
// daemon returns under service mode are logged so they are not silently
// dropped.
func newServiceManager() (serviceManager, error) {
	return service.NewManager(runDaemon, func() {}, func(err error) {
		log.Printf("mcpeach service: %v", err)
	})
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
