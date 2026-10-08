---
phase: 8
slug: incremental-index-foundation
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-10-08
---

# Phase 8 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + pgregory.net/rapid v1.3.0 (test-only, to add) |
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
| TBD | TBD | TBD | DAEMON-02 | rapid stateful | `go test ./internal/infrastructure/terragrunt/ -run TestIncrementalEqualsFull -count=1` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | DAEMON-02 (SC4) | unit | `go test ./internal/infrastructure/terragrunt/ -run TestPersistentCache -count=1` | ❌ W0 | ⬜ pending |
| TBD | TBD | TBD | DAEMON-02 (SC3) | golden/e2e | `go test ./cmd/gruntled/ -count=1` | ✅ | ⬜ pending |
| TBD | TBD | TBD | proof | shell | `go list -deps ./cmd/gruntled \| grep -c rapid` → 0 | n/a | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `go get pgregory.net/rapid@v1.3.0` + `go mod tidy`
- [ ] `internal/infrastructure/terragrunt/incremental_test.go` — model + render helper + non-vacuity guard
- [ ] `internal/infrastructure/terragrunt/cache_test.go` — stats, no stale GRT100, negative-entry and dir-prefix invalidation

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Shrinking yields a minimal sequence | DAEMON-02 (SC2) | Needs a deliberately broken Invalidate | Temporarily skip eviction, run the rapid test, confirm a short failing sequence, revert |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 90s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
