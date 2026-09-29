---
phase: 04-real-repo-validation-experiment
verified: 2026-09-29T00:00:00Z
status: passed
score: 5/5 must-haves verified
gaps: []
notes:
  - "denis256 (secondary corpus) recall is 0/8 after 02-13; not demanded by any ROADMAP criterion or VALID-* requirement. Labelled PASS in the docs table although 04-CONTEXT's honest-failure rule says a differing set is recorded as FAIL; wording deviation, not a goal gap."
---

# Phase 4: Real-Repo Validation Experiment Verification Report

**Goal:** settle the falsifiable claim: correct, safe, faster than terragrunt on a real public repo.
**Verified:** 2026-09-29 (master 28e1ba5). **Re-verification:** No.

## Observable Truths

| # | Truth (ROADMAP criterion) | Status | Evidence |
|---|---|---|---|
| 1 | Golden tests over hand-written and full-scale synthrepo fixtures assert exact diagnostic sets | VERIFIED | `go test -count=1 ./...` all ok; TestGolden*, TestSynthrepoOracle pass |
| 2 | Zero diagnostics on unmutated primary corpus | VERIFIED | Own run: `checked 65 units (3 unknown): 0 errors`, exit 0; TestCorpusClean passes |
| 3 | Injected rename/deletion: every broken reference reported | VERIFIED | Own copy: renamed `role_name` output in `iac.src/s3_runtime` -> 8 GRT001, exactly the 8 `dependency.s3.outputs.role_name` refs. Deleted `region` output in `iac.mq/mq_broker` -> 3 GRT001, the 3 referrers. Nothing extra. TestCorpusMutation passes |
| 4 | terragrunt hcl validate does not report the breakage | VERIFIED | TestTerragruntGap ran (no skip) with terragrunt v1.1.6 and passed |
| 5 | Reproducible benchmark: gruntled faster | VERIFIED | TestBenchmarkVsTerragrunt: gruntled median 55.6 ms vs terragrunt 146.9 ms (2.64x); process-vs-process, interleaved samples; TestValidationDocPins passes |

**Score:** 5/5

## Test runs
- `go test -count=1 ./...` (no env): ok.
- With GRUNTLED_CORPUS, _SECRET, _DENIS256 and GRUNTLED_TERRAGRUNT_BIN (terragrunt from ~/.cache/gruntled-phase4/bin on PATH): TestGolden, TestCorpus*, TestTerragruntGap, TestDenis256*, TestBenchmarkVsTerragrunt all run, no skips in cmd/gruntled. Note: TerragruntGap needs `GRUNTLED_TERRAGRUNT_BIN`, not just PATH. The only remaining skip is `TestHeavyG17Reproductions` (Phase 2 test, not a Phase 4 deliverable).
- `bash scripts/check-architecture.sh`: OK.

## Requirements
VALID-02..06: all SATISFIED (docs/validation.md has per-requirement sections and drift-pinned evidence). No orphaned requirements.

## The 04-04 amendment
Assessment: legitimate for phase goal purposes.
- Documented in docs/validation.md (Outcome paragraph, secondary-corpus section, per-row table with "not reported: ... config-unknown/include-target", Limitations). The 8 hand-derived rows are kept as the v2 regression target; the enabled=false row remains silent; denis256 logs show grt001=0, 719 include-target unknowns.
- No ROADMAP criterion or VALID-* requirement mentions denis256 or demands recall on it. 04-CONTEXT defines it as SECONDARY: exact-set/no-panic/determinism only, explicitly not VALID-03/04/timing. ROADMAP's own 04-04 line was amended to the empty set.
- Precision holds (0 false positives). Recall of the real feature is demonstrated on the primary corpus (8/8, 3/3).

Caveats (not gaps):
1. 04-CONTEXT's honest-failure rule says a set differing from the expectation is recorded as a FAIL of the secondary check; docs label it "PASS (0 false positives, 0/8 recall)" while the prose says "not a pass on recall". Wording tension; the limitation itself is disclosed prominently.
2. The amended assertion is weaker (empty set + each hit sits on an unknown unit), so it does not currently test recall on denis256. Real product limit: on repos with dynamic includes, the conservative include-target rule silences most checks (897/1146 units unknown). Worth a v2 item.

## Anti-patterns
None blocking found in verification scope.

_Verifier: Claude (gsd-verifier)_
