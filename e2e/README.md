# mcpeach E2E tests

This suite drives the real `mcpeach serve` daemon and the real `mcpeach` TUI
through the `tui-test` CLI over a real PTY, against isolated XDG directories.

## Prerequisites

- `tui-test` installed (see `scripts/install-tui-test.sh`).
- Go toolchain.

## Run

```sh
go test ./e2e/...
```

## Structure

- `harness/` — daemon, session, fixture, process, assertion, and artifact helpers.
- `tui/` — the E2E test suites (startup, navigation, lifecycle, tools, logs,
  errors, cleanup, security, persistence, resize, remote).
- `fixtures/` — deterministic loopback streamable-HTTP and SSE MCP servers.

## Notes

- Each test gets an isolated root under `/tmp/mcpeach-e2e/<test-name>` (a short
  path, because the unix control socket must stay under the ~108-byte AF_UNIX
  limit).
- No test touches the developer's real keychain or real config.
- The add/edit form E2E tests are deferred: the current form uses blocking
  `huh.Form.Run()`, which conflicts with the tui-test PTY's terminal ownership.
  Once the form is integrated into the Bubble Tea model, those tests can be
  added.
