# Phase 10: Watch Daemon & Status File - Research

**Researched:** 2026-10-08
**Domain:** Go file-watching daemon (fsnotify + stdlib stat polling), atomic status file, hexagonal layering under a strict import/binary proof
**Confidence:** HIGH on dependency/proof facts (measured), MEDIUM on darwin/windows runtime behaviour (cross-compile only, no CI runner)

<user_constraints>
## User Constraints (from CONTEXT.md)

No CONTEXT.md exists for this phase (unattended run). Constraints below are copied from REQUIREMENTS.md, STATE.md, 08-VERIFICATION.md and the orchestrator prompt. Treat them as locked.

### Locked Decisions
- fsnotify v1.10.1 only behind `//go:build !windows`; stat polling (stdlib) on windows and via `--poll`.
- No-net/no-exec proof (`scripts/check-architecture.sh` Step 9) must keep passing on all 6 release targets at the end of this phase (it runs in CI on every push).
- `application` layer stdlib allowlist excludes `time`, `os`, `sync`; `interfaces` allows only domain allowlist + `fmt`, `io`, `encoding/json`. Debounce, watchers, status file live in infrastructure/cmd.
- Status file: one line, atomic (tmp + rename), per-repository documented path OUTSIDE the repository, readable with `cat` on every OS.
- Phase 8 security findings (medium #1 concurrency, medium #2 stale cache on dropped events, low #3 O(n*cache) Invalidate, low #4 path contract) must be addressed here.
- Decision from v0.3 research: watcher events are hints (dirty path), not meaning; discovery + graph are rebuilt on every reindex.
- Trailing debounce ~150 ms; ignore `.git`, `.terraform`, `.terragrunt-cache`, editor swap/backup files; new/deleted directories picked up.

### Claude's Discretion
- Package names/layout, flag names beyond `--poll`, exact status text for non-ok states, what `watch` prints to stdout, poll interval default, safety-net interval, status directory selection rule.

### Deferred Ideas (OUT OF SCOPE)
- Unix socket, `report`, lock/single instance, attach (Phase 11: DAEMON-04/05/06 formal proof + docs/README).
- Daemonizing/forking, `report` auto-start, state files inside the repo, git-based blast, `x/sys/windows`, gofrs/flock, go-winio, gRPC, on-disk index cache, desktop notifications, LSP, windows IPC.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| DAEMON-01 | `gruntled watch [path]`: full index at start, then reindex only changed files after a save (~150 ms trailing debounce), ignore `.git`/`.terraform`/`.terragrunt-cache`/swap files, pick up new and deleted dirs | Watcher port + accumulator, `Debouncer` state machine, ignore predicate, fsnotify (`!windows`) and poll adapters, `Loader.Invalidate` fed with repo-relative slash paths, single indexer goroutine |
| DAEMON-03 | One status line written atomically to a documented per-repository path outside the repo, readable on every OS | `internal/infrastructure/statusfile` (path resolution + CreateTemp/Rename writer with Windows retry), `presenter.StatusLine` golden-tested, injected clock |
</phase_requirements>

## Summary

The incremental core already exists (Phase 8): `terragrunt.Loader` keeps a persistent parse store, `Invalidate(paths...)` evicts, and `LoadUnits` redoes discovery and assembly every call; `checking.Check(ctx, loader, surfaces)` yields the same `Report` `check` prints. Phase 10 therefore adds only a thin shell around it: a watcher that produces batches of dirty repo-relative paths, a debouncer, a single-goroutine run loop that calls `Invalidate` then `Check`, and a status-file publisher. No analysis logic changes.

The one finding that changes the plan: the milestone research said fsnotify is "clean" on linux/darwin, and the `go list -deps` import deny-list half of Step 9 does confirm that (re-measured: no `net`, `os/exec`, `crypto/tls` on linux/arm64/amd64 and darwin; windows links `net`, `net/netip`, `x/sys/windows` as expected). BUT the second half of Step 9, the textual spawner scan over non-std GoFiles, **fails** once fsnotify is linked: `golang.org/x/sys@v0.46.0/unix/syscall_unix.go:582` contains `return syscall.Exec(argv0, argv, envv)` (`unix.Exec`), which matches `\bsyscall\.(ForkExec|Exec|StartProcess)\b`. fsnotify imports `x/sys/unix`, so linux and darwin targets would trip `binary-no-net-no-exec` the moment fsnotify lands. The linker proof is stronger and passes: a binary linking fsnotify has no `os.StartProcess`, `syscall.forkExec` or `syscall.Exec` symbols (dead-code eliminated; control build importing `os/exec` does show them). The plan must extend Step 9 in this phase (see Architecture, "Proof extension") or CI goes red on the first fsnotify commit.

**Primary recommendation:** Build `internal/infrastructure/watch` (ignore filter, pending-set accumulator, poll adapter, fsnotify adapter in `_unix.go`/`_windows.go` pair, debouncer, run loop) and `internal/infrastructure/statusfile`, add `presenter.StatusLine`, a tiny `application/watching` use case, wire `watch` in `cmd/gruntled/watch.go`, harden `Loader` (mutex, batch Invalidate, path contract), and extend Step 9 with a narrow `x/sys/unix` scan exemption backed by a per-target `go tool nm` symbol check, in the same plan wave as the `go get fsnotify`.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| github.com/fsnotify/fsnotify | v1.10.1 (go 1.23, requires x/sys v0.13.0; repo already has x/sys v0.46.0 so no bump) | inotify (linux) and kqueue (darwin) backend | Only mature option; verified net-free on linux/darwin with `go list -e -deps` (this session, CGO_ENABLED=0). Import only from `//go:build !windows` files. |
| stdlib `os`, `path/filepath`, `io/fs`, `time`, `os/signal`, `crypto/sha256`, `encoding/hex` | go1.27 | stat poller, ignore filter, status path hash, shutdown | `os/signal` verified net-free on linux and windows (`go list -deps`). |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| pgregory.net/rapid | v1.3.0 (already in go.mod) | existing incremental==full model test | Do not extend for watchers; keep real-fs tests deterministic via conditions. |
| rogpeppe/go-internal testscript | v1.16.0 (already) | CLI usage/exit-code scenarios for `watch` | Only for commands that exit immediately; long-running tests are Go tests (see Validation). |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| fsnotify | raw inotify/kqueue syscalls | ~500 lines per OS and untestable on darwin here; no. |
| Per-dir fsnotify adds | fsnotify recursive | Not public API (README "Recursive watching is not currently enabled through fsnotify's public API"); add one watch per directory yourself. |
| XDG_RUNTIME_DIR everywhere | `os.UserCacheDir` | RUNTIME_DIR does not exist on darwin/windows; see status path rule below. |

**Installation:**
```bash
go get github.com/fsnotify/fsnotify@v1.10.1
go mod tidy          # CI runs `go mod tidy -diff`; commit go.mod/go.sum together
```

## Architecture Patterns

### Recommended Project Structure
```
cmd/gruntled/
  main.go                  # + case "watch"; run() keeps signature, delegates to runCtx(ctx, ...)
  watch.go                 # runWatch(ctx, args, stdout, stderr): flags, wiring, exit codes
internal/application/
  ports/ports.go           # + InvalidatingLoader { UnitLoader; Invalidate(paths ...string) }
  watching/                # Indexer: Index(ctx, dirty []string, resync bool) (Snapshot, error); NOT concurrency-safe by contract
internal/infrastructure/
  watch/
    ignore.go              # Ignored(rel string, isDir bool) bool  (pure, table-tested)
    pending.go             # mutex'd dirty set + overflow flag + size-1 ready chan (shared by adapters)
    poll.go                # stat-poll adapter, all OSes
    native_unix.go         # //go:build !windows  fsnotify adapter
    native_windows.go      # //go:build windows   NewNative returns ErrNativeUnavailable
    debounce.go            # pure state machine, no timers, takes time.Time args
    run.go                 # single-goroutine loop (the only caller of Indexer)
  statusfile/
    path.go                # Dir(root, env) -> <base>/gruntled/<hash12>
    write.go               # atomic CreateTemp + Rename (+ windows retry)
internal/interfaces/presenter/status.go   # StatusLine(w, state, diags, stamp string)
```
`watch` and `statusfile` are infrastructure: layer rules allow third-party and build tags there (platform-neutral rule covers only domain/application/interfaces; compile-gate vets only domain/application/interfaces/cmd, so cross-compile infra explicitly, see Validation).

### Pattern 1: Watcher port = pull-style dirty set (not an event channel)
**What:** The adapter owns a goroutine that drains the OS source continuously into `pending` (set of repo-relative slash paths + `resync bool`). Consumer waits on `Ready() <-chan struct{}` (buffered 1, non-blocking send) and calls `Take() Changes{Paths []string; Resync bool}`.
**Why:** The run loop is busy during a reindex; a push channel would block the fsnotify reader and overflow the kernel queue. Pull + accumulator never blocks the producer, coalesces bursts, and carries the overflow signal. Maps directly to "events are hints".
```go
type Changes struct {
	Paths  []string // repo-relative, slash-separated, cleaned; never absolute, never ".."-prefixed
	Resync bool     // events may have been lost: treat as Invalidate(".")
}
type Watcher interface {
	Ready() <-chan struct{}
	Take() Changes
	Close() error
}
```
Conversion at the adapter boundary: `rel, err := filepath.Rel(root, abs); filepath.ToSlash(rel)`; if `err != nil` or rel starts with `..` set `Resync` instead of emitting the path (Phase 8 finding #4).

### Pattern 2: Debouncer as a pure state machine
```go
type Debouncer struct{ quiet, maxWait time.Duration; first, last time.Time; set map[string]struct{}; resync bool }
func (d *Debouncer) Add(now time.Time, c Changes)
func (d *Debouncer) Due() (deadline time.Time, ok bool)   // min(last+quiet, first+maxWait)
func (d *Debouncer) Flush(now time.Time) (Changes, bool)  // returns batch when now >= deadline
```
Defaults: quiet 150 ms, maxWait 1 s (continuous formatter output cannot starve the index). The loop owns one `time.Timer` and resets it to `Due()`. Unit tests pass synthetic `time.Time` values: no clock interface, no sleeps. `time` is fine here (infrastructure).

### Pattern 3: Single indexer goroutine (Phase 8 finding #1)
`run.go` is the only code that calls `Indexer.Index` (and therefore `Loader.Invalidate`/`LoadUnits`). Selects on `ctx.Done()`, `watcher.Ready()`, the debounce timer, and an optional safety ticker. Also add a `sync.Mutex` inside `terragrunt.Loader` guarding `LoadUnits`, `Invalidate`, `CacheStats` (enforces the documented contract; Phase 11's socket handler must not be able to crash the runtime). Both, cheap. Update the Loader doc comment; keep `incremental_test.go` green.

Loop sketch:
```go
for {
	select {
	case <-ctx.Done():                 publish(stopped); return nil
	case <-w.Ready():                  deb.Add(now(), w.Take()); arm timer
	case <-timer.C:                    if b, ok := deb.Flush(now()); ok { reindex(b) }
	case <-safety.C:                   // fsnotify only: stat scan feeds pending via same path
	}
}
```
`reindex`: `snap, err := idx.Index(ctx, b.Paths, b.Resync)`; if the watcher already has newer pending changes (`len(ready)>0`) skip publishing this intermediate result (avoids a GRT100 flash from a torn read, Pitfall 5) and loop again; otherwise publish status (and stdout diagnostics if changed). Never crash on a reindex error: publish a `failed` status and keep running; only the initial index failure exits 3.

### Pattern 4: Start order (no lost changes)
1. Resolve root (abs, `EvalSymlinks`), open `os.OpenRoot` for the Loader.
2. Start the watcher FIRST (install watches / take poll baseline).
3. Publish `gruntled: indexing... @ t`.
4. Initial full index (`Index(ctx, nil, false)`).
5. Publish real status; enter loop. Changes made during step 4 sit in `pending`.

### fsnotify adapter rules (`native_unix.go`)
- `filepath.WalkDir(root)` adding every directory; `fs.SkipDir` for ignored dir names; do not follow symlinked dirs (matches `discoverUnits`).
- On `Create` of a directory (also how a renamed-in directory appears: IN_MOVED_TO maps to Create): `Lstat`, recursively `Add`, and emit the directory path as dirty (`Invalidate(dir)` evicts every cached entry under it, including negative entries). Files created before the watch existed are covered because the dir path is dirty and the next load reads them.
- Any Create/Write/Remove/Rename/Chmod on a non-ignored path emits `dirty(rel)`. Never branch on event type for correctness.
- `fsnotify.ErrEventOverflow` or any Errors-channel error: set `Resync`.
- `Add` failing with `errors.Is(err, syscall.ENOSPC)` (inotify watch limit) or `syscall.EMFILE` (kqueue fds): constructor returns `ErrWatchLimit`; `cmd` falls back to poll with a stderr warning (unless `--poll` was explicit).
- Safety net (closes Phase 8 finding #2 cheaply): every 30 s run the poll scanner against its previous snapshot and feed differences into `pending`. A lost event is then bounded to 30 s of staleness; a duplicate (already seen by fsnotify) costs one extra single-file reparse. Minimal alternative if the planner wants less code: `Resync` every N minutes (full reparse, costlier on large repos). Do not add stat-on-hit to the parse cache in this phase (touches the Phase 8 equivalence model; mtime granularity makes it unsound anyway).
- Heuristic warning only (not auto-switch): on linux, if the abs root starts with `/mnt/` print "inotify may miss Windows-side edits; use --poll".

### Poll adapter (`poll.go`)
Walk with `os.Lstat`/`os.ReadDir`, skipping ignored dirs, no symlink follow. Snapshot `map[rel]{isDir bool; size int64; mtime int64 ns; mode fs.FileMode}`. Each tick (default 500 ms, flag `--poll-interval`, tests use ~10 ms): diff against previous snapshot, emit added, removed and changed paths plus, for a removed or added dir, the dir path. First snapshot is the baseline (taken before the initial index). Same-size-same-mtime edits are invisible to polling (and to the safety net); document it, and in tests change size or `os.Chtimes` forward.

### Ignore predicate (`ignore.go`, shared by both adapters and the poller)
Ignore a path when any path component is `.git`, `.terraform`, `.terragrunt-cache`, or its base name matches: suffix `~`; `.swp`/`.swo`/`.swn`/`.swx` suffix; `.#*` and `#*#` (emacs); `___jb_tmp___`, `___jb_old___` (JetBrains); `*.tmp`; `.DS_Store`; vim's probe file `4913` (pure-digit name of length 4 to 5, no extension). Keep the directory set a superset-compatible with `skipDirNames` in `terragrunt/walk.go` except `vendor` (a unit `source` may point into `vendor`; watching it is correct). Table-test it, including `.terragrunt-cache/x/y.hcl`, `a/.terraform/b`, `main.tf.swp`, `.main.tf.swp`, `terragrunt.hcl~`, and that `terragrunt.hcl`, `.terraform.lock.hcl`, `a.hcl.json` are NOT ignored.

### Loader hardening (Phase 8 findings, all in `terragrunt/loader.go`)
1. Mutex across `LoadUnits`, `Invalidate`, `CacheStats` (finding 1).
2. `Invalidate`: clean all inputs into a set once; if any is `""`/`.`/absolute/`..`-escaping, `clear` the store (fail safe, finding 4); otherwise one pass over store keys testing the key and each ancestor (`path.Dir` loop) against the set: O(cache x depth) per batch (finding 3).
3. Document the contract: repo-relative, slash-separated, rename = old and new path.
4. Finding 2: handled by `Resync` -> `Invalidate(".")` plus the safety net above. Finding 5 (stat per load) is cost-only; no action.

### Status file (DAEMON-03)
**Line format (golden-tested in presenter, UTF-8, no BOM, exactly one line + `\n`):**
```
gruntled: ok @ 14:02:11
gruntled: 1 error (GRT001×1) @ 14:02:11
gruntled: 2 errors (GRT001×1 GRT003×1) @ 14:02:11
gruntled: indexing... @ 14:02:09        (startup only)
gruntled: failed (<one-line reason, <=120 chars, no newlines>) @ 14:02:11
gruntled: stopped @ 14:09:40            (clean shutdown)
```
Codes sorted ascending, counts are errors only; `ok` means no error-severity diagnostic, matching `check` exit status. Warnings: no analyzer emits `SeverityWarning` today; if present append `, N warnings` after `ok`/the error list (discretion). Timestamp is local `15:04:05` from an injected `now func() time.Time`; the presenter takes the already-formatted stamp string (`interfaces` cannot import `time`).

**Path (documented in docs/cli.md; also printed on stderr at startup; `--status-file` overrides; a `--print-status-path` style flag is recommended so prompts can do `cat "$(gruntled watch --print-status-path .)"`):**
`<base>/gruntled/<hash12>/status` where `hash12 = hex(sha256(abs root after EvalSymlinks))[:12]` (on windows and darwin lowercasing is NOT applied; document the aliasing limitation) and `<base>` is, first match:
1. linux only: `$XDG_RUNTIME_DIR` if set and absolute (tmpfs, cleared at logout, 0700 by spec);
2. `os.UserCacheDir()` (linux `$XDG_CACHE_HOME` or `~/.cache`; darwin `~/Library/Caches`; windows `%LocalAppData%`; stdlib-documented);
3. `os.TempDir()` fallback (when `HOME` is unset).
Phase 11 should reuse the same directory for `sock` and `lock` (short: well under macOS's ~104-byte `sun_path`). Reject (exit 2) a `--status-file` that resolves (after `EvalSymlinks` of its existing parent) inside the analysed root: the "never write inside the repo" rule.
Permissions: dirs `0o700` via `MkdirAll`, file `0o600` (`os.CreateTemp` already creates 0600). On unix, if the directory exists with `mode&0o077 != 0` (possible in the `/tmp` fallback), refuse and warn instead of writing.

**Atomic write:** `os.CreateTemp(dir, ".status-*")` in the SAME directory, write line, `Close`, `os.Rename(tmp, final)`; on error remove tmp. On Windows `os.Rename` is `MoveFileEx(MOVEFILE_REPLACE_EXISTING)` (verified in GOROOT 1.27 `internal/syscall/windows`), which fails with a sharing violation if a reader holds the file open without `FILE_SHARE_DELETE` (Go's own `os.Open` does not set it): retry up to 5 times with 10/20/40/80 ms backoff, then log once to stderr and carry on (a status write failure must never kill the daemon; the next publish retries). The write function takes a `rename func(old, new string) error` seam so the retry is testable on linux.

### Process behaviour (`cmd/gruntled/watch.go`)
- Usage: `gruntled watch [--poll] [--poll-interval d] [--status-file p] [--debounce d] [path]`, reusing `parseArgs`. Exit codes keep the existing set: 0 clean shutdown on SIGINT/SIGTERM (a daemon stopped by the user is success), 2 usage error, 3 cannot start (path unreadable, initial index failed, watcher and poll fallback both failed, status file inside repo).
- Signals: `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)` in `main`; `syscall.SIGTERM` compiles on windows (only Ctrl-C is delivered there). `os/signal` links no net/exec. Keep `run(args, stdout, stderr) int` as a wrapper over `runCtx(ctx, ...)` so the 14 existing call sites and TestMain stay untouched; tests call `runCtx` with a cancellable context.
- Output: stderr gets the banner (`gruntled: watching <root> (fsnotify|poll); status: <path>`), warnings and fallback notices. stdout gets `presenter.Text` of the full diagnostics, byte-identical to `check`'s text output, only when the diagnostic set changed since the last print (`diagnostic.Set.Equal`), so a terminal user sees findings and a quiet repo stays quiet. No colour, no ANSI. Absolute root never appears on stdout (reproducibility rule).
- Backend choice: `--poll` or `runtime.GOOS=="windows"` -> poll; otherwise `NewNative`, falling back to poll on `ErrWatchLimit`/`ErrNativeUnavailable`.
- No fork/daemonize (out of scope); document `gruntled watch &`, tmux, systemd.

### Proof extension (must land in the same wave as `go get fsnotify`)
`scripts/check-architecture.sh` Step 9, spawner scan: change the `go list -f` template to skip import path `golang.org/x/sys/unix` (`{{if and (not .Standard) (ne .ImportPath "golang.org/x/sys/unix")}}`), state the reason in the comment block (it only declares `unix.Exec`; reachability is disproved below), and add a per-target linker check: `GOOS/GOARCH go build -o "$tmp" ./cmd/gruntled` then `go tool nm "$tmp"` must contain none of `os.StartProcess`, `syscall.forkExec`, `syscall.ForkExec`, `syscall.Exec`, `unix.Exec` (measured: absent for an fsnotify-linking binary, present when `os/exec` is linked, so the check can fail). Cross-GOOS `go tool nm` reads ELF/Mach-O/PE. Also add: windows targets must not list `github.com/fsnotify/fsnotify` or `golang.org/x/sys/windows` in `-deps`. Add matching self-test cases to `scripts/test-check-architecture.sh` (author them, but the orchestrator said not to run that script here). DAEMON-06 formally closes in Phase 11 but CI enforces the script now.

### Anti-Patterns to Avoid
- Event-type-driven invalidation (Rename vs Write): editors disagree; always dirty(path).
- Watching individual files: inode swaps on save lose the watch; watch directories.
- Push channel of events from the adapter to a busy loop (blocks the reader, overflows the kernel queue).
- `time.AfterFunc` or `sync` in `application` (violates allowlist).
- Assertions after `time.Sleep`; per-event goroutines touching the Loader.
- Status or lock files under the repo; status line with colour or multiple lines.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| inotify/kqueue | raw syscalls | fsnotify v1.10.1 (`!windows` only) | Edge cases (move-self, ignored, overflow) already handled |
| Atomic file replace | write-in-place or custom locking | `os.CreateTemp` same dir + `os.Rename` | Atomic on POSIX; replace-existing on windows |
| Per-user dirs | `$HOME` string concat | `os.UserCacheDir`, `os.TempDir` | Correct per-OS rules |
| Repo hashing | custom hash | `crypto/sha256` + `encoding/hex` | stdlib, deterministic |
| Signal plumbing | manual `signal.Notify` + chans | `signal.NotifyContext` | Context-native shutdown |
| Diagnostic set comparison | text diffing | `diagnostic.Set.Equal` / `Diff` | Already exists and canonical |
| Full-check semantics | a second analysis path | `checking.Check` | Keeps watch == check by construction |

**Key insight:** correctness lives in the Phase 8 cache and `checking.Check`; the daemon must only guarantee that every change eventually reaches `Invalidate` (hints + safety net + Resync) and that one goroutine drives the Loader.

## Common Pitfalls

### Pitfall 1: fsnotify trips the spawner scan (NEW, measured)
**What goes wrong:** `binary-no-net-no-exec` fails on 4 of 6 targets with `.../x/sys@v0.46.0/unix/syscall_unix.go` flagged, although the import deny-list is clean.
**Why:** textual grep matches `syscall.Exec` inside `unix.Exec`, which the linker removes as unreachable.
**How to avoid:** the Proof extension above, in the same commit that adds the dependency. Verify with `bash scripts/check-architecture.sh` (not the self-test script).
**Warning signs:** CI red on the `go get` commit only.

### Pitfall 2: Events dropped or bursts coalesced
Overflow, `git checkout`, kqueue limits. Mitigation: `Resync` -> `Invalidate(".")`, 30 s stat safety net, accumulator never blocks.

### Pitfall 3: Lost changes between index and watch start
Start watcher before the initial index (Pattern 4).

### Pitfall 4: Torn reads / transient missing file on save (vim rename dance)
Debounce 150 ms trailing; skip publishing an intermediate result when more changes are already pending; ignore `4913`, `~`, swap files.

### Pitfall 5: Windows rename over an open reader
Retry with backoff; never fatal.

### Pitfall 6: Path leakage and the repo-relative contract
Everything entering `Invalidate` is repo-relative slash; adapters convert with `filepath.Rel` + `ToSlash`; escapes become `Resync`. Windows paths with backslashes must never reach the Loader.

### Pitfall 7: Self-triggering
Status file outside the repo and rejecting `--status-file` inside it prevents the daemon from reindexing its own output.

### Pitfall 8: Flaky real-watcher tests
See Validation: conditions with deadlines, sentinel files for negative assertions, never `Sleep` then assert. CI runs `go test -race` on ubuntu only, so the accumulator must be genuinely race-free even though it cannot be checked locally (no C compiler).

### Pitfall 9: Stale status after a crash
A killed process leaves its last line. Clean exit writes `stopped`; a crash cannot. Document; Phase 11's lock/socket gives real liveness. Do not add a pid to the line (the requirement fixes the format).

## Code Examples

### Native adapter skeleton (`native_unix.go`)
```go
//go:build !windows

package watch

import (
	"errors"
	"syscall"

	"github.com/fsnotify/fsnotify"
)

var ErrWatchLimit = errors.New("watch: OS watch limit reached")

func NewNative(root string) (Watcher, error) {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	n := &native{root: root, fw: fw, p: newPending()}
	if err := n.addTree(root); err != nil { // WalkDir + fw.Add per non-ignored dir
		fw.Close()
		if errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EMFILE) {
			return nil, ErrWatchLimit
		}
		return nil, err
	}
	go n.loop() // drains fw.Events/fw.Errors into n.p until fw is closed
	return n, nil
}
```
```go
//go:build windows

package watch

import "errors"

var ErrNativeUnavailable = errors.New("watch: native watcher not available on windows")
var ErrWatchLimit = errors.New("watch: OS watch limit reached")

func NewNative(string) (Watcher, error) { return nil, ErrNativeUnavailable }
```
Source for fsnotify API (`NewWatcher`, `Add`, `Events`, `Errors`, `ErrEventOverflow`, `Op.Has`): module cache `fsnotify@v1.10.1/fsnotify.go`.

### Atomic status write
```go
func Write(dir, line string, rename func(a, b string) error) error {
	f, err := os.CreateTemp(dir, ".status-*")
	if err != nil { return err }
	tmp := f.Name()
	if _, err := f.WriteString(line + "\n"); err != nil { f.Close(); os.Remove(tmp); return err }
	if err := f.Close(); err != nil { os.Remove(tmp); return err }
	var rerr error
	for i, d := 0, 10*time.Millisecond; i < 5; i, d = i+1, d*2 {
		if rerr = rename(tmp, filepath.Join(dir, "status")); rerr == nil { return nil }
		time.Sleep(d) // only reached on failure; windows sharing violations
	}
	os.Remove(tmp)
	return rerr
}
```

### Presenter (pure, golden-tested)
```go
func StatusLine(w io.Writer, diags diagnostic.Set, stamp string) error // "ok" or "N error(s) (CODE×n ...)"
```
Count errors per `d.Code()` with `map[string]int`, sort keys, join `"%s×%d"`; singular `1 error`, plural otherwise (existing `Summary` avoids pluralisation for determinism; this line follows the requirement's literal examples, so fix both forms in the golden).

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Watch individual files | Watch directories, treat events as dirty hints | long-standing | Survives atomic-rename saves |
| fsnotify event-type logic | Stat/dirty-set reconciliation | n/a | Editor-agnostic |

**Deprecated/outdated:** `github.com/howeyc/fsnotify`, `radovskyb/watcher`, `rjeczalik/notify` (unmaintained or redundant); gofrs/flock and x/sys/windows (excluded by the proof).

## Open Questions

1. **Should `watch` print findings to stdout at all?**
   - Known: requirement text only mandates the status file.
   - Recommendation: yes, on change only, identical to `check` text (cheap, makes the foreground use useful, testable). Revisit if reviewers want a `--quiet`.
2. **Safety-net cadence (30 s) and poll default (500 ms)**
   - No measured data on a 5k-unit repo; both are flags/constants, tune later. LOW confidence on defaults, HIGH on mechanism.
3. **Darwin kqueue behaviour and Windows polling were not run** (CI is ubuntu-only). Cross-compile in verification; consider a macOS/Windows CI job in Phase 11.
4. **Warning-severity wording in the status line** is unspecified; currently moot (no warning diagnostics exist).
5. **Stale status after crash** accepted for this phase (Pitfall 9).

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` (go1.27), `testing/fstest`, rogpeppe/go-internal testscript v1.16.0, pgregory.net/rapid v1.3.0 (existing) |
| Config file | none; `go.mod`; CI `.github/workflows/ci.yml` runs `go test -race -count=1 ./...` (ubuntu only) |
| Quick run command | `go test -count=1 ./internal/infrastructure/watch/... ./internal/infrastructure/statusfile/... ./internal/infrastructure/terragrunt/... ./internal/application/watching/... ./internal/interfaces/presenter/... ./cmd/gruntled/ -run 'Watch\|Status\|Ignore\|Debounce\|Invalidate\|Contract\|Incremental'` |
| Full suite command | `go test -count=1 ./... && bash scripts/check-architecture.sh && GOOS=windows GOARCH=arm64 go vet ./... && GOOS=darwin GOARCH=arm64 go vet ./...` |

No `-race` locally (no C compiler); CI provides it, so design for it.

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DAEMON-01 | Ignore predicate table (.git, .terraform, .terragrunt-cache, swap/backup, vim 4913; real files not ignored) | unit | `go test ./internal/infrastructure/watch -run TestIgnored` | Wave 0 |
| DAEMON-01 | Debouncer: trailing 150 ms, max wait 1 s, resync sticky, synthetic times | unit | `go test ./internal/infrastructure/watch -run TestDebouncer` | Wave 0 |
| DAEMON-01 | Watcher contract (create/modify/delete/rename, mkdir with files, rmdir, vim-style save, ignored dirs via sentinel) run against poll AND native | integration | `go test ./internal/infrastructure/watch -run TestWatcherContract` | Wave 0 |
| DAEMON-01 | poll and native yield same final status for same scripted fs changes; equals full `checking.Check` | integration | `go test ./cmd/gruntled -run TestWatchParity` | Wave 0 |
| DAEMON-01 | Run loop: initial index then only dirty paths reindexed (Loader CacheStats misses == changed files) | integration | `go test ./cmd/gruntled -run TestWatchReindexesOnlyChanged` | Wave 0 |
| DAEMON-01 | Phase 8 hardening: Invalidate batch/contract (absolute, `..`, "." clear all), concurrent Invalidate+LoadUnits no crash, Resync == full | unit | `go test ./internal/infrastructure/terragrunt -run 'Invalidate\|Concurrent\|Incremental'` | partly (incremental_test.go exists) |
| DAEMON-01 | native -> poll fallback on `ErrWatchLimit` (fake constructor), `--poll` forced | unit | `go test ./cmd/gruntled -run TestWatchBackendSelection` | Wave 0 |
| DAEMON-01 | Clean shutdown on ctx cancel: exit 0, final `stopped` status; usage/exit codes (2, 3, status file inside repo) | testscript + go test | `go test ./cmd/gruntled -run 'TestScript/watch_'` | Wave 0 |
| DAEMON-03 | Status line golden: ok / 1 error / 2 errors with codes / indexing / failed / stopped, fixed stamp | unit golden | `go test ./internal/interfaces/presenter -run TestStatusLine` | Wave 0 |
| DAEMON-03 | Path resolution per OS using injected goos/env: XDG_RUNTIME_DIR, cache dir, temp fallback, hash stable, symlink alias same hash, outside repo, 0700/0600 | unit | `go test ./internal/infrastructure/statusfile -run TestDir` | Wave 0 |
| DAEMON-03 | Atomic write: concurrent reader never sees partial line; no leftover tmp; rename retry via seam | unit | `go test ./internal/infrastructure/statusfile -run 'TestWrite'` | Wave 0 |
| DAEMON-03 | Status file readable and equals expected line after a save in a live `watch` | integration | `go test ./cmd/gruntled -run TestWatchStatusFile` | Wave 0 |
| (proof, CI) | fsnotify linked: Step 9 passes on 6 targets; windows has no fsnotify | script | `bash scripts/check-architecture.sh` | exists, needs edit |

### Test technique rules (flake control)
- Helper `eventually(t, 10*time.Second, func() bool)` polling every 5 ms; fails with the last observed state. No `time.Sleep` followed by an assertion anywhere.
- Negative assertion (ignored path produced no event): write the ignored file, THEN a sentinel real file; once the sentinel appears assert the ignored path was never collected. Ordering holds for one inotify instance and for a single poll scan.
- Contract tests in package `watch` (`contract_test.go` with `runContract(t, newWatcher func(root string) (Watcher, error))`); `poll_test.go` always runs it; `native_unix_test.go` (`//go:build !windows`) runs it with `NewNative`. Poll interval 10 ms, debounce 5 to 10 ms in loop tests, so the whole suite stays in low seconds.
- Poll tests change size or `os.Chtimes` forward to avoid same-mtime-same-size misses.
- Clock: loop and CLI tests inject `now` returning a fixed `time.Date(..., time.UTC)`; status stamp assertions are exact.
- Use `t.TempDir()` for repo and `--status-file` in a different `t.TempDir()`.
- Parity test drives the same op script (via a `[]op` table of real fs calls, each followed by `eventually` on the expected status line) through both backends.

### Sampling Rate
- **Per task commit:** the quick run command for touched packages.
- **Per wave merge:** full suite command.
- **Phase gate:** full suite green (including `check-architecture.sh` and the two cross-vets) before `/gsd:verify-work`.

### Wave 0 Gaps
- [ ] `internal/infrastructure/watch/{ignore,debounce,contract,poll,native_unix}_test.go`
- [ ] `internal/infrastructure/statusfile/{path,write}_test.go`
- [ ] `internal/interfaces/presenter/status_test.go` + golden strings
- [ ] `internal/application/watching/watching_test.go` (fake `InvalidatingLoader`: dirty paths forwarded, resync -> "." )
- [ ] `cmd/gruntled/watch_test.go` and `testdata/script/watch_usage.txtar`, `watch_exitcodes.txtar`
- [ ] `internal/infrastructure/terragrunt` tests: batch Invalidate, path contract, concurrent use
- [ ] `scripts/check-architecture.sh` + `scripts/test-check-architecture.sh` cases (nm check, x/sys/unix exemption pinned, windows-no-fsnotify)
- [ ] Framework install: only `go get github.com/fsnotify/fsnotify@v1.10.1`

## Sources

### Primary (HIGH confidence)
- Probe module, `go list -e -deps` per GOOS/GOARCH with CGO_ENABLED=0, Go 1.27.0, fsnotify v1.10.1 + x/sys v0.46.0 (this session): linux/darwin clean, windows links `net`, `net/netip`, `x/sys/windows`.
- `go build` + `go tool nm` on an fsnotify-linking binary vs. one linking `os/exec` (this session): spawner symbols absent vs present.
- `~/go/pkg/mod/golang.org/x/sys@v0.46.0/unix/syscall_unix.go:582` (`syscall.Exec`), and the repo's `scripts/check-architecture.sh` Step 9 regex.
- `~/go/pkg/mod/github.com/fsnotify/fsnotify@v1.10.1` (`go.mod`, `fsnotify.go`, `backend_inotify.go`, `README.md`): API, ErrEventOverflow, move-self handling, limits, recursive not public.
- GOROOT 1.27 `os/file_windows.go`, `internal/syscall/windows` (rename = `MoveFileEx` replace-existing), `syscall/syscall_windows.go` (share mode READ|WRITE only).
- Repo: `loader.go`, `parse.go`, `walk.go`, `ports.go`, `check.go`, `main.go`, `08-VERIFICATION.md` Security, v0.3 research docs, REQUIREMENTS/STATE.

### Secondary (MEDIUM confidence)
- v0.3 `PITFALLS.md` editor save patterns and OS watcher behaviours (documented behaviour, not re-verified online).

### Tertiary (LOW confidence)
- Default intervals (500 ms poll, 30 s safety net, 5 attempts rename retry): engineering judgement, unmeasured.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - measured dependency graph and symbols
- Architecture: HIGH on layering fit (rules read from the script), MEDIUM on loop details (design, unbuilt)
- Pitfalls: MEDIUM-HIGH - Pitfall 1 measured; OS watcher quirks documented but not run on darwin/windows

**Research date:** 2026-10-08
**Valid until:** 2026-11-07 (re-run the six-target proof after any dependency bump)
