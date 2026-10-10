---
phase: 1
slug: domain-foundation-test-substrate
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-09-25
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib `testing`) |
| **Config file** | none — `go.mod` (`go 1.27`) is created in Wave 0 |
| **Quick run command** | `go build ./... && go vet ./... && go test ./...` |
| **Full suite command** | quick run + `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` + `test -z "$("$(go env GOROOT)/bin/gofmt" -l .)"` (use the go1.27 toolchain gofmt, not the local 1.24 binary) |
| **Estimated runtime** | ~5 seconds |

---

## Sampling Rate

- **After every task commit:** Run `go build ./... && go vet ./... && go test ./...`
- **After every plan wave:** Run the full suite command
- **Before `/gsd:verify-work`:** Full suite must be green, and `.github/workflows/ci.yml` green on GitHub
- **Max feedback latency:** 10 seconds

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 01-01-T1 | 01-01 | 1 | ARCH-02 | unit | `go test ./internal/domain/repograph/... -count=1` (value objects, hand-built) | ❌ W0 | ⬜ pending |
| 01-01-T2 | 01-01 | 1 | ARCH-02 | unit | `go test ./internal/domain/repograph/... -count=1 -run 'Graph\|Reference\|Module\|Dependency' -v` (aggregate + pure-query GRT001-shaped proof) | ❌ W0 | ⬜ pending |
| 01-01-T3 | 01-01 | 1 | ARCH-02 | unit | `go test ./internal/domain/... -count=1` (Diagnostic, Set, Diff) | ❌ W0 | ⬜ pending |
| 01-02-T1 | 01-02 | 2 | ARCH-01 | static | `bash scripts/check-architecture.sh` — compile gate; direct imports incl. **XTestImports** (no `os`/`io/fs`/`io/ioutil`/`path/filepath`/`net`/`syscall`); transitive non-stdlib deps allowlisted to `internal/domain/` (rejects hcl/terraform-config-inspect/go-getter/fsnotify); `cmd/gruntled` never links `internal/testsupport`; ≥2 domain packages | ❌ W0 | ⬜ pending |
| 01-02-T1 | 01-02 | 2 | ARCH-01 | self-test | `bash scripts/test-check-architecture.sh` — 7 cases on throwaway copies, each violation must make the check fail | ❌ W0 | ⬜ pending |
| 01-02-T2 | 01-02 | 2 | ARCH-01 | CI | `gh run list --branch master --commit "$(git rev-parse HEAD)" --limit 1 --json conclusion` → `success` | ❌ W0 | ⬜ pending |
| 01-03-T1 | 01-03 | 3 | VALID-01 | unit | `go test ./internal/testsupport/synthrepo/... -count=3 -run 'Render\|RelPath\|LineCol\|Validate'` (incl. pinned digest) | ❌ W0 | ⬜ pending |
| 01-03-T2 | 01-03 | 3 | VALID-01 | integration | `go test ./internal/testsupport/synthrepo/... -run TestGenerate_Deterministic -v` | ❌ W0 | ⬜ pending |
| 01-03-T2 | 01-03 | 3 | VALID-01 | integration | `go test ./internal/testsupport/synthrepo/... -run 'TestGenerate_InjectsBadOutputRef\|TestGenerate_ManifestIsExactOracle' -v` | ❌ W0 | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky. Task IDs are filled in by the planner.*

---

## Wave 0 Requirements

- [ ] `go.mod` — module `github.com/GiulioSavini/gruntled`, `go 1.27`
- [ ] `cmd/gruntled/main.go` — composition-root stub
- [ ] `internal/domain/repograph/*_test.go` — ARCH-02
- [ ] `internal/domain/diagnostic/*_test.go` — ARCH-02
- [ ] `internal/testsupport/synthrepo/*_test.go` — VALID-01
- [ ] `.github/workflows/ci.yml` — build/vet/test plus both ARCH-01 checks; each job must be able to fail

---

## Manual-Only Verifications

None. The former manual check ("the ARCH-01 job actually fails on a violation") is automated by `scripts/test-check-architecture.sh` (plan 01-02), which runs in CI.

**Planner corrections to RESEARCH.md §Pattern 1 (verified on go1.27.0):**
- external test packages list imports in `.XTestImports`; `.TestImports` alone misses them;
- `go list` exits 0 on Go syntax errors, so `found=$(go list … | grep … || true)` passes on broken code; the script starts with a `go vet` compile gate;
- CI triggers on `master`, not `main`.

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [ ] Feedback latency < 10s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
