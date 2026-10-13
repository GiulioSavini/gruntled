---
phase: 13-removed-output-diagnostic-shared-blast-core
verified: 2026-10-10T16:07:07Z
status: passed
score: 4/4 must-haves verified
covered_files:
  - .planning/phases/13-removed-output-diagnostic-shared-blast-core/13-01-PLAN.md
  - .planning/phases/13-removed-output-diagnostic-shared-blast-core/13-01-SUMMARY.md
  - .planning/phases/13-removed-output-diagnostic-shared-blast-core/13-02-PLAN.md
  - .planning/phases/13-removed-output-diagnostic-shared-blast-core/13-02-SUMMARY.md
  - .planning/phases/13-removed-output-diagnostic-shared-blast-core/13-03-PLAN.md
  - .planning/phases/13-removed-output-diagnostic-shared-blast-core/13-03-SUMMARY.md
  - README.md
  - cmd/gruntled/escape_test.go
  - cmd/gruntled/main.go
  - cmd/gruntled/more07_test.go
  - cmd/gruntled/report_test.go
  - cmd/gruntled/rules_doc_test.go
  - cmd/gruntled/testdata/script/blast_grt004.txtar
  - cmd/gruntled/testdata/script/blast_impacted.txtar
  - cmd/gruntled/watch_test.go
  - docs/cli.md
  - internal/application/blasting/between_property_test.go
  - internal/application/blasting/blasting.go
  - internal/application/blasting/blasting_test.go
  - internal/domain/analysis/grt001.go
  - internal/domain/analysis/grt001_test.go
  - internal/domain/analysis/grt004.go
  - internal/domain/analysis/grt004_internal_test.go
  - internal/domain/analysis/grt004_test.go
  - internal/domain/diagnostic/diagnostic.go
  - internal/domain/diagnostic/diagnostic_test.go
  - internal/interfaces/presenter/blast_test.go
  - scripts/compare-ref.sh
covered_digest: "v3:sha256:4ee4a2e01959841929e2bbf2b0ecf9ea8732fcdd478d61150f762b584b06c572"
behavior_unverified: 0
overrides_applied: 0
deferred:
  - truth: "MORE-03 in `report --blast`: GRT004 at the same sites, exit 1, through the daemon"
    addressed_in: "Phase 16"
    evidence: "Phase 16 SC1: `report --blast` bytes equal `blast --base <copy of the tree at baseline time>` (property test). Phase 13 ships the shared pure core it must call, `blasting.Between`."
---

# Phase 13: Removed-Output Diagnostic & Shared Blast Core Verification Report

**Phase Goal:** User sees an output that was removed from a module but is still referenced by `dependency.X.outputs.Y` reported as an error at each consuming reference (`GRT004`), by `gruntled blast --base`, through one shared comparison core that every later blast surface reuses.
**Verified:** 2026-10-10T16:07:07Z, HEAD 307e83d. The code is identical to 8a6dd48: `git diff --stat 8a6dd48 HEAD -- . ':!.planning'` is empty, since the three later commits only plan Phase 14.
**Status:** passed
**Re-verification:** No. This is the initial verification.

## Observable Truths

