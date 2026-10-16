# gruntled

> *Terragrunt, but gruntled.*

## Current State: v0.4 shipped 2026-10-13 (tag v0.4.0 pending — Giulio's decision); v0.5 in planning

`gruntled check` reports `GRT001`-`GRT004` and `GRT100` in text, JSON or SARIF; `GRT004` replaces
`GRT001` at every reference to an output the change removed. `gruntled graph --json` prints the
repository graph. `gruntled watch` keeps an in-memory index current on every save (fsnotify, or
stat polling with `--poll` and on windows) and writes a one-line status file outside the
repository. `gruntled blast --base <dir>` separates Broken from Impacted, Impacted now following
dependents past one hop (with distance, a connecting path and `--depth`) and through type-level
surface changes (new required variable, variable/output type, `sensitive`). The daemon answers
`report --blast` against its in-memory baseline over IPC v2, and `rebase` moves that baseline.
100% mutation catch and zero noise proven on three real corpora, network-isolated against a
frozen v0.3.0 binary. Installable from tagged GitHub releases, as a pre-commit hook, or via the
CI recipes in `docs/ci.md`.

## Current Milestone: v0.5 Adoption & Completeness

**Goal:** Make gruntled a complete, 360-degree project that anyone can install, run, trust and contribute to on every supported platform — without weakening the zero-false-positive, no-net/no-exec guarantees.

