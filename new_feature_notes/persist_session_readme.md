# Session Persistence Feature

## Overview

When the MATLAB MCP server starts, it normally launches a fresh MATLAB process. If the server restarts (e.g., VS Code reloads), the old MATLAB process is killed on shutdown (post v0.1.0) and a new one is launched. This wastes resources and loses session state.

**Session persistence** solves this by:
1. Saving MATLAB connection details to a file after launch
2. Keeping MATLAB alive across server restarts (detached mode)
3. Reading the session file on next startup to reconnect instead of launching a new process

## CLI Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--use-last-session` | `bool` | `false` | Enable session persistence (reconnect to previous MATLAB if available) |
| `--last-session-file-path` | `string` | OS cache dir | Custom directory for the session file |

### Default Session File Location

Uses `os.UserCacheDir()`:
- **macOS:** `~/Library/Caches/matlab-mcp/sessions/last_matlab_session.json`
- **Linux:** `~/.cache/matlab-mcp/sessions/last_matlab_session.json`
- **Windows:** `C:\Users\<user>\AppData\Local\matlab-mcp\sessions\last_matlab_session.json`

### Session File Format

```json
{
  "api_key": "...",
  "port": "31415",
  "cert_pem": "<base64-encoded TLS certificate PEM>",
  "session_dir": ""
}
```

## How It Works

### Startup Flow (with `--use-last-session`)

```
Server starts
  └─ initializeStartupConfig()
       ├─ SelectMATLABRoot()
       └─ SelectMatlabStartingDir()
  └─ getOrCreateClient()
       ├─ sessionID == 0 && useLastSession?
       │     └─ tryReconnectFromFile()
       │           ├─ Read session file
       │           ├─ Decode base64 cert
       │           ├─ Call ReconnectToSession()
       │           │     ├─ Create client from saved connection details
       │           │     ├─ Ping MATLAB to verify it's alive
       │           │     └─ Register in session store
       │           └─ On failure: delete stale file, fall through
       ├─ sessionID == 0?
       │     └─ startNewSession()
       │           └─ writeSessionFile()  ← saves connection details
       └─ GetMATLABSessionClient()
```

### Key Behaviors

1. **Reconnect succeeds** → Uses existing MATLAB, no new process launched
2. **Reconnect fails** (MATLAB died, cert expired, etc.) → Deletes stale session file, starts new MATLAB, writes new session file
3. **No session file** → Starts new MATLAB normally, writes session file
4. **Feature disabled** (`SessionPersistenceConfig{}`) → Identical to pre-feature behavior, no file I/O
5. **Session file write fails** → Logged as warning, server continues normally (best-effort)
6. **Server shutdown with `--use-last-session`** → MATLAB is NOT killed (detached mode); without the flag, MATLAB is stopped normally

## Architecture

### New Package: `internal/adaptors/sessionfile`

Standalone package with no dependencies on the rest of the codebase. Provides:
- `Write(dir, info)` — Persist session info as JSON
- `Read(dir)` — Load session info from JSON
- `Delete(dir)` — Remove session file
- `DefaultDir()` — OS-specific default directory
- `ResolveDir(flagValue)` — Use flag value or fall back to default

### New Type: `globalmatlab.SessionPersistenceConfig`

```go
type SessionPersistenceConfig struct {
    UseLastSession bool
    SessionFileDir string
}
```

A zero-value struct disables the feature. This is injected via wire at construction time, avoiding any extra `Config()` calls during the hot path. Existing tests pass `SessionPersistenceConfig{}` and require zero changes to their mock expectations.

### Detached Mode: `matlabmanager.DetachedMode`

A named `bool` type used by wire to inject the `UseLastSession` flag into two places:

