---
phase: 02-parsing-graph-construction
plan: 01
subsystem: domain
tags: [go, ddd, value-objects, terragrunt]

# Dependency graph
requires:
  - phase: 01-domain-foundation-test-substrate
    provides: repograph value objects (RepoPath, Position, Surface), diagnostic package, synthrepo test substrate
provides:
  - "config-unknown / module-unknown unit split (StatusResolved / StatusModuleUnknown / StatusConfigUnknown)"
  - "per-dependency unresolved state (NewUnresolvedDependency, Target() (RepoPath, bool))"
  - "DependencyOptions/Tristate/NameList carrying DIAG-03 facts (enabled, skip_outputs, mock_outputs, mock_outputs_merge_with_state, mock_outputs_allowed_terraform_commands)"
  - "Module surface-unknown state (NewUnknownModule, Surface() (Surface, bool))"
  - "diagnostic identity including Unit (Key.Unit, NewForUnit, Diagnostic.Unit()); Severity documented and tested as excluded from Key"
affects: [02-02, 02-03, 02-04, 02-05, phase-3-analysis]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Fail-safe zero values: Tristate and NameList zero to Unknown, not to a false/absent literal, so a caller that forgets to set a DependencyOptions field gets 'not literal' rather than a false default"
    - "Shared unexported validation helper (sortAndValidateDepsRefs) between NewResolvedUnit and NewModuleUnknownUnit keeps duplicate-dependency-name rejection and sort order identical without duplicating logic (staticcheck-clean, no dead code)"

key-files:
  created:
    - internal/domain/repograph/options.go
    - internal/domain/repograph/options_test.go
  modified:
    - internal/domain/repograph/unit.go
    - internal/domain/repograph/module.go
    - internal/domain/repograph/graph.go
    - internal/domain/repograph/graph_test.go
    - internal/domain/repograph/valueobjects_test.go
    - internal/domain/diagnostic/diagnostic.go
    - internal/domain/diagnostic/set.go
    - internal/domain/diagnostic/diagnostic_test.go
    - internal/domain/diagnostic/set_test.go

key-decisions:
  - "Dependency.Target() changed from RepoPath to (RepoPath, bool) so every caller must handle the unresolved case explicitly; there is no zero-value RepoPath that could be mistaken for a real target"
  - "Module.Surface() changed from Surface to (Surface, bool) for the same reason: a module whose local directory is missing/unparsable stays in the graph as 'target exists, surface unknown' instead of being dropped"
  - "NewRepositoryGraph's module-presence check already worked unchanged for the new StatusModuleUnknown status, because Unit.Module() returns false for any non-resolved status; no new branch needed"
  - "diagnostic.Key keeps Severity out on purpose (documented and tested via TestDiffIgnoresSeverityOnlyChange) and adds Unit (documented and tested via TestNewSetKeepsBothUnitsWhenOnlyUnitDiffers, TestDiffOnlyUnitChangeIsOneAddedOneRemoved, TestCompareOrdersByUnitZeroFirst)"

patterns-established:
  - "Pattern: unresolved/unknown states carry a mandatory non-empty reason string (kebab-case, e.g. 'remote-source', 'config-path-dynamic') validated by the constructor, never a bare bool"

requirements-completed: [GRAPH-03, GRAPH-04, PARSE-04]

# Metrics
duration: 25min
completed: 2026-09-25
---

# Phase 2 Plan 1: Domain unknown-state model Summary

**Extended `internal/domain/repograph` and `internal/domain/diagnostic` so every GRT001 fact (config-unknown vs module-unknown units, unresolved dependencies, tri-state dependency options, unknown-surface modules, unit-scoped diagnostic identity) has a home before any HCL parser exists.**

## Performance

- **Duration:** 25 min
- **Started:** 2026-09-25T11:06:00Z
- **Completed:** 2026-09-25T11:31:29Z
- **Tasks:** 2
- **Files modified:** 9 (2 new, 7 edited)

