---
phase: 10
slug: watch-daemon-status-file
status: draft
nyquist_compliant: false
wave_0_complete: false
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
| TBD | TBD | TBD | DAEMON-01 | unit | `go test ./internal/infrastructure/watch -run 'TestIgnored\|TestDebouncer'` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | DAEMON-01 | integration | `go test ./internal/infrastructure/watch -run TestWatcherContract` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | DAEMON-01 | integration | `go test ./cmd/gruntled -run 'TestWatchParity\|TestWatchReindexesOnlyChanged\|TestWatchBackendSelection'` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | DAEMON-01 (phase 8 sec) | unit | `go test ./internal/infrastructure/terragrunt -run 'Invalidate\|Concurrent\|Incremental'` | partial | ⬜ pending |
| TBD | TBD | TBD | DAEMON-03 | unit golden | `go test ./internal/interfaces/presenter -run TestStatusLine` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | DAEMON-03 | unit | `go test ./internal/infrastructure/statusfile -run 'TestDir\|TestWrite'` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | DAEMON-03 | integration | `go test ./cmd/gruntled -run TestWatchStatusFile` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | proof | script | `bash scripts/check-architecture.sh` (6 targets, nm check, windows without fsnotify) | ✅ needs edit | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/infrastructure/watch/{ignore,debounce,contract,poll,native_unix}_test.go`
- [ ] `internal/infrastructure/statusfile/{path,write}_test.go`
- [ ] `internal/interfaces/presenter/status_test.go`
- [ ] `internal/application/watching/watching_test.go`
- [ ] `cmd/gruntled/watch_test.go`, `testdata/script/watch_usage.txtar`, `watch_exitcodes.txtar`
- [ ] `internal/infrastructure/terragrunt` tests: batch Invalidate, path contract, concurrent use
- [ ] `scripts/check-architecture.sh` + `scripts/test-check-architecture.sh` cases (nm check, x/sys/unix exemption, windows-no-fsnotify)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `test-check-architecture.sh` with the new proof cases | proof | ~18 min locally; runs in CI | Confirm the CI `architecture` job is green on master after the push |
| darwin kqueue and windows polling at runtime | DAEMON-01 | CI is ubuntu-only | Cross-vet locally; real runtime check deferred |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 150s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
