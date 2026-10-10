---
phase: 14
slug: transitive-impact-with-paths
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-10-10
---

# Phase 14 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (domain/application: stdlib + in-test xorshift/exhaustive enumeration, no rapid/math/rand per check-architecture.sh:270-297) + testscript (rogpeppe/go-internal v1.16.0) + synthrepo |
| **Config file** | none |
| **Quick run command** | `go test -count=1 ./internal/domain/impact/ ./internal/application/blasting/ ./internal/interfaces/presenter/ && go test -count=1 -run 'TestScripts/blast\|TestBlast\|TestTextOutputsEscapeControls\|TestRuleRegistryDoc\|TestHelpMatchesDocs' ./cmd/gruntled/` |
| **Full suite command** | `go test -count=1 ./... && go vet ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh && bash scripts/compare-ref.sh v0.3.0` (CI adds `-race` on linux/macos/windows; no local gcc) |
| **Estimated runtime** | ~12 seconds (quick), ~90 seconds (full incl. compare-ref) |

PATH for every command: `export PATH=$HOME/.local/go/bin:$HOME/go/bin:$PATH`.

---

## Sampling Rate

- **After every task commit:** Run the quick run command
- **After every plan wave:** Run the full suite command
- **Before `/gsd:verify-work`:** Full suite green, 14-03 gate evidence in 14-03-SUMMARY.md, CI green on the three OSes
- **Max feedback latency:** 15 seconds

---

## Success Criteria → Tests

