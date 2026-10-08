---
phase: 08-incremental-index-foundation
verified: 2026-10-08T00:00:00Z
status: passed
score: 4/4 must-haves verified
---

# Phase 8: Incremental Index Foundation Verification Report

**Phase Goal:** The index can be updated incrementally from a set of dirty paths and always yields exactly what a full rescan yields.
**Status:** passed. Initial verification.

## Success Criteria

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | Incremental reindex equals full rescan after create/edit/rename/delete on an in-memory fs | VERIFIED | `TestIncrementalEqualsFull` passes (rapid stateful model in `internal/infrastructure/terragrunt/incremental_test.go`). Its operations are create, edit, delete, rename and rename-dir. `TestIncrementalModelNonVacuous` passes. |
| 2 | The rapid test runs in CI, shrinks failures, and uses no watcher or sleeps | VERIFIED | It uses `rapid.Check` with `rt.Repeat`, so shrinking is built in. `ci.yml` runs `go test -race -count=1 ./...`. The test file has no `time.Sleep` and no `fsnotify`. |
| 3 | `check` output on the existing fixtures is unchanged | VERIFIED | `git diff 3c974f1~6 -- cmd/gruntled/testdata` is empty. `go test ./cmd/gruntled` passes. |
| 4 | Unchanged files are not re-parsed (cache hit count) | VERIFIED | `Loader.Invalidate` and `Loader.CacheStats` exist in `loader.go`. `TestPersistentCacheStats`, `TestPersistentCacheNoStaleSyntaxDiag`, `TestPersistentCacheNegativeEntry` and `TestPersistentCacheInvalidateDirPrefix` pass. |

**Score:** 4/4

## Commands Run

- `go test -count=1 ./internal/infrastructure/terragrunt/ -run 'TestIncremental|TestPersistentCache' -v`: 6 tests, all PASS.
- `go test -count=1 ./...`: all packages ok.
- `go list -deps ./cmd/gruntled | grep -c rapid`: 0. rapid is test-only.
- `go mod tidy -diff`: clean, exit 0.
- Golden `testdata` diff against `3c974f1~6`: empty.
- Not run, per constraints: `-race` and `scripts/test-check-architecture.sh`.

## Requirements Coverage

| Requirement | Plans | Status | Evidence |
|-------------|-------|--------|----------|
| DAEMON-02 | 08-01, 08-02 | SATISFIED | Both plans declare it. The 08-02 SUMMARY has `requirements-completed: [DAEMON-02]`. The 08-01 SUMMARY has `requirements-completed` set but notes the requirement was only advanced there. REQUIREMENTS.md shows it checked `[x]` and "Complete". There are no orphaned requirements. |

## Anti-Patterns

None found in the phase files. `rapid` appears only in `_test.go` files.

## Human Verification

None required.

## Gaps

None.

_Verifier: Claude (gsd-verifier)_

## Security

Audit by `proj-sec:auditor` on 2026-10-08, diff `4c9a380~1..d73c85f`. Verdict: fix-small with no critical or high findings, so nothing was fixed in this phase. All items below must be handled by Phase 10 (watch daemon), which is the first concurrent, long-lived user of the cache.

| # | Severity | Location | Problem | Fix |
|---|----------|----------|---------|-----|
| 1 | medium | `internal/infrastructure/terragrunt/loader.go:21-35` | `Loader` is documented as not safe for concurrent use, but nothing enforces it. Concurrent `Invalidate` and `LoadUnits` would cause a concurrent map write and crash the runtime. | Add a `sync.Mutex` across `LoadUnits`, `Invalidate` and `CacheStats`, or make "single indexer goroutine" a Phase 10 requirement. |
| 2 | medium | `internal/infrastructure/terragrunt/parse.go:197-208` | Correctness depends on every change reaching `Invalidate`. Dropped watcher events or changes made while the daemon was down leave stale entries, including negative ones. | Store size and mtime in `parsedFile` and re-stat on each hit, or call `Invalidate(".")` whenever the watcher overflows. A periodic revalidate is already in the milestone research. |
| 3 | low | `loader.go:55-68` | `Invalidate` makes one cache scan per path, O(n·cache) on event bursts. | Build the prefix set once and do a single pass. |
| 4 | low | `loader.go:55` | Absolute, `../` or backslash paths match nothing and are ignored silently. | Document the repo-relative slash-path contract, or report the ignored paths. |
| 5 | low | `loader.go:194,651` | `fs.Stat` of the autoinclude and stack files runs on every load. | No staleness risk; cost note only. |

Checked and OK:
- Cached `parsedFile` values are never mutated.
- Memory is bounded by pruning after each load.
- A failed load leaves the cache valid.
- The read path is still size-capped.
- No new exec, net, cgo or unsafe use.
- `rapid` is not linked into the binary.

Not run:
- `govulncheck`: the `@latest` build is go1.26 and two packages need go1.27.
- `-race`: no C compiler.
