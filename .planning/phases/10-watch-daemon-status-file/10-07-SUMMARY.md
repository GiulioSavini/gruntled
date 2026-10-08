---
phase: 10-watch-daemon-status-file
plan: 07
subsystem: infrastructure/watch
tags: [watch, daemon, run-loop, debounce, single-indexer]
requires:
  - "10-04: Watcher port, pending take-drains-Ready invariant, Debouncer"
provides:
  - "watch.Run(ctx, Config) error: single indexer goroutine (initial index, debounced reindex, failure handling, shutdown)"
  - "watch.Config{Watcher, Indexer, Publish, Now, Quiet, MaxWait}"
  - "watch.Event{Kind, Report, Err} with EventIndexing/EventReady/EventFailed/EventStopped"
  - "watch.Indexer seam (satisfied by *watching.Indexer, not imported)"
affects: [10-06 (cmd wires Run to statusfile.Writer and stdout)]
tech-stack:
  added: []
  patterns: [single owner goroutine for the Indexer, reusable Stop+drain timer with nil channel when idle, publish-skip gated on real pending data, timer-authoritative flush]
key-files:
  created:
    - internal/infrastructure/watch/run.go
    - internal/infrastructure/watch/run_test.go
  modified: []
decisions:
  - "Initial index failure publishes Failed (not Stopped) and returns 'initial index: %w'; cancellation during any Index call publishes Stopped, returns nil, never Failed"
  - "Run closes the Watcher on every return once the config is valid; nil Watcher/Indexer/Publish is an error before anything runs"
  - "On timer fire the batch is flushed at max(Now(), Due()): the timer is re-armed on every Add, so its firing is authoritative even with a lagging/fixed injected clock"
  - "Reindex result is skipped only when a post-Index Take yields non-empty Changes; a stale/spurious Ready falls through to Publish"
requirements-completed: []
requirements-advanced: [DAEMON-01, DAEMON-03]
metrics:
  duration: 10min
  completed: 2026-10-08
  tasks: 1
  files: 2
---

# Phase 10 Plan 07: Watch run loop Summary

`watch.Run` is the daemon's single indexer goroutine. It publishes Indexing, runs the initial full index, then reindexes once per debounced batch with exactly the dirty paths. A failed reindex publishes Failed and keeps running. Cancellation publishes Stopped and returns nil. An intermediate result is skipped only while newer changes are really pending, so the status never stays stale. DAEMON-01 and DAEMON-03 are advanced, not complete: 10-06 (CLI wiring + status file) completes them.

## What was built

- **`run.go`**: `Indexer`, `EventKind`, `Event`, `Config`, `Run`, `stopTimer`. Selects on `ctx.Done()`, `Watcher.Ready()` and one reusable `time.Timer` (channel set to nil while the debouncer is idle).
  - Ready: Take; non-empty goes to `Debouncer.Add`; re-arm. If a skipped result is still unpublished and the debouncer is empty, publish it.
  - Timer: Flush, then Index. Afterwards, if `len(Ready())>0`, Take into the debouncer. If the debouncer then has work, skip and re-arm. Otherwise publish Ready.
- **`run_test.go`**: fake watcher built on the real `pending` (so the take-drains-Ready invariant holds), plus `spurious()` to send a Ready with no data. A fake Indexer records calls, has a per-call hook and an atomic in-flight counter (any overlap fails the test in Cleanup). Each call returns a report with its own graph pointer, so every published report can be traced to the call that made it. The clock is injected and fixed. Gates block chosen Index calls. Cases: initial, one save, 20-feed burst giving one union call, resync, reindex error then recovery, initial error (wrapped, `[indexing failed]`, Close once), skip intermediate, stale signal still publishes, spurious Ready after a skip ends with the last result published, cancel (idle / during initial / during reindex), a 200-path stress test (all paths indexed, last event = last call), and config validation.

## Verification

- `go test -count=1 ./internal/infrastructure/watch/`: green.
- `go test -count=20 -cpu 1,2,8 -run TestRun`: green (about 32 s).
- `GOOS=windows GOARCH=arm64` and `GOOS=darwin GOARCH=arm64 go vet`: clean.
- `bash scripts/check-architecture.sh`: OK. `go test ./...`: green.
- No `time.Sleep` in run_test.go; every wait goes through `eventually` or a gate channel with a deadline.
- Not run with `-race` locally (no C compiler). Shared test state is behind mutexes or atomics. Loop state belongs to the Run goroutine.

## Commits

- 72e87a5 test(10-07): add failing tests for watch run loop
- c81aa7d feat(10-07): watch run loop

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] A fixed injected clock would never flush**
- **Found during:** Task 1
- **Issue:** The plan asks for tests with `Now` returning a fixed time. In that case `Flush(now())` never reaches `Due() = now+quiet`, so no batch would ever be indexed.
- **Fix:** When the timer fires, flush at `max(now(), Due())`. The timer is re-armed to `Due()` on every Add, so its firing is authoritative.
- **Commit:** c81aa7d

**2. [Rule 2 - Correctness] Cancellation is shutdown, not failure; initial failure is not followed by Stopped**
- **Found during:** Task 1
- **Issue:** The plan says "on exit: Publish(Stopped)". If that applied to the initial-failure exit too, the Failed status would be overwritten just before the exit-3 return.
- **Fix:** Stopped is published only on cancellation (idle, or during any Index call). Initial failure leaves Failed as the last event. The Watcher is closed on every return.
- **Commit:** c81aa7d

**3. [Rule 2 - Correctness] Config validation**
- Run returns an error (no Index call) when Watcher, Indexer or Publish is nil.

Beyond the plan, tests cover cancellation during the initial index and during a reindex, a 200-path stress run, and config validation. The burst test uses a 300 ms quiet window rather than 5 ms, so 20 back-to-back feeds can never be split by scheduler jitter.

## Notes for next plans

- 10-06: build the Watcher first (Pattern 4), then call `watch.Run(ctx, watch.Config{Watcher: w, Indexer: watching.NewIndexer(...), Publish: ...})`. Map EventIndexing/Ready/Failed/Stopped to statusfile lines. `Run`'s non-nil error means initial index failure (exit 3). Run closes the Watcher itself.

## Self-Check: PASSED
