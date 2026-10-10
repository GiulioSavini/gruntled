---
phase: 08-incremental-index-foundation
plan: 01
subsystem: infra
tags: [terragrunt, parse-cache, incremental, loader]

# Dependency graph
requires:
  - phase: 02 (terragrunt loader)
    provides: Loader, fileCache, parsedFile
provides:
  - parseStore persisted on Loader across LoadUnits calls
  - Loader.Invalidate(paths...) (file, directory prefix, "." clears all)
  - Loader.CacheStats() (hits, misses of last successful LoadUnits)
  - touched-set driven GRT100 + end-of-load prune
affects: [08-02 rapid incremental==full test, 09 daemon indexer]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Persistent pure-parse cache, dirty-path invalidation; discovery/resolution/assembly always recomputed"
    - "Per-call fileCache view over shared store with touched set"

key-files:
  created:
    - internal/infrastructure/terragrunt/cache_test.go
  modified:
    - internal/infrastructure/terragrunt/parse.go
    - internal/infrastructure/terragrunt/loader.go

key-decisions:
  - "Only parse results persist (pure function of path+bytes); discovery, resolution, graph and tfsurface recomputed per load so incremental == full by construction"
  - "GRT100 computed from touched set, not store: an unreferenced broken file yields no stale diagnostic even if never invalidated"
  - "Store pruned to touched set only on successful LoadUnits; early ctx/walk errors leave it intact"
  - "Negative (readErr) entries are cached: dirty-set contract requires Invalidate on create, and old+new paths on rename"
  - "parsedFile verified immutable after parse (mergeReferences appends into a fresh slice before sorting); documented on the type"
  - "Loader not safe for concurrent use; Invalidate must not overlap LoadUnits"

patterns-established:
  - "Dirty-set contract: every create/write/remove/rename -> Loader.Invalidate(path)"

requirements-completed: [DAEMON-02]  # advanced (foundation, SC3/SC4); final equivalence proof is 08-02's rapid test, REQUIREMENTS.md left Pending until then

# Metrics
duration: 2min
completed: 2026-10-08
---

# Phase 8 Plan 01: Persistent Parse Store Summary

**Terragrunt Loader now keeps a parseStore across LoadUnits calls, evicted via Loader.Invalidate (file or directory prefix), with CacheStats proving zero re-parses on a no-op reload and exactly one after a single edit.**

## Performance

- **Duration:** ~2 min (execution)
- **Completed:** 2026-10-08
- **Tasks:** 2 (Task 2 verification-only)
- **Files modified:** 3

## Accomplishments
- `parseStore` + per-call `fileCache` view (touched set, hits/misses); `newFileCache(fsys)` signature preserved for parse_test.go, `newFileCacheOn(fsys, store)` added.
- `Loader.Invalidate` / `Loader.CacheStats`; prune of untouched entries and stats publication on success only.
- `syntaxDiagnostics` ranges the touched set: no stale GRT100 after delete or unreference.
- Four deterministic tests (`TestPersistentCacheStats`, `...NoStaleSyntaxDiag`, `...NegativeEntry`, `...InvalidateDirPrefix`), each also asserting result equals a fresh Loader.
- Full suite (`go vet ./...`, `go test -count=1 ./...`, `gofmt -l .`) green; no golden/fixture files touched (SC3).

## Task Commits

1. **Task 1 RED: failing tests** - `4c9a380` (test)
2. **Task 1 GREEN: persistent parseStore, Invalidate, CacheStats** - `20ab8c9` (feat)
3. **Task 2: no-regression gate** - no commit (verification only, no file changes needed)

## Files Created/Modified
- `internal/infrastructure/terragrunt/cache_test.go` - TestPersistentCache* suite
- `internal/infrastructure/terragrunt/parse.go` - parseStore, fileCache view, prune, parsedFile immutability doc
- `internal/infrastructure/terragrunt/loader.go` - store field, Invalidate, CacheStats, dirty-set/concurrency docs

## Decisions Made
See key-decisions. Immutability audit: grep over merge.go, refs.go, eval.go, include_targets.go, depfacts.go, loader.go found only reads of parsedFile fields; `mergeReferences` appends `pf.refs` into a nil slice (fresh allocation) before `sortRefs`, so cached slices are never sorted in place.

## Deviations from Plan

### Test design adjustment

**1. NegativeEntry test seeds the readErr entry directly**
- **Found during:** Task 1
- **Issue:** `resolveIncludes` stats the include before `cache.get`, so a missing include yields `include-not-found` without ever caching a negative entry; the plan's loader-only scenario would not exercise negative caching.
- **Fix:** Test keeps the plan scenario (missing include -> create -> Invalidate -> equals fresh Loader) and additionally seeds a readErr entry via `newFileCacheOn(fsys, l.store).get("root.hcl")`, then shows a Loader sharing the store without Invalidate reports `unreadable-config` (contract enforcement), and after Invalidate resolves correctly.
- **Files modified:** cache_test.go
- **Commit:** 4c9a380

**2. Defensive nil-store guard in LoadUnits**
- `LoadUnits` lazily creates the store if a `Loader` was built as a struct literal (zero-value safety). No behaviour change for `NewLoader`.

## Issues Encountered
None.

## Next Phase Readiness
- 08-02 can drive a reused Loader with Invalidate in a rapid stateful test against fresh-Loader results.
- DAEMON-02 stays Pending in REQUIREMENTS.md until 08-02 lands the property test.

## Self-Check: PASSED
