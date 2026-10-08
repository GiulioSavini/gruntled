---
phase: 10-watch-daemon-status-file
plan: 01
subsystem: build, security-proof
tags: [check-architecture, no-net-no-exec, go-tool-nm, fsnotify-prep]
requires:
  - scripts/check-architecture.sh Step 9 (binary-no-net-no-exec)
provides:
  - "Step 9 half 3: per-target CGO_ENABLED=0 go build + go tool nm linker symbol check"
  - "Step 9 half 2: textual spawner scan exempts exactly import path golang.org/x/sys/unix"
  - "Step 9 half 4: windows targets reject github.com/fsnotify/fsnotify and golang.org/x/sys/windows"
  - "run_case_msg self-test helper (rule must fail + +ERE/-ERE output assertions)"
affects: [10-05 (go get fsnotify), 10-06 (final suite)]
tech-stack:
  added: []
  patterns: [linker-level proof over built binary per release target, message-asserting self-test cases]
key-files:
  created: []
  modified:
    - scripts/check-architecture.sh
    - scripts/test-check-architecture.sh
decisions:
  - x/sys spawner names enumerated from pinned v0.46.0 (unix.Exec, windows.CreateProcess/CreateProcessAsUser/ShellExecute, plus KexecFileLoad); Execveat/ForkExec/StartProcess/forkExec* listed by name for future bumps; re-run grep recorded in script comment
  - nm symbol regex anchored as `($|\.)` so closures/deferwrap of a flagged function also match
  - darwin `_execve` dynamic import (present in every darwin Go binary, libc stub) is not flagged; only Go-level spawner symbols are
  - Self-test case 2 stub dir created with `mktemp -d -t zzprobe.XXXXXX` so the grep-half "file" line is identifiable; case asserts both the file line (grep half) and linked syscall.Exec (nm half)
  - Residual gaps documented in script: raw Syscall(SYS_EXECVE), run-time DLL proc lookup, cgo (off in releases), assembly
requirements-completed: []
requirements-advanced: [DAEMON-01]
metrics:
  duration: 9min
  completed: 2026-10-08
  tasks: 2
  files: 2
---

# Phase 10 Plan 01: Binary proof linker check Summary

Step 9 now proves no-exec on the real built binary per release target via `go tool nm`, exempts only `golang.org/x/sys/unix` from the textual spawner grep, and rejects fsnotify / x/sys/windows on windows, so the later `go get fsnotify` (10-05) cannot turn CI red.

DAEMON-01 is only advanced here (proof prerequisite); it is not complete. No requirement is marked complete by this plan.

## Tasks

| Task | Name | Commit | Files |
| ---- | ---- | ------ | ----- |
| 1 | Step 9: x/sys/unix scan exemption, per-target nm check, windows-no-fsnotify | bf574d7 | scripts/check-architecture.sh |
| 2 | Self-test cases pinning new proof behaviour | ea7254b | scripts/test-check-architecture.sh |

## Verification

- Real tree: `bash scripts/check-architecture.sh` -> `architecture: OK` (~26s). Measured flagged symbols on all 6 targets before editing: none.
- Scratch aliased probe (`o.StartProcess` from init): only `binary-no-net-no-exec` fails, 6 `linked symbol os.StartProcess` lines, zero `file` lines.
- New cases hand-run by extracting the script preamble (helpers) + the three new case blocks verbatim into a scratch harness (self-test script never executed): all 3 PASS (~79s).
- Mutation check (scratch repo copies): disabling the grep half fails `binary-xsys-exemption-narrow`; disabling the nm half fails `binary-aliased-start-process`; disabling the windows half fails `binary-windows-fsnotify`. Each assertion is load-bearing.
- `bash -n scripts/test-check-architecture.sh` OK.

## Deviations from Plan

- Added `run_case_msg` as a sibling helper (plan allowed either extending run_case or a sibling); it takes `+ERE`/`-ERE` assertions so case 1 can also assert absence of a `file` line.
- Enumeration regex widened beyond the plan's literal grep (also Spawn/CreateProcess/Fork/Clone/ShellExecute/WinExec) so windows spawners not spelled "exec" are found; adds windows.CreateProcess, CreateProcessAsUser, ShellExecute to sym_re. Strengthens, never weakens.
- Task 2 case 3 note: after 10-05, the unconditional fsnotify stub replace will break the unix build in that copy (stub lacks the real API); the case still passes because it asserts only the windows-half message, which comes from `go list -e -deps`. Documented in the case comment.

## Self-Check: PASSED
