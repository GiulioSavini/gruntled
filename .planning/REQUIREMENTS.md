# Requirements: gruntled v0.5 — Adoption & Completeness

**Defined:** 2026-10-16
**Core Value:** Tell the user, before they run anything slow, that `dependency.X.outputs.Y` does not exist in the module it points to.
**Milestone goal:** Make gruntled a complete, 360-degree project that anyone can install, run, trust and contribute to on linux, macOS and windows — without weakening the zero-false-positive, deterministic, read-only, no-net/no-exec guarantees.

Research: `.planning/research/SUMMARY.md` (STACK, FEATURES, ARCHITECTURE, PITFALLS). Feature ids (T1..T18, D1..D11) refer to `research/FEATURES.md`.

## Hard rules for every requirement below

- The binary must keep the six-target `binary-no-net-no-exec` proof green: no `net`, no `os/exec`, no `golang.org/x/sys/windows`, no `golang.org/x/term`, no `text/template`/`html/template`, no cobra/pflag (all link `net`). Windows syscalls go through std `syscall` + `NewLazyDLL` under an allowlist rule.
- `gruntled check` (text, json, sarif) stays byte-identical to v0.4 for non-TTY output; `check --format sarif` keeps its 4-rule table.
- Read-only on the analysed repository; nothing written inside it.
- Any doc that states a command, a version or an output is pinned by a test (precedent: `TestCIDoc`, `TestTopUsageMatchesDocs`, `recipe-check`).
- No workflow creates a git tag or a release; v0.5.0 is Giulio's decision.

## v1 Requirements

### Installation & Version Truth (INST)

- [ ] **INST-01**: `gruntled --version` prints the real tag for a `go install github.com/GiulioSavini/gruntled/cmd/gruntled@vX.Y.Z` build (ldflags win; `debug.ReadBuildInfo` `Main.Version` accepted only when it is a clean release semver of this module with no `vcs.revision`; `(devel)`, `+dirty` and pseudo-versions keep `dev (none)`); the same resolved string feeds SARIF `tool.driver.version` and `ipc.Info`; the commit field for `go install` builds has a documented placeholder. (T1)
- [ ] **INST-02**: One version source of truth (`VERSION` file or latest CHANGELOG entry) from which `docs/ci.md`, README install snippets, the pre-commit `rev`, install-script and image/formula snippets are checked by a doc-pin test that fails on drift; `docs/ci.md` no longer pins v0.2.0. A release guard checks `VERSION` equals the tag. (T4)
- [ ] **INST-03**: A POSIX `install.sh` (everything inside `main()`, curl or wget, `sha256sum` or `shasum -a 256`, OS/arch detection incl. darwin arm64 and Rosetta, installs to `~/.local/bin` or `$BINDIR` by atomic rename, explicit version or `releases/latest` redirect, no sudo, `GRUNTLED_BASE_URL` override) shipped as a release asset with that release's SHA-256s embedded; fails closed on mismatch or truncation; tested in CI under dash and busybox against a local file server with truncation and tamper negatives; shellcheck-clean. (T2)
- [ ] **INST-04**: A `FROM scratch` container image on GHCR (`ghcr.io/giuliosavini/gruntled:vX.Y.Z` and `latest`, linux amd64+arm64 manifest, non-root `USER`, `ENTRYPOINT ["/gruntled"]`) built in a separate `packages: write` job of `release.yml` from the released linux archives (no recompile), proven in CI by a dry run and by a `--read-only --network none` run whose `check` bytes equal the host binary's; docs show how to mount the repository root and pin by digest. (T3)
- [ ] **INST-05**: A Homebrew formula (`gruntled.rb`: 4 URLs + 4 sha256 from `checksums.txt`, `test do` runs `--version`, completions via `generate_completions_from_executable`) rendered by `scripts/render-formula.sh` and attached as a release asset; `release.yml` updates the tap repository only when Giulio has created it and provided a tap-scoped token (skipped otherwise, never failing the release); a Scoop manifest is rendered by the same script. winget deferred. (T5, T6)
- [ ] **INST-06**: Release archives and `checksums.txt` carry GitHub artifact attestations (OIDC, `actions/attest` with `subject-checksums`), documented with a `gh attestation verify` recipe; a CI check proves two builds of the same commit are byte-identical. (D7)
- [ ] **INST-07**: A post-publish `verify-published` job (and its dry-run counterpart on every push) executes the documented install paths: `install.sh`, `go install @tag`, the image pull, and asserts `--version` prints the tag. (T1, T2, T3)

