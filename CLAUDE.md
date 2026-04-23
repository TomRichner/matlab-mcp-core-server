# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Go implementation of a Model Context Protocol (MCP) server for MATLAB. Exposes MATLAB operations (code evaluation, static analysis, test running, toolbox detection) as MCP tools to AI clients (Claude Code, Claude Desktop, GitHub Copilot). Communicates with MATLAB via HTTP over local sockets. This is a community fork adding session persistence, non-blocking watchdog, and command window output echoing.

## Build & Test Commands

```bash
make all                  # Full pipeline: wire, mockery, lint, unit/integration/functional tests, build

make build                # Cross-compile for all platforms (win64, glnxa64, maci64, maca64)
make build-for-maca64     # Build for macOS Apple Silicon only
RELEASE=true make build   # Strip symbols (-ldflags "-s -w") for release

make wire                 # Regenerate dependency injection (Google Wire)
make mockery              # Regenerate interface mocks (deletes mocks/ and tests/mocks/ first)

make lint                 # Run golangci-lint
make fix-lint             # Auto-fix lint issues

make unit-tests           # Unit tests with race detection and coverage
make integration-tests    # Integration tests
make functional-tests     # Functional tests
make system-tests         # End-to-end tests (requires MATLAB, 30min timeout); also runs check-matlab-leaks

make mcp-inspector        # Build and launch @modelcontextprotocol/inspector against the binary (debug tool calls)
make update-coding-guidelines        # Refetch matlab-coding-standards.md from matlab/rules (embedded resource)
make update-live-code-guidelines     # Refetch live-script-generation.md from matlab/rules (embedded resource)
```

After `make system-tests`, `check-matlab-leaks` greps for orphaned MATLAB processes launched by the test binary. A failure there almost always means a `StopSession` path is broken — don't just re-run; investigate.

Run a single test:
```bash
go test -race -run TestName ./path/to/package/...
```

All Go tools (wire, mockery, golangci-lint, gotestsum) are managed as `go tool` dependencies — no separate installation needed.

## Architecture

**Layered clean architecture** with dependency injection via Google Wire:

- **`cmd/matlab-mcp-core-server/`** — Entry point. Creates server with embedded instructions, starts it.
- **`pkg/`** — Public API: `server/` (server lifecycle), `tools/` (tool registration/annotations), `i18n/` (internationalization).
- **`internal/entities/`** — Domain interfaces: `MATLABManager`, `MATLABSessionClient`, `Logger`, `GlobalMATLAB`.
- **`internal/usecases/`** — Business logic per operation (eval code, run file, run tests, check code, detect toolboxes, start/stop session).
- **`internal/adaptors/`** — Implementation layer:
  - `application/` — CLI config parsing, mode selection (Server vs Watchdog), orchestration.
  - `matlabmanager/` — MATLAB session lifecycle (start, stop, reconnect).
  - `mcp/` — MCP protocol: tool handlers (`tools/`), resource providers (`resources/`), server SDK wrapper.
  - `globalmatlab/` — Session persistence: stores/loads connection details as JSON files, reconnects across restarts.
  - `watchdog/` — Separate process monitoring MATLAB health via HTTP. Non-blocking startup.
  - `http/` — HTTP server/client for IPC with MATLAB and watchdog.
- **`internal/facades/`** — OS/IO/process/env abstractions for testability.
- **`internal/wire/`** — Wire provider set (~70 bindings). Run `make wire` after changing dependencies.
- **`internal/messages/`** — Structured error keys and localization strings.

**Key data flow:** MCP Client → stdio → MCP Server → Use Case → MATLAB Manager → HTTP → MATLAB Process

## Fork-Specific Features

This repo is a community fork of the MathWorks official `matlab-mcp-core-server`. The `local/combined` branch is the working branch — it merges three feature/bugfix branches on top of upstream `main`. Detailed design docs are in `new_feature_notes/`; a feature-by-feature review lives in `feature_review.md`.

**Remote topology:** Only `origin` is configured, pointing to `TomRichner/matlab-mcp-core-server`. There is no `upstream` remote — to sync with MathWorks, add it explicitly:

```bash
git remote add upstream git@github.com:matlab/matlab-mcp-core-server.git
git fetch upstream
```

### Session Persistence (`feature/use-last-session`)

Allows MATLAB to survive server restarts. Enabled via `--use-last-session`.

- **`internal/adaptors/sessionfile/`** — Standalone package (no codebase deps) for PID-namespaced session file I/O. Files are named `session_vsc<ppid>_ml<mpid>.json` and stored in OS cache dir by default.
- **`globalmatlab/globalmatlab.go`** — Core logic: `tryReconnectFromSessions()` triages session files by PID liveness (reconnect own, skip foreign live, adopt orphans with `--try-to-adopt`, garbage-collect dead).
- **`matlabmanager/reconnecttosession.go`** — `ReconnectToSession()` creates a client from saved connection details without launching MATLAB.
- **`matlabmanager.DetachedMode`** — Named `bool` wired from `SessionPersistenceConfig`. When true: `StopSession()` is a no-op (MATLAB not killed), `SkipWatchdog=true` (no PID registration).
- **Wire chain:** `SessionPersistenceConfig` → `provideDetachedMode()` → `DetachedMode` → `matlabmanager.New()` + `provideStarter()`.
- Zero-value `SessionPersistenceConfig{}` disables the feature entirely — existing tests need no mock changes.

### Non-Blocking Watchdog (`bugfix/watchdog-non-blocking`)

Prevents server crash when the watchdog process fails to start (e.g., socket timeout under CPU load).

- **`watchdog.Stop()`** — Changed from blocking `<-w.startedC` to `select` with `default` return, preventing deadlock when `Start()` never completed.
- **`orchestrator.go`** — Watchdog `Start()` failure is now a warning, not fatal. Server continues without watchdog.

### Echo Output to Command Window (`bugfix/echo-output-to-command-window`)

Captured MATLAB output is echoed back to the MATLAB command window so the user sees it in both the MCP client and the MATLAB desktop.

## Dependency Injection

The Wire configuration at `internal/wire/wire.go` wires the entire dependency graph. After adding/removing/changing any provider or interface binding, run `make wire` to regenerate `wire_gen.go`.

## Testing Structure

- Unit tests are colocated with source (`*_test.go` alongside implementation).
- `tests/integration/`, `tests/functional/`, `tests/system/` for higher-level tests.
- `tests/testutils/` — Shared test helpers.
- `tests/testconfig/` — Platform-specific test configuration.
- `mocks/` and `tests/mocks/` — Auto-generated by Mockery from `.mockery.yml`. Never edit manually.
- System tests require a local MATLAB installation and check for leaked MATLAB processes after completion.
- `testPlotClose.m` in the repo root is an ad-hoc manual fixture, not part of the Go test suite.

## CLI Flags

Key flags for the binary: `--use-last-session` (reconnect to previous MATLAB), `--last-session-file-path` (persistence directory), `--try-to-adopt` (adopt orphaned sessions), `--matlab-root`, `--initialize-matlab-on-startup`, `--matlab-display-mode` (`desktop`|`nodesktop`), `--disable-telemetry`.

## Environment

- Requires Go 1.25.5+, MATLAB R2020b+ for runtime.
- Build output goes to `.bin/{win64,glnxa64,maci64,maca64}/`.
- Optional `.env` file for setting `MATLAB_MCP_CORE_SERVER_BUILD_DIR` and `MCP_MATLAB_PATH`.
