---
phase: 10-watch-daemon-status-file
plan: 04
subsystem: infrastructure/watch
tags: [watch, debounce, ignore, poll, contract-tests, daemon]
requires:
  - "nothing new: platform-neutral core, stdlib only (os, io/fs, path, sync, time)"
provides:
  - "watch.Watcher / Changes / ErrWatchLimit / ErrNativeUnavailable: pull-style dirty-set port (Ready buffered(1), Take, idempotent Close)"
  - "watch.Ignored(rel) / IgnoredDir(name): .git, .terraform, .terragrunt-cache components; ~, .sw[pox n], .#*, #*#, ___jb_tmp___/___jb_old___, *.tmp, .DS_Store, 4-5 digit vim probe; vendor is watched"
  - "pending accumulator: single mutex, take drains Ready, out-of-contract paths (empty, '.', absolute, volume, backslash, '..') become resync"
  - "watch.Debouncer + DefaultQuiet 150ms / DefaultMaxWait 1s: pure state machine (Add/Due/Flush with caller time)"
  - "watch.NewPoll(root, interval) + DefaultPollInterval 500ms; unexported scanner (newScanner/diff/resync) reusable as the native safety net"
  - "runContract(t, newWatcher) in contract_test.go (no build tag) for the native adapter"
affects: [10-05 (native adapter runs runContract, reuses scanner), 10-06 (cmd chooses poll/native), 10-07 (run loop uses Debouncer + Watcher)]
tech-stack:
  added: []
  patterns: [pull-style accumulator with mutex-held signal/drain, pure debouncer state machine, synchronous baseline in constructor, shared adapter contract suite, sentinel-file negative assertions]
key-files:
  created:
    - internal/infrastructure/watch/watch.go
    - internal/infrastructure/watch/ignore.go
    - internal/infrastructure/watch/ignore_test.go
    - internal/infrastructure/watch/pending.go
    - internal/infrastructure/watch/pending_test.go
    - internal/infrastructure/watch/debounce.go
    - internal/infrastructure/watch/debounce_test.go
    - internal/infrastructure/watch/poll.go
    - internal/infrastructure/watch/poll_test.go
    - internal/infrastructure/watch/contract_test.go
    - internal/infrastructure/watch/helpers_test.go
  modified: []
decisions:
  - "pending.add treats '.' and Windows volume paths (C:/...) as out-of-contract -> resync, in addition to empty/absolute/backslash/'..'"
  - "Ignored paths are dropped silently and never signal Ready"
  - "Debouncer ignores empty Changes (does not arm); Flush resets the burst so maxWait counts from the next burst's first change"
  - "Poll scan reports a directory only on add/remove/type change, never on its own mtime moving (children are diffed individually)"
  - "Unreadable subdirectory keeps its previous snapshot entries for that scan (no spurious remove/re-add); only root failure sets resync and keeps the whole previous snapshot"
  - "NewPoll returns an error for a missing or non-directory root"
requirements-completed: []
requirements-advanced: [DAEMON-01]
metrics:
  duration: 5min
  completed: 2026-10-08
  tasks: 2
  files: 11
---

# Phase 10 Plan 04: Watch core (ignore, debounce, pending, poll adapter) Summary

The new `internal/infrastructure/watch` package holds the platform-neutral half of the watch daemon: the Watcher port, the ignore predicate, a race-free pending accumulator, a pure debouncer with the 150 ms / 1 s defaults pinned, and a stat-poll adapter that passes a shared contract suite the native adapter (10-05) will also run. DAEMON-01 is advanced, not complete: the run loop (10-07) and CLI (10-06) are still missing.

## What was built

