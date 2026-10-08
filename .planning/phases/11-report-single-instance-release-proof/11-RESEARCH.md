# Phase 11: Report, Single Instance & Release Proof - Research

**Researched:** 2026-10-08
**Domain:** Go raw AF_UNIX IPC without `net`, single-instance locking, daemon diagnostics query, six-target no-net/no-exec proof
**Confidence:** HIGH on the socket/lock approach and the proof (probe built and run on linux, cross-compiled and vetted for darwin and windows); MEDIUM on darwin and windows runtime behaviour (never executed here; CI runs ubuntu only)

<user_constraints>
## User Constraints (from CONTEXT.md)

No CONTEXT.md exists for this phase (no discuss-phase run). Binding inputs instead:

### Locked Decisions
- Requirements DAEMON-04, DAEMON-05, DAEMON-06 as worded in REQUIREMENTS.md (unix socket on linux/darwin, status-file-based `report` on windows, lock + socket detection, proof on six targets).
- v0.3 unattended-run decisions (research SUMMARY.md): raw syscall sockets on linux/darwin; windows has no socket; proof stays strict ("cannot", not "does not"); DAEMON-05 = detect running instance, print where, exit 0, no multiplexed client; never `x/sys/windows`, gofrs/flock, gRPC, go-winio.
- Project instruction: no tags/releases in this phase. Any release step is a checkpoint for the person.
- Do not commit the research file.

### Claude's Discretion
- Which mechanism implements the socket (resolved below), lock sequence, wire protocol, windows diagnostics source, exit codes of `report`, whether to fold in phase-10 low findings.

### Deferred Ideas (OUT OF SCOPE)
- Multiplexed client / streaming attach, daemon-held blast baseline over the socket (`blast` op), `rebase`, Windows IPC, peer-credential checks.
</user_constraints>

<phase_requirements>
## Phase Requirements

| ID | Description | Research Support |
|----|-------------|-----------------|
| DAEMON-04 | `gruntled report` returns running daemon's diagnostics in text/json/sarif like `check`; unix socket on linux/darwin, status-file based on windows; no daemon -> non-zero, never starts one | Stdlib `syscall` raw AF_UNIX (probe-verified), snapshot of pre-rendered bytes, windows `report` dump file next to status file, lock-based liveness |
| DAEMON-05 | Second `watch` on same repo detects daemon (lock + socket, stale socket recovered), prints where, exits 0; crashed daemon leaves no blocking lock | Lock-first sequence with `syscall.Flock` (kernel-released) / windows exclusive-open `syscall.CreateFile` (OS-released); unlink stale socket only while holding lock |
| DAEMON-06 | No-net/no-exec proof passes on all six targets with watcher and socket linked | Probe: stdlib `syscall` AF_UNIX adds zero `net*`/`os/exec` packages and zero `sym_re` symbols on all six targets; Step 9 needs no exemption change, only new self-test cases |
</phase_requirements>

## Summary

The tension resolves cleanly with option (a), and it needs even less than the brief assumed: **the stdlib `syscall` package alone is enough** (no `golang.org/x/sys/unix` import, so no new Step 9 exemption question). `syscall.Socket/Bind/Listen/Accept/Connect/SockaddrUnix/Flock/SetNonblock/CloseOnExec/Shutdown/SetsockoptTimeval` all exist on linux and darwin. Wrapping the fds with `os.NewFile` (non-blocking mode) gives a runtime-poller-integrated `*os.File`: deadlines work, and `Close()` wakes a blocked accept (both verified on linux; darwin uses the same kqueue poller path). I built a probe with lock, listen, accept, dial, echo, deadline, close-wakes-accept and stale-socket refusal; it ran correctly on linux. For all six targets (`linux|darwin|windows` x `amd64|arm64`) `go build` succeeded, `go list -deps` shows no `net`, `net/*`, `os/exec`, and `go tool nm` shows no `sym_re` symbol. Baseline `scripts/check-architecture.sh` on the current tree passes (24 s).

Windows keeps no socket (locked decision). `syscall.CreateFile` with share mode 0 (stdlib, no `x/sys/windows`) gives an exclusive, crash-safe lock; it compiled and vetted for windows/amd64 (not run). Windows `report` needs full diagnostics, but the status file is one line: the daemon therefore also writes an atomic 0600 **snapshot file** (`report`) beside the status file on windows. Liveness on windows is a lock probe, not file existence.

