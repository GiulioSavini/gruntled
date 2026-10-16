# Architecture Research

**Domain:** Terragrunt static analysis daemon, adding baseline-aware diagnostics (v0.4 Blast-aware Diagnostics)
**Researched:** 2026-10-10
**Confidence:** HIGH on integration points and layering (read from the code); MEDIUM on memory figures (no Go toolchain in the research environment, so they are derived from the data structures, not measured); MEDIUM-HIGH on Terraform type semantics (checked against current Terraform docs/changelog).

Line references are to the tree at v0.3.0 and name the symbol too, because lines drift.

## Summary of Decisions

| Question | Decision |
|----------|----------|
| Where does the baseline live? | In the `watchPublisher` (cmd/gruntled/watch.go), as a retained immutable `checking.Report` (graph + diagnostics). No deep copy, no copy-on-write, no snapshot of the Loader's parse cache. |
| Default baseline | The daemon's first successful index. `rebase` replaces it with the latest published Report. |
| How does the daemon serve blast? | Pre-render the blast text/JSON at every publish, next to the check bytes, so socket handlers stay byte-readers and the Windows report file carries it for free. |
| IPC protocol | Bump `ProtocolVersion` 1 to 2 (exact-match policy kept). Add ops `blast` (read) and `rebase` (the only mutating op, in-memory only). `report` stays byte-identical to `check`. |
| Windows | `report --blast` works via the report file. `rebase` is not available (needs a socket); documented, exit 3. |
| GRT004 | Never produced by `check`/`report`. Produced only where two graphs exist: `blast` and `report --blast`. In those views it supersedes the matching GRT001 (same unit and position). |
| GRT004 shape | One diagnostic per referencing site, error severity, plus a grouped `RemovedOutputs` list in the blast `Result` so the presenter can name consumers. |
| Type extraction | In `tfsurface` (infrastructure), via `hcl/v2/ext/typeexpr` to `cty.Type`, rendered to a canonical string. Domain gets an opaque `TypeConstraint` value object (equality only). |
| Output "type" | Terraform 1.15.0 (2026-04-29) added an optional `type` argument to `output`. Absent means unknown, never compared. Output `sensitive` is a literal and is compared. |
| Transitive impact | Pure multi-source BFS in `domain/impact` over reverse data-flow edges (enabled-true block edges), each level sorted by `RepoPath`, first discovery wins, so the path is shortest and canonical. |
| Build order | Between-refactor + GRT004, transitive impact, type-level surface, daemon baseline + IPC v2, then a cross-phase audit. |

## Standard Architecture

### System Overview (v0.4 additions marked NEW / MOD)

```
┌──────────────────────────────────────────────────────────────────────────┐
│ cmd/gruntled (composition root)                                           │
│  check  graph  blast(MOD)  watch(MOD)  report(MOD: --blast)  rebase(NEW)  │
│  instance.go(MOD)  render.go(MOD: buildBlastView)                         │
├──────────────────────────────────────────────────────────────────────────┤
│ interfaces/presenter                                                      │
│  blast.go (MOD: path, removed outputs, type changes, schema v2)           │
├──────────────────────────────────────────────────────────────────────────┤
│ application                                                               │
│  checking (unchanged)   indexing (unchanged)   watching (unchanged)       │
│  blasting (MOD: Between(base, cur Report) NEW; Blast delegates to it)     │
│  ports (unchanged: SurfaceResult.Surface is a domain type, just richer)   │
├──────────────────────────────────────────────────────────────────────────┤
│ domain (pure; no HCL, no fmt)                                             │
│  repograph: Surface(MOD) VariableDecl(NEW) OutputDecl(NEW)                │
│             TypeConstraint(NEW)                                           │
│  analysis:  grt001(MOD: shared resolver)  grt004(NEW)                     │
│  impact:    surface diff(MOD)  transitive(NEW)  Compute(MOD)              │
│  diagnostic: CodeRemovedOutput GRT004 (MOD)                               │
├──────────────────────────────────────────────────────────────────────────┤
│ infrastructure                                                            │
│  tfsurface(MOD: attrs + typestring.go NEW)   hclconv(MOD: literalBool)    │
│  ipc(MOD: v2, OpBlast, OpRebase, Handler)    watch(unchanged)             │
└──────────────────────────────────────────────────────────────────────────┘
```

### Component Responsibilities: new vs modified

