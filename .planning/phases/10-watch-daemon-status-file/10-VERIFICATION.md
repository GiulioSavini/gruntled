---
phase: 10-watch-daemon-status-file
verified: 2026-10-08T00:00:00Z
status: passed
score: 4/4 must-haves verified
---

# Phase 10: Watch Daemon & Status File Verification Report

**Phase Goal:** User can leave `gruntled watch` running and see current diagnostics from a prompt or editor bar after each save.
**Status:** passed. **Re-verification:** No.

## Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | Full index at start, then only changed files reindexed after ~150 ms trailing debounce | VERIFIED | `--debounce` default `watch.DefaultQuiet` (150ms) in cmd/gruntled/watch.go; `watching.Indexer.Index` calls `Loader.Invalidate(dirty...)`; tests TestRunInitialIndex, TestRunOneSave, TestRunBurstCoalesces, TestWatchReindexesOnlyChanged, TestWatchDebounceDefault pass |
| 2 | `.git`, `.terraform`, `.terragrunt-cache`, swap/backup ignored; new/deleted dirs picked up | VERIFIED | internal/infrastructure/watch/ignore.go (3 dirs, `~`, `.swp`, `.swo`); pending.go drops ignored paths; native_unix.go handles Create of dirs; ignore/native/contract tests pass |
| 3 | One-line status written atomically to documented per-repo path outside repo, cat-able | VERIFIED | statusfile/write.go: MkdirAll 0700, insecure-dir refusal, CreateTemp (0600) + rename; path.go uses EvalSymlinks; `--print-status-path`; docs/cli.md documents it; TestWatchStatusFile and TestWatchStatusInsideRepo pass |
| 4 | `--poll` (and windows) uses stat polling with same results as fsnotify | VERIFIED | poll.go; native_unix.go is `//go:build !windows`, native_windows.go stub; TestWatchParity, TestWatchBackendSelection and the contract suite pass; windows/amd64+arm64 and darwin/arm64 cross-builds OK |

**Score:** 4/4

## Required Artifacts

All exist, are substantive and are wired from cmd/gruntled/watch.go: watch/{ignore,pending,debounce,poll,native_unix,native_windows,run,watch}.go, application/watching/watching.go, statusfile/{path,write}.go, terragrunt/loader.go (mutex, batch Invalidate), presenter status lines, docs/cli.md.

## Binary no-net/no-exec proof (Step 9)

scripts/check-architecture.sh Step 9 covers 6 targets (linux/darwin/windows x amd64/arm64): `go tool nm` symbol check (os/exec, x/sys Exec/ForkExec/StartProcess etc.), exemption narrowed to exactly golang.org/x/sys/unix, and windows asserts no fsnotify / x/sys/windows in deps. Run result: `architecture: OK`. (The self-test script was intentionally not run.)

## Phase 8 Security Findings

| # | Finding | Status | Evidence |
|---|---------|--------|----------|
| 1 | Loader not concurrency-safe | Resolved | Loader mutex (loader.go) plus single indexer goroutine in run.go |
| 2 | Dropped events leave stale cache | Resolved | Resync path `Invalidate(".")` in watching.Indexer; native adapter safety net; TestRunResync |
| 3 | O(n*cache) per-path Invalidate | Resolved | Batch Invalidate, single pass |
| 4 | Non repo-relative paths silently ignored | Resolved | Contract documented; pending.add turns out-of-contract paths into resync |
| 5 | Stat per load (cost note only) | N/A | Informational |

## Requirements Coverage

| Requirement | Plans | Status | Evidence |
|-------------|-------|--------|----------|
| DAEMON-01 | 10-02..10-07 | SATISFIED | Truths 1, 2 |
| DAEMON-03 | 10-03, 10-06 | SATISFIED | Truth 3 (windows path handled; perm check skipped on windows) |

Traceability: REQUIREMENTS.md marks both Phase 10 / Complete. No orphans: DAEMON-02 is Phase 8; DAEMON-04/05/06 are Phase 11 (pending, out of scope).

## Tests

- `go test -count=1 ./...`: all packages ok.
- `bash scripts/check-architecture.sh`: OK.
- -race not run by instruction.

## Anti-Patterns

None blocking found in the scanned files.

## Human Verification (non-blocking, optional)

- Real save in an editor on WSL /mnt/ or macOS kqueue: confirm native events and the hint to use `--poll`.
- Check the status line renders in a tmux or prompt bar.

## Gaps Summary

None.

_Verifier: Claude (gsd-verifier)_

## Security

Audit by proj-sec:auditor (2026-10-08), verdict fix-small. No critical or high findings.

| # | Severity | Location | Finding | Status |
|---|----------|----------|---------|--------|
| 1 | Medium | scripts/check-architecture.sh `sym_re` + text scan | Direct `syscall.CreateProcess`/`CreateProcessAsUser` on windows bypassed the no-exec proof (syscall is standard; regexes lacked the names) | Fixed in fix(10-sec): names added to both halves; self-test `binary-syscall-createprocess-windows` |
| 2 | Low | internal/infrastructure/statusfile/write.go:188-199 | Dir check uses `os.Stat` (follows symlinks) and only mode bits; on TempDir fallback (HOME unset) another user can pre-create `/tmp/gruntled/<hash12>` as a symlink or foreign-owned 0700 dir | Open: Lstat, reject symlink, require owner uid == Geteuid (unix) |
| 3 | Low | internal/interfaces/presenter/status.go:301, cmd/gruntled/watch.go:251 | Control bytes (ESC) from `err.Error()` reach status line and stderr | Open: drop `unicode.IsControl` runes before 120-rune cut |
| 4 | Low | scripts/test-check-architecture.sh `run_case_msg` | Fixed names `/tmp/tca-out.$$`, `/tmp/tca-err.$$` in shared /tmp | Open: one `mktemp -d` per run, cleaned via COPIES |
| 5 | Low | internal/infrastructure/watch/native_unix.go:125 | WalkDir→Add race: dir swapped for symlink gets watched outside root (extra events only; loader reads via os.Root) | Open (optional): Lstat before addWatch |
