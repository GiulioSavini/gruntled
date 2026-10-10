---
phase: 10-watch-daemon-status-file
plan: 03
subsystem: interfaces/presenter, infrastructure/statusfile
tags: [status-file, presenter, atomic-write, permissions, daemon]
requires:
  - diagnostic.Set / Code / Severity
provides:
  - "presenter.StatusLine / StatusIndexing / StatusFailed / StatusStopped: one line + newline, single io.WriteString, caller-supplied stamp"
  - "statusfile.Env / OSEnv, Dir, Path: <base>/gruntled/<sha256(canonical root)[:12]>/status, base = linux absolute XDG_RUNTIME_DIR -> UserCacheDir -> TempDir"
  - "statusfile.Inside(root, p): symlink-aware containment check for rejecting --status-file inside the repo"
  - "statusfile.Writer / NewWriter / ErrInsecureDir: 0700 dir, 0600 temp + rename, injectable Rename/Sleep, 5 attempts with 10/20/40/80 ms backoff"
affects: [10-06 (daemon writes status via presenter + Writer; cmd uses Path and Inside)]
tech-stack:
  added: []
  patterns: [golden one-line presenter, tmp-in-same-dir + rename, injected env and rename/sleep seams]
key-files:
  created:
    - internal/interfaces/presenter/status.go
    - internal/interfaces/presenter/status_test.go
    - internal/infrastructure/statusfile/path.go
    - internal/infrastructure/statusfile/write.go
    - internal/infrastructure/statusfile/path_test.go
    - internal/infrastructure/statusfile/write_test.go
  modified: []
decisions:
  - StatusFailed collapses whitespace with strings.Fields (also trims ends); a blank-only reason reads "unknown error"; cap is 120 runes after collapsing
  - Warnings never appear on the status line; warnings-only reads "ok"
  - Root hash input is filepath.Abs + EvalSymlinks, not case-folded on any OS (documented aliasing limitation)
  - Insecure-dir check applies only to the leaf status dir and only when not on windows; MkdirAll never chmods an existing dir
  - Inside resolves p through its deepest existing ancestor so non-existent targets are still checked; different volumes count as outside
requirements-completed: []
requirements-advanced: [DAEMON-03]
metrics:
  duration: 4min
  completed: 2026-10-08
  tasks: 2
  files: 6
---

# Phase 10 Plan 03: Status line presenter and atomic status file Summary

The status line is a pure presenter (`gruntled: 2 errors (GRT001×1 GRT003×1) @ 14:02:11`), and the new `statusfile` package works out a per-repository path outside the repo and replaces the file atomically with owner-only permissions. Nothing is wired yet; 10-06 connects them to the daemon.

## What was built

- **presenter/status.go**: `StatusLine` counts only `SeverityError` per code, sorts the codes, and uses "error" for exactly 1 and "errors" otherwise. `StatusIndexing`, `StatusStopped` and `StatusFailed` cover the fixed states. Every function writes the whole line in a single `io.WriteString`. Imports are io, sort, strconv, strings and domain only, so there is no time import and the architecture check still passes.
- **statusfile/path.go**: `Env` (GOOS, Getenv, UserCacheDir, TempDir) is injected and `OSEnv()` supplies the real values. `Dir` builds the path as base + `gruntled` + a 12-hex hash of the canonical root, which leaves a short path for Phase 11's socket. `Path` adds `status`. `Inside` uses `filepath.Rel`, so `/r/repo2` does not count as inside `/r/repo`.
- **statusfile/write.go**: `Writer{Path, Rename, Sleep}`. `Write` runs MkdirAll 0700, refuses a dir whose `perm&0o077 != 0` with `ErrInsecureDir` (except on windows), then does CreateTemp `.status-*`, write, close, and renames with retries. It sleeps between attempts but not after the last one, removes the temp file on any failure, and never logs.

## Tests

- `TestStatusLine` (table: ok, 1 error, 2 errors in sorted order, 3 errors counted per code, warnings only) also checks that each line takes exactly 1 Write call. `TestStatusLineFixedStates` covers indexing, stopped, failed, an empty reason, a blank reason and whitespace collapse. `TestStatusLineFailedTruncation` uses 130 two-byte runes and expects exactly 120 runes back as valid UTF-8. `TestStatusLineEncoding` checks for no BOM, U+00D7 encoded as C3 97, and exactly one line. `TestStatusLineWriterError` checks that a writer error is returned.
- `TestDirBaseSelection` covers linux with an absolute, relative and empty XDG, darwin and windows ignoring XDG, and the cache-error fallback to temp; it checks both Dir and Path. Other Dir tests: `TestDirHashStable`, `TestDirSymlinkAlias` (skipped if symlinks are unsupported), `TestDirMissingRoot`, `TestDirInside` (root, nested non-existent path, sibling prefix, parent, elsewhere) and `TestDirInsideViaSymlink`. The bases are t.TempDir values, so they are absolute on every host OS.
- `TestWriteCreatesDirAndFile` checks the content, an overwrite, modes 0700 (leaf and parent) and 0600, and that no leftovers remain. `TestWriteInsecureDir` (unix only) chmods a dir to 0755 and expects ErrInsecureDir with nothing written. `TestWriteConcurrentReader` runs 200 alternating writes against a hammering reader. All shared state goes through the filesystem plus channels, so it is safe under CI's `-race`; it was not run with -race locally because there is no C compiler. `TestWriteRenameRetry` covers 0, 2 and 4 failures and an always-failing rename: it checks the call count, the recorded sleeps 10/20/40/80 ms, that the error is returned, that no status file exists after failure, and that no temp files are left.
- `go test -count=1 ./...` is green, `-count=10` on statusfile is green, `scripts/check-architecture.sh` reports OK, and cross `go vet ./...` passes for windows/arm64 and darwin/arm64.

## Task Commits

| Task | Name | Commit |
|------|------|--------|
| 1 (RED) | failing status line golden tests | 41bc8a0 |
| 1 (GREEN) | status line presenter | d2d4f11 |
| 2 (RED) | failing statusfile path/writer tests | 59b9fe9 |
| 2 (GREEN) | statusfile path resolution and atomic writer | e428e36 |

## Deviations from Plan

1. **[Rule 1 - Bug] Test-only: the counting writer was bypassed.** The first `countingWriter` embedded `bytes.Buffer`, so `io.WriteString` called the promoted `WriteString` and the Write counter stayed at 0. The fix is a named buffer field with no `WriteString` method. Fixed in d2d4f11.
2. **Commit message reuse.** The plan's suggested message "feat(10-03): status line presenter and atomic status file writer" is used for the Task 2 GREEN commit. Task 1 got its own feat commit, following the per-task protocol.
3. **Windows tolerance in the concurrent reader test.** On windows, read errors and write errors caused by a racing reader are tolerated, because a reader that holds the file open can block the replace. Torn reads still fail the test on every OS. CI is linux-only, so the strict path is the one that runs there.

## Requirements

DAEMON-03 is advanced, not completed. The line format, path rule, permissions and atomicity exist and are tested on their own. 10-06 wires them into the daemon and the `--status-file` flag, and DAEMON-03 is completed there.

## Self-Check: PASSED
