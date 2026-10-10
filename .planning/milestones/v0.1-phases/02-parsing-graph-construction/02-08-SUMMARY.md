---
phase: 02-parsing-graph-construction
plan: 08
subsystem: domain
tags: [go, domain-invariants, repograph, value-objects]

# Dependency graph
requires:
  - phase: 02-parsing-graph-construction
    provides: internal/domain/repograph (Position, Tristate, Dependency, Reference, Unit, RepositoryGraph)
provides:
  - Position.IsZero query method
  - Tristate.IsValid and UnitStatus.IsValid query methods
  - NewDependency/NewUnresolvedDependency/NewReference reject a zero Position and any invalid Tristate option
  - Unit constructors (NewResolvedUnit, NewModuleUnknownUnit) reject a zero-value Dependency/Reference entry instead of silently keeping it
  - NewRepositoryGraph rejects a zero-value Unit or Module in its input, with typed InvalidUnitError/InvalidModuleError carrying the input index
affects: [03-analysis-diagnostics]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Shared validation helper (validateDependencyCommon) for two constructors that share most of their checks"
    - "IsZero/IsValid query methods on value objects, checked at every constructor boundary that accepts them as input, so a forged zero/invalid value can never enter the aggregate"
    - "Typed graph errors carry the original (pre-sort) input index so a test or caller can trace the error back to its slice position"

key-files:
  created: []
  modified:
    - internal/domain/repograph/position.go
    - internal/domain/repograph/options.go
    - internal/domain/repograph/unit.go
    - internal/domain/repograph/graph.go
    - internal/domain/repograph/valueobjects_test.go
    - internal/domain/repograph/options_test.go
    - internal/domain/repograph/graph_test.go

key-decisions:
  - "UnitStatus.IsValid landed in the same commit as the Dependency/Reference validation (task 1) rather than task 2's commit, since NewRepositoryGraph's own fix (task 2) needed it and it is a small, independent query next to the other IsValid/IsZero additions on the same file."
  - "validateDependencyCommon is unexported and shared by NewDependency and NewUnresolvedDependency only; NewReference keeps its own body since it does not share the Tristate option checks."
  - "sortAndValidateDepsRefs checks for a zero-value Dependency/Reference (empty name/dependency field) in the original, unsorted order, before cloning or sorting, so the error message's index matches the caller's input slice."
  - "NewRepositoryGraph's new zero-value checks run before sorting, over the original units/modules slices, for the same input-index-traceability reason."

patterns-established:
  - "Every repograph value type that flows into a constructor as an argument now has an IsZero (Position) or IsValid (Tristate, UnitStatus) query, and every constructor that accepts one checks it before building anything: no zero/invalid value can silently propagate into the aggregate."

requirements-completed: [GRAPH-04, PARSE-04]

duration: 6min
completed: 2026-09-28
---

# Phase 2 Plan 08: Repograph zero-value and invalid-option rejection Summary

**Closed gap G12: `Position.IsZero`, `Tristate.IsValid` and `UnitStatus.IsValid` now back every repograph constructor, so a zero Position, a forged `Tristate(99)`, a zero-value `Dependency{}`/`Reference{}` inside a unit, or a zero `Unit{}`/`Module{}` inside the graph are all rejected instead of silently accepted.**

## Performance

- **Duration:** 6 min (from first failing-test commit to last fix commit; total wall time for the plan including reading, review, and full verification was longer)
- **Started:** 2026-09-28T09:49:45+02:00
- **Completed:** 2026-09-28T09:55:10+02:00
- **Tasks:** 2
- **Files modified:** 7

## Accomplishments
- `Position.IsZero()`, `Tristate.IsValid()` and `UnitStatus.IsValid()` — three small query methods that let every constructor refuse an input it cannot represent honestly.
- `NewDependency`, `NewUnresolvedDependency` and `NewReference` all reject a zero `Position`; the two dependency constructors additionally reject any of `Enabled`/`SkipOutputs`/`MockMergeWithState` set to a value outside the three defined `Tristate` constants (via a new shared `validateDependencyCommon` helper).
- `sortAndValidateDepsRefs` (shared by `NewResolvedUnit` and `NewModuleUnknownUnit`) now rejects a zero-value `Dependency` or `Reference` entry — including a valid entry mixed with a zero one — instead of letting it through unnamed.
- `NewRepositoryGraph` rejects a zero-value `Unit` (zero path or an invalid `UnitStatus`) or `Module` (zero path, or an unknown module with an empty reason) anywhere in its input, before sorting, via two new typed errors: `InvalidUnitError{Index}` and `InvalidModuleError{Index}`, where `Index` is the position in the caller's original (unsorted) slice.
- Confirmed via `go test -count=1 ./...` (whole module) and the env-gated `TestCorpusSmoke` that no existing production caller (terragrunt loader, indexing use case, hclconv) or test helper builds a zero `Position` or an invalid option — every one already goes through `repograph.NewPosition`, the `Tristate` constants, or `DependencyOptions{}` (all-Unknown, still valid).

## Task Commits

Each task was committed atomically (test-first / TDD):

