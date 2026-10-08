---
phase: 11-report-single-instance-release-proof
verified: 2026-10-08T00:00:00Z
status: passed
score: 6/6 must-haves verified
---

# Phase 11 Verification

Goal: query a running daemon, never two daemons per repo, release binaries still cannot touch net/exec.

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | report text/json/sarif matches check (unix socket linux/darwin; status file windows) | VERIFIED | Smoke: `report` output identical to `check`; `--format json` OK; ipc + statusfile tests pass; CI test-os macos/windows green |
| 2 | no daemon: non-zero, clear message, starts nothing | VERIFIED | exit 3, "no watch daemon is running for ..."; no process spawned |
| 3 | second watch prints location, exit 0 | VERIFIED | "already watching ... (pid, socket, status)" rc=0 |
| 4 | crash recovery (stale socket + lock) | VERIFIED | kill -9 then `watch` restarts, report works |
| 5 | no-net/no-exec proof on six targets with watcher+socket linked | VERIFIED | check-architecture.sh OK (step 9 loops GOOS/GOARCH targets; CI architecture job green) |
| 6 | README documents watch, report, blast, status file path, Windows limitation | VERIFIED | README.md lines 145-217 |

Requirements: DAEMON-04, DAEMON-05, DAEMON-06 all declared in plans, all in REQUIREMENTS.md (Complete); no orphans.

Tests: `go test -count=1 ./...` all ok. `check-architecture.sh` OK. CI run 37784893384: check, architecture, recipe-check, sarif-upload, test-os macos/windows all success.

Human verification: none required (Windows report path covered by CI test-os windows only, not hand-run).
Gaps: none.