### Project Hygiene (HYG)

- [ ] **HYG-01**: `SECURITY.md` with supported versions (latest minor), private vulnerability reporting as the only channel (no email, no SLA), and the threat-model scope (what is in: terminal-escape injection, path escape from the analysed tree, socket/pipe permissions, parser resource exhaustion, response size bound; what is out: findings gruntled chooses not to report). (T7)
- [ ] **HYG-02**: `CONTRIBUTING.md` and `CODE_OF_CONDUCT.md` stating the non-negotiables (no `net`/`os/exec`/cobra, layering, zero-false-positive bar for new diagnostics, determinism, tests first, how to run `check-architecture.sh` and the corpus gate), Apache-2.0 inbound=outbound, no CLA. (T8)
- [ ] **HYG-03**: `CHANGELOG.md` in Keep a Changelog 1.1.0 format, backfilled v0.1.0..v0.4.0 from `MILESTONES.md`, with an `Unreleased` section; `release.yml` uses the matching section as release notes (`--notes-file`) and fails if the section is missing. (T9)
- [ ] **HYG-04**: Issue forms (bug, **false-positive report** asking for the minimal tree or `graph --json`, `--version`, OS, and stating false positives are release blockers; feature request), `config.yml` with `blank_issues_enabled: false` and a security contact link, and a PR template checklist (tests, determinism, no deps that link `net`, docs updated). (T10)
- [ ] **HYG-05**: `.github/dependabot.yml` for `gomod` and `github-actions` (weekly, grouped, cooldown, `go` directive bumps excluded), shipped together with a `TestCIDoc` that derives action pins from the workflow files instead of hard-coding them, a lint that every `uses:` is a 40-hex SHA with a version comment, and `actionlint` + `zizmor` in CI; `hcl/v2`, `go-cty` and `fsnotify` bumps are documented as corpus-gate-required. (T11)
- [ ] **HYG-06**: `CODEOWNERS` with the single owner, and `docs/RELEASING.md` listing the repository settings that cannot be automated (branch protection without required code-owner review, private vulnerability reporting, immutable releases, GHCR package visibility, tap token expiry). (T12, D8)

### CLI Usability (UX)

- [ ] **UX-01**: One command/flag registry (`cmd/gruntled/commands.go`) drives `topUsage`, every per-command usage, enumerated flag values and their validation; `gruntled help [cmd]` and `-h/--help` exit 0 on stdout for every command; a test walks every `flag.FlagSet` and fails if a flag is missing from usage or `docs/cli.md`. Landed first as a pure refactor guarded by `TestTopUsageMatchesDocs`. (T15)
- [ ] **UX-02**: `--color=auto|always|never` and `--no-color` on text output only, precedence `--no-color` > `NO_COLOR` (non-empty) > `CLICOLOR_FORCE` > `TERM=dumb` > per-stream TTY (ioctl on unix, `GetConsoleMode` on windows with VT enabled only if `SetConsoleMode` succeeds; never `ModeCharDevice` alone); colour applied at the output edge after `escapeTerm`, with the invariant `stripSGR(Colorize(x)) == x` property-tested; default `auto` leaves every golden byte-identical; json, sarif, status file and daemon-rendered `report` text never carry colour. (T13)
- [ ] **UX-03**: `--quiet`/`-q` suppresses only the stderr summary and informational watch chatter; never a diagnostic, an error or an exit code (tested). (T14)
- [ ] **UX-04**: `gruntled completion bash|zsh|fish|powershell` prints a static completion script generated from the registry (subcommands, flags, `--format` values, directories; no dynamic callbacks), with a golden test, a `VisitAll` drift test, a bash `COMPREPLY` behavioural test and shell smoke tests in CI; bash 3.2-compatible, zsh dual-mode guard, PowerShell pure ASCII. (D1)
- [ ] **UX-05**: `gruntled man` prints `gruntled.1` roff generated from the registry (fixed `.TH` date, `\-` escapes), with a freshness test; pre-generated into the unix release archives only if `TestBuildRelease`, the `release.yml` member count and `docs/ci.md` are updated together. (D2)
- [ ] **UX-06**: `gruntled blast --format sarif`: Broken findings as results (GRT001..GRT004, GRT100) from a separate blast rule table that appends GRT004 at index 4 (the `check` table keeps length 4, regression-tested), Impacted carried as deterministic `toolExecutionNotifications`/run `properties` and never as results, no `--base` path in the document, URIs relative to the analysed tree, accepted by the existing SARIF schema and upload proofs; exit codes unchanged; the "GRT004 has no SARIF rule" limitation removed from docs. `report --blast --format sarif` deferred. (T16)
- [ ] **UX-07**: `gruntled doctor [path]` and `gruntled version`: read-only explanation of silence and environment (unit count, unknown units with reasons, conservative include-rule skips, runtime dir and status path, daemon liveness via the status probe, WSL `/mnt/c` hint, GOOS/GOARCH, module and vcs info); documented as outside the byte-determinism promise; never checks for updates, runs anything or writes. (D3)

