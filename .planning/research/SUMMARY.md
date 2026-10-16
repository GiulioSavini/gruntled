# Project Research Summary

**Project:** gruntled | **Milestone:** v0.5 Adoption & Completeness | **Researched:** 2026-10-16
**Domain:** Distribution, CLI usability and daemon completeness for a deterministic, read-only, no-network, no-exec Go static analyzer (Terragrunt)
**Confidence:** HIGH on constraints and Go-side design. The link analysis, build-info behaviour and Windows pipe were built and run. MEDIUM on third-party services (Homebrew, GHCR, Scoop, winget, Dependabot), which were read from docs but not exercised.

## Executive Summary

v0.5 is an adoption milestone, not an analysis milestone. Peers (tflint, trivy, golangci-lint) ship an install script, brew, a container image, a working `--version`, colour control and documented exit codes. gruntled has none of these beyond the release archives. The milestone also closes four documented gaps:
- no SARIF for `blast`
- no liveness for the status file
- no `watch --base`
- no live `report`/`rebase` on Windows

**The Go side needs zero new runtime modules.** It uses stdlib, `golang.org/x/sys/unix` (already linked via fsnotify) and std `syscall`. The risk sits in release/distribution plumbing and in two import traps that would silently void the `binary-no-net-no-exec` proof.

The approach:
1. Build one command/flag registry. Derive usage, completions and the man page from it.
2. Keep every new Go import free of `net` and `os/exec`.
3. Give every release step a CI dry run. Tagging stays Giulio's decision, so the first real run of post-publish jobs would otherwise be the real, possibly immutable, release.

## Hard Rules (state once, apply to every phase)
1. **No import that links `net` or `os/exec`.** Forbidden:
   - `golang.org/x/sys/windows`: it imports `net` in every released version, v0.1.0 to v0.49.0, verified.
   - `golang.org/x/term`: it pulls in x/sys/windows.
   - `text/template` and `html/template`: they link `net/url` and `net/netip`.
   - go-winio, npipe, go-colorable, fatih/color, mgutz/ansi, termenv, lipgloss.
   - cobra, pflag, urfave/cli, kong, posener/complete.
2. Generators (completions, man, formula text) use `strings.Builder`/`fmt`. Add self-test case `binary-text-template` to `scripts/test-check-architecture.sh`.
3. Windows syscalls use std `syscall` plus `syscall.NewLazyDLL` (kernel32/advapi32 only). Add a `NewProc`/DLL allowlist rule to `scripts/check-architecture.sh`. Step 9 half 4 stays, reworded to say it also keeps `net` out.
4. Default `check` output stays byte-identical. Colour, version, completions and blast SARIF must not move any `check` byte.

## Cross-file conflicts, resolved

