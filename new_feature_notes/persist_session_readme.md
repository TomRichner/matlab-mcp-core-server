# Session Persistence Feature

## Overview

When the MATLAB MCP server starts, it normally launches a fresh MATLAB process. If the server restarts (e.g., VS Code reloads), the old MATLAB process is killed on shutdown (post v0.1.0) and a new one is launched. This wastes resources and loses session state.

**Session persistence** solves this by:
1. Saving MATLAB connection details to PID-namespaced session files after launch
2. Keeping MATLAB alive across server restarts (detached mode)
3. Scanning session files on next startup to reconnect instead of launching a new process
4. Supporting multiple concurrent VS Code windows without conflict
5. Optionally adopting orphaned MATLAB sessions from dead VS Code instances

## CLI Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--use-last-session` | `bool` | `false` | Enable session persistence (reconnect to previous MATLAB if available) |
| `--last-session-file-path` | `string` | OS cache dir | Custom directory for session files |
| `--try-to-adopt` | `bool` | `false` | When set alongside `--use-last-session`, adopt orphaned MATLAB sessions from dead VS Code windows |

### Default Session File Location

Uses `os.UserCacheDir()`:
- **macOS:** `~/Library/Caches/matlab-mcp/sessions/`
- **Linux:** `~/.cache/matlab-mcp/sessions/`
- **Windows:** `C:\Users\<user>\AppData\Local\matlab-mcp\sessions\`

### Session File Naming

Files use PID-namespaced names: `session_vsc<ppid>_ml<mpid>.json`

```
~/.cache/matlab-mcp/sessions/
├── session_vsc12345_ml67890.json    ← VS Code 12345 owns MATLAB 67890
├── session_vsc11111_ml22222.json    ← VS Code 11111 (dead), MATLAB 22222 (alive) → adoptable
└── session_vsc33333_ml44444.json    ← both dead → garbage collect
```

### Session File Format

```json
{
  "api_key": "...",
  "port": "31415",
  "cert_pem": "<base64-encoded TLS certificate PEM>",
  "session_dir": "",
  "parent_pid": 12345,
  "matlab_pid": 67890
}
```

## How It Works

### Startup Flow (with `--use-last-session`)

```
Server starts (my PPID = os.Getppid())
  └─ initializeStartupConfig()
       ├─ SelectMATLABRoot()
       └─ SelectMatlabStartingDir()
  └─ getOrCreateClient()
       ├─ sessionID == 0 && useLastSession?
       │     └─ tryReconnectFromSessions()
       │           ├─ ScanSessions() — read all session_vsc*_ml*.json
       │           ├─ For each session file, triage by PID liveness:
       │           │     ├─ PPID == my PPID && MATLAB alive → reconnect ✅
       │           │     ├─ PPID == my PPID && MATLAB dead → delete stale file
       │           │     ├─ PPID alive (not mine) → skip (another VS Code owns it)
       │           │     ├─ PPID dead && MATLAB alive → orphan candidate
       │           │     └─ PPID dead && MATLAB dead → garbage collect (delete)
       │           ├─ If no reconnect && --try-to-adopt && orphans exist:
       │           │     └─ Adopt first orphan (reconnect + rewrite with my PPID)
       │           └─ On failure: delete stale file, fall through
       ├─ sessionID == 0?
       │     └─ startNewSession()
       │           └─ writeSessionFile()  ← saves connection details with PIDs
       └─ GetMATLABSessionClient()
