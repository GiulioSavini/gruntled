---
phase: 1
slug: domain-foundation-test-substrate
status: draft
nyquist_compliant: false
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
| **Full suite command** | quick run + both ARCH-01 `go list` checks (direct imports: no `os`/`io/fs`/`path/filepath`; transitive deps: no hcl/terraform-config-inspect/go-getter/fsnotify) + `test -z "$(gofmt -l .)"` |
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
| 1-xx | tbd | tbd | ARCH-01 | static/CI | `go list -f '{{range .Imports}}{{.}}{{"\n"}}{{end}}{{range .TestImports}}{{.}}{{"\n"}}{{end}}' ./internal/domain/... \| grep -E '^(os\|io/fs\|path/filepath)$'` → empty | ❌ W0 | ⬜ pending |
| 1-xx | tbd | tbd | ARCH-01 | static/CI | `go list -deps -test ./internal/domain/... \| grep -E 'hashicorp/hcl\|terraform-config-inspect\|hashicorp/go-getter\|fsnotify'` → empty | ❌ W0 | ⬜ pending |
| 1-xx | tbd | tbd | ARCH-02 | unit | `go test ./internal/domain/... -v` | ❌ W0 | ⬜ pending |
| 1-xx | tbd | tbd | VALID-01 | unit/integration | `go test ./internal/testsupport/synthrepo/... -run TestGenerate_Deterministic -v` | ❌ W0 | ⬜ pending |
| 1-xx | tbd | tbd | VALID-01 | unit/integration | `go test ./internal/testsupport/synthrepo/... -run TestGenerate_InjectsBadOutputRef -v` | ❌ W0 | ⬜ pending |

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

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| The ARCH-01 CI job actually fails on a violation | ARCH-01 | Proving a check fails needs a deliberate violation that must not be committed | Temporarily add `import _ "os"` to a domain file, run the check locally, confirm it exits non-zero, then revert |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 10s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
