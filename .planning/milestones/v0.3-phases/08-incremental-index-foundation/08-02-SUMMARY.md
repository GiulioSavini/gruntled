---
phase: 08-incremental-index-foundation
plan: 02
subsystem: testing
tags: [rapid, property-test, incremental, loader, terragrunt]

# Dependency graph
requires:
  - phase: 08-01
    provides: Loader.Invalidate, Loader.CacheStats, persistent parseStore
provides:
  - pgregory.net/rapid v1.3.0 test-only dependency
  - TestIncrementalEqualsFull rapid stateful model (incremental == full rescan)
  - TestIncrementalModelNonVacuous deterministic companion
  - incModel helpers (write/remove/rename/renameDir/reindex) reusable by Phase 9
affects: [09 daemon indexer]

# Tech tracking
tech-stack:
  added: [pgregory.net/rapid v1.3.0 (test-only)]
  patterns:
    - "Stateful property test: mutable fstest.MapFS + persistent Loader + dirty set, compared byte-for-byte against a fresh Loader via presenter.JSON + presenter.Graph"
    - "Seeded baseline per sequence so includes resolve often; non-vacuity counters asserted after rapid.Check"

key-files:
  created:
    - internal/infrastructure/terragrunt/incremental_test.go
  modified:
    - go.mod
    - go.sum
    - .gitignore
    - .planning/phases/08-incremental-index-foundation/08-VALIDATION.md

key-decisions:
  - "Compare serialised output (JSON report + graph), never pointers; Go-level errors rendered as strings so both sides must agree on failure too"
  - "Each rapid sequence starts from a seeded valid baseline (modules, root.hcl, include unit, dependency unit); empty-start sequences resolved includes only ~1% of reindexes"
  - "Non-vacuity enforced twice: aggregate counters after rapid.Check (resolved includes, GRT100, no-op reindexes all > 0) and a fixed-sequence companion test"
  - "rapid .fail files ignored via .gitignore (internal/infrastructure/terragrunt/testdata/rapid/)"

patterns-established:
  - "rename-dir with dirsOnly variant dirties only the two directory paths, proving prefix eviction is load-bearing"

requirements-completed: [DAEMON-02]

# Metrics
duration: 8min
completed: 2026-10-08
---

# Phase 8 Plan 02: Rapid Incremental == Full Proof Summary

**rapid v1.3.0 stateful test drives create/edit/delete/rename/rename-dir over a mutable MapFS and proves a persistent Loader + Invalidate renders byte-identical JSON + graph to a fresh Loader; broken Invalidate shrinks to 3-action counterexamples.**

## Performance

- **Duration:** ~8 min
- **Completed:** 2026-10-08
- **Tasks:** 2
- **Files modified:** 5

## Accomplishments
- rapid added as test-only dep: `go mod tidy -diff` clean, `go list -deps ./cmd/gruntled | grep -c rapid` = 0.
- `TestIncrementalEqualsFull`: 100 checks ~60 ms; 300 checks ~165 ms, green. Typical 100-check run: ~520 reindexes, ~75-130 no-op reindexes (misses == 0 asserted), ~105-130 resolved-include observations, ~70-100 GRT100.
- `TestIncrementalModelNonVacuous`: fixed sequence proves MapFS resolves includes (fs.ReadLinkFS ok), GRT100 reached, no-op reindex has zero misses, dirs-only rename-dir and file rename stay equal to full.
- No goroutines, sleeps, watchers or build tags; runs under plain `go test ./...`.
- Phase-end gates: `go vet ./...`, `go test -count=1 ./...`, `gofmt -l .` empty, `go mod tidy -diff` clean, `scripts/check-architecture.sh` OK.

## Shrink Proof (SC2, not committed)

| Mutant | Shrunk counterexample | Length |
|--------|----------------------|--------|
| `Invalidate` returns immediately | delete a/terragrunt.hcl, reindex, edit b/terragrunt.hcl (kind 0), final reindex | 3 actions (+ final reindex) |
| prefix eviction disabled | reindex, rename-dir a -> b (dirsOnly=true), reindex | 3 actions (+ final reindex) |

Both mutants reverted with `git checkout internal/infrastructure/terragrunt/loader.go`; generated `testdata/rapid` removed; `git status` clean apart from planned files; suite re-run green.

## Task Commits

1. **Task 1: add rapid test-only dependency** - `bba96d5` (chore)
2. **Task 2: rapid stateful incremental == full test** - `4975f1b` (test)

## Files Created/Modified
- `internal/infrastructure/terragrunt/incremental_test.go` - incModel, render helper, TestIncrementalEqualsFull, TestIncrementalModelNonVacuous
- `go.mod`, `go.sum` - pgregory.net/rapid v1.3.0
- `.gitignore` - rapid .fail output dir
- `08-VALIDATION.md` - per-task map filled, nyquist_compliant/wave_0_complete true

## Decisions Made
See key-decisions.

## Deviations from Plan

**1. [Rule 2 - Coverage] Seeded baseline per rapid sequence**
- **Found during:** Task 2
- **Issue:** Starting from an empty MapFS, only 5 of ~500 reindexes saw a resolved include: non-vacuity was technically met but thin.
- **Fix:** `incModel.seed()` writes a valid baseline before `Repeat`; resolved-include observations rose to ~100+ per run. Same seed reused by the companion test.
- **Commit:** 4975f1b

**2. TDD RED not applicable**
- Task 2 is tdd="true", but the implementation (Invalidate) landed in 08-01, so the property test passes on first run. RED was substituted by the mutation/shrink proof above, which shows the test fails on a broken Invalidate. Single `test(...)` commit.

## Issues Encountered
None.

## Next Phase Readiness
- DAEMON-02 complete. Phase 9 indexer can reuse the dirty-set contract (create/edit/delete -> path, rename -> both paths, dir rename -> dir paths) proven here.

## Self-Check: PASSED