### Daemon Completeness (DAEMON)

- [ ] **DAEMON-11**: A dead or SIGKILLed daemon is never mistaken for a live one: `gruntled status [path]` prints the status line or `gruntled: not running` (exit 0 live, 1 not running, 3 error) using an IPC ping (socket; later pipe) with the instance lock as fallback and never `ipc.Probe` on a polled path; the daemon retries `TryLock` briefly before concluding "already watching" (the start-vs-probe race is reproduced by a test and closed); graceful shutdown replaces the line with a stopped marker; a `kill -9` test proves the detection; the status-line grammar is unchanged. Optional mtime heartbeat via `Chtimes` only. (T17)
- [ ] **DAEMON-12**: `gruntled watch --base <dir>` seeds the daemon's blast baseline from another tree using the `blast --base` loader pair, keeps only the resulting `checking.Report`, reads `<dir>` once and never watches or writes it, labels the baseline distinctly, rejects a base inside the watched tree (usage error) and a missing/unreadable one (exit 3); `rebase` still moves the baseline; no protocol bump; the dirty-start limitation removed from docs. (D4)
- [ ] **DAEMON-13**: Live `report`, `report --blast` and `rebase` on windows over a named pipe implemented with std `syscall` + `NewLazyDLL` (overlapped I/O with `os.NewFile` deadlines, `FILE_FLAG_FIRST_PIPE_INSTANCE`, `PIPE_REJECT_REMOTE_CLIENTS`, explicit owner-only SDDL DACL, byte mode, unpredictable per-user name, client `SECURITY_SQOS_PRESENT`), behind the existing `ipc` package API with the unix framing, protocol v2 mismatch error and 64 MiB bound shared through one conformance table run on both transports; `check-architecture.sh` gains a `windows-dll-allowlist` rule with non-vacuous self-tests landed before any pipe code; the report-file dump is kept for one more release; a Windows CI job exercises a real daemon. A spike precedes the plan; if the spike fails, the fallback is a runtime-dir file mailbox with no proof change, and the requirement is re-scoped by decision, not silently dropped. (D5, research flag)
- [ ] **DAEMON-14**: Editor and shell integration recipes for the status file and the text format (tmux `#(gruntled status "#{pane_current_path}")`, starship, bash/zsh/fish/PowerShell prompts, vim/neovim statusline, VS Code `tasks.json` `problemMatcher`, vim `errorformat`), each pattern tested in Go against real `check` and `status` output. (D6)

### Documentation for Adoption (DOCS)

- [ ] **DOCS-01**: A 5-minute quickstart on a checked-in broken fixture whose printed output is asserted by a test, an FAQ (why silent, why no suppressions, why no network, relation to `terragrunt hcl validate`, stacks, `mock_outputs`, exit codes, Windows/WSL, Gatekeeper/SmartScreen on unsigned binaries, NFS runtime dir), and a symptom-driven troubleshooting page linking the known limitations. (T18)
- [ ] **DOCS-02**: A date-stamped comparison page (gruntled vs `terragrunt hcl validate`, `run-all plan`, Terramate, tflint/`tofu validate`) positioned as complement, reusing only measured numbers from `docs/validation.md`, every external claim sourced. (D9)
- [ ] **DOCS-03**: Every "Known limitations" bullet made false by this milestone (GRT004 SARIF, status liveness, windows `report`/`rebase`, dirty-start baseline, `go install` version) is deleted in the plan that closes it; README Status and `docs/cli.md` describe v0.5 accurately, pinned by the existing doc tests.

