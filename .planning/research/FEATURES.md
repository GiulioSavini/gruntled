# Feature Research

**Domain:** Adoption and completeness of a Go static-analysis CLI + local daemon (gruntled v0.5)
**Researched:** 2026-10-16
**Confidence:** MEDIUM overall (HIGH on gruntled-internal constraints and dependencies, read from the repo; MEDIUM on what peer tools ship, from prior knowledge plus partial web verification; LOW where flagged inline)

Scope: only the NEW v0.5 features. Existing features (check, graph, blast, watch, report, rebase, SARIF for `check`, pre-commit, CI recipes, 6-target releases) are context, not research targets.

## How peers solve this (calibration)

| Tool | Install channels | Container | Signing | Completions / man | Colour flags | Config / suppress |
|------|------------------|-----------|---------|-------------------|--------------|-------------------|
| tflint | install script, brew, choco (community), releases | `ghcr.io/terraform-linters/tflint` | cosign keyless on `checksums.txt` (verified from docs) | none known | `--color` / `--no-color` | `.tflint.hcl`, `tflint-ignore` comments |
| trivy | install script, brew, apt/rpm, releases | ghcr + docker hub | cosign + attestations | cobra completion | `--no-color`-style | `.trivyignore`, config |
| golangci-lint | install script (pinned version arg), brew, releases | ghcr/docker hub | checksums + cosign | cobra completion | `--color auto/always/never` | `.golangci.yml`, `//nolint` |
| shellcheck | distro pkgs, brew, scoop/choco, releases | docker | none | man page | `--color=auto/always/never` | `# shellcheck disable=` |
| hadolint | brew, scoop, releases | ghcr | none | none | `--no-color` | `.hadolint.yaml`, ignore comments |
| staticcheck | `go install`, brew, releases | none | none | none | none | `staticcheck.conf`, `//lint:ignore` |

Reading: the universal floor is (1) a checksummed release binary, (2) a one-line install for the main OS families, (3) a container image for CI, (4) `--version` that works however you installed, (5) a way to turn colour off, (6) documented exit codes. Completions and man pages are only universal among cobra-based tools (they come free), so for a stdlib-`flag` tool they are polish, not a floor. Every mature linter also ships a suppression mechanism, which gruntled deliberately does not (see Anti-Features).

Sources: tflint install and verification docs (cosign `verify-blob` on `checksums.txt`, GHCR image `ghcr.io/terraform-linters/tflint`) via web search, MEDIUM. Other rows from prior knowledge, MEDIUM-LOW; the requirements writer should not quote peer behaviour as fact without re-checking.

## Constraint filter (applied to every feature below)

Every candidate was checked against: **N** no network at runtime in the binary, **X** no external process from the binary, **R** read-only on the analysed repo, **D** deterministic output / zero false positives, **S** `binary-no-net-no-exec` proof (cobra/pflag/`net`/`go-winio` are forbidden dependencies). Scripts, workflows and docs are not the binary: an install script may use `curl`, a workflow may call `gh`. The guarantees apply to what the `gruntled` executable does.

## Feature Landscape

### Table Stakes (Users Expect These)

