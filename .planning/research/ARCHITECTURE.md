# Architecture Patterns (v0.3 Watch & Blast)

**Domain:** daemon + impact analysis inside an existing hexagonal Go CLI
**Researched:** 2026-10-08
**Confidence:** HIGH on fit with the existing code (read `ports.go`, `loader.go`, `parse.go`, `surface.go`, check-architecture.sh); MEDIUM on daemon concurrency details (design, not yet built)

## Existing seams this milestone plugs into
- `ports.UnitLoader.LoadUnits(ctx)` and `ports.SurfaceReader.ReadSurface(ctx, module)` feed `indexing.Build` -> `*repograph.RepositoryGraph`; `checking` produces diagnostics.
- `terragrunt.Loader` is built on `fs.FS`; `fileCache` (parse.go) parses each file at most once **per `LoadUnits` call**.
- `repograph.Surface` = sorted variable and output names only.
- check-architecture.sh layer rules: domain pure stdlib; application = domain + context only, platform-neutral; `interfaces` = pure presenters; only `cmd` and `infrastructure` import infrastructure; Step 9 forbids `net`, `os/exec`, `plugin`, `crypto/tls` in the binary for all 6 targets.

## Recommended Architecture

```
cmd/gruntled            watch | report | blast subcommands (stdlib flag), wiring only
internal/domain/
  repograph/            + SurfaceDiff(before, after) ; + Impact(before, after graph) pure functions
  impact/  (new)        Broken / Impacted sets as value objects (sorted, disjoint)
internal/application/
  ports/                + Watcher, SnapshotPublisher (status), InstanceLock, QueryServer/Client contracts
  indexing/             Build unchanged
  watching/ (new)       Session: owns current Snapshot, applies dirty sets, debounce policy is NOT here
  blasting/ (new)       use case: baseline + current -> impact.Result
internal/infrastructure/
  terragrunt/           Loader gains an optional persistent parse cache + Invalidate(paths)
  fswatch/ (new)        fsnotify adapter (!windows), poll adapter (all); same port
  statusfile/ (new)     atomic tmp+rename writer
  ipc/ (new)            unix: raw-syscall AF_UNIX server+client, flock lock; windows: stubs returning ErrUnsupported
internal/interfaces/presenter   + status-line formatter, blast text/json
```

`application` may not import `time` or `os` per the allowlist (it permits `context` only). Therefore:
**debounce lives in the adapter (`fswatch`) or in `cmd`**; the application receives already-coalesced `[]RepoPath` dirty batches and a `context`. The allowlist then does not need changing. If a `sync` primitive is needed in the application, either add `sync` and `sync/atomic` to the allowlist deliberately (they are pure) or keep concurrency in `cmd`/infrastructure and give the application a single-threaded `Apply(batch) Snapshot` method. Prefer the latter.

