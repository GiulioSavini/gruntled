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
| 02-06-T1 | 02-06 | GC-1 | PARSE-01, PARSE-04 (G1) | unit (table) | `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestExtractRefs'` | ✅ extend | ⬜ pending |
| 02-06-T2 | 02-06 | GC-1 | PARSE-01 (G1) | integration + catalogue | `go test -count=1 ./internal/infrastructure/terragrunt -run TestWholeBodyRefs && grep -q "Reversed by gap G1" .planning/phases/02-parsing-graph-construction/02-TERRAGRUNT-EDGECASES.md` | ✅ extend | ⬜ pending |
| 02-07-T1 | 02-07 | GC-1 | GRAPH-04, PARSE-04 (G2, G4, G8) | unit (table) | `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestUnknownReasons\|TestDependency'` | ✅ extend | ⬜ pending |
| 02-07-T2 | 02-07 | GC-1 | PARSE-03, PARSE-04 (G3, G5, G11) | unit + integration (exact corpus reproduction) | `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestIncludeTarget\|TestUnknownReasons\|TestInclude'` | ❌ W0 (creates include_target_test.go) | ⬜ pending |
| 02-07-T3 | 02-07 | GC-1 | GRAPH-01, PARSE-04 (G6, G9) | unit (table) | `go test -count=1 ./internal/infrastructure/terragrunt -run TestUnknownReasons` | ✅ extend | ⬜ pending |
| 02-08-T1 | 02-08 | GC-1 | GRAPH-04, PARSE-04 (G12) | unit (table) | `go test -count=1 ./internal/domain/repograph -run 'TestTristateIsValid\|TestPositionIsZero\|TestNewDependency\|TestNewUnresolvedDependency\|TestUnitConstructors' && go test -count=1 ./...` | ✅ extend | ⬜ pending |
| 02-08-T2 | 02-08 | GC-1 | GRAPH-04 (G12) | unit (table) | `go test -count=1 ./internal/domain/repograph -run 'TestUnitStatusIsValid\|TestNewRepositoryGraph'` | ✅ extend | ⬜ pending |
| 02-09-T1 | 02-09 | GC-1 | ARCH-01 (G13) | script self-test | `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` | ✅ extend | ⬜ pending |
| 02-09-T2 | 02-09 | GC-1 | ARCH-01 (G14) | script self-test | `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` | ✅ extend | ⬜ pending |
| 02-10-T1 | 02-10 | GC-1 | PARSE-05 (G7a) | unit (table, generated inputs) | `go test -count=1 ./internal/infrastructure/hclconv -run 'TestReadFileLimited\|TestCheckNativeDepth\|TestCheckJSONDepth'` | ❌ W0 (creates limits_test.go) | ⬜ pending |
| 02-10-T2 | 02-10 | GC-1 | GRAPH-05, PARSE-05 (G7a) | unit (crash regression) | `go test -count=1 ./internal/infrastructure/tfsurface -run 'TestReadSurfaceDeep\|TestReadSurfaceOversize\|TestReadSurfaceAtDepthLimit\|TestReadSurfaceLimitPrecedence'` | ✅ extend | ⬜ pending |
| 02-11-T1 | 02-11 | GC-2 | PARSE-03, PARSE-05 (G7b) | unit + integration (crash regression) | `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestDeep\|TestFileCacheLimits\|TestUnknownReasons\|TestLoaderParseOnce\|TestParseOnce'` | ❌ W0 (creates limits_test.go) | ⬜ pending |
| 02-11-T2 | 02-11 | GC-2 | PARSE-05 (G10) | fuzz seeds (+ 60 s local fuzz) | `go test -count=1 ./internal/infrastructure/terragrunt -run FuzzLoadUnits` | ✅ extend | ⬜ pending |
| 02-11-T3 | 02-11 | GC-2 | PARSE-04 (catalogue) | doc grep | the reason-grep loop in 02-11-PLAN Task 3 `<verify>` | ✅ extend | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

*Gap closure (02-REVIEW.md G1..G14): wave GC-1 = 02-06, 02-07, 02-08, 02-09, 02-10 in parallel with disjoint files; wave GC-2 = 02-11 (after 02-06, 02-07, 02-10). G7 is split: G7a (module files, 02-10) and G7b (unit/include files, 02-11). Every other gap maps to exactly one task above.*

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
| Corpus smoke: 65 units, 3 config-unknown, 22 refs (corrected in 02-05) | GRAPH-04/05 | needs a cloned public repo | `GRUNTLED_CORPUS=<clone> go test ./internal/infrastructure/terragrunt -run TestCorpusSmoke -count=1` (lives in infrastructure: the application layer may not import os) |
| Secondary corpus (cds-snc/secret @ 341e8a95): parent config include-target, 0 missing outputs | PARSE-03, GRAPH-04 (G3) | needs a cloned public repo | `GRUNTLED_CORPUS_SECRET=<clone> go test ./internal/infrastructure/terragrunt -run TestIncludeTargetSecretCorpus -count=1` |
| Loader fuzz session, two inputs, 60 s, no crasher | PARSE-05 (G10) | too long for the sampling loop | `go test -run '^$' -fuzz '^FuzzLoadUnits$' -fuzztime 60s ./internal/infrastructure/terragrunt` |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 30s (the 60 s local fuzz run in 02-05-T3 is a one-off, not part of the sampling loop)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