## Accomplishments
- Split `UnitStatus` into `StatusResolved` / `StatusModuleUnknown` / `StatusConfigUnknown`: a unit whose module is unknown (remote/dynamic source, unreadable module dir) keeps its dependencies and references, while a unit whose own config is broken/dynamic carries none — matching the architecture review's finding that the old single `StatusUnknown` forced information loss.
- Dependencies can now be unresolved (`NewUnresolvedDependency`) instead of dropped when `config_path` is dynamic or escapes the repo; `Target()` returns `(RepoPath, bool)` so callers can no longer mistake an unresolved dependency for a resolved zero path.
- Added `DependencyOptions`/`Tristate`/`NameList` in a new `options.go`: every DIAG-03 fact (`enabled`, `skip_outputs`, `mock_outputs`, `mock_outputs_merge_with_state`, `mock_outputs_allowed_terraform_commands`) is captured as a tri-state with a fail-safe zero value (`TristateUnknown`, `NameList` zero-value `IsUnknown()`), plus `DefaultDependencyOptions()` for Terragrunt's own absent-attribute defaults.
- Added `NewUnknownModule` / `Module.Surface() (Surface, bool)` so a module whose local directory is missing, empty, or unparsable stays in the graph instead of the referencing unit being dropped.
- `diagnostic.Key` now includes `Unit` (a shared include's reference is evaluated once per including unit, so two units can resolve `dependency.vpc` to different targets and must not collapse into one diagnostic) while deliberately excluding `Severity` (a severity-only change is not a new finding) — both documented on `Key`/`Diff` and covered by dedicated tests.

## Task Commits

Each task was committed atomically:

1. **Task 1: Dependency options, unresolved dependencies, unit and module unknown states** - `e5c8b57` (feat)
2. **Task 2: Diagnostic identity includes the Unit; Severity stays out of the Key on purpose** - `93ae52e` (feat)

**Plan metadata:** (this commit) `docs(02-01): complete domain unknown-state model plan`

## Files Created/Modified
- `internal/domain/repograph/options.go` - `Tristate`, `NameList`, `DependencyOptions`, `DefaultDependencyOptions`
- `internal/domain/repograph/options_test.go` - table tests for Tristate/NameList/DependencyOptions zero-value and default behavior
- `internal/domain/repograph/unit.go` - `Dependency` resolved/unresolved split, `DependencyOptions` plumbed through, three-way `UnitStatus`, `NewModuleUnknownUnit`/`NewConfigUnknownUnit` replacing `NewUnknownUnit`, shared `sortAndValidateDepsRefs` helper
- `internal/domain/repograph/module.go` - `Module` known/unknown split, `NewUnknownModule`, `Surface() (Surface, bool)`
- `internal/domain/repograph/graph.go` - `DependencyTarget` and doc comments updated for the new `Target()`/`Surface()` two-value contracts
- `internal/domain/repograph/graph_test.go`, `internal/domain/repograph/valueobjects_test.go` - updated to new signatures; added cycle/self-dependency, unknown-surface-module, module-unknown-needs-no-module tests
- `internal/domain/diagnostic/diagnostic.go` - `Diagnostic.unit`, `NewForUnit`, `Unit()`, `Key.Unit`, `Compare` ordering by Unit
- `internal/domain/diagnostic/set.go` - doc comments updated to describe Unit-aware dedup/diff
- `internal/domain/diagnostic/diagnostic_test.go`, `internal/domain/diagnostic/set_test.go` - `NewForUnit` tests, unit-distinct dedup/diff tests, severity-only-diff-is-empty test, Compare permutation-stability test

## Decisions Made
- `Dependency.Target()` and `Module.Surface()` both became two-value returns rather than adding separate `IsResolved()`/`IsKnown()` predicate methods — forces every call site (including future Phase 3 analyzer code) to handle the unknown case at the point of use.
- Kept `NewRepositoryGraph`'s existing module-presence logic unchanged: it already calls `Unit.Module()`, which now returns `false` for both `StatusModuleUnknown` and `StatusConfigUnknown`, so no new branching was needed to satisfy "module-unknown unit needs no module in the graph."
- `KnownNames(nil)` returns a Known, empty list (distinct from `AbsentNames()`), matching the plan's explicit `mock_outputs = {}` requirement.

## Deviations from Plan

None - plan executed exactly as written. All `<behavior>` bullets from both tasks have a corresponding table test; `NewUnknownUnit`/`StatusUnknown` are fully removed (verified by `grep -rn "NewUnknownUnit\|StatusUnknown\b" internal/`).

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- The domain model now has a home for every fact Plans 02-02 through 02-05 (HCL parsing, includes, walk, graph assembly) need to write into: config-unknown vs module-unknown units, unresolved dependencies with reasons, tri-state dependency options, and unknown-surface modules.
- `diagnostic.NewForUnit`/`Key.Unit` are ready for Phase 3's GRT001 analyzer to attribute findings per including unit.
- No blockers. `internal/testsupport/synthrepo` was confirmed to use none of the changed APIs (only `NewPosition`, `MustRepoPath`, `diagnostic.CodeUnknownOutput`) and needed no changes; full suite (build, vet, test, gofmt, `go mod tidy -diff`, staticcheck, arch check + self-test) and CI (`check` + `architecture` jobs, including `-race`) are green on `master` at `93ae52e`.

---
*Phase: 02-parsing-graph-construction*
*Completed: 2026-09-25*

## Self-Check: PASSED

All created/modified files and both task commit hashes (e5c8b57, 93ae52e) verified present.
