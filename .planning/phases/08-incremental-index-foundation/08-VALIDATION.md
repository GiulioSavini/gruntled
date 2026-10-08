---
phase: 8
slug: incremental-index-foundation
status: approved
nyquist_compliant: true
wave_0_complete: true
created: 2026-10-08
---

# Phase 8 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + pgregory.net/rapid v1.3.0 (test-only) |
| **Config file** | none |
| **Quick run command** | `go test ./internal/infrastructure/terragrunt/ -run 'TestIncremental\|TestPersistentCache\|TestLoader' -count=1` |
| **Full suite command** | `go vet ./... && go test -count=1 ./...` (CI adds `-race`) |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** quick run command
- **After every plan wave:** full suite command
- **Before `/gsd:verify-work`:** full suite green, `gofmt -l .` empty, `go mod tidy -diff` clean, `go list -deps ./cmd/gruntled | grep -c rapid` = 0
- **Max feedback latency:** 90 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 08-01-T1 | 08-01 | 1 | DAEMON-02 (SC4) | unit | `go test ./internal/infrastructure/terragrunt/ -run TestPersistentCache -count=1` | ✅ | ✅ green |
| 08-01-T2 | 08-01 | 1 | DAEMON-02 (SC3) | golden/e2e | `go test ./cmd/gruntled/ -count=1` | ✅ | ✅ green |
| 08-02-T1 | 08-02 | 2 | proof | shell | `go list -deps ./cmd/gruntled \| grep -c rapid` → 0; `go mod tidy -diff` clean | n/a | ✅ green |
| 08-02-T2 | 08-02 | 2 | DAEMON-02 (SC1) | rapid stateful | `go test ./internal/infrastructure/terragrunt/ -run TestIncremental -count=1` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `go get pgregory.net/rapid@v1.3.0` + `go mod tidy`
- [x] `internal/infrastructure/terragrunt/incremental_test.go` — model + render helper + non-vacuity guard
- [x] `internal/infrastructure/terragrunt/cache_test.go` — stats, no stale GRT100, negative-entry and dir-prefix invalidation

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Shrinking yields a minimal sequence | DAEMON-02 (SC2) | Needs a deliberately broken Invalidate | Temporarily skip eviction, run the rapid test, confirm a short failing sequence, revert. Done in 08-02: no-op Invalidate shrinks to 3 actions, no prefix eviction to 3 actions (see 08-02-SUMMARY) |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-10-08
