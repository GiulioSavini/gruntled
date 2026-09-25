---
phase: 2
slug: parsing-graph-construction
status: draft
nyquist_compliant: true
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
| 02-01-T1 | 02-01 | 1 | GRAPH-03, GRAPH-04, PARSE-04 | unit (table) | `go test -count=1 ./internal/domain/...` | ✅ extend | ⬜ pending |
| 02-01-T2 | 02-01 | 1 | (identity, CLI-03 prep) | unit (table) | `go test -count=1 ./internal/domain/diagnostic` | ✅ extend | ⬜ pending |
| 02-02-T1 | 02-02 | 2 | GRAPH-04 | compile + import check | `go build ./... && go list -f '{{join .Imports " "}}' ./internal/application/ports` | ❌ W0 (creates) | ⬜ pending |
| 02-02-T2 | 02-02 | 2 | (architecture) | script self-test | `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` | ✅ extend | ⬜ pending |
| 02-02-T3 | 02-02 | 2 | GRAPH-04, PARSE-04 | unit (fakes) | `go test -count=1 ./internal/application/...` | ❌ W0 (creates) | ⬜ pending |
| 02-03-T1 | 02-03 | 3 | PARSE-05, GRAPH-01, GRAPH-03 | unit (table) | `go test -count=1 ./internal/infrastructure/hclconv ./internal/infrastructure/sourceresolve` | ❌ W0 (creates) | ⬜ pending |
| 02-03-T2 | 02-03 | 3 | PARSE-02 | unit (table) | `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestPathFuncs\|TestResolvePath\|TestLiteral\|TestEvalPath'` | ❌ W0 (creates) | ⬜ pending |
| 02-03-T3 | 02-03 | 3 | GRAPH-05, PARSE-05 | unit + real-FS | `go test -count=1 ./internal/infrastructure/tfsurface` | ❌ W0 (creates) | ⬜ pending |
| 02-04-T1 | 02-04 | 4 | PARSE-06 | unit (MapFS + real-FS symlinks) | `go test -count=1 ./internal/infrastructure/terragrunt -run TestWalk` | ❌ W0 (creates) | ⬜ pending |
| 02-04-T2 | 02-04 | 4 | PARSE-01 | unit (table) | `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestExtractRefs\|TestRefs'` | ❌ W0 (creates) | ⬜ pending |
| 02-04-T3 | 02-04 | 4 | PARSE-01, PARSE-05 | unit (table + countingFS) | `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestFileCache\|TestParse\|TestDependencyOptions'` | ❌ W0 (creates) | ⬜ pending |
| 02-05-T1 | 02-05 | 5 | PARSE-01..05, GRAPH-01..04 | unit (table, catalogue coverage) | `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestUnknownReasons\|TestLoader\|TestInclude\|TestSource\|TestDependency\|TestStructuralOnly'` | ❌ W0 (creates) | ⬜ pending |
| 02-05-T2 | 02-05 | 5 | GRAPH-01..05, PARSE-03, PARSE-05 | integration (MapFS, os.Root, synthrepo) | `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestTwoHop\|TestModuleUnknownBlastRadius\|TestSurfaceUnknownTarget\|TestWholeBodyRefs\|TestSyntaxEndToEnd\|TestParseOnce\|TestSynthrepoOracle\|TestDeterministic\|TestCorpusSmoke'` | ❌ W0 (creates) | ⬜ pending |
| 02-05-T3 | 02-05 | 5 | PARSE-05 | fuzz seeds | `go test -count=1 ./internal/infrastructure/terragrunt -run FuzzLoadUnits` | ❌ W0 (creates) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

No separate Wave 0 plan: every test file is created by the task that needs it (RED before GREEN), and the order of plans enforces the prerequisites.

- [ ] `go get github.com/hashicorp/hcl/v2@v2.25.0` with go.sum committed; self-test still green — 02-03-T1 (the only plan touching go.mod/go.sum)
- [ ] `internal/application/ports` DTOs + interfaces the tests compile against — 02-02-T1
- [ ] New arch rules + self-test cases (application allowlist, application external deps, hcl-only-in-infrastructure, application platform-neutral, application non-vacuous guard) before any infrastructure package — 02-02-T2 (wave 2; infrastructure starts in wave 3)
- [ ] Shared test helpers: MapFS from synthrepo.Tree (02-04-T1), counting FS (02-04-T3), canonical graph dump (02-02-T3 for fakes, 02-05-T2 for real adapters)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Corpus smoke: 65 units, 0 config-unknown, 22 refs | GRAPH-04/05 | needs a cloned public repo | `GRUNTLED_CORPUS=<clone> go test ./internal/infrastructure/terragrunt -run TestCorpusSmoke -count=1` (lives in infrastructure: the application layer may not import os) |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 30s (the 60 s local fuzz run in 02-05-T3 is a one-off, not part of the sampling loop)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
