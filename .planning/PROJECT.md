# gruntled

> *Terragrunt, but gruntled.*

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

(None yet — ship to validate)

### Active

**v0.1 — the falsifiable experiment**

- [ ] Parse a Terragrunt repository and build the unit graph
- [ ] Extract module surface (`variable` and `output` names) from `.tf` files
- [ ] `GRT001`: report `dependency.X.outputs.Y` where `Y` is not an output of the target module
- [ ] `GRT100`: report HCL syntax errors
- [ ] `gruntled check` — one-shot, deterministic output, stable exit code
- [ ] Synthetic Terragrunt repository generator (N units, nested includes, dependencies)
- [ ] Golden tests over generated and hand-written fixtures
- [ ] Proven on at least one real public Terragrunt repository

**Later**

- [ ] Own HCL parser: parse each include once and share it (the speed claim)
- [ ] `gruntled watch` — daemon with in-memory incremental reindexing
- [ ] Status file + `gruntled report`
- [ ] `gruntled blast` — Broken vs Impacted
- [ ] Remaining diagnostics: `GRT002`-`GRT006`
- [ ] `gruntled graph --json`
- [ ] SARIF output, pre-commit hook, CI integration

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
- **Planning or applying infrastructure** — gruntled is strictly read-only.

## Context

**The problem is parsing, not the cloud.** Terragrunt documents O(n²) complexity in
`locals` evaluation and the fact that `include` files are re-evaluated in the context
of every unit that includes them; a `run_cmd` in a root config included by a hundred
units runs a hundred times. Documented consequence: `run-all plan` at 8+ minutes on
30-50 modules ([performance docs](https://terragrunt.gruntwork.io/docs/troubleshooting/performance),
[#2806](https://github.com/gruntwork-io/terragrunt/issues/2806)).

**What already exists** (surveyed 2026-09-01):

- `terragrunt hcl validate --inputs --strict` already covers missing required inputs
  and undeclared inputs — one-shot, from scratch every time.
- Terragrunt's module-output probe targets the same check as `GRT001` but is currently
  broken ([#5811](https://github.com/gruntwork-io/terragrunt/issues/5811)).
- Terramate does git-based change detection, Terragrunt-aware — it reports what
  *changed*, not what *breaks*.
- tflint and `tofu validate` cover correctness inside a module, per directory,
  requiring `init`.
- Nobody keeps a warm index. The community answer to watch mode is `entr` or
  `watchexec` re-running the slow command.

**The defensible position** is the warm index, not the checks. Terragrunt is a
stateless CLI by design; upstream can absorb individual checks but will not become a
daemon.

**No local test corpus.** The largest Terragrunt repository on the developer's machine
has one unit. A synthetic generator plus real public repositories is a v0.1
requirement, not an afterthought.

## Constraints

- **Tech stack**: Go 1.24. `hashicorp/hcl/v2`, `spf13/cobra`. `fsnotify` when the
  daemon arrives. No cloud SDKs.
- **Scope**: Terragrunt only. `.tf` parsing limited to module surface extraction.
- **No external processes, no network** at runtime. If the imported Terragrunt library
  would execute `run_cmd` or read the environment, that path must be disabled and the
  affected unit marked `unknown`.
- **Read-only**: never writes inside the analysed repository. No `.terraform/`, no
  lock files, no `init`.
- **Zero false positives**: when an analyzer is not certain, it stays silent. Ten
  false negatives are better than one false positive.
- **Deterministic output**: same input, same diagnostics, same order, same exit code.
  Go randomises map iteration — every collection must be sorted explicitly.
- **Licence**: Apache-2.0, matching OpenTofu and Terragrunt.
- **Architecture**: DDD / hexagonal. The domain layer must not import HCL.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Terragrunt-only scope | Removes every external binary dependency; makes the idempotency and air-gap guarantees demonstrable instead of declared | — Pending |
| Warm index as the differentiator, not the checks | Terragrunt is stateless by design; upstream can absorb checks but not this | — Pending |
| Import `gruntwork-io/terragrunt` as a library in M1 | Correct HCL function semantics for free, works on real repos from day one. Consequence: inherits its parsing cost, so the speed claim moves to a later milestone | — Pending |
| v0.1 ships `GRT001` + syntax only | One diagnostic justifies the tool. More checks before the idea is validated means more bug surface and more false-positive risk | — Pending |
| v0.1 ships `check` only, no daemon | A daemon on top of an unverified engine is wasted work. `check` is itself the experiment, and is already useful in pre-commit and CI | — Pending |
| No on-disk index cache in v1 | Premature optimisation: versioning, invalidation and corruption traded against a sub-second startup | — Pending |
| Status file over desktop notifications | `notify-send` is absent on the developer's WSL2 machine; PowerShell toasts cost ~1s and break the no-external-process rule. A status file is readable from a prompt, tmux or an editor and works everywhere | — Pending |
| Impacted only when module surface changes | Reporting twelve units because a comment changed destroys trust in the signal | — Pending |
| Open the repository once M1 passes its test | If the experiment fails, nothing was published and nothing needs explaining | — Pending |
| Name: gruntled | Memorable, ownable, no relevant collision, and the joke carries the README | — Pending |

## Success Criteria for v0.1

The milestone is a **falsifiable experiment**, not a feature list. It succeeds only if,
on a real public Terragrunt repository, `gruntled check` reports at least one genuine
wiring error that `terragrunt hcl validate` does not report — with zero false positives.

If it fails, the idea is wrong and that was learned in one milestone rather than three.

Speed is explicitly *not* part of the v0.1 criterion: importing the Terragrunt library
inherits its parsing cost. The speed claim belongs to the milestone that replaces it
with an own parser that parses each include once.

---
*Last updated: 2026-09-01 after initialization*
