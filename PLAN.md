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
  addr: "127.0.0.1:11585"        # MCP streamable-HTTP endpoint
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

## Secrets: env-var references + system keychain

Secrets (API keys, tokens) are never stored in plaintext in the config file.
Two mechanisms, used together:

### 1. Environment-variable references

Any config value may reference an environment variable with the `env:` prefix:

```yaml
servers:
  github:
    command: "npx"
    args: ["-y", "@modelcontextprotocol/server-github"]
    env:
      GITHUB_PERSONAL_ACCESS_TOKEN: "env:GITHUB_TOKEN"   # read from env at use time
llm:
  api_key: "env:OPENAI_API_KEY"
```

- A value starting with `env:` is resolved from the process environment at the
  point of use (server spawn, LLM call), not stored in the file.
- If the referenced env var is unset, resolution fails with a clear error.
- This keeps secrets out of the YAML while allowing shell/CI-provided values.

### 2. System keychain storage

For values the user enters interactively (via the TUI or `mcpeach add`), store
secrets in the OS keychain instead of the config file:

- **macOS**: Keychain via `security` CLI or the `keyring` Go library.
- **Linux**: Secret Service (libsecret) / `keyring`.
- **Windows**: Credential Manager / `keyring`.

Config stores a keychain reference, not the secret:

```yaml
servers:
  github:
    env:
      GITHUB_PERSONAL_ACCESS_TOKEN: "keychain:mcpeach/github/GITHUB_PERSONAL_ACCESS_TOKEN"
```

- A value starting with `keychain:` is fetched from the OS keychain at use time.
- The TUI's add/edit forms offer "store in keychain" for secret fields.
- `mcpeach` uses the `zalando/go-keyring` library (cross-platform, stdlib-adjacent)
  for keychain access.

### Resolution order

1. `keychain:` reference → fetch from OS keychain.
2. `env:` reference → read from process environment.
3. Plain value → used as-is.

