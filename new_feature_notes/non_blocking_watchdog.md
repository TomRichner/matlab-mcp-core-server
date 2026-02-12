# Watchdog Non-Blocking Fix

## Problem

When starting the MATLAB MCP server via Antigravity (VS Code fork), the server intermittently crashed with a deadlock panic during startup:

```
{"level":"ERROR","msg":"Failed to connect to watchdog socket","error":"socket file access timed out"}
{"level":"INFO","msg":"Initiating MATLAB MCP Core Server application shutdown"}
fatal error: all goroutines are asleep - deadlock!
goroutine 1 [chan receive]:
github.com/.../watchdog.(*Watchdog).Stop(...)
    watchdog.go:100 +0x28
```

### Root Cause

Two issues in the watchdog lifecycle:

1. **Deadlock in `Stop()`**: The watchdog used a `startedC` channel as a synchronization barrier. Both `Stop()` and `RegisterProcessPIDWithWatchdog()` blocked on `<-w.startedC`, waiting for `Start()` to complete. But when `Start()` failed (socket timeout), the channel was never closed — `Stop()` blocked forever, causing a deadlock in the orchestrator's `defer` cleanup.

2. **Fatal `Start()` failure**: The orchestrator treated watchdog `Start()` failure as fatal (`return err`), so the MCP server never started. Even without the deadlock, the server would exit and VS Code would show an error.

### Why Intermittent

The watchdog is a separate Go process. `Start()` launches it, then polls for its Unix Domain Socket file every 500ms with a **10-second timeout**. Under CPU load during Antigravity startup (multiple MCP servers, extensions, possibly a persisted MATLAB consuming resources), the watchdog process can't create its socket file in time.

### Why the Watchdog is Non-Essential

The watchdog monitors the MCP server and kills registered child MATLAB processes when the server dies unexpectedly. With `--use-last-session`, `SkipWatchdog=true` means zero PIDs are registered — the watchdog has no purpose. Even without `--use-last-session`, the watchdog is a safety net, not critical infrastructure.

## Fixes

### Fix 1: Non-blocking `Stop()` (watchdog.go)

Changed `Stop()` from blocking to non-blocking when `Start()` never completed:

```diff
 func (w *Watchdog) Stop() error {
-    <-w.startedC
+    select {
+    case <-w.startedC:
+    default:
+        // Start() never completed — nothing to stop.
+        return nil
+    }
     w.logger.Debug("Sending graceful shutdown signal to watchdog")
     _, err := w.client.SendStop()
     return err
 }
```

### Fix 2: Non-fatal `Start()` (orchestrator.go)

Changed watchdog startup from fatal error to warning:

```diff
-err := o.watchdogClient.Start()
-if err != nil {
-    return err
-}
+if err := o.watchdogClient.Start(); err != nil {
+    logger.WithError(err).Warn("Watchdog startup failed, continuing without watchdog")
+}
```

## Files Changed

| File | Change |
|------|--------|
| `internal/adaptors/watchdog/watchdog.go` | Non-blocking `Stop()` |
| `internal/adaptors/watchdog/watchdog_test.go` | `TestWatchdog_Stop_ReturnsNilIfNotStarted` |
| `internal/adaptors/application/orchestrator/orchestrator.go` | Non-fatal `Start()` |
| `internal/adaptors/application/orchestrator/orchestrator_test.go` | `TestOrchestrator_WatchdogStartError_ContinuesRunning` |

## Design Rationale

- The watchdog is kept in place for all configurations — it may grow new responsibilities in the future
- `RegisterProcessPIDWithWatchdog()` still blocks on `<-w.startedC` — this is correct because it only runs after `Start()` in normal flow, and `SkipWatchdog=true` prevents it from being called in `--use-last-session` mode
- The `startedC` channel pattern is preserved as the synchronization mechanism, with the minimal fix of non-blocking `Stop()` to handle the error path
