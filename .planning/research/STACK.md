# Technology Stack: v0.5 Adoption & Completeness (additions only)

**Project:** gruntled
**Milestone:** v0.5 Adoption & Completeness (subsequent milestone; v0.1-v0.4 stack is validated and not re-researched)
**Researched:** 2026-10-16
**Overall confidence:** HIGH for everything marked "verified here" (built and run in this session on go1.27.2, including Windows binaries executed through WSL interop). MEDIUM for third-party service behaviour (Homebrew, Scoop, winget, GHCR) that was read from docs but not exercised.

## Headline

v0.5 needs **zero new runtime Go modules**. Every Go-side feature (version, completions, man pages, colour/TTY, Windows named pipe) is best done with the standard library plus `golang.org/x/sys/unix`, which is already linked on unix targets through fsnotify. Everything else is release-pipeline tooling (GitHub Actions, shell, Ruby formula text) that never enters the binary.

The research turned up one trap that changes the plan:

> **`golang.org/x/sys/windows` imports `net` (in `types_windows.go`) in every version ever released (checked v0.1.0, v0.10.0, v0.20.0, v0.30.0, v0.38.0, v0.47.0, v0.49.0).** Anything that imports it links `net` and `net/netip` on windows. That kills `golang.org/x/term` on windows (it imports x/sys/windows), `go-colorable`, `fatih/color`, `mgutz/ansi`, `termenv`, `lipgloss`, `Microsoft/go-winio` and `natefinch/npipe`. The existing proof half 4 ("windows must not link x/sys/windows") is therefore not just a polling rule, it is also a no-net rule. **Keep it. Do all windows syscalls with the standard `syscall` package.**

A second trap: **`text/template` and `html/template` import `net/url` and `net/netip`.** The architecture proof forbids `net/.+`, so no generator (completions, man pages, formula rendering inside Go) may use `text/template`. Use `strings.Builder` / `fmt`.

## Verified link table (what links `net` / `os/exec`)

`go list -deps` run on linux/amd64 and windows/amd64 (module versions as resolved on 2026-10-16). "net" means `net`, `net/url` or `net/netip` is in the dependency closure; the proof's pattern is `^(net|net/.+|os/exec|plugin|crypto/tls)$`.

| Package | Version | linux/darwin | windows | Verdict |
|---|---|---|---|---|
| stdlib `flag`, `log/slog`, `embed`, `text/tabwriter`, `encoding/json`, `compress/gzip`, `archive/*`, `crypto/sha256` | go1.27.2 | clean | clean | OK |
| stdlib `text/template`, `html/template` | go1.27.2 | **net/url, net/netip** | **net/url, net/netip** | FORBIDDEN |
| `golang.org/x/sys/unix` | v0.49.0 (repo pins v0.47.0) | clean | n/a | OK (already linked by fsnotify) |
| `golang.org/x/sys/windows` | every version | n/a | **net, net/netip** | FORBIDDEN |
| `golang.org/x/term` | v0.47.0 | clean | **net, net/netip** (via x/sys/windows) | FORBIDDEN (windows) |
| `github.com/mattn/go-isatty` | v0.0.24 | clean | clean (uses `syscall` LazyDLL) | Acceptable fallback only |
| `github.com/mattn/go-colorable` | v0.1.16 | clean | **net** (x/sys/windows) | FORBIDDEN |
| `github.com/fatih/color` | v1.19.0 | clean | **net** | FORBIDDEN |
| `github.com/mgutz/ansi` | 2020 | clean | **net** | FORBIDDEN |
| `github.com/muesli/termenv` | v0.16.0 | **net/url, template** | **net** | FORBIDDEN |
| `github.com/charmbracelet/lipgloss` | v1.1.0 | **net/url, os/exec, template** | **net, os/exec** | FORBIDDEN |
| `github.com/spf13/cobra` / `pflag` | latest | **net** | **net** | FORBIDDEN (already decided) |
| `github.com/urfave/cli/v3` | v3.14.0 | **net/url, template** | same | FORBIDDEN |
| `github.com/alecthomas/kong` | v1.16.1 | **net/url** | same | FORBIDDEN |
| `github.com/posener/complete/v2` | v2.1.0 | **os/exec, net/url, template** | same | FORBIDDEN |
| `github.com/Microsoft/go-winio` | v0.6.4 | n/a | **net** (it returns `net.Listener`) | FORBIDDEN |
| `gopkg.in/natefinch/npipe.v2` | v2 | n/a | **net** | FORBIDDEN |
| `github.com/muesli/roff` / `mango` | v0.1.0 / v0.2.0 | clean | clean | Clean but unmaintained since 2022; not worth a dependency (see 3) |
| `github.com/cpuguy83/go-md2man/v2/md2man` | v2.0.7 | clean | clean | Clean; needs a markdown source and a build step; rejected (see 3) |

## Recommended stack, by feature

### 1. Installation parity