Phase 10's open Low finding 2 (status dir `os.Stat` + mode only, no owner/symlink check) stops being optional: the socket and lock live in the same per-repo directory, so a hijacked directory becomes a socket-hijack. Fold it in. Finding 3 (control bytes in status reason) is cheap and touches `status.go`; fold it in too.

**Primary recommendation:** new package `internal/infrastructure/ipc` built only on stdlib `syscall` (+`os`, `encoding/json`), lock-first single-instance sequence, a pre-rendered `Snapshot{Text,Summary,JSON,SARIF []byte}` served over the socket on unix and written to a dump file on windows, no changes to domain/application/interfaces layers except a presenter sanitiser, and four new Step 9 self-test cases.

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| stdlib `syscall` | Go 1.27 | AF_UNIX socket, `Flock` (unix), `CreateFile` (windows) | Only way to get sockets/locks with no `net` and no `x/sys/windows`; probe-verified clean on 6 targets |
| stdlib `os` | Go 1.27 | `os.NewFile` over fds -> pollable files with deadlines | Close-wakes-accept and deadlines verified |
| stdlib `encoding/json` | Go 1.27 | Request/response framing, snapshot file | Already used by presenters; `[]byte` fields are base64 so output is byte-exact |
| `internal/infrastructure/statusfile` | repo | `Dir(root, env)` per-repo dir, atomic `Writer` | Reuse for lock, socket and snapshot paths; extend with `EnsureDir` |

### Supporting
| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `testscript` (rogpeppe/go-internal v1.16.0) | existing | `report` with no daemon, `-h` text | Only for scenarios without a live daemon |
| `pgregory.net/rapid` | existing | not needed this phase | - |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| stdlib `syscall` | `golang.org/x/sys/unix` | Already linked (via fsnotify) and exempt in Step 9 half 2, but adds nothing needed; x/sys bumps then risk the spawner-name list. Do not import. |
| raw AF_UNIX (a) | status/dump file on all platforms (b) | Works everywhere but violates the DAEMON-04 wording and gives no liveness proof; not needed because (a) is clean. Keep (b)'s file only as the windows transport. |
| raw AF_UNIX (a) | `net.Listen("unix")` (c) | Links `net` on all 6 targets; breaks the core CLI-04 guarantee. NOT acceptable. |

**Installation:** none. No go.mod change. `go mod tidy -diff` stays clean. x/sys remains an indirect dep of fsnotify.

## Architecture Patterns

### Recommended Project Structure
```
internal/infrastructure/ipc/
├── ipc.go             # Snapshot, request/response structs, codec, caps (no build tag)
├── lock_unix.go       # //go:build linux || darwin : Flock (non-blocking, EINTR loop)
├── lock_windows.go    # //go:build windows : syscall.CreateFile share mode 0, OPEN_ALWAYS; ErrHeld on ERROR_SHARING_VIOLATION
├── lock_other.go      # //go:build !linux && !darwin && !windows : no-op lock
├── sock_unix.go       # //go:build linux || darwin : Listen/Accept/Dial/Serve
├── sock_other.go      # //go:build !linux && !darwin : ErrUnsupported stubs
└── *_test.go
internal/infrastructure/statusfile/   # + EnsureDir (Lstat, symlink reject, owner==euid on unix), shared by Writer and ipc
cmd/gruntled/report.go                # runReport + snapshot rendering (reuses check's render code)
cmd/gruntled/watch.go                 # acquire instance before openWatcher; publisher also updates snapshot
```
Use `linux || darwin`, not the `unix` tag: `syscall.Flock` is missing on some other unix GOOSes, and only linux/darwin/windows are release targets. DDD: ipc is infrastructure and cmd (composition root) wires it; no new application port is required, so the interfaces/application stdlib allowlists and platform-neutral rules are untouched (build tags are legal in infrastructure; Step 3 only polices domain/application/interfaces).

