# mcpeach — Local MCP Gateway with TUI

Run and manage all of your local and remote MCP servers in one place. mcpeach gives
clients **one gateway URL** while keeping process control, permissions, secrets,
logs, and configuration local to your machine.

A daemon (`mcpeach serve`) aggregates multiple MCP servers (stdio + SSE +
streamable-HTTP) and re-exposes them as one streamable-HTTP MCP endpoint that any
agent/LLM client can connect to. A TUI (`mcpeach`, or `mcpeach tui` as an alias)
manages everything: add/edit/delete servers, start/stop, per-server logs, process
stats, MCP tool listing, permissions, secrets, and import/export.

## Features

- **Single gateway endpoint** — aggregate stdio, SSE, and streamable-HTTP MCP
  servers behind one local endpoint (`/mcp`).
- **Server management screen** — per-server overview, configuration, environment,
  MCP tools, process telemetry (PID, CPU, memory, ports), and logs.
- **Add / edit / delete servers** — shared create/edit form with transactional
  save, including save-and-restart for running servers.
- **Permissioning** — allow/block tool filters and curated tool groups.
- **Secrets** — `env:` references and OS keychain storage (`keychain:` refs) for
  server credentials; secrets are write-only and never returned by the API.
- **Import/export** — read/write Claude Code MCP config JSON, with conflict
  policies and safe (reference-preserving) export.
- **Background service** — install/uninstall/status as a launchd (macOS) or
  systemd (Linux) service.
- **Default TUI** — running `mcpeach` with no arguments launches the TUI.

## Install

Install the CLI from the Go module proxy:

```sh
go install github.com/voxslopuli/mcpeach/cmd/mcpeach@latest
```

Or build from source:

```sh
task build        # build into bin/
task install      # go install the binary
```

## Quick start

```sh
mcpeach install          # install as a background service (optional)
mcpeach serve            # run the daemon (or start the service)
mcpeach                  # launch the TUI
```

In the TUI: press `n` to add a server, then point a client at the gateway URL.

## Client setup

Point any MCP client at the gateway endpoint:

```text
http://127.0.0.1:8080/mcp
```

The daemon also exposes per-group endpoints at `/v0/groups/{name}/mcp`.

## TUI

### Root server list

```
↑/↓       select server
space     start/stop selected server (state-aware toggle)
enter     manage selected server
n         new server
i         import configuration
x         export configuration
s         secrets (per-server env sources)
l         toggle log viewer
q/esc     quit (esc also goes back from sub-views)
```

### Server management (enter)

```
e         edit configuration
space     start/stop
d         delete (confirm with y, cancel with esc)
esc       back
```

The management screen shows: transport, command/arguments, endpoint, configured
and runtime state, MCP tool count and names, process telemetry (PID, CPU, memory,
ports) for local servers, and environment source references (never values).

### Logs

```
↑/↓       scroll
l         toggle log viewer
esc/q     back
```

## Configuration

Config lives at `~/.config/mcpeach/mcpeach.yml` (XDG). See `PLAN.md` for the full
schema.

Environment values may be:

| Value | Meaning |
|-------|---------|
| `plain-text` | literal value stored in the config (non-sensitive only) |
| `env:VAR` | read `VAR` from the daemon's environment at startup |
| `keychain:mcpeach/<server>/<var>` | stored in the OS keychain, resolved at startup |

Secrets are write-only: the control API and TUI display source references, never
resolved values.

## Control-plane HTTP API

The daemon exposes a JSON API over a unix socket at
`XDG_RUNTIME_DIR/mcpeach/mcpeach.sock` (mode `0600`). The TUI and any client talk
to this API.

```sh
# List servers
curl --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/servers

# Server detail (config + state + tools + process)
curl --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/servers/github

# List aggregated tools
curl --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/tools

# Start / stop a server
curl -X POST --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/servers/github/start
curl -X POST --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/servers/github/stop

# Per-server logs
curl --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/servers/github/logs

# Add / edit / delete a server
curl -X POST   --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock \
  -H 'Content-Type: application/json' \
  -d '{"name":"github","command":"npx","args":["-y","@modelcontextprotocol/server-github"]}' \
  http://unix/v0/servers
curl -X PUT    --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock \
  -H 'Content-Type: application/json' \
  -d '{"command":"npx","args":["-y","@modelcontextprotocol/server-github"]}' \
  http://unix/v0/servers/github
curl -X DELETE --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock \
  http://unix/v0/servers/github

# Secrets: list sources (references, never values), store, delete
curl --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock http://unix/v0/secrets
curl -X POST   --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock \
  -H 'Content-Type: application/json' -d '{"value":"mysecret"}' \
  http://unix/v0/secrets/github/GITHUB_TOKEN
curl -X DELETE --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock \
  http://unix/v0/secrets/github/GITHUB_TOKEN

# Import / export (daemon-owned, safe defaults)
curl -X POST --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock \
  -H 'Content-Type: application/json' \
  -d '{"path":"/path/to/claude.json","conflicts_policy":"review","secrets_policy":"keychain"}' \
  http://unix/v0/import
curl -X POST --unix-socket $XDG_RUNTIME_DIR/mcpeach/mcpeach.sock \
  -H 'Content-Type: application/json' \
  -d '{"path":"/path/to/out.json","secrets":"references"}' \
  http://unix/v0/export
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
- `env` values may be `env:VAR` (process env) or `keychain:...` (OS keychain)
  references.

Import/export are also available as CLI commands (safe defaults: conflict review and keychain migration on import, references preserved on export):

```sh
mcpeach import <file>
mcpeach export <file>
```

## Security

- The control-plane unix socket is mode `0600` (owner-only). Any process running
  as the same user can start/stop servers and execute configured commands with
  that user's privileges.
- Run the mcpeach daemon as the same user who owns the config file; do not run
  as root unless necessary.
- The daemon executes server commands with the same privilege level as the
  mcpeach process.
- Secret values are write-only and resolved only at runtime; the control API,
  TUI, and safe export never return resolved values. Plaintext export requires
  an explicit `--allow-plaintext-secrets` flag.

## Development

```sh
task build   # build the binary
task test    # run all tests with race detector + coverage
task lint    # go vet + golangci-lint
task ci      # all CI gates (vet, test, build)
```

## Architecture

See `PLAN.md` for the full architecture: the daemon owns child processes, remote
connections, permissioning, log capture, secret resolution, and the gateway; the
TUI is a thin client over the control-plane unix-socket API.

```
Claude / Cursor / agents
          │
          ▼
http://127.0.0.1:8080/mcp
          │
          ▼
      mcpeach daemon
       ├── stdio MCP servers
       ├── SSE MCP servers
       ├── streamable HTTP MCP servers
       ├── tool permissions
       ├── secret resolution
       ├── process telemetry
       └── log capture
          ▲
          │ Unix socket
          │
      mcpeach TUI
```
