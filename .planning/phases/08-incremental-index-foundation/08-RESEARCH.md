# Phase 8: Incremental Index Foundation - Research

**Researched:** 2026-10-08
**Domain:** Go; persistent parse cache in the terragrunt loader + rapid stateful property test
**Confidence:** HIGH (code read directly; rapid API read from module cache v1.3.0)

<user_constraints>
## User Constraints (from CONTEXT.md)

No CONTEXT.md exists (unattended run). Constraints supplied by the orchestrator:
- Make the loader's `fileCache` persistent, invalidated by dirty path; rebuild discovery + graph on every reindex so incremental == full by construction.
- rapid v1.3.0, test-only, stateful test over an in-memory `fs.FS`.
- No C compiler locally (no `-race`). Do NOT run `scripts/test-check-architecture.sh` (~18 min). Do not commit (orchestrator commits). No Claude attribution in commits.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| DAEMON-02 | Incremental reindexing yields exactly the diagnostics and graph a full rescan yields, proven by a rapid stateful property test over an in-memory fs | Persistent per-Loader parse store + `Invalidate`; rapid `T.Repeat` model test comparing against a fresh Loader; hit/miss stats for SC4 |
</phase_requirements>

## Summary

Verified in code: `terragrunt.Loader` (`internal/infrastructure/terragrunt/loader.go`) holds only `fsys fs.FS`. `LoadUnits` creates `newFileCache(l.fsys)` on every call (parse.go:146-158). `fileCache.get` is read-once-parse-once, keyed by `RepoPath.String()`. Discovery (`discoverUnits`, walk.go) takes an `fs.FS` and uses `fs.WalkDir`, so an in-memory `fstest.MapFS` works directly. Existing test helpers already do this (`mapFS`, `filesFS`, `countingFS` in `testhelpers_test.go`). `tfsurface.Reader` also takes an `fs.FS` and reads `*.tf` on each `ReadSurface` call (uncached).

`parse(p)` is a pure function of (path, file bytes). Nothing else (Stat results, include resolution, symlinks, `ReadDir`) is stored in `parsedFile`. Those other filesystem lookups (`fs.Stat` for autoinclude/includes/targets, `ReadDir` in `unitDirOverlaysModule`, `canonicalPath`) are re-done every `LoadUnits` and are never cached. So persisting only the parse results, and rebuilding everything else each reindex, makes incremental == full except for cache-invalidation bugs, which is what the rapid test targets.

**Primary recommendation:** Add a persistent `parseStore` (map path -> `*parsedFile`) owned by `Loader`, with `Loader.Invalidate(paths ...string)` and `Loader.CacheStats()`. Keep `NewLoader(fsys)` and `check` behaviour unchanged. Put the rapid model test in package `terragrunt` (infrastructure; not covered by any external-deps rule). Add `pgregory.net/rapid v1.3.0` to go.mod.

## Standard Stack

### Core
| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| pgregory.net/rapid | v1.3.0 (go 1.23 min; project is go 1.27) | Stateful property test with shrinking | `T.Repeat(map[string]func(*T))`; key `""` is an invariant action run before/after every action (verified in statemachine.go). Zero transitive deps. Already in local module cache. |
| testing/fstest.MapFS | stdlib | In-memory fs.FS | Already used in loader tests; mutable map, reads are live, so mutating the map between reindexes simulates edits with no watcher/sleeps |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| rapid | gopter | Locked by milestone research; rapid has simpler state machines and auto-shrinking |

**Installation:**
```bash
go get pgregory.net/rapid@v1.3.0   # then: go mod tidy  (CI runs `go mod tidy -diff`)
```
go.sum currently has no rapid entries; the module zip is in `~/go/pkg/mod/cache/download/pgregory.net/rapid/@v/` so it should resolve offline (`GOFLAGS=-mod=mod`, `GOPROXY=off` works if cached).

## Architecture Patterns

### Where things go
```
internal/infrastructure/terragrunt/
  parse.go          fileCache becomes a per-call view over a persistent store
  loader.go         Loader gains store field, Invalidate, CacheStats
  incremental_test.go   rapid model test (package terragrunt, uses mapFS/filesFS helpers)
```
The test imports `internal/application/checking` (or `indexing`), `tfsurface`, and `internal/interfaces/presenter`. Infrastructure tests already import `indexing`; layer rules (`infrastructure-importers`, external-deps) only police domain/application/interfaces, so this is allowed. No change to `scripts/check-architecture.sh`.

