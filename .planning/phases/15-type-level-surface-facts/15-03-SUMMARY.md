---
phase: 15-type-level-surface-facts
plan: 03
subsystem: impact
tags: [domain, blast, type-facts, property-test]
requires: [15-01]
provides:
  - "impact.TypeChange, impact.TypeChangeKind (variable_required, variable_type, output_type, output_sensitive)"
  - "SurfaceChange.TypeChanges, filled by SurfaceDiff; a type-only module is a changed module and seeds propagation"
affects: [15-04, 15-05]
tech-stack:
  added: []
  patterns: ["unknown on either side is silent", "Impacted-only facts proven against a names-only Compute"]
key-files:
  created:
    - internal/domain/impact/typefacts_test.go
  modified:
    - internal/domain/impact/surface.go
key-decisions:
  - "Compute needed no change: it already seeds from SurfaceDiff, and Empty() now counts TypeChanges"
  - "TypeChanges are sorted by Kind (declaration order of the constants), then Name; never nil, also for name-only changes"
  - "The Broken half of the property compares Subject and Findings; Broken Reach is propagation metadata and is compared only in the no-type-only-seed cases, together with the Impacted Reach"
requirements-completed: []
duration: 25min
completed: 2026-10-11
---

# Phase 15 Plan 03: Domain Type-Level Diff Summary

**`SurfaceDiff` now reports type-level changes next to the name changes: a newly required variable, a changed variable type, a changed declared output type, a changed `sensitive`. Any unknown fact on either side is silent. A module whose only change is type-level is a changed module, so it seeds Impacted and propagates exactly like a name change, but it never becomes a diagnostic, a Broken unit or an exit code. A 2,000-case property against a names-only Compute proves it.**

## Performance

- Duration: ~25 min
- Tasks: 2
- Files: 1 created, 1 modified

## Accomplishments

- **surface.go:**
  - `SurfaceChange.TypeChanges`. The doc comment states the Impacted-only contract and the Pitfall 4 reason.
  - `Empty()` (:32) now includes TypeChanges.
  - `TypeChangeKind` (:40), its constants and `String()`, plus `TypeChange` (:76).
  - `SurfaceDiff` fills TypeChanges (:113) from `typeChanges` (:133):
    - a new variable reports only that it is Required True (`absent -> required`);
    - an existing variable reports `optional -> required` and a type change only when both types are known (:147);
    - an output type is compared only when declared and known on both sides (:157), so absent is never `any`;
    - sensitive is compared only when known on both sides (:160, `knownBool` :177);
    - the result is sorted by kind, then name (:166).
- **typefacts_test.go:**
  - TestTypeChangeKindString (:48).
  - TestTypeChangeMatrix (:61) has 22 rows, one per rule and per silent case: new variable required, optional or unknown; required↔optional; unknown on either side; type change; `T -> any` (D-15-04); an unknown or equal type; an output type newly declared or dropped (silent); an output type change; sensitive both ways and unknown; removed or new names giving a name change only.
  - TestTypeChangeOrder (:133): 9 changes over 4 kinds, stable across 50 xorshift shuffles of the decl input.
  - TestSurfaceDiffTypeOnlyModule (:167).
  - TestSurfaceDiffNamesOnlyUnchanged (:205): 2,000 xorshift name-only pairs built with `NewSurface` match a reference rebuild of the pre-phase name diff, with TypeChanges empty and non-nil.
  - TestComputeTypeOnlySeeds (:232): a type-only module gives unit a at distance 1 with the TypeChanges and b at distance 2 through a, with nothing Broken.
  - TestTypeFactsNeverBroken (:258): a newly required variable the unit does not set leaves Broken empty and HasErrors false; with an unrelated GRT001, Broken holds exactly that finding.
  - TestTypeFactsNeverChangeBrokenProperty (:343), sec #247. Each of 2,000 xorshift cases has 1-4 modules, 2-8 units (some config-unknown), random dependency edges, and random name and fact changes plus random base and current diagnostics. Every case is computed twice, with facts (`NewSurfaceFacts`) and with names only (`NewSurface`). The checks:
    - Broken subjects and findings, and HasErrors, are identical;
    - Impacted with facts is a superset of Impacted with names only;
    - Distance with facts ≤ Distance with names only, for every common unit;
    - full Reach equality (Impacted and Broken) holds whenever the facts add no type-only seed.

    The run logged: facts added Impacted units in 1,156 cases and shortened a distance in 183; 1,444 cases had a type-only seed and 556 had equal seeds. All counters must be > 0.
- **Mutation spot-checks (each reverted), all caught:**
  - an unknown variable type compared as known
  - an unknown output type compared as known
  - unknown required treated as optional
  - Empty() ignoring TypeChanges
  - sort disabled
  - a new variable with unknown Required reported as required

## Task Commits

1. `af6e399` test(15-03): type-level change matrix, order, type-only seeds and the Broken invariant property
2. `f17981a` feat(15-03): type-level changes in SurfaceDiff, Impacted-only

This summary is its own `docs(15-03): summary` commit.

## Verification

```
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	15.198s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.497s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.004s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.189s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	1.178s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.018s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	4.084s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.053s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.060s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	3.355s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	4.676s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.101s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.052s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.043s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.022s
VET-OK
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
```

```
bash scripts/compare-ref.sh v0.3.0
244/244 identical
git diff b2598c5 -- cmd/gruntled/testdata     # empty (0 lines)
```

The domain imports only `slices` and `strings` (stdlib, allowlisted), and the tests use only xorshift (no rapid, no math/rand). `-race` was not run locally (no gcc); CI runs it.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-15-03-1 | mitigated | facts live only in SurfaceChange (surface.go:113/:133); Broken and HasErrors come only from diagnostics (impact.go Compute, unchanged); TestTypeFactsNeverBroken typefacts_test.go:258 and the property at :343 |
| T-15-03-2 | mitigated | unknown on either side is silent (surface.go:147/:157/:160 and knownBool :177); a new variable needs Required True; output type only when declared on both sides; matrix rows at typefacts_test.go:61 |
| T-15-03-3 | mitigated | sort by kind, then name (surface.go:166); TestTypeChangeOrder :133 over 50 shuffles |
| T-15-03-4 | mitigated | TypeChanges is a slice in the module's SurfaceChange; ImpactedUnit.Change is copied by value and shares the backing array, so per-unit copies are a slice header, not the strings; the presenter renders them once per module in 15-04 |

## Deviations from Plan

**1. [Test placement] Task 2 tests are in typefacts_test.go, not impact_test.go**
- They reuse typefacts_test.go's fact helpers. impact_test.go is unchanged, and so are the Phase 14 oracle tests and TestCompute (all green).
- Tasks 1 and 2 share one red commit, because Task 2 needed no production change.

**2. [Property detail] "Identical Broken" compares Subject and Findings**
- BrokenUnit.Reach is propagation metadata: with a type-only seed it can legitimately become shorter or appear. It is checked for exact equality in the cases without a type-only seed, exactly as Impacted Reach is (sec #247).

## Known Risks / Unverified

- **Interim state until 15-04:** the 15-02 reader feeds real facts. A module whose only change is type-level now makes its units Impacted in `report --blast` output, but the presenter does not yet print the type reasons or a `Type changes` section. No existing golden has a type-only change (compare-ref 244/244, testdata unchanged); 15-04 adds the tokens and the section.
- `-race` was not run locally.

## Self-Check: PASSED
