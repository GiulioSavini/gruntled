---
phase: 11-report-single-instance-release-proof
plan: 05
subsystem: release-proof
tags: [architecture, ci, ipc, no-net-no-exec, self-test]
requires:
  - phase: 11-02
    provides: ipc package (raw AF_UNIX socket, flock, CreateFile share 0)
  - phase: 11-03
    provides: cmd/gruntled links ipc
provides:
  - Step 9 self-tests pinning net out of both ipc builds and allowing raw syscall sockets
  - test-check-architecture.sh with one private mktemp -d per run
  - CI test-os job (macos-latest, windows-latest)
affects: [11-07]
tech-stack:
  added: []
  patterns: [probe file injected into a linked package, asserted per release target]
key-files:
  created: []
  modified:
    - scripts/check-architecture.sh
    - scripts/test-check-architecture.sh
    - .github/workflows/ci.yml
key-decisions:
  - "Step 9 needs no new exemption for ipc: sockets/locks are stdlib syscall only; regexes and exemption list untouched (comment-only diff)"
  - "Self-test output and all repo copies live under one OUT_DIR=$(mktemp -d) removed by the EXIT trap"
  - "test-os runs go test -count=1 ./... without -race; ubuntu keeps -race and the architecture scripts"
requirements-completed: [DAEMON-06]
duration: 12min
completed: 2026-10-08
---

# Phase 11 Plan 05: Release Proof with Socket Code + OS Test Matrix Summary

**The six-target no-net/no-exec proof passes with ipc linked and has three new self-tests (net in unix ipc file, net in windows ipc file, raw syscall sockets allowed); self-tests now use a private temp dir; CI runs go test on macOS and Windows.**

## Performance

- Duration: ~12 min
- Tasks: 2
- Files modified: 3

## Accomplishments

- Real tree `bash scripts/check-architecture.sh` passes with `internal/infrastructure/ipc` linked into cmd/gruntled, no new exemption.
- New cases in `scripts/test-check-architecture.sh`:
  - `binary-net-in-unix-socket-file`: `//go:build linux || darwin` probe in ipc importing net; asserts `package net` on all four linux/darwin targets and on no windows target.
  - `binary-net-in-windows-ipc-file`: windows probe; asserts both windows targets and no linux/darwin target.
  - `binary-raw-socket-allowed`: syscall.Socket/Bind(SockaddrUnix)/Listen/Accept kept reachable via an exported var set in init; expects exit 0. Checked by hand that the probe really lands in the darwin/arm64 binary (`go tool nm` shows `ipc.zzProbeRawSocket`).
- Step 9 header comment explains that sockets and locks are stdlib syscall only, net/os/exec/x/sys/windows stay forbidden, and the x/sys/unix exemption is unchanged.
- Finding 4: `/tmp/tca-out.$$` and `/tmp/tca-err.$$` are gone. Output goes to `$OUT_DIR/out` and `$OUT_DIR/err`, and OUT_DIR is registered in COPIES for the EXIT trap.
- `.github/workflows/ci.yml`: new `test-os` job with matrix [macos-latest, windows-latest] and fail-fast false. Checkout and setup-go use the same SHA pins as recipe-check. It runs `go test -count=1 ./...` and inherits GOTOOLCHAIN=local from the top-level env.

## Task Commits

1. Task 1: Step 9 self-tests, header comment, mktemp outputs - `470c7f1`
2. Task 2: CI matrix job for macOS and Windows tests - `c538a0b`

## Verification

- `bash -n` passes on both scripts. The real tree check passes. `grep /tmp/tca-` finds nothing.
- Per the executor constraints, the full self-test script was not run locally. The helpers plus the three new cases were extracted and run against the real tree, and all 3 passed (about 90 s). OUT_DIR was confirmed removed on exit.
- Ran a manual probe copy and checked its failure lines. The unix probe produced only darwin and linux `package net` and `package net/netip` lines, with no windows line.
- The workflow YAML parses (python yaml.safe_load). Jobs are: check, architecture, test-os, recipe-check, sarif-upload.
- `go test ./cmd/gruntled -run 'TestCIDoc|TestRelease'` passes. TestCIDoc only inspects recipe-check and sarif-upload, so it needed no change, and neither did docs/ci.md.
- `GOOS=windows go vet ./...` and `GOOS=darwin go vet ./...` both pass, so all tests compile on both OSes.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Self-test repo copies leaked in /tmp**
- **Found during:** Task 1 (finding 4)
- **Issue:** `copy=$(mkcopy)` runs mkcopy in a command substitution. Its `COPIES+=` never reached the parent shell, so every full repo copy stayed in /tmp after the run.
- **Fix:** mkcopy now creates copies with `mktemp -d "$OUT_DIR/copy.XXXXXX"`, so the single trap-cleaned OUT_DIR removes them.
- **Files modified:** scripts/test-check-architecture.sh
- **Commit:** 470c7f1

Plan verify ran `-race` and the full test-check-architecture.sh. Per the orchestrator constraints both were left to CI and replaced by the targeted manual checks above.

## Known Risks / Unverified

- macOS and Windows **runtime** is unverified until the first CI run of `test-os`, which is plan 11-07's CI confirmation. Only compilation was proven locally (GOOS vet).
- The repo has no `.gitattributes`, and GitHub windows runners check out with core.autocrlf=true. Some tests already normalise `\r\n` (e2e, sarif doc tests). Any golden or byte comparison against checked-out fixtures that doesn't normalise could fail on windows. If it does, a fix (`.gitattributes` `* text eol=lf`, or normalising in the test) belongs to 11-07.
- Symlink-based tests already `t.Skip` when os.Symlink fails, and unix permission assertions are already guarded by `runtime.GOOS != "windows"`. No new skips were added.

## Next Phase Readiness

- 11-07 pushes and confirms that CI is green, including test-os on both OSes.

## Self-Check: PASSED