| # | Truth (ROADMAP success criterion) | Status | Evidence |
|---|-----------------------------------|--------|----------|
| 1 | Removing from a module an output that a unit still references makes `gruntled blast --base <dir> <path>` report one `GRT004` error at each such reference site and no `GRT001` at those sites, with exit code 1. The message names the output, the module and the target unit, and uses repo-relative paths only. | VERIFIED | **Code:** `analysis.RemovedOutputs` (grt004.go) emits one `CodeRemovedOutput` at `SeverityError` per cur reference site, at the GRT001's `(Unit, Pos)`. The message is `dependency "<label>" output "<Y>" was removed from module "<module>" (target unit "<target>")`, built from `RepoPath.String()` values. `blasting.Between` composes `RemovedOutputs` → `SupersedeUnknownOutputs` → `impact.Compute`, and `Blast` returns `Between(baseRep, curRep)`. **Live, on my own trees** (scratchpad `vf13`, built with `go build -o`): app1 reads a removed `vpc_id` on lines 8 and 9, and I got two `GRT004 dependency "net" output "vpc_id" was removed from module "modules/net" (target unit "live/net")` lines at `:8:7` and `:9:7`, no GRT001 there, and exit 1. JSON has 3 `"code": "GRT004"` and `"version": 1`, with no `/tmp/` in the output. **Second pair** (`vf13b`): a reference in a shared include `live/_shared/common.hcl` gives one GRT004 per including unit (`live/x`, `live/y`). A reference whose line moved (8 → 10) is still GRT004, because the baseline match is position-free. Running the pair in reverse (output added back) gives `Broken (0)` and exit 0. **Tests:** `TestRemovedOutputsMatrix`, `blast_grt004.txtar` cases 1, 10 and 11, and `blast_impacted.txtar` (line moved GRT001 → GRT004 on purpose) all PASS. |
| 2 | `GRT004` only reclassifies an existing `GRT001`. For the same two trees, the Broken units and the number of findings per unit equal what v0.3.0 `blast` printed. No site ever shows both codes, and removing an output that nothing references adds no finding. | VERIFIED | **Against a real v0.3.0 binary** (built from the tag through a scratch worktree): on `vf13`, HEAD and v0.3.0 `blast --base base cur` both exit 1 with `Broken (6)` and the same `Impacted`. The HEAD text, with `GRT004 … was removed from module` rewritten to `GRT001 … is not declared by module`, is byte-identical to v0.3.0. In JSON the broken units, per-unit counts and every finding's file/line/column are equal, and codes differ only as GRT001 (v0.3.0) → GRT004 (HEAD). The `vf13b` pair also matches v0.3.0 in the same way. The removed but unreferenced `unused` output adds no finding: it appears only in Impacted (`-output unused`), as it did in v0.3.0. **Code:** `SupersedeUnknownOutputs` consumes one GRT001 per GRT004 by `(Unit, Pos)`, never by message, and drops a GRT004 with no matching GRT001, so `Len` never grows. Why Broken cannot change: NewFindings is position-free on (code, unit, file, message). A base GRT001 with the same key would mean the base module at the same path does not declare Y, which contradicts precondition 4, so every reclassified site was already new in v0.3. **Tests:** `TestBetweenOnlyReclassifiesGRT001` runs 3000 generated pairs against v0.3's `impact.Compute` on un-superseded sets, with an oracle written independently from the MORE-03 wording over the generator's model. Its non-vacuity counters (≥1 GRT004, GRT001 kept beside a GRT004, unreferenced removal) are asserted > 0. `TestSupersedeUnknownOutputs` and `TestSupersedeNeverAddsProperty` also PASS. **My mutations** (scratch worktree): with `SupersedeUnknownOutputs` removed from `Between`, `TestBetweenOnlyReclassifiesGRT001` fails (`case 12: GRT004 sites map[], oracle …`), and so do `blast_grt004` and `TestNoGRT004OutsideBlast`. |
| 3 | Wherever a precondition fails, the site behaves exactly as `check` does in v0.3.0. A reference added in the same change, a dependency re-pointed to another module, a module unknown on either side, or an output the baseline surface did not declare keeps `GRT001`. `enabled = false`, `skip_outputs = true` or a non-literal value for either stays silent. `mock_outputs` never suppresses the error. | VERIFIED | **Live on `vf13`, same tree pair:** a reference added in the same change (app3) stays GRT001. A dependency re-pointed from `../net` to `../oth` (app4) stays GRT001 with module `modules/other`. A module unknown in the baseline (app5, `modules/net3` absent in base) stays GRT001. A module unknown in the current tree (app6) is silent, as in v0.3.0 `check`, because row 5 is silent. A baseline that did not declare the output (app7: base reference silenced by `skip_outputs = true`, cur flips to `false`, `never` never declared) stays GRT001. `enabled = false` (app8), `skip_outputs = true` (app9), non-literal `enabled = local.on` (app10) and non-literal `skip_outputs = local.skip` (app11) are all silent. `mock_outputs = { vpc_id = … }` with merge-with-state and `apply` allowed (app2) still gives GRT004, with GRT001's mock suffix. Every one of these matches the real v0.3.0 binary (truth 2). **Code:** `fires` uses the same `resolveReference` predicate as `UnknownOutputs` (rows 1-5 extracted, and `TestDIAG03`'s 22 rows are unchanged, only hoisted into `diag03Rows`). `sameModule` and `baseDeclared` use `resolveDependency` on the base, so the module must be known on both sides. The suffix comes from `mockMasksAtApply` on the cur options and surface. **Tests:** `TestRemovedOutputsDIAG03Parity` (all 22 rows), `TestRemovedOutputsChecksAreNecessary` (each of the 4 checks killed by a named row) and `blast_grt004.txtar` cases 3-9 PASS. **My mutation:** with `baseDeclared` reduced to `baseOK`, `TestRemovedOutputsMatrix`, `TestBetweenOnlyReclassifiesGRT001`, `TestBlastShiftedFindingNotBroken` and `blast_grt004.txtar:81` FAIL. |
| 4 | `check` (text, JSON, SARIF), plain `report`, `graph --json` and the status file never print `GRT004` and stay byte-identical to v0.3.0 on pinned goldens. | VERIFIED | **`bash scripts/compare-ref.sh v0.3.0`: `244/244 identical`, exit 0.** It builds v0.3.0 from the verified commit in its own worktree, then compares check text/json/sarif and graph --json bytes and exit codes on every golden, diag03 and blast_grt004 tree, plus clean-fixture and sarif-fixture. Its self-test `COMPARE_REF_PERTURB=1` gives `243/244`, exit 1, so the comparison can fail. `git diff --stat v0.3.0 HEAD -- cmd/gruntled/testdata/` lists only `blast_grt004.txtar` (new) and `blast_impacted.txtar` (blast, not check). No check, graph or SARIF golden changed. **Live on `vf13/cur`:** HEAD and v0.3.0 `check` text/json/sarif and `graph --json` are byte-identical (`cmp`), with 0 GRT004 and 7 GRT001. A real `gruntled watch --poll` daemon gave `report` text/json/sarif byte-equal to `check`, with 0 GRT004. The status file read `gruntled: 7 errors (GRT001×7)` and the watch stdout had 0 GRT004. **Code:** `CodeRemovedOutput` is used in production only in grt004.go. It is not in `checking.analyzers`, and presenter/statusfile are unchanged since v0.3.0. The SARIF rule table is still GRT001/002/003/100. **Tests:** `TestNoGRT004OutsideBlast` (non-vacuous blast run first; `report_linux` and `report_windows` subtests ran), `TestCheckCannotProduceGRT004` (every tree in blast_grt004.txtar, ≥20 trees) and `TestSARIFDoc` PASS. |