| Component | Technology | Version | Links net? | Why |
|---|---|---|---|---|
| Install script | POSIX `sh` (`install.sh` at repo root, also attached to the release) | n/a | n/a, not Go | `curl \| sh` users have `sh`, not bash. Lint with `shellcheck`, test under `dash` and `bash`. |
| Release packaging | existing `scripts/build-release.sh` (keep, no goreleaser) | n/a | n/a | Decision already locked. Extend it, do not replace it. |
| Homebrew | Tap repo `GiulioSavini/homebrew-tap`, `Formula/gruntled.rb` rendered by `scripts/render-formula.sh` | Homebrew formula DSL | n/a | Separate repo is required for `brew tap GiulioSavini/tap` (prefix `homebrew-`, `Formula/` dir; verified in docs.brew.sh). Prebuilt binaries via `on_macos`/`on_linux` x `on_arm`/`on_intel` blocks with `url`+`sha256` (these are plain binaries, not bottles). |
| Container image | `Dockerfile` `FROM scratch` + `docker/build-push-action` v7.4.0, `docker/setup-buildx-action` v4.4.1, `docker/login-action` v4.6.0, `docker/metadata-action` v6.2.0 | see SHAs below | not in binary | Image contains the exact release binary (COPY from the already-built, already-proven dist), so there is nothing new to prove. |
| Attestation | `actions/attest` v4.2.2 (`actions/attest-build-provenance` v4.2.2 is a thin wrapper; docs say prefer `actions/attest`) | pin by SHA | not in binary | `subject-checksums: dist/checksums.txt` consumes the existing `sha256sum` output directly; for the image use `subject-name` + `subject-digest` + `push-to-registry: true`. Users verify with `gh attestation verify`. Needs `id-token: write`, `attestations: write` (+ `artifact-metadata: write`). |
| Scoop | Own bucket repo `GiulioSavini/scoop-bucket`, `bucket/gruntled.json` rendered by the same script | Scoop manifest | n/a | Cheapest windows channel; same `checksums.txt` source. |
| winget | Defer. One manual first PR with `wingetcreate`, then `vedantmgoyal9/winget-releaser` v2 | n/a | n/a | Moderation delay, classic-PAT requirement, `InstallerType: zip` + `NestedInstallerType: portable` (zip support needs winget 1.5+). Not worth v0.5 scope; document as "planned". |

**install.sh design (opinionated):**
- `#!/bin/sh`, `set -eu`, everything inside `main()` invoked on the last line, so a truncated download executes nothing.
- No root: default `${GRUNTLED_INSTALL_DIR:-$HOME/.local/bin}`; never calls `sudo`; prints a PATH hint instead.
- Version: positional arg or `GRUNTLED_VERSION`; default resolves the latest tag by following the redirect of `https://github.com/GiulioSavini/gruntled/releases/latest` (`curl -fsSLI -o /dev/null -w '%{url_effective}'`), which needs no API token, no rate limit, no `jq`.
- OS/arch: `uname -s` Linux|Darwin, `uname -m` x86_64|amd64 -> amd64, aarch64|arm64 -> arm64. Reject MINGW/MSYS/CYGWIN with a pointer to the zip/Scoop. Alpine/musl is fine (binaries are static).
- Verification: download the archive and `checksums.txt` from the same tag; compute SHA-256 with `sha256sum`, falling back to `shasum -a 256`, then `openssl dgst -sha256`; match the exact archive filename in `checksums.txt` (never `--ignore-missing` blindly); abort on mismatch or missing entry; extract in a `mktemp -d` dir with a `trap` cleanup; `install -m 0755`; finally run `gruntled --version` and compare with the requested tag.
- Honest trust statement in docs: a checksum fetched from the same origin proves integrity, not authenticity. Authenticity is the attestation (`gh attestation verify`) or GitHub immutable releases (`gh release verify`, GA since 2025-10-28). The script may opportunistically run `gh attestation verify` when `gh` exists and `GRUNTLED_VERIFY=1`; it must never require it.
- Testability: `GRUNTLED_BASE_URL` override so CI runs the script against the `dist/` produced by the existing release dry run (curl supports `file://`), on ubuntu and macos.

**release.yml shape (least privilege, keeps "one atomic gh release create"):**
1. `release` job (as today, `contents: write`) builds, proves, publishes. Add `completions/`, `man/` to archives here if chosen (see 3).
2. Post-publish jobs, each idempotent and re-runnable by `workflow_dispatch` with a tag input, each with only the permission it needs: `image` (`packages: write`, `id-token: write`, `attestations: write`), `attest` (`id-token`, `attestations`), `tap` (`GITHUB_TOKEN` permissions `{}`, a fine-grained PAT scoped to the tap repo only, `contents: write`). A tap or image failure must not un-publish a release.
3. Image: `docker buildx build --platform linux/amd64,linux/arm64`, Dockerfile is `FROM scratch` / `ARG TARGETARCH` / `COPY` the per-arch binary extracted from the release archive / `ENTRYPOINT ["/gruntled"]`. No `RUN`, so **no QEMU needed**. Set `org.opencontainers.image.source` (metadata-action does) so the package links to the repo. Tags via metadata-action semver: `vX.Y.Z`, `X.Y`, and `latest` only for non-prerelease (`flavor: latest=auto`). Document `docker run --rm -v "$PWD:/repo:ro" ghcr.io/giuliosavini/gruntled check /repo`; `watch`/`report`/`rebase` are not supported in the image (no writable runtime dir, no shell).
4. CI dry run: build the image (host arch, no push) in `ci.yml` and `docker run ... --version`, exactly like the existing packaging dry run.

**Pinned action SHAs resolved on 2026-10-16 (re-resolve at implementation time; Dependabot will bump them):**