### Incremental reindex (DAEMON-01/02)
**Pattern: memoise parsing, never memoise assembly.** Make `fileCache` outlive one call, keyed by repo path with the file's content (or mtime+size+hash) as validity token, and add `Invalidate(paths)`. Each reindex still: re-walks to discover units, resolves every unit from cached parses, rebuilds the graph and re-runs the checks. Parsing is the cost (project premise); discovery + assembly are cheap. Consequences:
- Equality with a full rescan holds **by construction** except for cache-invalidation bugs, which is exactly what the rapid model test targets.
- No reverse include index needed: an edited include only invalidates that file's parse; unit resolution re-runs for everyone.
- Cache soundness rule: a cached parse is valid iff the file bytes are identical. Validate on use with `fs.Stat` (mtime,size) as the cheap check; the dirty set from the watcher is an optimisation hint, **not** the correctness mechanism. So a lost event costs staleness only until the next event, and the poller/periodic full revalidate (e.g. every N seconds, or on `report --rescan`) bounds it.
- Deleted files: dirty path no longer stat-able -> evict. Newly created files: found by the discovery walk and by `ReadDir` of the surface reader.
- Surfaces: `tfsurface` reader result cached per module dir, invalidated when any `*.tf` / `*.tf.json` in that dir is dirty (dirty path's `filepath.Dir`).

### Event handling
`fsnotify/poll events -> filter noise -> dirty set -> debounce (150 ms trailing, 1 s max wait) -> batch -> Session.Apply -> new immutable Snapshot -> atomic pointer swap -> publish`.
- Events never carry meaning beyond "this path may have changed". Rename, remove, create, chmod, write all map to `dirty(path)`. This single rule neutralises vim (`4913` probe, `file~` rename, swap files), VSCode and JetBrains atomic writes, `git checkout` storms.
- Snapshot = immutable value {graph, diagnostics, generation, indexedAt, duration}. Readers (socket handler, status writer) read the current pointer; the single indexer goroutine is the only writer. No locks around the graph.
- Overlapping saves during an index run: indexer loop drains the dirty set again after finishing (coalescing queue of size 1).

### Status file (DAEMON-03) - adapter
`SnapshotPublisher` implementation writes one line after each swap: write to `<name>.tmp` in the same directory, `os.Rename` over the target (atomic on POSIX; on Windows Go's Rename replaces via MoveFileEx). Format fixed by a golden test, e.g. `gruntled ok units=65 errors=0 gen=17` / `gruntled err errors=3 ...` / `gruntled indexing...`. Location: **not inside the analysed repo** (read-only rule). Default `$XDG_RUNTIME_DIR/gruntled/<sha256(abs root)[:12]>/status` falling back to `os.TempDir()`; `--status-file` overrides; `watch` prints the paths at startup. Remove (or write `stopped`) on clean exit; prompt readers must treat mtime older than N seconds plus a missing pid as stale -> include pid and unix time in the line.

### Single instance + socket (DAEMON-05/04)
Runtime dir `<runtime>/gruntled/<hash>/` containing `lock`, `sock`, `status`.
Start sequence (unix): (1) try connect to `sock` and send `ping`; success -> attach: print "already running (pid)", optionally stream/report, exit 0. (2) else `flock(LOCK_EX|LOCK_NB)` on `lock`; if it fails another instance is mid-start: retry connect for ~2 s. (3) holding the lock, `unlink(sock)` (stale, proven stale because connect failed *and* we own the lock), bind, `chmod 0600`, listen. Lock is released by the kernel on crash, so no stale lock logic. Protocol: one request line (`report <format>`, `blast <path>`, `ping`, `rebase`) -> response bytes -> close. Versioned first line (`gruntled-ipc 1`) so mismatched binaries fail loudly.
Windows: lock via exclusive `CreateFile` share mode 0; no socket; `report` reads the status file and states the limitation.
"Attach" semantics: pick the minimal meaning, "second `watch` detects the first and exits 0 pointing at it". Do not build a multiplexed streaming client in v0.3.

### Blast (BLAST-01/02) - the "before" state
The question needs two states; options evaluated:
| Option | Verdict |
|--------|---------|
| git (`git show`, `git diff`) | forbidden: exec. Reimplementing object reading in pure Go = large; go-git = huge dep. Reject. |
| In-memory previous index (daemon) | Works naturally: daemon holds `baseline` snapshot (taken at start; `rebase` advances it to current). Blast = diff(baseline, current). Good for live use. |
| Two directories (`--base <dir>`) | Works one-shot and in CI (`git worktree add ../base origin/main` is the user's job). Needs only a second `Loader` over another `fs.FS` root. Deterministic, testable with testscript. |
| Previous index from disk cache | Out of Scope. |
Recommendation: implement the pure domain function first, then both front ends (daemon baseline, `--base`). Without either, `blast` reports Broken only, labelled "no baseline".

Domain:
```go
// impact (pure)
func Compute(before, after *repograph.RepositoryGraph, scope []RepoPath) Result
type Result struct{ Broken, Impacted []RepoPath /* sorted, disjoint */ ; Changes []SurfaceChange }
```
- `scope`: the `<path>` argument resolved to modules (a file inside a module dir, a module dir, or a unit dir; a unit dir -> its module).
- `SurfaceDiff(before, after Surface)`: added/removed variables and outputs; empty diff => Impacted empty (BLAST-02).
- Consumers of module M: units whose `ModuleOf == M` (they bind variables), and units holding a reference (`dependency.X.outputs.Y`) to a unit whose module is M (they bind outputs).
- Broken = units with a GRT001/GRT100 finding in `after` that references M (reuse `checking`), i.e. a removed output that is referenced. Impacted = consumers of M with non-empty diff **minus** Broken. Transitivity: because unit outputs *are* module outputs, an unchanged downstream unit is impacted only through a changed output it references, so one hop is enough; do not walk the dependency graph transitively in v0.3 (it would report units whose inputs did not change; contradicts BLAST-02's anti-noise intent). Needs an explicit product decision; record it in the requirement.
- Pre-existing breakage: Broken must be defined relative to the change (finding present in after, absent in before), otherwise blast reports debt. Use a diagnostic set difference keyed by (code, unit, message).

## Build order (dependencies)
1. Persistent parse cache + `Invalidate` in `terragrunt.Loader` (behaviour-preserving; full-scan tests must still pass byte-identical).
2. rapid model test over an in-memory `fs.FS` (random create/modify/delete/rename of units, includes, `.tf`) comparing cached loader vs fresh loader. Do this **before** any watcher exists; it is deterministic, fast, flake-free.
3. Domain `SurfaceDiff` + `impact.Compute` + `blast --base` CLI (shippable on its own; BLAST-01/02).
4. `Watcher` port + poll adapter + `watching.Session` + `gruntled watch` (foreground, prints findings) with fake-clock tests.
5. fsnotify adapter (!windows) behind the same contract-test suite the poller passes.
6. Status file publisher.
7. ipc (lock, server, client), `report`, attach; daemon baseline for blast via socket.
8. Architecture script updates (new package rules, windows proves fsnotify absent), docs, release.

## Anti-patterns
- Driving cache invalidation from event *types* (Rename vs Write): editors disagree; use stat.
- Putting `time.AfterFunc` debounce in `application`: violates the stdlib allowlist.
- Mutable shared graph with a RWMutex: invites torn reads; use immutable snapshots.
- Diffing text of module files for BLAST-02: comment edits then count; diff parsed surfaces.
- Writing status/lock/socket under the repo root: breaks "never writes inside the analysed repository".
- Letting `report` auto-start the daemon.

## Scalability
| Concern | 100 units | 5k units / 50k dirs | Notes |
|---------|-----------|---------------------|-------|
| Walk per reindex | negligible | tens of ms (warm stat) | if too slow, restrict re-walk to dirty dirs; keep the periodic full walk |
| inotify watches | few hundred | may exceed 8192 default on older kernels | fall back to poll on ENOSPC with a visible warning |
| Memory | MBs | hundreds of MB if parses retain ASTs | cache only what resolution needs, or bound it; measure on the corpus |

## Sources
- Repository inspection 2026-10-08 (ports.go, loader.go, parse.go, surface.go, check-architecture.sh) (HIGH)
- Go syscall/os docs for flock, rename semantics (HIGH); daemon concurrency pattern is original design (MEDIUM)