| Component | Status | Responsibility | File |
|-----------|--------|----------------|------|
| `repograph.TypeConstraint` | NEW | Opaque canonical type string; zero value = unknown; `Equal`, `String`. Equality only. | `internal/domain/repograph/typeconstraint.go` |
| `repograph.VariableDecl`, `OutputDecl` | NEW | Variable: `Name`, `Type TypeConstraint`, `Required Tristate`, `Sensitive Tristate`. Output: `Name`, `Type TypeConstraint`, `Sensitive Tristate`. | `internal/domain/repograph/surface.go` |
| `repograph.Surface` | MOD (`surface.go:13`) | Keeps `Variables()`/`Outputs()` name API and `NewSurface` (20 test callers, graph presenter at `presenter/graph.go:143`, GRT001). Adds `NewDetailedSurface(vars, outs)`, `VariableDecls()`, `OutputDecls()`. `NewSurface` wraps it with all facts unknown. | same |
| `diagnostic.CodeRemovedOutput` | NEW const | `GRT004` | `diagnostic/diagnostic.go:18-29` |
| `analysis.RemovedOutputs(base, cur)` | NEW | GRT004 analyzer over two graphs. | `internal/domain/analysis/grt004.go` |
| `analysis.UnknownOutputs` | MOD (`grt001.go:40`) | Extract rows 1-5 (reference to dep to target unit to module to known surface) into an unexported `resolveReference` shared with GRT004. No behaviour change; `check` output stays byte-identical. | `grt001.go` |
| `analysis.SupersedeUnknownOutputs(curD, grt004)` | NEW | Pure: drops GRT001 whose `(Unit, Pos)` matches a GRT004, adds the GRT004s, returns a canonical `diagnostic.Set`. | `grt004.go` |
| `impact.SurfaceChange` / `SurfaceDiff` | MOD (`impact/surface.go:13,33`) | Add `AddedRequiredVariables`, `BecameRequired`, `RetypedVariables []TypeChange`, `RetypedOutputs`, `OutputSensitivity []SensitivityChange`; `Empty()` includes them. Compare only when both sides are known. | `impact/surface.go` |
| `impact.Propagate` | NEW | Multi-source BFS over reverse data-flow edges; returns per-unit shortest path. | `internal/domain/impact/transitive.go` |
| `impact.Compute` | MOD (`impact.go:97`) | Seeds = direct consumers (existing loop `impact.go:104-131`), then `Propagate`; `ImpactedUnit` gains `Path []RepoPath`; `Result` gains `RemovedOutputs`. Signature unchanged. | `impact/impact.go` |
| `blasting.Between(base, cur checking.Report) impact.Result` | NEW | The pure, shared core: runs `analysis.RemovedOutputs` + supersede, then `impact.Compute`. | `application/blasting/blasting.go` |
| `blasting.Blast` | MOD (`blasting.go:44`) | Still checks cur then base (error stages unchanged), then calls `Between`. `blast --base` and the daemon share `Between`, so they agree by construction (same trick as `renderReport`). | same |
| `tfsurface.Reader` | MOD (`reader.go:87,183,213`) | Decode per-block attributes (`type`, `default`, `sensitive`) with a second `PartialContent`; render types via `typestring.go`. A type that fails to parse makes only that fact unknown, never the surface. | `tfsurface/reader.go`, `typestring.go` (NEW) |
| `hclconv.LiteralBool` | MOD (move) | `literalBool` currently sits in `terragrunt/eval.go:75`; move to `hclconv` so `tfsurface` reuses the exact tri-state rule. | `hclconv` |
| `ipc` | MOD | `ProtocolVersion = 2` (`ipc.go:22`); `OpBlast`, `OpRebase`; `Snapshot.Blast *BlastView`; `Handler` interface; response/dump carry the blast view. | `ipc/ipc.go`, `sock_unix.go`, `sock_other.go`, `dump.go` |
| `watchPublisher` | MOD (`watch.go:227,245`) | Owns `base`, `lastRep`, a mutex; computes the blast view at each `EventReady`; implements `Rebase()`. | `cmd/gruntled/watch.go` |
| `instance` | MOD (`instance.go:126,170`) | Passes the publisher as the ipc `Handler`; `store` unchanged. | `cmd/gruntled/instance.go` |
| `report --blast` | MOD (`report.go:19,58`) | New flag; sends `OpBlast`; `--format sarif` with `--blast` is a usage error. | `cmd/gruntled/report.go` |
| `rebase` command | NEW | Sends `OpRebase`; prints the new baseline generation. | `cmd/gruntled/rebase.go`, dispatch at `main.go:144-153` |
| `presenter.BlastText/BlastJSON` | MOD (`blast.go:78,156`) | Render path, removed-output consumers, type/requiredness/sensitivity tokens; `blastSchemaVersion` 1 to 2 (`blast.go:15`). | `presenter/blast.go` |

Unchanged on purpose: `checking.Check`/`analyzers` (`check.go:47`), `indexing.Build`, `watching.Indexer`, `watch.Run`, `ports`, SARIF, `graph --json`, the status file. `GRT004` is not added to the `analyzers` slice.

