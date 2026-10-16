# Architecture Research: v0.5 Adoption & Completeness

**Domain:** Go static analyzer + daemon for Terragrunt repositories (hexagonal, no `net`, no `os/exec`)
**Milestone:** v0.5 (subsequent milestone; integration of NEW features only)
**Researched:** 2026-10-16
**Method:** read the code at HEAD `ca4f07f`, plus `go list -deps` / `go doc` probes on the local Go 1.27.2 toolchain and the pinned `golang.org/x/sys v0.47.0`.
**Overall confidence:** HIGH for everything read or probed in this repo; MEDIUM for Windows runtime behaviour (cannot be executed from this Linux/WSL host) and for the Homebrew/GHCR mechanics (from knowledge, not probed).

---

## Headline findings (read these first)

1. **`golang.org/x/sys/windows` cannot be the named-pipe route.** Its `syscall.go` and `types_windows.go` import `net`. `GOOS=windows go list -deps golang.org/x/sys/windows` lists `net`, `net/netip` and `vendor/golang.org/x/net/dns/dnsmessage`. Linking it fails `check-architecture.sh` Step 9 half 1 (`net` forbidden) *and* half 4 (x/sys/windows forbidden on windows) at once. That is not a "scoped exception" - it would delete the proof. The stdlib `syscall` package on windows has `CreateFile`, `ReadFile`, `WriteFile`, `CancelIoEx` but **no** `CreateNamedPipe`/`ConnectNamedPipe`. The only proof-compatible routes are (a) `syscall.NewLazyDLL("kernel32.dll")` for three procs behind a *new, narrower* source-scan rule, or (b) a file mailbox needing no new syscalls. See "Windows live report/rebase".
2. **`text/template` and `html/template` link `net/url` and `net/netip`** (probed). Step 9 half 1 forbids `net/.+`. Completion scripts and the man page MUST be built with `strings.Builder`/`fmt`, never templates. `golang.org/x/term` (for TTY detection) pulls x/sys/windows on windows and has the same problem; use `os.Stdout.Stat()` + `ModeCharDevice`.
3. **Adding GRT004 to the shared SARIF rule table would change `check --format sarif` bytes** (the `rules` array is always emitted in full, and `GRT100`'s `ruleIndex` would shift from 3 to 4). Blast needs its own rule list that *extends* the check list by appending, so check output stays byte-identical.
4. **Polling the instance lock from a prompt is hazardous on unix.** `ipc.Probe` takes the exclusive `flock` briefly; a `watch` starting at that instant gets `ErrHeld` and exits 0 with "already watching" while nothing is watching (unix `LockRetry` attempts = 1; only windows retries 3x). A frequent reader (`status` for a shell prompt) must liveness-check by pinging the socket, not by probing the lock.
5. **`TestCIDoc` hard-codes action SHAs** (`ciPins`). Automated dependency updates (Dependabot) will turn every `actions/checkout`/`setup-go`/`upload-sarif` bump red unless the test derives its expectations from `ci.yml` instead of constants.
6. **Everything except the Windows pipe is proof-neutral and check-neutral by construction.** The risky 10 percent is one phase; build it with a spike gate and keep it away from the other work.

---

## Existing architecture (as read)

```
cmd/gruntled (composition root; stdlib flag; no analysis logic)
  main.go     runCtx switch -> run{Check,Graph,Blast,Watch,Report,Rebase}; topUsage/xUsage consts;
              `version`/`commit` package vars set by -ldflags -X
  render.go   renderReport(format, rep)  <- shared by check AND the daemon snapshot (byte identity by construction)
              buildSnapshot / buildBlastView (daemon pre-renders bytes; snapshot holds bytes only)
  watch.go    runWatchWith, watchPublisher (status file, stdout, snapshots, BASELINE: base/baseGen/baseSource/hasBase)
  instance.go acquireInstance: statusfile.EnsureRepoDir -> ipc.LockRetry -> ipc.Listen (socket) | dump fallback
  report.go   runReportWith, queryDaemon(rt, goos, op)   (goos string seam; ErrUnsupported fallthrough)
  rebase.go   runRebaseWith (refuses on goos == "windows")
          |
internal/interfaces/presenter   text, json, sarif, graph, blast(text/json), status, summary, escape
internal/application            checking, blasting (Blast, Between), indexing, watching, ports
internal/domain                 repograph, diagnostic, analysis, impact
internal/infrastructure         terragrunt, tfsurface, hclconv, sourceresolve, watch, statusfile,
                                ipc (ipc.go protocol v2; sock_unix.go; sock_other.go; dump.go; lock_{unix,windows,other}.go)
scripts/check-architecture.sh   Step 9 binary-no-net-no-exec on 6 targets (4 halves) + test-check-architecture.sh self-tests
```

Facts that constrain the new work:

- **Status file** is `<base>/gruntled/<hash12>/status`, one line `gruntled: <body> @ HH:MM:SS` (local time, no date, no pid). The lock (`lock`), socket (`sock`) and windows dump (`report`) live in the *same directory*, always derived from `statusfile.Dir(root)` even when `--status-file` overrides the status path (`instance.go` comment). `docs/cli.md` says: "A daemon that crashed or was killed with SIGKILL leaves its last line behind; nothing marks it stale yet."
- **`report` is already liveness-correct** (`report.go` `queryDaemon`): unix = dial the socket (ENOENT/ECONNREFUSED -> `ErrNoDaemon` -> "no watch daemon is running", exit 3, so a stale socket file after SIGKILL is harmless); windows = `ipc.Probe(lock)` then read the dump only while the lock is held. It never reads the status file. Only the *status file consumers* lack liveness.
- **There is no formal daemon-transport port.** The seam is the *package-level API* of `ipc`: `Listener`, `Server`, `Listen`, `Serve`, `ServeHandler`, `(*Server).Close`, `Query`, `Rebase`, plus the `Handler` interface (`Snapshot()`, `Rebase(expect)`). `sock_unix.go` (`linux || darwin`) implements it; `sock_other.go` (`!linux && !darwin`) is the stub whose signatures *define* the contract and return `ErrUnsupported`. `cmd/gruntled` already reacts to `ErrUnsupported` (`acquireInstance`, `queryDaemon`) and *additionally* gates on `deps.goos == "windows"` (`instance.go:91`, `report.go:240`, `rebase.go:68`); tests rely on that string to simulate windows on linux.
- **Baseline** is `checking.Report` only (`watchPublisher.base`), seeded in `commitReady` from the first `EventReady` and moved by `rebase`. `blasting.Between(base, cur checking.Report)` is the shared pure core; `blasting.Blast(ctx, cur, *base Sources)` loads both trees with independent loaders. The `blast --base` loader path is `terragrunt.NewLoader(baseRoot.FS())` + `tfsurface.NewReader(...)` (`main.go` `runBlast`).
- **Frozen guards that protect `check` byte-identity** (any change that moves a check byte fails one of these): `golden_test.go` + `testdata/golden/*.txtar`, `testdata/script/{text,json,sarif}_golden.txtar`, `presenter` tests, `scripts/compare-ref.sh <ref>` (byte-compares `check`/`graph` against a built reference tag, optionally on real corpora), `scripts/corpus-gate.sh` (network-isolated, frozen v0.3.0 reference), `testdata/blast_v1` + `scripts/blast-snapshots.sh` (frozen blast outputs), `scripts/compat-v1.sh` (protocol mismatch matrix).

---

## 1. `blast --format sarif`

### How `check` SARIF is wired today
`presenter.SARIF(w, g *repograph.RepositoryGraph, diags diagnostic.Set, tool ToolInfo)` (`presenter/sarif.go`):
- `sarifRuleTable` = 4 `sarifRuleDef` in code order (GRT001, GRT002, GRT003, GRT100). `ruleIndex` of a result is the *position in this table*; the whole `driver.rules` array is emitted every time. `ruleIndexOf` errors on an unknown code (pinned by `TestSARIFUnknownCodeErrors`) - this is the safety net that guarantees `check` can never emit GRT004.
- URIs: `sarifArtifact(path)` = percent-encoded repo-relative path + `uriBaseId: %SRCROOT%`. Relative to the analysed directory; CI prefixes with `jq` (documented decision).
- No `partialFingerprints`, no timestamps, no absolute paths. Notifications: `note`-level `toolExecutionNotifications` for non-resolved units (location = `unitConfigFile`) and unknown-surface modules (no location). Always `executionSuccessful: true`.
- Rendered through `renderReport("sarif", rep)` in `cmd/gruntled/render.go` (shared with the daemon snapshot).
- Rule `helpUri` anchor = `githubAnchor(code + ": " + title + " (" + severity + ")")`. The docs heading for GRT004 is `### GRT004: dependency output removed (error, blast only)`, so the builder's anchor would be `...-error` instead of `...-error-blast-only` - **the rule def needs a heading-severity string field** (e.g. `headingSeverity: "error, blast only"`).

### Where blast rendering lives
Not in `render.go` for the one-shot path: `runBlast` in `main.go` calls `presenter.BlastJSON` / `presenter.BlastText(&buf, res, *base)` inline. `render.go` only holds the *daemon* path (`buildBlastView`: `presenter.BlastText` + `presenter.BlastJSONDaemon`). Both consume `impact.Result` (`Baseline`, `Broken []BrokenUnit{Subject, Findings, Reach}`, `Impacted []ImpactedUnit{Unit, Change, Reach}`, `Changes`).

### What is needed

| Item | New / Modified | Where | Notes |
|------|----------------|-------|-------|
| `presenter.BlastSARIF(w, g *repograph.RepositoryGraph, res impact.Result, tool ToolInfo) error` | **New** | `internal/interfaces/presenter/blast_sarif.go` | Reuses `sarifLog`, `sarifResult`, `sarifArtifact`, `unitConfigFile`, `escapeJSON`; stdlib only (interfaces allowlist: fmt, io, encoding/json + domain allowlist). |
| Blast rule list | **New** (alongside, not editing) | same file | `blastRuleTable = append(slices.Clone(sarifRuleTable), grt004Def)` -> GRT004 at index **4**; GRT001/2/3/100 keep indexes 0-3. `SARIF` (check) keeps using the 4-entry table untouched. |
| `sarifRuleDef` gains `headingSeverity` | **Modified** (additive; zero for existing rows -> existing anchors unchanged) | `sarif.go` | Needed for the GRT004 anchor. |
| Results from `res.Broken[*].Findings` | New code | | One result per finding; level from `Severity()`; `ruleIndex` via the blast table. Position = finding position in the *current* tree. `Reach` (distance/via of a traversed Broken unit) is dropped in SARIF; it stays in JSON. |
| Impacted units | New code | | **Recommendation: `note` tool-execution notifications, not results.** Impacted has no `diagnostic.Code`, never changes the exit code, and results at `note` level become code-scanning alerts (noise on a PR, exactly what PROJECT.md forbids: "reporting twelve units because a comment changed destroys trust"). A notification carries message `unit "<u>" is impacted (distance D, module M, path a -> b -> c)` and location = `unitConfigFile(unit)`, no region. Escape with the same escaping as other messages. |
| Graph for unit notes | **Modified (additive)** | `application/blasting` | `Blast` returns only `impact.Result`; the SARIF writer needs the *current* graph for the "unit unknown" notes that `check` SARIF has. Add `blasting.BlastDetailed(ctx, cur, base) (Detail{Result, Cur checking.Report}, error)` and re-implement `Blast` as a thin wrapper (signature unchanged -> no churn in the many callers/tests). Alternative with less surface: omit unknown-unit notes in blast SARIF and document it; I recommend the additive function so the two SARIF flavours agree. |
| "no baseline" | New code | | Emit one `note` notification `no baseline: every finding is Broken`, mirroring the JSON `note` field. |
| `runBlast` format switch | **Modified** | `main.go` | `text`/`json`/`sarif`; usage text, error message, `blastUsage`, `docs/cli.md` mirror (`TestRuleRegistryDoc` pins a "blast usage mirror"), exit-code line counts (`TestHelpMatchesDocs` expects 4 for blast; unchanged if no new exit code). |
| Format enumeration | Modified | | Put `[]string{"text","json","sarif"}` per command in one place (see CLI command table) so validation and completions cannot diverge. |
| Tests/docs | Modified | `sarif_test.go` `TestSARIFDoc` (expects docs rule headings == emitted rules; GRT004's heading carries ", blast only" so the docs regex must continue to exclude it from the *check* table, and a new assertion covers the blast table), `rules_doc_test.go` (comment says "SARIF table (no GRT004 rule)"; `docs/cli.md:1233` "GRT004 has no SARIF rule: blast has no SARIF output" must be rewritten), `schema` CI step (`sarif-schema` job validates `check` output; add a blast run with the same `jv` call). |

**Deliberately out of scope:** `report --blast --format sarif` (daemon). It would add a `SARIF []byte` to `ipc.BlastView`, grow every blast response toward the 64 MiB cap, and need an explicit "daemon too old" refusal (a missing field must never read as empty). `report` keeps refusing sarif with `--blast` (exit 2, already documented). Revisit after v0.5.

**Data flow:** `runBlast` -> `blasting.BlastDetailed` -> `(Result, Cur.Graph)` -> `presenter.BlastSARIF` -> stdout. No new I/O, no new dependency.

**Effect on the proof:** none (stdlib only; presenter allowlist unchanged). **Effect on `check` byte-identity:** none *provided* the check table is not edited; `TestSARIF*`, `sarif_golden.txtar`, `compare-ref.sh` fail loudly if someone appends GRT004 to the shared table. **Pitfall to guard:** do not "simplify" by adding GRT004 to `sarifRuleTable`; add a regression test that `check` SARIF `rules` has length 4.

---

## 2. Status-file liveness

### What exists
Status line: `gruntled: ok @ 14:02:11` / `N error(s) (GRT001×1 ...)` / `indexing...` / `failed (...)` / `stopped`. `statusfile.Writer.Write` = temp file + rename in a 0700 dir. Producers: `watchPublisher.publish` (one write per event; none on a timer). The runtime dir also holds `lock` (flock / share-mode-0 handle: OS releases on any death), `sock` (unix), `report` dump (windows). `ipc` doc: "The lock is the only source of truth for 'a daemon runs'."

On a dead daemon (SIGKILL): lock free, socket file left (dial -> ECONNREFUSED), status line left. `report` correctly says "no daemon"; a `cat status` reader shows a stale `ok`.

### Options

| Option | Mechanism | Verdict |
|--------|-----------|---------|
| **A. `gruntled status [path]` reader command (recommended, must-have)** | Resolve runtime dir (`openRepoRoot` + `statusfile.Dir` + `CheckDir`, same as `report`). Liveness = `ipc.Query(sock, OpPing)` on unix (and on windows once the pipe exists); fall back to `ipc.Probe(lock)` only where no socket/pipe exists. Alive: print the status line (path from `Info.StatusPath` so a custom `--status-file` is honoured, through `presenter.SanitizeReason`), exit 0. Dead: print `gruntled: not running` (+ optionally `(last: ...)`), exit 3 (same meaning as `report`/`rebase` "no daemon"). | No format change, no heartbeat, no new wakeups, no net/exec. Shell prompt/tmux/editor recipes call this. Costs one process spawn per refresh (~ms). |
| B. Embed pid/epoch in the line | `gruntled: ok @ 14:02:11 pid=123` | Breaks the documented, test-pinned line contract (`presenter/status_test.go`, docs) and gives raw readers `kill -0`, which has pid-reuse races. Reject. |
| C. Heartbeat (mtime touch) | `watch.Config` gains `Heartbeat time.Duration`; `Run`'s select gets a ticker case calling `publish` -> `Writer.Write` (or `os.Chtimes`) every ~30 s; stale = mtime older than ~3x. | Only option that helps *plain `cat`/`find -mmin`* readers. Adds a periodic wakeup and one atomic write per interval to the daemon; changes `watch.Run` (tested with fakes). Does not affect `check`/snapshots. **Make it optional/phase-2 inside the daemon phase**, decide after A ships. |
| D. Lock probe as the liveness test | `ipc.Probe` | **Do not use for anything polled.** On unix `Probe` takes the exclusive flock; a starting `watch` that races it sees `ErrHeld` and exits 0 "already watching" (unix `LockRetry` attempts = 1, `instance.go`). Cheap hardening: raise unix `attempts` from 1 to 3 like windows. |

**New / modified:** `cmd/gruntled/status.go` (**new**, composition root only); `cmd/gruntled/main.go` dispatch + `topUsage` (modified; `docs/cli.md` fenced block and `TestHelpMatchesDocs` table must change in the same commit); `docs/cli.md` "Status file" section (replace "nothing marks it stale yet"); `docs/integrations.md` or a section (recipes, see below). `ipc`/`statusfile` need no change for option A (ping already returns `Info{PID, Version, Root, StatusPath}`).

**What `report` does when the daemon is gone:** unchanged and already correct (exit 3, "no watch daemon is running for <root>"). The new `status` shares `noDaemon`-style messaging; factor the "resolve rt + CheckDir loop" now duplicated in `report.go` and `rebase.go` into one helper that `status.go` reuses (pure refactor, no behaviour change).

**Recipes must be executed, not just written.** Mirror the `recipe-check` philosophy: a Go test (builds the binary, as `TestVersionLdflags` does) that starts a real `gruntled watch`, asserts `status` exits 0, `kill -9`s it, asserts `status` exits 3 while the status file still exists. Unix only; windows covered by the pipe phase.

**Effect on the proof:** none (`status` uses existing `ipc`/`statusfile`; no new package imports). **On `check` bytes:** none.

---

## 3. `watch --base <dir>`

### Where the baseline lives today
`watchPublisher` (`cmd/gruntled/watch.go`): fields `base checking.Report`, `baseGen uint64`, `baseSource string`, `hasBase bool`, guarded by `mu`. `commitReady(rep)` computes `next = gen+1`; if `!hasBase` it sets `base, baseGen, src = rep, next, sourceWatchStart`; then builds the snapshot and `blastViewOrError(base, rep, DaemonBaseline{Source, Generation, CurrentGeneration})`. `rebase()` replaces `base` with `lastRep`, source `"rebase"`. `Between(base, cur)` is the only comparison.

### What seeding from another tree takes
`blast --base` already builds a `blasting.Sources` from `terragrunt.NewLoader(baseFS)` + `tfsurface.NewReader(baseFS)`. `checking.Check(ctx, units, surfaces)` returns a `checking.Report` - exactly the baseline type. So:

| Step | New / Modified | Detail |
|------|----------------|--------|
| `--base dir` flag | **Modified** `watch.go` (`watchOpts`, `defineWatchFlags`, `watchUsage`) | Mirror `blast`: `openRepo(*base)`; unreadable -> exit 3, never silent fallback. |
| Reject a base inside the repository | New check in `runWatchWith` | Use `statusfile.Inside(root, base)` (identity-based, already used for status/runtime dirs) -> exit 2. Otherwise the base tree's `.tf`/`terragrunt.hcl` files would be indexed as units of the *current* repo and watched. `blast --base` has the same hazard today (document, do not change). |
| Compute `baseRep` | New glue | `checking.Check(ctx, terragrunt.NewLoader(baseFS), tfsurface.NewReader(baseFS))` (a tiny `blasting.Baseline` helper is optional; calling `checking.Check` from the composition root is equally fine). Close `baseRoot` immediately after: the baseline holds only a `Report`, preserving the v0.4 invariant "never a loader, FS or parse cache" and the memory bound. The base tree is **not watched**. |
| Seed the publisher | **Modified** `watchPublisher` | New `seedBase(rep checking.Report)` sets `base`, `hasBase=true`, `baseGen=0`, `baseSource=sourceBaseDir` (`"base dir"`; **do not** persist the user-typed path in snapshots - labels must be deterministic and path-free). `commitReady` already honours `hasBase`, so the first index builds the blast view against the seeded base with no further change. Generation 0 means "external"; `rebase --expect N` (N >= 1) is unaffected. |
| Label | **Modified** `presenter.DaemonBaselineLabel` | Add a branch for the new source (`baseline: base directory, current generation N`); keep the existing two sources' bytes (pinned by `TestDaemonDocs`: `baseline: daemon generation 1 (watch start), current generation 1`). JSON `daemon_baseline.source` gains a third value; `generation` may be 0 -> document. |
| Ordering in `runWatchWith` | Important | After `acquireInstance` (a second `watch` must say "already watching" without paying for a base index) and after the watcher is open; **before** `watch.Run`. Write the `indexing...` status line first (`pub.publish(watch.Event{Kind: EventIndexing})`; `Run` repeats it harmlessly), otherwise a dead predecessor's stale line survives for the whole base index. Failure to check the base -> exit 3 with the same `gruntled: ...` shape as `blasting.Error{Stage:"baseline"}`. |
| Protocol | **No change** | `BlastView.BaselineSource` is a free string; clients print the pre-rendered `Text`/`JSON` (label already inside). Old clients keep working. `compat-v1.sh` stays valid. |
| `rebase` | Unchanged | Moves to `lastRep` with source `"rebase"`; the seeded base is discarded (memory freed). |

**Data flow change:** `[--base dir] -> loader pair -> checking.Check -> Report -> pub.seedBase` (one-off, before Run). Steady state is identical to v0.4.

**Effect on the proof:** none (same loaders). **On `check` bytes:** none (`check` never sees the daemon path). **Risk:** `GRT004` correctness relies on `base` coming from `checking.Check` (documented contract of `Between`); `--base` satisfies it by construction. Add a test that a daemon seeded with a mutated tree reports the same Broken/Impacted bytes as `blast --base` on the same pair (reuse `daemon_blast_test.go` machinery; this is the "byte-identical to blast --base" property from v0.4).

---

## 4. Windows live `report` / `rebase`

### Current shape
- Unix: `ipc.Listen(sock)` (raw `syscall.Socket/Bind/Listen`, non-blocking fd registered with the runtime poller via `os.NewFile`), `ServeHandler` accept loop, one request line -> one response line (`respond`/`answer`, shared, platform-neutral in `ipc.go`), `Query`/`Rebase` clients (`dial`, `exchange`, deadlines on the `*os.File`). Server API is behind `//go:build linux || darwin`; `sock_other.go` returns `ErrUnsupported`.
- Windows: the daemon rewrites the dump file (`dump.go`, `WriteDump`, platform-neutral) after every index; `report` does `Probe(lock)` + `ReadDump`; `rebase` is refused (`deps.goos == "windows"`; `instance.Rebase` also refuses with `CodeUnsupported` in dump mode). So windows `report` is *already* current as of the last publish; the real gaps are `rebase` and the per-index dump cost (all of text+summary+JSON+SARIF+blast, base64, rewritten each index).

### Why not `golang.org/x/sys/windows` (verified)
`go list -deps` for `golang.org/x/sys/windows` on GOOS=windows includes `net`, `net/netip`, `vendor/golang.org/x/net/dns/dnsmessage` (two files import `"net"`). Importing it from `ipc` makes `cmd/gruntled` link `net` on both windows targets: Step 9 halves 1 and 4 fail. There is no exemption that keeps the claim "the binary links no `net`" true. Do not propose one.

### Options

| Option | Proof impact | Complexity | Verdict |
|--------|--------------|------------|---------|
| **P1. Named pipe via `syscall.NewLazyDLL("kernel32.dll")`** for `CreateNamedPipeW`, `ConnectNamedPipe`, `DisconnectNamedPipe` (client side needs only `syscall.CreateFile` on `\\.\pipe\...`) | No `net`, no `os/exec`; `sym_re` cannot see dynamically looked-up procs (already listed in the script's "residual gaps": "a DLL procedure looked up by name at run time"). Needs a **new source-scan rule** (below) to make that gap non-silent. | Medium: blocking handle I/O, shutdown unblocking, deadlines, ACL. | Recommended if the milestone keeps "named pipe". |
| P2. File mailbox in the runtime dir (client creates `req-<nonce>` with `CREATE_NEW`, daemon's windows poll loop picks it up, writes `resp-<nonce>`) | None: only `os`/`syscall.CreateFile`. Protected by the same `%LocalAppData%` ACL as lock/dump (no new security surface). | Low-medium, but latency = poll period and a new wakeup in `watch.Run`. | Fallback if the P1 spike shows deadline/ACL problems. Keeps the proof strictly unchanged. |
| P3. Keep windows as is, document | None | Zero | Only if scope is cut. |

### P1 integration points (same port, no `cmd` redesign)
| Item | New / Modified | Detail |
|------|----------------|--------|
| `internal/infrastructure/ipc/pipe_windows.go` | **New** (`//go:build windows`) | Implements the *exact* `sock_other.go` surface: `Listener`, `Server`, `Listen(path)`, `Serve`, `ServeHandler`, `(*Server).Close`, `Query`, `Rebase`. The `sock` path argument is mapped to `\\.\pipe\gruntled-<hash of path>` so `cmd` keeps passing `filepath.Join(rt, ipc.SockName)`. `respond`/`answer`/`decodeResponse`/`readLine` are reused verbatim - the wire protocol (v2, canonical request line, 64 MiB cap, `Seal`) does not change. |
| `sock_other.go` | **Modified** build tag | `//go:build !linux && !darwin && !windows`. |
| `cmd/gruntled/instance.go` (`useSocket := deps.goos != "windows"`), `report.go` (`if goos != "windows"`), `rebase.go` (`if deps.goos == "windows"` -> unsupported) | **Modified** | Replace the GOOS string tests by capability: rely on `ErrUnsupported` (already handled in two of the three places) plus a `deps.noSocket bool` test seam that keeps simulating "no socket platform" on linux. Today's tests (`instance_test`, `report_test`, `rebase_test`, `watch_test` pass `goos: "windows"`) must be migrated to that seam. |
| `instance.Rebase` refusal in dump mode | Modified | Only refuse when there is genuinely no server (`in.server == nil`). |
| Dump file | **Recommend keep writing it for v0.5** (mark deprecated), serve the pipe in addition | An old (v0.4) windows `report` against a new daemon would otherwise find no dump, read `ErrNotExist` as "still indexing" and say "try again" forever - a silent wrong answer, the exact failure class v0.4's "mismatch is explicit" rule forbids. Remove the dump in v0.6. New client + old daemon: pipe is absent -> fall back to the existing `Probe` + `ReadDump` path (keeps v0.4 daemons readable). |
| Security: pipe namespace is global | **New surface** | Windows currently "relies on `%LocalAppData%` ACLs" for lock/dump. Named pipes live in `\\.\pipe\`, enumerable by any local user, with a default DACL that grants read to Everyone. Required: `PIPE_REJECT_REMOTE_CLIENTS` (0x8) in the pipe mode, and either an explicit DACL (owner-only SDDL via `advapi32!ConvertStringSecurityDescriptorToSecurityDescriptorW`, a second allowlisted proc) or a capability token (random 128-bit value in a 0600 file in the ACL'd runtime dir, sent in the request). The token needs a request-shape change that the strict canonical-line parser rejects -> protocol v3. Prefer the DACL. This is the main research flag. |
| Tests | **New** | `pipe_windows_test.go` (runs only in `ci.yml` `test-os`/windows-latest, which already exists); a shared table-driven transport conformance test run against both `sock_unix.go` and `pipe_windows.go` (build-tagged files each register a constructor). `windows/arm64` is cross-compiled but never executed in the current CI (check whether a `windows-11-arm` runner is available; unverified). |
| Unverified, spike first | | Blocking `ConnectNamedPipe` shutdown (connect a dummy client or `CancelIoEx`), per-call deadlines on a non-overlapped pipe handle wrapped by `os.NewFile` (`SetDeadline` returns `ErrNoDeadline` for non-pollable files; use a goroutine + `CancelIoEx`/`CloseHandle` watchdog), `ERROR_PIPE_BUSY` retry loop in the client instead of `WaitNamedPipe`. |

### What the six-target proof needs for P1 (not an exception: a new rule)
1. Keep Step 9 halves 1-4 **unchanged**. `x/sys/windows` stays forbidden; `net` stays forbidden.
2. Add a rule `windows-dll-allowlist` (own `=== RULE FAILED` block, in `check-architecture.sh`, with `test-check-architecture.sh` cases):
   - `NewLazyDLL`, `NewProc`, `LoadDLL`, `MustLoadDLL`, `LoadLibrary`, `GetProcAddress` may appear in non-std Go files **only** in `internal/infrastructure/ipc/pipe_windows.go` (scan like half 2: `go list -deps -f` GoFiles for each windows target, `std` excluded);
   - string literals passed to `NewLazyDLL`/`NewProc` must be in an allowlist (`kernel32.dll`, `advapi32.dll`, `CreateNamedPipeW`, `ConnectNamedPipe`, `DisconnectNamedPipe`, `ConvertStringSecurityDescriptorToSecurityDescriptorW`, `LocalFree`);
   - no non-std windows file may contain the substrings `CreateProcess`, `ShellExecute`, `WinExec` (this catches a constructed proc name).
   Both kernel32 and advapi32 are KnownDLLs, so `LazyDLL` resolution cannot be hijacked from the application directory. State the residual gap plainly in the script header (a proc name assembled at run time is invisible to nm and to this scan).
3. New self-test cases: `binary-xsys-windows-net` (an `ipc` windows file importing `golang.org/x/sys/windows` must fail with the *net* diagnosis), `windows-dll-outside-allowlist`, `windows-dll-proc-not-allowlisted`, `windows-createprocess-literal`, and `windows-pipe-allowed` (the legitimate file passes).
4. `TestReleaseTargetsInSync` is unaffected (target list unchanged).

**Effect on `check` bytes:** none (daemon path only). **Effect on the proof:** P1 is the only v0.5 item that touches the proof script; P2 touches nothing. **Build rule:** land the proof changes + self-tests in the *first* plan of the phase, before any pipe code, so the rule exists before the code it polices (same discipline as v0.3's DAEMON-06).

---

## 5. CLI usability: completions, man page, `--version`, colour, `--quiet`

### Where the command table is today
There is no table. `runCtx` (`main.go`) is a `switch args[0]`; `topUsage` and the per-command `checkUsage/graphUsage/blastUsage/watchUsage/reportUsage/rebaseUsage` are string consts in different files; flags are defined imperatively inside each `run*` (`parseArgs(name, usage, args, stderr, define)`). `TestTopUsageMatchesDocs` requires `docs/cli.md` to contain the fenced block `"```\n"+topUsage+"```\n"` verbatim and a `report` line mentioning `--blast`; `TestHelpMatchesDocs` requires each command's exit-code lines (`^  [0-3]  `) to appear verbatim in `docs/cli.md` and counts them per command (check 4, graph 3, blast 4, watch 3, report 4, rebase 3).

### Table-driven design without cobra (recommended)
New `cmd/gruntled/commands.go` (**new**, package main):

```go
type command struct {
    name, summary string                  // topUsage line
    usage         string                  // existing xUsage const
    flags         func(*flag.FlagSet)     // defines flags on a scratch or real FlagSet
    values        map[string][]string     // enumerated flag values: "format" -> {"text","json","sarif"}
    run           func(ctx context.Context, args []string, stdout, stderr io.Writer) int
}
var commands = []command{ ... } // single source of truth, in topUsage order
```

- `topUsage` stays a *constant-looking* string produced by one function from `commands` (so `docs/cli.md`'s fenced block can still be pinned byte-for-byte). Do this refactor first as a **pure refactor** with the existing tests as the safety net.
- Each `run*` calls `cmd.flags(fs)` (watch already has `defineWatchFlags`; the others have inline closures that assign local variables - convert to `defineCheckFlags(fs) *checkOpts` etc.). A scratch `flag.NewFlagSet` + `VisitAll` yields flag name, usage and default; boolean flags are detected through `Value.(interface{ IsBoolFlag() bool })`.
- `values` is the *same* slice the validation `switch *format` uses (`slices.Contains`), so completions cannot drift from validation.
- **Generators use `strings.Builder` + `fmt`, never `text/template`** (links `net/url`, probed).

| Feature | New / Modified | Detail |
|---------|----------------|--------|
| `gruntled completion bash\|zsh\|fish\|powershell` | **New command** (`completion.go`) | Generated at run time from `commands`; nothing shipped in archives, so `TestBuildRelease` (pins archive members to exactly `LICENSE` + binary) and the `gruntled_<tag>_<os>_<arch>` layout stay untouched. Path positional -> shell's directory completion. |
| `gruntled man` | **New command** (`man.go`) | roff on stdout from `commands`: NAME, SYNOPSIS, per-command section embedding `usage` in `.nf`/`.fi` with `\` and leading `.`/`'` escaped, EXIT STATUS parsed from the same `^  [0-3]  ` lines, FILES (status/lock paths), SEE ALSO. No date (deterministic output). Homebrew installs it with `(man1/"gruntled.1").write Utils.safe_popen_read(bin/"gruntled", "man")`; `generate_completions_from_executable(bin/"gruntled", "completion", shells: [...])`. |
| `status` (section 2) | New entry in the table | |
| Golden files | New | `cmd/gruntled/testdata/completion/*.{bash,zsh,fish,ps1}` and `man.1`, regenerated by a `-update` flag; plus `bash -n`/`zsh -n`/`fish -n` syntax checks in CI where the shells exist. **Write the goldens last** (after all flags exist) or they churn on every flag addition. |
| `--version` | **Modified** `main.go`, `render.go`, `instance.go` | Today the three consumers read the package var `version` directly: the `--version` print, SARIF `ToolInfo{Version: version}` in `renderReport`, and `ipc.Info{Version: version}`. Introduce one `resolvedVersion() (version, commit string)` and use it in all three. Logic: ldflags value wins unless it is `"dev"`; then `debug.ReadBuildInfo()` (`runtime/debug` is already linked): accept `Main.Version` **only** if `Main.Path == "github.com/GiulioSavini/gruntled"`, it is a clean release tag (`^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$`), contains no `+`, and is **not a pseudo-version** (suffix `\d{14}-[0-9a-f]{12}`); commit = first 12 of `vcs.revision` else `"none"`. Anything else stays `dev (none)`. This is the "revisit" in PROJECT.md (the old drop was because a local `go build` stamps a VCS pseudo-version). Make the decision a pure function of a `*debug.BuildInfo` and table-test it (tag, pseudo, `+dirty`, `(devel)`, other module path, test binary). `TestVersion`/`setVersion` seam keep working because they set the vars. Release smoke (`case "$out" in "gruntled $GITHUB_REF_NAME (")`) is unchanged. CI cannot test `go install ...@tag` before the tag exists; add a post-publish job (see section 6). Effect on bytes: only `go install @tag` SARIF `tool.driver.version` and `--version` change from `dev`; text/JSON `check` output is untouched. |
| Colour | **New**, two small pieces | (1) `cmd/gruntled/termenv.go`: pure `colorEnabled(flag string, env func(string) string, isTTY bool, goos string) bool`: `--color auto\|always\|never`, `NO_COLOR` (any non-empty) wins over auto, `CLICOLOR_FORCE`, `TERM=dumb`; TTY = `os.Stdout.Stat()` with `Mode()&os.ModeCharDevice != 0` (stdlib; NUL/`/dev/null` also count as char devices - harmless). Windows: default off unless `WT_SESSION`/`ANSICON` is set, because stdlib `syscall` has no `SetConsoleMode` (VT enable) and `x/term`/`x/sys/windows` are proof-forbidden. (2) `presenter.Colorize(plain []byte) []byte`, a pure line-oriented post-processor over the final `Text` / `BlastText` bytes (stdlib `bytes` only). **It must run at the edge, after `escapeTerm`**: the existing guarantee is "nothing printed can carry an ESC from data"; colour is the only place ESC is introduced deliberately. Invariant, property-tested: `stripSGR(Colorize(x)) == x` for every golden text, and `Colorize` is not applied to json/sarif. Applying it at the edge also colours `report` (the daemon snapshot holds pre-rendered plain bytes and must stay plain). |
| `--quiet` | **New**, tiny | `check`: skip writing the stderr summary line (`renderReport` already returns it separately; stdout bytes and exit code unchanged). `status`/`report`: suppress the "not running" message so a prompt can use exit code alone. `watch`: suppress the banner. Never changes stdout content. |
| Where flags attach | | A shared `defineCommonFlags(fs)` (`--color`, `--quiet`) added by the commands that print text (check, blast, report, status). Needs `docs/cli.md` per-command flag tables and usage text updates (pinned by tests). |

**Effect on the proof:** none *if* the two traps are avoided (no `text/template`, no `x/term`). Add to `test-check-architecture.sh` a case `binary-text-template` (a `text/template` import in `cmd/gruntled` must fail with `net/url`) so nobody re-discovers this later. **Effect on `check` bytes:** default (non-TTY, no flags) output is byte-identical by construction (the colour path is a separate function applied only when enabled); `compare-ref.sh` and the goldens run non-TTY and prove it.

---

## 6. Release pipeline integration

### Current shape
- `scripts/build-release.sh <version> <commit> <outdir>`: 6 targets, `CGO_ENABLED=0 -trimpath -ldflags "-s -w -X main.version -X main.commit"`, archives named `gruntled_<tag>_<os>_<arch>.tar.gz|zip` containing exactly `gruntled[.exe]` + `LICENSE`; `checksums.txt` over `gruntled_*`; host `--version` smoke. `release_targets` lives in this file and in `check-architecture.sh` (`TestReleaseTargetsInSync`). `TestBuildRelease` pins the output file set (6 archives + `checksums.txt`) and the archive members.
- `release.yml` (on tag `v*`): job `release` with `contents: write`: guard (tag regex + on master) -> proofs (`check-architecture.sh` + `go test`) -> build -> smoke (expects exactly 7 files in dist, `--version` prefix) -> `gh release create --generate-notes --verify-tag [--prerelease] dist/*`. Single publication step so a failure leaves no partial release.
- `ci.yml`: jobs `check` (gofmt, tidy, vet, staticcheck, govulncheck, `go test -race`, fuzz, sarif-schema, release-build dry run with `v0.0.0-ci`), `architecture`, `test-os` (macOS, windows), `recipe-check` (installs from checkout, pre-commit try-repo), `sarif-upload`. All actions pinned to SHAs.
- `TestCIDoc` pins `docs/ci.md` to `ci.yml` via the hard-coded `ciPins` (checkout v7.0.1, setup-go v7.0.0, upload-sarif v4.38.2) and exact command strings; separate assertions fix `corpus.yml` (e.g. exactly 2 `uses:`).
- `docs/ci.md` contains `v0.2.0` in 7 places (`go install ...@v0.2.0`, `tag=v0.2.0`, `rev: v0.2.0`, archive names, GitLab snippet). v0.2.0 is already two milestones stale and v0.4.0 is untagged.

### Hook points

| Deliverable | New / Modified | Where it hooks | Notes |
|-------------|----------------|----------------|-------|
| **Install script** | **New** `install.sh` (repo root or `scripts/install.sh`) | Published as a **release asset** (immutable per tag), not "curl master | sh". Resolves the tag via the `/releases/latest` redirect (no API, no jq), downloads archive + `checksums.txt` from the same release, verifies `sha256sum`/`shasum -a 256`, installs to `$PREFIX/bin`. Windows = documented manual/zip. Env override `GRUNTLED_BASE_URL` for tests. | New CI job `install-script`: run it against the *dry-run dist* served via `file://`, plus a tamper case (flip a byte -> must fail non-zero, installs nothing) and a table test of the `uname` -> `os/arch` mapping. Needs no change to `build-release.sh`. It is a shell script outside the Go binary, so the no-net claim of the *binary* is untouched. |
| **Homebrew** | **New** `packaging/homebrew/gruntled.rb.tmpl` + `scripts/render-formula.sh <tag> checksums.txt` | `release.yml` `release` job: render `gruntled.rb` from the just-built `checksums.txt` and attach it as an extra release asset in the *same* `gh release create` (atomic with the release). The tap repo (`GiulioSavini/homebrew-gruntled`, separate repository) updates itself **by pull** (scheduled/dispatch workflow downloads `gruntled.rb` from the latest non-prerelease). This avoids storing a cross-repo write token in this repo and keeps the `contents: write`-only permission of the release job. Formula: `on_macos/on_linux` x `on_arm/on_intel` blocks with url + sha256, `bin.install "gruntled"`, completions/man via the new subcommands, `test do assert_match version.to_s, shell_output("#{bin}/gruntled --version")`. | CI `formula-check`: render for `v0.0.0-ci` against the dry-run dist, assert each sha256 equals the `checksums.txt` line (Go or shell test), `ruby -c`. Prereleases (`-suffix`) never move the formula. |
| **GHCR image** | **New** `Dockerfile` (`FROM scratch`, `COPY` the released linux binary, `USER 65532`, `WORKDIR /repo`, `ENTRYPOINT ["/gruntled"]`) + **new job** `image` in `release.yml` | `needs: release`, `permissions: packages: write, contents: read` (only this job). Build **from the released linux archives** (download from the release and verify against `checksums.txt`), not by recompiling, so the image contains exactly the released bytes. Use `docker buildx build --platform linux/amd64,linux/arm64` and `docker login ghcr.io` from the preinstalled Docker on `ubuntu-latest` (no QEMU needed: `COPY` only) - **no new third-party actions to SHA-pin**. Tags `:vX.Y.Z` always; `:latest` and `:vX.Y` only for non-prerelease tags. | Failure mode: the release is already public when the image job fails. Add `workflow_dispatch` with a `tag` input for re-run. CI `container-check` job: build (no push) and run `docker run --rm --network none -v "$PWD:/repo:ro" ... check cmd/gruntled/testdata/clean-fixture` (exit 0) and the broken fixture (exit 1) - a nice executable demonstration of "read-only, no network". |
| **`go install` version** | See section 5 | Post-publish verification job `verify-published` in `release.yml` (`needs: release`, `contents: read`): `go install github.com/GiulioSavini/gruntled/cmd/gruntled@$TAG` then `gruntled --version` must start `gruntled $TAG (`. Retry for proxy lag. |
| **Smoke count** | **Modified** `release.yml` smoke | Keep `dist/` at exactly 7 files (so `TestBuildRelease` and `build-release.sh` stay untouched); stage `install.sh` and `gruntled.rb` in a separate `$RUNNER_TEMP/extras`, publish `dist/* extras/*`, and make the smoke assert both counts. |
| **Docs pins never stale** | **New** `VERSION` file + test + script | `VERSION` (repo root) holds the latest *published* tag. `scripts/set-release-version.sh vX.Y.Z` rewrites every pin in `docs/ci.md`, `README.md`, the install snippet and formula docs. A Go test `TestDocPinsMatchVersionFile` scans `docs/*.md` and `README.md` for `@v\d+\.\d+\.\d+`, `rev: v...`, `tag=v...`, `gruntled_v...` and requires equality with `VERSION` (excluding historical references such as `v0.3.0` in `docs/validation.md`, by scanning only an explicit file list). `release.yml` guard gains: `VERSION` content == `$GITHUB_REF_NAME` (the release commit bumps `VERSION` and is the commit that is tagged; the checklist does bump -> tag -> push in one go, so docs never name a tag that does not exist for more than the tagging gap). Because tagging is Giulio's decision, `VERSION` stays at the last real tag until then. |
| **Dependabot / hygiene vs `TestCIDoc`** | **Modified** `ci_doc_test.go` | Replace the constant `ciPins` with values *parsed from `ci.yml`'s `recipe-check`/`sarif-upload` jobs*, and assert `docs/ci.md` contains those exact strings. Then a Dependabot bump of `ci.yml` makes the test fail only on the doc half; add `scripts/sync-ci-doc.sh` (rewrite the pins in `docs/ci.md` from `ci.yml`) and group Dependabot `github-actions` updates so one PR carries ci.yml + corpus.yml + release.yml + the doc fix. Also keep `corpus.yml`'s "exactly checkout and setup-go" assertion in mind: a grouped bump must move both workflows together (`corpus.yml` asserts the same SHAs as `ci.yml`). `.github/dependabot.yml` (gomod, github-actions), CODEOWNERS, templates, CONTRIBUTING, SECURITY, CHANGELOG are doc/config only (no Go impact), except that CONTRIBUTING must name `scripts/check-architecture.sh` and `scripts/test-check-architecture.sh` as the gate and CHANGELOG should be fed from the same `gh --generate-notes` flow. Dependabot x/sys bumps: the enumerated spawner list in `check-architecture.sh` is manual ("re-run on every x/sys bump"); a green Dependabot PR does not prove it was re-enumerated - add a small CI step that re-runs the enumeration grep and diffs it against a checked-in expected list, or pin x/sys in `ignore`. |

**Effect on the proof:** none of section 6 touches `cmd/gruntled` imports; the proof is about the binary, and these are build/publish plumbing. The `image` job adds `packages: write` and a Docker daemon to the *release* workflow only, never to `ci.yml` (`ci.yml` builds the image without pushing). **On `check` bytes:** none (the release binary is the same `build-release.sh` output; the image copies it).

---

## Component summary (new vs modified)

| Component | Status | Layer | Proof effect | `check` bytes |
|-----------|--------|-------|--------------|---------------|
| `cmd/gruntled/commands.go` (command table) | New | composition root | none | none (pure refactor, existing tests) |
| `cmd/gruntled/completion.go`, `man.go` | New | composition root | none (no templates) | none |
| `cmd/gruntled/status.go` | New | composition root | none | none |
| `cmd/gruntled/termenv.go`, `presenter/color.go` | New | root + interfaces | none (no x/term) | none by default |
| `cmd/gruntled/main.go`, `render.go`, `instance.go`, `watch.go`, `report.go`, `rebase.go` | Modified | composition root | none | none |
| `presenter/blast_sarif.go`; `sarif.go` (additive field) | New / Modified | interfaces | none | none if the check table is untouched |
| `application/blasting` (`BlastDetailed`) | Modified (additive) | application | none | none |
| `presenter.DaemonBaselineLabel` | Modified (new branch) | interfaces | none | none |
| `ipc/pipe_windows.go` | New | infrastructure | **adds `windows-dll-allowlist` rule + self-tests** | none |
| `ipc/sock_other.go` | Modified (build tag) | infrastructure | none | none |
| `watch.Config.Heartbeat` (optional) | Modified | infrastructure | none | none |
| `install.sh`, `render-formula.sh`, `Dockerfile`, `VERSION`, `set-release-version.sh`, `sync-ci-doc.sh`, `dependabot.yml` | New | repo/CI | none | none |
| `release.yml`, `ci.yml`, `ci_doc_test.go` | Modified | CI | none | none |
| `check-architecture.sh`, `test-check-architecture.sh` | Modified | proof | **the proof itself** (windows rule; template trap case) | n/a |

---

## Recommended build order

Order is chosen so that (1) refactors that many later items depend on land first with the existing byte-identity guards as the safety net, (2) every flag-adding item lands before the golden-generating item, (3) the only proof-touching change is isolated, preceded by a spike, and (4) the release plumbing consumes finished CLI features.

| # | Phase | Contents | Depends on | Risk to proof / check bytes |
|---|-------|----------|-----------|------------------------------|
| 0 | **Hygiene track (parallel, doc/config only)** | CONTRIBUTING, SECURITY, CHANGELOG, templates, CODEOWNERS. Dependabot config **together with** the `TestCIDoc` rewrite and `sync-ci-doc.sh` (never one without the other). | none | none |
| 1 | **CLI foundations** | Command table refactor (pure, tests unchanged) -> version resolver -> `--color`/`--quiet` common flags. | none | none; `compare-ref.sh` + goldens must stay green after the refactor commit, *before* behaviour is added |
| 2 | **`blast --format sarif`** | `BlastDetailed`, `blast_sarif.go`, rule table extension, docs/tests rewrite, CI schema step. | 1 (format enumeration) | none; guard "check rules length 4" |
| 3 | **Daemon: liveness + `watch --base`** | Shared runtime-dir helper, `status` (ping-based), docs, real-process kill -9 test; then `--base` seeding + label + byte-equality-with-`blast --base` test. Optional heartbeat after `status` ships. | 1 | none |
| 4 | **Windows pipe** (*spike gate first*) | Spike P1 vs P2 on a Windows host; then proof rule + self-tests **first**, `pipe_windows.go`, capability refactor of the three `goos` guards, transport conformance tests, ACL, keep-dump compat. | 3 (`status` uses the pipe ping on windows) | **touches the proof**; isolate; full `check-architecture.sh` on all 6 targets after every plan |
| 5 | **Completions and man** | `completion`, `man`, golden files, shell syntax checks. Last CLI item so goldens are written once against the final flag set. | 1-4 | none (no templates) |
| 6 | **Release pipeline** | `VERSION` + doc-pin test + release guard, `install.sh` + CI job, formula template + `formula-check`, `Dockerfile` + `container-check` + `image` job, `verify-published`, smoke-count change. | 1 (version), 5 (Homebrew uses completion/man) | none for the binary; permission changes confined to new jobs |
| 7 | **Proof and release readiness** | Re-run `corpus-gate.sh` (network-isolated, frozen v0.3.0 reference) and `compare-ref.sh` on v0.5 HEAD, `compat-v1.sh`, docs-vs-binary verification, independent cross-phase audit (previous milestones found real BLOCKERs between phases), release checklist (tagging stays Giulio's decision). | all | this phase *is* the check-bytes/proof proof |

Parallelism: 0 is independent of everything. 2 and 3 are independent of each other after 1. 4 should not run in parallel with 3 on the same files (`instance.go`, `report.go`, `rebase.go` are shared hot spots; give 3 the lock/runtime-dir helper extraction first).

---

## Anti-patterns to avoid

1. **Editing the shared `sarifRuleTable` to add GRT004.** Changes `check --format sarif` bytes and GRT100's `ruleIndex`. Extend a copy for blast.
2. **Importing `golang.org/x/sys/windows`, `golang.org/x/term`, `text/template`, `html/template` or `net/*` anywhere under `cmd/gruntled`'s windows or unix dependency closure.** Each one trips Step 9 half 1; none can be excepted without voiding the claim.
3. **Polling `ipc.Probe` from a prompt.** Races daemon start on unix. Ping the socket.
4. **Colouring inside the daemon or inside `escapeTerm`.** Snapshots must stay plain bytes; colour belongs at the output edge, after escaping.
5. **Persisting the user-typed `--base` path (or any absolute path) in daemon snapshots or labels.** Breaks determinism and repository-relative output rules.
6. **Rewriting the status line format** to carry liveness. It is a documented, test-pinned contract for third-party shell/editor integrations.
7. **Letting new-client/old-daemon (or the reverse) on windows produce an empty or "still indexing" answer.** Keep the dump for one release or bump the protocol; never rely on absence.
8. **Hand-maintained usage, docs and completion lists.** One table; docs blocks asserted by tests.
9. **A cross-repo write token in `release.yml` for the Homebrew tap.** Use a pull model with the formula as a release asset.

---

## Confidence

| Claim | Confidence | Basis |
|-------|------------|-------|
| `x/sys/windows` links `net`; `text/template` links `net/url` | HIGH | `go list -deps` on the pinned versions |
| stdlib `syscall` on windows lacks `CreateNamedPipe` | HIGH | grep of GOROOT `syscall` sources |
| Check SARIF bytes change if GRT004 enters the shared table | HIGH | read `sarifRules()`/`ruleIndexOf` |
| Probe-vs-watch-start race on unix | HIGH | read `TryLock`, `Probe`, `acquireInstance` (`attempts := 1`) |
| `TestCIDoc` vs Dependabot coupling | HIGH | read `ciPins` |
| Named pipe via `LazyDLL`: deadlines, shutdown, DACL details | MEDIUM-LOW | design from API knowledge; **requires a Windows spike** |
| `windows-11-arm` runner availability | LOW | not verified |
| Homebrew DSL (`generate_completions_from_executable`, on_arm/on_intel), `docker buildx` from scratch without QEMU | MEDIUM | knowledge, not probed |
| Build order | HIGH | follows verified file/test coupling |

## Open questions for phase-level research

- **Phase 4:** P1 vs P2 decision after a Windows spike (deadline mechanism, `Close()` unblocking, DACL via SDDL vs token, protocol v3 needed only for the token variant). Ask whether keeping the dump for one more release is acceptable.
- **Phase 3:** heartbeat in or out of v0.5.
- **Phase 6:** location of the Homebrew tap repository and who owns it; whether `install.sh` should offer completion installation.

## Sources

- Repository code read at HEAD `ca4f07f`: `cmd/gruntled/{main,render,report,rebase,watch,instance}.go`, `internal/infrastructure/{ipc,statusfile,watch}`, `internal/interfaces/presenter/{sarif,blast,status,text}.go`, `internal/application/blasting`, `internal/domain/{impact,diagnostic}`, `scripts/{check-architecture,build-release,test-check-architecture}.sh`, `.github/workflows/{ci,release,corpus}.yml`, `docs/{cli,ci}.md`, `cmd/gruntled/{ci_doc,release,e2e,rules_doc,sarif}_test.go`, `.planning/PROJECT.md`.
- Toolchain probes (go 1.27.2, x/sys v0.47.0): `GOOS=windows go list -deps golang.org/x/sys/windows`, `go list -deps text/template`, `go list -deps html/template`, GOROOT `syscall` grep.
