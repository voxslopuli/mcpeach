# mcpeach — Local MCP Gateway with TUI

A daemon (`mcpeach serve`) that runs as a background service, aggregates multiple MCP servers (stdio + remote), and re-exposes them as **one** MCP gateway endpoint that any agent/LLM client (Claude, Cursor, custom agents) can connect to. A TUI (`mcpeach tui`) manages it: add/remove/configure servers, permission tools, start/stop, and watch logs.

## Features

- **Single gateway endpoint** — aggregate stdio, SSE, and streamable-HTTP MCP servers behind one local streamable-HTTP endpoint (`/mcp`).
- **TUI** — server list, start/stop, per-server logs, tool list, add-server form, process stats (CPU/RAM/ports).
- **Permissioning** — allow/block tool filters and curated tool groups.
- **Secrets** — `env:` references and OS keychain storage for server credentials.
- **Background service** — install/uninstall/status as a launchd (macOS) or systemd (Linux) service.
- **Import/export** — read/write Claude Code MCP config JSON.

## Install

```sh
task build        # build into bin/
task install      # go install the binary
```

## Usage

```sh
mcpeach serve      # run the daemon (headless)
mcpeach tui        # launch the TUI
mcpeach install    # install as a background service
mcpeach uninstall  # remove the service
mcpeach status     # show service status
```

### TUI keys

```
↑/↓   select server
enter start server
space stop server
l     toggle log viewer
t     toggle tools view
a     add server (form)
q/esc quit (or back from a sub-view)
```

## Configuration

Config lives at `~/.config/mcpeach/mcpeach.yml` (XDG). See `PLAN.md` for the full schema.

## Development

```sh
task build   # build the binary
task test    # run all tests with race detector + coverage
task lint    # go vet + golangci-lint
task ci      # all CI gates (vet, test, build)
```

## Architecture

See `PLAN.md` for the full architecture: daemon owns child processes, remote connections, permissioning, log capture, and the gateway; the TUI is a thin client over the control-plane unix-socket API.
