---
phase: 13-removed-output-diagnostic-shared-blast-core
plan: 02
subsystem: blasting
tags: [grt004, blast, between, property-test, testscript, escaping]
requires:
  - phase: 13-01
    provides: RemovedOutputs, SupersedeUnknownOutputs, CodeRemovedOutput
  - phase: 09
    provides: blasting.Blast, impact.Compute, blast text/JSON presenters
provides:
  - blasting.Between(base, cur checking.Report), the one pure composition point for two Reports
  - Blast delegating to Between, with Error.Stage "compare"
  - TestBetweenOnlyReclassifiesGRT001 (v0.3 oracle plus an independent MORE-03 oracle)
  - blast_grt004.txtar (cases 1-11 on real HCL)
affects: [13-03, 14, 16]
tech-stack:
  added: []
  patterns: [one pure core shared by CLI and daemon; property test against the previous release's composition]
key-files:
  created:
    - internal/application/blasting/between_property_test.go
    - cmd/gruntled/testdata/script/blast_grt004.txtar
  modified:
    - internal/application/blasting/blasting.go
    - internal/application/blasting/blasting_test.go
    - internal/interfaces/presenter/blast_test.go
    - cmd/gruntled/testdata/script/blast_impacted.txtar
    - cmd/gruntled/escape_test.go
key-decisions:
  - "The property is driven by a fixed in-test xorshift (3000 cases), not rapid: the application test allowlist (check-architecture.sh:305-309) is the domain one, stdlib plus testing/reflect only. Same choice as the 13-01 deviation the orchestrator accepted on bus #161."
  - "Between skips RemovedOutputs when either Report has a nil Graph (impact.Compute already treats nil as empty); Blast never passes one"
  - "Tasks 2 and 3 needed no production change (the plan expected none), so each is one test commit that fails against the pre-Between Blast"
requirements-completed: [MORE-03]
duration: 10min
completed: 2026-10-10
---

# Phase 13 Plan 02: Between, the Shared Blast Core Summary

**`blasting.Between` is now the only place where two check Reports become a blast Result: RemovedOutputs, then SupersedeUnknownOutputs on cur, then impact.Compute. `blast --base` reports a removed but still-referenced output as GRT004 at each reading site. A 3000-case property test compares every result with v0.3: findings may change only from GRT001 to GRT004 at the same site, and Broken, Impacted and the exit code stay the same.**

## Performance

- Duration: ~10 min
- Tasks: 3
- Files: 2 created, 5 modified

## Accomplishments

- `Between` (blasting.go:77): it calls `analysis.RemovedOutputs(base.Graph, cur.Graph)` (:80) and `analysis.SupersedeUnknownOutputs(cur.Diagnostics, removed)` (:84), then `impact.Compute(base.Graph, base.Diagnostics, cur.Graph, curD)` (:86). It does no I/O and never mutates its inputs (`TestBetweenPure`). The doc comment (:74) requires `base` to come from `checking.Check` (sec #138).
- `Blast` keeps its stage order (current, then baseline) and calls `Between(baseRep, curRep)`. An error from Between becomes `&Error{Stage: "compare"}` (blasting.go:62). The package doc and the `Error.Stage` doc name the new stage and say GRT004 is produced only here.
- `TestBlastBrokenAndImpacted` now pins the one finding as GRT004 at live/app/terragrunt.hcl:6:12, with the exact message. `TestBetweenEqualsBlast` checks that Between and Blast agree on four tree pairs: shifted, removed, unchanged, and a pre-existing GRT001. `TestErrorMessageAndUnwrap` covers `blasting: compare: inner`.
- `TestBetweenOnlyReclassifiesGRT001` generates 3000 base/cur pairs. Each has 6 units and 3 modules, with output subsets of {a,b,c}, unknown surfaces, module-unknown units, unresolved or non-unit targets, and the options default, enabled false/unknown, skip_outputs true/unknown and mock_outputs. Each cur tree gets 1-3 edits (drop or add an output, re-point a dependency, change a module, add or remove a reference, change an option, flip a surface to unknown). Against `impact.Compute` on the un-superseded sets, every case must have:
  - the same Broken subjects, per-subject counts, (Unit, Pos) sequences, Impacted units and HasErrors;
  - every differing finding GRT001 in v0.3 and GRT004 now, both SeverityError;
  - no site holding both codes;
  - a set of GRT004 sites equal to an oracle written from the MORE-03 text over the generator's own trees, without calling internal/domain/analysis.

  Counters: `3000 cases: 248 with >=1 GRT004, 10 with a GRT001 kept beside a GRT004, 428 with an unreferenced output removed`. The test fails if any counter is 0.
- `blast_grt004.txtar` runs 16 `blast` execs on real HCL. Positions are counted in the header comment: `  a = ` is 6 bytes, so column 7.
  - Case 1: two GRT004 lines at :8:7 and :9:7, with no GRT001. Repo-relative paths only (`! stdout $WORK`, `! stdout '^/'`).
  - Case 2: an unreferenced output is removed. Broken (0); live/db is Impacted with `-output name`.
  - Case 3: the reference was added in the same change, so it stays GRT001.
  - Case 4: the dependency was re-pointed to modules/other, so it stays GRT001.
  - Case 5a: the module is unknown in base, so cur reports GRT001. Case 5b: the module is unknown in cur, so the site is silent and only the GRT100 is Broken.
  - Case 6: the output was never declared in the baseline. Broken (0).
  - Cases 7a-7d: enabled false, skip_outputs true, and non-literal `local.*` values are silent.
  - Case 8a: a mock that applies gives GRT004 with the suffix. Case 8b: apply is not allowed, so GRT004 has no suffix.
  - Case 9: the output moved to outputs.tf and gained `deprecated`. Broken (0), Impacted (0).
  - Case 10: JSON has version 1 and two `"code": "GRT004"`.
  - Case 11: the swap direction gives Broken (0).
- `blast_impacted.txtar`: line 9 is now the GRT004 message, with `! stdout GRT001` added. The JSON block now asserts `"code": "GRT004"` and `! "code": "GRT001"`. No other line changed.
- `TestBlastTextGRT004` / `TestBlastJSONGRT004` (presenter): the GRT004 message is injected with raw ESC, BEL, U+009B, DEL and U+202E. The text output has the escaped forms and passes assertTerminalSafe. The JSON output has the `\u` escapes, decodes back to the raw message, and carries `"code": "GRT004"` and `"version": 1`.
- escape_test.go: `craftedRepo` gains unit `rm`, which reads `dependency.vpc.outputs.old` of producer `v<U+202E>p`.
  - Blast text has `rm/terragrunt.hcl:4:17: GRT004 dependency "vpc" output "old" was removed from module "v\u202ep" (target unit "v\u202ep")` and no GRT001 at that site.
  - Blast JSON decodes to the same message, with a literal 6-byte `\u202e` from strconv.Quote, as sec #134 requires; strconv.Quote is kept.
  - Check shows the GRT001 for rm and no GRT004 anywhere. Report text still equals check. No existing count changed.

### Hand mutations (run, then reverted)

| Mutation | Fails |
|----------|-------|
| (a) SupersedeUnknownOutputs call removed (curD = cur.Diagnostics) | TestBetweenOnlyReclassifiesGRT001, case 12: `GRT004 sites map[], oracle map[{live/u0 live/u0/terragrunt.hcl:14:3}:true]`; TestBlastBrokenAndImpacted |
| (b) removed appended to cur instead of superseding | TestBetweenOnlyReclassifiesGRT001, case 12: `Broken[0] = live/u0 (2), v0.3 live/u0 (1)`; TestBlastBrokenAndImpacted |
| Blast restored to the v0.3 body (impact.Compute directly) | TestScripts/blast_impacted (line 8), TestScripts/blast_grt004 (case 1, `no match for GRT004`), TestTextOutputsEscapeControls/blast and /blast_json |

## Task Commits

1. Task 1 red: `68ae2e1` test(13-02): Between equals Blast, purity and GRT001-only reclassification property
2. Task 1 green: `eb5335c` feat(13-02): blasting.Between as the shared pure core; Blast delegates
3. Task 2: `7081d27` test(13-02): GRT004 end to end on real HCL, blast_impacted moves to GRT004, presenter escape
4. Task 3: `fbe928f` test(13-02): crafted producer name through a GRT004 message, end to end

## Verification

- `go test -count=1 -v -run 'TestBetween|TestBlast|TestError' ./internal/application/blasting/`: all PASS.
- `go test -count=1 -run 'TestScripts/blast' ./cmd/gruntled/` and `go test -count=1 -run 'Blast' ./internal/interfaces/presenter/`: ok.
- `go test -count=1 -run TestTextOutputsEscapeControls -v ./cmd/gruntled/`: all 7 subtests PASS (watch, report, blast, check json, check sarif, graph json, blast json).
- `go test -count=1 ./...`: every package ok (cmd/gruntled 11.7s, blasting 0.63s, analysis 0.25s, ...).
- `go vet ./...`: clean.
- `bash scripts/check-architecture.sh`: `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)`.
- `bash scripts/test-check-architecture.sh`: `all architecture self-tests passed`.
- `git diff --stat v0.3.0 -- cmd/gruntled/testdata` shows only the two intended files:
  ```
   cmd/gruntled/testdata/script/blast_grt004.txtar   | 706 ++++++++++++++++++++++
   cmd/gruntled/testdata/script/blast_impacted.txtar |   5 +-
   2 files changed, 710 insertions(+), 1 deletion(-)
  ```

- `-race` was not run locally: there is no gcc and no cgo. CI runs it on linux, macos and windows.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-13-02-1 | mitigated | blasting.go:84 (SupersedeUnknownOutputs: replace only, never add); TestBetweenOnlyReclassifiesGRT001 (between_property_test.go:335) compares with v0.3 Compute and checks "no site with both codes"; hand mutations (a) and (b) above |
| T-13-02-2 | mitigated | blasting.go:77 (Between, the only composition point, pure); Blast calls it at blasting.go:60; TestBetweenEqualsBlast (blasting_test.go:224), TestBetweenPure (:243) |
| T-13-02-3 | mitigated | presenter's existing escapeTerm/escapeJSON, unchanged; TestBlastTextGRT004 (blast_test.go:327), TestBlastJSONGRT004 (:337); escape_test.go:164 (text) and :206 (JSON) for the GRT004 site |
| T-13-02-4 | mitigated | messages built only from RepoPath (grt004.go:101-104); blast_grt004.txtar:20-21 (`! stdout $WORK`, `! stdout '^/'`) |
| T-13-02-5 | mitigated | blast_grt004.txtar case 5a/5b (lines 62-77); 13-01 matrix rows for base/cur module unknown |
| T-13-02-6 | accepted | Between is linear in references; the property is 3000 small fixed cases (~0.4 s) |
| T-13-02-7 | accepted | blasting.go:74-76 doc comment requires a base from checking.Check; re-verify in Phase 16 |

## Deviations from Plan

**1. [Rule 3 - blocking] No rapid in the application tests**
- The plan names rapid for `TestBetweenOnlyReclassifiesGRT001`. `application-stdlib-allowlist` (check-architecture.sh:305-309) uses the domain test allowlist (stdlib plus testing/reflect), so rapid fails the gate there as it does in the domain. The property therefore uses a fixed xorshift over 3000 cases. All of the plan's assertions and its three non-vacuity counters are kept. This is the same resolution the orchestrator accepted for 13-01 (bus #161).

**2. [Hardening] Nil graph guard in Between**
- Between skips RemovedOutputs when a Report has a nil Graph, because `RepositoryGraph.References` on a nil pointer would panic. impact.Compute already treats nil as empty. Blast never passes a nil Graph, so no observable behaviour changes.

**3. [Process] Tasks 2 and 3 have no feat commit**
- They add tests and goldens only; the plan says no presenter or source change is expected. Each was checked as red against the pre-Between Blast (see the hand mutations above) before it was committed.

## Known Risks / Unverified

- `-race` not run locally (no gcc).
- The kept-beside-GRT004 counter is low (10 of 3000), but it is above zero and asserted.

## Self-Check: PASSED
