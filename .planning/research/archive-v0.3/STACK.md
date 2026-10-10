# Technology Stack (v0.3 Watch & Blast additions)

**Project:** gruntled
**Researched:** 2026-10-08
**Scope:** only what v0.3 adds. Existing: Go 1.27, hcl/v2, go-cty, stdlib `flag`, rogpeppe/go-internal (testscript).

## Headline finding

The hard constraint "binary links no `net`, `os/exec`, `net/*`" (check-architecture.sh Step 9) is
**incompatible with the obvious library picks**. Verified by compiling a probe module with
`go list -deps` per GOOS (CGO_ENABLED=0, Go 1.27.0):

| Package | linux | darwin | windows |
|---------|-------|--------|---------|
| fsnotify v1.10.1 | clean | clean | links `net`, `net/netip` (via `golang.org/x/sys/windows`) |
| gofrs/flock v0.13.1 | clean | clean | links `net`, `net/netip` (same reason) |
| stdlib `net.Listen("unix")` | links `net` | links `net` | links `net` |
| rapid v1.3.0 | links `net/url`, `net/netip` (via text/template) | same | same |

Consequences (HIGH confidence, measured):
- Anything importing `golang.org/x/sys/windows` breaks the proof on both windows targets. **Do not import it.**
- A unix socket via `net` breaks the proof on all 6 targets.
- rapid is test-only; the proof runs `go list -deps ./cmd/gruntled` without `-test`, so it is fine.

## Recommended Stack

### Filesystem watching
| Technology | Version | Purpose | Why |
|------------|---------|---------|-----|
| `github.com/fsnotify/fsnotify` | v1.10.1 (2026-05-04) | inotify/kqueue backend on linux+darwin | The only mature option; already named in PROJECT.md. Clean on unix. Must be **build-tagged out of windows** (`//go:build !windows`) so the windows binary never pulls `x/sys/windows`. |
| Own stat-polling watcher (stdlib only) | n/a | Windows backend, `--poll` flag, automatic fallback | Needed anyway: inotify does not fire for Windows-side edits seen from WSL2 `/mnt/c` (the developer's own setup), and inotify can hit ENOSPC. ~100 lines: walk, compare (mtime, size, inode where available), emit dirty paths. |

fsnotify has **no public recursive watch** (README: "Recursive watching is not currently enabled through
fsnotify's public API"). Add one watch per directory yourself, and on `Create` of a directory add it and
rescan it (files created between mkdir and AddWatch are otherwise lost).

Decision: both adapters implement one port `Watcher { Events() <-chan []RepoPath }` emitting *dirty path sets*.
Do not forward event types into the application (see ARCHITECTURE.md).

### Property-based testing
| Library | Version | Purpose | Why |
|---------|---------|---------|-----|
| `pgregory.net/rapid` | v1.3.0 (2026-03-30) | DAEMON-02 incremental == full | Has stateful testing (`rapid.T.Repeat` with an action map) which is exactly "random sequence of edits, after each compare with full rescan", plus automatic shrinking. Test-only dependency. |

Rejected: `gopter` (last release v0.2.11, 2024-04, stateful API clunky), `testing/quick` (no shrinking, no
stateful model; a failing 40-edit sequence is unreadable). Go native fuzzing already exists in the repo
(`fuzz_test.go`) but is coverage-guided on a `[]byte`, not a model-based sequence; keep it for the parser,
use rapid for the index.

### IPC (DAEMON-04) and single instance (DAEMON-05)
| Technology | Purpose | Why |
|------------|---------|-----|
| Raw `syscall` AF_UNIX (`syscall.Socket/Bind/Listen/Accept/Connect`, wrapped by `os.NewFile`) in `internal/infrastructure/ipc`, build tag `unix` | Keep the no-`net` proof intact | Stdlib `net` is the only thing that makes this trivial and it is forbidden. ~150 lines: non-blocking accept loop with 100 ms sleep on EAGAIN (closing a blocked accept does not wake it reliably on darwin), line-delimited request/response. |
| `syscall.Flock(fd, LOCK_EX\|LOCK_NB)` on `.gruntled/…lock`-style file, build tag `unix` | Single instance, auto-released on crash | No dependency needed; kernel drops flock on process death so there are no stale locks. |
| Windows | **No socket in v0.3** (see decision below) | |

Windows facts (verified in GOROOT 1.27): `net` supports `unix` on windows (build tag `unix || … || windows`,
AF_UNIX needs Windows 10 build 17063+), and stdlib `syscall` does have `SockaddrUnix` on windows. But wrapping raw
winsock handles into a blocking `io.ReadWriter` without `x/sys/windows` or `net` means overlapped I/O by hand.
Not worth it.

### DECISION NEEDED (surface to roadmap)
- **Option A (recommended): strict proof kept.** unix: raw-syscall socket + flock. Windows: polling watcher, status
  file, single-instance lock via `syscall.CreateFile(..., sharemode=0, ...)` on a lock file (exclusive open is
  released by the OS on crash), `gruntled report` on Windows reads the status file / prints "daemon IPC not
  available on windows; see status file". Reword DAEMON-04: "over a unix socket on linux/darwin".
- Option B: allow `net` only inside `internal/infrastructure/ipc`, loosen Step 9 to a grep proving only the
  literal `"unix"` network is used. Gains Windows IPC, loses "cannot" -> "does not" (the project's stated reason
  for the proof is that the guarantee is *demonstrable*). Still needs flock replaced on Windows (flock lib links
  `net` on windows anyway).

### Supporting
| Library | Version | Purpose | When |
|---------|---------|---------|------|
| `golang.org/x/sys` (unix only) | v0.48.0 | only if raw `syscall` lacks something | Already an indirect dep (v0.46). fsnotify will bump it to >=v0.47. `x/sys/unix` is net-free; **never import `x/sys/windows`**. |

## What NOT to add
- `gofrs/flock`, `nightlyone/lockfile`: link `net` on windows; flock is 5 lines of syscall on unix anyway.
- `gRPC`, `net/rpc`, `net/http` over socket, `cobra`: link `net`.
- `go-winio` (named pipes): imports `x/sys/windows`.
- `rjeczalik/notify`, `radovskyb/watcher`: unmaintained / redundant.
- `fswatch`/`watchman`/`entr`: external binaries, forbidden.
- Any on-disk index cache (Out of Scope).
- `os/signal` is fine (no net). `time.Timer` debounce is stdlib.

## Installation

```bash
go get github.com/fsnotify/fsnotify@v1.10.1      # only imported under !windows
go get pgregory.net/rapid@v1.3.0                 # _test.go only
go mod tidy
```

New go.mod lines: 1 direct runtime dep (fsnotify), 1 direct test dep (rapid; pulls no runtime transitive deps).
`scripts/check-architecture.sh` must be extended so `release_targets` windows iterations prove that
fsnotify is absent (the existing deny-list already catches `net`).

## Sources
- Probe module build, `go list -deps` per GOOS, 2026-10-08 (HIGH, measured).
- Go proxy `@latest`: fsnotify v1.10.1, rapid v1.3.0, gopter v0.2.11, flock v0.13.1, x/sys v0.48.0 (HIGH).
- fsnotify README/CHANGELOG in module cache (recursive watching not public API) (HIGH).
- GOROOT/src/net/unixsock_posix.go build tag includes windows; syscall_windows.go SockaddrUnix (HIGH).
- https://golang.org/issue/26072 AF_UNIX on Windows 10+ (MEDIUM).