**Score:** 4/4 truths verified (0 present-but-behavior-unverified)

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | MORE-03's `report --blast` half (GRT004 through the daemon, exit 1) | Phase 16 | Phase 16 SC1 requires `report --blast` bytes to equal `blast --base` (property test). `Between` is the pure core it must call. MORE-03's `blast --base` half is complete here. |

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/domain/analysis/grt004.go` | `RemovedOutputs`, `SupersedeUnknownOutputs`, `removedOutputChecks` | VERIFIED | 195 lines, substantive. Each fact is computed independently (never gated on `baseHadRef`), which is what the necessity test relies on. Called from `blasting.Between` |
| `internal/domain/analysis/grt001.go` | `resolveReference`, `resolveDependency` shared with GRT004 | VERIFIED | `UnknownOutputs` calls `resolveReference(g, ur)` for rows 1-5. grt004.go calls `resolveDependency(base, …)` |
| `internal/domain/diagnostic/diagnostic.go` | `CodeRemovedOutput = "GRT004"` | VERIFIED | Present. Not part of `checking.analyzers` |
| `internal/application/blasting/blasting.go` | `Between` (pure), with `Blast` delegating | VERIFIED | `Blast` keeps its stage order (current, then baseline), and an error from `Between` returns `&Error{Stage: "compare"}`. `Between` does no I/O (`TestBetweenPure`, `TestBetweenEqualsBlast`) |
| `cmd/gruntled/testdata/script/blast_grt004.txtar` | e2e per precondition | VERIFIED | 706 lines. PASS |
| `cmd/gruntled/more07_test.go`, `cmd/gruntled/rules_doc_test.go` | MORE-07 pins and doc-sync pins | VERIFIED | PASS |
| `scripts/compare-ref.sh` | byte comparison against a ref | VERIFIED | 244/244. The perturb self-test fails as designed. It validates the ref and rejects unsafe txtar names and links (sec F2) |
| `docs/cli.md`, `README.md`, `blastUsage` | GRT004 documented | VERIFIED | `### GRT004: dependency output removed (error, blast only)` has the four-condition table and the message. The README codes table has a blast-only GRT004 row. `blast -h` has the GRT004 sentence, mirrored in the docs (`TestHelpMatchesDocs`, `TestRuleRegistryDoc`) |

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `UnknownOutputs` (grt001.go) | `resolveReference` | rows 1-5 in one call | WIRED | `resolveReference(g, ur)`. `TestDIAG03` is unchanged and green, and the check goldens are byte-identical to v0.3.0 |
| grt004.go | `resolveReference`, `resolveDependency`, `mockMasksAtApply` | cur fact and base side | WIRED | `resolveDependency(base, ur.Unit, dep)` |
| `Blast` | `Between` | `Between(baseRep, curRep)` | WIRED | Live blast output carries GRT004 |
| `Between` | `RemovedOutputs` → `SupersedeUnknownOutputs` → `impact.Compute` | pure composition | WIRED | The mutation that removes Supersede is caught |
| `rules_doc_test.go` | docs/cli.md, README.md, blastUsage | exact-line pins | WIRED | `TestRuleRegistryDoc` PASS |

