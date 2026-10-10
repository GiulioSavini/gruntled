---
phase: 13-removed-output-diagnostic-shared-blast-core
type: research
created: 2026-10-10
sources:
  - .planning/research/SUMMARY.md (reconciliation #2 GRT004 vs GRT001, binding)
  - .planning/research/ARCHITECTURE.md (Q3, component table, build order 1)
  - .planning/research/PITFALLS.md (1, 2, 3, 10, 16, 17, 19)
  - .planning/REQUIREMENTS.md (MORE-03, MORE-07, Terms)
  - agent bus #108 (MORE-03 wording, per-site not per-triple), #116 (MORE-07 scope)
---

# Phase 13 — Research (short)

Milestone research already fixes the design (GRT004 only in `blasting.Between`, supersede by
structured key, shared GRT001 predicate, never in `check`). This file records only what reading the
code at HEAD `2b529d1` adds: exact integration points, the tests and docs that must change on
purpose, and four decisions the milestone research left open. `git diff v0.3.0 HEAD -- cmd internal
docs` is empty, so every line number below is also the v0.3.0 line number.

## 1. Integration points (file:line at 2b529d1)

| Where | What is there | Phase 13 change |
|-------|---------------|-----------------|
| `internal/domain/analysis/grt001.go:40-88` | `UnknownOutputs`: rows 1-5 inline at 47-69 (`unit.Dependency` row 1, `opts.Enabled != TristateTrue` row 2, `opts.SkipOutputs != TristateFalse` row 3, `g.DependencyTarget` row 4, `g.ModuleOf` + `mod.Surface()` row 5), row 6 `surf.HasOutput` at 71, message 74-80, `mockMasksAtApply` 97-109 | Extract rows 1-5 into `resolveReference`; rows 1, 4, 5 into `resolveDependency` (used for the baseline side). No output change. |
| `internal/domain/analysis/grt001_test.go:184-335` | `TestDIAG03`: 22 rows (`row1`..`row7j`) as a local slice, scenario builder `buildScenario` 118-174 | Hoist the rows to a package-level `diag03Rows()` (test-only move, TestDIAG03 body unchanged) so `grt004_test.go` reuses the same table for the parity matrix. |
| `internal/domain/diagnostic/diagnostic.go:18-29` | Code constants GRT001-003, GRT100 | Add `CodeRemovedOutput Code = "GRT004"`. |
| `internal/application/checking/check.go:47-51` | `analyzers []func(*repograph.RepositoryGraph) (...)` | Unchanged. The one-graph signature makes it a type error to add a two-graph analyzer: this is the structural half of MORE-07. |
| `internal/application/checking/check.go:18-25` | `Report{Graph, Diagnostics}` | Unchanged; `Between` takes two of these. |
| `internal/domain/impact/impact.go:39-54` | `NewFindings` (multiset, position-free `FindingKey` 22-33) | Unchanged. GRT004 never exists in a baseline set, so it is always new. |
| `internal/domain/impact/impact.go:97-134` | `Compute(baseG, baseD, curG, curD)` | Unchanged signature and body. `Between` feeds it the superseded `curD`. |
| `internal/application/blasting/blasting.go:44-57` | `Blast` checks cur, then base, then calls `impact.Compute` at 56 | Add `Between(base, cur checking.Report) (impact.Result, error)`; `Blast` calls it. Error type 23-37: `Stage` doc says "current" or "baseline"; add "compare". |
| `internal/interfaces/presenter/blast.go:78-128, 156-197` | `BlastText` writes `d.Code()` then `escapeTerm(d.Message())` (104-106); `BlastJSON` writes code/message and post-passes `escapeJSON` (195) | No code change needed: GRT004 is a code string and a message like any other. `blastSchemaVersion` stays 1 (Phase 14 bumps it once). Tests added for a GRT004 finding with crafted names. |
| `cmd/gruntled/main.go:64-84` | `blastUsage` | One sentence on GRT004. Exit-code lines unchanged (`TestHelpMatchesDocs` pins only those, e2e_test.go:411-446). |
| `cmd/gruntled/main.go:309-367` | `runBlast` -> `blasting.Blast` (347), exit 1 iff `res.HasErrors()` (362) | Unchanged: GRT004 is SeverityError, so exit 1 follows. |

## 2. Tests and docs that change on purpose

These are exactly the GRT004 shape (base references an output its module declared, cur module lost
it, same module path, defaults for `enabled`/`skip_outputs`). Any other golden diff is a bug.

| File | Line | Today | After |
|------|------|-------|-------|
| `cmd/gruntled/testdata/script/blast_impacted.txtar` | 9 | `GRT001 dependency "db" output "id"` | `GRT004 dependency "db" output "id"`; add `! stdout GRT001` |
| `docs/cli.md` (Blast text example) | 488 | `GRT001 dependency "db" output "id" is not declared by module "modules/vpc" (target unit "live/db")` | the GRT004 message for the same site |
| `internal/application/blasting/blasting_test.go` | 152-171 | `TestBlastBrokenAndImpacted` asserts subjects only (still passes) | add: the one finding is GRT004 at live/app:6:12 |

Not changing (checked): `blast_exitcodes.txtar` (clean->broken edits the reference name `vpc_id`
-> `vpc_idd`: base had no reference to `vpc_idd`, stays GRT001); `cmd/gruntled/escape_test.go`
`craftedRepo` (base references `vpc_id`, cur references `vpc_idd`: stays GRT001); every check,
report, graph and SARIF golden.

## 3. Doc-sync traps

- `TestSARIFDoc` (sarif_test.go:219-275) collects every docs heading matching
  `^### (GRT\d{3}): (.+) \((error|warning|note)\)$` and requires the list to equal the SARIF rules
  `check` emits. A `### GRT004: ... (error)` heading would fail it, and adding a SARIF rule would
  break MORE-07 (SARIF bytes). Use `### GRT004: dependency output removed (error, blast only)`,
  which the regex does not match, and pin it with a new doc test instead.
- `sarif_internal_test.go:47-50` pins four `helpUri`s: unchanged.
- `docs/cli.md:684-687` "Reserved codes": "GRT004 to GRT006 are planned" becomes "GRT005 and GRT006".
- `README.md:150` "Later: diagnostics GRT004-GRT006" and the code table 380-393 ("Four are defined
  in code today", "All four are emitted by `gruntled check`") must say GRT004 exists and is blast-only.
- `docs/cli.md` Known limitations 965-975: add the GRT004 boundaries (reference added in the same
  change, re-pointed dependency, unknown module either side stay GRT001; blast only, no SARIF rule).
- `TestReadmeDocumentsV03` pins substrings only; still green.

## 4. Site identity and the supersede key

- A GRT001 is raised per reference site (`g.References()`, grt001.go:42), with `Unit = ur.Unit` and
  `Pos = ur.Reference.Pos()` (grt001.go:81). Reference positions are the start byte of the
  traversal (terragrunt/refs.go:64-77), so within one unit two sites never share a `Pos`; a shared
  include gives one site per including unit (different `Unit`).
- Supersede key: `(Code == GRT001, Unit, Pos)` — never the message. Multiset matching (each GRT004
  consumes one GRT001) is still used so a theoretical duplicate site cannot leave a stray GRT001.
  `diagnostic.NewSet` (set.go:23) already collapses identical Keys, so two GRT001 with equal
  `(Unit, Pos, Message)` cannot coexist.
- A GRT004 with no GRT001 to consume is dropped, never added (keeps "never adds a finding" true even
  if the two computations ever drift). Tested directly.

## 5. Why the Broken sets equal v0.3.0 (success criterion 2)

Every GRT004 site S had, in v0.3.0, a GRT001 in cur. Was that GRT001 new? `NewFindings` matches
position-free keys `(code, unit, file, message)`. A base GRT001 with the same key would need the
same dependency, output, module and target with the output undeclared in base, which contradicts
the GRT004 precondition "the baseline surface declares the output" for the same module path. So
every superseded GRT001 was new in v0.3.0, and the replacing GRT004 (no GRT004 ever in base) is new
now: Broken subjects, per-subject counts, Impacted and `HasErrors` are unchanged; only codes and
messages at those sites differ. Phase 13 proves this with a rapid property (`Between` vs
`impact.Compute` on the un-superseded sets) rather than relying on the argument.

## 6. Decisions taken here (open in milestone research)

| ID | Decision | Why |
|----|----------|-----|
| D-13-01 | **Baseline side uses rows 1, 4, 5 only** (`resolveDependency`: the base unit has a `dependency` block with that name, it resolves to a unit, whose module and surface are known). Rows 2-3 (`enabled`, `skip_outputs`) are judged in cur only, where the GRT001 fires. | MORE-03 says "both trees resolve the dependency to the same module path, known on both sides"; enabled/skip decide whether Terragrunt reads outputs now, which is already gated by the cur GRT001. Either choice yields exactly one error at the site; this one matches the requirement literally. Pinned by a matrix row (base `enabled = false`, cur default -> GRT004). |
| D-13-02 | Same module path, target unit may differ (dependency re-pointed to another unit of the same module still gives GRT004). | Requirement compares module paths only. Pinned by a row. A re-pointed dependency to a different module stays GRT001. |
| D-13-03 | **No `Result.RemovedOutputs` grouping in Phase 13.** Each site is its own GRT004 in Broken (grouped by unit as today). | MORE-03 asks for per-site findings and "no consumer list"; a new grouped section would change the blast JSON shape, and the schema is bumped once in Phase 14 (BLAST-04). Recorded as a candidate for Phase 14's schema bump, not a requirement. |
| D-13-04 | `Between` returns `(impact.Result, error)`; `Blast` wraps an error as `&Error{Stage: "compare"}` (exit 3 in the CLI). | `RemovedOutputs` builds diagnostics with `diagnostic.NewForUnit`, which returns an error on invalid input (cannot happen for graph-derived values, same as `UnknownOutputs`). |

GRT004 message (repo-relative paths, no list, no position, deterministic; part of `FindingKey`):

```
dependency "<label>" output "<Y>" was removed from module "<module>" (target unit "<target>")
```

plus GRT001's mock suffix (`mockMaskSuffix`) under exactly the same condition (`mockMasksAtApply`
on the cur options and cur surface), so mock facts only pick the suffix, as in GRT001.

## 7. Precondition checks and their killing rows

`RemovedOutputs(base, cur)` evaluates, per cur reference site, an ordered table of named checks
over precomputed facts; GRT004 iff all hold. Each check must be killed by at least one row of the
precondition matrix (`TestRemovedOutputsMatrix`), proven automatically by an internal test that
disables one check at a time (`TestRemovedOutputsChecksAreNecessary`).

| Check | Fact | Killing row (expected GRT001, mutant gives GRT004) |
|-------|------|----------------------------------------------------|
| `fires` | `resolveReference(cur)` ok and cur surface lacks Y (the GRT001 row 7 condition) | output still declared in cur (mutant emits GRT004 where nothing fires) |
| `baseHadRef` | base has a reference with the same (unit, dependency name, output) | reference added in the same change |
| `sameModule` | `resolveDependency(base)` ok and its module path == cur module path | dependency re-pointed to a unit of another module that declares Y in base |
| `baseDeclared` | base surface (known) declares Y | base module never declared Y (pre-existing GRT001) |

Rows that must stay silent in cur (enabled false/unknown, skip_outputs true/unknown, unresolved,
target not a unit, module or surface unknown in cur) are the DIAG-03 rows reused through
`diag03Rows()`; module unknown in base only (syntax error mid-edit) keeps GRT001.

## 8. MORE-07 proof plan

1. Type level: `analyzers` takes one graph (check.go:47); `RemovedOutputs` takes two.
2. Behaviour: an e2e test on a GRT004-shaped tree runs `check` (text/json/sarif), `report` (daemon,
   text/json/sarif), `graph --json` and the status file, asserts no `GRT004` anywhere and the GRT001
   present where expected.
3. Bytes: `git diff --exit-code v0.3.0 --` the check/report/graph/SARIF golden files, and a
   one-off comparison against a v0.3.0 binary built from a `git worktree` of the tag on the golden
   fixtures plus the new GRT004 fixtures (check text/json/sarif and graph --json byte-identical).
