# mcpeach — Local MCP Gateway with TUI

## Overview

A daemon (`mcpeach serve`) that runs as a background service, aggregates multiple MCP servers (stdio + remote), and re-exposes them as **one** MCP gateway endpoint that any agent/LLM client (Claude, Cursor, custom agents) can connect to. A TUI (`mcpeach tui`) manages it: add/remove/configure servers, permission tools, start/stop, and watch logs. The gateway condenses tools into **groups** (mcpjungle-style) so each client sees only a curated subset. An LLM tool-finder (OpenAI-compatible, default Ollama) recommends which tools to enable.

## Architecture

```
┌─────────────┐   control-plane (unix socket, JSON)   ┌──────────────────────┐
│  mcpeach tui│ ─────────────────────────────────────▶ │  mcpeach serve (daemon)│
│  (bubbletea)│                                        │                      │
└─────────────┘                                        │  ┌────────────────┐  │
                                                       │  │ Server Manager │  │  spawns/captures
┌─────────────┐   MCP streamable-HTTP                  │  │  (processes)    │──┼──▶ stdio servers
│ Claude/Cursor│ ────────────────────────────────────▶ │  └────────────────┘  │
│  / agents    │   /mcp  +  /v0/groups/{name}/mcp      │  ┌────────────────┐  │  connects
└─────────────┘                                        │  │ Gateway (mcp-go)│──┼──▶ sse/http servers
                                                       │  │  + permission   │  │
                                                       │  └────────────────┘  │
                                                       │  ┌────────────────┐  │
                                                       │  │ Tool-finder    │  │──▶ ollama / any
                                                       │  │ (search+load)  │  │
                                                       │  └────────────────┘  │
                                                       └──────────────────────┘
```

- **Daemon** owns child processes, remote connections, permissioning, log capture, and the gateway. Runs headless as a service.
- **TUI** is a thin client over the control-plane API — no direct process management.
- **Future web client** plugs into the same control-plane API (that's why it's a separate surface).

## Tech Stack

| Concern | Library | Why |
|---|---|---|
| MCP server + client, all transports | `mark3labs/mcp-go` | De-facto Go MCP lib (9.1k★), stdio/SSE/streamable-HTTP, session mgmt, tool filtering, hooks. Used by tbxark/mcp-proxy. |
| TUI framework | `charmbracelet/bubbletea` v2 | Built on `ultraviolet` cell-based renderer. |
| TUI components | `charmbracelet/bubbles` | list, table, viewport, textarea, spinner, help. |
| Styling | `charmbracelet/lipgloss` | |
| Forms | `charmbracelet/huh` | add/edit server config. |
| Logging | `charmbracelet/log` | |
| JSON syntax highlight | `alecthomas/chroma` | JSON lexer for log view. |
| CLI | `charmbracelet/fang` + `spf13/cobra` | Batteries-included Cobra: styled help/errors, `--version`, manpages, completions. |
| YAML config | `gopkg.in/yaml.v3` | |
| XDG paths | `adrg/xdg` | |
| Service install | `kardianos/service` | systemd/launchd/windows. |
| LLM client | `sashabaranov/go-openai` | OpenAI-compatible (works with Ollama). |
| Config hot-reload | `fsnotify` | optional, later. |

## Repo Layout

```
mcpeach/
├── cmd/mcpeach/            # fang+cobra: serve, tui, add, remove, list, install, status
├── internal/
│   ├── config/             # YAML load/save/validate, XDG paths
│   ├── server/             # process lifecycle, state machine, remote conns
│   ├── gateway/            # mcp-go aggregation, canonicalization, groups
│   ├── permission/         # allow/block filters, group resolution
│   ├── control/            # daemon HTTP API (unix socket)
│   ├── client/             # control-plane client (TUI side)
│   ├── llm/                # OpenAI-compatible client + tool-finder
│   ├── logs/               # ring-buffer capture, JSON parse
│   └── tui/                # bubbletea app
├── testdata/               # fake MCP server binary for tests
└── .github/workflows/ci.yml
```

One binary with subcommands (`mcpeach serve`, `mcpeach tui`, `mcpeach add …`) — simplest install, matches mcpjungle's CLI model.

## Config Schema (`~/.config/mcpeach/mcpeach.yml`)

```yaml
gateway:
  addr: "127.0.0.1:8080"        # MCP streamable-HTTP endpoint
  name: "mcpeach"
  version: "0.1.0"
llm:                            # tool-finder + description enrichment
  base_url: "http://localhost:11434/v1"   # default Ollama
  model: "llama3.2"
  api_key: ""                   # empty for Ollama
  enrich_descriptions: true     # generate implicit use-cases for retrieval
servers:
  github:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-github"]
    env: { GITHUB_PERSONAL_ACCESS_TOKEN: "…" }
    enabled: true
    tools:                      # permission: allow/block per tool
      mode: allow               # allow | block
      list: ["create_or_update_file"]
  context7:
    url: "https://mcp.context7.com/mcp"
    transport: "streamable-http"
    enabled: true
groups:
  claude-tools:
    description: "Curated set for Claude Desktop"
    included_servers: ["github"]
    included_tools: ["context7__get-library-docs"]
    excluded_tools: []
```

## Permission Model

- **Per-server**: `enabled` toggle (start/stop).
- **Per-tool**: `tools.mode` = `allow` (only listed) or `block` (all but listed). Canonical name `<server>__<tool>` (mcpjungle convention).
- **Groups**: `included_servers` + `included_tools` + `excluded_tools` (exclusion applied last), exposed at `/v0/groups/{name}/mcp`. A tool disabled/deregistered globally drops out of groups automatically.

## Gateway