| # | Feature | Why Expected | Complexity | Constraint check / notes |
|---|---------|--------------|------------|--------------------------|
| T1 | `--version` correct for `go install pkg@vX.Y.Z` (prefer ldflags; else `debug.ReadBuildInfo` `Main.Version` accepted only if a clean semver tag, rejecting `(devel)`, `+dirty` and pseudo-versions) | Every Go CLI shows its version under `go install`; `dev (none)` looks broken and makes bug reports unusable | S | N/X/R fine, stdlib only. Reverses the v0.2 decision ("ReadBuildInfo fallback dropped") for the one safe case. Strict regexp: pseudo-versions contain a 14-digit timestamp and 12-hex commit. `go install @tag` carries no `vcs.*` settings, so the commit field has no source: the output contract (`gruntled <tag> (<commit>)`) needs a defined placeholder. Depends on `TestCIDoc`/`release_test` (docs say `dev (none)` today, must change). |
| T2 | POSIX `install.sh` (download archive + `checksums.txt`, verify sha256, install to `~/.local/bin` or `$BINDIR`, version arg, no silent sudo) | tflint, trivy, golangci-lint all ship one; it is the dominant "just install it" path in CI | S-M | Script not binary, so network use is fine, but it must fail closed on checksum mismatch, support Linux and macOS and both arches, work with `curl` or `wget`, `sha256sum` or `shasum -a 256`. Must be shellcheck-clean and executed in CI (extend `recipe-check` precedent). Do not default to "latest" via the GitHub API (rate limits); resolve via the `releases/latest` redirect or require an explicit version. Depends on `build-release.sh` asset naming (`gruntled_<tag>_<os>_<arch>`), which must stay frozen. |
| T3 | Container image on GHCR, built by `release.yml` | Needed for GitLab/Jenkins/Tekton users with no Go; tflint, trivy, hadolint ship one | M | `FROM scratch`, copy the already-built linux static binary, non-root `USER`, `ENTRYPOINT ["/gruntled"]`, multi-arch (amd64, arm64). Tags: `vX.Y.Z` plus `latest` only; docs recommend digest pinning. Build from the same binary that is in the release archive (no second compile) so the image and archive are provably the same bytes. `needs: contents/packages: write` scoped to that job only. Depends on `release.yml` and `build-release.sh`. Linux containers only. |
| T4 | CI recipes and docs pin the version from ONE source of truth, with a test that fails on drift | v0.2 docs pin `v0.2.0` while v0.4 is current: "docs verified against the binary" is the milestone's trust claim | S-M | Pure repo tooling. Extend `TestCIDoc` so `docs/ci.md`, README install snippets and the pre-commit `rev` derive from or are checked against one file (e.g. latest CHANGELOG entry or a `VERSION` file). Also covers the new install.sh, container and brew snippets. |
| T5 | Homebrew tap (`homebrew-gruntled`) with a formula generated from the release `checksums.txt` | `brew install` is the macOS default; tflint/trivy/hadolint/shellcheck all in brew | M | Own tap, not homebrew-core (core has notability thresholds, MEDIUM confidence, and builds from source). Formula installs the prebuilt archive for darwin and linux x amd64/arm64 (4 URLs + 4 sha256), `test do` runs `--version`. A generator script (no goreleaser, per decision) renders the formula; the release workflow opens a commit/PR in the tap repo. Needs a second repo and a token secret: decision for Giulio, scope token to the tap repo only. Pitfall: formula must not trigger a source build (needs Go 1.27). |
| T6 | Windows package manager coverage: Scoop manifest (with `checkver`/`autoupdate`) and a winget manifest | Windows is a first-class target in the milestone; zip-only distribution is a barrier | M | Scoop: S, single JSON in an own bucket, autoupdate keyed on the archive naming. winget: PR to `microsoft/winget-pkgs` (zip with nested portable exe; the exact manifest shape for a zipped portable was NOT confirmed by search, LOW, verify with `winget validate` before promising it); can be automated with `wingetcreate` in the release workflow but is externally gated by moderator review, so treat as "submit", not "ship". Skip Chocolatey (moderation queue, community-maintained in tflint's case). |
| T7 | `SECURITY.md` + GitHub private vulnerability reporting enabled | Standard community-health file; security-minded Terraform shops check it before adopting a tool that reads their repos | S | Supported versions: latest minor only. Include a short threat model scope (what counts: terminal-escape injection through repo content, symlink/path escape from the analysed tree, daemon socket/pipe permissions, parser denial of service, response size bound; what does not count: findings gruntled does not report). Use advisories, not an email address. |
| T8 | `CONTRIBUTING.md` (+ Code of Conduct file) | GitHub "community standards" checklist; contributors need the build/test/arch commands and the unusual rules | S | Must state the non-negotiables: no cobra/pflag/`net`/`os/exec` (architecture job will fail), domain/application layering, zero-false-positive bar for any new diagnostic, determinism, tests-first for analyzers, how to run `scripts/check-architecture.sh` and the corpus gate. Licence is Apache-2.0 with inbound=outbound; no CLA or DCO bot. |
| T9 | `CHANGELOG.md` (Keep a Changelog format), backfilled v0.1 to v0.4, release notes taken from it | Users need "what changed" without reading commits; also a precondition for T4 and the release checklist | S | Hand-maintained, not generated (no conventional-commits mandate). Backfill from `.planning/MILESTONES.md`. `release.yml` should publish the matching section as the release body instead of ad hoc notes. |
| T10 | Issue templates (bug, **false-positive report**, feature) + `config.yml` + PR template | Reduces back-and-forth; a false-positive report is the project's most important bug class | S | The false-positive form asks for the minimal tree (or `gruntled graph --json` output), `--version`, OS, and states that FPs are treated as release blockers. `blank_issues_enabled: false`, contact link to private security reporting. PR template checklist: tests, determinism, no new deps that link `net`, docs updated. |
| T11 | `.github/dependabot.yml` for `gomod` and `github-actions` (weekly, grouped) | Expected hygiene; existing SHA pins in workflows rot without it | S | Dependabot updates a SHA pin and its trailing version comment together (web, MEDIUM). The safety net already exists: the architecture job fails any bump that links `net`/`exec`. `gomod` PRs must not bump the `go` directive silently (it forces user toolchain upgrades): ignore the `go` line or review by hand. It cannot update `docs/ci.md` (see T4). |
| T12 | `CODEOWNERS` | Auto-requests review, appears in community checks | S | Single owner. Low value alone; it only matters once branch protection requires review (a repository setting, outside the repo, document in the release checklist). |
| T13 | A way to turn colour off, and colour only on a real TTY: `--color=auto\|always\|never` plus `--no-color` alias, honour `NO_COLOR`, `TERM=dumb`; `CLICOLOR_FORCE` optional | shellcheck/golangci-lint/tflint/hadolint all have it; CI logs and pipes must stay clean | S-M | Colour only in the **text** presenter on stdout (summary on stderr judged by stderr's TTY-ness); never in json, sarif, status file or `report` output when piped. Colour escapes must be added AFTER the existing control/bidi escaping, never derived from data. Default `auto` leaves all golden and byte-identical tests untouched; add `always` goldens. TTY detection: do NOT use `ModeCharDevice` alone (`/dev/null` is a char device); use `x/term`-style ioctl on unix. `golang.org/x/sys` is already an indirect dep; verify the `binary-no-net-no-exec` proof stays green. Windows needs VT processing enabled via `x/sys/windows` (or colour stays off on legacy conhost). The Windows part is the M. |
| T14 | `--quiet` / `-q` | Expected in CI; trivy/ruff have it | S | Define narrowly: suppress the stderr summary line and informational watch chatter. **Never** suppress diagnostics or errors, never change exit codes (a quiet mode that hides findings is a silent pass). Reject `--quiet` with `--verbose`-like ambiguity by not adding `--verbose`. |
| T15 | Consistent help: `gruntled help [cmd]`, `-h/--help` exits 0 on stdout for every command, every flag appears in usage and in `docs/cli.md` | Users discover tools through `--help`; inconsistent help is the first sign of an unfinished tool | S-M | Today each command defines flags inside closures and usage strings are hand-written. Introduce one flag/command registry that feeds usage text, completions and man page (see D1, D2), plus a doc test comparing it to `docs/cli.md`. This refactor is the real cost and the dependency for D1/D2. |
| T16 | `blast --format sarif` (and `report --blast --format sarif`, currently a usage error), with a `GRT004` rule descriptor | Closes the documented gap "GRT004 has no SARIF rule"; PR workflows want blast results in code scanning | M | Emit **Broken** findings as results (GRT001..GRT004, GRT100) with correct levels; do **not** emit Impacted as code-scanning alerts (location-less or file-level notes would be noise and contradict "reporting twelve units destroys trust"). Carry Impacted as deterministic `run.properties`/`invocations.properties` so tooling can read it. Rule descriptors sorted; reuse the existing SARIF builder so schema/upload proofs extend to blast. Exit codes unchanged (blast exits 1 only on Broken errors). Same bytes from daemon and CLI (existing `Between` seam). Consider adding `partialFingerprints` (hash of code + unit + file + message, no line) in the same change so base-vs-head alerts do not churn when a line shifts; currently a known limitation. Depends on the shared blast core, presenter/sarif, DAEMON-07..10. |
| T17 | Status-file liveness: a dead or SIGKILLed daemon is never mistaken for a live one | Known limitation today: stale line "looks current"; a wrong green in the prompt is worse than none | M | Authoritative probe is "is a daemon answering for this repository" (IPC ping on unix socket, later named pipe) with the instance lock as fallback; expose as `gruntled status [path]` printing the one line, or `gruntled: not running`, exit 0 live / 1 not running / 3 error. Optional for file-only consumers (tmux `cat`): daemon refreshes the file mtime on a heartbeat so consumers can age-check. PITFALL: a probe that takes the instance lock can race a starting daemon, making it print "already watching" and exit 0 falsely; probe via socket/pipe, or retry. Graceful shutdown should replace the line with a stopped marker. Depends on DAEMON-03, DAEMON-05, IPC v2. N/X/R fine (status file is outside the repo). |
| T18 | Quickstart + FAQ + troubleshooting docs, executed or checked by tests like the CI recipes | Adoption requires a 5-minute path; "why did it say nothing" is the dominant support question for a zero-FP tool | M | Quickstart uses a checked-in broken fixture and asserts the printed output in a test (precedent: `recipe-check`, `TestCIDoc`, `readme_doc_test`). FAQ must answer: why does it stay silent (unknown units, conservative include rule), why no suppressions, why no network, how it relates to `terragrunt hcl validate`, stacks, `mock_outputs`, exit codes, Windows/WSL. Troubleshooting is symptom-driven and links the known limitations in `docs/cli.md`. |

### Differentiators (Competitive Advantage)

| # | Feature | Value Proposition | Complexity | Constraint check / notes |
|---|---------|-------------------|------------|--------------------------|
| D1 | Shell completions for bash, zsh, fish, PowerShell via `gruntled completion <shell>` (script on stdout) and shipped in release archives | Cobra tools get this free; a stdlib-`flag` tool that ships it feels finished, and PowerShell covers the Windows promise | M | Pure string generation from the T15 registry: no exec, no net. Static completion only (subcommands, flags, `--format` values, directories); path completion delegates to the shell. Do NOT implement dynamic callbacks that re-invoke the binary. Drift guard: golden-file test plus a test that walks every `flag.FlagSet` (`VisitAll`) and fails if a flag is missing from the generated scripts. Homebrew formula installs them (`generate_completions_from_executable`). Depends on T15. |
| D2 | Man page `gruntled.1` (roff), generated from the same registry, shipped in Unix archives and brew | shellcheck-style polish; distro packagers expect it | M | Small internal generator, committed output plus a freshness test. Unix archives only. Lowest priority of the usability set: cut first if scope tight. Depends on T15. |
| D3 | `gruntled doctor [path]` (and a `gruntled version` subcommand with build info) | Turns the zero-FP tool's silence into an explanation: coverage, unknown units and reasons, runtime/status dir, daemon liveness, WSL `/mnt/c` hint, platform. The most useful support tool for "why no findings" | M | Strictly read-only diagnostics over the existing graph (`graph --json` already exposes unknowns) and the T17 probe. Output contains environment paths so it is explicitly outside the byte-determinism promise (document it, do not golden-test absolute paths). Must not check for updates (N), run `terragrunt --version` (X), or "fix" anything (R). `version` subcommand: Go version, GOOS/GOARCH, module version, vcs info when present; `--version` one-liner stays byte-stable. |
| D4 | `watch --base <dir>`: seed the daemon's blast baseline from another tree (e.g. a `git worktree` of main) | Fixes the documented dirty-start baseline problem and survives restarts; the daemon becomes usable for "what did my uncommitted edits break" from the first save | M | Index `<dir>` once with the existing loader, keep only the resulting `checking.Report` as baseline (honours the v0.4 decision: baseline holds only `Report`, never a loader). `<dir>` is read once, never watched, never written. Validation: missing/unreadable base is exit 3 (as `blast --base`), base equal to or inside the watched tree is a usage error. `rebase` still moves the baseline to the last result. Peer analogue: `golangci-lint --new-from-rev`. Depends on DAEMON-07..10, `blast --base` loading path, single-instance lock. |
| D5 | Live `report` / `report --blast` / `rebase` on Windows over a named pipe, without `net` | Closes the biggest platform parity gap; today `rebase` is simply unavailable on Windows | L | Cannot use `go-winio` (its pipe listener implements `net.Listener`, so it links `net` and breaks the proof). Hand-roll on `golang.org/x/sys/windows` (`CreateNamedPipe`, `ConnectNamedPipe`, overlapped I/O). SECURITY is the hard part: default named-pipe DACL grants read to Everyone, so set an explicit security descriptor limited to the current user SID and `PIPE_REJECT_REMOTE_CLIENTS`; reuse the IPC v2 framing, the 64 MiB bound and protocol-mismatch behaviour. Needs the Windows CI job to exercise a real daemon. Keep the report-file fallback until proven. Research flag. Also the foundation for a uniform liveness ping (T17). |
| D6 | Editor/shell integration recipes for the status file: tmux, starship, bash/zsh/fish/PowerShell prompt, vim/neovim statusline, VS Code, plus **problem matchers**: VS Code `tasks.json` `problemMatcher` and vim/neovim `errorformat`/`makeprg` for `gruntled check` | Gives editor-level feedback (Problems panel, quickfix) with no LSP and no extension, using the text format `file:line:col: GRT001 msg` | S-M | Docs only, but test the claims: a Go test that runs the documented regex/errorformat pattern against real `check` output (docs that execute cannot drift). tmux: `#(gruntled status "#{pane_current_path}")` (session cwd is not the pane's). Neovim: read the file on a timer or fs-event. Status line contains a non-ASCII `×`; note it. Depends on `--print-status-path`, T17. No colour in status output. |
| D7 | Artifact provenance: GitHub artifact attestations (or cosign keyless) on release archives and the image, documented `gh attestation verify` recipe | tflint/trivy sign; security-conscious Terraform shops ask for it. Strengthens the "trust" half of the milestone goal | S-M | Release-workflow only; uses OIDC, no long-lived key. Binary unaffected. Skip a separate SBOM: `go version -m gruntled` already lists every module in a static Go binary. `-trimpath` is already used; add a CI check that two builds of the same tag are byte-identical (fits the determinism ethos). |
| D8 | Executable release checklist: `docs/RELEASING.md` + `scripts/release-check.sh` preflight (CHANGELOG entry exists, docs pins consistent, README status matches, archives + checksums + image + formula render, completions/man fresh) | The milestone ends in a v0.5.0 checklist; making it a script keeps it honest. Tagging stays Giulio's decision | M | Pure repo tooling. Must not create a tag. Repository settings that cannot be automated (branch protection, private vulnerability reporting, Discussions) go in a manual section. |
| D9 | Comparison page: gruntled vs `terragrunt hcl validate`, `run-all plan`, Terramate, tflint/`tofu validate` | Answers the first adoption question ("don't I already have this?") honestly | S (writing) / M (verification) | Facts about other tools go stale: date-stamp every claim, link sources, avoid benchmark tables (reuse the measured numbers already in `docs/validation.md`). Competitor facts here are LOW until re-verified by the author. Position: complement, not replacement (Terramate reports what changed, tflint/validate check inside a module, validate treats `dependency.X.outputs.Y` as opaque since #5811). |
| D10 | Package-manager reach via convention: keep asset names aqua/ubi-friendly and submit to the aqua registry (used by mise) | Terragrunt users overwhelmingly install tools through mise/asdf/tenv; zero binary change | S (externally gated) | No code beyond not renaming assets. Treat as opportunistic. MEDIUM-LOW confidence on registry process; verify. |
| D11 | OpenSSF Scorecard workflow + badge | Cheap public trust signal once T7, T11 and SHA pinning exist | S | Workflow only; runs in CI with network, not in the binary. Defer the OpenSSF Best Practices badge (paperwork). |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Cobra/pflag (or any dependency linking `net`) to get completions, man pages and help for free | Every peer does it | Breaks the one-line `binary-no-net-no-exec` proof, the core trust claim | Registry-driven generators (T15, D1, D2) on stdlib `flag` |
| `go-winio` for the Windows named pipe | Easiest named-pipe library | Links `net`; fails the six-target proof | Hand-rolled `x/sys/windows` pipe (D5) |
| Update check / "new version available" notice, telemetry | Common in CLIs (trivy, brew-style notices) | Network at runtime is out of scope by construction | CHANGELOG, Dependabot-style notifications on GitHub releases, `doctor` prints the version only |
| `doctor`/`install` that shells out (`terragrunt --version`, `tofu`, `brew`) or self-updates (`gruntled upgrade`) | Convenience | Violates no-exec and no-net; self-replacing binaries bypass checksum verification | `install.sh` with explicit version and checksum verification |
| Suppression mechanism: config file with ignore lists, inline `# gruntled:ignore`, baseline files | Every peer linter has one; users will ask on day one | GRT001-003 are exact structural checks; a suppressor invites silencing real breakage, and diagnostics are already silent when uncertain. Also the config file adds a second input to determinism | Answer in the FAQ: fix the reference, or the report is a false positive and a release-blocking bug (T10 template). Revisit only if real demand appears; out of this milestone |
| `--quiet` that hides diagnostics, or `--warn-only` / exit-0 mode | "Don't fail my pipeline" | A silent pass is the worst failure of a checker; `docs/ci.md` already states there is no warn-only mode | `gruntled check ... \|\| true` in the user's own pipeline, explicit and visible |
| Emitting `Impacted` units as SARIF results / code-scanning alerts | Completeness of `blast --format sarif` | Impacted is a lower-bound hint, not a defect; alerts would be noisy and un-dismissable, contradicting the zero-noise philosophy | Impacted in SARIF `properties` only (T16) |
| Desktop notifications (`notify-send`, toasts) | Obvious daemon UX | Needs an external process; already rejected in key decisions | Status file + recipes (D6) |
| Daemonising / `gruntled watch --daemon`, `launchd`/`systemd` units generated by the binary | Users want it to run in the background | Fork/exec breaks no-exec; decision "watch never forks" | Document a systemd user unit, a launchd plist and a Windows Task Scheduler snippet as copy-paste docs (inside D6) |
| LSP server / VS Code extension | "Real" editor integration | Out of scope by decision; large maintenance surface | Problem matchers and errorformat (D6) |
| goreleaser, release-please, semantic-release | Standard release automation | goreleaser rejected by decision (single packaging path); auto-tagging contradicts "tagging is Giulio's decision" | `build-release.sh` + generator scripts + release checklist (D8) |
| Documentation website (mkdocs/Docusaurus), asciinema/GIF demos | Looks professional | A second toolchain and link-rot surface; GIFs are unreproducible binaries. Markdown in the repo renders on GitHub and is already test-checked | Keep `docs/*.md`, add a `docs/README.md` index; revisit with real traffic |
| Chocolatey, apt/rpm, AUR, Nix, snap in this milestone | Breadth | Each is a separate moderated or maintained channel; small audience before brew/scoop/winget/container are proven | Defer; do not rename assets so third parties can package it (D10) |
| Bots: stale-bot, CLA assistant, all-contributors | Maintainer convenience | A stale bot would auto-close false-positive reports, the most valuable issue type; CLA adds friction under Apache-2.0 inbound=outbound | Manual triage, labels via templates |
| Windows container images, Windows install via `.msi` | Windows parity | Tiny audience, signing cost, no bearing on a static CLI | Scoop + winget + zip |

## Feature Dependencies

```
T15 flag/command registry ──> D1 completions
                          ├─> D2 man page
                          └─> T15 doc test vs docs/cli.md

T1 correct --version ──> D3 `version` subcommand
                    └─> T4 never-stale docs (go install snippets)

release.yml + build-release.sh asset naming (frozen)
    ├─> T2 install.sh
    ├─> T3 GHCR image ──> D7 attestations
    ├─> T5 Homebrew tap (needs checksums.txt) ──> installs D1/D2 artifacts
    ├─> T6 Scoop / winget
    └─> D10 aqua/ubi reach

T9 CHANGELOG ──> T4 single version source ──> D8 release checklist
T7 SECURITY + T11 Dependabot ──> D11 Scorecard

Instance lock (DAEMON-05) + IPC v2 + status file (DAEMON-03)
    ├─> T17 liveness ──> D6 recipes (`gruntled status` in tmux/starship) ──> D3 doctor
    ├─> D4 watch --base (also needs blast --base loading + DAEMON-07..10)
    └─> D5 Windows named pipe ──> uniform liveness ping on Windows (T17), live rebase

Shared blast core `Between` + presenter/sarif ──> T16 blast SARIF (and report --blast SARIF)
T13 colour ──> must run after control/bidi escaping; text presenter only
```

Conflicts / ordering notes:
- T16 and D5 both touch the report/blast wire and presenters; keep them in separate phases or the daemon-vs-CLI byte-identity tests become hard to attribute.
- T17 before D6: recipes should document `gruntled status`, not the raw file, so liveness must exist first.
- D5's named pipe is the only item that can plausibly slip; T17 on Windows can fall back to the instance lock until it lands.
- T4 (single version source) should land with or before T2/T3/T5/T6, otherwise each new install channel adds another pin that can go stale.

## MVP Recommendation

Prioritise (the v0.5 milestone is adoption; these are the floor):
1. T1, T4, T9: truthful version and docs, CHANGELOG as the version source. Cheap, and everything else cites them.
2. T2, T3, T5, T6: install channels (script, GHCR, brew, scoop/winget). T2 and T3 first; T5/T6 are externally gated and can trail.
3. T7, T8, T10, T11, T12: hygiene bundle, all S, one phase.
4. T13, T14, T15, T16: CLI usability floor, with T15's registry done before D1/D2.
5. T17, D4: daemon completeness that fixes two documented limitations without new platform risk.
6. T18 plus D6 (recipes with executed matcher tests) and D9 (comparison): documentation for adoption.
7. One differentiator with the best value per cost: **D3 `doctor`** (explains silence; backs the FAQ), then D1 completions.

Defer:
- **D5 Windows named pipe (L)**: highest risk (ACL, overlapped I/O, no `net`). Keep the report-file path and document it; if the milestone must include it, give it its own research-flagged phase with a Windows CI job exercising a real daemon.
- **D2 man page**: lowest user value of the usability set.
- **D7, D8, D10, D11**: D8 is required by the milestone's release-readiness phase; D7 is cheap and recommended with the image; D10/D11 opportunistic.
- Suppression/config file, Chocolatey/apt/Nix, docs site: see Anti-Features.

## Requirement-writing hints

- Split T16 into: (a) `blast --format sarif` Broken-only with GRT004 rule, (b) Impacted in properties, (c) `report --blast --format sarif` parity, (d) `partialFingerprints` as an explicit, separately testable requirement.
- Define "liveness" observably: which command prints what and which exit code on live / not running / error, plus the SIGKILL test (kill -9 the daemon, assert the prompt path reports not running within N seconds).
- Every doc artifact should name the test that keeps it true (precedent: `TestCIDoc`, `readme_doc_test.go`, `rules_doc_test.go`, `daemon_doc_test.go`, `validation_doc_test.go`).
- Every new install channel needs a verification step that does not require publishing (formula renders and `brew audit`-style lint locally, manifest validates, image runs `--version`, install.sh run against a locally built release dir via `file://` or a local server).

## Sources

- gruntled repo, read directly (HIGH): `.planning/PROJECT.md`, `README.md` (Status, Known limitation), `docs/cli.md` (Known limitations, exit codes), `docs/ci.md`, `.github/workflows/release.yml`, `scripts/build-release.sh` (`-trimpath`, ldflags), `go.mod` (`x/sys` indirect, no cobra), `cmd/gruntled/main.go` (hand-written usage, closure-defined flags).
- tflint installation docs as mirrored on a fork: install script, brew, choco (community), cosign `verify-blob` on `checksums.txt`, GHCR image (MEDIUM; fork copy, not the upstream page): https://github.com/papkos/tflint
- Microsoft winget-pkgs authoring guide (wingetcreate, manifest validation; zipped-portable shape unconfirmed, LOW): https://github.com/microsoft/winget-pkgs/blob/master/doc/Authoring.md
- Dependabot with SHA-pinned actions updating pin and version comment together (MEDIUM): https://pydevtools.com/handbook/how-to/how-to-pin-github-actions-by-sha-for-python-projects/ and https://topmark.readthedocs.io/en/stable/ci/dependabot/
- Go `runtime/debug` `ReadBuildInfo`, `Main.Version` handling pattern (`(devel)` / empty means not a `go install` build) (MEDIUM): https://go.googlesource.com/go/+/HEAD/src/runtime/debug/mod.go
- Peer-tool table (colour flags, completions, suppression files, man pages): prior knowledge, MEDIUM-LOW, re-verify before quoting in user-facing docs.
- Prior milestone research for context: `.planning/research/archive-v0.4/FEATURES.md`.
