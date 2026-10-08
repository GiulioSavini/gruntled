---
phase: 11-report-single-instance-release-proof
plan: 03
subsystem: cmd/gruntled (watch composition root)
tags: [single-instance, ipc, snapshot, render, sanitize]
requires:
  - ipc lock / Listen / Serve / Query / WriteDump / ReadDump (11-02)
  - statusfile.EnsureRepoDir, presenter.SanitizeReason (11-01)
provides:
  - renderReport(format, rep) shared by check and the daemon
  - buildSnapshot(rep, gen) pre-rendered text/summary/json/sarif bytes
  - acquireInstance (lock-first single instance) + instance.store / close
  - watchDeps.sleep seam
affects: [11-04 report command, 11-06 docs]
tech-stack:
  added: []
  patterns: [lock-first single instance, atomic.Pointer snapshot hand-off, close-before-release shutdown, blocking loaderHook to test the indexing window]
key-files:
  created:
    - cmd/gruntled/render.go
    - cmd/gruntled/instance.go
    - cmd/gruntled/instance_test.go
    - cmd/gruntled/instance_unix_test.go
  modified:
    - cmd/gruntled/main.go
    - cmd/gruntled/watch.go
    - cmd/gruntled/watch_test.go
decisions:
  - "Already-running line: unix `gruntled: already watching <root> (pid N); socket: <sock>; status: <path>`, without pid/status when 3 pings (900ms each, 100ms pause) fail; report-file mode `gruntled: already watching <root>; report: <dump>[; status: <path>]`"
  - "ipc.Listen returning ErrUnsupported (non linux/darwin unix) falls back to report-file mode instead of failing"
  - "instance.close does server.Close -> dump removal -> lock.Release explicitly in one deferred call (same order as the planned defer chain)"
  - "Snapshot stored before the status line is written, so a status-line wait in tests implies the snapshot is visible"
  - "testDeps(t) now gives every daemon test a private short runtime base (envAt(os.MkdirTemp(\"\", \"g\"))); tests never touch the real XDG_RUNTIME_DIR"
requirements-completed: [DAEMON-05]
metrics:
  duration: 6min
  completed: 2026-10-08
  tasks: 2
  files: 7
---

# Phase 11 Plan 03: Wire ipc into watch Summary

`gruntled watch` takes the per-repo lock before the watcher and the initial index start, serves pre-rendered snapshots over the AF_UNIX socket (report file on windows), and prints where an existing daemon runs and exits 0; check and the daemon render through one function.

## Tasks

| Task | Name | Commits |
| ---- | ---- | ------- |
| 1 | Shared renderer and snapshot building | 3481536 (test), c2d73b0 (feat) |
| 2 | Lock-first single instance, serving, dump, already-running | 4cb5053 (test), eccd5d5 (feat) |

## What was built

- `render.go`: `renderReport` (text => out + summary, json/sarif => out, unknown => error) and `buildSnapshot`. `runCheck` now calls `renderReport`; stdout, stderr and exit codes unchanged (golden/e2e/testscript suites untouched and green).
- `instance.go`: `acquireInstance` runs after `--print-status-path` / inside-repo checks: `EnsureRepoDir` -> `CheckSockPath` (unix goos) -> `LockRetry` (1 attempt unix, 3 windows) -> `ErrHeld` prints the location line and returns 0 -> `Listen` + `Serve(ln, info, snap.Load)` or initial `WriteDump(nil)`. Paths always from `statusfile.Dir(root)`.
- `watch.go`: publisher builds a snapshot per Ready (generation counter), keeps the last good bytes as `failed` with `SanitizeReason(reason)` on Failed, and hands every snapshot to `instance.store` (atomic pointer + dump rewrite, failures logged once). Reindex-failed stderr line sanitised.

## Verification

- `go test -count=1 ./...` green; new and existing watch/cmd tests `-count=10` green (4.3s).
- `go vet` clean on linux/{amd64,arm64}, darwin/{amd64,arm64}, windows/{amd64,arm64}; freebsd build ok.
- `bash scripts/check-architecture.sh` exit 0 with ipc linked into cmd/gruntled. `go list -deps ./cmd/gruntled` on all six targets: no `net`, `net/*`, `os/exec`; `golang.org/x/sys/unix` present on linux/darwin only through fsnotify (pre-existing, ipc itself has none).
- Tests: second instance (socket and report-file mode, different `--status-file`, no watcher started, A still pings and reindexes), no-ping fallback line with 2 injected sleeps, stale socket + unheld lock recovered, regular file at sock => exit 3 untouched, sock path > 103 bytes => exit 3 naming length and XDG_RUNTIME_DIR with nothing created, windows goos accepts the same path, indexing window (blocking loaderHook) serves nil snapshot / indexing dump, Ready bytes equal `buildSnapshot`, failed snapshot keeps bytes with sanitised reason, clean shutdown removes sock/dump and releases the lock, `--print-status-path` creates nothing.
- `-race` and `scripts/test-check-architecture.sh` left to CI per orchestrator constraint.

## Deviations from Plan

- [Rule 2 - Missing critical] `ErrUnsupported` from `ipc.Listen` (unix targets other than linux/darwin) falls back to report-file mode instead of exiting 3. Files: cmd/gruntled/instance.go. Commit eccd5d5.
- [Rule 3 - Blocking] `testDeps()` became `testDeps(t)` with a private short runtime base: with the lock now taken on every start, tests would otherwise write lock/socket under the real `$XDG_RUNTIME_DIR`. Files: cmd/gruntled/watch_test.go. Commit 4cb5053.
- Socket tests live in `instance_unix_test.go` (`linux || darwin`); report-file-mode tests in `instance_test.go` run on every host.
- The failed-snapshot behaviour is tested by driving `watchPublisher` directly (no fixture makes the real indexer fail deterministically).
- `watchUsage` / docs not changed (exit 0 for "already watching" is documented by 11-06).

## Self-Check: PASSED