| Conflict | Resolution |
|---|---|
| Windows pipe | **PITFALLS' and FEATURES' `x/sys/windows` suggestion is superseded.** Hand-roll it over std `syscall` plus `NewLazyDLL`. STACK's prototype ran on a real Windows host (amd64 executed, arm64 compiled only). It uses an overlapped handle plus `os.NewFile`, which gives real deadlines and interruptible close. It also needs `FILE_FLAG_FIRST_PIPE_INSTANCE`, `PIPE_REJECT_REMOTE_CLIENTS` and a protected user-only DACL (`D:P(A;;GA;;;<userSID>)`) from the first implementation. ARCHITECTURE's "spike first" is downgraded to "re-verify in Windows CI". The file-mailbox fallback is dropped. |
| DLL allowlist | Union of STACK and ARCHITECTURE: procs CreateNamedPipeW, ConnectNamedPipe, DisconnectNamedPipe, CreateEventW, WaitForSingleObject, GetOverlappedResult, WaitNamedPipeW, LocalFree, ConvertStringSecurityDescriptorToSecurityDescriptorW, SetConsoleMode; DLLs kernel32 and advapi32 only; reject CreateProcess/ShellExecute/WinExec substrings; non-vacuous guard. |
| TTY detection | STACK wins: x/sys/unix ioctl (TCGETS on linux, TIOCGETA on darwin) and `syscall.GetConsoleMode` on Windows. ARCHITECTURE's `ModeCharDevice` is rejected because `/dev/null` counts as a TTY. Windows VT enable goes through `SetConsoleMode` via LazyDLL, and colour is off on failure. |
| Colour | Edge-only, after escaping, text format only, never in the daemon. Property test `stripSGR(Colorize(x)) == x`. |
| Version | ldflags wins unless it is `dev`. Otherwise accept build info only if `Main.Version` is a clean tag: not `(devel)`, no `+dirty`, not a pseudo-version, **and no `vcs.revision` setting**. That is true exactly for `go install pkg@vX.Y.Z`. Commit prints `none`. Inject the reader for table tests. |
| Completions and man | Runtime subcommands `gruntled completion <shell>` and `gruntled man` are canonical. Do **not** change the archive layout, because `TestBuildRelease` pins it. Homebrew generates at install time. |
| Blast SARIF | Blast gets its own rule table that appends GRT004 at index 4. The shared 4-rule table is untouched so `check` bytes are unchanged. Add a regression test that `check` SARIF has 4 rules. Impacted units go in `toolExecutionNotifications`, never `results`. `report --blast --format sarif` is out of scope. The `--base` path never appears in the document. `partialFingerprints` is a separate optional requirement. |
| Liveness | `gruntled status [path]` pings the socket (pipe on Windows later). It never polls `ipc.Probe`. Raise the unix LockRetry from 1 to 3. No pid file and no status-line grammar change. Heartbeat (`Chtimes` only, >=10s) is optional and decided later. |
| Homebrew update | Pull model: `render-formula.sh` output is attached as a release asset and the tap self-updates. No cross-repo token. If push is preferred, use a fine-grained tap-only token on a protected environment. Tap repo name (`homebrew-tap` vs `homebrew-gruntled`) is an open decision for Giulio. |
| Image | `FROM scratch`, COPY the released binary per arch (no RUN, no QEMU), numeric non-root USER. Use `provenance: false` and `sbom: false` plus a separate attestation. Prefer the plain docker buildx CLI. `watch` and `rebase` are unsupported in the image. Docs say to mount the repo root read-only. |
| Installer | `install.sh` is a release asset, never documented from `main`. POSIX sh with `main()` wrapping, no sudo, atomic mv. State plainly that a same-origin checksum proves integrity, not authenticity. Prefer embedded hashes. The attestation check is optional. |

## Key Findings

**Stack.** No new runtime modules. `go mod tidy` only promotes x/sys from indirect to direct.
- stdlib `flag`, `runtime/debug`, `strings`, `fmt`
- `x/sys/unix` for the TTY ioctl
- std `syscall` plus LazyDLL for the pipe and console mode
- POSIX sh installer
- kept `build-release.sh` (no goreleaser) plus `render-formula.sh`
- GHCR scratch image
- `actions/attest`
- Dependabot (weekly, cooldown 7 days, grouped)
- actionlint and zizmor in CI

**Features.**
- Must have: T1 version, T2 install.sh, T3 GHCR, T4 doc pins, T5 brew, T6 Scoop, T7 SECURITY, T8 CONTRIBUTING, T9 CHANGELOG, T10 templates (including a false-positive form), T11 Dependabot, T12 CODEOWNERS, T13 colour, T14 `--quiet`, T15 help registry, T16 blast SARIF, T17 liveness, T18 quickstart/FAQ.
- Should have: D1 completions, D4 `watch --base`, D5 Windows pipe, D6 recipes, D3 doctor, D7 attestations, D8 release checklist, D9 comparison.
- Defer: suppression/config file, update checks and self-update, daemonising, LSP, goreleaser, docs site, Chocolatey/apt/Nix, Windows containers, Impacted as SARIF results.
- D2 man page is cut first if scope is tight. winget is submit-only or deferred.

