---
phase: 01-domain-foundation-test-substrate
plan: 01
subsystem: domain
tags: [go, ddd, value-objects, aggregate-root, stdlib-only]

# Dependency graph
requires: []
provides:
  - "github.com/GiulioSavini/gruntled Go module (go 1.27, zero third-party dependencies)"
  - "internal/domain/repograph: RepoPath, Position, Surface, Module, Dependency, Reference, Unit value objects + RepositoryGraph aggregate root with pure queries (Units, Modules, ModuleOf, DependencyTarget, References)"
  - "internal/domain/diagnostic: Code/ParseCode, Severity, Diagnostic, Key, Set with pure Diff"
  - "cmd/gruntled/main.go composition-root stub"
affects: [01-02, 02-parser, 03-cli]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Aggregate root sorts and defensively copies at construction (RepositoryGraph.NewRepositoryGraph), so any input order yields an identical graph"
    - "Value objects carry unexported fields so only exported constructors can produce a valid instance; every returned slice is a sorted slices.Clone"
    - "Constructor error messages start with the package name (e.g. `repograph: invalid repo path ...`)"
    - "Domain error types are structs with Error() (DuplicateUnitError, DuplicateModuleError, MissingModuleError), checked in tests via errors.As"
    - "Pure graph-query proof pattern for ARCH-02: a test-local function chains only exported queries (References -> DependencyTarget -> ModuleOf -> Surface.HasOutput) over hand-built objects, with no analyzer package yet"

key-files:
  created:
    - go.mod
    - cmd/gruntled/main.go
    - internal/domain/repograph/path.go
    - internal/domain/repograph/position.go
    - internal/domain/repograph/surface.go
    - internal/domain/repograph/module.go
    - internal/domain/repograph/unit.go
    - internal/domain/repograph/graph.go
    - internal/domain/repograph/valueobjects_test.go
    - internal/domain/repograph/graph_test.go
    - internal/domain/diagnostic/diagnostic.go
    - internal/domain/diagnostic/set.go
    - internal/domain/diagnostic/diagnostic_test.go
    - internal/domain/diagnostic/set_test.go
  modified: []

key-decisions:
  - "go.mod declares go 1.27; the go1.27.0 toolchain was already cached in GOMODCACHE alongside go1.26.8, so GOTOOLCHAIN=auto switched with zero network calls (verified with go version)"
  - "Unit.Dependency(name) lookup uses a linear scan over the sorted []Dependency slice rather than a map, keeping the type simple while still respecting the never-range-a-map-for-output rule (there's no map at all)"
  - "diagnostic.Set.Diff builds two map[Key]struct{} for O(1) membership lookup only, then filters the already-Compare-sorted prev/next items in place, so results stay in canonical order without a second sort"

requirements-completed: [ARCH-02]

# Metrics
duration: ~35min
completed: 2026-09-25
---

# Phase 1 Plan 01: Domain Foundation Summary

**Bootstrapped the Go module and built two stdlib-only domain packages — `repograph` (value objects + `RepositoryGraph` aggregate root) and `diagnostic` (`Diagnostic` + `Set` with pure `Diff`) — both fully covered by hand-built-object unit tests, zero filesystem or third-party imports.**

## Performance

- **Duration:** ~35 min
- **Completed:** 2026-09-25T10:16:30Z
- **Tasks:** 3/3 completed
- **Files modified:** 14 created (4 package files bootstrap, 4 repograph source, 2 repograph test, 2 diagnostic source, 2 diagnostic test)

## Accomplishments
- `go.mod` at `github.com/GiulioSavini/gruntled`, `go 1.27`, no `require` block — confirmed `go version` resolves to `go1.27.0` via the cached toolchain with no network access
- `internal/domain/repograph`: `RepoPath`, `Position`, `Surface`, `Module`, `Dependency`, `Reference`, `UnitStatus`, `Unit` value objects, all immutable with validated construction and defensive-copy accessors
- `RepositoryGraph` aggregate root: sorts/deduplicates units and modules at construction, validates that every resolved unit's module exists, and exposes pure queries (`Unit`, `Module`, `ModuleOf`, `DependencyTarget`, `References`) that are order-independent by construction
- `internal/domain/diagnostic`: `Code`/`ParseCode` (hand-written "GRT" + 3-digit validation, no regexp), `Severity`, `Diagnostic`, stable `Key`, `Compare`, and `Set` with dedup-by-`Key` canonical ordering and a pure `Diff`
- ARCH-02 proof: a test-local `unknownOutputRefs` helper in `graph_test.go` finds a bad `dependency.X.outputs.Y` reference using only exported graph queries over a hand-built 3-unit graph (app -> vpc good output, app -> vpc bad output, app -> legacy Unknown unit skipped)
- Verified: no file under `internal/domain` directly imports `os`, `io/fs`, `io/ioutil`, or `path/filepath`; `go list -deps` on `internal/domain/...` resolves to only the two intra-module domain packages

