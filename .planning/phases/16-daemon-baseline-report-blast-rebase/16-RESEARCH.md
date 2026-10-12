---
phase: 16-daemon-baseline-report-blast-rebase
type: research
created: 2026-10-12
code: master 56c0e39
requirements: [DAEMON-07, DAEMON-08, DAEMON-09, DAEMON-10]
deferred-in: [BLAST-11 report --blast half, SEC-01 report --blast half, T-13-02-7 re-check]
sources:
  - .planning/research/ARCHITECTURE.md (Q1 baseline, Q2 IPC v2, data flow), PITFALLS.md (7, 11, 12, 13, 15)
  - .planning/REQUIREMENTS.md (DAEMON-07..10, BLAST-11, SEC-01, MORE-03), ROADMAP.md phase 16
  - 13-02-SUMMARY (T-13-02-7), 15-SECURITY.md (F1-F5, for-expression cost class), 15-05-SUMMARY (fuzz log), 15-VERIFICATION
  - code at 56c0e39: internal/infrastructure/ipc/*, cmd/gruntled/{watch,instance,report,render,main}.go,
    internal/application/blasting/blasting.go, internal/interfaces/presenter/blast.go,
    internal/infrastructure/watch/run.go, scripts/check-architecture.sh, .github/workflows/ci.yml
  - no experiments were run (reading only, per the environment limits)
---

# Phase 16: Research

## 0. Summary

Everything phase 16 needs already exists in some form. The pieces are an immutable
pre-rendered `ipc.Snapshot`, a publisher that the `watch.Run` goroutine calls, a pure
`blasting.Between`, and presenters shared with `blast`. The work is five things:

1. Add a baseline `checking.Report` and the last published `checking.Report` to the publisher,
   under a mutex.
2. Pre-render a blast view (text and JSON) at every publish, and on every rebase.
3. Bump IPC to v2 and add ops `blast` and `rebase`, with explicit refusals.
4. Add `report --blast` and `rebase` to the CLI.
5. Write the proofs: an equivalence property, escape and bound e2e tests, the compatibility
   matrix, and a memory record.

No new dependency is needed and no new evaluation site is added. The six-target no-net/no-exec
proof is unaffected, because everything stays on `syscall` AF_UNIX and the dump file.

## 1. Daemon state at 56c0e39

### 1.1 The publish path

- `watch.Run` (`internal/infrastructure/watch/run.go:63-176`) is the only indexer goroutine. It
  publishes `EventIndexing` once (before the initial index), then `EventReady{Report}` or
  `EventFailed{Err}`. A reindex result is **not published** while newer changes are pending
  (`run.go:160-167`, the `unpublished` flag). That result may come from a torn read. The
  publisher therefore never sees an unpublished Report. This is the structural basis for
  "rebase installs only the last published report".
- After the initial index, `EventIndexing` is never published again. "Indexing" from the
  client's point of view is therefore exactly "no snapshot yet" (`p.last == nil`,
  `instance.snap.Load() == nil`).
- `watchPublisher` (`cmd/gruntled/watch.go:227-243`) holds `printed`, `ready`, `last
  *ipc.Snapshot` and `gen uint64`, and documents itself as "called only from the watch.Run
  goroutine" (`:225-226`).
  - `publish` (`:245-292`), on `EventReady`, writes the stdout diff (`writeOut`), runs `gen++`
    and calls `buildSnapshot(ev.Report, gen)`, then `setSnapshot`.
  - On `EventFailed` it copies `*p.last` with `State=failed` and a sanitised `LastError`
    (`:277-282`); the copy shares the byte slices.
  - It writes the status line for every event (`:288`).
- `setSnapshot` (`:296-301`) sets `p.last` and calls `inst.store`. `inst.store`
  (`instance.go:170-175`) does `atomic.Pointer.Store` and, in report-file mode, `writeDump`.
  `store` is documented as "Called from one goroutine at a time".
- `buildSnapshot` (`render.go:43-56`) pre-renders text, summary, JSON and SARIF through
  `renderReport`, the function `check` uses (`render.go:16`). That is why `report` equals
  `check`. The snapshot holds bytes only, never the Report.
- `Generation` counts successful indexes (`gen++` only on `EventReady`). A failure does not
  change it.

### 1.2 Where the baseline lives (DAEMON-10)

The baseline is held in the publisher as a `checking.Report` value, next to the last published
`checking.Report`, both under one `sync.Mutex`:

```go
// cmd/gruntled/baseline.go (new) or inside watchPublisher
type daemonBlast struct {
    mu       sync.Mutex
    base     checking.Report // nil Graph until the first EventReady
    baseGen  uint64
    baseFrom string          // "watch start" | "rebase"
    lastRep  checking.Report // the Report of the last EventReady, never of a skipped index
    // p.gen, p.last (snapshot) move under the same mutex
}
```

- `checking.Report` (`internal/application/checking/check.go:18-25`) is `{Graph
  *repograph.RepositoryGraph; Diagnostics diagnostic.Set}`. Both are domain values with
  unexported fields and cloning getters, and they cannot import HCL (ARCH-01). They cannot hold
  a loader, an `fs.FS` or a parse cache. `indexing.Build` builds a fresh graph on every `Index`,
  so the previous generation's Report is immutable and never touched again.
- **Never** store `blasting.Sources`, a `terragrunt.Loader`, `root.FS()` or the `watching.Indexer`
  in the publisher.
- Structural proof (recommended test): a reflection walk over the publisher's baseline field
  type, and over everything reachable from `checking.Report`, that fails on any
  `reflect.Interface`, `Func`, `Chan` or `UnsafePointer` kind, or on a named type from
  `internal/infrastructure`. Today the walk reaches only structs, slices, maps keyed by
  `RepoPath`, strings, ints and bools: `Diagnostic` is code/severity/unit/pos/message,
  `Set{items}`, and `Unit`/`Module`/`Surface` are plain fields. The executor confirms this when
  writing the test. Any future interface field would trip the test, which then needs a
  deliberate edit.
- Memory: base and lastRep are two graphs plus two diagnostic sets. They are the same pointer
  at start and right after a rebase. The parse cache is not duplicated.

### 1.3 Blast view per publish

At `EventReady`, under the mutex:

1. `lastRep = ev.Report`, then `gen++`.
2. On the first ready, `base, baseGen, baseFrom = ev.Report, gen, "watch start"`.
3. `res, err := blasting.Between(base, lastRep)`.
4. The view is `presenter.BlastText(&t, res, label)`, plus the JSON with the daemon baseline
   member (section 4.2), plus `HasErrors = res.HasErrors()`, plus `BrokenErrors` (the number of
   error-severity findings in `res.Broken`, needed by rebase).
5. Snapshot = `buildSnapshot(lastRep, gen)` + `Blast = &view`, then `setSnapshot`.

Text label (escaped by `BlastText` already, `blast.go:136`):
`baseline: daemon generation 1 (watch start), current generation 3`. After a rebase it reads
`baseline: daemon generation 3 (rebase), current generation 3`. No wall-clock time, so the
output is deterministic (Pitfall 11).

`EventFailed` keeps the previous view: it copies the struct, the pointer is shared and
immutable. `report --blast` then prints the previous view with the existing warning
(`report.go:130-136`). This needs no new code beyond copying `Blast` with the struct.

If `Between` errors (only on a diagnostic build error), the view is
`BlastView{Err: "<sanitised>"}`. It is never `nil` in a ready snapshot: see the zero-value
trap in section 2.4.

**Lock discipline.** `writeOut` (stdout) and the status-file write stay **outside** the
mutex, so a slow terminal cannot stall a rebase. `Between` and the blast render run
**inside** it (`base` must not change mid-render). The cost is O(U+E+surface text), in
milliseconds at 5,000 units with small types. With the BLAST-11 4 MiB type it is about
100 ms or more (estimate: a 4 MiB type is rendered and copied a few times). Rebase uses a
10 s client timeout (section 3.2), not the 2 s report timeout.

### 1.4 Rebase

`Rebase(expect *uint64) (RebaseResult, error)` runs on the socket handler goroutine:

1. Lock.
2. `p.last == nil` returns `refused{indexing}`.
3. `p.last.State == failed` returns `refused{failed, LastError}`.
4. `expect != nil && *expect != p.gen` returns `refused{expect_mismatch, current p.gen}`.
   Nothing changes in any refusal case.
5. `accepted := p.last.Blast.BrokenErrors`, the count at that moment, read from the view the
   user could have seen.
6. `base, baseGen, baseFrom = lastRep, p.gen, "rebase"`.
7. Rebuild the view from `Between(lastRep, lastRep)`, which by construction gives
   Broken = Impacted = Changes = empty: `NewFindings` of equal sets is empty, `SurfaceDiff` of
   the same graph is empty, and `RemovedOutputs` of the same graph is empty.
8. Build a new snapshot that copies `*p.last`, with the same check bytes and generation and
   the new `Blast`. `setSnapshot` it, then unlock.
9. Return `{Generation: p.gen, BrokenErrors: accepted}`.

- Serialised with publish by the same mutex. Because the snapshot is replaced, not mutated,
  handlers that are already reading the old pointer are unaffected (`setSnapshot` comment,
  `watch.go:294-295`).
- Installs only the last *published* report: `lastRep` is assigned only in `publish(EventReady)`,
  and `watch.Run` never publishes a skipped result (1.1).
- Writes nothing:
  - no status line (`publish` writes it, `Rebase` does not);
  - no stdout;
  - no dump: rebase is unreachable in report-file mode, because there is no socket there.
    `Rebase` still must not call `writeDump`. Make `Rebase` use `inst.snap.Store` through the
    same `snapshots` callback only in socket mode, or assert `dumpW == nil` in a test.
- Generation is **not** incremented: it counts indexes. `--expect N` compares with the current
  published generation `p.gen`, the number `report --blast` shows as "current generation".
- Never auto-rebase (Pitfall 11).

### 1.5 T-13-02-7 re-check: baseline never contains GRT004

- `Between`'s contract (`blasting.go:74-76`) requires `base` from `checking.Check`.
- The daemon's base is always an `EventReady` Report, and `watching.Indexer.Index`
  (`watching.go:35-42`) returns `checking.Check(ctx, loader, surfaces)` unchanged.
- GRT004 is produced only in `analysis.RemovedOutputs` (`domain/analysis/grt004.go:108`) and
  enters diagnostics only through `SupersedeUnknownOutputs` inside `Between`
  (`blasting.go:84`). That result lives in `impact.Result`, never in a Report.
- Rebase installs `lastRep`, the event Report, **never** `res` or the superseded `curD`.

Risk: a future refactor that stores the superseded set "for reuse". Mitigation tests:

- (a) White-box: publish base, then publish a cur where the view has GRT004. Rebase. Assert
  `base.Diagnostics` holds no `CodeRemovedOutput` and that `report --blast` is empty.
- (b) e2e (Pitfall 15): before the rebase, `report --blast` shows GRT004. After it, Broken is
  empty, while plain `report` still shows GRT001 at that site.
- (c) Optional defence in depth: a test-only assertion helper that scans base for GRT004
  after every publish and rebase in the concurrency test.

## 2. IPC at 56c0e39 and the v2 design

### 2.1 Current v1 (`internal/infrastructure/ipc`)

| Item | Where | Value |
|---|---|---|
| `ProtocolVersion` | `ipc.go:22` | `1`, exact-match both sides, also the dump format version |
| Ops | `ipc.go:29-30` | `ping`, `report` |
| Request | `ipc.go:136-139` | `{"v":int,"op":string}`, one line, lenient `json.Unmarshal` |
| Response | `ipc.go:142-149` | `{v, ok, error?, info?, state?, snapshot?}` |
| Snapshot | `ipc.go:77-86` | `state, last_error?, has_errors, generation, text, summary, json, sarif` ([]byte, base64) |
| `maxRequest` | `ipc.go:42` | 4 KiB, server side (`sock_unix.go:211`); oversize means the connection is dropped silently |
| `maxResponse` | `ipc.go:43` | 64 MiB, client side (`sock_unix.go:283`) and `ReadDump` (`dump.go:47-52`); the error is `ipc: line too long`, which **does not name the cap** |
| Version check | `respond` `ipc.go:188`; `decodeResponse` `ipc.go:217`; `ReadDump` `dump.go:58` | the server checks the version **before** the op, so an unknown future op still gets a version error |
| Socket mode | `sock_unix.go:74` | `chmod 0600` after bind; the dir comes from `statusfile.EnsureRepoDir` (0700, owner-checked: `statusfile/ensure.go:16-31` `CheckDir` refuses symlink, non-dir, any group/other bit, foreign owner) |
| Client dir check | `report.go:86-95` | `CheckDir` on `rt` and its parent before querying, so a foreign dir cannot hand a forged result |
| Server I/O | `sock_unix.go:205-223` | read deadline 2 s, one line ≤ 4 KiB, `respond`, write deadline 2 s, no server-side response cap |
| windows | `sock_other.go` | `Listen`/`Query` return `ErrUnsupported`; `Serve` is inert; the daemon writes the dump (`instance.go:135-137`) and `report` reads it only while the lock is held (`report.go:152-172`) |
| Dump | `dump.go:13-37` | `{v, info, state, snapshot?}`, written atomically through `statusfile.Writer` (0600 file in a 0700 dir) |

### 2.2 v2 shapes (recommended)

```go
const ProtocolVersion = 2
const OpBlast, OpRebase = "blast", "rebase"

type request struct {
    V      int     `json:"v"`
    Op     string  `json:"op"`
    Expect *uint64 `json:"expect,omitempty"` // OpRebase only
}
```

- **Fixed shape.** Decode with `json.NewDecoder(bytes.NewReader(line))`, using
  `DisallowUnknownFields()`, then require that one value is followed by EOF. `expect` on any op
  other than `rebase` is `malformed request`. A number that is negative, fractional or larger
  than 2^64-1 is a decode error and gives `malformed request`. The largest valid request
  (`{"v":2,"op":"rebase","expect":18446744073709551615}`) is 52 bytes, so `maxRequest` stays at
  4 KiB (pin it with a test).
- Note the order: the strict decode must not change the "version first" rule. Decode `v` and
  check it before anything else (decode into a `struct{V int}` leniently first). That way a v1
  or v3 request with unknown fields still gets the version error, not "malformed".

```go
type BlastView struct {
    BaselineGeneration uint64 `json:"baseline_generation"`
    BaselineSource     string `json:"baseline_source"`     // "watch start" | "rebase"
    HasErrors          bool   `json:"has_errors"`          // Broken has an error
    BrokenErrors       int    `json:"broken_errors"`       // error findings in Broken
    Text               []byte `json:"text,omitempty"`
    JSON               []byte `json:"json,omitempty"`
    TooLarge           int64  `json:"too_large,omitempty"` // encoded size when over the cap; bytes then empty
    Err                string `json:"error,omitempty"`     // sanitised Between failure
}
// Snapshot gains: Blast *BlastView `json:"blast,omitempty"`
// response gains: Code string `json:"code,omitempty"` (refusal reason) and
//                 Rebase *RebaseResult `json:"rebase,omitempty"` {Generation uint64; BrokenErrors int}
```

- `respond` strips per op, using struct copies of slice headers:
  - `OpReport` returns a copy with `Blast = nil`. That satisfies DAEMON-10 "report response
    does not carry blast bytes". The response line differs from v1 only in `"v":2`.
  - `OpBlast` returns a copy with `Text/Summary/JSON/SARIF = nil` and the `Blast` kept, so
    `has_errors`, `state`, `last_error` and `generation` travel unchanged.
- The server hook adds `type Handler interface { Snapshot() *Snapshot; Rebase(expect *uint64)
  (RebaseResult, error) }` and `ServeHandler(ln, info, h)`.
  - `Serve(ln, info, get)` stays as a wrapper whose `Rebase` returns a refusal with code
    `unsupported`, so every existing ipc test is untouched.
  - `sock_other.go` gets the matching `ServeHandler` stub and the new query functions returning
    `ErrUnsupported`.
- Refusals are typed: `*RefusedError{Code, Msg}` with codes `indexing`, `failed`,
  `expect_mismatch`, `too_large`, `unsupported` and `malformed`. `decodeResponse` returns it
  for `ok:false` when the version matches. The CLI maps each code to a fixed message.
  `Msg` from the daemon goes through `presenter.SanitizeReason` before printing: a v2 daemon is
  trusted, but the message may echo a path.
- Client API: `Query(sock, op, timeout)` keeps its signature for ping/report/blast. Add
  `Rebase(sock string, expect *uint64, timeout) (Info, RebaseResult, error)`.
  - Internally `query` takes a `request`, so tests can still send v1 (`query(sock, req{V:1})`).
  - Errors that happen **after the request was written** are wrapped as `ErrNoReply`
    (deadline, EOF, reset). The CLI needs this to say "outcome unknown" (DAEMON-08).

### 2.3 Mismatch is always explicit (DAEMON-09)

| Client → daemon | What happens on the wire | Client result |
|---|---|---|
| v2 client, v1 daemon (any op, including `blast`, `rebase`) | v1 `respond` checks `v` first (`ipc.go:188`) and answers `{"v":1,"ok":false,"error":"unsupported protocol version 2 ..."}` | `decodeResponse` sees `r.V != 2` and returns `*VersionError{1,2}`, exit 3 with `gruntled: daemon runs protocol v1, this gruntled speaks v2; restart the daemon` |
| v1 client (v0.3 `report`), v2 daemon | v2 `respond` answers `{"v":2,"ok":false,...}` | v0.3 `decodeResponse` returns `*VersionError{2,1}`, so v0.3 `report` exits 3 with its own message |
| v2 client, v1 dump (windows) | `ReadDump` `d.V != 2` | `*VersionError{1,2}`, exit 3 |
| v2 client `blast`, v2 daemon, ready, **no `blast` member** (zero-value trap) | n/a | `decodeResponse` and `ReadDump` (for `report --blast`) treat a ready or failed snapshot without `Blast` as `malformed response: no blast view`, exit 3, never an empty "Broken (0)" |

The compatibility matrix test (DAEMON-09) lives in `ipc` (`compat_unix_test.go`, linux and
darwin), plus one CLI-level case per row:

1. **Fake v1 daemon × v2 client.** A test server that runs a *frozen verbatim copy* of the
   v0.3.0 `respond`/`request`/`response`. Generate it with
   `git show v0.3.0:internal/infrastructure/ipc/ipc.go` and keep it in a `_test.go` file with
   that provenance noted. Serve it over a raw socket. For each of `report`, `blast` and `rebase`
   from the v2 client, assert `*VersionError{1,2}`, and that no snapshot or view is returned.
   At CLI level: `report`, `report --blast` and `rebase` each exit 3 with an empty stdout and
   the mismatch message.
2. **v2 daemon × v1 request.** Send a raw `{"v":1,"op":"report"}` and `{"v":1,"op":"ping"}` to a
   real `ServeHandler` and assert that the response decodes as `v:2, ok:false` and that the v1
   client view gives `*VersionError{2,1}`. Reuse the existing `query(sock, op, v, ...)` seam.
3. **v1 report dump.** Write a dump with the exact v0.3.0 byte shape `{"v":1,"info":...,
   "state":"ready","snapshot":{...}}` and hold the lock (`holdDump` pattern,
   `report_test.go:231-239`). `report` (every format) and `report --blast` exit 3 with the
   mismatch message and print nothing on stdout.
4. **Plain `report` bytes unchanged.** The existing `TestReportParity`/`TestReportParityClean`
   (both transports) and the goldens pinned since 13-03 (`report_nodaemon.txtar` etc.) stay
   green. Add a raw-socket assertion that the `report` response has no `"blast"` key.
5. Optional but strongest: a script check against a **real v0.3.0 binary** (built offline from
   the tag the way `compare-ref.sh` already does, with `GOFLAGS=-mod=readonly GOPROXY=off`).
   Run v0.3.0 `watch` with HEAD `report`, `report --blast` and `rebase`, then HEAD `watch` with
   v0.3.0 `report`. All exit 3 with the mismatch message. This belongs in the phase gate, not
   in `go test`, because it spawns processes and builds a binary.

### 2.4 The 64 MiB cap (DAEMON-10)

The size of a blast view is O(units + edges + surface text) (BLAST-11). At the BLAST-11 worst
case (5,000-unit chain, 4 MiB type) `blast` prints about 8 MiB of text and somewhat more JSON.
In base64 that is about 25-30 MiB, under the cap. Several modules with multi-MiB type changes
can exceed it.

- **Socket.** In `handle` (`sock_unix.go:215`), after encoding an `OpBlast` response, if
  `len(b) > maxResponse`, answer instead with
  `{ok:false, code:"too_large", error:"blast view is N bytes, over the 64 MiB response cap; run blast --base"}`.
  The CLI prints it and exits 3. Apply this to `OpBlast` only, so plain `report` keeps its exact
  v1 behaviour on an over-cap snapshot (the client-side `line too long`, exit 3).
- **Client.** Wrap `errLineTooLong` so it names the cap
  (`ipc: response over the 64 MiB cap`) for every op. The response is otherwise unchanged.
- **Dump (windows).** The dump carries the whole snapshot, check bytes and the blast view
  together (`dump.go:23-37`). Without care, a large blast view would push the **plain** `report`
  over the cap, which v0.3 would have served: a regression. `WriteDump` therefore encodes once.
  If the line is over `maxResponse` and `s.Blast` has bytes, it re-encodes with
  `Blast = &BlastView{TooLarge: N, ...metadata}`. `report --blast` then exits 3 naming the cap,
  and `report` behaves exactly as in v0.3.
- **Test seam.** Make the cap a package variable used by `handle`, `readLine` callers and
  `WriteDump`, overridable in tests. Tests must **never** allocate 64 MiB on this machine, per
  the OOM rule. Lower the cap to a few KiB and build a view just above it.
- **Per-request memory.** Each handler encodes its own response, up to 64 MiB plus base64.
  Concurrent `report --blast` calls multiply this. Only the daemon's uid can connect (section 6),
  and v1 `report` already has the same property. Accept it (threat row), or optionally
  pre-encode the blast response line once per publish.
- Docs: the "Response cap" bullet (`docs/cli.md:1019-1023`) changes. The `report` response
  no longer depends on blast. The blast view has its own cap. On windows the report file drops
  the blast view first.

## 3. CLI

### 3.1 `report --blast` (DAEMON-07)

- Flags: `--blast` (bool). With `--blast`, `--format` must be `text` or `json`; `sarif` gives
  exit 2 (`invalid --format "sarif" with --blast (want text or json)`).
- `--depth`: `report` defines no such flag today, so the stdlib already gives exit 2
  (`flag provided but not defined: -depth`) with usage. A clearer option is to define a
  `--depth` that always errors with
  `--depth is not supported by report --blast; use blast --base --depth`. This is a planner
  choice: both satisfy "2 for `--depth`". Pin whichever is chosen in `report_usage.txtar`.
- Flow, the same as `runReportWith` up to `queryDaemon`, with op `blast`:
  - over the socket, `ipc.Query(sock, OpBlast, reportTimeout)`;
  - on windows, `ReadDump` and use `snap.Blast`.
- Printing:
  1. Print `snap.Blast.Text` or `.JSON` on stdout, and **no summary on stderr** (`blast` prints
     none).
  2. Print the failed-reindex warning, as `report` does.
  3. Exit 1 iff `Blast.HasErrors`, else 0.
- Exit 3 for: no daemon, indexing, `*VersionError`, `RefusedError{too_large}`, `Blast.TooLarge`
  (dump), `Blast.Err`, a missing `Blast` (malformed), and any query error.
- Read-only: `report --blast` creates nothing, which `TestReportNoDaemonCreatesNothing` already
  covers. Extend it to `--blast`.

### 3.2 `rebase` (DAEMON-08)

- Usage: `gruntled rebase [--expect N] [path]`. Dispatch it in `runCtx` (`main.go:144-163`) and
  list it in `topUsage` (`main.go:50-58`) and in the docs mirror.
- `--expect`: use the same strictness as `parseDepth` (`main.go:410-424`):
  `^[1-9][0-9]*$` that fits `uint64`, else exit 2. Use a flag.Value that records "set", so
  `--expect ""` is exit 2.
- On windows (`deps.goos == "windows"`), or when the query returns `ipc.ErrUnsupported`, print
  `gruntled: rebase needs the daemon socket, which this platform does not have; restart gruntled watch to reset the baseline`
  and exit 3. Do this **after** argument parsing (usage errors stay 2) and **before** any
  filesystem access beyond what `report` does. Create nothing.
- Same root and runtime-dir checks as `report` (`openRepoRoot`, `statusfile.Dir`, `CheckDir` on
  both levels; a missing dir means "no daemon", exit 3). It never calls `EnsureRepoDir` and
  never starts a daemon.
- Success, on stdout:
  `gruntled: baseline moved to generation N; M error findings in Broken accepted` (exit 0).
  Pitfall 15 suggests the count, so the masking is visible. The planner fixes the exact
  wording and pins it.
- Refusals (exit 3, empty stdout):
  - `indexing`: "the daemon is still indexing; try again";
  - `failed`: "the last reindex failed: <reason>; fix it and save, then rebase";
  - `expect_mismatch`: "the daemon is at generation M, not N; nothing changed";
  - no daemon, `*VersionError`, or a `too_large` that cannot apply.
- `ErrNoReply` (timeout or EOF after sending):
  `gruntled: no answer from the daemon within 10s; the baseline may or may not have moved; run gruntled report --blast to check`,
  exit 3.
- Timeout: 10 s (`rebaseTimeout`), longer than `reportTimeout` 2 s, because the daemon may be
  inside a publish render (1.3). The server's own write deadline is 2 s after the handler gets
  the lock, which is fine.
- "Writes no file": a test snapshots the repo tree, the runtime dir (names, modes, sizes,
  mtimes, contents) and the status file before and after a successful rebase and after each
  refusal, and requires them to be identical.

### 3.3 Windows summary

| | linux/darwin | windows |
|---|---|---|
| `report` | socket `OpReport` (view stripped) | dump, view ignored |
| `report --blast` | socket `OpBlast` | the dump's `snapshot.blast`. A failed state shows the previous view and the warning; `TooLarge` gives exit 3 |
| `rebase` | socket `OpRebase` | exit 3 "unsupported", nothing touched. No request file (Pitfall 13, PITFALLS anti-pattern table) |

Tests use the existing `reportGOOS` pattern (`report_test.go:74`), which forces dump mode via
`deps.goos = "windows"` on linux. Windows CI (`test-os`) runs the dump path natively.

## 4. `report --blast` == `blast --base <copy>` (DAEMON-07, SEC-01 and BLAST-11 halves)

### 4.1 Equivalence by construction

`blast --base` (`main.go:316-393`) runs `blasting.Blast`. That is `checking.Check(cur)`, then
`checking.Check(base)`, then `Between(baseRep, curRep)`, then `presenter.BlastText(res,
*base)` or `BlastJSON(res)`. The daemon calls the same `Between` on two `checking.Check`
Reports (1.1, 1.5) and the same presenters. The only intended difference is the label line
(text) and the daemon baseline member (JSON).

Divergence risks the property test must catch:

- (a) **Incremental cache drift.** The daemon's Report comes from an invalidating Loader cache
  plus `ReadSurface`, while `blast` re-parses from scratch. `report` == `check` already pins
  this for a single tree (`TestReportParity`), so the risk is low. The property extends it to
  two trees.
- (b) **Wrong base.** For example, the baseline taken from a skipped torn read, or GRT004 in
  base (1.5).
- (c) **Label leaking** into other lines.
- (d) `--depth`: there is none in the daemon (unlimited), so the property compares with
  `blast --base` without `--depth`.

### 4.2 The JSON "baseline object"

`blastDoc.Baseline` is a **bool** today (`presenter/blast.go:30`). Two options:

- **A (recommended).** Keep `"baseline": true` and add an optional member right after it:
  `"daemon_baseline": {"source": "watch start", "generation": 1, "current_generation": 3}`.
  Use `Daemon *blastDaemon` with `json:"daemon_baseline,omitempty"`, so `blast --base` bytes
  stay identical and the change is additive. The schema `version` stays 2. Add a presenter
  entry `BlastJSONDaemon(w, res, DaemonBaseline)` (or an options struct). `BlastJSON` keeps its
  signature. The property normaliser deletes exactly the lines from `  "daemon_baseline": {`
  to the matching `  },`. It must assert that exactly one such block was removed (non-vacuity,
  the 15-04 lesson).
- **B.** Turn `"baseline"` into an object in daemon output. This changes the key's type
  between two documents of the same `kind`/`version`, and a reader that types it as bool
  breaks. Not recommended.

DAEMON-07's wording ("replacing only ... the baseline object") reads naturally under either
option. **Decision for the orchestrator: D-16-01.**

### 4.3 The property test

- **In-process, many cases** (cheap, `cmd/gruntled`):
  1. Generate a baseline tree D with a seeded xorshift. Use synthrepo or a small hand-made
     generator with modules, outputs, type facts, dependency chains and hostile-free names.
  2. Apply k random mutations to get C: remove or rename an output (GRT004), add a variable,
     change a type or `sensitive`, add a GRT001/GRT002/GRT100, move a line, or delete a unit.
  3. Feed `checking.Check(D)` then `checking.Check(C)` into a test publisher. Use **fresh**
     loaders, plus a third variant that runs C through **the same invalidating loader** as D
     with the dirty paths, which reproduces the daemon cache path (risk 4.1a).
  4. Compare `snap.Blast.Text/JSON` with `run(["blast","--base",D,C])` and `--format json`
     after normalising. Compare exit codes too.
  5. Run about 200 cases under `-short` and more without it. Use a fixed in-test xorshift,
     because cmd tests may use `math/rand`, but determinism matters.
- **e2e through a real daemon, both transports** (`reportGOOS`):
  1. Copy D to W and to `baseCopy`. `startWatch(W, "--poll")`.
  2. Wait for the first report.
  3. Rewrite W to C. Wait until `report --blast` shows `current generation ≥ 2` **and** the plain
     `report` equals `check C`, which proves the daemon has the final tree.
  4. Normalise and compare with `blast --base baseCopy W`, for text and JSON and exit codes.
  5. Then `rebase`. `report --blast` shows `Broken (0)`, `Impacted (0)` and no Type changes
     section. JSON has empty lists. The exit code is 0.
- **SEC-01 report --blast half.** Extend `TestTextOutputsEscapeControls`
  (`escape_test.go:141-325`). Start a daemon on a copy of `craftedRepo(base, true)` and rewrite
  it to `craftedRepo(dir, false)`. Then:
  - `report --blast` text: `assertNoRawControls`, and every `want` line of the `blast` subtest
    except the label line;
  - JSON: the same `anyItem` predicate as `blast json`, decoding back to the raw names;
  - for both: equal to `blast --base` after normalisation.

  Linux and darwin only, like the existing test: control characters in NTFS file names are
  invalid.

### 4.4 Memory and size (BLAST-11 report half, DAEMON-10)

- **BLAST-11 report --blast bound** (`blast_bound_test.go` pattern, linux and darwin, skipped
  under `-short`, **CI only**):
  1. Start the built binary's `watch --poll` on the `boundTree(oldType)` chain (5,000 units,
     about 4 MiB type).
  2. Rewrite `modules/big/main.tf` to `newType`.
  3. Poll `report --blast --format json` until `len(impacted)==5000`.
  4. Fetch text and JSON. Assert the same byte bounds as `TestBlastSizeBound`
     (`rendered + 200·U + 1 KiB`; `2·rendered + 400·U + 1 KiB`), and that the type string
     appears once in the text.
  5. SIGTERM the daemon and read `Maxrss` from `ProcessState.SysUsage()`.
  6. Bound: `rss(big) <= rss(control) + 64·typeBytes`, where the control is the same daemon run
     with a same-size file and a tiny type change (the #249b/#311 in-run control). Log the
     absolute values.

  This also proves the base64 response fits under 64 MiB at the worst BLAST-11 case.
- **DAEMON-10 synthrepo record** at 65, 500 and 5,000 units (`synthrepo.Spec{Units,
  IncludeDepth: 3, DependencyFanout: 3, Seed}`):
  - Primary, deterministic-ish: in process, `runtime.GC()` and `ReadMemStats` (`HeapInuse`)
    after `checking.Check` for one Report, then for a publisher holding base plus lastRep plus
    the snapshot with the view after one edit.
  - Secondary: daemon binary `Maxrss` after an initial index plus one reindex.
  - Record a table in `docs/validation.md` (new "v0.4 daemon memory" section with the
    `VALIDATION-BENCH` machine lines) and in `16-VALIDATION.md`.
  - Assertion, loose and relative only: the publisher's retained heap is at most 2.5 × one
    Report plus the rendered bytes. That catches a pinned parse cache, which is many times one
    Report, without flaking across runners.
  - Skip the 5,000-unit case under `-short`. Locally, run it at most once, alone, with
    `GOMAXPROCS=2` (8 GB WSL).
- The ARCHITECTURE estimate to confirm: about 1-3 KB/unit, so 100-200 KB at 65 units and
  10-15 MB at 5,000.

## 5. The for-expression DoS class (15-SECURITY F1/F2/F5): what it means here

- Phase 16 adds **no** evaluation site. The five guarded `Value`/typeexpr calls are:
  - `LiteralBool`;
  - `typeAttrSig` (native and JSON);
  - `evalPath`;
  - `literalString`.

  They stay the only ones. Each new file (`baseline.go`, `rebase.go`, the ipc changes, the
  presenter member) touches only domain values and bytes. A plan acceptance grep:
  `grep -rnE '\.Value\(|typeConstraint\(' internal --include=*.go | grep -v _test.go | grep -v ':\s*//'`
  gives exactly the four lines it gives at 56c0e39:
  - `tfsurface/typesig.go:161`;
  - `hclconv/literal.go:26`;
  - `terragrunt/eval.go:28`;
  - `terragrunt/eval.go:76`.

  `typesig.go:161` serves both the native and the JSON path.
