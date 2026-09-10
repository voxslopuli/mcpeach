# AGENTS.md

## Project

mcpeach is a local MCP gateway with a TUI. It aggregates multiple MCP servers
(stdio + remote) behind a single streamable-HTTP endpoint, permissions tools,
and exposes curated tool groups to clients. Written in Go.

## Commands

- `task build` — build the `mcpeach` binary into `bin/`
- `task test` — run all tests with race detector and coverage
- `task lint` — `go vet` + golangci-lint (if installed)
- `task fmt` — gofmt all files
- `task install` — `go install` the binary
- `task ci` — run all CI gates (vet, test, build)

## Layout

- `cmd/mcpeach/` — cobra/fang CLI entrypoint (serve, tui, add, remove, list, install, status)
- `internal/config/` — YAML config load/save/validate, XDG paths
- `internal/server/` — process lifecycle, state machine, remote connections
- `internal/gateway/` — mcp-go aggregation, tool canonicalization, groups
- `internal/permission/` — allow/block filters, group resolution
- `internal/control/` — daemon control-plane HTTP API (unix socket)
- `internal/client/` — control-plane client (TUI side)
- `internal/llm/` — OpenAI-compatible client + tool-finder
- `internal/logs/` — ring-buffer capture, JSON parse
- `internal/tui/` — bubbletea app
- `testdata/` — fake MCP server binary for tests

## Conventions

- Strict TDD: write a failing test first, then the minimal code to pass it.
- All CI gates (vet, test, build) must be green before commit, unless the user
  explicitly authorizes committing with a known-failing gate.
- One PR per phase. Conventional commits (`feat(scope):`, `fix(scope):`, `chore(scope):`).
- Tool names canonicalized as `<server>__<tool>`.
- Server names must NOT contain `__` (reserved for tool canonicalization),
  unless the user explicitly authorizes a name that does.
- Config lives at `~/.config/mcpeach/mcpeach.yml` (XDG).
- Use `mark3labs/mcp-go` for MCP protocol; do not hand-roll the protocol.

## Security

- The control-plane unix socket (`XDG_RUNTIME_DIR/mcpeach/mcpeach.sock`) is
  mode `0600` (owner-only). Any process running as the same user can start/stop
  servers and execute configured commands with that user's privileges.
- Run the mcpeach daemon as the same user who owns the config file; do not
  run as root (the daemon executes server commands with the same privilege
  level as the mcpeach process).
- The daemon executes server commands with the same privilege level as the
  mcpeach process.

## Boundaries

- Do not modify another agent's or the user's work without explicit approval.
- Do not push or open PRs unless the user asks.
- Do not hand-roll the MCP protocol; always use `mark3labs/mcp-go` unless the
  user explicitly requests a custom protocol implementation.
- Do not add dependencies for what a few lines of stdlib can do.
- Do not commit secrets, tokens, or credentials.
- Do not delete or rewrite committed history without explicit authorization.