1. **`MATLABManager`** — sets `detached=true` on session wrappers so `StopSession()` is a no-op (won't send `exit()` to MATLAB)
2. **`localmatlabsession.Starter`** — sets `SkipWatchdog=true` so MATLAB is not registered with the watchdog process (the watchdog normally monitors the server and kills all registered child processes when the server dies)

Both mechanisms are required for MATLAB to survive server restarts. The wire provider chain is: `SessionPersistenceConfig` → `provideDetachedMode()` → `DetachedMode` → `matlabmanager.New()` + `provideStarter()`.

### New Method: `matlabmanager.ReconnectToSession()`

Creates a client from saved connection details (without launching MATLAB), pings to verify liveness, and registers in the session store with a no-op cleanup function (since we didn't launch the process).

### Modified: `matlabmanager.matlabSessionClientWithCleanup`

Added a `detached bool` field. When `detached` is true, `StopSession()` returns nil immediately without sending `exit()` to MATLAB or running the process cleanup. This is set by `MATLABManager` when `DetachedMode` is enabled.

### New Method: `matlabmanager.LastConnectionDetails()`

Returns the `*embeddedconnector.ConnectionDetails` from the most recent `StartMATLABSession()` call. Used by `writeSessionFile()` to persist connection details after launch.

### Interface Changes

The **local** `globalmatlab.MATLABManager` interface was extended with two methods:
```go
ReconnectToSession(ctx, logger, connectionDetails) (SessionID, error)
LastConnectionDetails() *embeddedconnector.ConnectionDetails
```

The **global** `entities.MATLABManager` interface was NOT changed — these methods are only needed by the globalmatlab package.

## Files Changed

### New Files
| File | Purpose |
|------|---------|
| `internal/adaptors/sessionfile/sessionfile.go` | Session file read/write/delete |
| `internal/adaptors/sessionfile/sessionfile_test.go` | 10 tests |
| `internal/adaptors/matlabmanager/reconnecttosession.go` | ReconnectToSession method |
| `internal/adaptors/matlabmanager/reconnecttosession_test.go` | 5 tests |
| `internal/adaptors/globalmatlab/globalmatlab_session_persistence_test.go` | 9 tests |

### Modified Files
| File | Changes |
|------|---------|
| `flags/flags.go` | Added `UseLastSession`, `LastSessionFilePath` constants |
| `parser/parser.go` | Registered new flags |
| `config/config.go` | Added `UseLastSession()`, `LastSessionFilePath()` accessors |
| `messagekeys.go` | Added message keys |
| `messages.go` | Added description strings |
| `globalmatlab/globalmatlab.go` | Core session persistence logic, `SessionPersistenceConfig`, `NewSessionPersistenceConfig`, `tryReconnectFromFile`, `writeSessionFile` |
| `matlabmanager/matlabmanager.go` | Added `lastConnectionDetails` field, `DetachedMode` type, accessor |
| `matlabmanager/matlabsessionclientwithcleanup.go` | Added `detached` field; `StopSession()` is no-op when true |
| `matlabmanager/startmatlabsesssion.go` | Stores connection details, sets `detached` flag on wrapper |
| `localmatlabsession/localmatlabsession.go` | Added `SkipWatchdog` field; conditionally skips watchdog registration |
| `wire/wire.go` | Added `provideDetachedMode`, `provideStarter` providers |
| `wire/wire_gen.go` | Regenerated via `make wire` |
| `mocks/.../Config.go` | Regenerated via `make mockery` |
| `mocks/.../MATLABManager.go` | Regenerated via `make mockery` |
| `globalmatlab_test.go` | Updated constructor calls (5th arg) |
| `globalmatlab_client_test.go` | Updated constructor calls (5th arg) |
| `tests/testconfig/config_darwin.go` | **New** — macOS test config (unblocks `make mockery` on Mac) |

## Test Coverage

**24 new tests total**, all passing:

- **sessionfile** (10): Write, Read, Delete, DefaultDir, ResolveDir — happy paths and error cases
- **reconnecttosession** (5): Happy path, client factory error, ping failure, LastConnectionDetails nil/populated
- **globalmatlab session persistence** (9): Reconnect from file, fallback on failure, writes file after launch, no-ops when disabled, skips reconnect when no file, NewSessionPersistenceConfig variants

All 16 pre-existing globalmatlab tests continue to pass with zero mock expectation changes.

## Generated Files Workflow

All generated files are regenerated using Makefile targets — **never edit them by hand**:

```bash
make wire      # Regenerates internal/wire/wire_gen.go from wire.go
make mockery   # Regenerates all files in mocks/ and tests/mocks/
```

> **Note:** `make mockery` deletes `mocks/` and `tests/mocks/` before regenerating. If mockery fails mid-run, restore from git: `git checkout HEAD -- mocks/ tests/mocks/`

## Known Limitations (v1)

1. **Multi-instance conflict** — Two VS Code windows using the same session directory will overwrite each other's session file.

2. **Orphaned MATLAB on VS Code restart** — Detached mode means MATLAB survives shutdown. If VS Code fully restarts (new PID), the server can't find the old session file and launches a new MATLAB, leaving the old one orphaned.

3. **Certificate rotation** — If MATLAB's self-signed TLS certificate changes, the saved cert will be invalid. Reconnect will fail and fall back to a new session.

## v2 Roadmap: Multi-Session with PID Namespacing

### Problem
v1 uses a single `last_matlab_session.json` file. Multiple VS Code windows overwrite each other, and orphaned MATLABs accumulate.

### Design

**New flag:** `--try-to-adopt` (alongside `--use-last-session`)
**Rename:** `--last-session-file-path` → `--last-session-dir` (already takes a directory)

**File naming:** `session_vsc<ppid>_ml<mpid>.json`

```
~/.cache/matlab-mcp/sessions/
├── session_vsc12345_ml67890.json    ← VS Code 12345 owns MATLAB 67890
├── session_vsc11111_ml22222.json    ← VS Code 11111 (dead), MATLAB 22222 (alive) → adoptable
└── session_vsc33333_ml44444.json    ← both dead → garbage collect
```

### Startup Flow

```
Server starts (my PPID = 12345)

1. Scan session_vsc*_ml*.json files in session dir
2. For each, extract vscPID and mlPID from filename:

   ├─ vscPID == my PPID?
   │     ├─ MATLAB alive → reconnect (my session from last refresh) ✅
   │     └─ MATLAB dead → delete file, continue
   │
   ├─ vscPID alive?
   │     └─ Yes → skip (another VS Code window owns it)
   │
   └─ vscPID dead?
         ├─ MATLAB alive → orphaned, candidate for adoption
         └─ MATLAB dead → delete stale file (garbage collect)

3. If no reconnect happened:
   ├─ --try-to-adopt + orphan candidates exist?
   │     → adopt first one (reconnect + rewrite file with my vscPID)
   └─ No candidates? → launch fresh MATLAB, write new session file
```

### PID Aliveness Check

```go
func isProcessAlive(pid int) bool {
    process, err := os.FindProcess(pid)
    if err != nil {
        return false
    }
    // Signal 0 doesn't kill — just checks if process exists
    err = process.Signal(syscall.Signal(0))
    return err == nil
}
```

### Key Properties

- **Zero user configuration** — no per-project paths needed
- **Multi-instance safe** — each VS Code window gets its own session file
- **Self-cleaning** — dead JSON files are garbage collected on startup
- **Never kills MATLAB** — orphaned MATLABs stay alive; user manages them
- **Adoption is opt-in** — requires `--try-to-adopt` flag
- **Backward compatible** — old `last_matlab_session.json` treated as legacy v1 file