- The daemon pays the per-file evaluation cost once per save. `ReadSurface` ignores ctx, so
  `watch` cannot cancel it (F1 note). The baseline does **not** add a second parse: it reuses a
  Report, unlike `blast --base`, which checks two trees. The blast view adds O(U+E+surface)
  rendering per publish. That is linear and bounded by BLAST-11, not multiplicative.
- The lesson from F1: plan review checked recursion only, and the cost class got through. For
  phase 16 the cost classes to review explicitly are:
  1. Render cost per publish: linear, measured by 4.4.
  2. Mutex hold time: the rebase latency, bounded by one render, with the 10 s client timeout
     and the "outcome unknown" message.
  3. Per-request encode memory: 2.4, same uid only.
  4. Windows dump rewrite size per save: now check bytes plus view, up to 64 MiB written per
     save at the extreme. Accept and document it, because the dump write is atomic and the file
     is 0600.
  5. Request parsing: 4 KiB, strict decoder, no recursion. `encoding/json` depth on a 4 KiB
     line is trivially bounded.

## 6. Trust boundary and docs

- **Boundary.** The socket is 0600 inside a 0700 runtime dir. `CheckDir` verifies the owner on
  both levels, on the daemon (`EnsureRepoDir`) and the client (`report.go:86-95`). Only the
  daemon owner's uid, and root, can connect. That is the documented trust boundary for the
  first mutating op.
