# Pitfalls Research

**Domain:** Adoption, distribution and UX features (install script, Homebrew tap, GHCR image, `--version` from build info, completions and man pages, colour, status-file liveness, Windows named pipes, SARIF for `blast`, project hygiene, executable docs) added to a deterministic, no-network, no-exec, read-only Go CLI with a daemon (gruntled v0.5 "Adoption & Completeness").
**Researched:** 2026-10-16
**Confidence:** MEDIUM-HIGH. Everything about gruntled's own code and contracts was read from the repository (HIGH). Platform facts (Go build-info stamping, GitHub immutable releases, SARIF/code-scanning rules, named-pipe default ACL, `os.NewFile` overlapped handles) come from official docs or search results (MEDIUM). Items marked "verify in a rehearsal" are reasoned but not proven here.

## How to read the phase tags

No v0.5 roadmap exists yet. Phases continue from 17 and the tags below are a **suggested** split. The roadmapper may renumber, but the ordering constraints in "Pitfall-to-Phase Mapping" hold.

| Tag | Suggested phase | Scope |
|-----|-----------------|-------|
| P18 | Version and release-pipeline foundations | `--version` from build info, single version source for docs and recipes, release asset layout, release dry-run harness, immutable releases and attestations |
| P19 | Install channels | `install.sh`, Homebrew tap, GHCR image |
| P20 | CLI surface | command/flag registry, completions, man page, colour and `--quiet`, `blast --format sarif` |
| P21 | Daemon completeness | status-file liveness, `watch --base`, Windows named pipe, editor/shell recipes |
| P22 | Project hygiene | CONTRIBUTING, SECURITY, CHANGELOG, templates, CODEOWNERS, Dependabot |
| P23 | Proof and release readiness | corpus gate on v0.5, docs verified against the binary, cross-phase audit, release checklist |

## Critical Pitfalls

### Pitfall 1: `--version` reads build info and silently breaks the `dev (none)` contract and the determinism of dev builds

