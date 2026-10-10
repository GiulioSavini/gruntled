---
phase: 12-gap-closure-watcher-directory-ignore-and-v0-3-audit-findings
plan: 03
subsystem: watch-daemon-state
tags: [statusfile, runtime-dir, case-insensitive, file-identity, security]
requires:
  - phase: 10
    provides: statusfile.Dir, statusfile.Inside, EnsureRepoDir
  - phase: 11
    provides: acquireInstance (lock, socket, report file)
provides:
  - statusfile.Inside by file identity (os.SameFile on existing ancestors) with string fast path
  - statFn seam for simulating two spellings of one directory
  - acquireInstance refusal (exit 2) when the runtime directory lies inside the repository
affects: [12-05]
tech-stack:
  added: []
  patterns: [identity-not-spelling path containment; refuse before create]
key-files:
  created:
    - internal/infrastructure/statusfile/inside_internal_test.go
  modified:
    - internal/infrastructure/statusfile/path.go
    - internal/infrastructure/statusfile/path_test.go
    - cmd/gruntled/instance.go
    - cmd/gruntled/instance_test.go
key-decisions:
  - "The ancestor walk starts at filepath.Abs(p) (lexical), not at the symlink-resolved form; os.Stat follows symlinked ancestors anyway"
  - "A stat error on the root itself is returned; a stat error on an ancestor of p is skipped"
  - "The statFn seam (var statFn = os.Stat) went into the test commit so the red run fails on assertions, not on a build error"
requirements-completed: [DAEMON-03, DAEMON-05]
duration: 6min
completed: 2026-10-10
---

# Phase 12 Plan 03: State Path Guards Summary

**`statusfile.Inside` now decides by file identity, not spelling. A case variant or alias of the root counts as inside. `gruntled watch` exits 2 when the per-repository runtime directory (lock, socket, report file) would land inside the repository, and it checks this before creating anything or starting a watcher. This closes sec LOW bus #5 and LOW bus #12.**

## Performance

- Duration: ~6 min
- Tasks: 2
- Files: 1 created, 4 modified

## Accomplishments

- `Inside` (path.go:80) keeps the canonical/Rel comparison as a fast `true` (`insideByName`, path.go:89). When that says outside, it stats the root and walks every ancestor of `p`, `p` included, up to the volume root. It returns true on `os.SameFile` (path.go:101). Ancestors that do not exist or cannot be stat'ed are skipped. `Dir` still does not case-fold, so the hashing is unchanged.
- `acquireInstance` (instance.go:71-84) computes `statusfile.Dir(root, env)` and asks `statusfile.Inside(root, rt)`. It refuses with `gruntled: runtime directory <rt> is inside the repository <root>; set XDG_RUNTIME_DIR (linux) or the user cache directory outside it` and `exitUsage`. All of this runs before `EnsureRepoDir`, the first call that creates anything. The guard covers socket mode and report-file mode. `--print-status-path` returns before `acquireInstance`, so it still takes no lock.
- Red output, for the record:
  ```
  695def2  --- FAIL: TestInsideBySameFile/case_variant_of_root
             Inside(".../001/repo", ".../001/Repo/st/status") = false, want true
           --- FAIL: TestInsideBySameFile/case_variant_itself
           --- FAIL: TestInsideSkipsUnreadableAncestor
  063b618  --- FAIL: TestWatchRuntimeDirInsideRepo/socket       exit 3, want 2; stderr: gruntled: cannot start watcher: no
           --- FAIL: TestWatchRuntimeDirInsideRepo/report-file  exit 3, want 2; stderr: gruntled: cannot start watcher: no
  ```
  On HEAD both modes created the lock (and the report file) inside `<repo>/cache` and went on to start the watcher.
- I also ran the real case-variant test, `TestDirInsideCaseVariant`, on a case-insensitive FS here by setting `TMPDIR=/mnt/c/...` (WSL drvfs). It FAILS at 695def2 and PASSES at 7106694. On ext4 it skips with "filesystem at ... is case-sensitive". CI covers macos and windows.

## Task Commits

1. Task 1 red: Inside identity tests and statFn seam: `695def2`
2. Task 1 green: SameFile ancestor walk: `7106694`
3. Task 2 red: TestWatchRuntimeDirInsideRepo (socket + report-file): `063b618`
4. Task 2 green: acquireInstance refusal: `865ef5f`

## Verification

- `go vet ./...`: ok
- `GOOS=windows go vet ./internal/infrastructure/statusfile/ ./cmd/gruntled/`: ok
- `GOOS=darwin go vet ./internal/infrastructure/statusfile/ ./cmd/gruntled/`: ok
- `go test -count=1 ./...`: every package ok (cmd/gruntled 18.2s, statusfile 0.14s, watch 1.2s, ...)
- `go test -count=1 -run 'TestWatchRuntimeDirInsideRepo|TestWatchDumpMode|TestWatchSecondInstance|TestPrintStatusPathTakesNoLock|TestWatchStatusInsideRepo|TestSockPathTooLong' ./cmd/gruntled/`: all PASS
- `bash scripts/check-architecture.sh`: `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)`, exit 0. The change adds no new imports.
- TestDirInside and TestDirInsideViaSymlink are unchanged and pass.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-12-03-1 | mitigated | cmd/gruntled/instance.go:71-84 (refusal before EnsureRepoDir at :84); TestWatchRuntimeDirInsideRepo socket + report-file (instance_test.go) |
| T-12-03-2 | mitigated | same guard; the test asserts that `<repo>/cache` is absent, that no lock/sock/report exists anywhere under the repo, and that no watcher was constructed |
| T-12-03-3 | mitigated | internal/infrastructure/statusfile/path.go:80-108 (SameFile walk at :101); TestInsideBySameFile, TestInsideSkipsUnreadableAncestor (seam), TestDirInsideCaseVariant (real, case-insensitive hosts) |
| T-12-03-4 | accepted | one stat per ancestor, once at startup |
| T-12-03-5 | accepted | the message echoes the user's own env-derived paths on their own stderr |
| T-12-03-6 | accepted | a false "inside" fails closed: exit 2, nothing created |

## Deviations from Plan

**1. [Rule 3, TDD hygiene] statFn seam declared in the red commit**
- `var statFn = os.Stat` (path.go:36) is in 695def2 together with the tests. Without it the red run is a build failure, not a failing assertion. The seam does nothing until 7106694 uses it.

**2. [Extra test] TestInsideSkipsUnreadableAncestor**
- This test is not in the plan's behaviour list. It pins the action's rule that "a stat error other than not-exist on an ancestor is skipped" and that the call never errors because of it. It lives in inside_internal_test.go, which is in files_modified.

## Known Risks / Unverified

- `-race` could not run locally (`go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`, and there is no gcc). CI ubuntu runs it.
- The real case-variant test was exercised on drvfs only. APFS and NTFS-native runs are left to CI.
- When no `--status-file` is given and the runtime base is inside the repo, the existing status-path check in watch.go fires first ("status file ... is inside the repository"). It is still exit 2, but the message names the status file rather than the runtime directory. watch.go was out of scope.

## Self-Check: PASSED