## Q1. Where the baseline lives (snapshot, copy-on-write, memory)

**Decision: retain the derived `checking.Report` by pointer in the publisher. No snapshot of the index, no copy-on-write.**

Why this is safe and sufficient:
- "The index" in the daemon is the Loader parse cache (mutable, mutex-guarded, keyed by path). The Report is a different thing: `indexing.Build` constructs a brand-new `RepositoryGraph` on every `Index` (`build.go:271`, `NewRepositoryGraph`). Graph, `Unit`, `Module`, `Surface`, `Diagnostic` have unexported fields and defensively-cloning getters (`graph.go` `Units()`/`Modules()`, `surface.go` `Variables()`), and cannot import HCL. The previous generation's Report is therefore already an immutable value that the next `Index` never touches.
- Holding it costs a pointer. Do **not** keep the Loader, an `fs.FS`, or parse-cache entries as part of the baseline: that would freeze stale ASTs and pin the large part of memory.

Memory (estimate, not measured, no Go toolchain was available): a unit is a few hundred bytes plus its dependencies, references, positions; call it 1-3 KB per unit including `Diagnostic`s. For the 65-unit primary corpus the baseline is on the order of 100-200 KB; 5,000 units on the order of 10-15 MB. The parse cache (ASTs of every `terragrunt.hcl`, include and `.tf`) dominates and is not duplicated. The publisher then holds two Reports (`base`, `lastRep`) plus the pre-rendered bytes. Verify with a `runtime.MemStats` probe in the daemon phase (before/after `Check` on `synthrepo` at 65, 500, 5000 units); treat that as a verification task, not a design risk.

Baseline semantics:
- Set automatically at the first `EventReady` (the initial index). A torn first read produces config-unknown units and unknown surfaces, which `SurfaceDiff` skips silently (zero false positives), and `rebase` is the repair.
- `rebase` sets `base = lastRep` (latest published Report, which after a failed reindex is the last good one). Idempotent. A rebase while still indexing returns an error ("still indexing").
- In-memory only. Persisting a baseline is out of scope (the project already defers an on-disk index).

Concurrency (the one real hazard): `watchPublisher.publish` is documented as called only from the `watch.Run` goroutine (`watch.go:226`), and `instance.store`/`writeDump` assume one caller at a time (`instance.go:169`). `rebase` arrives on a socket-handler goroutine. Resolution: a single `sync.Mutex` in the publisher guarding `base`, `lastRep` and the `setSnapshot` call; `publish` and `Rebase` both take it. Do the stdout write (`writeOut`) outside the lock so a slow terminal cannot stall a rebase past the 2 s client timeout. Alternative (route rebase into the `watch.Run` select loop via a new `Config` channel) is cleaner for the single-goroutine rule but changes infrastructure/watch for no gain; reject it.

## Q2. IPC protocol extension and versioning

Today: one JSON request line `{v, op}` (`ipc.go:136`), one response line (`ipc.go:142`), ops `ping`/`report` (`ipc.go:29-30`), exact-match version check on both sides (`respond` `ipc.go:188`, `decodeResponse` `ipc.go:217`, `ReadDump` `dump.go:41`). The server never touches the index: it calls `get() *Snapshot` and `Serve(ln, info, get)` (`sock_unix.go:107`).

**Decision**

1. `ProtocolVersion = 2`. Keep the exact-match policy. Rationale: a new client sending `rebase` to a v1 daemon would get "unknown op", while a bump gives the existing, good message ("restart the daemon with the same gruntled"). Tests use 9 and 99 for mismatch (`ipc_test.go:257`, `report_test.go:290`), so the bump breaks no pin. No negotiation: daemon and client are one binary.
2. Ops: add `OpBlast = "blast"` (read-only) and `OpRebase = "rebase"` (in-memory mutation; socket is 0600 inside a checked 0700 dir, so the trust boundary is unchanged and the repository stays read-only).
3. Payload:
   ```go
   type BlastView struct {
       BaselineGeneration uint64 `json:"baseline_generation"`
       HasErrors          bool   `json:"has_errors"`  // Broken has an error finding
       Broken, Impacted   int    `json:"broken","impacted"`
       Text, JSON         []byte `json:"text","json"` // pre-rendered, base64 on the wire
   }
   // Snapshot gains: Blast *BlastView `json:"blast,omitempty"`
   ```
   `respond` strips what the op does not need: `OpReport` returns a copy of the Snapshot with `Blast = nil` (so `report` bytes and size are unchanged); `OpBlast` returns a copy with the four check byte fields nil. Both are cheap struct copies of slice headers; snapshots stay immutable.