**What goes wrong:**
`debug.ReadBuildInfo()` does not return one clean answer. Since Go 1.24, a plain `go build` inside a git checkout stamps `Main.Version` with a VCS-derived pseudo-version (`v0.4.1-0.2026…-abcdef123456`) and appends `+dirty` for an uncommitted tree. `go test` binaries and non-VCS builds report `(devel)`. `go install pkg@vX.Y.Z` reports the real tag but has no `vcs.*` settings at all (no commit). `go install pkg@latest` before a tag exists resolves to a pseudo-version. This is the trap that made v0.2 drop the `ReadBuildInfo` fallback ("Local `go build` stamps a VCS pseudo-version that broke the `dev (none)` contract", PROJECT.md Key Decisions).
Two concrete gruntled consequences:
1. **v0.4.0 is not tagged yet** (Giulio's decision). Today `go install …@latest` resolves to a master pseudo-version, so a naive implementation prints `gruntled v0.3.1-0.2026… ()` to exactly the users this feature is for.
2. The version string is the SARIF `tool.driver.version`. A `+dirty` or commit-bearing dev version makes SARIF bytes differ between two runs on the same input depending on the working tree state. That violates "same input, same bytes" for dev builds and pollutes any golden comparison that runs a locally built binary.

**Why it happens:**
Developers treat `Main.Version` as "the tag". It is "whatever the Go toolchain decided", and the rules changed in 1.18 (vcs.* settings) and 1.24 (version stamping).

**How to avoid:**
- Precedence: (1) ldflags `main.version`/`main.commit` when set (release builds, unchanged); (2) build info only if `Main.Version` is a canonical release version, i.e. matches `^v\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$` and is NOT a pseudo-version (reject any `-\d{14}-[0-9a-f]{12}` suffix or `.0.\d{14}-` form), contains no `+dirty`/`+incompatible`, and is not `(devel)`; (3) otherwise the existing `dev` / `none`.
- Commit for the build-info path: `vcs.revision` truncated to 12 chars only when `vcs.modified` is not `true`; absent (the `go install @tag` case) stays `none`. Never print `+dirty` or time.
- Inject the build-info reader as a function (`func() (*debug.BuildInfo, bool)`) so tests do not depend on how the test binary was built. Add table rows (tag, pseudo-version, `(devel)`, dirty, ldflags-over-buildinfo, missing info) and keep the three existing `dev (none)` rows in `cmd/gruntled/main_test.go` (lines 160-162) unchanged: they prove the fallback path.
- Update `docs/cli.md` and `docs/ci.md` in the same plan: both state that `go install` prints `dev (none)`. The CI doc test (`TestCIDoc`) will force this.
- Keep `release.yml` smoke (`gruntled $GITHUB_REF_NAME (*`) and `build-release.sh` smoke as the guard that ldflags still win.
- Add a post-publish job that runs `go install github.com/GiulioSavini/gruntled/cmd/gruntled@<tag>` (network is fine in a workflow) and asserts `--version` starts with `gruntled <tag>`. `recipe-check` cannot cover this because it installs from the checkout.

**Warning signs:**
`--version` output contains `+dirty`, `-0.2026`, `(devel)` or an empty commit; two builds of the same commit give different SARIF; the formula test (`assert_match version`) fails.

**Phase to address:** P18.

---

### Pitfall 2: `curl | sh` installer that verifies a checksum from the same trust root, or runs a truncated script

**What goes wrong:**
- **Truncated execution.** `curl … | sh` executes whatever has arrived. A connection drop in the middle of a function body or a line (`rm -rf "$DIR"/…` cut at `rm -rf "$DIR"`) runs a partial command.
- **False assurance from `checksums.txt`.** The checksum is downloaded from the same GitHub release as the archive. It detects corruption, not a compromised release. Docs that say "verified" overstate it.
- **`sha256sum -c --ignore-missing`** (used in `docs/ci.md`) does not exist on macOS (`shasum -a 256`), busybox/Alpine (`sha256sum` without `--ignore-missing`) or minimal containers; a script copied from the docs fails on exactly the platforms people try first. `--ignore-missing` also passes if the checksum line for the archive is absent (exit 0 with "no file was verified" on some versions).
- **Wrong asset.** `uname -m` under Rosetta reports `x86_64` on an Apple Silicon Mac; `aarch64` (Linux) vs `arm64` (macOS) naming; Git Bash/MSYS reports `MINGW64_NT-…` and would pick the wrong archive type; the glibc/musl question does not exist for static binaries (CGO_ENABLED=0), so do not add it.
- **Redirect and rate limit.** Resolving "latest" through the REST API is limited to 60 requests/hour unauthenticated, which a shared CI NAT exhausts. Archive names embed the tag (`gruntled_v0.5.0_linux_amd64.tar.gz`), so `releases/latest/download/<asset>` cannot be used directly.
- **`curl -s` without `-f`** saves an HTML 404 page as the "archive"; the failure appears as a confusing checksum mismatch.
- **Overwriting a running binary** with `cp` gives "text file busy" while `gruntled watch` runs; `rm`+`cp` leaves a window with no binary.
- **Silent `sudo`**, editing shell rc files, and `~/.local/bin` not being on PATH in CI.

**Why it happens:**
Installers are copied from other projects and tested only on the author's machine.

**How to avoid:**
- Wrap everything in `main() { … }` and call `main "$@"` as the last line, so a truncated download defines a function and runs nothing. POSIX `sh` only (`dash`, busybox `ash`), no `pipefail`, no bashisms; `set -eu`.
- **Make the script a release asset with the expected SHA-256 of that release's archives embedded**, generated by the release workflow after `checksums.txt` exists. Stable URL `https://github.com/GiulioSavini/gruntled/releases/latest/download/install.sh`, pinned URL `…/releases/download/vX.Y.Z/install.sh`. This removes the "latest" API call (version is baked in, `GRUNTLED_VERSION` overrides) and the second download. Never document `raw.githubusercontent.com/…/main/install.sh`: it is mutable.
- State the honest guarantee: the embedded hash defends against asset swap after publication and CDN corruption, not against a compromised release workflow. Strengthen at the source: enable **GitHub immutable releases** (GA since 2025-10; assets, tags and an attestation are locked after publish) and publish `actions/attest-build-provenance` attestations so users can optionally run `gh attestation verify`. Make attestation checking optional in the script (only if `gh` exists), never required.
- Checksum tool fallback chain: `sha256sum` → `shasum -a 256` → `openssl dgst -sha256`; compare by exact string equality against the embedded value; fail if no tool exists. Never skip verification on a missing tool.
- Detect OS/arch from `uname -s`/`uname -m` with an explicit allow-list (`Linux|Darwin`, `x86_64|amd64`, `aarch64|arm64`); on Darwin ask `sysctl -n hw.optional.arm64` (read by the script, not by gruntled) to see through Rosetta; on anything else exit with "download the zip from the releases page" rather than guessing. Windows is a separate story (zip; consider a short PowerShell snippet in docs, not a `sh` script).
- `curl -fsSL --proto '=https' --tlsv1.2 --retry 3` (fall back to `wget -q -O` for Alpine/busybox, which lacks `--proto`).
- `mktemp -d` plus `trap 'rm -rf "$tmp"' EXIT INT TERM`; extract **only** the member named `gruntled` (never untar the whole archive into the destination); install with a write-then-`mv` into the destination directory (atomic rename; a running daemon keeps its old inode).
- Default destination `$HOME/.local/bin` (no sudo). If the chosen directory is not writable, print the exact command and exit non-zero; never invoke `sudo` implicitly. Never edit rc files: print a PATH hint. When `GITHUB_PATH` is set, append the destination to it (that file is outside the repository and the right mechanism in Actions).
- Idempotent: if `$dest/gruntled --version` already reports the requested tag, print "already installed" and exit 0. After install run `--version` and compare to the requested tag; this also guards the ldflags contract (Pitfall 1).

**How to test (concrete):**
- CI job running `shellcheck -s sh install.sh`, then the script under `dash`, `bash --posix`, and an `alpine` container (busybox) against a **local** file server or `file://` base populated by the `build-release.sh` dry-run output (no real network; `GRUNTLED_BASE_URL` override is for tests only and must refuse non-https unless `file://` or `127.0.0.1`).
- Negative tests that must exit non-zero and install nothing: tampered archive, truncated archive, 404, unsupported OS, unwritable destination, hash tool missing.
- Truncation test: cut the script at every line boundary (and a few mid-line offsets) and assert nothing outside `$tmp` is touched.
- Idempotency test: run twice, second run reports "already installed" and the binary inode is unchanged; run a third with a different tag and assert the old inode keeps working.

**Warning signs:**
The docs tell users to `curl … | sudo sh`; the script contains `sha256sum -c --ignore-missing`; a test only runs on ubuntu-latest with network.

**Phase to address:** P19 (after P18 fixes the asset layout and single version source).

---

### Pitfall 3: The status-file liveness check races the instance lock, and a second "truth" (pid file) is introduced

**What goes wrong:**
Today the flock (unix) or exclusive share-mode open (windows) is "the only source of truth for 'a daemon runs'" (`ipc.go`). Two ways to break that:
1. **Probe contends with the daemon.** `ipc.Probe` on a free lock acquires the exclusive lock and releases it immediately. A liveness check run from a shell prompt or status bar every second means a daemon starting at that instant gets `ErrHeld`, concludes "already watching (another daemon runs for path)" and **exits 0**. Nothing runs and nobody is told. The risk scales with how often liveness is polled, which is exactly what v0.5 encourages. The same applies to the windows `openExclusive(…, share 0)` probe.
2. **A pid file for liveness.** `kill(pid,0)` succeeds for a recycled pid, returns EPERM for another user's process, and says nothing about the daemon's lock; Windows `OpenProcess`+`GetExitCodeProcess` returns `STILL_ACTIVE` (259) for a process that exited with code 259 and also needs a creation-time comparison against reuse. Cross-platform pid probing without `os/exec` is also easy to get subtly wrong.

Other traps in the same feature:
- **The status line has no date** (`@ HH:MM:SS`, local time). Age cannot be derived from it, and across midnight or a DST change it is ambiguous. Use file **mtime** (epoch) for age.
- **A reader that "repairs" the stale line** (rewrites or deletes it) races a freshly started daemon's atomic rename and can clobber a live status. Readers must be strictly read-only.
- **Changing the one-line grammar** breaks consumers documented in v0.3 (`cat "$(gruntled watch --print-status-path .)"` in prompts and tmux). Appending text or a new prefix breaks them silently.
- **Heartbeat writes**: rewriting the status every few seconds wears laptop SSDs and, on macOS where the runtime dir falls back to `~/Library/Caches`, produces constant fsevents for anything watching the file.
- **Cross-host readers (NFS):** if `$XDG_RUNTIME_DIR` is unset the runtime dir falls back to the user cache dir, which is on NFS in many enterprises. flock on NFS is emulated, a unix socket on NFS is not connectable across hosts, `mtime` comes from the server clock while "now" is the client clock (skew makes a live daemon look stale or a dead one look live), and two hosts sharing the home directory and the repository path share one `hash12`, so a daemon on host B appears as "already watching" to host A while its socket is unreachable.

**How to avoid:**
- Keep the lock as the single source of truth. Add a read-only `gruntled status [path]` (or equivalent) that: reads the status file, then determines liveness by lock probe, and prints the line unchanged when alive or a clearly marked stale form (for example `gruntled: not running (last: <line>)`) when not. Exit codes follow the existing 0/2/3 discipline and are documented.
- **Fix the probe race at the daemon, not the reader:** when `TryLock` returns `ErrHeld`, retry for a short bounded period (for example 5 x 50 ms) before concluding another daemon runs. Add a test that runs Probe in a tight loop against a starting daemon (race test, as in `lock_test.go`). Alternatively probe via a socket `ping` first (no lock contention) and fall back to the lock only when no socket exists; never hold the lock longer than one syscall.
- Holder information (pid, start time, hostname, protocol version) may be written into the lock file **after** acquiring it, as diagnostics only, never as a liveness signal. If shown, label it "reported by daemon".
- For dumb readers (a `cat` in a prompt), offer an optional heartbeat that only touches **mtime** via `os.Chtimes` (no content change, no rename) at a coarse period (>= 10 s, stale after 3 periods), and document that it is a hint. The recipe documents `gruntled status` as the authoritative check.
- Document the NFS/shared-home case as a limitation, and consider refusing (exit 2 with the existing message style) to use a runtime directory on a network filesystem if it can be detected without exec; otherwise document `XDG_RUNTIME_DIR`.
- Preserve the 0700 directory / 0600 file checks; the new reader must apply the same owner/mode checks as `report` before trusting anything it reads.

**Warning signs:**
A daemon test passes only without concurrent `status` calls; the design doc mentions "pid"; `gruntled watch` occasionally prints "already watching" with no daemon running; the status line grammar changes.

**Phase to address:** P21.

---

### Pitfall 4: Windows named pipe built on `net`-linking libraries, a permissive default ACL, or blocking IO

**What goes wrong:**
- **Wrong library.** The usual answer (`go-winio`) returns `net.Listener`/`net.Conn` and links `net`, which breaks the `binary-no-net-no-exec` proof on windows/amd64 and windows/arm64. The public stdlib `syscall` package has no `CreateNamedPipe` (it exists only in `internal/syscall/windows`); you need `golang.org/x/sys/windows` or a `syscall.NewLazyDLL("kernel32.dll")` binding. `x/sys` is currently only an indirect dependency and `GOOS=windows go list -deps ./cmd/gruntled` shows none of it linked today, so adding it changes the windows dependency set and the source scan for process-spawn calls must be re-run on both windows targets before anything else is built on it.
- **Default security descriptor.** With a NULL descriptor, the pipe's DACL grants full control to LocalSystem, Administrators and the creator owner **and read access to Everyone and the anonymous account** (Microsoft docs). Combined with an unverified server identity this reproduces the Windows equivalent of a world-accessible socket, on top of the existing documented limitation that the windows runtime directory relies on `%LocalAppData%` ACLs.
- **Name squatting and collisions.** The pipe namespace (`\\.\pipe\`) is machine-global, across sessions. A name derived only from `hash12` collides when two users (RDP, shared build agent) analyse the same path, and lets any local process create the name first. Without `FILE_FLAG_FIRST_PIPE_INSTANCE` a second process can create another instance, and the descriptor of the first instance applies to later ones (CyberArk). A squatter that serves `report` output also controls bytes the client prints to a terminal.
- **Remote clients.** Without `PIPE_REJECT_REMOTE_CLIENTS`, other machines can reach the pipe over SMB.
- **Message vs byte mode.** The protocol is "one request line, one response line" up to 64 MiB. In message mode a large reply is returned in `ERROR_MORE_DATA` chunks and a short buffer truncates silently if the client treats one `ReadFile` as one message. Use byte mode and read to `\n` under the cap.
- **Blocking IO.** A synchronous `WriteFile` of a multi-MiB reply blocks until the client reads; a client that stops reading wedges the server goroutine forever, and a blocked `ConnectNamedPipe`/`ReadFile` cannot be woken by `Close`. There is no `SetDeadline` on a raw handle. Per the Go docs, `os.NewFile` on a handle opened with `FILE_FLAG_OVERLAPPED` attaches it to the runtime's completion port so deadlines and `Close` work; a handle without the flag is synchronous (verify in a Windows CI rehearsal; Go has recent issue traffic around `NewFile` blocking semantics, go.dev/issue/75949 and 76391).
- **Antivirus and sharing violations.** Rename-over of the report file or status file fails with a sharing violation when a reader or scanner holds it open without `FILE_SHARE_DELETE`; an unsigned downloaded `.exe` carries Mark-of-the-Web (SmartScreen) and zips unpacked from the web may need unblocking.

**How to avoid:**
- Implement the pipe with `CreateNamedPipe` + overlapped IO + `os.NewFile` (or own `OVERLAPPED`/event/`CancelIoEx`), no `net`. Add `x/sys/windows` only if the proof passes with it linked on both windows targets; otherwise use `syscall.NewLazyDLL`.
- Create with: `PIPE_TYPE_BYTE|PIPE_READMODE_BYTE|PIPE_WAIT|PIPE_REJECT_REMOTE_CLIENTS`, `FILE_FLAG_FIRST_PIPE_INSTANCE|FILE_FLAG_OVERLAPPED`, an **explicit** security descriptor granting only the current user's SID (for example SDDL `D:P(A;;GA;;;<user-SID>)`), `nMaxInstances` small and fixed.
- Name the pipe by capability: `gruntled-<hash12>-<random>` where the random part is written to a file in the runtime directory (per-user ACL), read by the client. Attackers who cannot read that file cannot predict the name; a crashed daemon leaves a name that fails with file-not-found (= "no daemon"; liveness stays with the lock, Pitfall 3). If a SID/username is hashed into the name instead, vet the stdlib import (`os/user` etc.) against the no-exec proof first.
- Reuse the **same framing/cap/refusal code** as the unix transport (one reader that stops at `\n`, enforces `maxResponse = 64 MiB`, returns the same refusal codes). Do not re-implement the 64 MiB bound for the pipe. Put the transport behind one interface and run one conformance table (ping, report, blast, rebase, oversize, malformed JSON, wrong protocol version, client disconnect mid-reply, stalled client with deadline) on unix sockets and on pipes.
- Retry `MoveFileEx`-style renames a few times with backoff on sharing violations.
- Keep the windows report-file path as fallback only if the pipe is unavailable, and keep `report`/`rebase` windows behaviour documented until the pipe lands; remove the "no live query on windows" limitation text in the same plan that makes it false.

**Warning signs:**
`go list -deps` for windows lists `net`; the pipe test passes only when run alone; `icacls`-style inspection of the pipe shows Everyone; a stalled client leaves the daemon's goroutine count growing.

**Phase to address:** P21 (the named-pipe plan needs a research flag for the Windows specifics and a Windows CI job, which already exists for tests).

---

### Pitfall 5: `blast --format sarif` emits results GitHub treats as alerts, shifts the shared rule table, and double-reports with `check`

**What goes wrong:**
- **Impacted units become alerts.** GitHub code scanning needs a location to display a result and shows `note`-level results as alerts. Impacted is documented as a lower bound / may-be-affected set with no line. Emitting Impacted as results yields alerts on lines that are not findings, which is the kind of noise that ends adoption (zero-noise is the product).
- **Rule table drift.** `tool.driver.rules` is a static list in code order with `ruleIndex`. Adding GRT004 to the shared list changes the bytes of `check`/`report` SARIF for every user (a new rule in `rules`) even though `check` never emits GRT004. Either that is a deliberate documented change or the list must be per-command.
- **Duplicate alerts.** `check` (all findings) and `blast` (new findings only) both use tool name `gruntled`. Same category: the second upload for a commit **replaces** the first. Different categories: one finding shows twice. On a PR that removes an output, `check` says GRT001 and `blast` says GRT004 at the same reference site, so two alerts with different rule ids on one line.
- **Multiple runs in one file** are no longer combined by code scanning (since July 2025); a combined check+blast SARIF would fail. Exactly one run per file, as today.
- **Determinism leak.** The text format prints `baseline: <as given to --base>` (for example `../base`). Putting that into SARIF (invocation properties, a message, `originalUriBaseIds`) makes output differ per checkout, breaking the byte-identical guarantee. The existing SARIF deliberately omits `originalUriBaseIds`.
- **Rule id stability.** GRT004 once published must never be repurposed or renamed; consumers key suppressions on `ruleId`.

**How to avoid:**
- Emit **Broken findings only** as results (code, level, location, message identical to the text/JSON data, GRT004 where the blast core says GRT004). Represent Impacted as `note`-level entries in `invocations[0].toolExecutionNotifications` (the established pattern for unknown units/modules) or omit them; never as results. Add a test asserting a document with Impacted units has the same `results` as one without.
- Decide the rule table explicitly: recommended is the full five-rule list for every SARIF producer (stable, simple, one-line doc change, `GRT004` helpUri to the existing heading), with the one-time byte change recorded in CHANGELOG and handled in the compat/corpus gates. Add a golden test that `rules` is identical across `check`, `report` and `blast`.
- Never put `--base` into the document. Add a test that runs the same comparison from two different base paths and diffs the bytes.
- Document the CI recipe: PR job uploads `blast` SARIF under a dedicated `category` (for example `gruntled-blast`) **instead of** the `check` upload for that PR job, or uploads both under different categories and accepts the documented duplicate; recommend the first. Keep `docs/ci.md` examples tested by `recipe-check` (extend it with a blast run against a fixture `--base`).
- Validate with the same SARIF schema CI step used for `check`, plus the existing upload-sarif proof.
- Update the "GRT004 has no SARIF rule" and "`sarif` is not supported for `blast`" statements and the usage text in `docs/cli.md`; both are asserted by doc tests and will fail the build until fixed (good).

**Warning signs:**
`check` SARIF golden changes unexpectedly; SARIF for `blast` contains `Impacted` text in `results`; GitHub shows two alerts at one line after an output rename.

**Phase to address:** P20 (rule table and Impacted representation need a decision before coding); P23 verifies against the real upload.

---

### Pitfall 6: Colour and TTY detection leak into the bytes that gruntled promises are identical

**What goes wrong:**
gruntled guarantees identical bytes, and `report` text is documented as "the same bytes as `check`", rendered **once per index inside the daemon** and copied by the client. Any of these breaks that:
- Colour applied at render time inside the shared renderer: the daemon (no TTY) or a golden test then sees (or lacks) ANSI codes depending on environment, and `report` to a pipe prints escape sequences.
- Detection reading `os.Stdout` directly inside library code: tests run in a terminal pass or fail depending on who runs them (the v0.4 `-short` flaky-test lesson: a guard only as good as its real sample).
- `Mode()&os.ModeCharDevice` as "is a terminal": `/dev/null` and `NUL` are character devices, so output redirected to them is "a TTY".
- One decision for stdout and stderr: `gruntled check > out.txt` leaves stderr on the terminal and the summary line should still be coloured/uncoloured by **its own** stream.
- Windows: legacy conhost prints raw `←[31m` unless `ENABLE_VIRTUAL_TERMINAL_PROCESSING` is enabled (and restored); `GetConsoleMode` failing means "not a console" (pipe/file) and must disable colour.
- Precedence mistakes: `NO_COLOR` is honoured only when set **and non-empty**; `--no-color` (and any `--color=never`) must beat `CLICOLOR_FORCE`/`FORCE_COLOR`; `TERM=dumb` disables; `CI` should not disable colour by itself (Actions and GitLab render ANSI; users opt in with `CLICOLOR_FORCE=1`).
- JSON, SARIF and `graph --json` must never be coloured, even with force flags.
- Colouring after escaping is mandatory: gruntled escapes ESC (`\x1b`) in repository-derived text so a hostile directory name cannot drive the terminal. If colour is added before or inside the escape step, the escape layer will either mangle colour codes or, worse, be bypassed.

**How to avoid:**
- Make colour a **pure edge decoration** over finished canonical bytes: `Colorize(canonicalText []byte, mode)`, applied in `cmd/gruntled` when writing to the process stream, never in `checking`/renderer packages, never in the daemon. `report` and `check` then stay identical under the existing renderer.
- Decision as a pure function `ColorEnabled(flag, env, isTTY(stream)) bool`, injected `isTTY` and `getenv`, table-tested over the whole precedence matrix: `--no-color`/`--color=never` > `NO_COLOR` (non-empty) > `CLICOLOR_FORCE`/`FORCE_COLOR` (non-empty, not `0`) > `TERM=dumb` > per-stream TTY. Prefer `--color auto|always|never` with `--no-color` as an alias.
- TTY detection by ioctl (`TCGETS` on linux, `TIOCGETA` on darwin) behind build tags, as `lock_unix.go` is, and `GetConsoleMode` on windows; or `golang.org/x/term` if the architecture proof passes with it. Add a test that `/dev/null` is not a terminal.
- **Property test (exact, thanks to the ESC escaping):** for generated diagnostics (including hostile names), `stripANSI(colorize(x)) == x` byte for byte, and `colorize` output never contains an ESC that was not emitted by the colouriser. Because gruntled's own output cannot contain a raw ESC, the strip regexp is unambiguous.
- Golden tests keep running with colour off (injected non-TTY); add one golden set for forced colour to catch accidental changes.
- `--quiet` removes only the stderr summary/hints; it never changes stdout, exit codes, JSON `summary`, or which stream carries what. Document that explicitly and test `--quiet` exit-code parity across a findings/no-findings/usage-error matrix.
- Windows: enable VT only on success of `SetConsoleMode`; on failure, disable colour (do not emit raw escapes). Restore the console mode on exit.

**Warning signs:**
`go test` result differs between a terminal and CI; `report | cat -v` shows `^[[`; the daemon's rendered text contains `\x1b`.

**Phase to address:** P20.

---

### Pitfall 7: Completions and man page drift from the real flag set

**What goes wrong:**
Without cobra there is no generator. Flags are registered with stdlib `flag`, usage text is hand-written, `docs/cli.md` embeds the usage text, and completions would be a **fourth** copy. Drift is certain:
- `--format` has different value sets (`check`/`report`: `text|json|sarif`; `blast`: `text|json`; `report --blast` rejects sarif; after P20 `blast` gains sarif).
- Flags may follow the positional path, so completion must not assume order; `--` ends flags; `path` is a directory only.
- zsh: sourcing the completion before `compinit` yields `command not found: compdef`; a stale `~/.zcompdump` hides a newly installed `_gruntled`; the `#compdef gruntled` header line must be first in a file named `_gruntled` that sits in an `fpath` directory.
- bash: macOS ships bash 3.2 without `bash-completion` v2, so `_init_completion` and `_get_comp_words_by_ref` may not exist; `mapfile` and associative arrays fail.
- PowerShell: PS 5.1 reads BOM-less scripts as ANSI, execution policy and Mark-of-the-Web block dot-sourced downloaded scripts; `Register-ArgumentCompleter -Native -CommandName gruntled` must return `CompletionResult` objects.
- fish: completions live in `~/.config/fish/completions/gruntled.fish` (or `$XDG_DATA_HOME/fish/vendor_completions.d`).
- Homebrew: use the formula DSL (`bash_completion.install`, `zsh_completion.install`, `fish_completion.install`, `man1.install`); zsh still needs Homebrew's `site-functions` on `FPATH` before `compinit`.
- Man page: a `.TH` date from `time.Now()` makes the output non-reproducible; unescaped `-` renders as a Unicode hyphen (`\-` needed for copyable flags); a line starting with `.` or `'` is parsed as a roff request; flag values from repository data must never appear (they do not).
- Adding `completion`/`man` subcommands changes `gruntled -h`, the `docs/cli.md` embedded usage, exit-code tables and the command list; several tests pin these (good, but plan for the churn).

**How to avoid:**
- **Introduce a single command/flag registry first** (command name, summary, flags with kind, enum values, default, description, positional spec), and derive from it: the `flag.FlagSet` registration, the usage text, completion scripts, the man page and the `docs/cli.md` usage blocks. If a full refactor is too large for one plan, derive completions and man from the real `FlagSet`s via `VisitAll` at runtime plus a small table for enums and positional kinds, and add a test that every flag in `VisitAll` appears in each shell's output with its description and that enum tables match the validation code (the same constants `--format` validation uses).
- Ship generation in the binary (`gruntled completion <bash|zsh|fish|powershell>`, `gruntled man`), so `go install` users and Homebrew's `generate_completions_from_executable` work and no generated file can be stale relative to the binary. Static completions only: do not invent a `__complete` callback that re-invokes the binary on every tab press.
- zsh output ends with the guard `if [ "$funcstack[1]" = "_gruntled" ]; then _gruntled "$@"; else compdef _gruntled gruntled; fi` so both `fpath` install and `source` after `compinit` work. bash: no hard dependency on bash-completion; plain `COMPREPLY` with `compgen`, bash 3.2 compatible. PowerShell: pure ASCII, no BOM dependence.
- Man: fixed `.TH` (no date, or `SOURCE_DATE_EPOCH`), `\-` for hyphens, escape leading `.`/`'`, one page `gruntled(1)`.
- CI: `bash -n` and `shellcheck` on bash output; `zsh -f -c 'autoload -Uz compinit && compinit -u && source <(gruntled completion zsh)'` smoke; `fish --no-execute`; PowerShell parse check with `[System.Management.Automation.Language.Parser]::ParseInput` (pwsh is on the GitHub runners); `mandoc -T lint` or `groff -ww -z`; and a **behavioural** bash test (set `COMP_WORDS`, `COMP_CWORD`, call the function, assert `COMPREPLY`) for `gruntled blast --format <TAB>` = `text json` (+ `sarif` once added) and `gruntled check --<TAB>` = real flags.
- Release archives: if completions/man are added to the archive, keep `docs/ci.md`'s `tar -xzf … gruntled` extraction working, and extend the `build-release.sh` and `release.yml` file-count smoke checks (7 files today, asserted in `release.yml`).

**Warning signs:**
A new flag is added and only `-h` changes; completions suggest `--format sarif` for `blast` before it is supported; `man gruntled` differs between two builds.

**Phase to address:** P20 (registry first, before completions, man, and `blast` SARIF/`--quiet` flags are added).

---

### Pitfall 8: GHCR image that works in `docker run` demos and fails (or misleads) in real repositories

**What goes wrong:**
- **Package private by default.** The first push of a package creates it private; `docker pull` from anonymous users fails (403/denied). Public cannot be reverted to private. The workflow token can push but visibility is a one-time manual setting, so the first real release is the first time anyone notices.
- **Tag mutability.** GHCR tags are mutable; re-running the workflow for a tag re-pushes under the same name. `:latest` must not move on prerelease tags (`vX.Y.Z-rc.N`, which `release.yml` treats as prerelease).
- **Attestation manifests.** Default buildx provenance/SBOM adds `unknown/unknown` entries to the index, which GHCR lists as odd architectures and which older tooling chokes on.
- **Different binary than the release.** A multi-stage Dockerfile that runs `go build` produces a binary with other flags/toolchain, bypassing the release proofs (`check-architecture`, the checksum). The image must contain the **same bytes** as the release archive.
- **`FROM scratch` has no `/tmp`, no `/etc/passwd`, no `$HOME`.** `gruntled watch` inside the image needs a runtime directory: `XDG_RUNTIME_DIR` unset, `os.UserCacheDir` fails (no HOME), `os.TempDir()` = `/tmp` does not exist. (CA certificates are indeed not needed; gruntled makes no network calls.) State that the image is for `check`, `graph`, `blast`, not `watch`, or add an explicit empty `/tmp` and document volumes.
- **uid/gid and permissions.** A numeric non-root `USER 65532:65532` is right for scratch, but a bind-mounted checkout with mode 0700/0600 files (umask 077, some CI checkouts) is unreadable and the result is a permission error at exit 3. Users need `--user "$(id -u):$(id -g)"`; document it.
- **Mounting only part of the repository gives silent false negatives.** `-v $PWD/live:/repo` hides `../modules`, so every unit that sources a module outside the mount becomes `module-unknown` (silent by design, no error). The docs must say: mount the repository root, run `gruntled check live/`. A symlink pointing outside the mount is refused by `os.OpenRoot` and also becomes unknown.
- **`blast --base`** needs two mounts (`/repo` and `/base`); `git worktree add` produces a `.git` file with an absolute host path that is useless in the container (irrelevant to gruntled, which never reads git, but users will hit confusion).
- **Output paths.** Output is relative to the analysed path, not to the mount, so `-w /repo … check .` prints exactly what the host binary prints. This is a feature; **prove it**.
- Docker Desktop on Windows: path translation (`-v ${PWD}:/repo`), CRLF checkouts are irrelevant but permission mapping is not (all files appear 0777/root-owned).

**How to avoid:**
- Dockerfile: `FROM scratch`; `COPY` the **already-built** linux binary per architecture from the release dist (`ARG TARGETARCH`, files `gruntled_linux_amd64`/`arm64` extracted from the verified archives); `USER 65532:65532`; `ENTRYPOINT ["/gruntled"]`; `LABEL org.opencontainers.image.source=https://github.com/GiulioSavini/gruntled` (links the package to the repo), plus `version`, `revision`, `licenses=Apache-2.0`. In the job, assert the image's binary hash equals the line in `checksums.txt`.
- `docker/build-push-action` with `platforms: linux/amd64,linux/arm64` (no QEMU needed, there is no `RUN`); set `provenance: false` and `sbom: false` and publish a separate `actions/attest-build-provenance` attestation with `push-to-registry`, to avoid `unknown/unknown`. Record the pushed digest in the release notes and tell users to pin by digest.
- Tags: `X.Y.Z` (decide v-prefix once and keep it, matching the asset naming policy) and `latest` only for non-prerelease. Permissions on the job: `packages: write`, `id-token: write`, `attestations: write`, `contents: read` only on this job.
- **Order inside `release.yml`:** the file's invariant is "nothing published unless every earlier step passed, publication is a single `gh release create`". Keep it: proofs → build → publish release → push image → bump tap, as separate jobs after `publish`, each idempotent and re-runnable via `workflow_dispatch` with a `tag` input. Never push the image before the release exists (a failed release must not leave a `:X.Y.Z` image behind).
- **Post-publish verification job (CI proof of the claims):** `docker logout ghcr.io` then anonymous `docker pull`; run the image against a fixture with `--network none --read-only --cap-drop ALL -v "$PWD/fixture:/repo:ro" -w /repo` and diff stdout against the host binary byte for byte; run `--version`. A pre-release rehearsal must exist (Pitfall 12).
- Docs: a short "Running the image" section with the exact commands (mount root, `:ro`, `--user`, two mounts for `blast`), executed by the new CI job so it cannot rot.

**Warning signs:**
`docker run` works for the maintainer (logged in) and not for a user; an architecture listed as `unknown/unknown`; `docker run … watch` fails with a missing directory; results differ between host and container.

**Phase to address:** P19.

---

### Pitfall 9: Homebrew tap automation with an over-broad token, a stale formula, or a formula that installs the wrong thing

**What goes wrong:**
- **Token scope.** The workflow `GITHUB_TOKEN` cannot push to another repository (403 "Resource not accessible by integration", the well-known GoReleaser failure). The common fix is a classic PAT with `repo`+`workflow` scope, which can write to every repository of the owner, including this one. If it leaks through a compromised action it is far worse than a tap leak.
- **Silent expiry.** A fine-grained PAT expires (up to 1 year). The release succeeds, the tap stays on the old version, nobody notices until a user reports it.
- **Prereleases in the stable channel.** Bumping the formula for `vX.Y.Z-rc.1` publishes a release candidate to `brew install`.
- **Hash source.** Computing SHA-256 by downloading the published assets races CDN propagation and trusts what is served; computing from the build's own `checksums.txt` is deterministic.
- **Non-idempotent updates.** Re-running for the same tag produces an empty commit/force-push or a non-fast-forward failure.
- **Wrong formula shape.** Building from source needs `depends_on "go" => :build` and the `-X main.version` ldflags (easy to forget, producing `dev (none)` and failing the formula `test`). Installing prebuilt archives needs one `url`/`sha256` per `on_macos`/`on_linux` x `on_arm`/`on_intel` combination; a missing combination fails only for that user.
- **Formula `test do`** typically asserts the version: `assert_match version.to_s, shell_output("#{bin}/gruntled --version")`. It silently couples the tap to Pitfall 1: `gruntled v0.5.0 (…)` contains `0.5.0`, so it passes only if the version string really carries the tag.
- Tap repo must be named `homebrew-<name>` for `brew install GiulioSavini/<name>/gruntled` to work; `brew audit --strict` cannot run on a Linux runner.

**How to avoid:**
- Fine-grained PAT (or a GitHub App installation token) limited to the **tap repository only**, `contents: write` only, stored as a secret on a protected `release` environment (deployment branch/tag rule) and exposed only to the tap job, never to the build job. Calendar the expiry; make the tap job fail loudly (red workflow) rather than skip when the secret is absent or invalid, and allow re-run via `workflow_dispatch(tag)`.
- Skip the tap (and `:latest`) when the tag has a suffix (reuse the guard regex in `release.yml`).
- Render the formula from a template in this repository using `checksums.txt` values; commit to the tap only when the rendered file differs; rebase/retry on non-fast-forward.
- Prefer installing the release archives (identical bytes to the GitHub release, hash-pinned) over building from source; generate completions with the Pitfall 7 subcommand (`generate_completions_from_executable`).
- Rehearsal checks that run without a tag and without brew: `ruby -c` on the rendered formula, a test that the template mentions exactly the six release targets that apply to Homebrew (darwin/linux x amd64/arm64) and is checked for equality against the target list (the same single-source discipline as `TestReleaseTargetsInSync`), and a macOS runner job that runs `brew install --formula ./gruntled.rb` and `brew test gruntled` against the dry-run archives served from a local path.

**Warning signs:**
A secret called `PAT` with `repo` scope; tap has a release candidate; the tap's last commit is older than the last release.

**Phase to address:** P19.

---

### Pitfall 10: Release automation that cannot be exercised before the first real tag, and immutable releases that make mistakes permanent

**What goes wrong:**
The project's strongest lesson (v0.2 retrospective) is that the first real release went green because every step had run in CI before. v0.5 adds steps that only run on a tag (GHCR push, tap PAT, attestations, `install.sh` generation) and **tagging is explicitly Giulio's decision** (`git.create_tag: false`). So the first execution is the real release. Compounding that, GitHub immutable releases (recommended in Pitfall 2) lock assets and the tag name forever after publication: a botched `v0.5.0` cannot be fixed by re-uploading an asset or deleting and re-creating the tag. The recommended pattern is: create the release as a **draft**, attach all assets, then publish; `gh release create <tag> files…` may publish before assets are attached depending on the `gh` version on the runner (verify).

**How to avoid:**
- Every new release step has a **dry-run mode that runs on every push/PR** like `build-release.sh` does today: render `install.sh` and the formula as artifacts; `docker buildx build` without `--push` (load and run the image on the runner); `shellcheck`; template tests.
- A `workflow_dispatch` rehearsal path that targets a **sandbox fork or a scratch repository** (separate tap, separate GHCR namespace) for the parts that need real credentials, so PAT scope, GHCR visibility and attestation permissions are proven without burning a version number in the real repository. If immutable releases are enabled on the real repository, never use real tags for rehearsal.
- Switch the publish step to draft → attach → publish, with `needs:`-ordered downstream jobs, and keep the existing tag guard and master-ancestor check.
- The v0.5.0 release checklist (P23) must list each manual one-time setting that automation cannot do: tap repository creation, GHCR package visibility and repository link, secret creation and expiry date, immutable releases toggle, private vulnerability reporting toggle.

**Warning signs:**
A plan's "verify" step says "will be exercised on the next release"; a release step has no corresponding CI job; the checklist is empty of settings.

**Phase to address:** P18 (dry-run harness and draft-publish ordering), P23 (checklist).

---

### Pitfall 11: Docs and recipes that pin or describe a world that no longer exists

**What goes wrong:**
`docs/ci.md` pins `v0.2.0` (a stale tag, v0.4 is shipped), README had stale status text until recently, and `docs/cli.md` "Known limitations" will contain statements v0.5 makes false: "GRT004 has no SARIF rule", "`watch`: no liveness check yet", "`report` on windows has no live query", "daemon baseline is its first index". Several traps:
- **Chicken and egg.** A doc that pins `vX.Y.Z` cannot be tested against a tag that does not exist until after the release commit; a test asserting "the pinned tag exists" fails on the release commit, and a hand-edit after tagging is forgotten.
- **`recipe-check` installs from the checkout**, not `@tag`, so a mistyped or stale pin is invisible (documented in Key Decisions).
- **Version strings in many places**: docs/ci.md, README, install script default, Homebrew template, Dockerfile labels, SARIF doc examples, help text, CHANGELOG, `.pre-commit-hooks.yaml` examples.
- **A fixed limitation left in the docs** (the symmetric failure of a missing doc).

**How to avoid:**
- One source of truth for "the released version docs point at": a single file (for example `docs/VERSION` or a Go const in a test-only package) read by a `TestDocPins` that (a) requires every `vN.N.N` in `docs/*.md` and `README.md` outside an allowlist (CHANGELOG, historical notes) to equal it, and (b) requires it to be a valid semver not greater than the next intended release. Bumping that one line is part of the release checklist, performed in the commit **after** tagging.
- Post-publish workflow job executes the documented install commands for real against the published release (`go install …@tag`, the `install.sh` line, `docker run`), so a stale pin fails loudly the day it is published.
- Each plan that removes a limitation deletes the limitation text in the same plan (done criteria), and P23 greps `docs/cli.md` and README for each retired phrase. Doc tests that already compare help text with `docs/cli.md` stay; add `-update`-style regeneration for the embedded usage blocks so the human step is a command, not an edit.
- New docs that users copy (Homebrew, image, install, completion install lines per shell, status-file prompt recipes) are executed in CI wherever a CI runner has the shell (bash, zsh, fish, pwsh are available on GitHub runners).
- Remember the retrospective lesson: finish doc edits before running the verifier, or budget a re-verification.

**Warning signs:**
`grep -rn 'v0\.[0-9]\.[0-9]' docs README.md` returns more than one distinct version; a "Known limitations" bullet cites a behaviour a test contradicts.

**Phase to address:** P18 (single source and pin test), every phase (limitation removal), P23 (audit).

## Moderate Pitfalls

### Pitfall 12: Dependabot noise versus SHA-pinned actions and the Go toolchain decision

**What goes wrong:**
All actions in `ci.yml`/`release.yml`/`corpus.yml` are pinned to 40-hex SHAs with a `# vX.Y.Z` comment. Dependabot updates SHA-pinned actions only when that comment is present, and produces one PR per action per release (checkout, setup-go, plus the new docker/login, setup-buildx, build-push, attest actions). A `gomod` ecosystem adds `hcl/v2`, `fsnotify`, `x/sys`, and may bump the `go` directive when a dependency requires a newer Go, which conflicts with `GOTOOLCHAIN: local` scripts and the "Go 1.27" decision. Dependabot-triggered workflows get a read-only token and no repository secrets; an `hcl` bump can change findings, yet the corpus gate runs only on dispatch/weekly. New actions added in P19 may be unpinned by habit.

**Prevention:**
- `dependabot.yml`: `github-actions` and `gomod`, monthly schedule, one `groups` entry per ecosystem, `cooldown` of several days (security updates bypass it), `open-pull-requests-limit` small.
- Add a CI test that every `uses:` in `.github/workflows/*.yml` matches `@[0-9a-f]{40}` (excluding local `./` actions) and carries a version comment; this also catches P19's new actions.
- Ignore `go` directive bumps via an `ignore` rule or a CI check that `go.mod`'s `go` line is unchanged unless the PR title says so.
- Require the full test suite (golden SARIF/JSON, byte-identical output) on dependency PRs and run `corpus.yml` once manually before merging an `hcl/v2` bump; note it in CONTRIBUTING.
- Release secrets are never needed on PR workflows (they are tag-only), which also keeps Dependabot PRs working.

**Phase to address:** P22 (after P19 so new actions are included).

### Pitfall 13: CHANGELOG drift and duplicated release notes

**What goes wrong:**
`release.yml` uses `gh release create --generate-notes`, so a hand-written CHANGELOG becomes a second, diverging record. Writing history for v0.1-v0.4 from memory invents dates; v0.4.0 is untagged, so link references to `compare/v0.3.0...v0.4.0` 404 today.

**Prevention:**
- Keep a Changelog format with an `[Unreleased]` section; derive v0.1-v0.4 entries from `.planning/milestones/` archives, not memory; `v0.4.0` listed as unreleased/pending until tagged.
- Single source: the release workflow fails if `CHANGELOG.md` lacks `## [X.Y.Z]` for the tag, and publishes that section as the release body (`--notes-file`) instead of `--generate-notes`. Test the extraction script in CI on the current `[Unreleased]` section.
- Add the changelog-entry line to the PR template and the release checklist.

**Phase to address:** P22 (workflow change coordinated with P18's draft-publish step).

### Pitfall 14: SECURITY.md that promises what one maintainer cannot keep, and CODEOWNERS that blocks the only maintainer

**What goes wrong:**
SLAs ("acknowledged within 48 hours") nobody can staff, a private email address that is actually a personal mailbox, a "supported versions" table that grows stale, and a disclosure channel that is not enabled (private vulnerability reporting is a repository setting, not a file). CODEOWNERS with "Require review from Code Owners" in branch protection deadlocks a one-maintainer repository because authors cannot approve their own PRs; an invalid owner or syntax makes the file silently ineffective (GitHub flags errors).

**Prevention:**
- SECURITY.md: use GitHub private vulnerability reporting (enable it, then link to the advisories page), say "best effort" with no SLA, support only the latest minor, list in-scope surfaces specific to gruntled (the read-only and no-exec guarantees, the daemon socket/pipe trust boundary, parser denial of service, terminal-escape handling) and out-of-scope (the analysed repository's own content). Do not publish a personal email.
- CODEOWNERS: a single `* @GiulioSavini` line is enough; do not turn on "require code owner review" while there is one maintainer; verify the file shows no errors in the GitHub UI.
- Issue templates: bug report asks for `gruntled --version`, OS, a minimal repository; a dedicated **false-positive** template (the product promise) with the highest visibility; `config.yml` disables blank issues only if intended; any label named in a template must exist or it is silently not applied; never ask users to paste private infrastructure code.
- Ensure every claim in SECURITY.md and CONTRIBUTING (for example "run `scripts/check-architecture.sh`") matches a command that works; the architecture script takes ~18 minutes locally per the retrospective, so say so and point at CI.

**Phase to address:** P22.

### Pitfall 15: Adding any dependency or stdlib import weakens the no-net/no-exec proof unnoticed

**What goes wrong:**
P20 (TTY detection, SIDs), P21 (named pipes, hostname, user identity), P19 (nothing in the binary) may pull `x/term`, `x/sys/windows`, `os/user` or other packages. The import half of the proof is exact; the process-spawn half is a source scan for qualified calls, "a strong guard, not a formal proof". A package that wraps `CreateProcess` in `x/sys/windows` is linked but unused; the scan may or may not flag it, and the failure mode is a false alarm or a hole.

**Prevention:**
- For every new import, run `scripts/check-architecture.sh` (or its per-target step) before writing code on top of it; record the result in the plan's summary.
- Prefer stdlib `syscall` with build tags (the existing `lock_unix.go`/`lock_windows.go` pattern) for ioctl and console mode; add `x/sys/windows` only if needed for `CreateNamedPipe`/security descriptors.
- Add the new windows pipe and TTY files to the existing per-target dependency check (it already runs `go list -deps` for all six targets), and add a case in `test-check-architecture.sh` that fails when `net` is linked for windows via the pipe package.

**Phase to address:** P20, P21 (each plan's first task).

### Pitfall 16: `watch --base <dir>` interacts with rebase, the watcher and the status contract

**What goes wrong:**
`--base` pointing inside the watched repository makes the watcher see the base tree's files change (or be reindexed as units); a `git worktree` base whose checkout is changed by the user does not update the in-memory baseline (it is read once), which may surprise; after `rebase` the baseline source changes from `--base` to `rebase` and a restart reverts to `--base`; the "baseline: daemon generation 1 (watch start)" label and JSON `daemon_baseline.source` gain a third value, and IPC protocol v2 consumers (`source` is an open string today) must be told. An unreadable `--base` must be exit 3, never a silent fallback to "first index" (the same rule `blast` already has).

**Prevention:**
- Apply the same "outside the repository by file identity" guard used for the runtime directory and `--status-file`; exit 2 on overlap.
- New `source` value documented in `docs/cli.md`; keep protocol version unless the wire shape changes (additive value only); add a golden for the first-line label.
- Test: restart with `--base`, run `rebase`, restart again and confirm the baseline returns to `--base`.

**Phase to address:** P21.

## Minor Pitfalls

### Pitfall 17: Smaller traps worth a checklist line

- **Archive layout change** (adding completions/man/README) breaks `tar -xzf … gruntled` only if names change; keep `gruntled`/`gruntled.exe` and `LICENSE` at archive root, and update the file-count smoke in `release.yml` (currently hard-coded 7) and `build-release.sh`.
- **`checksums.txt` scope**: it is built with `sha256sum gruntled_*`; a new `install.sh` asset is outside that glob. Decide whether it is covered (add it) and test the verification of every published asset by a post-publish job.
- **GitHub Actions `GITHUB_PATH`** is the only way an installer can influence later steps; `echo` to the parent shell does nothing.
- **Completion install doc lines** differ per shell; give one tested line per shell, not prose.
- **Hostname/uid in status paths** would change `--print-status-path` output and thus every user's prompt snippet; do not change `hash12` inputs on unix. If windows needs a user discriminator, change only the windows path.
- **`--quiet` plus `--format json`**: JSON has no stderr summary today; do not add one.
- **Windows arm64 emulation**: an amd64 install script/PowerShell snippet on arm64 Windows should select the arm64 archive; document.
- **Dependabot on `.pre-commit-hooks.yaml`** is not supported; the hook's `rev` pin in docs follows the single version source (Pitfall 11).

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Hand-written completion scripts per shell | Fast first version | Drift on every flag change; four copies of the flag list | Never; derive from the registry or `FlagSet.VisitAll` |
| `docker build` compiling Go inside the Dockerfile | Self-contained Dockerfile | Image binary differs from released binary, bypasses proofs | Never for the published image |
| Classic PAT with `repo` scope for the tap | Works in minutes | Owner-wide write token in a workflow | Never; fine-grained, tap-only |
| Pid file for daemon liveness | Looks simple | Pid reuse, cross-platform probing, a second source of truth | Never; lock is the truth, pid is diagnostic text |
| `Impacted` units as SARIF results | "Complete" SARIF | Alert noise and false-positive appearance | Never |
| `latest`-tracking docs/recipes | No bump chore | Unreproducible CI, drifting behaviour | Never; pin and test |
| Generating completions only in release workflow (not in binary) | No new subcommand | `go install` users get none; no sync test | Only if a committed, CI-diffed generated file exists |
| Skipping the rehearsal (sandbox fork) | Faster | First real release is the experiment, immutable | Never once immutable releases are on |
| Colour decided inside the renderer | One code path | Breaks `report`==`check` bytes, golden tests | Never |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| GitHub Releases | `gh release create` with assets on an immutable-release repo, assets added after publish | Draft, attach all assets, publish; downstream jobs `needs:` publish |
| GHCR | Assuming the package is public and linked to the repo | `org.opencontainers.image.source` label, set visibility once, anonymous-pull check post-publish |
| docker/build-push-action | Default provenance/SBOM creates `unknown/unknown` entries | `provenance: false`, `sbom: false`, separate attestation |
| Homebrew tap | `GITHUB_TOKEN` or broad PAT | Fine-grained, tap-only token on a protected environment |
| GitHub code scanning | Same tool name, same category for `check` and `blast` uploads | Distinct `category`, one run per file, Impacted not as results |
| GitHub Actions PATH | Installer exports PATH in its own process | Append to `$GITHUB_PATH` when set |
| Dependabot | Expecting PRs for SHA pins without version comments | Keep `# vX.Y.Z` comments, group, cooldown |
| Windows named pipe | `go-winio` / `net` | Overlapped handle + `os.NewFile` or own OVERLAPPED, explicit DACL, first-instance flag |
| Docker bind mounts | Mounting only `live/` | Mount the repository root, run on a subpath |
| zsh | Sourcing completion before `compinit` | Dual-mode script with `funcstack` guard; `fpath` install |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Liveness check per prompt render spawning the binary and probing the lock | Prompt latency, lock contention with daemon start | Cheap mtime hint for prompts, `gruntled status` for scripts, probe via `ping` | Prompt refresh at 1 s on a slow machine |
| Heartbeat rewriting the status file | Disk churn, fsevents/inotify noise for editors | `Chtimes` only, >= 10 s period, tmpfs runtime dir | Always on laptops |
| Pipe write of a ~48 MiB report to a stalled client | Goroutine pinned forever | Overlapped IO with deadlines, same cap as sockets | Any large repository with a hung client |
| Man/completion generation in `build-release.sh` per target | Slower dry-run on every push | Generate once from the host binary (platform independent) | n/a (minor) |
| Docker multi-arch build with QEMU for a `scratch` image | Slow CI | No `RUN` steps, so no emulation needed | n/a |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Checksum from the same origin treated as authenticity | Compromised release passes verification | Embed hashes in a release-asset installer, immutable releases, attestations, honest wording |
| `install.sh` documented from `main` branch | Mutable script | Release-asset URL only |
| Named pipe with default DACL and no first-instance flag | Local users read or squat the daemon pipe, remote SMB reach | Explicit user-only DACL, `FIRST_PIPE_INSTANCE`, `REJECT_REMOTE_CLIENTS`, unpredictable name |
| Client printing squatter-supplied bytes to a terminal | Terminal escape injection | Pipe capability name; keep escape handling for anything not rendered by the same trusted daemon |
| Tap/GHCR credentials visible to build steps or PR workflows | Secret exfiltration via a compromised action | Per-job scoping, protected environment, tag-only triggers, SHA-pinned actions |
| Colour codes wrapped around repository-derived text before escaping | Re-opens terminal injection fixed in v0.3 phase 12 | Colour after escaping, strip-equality property test |
| Reader "fixing" a stale status file | Clobbers a fresh daemon's state | Readers are read-only |
| Reusing `hash12` across users on windows pipes | Cross-user pipe collision or access | Capability name and per-user DACL |
| SECURITY.md exposing a personal email, or no enabled channel | Doxxing or undisclosed reports | Private vulnerability reporting enabled |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| `--version` shows `dev (none)` for `go install` users | Cannot report which version has the bug | Canonical-tag fallback from build info (Pitfall 1) |
| Silent module-unknown when only part of a repo is mounted in Docker | False sense of safety | Document root mount; consider a one-line stderr hint when many units are `module-unknown` (verify it does not change deterministic stdout) |
| Stale status line looks current | Developer trusts a dead daemon | `gruntled status` and prompt recipe using it |
| `--no-color` ignored because `FORCE_COLOR` is set in CI | Unreadable logs | Explicit precedence, documented table |
| Installer prints nothing on success or edits rc files | Mistrust | Print version, destination, PATH hint; never edit rc files |
| Completion works in bash but not zsh until `compinit` is re-run | Feature appears broken | Per-shell tested install line including the zcompdump note |
| Blast SARIF shows duplicate alerts with check SARIF | Reviewers distrust both | Single upload per PR job, distinct category |

## "Looks Done But Isn't" Checklist

- [ ] **`--version`:** passes locally but prints a pseudo-version for `@latest` — verify the table including pseudo-version, `(devel)` and `+dirty` rows, and the post-publish `go install @tag` job.
- [ ] **Installer:** works on ubuntu with curl — verify under `dash`, Alpine/busybox, macOS bash 3.2, Apple Silicon under Rosetta, truncated script, tampered archive, unwritable destination, second run.
- [ ] **Homebrew:** formula renders — verify per-OS/arch blocks cover all four, `brew test` passes on a macOS runner, prerelease tags are skipped, a re-run is a no-op.
- [ ] **GHCR:** image runs for the maintainer — verify anonymous pull, both architectures, no `unknown/unknown`, binary hash equals `checksums.txt`, `--read-only --network none` run matches host bytes.
- [ ] **Completions/man:** generated once — verify every `VisitAll` flag appears, enums match validation, zsh `compinit` smoke, bash 3.2 compatibility, `pwsh` parse, man lint, reproducible bytes.
- [ ] **Colour:** works in a terminal — verify `stripANSI(colorize(x)) == x`, `/dev/null` is not a TTY, `NO_COLOR`/`FORCE_COLOR`/`--no-color` matrix, JSON/SARIF never coloured, windows console fallback, `report` bytes unchanged.
- [ ] **Liveness:** stale detection works after `kill -9` — verify concurrent `status` calls do not make a starting daemon exit 0 ("already watching"), status grammar unchanged, readers never write.
- [ ] **Windows pipe:** passes alone — verify explicit DACL, first-instance, remote rejection, 64 MiB cap and malformed-input table on both transports, stalled-client deadline, `go list -deps` for both windows targets has no `net`/`os/exec`.
- [ ] **Blast SARIF:** schema-valid — verify Impacted is not in `results`, `rules` identical across producers, bytes independent of `--base` path, upload to a real repository with distinct category shows no duplicates.
- [ ] **Hygiene files:** present — verify private vulnerability reporting is enabled, CODEOWNERS shows no errors, template labels exist, Dependabot PRs appear grouped, every `uses:` is SHA-pinned.
- [ ] **Docs:** accurate at commit time — verify single pin source test, retired limitations removed, install commands executed post-publish.
- [ ] **Release readiness:** workflow edited — verify a rehearsal ran (fork or sandbox), the manual settings checklist is complete, and no step is "first exercised on the real tag".

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| Release published with wrong assets (immutable on) | HIGH | Burn the version, publish the next patch tag, mark the broken release as superseded in CHANGELOG; this is why rehearsal comes first |
| GHCR package private | LOW | Change visibility in package settings (one-way), link the repo, re-run anonymous-pull job |
| Tap token expired/leaked | LOW/MEDIUM | Rotate the fine-grained token, re-run tap job via `workflow_dispatch(tag)`; for a leak, revoke and audit the tap history |
| Colour broke golden/`report` bytes | MEDIUM | Move colouring to the process edge, restore goldens, add the strip-equality property test |
| Probe race causing silent non-start | MEDIUM | Add bounded `TryLock` retry, add the concurrent probe race test, document workaround (restart) |
| Completions drifted | LOW | Add the registry-to-completion sync test; regenerate |
| Docs pinned to stale tag | LOW | Bump the single pin source, run `TestDocPins` |
| SARIF rule table changed unexpectedly | MEDIUM | Decide intentionally, update goldens and compat gate, note in CHANGELOG |
| Pipe DACL too permissive found late | HIGH | Security fix release; this is why the explicit DACL is part of the first implementation |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| 1 `--version` build-info traps | P18 | Injected-reader table test; existing `dev (none)` rows kept; post-publish `go install @tag` job |
| 2 Installer trust, truncation, portability | P19 (asset layout from P18) | shellcheck + dash/busybox/macOS jobs, tamper/truncation negative tests, idempotency inode test |
| 3 Liveness race and pid | P21 | Concurrent Probe vs daemon-start race test; `kill -9` then `status` shows not running; grammar golden |
| 4 Windows named pipe | P21 | Conformance table on both transports; dependency check on windows targets; Windows CI stalled-client test |
| 5 Blast SARIF | P20 | Schema CI step; Impacted-not-in-results test; rules-equal golden; base-path independence test; real upload rehearsal in P23 |
| 6 Colour/TTY | P20 | Precedence matrix test; strip-equality property; `/dev/null` test; `report` bytes equal `check` |
| 7 Completion/man drift | P20 (registry first) | VisitAll-vs-completion test, enum table test, shell smoke jobs, man lint, reproducibility diff |
| 8 GHCR image | P19 | Anonymous-pull, host-vs-container diff under `--read-only --network none`, hash equals `checksums.txt` |
| 9 Homebrew tap | P19 | Template/target-list equality test, `ruby -c`, macOS `brew install`/`test` job, prerelease skip test |
| 10 Release automation untestable until tag; immutability | P18 (harness), P23 (checklist) | Dry-run jobs on every push; sandbox rehearsal log; manual-settings checklist signed off |
| 11 Stale docs/pins | P18 (source and test), all phases, P23 | `TestDocPins`; post-publish install job; limitation-phrase grep |
| 12 Dependabot noise | P22 | Pin-lint test; grouped/cooldown config; `go` directive guard |
| 13 CHANGELOG drift | P22 | Release workflow fails without section; extraction script test |
| 14 SECURITY/CODEOWNERS/templates | P22 | PVR enabled check; CODEOWNERS shows no errors; template labels exist |
| 15 Dependency weakens proof | P20, P21 (first task of each) | `check-architecture.sh` run per new import; windows target step |
| 16 `watch --base` interactions | P21 | Overlap guard test; restart/rebase/restart baseline test; label golden |
| 17 Minor traps | Respective phase | Checklist lines above |

## Sources

- gruntled repository (read directly, HIGH): `.planning/PROJECT.md` (constraints, Key Decisions: `--version` ldflags-only revisit, recipe-check, release targets), `.planning/RETROSPECTIVE.md` (rehearsal, doc-sync tests, `-short` flaky lesson, audit pattern), `docs/cli.md` (Guarantees, Known limitations, SARIF section, status file, report/rebase), `docs/ci.md` (v0.2.0 pins, `--ignore-missing`), `cmd/gruntled/main.go` and `main_test.go:160-162` (version contract), `.github/workflows/release.yml` and `scripts/build-release.sh` (single `gh release create`, 7-file smoke, `-ldflags -X main.version`), `internal/infrastructure/ipc/{ipc,lock_unix,lock_windows}.go` (lock as single source of truth, `Probe` acquires and releases the lock, `maxResponse` 64 MiB), `go.mod` (`x/sys` indirect), local check: public `syscall` lacks `CreateNamedPipe`, `GOOS=windows go list -deps` shows no `x/sys` linked.
- Go 1.24 release notes via search (MEDIUM): `go build` stamps the main module version from VCS tag/commit, `+dirty` when modified, `-buildvcs=false` omits it. https://go.googlesource.com/website/+/2a8b895514c4e52860e9c0b1ef8626db3ea36bf5%5E%21/ and https://go.googlesource.com/go/+/f5fe5a4524d5e5390238ae7f5abcaa4299b31a37/src/runtime/debug/mod.go
- GitHub immutable releases GA (MEDIUM-HIGH): https://github.blog/changelog/2025-10-28-immutable-releases-are-now-generally-available/ and https://docs.github.com/en/code-security/concepts/supply-chain-security/immutable-releases (draft, attach, publish pattern; community reports on asset-upload ordering in https://github.com/orgs/community/discussions/178351)
- Code scanning SARIF behaviour (MEDIUM): https://github.blog/changelog/2025-07-22-code-scanning-will-stop-combining-multiple-sarif-runs-uploaded-in-the-same-sarif-file and GitHub docs on categories / same tool+category replacing results (https://docs.github.com/en/enterprise-server@3.19/code-security/how-tos/find-and-fix-code-vulnerabilities/integrate-with-existing-tools/upload-sarif-file)
- Named pipe security (MEDIUM-HIGH): https://learn.microsoft.com/windows/win32/api/namedpipeapi/nf-namedpipeapi-createnamedpipew (default DACL grants Everyone read; `FILE_FLAG_FIRST_PIPE_INSTANCE`; `PIPE_REJECT_REMOTE_CLIENTS`), https://learn.microsoft.com/en-ie/Windows/Win32/ipc/named-pipe-security-and-access-rights, https://www.cyberark.com/resources/threat-research/that-pipe-is-still-leaking-revisiting-the-rdp-named-pipe-vulnerability (descriptor of the first instance applies to later ones)
- Go `os.NewFile` overlapped handles and deadlines (MEDIUM, verify in a Windows rehearsal): https://go.dev/src/os/file_windows.go and the `os.NewFile` documentation
- GHCR visibility and attestation index behaviour (MEDIUM): GitHub Packages visibility docs (https://docs.github.com/en/enterprise-server@3.4/packages/learn-github-packages/configuring-a-packages-access-control-and-visibility), https://github.com/orgs/community/discussions/45969 (`unknown/unknown` with attestations)
- Homebrew tap token scope (MEDIUM): https://goreleaser.com/resources/errors/resource-not-accessible-by-integration/ ; bump-formula token guidance via https://awesome.ecosyste.ms/projects/1463427
- Dependabot grouping and cooldown (MEDIUM): https://github.blog/security/application-security/tame-dependabot-group-your-updates-slow-the-cadence-keep-security-fast/ (SHA-pin comment requirement from PR discussions found by search; confirm against current Dependabot reference)
- Reasoned from platform knowledge without a fetched source (LOW-MEDIUM, flagged inline): Rosetta `uname -m`, busybox/macOS `sha256sum` differences, zsh `compdef`/`compinit` ordering, bash 3.2 limitations, NO_COLOR semantics (https://no-color.org), `/dev/null` as character device, `gh release create` draft behaviour per `gh` version.

---
*Pitfalls research for: adoption/distribution/UX features on a deterministic, no-network, no-exec Go CLI with a daemon (gruntled v0.5)*
*Researched: 2026-10-16*