**Target features:**
- Installation parity: a verified install script, a Homebrew tap, a container image on GHCR built by the release workflow, and a `--version` that is correct for `go install` users; CI recipes pinned from one source of truth, never stale
- Project hygiene for contributors and users: CONTRIBUTING, SECURITY (disclosure policy), CHANGELOG, issue/PR templates, CODEOWNERS, automated dependency updates
- CLI usability: shell completions and man pages generated without cobra, `--no-color`/`--quiet` ergonomics with TTY detection, `blast --format sarif` (closing the GRT004-has-no-SARIF-rule gap)
- Daemon completeness: status-file liveness so a dead daemon is never mistaken for a live one, `watch --base <dir>` for a dirty-start baseline, `report`/`rebase` live queries on windows over a named pipe without `net`, and editor/shell integration recipes for the status file
- Proof and release readiness: the corpus gate re-run on v0.5, docs verified against the binary, an independent cross-phase audit, and a v0.5.0 release checklist (tagging stays Giulio's decision)

## What This Is

A Go CLI (later a daemon) for Terragrunt repositories. It builds a graph of the
units, their dependencies and the public surface of the modules they call, and
reports two things: **what is broken** and **what is impacted**. It is for people
who maintain Terragrunt repositories large enough that finding a wiring mistake
means waiting minutes for a slow command that reports one error at a time.

The full design rationale, including the survey of what already exists, lives in
`docs/superpowers/specs/2026-09-01-gruntled-design.md`.

## Core Value

**Tell the user, before they run anything slow, that `dependency.X.outputs.Y`
does not exist in the module it points to.**

Everything else — the daemon, the blast radius, the speed, the extra diagnostics —
is built on top of that one answer being correct and trustworthy.

## Requirements

### Validated

- ✓ Structural HCL decoder, the six path functions, `include` merge, include parsed once — v0.1
- ✓ Unit→module resolution via `terraform.source` or the unit's own directory; `unknown` on anything unresolvable offline — v0.1
- ✓ Unit graph and module surface extraction — v0.1
- ✓ `GRT001` and `GRT100` — v0.1
- ✓ `gruntled check`: deterministic output, stable exit code, text and JSON — v0.1
- ✓ Synthetic repo generator and golden tests — v0.1
- ✓ Real-corpus experiment: 0 false positives, every injected mutation caught (denis256 8/8), faster than `terragrunt hcl validate` in 3 of 3 runs — v0.1
- ✓ MORE-01 `GRT002`: `dependency.config_path` pointing at no unit — v0.2
- ✓ MORE-02 `GRT003`: dependency cycle between units, deterministic member order — v0.2
- ✓ MORE-06 `GRT002`/`GRT003` corpus validation (0 findings on iso20022/secret, denis256 exact oracle set, 13/13 mutations) — v0.2
- ✓ INT-01 `gruntled graph --json` — v0.2
- ✓ INT-02 `gruntled check --format sarif`, accepted by GitHub code scanning — v0.2
- ✓ INT-03 pre-commit hook via `.pre-commit-hooks.yaml` — v0.2
- ✓ INT-04 GitHub Actions and GitLab CI recipes, GitHub recipe run in own CI — v0.2
- ✓ REL-01 static binaries for linux/darwin/windows on amd64/arm64 with checksums, tagged release — v0.2
- ✓ REL-02 `gruntled --version` with build-time injected version and commit — v0.2
- ✓ DAEMON-01 `gruntled watch`: full index, then incremental reindex after a save (~150 ms debounce), ignore rules, new/deleted dirs — v0.3
- ✓ DAEMON-02 incremental reindex == full rescan, rapid stateful property (incl. editor-pattern dirs and symlinked includes) — v0.3
- ✓ DAEMON-03 atomic one-line status file at a per-repository path outside the repository — v0.3
- ✓ DAEMON-04 `gruntled report` in text/json/sarif matching `check` (unix socket; report file on windows) — v0.3
- ✓ DAEMON-05 single instance per repository, crash-safe lock and stale socket recovery — v0.3
- ✓ DAEMON-06 six-target no-net/no-exec proof with watcher and socket linked — v0.3
- ✓ BLAST-01 `gruntled blast --base`: disjoint sorted Broken/Impacted, text and json, "no baseline" label — v0.3
- ✓ BLAST-02 one-hop Impacted on `variable`/`output` name changes; pre-existing findings never Broken — v0.3
- ✓ MORE-03/MORE-07 `GRT004` replaces `GRT001` at every reference to a removed output, through one shared blast core (`Between`) reused by `check`/`report`/`blast`/the daemon — v0.4
- ✓ BLAST-03/04/05 transitive Impacted follows dependents past one hop, with distance, one connecting path and `--depth` — v0.4
- ✓ BLAST-06..10 type-level surface facts: new required variable, variable/output type or `sensitive` changed puts instantiating units in Impacted, never Broken — v0.4
- ✓ BLAST-11/SEC-01 64 MiB blast-response size bound; text/JSON escaping on every new surface — v0.4
- ✓ DAEMON-07..10 daemon answers `report --blast` against its in-memory baseline over IPC v2; `rebase [--expect N]` moves the baseline; baseline holds only `checking.Report` — v0.4
- ✓ REL-03 byte-identical `check`, empty noise on reformat-only mutations, 100% mutation catch on three real corpora, independent cross-phase security/integration review — v0.4

### Active

v0.5 Adoption & Completeness — see `.planning/REQUIREMENTS.md` for the REQ-IDs (INST-*, HYG-*, UX-*, DAEMON-*, REL-*).

**Still deferred** (see `milestones/v0.4-REQUIREMENTS.md`): `GRT005` (MORE-04) and `GRT006`
(MORE-05) overlap `terragrunt hcl validate --inputs` and carry include-merge false-positive risk;
per-hop `file:line`, reference-gated propagation.

### Out of Scope

- **Validating Terraform itself** (provider schemas, resource attributes, expression
  evaluation) — a different, much larger problem already served by `tofu validate`
  and `tflint`. `.tf` files are read only to extract module surface.
- **Calling external binaries** (`tofu`, `tflint`, `terragrunt` the CLI) — would break
  the single-binary, no-external-process guarantee that makes air-gap and idempotency
  demonstrable rather than declared.
- **Network access at runtime** — no remote source downloading, no telemetry, no
  update checks. Unresolvable remote sources mark a unit `unknown`.
- **On-disk index cache** (deliberately deferred) — schema versioning, invalidation
  and corruption are three sources of bugs, traded against an initial index that
  should take under a second. Add it only if startup ever becomes slow.
- **LSP / editor integration** — the status file is readable from a shell prompt,
  tmux or an editor status bar without implementing a protocol.
- **`terragrunt.stack.hcl` (Terragrunt Stacks)** — `terragrunt stack generate` materialises
  ordinary `terragrunt.hcl` files on disk, which gruntled's tree walk then finds for free.
  Only ungenerated stack definitions are invisible. That is a documented absence, not a
  misreport, so it is acceptable for v0.1.
- **Planning or applying infrastructure** — gruntled is strictly read-only.

## Context

**Shipped state (v0.4, 2026-10-13):** ~56.5k lines of Go (~42.4k of them tests). Go 1.27,
`hashicorp/hcl/v2` + `ext/typeexpr`, `fsnotify` (not on windows), stdlib `flag`, raw-syscall
AF_UNIX (no `net`), IPC protocol v2. CI: architecture rules and the six-target no-net/no-exec
proof, `go test -race` on linux plus test jobs on macOS and Windows, staticcheck, govulncheck,
SARIF schema/upload proofs, release packaging dry run and `recipe-check`; tag-triggered
`release.yml`; `corpus.yml` (workflow_dispatch + weekly, network-isolated) proves the mutation
set on three real corpora against a frozen v0.3.0 reference binary.
Known limitations (documented): darwin native watcher late by ≤30 s after a dangling symlink
(fsnotify#787), 64 MiB report/blast response cap, Windows runtime dir relies on `%LocalAppData%`
ACLs, `report`/`rebase` unsupported over AF_UNIX on windows, the include-target conservative rule
hides all 8 known broken references on the denis256 secondary corpus (0/8 recall, 0 false
positives — the primary v2 candidate).
Work is run by a four-agent team (orchestrator, planner, executor, sec) on GSD Core, talking on
`.claude/agentbus` with a local GUI.

**The problem is parsing, not the cloud.** Terragrunt documents O(n²) complexity in
`locals` evaluation and the fact that `include` files are re-evaluated in the context
of every unit that includes them; a `run_cmd` in a root config included by a hundred
units runs a hundred times. Documented consequence: `run-all plan` at 8+ minutes on
30-50 modules ([performance docs](https://terragrunt.gruntwork.io/docs/troubleshooting/performance),
[#2806](https://github.com/gruntwork-io/terragrunt/issues/2806)).

**What already exists** (surveyed 2026-09-01):

- `terragrunt hcl validate --inputs --strict` already covers missing required inputs
  and undeclared inputs — one-shot, from scratch every time.
- Terragrunt's own probe: [#5811](https://github.com/gruntwork-io/terragrunt/issues/5811)
  was closed on 2026-04-10, but the fix makes `dependency.X.outputs.Y` evaluate to an
  opaque unknown during validation rather than checking that the output exists. The gap
  `GRT001` targets is therefore still open — and now deliberately so.
- Terramate does git-based change detection, Terragrunt-aware — it reports what
  *changed*, not what *breaks*.
- tflint and `tofu validate` cover correctness inside a module, per directory,
  requiring `init`.
- Nobody keeps a warm index. The community answer to watch mode is `entr` or
  `watchexec` re-running the slow command.

**The defensible position** is the warm index, not the checks. Terragrunt is a
stateless CLI by design; upstream can absorb individual checks but will not become a
daemon.

**Test corpus — identified and verified.** The largest Terragrunt repository on the
developer's machine has one unit, so the corpus is external. The obvious candidates are
traps: `gruntwork-io/terragrunt-infrastructure-live-example` has zero `dependency` blocks
in its entire history, and its official replacement uses Terragrunt Stacks, which
materialise units into a gitignored directory only after `terragrunt stack generate` —
invisible to a static walk.

Verified corpus:

| Repository | Licence | Role |
|---|---|---|
| `aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws` | MIT-0 | Primary. 65 units, 62 with `dependency` blocks, all modules local. Confirmed by hand that `GRT001` resolves correctly and reports nothing. |
| `cds-snc/secret` | MIT | Secondary, smaller cross-check |
| `denis256/terragrunt-tests` | MIT | Hand-written golden fixtures and robustness cases |

**Structural fact discovered in the corpus:** all 65 units omit `terraform { source }`
entirely — Terragrunt then runs against the `.tf` files in the unit's own directory. The
design had assumed unit→module always goes through `source`. Resolving a unit to its
module must handle both forms.

**`mock_outputs` is the main false-positive risk.** It appears in 11 files of the primary
corpus and exists precisely to let a plan proceed when an output is unavailable. Its
presence must not make a missing output acceptable, and its absence must not make one an
error — this needs an explicit, tested decision.

## Constraints

- **Tech stack**: Go 1.27. `hashicorp/hcl/v2`, stdlib `flag` (not cobra: cobra/pflag
  link `net` into the binary, which breaks the static no-network proof). `fsnotify` for the
  daemon (not linked on windows). No cloud SDKs.
- **Scope**: Terragrunt only. `.tf` parsing limited to module surface extraction.
- **No external processes, no network** at runtime. If the imported Terragrunt library
  would execute `run_cmd` or read the environment, that path must be disabled and the
  affected unit marked `unknown`.
- **Read-only**: never writes inside the analysed repository. No `.terraform/`, no
  lock files, no `init`.
- **Zero false positives**: when an analyzer is not certain, it stays silent. Ten
  false negatives are better than one false positive.
- **Deterministic output**: same input, same diagnostics, same order, same exit code.
  Go randomises map iteration — every collection must be sorted explicitly. All paths in
  output must be repository-relative: an absolute path leaking out breaks reproducibility
  across machines and CI.
- **Licence**: Apache-2.0, matching OpenTofu and Terragrunt.
- **Architecture**: DDD / hexagonal. The domain layer must not import HCL.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Terragrunt-only scope | Removes every external binary dependency; makes the idempotency and air-gap guarantees demonstrable instead of declared | — Pending |
| Warm index as the differentiator, not the checks | Terragrunt is stateless by design; upstream can absorb checks but not this | — Pending |
| Own HCL parser from M1 — do NOT import Terragrunt as a library | Verified by compiling: `ParseConfigFile`/`PartialParseConfigFile` are exported, but the only `*ParsingContext` constructor requires `*venv.Venv` from `internal/venv`, which Go refuses to import from an external module, and no exported function anywhere returns one. Importing also pulls 672 modules including full AWS/Azure/GCP SDKs and requires Go 1.27. Reversed on 2026-09-01 | ✓ Good |
| Structural decode only — never evaluate expression values | `GRT001` compares names read off the HCL AST; it never needs a value. This removes state, `mock_outputs` and most functions from scope by construction rather than by suppression | ✓ Good (0 false positives on corpus, v0.1 and v0.2) |
| v0.1 ships `GRT001` + syntax only | One diagnostic justifies the tool. More checks before the idea is validated means more bug surface and more false-positive risk | ✓ Good (v0.1 experiment passed) |
| v0.1 ships `check` only, no daemon | A daemon on top of an unverified engine is wasted work. `check` is itself the experiment, and is already useful in pre-commit and CI | ✓ Good (v0.1 experiment passed) |
| No on-disk index cache in v1 | Premature optimisation: versioning, invalidation and corruption traded against a sub-second startup | — Pending |
| Status file over desktop notifications | `notify-send` is absent on the developer's WSL2 machine; PowerShell toasts cost ~1s and break the no-external-process rule. A status file is readable from a prompt, tmux or an editor and works everywhere | — Pending |
| Impacted only when module surface changes | Reporting twelve units because a comment changed destroys trust in the signal | — Pending |
| Open the repository once M1 passes its test | If the experiment fails, nothing was published and nothing needs explaining | ✓ Good (public since v0.2 Phase 6) |
| Name: gruntled | Memorable, ownable, no relevant collision, and the joke carries the README | — Pending |
| mock_outputs never suppresses GRT001, and severity stays error | With merge-with-state, a renamed output silently falls back to the mock value at `apply`, which is exactly the bug gruntled exists to catch. Mock facts only enrich the message. `enabled = false`, `skip_outputs = true` or any non-literal value for either keeps GRT001 silent, because Terragrunt then never reads the module's outputs. Suppressing would also make the Phase 4 mutation run report 0/8 on the primary corpus | ✓ Locked (Phase 3) |
| v0.2 is adoption (CI, SARIF, releases, GRT002/003) before the daemon | The one-shot engine is validated but not installable. GRT002/003 are pure graph queries; GRT004 needs a diff (belongs with blast), GRT005/006 overlap `terragrunt hcl validate --inputs` and carry merge-related false-positive risk | ✓ Good (v0.2.0 shipped) |
| SARIF URIs relative to the analysed directory; CI prefixes them with jq | `upload-sarif` `checkout_path` does not rewrite artifact URIs; users analysing a subdirectory must prefix or run from repo root (documented in `docs/ci.md`) | ✓ Good (alerts land on correct files/lines) |
| Repository made public for code scanning | Code scanning without GHAS requires a public repo; history secret-scanned clean first | ✓ Good |
| 6 release targets incl. windows/arm64 | User decision 2026-10-01, overriding the 5-target lock; target list lives only in `check-architecture.sh` and `build-release.sh`, kept equal by a test | ✓ Good |
| No goreleaser; `scripts/build-release.sh` + one `gh release create` | Single packaging path also run on every push/PR as a dry run; no extra tool or config to trust | ✓ Good (first real run green) |
| `release.yml` re-runs architecture proof and tests on the tagged commit | `needs:` cannot span workflows, so the release cannot depend on `ci.yml`; tag guard `vX.Y.Z[-suffix]` + must be on master | ✓ Good |
| `--version` from ldflags only, `ReadBuildInfo` fallback dropped | Local `go build` stamps a VCS pseudo-version that broke the `dev (none)` contract; `go install @tag` therefore prints `dev (none)`, documented | ⚠️ Revisit (go install users see no version) |
| `recipe-check` CI job proves `docs/ci.md` recipes | Docs that are executed cannot drift; `TestCIDoc` guards the pins | ✓ Good (installs from checkout, not `@tag`) |
| Empty `config_path` matches terragrunt per case | Block `""` unresolved and silent; `dependencies.paths` entry `""` is a `GRT003` self-loop, as terragrunt v1.1.6 does | ✓ Good |
| Watch daemon never forks or daemonises; run under tmux/systemd/`&` | fork/exec breaks the no-exec guarantee | ✓ Good (v0.3) |
| `report` over raw-syscall AF_UNIX, report file on windows | `net` would break the no-net proof; windows reads a dump only while the daemon's lock is held | ✓ Good (v0.3) |
| Blast baseline from `--base <dir>`, not git | No external process or go-git; CI uses `git worktree` | ✓ Good (v0.3) |
| Editor-file ignore patterns apply to files only | Directories like `2024/` were never watched and the daemon went stale (v0.3 audit BLOCKER) | ✓ Good (phase 12) |
| Parse cache keyed by path but validated by canonical path | Files reached through symlinks went stale when the target changed (v0.3 audit MEDIUM) | ✓ Good (phase 12) |
| Escape control/bidi runes in every text and JSON output | Repository names could drive the user's terminal | ✓ Good (phase 12) |
| Cross-phase milestone audit with an independent sec agent before closing | Per-phase verifications all passed while a BLOCKER and a MEDIUM existed between phases | ✓ Good (v0.3) |
| stdlib flag instead of cobra for v0.1 | One subcommand. cobra/pflag link `net`, `net/url` and `net/netip` plus `text/template`, while the stdlib keeps `binary-no-net-no-exec` a one-line CI proof. Revisit when v2 adds several subcommands | ✓ Locked (Phase 3) |
| `GRT004` exists only in the shared blast core (`Between`), never in `check`/`report` | It reclassifies `GRT001`, never adds a finding; one seam avoids duplicate/divergent reclassification logic across surfaces | ✓ Good (v0.4, confirmed by milestone integration check) |
| IPC protocol bumped to v2; a version mismatch is an explicit error, never an empty answer | A silent "nothing impacted" from a stale client/daemon pair would be worse than refusing | ✓ Good (v0.4) |
| Daemon baseline holds only `checking.Report`, never a loader/`fs.FS`/parse cache | Keeps daemon memory bounded and the report op response blast-byte-free; measured at 65/500/5,000 synthrepo units | ✓ Good (v0.4) |
| Mutation proof runs against a frozen, fingerprinted v0.3.0 reference binary, network-isolated (`unshare -rn`) | A reference binary that silently drifted from v0.3.0, or a gate that ran with network, would invalidate the whole proof | ✓ Good (v0.4, REL-03 31/31) |
| No git tag created by any GSD workflow (`git.create_tag: false`) | Tagging a release is Giulio's decision, not an automated one | ✓ Good (v0.4, no tag at HEAD) |
| Five phases kept for v0.4 although `coarse` granularity suggests 2-4 | The type-fact and daemon phases are separately research-flagged, and the proof phase must follow both | ✓ Good (v0.4 shipped with 5 phases, 0 critical gaps) |

## Success Criteria for v0.1

The milestone is a **falsifiable experiment**, not a feature list.

Research established that maintained public repositories do not contain naturally
occurring wiring bugs — `apply` catches them long before they are committed. Hunting for
an organic bug would fail. The criterion is therefore mutation-based, which is stricter
and reproducible:

1. **Zero false positives.** On the unmutated corpus, `gruntled check` reports nothing.
   This is the harder and more valuable half: a single false positive ends adoption.
2. **Catches every injected mutation.** Rename or delete an output in a target module;
   gruntled must report every reference that no longer resolves. Verified by hand that
   the mechanism holds on the corpus: `dependency.s3.outputs.role_name` resolves to
   `output "role_name"` in `iac.src/s3_runtime`.
3. **Faster than `terragrunt hcl validate`** on the same corpus.
4. **`terragrunt hcl validate` does not report the injected mutations** — confirming the
   gap is real rather than assumed.

If it fails, the idea is wrong and that was learned in one milestone rather than three.

Speed returns to the v0.1 criterion. Writing the parser from the start means `include`
files are parsed once and shared from day one, so `gruntled check` should already beat
`terragrunt hcl validate` on the corpus. This was not true under the abandoned
import-the-library plan.

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd-complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-10-16 after starting milestone v0.5*
