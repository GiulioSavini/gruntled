# gruntled

> *Terragrunt, but gruntled.*

## Current Milestone: v0.2 CI-Ready

**Goal:** Make the validated `check` engine something a team can drop into pre-commit and CI
today, and add the two diagnostics the existing graph already answers.

**Target features:**
- `GRT002` (`config_path` to no unit) and `GRT003` (dependency cycle)
- `gruntled graph --json`
- SARIF output, pre-commit hook, CI recipes
- Tagged releases with static binaries

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

### Active

**v0.2 — CI-Ready**

- [ ] `GRT002`: `dependency.config_path` pointing at no unit
- [ ] `GRT003`: dependency cycle between units
- [ ] `gruntled graph --json`
- [ ] SARIF output accepted by GitHub code scanning
- [ ] pre-commit hook and CI recipes
- [ ] Tagged releases with static binaries

**Later**

- [ ] `gruntled watch`: daemon with in-memory incremental reindexing, status file, `gruntled report` (v0.3)
- [ ] `gruntled blast`: Broken vs Impacted (v0.3)
- [ ] `GRT004`-`GRT006`

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
  link `net` into the binary, which breaks the static no-network proof). `fsnotify` when
  the daemon arrives. No cloud SDKs.
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
| Structural decode only — never evaluate expression values | `GRT001` compares names read off the HCL AST; it never needs a value. This removes state, `mock_outputs` and most functions from scope by construction rather than by suppression | — Pending |
| v0.1 ships `GRT001` + syntax only | One diagnostic justifies the tool. More checks before the idea is validated means more bug surface and more false-positive risk | — Pending |
| v0.1 ships `check` only, no daemon | A daemon on top of an unverified engine is wasted work. `check` is itself the experiment, and is already useful in pre-commit and CI | — Pending |
| No on-disk index cache in v1 | Premature optimisation: versioning, invalidation and corruption traded against a sub-second startup | — Pending |
| Status file over desktop notifications | `notify-send` is absent on the developer's WSL2 machine; PowerShell toasts cost ~1s and break the no-external-process rule. A status file is readable from a prompt, tmux or an editor and works everywhere | — Pending |
| Impacted only when module surface changes | Reporting twelve units because a comment changed destroys trust in the signal | — Pending |
| Open the repository once M1 passes its test | If the experiment fails, nothing was published and nothing needs explaining | — Pending |
| Name: gruntled | Memorable, ownable, no relevant collision, and the joke carries the README | — Pending |
| mock_outputs never suppresses GRT001, and severity stays error | With merge-with-state, a renamed output silently falls back to the mock value at `apply`, which is exactly the bug gruntled exists to catch. Mock facts only enrich the message. `enabled = false`, `skip_outputs = true` or any non-literal value for either keeps GRT001 silent, because Terragrunt then never reads the module's outputs. Suppressing would also make the Phase 4 mutation run report 0/8 on the primary corpus | ✓ Locked (Phase 3) |
| v0.2 is adoption (CI, SARIF, releases, GRT002/003) before the daemon | The one-shot engine is validated but not installable. GRT002/003 are pure graph queries; GRT004 needs a diff (belongs with blast), GRT005/006 overlap `terragrunt hcl validate --inputs` and carry merge-related false-positive risk | — Pending |
| stdlib flag instead of cobra for v0.1 | One subcommand. cobra/pflag link `net`, `net/url` and `net/netip` plus `text/template`, while the stdlib keeps `binary-no-net-no-exec` a one-line CI proof. Revisit when v2 adds several subcommands | ✓ Locked (Phase 3) |

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

---
*Last updated: 2026-09-29 after starting milestone v0.2 CI-Ready*