- mcp-go server exposing aggregated tools/prompts/resources from all enabled servers.
- Tool names canonicalized to `<server>__<tool>` to avoid collisions.
- Group endpoints each mount a filtered view (mcp-go `WithToolFilter` per session/route).
- The gateway itself exposes the tool-finder meta-tools.

### Single local streaming MCP server interface

The gateway is mounted as **one** local `NewStreamableHTTPServer` (mcp-go) that
re-exposes every configured server — stdio subprocesses, remote SSE, and remote
streamable-HTTP — behind a single streamable-HTTP endpoint. Clients (Claude,
Cursor, agents) connect to `http://127.0.0.1:8080/mcp` (and per-group
`/v0/groups/{name}/mcp`) and see one unified MCP server. The unix socket is
**only** the control plane (TUI↔daemon); it is not the client-facing MCP
interface.

- **stdio** servers: spawned by the server manager, connected via mcp-go
  `NewStdioMCPClient`.
- **remote SSE** servers: connected via mcp-go `NewSSEMCPClient`.
- **remote streamable-HTTP** servers: connected via mcp-go
  `NewStreamableHttpClient`.
- All three are aggregated into the gateway and re-exposed as one local
  streamable-HTTP MCP server.

## Import / Export (Claude Code mcp config JSON)

`mcpeach import` / `mcpeach export` read and write JSON files conforming to the
Claude Code MCP config format (`.mcp.json` / `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "github": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-github"],
      "env": { "GITHUB_PERSONAL_ACCESS_TOKEN": "..." }
    },
    "context7": {
      "url": "https://mcp.context7.com/mcp",
      "type": "http"
    }
  }
}
```

- **Import**: parse the Claude Code JSON, map each entry to a `ServerConfig`
  (stdio `command`/`args`/`env`; remote `url`/`type` → `transport`), merge into
  the mcpeach config, and save. Preserves existing servers; conflicts are
  reported.
- **Export**: serialize the mcpeach `servers` map back to the Claude Code JSON
  format, so a user can point Claude Desktop / Claude Code at the same servers
  (or at the mcpeach gateway itself).
- Round-trip fidelity: import→export is lossless for the fields Claude Code
  supports.

## LLM Tool-Finder — Search-and-Load Architecture

Two meta-tools exposed on the gateway (and reusable via TUI):

1. **`mcpeach__search_tools(query)`** — takes a natural-language request, decomposes it into **atomic sub-queries** (per Dynamic ReAct 3.3), retrieves top-k candidates from the tool catalog, **validates** them (drop obvious-but-wrong matches, per RAG-MCP skill), and returns a ranked shortlist with rationale. Uses **context-enriched descriptions** (implicit use-cases generated at registration time via the LLM) for better retrieval.

2. **`mcpeach__load_tools(tool_ids)`** — after the LLM reviews the shortlist, it calls this to bind the selected tools into the current session (via mcp-go per-session tool filtering). This keeps the agent's context clean — only the loaded tools are exposed.

**Design principles from research:**
- **Atomic queries** beat raw-message search (Dynamic ReAct 3.2).
- **Deliberate loading of <5 tools** beats high-k retrieval (Dynamic ReAct 3.3).
- **Context-enriched descriptions** improve retrieval ~50% (Dynamic ReAct 5.3).
- **Validation step** prevents false positives (RAG-MCP skill).
- **Default tools** (e.g. a built-in `mcpeach__web_search` or similar) avoid futile searches for generic tasks (Dynamic ReAct 4.3) — optional, later.
- **Under ~10 tools, skip filtering** — just expose them all (RAG-MCP skill "When NOT to Use").

**Future (post-v1):** Well's **skill layer** — learned query→tool associations that narrow the candidate set before the LLM sees options. This is the compounding-efficiency play; not in v1.

## TUI

- **Left**: server list (running/stopped/error, enabled/disabled).
- **Right**: tool list with toggles, config, group membership.
- **Bottom**: log viewport, chroma JSON highlighting, follow-mode.
- **Keys**: start/stop, add/remove/edit (huh forms), toggle tool, view logs, **find tools** (invokes the search-and-load flow).
- **Design pillar**: "cute and fun but not cloying" — warm peach palette, friendly-but-professional copy, no emoji spam, clear help footer.

## Service Install

- `mcpeach install` → kardianos/service writes systemd/launchd unit running `mcpeach serve`. `status`/`uninstall`/`logs`.

## Phases (strict TDD, gates green before commit, confirmation at each phase end)

| # | Deliverable | Tests |
|---|---|---|
| 0 | Scaffold: go.mod, layout, Makefile, CI, AGENTS.md | trivial |
| 1 | Config: YAML schema, load/save/validate, XDG | round-trip, validation |
| 2 | Server manager: spawn/capture/stop/restart, state machine, remote conns | fake MCP server binary; lifecycle |
| 3 | Gateway + permission: aggregation, canonicalization, allow/block, groups | filtering, group resolution, collisions |
| 3b | Gateway streaming interface: mount as one local streamable-HTTP MCP server; connect stdio/SSE/streamable-HTTP clients | end-to-end client↔gateway round-trip |
| 4 | Control plane: unix-socket API + client | handler + client round-trip |
| 5 | LLM tool-finder: OpenAI client, search+load meta-tools, description enrichment, validation | mock endpoint; prompt/parse; retrieval/validation |
| 5b | Import/export: Claude Code mcp config JSON ↔ mcpeach config | round-trip, merge, conflict |
| 6 | TUI: list/detail/toggles/log viewer/forms | model state transitions |
| 7 | Service install: kardianos/service | config gen |
| 8 | Polish: theme, help, docs, final CI | — |
