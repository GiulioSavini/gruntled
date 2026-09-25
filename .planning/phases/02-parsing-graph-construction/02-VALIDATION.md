---
phase: 2
slug: parsing-graph-construction
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-09-25
---

# Phase 2 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | go test (stdlib `testing`, `testing/fstest.MapFS`, native fuzzing seeds) |
| **Config file** | none — `go.mod` (`go 1.27`) |
| **Quick run command** | `go build ./... && go vet ./... && go test -count=1 ./internal/...` |
| **Full suite command** | quick run + `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` + `test -z "$("$(go env GOROOT)/bin/gofmt" -l .)"` + `go mod tidy -diff` + `GOTOOLCHAIN=go1.27.0 go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./...` (`-race` needs gcc, CI only) |
| **Estimated runtime** | ~20 seconds (staticcheck cached) |

---

## Sampling Rate

- **After every task commit:** Run the quick run command
- **After every plan wave:** Run the full suite command
- **Before `/gsd:verify-work`:** Full suite must be green locally and CI green on master (`gh run watch --exit-status`)
- **Max feedback latency:** 30 seconds

---

## Per-Task Verification Map

Filled by the planner from 02-RESEARCH.md "Phase Requirements → Test Map" (every PARSE-0x /
GRAPH-0x row maps to at least one automated `go test -run` command in a plan task).

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| (see PLAN.md `<verify>` blocks) | | | | | | | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `go get github.com/hashicorp/hcl/v2@v2.25.0` with go.sum committed; self-test still green
- [ ] `internal/application/ports` DTOs + interfaces the tests compile against
- [ ] New arch rules + self-test cases (application allowlist, application external deps, hcl-only-in-infrastructure, application platform-neutral) before any infrastructure package
- [ ] Shared test helpers: MapFS from synthrepo.Tree, counting FS, canonical graph dump

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Corpus smoke: 65 units, 0 config-unknown, 22 refs | GRAPH-04/05 | needs a cloned public repo | `GRUNTLED_CORPUS=<clone> go test ./internal/application/indexing -run TestCorpusSmoke -count=1` |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 30s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