- Peer credentials (`SO_PEERCRED`) exist in `syscall` on linux only. darwin needs
  `getpeereid`/`LOCAL_PEERCRED`, which `syscall` does not expose, and `golang.org/x/sys/unix`
  is only an indirect dependency (via fsnotify). Recommendation: **document only**, as the
  requirement says. A linux-only uid check would add a platform split for no gain over 0700.
  Also note that `rebase` changes only in-memory state of the requester's own daemon: the
  impact of misuse is "my baseline moved", never a file write.
- **Docs that call `report` read-only, or that change** (all at 56c0e39):
  - `cmd/gruntled/report.go:23` usage: "The daemon is only read; report never starts one." It
    stays true for `report`/`report --blast`. Add the `--blast` flag and its exit codes.
  - `report.go:56-57` comment: "It only reads: no directory, file, socket or process is ever
    created." It stays true. Check the wording against rebase.
  - `docs/cli.md:224`: the usage mirror (`TestHelpMatchesDocs`, `e2e_test.go:411-449`, pins the
    exit-code lines; add `{"rebase", N}`).
  - `docs/cli.md:1000-1003`: the **"Read-only." bullet**: "The query never changes the daemon's
    state". This is the sentence DAEMON-08 must change. `report` still doesn't, but the daemon
    now has one mutating op, `rebase`.
  - `docs/cli.md:1010-1018`: protocol text (v2; `rebase` unsupported on windows).
  - `docs/cli.md:1019-1023`: the "Response cap" bullet (2.4).
  - `docs/cli.md:1025-1050`, Guarantees: "No cache" bullet. The daemon keeps a baseline in
    memory only. Add a trust-boundary sentence.
  - `docs/cli.md` "Blast text"/"Blast JSON" sections: the daemon label line and the
    `daemon_baseline` member.
  - A new `## gruntled rebase` section, and `report --blast` in `## gruntled report`.
  - `docs/cli.md:41-58`, top usage mirror: add `rebase`.
  - `README.md:200-204`: `report` paragraph, adding `--blast` and `rebase`.
  - `README.md:214-224`: Windows limitation, adding that `rebase` is unsupported.
  - Pitfall 15 note: after a rebase, a still-dangling reference leaves Broken but plain
    `report` keeps GRT001.
  - `.planning/PROJECT.md` has no "read-only" claim about `report`.