| Action | Tag | SHA |
|---|---|---|
| docker/build-push-action | v7.4.0 | c3c9e263c25d99ce0380d002d59b67737d91b0dc |
| docker/setup-buildx-action | v4.4.1 | f87e5991a6d7451dcb8d9637bfbc97413f497069 |
| docker/login-action | v4.6.0 | dbcb813823bdd20940b903addbd779551569679f |
| docker/metadata-action | v6.2.0 | dc802804100637a589fabce1cb79ff13a1411302 |
| actions/attest | v4.2.2 | 1e69f48acb82d1966a394da916b4c1698aa569d6 |
| actions/upload-artifact | v7.0.2 | cf430e030ddbb5b0abf93d22962f4752f3646cd9 |
| actions/download-artifact | v8.0.2 | 9000827ccba6bdab643e8b6fd33ac0654aef8333 |
| actions/create-github-app-token (optional, instead of a PAT) | v3.3.0 | 64ab729a6294000f5aa4a4bc0b73d1f32bc45510 |
| vedantmgoyal9/winget-releaser (deferred) | v2 | b7370d4d152efdce803a26cc6ca9eee58cb2ad54 |

**Homebrew update mechanism (recommended):** `scripts/render-formula.sh <tag> <checksums.txt>` (bash + awk, no Ruby, no brew needed on the runner) fills a `packaging/homebrew/gruntled.rb.in` skeleton with the four darwin/linux archive hashes from `checksums.txt`; `release.yml` clones the tap, commits and pushes. The script is exercised on every PR against the dry-run `checksums.txt`, so the generator cannot rot. Reject `dawidd6/action-homebrew-bump-formula` v10 and `mislav/bump-homebrew-formula-action` v4.2: they drive `brew bump-formula-pr`, which is built around one `url` and does not handle a four-URL `on_*` binary formula well (MEDIUM confidence; not exercised). Secretless alternative if a cross-repo PAT is unwanted: a scheduled workflow in the tap repo that reads the public release and commits with its own token (latency = cron interval).

Formula essentials: `version "X.Y.Z"` (no `v`), `url ".../download/v#{version}/gruntled_v#{version}_darwin_arm64.tar.gz"`, `def install; bin.install "gruntled"; end` (archives have no top-level directory, so Homebrew stages them in place), `livecheck { url :stable; strategy :github_latest }`, `test do assert_match version.to_s, shell_output("#{bin}/gruntled --version") end` (this makes `--version` correctness a Homebrew prerequisite, see 2). Completions: `generate_completions_from_executable(bin/"gruntled", "completion")` runs the binary at `brew install` time (acceptable: that is Homebrew running it, not gruntled spawning anything); verify the exact argument convention against the pinned Homebrew when implementing. Man page: `man1.install` from the archive or `(man1/"gruntled.1").write Utils.safe_popen_read(bin/"gruntled", "man")`. `brew tap-new` generates bottle-building workflows; delete them, a binary-only tap does not need bottles. Run `brew audit --strict --online` and `brew style` in the tap repo's CI.

**Platform notes (pitfalls feed):** darwin and windows binaries are unsigned and not notarized. Files fetched with `curl`/Homebrew/Scoop carry no quarantine flag and run; a browser-downloaded archive on macOS is quarantined by Gatekeeper (document `xattr -d com.apple.quarantine`); Windows SmartScreen may warn on browser downloads. Go's linker ad-hoc signs darwin/arm64 binaries, so arm64 macOS runs them. GHCR first-push packages may default to private: check visibility after the first push and flip it once in the UI (LOW-MEDIUM confidence on the default; verify).

### 2. `gruntled --version` for `go install ...@vX.Y.Z`

**No dependency. `runtime/debug` only (links no net).** Behaviour verified here on go1.27.2 with a scratch git repo and a real `go install github.com/GiulioSavini/gruntled/cmd/gruntled@v0.3.0`:

| Build | `Main.Version` | `vcs.revision` / `vcs.modified` settings |
|---|---|---|
| `go install mod@v0.3.0` (module cache) | `v0.3.0` | **absent** (`go version -m` shows `mod ... v0.3.0` and no vcs.* lines) |
| `go install mod@main` / `@<sha>` | pseudo-version `v0.0.0-<ts>-<sha12>` or `vX.Y.(Z+1)-0.<ts>-<sha12>` | absent |
| local `go build` in git checkout, HEAD = clean tag `v1.2.3` | `v1.2.3` | present, `modified=false` |
| same, with uncommitted/untracked changes (an untracked build output counts!) | `v1.2.3+dirty` | present, `modified=true` |
| N commits after tag | `v1.2.4-0.<ts>-<sha12>` (pre-release tag `v1.3.0-rc.1` gives `v1.3.0-rc.1.0.<ts>-<sha12>`) | present |
| no tag at all | `v0.0.0-<ts>-<sha12>` | present |
| `-buildvcs=false`, no `.git`, or `go run` | `(devel)` | absent |