### Pattern 1: Lock-first single instance (recommended over connect-first)
**What:** (1) `Lock(dir/lock)`. (2) If acquired: nobody else is running; `Lstat` sock, remove it if it is a socket (stale), bind, chmod 0600, listen. (3) If lock is held (`EWOULDBLOCK` / `ERROR_SHARING_VIOLATION`): another daemon exists or is starting; best-effort `Dial`+`ping` with a short bounded retry to learn the pid; print where it runs; exit 0.
**Why:** the lock is the single source of truth, kernel/OS-released on crash, so there is no connect-vs-bind race and no pid file. Liveness is never inferred from file existence. (ARCHITECTURE.md suggested connect-first; lock-first is simpler and race-free. A daemon that holds the lock but has no socket yet is still "running".)
**Where in `runWatchWith`:** after the `--print-status-path` early return and before `openWatcher`/initial index, so a second `watch` is detected while the first is still indexing. Lock, socket and snapshot paths always derive from `statusfile.Dir(root, env)`, NOT from `--status-file`, so a different `--status-file` cannot defeat single instance.
**Example (probe-verified on linux; compiles for darwin/windows):**
```go
//go:build linux || darwin

func Listen(path string) (*os.File, error) {
	syscall.ForkLock.RLock()
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_STREAM, 0)
	if err == nil { syscall.CloseOnExec(fd) }
	syscall.ForkLock.RUnlock()
	if err != nil { return nil, err }
	if err = syscall.Bind(fd, &syscall.SockaddrUnix{Name: path}); err == nil {
		err = syscall.Listen(fd, 16)
	}
	if err == nil { err = syscall.SetNonblock(fd, true) }
	if err != nil { syscall.Close(fd); return nil, err }
	return os.NewFile(uintptr(fd), path), nil // non-blocking => pollable
}

func Accept(l *os.File) (*os.File, error) {
	rc, err := l.SyscallConn()
	if err != nil { return nil, err }
	var nfd int
	var aerr error
	err = rc.Read(func(fd uintptr) bool {
		nfd, _, aerr = syscall.Accept(int(fd))
		return aerr != syscall.EAGAIN // false => wait for readiness
	})
	if err != nil { return nil, err }   // os.ErrClosed once l.Close() wakes us
	if aerr != nil { return nil, aerr }
	syscall.CloseOnExec(nfd); _ = syscall.SetNonblock(nfd, true)
	return os.NewFile(uintptr(nfd), "conn"), nil // SetReadDeadline works
}

func Lock(path string) (*os.File, error) { // flock released by kernel on crash
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil { return nil, err }
	rc, _ := f.SyscallConn()
	var ferr error
	_ = rc.Control(func(fd uintptr) {
		for { ferr = syscall.Flock(int(fd), syscall.LOCK_EX|syscall.LOCK_NB); if ferr != syscall.EINTR { break } }
	})
	if ferr != nil { f.Close(); return nil, ferr } // EWOULDBLOCK => held
	return f, nil
}
```
Windows lock: `syscall.CreateFile(p, GENERIC_READ|GENERIC_WRITE, 0 /*share*/, nil, syscall.OPEN_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL, 0)`, wrap handle with `os.NewFile`; `syscall.Errno(32)` (ERROR_SHARING_VIOLATION) means held. The handle is closed by the OS on crash.

### Pattern 2: Immutable snapshot of pre-rendered bytes (byte-for-byte parity with `check`)
**What:** on every `EventReady`, the publisher renders once with the exact functions `runCheck` uses (`presenter.Text`, `presenter.Summary`, `presenter.JSON`, `presenter.SARIF(..., ToolInfo{Version: version})`) into a `Snapshot{Version, PID, Generation, State, Text, Summary, JSON, SARIF []byte, HasErrors bool}` and stores it in an `atomic.Pointer[Snapshot]` (unix, served by the socket) and, on windows, writes it to the dump file. `report` prints the selected field verbatim; no decoding of diagnostics, no second renderer. Refactor `runCheck`'s switch into one shared `render(format, rep)` so parity is structural, not tested-in.
- Fields are `[]byte`, not `string`: `encoding/json` replaces invalid UTF-8 in strings with U+FFFD; `[]byte` is base64 and exact.
- `check` text prints Summary to stderr; `report --format text` prints Text to stdout and Summary to stderr the same way.
- Exit code: mirror `check` (0 clean, 1 at least one error diagnostic via `HasErrors`), and use 3 (exitFailure) for "no daemon"/transport/indexing-not-ready. Recommendation; document in `report -h` and docs/cli.md. `TestHelpMatchesDocs` requires every `^  [0-3]  ` exit-code line of the new `report -h` to appear verbatim in docs/cli.md and the table in that test needs a `{"report", N}` row.
- States: before the first Ready the snapshot has no bytes: server answers `state:"indexing"`, `report` exits 3 "daemon is still indexing". After a failed reindex keep serving the last good bytes and have `report` warn on stderr (`last reindex failed: <reason>; showing the previous result`). Reason text must be sanitised (below).

