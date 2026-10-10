---
phase: 13
slug: removed-output-diagnostic-shared-blast-core
status: validated
nyquist_compliant: true
wave_0_complete: true
created: 2026-10-10
---

# Phase 13 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing + pgregory.net/rapid v1.3.0 + testscript (rogpeppe/go-internal v1.16.0) |
| **Config file** | none |
| **Quick run command** | `go test -count=1 ./internal/domain/analysis/ ./internal/domain/diagnostic/ ./internal/application/blasting/ ./internal/interfaces/presenter/ && go test -count=1 -run 'TestScripts/blast\|Golden\|TestNoGRT004\|TestCheckCannotProduceGRT004\|TestRuleRegistryDoc\|TestSARIFDoc\|TestHelpMatchesDocs\|TestTextOutputsEscapeControls' ./cmd/gruntled/` |
| **Full suite command** | `go test -count=1 ./... && go vet ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` (CI adds `-race` on linux/macos/windows; no local gcc) |
| **Estimated runtime** | ~10 seconds (quick), ~70 seconds (full) |

PATH for every command: `export PATH=$HOME/.local/go/bin:$HOME/go/bin:$PATH`.

---

## Sampling Rate

- **After every task commit:** Run the quick run command
- **After every plan wave:** Run the full suite command
- **Before `/gsd:verify-work`:** Full suite green, 13-03 Task 3 gate evidence (v0.3.0 byte comparison) in 13-03-SUMMARY.md, CI green on the three OSes
- **Max feedback latency:** 15 seconds

---

## Success Criteria → Tests