### Pattern 1: persistent store + per-call view (CRITICAL design point)
Today `syntaxDiagnostics()` iterates ALL entries in `c.files` and emits GRT100 for each with `syntax != nil`. If the map simply persists, a file that had a syntax error in an earlier reindex but is no longer reachable (deleted include, unit removed, renamed) would keep emitting GRT100 -> incremental != full. Therefore:
- `parseStore` (persistent): `map[string]*parsedFile`.
- `fileCache` (per `LoadUnits` call): holds pointer to store plus `touched map[string]struct{}`; `get` marks touched, returns cached on hit (hits++), parses+stores on miss (misses++). `syntaxDiagnostics()` ranges over `touched` only.
- End of `LoadUnits`: evict store entries not in `touched` (bounds memory, PITFALLS #15, and drops deleted files). Sound because a parse depends only on bytes.
- Both the early error return (`ctx` cancel mid-load) paths must not leave the store inconsistent; simplest is to prune only on successful completion.

### Pattern 2: Invalidate by dirty path
`Loader.Invalidate(paths ...string)`: for each repo-relative slash path `p`, delete `store[p]` AND every key with prefix `p + "/"` (covers directory renames/removals when a watcher reports only the directory). Negative entries (`readErr` for a missing file) are cached too, so a create MUST be invalidated by its dirty path; the model test covers this (include created after the unit that references it).
Dirty-set contract for callers (Phase 9/10): any create/write/remove/rename marks the affected path(s), both old and new for a rename. Dirty set is the correctness mechanism here (per orchestrator); content-compare-on-use is NOT required, but note mtime cannot be used for validation under MapFS (zero ModTime).

### Pattern 3: stats for SC4
`Loader.CacheStats() (hits, misses int)` (or a `Stats` struct, reset per call). Test asserts: reindex with empty dirty set -> misses == 0; one edited file -> misses == 1 (plus any newly reachable files). Additionally reuse `countingFS` to assert unchanged files are not opened. Keep stats in infrastructure (application allowlist permits only `context`; do not add sync/time there).

### Pattern 4: rapid state machine
```go
// Source: rapid v1.3.0 statemachine.go (T.Repeat)
func TestIncrementalEqualsFull(t *testing.T) {
    rapid.Check(t, func(t *rapid.T) {
        fsys := fstest.MapFS{}
        inc := NewLoader(fsys)          // persistent across the whole sequence
        var dirty []string
        reindexAndCompare := func(t *rapid.T) {
            inc.Invalidate(dirty...); dirty = nil
            got := render(t, inc, fsys)              // checking.Check + presenter.JSON + presenter.Graph
            want := render(t, NewLoader(fsys), fsys) // fresh loader = full rescan
            if got != want { t.Fatalf("incremental != full\n%s\n---\n%s", got, want) }
        }
        t.Repeat(map[string]func(*rapid.T){
            "create": ..., "edit": ..., "delete": ..., "rename": ..., "rename-dir": ...,
            "reindex": reindexAndCompare,
            "":        func(t *rapid.T) { /* optional invariant */ },
        })
    })
}
```
- Compare SERIALISED output (JSON diagnostics+graph via `presenter.JSON` and `presenter.Graph`, as `gruntled graph --json` does), never graph pointers (PITFALLS #3).
- Use a SMALL path alphabet via `rapid.SampledFrom` (dirs a,b,c; names terragrunt.hcl, root.hcl, a few `.tf` modules) so ops collide; generate file contents from a few templates (valid unit, with include, with dependency, syntax-broken, empty, oversized not needed). Include rename of an include file and of a directory containing units.
- Dirty-set bookkeeping lives in the model: every op appends the touched path(s); ops may run several times before a `reindex` action (batches). Also include an op sequence where a reindex occurs with nothing dirty.
- No sleeps, no watcher, no goroutines: deterministic.
- Failure persistence: rapid writes `testdata/rapid/<Test>/*.fail` in the package dir on failure; commit-worthy only if intentionally kept, and the repo `.gitignore` should be considered. Reproduce with `-rapid.seed=N`; checks default 100 (`-rapid.checks=N`).
- tfsurface: `ReadSurface` is uncached and recomputed each build, so it cannot diverge. If a surface cache is added later, it needs the same model coverage (out of scope unless planner chooses to; recommended as a separate later plan).

### Anti-Patterns
- Caching assembly/graph/discovery results (not needed, risk of drift).
- Persisting `files` map without per-call touched set (stale GRT100, see Pattern 1).
- Time/mtime based validation in tests.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Random op sequences + shrinking | custom fuzz loop | rapid `T.Repeat` | automatic shrink to minimal sequence (SC2) |
| In-memory fs | custom FS | `fstest.MapFS` (+ existing `countingFS`) | already used by loader tests; supports ReadLink in current Go |
| Graph comparison | reflect.DeepEqual on pointers | `presenter.Graph` + `presenter.JSON` bytes | byte-stable, matches shipped output |

## Common Pitfalls

### Pitfall 1: stale GRT100 from persistent map
See Pattern 1. Warning sign: model test fails after delete/rename of a syntactically broken file. Fix: per-call touched set + prune.

### Pitfall 2: negative cache on create
A missing include/unit file cached as `readErr`; later created with no invalidation -> stale unknown reason. Invalidate must evict negative entries; model must create files after references exist.

### Pitfall 3: directory rename dirty-set
Rename of a directory moves many files; if the model (later: the watcher) dirties only the dir, children stay stale. Prefix eviction in `Invalidate` handles it; model test should include dir rename and dirty only the two directory paths in at least one variant.

### Pitfall 4: `check` regression
`check` builds a fresh Loader per run; with an empty store behaviour must be byte-identical. Run existing golden/corpus tests (`cmd/gruntled` golden_test, corpus_test, e2e_test, loader_test, parse_test `TestFileCacheLimits`) unchanged. `parse_test.go` calls `newFileCache(fsys)` directly: keep that constructor signature working (or update those tests minimally) so the parse-once tests still pass.

### Pitfall 5: shared mutable state
`fileCache` is documented single-threaded. A persistent store makes `Loader` non-reentrant: document "one LoadUnits at a time" (daemon indexer is a single goroutine) or guard with a mutex. Invalidate must not run concurrently with LoadUnits. `parsedFile` is treated as immutable after parse; confirm no resolve code mutates it (reads only seen; verify during implementation, otherwise cached facts get corrupted across reindexes).

### Pitfall 6: memory
`parsedFile` retains `src` and hcl expressions (AST). Pruning untouched entries bounds growth; note for Phase 10 measurement.

### Pitfall 7: rapid and the no-net/no-exec proof
Proof (check-architecture.sh Step ~7) uses `go list -deps` WITHOUT `-test`, comment explicitly says test-only deps are legitimately excluded. rapid is imported only from `_test.go`, so it never appears in `go list -deps ./cmd/gruntled`. STACK.md notes rapid links net/url (harmless; neither net nor os/exec). `check_external_deps` (uses `-test`) runs only on `./internal/domain/...`, `./internal/application/...`, `./internal/interfaces/...`; a test in `internal/infrastructure/terragrunt` is not scanned. DO NOT put the rapid test in application/domain/interfaces. Also `testsupport-only-in-tests` etc. unaffected. Confidence HIGH on reading; the 18 min script is not to be run, but `go list -deps ./cmd/gruntled | grep rapid` (empty) is a cheap check.

## Code Examples

Per-call cache view (shape, not final):
```go
type parseStore struct{ files map[string]*parsedFile }

type fileCache struct {
    fsys    fs.FS
    store   *parseStore
    touched map[string]struct{}
    hits, misses int
}

func (c *fileCache) get(p repograph.RepoPath) *parsedFile {
    k := p.String()
    c.touched[k] = struct{}{}
    if pf, ok := c.store.files[k]; ok { c.hits++; return pf }
    c.misses++
    pf := c.parse(p); c.store.files[k] = pf; return pf
}
// syntaxDiagnostics: iterate c.touched (sorted), not c.store.files.
// end of LoadUnits: for k := range store.files { if _, ok := touched[k]; !ok { delete } }
```
`newFileCache(fsys)` can remain as a convenience creating a private empty store, keeping parse_test.go compiling.

## State of the Art

| Old | Current | Impact |
|-----|---------|--------|
| Per-call fileCache | Per-Loader persistent store + Invalidate | Phase 9/10 daemon can reindex cheaply |

## Open Questions

1. **Should tfsurface get a cache in Phase 8?**
   - Known: uncached, recomputed each build; SC4 text says "unchanged files", loader files satisfy the measurable criterion.
   - Recommendation: out of Phase 8 core; optional follow-up plan with same model test extended. Planner decides; if included, invalidate per module dir (dirty path's parent) and prune like the store.
2. **Does MapFS in go 1.27 implement `fs.ReadLinkFS` so includes resolve (not "unknown")?** Likely yes (existing include tests use MapFS); the Wave 0 smoke test should confirm that generated includes actually resolve, otherwise the property is vacuous (assert at least some sequences yield a resolved include unit, e.g. via `rapid.T.Log`/coverage counter or a deterministic seed example).
3. **Public home for Invalidate**: no port needed yet (Phase 9 session wraps it). Keep as method on `*terragrunt.Loader`; no application change in Phase 8.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + pgregory.net/rapid v1.3.0 (to add), testscript already present |
| Config file | none |
| Quick run command | `go test ./internal/infrastructure/terragrunt/ -run 'TestIncremental|TestFileCache|TestLoader' -count=1` |
| Full suite command | `go test -count=1 ./...` (CI adds `-race`; unavailable locally, no C compiler) |

### Phase Requirements -> Test Map
| Req / SC | Behavior | Test Type | Automated Command | File Exists? |
|----------|----------|-----------|-------------------|-------------|
| DAEMON-02 / SC1 | incremental == full (serialised diagnostics + graph) after random create/edit/rename/delete | rapid stateful | `go test ./internal/infrastructure/terragrunt/ -run TestIncrementalEqualsFull -count=1` | Wave 0 |
| SC2 | runs in CI, shrinks, no watcher/sleeps | same test, no build tags, runs under plain `go test ./...` | `go test ./internal/infrastructure/terragrunt/ -run TestIncrementalEqualsFull -rapid.checks=200` | Wave 0 |
| SC2 (shrink works) | deliberately broken Invalidate is caught with a short sequence | manual one-off mutation check (e.g. skip eviction) | run test, observe minimal output | manual |
| SC3 | `check` output unchanged | existing golden/corpus/e2e | `go test ./cmd/gruntled/ -count=1` | exists |
| SC4 | unchanged files not re-parsed | unit: CacheStats misses==0 on no-op reindex, ==1 after one edit; countingFS opens | `go test ./internal/infrastructure/terragrunt/ -run TestPersistentCache -count=1` | Wave 0 |
| Pitfall 1 | stale GRT100 after deleting broken file | deterministic unit | `-run TestPersistentCacheNoStaleSyntaxDiag` | Wave 0 |
| Pitfall 7 | rapid not linked in binary | shell | `go list -deps ./cmd/gruntled \| grep -c rapid` -> 0 | n/a |

### Sampling Rate
- Per task commit: quick run command above
- Per wave merge: `go vet ./... && go test -count=1 ./...`
- Phase gate: full suite green, `gofmt -l .` empty, `go mod tidy -diff` clean; do NOT run `scripts/test-check-architecture.sh`. A cheap sanity: `scripts/check-architecture.sh` itself is the slow-ish one too; planner should limit to the `go list -deps` grep above.

### Wave 0 Gaps
- [ ] `go get pgregory.net/rapid@v1.3.0` + `go mod tidy` (go.mod/go.sum)
- [ ] `internal/infrastructure/terragrunt/incremental_test.go` (model + render helper)
- [ ] `internal/infrastructure/terragrunt/cache_test.go` (stats, no-stale-diag, negative-entry invalidation, dir-prefix invalidation)
- [ ] Non-vacuity guard: the model must reach states with resolved includes and with syntax errors

## Sources

### Primary (HIGH)
- Local code: internal/infrastructure/terragrunt/{loader,parse,walk}.go, testhelpers_test.go; internal/application/indexing/build.go; internal/application/checking/check.go; cmd/gruntled/main.go; scripts/check-architecture.sh (allowlist at lines ~299-309; external-deps at 338-344; proof comment ~466-475)
- pgregory.net/rapid v1.3.0 source in module cache (statemachine.go `Repeat`)
- .planning/research/{ARCHITECTURE,STACK,PITFALLS,SUMMARY}.md

### Tertiary (LOW)
- None. (Network not used; rapid release date 2026-03-30 from STACK.md.)

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH, rapid present in cache, API read
- Architecture: HIGH, derived from reading code; stale-GRT100 hazard verified in `syntaxDiagnostics`
- Pitfalls: MEDIUM-HIGH; parsedFile immutability and MapFS ReadLink need confirmation during implementation

**Research date:** 2026-10-08
**Valid until:** 2026-11-07