4. Server hook: introduce `type Handler interface { Snapshot() *Snapshot; Rebase() (*Snapshot, error) }` and `ServeHandler(ln, info, h)`; keep `Serve(ln, info, get)` as a wrapper whose `Rebase` returns "rebase not supported". This keeps the existing ipc tests untouched. `sock_other.go` (`!linux && !darwin`) needs the matching stub.
5. Windows (`sock_other.go`, report file): the dump already carries the whole Snapshot (`dump.go:14` `dumpFile`), so `report --blast` works there with no extra transport once `Snapshot.Blast` is populated. `rebase` has no transport: print "rebase needs the daemon socket (unix); restart gruntled watch to reset the baseline", exit 3. This matches the existing deferral (AF_UNIX on Windows needs a scoped `net` exception to the six-target proof). Do not invent a request-file protocol.
6. Size: blast bytes join the same line/file under the 64 MiB cap (`ipc.go:44`). Transitive paths are O(units x depth); irrelevant at 65 units, note for very large repos.

CLI surface:
- `gruntled report --blast [--format text|json] [path]`: prints `Blast.Text/JSON`; exit 1 if `HasErrors`, else 0; exit 3 for the same daemon-unavailable cases as `report`. `--blast --format sarif` is exit 2.
- `gruntled rebase [path]`: prints `baseline moved to generation N`; exit 0; 3 if no daemon / indexing / unsupported platform.
- Baseline label in the blast text: `daemon baseline (generation N)`. Never a timestamp: output must be deterministic.

## Q3. GRT004: produced by `check`, by blast only, or reclassifying GRT001?

**Decision: blast views only, as a reclassification of the GRT001 that the current tree already raises.**

- `check`/`report` have exactly one graph, so they cannot know an output was removed. Keeping GRT004 out of `analyzers` (`check.go:47`) preserves the v0.3 invariant "`report` is byte-identical to `check`" and keeps SARIF and the exit-code contract of `check` unchanged.
- In the blast view, GRT001 and GRT004 describe the same site. Emitting both double-reports and double-counts. So `blasting.Between` runs `analysis.RemovedOutputs(base.Graph, cur.Graph)` and `SupersedeUnknownOutputs` replaces the matching GRT001 (match key `(Unit, Pos)`) before `impact.Compute`. GRT001 for an output that never existed in the baseline stays GRT001. Pre-existing GRT001s remain non-Broken as today (`NewFindings` multiset).
- Condition (all must hold; otherwise stay silent and leave GRT001 alone): GRT001 rows 1-6 fail in cur (shared `resolveReference`), **and** the same target unit's module is known in base and declared the output, **and** (recommended) base had the same reference `(unit, dependency, output)`. The third clause is what makes it "still referenced" rather than "newly mistyped"; it is a few lines (a set built from `base.References()`) and trims the cases where GRT004 would mislead. Surface this choice in requirements.
- Severity error; `mock_outputs` handling inherits GRT001's (never suppresses; message suffix reuse is optional).
- Message names output, module and target unit (stable). "Names its consumers" is delivered structurally: `Result.RemovedOutputs []RemovedOutput{Module, Output, Consumers []{Unit, Dependency, Pos}}` (sorted), rendered as a header line per removed output in text/JSON. Consumers are also each Broken through their own per-site diagnostic, so `groupBroken` (`impact.go:152`) and exit codes need no change. Rejected alternative: one diagnostic per removed output anchored at the first consumer; it hides every other consumer from Broken.
- `FindingKey` stability is not at stake: GRT004 never exists in a baseline set, so it is always "new" (the intended behaviour).
- SARIF: `presenter/sarif.go:128-170` has a rule table keyed by code. GRT004 never reaches SARIF in v0.4; do not add a rule (a rule no run can emit is noise). Add a test asserting `check` can never emit GRT004.

## Q4. Type expressions: extraction point and domain representation

**Extraction: `internal/infrastructure/tfsurface`**, the only place that sees `.tf` HCL (rule `hcl-only-in-infrastructure`; the domain allowlist in `scripts/check-architecture.sh:278-292` forbids even `fmt`).

