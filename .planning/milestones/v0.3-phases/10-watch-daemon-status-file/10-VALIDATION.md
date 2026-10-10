---
phase: 10
slug: watch-daemon-status-file
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-10-08
---

# Phase 10 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing`, `testing/fstest`, testscript v1.16.0, rapid v1.3.0 (existing) |
| **Config file** | none (CI `.github/workflows/ci.yml` runs `-race` and `test-check-architecture.sh`) |
| **Quick run command** | `go test -count=1 ./internal/infrastructure/watch/... ./internal/infrastructure/statusfile/... ./internal/infrastructure/terragrunt/... ./internal/application/watching/... ./internal/interfaces/presenter/... ./cmd/gruntled/ -run 'Watch\|Status\|Ignore\|Debounce\|Invalidate\|Contract\|Incremental'` |
| **Full suite command** | `go test -count=1 ./... && bash scripts/check-architecture.sh && GOOS=windows GOARCH=arm64 go vet ./... && GOOS=darwin GOARCH=arm64 go vet ./...` |
| **Estimated runtime** | ~120 seconds |

---

## Sampling Rate

- **After every task commit:** quick run command for touched packages
- **After every plan wave:** full suite command
- **Before `/gsd:verify-work`:** full suite green; CI green on master (it runs `-race` and `scripts/test-check-architecture.sh`, which must cover the new proof cases)
- **Max feedback latency:** 150 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 10-01-01 | 01 | 1 | proof (DAEMON-01) | script | `bash scripts/check-architecture.sh` (6 targets, per-target nm check, x/sys/unix exemption, windows without fsnotify) | ✅ | ✅ green |
| 10-01-02 | 01 | 1 | proof (DAEMON-01) | script self-test | `bash -n scripts/test-check-architecture.sh` locally; full run in CI only | ✅ | ✅ green (syntax; CI runs cases) |
| 10-02-01 | 02 | 1 | DAEMON-01 (phase 8 sec 1, 3, 4) | unit + property | `go test ./internal/infrastructure/terragrunt -run 'Invalidate\|Concurrent\|Incremental'` | ✅ | ✅ green |
| 10-02-02 | 02 | 1 | DAEMON-01 (phase 8 sec 2) | unit | `go test ./internal/application/watching -run TestIndex` | ✅ | ✅ green |
| 10-03-01 | 03 | 1 | DAEMON-03 | unit golden | `go test ./internal/interfaces/presenter -run TestStatusLine` | ✅ | ✅ green |
| 10-03-02 | 03 | 1 | DAEMON-03 | unit | `go test ./internal/infrastructure/statusfile -run 'TestDir\|TestWrite'` | ✅ | ✅ green |
| 10-04-01 | 04 | 1 | DAEMON-01 | unit | `go test ./internal/infrastructure/watch -run 'TestIgnored\|TestPending\|TestDebouncer'` | ✅ | ✅ green |
| 10-04-02 | 04 | 1 | DAEMON-01 | integration | `go test ./internal/infrastructure/watch -run 'TestWatcherContractPoll\|TestPoll'` | ✅ | ✅ green |
| 10-07-01 | 07 | 2 | DAEMON-01, DAEMON-03 | unit | `go test ./internal/infrastructure/watch -run TestRun` | ✅ | ✅ green |
| 10-05-01 | 05 | 2 | DAEMON-01 | integration + proof | `go test ./internal/infrastructure/watch -run 'TestWatcherContractNative\|TestNative' && bash scripts/check-architecture.sh` | ✅ | ✅ green |
| 10-06-01 | 06 | 3 | DAEMON-01, DAEMON-03 | integration + testscript | `go test ./cmd/gruntled -run 'TestWatchStatusFile\|TestWatchDebounceDefault\|TestWatchParity\|TestWatchReindexesOnlyChanged\|TestWatchBackendSelection\|TestWatchStdoutOnChange\|TestWatchQuietRepo\|TestWatchStatusInsideRepo\|TestScripts/watch_'` | ✅ | ✅ green |
| 10-06-02 | 06 | 3 | DAEMON-01, DAEMON-03 | docs + full suite | `go test ./cmd/gruntled -run TestHelpMatchesDocs` then the full suite command | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/infrastructure/watch/{ignore,debounce,contract,poll,native_unix}_test.go`
- [x] `internal/infrastructure/statusfile/{path,write}_test.go`
- [x] `internal/interfaces/presenter/status_test.go`
- [x] `internal/application/watching/watching_test.go`
- [x] `cmd/gruntled/watch_test.go`, `testdata/script/watch_usage.txtar`, `watch_exitcodes.txtar`
- [x] `internal/infrastructure/terragrunt` tests: batch Invalidate, path contract, concurrent use
- [x] `scripts/check-architecture.sh` + `scripts/test-check-architecture.sh` cases (nm check, x/sys/unix exemption, windows-no-fsnotify)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `test-check-architecture.sh` with the new proof cases | proof | ~18 min locally; runs in CI | Confirm the CI `architecture` job is green on master after the push |
| darwin kqueue and windows polling at runtime | DAEMON-01 | CI is ubuntu-only | Cross-vet locally; real runtime check deferred |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 150s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** complete 2026-10-08 (10-06); CI-only items (`-race`, `scripts/test-check-architecture.sh`) still to be confirmed on CI after the push
