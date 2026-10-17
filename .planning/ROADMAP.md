# Roadmap: gruntled

## Milestones

- ✅ **v0.1 Validated Engine** - Phases 1-4 (shipped 2026-09-29) — archived in `milestones/v0.1-ROADMAP.md`
- ✅ **v0.2 CI-Ready** - Phases 5-7 (shipped 2026-10-08) — archived in `milestones/v0.2-ROADMAP.md`
- ✅ **v0.3 Watch & Blast** - Phases 8-12 (shipped 2026-10-10) — archived in `milestones/v0.3-ROADMAP.md`
- ✅ **v0.4 Blast-aware Diagnostics** - Phases 13-17 (shipped 2026-10-13) — archived in `milestones/v0.4-ROADMAP.md`
- 🚧 **v0.5 Adoption & Completeness** - Phases 18-25 (in progress)

## Phases

<details>
<summary>✅ v0.1 Validated Engine (Phases 1-4) - SHIPPED 2026-09-29</summary>

See `milestones/v0.1-ROADMAP.md`.

</details>

<details>
<summary>✅ v0.2 CI-Ready (Phases 5-7) - SHIPPED 2026-10-08</summary>

- [x] Phase 5: Graph Diagnostics (7/7 plans) — completed 2026-09-30
- [x] Phase 6: Machine-Readable Output (5/5 plans) — completed 2026-10-01
- [x] Phase 7: Distribution & CI Integration (5/5 plans) — completed 2026-10-01

See `milestones/v0.2-ROADMAP.md`.

</details>

<details>
<summary>✅ v0.3 Watch & Blast (Phases 8-12) - SHIPPED 2026-10-10</summary>

- [x] Phase 8: Incremental Index Foundation (2/2 plans) — completed 2026-10-08
- [x] Phase 9: Blast Radius (3/3 plans) — completed 2026-10-08
- [x] Phase 10: Watch Daemon & Status File (7/7 plans) — completed 2026-10-08
- [x] Phase 11: Report, Single Instance & Release Proof (7/7 plans) — completed 2026-10-08
- [x] Phase 12: Gap Closure — Watcher Directory Ignore & v0.3 Audit Findings (5/5 plans) — completed 2026-10-10

See `milestones/v0.3-ROADMAP.md`.

</details>

<details>
<summary>✅ v0.4 Blast-aware Diagnostics (Phases 13-17) - SHIPPED 2026-10-13</summary>

- [x] Phase 13: Removed-Output Diagnostic & Shared Blast Core (3/3 plans) — completed 2026-10-10
- [x] Phase 14: Transitive Impact with Paths (3/3 plans) — completed 2026-10-10
- [x] Phase 15: Type-Level Surface Facts (6/6 plans) — completed 2026-10-11
- [x] Phase 16: Daemon Baseline, `report --blast` & `rebase` (5/5 plans) — completed 2026-10-12
- [x] Phase 17: Corpus Mutation Proof & Cross-Phase Audit (6/6 plans) — completed 2026-10-13

See `milestones/v0.4-ROADMAP.md`.

</details>

<details open>
<summary>🚧 v0.5 Adoption & Completeness (Phases 18-25) - IN PROGRESS</summary>

**Milestone Goal:** Make gruntled a complete, 360-degree project that anyone can install, run, trust and contribute to on linux, macOS and windows, without weakening the zero-false-positive, deterministic, read-only, no-net/no-exec guarantees.

