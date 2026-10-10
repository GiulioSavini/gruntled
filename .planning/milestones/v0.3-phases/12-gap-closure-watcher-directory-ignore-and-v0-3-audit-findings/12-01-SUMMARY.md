---
phase: 12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings
plan: 01
subsystem: watch
tags: [watcher, ignore-rules, native, poll, safety-net, daemon-parity, security]
requires:
  - phase: 10
    provides: watch.Watcher adapters (poll, native), pending accumulator, contract suite
  - phase: 11
    provides: gruntled watch daemon and TestWatchParity harness
provides:
  - "IgnoredEntry(rel, typ): editor patterns apply to files and gone entries, to symlinks only via the Emacs '.#' rule, never to directories"
  - pending.add(rel, typ) as the single type-aware filter for every adapter
  - scanner diff returning the entry type with each path (poll and native safety net)
  - native patternDirs set, type-aware addTree/handleEvent, Lstat before addWatch
  - TestWatchPatternDirParity (poll + native end-to-end)
affects: [12-05]
tech-stack:
  added: []
  patterns: [ignore rule keyed by entry type; event paths classified only when the name matches an editor pattern]
key-files:
  created:
    - cmd/gruntled/watch_patterndir_test.go
  modified:
    - internal/infrastructure/watch/ignore.go
    - internal/infrastructure/watch/ignore_test.go
    - internal/infrastructure/watch/pending.go
    - internal/infrastructure/watch/pending_test.go
    - internal/infrastructure/watch/run_test.go
    - internal/infrastructure/watch/poll.go
    - internal/infrastructure/watch/poll_test.go
    - internal/infrastructure/watch/native_unix.go
    - internal/infrastructure/watch/native_unix_test.go
    - internal/infrastructure/watch/contract_test.go
key-decisions:
  - "Ignored deleted; IgnoredEntry is the only predicate, so no caller can keep using the type-blind rule"
  - "scanner.diff returns []change{rel, typ}: type from the current scan for added/changed paths, from s.prev for removed ones"
  - "IgnoredDir check in scanner.walk and addTree dropped: the component rule in IgnoredEntry subsumes it (IgnoredDir itself unchanged)"
  - "handleEvent drops component-ignored paths first (IgnoredEntry(rel, fs.ModeDir)), then classifies only names matching editorPattern"
  - "TestNativePatternDirReplacedByFile builds the native struct by hand (no loop goroutine) so handleEvent and patternDirs are touched from one goroutine"
requirements-completed: []
duration: 9min
completed: 2026-10-10
---

# Phase 12 Plan 01: Watcher Directory Ignore Summary

