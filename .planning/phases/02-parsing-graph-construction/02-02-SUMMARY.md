---
phase: 02-parsing-graph-construction
plan: 02
subsystem: application
tags: [go, hexagonal, ports-and-adapters, terragrunt, architecture-enforcement]

# Dependency graph
requires:
  - phase: 02-parsing-graph-construction (plan 01)
    provides: config-unknown/module-unknown unit split, unresolved dependencies, DependencyOptions tri-states, unknown-surface modules, diagnostic.Key.Unit
provides:
  - "internal/application/ports: UnitLoader/LoadResult/UnitConfig and SurfaceReader/SurfaceResult — the two driven ports Plans 03-05 implement"
  - "internal/application/indexing.Build: loads units, reads each distinct resolved module's surface exactly once, assembles a RepositoryGraph, tested entirely with fakes (no filesystem)"
  - "5 new arch rules (application-non-vacuous-guard, application-stdlib-allowlist, application-platform-neutral, application-external-deps, hcl-only-in-infrastructure) each proven by a dedicated self-test case, landed before any internal/infrastructure package exists"
affects: [02-03, 02-04, 02-05, phase-3-analysis]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Assemble units before reading any module surface: a UnitConfig the domain constructors would reject (e.g. a 'resolved' unit with a zero Module) fails at the assemble stage without ever calling ReadSurface, since a malformed unit's 'module' isn't meaningful to read in the first place"
    - "Shared bash functions (check_stdlib_allowlist, check_platform_neutral, check_external_deps) parameterized by rule name and path, reused by both domain and application arch rules, each caller keeping its own exact '=== RULE FAILED: <name> ===' line so self-tests stay rule-specific"
    - "Non-vacuous guards run before the compile gate, not after: internal/application imports internal/domain, so a self-test that empties internal/domain wholesale would otherwise trip compile-gate (a real, unrelated build error) instead of the non-vacuous-guard it exists to prove"

key-files:
  created:
    - internal/application/ports/ports.go
    - internal/application/indexing/build.go
    - internal/application/indexing/build_test.go
  modified:
    - scripts/check-architecture.sh
    - scripts/test-check-architecture.sh

key-decisions:
  - "Reordered check-architecture.sh: non-vacuous guards (domain and application) now run before the compile gate, not after. go vet ./internal/domain/... ./internal/application/... ./cmd/... genuinely fails to compile when internal/domain is emptied (internal/application imports it), which would otherwise hide the non-vacuous-guard failure behind an unrelated compile-gate failure and break the pre-existing 'vacuous' self-test case."
  - "check_stdlib_allowlist/check_platform_neutral/check_external_deps factored into bash functions shared by domain and application rules; only the RULE FAILED header line is asserted by self-tests, so message text was free to change without breaking the 13 pre-existing cases (verified empirically, not assumed)."
  - "hcl-only-in-infrastructure uses go list -e (not plain go list) specifically so an unrelated broken package under set -e cannot abort the script before this rule's own line is printed, per the plan's stated fact."
  - "indexing.Build assembles every unit from its UnitConfig BEFORE reading any module surface, reversing the naive 'read surfaces then assemble' order: this makes a resolved DTO with a zero Module fail at the assemble stage (matching the domain constructor's own rejection) instead of first calling ReadSurface with a nonsensical zero-value module path."

patterns-established:
  - "Pattern: fakeLoader/fakeSurfaces test doubles that record every call and fail the test (via a stored *testing.T) on an unmapped input, rather than silently returning a zero value — makes a forgotten test fixture a hard test failure, not a mysteriously wrong assertion"

requirements-completed: [GRAPH-04, PARSE-04]

# Metrics
duration: 24min
completed: 2026-09-25
---

# Phase 2 Plan 2: Application ports, indexing.Build, and the application/HCL layering rules Summary

**`internal/application/{ports,indexing}` (UnitLoader/SurfaceReader ports plus a fully fake-tested `indexing.Build` use case) landed alongside 5 new CI architecture rules — including `hcl-only-in-infrastructure` — so the Phase 3 adapters are policed from their first commit, before any of them exist.**