```

### Key Behaviors

1. **Reconnect succeeds** → Uses existing MATLAB, no new process launched
2. **Reconnect fails** (MATLAB died, cert expired, etc.) → Deletes stale session file, starts new MATLAB, writes new session file
3. **No session files** → Starts new MATLAB normally, writes session file
4. **Feature disabled** (`SessionPersistenceConfig{}`) → Identical to pre-feature behavior, no file I/O
5. **Session file write fails** → Logged as warning, server continues normally (best-effort)
6. **Server shutdown with `--use-last-session`** → MATLAB is NOT killed (detached mode); without the flag, MATLAB is stopped normally
7. **Multiple VS Code windows** → Each gets its own `session_vsc<ppid>_ml<mpid>.json`; no conflicts
8. **Garbage collection** → Dead session files (both PIDs gone) are automatically cleaned up on startup
9. **Orphan adoption** → With `--try-to-adopt`, the server adopts the first available orphaned MATLAB session
10. **Never kills MATLAB** → Orphaned MATLABs are left running for the user to manage

## Architecture

### Package: `internal/adaptors/sessionfile`

Standalone package with no dependencies on the rest of the codebase. Provides:
- `Write(dir, info, parentPID, matlabPID)` — Persist session info as PID-namespaced JSON file
- `ScanSessions(dir)` — Scan directory for all session files, return parsed contents with PIDs
- `DeleteFile(filePath)` — Remove a specific session file by path
- `IsProcessAlive(pid)` — Check if a process exists via `syscall.Signal(0)`
- `DefaultDir()` — OS-specific default directory
- `ResolveDir(flagValue)` — Use flag value or fall back to default

### Type: `globalmatlab.SessionPersistenceConfig`

```go
type SessionPersistenceConfig struct {
    UseLastSession bool
    SessionFileDir string
    TryToAdopt     bool
}
```

A zero-value struct disables the feature. This is injected via wire at construction time, avoiding any extra `Config()` calls during the hot path. Existing tests pass `SessionPersistenceConfig{}` and require zero changes to their mock expectations.

### Type: `embeddedconnector.ConnectionDetails`

```go
type ConnectionDetails struct {
    Host           string
    Port           string
    APIKey         string
    CertificatePEM []byte
    MatlabPID      int
}
```

The `MatlabPID` field is populated by `localmatlabsession.StartLocalMATLABSession()` when it launches the MATLAB process. This PID is used by `writeSessionFile()` to create the PID-namespaced filename.

### Detached Mode: `matlabmanager.DetachedMode`

A named `bool` type used by wire to inject the `UseLastSession` flag into two places:

1. **`MATLABManager`** — sets `detached=true` on session wrappers so `StopSession()` is a no-op (won't send `exit()` to MATLAB)
2. **`localmatlabsession.Starter`** — sets `SkipWatchdog=true` so MATLAB is not registered with the watchdog process (the watchdog normally monitors the server and kills all registered child processes when the server dies)

Both mechanisms are required for MATLAB to survive server restarts. The wire provider chain is: `SessionPersistenceConfig` → `provideDetachedMode()` → `DetachedMode` → `matlabmanager.New()` + `provideStarter()`.

### Method: `matlabmanager.ReconnectToSession()`

Creates a client from saved connection details (without launching MATLAB), pings to verify liveness, and registers in the session store with a no-op cleanup function (since we didn't launch the process). Also stores `lastConnectionDetails` (so `writeSessionFile` can persist the session) and sets `detached` mode on the wrapper (so the adopted MATLAB survives the next server shutdown).

### Modified: `matlabmanager.matlabSessionClientWithCleanup`

Added a `detached bool` field. When `detached` is true, `StopSession()` returns nil immediately without sending `exit()` to MATLAB or running the process cleanup. This is set by `MATLABManager` when `DetachedMode` is enabled.

### Method: `matlabmanager.LastConnectionDetails()`

Returns the `*embeddedconnector.ConnectionDetails` from the most recent `StartMATLABSession()` call. Used by `writeSessionFile()` to persist connection details after launch.

### Interface Changes

The **local** `globalmatlab.MATLABManager` interface was extended with two methods:
```go
ReconnectToSession(ctx, logger, connectionDetails) (SessionID, error)
LastConnectionDetails() *embeddedconnector.ConnectionDetails
```

The **global** `entities.MATLABManager` interface was NOT changed — these methods are only needed by the globalmatlab package.

## Files Changed

### New/Rewritten Files
| File | Purpose |
|------|---------|
| `internal/adaptors/sessionfile/sessionfile.go` | PID-namespaced session file scan/write/delete/aliveness |
| `internal/adaptors/sessionfile/sessionfile_test.go` | 14 tests |
| `internal/adaptors/matlabmanager/reconnecttosession.go` | ReconnectToSession method |
| `internal/adaptors/matlabmanager/reconnecttosession_test.go` | 5 tests |
| `internal/adaptors/globalmatlab/globalmatlab_session_persistence_test.go` | 12 tests |

### Modified Files
| File | Changes |
|------|---------|
| `flags/flags.go` | Added `UseLastSession`, `LastSessionFilePath`, `TryToAdopt` constants |
| `parser/parser.go` | Registered all three flags |
| `config/config.go` | Added `UseLastSession()`, `LastSessionFilePath()`, `TryToAdopt()` accessors |
| `messagekeys.go` | Added message keys |
| `messages.go` | Added description strings |
| `globalmatlab/globalmatlab.go` | Core session persistence logic: `SessionPersistenceConfig` (with `TryToAdopt`), `tryReconnectFromSessions` (PID triage + orphan adoption + garbage collection), `tryReconnectToSession`, `writeSessionFile` (with PIDs) |
| `embeddedconnector/client.go` | Added `MatlabPID int` to `ConnectionDetails` struct |
| `localmatlabsession/localmatlabsession.go` | Populates `MatlabPID` from launched process ID; `SkipWatchdog` support |
| `matlabmanager/matlabmanager.go` | Added `lastConnectionDetails` field, `DetachedMode` type, accessor |
| `matlabmanager/matlabsessionclientwithcleanup.go` | Added `detached` field; `StopSession()` is no-op when true |
| `matlabmanager/startmatlabsesssion.go` | Stores connection details, sets `detached` flag on wrapper |
| `wire/wire.go` | Added `provideDetachedMode`, `provideStarter` providers |
| `wire/wire_gen.go` | Regenerated via `make wire` |
| `mocks/.../Config.go` | Regenerated via `make mockery` |
| `mocks/.../MATLABManager.go` | Regenerated via `make mockery` |
| `globalmatlab_test.go` | Updated constructor calls (5th arg) |
| `globalmatlab_client_test.go` | Updated constructor calls (5th arg) |
| `tests/testconfig/config_darwin.go` | macOS test config (unblocks `make mockery` on Mac) |

### Deleted Files
| File | Reason |
|------|--------|
| `new_feature_notes/last_matlab_session.json` | Legacy v1 sample file, no longer applicable |

## Test Coverage

**31 tests total**, all passing:

- **sessionfile** (14): Write (PID-namespaced), ScanSessions (happy path, empty dir, nonexistent dir, malformed filenames, invalid JSON), DeleteFile, IsProcessAlive, DefaultDir, ResolveDir
- **reconnecttosession** (5): Happy path, client factory error, ping failure, LastConnectionDetails nil/populated
- **globalmatlab session persistence** (12): Reconnect from PID-namespaced file, fallback on failure, writes file after launch with PID verification, no-ops when disabled, skips reconnect when no file, garbage collects dead sessions, NewSessionPersistenceConfig variants (disabled, enabled, custom path, config error, with TryToAdopt)

All pre-existing globalmatlab tests continue to pass with zero mock expectation changes.

## Generated Files Workflow

All generated files are regenerated using Makefile targets — **never edit them by hand**:

```bash
make wire      # Regenerates internal/wire/wire_gen.go from wire.go
make mockery   # Regenerates all files in mocks/ and tests/mocks/
```

> **Note:** `make mockery` deletes `mocks/` and `tests/mocks/` before regenerating. If mockery fails mid-run, restore from git: `git checkout HEAD -- mocks/ tests/mocks/`

## Known Limitations

1. **Certificate rotation** — If MATLAB's self-signed TLS certificate changes, the saved cert will be invalid. Reconnect will fail and fall back to a new session.

2. **Orphan accumulation** — Orphaned MATLABs are never killed by the server. They stay alive until the user manually terminates them or the machine reboots.

3. **Race condition at scale** — Multiple servers starting simultaneously in the same session directory could theoretically race on orphan adoption. In practice this is unlikely with the single-VS-Code-window-per-PPID model.

4. **PID reuse** — If the OS reassigns a dead VS Code's PID to a new unrelated process, the server might misidentify a session file as "owned by another VS Code window" and skip it. This is extremely unlikely in practice.