**Editor file patterns now apply only to files. Directories named `2024`, `4913`, `x.tmp`, `bak~`, `#d#` or `.#d` are walked, watched (native), scanned (poll and the safety net) and reindexed. Emacs lock symlinks and vim probes still never reach the dirty set. This closes the v0.3 audit BLOCKER (bus #11) and carried INFO 10-sec#5.**

## Performance

- Duration: ~9 min
- Tasks: 3
- Files: 1 created, 10 modified

## Accomplishments

- `IgnoredEntry(rel, typ)` (ignore.go:36): `.git`/`.terraform`/`.terragrunt-cache` components are ignored for every type. A directory is never pattern-ignored (ignore.go:54). A symlink is ignored only by the `.#` Emacs lock rule (ignore.go:57). Everything else, including typ 0 for gone entries, gets the full editor patterns. `Ignored` is removed. `ignoredBase` and `isVimProbe` are unchanged. `editorPattern(rel)` (ignore.go:66) is the type-blind name check that native uses to decide when an Lstat is needed.
- `pending.add(rel, typ)` (pending.go:35) filters with `IgnoredEntry`. Out-of-contract paths still become resync whatever typ is.
- Poll scanner: `walk` skips `IgnoredEntry(childRel, d.Type())` (poll.go:120). `diff()` returns `[]change{rel, typ}`. Removed paths take their type from `s.prev` (poll.go:89). Both `poll.run` (poll.go:191) and the native safety net (native_unix.go:210) call `add(c.rel, c.typ)`.
- Native `addTree`: type-aware skip (native_unix.go:131), `n.add(rel, d.Type())` on emit, and Lstat before `addWatch` (native_unix.go:146). Pattern-named directories are recorded in `patternDirs` (native_unix.go:161). This includes the subtree root when addTree runs from handleEvent. The repo root is never recorded.
- Native `handleEvent`: component-ignored paths are dropped first (native_unix.go:233). `Remove`/`Rename` prune rel and every key under rel/ (native_unix.go:238, prunePatternDirs at native_unix.go:169). Only names that match an editor pattern are Lstat'ed (native_unix.go:246). typ is `fs.ModeDir` when the path is a recorded pattern dir OR Lstat says dir (native_unix.go:250). Normal names pay no extra syscall.
- Red evidence:
  - Contract sub-tests on the Task 1 tree (before the adapter fix, a7b5801 minus the unit tests):
    ```
    --- FAIL: TestWatcherContractNative/edit_inside_pattern-named_dirs  timed out ... last state: paths=[] resync=false
    --- FAIL: TestWatcherContractNative/mkdir_pattern-named_dir         timed out waiting for edit inside new pattern-named directory; paths=[]
    --- FAIL: TestWatcherContractNative/rename_pattern-named_dir_away   timed out ...; last state: paths=[y y/f.hcl]
    --- FAIL: TestWatcherContractNative/mkdir_pattern-named_dir_after_start,_then_rename_it_away  timed out waiting for 2024/f.hcl ...; paths=[]
    (same four FAIL lines for TestWatcherContractPoll; emacs lock symlink PASS, as on HEAD)
    ```
  - TestWatchPatternDirParity on the pre-plan code (worktree at 0f9d90f with the new test file copied in):
    ```
    watch_patterndir_test.go:54: timed out after 10s waiting for status gruntled: 1 error (GRT001×1) @ 14:02:11; last state: gruntled: ok @ 14:02:11
    --- FAIL: TestWatchPatternDirParity/poll (10.02s)
    --- FAIL: TestWatchPatternDirParity/native (10.01s)
    ```
- Mutation checks, each reverted after the run:
  - IgnoredEntry made type-blind (no dir exemption): the four pattern-dir sub-tests FAIL on both poll and native.
  - `.#` symlink rule removed: `emacs lock symlink` FAILs on both adapters (`Emacs lock symlink reached the dirty set: paths=[.#terragrunt.hcl s.hcl s0.hcl]`).
  - `known` dropped from the handleEvent classification: TestNativePatternDirReplacedByFile FAILs (`after Rename: Take() = {Paths:[] Resync:false}, want [2024]`).
  - Lstat-before-watch disabled: TestNativeAddTreeSkipsSwappedDir FAILs (`swapped directory ".../real" was watched`).

## Task Commits

1. Task 1, failing type-aware ignore/pending tests: `f7ecd2f` (build failed: undefined IgnoredEntry, too many args to p.add)
2. Task 1, IgnoredEntry and pending filter: `d041ff4`
3. Task 2, failing pattern-dir watcher regressions: `a7b5801`
4. Task 2, adapters pass the entry type, patternDirs, Lstat before watch: `a77c041`
5. Task 3, end-to-end daemon regression (red on 0f9d90f, green on a77c041): `8915285`

## Verification

- `go test -count=1 -run 'TestIgnored|TestPending' ./internal/infrastructure/watch/`: ok
- `go test -count=3 -run 'TestWatcherContract|TestNativePattern|TestPollScanner|TestNativeAddTree' ./internal/infrastructure/watch/`: ok
- `go test -count=1 -run TestWatchPatternDirParity -v ./cmd/gruntled/`: poll PASS (0.06s), native PASS (0.04s)
- `go vet ./...`: clean. `GOOS=windows` and `GOOS=darwin` `go vet ./internal/infrastructure/watch/ ./cmd/gruntled/`: clean. watch also vetted for linux/amd64, darwin/arm64, windows/amd64: clean
- `go test -count=1 ./...`: every package ok (cmd/gruntled 19.4s, terragrunt 5.9s, watch 1.1s, ...)
- `bash scripts/check-architecture.sh`: `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)`, exit 0
- Existing contract sub-tests ("vim-style save", "ignored dirs via sentinel", ...), TestNativeIgnoredDirNotWatched (still exactly root and `a` watched), TestWatchParity and TestWatchReindexesOnlyChanged pass.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-12-01-1 | mitigated | ignore.go:54 (dirs never pattern-ignored); poll.go:120, native_unix.go:131, native_unix.go:250 pass the real type; contract sub-tests contract_test.go:138/157/178/188, TestWatchPatternDirParity |
| T-12-01-2 | mitigated | ignore.go:57 (`.#` rule kept for symlinks); typ 0 for gone entries gets every pattern; contract "vim-style save" unchanged and "emacs lock symlink" (contract_test.go:203) green on both adapters |
| T-12-01-3 | mitigated | Lstat only when `editorPattern(rel)` holds (native_unix.go:246); normal names unchanged |
| T-12-01-4 | mitigated (narrowed) | lstatBeforeWatch before addWatch, skip on symlink or non-dir (native_unix.go:146); TestNativeAddTreeSkipsSwappedDir. Residual TOCTOU accepted as planned |
| T-12-01-5 | accepted | patternDirs is bounded by the pattern-named dirs in the repo; pruned on every Remove/Rename (native_unix.go:169, :238) |
| T-12-01-6 | mitigated | `known || Lstat dir` (native_unix.go:250); TestNativePatternDirReplacedByFile and contract "mkdir pattern-named dir after start, then rename it away" |

## Deviations from Plan

**1. [Rule 1, test strength] Emacs lock sub-test waits on a barrier before removing the link**
- As specified (symlink, remove, sentinel), the sub-test passed even with the `.#` symlink rule removed. Native processed both events after the link was already gone (Lstat failed, so typ 0 and the file patterns applied), and poll never saw the link at all. That made the mutation check vacuous. The sub-test now writes `s0.hcl` after creating the link and waits for it before removing the link. Native events are ordered, and a poll scan that sees `s0.hcl` also sees the link, so the link is observed while it exists. The mutation now fails on both adapters. This change is in the fix commit a77c041, because the sub-test was green on HEAD either way.

**2. [Clarity] Extra test TestPollScannerDiffTypes**
- This test asserts that removed entries carry their type from `s.prev` (`2024` as ModeDir, `2024/f.hcl` as 0) and added ones carry it from the current scan. It is in poll_test.go, which is in files_modified.

## Known Risks / Unverified

- `-race` could not run locally (`go: -race requires cgo`, no gcc). CI runs it on linux, macos and windows.
- The darwin (kqueue) and windows runtime behaviour is left to CI. Locally only cross-vet ran. On windows, TestWatchPatternDirParity/native skips like TestWatchParity, and the emacs sub-test skips if os.Symlink fails.
- `requirements-completed` is `[]`. DAEMON-01/02 also depend on 12-05's rapid model layer, per the orchestrator instruction.

## Self-Check: PASSED