Flow per module file (extends `reader.go:183-197`): after `body.PartialContent(moduleSchema)`, for each `variable`/`output` block call `b.Body.PartialContent` with an optional-attribute schema (`type`, `default`, `sensitive`; blocks like `validation`/`precondition` fall into the "remain" body, so use `PartialContent`, never `JustAttributes`).
- `type` to `typeexpr.TypeConstraintWithDefaults(expr)` (hcl/v2 `ext/typeexpr`; imports only hcl and go-cty/convert; `go-cty` is already a direct dependency; confirm the six-target no-net/no-exec proof stays green in CI). It accepts `optional(T)` and `optional(T, default)`; ignore the returned defaults (a changed `optional` default is a value change, not a type change). Works for `.tf.json` through the HCL JSON expression adapters; needs a dedicated JSON test.
- Render `cty.Type` to a canonical string in `typestring.go` (hand-written walk, not `FriendlyName`, which collapses objects to "object"): `string|number|bool|any`, `list(T)`, `set(T)`, `map(T)`, `tuple([A,B])`, `object({k=T,...})` with keys sorted and `optional(T)` marking. Insensitive to whitespace, comments and attribute order.
- `type` absent on a variable means `any` (known). Parse failure means `TypeConstraint{}` (unknown) for that variable only. The surface stays known, so GRT001 behaviour cannot regress.
- `default` attribute present means `Required = TristateFalse`, absent `TristateTrue` (attribute presence is certain; `default = null` is still a default).
- `sensitive`: reuse the `literalBool` rule (move from `terragrunt/eval.go:75` into `hclconv`); non-literal is `TristateUnknown`.
- The depth/size pre-scan (`hclconv.CheckNativeDepth`/`ReadFileLimited`) already runs before parsing, so `typeexpr` recursion is bounded by the same limits.