A `resolve` helper in `internal/config` (or a new `internal/secrets` package)
resolves any value through this chain, so server spawn and LLM calls get the
final secret without the config file ever holding it in plaintext.

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
Cursor, agents) connect to `http://127.0.0.1:11585/mcp` (and per-group
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

> **Status: not yet exposed.** The `internal/llm` Finder (`FindTools`) is built and unit-tested but is not wired into the gateway — the `mcpeach__search_tools` / `mcpeach__load_tools` meta-tools are not registered. Wiring is deferred to a follow-up PR; this section documents the intended architecture only.

## TUI — Comprehensive Overhaul (amended per 4th review + product contract audit)

### Product contract corrections (current state vs. claims)

The current TUI overstates its capabilities. `Enter` starts a server, `Space` stops,
`a` opens the add form, and there is no edit/delete/process-view/secrets workflow.
The add form is not rendered inside the Bubble Tea view (blocking `huh.Form.Run()`
takes over the terminal). "Tools" means MCP tools, not process info. The README
claims more than the TUI delivers. The overhaul below fixes the contract first,
then builds the real product.

### Target information architecture

**Root server list** (optimized for frequent lifecycle actions):

```text
↑/↓       select server
space     start/stop selected server (state-aware toggle)
enter     manage selected server
n         new server
i         import configuration
x         export configuration
?         help
q/esc     quit
```

- `Space` inspects state: stopped/error → start; running/starting → stop; stopping → no-op.
- Lifecycle ops show transitional states (never frozen).
- `Enter` opens the server management screen (never starts/stops).
- `n` replaces `a`. `i`/`x` expose import/export directly.
- `Escape` quits only from root. `q` quits except while a form owns text input.
- Footer derives from the active view's keymap (no static string).

**Server management screen** (`Enter`):

```text
Server: github                                  running

Overview  Configuration  Environment  MCP Tools  Process  Logs

Transport       stdio
Command         npx
Arguments       -y @modelcontextprotocol/server-github
Configured      enabled
Runtime         running
MCP tools       14
PID             42173
Memory          82.4 MiB

e edit configuration
space stop
d delete
tab/shift+tab change section
esc back
```

Sections: Overview (transport, states, tool count, recent error), Configuration
(editable command/args/URL/transport/enabled/permissions), Environment (names +
sources, never resolved values), MCP Tools (canonical names + permission state),
Process (PID/CPU/RSS/uptime/ports), Logs (scrollable viewport, follow mode).

**MCP Tools ≠ Process information** (never combined, never ambiguous):
- MCP tools = callable functions (`filesystem__read_file`).
- Process info = operational telemetry (PID, CPU, RSS, uptime, ports).
- Remote servers can expose tools with no local process; a local process can run
  with zero tools (init failure, filters, no tools defined).

### Shared create/edit form (replaces `addServerForm`)

One reusable configuration editor for create + edit modes. Fields: Name, Transport,
Command, Arguments, URL, Start-automatically, Environment, Tool-permission mode,
Allowed/blocked tools. Conditional: stdio → command/args/env; streamable-http/sse →
URL + env/auth. Command and URL mutually exclusive. Name rejects empty + `__`.
Edit mode prepopulates all fields but never retrieves/exposes resolved secrets.

Save behavior for running servers: **Save and restart** (connect replacement before
removing healthy client), **Save for next start** (persist only), **Cancel**.
Failed replacement preserves the running client + tools. Secret mutations are
transactional (no orphan keychain entry if config persistence fails).

Delete (`d` from management screen): confirmation dialog; stop + disconnect first;
remove tools from gateway; reject if a group still references it (unless user
removes references); delete only mcpeach-owned keychain entries; atomic persist;
restore runtime/config on failure.

### Control-plane API expansion

```text
GET    /v0/servers/{name}
PUT    /v0/servers/{name}
DELETE /v0/servers/{name}
GET    /v0/servers/{name}/process
```

Full server representation (no resolved secrets ever): name, state, enabled,
command, args, env (name + source + reference only), tools (mode + list).
Process endpoint: local (pid, cpu_percent, rss_bytes, uptime_seconds, ports) or
remote (endpoint, connected). Establish a real process-ownership contract first
(mcp-go owns stdio subprocesses; the manager may not expose a meaningful PID).

Mutation correctness: deep-copy config → apply mutation → validate candidate →
atomic write (tmp + fsync + rename) → commit active config → apply runtime
topology → rollback/preserve healthy runtime on failure. Serialize mutations.

### Environment variables and stored secrets

Model each env entry as name + explicit source:

| Source | Config value | Where value lives | Use |
|---|---|---|---|
| Literal | `plain-value` | YAML | non-sensitive only |
| Environment | `env:GITHUB_TOKEN` | daemon env | shell/CI/service injection |
| Keychain | `keychain:mcpeach/github/GITHUB_TOKEN` | OS store | interactive local secrets |
| 1Password | env ref after `op run` injection | 1Password | teams (external workflow, no core coupling) |

Keychain: service `mcpeach`, account `<server>/<env-name>`. Write-only in TUI;
edit shows "Stored in system keychain"; replace requires new value; clear requires
confirmation. Secrets never returned by control API, never in logs/errors/exports.
Literal values warn if name looks secret-like (TOKEN/SECRET/PASSWORD/API_KEY/...).

Secret management screen (root action menu): add/replace keychain secret, change
source, remove unused mcpeach-owned entries, validate references without revealing
values, find orphans. Never list other apps' credentials.

### Import / export as first-class TUI workflows

Import (`i`): source format (Claude JSON / mcpeach YAML), file, conflict policy
(review each / keep existing / replace existing), credential handling (move
probable secrets to keychain / env refs / literal with warnings). Preview before
apply (servers found, conflicts, plaintext-secret count). Nothing persists until
confirmed. Roll back the whole import on any failure. Do not auto-start imported
servers. Report unsupported source fields.

Export (`x`): format, servers (all/selected), secret handling (preserve refs /
env refs / plaintext [unsafe, double-confirmed]), destination. Mode 0600, refuse
overwrite without confirmation, never print resolved content, clean temp files.
CLI equivalents with explicit flags: `mcpeach import <file> --conflicts=review
--secrets=keychain`; `mcpeach export <file> --format=claude --secrets=plaintext
--allow-plaintext-secrets`.

### Default command behavior

`mcpeach` (no subcommand) launches the TUI. `mcpeach tui` remains a compatibility
alias for ≥1 release cycle. Daemon-not-running shows a deliberate startup screen
(Start for this session / Install background service / Retry / Quit) — never a
silent empty list. No auto-start of the daemon without consent.

### Keyboard contract (per view)

- Root: ↑/↓ select, space start/stop, enter manage, n new, i import, x export, ? help, q/esc quit.
- Management: tab/shift+tab section, e edit, space start/stop, d delete, r refresh, esc back, q quit.
- Logs: ↑/↓ scroll, pgup/dn page, f follow, c clear filter, esc back.
- MCP tools: ↑/↓ select, space allow/block, / filter, esc back.
- Forms: tab next, shift+tab prev, enter select/submit, ctrl+s save, esc cancel.

### README rewrite

Rewrite around the user problem and real workflows: product statement, why mcpeach,
architecture diagram, 5-minute quick start, TUI tour (screenshots only after E2E
passes), server config examples, secrets guide, import/export guide, client setup
(Claude Desktop/Code, Cursor, generic streamable-HTTP), operational guide, security
model, project status (implemented vs planned — never claim unimplemented features).

### TUI overhaul phases (strict TDD, gates green, confirmation at each phase end)

| # | Deliverable | Tests |
|---|---|---|
| T1 | Product contract + keybindings: typed active-view model, state-aware Space, Enter→manage, n→new, root command launches TUI, `tui` alias, daemon-connection error screen, view-specific footer | model transitions, keymap, root-vs-tui equivalence, missing-daemon screen |
| T2 | Complete server representations: GET /v0/servers/{name}, unresolved env metadata, deterministic sorting, client deadlines, last-known-data-on-failure | round-trip, no-secret-leak, 404, determinism, timeouts |
| T3 | Server management screen: Overview/Config/Env/MCP Tools/Process/Logs sections, keyboard nav, resize, loading/empty/error states, remote-without-PID | section rendering, tools≠process, remote process view, server-scoped logs, long/Unicode safety, narrow terminals |
| T4 | Shared create/edit form: Bubble-Tea-integrated (no blocking Run), create+edit modes, all fields, typed args editor, save-only vs save-and-restart, candidate validation, atomic persist, healthy-runtime preservation | create/edit every field, stdio↔remote switch, command+URL rejection, env/tool-filter preservation, failed-save no-mutation, failed-restart preserves client |
| T5 | Deletion: DELETE /v0/servers/{name}, group-reference analysis, confirmation, stop+deregister, owned-keychain cleanup after successful persist, deterministic selection return | delete stopped/running, catalog cleanup, child exit, group-block, 404, persist-failure no-partial-delete |
| T6 | Secret workflow: standardized keychain refs, write-only store/replace/validate/delete, env-source model, plaintext-secret detection, rollback, structured redaction, `op run` documented (external) | resolve at startup, missing-env fails clearly, no-disclosure, edit-never-retrieves, store-failure no-config-change, save-failure removes new entries, delete only owned, no-secret-in-output |
| T7 | TUI import/export: parse/preview/validate/apply/serialize stages, conflict policies, secret migration, safe export, plaintext gated, atomic files, unsupported-field report | import all transports, review conflicts, multi-server rollback, ref-preserving export, plaintext gated, 0600, no-leak |
| T8 | README rewrite + PLAN/AGENTS reconciliation, docs/ guides | every documented command on fresh XDG root, JSON/YAML examples valid, keys match keymap, routes match handler, no real credentials, docs agree |
| T9 | E2E plan revision (`microsoft/tui-test`): replace outdated keybinding scenarios, add management/edit/delete/process/tools/secrets/import/export/default-command suites, fake keychain backend | E2E suites green on isolated state |

Delivery order: T1 → T2 → T3 → T4 → T5 → T6 → T7 → T8 → T9 (each phase useful alone;
no advertising of incomplete functionality).

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
| 5c | Secrets: env-var references + system keychain storage | resolve chain, keychain round-trip |
| 6 | TUI: list/detail/toggles/log viewer/**process viewer**/forms | model state transitions |
| 7 | Service install: kardianos/service | config gen |
| 8 | Polish: theme, help, docs, final CI | — |