1. **Task 1: Value objects reject zero positions, invalid options and zero entries**
   - `293612b` test(02-08): add failing zero-value and invalid-option domain cases
   - `e9d9f8e` fix(02-08): reject zero positions, invalid options and zero entries in repograph
2. **Task 2: NewRepositoryGraph rejects zero units and zero modules**
   - `6b942d7` test(02-08): add failing zero unit/module graph cases
   - `589c035` fix(02-08): reject zero-value units and modules in NewRepositoryGraph

This plan runs in a parallel worktree; the orchestrator merges and creates the plan-metadata commit after merge, so there is no separate `docs(02-08): complete ...` commit here.

## Files Created/Modified
- `internal/domain/repograph/position.go` — added `Position.IsZero()`.
- `internal/domain/repograph/options.go` — added `Tristate.IsValid()`.
- `internal/domain/repograph/unit.go` — added `validateDependencyCommon` (shared by `NewDependency`/`NewUnresolvedDependency`), a zero-position check in `NewReference`, `UnitStatus.IsValid()`, and zero-entry rejection in `sortAndValidateDepsRefs`; updated the doc comments of all five affected constructors.
- `internal/domain/repograph/graph.go` — added `InvalidUnitError`/`InvalidModuleError` and the pre-sort validation loop in `NewRepositoryGraph`; updated its doc comment.
- `internal/domain/repograph/valueobjects_test.go` — `TestPositionIsZero`, `TestNewDependencyRejectsZeroPosition`, `TestNewDependencyRejectsInvalidOptions`, `TestUnitConstructorsRejectZeroEntries`.
- `internal/domain/repograph/options_test.go` — `TestTristateIsValid`.
- `internal/domain/repograph/graph_test.go` — `TestUnitStatusIsValid`, `TestNewRepositoryGraph_ZeroUnit`, `TestNewRepositoryGraph_ZeroModule`.

## For Phase 3 (analyzer tests that build graphs by hand)
- New query methods: `repograph.Position.IsZero() bool`, `repograph.Tristate.IsValid() bool`, `repograph.UnitStatus.IsValid() bool`.
- New typed errors from `repograph.NewRepositoryGraph`: `*repograph.InvalidUnitError{Index int}` and `*repograph.InvalidModuleError{Index int}` (use `errors.As`), alongside the existing `*DuplicateUnitError`, `*DuplicateModuleError`, `*MissingModuleError`.
- No exported signature changed: `NewDependency`, `NewUnresolvedDependency`, `NewReference`, `NewResolvedUnit`, `NewModuleUnknownUnit`, `NewRepositoryGraph` all keep their existing parameter lists; they simply reject more inputs than before.
- A hand-built graph fixture must now use `repograph.NewPosition(...)` (never `repograph.Position{}`), the `Tristate` constants or `DefaultDependencyOptions()`/`DependencyOptions{}` (never a raw `Tristate(N)` conversion), and must never append a bare `Dependency{}`/`Reference{}`/`Unit{}`/`Module{}` to a slice passed into a constructor.

## Decisions Made
- Placed `UnitStatus.IsValid` in task 1's fix commit (alongside `Position.IsZero`/`Tristate.IsValid`) rather than task 2's, since it is a small, independent query on the same file (`unit.go`) and task 2's `NewRepositoryGraph` fix depends on it. Documented in the commit message to avoid confusion when reading history.
- Kept `validateDependencyCommon` unexported and scoped to the two dependency constructors only; `NewReference` got its own inline zero-position check since it doesn't share the `Tristate` option checks.
- `sortAndValidateDepsRefs` and `NewRepositoryGraph` both validate over the *original, unsorted* input order (not the sorted internal representation), so a reported index always matches the caller's own slice — needed for the `InvalidUnitError{Index}`/`InvalidModuleError{Index}` contract and matches the plan's must_haves.

## Deviations from Plan

None - plan executed exactly as written. All five constructors and the two graph checks match the plan's must_haves and key_links exactly (`validateDependencyCommon` matches the `IsValid\(\)` key-link pattern; the `NewRepositoryGraph` pre-sort loop matches the `Status\(\)\.IsValid\(\)` key-link pattern).

## Issues Encountered
None. `go test -count=1 ./...` was green on the first attempt after each GREEN step, confirming the plan's safety analysis (every production caller already used `NewPosition`, the `Tristate` constants, or `DependencyOptions{}`) was correct.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- `internal/domain/repograph` now rejects every zero/invalid value the plan identified (G12 closed). Phase 3's analyzer and its hand-built graph test fixtures can rely on `NewRepositoryGraph`/unit/dependency constructors failing loudly on a forged input rather than propagating a `:0:0` position or a phantom option value into a diagnostic.
- No blockers. This plan touched only `internal/domain/repograph`; it does not depend on and was not blocked by the sibling gap-closure plans (02-06, 02-07, 02-09, 02-10) running in parallel.

---
*Phase: 02-parsing-graph-construction*
*Completed: 2026-09-28*

## Self-Check: PASSED

All 7 modified source/test files and this SUMMARY.md verified present on disk; all 4 task commits (`293612b`, `e9d9f8e`, `6b942d7`, `589c035`) verified present in `git log`.