**Output "type" (important correction to the milestone wording):** until recently outputs had no type, so "output type changed" was undecidable without evaluating `value` (against the project's structural-only decision). Terraform 1.15.0 (2026-04-29) added an optional `type` argument on `output` (hashicorp/terraform PR #36411). So: output type is `TypeConstraint` that is **unknown when not declared** and compared only when both sides declare it. Output `sensitive` is a literal and is always comparable. Never infer a type from `value`.

**Domain representation:** `TypeConstraint` as an opaque canonical-string value object (zero value = unknown). The only operation required is "did it change", for which string equality over a canonical form is exact. A structural type tree (with widening/narrowing analysis) would cost domain code and tests for no v0.4 feature; the string is a forward-compatible seam (a tree can be parsed from it later). Direction of change is deliberately not judged: Terraform converts values, so "narrowed vs widened" needs conversion semantics. A type change is reported as Impacted (risk), never as an error.

Diff semantics (`SurfaceDiff`, still silent unless both sides are known, preserving the zero-false-positive rule):
- New required variable: name added and `Required == true`. Existing optional variable that became required: `Required` flipped to true on both-known. Both are Impacted facts. They must NOT become diagnostics: that is GRT005, deferred because of include-merge false-positive risk (PROJECT.md).
- Variable type changed: both known and unequal.
- Output sensitivity changed: both known and unequal. Output type changed: both declared and unequal.
- A still-present name with all facts unknown on either side contributes nothing.

## Q5. Transitive impact: deterministic BFS

**Location:** `internal/domain/impact/transitive.go`, pure, called from `Compute` (`impact.go:97`). Signature unchanged for callers.

Model:
- Seeds: the existing direct consumers (units whose module is in `SurfaceDiff`; loop at `impact.go:104-131`), sorted by path. They keep `Path == [unit]`, distance 0 (the v0.3 behaviour).
- Edges (reverse of data flow): from `cur.Edges()` (`graph.go:309`) keep block edges with `Enabled() == TristateTrue` and `SkipOutputs() != TristateTrue` whose target is a unit in the graph; `dependencies { paths }` edges are ordering-only (no outputs flow) and are excluded. Precedent: GRT003 filters on `Enabled() == TristateTrue` (`grt003.go:57`); GRT001 gates on enabled/skip_outputs (`grt001.go` rows 2-3). Build `dependents[target] = sorted, de-duplicated sources`. This keeps "Impacted only when module surface changes" true: a unit is reported only if outputs can flow to it from a unit running a changed module. State this edge rule in the requirements, since it is the main precision lever against over-reporting.
- Traverse through Broken and unknown-status units (they are legitimate intermediaries) but list a unit only if it is not Broken (the disjointness contract at `impact.go:70-78` is preserved).

Algorithm, deterministic by construction:
```go
// level-synchronous multi-source BFS
frontier := sorted(seeds)                 // []RepoPath, RepoPath.Compare order
parent := map[RepoPath]RepoPath{}         // discovery parent; seeds map to themselves
for len(frontier) > 0 {
    var next []RepoPath
    for _, v := range frontier {          // frontier is sorted
        for _, u := range dependents[v] { // sorted
            if _, seen := parent[u]; !seen { parent[u] = v; next = append(next, u) }
        }
    }
    slices.SortFunc(next, RepoPath.Compare) // sort EACH level before expanding it
    frontier = next
}
```
- First discovery wins and each level is expanded in sorted order, so the path is a shortest path and, among shortest paths, the one choosing the lexicographically smallest predecessor at every step. Independent of map order, unit input order and `Edges()` order. Document this tie-break; one path per unit, one root-cause change per unit (the seed on that path), not all of them.
- Cycles (GRT003 can coexist) are handled by the `parent` visited set; self-loops are no-ops. The cost is O(U + E) per Compute. No iteration over maps without sorting; no recursion (deep chains cannot overflow, same discipline as GRT003's iterative Tarjan).
- Output shape: `ImpactedUnit{Unit, Change SurfaceChange, Path []RepoPath, Distance int}`. Path runs seed to unit, e.g. `vpc -> db -> app`, rendered `app (via db <- vpc: -output foo)`. Reconstruct by walking `parent`.
- Output compatibility: `Impacted` now contains units that v0.3 would not list, so consumers that count it change meaning. Bump `blastSchemaVersion` 1 to 2 once for the milestone (`blast.go:15`) and add `path` and `distance` (and the type-level fields) as part of that single bump, not one bump per feature.

## Recommended Project Structure (delta only)

```
internal/domain/
  repograph/typeconstraint.go     NEW  TypeConstraint
  repograph/surface.go            MOD  VariableDecl, OutputDecl, NewDetailedSurface
  analysis/grt001.go              MOD  resolveReference extraction (no behaviour change)
  analysis/grt004.go              NEW  RemovedOutputs, SupersedeUnknownOutputs
  impact/surface.go               MOD  type/requiredness/sensitivity diff
  impact/transitive.go            NEW  Propagate (BFS)
  impact/impact.go                MOD  Compute uses Propagate, Result.RemovedOutputs
  diagnostic/diagnostic.go        MOD  CodeRemovedOutput
internal/application/blasting/    MOD  Between(); Blast delegates
internal/infrastructure/
  tfsurface/reader.go             MOD  attribute decode
  tfsurface/typestring.go         NEW  cty.Type -> canonical string
  hclconv/                        MOD  LiteralBool moved here
  ipc/ipc.go sock_unix.go sock_other.go dump.go   MOD  v2, Handler, ops, BlastView
internal/interfaces/presenter/blast.go            MOD
cmd/gruntled/
  watch.go instance.go render.go report.go main.go   MOD
  rebase.go                       NEW
docs/cli.md docs/ci.md            MOD  (tests pin usage text and doc recipes)
```

No new top-level `internal/<x>` directory (rule `internal-layout`). No new layer dependency: `impact` may import `analysis` only if needed, but the design avoids it (the orchestration is in `blasting`, application layer, which may import domain). `tfsurface` importing `hclconv` is infrastructure-to-infrastructure, already allowed.

## Architectural Patterns

### Pattern 1: Pure core shared by two drivers
**What:** `blasting.Between(base, cur checking.Report)` is the only place that turns two Reports into a `Result`; `blast --base` (loads two trees) and the daemon (holds two Reports) both call it, and both render through the same `presenter.Blast*`.
**When:** Any feature that must agree across CLI and daemon (already how `renderReport` makes `report` byte-identical to `check`, `render.go:16`).
**Trade-off:** One more exported function; in exchange a property test "daemon blast bytes == `blast --base` bytes for the same two trees, modulo the baseline label" is trivially stateable.

### Pattern 2: Immutable derived values, retained by pointer
**What:** Treat `checking.Report` as a persistent value; retain instead of copy.
**When:** The producer rebuilds rather than mutates (true for `indexing.Build`).
**Trade-off:** Relies on the domain staying immutable; add a test that mutating a getter's returned slice does not alter a retained Report (the getters already clone).

### Pattern 3: Pre-render at publish, serve bytes
**What:** Keep the v0.3 invariant (`watch.go:262` `buildSnapshot`: "bytes only, never rep"); add `buildBlastView` next to it.
**When:** Handlers must not touch indexer state, and a no-socket platform must read the same data from a file.
**Trade-off:** Blast is recomputed on every save even if nobody asks: O(units + edges), microseconds to low milliseconds at 65-5000 units. The simpler compute-on-request alternative would need a lock on shared Reports and has no file-transport equivalent on Windows.

### Pattern 4: Unknown is silent
**What:** Every new comparison (type, requiredness, sensitivity, output type) requires both sides known; every new traversal edge requires literal facts.
**When:** Always, per the zero-false-positive constraint.

## Data Flow

### `report --blast` / `rebase` (new)

```
watch.Run goroutine                                      socket handler goroutines
 Index() -> checking.Report (graph+diags)
   -> publish(EventReady)
        lock
        base == nil ? base = report (generation g)
        res  = blasting.Between(base, report)
        view = presenter.Blast{Text,JSON}(res, "daemon baseline (generation g0)")
        snap = buildSnapshot(report) + Blast=view
        setSnapshot(snap)  -> atomic store (+ dump file on windows)
        unlock
                                                          report --blast -> OpBlast
                                                            respond: snapshot copy minus check bytes
                                                          rebase -> OpRebase -> Handler.Rebase()
                                                            lock; base = lastRep; rebuild view; setSnapshot; unlock
```

### blast / GRT004 / transitive (new, inside Between)

```
base Report ─┐
             ├─ analysis.RemovedOutputs ─> GRT004[] ─┐
cur  Report ─┘                                       ├─ Supersede(curD) ─> curD'
                                                     │
impact.Compute(baseG, baseD, curG, curD')            │
   NewFindings -> Broken (GRT004 always new)         │
   SurfaceDiff (names + types + required + sensitive)│
   direct consumers -> seeds -> Propagate (BFS) -> Impacted{Path}
   RemovedOutputs grouping from GRT004[]
```

### Failure behaviour
`EventFailed` keeps the last good snapshot, including its `Blast` view, marked failed (`watch.go:277-282` copies the struct, so the pointer is shared and still immutable). `report --blast` prints the previous result with the existing warning.

## Scaling Considerations

| Concern | 65 units (corpus) | 500 units | 5,000 units |
|---------|-------------------|-----------|-------------|
| Baseline memory | ~100-200 KB (estimate) | ~1-2 MB | ~10-15 MB; parse cache dominates, not duplicated |
| Blast compute per save | negligible | sub-ms to ms | low ms; O(U+E), allocations dominated by sorting |
| Response size | KBs | tens of KB | paths add O(U x depth); well under the 64 MiB cap |
| Existing hotspot (not new) | `ReadSurface` re-reads every module on every `Index` (surfaces are not cached) | same | revisit only if startup/reindex becomes slow; type extraction adds negligible work per file |

First bottleneck is not blast; it is the existing per-Index surface re-read. Do not fold a surface cache into v0.4.

## Anti-Patterns

### Snapshotting the Loader/parse cache as the baseline
**What people do:** Deep-copy or fork the index to "freeze" it.
**Why wrong:** Pins ASTs (the memory hog), reintroduces stale-file bugs the v0.3 audit fixed (symlinked includes, canonical-path validation), and buys nothing since the Report is already immutable.
**Instead:** Retain the Report.

### Emitting GRT004 from `check` or the `analyzers` slice
**Why wrong:** Needs two graphs; breaks `report == check`; would force a fake "no baseline" mode.
**Instead:** Blast-only, superseding GRT001.

### Double-reporting (GRT001 and GRT004 for one site)
**Instead:** Supersede by `(Unit, Pos)` in `Between`.

### Making a missing/new required variable a diagnostic
**Why wrong:** That is GRT005 (deferred, include-merge false positives; overlaps `terragrunt hcl validate --inputs`).
**Instead:** Impacted fact only.

### Inferring an output's type from its `value` expression
**Why wrong:** Violates the structural-only decision; only a declared `type` is comparable.

### Following `dependencies { paths }` or `enabled = false`/unknown edges in transitive impact
**Why wrong:** Ordering-only or non-certain edges inflate Impacted and destroy trust ("twelve units because a comment changed").

### Letting a type-parse failure make the surface unknown
**Why wrong:** Silently disables GRT001 for the module (a regression on the core value). Scope the unknown to the single fact.

### Mutating state from a handler without a lock, or writing the report file from two goroutines
**Instead:** One publisher mutex; stdout writes outside it.

## Integration Points

### Internal boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| tfsurface -> repograph | `ports.SurfaceResult.Surface` (domain type) | Port unchanged; richer value |
| blasting -> analysis, impact | direct calls on Reports | `analysis` and `impact` stay independent of each other |
| cmd/watch -> ipc | `Handler` interface | cmd supplies the publisher; ipc stays generic (bytes + counters, no impact import) |
| publisher -> presenter | `presenter.BlastText/BlastJSON` | The baseline label is a parameter already (`blast.go:78`) |
| report/rebase -> ipc | `ipc.Query(sock, op, timeout)` (`sock_unix.go:257`) | Reuse `queryDaemon` (`report.go:152`); rebase needs the socket branch only |

### Architecture gates to keep green
`scripts/check-architecture.sh`: domain allowlist (no `fmt`; build messages with `strconv`/string concat), application platform-neutral, HCL only in infrastructure (typeexpr/cty stay in `tfsurface`), `infrastructure-importers`, `interfaces-stdlib-allowlist`, and `binary-no-net-no-exec` for six targets (verify `ext/typeexpr` and `cty/convert` add no `net`/`os/exec`).

## Suggested Build Order (with dependencies)

1. **GRT004 + `Between` refactor** (no deps).
   `Between`, `resolveReference` extraction (guard with golden tests that `check` is unchanged), `analysis/grt004.go`, `CodeRemovedOutput`, supersede, `Result.RemovedOutputs`, presenter. Verifiable through the existing `blast --base` CLI before any daemon work, and with the existing mutation corpus (delete/rename an output: expect GRT004, not GRT001, in blast; `check` unchanged). Highest value, smallest surface, and it creates the shared core everything else uses.
2. **Transitive impact** (needs 1 only for the shared `Result`; logically independent).
   `impact/transitive.go`, `ImpactedUnit.Path`, presenter, blast schema to v2. Pure domain; rapid property tests: output independent of unit/edge input order (shuffle), path is shortest, cycle safe, Broken excluded from the list but traversed, only enabled-true block edges.
3. **Type-level surface** (needs nothing from 1-2; its output fields land in the v2 schema from step 2).
   `TypeConstraint`, decls, `tfsurface` attribute extraction + `typestring.go` + `hclconv.LiteralBool` move, `SurfaceDiff` extension, presenter tokens. Test matrix: `.tf` and `.tf.json`, `optional()`, `any`, absent type/default, nested objects with reordered keys (equal), unparsable type (fact unknown, surface known), output with and without `type`, unknown-on-either-side silence.
4. **Daemon baseline, `report --blast`, `rebase`, IPC v2** (needs 1; consumes 2-3 through the shared presenter, so it can start earlier but should merge after the output shape is settled).
   Publisher state + mutex, `Snapshot.Blast`, `Handler`, ops, version bump, dump path, `sock_other.go` stub, `report --blast`, `rebase`, usage/docs, version-mismatch messages. Test: daemon blast bytes equal `blast --base` of the same two trees (modulo label); rebase empties Broken/Impacted; rebase under concurrent publish (`-race`); windows report-file path; `report` bytes still equal `check`.
5. **Cross-phase audit, corpus and docs** (needs all).
   GRT004 mutation run on the three corpora (0 false positives on unmutated, every injected removal caught), blast of a tree against itself is empty, six-target proof, `docs/cli.md`/`docs/ci.md` pins, independent security/audit pass (the v0.3 audit found a BLOCKER and a MEDIUM between phases that per-phase checks missed).

Phase-level research flags: step 4 deserves the most attention (concurrency and the Windows asymmetry); step 3 needs a short spike on `typeexpr` over JSON bodies and on the no-net proof; steps 1 and 2 are standard patterns for this codebase.

## Open Decisions for Requirements

1. GRT004 requires the reference to exist in the baseline (recommended) or only the output to have existed.
2. Transitive edge rule: enabled-true block edges excluding `skip_outputs = true` and `paths` edges (recommended), versus all enabled edges.
3. `rebase` unsupported on Windows (recommended) versus a request-file protocol.
4. Optional `watch --base <dir>` to seed the baseline from a directory instead of the first index (cheap with `Between`; not required by the milestone).
5. Whether `blast`/`report --blast` should offer `--format sarif` (recommended: no, GRT004 stays out of SARIF in v0.4).

## Sources

- Repository code at v0.3.0, read directly: `internal/application/{blasting,checking,indexing,watching,ports}`, `internal/domain/{impact,repograph,analysis,diagnostic}`, `internal/infrastructure/{tfsurface,ipc,watch,terragrunt}`, `internal/interfaces/presenter/blast.go`, `cmd/gruntled/{main,watch,report,instance,render}.go`, `scripts/check-architecture.sh`. HIGH.
- `.planning/PROJECT.md` (constraints, key decisions, deferred GRT005/GRT006). HIGH.
- Terraform output block reference: https://developer.hashicorp.com/terraform/language/block/output (arguments incl. `type`, `sensitive`, `ephemeral`). HIGH.
- Terraform 1.15.0 changelog and PR "Type Constraints for Output Blocks": https://github.com/hashicorp/terraform/pull/36411 , https://github.com/hashicorp/terraform/blob/v1.15/CHANGELOG.md (output `type` introduced in 1.15.0, 2026-04-29). MEDIUM (secondary search summary; confirm exact release note wording before documenting).
- hcl/v2 `ext/typeexpr` source (imports hcl, go-cty, go-cty/convert; `optional()` with defaults): https://github.com/hashicorp/hcl/blob/v2.25.0/ext/typeexpr/get_type.go . MEDIUM (fetched through a summarizing tool; verify exported names against the pinned v2.25.0 in a spike).
- Memory figures: derived from data structures, not measured. LOW-MEDIUM until the MemStats probe in step 4.

---
*Architecture research for: gruntled v0.4 Blast-aware Diagnostics*
*Researched: 2026-10-10*
