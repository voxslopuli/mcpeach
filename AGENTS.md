# AGENTS.md

## Project

mcpeach is a local MCP gateway with a TUI. It aggregates multiple MCP servers
(stdio + remote) behind a single streamable-HTTP endpoint, permissions tools,
and exposes curated tool groups to clients. Written in Go.

## Commands

- `make build` — build the `mcpeach` binary into `bin/`
- `make test` — run all tests with race detector and coverage
- `make lint` — `go vet` + golangci-lint (if installed)
- `make fmt` — gofmt all files
- `make install` — `go install` the binary

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
- All CI gates (vet, test, build) must be green before commit.
- One PR per phase. Conventional commits (`feat(scope):`, `fix(scope):`, `chore(scope):`).
- Tool names canonicalized as `<server>__<tool>`.
- Config lives at `~/.config/mcpeach/mcpeach.yml` (XDG).
- Use `mark3labs/mcp-go` for MCP protocol; do not hand-roll the protocol.
