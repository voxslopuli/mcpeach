# mcpeach — Local MCP Gateway with TUI

A daemon (`mcpeach serve`) that runs as a background service, aggregates multiple MCP servers (stdio + remote), and re-exposes them as **one** MCP gateway endpoint that any agent/LLM client (Claude, Cursor, custom agents) can connect to. A TUI (`mcpeach tui`) manages it: add/remove/configure servers, permission tools, start/stop, and watch logs.

## Features

- **Single gateway endpoint** — aggregate stdio, SSE, and streamable-HTTP MCP servers behind one local streamable-HTTP endpoint (`/mcp`).
- **TUI** — server list, start/stop, per-server logs, tool list, add-server form. (Server management screen, edit/delete, process stats, and secrets workflow are planned — see PLAN.md.)
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
space start/stop (state-aware toggle)
enter manage server (coming soon)
n     add server (form)
l     toggle log viewer
t     toggle tools view
q/esc quit (or back from a sub-view)
```

Run `mcpeach` with no arguments to launch the TUI (`mcpeach tui` is an alias).

## Configuration

Config lives at `~/.config/mcpeach/mcpeach.yml` (XDG). See `PLAN.md` for the full schema.

## Control-plane HTTP API

The daemon exposes a JSON API over a unix socket at `XDG_RUNTIME_DIR/mcpeach/mcpeach.sock` (mode `0600`). The TUI and future web client talk to this API.

```sh
# List servers
curl --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/servers

# List aggregated tools
curl --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/tools

# Start / stop a server
curl -X POST --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/servers/github/start
curl -X POST --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/servers/github/stop

# Per-server logs
curl --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/servers/github/logs

# Process stats (CPU/RAM/ports)
curl --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/processes

# Add a server (stdio)
curl -X POST --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock \
  -H 'Content-Type: application/json' \
  -d '{"name":"github","command":"npx","args":["-y","@modelcontextprotocol/server-github"]}' \
  http://unix/v0/servers
```

### POST /v0/servers request body

```json
{
  "name": "github",
  "command": "npx",
  "args": ["-y", "@modelcontextprotocol/server-github"],
  "env": { "GITHUB_TOKEN": "env:GITHUB_TOKEN" },
  "url": "https://mcp.example.com/mcp",
  "transport": "streamable-http"
}
```

- `name` (required): server name; must not contain `__`.
- `command`/`args`/`env`: for stdio servers.
- `url`/`transport`: for remote servers (mutually exclusive with `command`).
- `env` values may be `env:VAR` (process env) or `keychain:service/user` (OS keychain) references.

## Security

- The control-plane unix socket is mode `0600` (owner-only). Any process running
  as the same user can start/stop servers and execute configured commands with
  that user's privileges.
- Run the mcpeach daemon as the same user who owns the config file; do not run
  as root unless necessary.
- The daemon executes server commands with the same privilege level as the
  mcpeach process.

## Development

```sh
task build   # build the binary
task test    # run all tests with race detector + coverage
task lint    # go vet + golangci-lint
task ci      # all CI gates (vet, test, build)
```

## Architecture

See `PLAN.md` for the full architecture: daemon owns child processes, remote connections, permissioning, log capture, and the gateway; the TUI is a thin client over the control-plane unix-socket API.