**Architecture.** The hexagonal layout is unchanged. Nearly all new work is in the composition root, presenters, CI and scripts, so it is proof-neutral. The exception is `ipc/pipe_windows.go`:
- It implements the exact `sock_other.go` surface.
- The v2 wire protocol and 64 MiB bound are unchanged.
- The `sock_other.go` build tag becomes `!linux && !darwin && !windows`.
- The Windows dump file is kept for one more release so old clients do not wait forever.

New components: `commands.go` registry; `presenter/blast_sarif.go` plus `blasting.BlastDetailed`; `status.go` plus a shared runtime-dir helper; `watchPublisher.seedBase`; `pipe_windows.go`; `termenv.go` plus `presenter.Colorize`; `install.sh`, `render-formula.sh`, Dockerfile, `VERSION` and the release jobs.

**Top pitfalls.**
1. Forbidden imports silently void the proof. Run `check-architecture.sh` on each new import and add self-tests.
2. A default pipe DACL grants Everyone read, and pipe names are machine-global. Use an explicit user-only SDDL, first-instance flag, reject-remote, byte mode, overlapped IO with deadlines, and a conformance table on both transports.
3. Probing the lock for liveness makes a starting daemon exit 0 "already watching". Ping the socket, retry the lock, keep readers read-only, and leave the status grammar alone.
4. `--version` and colour leak into the deterministic bytes. v0.4.0 is untagged, so `@latest` is a pseudo-version today. Colour is applied only after escaping.
5. Release automation that has never run, combined with immutable releases, makes a mistake permanent. Every release step gets a dry-run CI job. Use draft, attach, then publish, and rehearse credentials in a sandbox repo. Keep a manual-settings checklist covering: tap repo, GHCR visibility, secrets and their expiry, immutable releases, private vulnerability reporting.
6. Also: blast SARIF noise and rule-table drift; completion/man drift without a registry; installer truncation and checksum trust; `TestCIDoc` hard-coded SHAs vs Dependabot (derive pins from `ci.yml`); stale doc pins (single `VERSION` plus test).

## Implications for Roadmap: recommended phase order (continues from 18)

- **Phase 18: Truth Foundations.** Every channel cites the version and docs, so this goes first. Delivers `resolvedVersion()` used by `--version`, SARIF ToolInfo and `ipc.Info`; `VERSION` file plus a doc-pin test; `ci_doc_test` pins derived from `ci.yml`; CHANGELOG backfilled v0.1-v0.4; docs corrected. Covers T1, T4, T9.
- **Phase 19: CLI Surface.** The registry is a pure refactor and goes first; colour, `--quiet` and blast SARIF consume its format tables. Delivers `commands.go`; `--color`/`--no-color`/`--quiet`, per-OS TTY detection, edge `Colorize`; `BlastDetailed`, `BlastSARIF`, GRT004 descriptor, schema CI step for blast. Covers T13, T14, T15, T16.
- **Phase 20: Daemon Completeness.** Shares the `instance.go`/`report.go`/`rebase.go` hot spots, so it runs before the pipe. Delivers `gruntled status`; unix lock retry with a race test; a real `kill -9` test; `watch --base` with an overlap guard, `seedBase`, a third baseline label, and a byte-equality test against `blast --base`; limitation text removed. Covers T17, D4, D6 recipes.
- **Phase 21: Windows Named Pipe.** The only phase that edits the proof, so it is isolated; needs `status` from Phase 20. The proof rule and self-tests land in the first plan, before any pipe code. Delivers the `windows-dll-allowlist` rule plus self-tests; `pipe_windows.go`; capability-based replacement of the 3 `goos=="windows"` guards behind a test seam; a transport conformance table; a Windows CI job; the dump kept for one more release. Covers D5.
- **Phase 22: Completions and Man.** Last among the CLI items so goldens are written once against the final flag set. Delivers `completion bash|zsh|fish|powershell` and `man`; goldens plus a `VisitAll` drift test; shell syntax smoke checks; a behavioural bash test. Covers D1, D2.
- **Phase 23: Release Pipeline and Install Channels.** Consumes version, completions and man; everything gets a dry run. Delivers `install.sh` as a release asset plus an `install-script` CI job; Dockerfile, `container-check` and an `image` job; Homebrew template, `formula-check` and pull-model tap; Scoop manifest; attestations; draft, attach, publish ordering; CHANGELOG-driven notes; a `verify-published` job; extras staged so `dist/` stays 7 files; a sandbox rehearsal. Covers T2, T3, T5, T6, D7.
- **Phase 24: Hygiene and Adoption Docs.** Dependabot goes after Phase 23 so the new actions fall within its scope. Delivers `dependabot.yml`; actionlint/zizmor; a pin-lint test; CONTRIBUTING, SECURITY, CODE_OF_CONDUCT, CODEOWNERS; issue forms and a PR template; quickstart, FAQ, troubleshooting, comparison; recipes with executed matcher tests; `doctor`. Covers T7, T8, T10, T11, T12, T18, D6, D9, D3.
- **Phase 25: Proof and Release Readiness.** The independent cross-phase audit, which found real blockers in earlier milestones. Delivers `corpus-gate.sh`, `compare-ref.sh` and `compat-v1.sh` on v0.5 HEAD; `check-architecture.sh` on all 6 targets; docs verified against the binary; a real SARIF upload rehearsal; `RELEASING.md` plus `release-check.sh`; the manual-settings checklist. Covers D8. Tagging stays Giulio's decision.