- Usage tests that must move with the docs: `report_usage.txtar`, `usage.txtar` (top help),
  a new `rebase_usage.txtar`, `TestReportUsage` (`report_test.go:20-39`) and
  `TestHelpMatchesDocs`.

## 7. Architecture constraints (scripts/check-architecture.sh, scripts/archscan)

| Rule | Effect on phase 16 |
|---|---|
| domain/application stdlib allowlists (Steps 2, 2b) | no change; `Between` is reused, not modified. The application allowlist is the domain one plus `context` (`check-architecture.sh:270-310`): **no `sync`**, so the mutex-guarded baseline state must live in `cmd/gruntled` (or in infrastructure), not in `internal/application` |
| interfaces allowlist (Step 2c: domain allowlist + fmt, io, encoding/json) | the presenter `daemon_baseline` member needs nothing new |
| infrastructure-importers (Step 7) | only `cmd/...` and `internal/infrastructure/...` import `ipc`; the publisher (cmd) implements `ipc.Handler`; `ipc` must not import application/domain (keep `BlastView` as bytes and ints) |
| hcl-only-in-infrastructure, typeexpr-only-in-tfsurface (6, 6b) | untouched |
| testsupport-only-in-tests (8) | the synthrepo memory test is a `_test.go` file, fine |
| **binary-no-net-no-exec (Step 9), six targets** | `ipc` stays on `syscall` (AF_UNIX) and the dump. No `net`, no `x/sys` import. `sock_other.go` must keep compiling for windows/amd64 and arm64 with the new stubs. Process-spawning tests (v0.3.0 binary compat, BLAST-11 daemon bound) live in `_test.go` or scripts only, as `bench_test.go` does today. Self-tests `binary-net-in-unix-socket-file` and `binary-net-in-windows-ipc-file` keep proving that a `net` import in either ipc build fails |
| platform-neutral rules | apply to domain, application and interfaces only; `ipc` build tags (`linux \|\| darwin`, `!linux && !darwin`) are already allowed |