## Performance

- **Duration:** 24 min
- **Started:** 2026-09-25T13:33:09+02:00
- **Completed:** 2026-09-25T13:56:54+02:00
- **Tasks:** 3
- **Files modified:** 5 (3 new, 2 edited)

## Accomplishments
- `internal/application/ports` declares `UnitLoader`/`LoadResult`/`UnitConfig` and `SurfaceReader`/`SurfaceResult` exactly per the plan's TARGET API, importing only `context` and the two domain packages — verified by an exact `go list -f '{{join .Imports}}'` string match, not just "it compiles".
- `internal/application/indexing.Build` turns a `UnitLoader` + `SurfaceReader` pair into a `repograph.RepositoryGraph`: config-unknown units carry no deps/refs regardless of what the DTO holds, module-unknown units keep them, each distinct resolved module's surface is read exactly once in sorted order, and every uncertain DTO state (unresolved dependency, dependency whose target isn't a discovered unit, unknown module surface) maps to the matching unknown graph state rather than a resolved one. All 13 hand-written tests run entirely against fakes — no filesystem, no HCL.
- 5 new architecture rules extend `scripts/check-architecture.sh`: `application-non-vacuous-guard`, `application-stdlib-allowlist` (domain allowlist + `context`, no `fmt`), `application-platform-neutral`, `application-external-deps` (application may depend only on domain + application), and `hcl-only-in-infrastructure` (no package outside `internal/infrastructure` may directly import `hashicorp/hcl` or `zclconf/go-cty`). All 13 pre-existing domain self-test cases still pass unchanged; 9 new cases prove each new rule fails on exactly its own name, including a `zero` case proving infrastructure is legitimately exempt from the HCL rule.
- `indexing.Error{Stage, Path, Err}` wraps every failure with which of the three Build stages (`load-units`, `read-surface`, `assemble`) it happened in; `Unwrap` makes `errors.Is`/`errors.As` see through it, verified for a cancelled-context, a `LoadUnits` failure, a `ReadSurface` failure, and a domain-constructor rejection (zero `Module` on a resolved DTO).

## Task Commits

Each task was committed atomically:

1. **Task 1: Ports package** - `62ccf0d` (feat)
2. **Task 2: Architecture rules for the application layer and the HCL boundary, each proven by a self-test** - `3de8607` (ci)
3. **Task 3: indexing.Build use case, tested with fakes only** - `fac2569` (feat, TDD: RED stub verified failing, then GREEN implementation)

**Plan metadata:** (this commit) `docs(02-02): complete application ports and indexing.Build plan`

## Files Created/Modified
- `internal/application/ports/ports.go` - `UnitLoader`/`LoadResult`/`UnitConfig` (three-state DTO with documented precedence), `SurfaceReader`/`SurfaceResult`
- `internal/application/indexing/build.go` - `Build`, `Result`, `Error` (Stage/Path/Err, `Unwrap`), `assembleUnit`, `distinctResolvedModules`
- `internal/application/indexing/build_test.go` - `fakeLoader`, `fakeSurfaces` (records calls, fails on unmapped module), `dump` (canonical Result rendering for determinism assertions), 13 tests covering every `<behavior>` bullet
- `scripts/check-architecture.sh` - compile-gate now vets `internal/application` too; non-vacuous guards moved before it; `check_stdlib_allowlist`/`check_platform_neutral`/`check_external_deps` bash functions shared between domain and application rules; new `hcl-only-in-infrastructure` rule
- `scripts/test-check-architecture.sh` - `addstub` helper (offline `hashicorp`/`zclconf` probe modules via `go mod edit -replace`); 9 new self-test cases