### Pattern 3: Wire protocol
One JSON line each way, versioned, bounded:
- Request: `{"v":1,"op":"ping"|"report"}\n`, server reads at most 4 KiB with a 2 s read deadline, one request per connection, then closes.
- Response: one JSON object, `{"v":1,"ok":true,"snapshot":{...}}` or `{"v":1,"ok":false,"error":"..."}`. Unknown `v` or `op` -> `ok:false`. Client sets 2 s deadlines, caps response size (e.g. 64 MiB via `io.LimitReader`), and treats a version mismatch as a clear error ("daemon runs gruntled X, you run Y").
- Server loop: one goroutine accept loop, goroutine per connection with `sync.WaitGroup`; shutdown = `listener.Close()` (wakes Accept), wait for handlers, `os.Remove(sock)`, release lock. Handlers only read the `atomic.Pointer`, never touch the Loader (single-indexer invariant from phase 10 stays intact). Assumption to prove under `-race`: the `checking.Report`/graph is not mutated after publish (the snapshot holds only rendered bytes, which sidesteps this).

### Pattern 4: Windows report path
Dump file `Dir/report` (atomic via existing `statusfile.Writer`, 0600, dir 0700). Liveness: probe the lock with `CreateFile(OPEN_EXISTING, share 0)`; success => no daemon (close immediately, report "no daemon running", exit 3, even if a stale dump exists); `ERROR_SHARING_VIOLATION` => daemon alive, read the dump. Because a probe briefly holds the file exclusively, the daemon's own lock attempt needs 2-3 short retries before concluding "held" (injected sleep, no real sleeps in tests). Remove the dump on clean shutdown. Test the file transport on linux with an injected `goos` (existing `watchDeps.goos` seam) so the windows branch is covered without a windows runner.
Document the limitation: windows `report` reads a file the daemon rewrites after each index, so it is the last published result, with no live query and no version negotiation.

### Anti-Patterns to Avoid
- **Calling `f.Fd()` on a socket `*os.File`:** it forces blocking mode and silently breaks Close-wakes-Accept and deadlines. Use `SyscallConn()`.
- **Inferring liveness from file existence** (sock or lock): only lock acquisition/probe counts.
- **Deleting the lock file** on exit: unlink races a starting daemon that already opened it. Leave it; the kernel lock is what matters.
- **`syscall.Umask` around bind:** process-wide, races other goroutines creating files. Rely on dir 0700 (enforced by `EnsureDir`) plus `os.Chmod(sock, 0o600)` after bind.
- **Putting lock/sock/dump under the repo or keyed off `--status-file`.**
- **Importing `net`, `os/exec`, `x/sys/windows`** anywhere, including `_windows.go`/`_unix.go` files (the six-target loop sees them; see Step 9).
- **Writing the words `syscall.Exec`/`ForkExec`/`StartProcess`/`CreateProcess` in comments:** the half-2 source grep is textual.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| Per-repo runtime dir | A second path scheme | `statusfile.Dir(root, env)` | Already hashes `EvalSymlinks(Abs(root))`, short (12 hex) for `sun_path`, honours XDG_RUNTIME_DIR |
| Atomic file replace | Another tmp+rename | `statusfile.Writer` (retry/backoff on windows) | Already tested incl. sharing-violation retry |
| Output formatting | A daemon-side text/json/sarif renderer | The exact `presenter` calls `runCheck` makes | Byte parity with `check` is the requirement |
| Exclusive lock | pid file / sleep loop / gofrs/flock | `syscall.Flock` / `CreateFile` share 0 | Released on crash; gofrs links net on windows |
| Process liveness | pid-alive probing | lock probe (+ socket ping for pid) | pid reuse gives false "running" |
| Socket accept shutdown | Poll loop with sleeps | `os.NewFile` non-blocking + `SyscallConn().Read` | Poller-integrated, `Close()` wakes it |
| Dir ownership/symlink hardening | New check in ipc | `statusfile.EnsureDir` (new, shared) | One implementation for status, lock, sock, dump |