Gate per plan: `bash scripts/check-architecture.sh` and `bash scripts/test-check-architecture.sh`.
The last plan also runs `compare-ref.sh v0.3.0` (244/244; plain `check`/`report` bytes) and
`go mod verify`, with no go.mod change.

## 8. Fuzz step stall (CI `fuzz FuzzTypeSig`)

Observation: 15-05-SUMMARY:151 shows
`fuzz: elapsed: 1m1s, execs: 136397 (0/sec), new interesting: 211 (total: 382)`, and the
phase-15 verifier saw 0 execs/s after about 3 s while the step stayed green.

Likely cause: the coordinator spends its time **minimizing** new interesting inputs. The
default `-fuzzminimizetime` is 60 s per input, and minimization execs do not count toward the
reported rate. Several seeds are large, and the minimizer works on mutants of them:

- `deepList(5_000)`;
- `"!"x5_000`;
- `nestedIfDefault(1_100)`;
- `"a?b:\n"x1_100` (`typesig_test.go:603-607`).

Each exec on a mutant of these does lexing, a depth scan and parsing of several KB, in two
encodings. With 211 interesting inputs in 60 s, the 30 s window ends while workers are still
minimizing. The step is green, but very little fuzzing happens after the first seconds.

Fix, small enough for one task:

1. In `.github/workflows/ci.yml:56`, add `-fuzzminimizetime 5s`, or count-based `-fuzzminimizetime 200x` for
   runner-independent behaviour, so minimization cannot eat the window.
2. Move the large guard seeds (`deepList(5_000)`, `"!"x5_000`, `nestedIfDefault(1_100)`,
   `"a?b:\n"x1_100`) out of `f.Add` into a table unit test. Their shapes are already pinned at
   larger sizes by `TestTypeSigJSONDeepString` and the hostile-default table
   (`typesig_test.go:398-419, 486, 510`; `facts_test.go:338`), so the moved cases only keep
   these exact sizes asserted. Keep the seeds at or under about 256 bytes,
   plus one ~1,001-level shape just over the cap so the mutator still explores the boundary.
3. Optionally print the final `fuzz:` line by piping through `tail`, to keep the exit code
   (`set -o pipefail`). A later reader can then see execs/s in the log. Do not fail on a rate
   threshold: that flakes.

Verification: there is no local fuzz run (environment rule). CI is the proof: the step log
shows a non-zero exec rate at the last progress line, and the run takes about 30 s. Add a
`TestCIDoc` pin (`cmd/gruntled/ci_doc_test.go`) only if that test already pins the fuzz step.
It does not today (no `Fuzz` match in it).