**Recommendation (reverses the v0.2 "drop the fallback" decision without breaking its reason):** the ldflags value wins whenever it is not the default `dev`. Otherwise use `debug.ReadBuildInfo()` **only when `Main.Version` is a real module version (not empty, not `(devel)`) AND there is no `vcs.revision` setting.** That predicate is true exactly for `go install pkg@version` (module-cache build) and false for every local/contributor build, so the original v0.2 contract ("a local build prints `dev (none)`") is preserved while `go install ...@v0.5.0` prints `gruntled v0.5.0 (none)`. Keep the printed shape `gruntled <version> (<commit>)`; commit stays `none` for go-install users because the proxy build carries no commit. Do not parse or "clean" pseudo-versions; print them verbatim. Update the `docs/ci.md` sentence that says go-install prints `dev (none)`. The tests need a table over a fake `*debug.BuildInfo` (inject the reader as a function variable) because the real values depend on how the test binary was built. Confidence HIGH (experiment).

### 3. Shell completions and man pages without cobra

**No library. Hand-rolled generators from the real `flag.FlagSet`s, in pure Go, no `text/template`.**

- Source of truth: `cmd/gruntled` already builds one `flag.FlagSet` per subcommand inside `parseArgs`/`defineWatchFlags`. Introduce one table (command name, one-line summary, `define func(*flag.FlagSet)`), and derive from it: dispatch, `topUsage`, completions, the man page. Flags are enumerated with `fs.VisitAll` (name, usage, `DefValue`; bool flags via the `IsBoolFlag() bool` interface). Add only a small hand-written side table for what `flag` cannot express: value kind (`file`, `dir`, `enum` with the allowed words, `int`, `none`). Enums such as `--format text|json|sarif` are the main value of completion. A test fails if a flag exists with no kind metadata or a spec flag no longer exists, so completions cannot drift.
- Placement: the generators are pure string builders (`strings`, `fmt`, `io`, `sort`), so they fit `internal/interfaces/...` under the existing stdlib allowlist (fmt, io, encoding/json + domain allowlist) with no allowlist change. `cmd/gruntled` converts `FlagSet`s to a plain spec struct (the `flag` import stays in `cmd`).
- Surface: `gruntled completion bash|zsh|fish|powershell` and `gruntled man`, both print to stdout, exit 2 on an unknown shell. Static scripts only (completion never calls back into the binary, so nothing to exec at TAB time). Use each shell's builtin file/dir completion (`compgen -f`/`_filedir`, zsh `_files -/`, fish `-F`/`__fish_complete_directories`, PowerShell `Get-ChildItem`).
- Also pre-generate into the release archives with the freshly built host binary (output is target-independent) as `completions/gruntled.{bash,zsh,fish,ps1}` and `man/gruntled.1`, for distro packagers and non-Homebrew users. Changing the "no directory prefix" archive layout requires updating `build-release.sh`, `release_test.go`, `docs/ci.md` and the install script together.
- Man page: roff by hand with a small `esc()` (backslash -> `\e`, `-` -> `\-`, guard a leading `.` or `'` with `\&`). One `gruntled(1)` plus optionally `gruntled-<cmd>(1)`; sections NAME, SYNOPSIS, DESCRIPTION, COMMANDS, OPTIONS, EXIT STATUS, SEE ALSO. Golden test, plus `mandoc -T lint` in CI if available (optional). Roughly 100 lines; `muesli/roff` v0.1.0 / `muesli/mango` v0.2.0 are net-clean (verified) but unmaintained since 2022 and mango's value is its pflag/cobra adapters, so they add a dependency for nothing.
- CI syntax checks: `bash -n`, `zsh -n`, `fish --no-execute`, `pwsh -NoProfile -Command [scriptblock]::Create(...)` (ubuntu runners have bash and pwsh; `apt install zsh fish` is cheap).
- Confidence HIGH on the constraint analysis (verified links), MEDIUM on shell-script details (written in the phase, validated by the syntax checks above).

### 4. Terminal colour and TTY detection

**No new module. ~40 lines in `cmd/gruntled` (or `internal/infrastructure/tty`), per-OS files, verified to compile and stay net-free on all six release targets.**

- linux: `unix.IoctlGetTermios(fd, unix.TCGETS)`; darwin: `unix.TIOCGETA`; both via `golang.org/x/sys/unix`, already in the module graph (indirect today; `go mod tidy` will make it direct). Other unix: return false.
- windows: std `syscall.GetConsoleMode(handle, &mode)` is exported and enough for detection; to actually render ANSI on conhost call `kernel32!SetConsoleMode` with `ENABLE_VIRTUAL_TERMINAL_PROCESSING (0x0004)` through `syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")` (SetConsoleMode is not exported by `syscall`). On failure, disable colour, never error.
- Do NOT use `os.Stdout.Stat().Mode()&os.ModeCharDevice` alone: `/dev/null` is a char device and would count as a terminal.
- Policy (NO_COLOR spec, verified at no-color.org: "present and not an empty string, regardless of value"; user flags override the variable): colour on only if stdout is a TTY **and** `NO_COLOR` is empty/unset **and** `TERM != "dumb"` **and** no `--no-color`; `CLICOLOR_FORCE`/`FORCE_COLOR` non-empty (and not `0`) forces on unless `--no-color` (MEDIUM: de-facto conventions, not a standard). Colour only ever applies to `--format text`; JSON and SARIF stay byte-identical. Apply colour **after** the existing control/bidi escaping, on already-escaped strings, so a repository name can never inject an escape. Evaluate stdout and stderr separately.
- The presenter (`internal/interfaces/presenter`) must stay pure: pass it a `Style` value (a struct of SGR prefix/suffix strings, empty when colour is off), never an `*os.File` or env. The golden tests keep producing uncoloured output because test stdout is never a TTY.
- `go-isatty` v0.0.24 is the acceptable fallback if hand-rolling is rejected (verified net-free on all targets, one extra module); `golang.org/x/term` is not acceptable (net on windows).
- `--quiet` is flag plumbing, no dependency.