**Key insight:** every hard part (crash-safe lock, accept shutdown, atomic publish, dir path) has a stdlib or existing-repo answer; the new code is glue (~250 lines) plus tests.

## Common Pitfalls

### Pitfall 1: `sun_path` too long
**What goes wrong:** bind fails with EINVAL; darwin limit is 104 including NUL, linux 108.
**How to avoid:** before bind, check `len(path) <= 103` and fail `watch` with exit 3 and a message naming the length and `XDG_RUNTIME_DIR`/`TMPDIR`. Tests must use a short base (`os.MkdirTemp("", "g")`), not `t.TempDir()` with a long test name. Typical real paths are 50-80 chars (`/run/user/1000/gruntled/<12>/sock` = 40; `~/Library/Caches/gruntled/<12>/sock` ~ 60 + username).
**Warning signs:** EINVAL from Bind on macOS CI.

### Pitfall 2: Stale socket recovery done unsafely
**What goes wrong:** unlinking a socket a live daemon owns.
**How to avoid:** unlink only while holding the lock; `Lstat` first, remove only if `ModeSocket` (never follow or delete a symlink/regular file there; error out instead).

### Pitfall 3: Directory hijack on the TempDir fallback (Phase 10 finding 2, now security-relevant)
**What goes wrong:** with HOME and XDG unset, base is `/tmp`; another user pre-creates `/tmp/gruntled/<hash>` as symlink or foreign-owned 0700 dir and receives the socket/lock/dump or serves a fake `report`.
**How to avoid:** `EnsureDir`: `MkdirAll 0700`, then `Lstat` (reject symlink), require mode perm `&0o077 == 0`, and on unix `Stat_t.Uid == os.Geteuid()`; ownership in a `_unix.go` file, no-op on windows. Apply also to the parent `<base>/gruntled`. Client side (`report`) must call the same check before dialing (do not create the dir; if absent, "no daemon").

### Pitfall 4: Accept errors treated as fatal
**How to avoid:** `EINTR`, `ECONNABORTED`, `EMFILE` -> continue (with `EMFILE` backoff); only `os.ErrClosed` ends the loop. Per-connection deadlines so a silent client cannot wedge a handler; cap request size.

### Pitfall 5: Flaky tests
**How to avoid:** no sleep-then-assert (project rule). Wait on conditions with the existing `eventually` helper (10 s deadline) in `cmd/gruntled/watch_test.go`; the daemon's readiness is observable by `report`/ping succeeding. Inject sleep/clock for lock retry and ping retry.

### Pitfall 6: Untested platforms
**What goes wrong:** darwin accept/poller and windows lock are compiled but never run; `ci.yml` has only `ubuntu-latest` jobs.
**How to avoid:** add a small `os: [ubuntu, macos, windows]` matrix for `go test ./internal/infrastructure/ipc/... ./internal/infrastructure/statusfile/... ./cmd/...` (or at least the ipc and statusfile packages), because these are the only runs that can exercise them. Keep the other jobs ubuntu-only (project rule: few jobs, each able to fail). If the person declines CI changes, record darwin/windows runtime as unverified in VERIFICATION.

### Pitfall 7: x/sys bump or new file silently reintroduces `net`
**How to avoid:** Step 9 already runs per target; add self-test cases (below) so the rule demonstrably fails on a `net` import inside an ipc-style `linux || darwin` file.

### Pitfall 8: Control bytes in output (Phase 10 finding 3)
**How to avoid:** in `presenter.StatusFailed` map `unicode.IsControl` runes to space before `strings.Fields` and the 120-rune cut (`unicode` is already in the interfaces allowlist); add an exported presenter helper (e.g. `SanitizeReason`) used by `watch.go:251`'s stderr message. The same reason string now reaches `report` stderr.

## Code Examples

### Dial with connect timeout (probe-verified)
```go
fd, _ := cloexecSocket()
tv := syscall.NsecToTimeval(int64(timeout))
_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_SNDTIMEO, &tv)
if err := syscall.Connect(fd, &syscall.SockaddrUnix{Name: path}); err != nil {
	syscall.Close(fd); return nil, err // ECONNREFUSED / ENOENT => no daemon
}
_ = syscall.SetNonblock(fd, true)
f := os.NewFile(uintptr(fd), "conn")
_ = f.SetDeadline(time.Now().Add(2 * time.Second)) // returns nil: pollable; verified ErrDeadlineExceeded fires
```
Half-close for request/response: write the request line then read until newline; no shutdown needed with line framing.