**Ordering notes.** Phases 19 and 20 are independent after 18, but 20 must not overlap 21. Blast SARIF and the pipe stay in separate phases. If the pipe slips, Phases 22 to 25 can proceed. Plain community files may be written earlier in parallel.

**Research flags.** Needs research-phase: Phase 21 (multi-user denial, session-0, arm64 execution, allowlist implementation); Phase 23 (immutable-release behaviour with `gh release create`, GHCR default visibility, Homebrew completions convention, Scoop autoupdate, pull vs push for the tap); Phase 20 only for the heartbeat. Standard: 18, 19, 22, 24, 25.

## Confidence

| Area | Confidence | Notes |
|---|---|---|
| Stack | HIGH | `go list -deps` matrices, build-info matrix, pipe run on Windows. Service behaviour is MEDIUM. |
| Features | MEDIUM | Repo constraints are HIGH. Peer behaviour is MEDIUM-LOW. winget and aqua are LOW. |
| Architecture | HIGH | Read at HEAD `ca4f07f` and probed. arm64 and Homebrew/GHCR mechanics are MEDIUM. |
| Pitfalls | MEDIUM-HIGH | Own-repo contracts are HIGH. Platform facts are MEDIUM and several are marked "verify in rehearsal". |

**Gaps to address.** Tap repo name and pull vs push (Giulio); heartbeat in scope or not; `partialFingerprints`; winget handling; whether a `windows-11-arm` runner exists; GHCR visibility, immutable-release asset ordering and the Homebrew completions convention (sandbox); Dependabot cannot bump `go run tool@vX` pins (choose `go.mod` `tool` directives or `check-pins.sh`); re-resolve action SHAs at implementation time; the `blast --base` inside-repo hazard exists today, document and do not change.

## Sources
- Primary: the repo at HEAD `ca4f07f`; local go1.27.2 experiments; Microsoft Learn (named pipes); no-color.org; actions/attest; the Dependabot reference; the GitHub immutable releases GA notice.
- Secondary: Homebrew docs; GitHub code scanning SARIF docs; GHCR docs; Go issues 75949 and 76391.
- Tertiary: winget zipped-portable shape; Scoop autoupdate; the aqua registry; the `windows-11-arm` runner; shell quirks (Rosetta, busybox, zsh compinit).

*Research completed: 2026-10-16 | Ready for roadmap: yes*