## Task Commits

Each task was committed atomically:

1. **Task 1: Bootstrap module and repograph value objects** - `7565321` (feat)
2. **Task 2: RepositoryGraph aggregate root with pure queries** - `294f2fa` (feat)
3. **Task 3: Diagnostic value object and Set with pure Diff** - `bbf499a` (feat)

_All three tasks followed the TDD flow (tests written and confirmed to fail on missing symbols, then implementation, then tests passing) but were committed as a single `feat` commit per task rather than separate RED/GREEN commits, since the plan's own commit instructions specified one commit per task._

## Files Created/Modified
- `go.mod` - module declaration, go 1.27, no third-party deps
- `cmd/gruntled/main.go` - composition-root stub (prints to stderr, exits 1)
- `internal/domain/repograph/path.go` - `RepoPath` value object
- `internal/domain/repograph/position.go` - `Position` value object
- `internal/domain/repograph/surface.go` - `Surface` value object
- `internal/domain/repograph/module.go` - `Module` value object
- `internal/domain/repograph/unit.go` - `Dependency`, `Reference`, `UnitStatus`, `Unit`
- `internal/domain/repograph/graph.go` - `RepositoryGraph` aggregate root, `UnitReference`, error types
- `internal/domain/repograph/valueobjects_test.go` - table-driven tests for all value objects
- `internal/domain/repograph/graph_test.go` - graph construction, queries, ARCH-02 proof
- `internal/domain/diagnostic/diagnostic.go` - `Code`, `Severity`, `Diagnostic`, `Key`, `Compare`
- `internal/domain/diagnostic/set.go` - `Set`, `NewSet`, `Diff`
- `internal/domain/diagnostic/diagnostic_test.go` - `Diagnostic`/`Code`/`Severity` tests
- `internal/domain/diagnostic/set_test.go` - `Set` ordering, dedup, `Diff` tests

## Decisions Made
- go.mod's `go 1.27` directive is honored by the local toolchain's `GOTOOLCHAIN=auto` default with zero network calls (both go1.26.8 and go1.27.0 were already present in `GOMODCACHE`)
- Kept per-task commits as single `feat` commits (test file + implementation together) rather than splitting into separate RED/GREEN commits, matching this plan's explicit action step ("Commit with `feat(01-01): ...`" — one commit per task, not per TDD phase)

## Deviations from Plan

**1. [Process deviation, not a Rule 1-4 fix] Did not push after each task commit**

- **Found during:** Task 1, after the first commit
- **Issue:** The plan's own action text says "After each task commit, run `git push origin master`." The orchestrator invocation for this executor explicitly instructs "Do not push; the orchestrator handles pushing." These two instructions conflict.
- **Resolution:** Pushed once after Task 1 (before noticing the conflict), then stopped pushing for Tasks 2 and 3, deferring to the more specific/current orchestrator instruction that the orchestrator handles pushing.
- **Current state:** `origin/master` is in sync through commit `7565321` (Task 1) but is 2 commits behind local `master` (`294f2fa`, `bbf499a`) as of this SUMMARY. The orchestrator should push the remaining commits.

No Rule 1-4 auto-fixes were needed — the plan's `<interfaces>` block was precise enough (exact type/method signatures) that implementation matched it directly, and all `<behavior>` bullets were satisfied by the first working implementation.

## Issues Encountered
- `go vet` initially failed on `graph_test.go` because `Unit` (which contains slice fields) is not comparable with `!=`; fixed by comparing `Unit.Path()` instead of the `Unit` value itself in the defensive-copy test. This is a normal TDD RED-phase correction within the same task, not a plan deviation.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- `internal/domain/repograph` and `internal/domain/diagnostic` are complete, tested, and stdlib-only; Phase 1 Plan 02 (synthetic repository generator, per ROADMAP) and later phases (HCL parser, analyzers, CLI) can build on these types without further domain changes expected.
- Remaining local commits (`294f2fa`, `bbf499a`) need to be pushed to `origin/master` by the orchestrator.
- No blockers identified for the next plan in this phase.

---
*Phase: 01-domain-foundation-test-substrate*
*Completed: 2026-09-25*

## Self-Check: PASSED

All 14 created source/test files and the SUMMARY.md itself were verified present on disk; all 3 task commit hashes (`7565321`, `294f2fa`, `bbf499a`) were verified present in `git log --oneline --all`.
