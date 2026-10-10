# Domain Pitfalls (v0.3 Watch & Blast)

**Domain:** file-watching daemon with IPC for a static analyser
**Researched:** 2026-10-08
**Confidence:** MEDIUM-HIGH (OS watcher behaviour from long-standing documented behaviour and fsnotify docs; build/proof facts measured in this session)

## Critical Pitfalls

### 1. A "just add the library" breaks the no-net proof
**What:** fsnotify and flock on windows import `x/sys/windows`, which imports `net`; stdlib unix sockets import `net`. Step 9 fails on 2-6 of 6 targets.
**Prevention:** build-tag fsnotify to `!windows`; raw `syscall` sockets and flock; never import `x/sys/windows`; add a self-test case to check-architecture.sh (a windows-only file importing fsnotify must be caught). Run `go list -deps` for all six targets in CI after every dependency bump (a bump of x/sys can change this).
**Detection:** the Step 9 deny-list output names `net`.
**Phase:** the very first phase touching dependencies.

### 2. Trusting watcher events for correctness
**What:** inotify queue overflow (`IN_Q_OVERFLOW`, fsnotify surfaces it as an error), Windows `ReadDirectoryChangesW` buffer overflow (events silently dropped on bursts such as `git checkout` or `terragrunt stack generate`), macOS kqueue needs an fd per file/dir, coalescing differs per OS.
**Prevention:** events are hints. Validate cache entries with stat on use; on any watcher error or overflow mark everything dirty and do a full revalidate; also run a low-frequency safety revalidate (e.g. 30-60 s) while idle.
**Detection:** property test with a "drop random events" watcher fake; the result must still equal a full rescan after the next revalidate.

### 3. Incremental != full (determinism drift)
**What:** stale parse for an include file after rename; unit deleted but still in graph; surface cache not invalidated when a `.tf` is added; map iteration order differing between runs; diagnostics ordered by discovery order.
**Prevention:** memoise parsing only, rebuild assembly every time (ARCHITECTURE.md); the rapid model test generates create/edit/delete/rename of units, includes and `.tf`, runs the same sequence on a fresh loader, and compares the **serialised** `check` output byte for byte, not graph pointers. Include dependency-order pathologies (include created after the unit that references it; dir renamed).
**Detection:** shrunk counterexamples from rapid; also run the equivalence assertion under `-race`.

### 4. Editor save patterns
**What:** vim writes `4913` probe file, renames original to `file~`, writes new file (Rename+Create+Chmod, brief absence of the file); VSCode may write in place or via temp+rename depending on settings; JetBrains "safe write" writes `___jb_tmp___` then renames; `:w` on a file watched individually loses the watch after rename (inode changes).
**Prevention:** watch **directories**, never individual files; any event -> dirty(path); treat "file missing" at reindex time as possibly transient: re-stat after the debounce window (a missing file inside the window that reappears must not emit a flash of GRT100/GRT002 findings). Debounce trailing 150 ms. Ignore swap/backup patterns before they reach the debounce.
**Detection:** scripted "vim-style save" test using real fs ops (rename, create, remove) against the poll adapter and fsnotify adapter via a shared contract test.

### 5. Reads racing writes (torn files)
**What:** the daemon reads while the editor is mid-write -> truncated HCL -> spurious `GRT100` syntax error and status flips to red for 50 ms. A "zero false positives" violation by timing.
**Prevention:** debounce; after reading, re-stat and, if size/mtime changed during the read, re-read; consider suppressing GRT100 for a path for one extra debounce cycle if the parse fails on a file whose mtime is younger than ~50 ms. Never publish a snapshot built from a read that was invalidated mid-flight.

### 6. Stale socket and stale lock
**What:** crash leaves `sock` on disk; naive `bind` fails EADDRINUSE; naive "socket exists -> daemon running" refuses to start forever. Pid files go stale and pid reuse produces false "running".
**Prevention:** liveness = successful connect+ping, never file existence. flock is auto-released by the kernel on crash; take the lock, then unlink the stale socket, then bind. No pid-file-as-lock. On Windows use the exclusive-open lock file (also OS-released). Sun path limit ~104 bytes (macOS) / 108 (linux): socket path must be short (`$XDG_RUNTIME_DIR/gruntled/<12-hex>/sock`, fall back to a short temp path), otherwise bind fails with a confusing error on deep temp dirs (macOS `$TMPDIR` is already ~50 chars).
**Also:** flock is advisory and unreliable on some network filesystems; keep the runtime dir local (never under the repo, which may be NFS/`/mnt/c`).

### 7. Same-hash different-repo and symlink aliasing
**What:** two paths to the same repo (symlink, `..`, case-insensitive FS) produce two runtime dirs, so two daemons run on one repo; or hashing a relative path.
**Prevention:** hash `filepath.EvalSymlinks(filepath.Abs(root))`; on macOS/Windows lowercase before hashing is debatable, document the limitation.