## Decisions Made
- Reordered `check-architecture.sh`'s non-vacuous guards to run before the compile gate (see key-decisions above) — a genuine, empirically-verified interaction between the plan's own compile-gate change and the pre-existing domain "vacuous" self-test case, not anticipated by the plan's stated facts (which covered only the single-pattern `go vet ./internal/domain/...` case, not the combined multi-pattern invocation with `internal/application` depending on `internal/domain`).
- `indexing.Build` assembles every unit before reading any module surface (see key-decisions above), so the "resolved DTO with zero Module" error test asserts `Stage == "assemble"` and zero `ReadSurface` calls, matching the plan's `<behavior>` bullet exactly.
- Refactored the two pre-existing domain rules (`domain-stdlib-allowlist`, `domain-platform-neutral`) to share bash functions with their new application counterparts. This is a deviation from doing pure additive work, but the plan explicitly permits it ("You may factor the shared allowlist/platform logic into a bash function if both callers keep their exact `=== RULE FAILED: <name> ===` lines") and it was verified, not assumed, that self-tests only check that header line.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Empty test-import pipeline exit status trips `set -e`**
- **Found during:** Task 2, first run of `check-architecture.sh` after adding `check_stdlib_allowlist`
- **Issue:** `internal/application/ports` had no test file yet at that point, so `go list -f '{{range .TestImports}}...' ./internal/application/...` produced empty output; piping empty output through `grep -v '^$'` makes `grep` exit 1 (no lines selected), which under `pipefail` made the `test_imports=$(...)` assignment fail and silently abort the whole script under `set -e` (no error printed).
- **Fix:** Added `|| true` to the `prod_imports`/`test_imports` pipelines inside `check_stdlib_allowlist`, matching the pattern already used elsewhere in the script for exactly this reason.
- **Files modified:** scripts/check-architecture.sh
- **Verification:** `bash scripts/check-architecture.sh` now prints `architecture: OK (...)` instead of exiting silently; confirmed via `bash -x` trace before and after.
- **Committed in:** 3de8607 (Task 2 commit)

**2. [Rule 1 - Bug] Compile-gate/non-vacuous-guard ordering broke the pre-existing "vacuous" self-test**
- **Found during:** Task 2, running `test-check-architecture.sh` after wiring `internal/application` into the compile gate
- **Issue:** The "vacuous" case empties `internal/domain` wholesale. Once `internal/application` (which imports `internal/domain`) was added to the compile-gate's `go vet` invocation, that case started genuinely failing to compile (`no required module provides package .../internal/domain/diagnostic`), tripping `compile-gate` instead of the intended `non-vacuous-guard`.
- **Fix:** Moved both non-vacuous guards (domain, application) to run before the compile gate, using `fail=1` (soft) rather than `exit` so a genuinely broken tree still reaches and reports the compile-gate failure too.
- **Files modified:** scripts/check-architecture.sh
- **Verification:** `bash scripts/test-check-architecture.sh` — all 13 pre-existing cases (including `vacuous`) plus 9 new cases pass.
- **Committed in:** 3de8607 (Task 2 commit)

---

**Total deviations:** 2 auto-fixed (both Rule 1 - bugs found while building the CI check itself, both fixed before commit).
**Impact on plan:** Both fixes were necessary for the arch check to run correctly at all; no scope creep — no new rules, files, or behavior beyond what the plan specified.

## Issues Encountered

None beyond the two auto-fixed issues above.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plans 02-03 through 02-05 can now implement `UnitLoader` and `SurfaceReader` against a stable, tested contract; `indexing.Build`'s behavior for every DTO state (config-unknown, module-unknown, resolved with known/unknown surface, resolved/unresolved dependencies, dependency targets outside the unit set) is locked in by tests, so an adapter bug will surface as a graph-shape mismatch rather than a silent assumption change.
- `hcl-only-in-infrastructure` is live now, before any HCL-parsing code exists: Plan 03 (which adds `hashicorp/hcl/v2` and `zclconf/go-cty` as real dependencies) will be enforced by this rule from its very first commit.
- No blockers. Full local suite (build, vet, test, gofmt, `go mod tidy -diff`, staticcheck, arch check + self-test) is green at `fac2569`; push and CI confirmation follow this summary.

---
*Phase: 02-parsing-graph-construction*
*Completed: 2026-09-25*

## Self-Check: PASSED

All created/modified files and all three task commit hashes (62ccf0d, 3de8607, fac2569) verified present.