### Probe results (this machine, Go 1.27.0, evidence)
```
lock2 (same process, second open)   -> resource temporarily unavailable   (flock held)
listen / dial / accept / echo       -> ok, socket mode Srw-------
accept after listener Close()       -> use of closed file (woken, not hung)
dial after listener closed          -> connection refused (stale socket detectable)
SetReadDeadline on dialed conn      -> os.ErrDeadlineExceeded after 100ms
6 targets: go list -deps | grep -E '^(net|net/.*|os/exec)$'  -> empty
6 targets: go tool nm | grep sym_re  -> empty
GOOS=darwin go vet, GOOS=windows go vet -> clean
```
Probe source: `/tmp/claude-1000/-home-giulio/ed1f8183-a6f3-487e-b228-8923c72fcb0b/scratchpad/probe/` (scratch, not part of the repo; `ipc_unix.go`, `ipc_windows.go` are a direct starting point).

## Step 9 / Release proof (DAEMON-06)

- Current state: `bash scripts/check-architecture.sh` passes (4 domain, 5 application, 1 interfaces packages). fsnotify already links on linux/darwin and is excluded on windows (half 4).
- Expected effect of this phase: none on half 1 (no new std package beyond `syscall`/`os`/`encoding/json`/`sync`), none on half 3 (`syscall.Socket/Bind/Accept/Flock/CreateFile` are not in `sym_re`). Half 2's textual scan only matches `syscall.(ForkExec|Exec|StartProcess|CreateProcess|CreateProcessAsUser)` and `os.StartProcess`: `syscall.ForkLock` and `syscall.CreateFile` do not match. No exemption change; `x/sys/unix` is not imported by new code.
- Plan must still: (1) re-run the script after the ipc package lands (24 s), (2) add self-test cases in `scripts/test-check-architecture.sh` using the existing `mkcopy`/`run_case`/`run_case_msg` pattern: `binary-net-in-unix-socket-file` (a `zz_probe_unix.go` with `//go:build linux || darwin` and `import _ "net"` must fail on linux/darwin targets only), `binary-net-in-windows-ipc-file`, and a positive case `binary-raw-socket-allowed` (`syscall.Socket`/`Bind`/`Accept` in a probe passes), (3) update the Step 9 header comment to say sockets/locks are stdlib `syscall`, and README/docs "Guarantees" wording stays true.
- `release_test.go` checks `release_targets` matches `scripts/build-release.sh`; do not touch the lists.
- Optional hardening worth a task: a one-line assertion in Step 9 half 4 style that the *windows* deps do not include `syscall`-level net (already implied by half 1).
- **Checkpoint (person):** no tag/release is cut in this phase. Release verification beyond `build-release.sh` in CI is a human checkpoint.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| Unix sockets via `net` | `os.NewFile` over raw fds + `SyscallConn().Read` | long-standing, verified on Go 1.27 | Poller-integrated without `net` |
| Blocking accept + sleep loop (STACK.md suggestion) | Non-blocking fd via `os.NewFile` | this research | Close wakes Accept; no 100 ms sleep loop, no darwin hang |
| Connect-first then lock (ARCHITECTURE.md) | Lock-first | this research | No race; lock is the truth |