- [x] **Phase 18: Truth Foundations** - `--version` is correct for `go install` users, one `VERSION` pins every doc and recipe, and a CHANGELOG drives the release notes (completed 2026-10-17)
- [ ] **Phase 19: CLI Surface** - One command registry drives help; `--color`/`--no-color`, `--quiet` and `blast --format sarif` land without moving a `check` byte
- [ ] **Phase 20: Daemon Completeness** - A dead daemon is never mistaken for a live one, `watch --base` seeds a dirty-start baseline, and editor/shell recipes are tested
- [ ] **Phase 21: Windows Named Pipe** - Live `report`, `report --blast` and `rebase` on windows over an owner-only named pipe, with no `net` and a new DLL allowlist proof rule
- [ ] **Phase 22: Completions, Man Page & Doctor** - Shell completions, `gruntled man` and `gruntled doctor`, all generated from the one registry
- [ ] **Phase 23: Release Pipeline & Install Channels** - `install.sh`, GHCR image, Homebrew and Scoop, attestations, each proven by CI before and after publication
- [ ] **Phase 24: Hygiene & Adoption Docs** - Community files, issue and PR templates, Dependabot with derived pins, quickstart, FAQ, comparison, accurate limitations
- [ ] **Phase 25: Proof & Release Readiness** - Corpus gate on v0.5, independent cross-phase audit, and a v0.5.0 release checklist (tagging stays Giulio's decision)

**Hard rules for every phase** (from `REQUIREMENTS.md`; each plan must keep them true):
- The six-target `binary-no-net-no-exec` proof stays green: no `net`, `os/exec`, `golang.org/x/sys/windows`, `golang.org/x/term`, `text/template`/`html/template`, cobra/pflag (all link `net`). Windows syscalls use std `syscall` + `NewLazyDLL` under an allowlist rule.
- `gruntled check` (text, json, sarif) stays byte-identical to v0.4 for non-TTY output; `check --format sarif` keeps its 4-rule table.
- Read-only on the analysed repository. Any doc that states a command, a version or an output is pinned by a test. No workflow creates a git tag or a release; v0.5.0 is Giulio's decision.

</details>

## Phase Details

### Phase 18: Truth Foundations

**Goal**: User gets the same, correct version string from `--version`, SARIF and the daemon however gruntled was installed, and every install snippet and release note in the repository comes from one source of truth, so no later install channel can ship a stale pin.
**Depends on**: Phase 17 (shipped v0.4)
**Requirements**: INST-01, INST-02, HYG-03
**Success Criteria** (what must be TRUE):
  1. A binary built by `go install github.com/GiulioSavini/gruntled/cmd/gruntled@vX.Y.Z` prints `gruntled vX.Y.Z (none)`; a release build still prints the ldflags version and commit; a plain `go build` (build info `(devel)`, `+dirty`, a pseudo-version, or any `vcs.revision`) still prints `gruntled dev (none)`; the one resolved string is also the SARIF `tool.driver.version` and the `ipc.Info` version (table test over an injected build-info reader)
  2. One `VERSION` source feeds `docs/ci.md`, the README install snippets and the pre-commit `rev`; a doc-pin test fails when any of them drifts (proven by a negative case that edits one pin), `docs/ci.md` no longer pins v0.2.0, and the release guard fails when `VERSION` differs from the tag
  3. `CHANGELOG.md` follows Keep a Changelog 1.1.0, has v0.1.0..v0.4.0 backfilled from `MILESTONES.md` plus an `Unreleased` section, and `release.yml` uses the section matching the tag as the release notes and fails for a tag with no section (proven by a CI dry run with a negative case)
  4. `check` text, JSON and SARIF stay byte-identical to the v0.4 goldens and the six-target no-net/no-exec proof passes with the build-info reader linked; the "`go install` prints `dev (none)`" limitation is deleted from `docs/cli.md` and `docs/ci.md`

**Plans**: 3/3 plans complete
- [x] 18-01-PLAN.md — version resolver (`versionInfo()` from ldflags or clean go-install build info; `--version`, SARIF, `ipc.Info`) (wave 1)
- [x] 18-02-PLAN.md — `VERSION` single source, doc-pin test with negative case, docs bumped to v0.4.0, release guard VERSION == tag (wave 1, parallel with 18-01)
- [x] 18-03-PLAN.md — `CHANGELOG.md` backfilled v0.1.0..v0.4.0, `scripts/release-notes.sh`, release notes from the CHANGELOG with a CI dry run and negative case (wave 2)

### Phase 19: CLI Surface

**Goal**: User can discover and drive every command from `gruntled help`, control colour and chatter predictably, and send `blast` findings to code scanning as SARIF, without a single default `check` byte moving.
**Depends on**: Phase 18 (the resolved version feeds the blast SARIF `tool.driver.version`)
**Requirements**: UX-01, UX-02, UX-03, UX-06
**Success Criteria** (what must be TRUE):
  1. `gruntled help [cmd]`, `-h` and `--help` print usage on stdout and exit 0 for every command; one registry drives the top usage, each per-command usage, the enumerated flag values and their validation; a test walks every `flag.FlagSet` and fails if a flag is missing from usage or `docs/cli.md`; the registry lands first as a pure refactor with `TestTopUsageMatchesDocs` green and unchanged
  2. On a terminal `check` text output is coloured; piped, redirected, `/dev/null`, `NO_COLOR=1`, `TERM=dumb` and `--no-color` all give the plain v0.4 bytes, `--color=always` forces colour, and the precedence is `--no-color` > `NO_COLOR` > `CLICOLOR_FORCE` > `TERM=dumb` > per-stream TTY; JSON, SARIF, the status file and daemon-rendered `report` text never carry colour, and stripping the SGR sequences from coloured output returns the plain output exactly (property test)
  3. `--quiet`/`-q` removes only the stderr summary and informational watch chatter: on a repository with findings the stdout bytes and the exit code are identical with and without it, and an error is still printed
  4. `gruntled blast --base <dir> --format sarif` prints a document the existing SARIF schema and upload proofs accept: Broken findings (GRT001..GRT004, GRT100) are results from a blast-only rule table that has GRT004 at index 4, Impacted units appear only as deterministic notifications or run properties and never as results, no `--base` path appears in the document, URIs are relative to the analysed tree and exit codes are unchanged; `check --format sarif` still has exactly 4 rules (regression test) and the "GRT004 has no SARIF rule" limitation is deleted from the docs
  5. The six-target proof passes with terminal detection linked (`x/sys/unix` ioctl on unix, `GetConsoleMode` through std `syscall` on windows; never `x/term`, never `ModeCharDevice` alone)

**Plans**: TBD

### Phase 20: Daemon Completeness

**Goal**: User running `gruntled watch` can always tell whether the daemon is alive, can start it with a baseline taken from another tree, and can put its status in their editor and shell prompt with patterns that are tested against real output.
**Depends on**: Phase 19 (the new `status` command and the `watch --base` flag are declared in the UX-01 registry; `--quiet` governs watch chatter)
**Requirements**: DAEMON-11, DAEMON-12, DAEMON-14
**Success Criteria** (what must be TRUE):
  1. After `kill -9` of a running daemon, `gruntled status [path]` prints `gruntled: not running` and exits 1 (live: the status line and exit 0; error: exit 3), and a status file left behind by the dead daemon is never reported as live; a graceful shutdown replaces the status line with a stopped marker; the status-line grammar is unchanged
  2. A daemon that is starting is never told "already watching" by a concurrent status probe: the start-versus-probe race is reproduced by a test and closed, and `status` never calls `ipc.Probe` on a polled path
  3. `gruntled watch --base <dir>` starts with a baseline taken from `<dir>`: `report --blast` right after start equals `blast --base <dir>` apart from a distinct baseline label, `rebase` still moves the baseline, `<dir>` inside the watched tree exits 2, a missing or unreadable `<dir>` exits 3, `<dir>` is read once and never watched or written, there is no protocol bump, and the "baseline starts clean" limitation is deleted from the docs
  4. Recipes for tmux, starship, bash/zsh/fish/PowerShell prompts, vim/neovim statusline, a VS Code `problemMatcher` and a vim `errorformat` are documented, and each pattern is tested in Go against real `check` and `status` output
  5. The daemon baseline still holds only `checking.Report`, `check` bytes are unchanged and the six-target no-net/no-exec proof stays green

**Plans**: TBD

**Research flag**: light — only whether the optional mtime heartbeat (`Chtimes`, at least 10 s) is taken for DAEMON-11; everything else follows existing v0.3/v0.4 patterns

### Phase 21: Windows Named Pipe

**Goal**: User on windows gets the same live `report`, `report --blast` and `rebase` answers from a running daemon as on unix, over an owner-only named pipe, while the binary still links no `net`, no `os/exec` and no `x/sys/windows`.
**Depends on**: Phase 20 (needs `status`, whose ping gains the pipe transport; shares the instance/report/rebase files, so the phases must not overlap)
**Requirements**: DAEMON-13
**Success Criteria** (what must be TRUE):
  1. On a windows runner a real `gruntled watch` daemon answers `report`, `report --blast` and `rebase [--expect N]` live over a named pipe, and one conformance table (unix framing, protocol v2 mismatch error, 64 MiB bound) passes against both the unix socket and the pipe; a Windows CI job exercises a real daemon
  2. The pipe is private to the user who started the daemon: the security descriptor is asserted by a test to be an explicit owner-only DACL (no Everyone or Anonymous entry), remote clients are rejected, the name is unpredictable per user, and a process that pre-creates the pipe name cannot pass for the daemon (first-instance flag)
  3. The `windows-dll-allowlist` rule in `check-architecture.sh` lands before any pipe code, with non-vacuous self-tests (planting `x/sys/windows`, a non-allowlisted DLL or proc, or a `CreateProcess`/`ShellExecute`/`WinExec` reference makes it fail), and the six-target no-net/no-exec proof is green with the pipe linked on windows/amd64 and windows/arm64
  4. The report-file dump is kept for one more release so a v0.4 client still works against a v0.5 windows daemon, and the "`report`/`rebase` unsupported on windows" limitation is deleted from the docs

**Plans**: TBD

**Research flag**: yes — spike before planning, recorded as a decision: overlapped I/O with `os.NewFile` deadlines and interruptible close, `FILE_FLAG_FIRST_PIPE_INSTANCE`, `PIPE_REJECT_REMOTE_CLIENTS`, explicit SDDL `D:P(A;;GA;;;<userSID>)`, multi-user denial and session-0 behaviour, windows/arm64 execution (compile-only so far; does a `windows-11-arm` runner exist), how the allowlist rule is implemented. If the spike fails the fallback is a runtime-dir file mailbox with no proof change, and DAEMON-13 is re-scoped by an explicit decision, never silently dropped. The research summary dropped that fallback; the requirement text keeps it and governs

### Phase 22: Completions, Man Page & Doctor

**Goal**: User can tab-complete every command and flag in their shell, read `man gruntled`, and ask `gruntled doctor` why gruntled is silent on their tree, all generated from the one registry so none of them can drift from the binary.
**Depends on**: Phase 21 (the command and flag set is final only after Phases 19-21, so the goldens are written once; `doctor`'s liveness probe covers every transport). If Phase 21 slips, Phases 22-25 proceed and `doctor` reports windows liveness through the lock fallback
**Requirements**: UX-04, UX-05, UX-07
**Success Criteria** (what must be TRUE):
  1. `gruntled completion bash|zsh|fish|powershell` prints a static script (subcommands, flags, `--format` values, directories; no dynamic callbacks) pinned by goldens, by a `VisitAll` drift test that fails when a flag is added without regeneration, and by a bash `COMPREPLY` behavioural test; CI smoke-tests each script in its shell (bash 3.2-compatible, zsh dual-mode guard, PowerShell pure ASCII)
  2. `gruntled man` prints a `gruntled.1` roff page with a fixed `.TH` date and `\-` escapes, pinned by a freshness test; the release archive layout does not change unless `TestBuildRelease`, the `release.yml` member count and `docs/ci.md` change together
  3. `gruntled doctor [path]` explains silence and environment read-only: unit count, unknown units with reasons, conservative include-rule skips, runtime dir and status path, daemon liveness through the status probe, a WSL `/mnt/c` hint, GOOS/GOARCH and module/vcs info; it never writes, runs a process, uses the network or checks for updates, and `gruntled version` prints the same string as `--version`; both are documented as outside the byte-determinism promise
  4. Generators use only `strings.Builder` and `fmt`: the architecture self-test case `binary-text-template` proves `text/template` is rejected, the six-target proof stays green, and `check` bytes are unchanged

**Plans**: TBD

### Phase 23: Release Pipeline & Install Channels

**Goal**: User can install a verified gruntled with `install.sh`, `brew`, `scoop`, a container image or `go install`, and every one of those routes is proven by CI before a release exists and again after it is published.
**Depends on**: Phase 22 (completions and man feed the formula and archives); Phase 18 (`VERSION`, CHANGELOG notes)
**Requirements**: INST-03, INST-04, INST-05, INST-06, INST-07
**Success Criteria** (what must be TRUE):
  1. The release's `install.sh` (a release asset with that release's SHA-256s embedded) installs the right binary for the OS and architecture into `~/.local/bin` or `$BINDIR` without sudo from a local file server in CI, under dash and busybox, and fails closed (non-zero exit, nothing installed) on a tampered archive, a truncated download or a checksum mismatch; it is shellcheck-clean
  2. `ghcr.io/giuliosavini/gruntled:vX.Y.Z` and `latest` are a linux amd64+arm64 `FROM scratch` image with a non-root `USER`, built in a separate `packages: write` job from the released linux archives (no recompile); a `--read-only --network none` run of `check` prints bytes equal to the host binary's, and the docs show how to mount the repository root and pin by digest
  3. `scripts/render-formula.sh` renders a Homebrew formula (4 URLs, 4 sha256 from `checksums.txt`, `test do` runs `--version`, completions generated from the binary) and a Scoop manifest, both attached as release assets; the tap is updated only when Giulio has created it and provided a tap-scoped token, and otherwise the step is skipped without failing the release
  4. Release archives and `checksums.txt` carry GitHub artifact attestations and the documented `gh attestation verify` recipe succeeds; a CI check proves two builds of the same commit are byte-identical
  5. A `verify-published` job runs the documented install paths (`install.sh`, `go install @tag`, the image pull) and asserts `--version` prints the tag, its dry-run counterpart runs on every push, and no workflow creates a tag or a release

**Plans**: TBD

**Research flag**: yes — immutable-release behaviour with `gh release create` and asset ordering (draft, attach, publish); GHCR default package visibility; Homebrew completions convention and pull-versus-push for the tap; Scoop `autoupdate`; rehearsal of credentials and ordering in a sandbox repository before the real release. Open decisions for Giulio: tap repository name (`homebrew-tap` or `homebrew-gruntled`) and token, GHCR visibility, winget (deferred)

### Phase 24: Hygiene & Adoption Docs

**Goal**: First-time visitor can understand, try, trust, report a problem with and contribute to gruntled without asking anyone, and the repository's automation keeps its own pins fresh without breaking a proof.
**Depends on**: Phase 23 (Dependabot's scope must include the new actions; quickstart and FAQ cite the install channels)
**Requirements**: HYG-01, HYG-02, HYG-04, HYG-05, HYG-06, DOCS-01, DOCS-02, DOCS-03
**Success Criteria** (what must be TRUE):
  1. `SECURITY.md` (latest minor supported, private vulnerability reporting as the only channel, the threat-model scope), `CONTRIBUTING.md` and `CODE_OF_CONDUCT.md` (the non-negotiables: no `net`/`os/exec`/cobra, layering, zero-false-positive bar for a new diagnostic, determinism, tests first, how to run `check-architecture.sh` and the corpus gate; Apache-2.0 inbound=outbound, no CLA) and `CODEOWNERS` exist; issue forms for a bug, a false-positive report (asks for the minimal tree or `graph --json`, `--version`, OS) and a feature request, `config.yml` with `blank_issues_enabled: false` and a security contact link, and a PR template checklist are present and lint-clean in CI
  2. `.github/dependabot.yml` covers `gomod` and `github-actions` (weekly, grouped, cooldown, `go` directive bumps excluded); `TestCIDoc` derives action pins from the workflow files, a lint fails any `uses:` that is not a 40-hex SHA with a version comment, `actionlint` and `zizmor` pass in CI, and `hcl/v2`, `go-cty` and `fsnotify` bumps are documented as corpus-gate-required
  3. The 5-minute quickstart runs on a checked-in broken fixture and a test fails if its printed output differs from the real `gruntled check` output; the FAQ (why silent, why no suppressions, why no network, relation to `terragrunt hcl validate`, stacks, `mock_outputs`, exit codes, Windows/WSL, Gatekeeper/SmartScreen, NFS runtime dir) and the symptom-driven troubleshooting page link the known limitations; the comparison page is date-stamped, reuses only measured numbers from `docs/validation.md` and sources every external claim
  4. `docs/RELEASING.md` lists the settings that cannot be automated (branch protection without required code-owner review, private vulnerability reporting, immutable releases, GHCR package visibility, tap token expiry); README Status and `docs/cli.md` describe v0.5 accurately, no "Known limitations" bullet made false by this milestone remains (GRT004 SARIF, status liveness, windows `report`/`rebase`, dirty-start baseline, `go install` version), and the existing doc tests pin it

**Plans**: TBD

### Phase 25: Proof & Release Readiness

**Goal**: Giulio can trust v0.5 on real repositories and across phase boundaries, and can tag v0.5.0 by following one checked list.
**Depends on**: Phase 24
**Requirements**: REL-04, REL-05, REL-06
**Success Criteria** (what must be TRUE):
  1. On v0.5 HEAD `corpus-gate.sh` (network-isolated, against the frozen v0.3.0 reference), `compare-ref.sh` and `compat-v1.sh` pass: `check` is byte-identical on the three corpora, reformat-only mutations give zero noise, every injected mutation is caught, `check --format sarif` and `blast --format sarif` still pass the schema and upload proofs, and the results are recorded in `docs/validation.md`
  2. An independent cross-phase security and integration audit (sec agent) across the feature phases 18-24 closes with zero open BLOCKER, HIGH or MEDIUM findings (any found is closed in this phase), and the six-target no-net/no-exec proof, including the new `windows-dll-allowlist` rule, is green on the final tree
  3. `scripts/release-check.sh` passes on HEAD: CHANGELOG section present, `VERSION` equals the intended tag, doc pins consistent, README Status accurate, archives, `checksums.txt`, image and formula render, completions and man page fresh, and it confirms no tag exists and creates none
  4. `docs/RELEASING.md` gives the v0.5.0 release steps including the manual-settings checklist, so tagging remains Giulio's decision and nothing in the milestone ran a release

**Plans**: TBD

## Progress

| Phase | Milestone | Plans Complete | Status | Completed |
|-------|-----------|----------------|--------|-----------|
| 5. Graph Diagnostics | v0.2 | 7/7 | Complete | 2026-09-30 |
| 6. Machine-Readable Output | v0.2 | 5/5 | Complete | 2026-10-01 |
| 7. Distribution & CI Integration | v0.2 | 5/5 | Complete | 2026-10-01 |
| 8. Incremental Index Foundation | v0.3 | 2/2 | Complete | 2026-10-08 |
| 9. Blast Radius | v0.3 | 3/3 | Complete | 2026-10-08 |
| 10. Watch Daemon & Status File | v0.3 | 7/7 | Complete | 2026-10-08 |
| 11. Report, Single Instance & Release Proof | v0.3 | 7/7 | Complete | 2026-10-08 |
| 12. Gap Closure — Watcher Dir Ignore & Audit Findings | v0.3 | 5/5 | Complete | 2026-10-10 |
| 13. Removed-Output Diagnostic & Shared Blast Core | v0.4 | 3/3 | Complete    | 2026-10-10 |
| 14. Transitive Impact with Paths | v0.4 | 3/3 | Complete    | 2026-10-10 |
| 15. Type-Level Surface Facts | v0.4 | 6/6 | Complete    | 2026-10-11 |
| 16. Daemon Baseline, `report --blast` & `rebase` | v0.4 | 5/5 | Complete    | 2026-10-12 |
| 17. Corpus Mutation Proof & Cross-Phase Audit | v0.4 | 6/6 | Complete    | 2026-10-13 |
| 18. Truth Foundations | v0.5 | 3/3 | Complete    | 2026-10-17 |
| 19. CLI Surface | v0.5 | 0/TBD | Not started | - |
| 20. Daemon Completeness | v0.5 | 0/TBD | Not started | - |
| 21. Windows Named Pipe | v0.5 | 0/TBD | Not started | - |
| 22. Completions, Man Page & Doctor | v0.5 | 0/TBD | Not started | - |
| 23. Release Pipeline & Install Channels | v0.5 | 0/TBD | Not started | - |
| 24. Hygiene & Adoption Docs | v0.5 | 0/TBD | Not started | - |
| 25. Proof & Release Readiness | v0.5 | 0/TBD | Not started | - |