- **Port (`watch.go`)**: `Changes{Paths, Resync}` plus `Empty()`, and `Watcher{Ready, Take, Close}`. `ErrWatchLimit` and `ErrNativeUnavailable` are declared here for 10-05 and 10-06.
- **Ignore (`ignore.go`)**: `Ignored` checks every path component against the ignored directory set, then checks the base name against the editor swap, backup and probe patterns. `IgnoredDir` is for walkers. `vendor` is deliberately watched.
- **Pending (`pending.go`)**: `add` is the only way a path gets in. It normalises with `path.Clean`, drops ignored paths, and turns anything outside the contract into resync (this enforces Phase 8 finding 4 at the boundary). Signal and drain both happen under the same mutex, so a leftover Ready signal always means data is really pending.
- **Debouncer (`debounce.go`)**: `Due = min(last+quiet, first+maxWait)`. Tests use fixed `time.Time` values and no clock.
- **Poll (`poll.go`)**: the constructor takes the baseline snapshot before it returns (Pattern 4 start order). One ticker goroutine then diffs each new scan against the last. The scan uses lstat semantics, never follows symlinks, and skips ignored dirs and files. Added or removed directories report the dir path and also every child path. `Close` uses `sync.Once` plus a `WaitGroup`. Edits that keep the same size and mtime cannot be seen; the code comment says so.
- **Tests**: `runContract` covers create, modify, delete, rename (old and new path), mkdir with files followed by an edit inside, rmdir, a vim-style save, ignored dirs checked with a sentinel, no escapes, Ready, and idempotent Close. The `scanner` tests are synchronous: no goroutine, no timing.

## Verification

- `go test -count=1 ./internal/infrastructure/watch/`: all green. `-count=5`, `-count=30`, and `-cpu 1,2,8 -count=3` on the contract suite are all green (contract suite about 0.15 s).
- `time.Sleep` appears only inside `eventually`'s 5 ms poll loop. No test sleeps and then asserts.
- `GOOS=windows GOARCH=arm64 go vet` and `GOOS=darwin GOARCH=arm64 go vet` on the package: clean.
- `bash scripts/check-architecture.sh`: OK. `go test ./...`: green.
- Not run with `-race` locally (no C compiler). All shared state sits behind the pending mutex; the collector and scanner each belong to a single goroutine.

## Commits

- 677d785 test(10-04): add failing tests for ignore predicate, pending accumulator, debouncer
- c533e2f feat(10-04): watch contract types, ignore predicate, pending accumulator, debouncer
- 9784403 test(10-04): add failing watcher contract suite and poll adapter tests
- 2d9b22d feat(10-04): watch core - ignore, debounce, pending, poll adapter

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 2 - Correctness] Extra out-of-contract inputs become resync**
- **Found during:** Task 1
- **Issue:** The plan lists empty, absolute, backslash and `..` escapes. Two other inputs also break the repo-relative contract: `.` (the whole root) and a Windows volume path like `C:/x` (on linux, `filepath.IsAbs` does not treat it as absolute).
- **Fix:** `normalise` rejects both, so they become resync. Both cases are covered in `TestPendingOutOfContractBecomesResync`.
- **Commit:** c533e2f

**2. [Rule 2 - Correctness] Unreadable subdirectory keeps previous entries**
- **Found during:** Task 2
- **Issue:** If an unreadable subdirectory were simply skipped, its children would look removed on this scan and added again on the next, giving spurious dirty paths each time.
- **Fix:** `keepPrevious` copies the subtree's previous entries into the new snapshot.
- **Commit:** 2d9b22d

Beyond the plan, tests were added for `IgnoredDir`, a missing root, a vanished root (resync), and a symlinked dir that must not be followed.

## Notes for next plans

- 10-05: call `runContract(t, func(root string) (Watcher, error) { return NewNative(root) })` from `native_unix_test.go`. For the 30 s safety net, reuse `newScanner(root)`: feed `diff()` into `pending.add` and `resync` into `markResync`.
- 10-07: the loop should check `len(w.Ready()) > 0` to skip publishing intermediate results. This is reliable because `take` drains Ready under the mutex.

## Self-Check: PASSED