## Moderate Pitfalls

### 8. Watch limits
inotify `max_user_watches` (8192 on older kernels, scaled by RAM on newer) and `max_user_instances` (128); ENOSPC on `Add`. Exclude `.git`, `.terraform`, `.terragrunt-cache`, `node_modules` from watching entirely. On ENOSPC fall back to polling and say so on stderr and in the status line. macOS: raise via `ulimit -n` message; kqueue needs an fd per watched directory *and* file it enumerates.

### 9. WSL2 and network filesystems
inotify does not see Windows-side writes to `/mnt/c`, nor changes on NFS/SMB. This is the developer's own environment. Provide `--poll` and auto-detect (`/mnt/` prefix on WSL is a heuristic, not proof); document.

### 10. fsnotify new-directory race
A directory created with files already inside it (`git checkout`, `cp -r`, `terragrunt stack generate`) delivers Create for the dir only; watching it afterwards misses the contents. Rescan the dir immediately after `Add`.

### 11. Test flakiness of real watchers in CI
Never assert on timing. Rules: (a) correctness tests use the pure `Session.Apply(batch)` with in-memory FS, no real watcher; (b) watcher contract tests wait on a condition with a generous deadline (5-10 s) via polling a channel, never `time.Sleep(n)` then assert; (c) a fake clock for debounce in unit tests; (d) macOS CI runners have coalescing delays and Windows runners lose events on bursts: run adapter tests per OS in the CI matrix but allow the poll adapter to be the only one asserted strictly; (e) run with `-race` and `-count=20` locally before merging.

### 12. Socket server goroutine and shutdown hangs
Blocked `accept` cannot be woken by closing the fd on darwin. Use non-blocking accept with short sleep loop and a `context`, so `SIGINT` terminates promptly. Per-connection deadlines (read 2 s) so a client that connects and says nothing cannot wedge the server. Socket mode 0600 and directory 0700 (local privilege boundary: any local user could otherwise read the repo's structure).

### 13. Status file semantics
Non-atomic writes make a prompt read half a line; a status that says "ok" after the daemon died misleads. Use tmp+rename in the same directory; include pid and unix timestamp; delete (or write `stopped`) on clean exit; document that readers should consider it stale after N seconds without a heartbeat (write a heartbeat every 5-10 s only if the content changes, otherwise the file's mtime is the heartbeat — choose one and test). Prompt-friendly: single line, no colour, no newline surprises, bounded length.

### 14. Blast semantics confusion
No before-state, or before-state = daemon start when the user has been editing for hours: "Impacted" shows an accumulated delta. Show the baseline's age/generation in output and provide `rebase`. Pre-existing breakage must not be reported as Broken for this change (diagnostic set difference). Surface = names only: an added required variable or a type change is invisible; say so rather than imply completeness.

### 15. Memory growth
A persistent parse cache that retains HCL ASTs and bodies for the whole repo; evict on delete, cap or store only the extracted structural facts. Measure on the 65-unit corpus and the synthetic repo generator at 5k units.

## Minor Pitfalls
- `os.Rename` onto an existing file is not atomic on every Windows filesystem state (file open by a reader fails): retry a few times with backoff on the status writer.
- Signal handling: `os/signal` on Windows only supports Ctrl-C/Ctrl-Break; do not rely on SIGTERM there.
- `report` exit codes must be distinct for "no daemon", "protocol mismatch", "findings present".
- Path output must remain repo-relative through the socket protocol; the daemon's absolute root must not leak (reproducibility rule).
- rapid pulls `net/url` into test binaries; harmless, but do not let a `go list -test` variant of the proof get added later.
- go.mod `x/sys` will be bumped (v0.46 -> >=v0.47) by fsnotify; re-run the full test suite and the six-target proof.

## Phase-Specific Warnings

| Phase topic | Likely pitfall | Mitigation |
|-------------|---------------|------------|
| Persistent parse cache | determinism drift (#3), memory (#15) | rapid model test first |
| Poll/fsnotify adapters | #2, #4, #8, #9, #10 | contract test shared by both; stat-based reconcile |
| Debounce | torn reads (#5) | trailing window + re-stat |
| Status file | #13 | atomic write, golden format, pid/time |
| Socket + lock | #6, #7, #12, #1 | connect-before-lock, flock, short path, raw syscall |
| Blast | #14 | baseline age, set difference, doc names-only limits |
| Release | #1 on windows, x/sys bump | six-target proof in CI, self-test case |

## Sources
- Measured probe builds (HIGH); fsnotify README/CHANGELOG (HIGH)
- Editor/OS watcher behaviours: long-standing documented behaviour, not re-verified online this session (MEDIUM)