| # | Phase success criterion (ROADMAP) | Proven by |
|---|-----------------------------------|-----------|
| 1 | Removed still-referenced output: one GRT004 per site, no GRT001 there, exit 1; message names output, module, target unit, repo-relative | `TestRemovedOutputsMatrix` ("removed" exact message, two sites, shared include); `blast_grt004.txtar` cases 1, 10; `blast_impacted.txtar` line 9; `TestBlastBrokenAndImpacted` (extended) |
| 2 | Only reclassifies: Broken units and per-unit counts equal v0.3.0, never both codes, unreferenced removal adds nothing | `TestBetweenOnlyReclassifiesGRT001` (rapid vs v0.3 `impact.Compute`, independent oracle, non-vacuity counters); `TestSupersedeUnknownOutputs` + `TestSupersedeNeverAddsProperty`; `blast_grt004.txtar` cases 2, 11 |
| 3 | Failed precondition behaves as v0.3.0 check: added reference, re-pointed to another module, module unknown either side, undeclared in baseline -> GRT001; enabled/skip/non-literal silent; mock never suppresses | `TestRemovedOutputsMatrix` rows; `TestRemovedOutputsChecksAreNecessary` (each of fires, baseHadRef, sameModule, baseDeclared killed by a named row); `TestRemovedOutputsDIAG03Parity` (all 22 DIAG-03 rows); `TestDIAG03` unchanged; `blast_grt004.txtar` cases 3-9 |
| 4 | check (text/json/sarif), plain report, graph --json, status file never print GRT004; byte-identical to v0.3.0 | `TestNoGRT004OutsideBlast` (non-vacuous blast run first), `TestCheckCannotProduceGRT004`; `git diff --exit-code v0.3.0 --` goldens; `bash scripts/compare-ref.sh v0.3.0` (N/N identical over golden/*, text/json/sarif/graph goldens, diag03_*, clean-fixture, sarif-fixture, blast_grt004 trees); `TestSARIFDoc` (rule table unchanged) |

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 13-01-01 | 01 | 1 | MORE-03 | unit + goldens (refactor pin) | `go test -count=1 ./internal/domain/diagnostic/ ./internal/domain/analysis/ && go test -count=1 -run 'Golden\|TestScripts\|Corpus\|Denis\|Sarif\|Graph' ./cmd/gruntled/ && git diff --exit-code v0.3.0 -- cmd/gruntled/testdata` | ✅ grt001_test.go (rows hoisted), diagnostic_test.go | ✅ green |
| 13-01-02 | 01 | 1 | MORE-03 | unit matrix + parity + necessity (mutation) | `go test -count=1 -run 'TestRemovedOutputs\|TestDIAG03' -v ./internal/domain/analysis/` | ✅ grt004.go, grt004_test.go, grt004_internal_test.go (created) | ✅ green |
| 13-01-03 | 01 | 1 | MORE-03 | unit + property (rapid) | `go test -count=1 -run 'TestSupersede' -v ./internal/domain/analysis/` | ✅ grt004_test.go extended (created) | ✅ green |
| 13-02-01 | 02 | 2 | MORE-03 | unit + property (rapid, oracle) | `go test -count=1 -v -run 'TestBetween\|TestBlast\|TestError' ./internal/application/blasting/` | ✅ between_property_test.go (created) | ✅ green |
| 13-02-02 | 02 | 2 | MORE-03 | testscript e2e + presenter unit | `go test -count=1 -run 'TestScripts/blast' ./cmd/gruntled/ && go test -count=1 -run 'Blast' ./internal/interfaces/presenter/` | ✅ blast_grt004.txtar (created) | ✅ green |
| 13-02-03 | 02 | 2 | MORE-03 | e2e escape | `go test -count=1 -run 'TestTextOutputsEscapeControls' ./cmd/gruntled/` | ✅ escape_test.go (extended) | ✅ green |
| 13-03-01 | 03 | 3 | MORE-07 | e2e (check, report daemon, graph, status) | `go test -count=1 -run 'TestNoGRT004OutsideBlast\|TestCheckCannotProduceGRT004' ./cmd/gruntled/` | ✅ more07_test.go (created) | ✅ green |
| 13-03-02 | 03 | 3 | MORE-03, MORE-07 (docs) | doc guard | `go test -count=1 -run 'TestRuleRegistryDoc\|TestSARIFDoc\|TestHelpMatchesDocs\|TestReadme\|TestCIDoc\|TestValidationDoc\|TestScripts/usage' ./cmd/gruntled/` | ✅ rules_doc_test.go (created) | ✅ green |
| 13-03-03 | 03 | 3 | MORE-07 | gate + byte compare | full suite + `git diff --exit-code v0.3.0 -- <goldens>` + `bash scripts/compare-ref.sh v0.3.0` + `bash scripts/build-release.sh v0.0.0-ci <sha> $(mktemp -d)` + `git diff --exit-code go.mod go.sum` | ✅ scripts/compare-ref.sh (created) | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure (Go testing, rapid, testscript, txtar, runCLI/startWatch/runReportAt helpers)
covers the phase; every ❌ W0 file above is created by the first (TDD, red-first) step of its own
task, so no separate Wave 0 plan is needed.

---

## Mutation Proofs (Retrospective lesson 1: non-vacuity)

| Mutation | Must fail | Where recorded |
|----------|-----------|----------------|
| Disable each of `fires`, `baseHadRef`, `sameModule`, `baseDeclared` | `TestRemovedOutputsChecksAreNecessary` (automated, in CI) | 13-01-SUMMARY |
| Flip the sameModule comparison; drop the mock-suffix condition | `TestRemovedOutputsMatrix` | 13-01-SUMMARY (by hand) |
| Supersede by Unit only / by message / by (Unit, line, col) without file | `TestSupersedeUnknownOutputs` rows | 13-01-SUMMARY |
| compare-ref.sh on a perturbed fixture | script exits 1 | 13-03-SUMMARY (self-test) |
| Remove the `SupersedeUnknownOutputs` call in `Between`; append GRT004 without consuming | `TestBetweenOnlyReclassifiesGRT001` | 13-02-SUMMARY (by hand, shrunk counterexample pasted) |
| Any GRT001 row 1-5 change during extraction | `TestDIAG03` + goldens + `git diff v0.3.0` | 13-01-SUMMARY |

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `-race` on the new blasting/cmd tests | MORE-03, MORE-07 | No local gcc | Confirm CI `check` job `go test -race -count=1 ./...` green after the push |
| Daemon `report` subtest on windows | MORE-07 | No socket on windows; subtest skips there | Confirm `TestNoGRT004OutsideBlast` ran its daemon part on linux and macos in CI `test-os` |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 15s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-10-10 by gsd-verifier (every per-task command re-run green at 307e83d, code identical to 8a6dd48; CI run 38064624899 at 8a6dd48 green on linux -race, macos and windows). See 13-VERIFICATION.md.