## Data-Flow Trace (Level 4)

| Artifact | Data | Source | Real data | Status |
|----------|------|--------|-----------|--------|
| `blast --base` text/JSON | GRT004 findings | real HCL in both trees → `checking.Check` ×2 → `Between` → presenter | yes. Hand-built trees give the predicted sites, with messages built from the real module and target paths | FLOWING |

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| SC1-SC3 on hand-built trees | `ghead blast --base base cur` (vf13: 12 units, every precondition) | 3 GRT004 (app1 ×2, app2 with the mock suffix), GRT001 at app3/4/5/7, app6/8/9/10/11 silent, exit 1 | PASS |
| v0.3.0 equivalence | `gv03 blast --base base cur`, text and JSON diff | identical modulo GRT001→GRT004 at the same file:line:col | PASS |
| Shared include, moved line, reverse | vf13b pair | GRT004 per including unit; moved reference still GRT004; reverse gives Broken (0), exit 0 | PASS |
| check/graph byte identity | `cmp` of HEAD vs v0.3.0 on `vf13/cur` | identical, 0 GRT004 | PASS |
| report and status file | `ghead watch --poll --status-file <0700 dir>/status.txt cur`, then `report --format text/json/sarif` | report == check (3 formats); status `gruntled: 7 errors (GRT001×7)`; SIGINT exit 0; no leftover daemon | PASS |
| Full suite | `go test ./...` | ok in all 20 packages | PASS |
| Vet / fmt | `go vet ./...`, `gofmt -l .` | clean | PASS |
| Architecture | `bash scripts/check-architecture.sh`; `bash scripts/test-check-architecture.sh` | `architecture: OK (4 domain, 5 application, 1 interfaces)`; all self-tests passed | PASS |
| v0.3.0 byte gate | `bash scripts/compare-ref.sh v0.3.0` | 244/244 identical, exit 0 (perturbed: 243/244, exit 1) | PASS |
| Release build | `bash scripts/build-release.sh v0.0.0-ci <sha> <tmp>` | all archives OK, smoke prints the version | PASS |
| No new deps | `git diff --exit-code v0.3.0 -- go.mod go.sum` | exit 0 | PASS |
| CI | `gh run view 38064624899` (headSha 8a6dd48) | success: check (`go test -race`), architecture, recipe-check, sarif-upload, test-os macos and windows. Run 38063796858 at e52e952 was green too | PASS |