### 5. Windows named pipes without `net`

**No library. `syscall` (std) + `syscall.NewLazyDLL` for the four functions `syscall` does not export. Prototyped and run on a real Windows host through WSL interop (amd64 build; arm64 build also compiles), zero `net`/`os/exec` in `go list -deps`.**

What std `syscall` already has on windows: `CreateFile`, `ReadFile`, `WriteFile`, `CloseHandle`, `CancelIoEx`, `OpenProcessToken`, `Token.GetTokenUser`, `(*SID).String`, `GetCurrentProcess`, `SecurityAttributes`, `Overlapped`, `UTF16PtrFromString`. Missing and loaded lazily from `kernel32.dll`/`advapi32.dll`: `CreateNamedPipeW`, `ConnectNamedPipe`, `CreateEventW` (+ `WaitForSingleObject`, `GetOverlappedResult`), `WaitNamedPipeW`, `LocalFree`, `ConvertStringSecurityDescriptorToSecurityDescriptorW`.

Design that worked in the prototype (and should be the phase's starting point):
- **Name:** `\\.\pipe\gruntled-<hash of canonical repo path>`, same hash that derives the unix socket path.
- **Server:** `CreateNamedPipeW` with `PIPE_ACCESS_DUPLEX | FILE_FLAG_OVERLAPPED | FILE_FLAG_FIRST_PIPE_INSTANCE`, `PIPE_TYPE_BYTE | PIPE_READMODE_BYTE | PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS`. `FILE_FLAG_FIRST_PIPE_INSTANCE` makes a squatted pipe name fail instead of silently joining someone else's pipe (verified: a second first-instance create is rejected). Use it only on the first instance; later instances of the same pipe must omit it.
- **Security descriptor (required, not optional):** MS docs state the default DACL grants Everyone and the anonymous account read access. Build `"D:P(A;;GA;;;" + currentUserSID + ")"` where the SID string comes from `Token.GetTokenUser()` + `(*SID).String()` (do not use `OW`/owner-rights: an elevated process's default owner is the Administrators group), convert with `ConvertStringSecurityDescriptorToSecurityDescriptorW`, free with `LocalFree`. Protected DACL (`P`) with a single ACE for the current user only. This replaces today's "Windows runtime dir relies on `%LocalAppData%` ACLs" known limitation with an explicit control.
- **Client:** `syscall.CreateFile(name, GENERIC_READ|GENERIC_WRITE, 0, nil, OPEN_EXISTING, FILE_FLAG_OVERLAPPED | SECURITY_SQOS_PRESENT, 0)` (SQOS present with `SECURITY_ANONYMOUS`=0 stops a rogue server from impersonating the client); `ERROR_FILE_NOT_FOUND` means no daemon; `ERROR_PIPE_BUSY` (231) -> `WaitNamedPipeW` then retry with a bound.
- **Overlapped vs blocking: use overlapped.** Go 1.27's `os.NewFile` on windows detects an overlapped handle (`windows.IsNonblock`, go.dev/issue 75949/76391/80979) and registers it with its IOCP poller, so the resulting `*os.File` supports `SetReadDeadline`/`SetWriteDeadline` and interruptible `Close`. Verified: a read with a 300 ms deadline returned `i/o timeout` (`errors.Is(err, os.ErrDeadlineExceeded)`) after 300 ms. A blocking (non-overlapped) handle would give no deadlines, and the server's per-connection request deadline (`serverReadDeadline`) is a security property of the current unix server, so overlapped is the only choice that keeps parity.
- **Accept loop:** `ConnectNamedPipe` with an event-backed `OVERLAPPED`, wait in the accept goroutine, `GetOverlappedResult`; treat `ERROR_PIPE_CONNECTED` (535) as success; wake it on shutdown with `CancelIoEx(h, &ov)` (completes with `ERROR_OPERATION_ABORTED` 995). Wrap the handle with `os.NewFile` only after the connect completes. A named pipe serves one client per instance: create the next instance before handing the connected one off, so there is always one pending instance (clients that arrive in the gap see `ERROR_PIPE_BUSY` and retry).
- **Reuse:** keep the wire protocol, `Handler`, `Info`, request-line framing and the 64 MiB bound from `ipc.go` unchanged; only `Listen`/`Serve`/`Query`/`Rebase` plumbing is new in `sock_windows.go`. The build tag of `sock_other.go` becomes `!linux && !darwin && !windows`, and the daemon on windows should serve both the pipe and (for downgrade safety, one release) the existing report file. The lock stays the share-mode-0 file (`lock_windows.go`), so single-instance and "a dead daemon leaves nothing to clean up" already hold; the pipe disappears with the process.
- **Liveness idea for the status-file feature:** reuse `ipc.Probe(lockPath)` (lock held == daemon alive; immune to PID reuse, works on every OS, already exists) rather than adding PID probing.
- **Proof impact (do these in the same phase):**
  1. Keep half 4 (windows must not link `golang.org/x/sys/windows`); it is now doubly justified. Update its message to say it also keeps `net` out.
  2. The residual-gap note "a DLL procedure looked up by name at run time" becomes live because we now call `NewLazyDLL(...).NewProc("...")`. Add a source rule (via `scripts/archscan` or a Go test) that every `NewProc` literal in `internal/` belongs to a small allowlist (`CreateNamedPipeW`, `ConnectNamedPipe`, `CreateEventW`, `WaitForSingleObject`, `GetOverlappedResult`, `WaitNamedPipeW`, `LocalFree`, `ConvertStringSecurityDescriptorToSecurityDescriptorW`, `SetConsoleMode`) and that no DLL other than kernel32/advapi32 is loaded; add self-test cases next to `binary-net-in-windows-ipc-file`. Add a non-vacuous guard so the rule cannot pass by matching nothing.
  3. The `go vet`/staticcheck "NewLazyDLL searches the application directory" concern does not apply to `kernel32.dll`/`advapi32.dll` (KnownDLLs), but pin the names as constants and add a comment so a reviewer sees it is deliberate.
  4. Add the pipe to the windows CI job (it already runs tests on windows): a test that listens, queries, hits the deadline, and checks a second `FILE_FLAG_FIRST_PIPE_INSTANCE` listener is refused. Add a negative test that a different-user connection is denied if a second account is available (otherwise assert the SDDL string).
- Confidence: HIGH (built, ran, measured). Not exercised: multi-user denial, session-0/service contexts, Windows Server containers.

### 6. Dependency automation and community health

| Item | Choice | Why |
|---|---|---|
| Update bot | **Dependabot** (`.github/dependabot.yml`, `version: 2`) | Native to GitHub, no app install, understands SHA pins with a trailing `# vX.Y.Z` comment (updates both the SHA and the comment, which is the format already used in `ci.yml`/`release.yml`). Renovate is stronger only if you want it to bump `go run pkg@vX` pins and doc pins via regex managers; not needed to meet the milestone. |
| Ecosystems | `gomod` at `/` and `github-actions` at `/` | Dependabot scans `.github/workflows` and root `action.yml` automatically for `github-actions`. |
| Cadence | `weekly` | Low noise for a solo maintainer. |
| Supply-chain delay | `cooldown: default-days: 7` for both (gomod also accepts `semver-major-days`/`semver-minor-days`/`semver-patch-days`; github-actions only `default-days`; security updates bypass cooldown) | A project that sells "trust" should not adopt a release minutes after it is published. |
| Grouping | gomod: one group for `minor`+`patch`; actions: one group `patterns: ["*"]` | Cuts PR count. `dependency-type` grouping does not apply to Go. |
| Guard rail | Treat `hashicorp/hcl`, `zclconf/go-cty`, `fsnotify` bumps as "needs the corpus gate before merge" (document in CONTRIBUTING; optionally keep them out of the group). | A parser bump can break the zero-false-positive guarantee, which unit tests alone do not prove. |
| Known gap | Dependabot cannot bump `go run honnef.co/go/tools/cmd/staticcheck@v0.8.1`, `govulncheck@v1.8.0`, `jv@v0.7.0` in `ci.yml`, nor the tag pins in `docs/ci.md`. Options: (a) move the tools to `go.mod` `tool` directives (Go 1.24+; Dependabot then sees them as ordinary requires; cost: tool deps enter the single module's `go.sum`, which is allowed by the single-module rule), or (b) a small `scripts/check-pins.sh` run in CI that fails when a pin is older than the latest release. (a) is simpler; decide in the phase. | |
| Workflow linting (add) | `actionlint` v1.7.12 and `zizmor` v1.31.1 in CI | v0.5 adds workflows with `packages: write`, PATs and OIDC; static analysis of those workflows is cheap insurance. Both are standalone tools run in CI only. |
| Community files | `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1), `CHANGELOG.md`, `.github/CODEOWNERS` (`* @GiulioSavini`), `.github/ISSUE_TEMPLATE/{bug,feature}.yml` issue forms + `config.yml` with `blank_issues_enabled: false`, `.github/PULL_REQUEST_TEMPLATE.md`, `.github/release.yml` (generated-notes categories) | All plain files, no tooling. Issue forms (`.yml`) beat markdown templates: required fields (gruntled version, OS, minimal repro) reduce triage. For a solo maintainer CODEOWNERS is documentary unless rulesets require code-owner review. |
| SECURITY.md | Enable GitHub **private vulnerability reporting** (repo setting) and link it; state supported versions (latest minor), response targets, and the "no network, no exec" threat model as scope | Disclosure policy with a working private channel. |
| CHANGELOG | **Keep a Changelog 1.1.0** + SemVer: `## [Unreleased]` on top, groups Added/Changed/Deprecated/Removed/Fixed/Security, link references at the bottom, backfill v0.1-v0.4 from the milestone archives. `release.yml` extracts the section for the tag (awk) into `--notes-file`, replacing `--generate-notes`, and a guard fails the release if the section is missing or empty. | Hand-written changelog is the deliverable; the release page then cannot disagree with it. Do not generate it from commit messages (this repo's `docs(...)` commits are noise). |
| Repo settings to enable (manual checklist) | Immutable releases (GA 2025-10-28; verify with `gh release verify`), tag ruleset for `v*`, private vulnerability reporting, secret scanning + push protection | Immutability makes the published `checksums.txt` trustworthy after the fact. Check that the single `gh release create ... files` still works with it (GitHub documents draft -> upload -> publish for immutable releases; community reports that adding assets after publication breaks attestation), and switch to explicit `--draft` then `gh release edit --draft=false` if needed. |

## Alternatives considered

| Category | Recommended | Alternative | Why not |
|---|---|---|---|
| TTY/colour | Hand-rolled `x/sys/unix` + `syscall` | `golang.org/x/term` | Links `net` on windows via x/sys/windows |
| TTY/colour | Hand-rolled | `mattn/go-isatty` v0.0.24 | Works and is net-free, but one more module for ~30 lines; keep as fallback |
| TTY/colour | Hand-rolled SGR strings | fatih/color, termenv, lipgloss, mgutz/ansi | net / os/exec / template (table above) |
| Windows pipe | `syscall` + LazyDLL | `x/sys/windows` (`CreateNamedPipe`, `SecurityDescriptor` helpers) | Links `net`; ergonomic but forbidden |
| Windows pipe | `syscall` + LazyDLL | go-winio, npipe | Return `net.Listener`; link `net` |
| Windows pipe | Overlapped + `os.NewFile` | Blocking handle | No deadlines; loses the server's slow-client defence |
| Completions | Hand-rolled from `flag.FlagSet` | cobra, urfave/cli, kong, posener/complete | net / os/exec / template |
| Completions generator | `strings.Builder` | `text/template` | Links `net/url` + `net/netip` |
| Man pages | Hand-rolled roff | `muesli/mango`+`roff`, `go-md2man`, `help2man` | Unmaintained / extra markdown pipeline / executes the binary at build time |
| Version | `ReadBuildInfo` gated on "no `vcs.revision`" | ldflags only (status quo) | `go install @tag` prints `dev (none)` |
| Version | gated `ReadBuildInfo` | ungated `ReadBuildInfo` | Local builds print pseudo-versions; that is what v0.2 removed |
| Release tooling | `build-release.sh` + custom render scripts | goreleaser (brews, scoops, dockers, nfpms) | Locked decision "no goreleaser"; one packaging path |
| Brew update | Custom render + push | `brew bump-formula-pr` actions | Single-URL assumption |
| Image | `FROM scratch` + prebuilt binary | distroless / alpine / `ko` | A static binary needs nothing else; copying the proven release binary keeps the no-net/no-exec proof single-sourced |
| Image provenance | `actions/attest` (GitHub attestations) | cosign keyless | Equivalent strength; actions/attest needs no extra installer and `gh attestation verify` is the user-side tool. Add cosign only if a consumer asks for `cosign verify` |
| Image SBOM | none | BuildKit `sbom: true` | Adds `unknown/unknown` manifests in the GHCR UI; `go version -m` already lists the module set |
| Bot | Dependabot | Renovate | Needs hosted app/config; only advantage is regex managers for pins |

## What NOT to add (checklist for planners and reviewers)

- Any import of `golang.org/x/sys/windows`, `golang.org/x/term`, `go-colorable`, `fatih/color`, `mgutz/ansi`, `termenv`, `lipgloss`, `go-winio`, `npipe`: they link `net` on windows (verified).
- `text/template`, `html/template`, anywhere in the shipped binary (completions, man, help, version output): link `net/url` and `net/netip`.
- cobra, pflag, urfave/cli, kong, posener/complete, any "completion framework".
- `os/exec` for anything, including "convenience" calls to `git`, `gh`, `man`, `tput`, `stty`, or opening a browser. TTY width/colour support come from syscalls and environment variables only.
- A `RUN` step or a base image other than `scratch` in the Dockerfile.
- goreleaser, ko, nfpm, or any second packaging path.
- Auto-update or "new version available" checks in the binary: `--version` must stay a pure local read (project rule: no network at runtime).
- `docker/setup-qemu-action`: a `COPY`-only Dockerfile needs no emulation, and QEMU adds attack surface to the release job.
- `pull_request_target` or long-lived broad PATs in `release.yml`; the tap token must be scoped to the tap repo only.
- Unpinned actions: every new `uses:` must be a full 40-character SHA with a `# vX.Y.Z` comment, like the current ones.

## Installation (what gets added; nothing here changes `go.mod` except `x/sys` becoming direct)

```bash
# Go side: no new modules.
go mod tidy                      # golang.org/x/sys moves from indirect to direct (unix tty ioctl)
bash scripts/check-architecture.sh   # must still print "architecture: OK" on all six targets

# Dev/CI tools (not in the binary)
go install github.com/rhysd/actionlint/cmd/actionlint@v1.7.12
# zizmor v1.31.1: use the project's published release binary or its GitHub Action, pinned by SHA
sudo apt-get install -y shellcheck zsh fish     # ubuntu CI runner, for install.sh and completion syntax checks
```

New repository files (all plain text): `install.sh`, `Dockerfile`, `.dockerignore`, `packaging/homebrew/gruntled.rb.in`, `packaging/scoop/gruntled.json.in`, `scripts/render-formula.sh`, `scripts/test-install.sh`, `.github/dependabot.yml`, `.github/CODEOWNERS`, `.github/ISSUE_TEMPLATE/*.yml`, `.github/PULL_REQUEST_TEMPLATE.md`, `.github/release.yml`, `CONTRIBUTING.md`, `SECURITY.md`, `CODE_OF_CONDUCT.md`, `CHANGELOG.md`. New repos outside this one: `GiulioSavini/homebrew-tap`, `GiulioSavini/scoop-bucket`.

## Proof-script impact summary (`scripts/check-architecture.sh`)

| Change | Needed? |
|---|---|
| Half 1 import deny-list | Unchanged; already forbids `net/.+`, which is what catches `text/template` and `x/sys/windows` |
| Half 4 (windows: no fsnotify, no x/sys/windows) | Keep; reword to say it also keeps `net` out |
| Half 2/3 spawner scan and `go tool nm` | Unchanged; `NewLazyDLL`/`NewProc` do not appear in `sym_re` |
| New: allowlist of `NewProc` names and DLLs under `internal/` and `cmd/` | **Add** (closes the "DLL procedure looked up by name" residual gap that the pipe code makes live) |
| `release_targets` | Unchanged (`release_test.go` keeps the two lists equal) |
| Archive contents (completions, man) | Update `build-release.sh` file count in `release.yml` smoke step (currently hard-codes 7 files = 6 archives + checksums.txt; adding `install.sh` as a release asset makes it 8) |
| Interfaces allowlist | Unchanged if generators use only `strings`, `fmt`, `io`, `sort` |

## Confidence

| Area | Level | Basis |
|---|---|---|
| net/exec link analysis of candidate libraries | HIGH | `go list -deps` executed for linux and windows on each candidate |
| `x/sys/windows` imports `net` in all versions | HIGH | grep over seven module versions |
| ReadBuildInfo semantics (Go 1.27.2) | HIGH | experiments on tag / dirty / post-tag / no-vcs / `go install @v0.3.0` |
| Windows named pipe design (net-free, overlapped, deadlines, first-instance) | HIGH | built and executed on Windows via WSL interop; arm64 cross-compiled only |
| TTY detection per OS | HIGH for compile + net-free on six targets, MEDIUM for behaviour on a real Windows console and darwin (not run) |
| NO_COLOR rule | HIGH (no-color.org); CLICOLOR_FORCE/FORCE_COLOR MEDIUM |
| Homebrew tap layout and `on_*` blocks | MEDIUM (docs.brew.sh; formula not built); `generate_completions_from_executable` argument convention LOW-MEDIUM, verify |
| GitHub Actions versions and SHAs | HIGH on 2026-10-16 (resolved via `gh api`); re-resolve at implementation time |
| Dependabot cooldown/groups/SHA-comment behaviour | MEDIUM-HIGH (official options reference + multiple project PRs) |
| Scoop autoupdate / winget zip-portable | LOW-MEDIUM (search results and docs, unexercised); hence deferred/optional |
| GHCR default package visibility, immutable-release interaction with `gh release create` | LOW-MEDIUM; both are explicit verification items for the phase |

## Sources

- Local experiments (this session, scratchpad): `go list -deps` matrices; `x/sys/windows/types_windows.go` imports `net` (module cache v0.47.0, v0.49.0 and five older versions); `debug.ReadBuildInfo` matrix on go1.27.2; named-pipe prototype executed as a Windows binary; `$GOROOT/src/os/file_windows.go` (`newFile` overlapped detection via `windows.IsNonblock`), `$GOROOT/src/syscall` export inventory.
- [Microsoft Learn: CreateNamedPipe](https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-createnamedpipea) (FILE_FLAG_FIRST_PIPE_INSTANCE, PIPE_REJECT_REMOTE_CLIENTS, default DACL) - HIGH
- [Microsoft Learn: Named Pipe Security and Access Rights](https://learn.microsoft.com/en-us/windows/win32/ipc/named-pipe-security-and-access-rights) (default DACL grants Everyone read) - HIGH
- [no-color.org](https://no-color.org/) - HIGH
- [Homebrew: How to Create and Maintain a Tap](https://docs.brew.sh/How-to-Create-and-Maintain-a-Tap) and [Formula Cookbook](https://docs.brew.sh/Formula-Cookbook) - HIGH for layout, MEDIUM for binary-formula idioms
- [Homebrew Formula API (generate_completions_from_executable signature)](https://docs.brew.sh/rubydoc/Formula.html) - MEDIUM
- [actions/attest README](https://github.com/actions/attest) and [actions/attest-build-provenance README](https://github.com/actions/attest-build-provenance) (wrapper note, `subject-checksums`, container usage) - HIGH
- [Dependabot options reference](https://docs.github.com/en/code-security/dependabot/working-with-dependabot/dependabot-options-reference) (cooldown, groups) - HIGH
- [GitHub changelog: Immutable releases GA](https://github.blog/changelog/2025-10-28-immutable-releases-are-now-generally-available/) and [verifying release integrity](https://docs.github.com/en/code-security/how-tos/secure-your-supply-chain/secure-your-dependencies/verifying-the-integrity-of-a-release) - HIGH
- [winget manifest docs via winget-types](https://docs.rs/winget-types/0.5.0/winget_types/struct.InstallerManifest.html) (zip needs winget 1.5, portable 1.3) - LOW-MEDIUM
- GitHub release API (`gh api repos/<owner>/<repo>/releases/latest`) for action versions and SHAs, 2026-10-16 - HIGH