## 9. Validation Architecture

| Req / item | Proving test(s) | Where | Runs |
|---|---|---|---|
| DAEMON-07 equivalence | `TestDaemonBlastEqualsBlastBase` (in-process property, xorshift, fresh loader and shared invalidating loader) + `TestReportBlastE2E` (real daemon, both transports, text and JSON, exit codes) | `cmd/gruntled/daemon_blast_test.go` | always (small N under `-short`) |
| DAEMON-07 exit codes | `report_blast_usage.txtar` (sarif and depth give 2); `TestReportBlastStates` (no daemon, indexing, mismatch, too_large, missing view give 3; failed gives the previous view plus the warning; 0/1 by `HasErrors`) | `cmd/gruntled` | always |
| DAEMON-07 windows | `TestReportBlastWindowsFile` (`holdDump` with a view; stale gives no daemon; `TooLarge` gives 3) | `report_test.go` | always |
| DAEMON-07 / SEC-01 escape | `TestTextOutputsEscapeControls` subtests `report --blast` and `report --blast json` | `escape_test.go` | linux and darwin |
| BLAST-11 report half | `TestReportBlastSizeBound` (5,000 chain, 4 MiB type, byte bounds, RSS ≤ control + 64·type) | `blast_bound_test.go` | CI only (`-short` skip) |
| DAEMON-08 rebase semantics | `TestPublisherRebase` (indexing/failed/expect refusals change nothing; success: generation, BrokenErrors, empty view, label `(rebase)`; no status write) | `watch_test.go` / `baseline_test.go` | always |
| DAEMON-08 serialisation | `TestRebaseConcurrentPublish` (N publishes ∥ M rebases; invariants: baseline ∈ published set, baseGen ≤ gen, view consistent with (base,last), final rebase gives empty) | `baseline_test.go` | `-race` in CI |
| DAEMON-08 only last published | `TestRebaseNeverSkippedReport` (`watch.Run` with fake Indexer/Watcher: a skipped-while-pending report is never the baseline) | `cmd/gruntled` | always |
| DAEMON-08 no writes / no start | `TestRebaseWritesNothing` (repo, runtime dir, status file byte and metadata identical; no daemon gives 3 and creates nothing) | `cmd/gruntled` | always |
| DAEMON-08 CLI | `rebase_usage.txtar` (`--expect` validation gives 2); `TestRebaseWindowsUnsupported` (3, nothing created); `TestRebaseTimeoutUnknown` (fake server reads, never answers: "may or may not", 3) | `cmd/gruntled` | always |
| DAEMON-09 v2 + matrix | `TestCompatV1DaemonV2Client`, `TestCompatV2DaemonV1Request`, `TestCompatV1Dump` (ipc and CLI rows); `TestRespond` v2 table (strict decode, expect on wrong op, unknown field, version-first); `TestDecodeResponseNoBlastView` | `internal/infrastructure/ipc`, `cmd/gruntled` | always (socket parts linux and darwin) |
| DAEMON-09 plain report unchanged | existing `TestReportParity*`, goldens, `compare-ref.sh v0.3.0` 244/244; new `TestReportResponseHasNoBlast` | both | always and gate |
| DAEMON-10 Report only | `TestBaselineHoldsOnlyReport` (reflection kind walk) | `cmd/gruntled` | always |
| DAEMON-10 64 MiB cap | `TestBlastResponseCap` (socket: lowered cap gives `too_large` naming the cap; plain report unaffected); `TestWriteDumpDropsOversizeView`; client `line too long` names the cap | `ipc` | always (seam, no 64 MiB allocs) |
| DAEMON-10 memory | `TestDaemonMemorySynthrepo` (65/500/5,000, logged, loose relative bound) + `docs/validation.md` table | `cmd/gruntled` | 5,000 only without `-short` |
| T-13-02-7 | `TestRebaseBaselineNoGRT004` (white-box plus e2e Pitfall 15: GRT004 before, Broken empty after, `report` still GRT001) | `cmd/gruntled` | always |
| Docs | `TestHelpMatchesDocs` (+ rebase), usage txtars, grep gate on "Read-only." bullet rewritten | `cmd/gruntled` | always |
| Fuzz step | CI log of `fuzz FuzzTypeSig` with non-zero final rate; seeds-moved unit test | CI | CI |
| Arch | `check-architecture.sh` (six targets), `test-check-architecture.sh`, Value-site grep (5) | scripts | every plan |

Sampling: every plan runs its package tests with `-run` and `-count=1`. The last plan runs
`go test ./cmd/gruntled/ ./internal/infrastructure/ipc/ -short` locally and the full suite in
CI. Do not run `go test ./...` or `-race` locally.

## 10. Candidate plan breakdown