### Proof & Release Readiness (REL)

- [ ] **REL-04**: The corpus gate (`corpus-gate.sh`, network-isolated, against the frozen v0.3.0 reference), `compare-ref.sh` and `compat-v1.sh` pass on v0.5 HEAD with `check` byte-identical and zero reformat noise; results recorded in `docs/validation.md`.
- [ ] **REL-05**: An independent cross-phase security and integration audit (sec agent) across phases 18-23 closes with zero open BLOCKER/HIGH/MEDIUM findings, with the six-target architecture proof (including the new DLL allowlist rule) green.
- [ ] **REL-06**: `scripts/release-check.sh` preflight (CHANGELOG section, VERSION equals intended tag, doc pins consistent, README status, archives + checksums + image + formula render, completions/man fresh, no tag created) and `docs/RELEASING.md` for v0.5.0; tagging remains Giulio's decision.

## v2 Requirements

Deferred. Tracked, not in the current roadmap.

- **MORE-04** `GRT005` (`inputs` key matching no `variable`) and **MORE-05** `GRT006` (`variable` without default that no unit sets): overlap `terragrunt hcl validate --inputs`, include-merge false-positive risk
- `report --blast --format sarif`; SARIF `partialFingerprints`
- winget manifest (externally moderated); aqua/mise registry submission; Chocolatey
- OpenSSF Scorecard workflow and badge; OpenSSF Best Practices badge
- Code signing / notarisation for darwin and windows binaries
- Per-hop `file:line` in Impacted paths; reference-gated propagation
- Status-file heartbeat if not taken in DAEMON-11

## Out of Scope

| Feature | Reason |
|---------|--------|
| Suppression comments or an ignore file | A wrong finding is a release-blocking bug, not something to silence; the false-positive issue form is the channel |
| cobra/pflag, urfave/cli, kong, `x/term`, `go-winio`, `fatih/color`, `text/template` | Every one links `net` on at least one target and breaks the architecture proof |
| Update checks, telemetry, crash reporting | Network access at runtime is out of scope since v0.1 |
| Homebrew core submission | Notability thresholds and source builds requiring Go 1.27; the tap covers users |
| LSP server | The status file, problem matchers and errorformat give editor feedback without a protocol |
| Git-based baseline for `watch --base` | External process or git object reader; `--base <dir>` with `git worktree` covers it |
| Running inside the image with `watch` | `FROM scratch` has no writable runtime dir; the image is for `check`/`blast` in CI |
| Dependabot bumping the `go` directive | Forces toolchain upgrades on users; reviewed by hand |

## Traceability

Which phases cover which requirements. Filled by the roadmapper.

| Requirement | Phase | Status |
|-------------|-------|--------|
| INST-01 | — | Pending |
| INST-02 | — | Pending |
| INST-03 | — | Pending |
| INST-04 | — | Pending |
| INST-05 | — | Pending |
| INST-06 | — | Pending |
| INST-07 | — | Pending |
| HYG-01 | — | Pending |
| HYG-02 | — | Pending |
| HYG-03 | — | Pending |
| HYG-04 | — | Pending |
| HYG-05 | — | Pending |
| HYG-06 | — | Pending |
| UX-01 | — | Pending |
| UX-02 | — | Pending |
| UX-03 | — | Pending |
| UX-04 | — | Pending |
| UX-05 | — | Pending |
| UX-06 | — | Pending |
| UX-07 | — | Pending |
| DAEMON-11 | — | Pending |
| DAEMON-12 | — | Pending |
| DAEMON-13 | — | Pending |
| DAEMON-14 | — | Pending |
| DOCS-01 | — | Pending |
| DOCS-02 | — | Pending |
| DOCS-03 | — | Pending |
| REL-04 | — | Pending |
| REL-05 | — | Pending |
| REL-06 | — | Pending |

**Coverage:**
- v1 requirements: 30 total
- Mapped to phases: 0
- Unmapped: 30 ⚠️ (filled by roadmap)

---
*Requirements defined: 2026-10-16*
*Last updated: 2026-10-16 after initial definition*