**Deprecated/outdated in earlier milestone docs:** the "non-blocking accept with 100 ms sleep" advice (PITFALLS #12) is superseded by the poller approach; keep a deadline-based fallback only if a darwin CI run proves `SetDeadline` returns `ErrNoDeadline`.

## Open Questions

1. **Does the darwin runtime behave like linux for `os.NewFile` non-blocking sockets and flock?**
   - Known: compiles and vets for darwin/amd64 and arm64; Go's `os.NewFile` poller path is OS-generic (kqueue on darwin).
   - Unclear: never executed. Recommendation: macos CI job (Pitfall 6); mark runtime MEDIUM until it passes.
2. **Windows lock probe/daemon retry exactness (ERROR_SHARING_VIOLATION = 32 from `syscall.CreateFile`).**
   - Known: share mode 0 excludes other opens; handle released on process death. Unclear: not run. Recommendation: windows CI job or declare unverified; keep retries injectable.
3. **Report exit code semantics** (mirror `check`'s 0/1 vs always 0). Recommended: mirror `check`, 3 for transport/no-daemon/not-ready. Planner may confirm with the person; low cost to change.
4. **Where `watch` prints "already running"** (stdout vs stderr). Recommended stdout, single line: `gruntled: already watching <root> (pid N); socket: <p>; status: <p>`; windows omits socket and pid. Must exit 0.
5. **`sun_path` overflow behaviour:** hard exit 3 (recommended) vs run without socket and warn.
6. **Snapshot cost on huge repos:** eager rendering of 3 formats per Ready; measure on the denis256 corpus in a bench; lazy rendering (unix only) is the fallback.

## Validation Architecture

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go `testing` + testscript v1.16.0 (`cmd/gruntled/main_test.go` `TestScripts`); `-race` in CI |
| Config file | none; txtar in `cmd/gruntled/testdata/script/` |
| Quick run command | `go test -race -count=1 ./internal/infrastructure/ipc/... ./internal/infrastructure/statusfile/... ./internal/interfaces/presenter/... && go test -race -count=1 -run 'Watch|Report|Instance|TestScripts|TestHelpMatchesDocs' ./cmd/gruntled/` |
| Full suite command | `go test -race -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` |

### Phase Requirements -> Test Map
| Req ID | Behavior | Test Type | Automated Command | File Exists? |
|--------|----------|-----------|-------------------|-------------|
| DAEMON-04 | In-process daemon + `report` in text/json/sarif equals `check` on same repo, byte-for-byte (stdout; text stderr summary too), after an edit too | integration (Go, `startWatch` + `eventually`) | `go test -race -run TestReportParity ./cmd/gruntled/` | no - Wave 0 |
| DAEMON-04 | ipc server/client round trip, bad version/op, oversize request, silent client times out, Close wakes Accept, sock mode 0600 | unit | `go test -race ./internal/infrastructure/ipc/` | no - Wave 0 |
| DAEMON-04 | windows branch: dump file written atomically 0600, `report` reads it with injected goos=windows; stale dump + free lock => "no daemon" | unit/integration | `go test -run TestReportWindowsFile ./cmd/gruntled/` | no - Wave 0 |
| DAEMON-04 | `report` with no daemon: exit 3, clear message, creates no dir/file/daemon | testscript | `go test -run 'TestScripts/report_nodaemon' ./cmd/gruntled/` | no - Wave 0 |
| DAEMON-04 | `report` while initial index running -> exit 3 "still indexing"; after failed reindex -> last good + warning | integration | `go test -run TestReportStates ./cmd/gruntled/` | no - Wave 0 |
| DAEMON-05 | Second `watch` on same repo (even with different `--status-file`) prints location, exits 0, first daemon unaffected | integration | `go test -race -run TestWatchSecondInstance ./cmd/gruntled/` | no - Wave 0 |
| DAEMON-05 | Stale sock file (regular leftover socket, no listener) and unheld lock -> restart succeeds, `report` works | unit+integration | `go test -run 'TestStale' ./internal/infrastructure/ipc/ ./cmd/gruntled/` | no - Wave 0 |
| DAEMON-05 | True crash: helper subprocess (re-exec of test binary, `os/exec` is test-only so allowed) takes lock + socket, is SIGKILLed, parent acquires lock and rebinds; sync via child's ready line, no sleeps | integration (unix only, `//go:build linux \|\| darwin`) | `go test -run TestCrashRecovery ./internal/infrastructure/ipc/` | no - Wave 0 |
| DAEMON-05 | Clean shutdown removes sock, releases lock; `EnsureDir` rejects symlink, foreign-owned (unix), group/other-accessible dir | unit | `go test ./internal/infrastructure/statusfile/` | partly (write_test.go exists) |
| DAEMON-06 | Six-target no-net/no-exec with ipc linked; self-tests pin failure on `net` in linux/darwin and windows ipc files and pass for raw socket | script | `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` | script yes; new cases Wave 0 |
| (docs) | `report -h` exit-code lines present verbatim in docs/cli.md; README documents watch, report, blast, status file path, windows limitation | unit | `go test -run 'TestHelpMatchesDocs' ./cmd/gruntled/` (+ add a README/doc keyword test) | exists; extend |
| (folded P10) | `StatusFailed` strips control runes; stderr reindex message sanitised | unit | `go test ./internal/interfaces/presenter/ -run Status` | status_test.go exists; extend |

### Sampling Rate
- **Per task commit:** the quick run command above (< 30 s).
- **Per wave merge:** full suite command.
- **Phase gate:** full suite green (incl. both architecture scripts) before `/gsd:verify-work`; if CI matrix added, macos/windows jobs green or documented as unverified.

### Wave 0 Gaps
- [ ] `internal/infrastructure/ipc/*_test.go` - protocol, lock, socket, stale, crash helper (needs a `TestMain` helper-process mode)
- [ ] `cmd/gruntled/report_test.go` - parity, states, windows-file branch, second instance
- [ ] `cmd/gruntled/testdata/script/report_nodaemon.txtar`, `report_usage.txtar`
- [ ] `scripts/test-check-architecture.sh` - the three new cases
- [ ] test env: inject `statusfile.Env` (via `watchDeps.env`) with a short temp base so tests never touch the real runtime dir and stay under `sun_path`
- [ ] `TestHelpMatchesDocs` table row for `report`
- No framework install needed.

## Planning guidance (suggested plan split)

1. **statusfile hardening** (`EnsureDir`, Lstat/owner, shared by Writer) + presenter control-char sanitiser. Small, unblocks the rest.
2. **ipc package**: lock (3 files), socket (unix + stub), protocol/codec, tests incl. crash helper. Gate with a quick `check-architecture.sh` run.
3. **Wire `watch`**: acquire instance before watcher/initial index, snapshot rendering shared with `check`, serve socket (unix) / write dump (windows), already-running message, shutdown ordering (stop server, unlink sock, release lock). Update `watchUsage`/docs.
4. **`report` command**: parseArgs reuse (`[--format] [path]`), unix dial / windows file, exit codes, `topUsage` + `reportUsage` + docs/cli.md exit lines + `TestHelpMatchesDocs`.
5. **Proof + docs + CI**: new self-test cases, Step 9 comment, README (replace the stale "Not built yet: gruntled watch / blast" line, document watch, report, blast, status file path from `watch --print-status-path`, windows limitation), docs/cli.md `report` section and Known limitations, optional CI OS matrix. Release is a human checkpoint.

Shutdown ordering note: `watch.Run` closes the Watcher on return; the ipc server must be stopped after `Run` returns (or on ctx cancel) and before the lock is released, and the lock must outlive the socket unlink.

## Sources

### Primary (HIGH confidence)
- Probe built and run in this session (Go 1.27.0 linux/amd64): lock, listen/accept/dial, deadline, close-wakes-accept, stale refused; cross-built for all six targets with `go list -deps` and `go tool nm` checked against Step 9's `sym_re`; `go vet` for darwin and windows.
- Repo: `scripts/check-architecture.sh` Step 9 (lines 461-590) and baseline run (passes), `cmd/gruntled/watch.go`, `main.go`, `internal/infrastructure/statusfile/{path,write}.go`, `internal/infrastructure/watch/run.go`, `.github/workflows/ci.yml`, `cmd/gruntled/e2e_test.go` `TestHelpMatchesDocs`.
- Phase 10 VERIFICATION.md Security table (findings 2 and 3).

### Secondary (MEDIUM confidence)
- `.planning/research/{SUMMARY,ARCHITECTURE,PITFALLS,STACK}.md` (milestone research; two recommendations here supersede it: lock-first, poller-based accept).
- Go `os.NewFile` / `syscall` semantics (pollable when fd is non-blocking) - behaviour confirmed empirically on linux; darwin by compile only.

### Tertiary (LOW confidence)
- Windows exclusive-open semantics (`ERROR_SHARING_VIOLATION`, handle freed on crash) and darwin runtime behaviour: from documentation/training knowledge, not executed here.

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH - empirical probe on all six targets against the real proof regexes
- Architecture: MEDIUM-HIGH - lock-first, snapshot-of-bytes design is straightforward; Report immutability handled by storing rendered bytes
- Pitfalls: MEDIUM - darwin/windows runtime pitfalls unverified; security finding mapped from the Phase 10 audit

**Research date:** 2026-10-08
**Valid until:** 2026-11-07 (re-run the Step 9 probe on any Go or x/sys bump)