| Plan | Wave | Depends | Scope | Files (main) |
|---|---|---|---|---|
| **16-00** CI fuzz step | 1 | none | section 8: `-fuzzminimizetime`, move large seeds to a unit test | `.github/workflows/ci.yml`, `internal/infrastructure/tfsurface/typesig_test.go` |
| **16-01** IPC v2 | 1 | none | `ProtocolVersion=2`; strict fixed-shape request with `expect`; ops `blast`/`rebase`; `BlastView`, `RebaseResult`, `RefusedError` codes; `Handler`/`ServeHandler` (Serve wrapper); per-op stripping; zero-value guard; server-side `too_large` for `OpBlast`; `WriteDump` downgrade; cap named in client errors; `ErrNoReply`; `sock_other.go` stubs; compat matrix in ipc | `internal/infrastructure/ipc/{ipc,dump,sock_unix,sock_other}.go`, `ipc_test.go`, `sock_unix_test.go`, new `compat_unix_test.go` (+ frozen v0.3.0 respond) |
| **16-02** Daemon baseline and view | 2 | 01 | publisher mutex, base/lastRep/baseGen/source; `buildBlastView` in `render.go` (`Between` + `BlastText` label + `BlastJSONDaemon`); `Rebase`; instance wiring to `ServeHandler`; presenter `daemon_baseline` member | `cmd/gruntled/{watch,instance,render}.go`, new `cmd/gruntled/baseline.go` (optional), `internal/interfaces/presenter/blast.go` (+ test), `cmd/gruntled/watch_test.go`/`baseline_test.go` |
| **16-03** CLI and docs | 3 | 02 | `report --blast`, `rebase` command, dispatch, top usage, exit codes, windows unsupported, timeout message, no-writes test; docs (section 6) | `cmd/gruntled/{report,main}.go`, new `rebase.go`, `report_test.go`, new `rebase_test.go`, `testdata/script/{report_usage,usage}.txtar` + new `rebase_usage.txtar`, `e2e_test.go` (`TestHelpMatchesDocs`), `docs/cli.md`, `README.md` |
| **16-04** Proofs and gate | 4 | 03 | DAEMON-07 property (in-process and e2e), SEC-01 escape `report --blast`, BLAST-11 report bound (CI), DAEMON-10 memory record, T-13-02-7 e2e, optional v0.3.0-binary compat script, phase gate | new `cmd/gruntled/daemon_blast_test.go`, `escape_test.go`, `blast_bound_test.go`, new `daemon_mem_test.go`, `docs/validation.md`, optional `scripts/compat-v1.sh` |

Plans 16-02 and 16-03 could be merged under `coarse` granularity. Keeping them apart keeps each
plan at three tasks or fewer and lets sec review the state machine apart from the CLI and docs.

### Threats per plan (STRIDE seeds for the threat models)

- **16-00:**
  - T: CI step weakened. Moving the seeds must not drop guard coverage, so the moved seeds
    become asserted unit cases.
  - D: none new.
- **16-01:**
  - **Version confusion and zero-value trap**: a missing blast view must never read as an empty
    view (2.3).
  - Request-shape abuse: unknown fields, `expect` on the wrong op, huge numbers, trailing
    garbage, all within 4 KiB; version checked first.
  - DoS: response size (`too_large`), per-request encode memory (accept, same uid), oversize
    or slow request (unchanged drop and deadline).
  - Dump: an oversize view must not break plain `report` on windows.
  - Spoofing: a forged socket or dump is still prevented by `CheckDir` (unchanged).
  - Info leak: refusal messages are sanitised.
- **16-02:**
  - **Race and torn state** between rebase and publish, guarded by the mutex.
  - A skipped torn report installed as the baseline: `lastRep` only from `EventReady`.
  - GRT004 in the baseline (T-13-02-7).
  - Baseline pinning a loader or FS (memory): the reflection test.
  - Lock held during stdout or status writes: kept outside.
  - Deadlock with `Server.Close`.
  - A presenter label or member that is not escaped: digits only, but still through
    `escapeTerm`/`escapeJSON`.
- **16-03:**
  - Rebase from a foreign user: the trust boundary is documented, plus 0600/0700/owner checks.
  - Rebase silently hiding errors: the accepted count is printed.
  - A user rebasing past an unreviewed save: `--expect` CAS.
  - Timeout misread as failure or success: the "unknown outcome" message.
  - Windows silent no-op: exit 3.
  - Rebase writing files or starting a daemon: the no-writes test.
  - Usage and exit-code drift from the docs: `TestHelpMatchesDocs`.
- **16-04:**
  - A vacuous property: the normaliser must remove exactly one label line or one member, a
    counter must confirm that cases had non-empty Broken, Impacted and Changes, and a mutant
    must be caught (for example, the daemon using its label in other lines, or `base` after a
    skipped report).
  - Bound tests flaking: in-run controls and logged absolutes.
  - The OOM risk on dev machines: heavy tests are `-short`-skipped and run in CI only.

### Risks

1. **Equivalence under the incremental cache (4.1a).** If daemon and `blast` disagree, the root
   cause is in the Loader cache, not in phase 16. Budget time for that case. `report` == `check`
   already holds, so this is unlikely.
2. **Lock hold during large renders.** Rebase latency grows with the view. The 10 s client
   timeout and the "unknown" message cover it.
3. **Windows dump growth.** Every save rewrites check bytes plus the view. This is a
   performance cost only; the downgrade covers the cap.
4. **e2e flakiness** on poll-based daemons with multi-step edits. Use `eventually` on a
   *conjunction*: the generation advanced **and** `report` equals `check C`.
5. **Memory tests on 8 GB WSL.** Keep 5,000-unit and 4 MiB runs CI-only and run them one at a
   time.
6. **Frozen v0.3.0 respond copy drifts.** Generate it with `git show v0.3.0:...` and record the
   blob hash in the test comment. The optional script against the real binary is the stronger
   proof.
7. **JSON baseline object (D-16-01)** must be settled before 16-02, because it shapes the
   presenter and the property normaliser.

## 11. Decisions to confirm (orchestrator)

- **D-16-01**: JSON uses an additive `"daemon_baseline": {source, generation,
  current_generation}` and keeps `"baseline": true` (option A, 4.2).
- **D-16-02**: Text label `baseline: daemon generation G0 (watch start|rebase), current generation G`.
- **D-16-03**: The blast view is pre-rendered at publish (both formats) and stored in the
  snapshot. The `report` response strips it, the `blast` response strips the check bytes, and
  the dump carries both, with the oversize view downgraded to `too_large`.
- **D-16-04**: The cap applies server-side to `OpBlast` only. Plain `report` keeps its v1
  failure mode, and the client error now names the cap.
- **D-16-05**: Rebase does not increment the generation. `--expect` compares with the current
  published generation. Output reports that generation and the error findings in Broken
  accepted.
- **D-16-06**: The rebase client timeout is 10 s, and `ErrNoReply` gives "outcome unknown"
  (exit 3).
- **D-16-07**: Trust boundary: document only. No `SO_PEERCRED` (darwin needs x/sys).
- **D-16-08**: `report --blast --depth`: a stdlib unknown-flag (exit 2), or a dedicated message.
  This is the planner's choice, pinned in the txtar.
- **D-16-09**: The fuzz fix (16-00): `-fuzzminimizetime` plus moving the large seeds into a unit
  test.

## 12. Open questions

1. D-16-01 (JSON shape). The requirement text accepts both readings. Option A is recommended.
2. Should `report --blast` print, in text, the "N errors existed at baseline" line that
   Pitfall 15 suggests? It would add a second differing line and break DAEMON-07's "only the
   label line differs" property. Recommendation: put it in the label line only if at all, for
   example `... current generation 3; 2 errors at baseline`. Or leave it out, since `rebase`
   already prints the accepted count.
3. Should the optional v0.3.0-binary compat script be a hard phase-gate item? It is the
   strongest DAEMON-09 evidence, at the cost of an offline tag build. Recommendation: yes, in
   the 16-04 gate, as compare-ref already does.
4. Should `report` with an over-cap snapshot also get the server-side `too_large` code? It would
   be a clearer message, but plain `report` behaviour would change. The recommendation is no
   (D-16-04).
