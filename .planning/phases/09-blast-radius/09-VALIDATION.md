---
phase: 9
slug: blast-radius
status: complete
nyquist_compliant: true
wave_0_complete: true
created: 2026-10-08
---

# Phase 9 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + testscript (txtar) + pgregory.net/rapid v1.3.0 (test-only) |
| **Config file** | none |
| **Quick run command** | `go test ./internal/domain/impact/ ./internal/application/blasting/ ./internal/interfaces/presenter/ -count=1 && go test ./cmd/gruntled -run 'TestScripts/blast' -count=1` |
| **Full suite command** | `go vet ./... && go test -count=1 ./... && scripts/check-architecture.sh` (no `-race` locally) |
| **Estimated runtime** | ~90 seconds |

---

## Sampling Rate

- **After every task commit:** quick run command
- **After every plan wave:** full suite command
- **Before `/gsd:verify-work`:** full suite green, `gofmt -l .` empty, `go mod tidy -diff` clean
- **Max feedback latency:** 120 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 9-01-T1 | 01 | 1 | BLAST-01 | unit (GRT100 stability) | `go test ./internal/infrastructure/hclconv/ -run Stability -count=1` | ✅ | ✅ green |
| 9-01-T2 | 01 | 1 | BLAST-01, BLAST-02 | unit+rapid | `go test ./internal/domain/impact/ -count=1` | ✅ | ✅ green |
| 9-02-T1 | 02 | 2 | BLAST-01 | unit | `go test ./internal/application/blasting/ -count=1` | ✅ | ✅ green |
| 9-02-T2 | 02 | 2 | BLAST-01 | presenter golden | `go test ./internal/interfaces/presenter/ -run Blast -count=1` | ✅ | ✅ green |
| 9-03-T1 | 03 | 3 | BLAST-01, BLAST-02 | script | `go test ./cmd/gruntled -run 'TestScripts/(blast|usage)' -count=1` | ✅ | ✅ green |
| 9-03-T2 | 03 | 3 | BLAST-01, BLAST-02 | full gate + doc tests | `go vet ./... && go test -count=1 ./... && scripts/check-architecture.sh` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [x] `internal/domain/impact/*_test.go` — table + rapid: disjoint, sorted, line-shift invariance, identical trees ⇒ empty
- [x] `internal/application/blasting/blasting_test.go` — fake ports
- [x] `internal/interfaces/presenter/blast_test.go`
- [x] `cmd/gruntled/testdata/script/blast_*.txtar` — nobase, exitcodes, impacted, shifted-line

---

## Manual-Only Verifications

*All phase behaviors have automated verification.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 120s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** approved 2026-10-08 (phase gate green: vet, full test suite, check-architecture, gofmt, go mod tidy -diff)
