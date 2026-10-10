---
phase: 11
slug: report-single-instance-release-proof
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-10-08
---

# Phase 11 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing + testscript v1.16.0 |
| **Config file** | none |
| **Quick run command** | `go test -race -count=1 ./internal/infrastructure/ipc/... ./internal/infrastructure/statusfile/... ./internal/interfaces/presenter/... && go test -race -count=1 -run 'Watch\|Report\|Instance\|Stale\|TestScripts\|TestHelpMatchesDocs' ./cmd/gruntled/` |
| **Full suite command** | `go test -race -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` |
| **Estimated runtime** | ~11 seconds (quick), ~60 seconds (full) |

---

## Sampling Rate

- **After every task commit:** Run the quick run command
- **After every plan wave:** Run the full suite command
- **Before `/gsd:verify-work`:** Full suite must be green
- **Max feedback latency:** 11 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 11-01-01 | 01 | 1 | DAEMON-05 | unit | `go test -count=1 ./internal/infrastructure/statusfile/ -run 'Ensure\|CheckDir'` | ✅ | ✅ green |
| 11-01-02 | 01 | 1 | DAEMON-05 (folded P10 finding) | unit | `go test -count=1 ./internal/interfaces/presenter/ -run 'Sanitize\|StatusFailed'` | ✅ | ✅ green |
| 11-02-01 | 02 | 2 | DAEMON-05, DAEMON-04 | unit | `go test -count=1 ./internal/infrastructure/ipc/ -run 'Lock\|Probe\|Dump'` | ✅ | ✅ green |
| 11-02-02 | 02 | 2 | DAEMON-04, DAEMON-05 | unit + crash helper | `go test -count=1 ./internal/infrastructure/ipc/` | ✅ | ✅ green |
| 11-03-01 | 03 | 3 | DAEMON-04 | unit | `go test -count=1 ./cmd/gruntled/ -run 'Render\|Snapshot'` | ✅ | ✅ green |
| 11-03-02 | 03 | 3 | DAEMON-05 | integration | `go test -count=1 ./cmd/gruntled/ -run 'Instance\|Watch'` | ✅ | ✅ green |
| 11-04-01 | 04 | 4 | DAEMON-04 | CLI + docs | `go test -count=1 ./cmd/gruntled/ -run 'TestHelpMatchesDocs\|TestScripts'` | ✅ | ✅ green |
| 11-04-02 | 04 | 4 | DAEMON-04 | integration | `go test -count=1 ./cmd/gruntled/ -run 'Report'` | ✅ | ✅ green |
| 11-05-01 | 05 | 4 | DAEMON-06 | script self-test | `bash scripts/test-check-architecture.sh` | ✅ | ✅ green (CI) |
| 11-05-02 | 05 | 4 | DAEMON-06 | CI matrix | CI `test-os` job (macos-latest, windows-latest) | ✅ | ⬜ macos green, windows pending 11-07 |
| 11-06-01 | 06 | 5 | DAEMON-06 (doc) | doc guard | `go test -count=1 ./cmd/gruntled/ -run 'TestReadme\|TestHelpMatchesDocs'` | ✅ | ✅ green |
| 11-06-02 | 06 | 5 | all | gate | full suite + `bash scripts/check-architecture.sh` + `scripts/build-release.sh` (six targets) | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers all phase requirements (tests are created inside the same tasks, TDD).

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| darwin and windows runtime of watch/report/single instance | DAEMON-04, DAEMON-05, DAEMON-06 | No darwin/windows host locally; covered by the CI `test-os` matrix | Confirm CI `test-os` green on macos-latest and windows-latest (plan 11-07). Run 37780137349: macos green, windows red on pre-existing never-run-on-windows tests (CRLF, .exe, bash, symlink, a watch test hang), fixed in 11-07 |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 11s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-10-08 (windows CI confirmation pending 11-07)