## Probe Execution

Not applicable. The phase declares no `scripts/*/tests/probe-*.sh`. `compare-ref.sh` is the phase gate and was run above.

## Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|--------------|-------------|--------|----------|
| MORE-03 | 13-01, 13-02, 13-03 | GRT004 replaces GRT001 at exactly the removed-output sites, under four preconditions, sharing GRT001's table; error, exit 1; repo-relative message | SATISFIED for `blast --base`; `report --blast` deferred to Phase 16 | Truths 1-3 |
| MORE-07 | 13-03 | check/report/graph/status never emit GRT004 and stay byte-identical to v0.3.0 | SATISFIED | Truth 4 |

No orphaned requirements: REQUIREMENTS.md maps only MORE-03 and MORE-07 to Phase 13.

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None | | No TBD/FIXME/XXX/TODO/HACK/placeholder in the 22 files changed since v0.3.0. The only match, docs/cli.md:370 `\uXXXX`, is an escape notation, not a marker | | |

Test quality: no skipped tests in the phase test files. The oracle in `TestBetweenOnlyReclassifiesGRT001` works over the generator's model, not over `RemovedOutputs`, and both of my mutations were caught.

Informational (non-blocking):
- `RemovedOutputs` keeps a defensive `if !curDepOK { continue }` after the checks. With the full check list it is unreachable, because `fires` and `sameModule` both imply `curDepOK`, and the comment says so. It is harmless.
- `TestBetweenOnlyReclassifiesGRT001` models v0.3.0 as `impact.Compute` on the un-superseded sets, not as the real v0.3.0 binary. My hand-built pairs, compared against a binary built from the tag, close that gap for the cases they cover.

## Human Verification Required

None. VALIDATION's two manual items are covered. `-race` is green in the CI `check` job at 8a6dd48. The daemon `report` path of `TestNoGRT004OutsideBlast` ran here on linux, both the socket and the windows report-dump subtests, and CI test-os macos and windows are green.

## Gaps Summary

None. GRT004 is produced only by `blast --base`, through `blasting.Between`, at exactly the sites the four preconditions select, and only by replacing a GRT001 at the same `(Unit, Pos)`. I confirmed this on two independent hand-built tree pairs against a real v0.3.0 binary, and the property and mutation tests catch regressions. `check`, `report`, `graph` and the status file are byte-identical to v0.3.0 (244/244). The only open item is MORE-03's `report --blast` half, which is scheduled for Phase 16 and will reuse `Between`.

## Security

Verdict **SECURED** (13-SECURITY.md, gruntled-sec bus verdict #180): 0 open threats. Both findings were INFO and are fixed. F1, the README GRT004 guard, is now paragraph-scoped (ddfbf2d), and mutation M15 now fails. F2, compare-ref.sh, now verifies the ref with `rev-parse --verify --end-of-options` and rejects unsafe txtar names and links (f4b1e1f). Both files are in the covered set. `TestRuleRegistryDoc` passes, and `compare-ref.sh` is still 244/244 on them.

---

_Verified: 2026-10-10T16:07:07Z_
_Verifier: Claude (gsd-verifier)_
