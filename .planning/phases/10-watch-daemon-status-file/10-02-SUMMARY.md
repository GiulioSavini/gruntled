---
phase: 10-watch-daemon-status-file
plan: 02
subsystem: infrastructure/terragrunt, application/watching
tags: [loader, parse-cache, mutex, invalidate, daemon-indexer, phase8-security]
requires:
  - Phase 8 persistent parseStore and Loader.Invalidate
  - checking.Check
provides:
  - "Loader guarded by sync.Mutex across LoadUnits, Invalidate, CacheStats"
  - "Batch Invalidate: one clean pass into a set, one scan of the store, ancestor-walk prefix semantics"
  - "Fail-safe path contract: empty, '.', absolute, '..'-escaping or backslash path clears the whole store"
  - "ports.InvalidatingLoader (UnitLoader + Invalidate) with compile-time assertion on *terragrunt.Loader"
  - "watching.Indexer / NewIndexer: Index(ctx, dirty, resync) -> Invalidate(dirty...) or Invalidate(\".\"), then checking.Check"
affects: [10-05 (resync on watcher overflow), 10-06 (daemon run loop drives Indexer)]
tech-stack:
  added: []
  patterns: [mutex-guarded adapter with single-goroutine usage contract, fail-safe cache invalidation, call-order recording fakes]
key-files:
  created:
    - internal/infrastructure/terragrunt/loader_hardening_test.go
    - internal/application/watching/watching.go
    - internal/application/watching/watching_test.go
  modified:
    - internal/infrastructure/terragrunt/loader.go
    - internal/application/ports/ports.go
decisions:
  - Loader mutex held for the full LoadUnits; doc still prescribes a single indexer goroutine (mutex is the safety net, not the concurrency model)
  - Any out-of-contract path in a batch clears the whole store even when valid paths sit next to it; never silently ignored
  - Prefix eviction by walking each key's ancestors against the dirty set (O(cache x depth) per batch) keeps "a" from evicting "ab/x"
  - Indexer returns checking.Check's Report and error unchanged; resync ignores the dirty list
requirements-completed: []
requirements-advanced: [DAEMON-01]
metrics:
  duration: 6min
  completed: 2026-10-08
  tasks: 2
  files: 5
---

# Phase 10 Plan 02: Loader hardening and watching.Indexer Summary

The Loader is now mutex-guarded, invalidates a whole dirty batch in one pass, and treats any path outside the repo-relative slash contract as "everything changed". That closes Phase 8 security findings 1, 3 and 4. The new `watching.Indexer` is the only seam between the daemon run loop and the Loader, and its resync path (`Invalidate(".")`) is the hook for finding 2.

## What was built

- **loader.go**: `mu sync.Mutex` locked (with defer) at the top of `LoadUnits`, `Invalidate` and `CacheStats`. `LoadUnits` never calls the other two, so the lock is not re-entered. `Invalidate` cleans every input with `path.Clean` into a set. If any raw input is empty or contains a backslash, or its cleaned form is `.`, absolute, `..` or starts with `../`, the store is cleared and the call returns. Otherwise the store is scanned once and each key is deleted when it or one of its ancestors is in the set. The doc comment states the contract and the concurrency model. `var _ ports.InvalidatingLoader = (*Loader)(nil)` replaces the `UnitLoader` assertion.
- **ports.go**: `InvalidatingLoader` interface.
- **watching**: `Indexer`, `NewIndexer`, `Index`. The package comment says an Indexer is not safe for concurrent use. Imports are `context` plus application packages only.

## Tests

- `TestInvalidateBatchEquivalent`: invalidating `[a, ab/x/terragrunt.hcl]` in one call leaves the same store keys as two single calls. `a` evicts `a/b/...` but not `ab/...`.
- `TestInvalidateEscapingClearsAll`: `""`, `.`, `/abs/x`, `..`, `../x` and `a/../../x` each clear the store, alone and mixed with valid paths. The next load has hits=0 and misses equal to a cold load's.
- `TestInvalidateBackslashClearsAll`: `a\b` and `a\b\terragrunt.hcl` clear the store, alone and mixed with valid paths.
- `TestInvalidateCleanedInputs`: `a//b/` and `./a/b` behave the same as `a/b`.
- `TestConcurrentInvalidateLoad`: 8 goroutines run 20 iterations each, mixing Invalidate("a"), Invalidate("."), CacheStats and LoadUnits. The final load must DeepEqual a fresh Loader's. It is written for CI's `-race` run and was not run with `-race` locally (no C compiler).
- watching: Index forwards the dirty paths once and before the load; resync calls only `Invalidate(".")`; the initial nil batch calls no Invalidate; the Report DeepEquals `checking.Check`; a loader error comes back the same as Check's (it still wraps the sentinel) and the Report is zero.
- `TestIncrementalEqualsFull` and every Phase 8 test still pass. `go test ./...` is green and `scripts/check-architecture.sh` prints `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)`.

## Task Commits

| Task | Name | Commit |
|------|------|--------|
| 1 | Loader mutex, batch Invalidate, path contract (+ ports.InvalidatingLoader) | 7295812 |
| 2 | watching.Indexer use case | d4cb0bd |

## Deviations from Plan

1. **[Rule 3 - Blocking] ports.go edit moved into the Task 1 commit.** Task 1's compile-time assertion `var _ ports.InvalidatingLoader` needs the port, as the plan expected. The interface went into the Task 1 commit and Task 2 committed only the watching package.
2. **Commit granularity.** The plan's done line suggested one combined commit for both tasks. The executor protocol asks for one commit per task, so there are two `feat(10-02)` commits.
3. **TDD RED/GREEN not split into separate commits.** Tests and implementation for each task landed in the same commit. The new tests cover behaviour the old code lacked: the old Invalidate ignored absolute and backslash paths, so the clear-all tests fail against it, and the watching package did not exist.

## Requirements

DAEMON-01 is advanced, not completed: the Loader is now safe for a daemon to share and the Indexer seam exists. The watcher, run loop and status file come in later Phase 10 plans.

## Self-Check: PASSED
