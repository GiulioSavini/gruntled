---
phase: 10-watch-daemon-status-file
plan: 05
subsystem: infrastructure/watch
tags: [watch, fsnotify, inotify, kqueue, safety-net, build-tags, daemon]
requires:
  - "10-01: Step 9 x/sys/unix exemption + per-target nm proof (keeps go get fsnotify green)"
  - "10-04: Watcher port, pending accumulator, Ignored/IgnoredDir, scanner, runContract"
provides:
  - "watch.NewNative(root) Watcher (fsnotify, //go:build !windows); windows stub returns ErrNativeUnavailable"
  - "watch.DefaultSafetyNet = 30s stat re-scan feeding the same dirty set"
  - "watch.WarnPollAdvised(root): linux + /mnt/ prefix hint (false on windows)"
  - "github.com/fsnotify/fsnotify v1.10.1 direct requirement; x/sys stays v0.46.0"
affects: [10-06 (cmd picks native vs poll, ErrWatchLimit fallback, /mnt hint, re-runs six-target proof with fsnotify linked), 10-07 (run loop)]
tech-stack:
  added: [github.com/fsnotify/fsnotify v1.10.1]
  patterns: [build-tag split adapter + stub, package-var test seams (addWatch, dropEvent), baseline-after-watches ordering, stat safety net over event stream]
key-files:
  created:
    - internal/infrastructure/watch/native_unix.go
    - internal/infrastructure/watch/native_windows.go
    - internal/infrastructure/watch/native_unix_test.go
  modified:
    - go.mod
    - go.sum
decisions:
  - "ENFILE treated as a watch-limit error alongside ENOSPC/EMFILE; fsnotify.NewWatcher failing with a limit errno also yields ErrWatchLimit"
  - "Safety-net scanner baseline taken AFTER all watches are installed, so a change in the gap is seen by fsnotify, the scanner, or both"
  - "addTree with emit marks every non-ignored path under a created/renamed-in dir dirty (files and subdirs), plus the dir itself via the event"
  - "Subdirectory Add failures other than the limit (vanished, EACCES) skip that subtree silently; the safety net covers it. A limit error after construction -> markResync, keep running"
  - "Events on root itself or outside root map to '.'/'..' and become Resync through pending.add"
  - "Test seams are package vars (addWatch, dropEvent) set before construction and restored by t.Cleanup registered before the watcher's Close (LIFO), so no race with the loop goroutine"
requirements-completed: []
requirements-advanced: [DAEMON-01]
metrics:
  duration: 8min
  completed: 2026-10-08
  tasks: 1
  files: 5
---

# Phase 10 Plan 05: fsnotify native watcher Summary

fsnotify v1.10.1 native watcher (inotify/kqueue) behind `//go:build !windows`, with recursive watches on created/renamed-in directories, overflow-to-Resync, ENOSPC/EMFILE/ENFILE-to-ErrWatchLimit, and a 30 s stat safety net feeding the same dirty set; windows gets a stub returning ErrNativeUnavailable and links no fsnotify. DAEMON-01 advanced, not complete (CLI 10-06 and run loop 10-07 pending).

## Tasks

| Task | Name | Commits | Files |
| ---- | ---- | ------- | ----- |
| 1 (TDD) | dependency, native adapter, windows stub, contract run | 6a160de (RED), 4a7bb5d (GREEN) | go.mod, go.sum, native_unix.go, native_windows.go, native_unix_test.go |

## What was built

- **native_unix.go**: `NewNative(root)` = `newNative(root, DefaultSafetyNet)`. Stat-checks root, creates the fsnotify watcher, `addTree(root)` via `filepath.WalkDir` (skips `IgnoredDir`/`Ignored`, does not follow symlinks), then takes the scanner baseline and starts one `loop` goroutine selecting on Events, Errors, safety ticker and done.
- `handleEvent`: rel via `filepath.Rel` + `ToSlash`, drop ignored, `add(rel)` for every Op; on Create of a non-symlink dir, `addTree(dir, emit=true)` watches and marks the whole new tree dirty.
- `handleErr`: any error (ErrEventOverflow included) -> `markResync`.
- `Close`: sync.Once closes done + fw, WaitGroup waits for loop; Ready/Take usable afterwards.
- **native_windows.go**: `NewNative` -> `ErrNativeUnavailable`, `WarnPollAdvised` -> false. No imports.

## Verification

- `go test -count=1 ./internal/infrastructure/watch/`: green (contract for poll and native + 4 native tests).
- `-count=5 -run Native`, `-count=30` native contract, `-count=3 -cpu 1,2,8` whole package: all green. No sleep-then-assert (only `eventually`'s 5 ms poll).
- `go mod tidy -diff`: clean. fsnotify direct; x/sys v0.46.0 unchanged.
- `bash scripts/check-architecture.sh`: `architecture: OK` before the first commit and after the implementation.
- `go vet` + `go build` of the package on windows/{amd64,arm64}, darwin/{amd64,arm64}, linux/{amd64,arm64}: clean.
- `GOOS=windows [GOARCH=arm64] go list -deps ./internal/infrastructure/watch | grep -E 'fsnotify|x/sys/windows'`: empty.
- `go test ./...`: green.
- Not run with `-race` locally (no C compiler); CI covers it.

Note: cmd/gruntled does not import the watch package yet, so the six-target nm proof with fsnotify actually linked is re-run in 10-06.

## Deviations from Plan

None - plan executed as written. (ENFILE added to the limit errnos, as the plan's "also ENFILE" allowed.)

## Self-Check: PASSED