| # | Phase success criterion (ROADMAP) | Proven by |
|---|-----------------------------------|-----------|
| 1 | A<-B<-C chain: A 1, B 2, C 3, each with one shortest path back to the instantiating unit | `TestPropagateChain`, `TestCompute` (updated), `TestBlastTextGolden`/`TestBlastJSONGolden` (v2), `blast_transitive.txtar` case 1/7, `blast_impacted.txtar` live/edge |
| 2 | Only live block edges propagate; Broken traversed, listed only under Broken; disjoint and sorted | `TestPropagatesEdgeRule`, `TestComputeBrokenTraversed`, `TestDisjoint`, oracle (edge-fact variants), `blast_transitive.txtar` cases 2-3, hand mutations (a)(c) |
| 3 | Cycles, self-loops, diamonds terminate; minimum distance; canonical path; order-independent; brute-force oracle with shuffled input | `TestPropagateCycleSelfLoopDiamond`, `TestPropagateTieBreak` (sec #111 counterexample), `TestPropagateOrderIndependent`, `TestPropagateNoRecursion`, `TestTransitiveMatchesOracleExhaustive`, `TestTransitiveMatchesOracleRandom` (two shuffles, non-vacuity counters), `blast_transitive.txtar` cases 4-5 |
| 4 | `--depth N`; `--depth 1` == v0.3 Impacted set; 0/negative/non-integer exit 2 | `TestWithMaxDistance`, oracle per depth 1..4, `TestBlastDepthFlag`, `blast_exitcodes.txtar` depth cases, `TestBlastDepth1MatchesV1` (snapshots from the pre-Phase-14 commit), `blast_transitive.txtar` case 6 |
| 5 | JSON version 2, additive (distance, source, via; linear); text full path up to a fixed hop count, deterministic elision | `TestBlastJSONGolden`, `TestBlastJSONV1Compat`, `TestBlastJSONLinear`, `TestBlastJSONBrokenReach`, `TestBlastTextPathElision`, `TestBlastTextPathThroughBroken`, `TestBlastTextLinear`, escape tests, `TestBlastTransitiveSynthrepo` (size) |

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 14-01-01 | 01 | 1 | BLAST-03, BLAST-04 | unit (internal) | `go test -count=1 -run 'TestPropagate' ./internal/domain/impact/` | ❌ W0 (transitive.go, transitive_internal_test.go) | ⬜ pending |
| 14-01-02 | 01 | 1 | BLAST-03, BLAST-05 | unit + intended script updates | `go test -count=1 ./internal/domain/impact/ ./internal/application/blasting/ && go test -count=1 ./cmd/gruntled/ && bash scripts/compare-ref.sh v0.3.0` | ✅ impact_test.go (updated) | ⬜ pending |
| 14-01-03 | 01 | 1 | BLAST-04, BLAST-05 | property vs independent oracle (exhaustive + xorshift) | `go test -count=1 -run 'TestTransitiveMatchesOracle' -v ./internal/domain/impact/` | ❌ W0 (oracle_test.go) | ⬜ pending |
| 14-02-01 | 02 | 2 | BLAST-04 | unit (JSON v2) | `go test -count=1 -run 'TestBlastJSON\|TestBlastDeterministic' ./internal/interfaces/presenter/` | ✅ blast_test.go (extended) | ⬜ pending |
| 14-02-02 | 02 | 2 | BLAST-04 | unit (text, elision, escape) | `go test -count=1 ./internal/interfaces/presenter/` | ✅ blast_test.go (extended) | ⬜ pending |
| 14-02-03 | 02 | 2 | BLAST-04 | testscript + e2e escape + docs pins | `go test -count=1 -run 'TestScripts/blast\|TestTextOutputsEscapeControls\|TestRuleRegistryDoc\|TestHelpMatchesDocs' ./cmd/gruntled/` | ✅ | ⬜ pending |
| 14-03-01 | 03 | 3 | BLAST-05 | cmd unit + testscript + doc pins | `go test -count=1 -run 'TestBlastDepthFlag\|TestScripts/blast_exitcodes\|TestScripts/usage\|TestHelpMatchesDocs\|TestRuleRegistryDoc' ./cmd/gruntled/` | ❌ W0 (blast_depth_test.go) | ⬜ pending |
| 14-03-02 | 03 | 3 | BLAST-03, BLAST-04, BLAST-05 | testscript + synthrepo + snapshot equivalence | `go test -count=1 -run 'TestScripts/blast_transitive\|TestBlastTransitiveSynthrepo\|TestBlastDepth1MatchesV1' ./cmd/gruntled/ && bash scripts/compare-ref.sh v0.3.0` | ❌ W0 (blast_transitive.txtar, blast_transitive_test.go, blast_v1/, scripts/blast-snapshots.sh) | ⬜ pending |
| 14-03-03 | 03 | 3 | all | doc guard + gate | full suite + compare-ref (+ perturb self-test) + build-release + `git diff --exit-code go.mod go.sum` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers the phase; every ❌ W0 file is created by the first (TDD, red-first)
step of its own task, so no separate Wave 0 plan is needed.

---

## Mutation Proofs (non-vacuity)

| Mutation | Must fail | Where recorded |
|----------|-----------|----------------|
| `propagates` ignores Kind (paths edges propagate) | `TestPropagatesEdgeRule`, oracle | 14-01-SUMMARY |
| Unknown SkipOutputs/Enabled propagate | `TestPropagatesEdgeRule`, oracle | 14-01-SUMMARY |
| Frontier not sorted / first discovery not kept | `TestPropagateTieBreak`, `TestPropagateOrderIndependent`, oracle | 14-01-SUMMARY |
| Walk forward edges | `TestPropagateDirection`, oracle | 14-01-SUMMARY |
| Broken units not traversed | `TestComputeBrokenTraversed`, oracle counter "Broken on a shortest path" | 14-01-SUMMARY |
| Text path not capped / elision count off by one | `TestBlastTextPathElision`, `TestBlastTextLinear` | 14-02-SUMMARY |
| compare-ref.sh perturbed | exits 1 with a `DIFF ` line | 14-03-SUMMARY |

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `-race` on new tests | BLAST-03..05 | No local gcc | Confirm CI `check` job `go test -race -count=1 ./...` green after the push |
| Snapshot provenance | BLAST-05 | Snapshots are generated once from a past commit | 14-03-SUMMARY records the pre-Phase-14 commit hash and the exact `blast-snapshots.sh` command; `cmd/gruntled/testdata/blast_v1/README` names it |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 15s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending (draft at plan time; set to validated after execution)
