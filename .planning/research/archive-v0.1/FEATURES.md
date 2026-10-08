# Feature Research: Terragrunt Built-in HCL Functions

**Domain:** Static analysis of Terragrunt repositories — offline, no external processes, no network
**Researched:** 2026-09-01
**Confidence:** MEDIUM-HIGH (function catalogue and semantics verified against current official docs and Go package surface; prevalence claims are MEDIUM/LOW — no authoritative usage telemetry exists publicly, see Sources)

---

## Lead Finding: The Minimum Viable Function Set for v0.1 (Question 5)

**The single most important discovery of this research: v0.1 does not need to evaluate `dependency.X.outputs.Y` as a VALUE at all.**

`GRT001` ("does output `Y` exist on the module `X` points to?") is a **name-existence check**, not a
value computation. It needs:
1. The **graph edge**: which unit does `dependency "X"` point to? → requires resolving `config_path`.
2. The **identifier**: what is `Y` in the expression `dependency.X.outputs.Y`? → this is read off the
   HCL **traversal syntax** in the `inputs` block's expression AST, not evaluated.

Neither of these requires running `terragrunt output`, touching Terraform state, or evaluating
`mock_outputs`. This means **STATE-DEPENDENT functions are entirely out of scope for v0.1** — not
because they've been suppressed, but because the check gruntled ships first structurally never reaches
them.

### The minimum function set gruntled v0.1 must evaluate

To build the unit graph (resolve `include` targets and `dependency.config_path` values), gruntled needs
working, PURE implementations of exactly these:

| Function | Why it's in the minimum set |
|---|---|
| `find_in_parent_folders(name, [fallback])` | Near-universal in `include.path`; walks the tree to locate root config |
| `path_relative_to_include([name])` | Common in root config's `generate`/`remote_state` blocks; rarely gates `config_path` itself, but must not error the parse |
| `path_relative_from_include([name])` | Used to build `dependency.config_path` as `"${path_relative_from_include()}/../vpc"` in some conventions |
| `get_terragrunt_dir()` | Common in `config_path = "${get_terragrunt_dir()}/../vpc"` patterns |
| `get_parent_terragrunt_dir([name])` | Same family, used when composing paths from the root |
| `get_original_terragrunt_dir()` | Rare, but needed when `read_terragrunt_config` is chained |

Plus, structurally (not "functions" but required parser behavior):
- **`include` resolution** with `merge_strategy` semantics (`no_merge` / `shallow` / `deep`) and
  `expose` — needed because `config_path` and `inputs` may be inherited from a parent config.
- **AST-level traversal extraction** for `dependency.<label>.outputs.<attr>` inside `inputs` —
  implemented as a walk over the HCL expression tree, not as HCL evaluation.
- **Static string literal / simple interpolation resolution** for `config_path` — most real configs
  write `config_path = "../vpc"` or `config_path = find_in_parent_folders("vpc")`-style expressions
  built only from the path functions above plus string concatenation. No repo pattern found in research
  builds `config_path` from `run_cmd`, `get_env`, or AWS identity functions (semantically those return
  account IDs / secrets / command output, not filesystem paths — see Question 2).

Everything else in the catalogue below (`get_env`, `run_cmd`, `sops_decrypt_file`, AWS identity
functions, `read_terragrunt_config`, `deep_merge`, native OpenTofu functions in `inputs` values) affects
only **`inputs` VALUES**, which v0.1 never evaluates. This is the deferral answer to Question 5's second
half: **defer everything that does not gate `config_path`/`include.path` resolution.**

**Practical implication for the architecture:** gruntled should request a **partial parse** — decode
only `IncludeBlock` + `DependencyBlock` (+ `TerragruntFlags` if needed) via the upstream library's
`PartialParseConfigFile` / `PartialDecodeSectionType` mechanism, combined with
`ParsingContext.WithSkipOutputsResolution()` — rather than the library's full `ParseConfigFile`, which
decodes and evaluates the entire body including `inputs`, `locals` used only for tagging, and any
`run_cmd`/`sops_decrypt_file` calls anywhere in the file. This was verified as a real, exported
capability of `github.com/gruntwork-io/terragrunt/config` (HIGH confidence for the mechanism's
existence; MEDIUM confidence on exact call signatures — verify against the pinned dependency version
before implementation, see Gaps).

---

## Function Catalogue

Terragrunt's function set has two layers: (1) all native OpenTofu/Terraform built-in functions
(`jsonencode`, `merge`, `file`, `templatefile`, `timestamp`, `uuid`, etc. — inherited, not
Terragrunt-specific), and (2) 32 Terragrunt-specific functions. This catalogue covers layer 2 in full,
plus a note on layer 1's determinism hazard.

Source for the full list and signatures: [Terragrunt built-in functions reference](https://docs.terragrunt.com/reference/hcl/functions/) (current docs, fetched 2026-09-01). Confidence: HIGH for names/signatures/returns (official docs); MEDIUM for exact implementation mechanics (execution model, caching) where cross-checked against GitHub issues rather than source code directly.

### Path & Directory Functions — all PURE

| Function | Signature | Returns | Class | Prevalence |
|---|---|---|---|---|
| `find_in_parent_folders` | `(name string, fallback? string) → string` | absolute path of first match walking up the tree | **PURE** | Near-universal — the canonical way to locate root config |
| `path_relative_to_include` | `(name? string) → string` | relative path from included file to current config | **PURE** | Near-universal — standard `generate "backend"` / `remote_state` pattern in every Gruntwork-style repo |
| `path_relative_from_include` | `(name? string) → string` | inverse relative path | **PURE** | Common, used to build `config_path` in `dependency` blocks |
| `get_terragrunt_dir` | `() → string` | dir containing current `terragrunt.hcl` | **PURE** | Common |
| `get_working_dir` | `() → string` | dir where Terraform/OpenTofu would run | **PURE** | Rare — mostly relevant to generated code, not graph structure |
| `get_parent_terragrunt_dir` | `(name? string) → string` | dir of root/parent config | **PURE** | Common in multi-include repos |
| `get_original_terragrunt_dir` | `() → string` | dir of the originally-invoked config, across `read_terragrunt_config` chains | **PURE** | Rare |

### Git Repository Functions — SIDE-EFFECTING in Terragrunt's implementation, but PURE in principle

| Function | Signature | Returns | Class | Prevalence |
|---|---|---|---|---|
| `get_repo_root` | `() → string` | absolute path to git repo root | **SIDE-EFFECTING*** | Common in monorepo-style path composition |
| `get_path_from_repo_root` | `() → string` | relative path from repo root to current dir | **SIDE-EFFECTING*** | Occasional |
| `get_path_to_repo_root` | `() → string` | relative path to repo root | **SIDE-EFFECTING*** | Occasional |

\* Verified (MEDIUM confidence, via [GitHub issue #2873](https://github.com/gruntwork-io/terragrunt/issues/2873) — `"git" executable not found in $PATH` error, and [issue #5976](https://github.com/gruntwork-io/terragrunt/issues/5976) referencing `git rev-parse --show-toplevel` wrapped in `filepath.FromSlash()`): Terragrunt's own implementation **shells out to the `git` binary**. Under gruntled's "no external processes" constraint this classifies as SIDE-EFFECTING if gruntled reuses Terragrunt's implementation unmodified.

**Important design note:** the *answer* these functions compute (nearest ancestor directory containing
`.git`) is fully derivable from the file tree alone — it does not need process execution in principle.
**gruntled should reimplement these three functions as PURE, walking up for a `.git` entry itself**,
rather than inheriting Terragrunt's git-shell-out behavior. This converts three otherwise-blocking
functions into part of the safe, evaluable set at near-zero cost. Flagged as a build decision, not just
a classification.

### Environment & Platform Functions

| Function | Signature | Returns | Class | Prevalence |
|---|---|---|---|---|
| `get_env` | `(name string, default? string) → string` | env var value or default; **errors** if absent and no default | **ENVIRONMENT** | Near-universal — used for account/region parameterization, CI variables |
| `get_platform` | `() → string` | `"darwin"` \| `"linux"` \| `"windows"` \| `"freebsd"` of the *executing machine* | **ENVIRONMENT** (host-dependent, not repo-dependent) | Rare |

`get_env` trade-off (per Question asked): evaluating it against the real process environment makes
gruntled's output depend on *who runs it and where* — violates the "same input, same diagnostics"
determinism guarantee in `PROJECT.md`. Two options: (a) never read real env vars, always fall back to
`default` if present else mark the consuming value `unknown`; (b) offer an explicit `--env-file` /
declared-environment mode for users who want full evaluation, clearly separated from the default
zero-config path. Recommendation: **(a) for v0.1** — `get_env` calls without a `default` argument should
mark the unit `unknown` for any check that depends on the resulting value; calls **with** a `default`
are the common case and evaluate to the literal default (still PURE from gruntled's point of view, since
the default is a string literal in the file).

### AWS Identity Functions — all SIDE-EFFECTING (network / cloud API)

| Function | Signature | Returns | Class | Prevalence |
|---|---|---|---|---|
| `get_aws_account_id` | `() → string` | current AWS account ID via API call | **SIDE-EFFECTING** | Common in multi-account repos (used in `inputs`/tags, not graph structure) |
| `get_aws_account_alias` | `() → string` | account alias or `""` | **SIDE-EFFECTING** | Occasional |
| `get_aws_caller_identity_arn` | `() → string` | caller ARN | **SIDE-EFFECTING** | Occasional |
| `get_aws_caller_identity_user_id` | `() → string` | caller user ID | **SIDE-EFFECTING** | Rare |

These require live AWS credentials and network access — categorically impossible offline, no trade-off
to consider (unlike `get_env`). Always suppress; mark consuming unit `unknown`.

### Terraform/OpenTofu Invocation-Context Functions — PURE but contextless in gruntled

| Function | Signature | Returns | Class | Prevalence |
|---|---|---|---|---|
| `get_terraform_command` | `() → string` | name of the tf/tofu command currently being wrapped | **PURE**, but N/A | Rare in graph-relevant code — used in conditional `retryable_errors`/`iam_role` logic |
| `get_terraform_cli_args` | `() → string` | CLI args passed through | **PURE**, N/A | Rare |
| `get_terraform_commands_that_need_vars` | `() → list(string)` | fixed built-in list | **PURE** (constant) | Rare, used in `extra_arguments` blocks |
| `get_terraform_commands_that_need_input` | `() → list(string)` | fixed built-in list | **PURE** (constant) | Rare |
| `get_terraform_commands_that_need_locking` | `() → list(string)` | fixed built-in list | **PURE** (constant) | Rare |
| `get_terraform_commands_that_need_parallelism` | `() → list(string)` | fixed built-in list | **PURE** (constant) | Rare |
| `get_default_retryable_errors` | `() → list(string)` | fixed built-in list | **PURE** (constant) | Rare |
| `get_terragrunt_source_cli_flag` | `() → string` | value of `--source`/`TG_SOURCE` | **ENVIRONMENT**-ish (CLI-flag-dependent) | Rare |

`get_terraform_command` and `get_terraform_cli_args` are honestly **undefined** in gruntled's context —
gruntled never invokes tofu/terraform, so "the command currently being wrapped" has no real answer.
Recommendation: return a fixed sentinel (empty string / `"plan"`) since in practice these gate
retry/locking logic inside `extra_arguments` or `remote_state`, not `config_path`/`include.path` — so
they never block v0.1's minimum set regardless of the sentinel chosen.

### Command Execution — SIDE-EFFECTING, must suppress

| Function | Signature | Returns | Class | Prevalence |
|---|---|---|---|---|
| `run_cmd` | `(command string, args ...string, [--terragrunt-quiet], [--terragrunt-global-cache], [--terragrunt-no-cache]) → string` | stdout of the executed command | **SIDE-EFFECTING** | Common but not universal — typically used for `git rev-parse` (version tags), timestamp generation, or wrapper scripts, feeding `inputs`/`locals`, essentially never `config_path` |

Verified (MEDIUM confidence, [docs.terragrunt.com functions page](https://docs.terragrunt.com/reference/hcl/functions/)): results are **cached** per directory+command by default (`--terragrunt-no-cache` disables caching, forcing re-execution; `--terragrunt-global-cache` makes the cache directory-independent). No public CLI/env flag was found in the docs that **globally disables** `run_cmd` execution — this is an **UNVERIFIED gap**: whether the underlying Go library exposes a programmatic way to no-op `run_cmd` when imported (as opposed to shelled out via the `terragrunt` CLI binary, which gruntled will never invoke) needs source-level verification against the pinned `github.com/gruntwork-io/terragrunt` version before M1 implementation. `ParsingContext`'s partial-decode mechanism (see Lead Finding) is gruntled's actual mitigation — by never decoding `inputs`/`locals` sections that reference `run_cmd`, the function is never reached, sidestepping the need for a disable flag entirely for v0.1's scope.

### Data Reading & Merging — PURE, but recursively contingent

| Function | Signature | Returns | Class | Prevalence |
|---|---|---|---|---|
| `read_terragrunt_config` | `(path string, default_val? any) → map` | parsed map of another config's blocks/attributes | **PURE**, contingent | Occasional — used for cross-referencing sibling configs |
| `read_tfvars_file` | `(path string) → map` | parsed `.tfvars`/`.tfvars.json` | **PURE** | Occasional |
| `deep_merge` | `(map1, map2, ... ) → map` | recursively merged map (requires `deep-merge` experiment flag) | **PURE** | Rare (experimental, opt-in) |

`read_terragrunt_config` reads and evaluates *another* HCL file wholesale — its classification is only
as safe as the target file's contents. If the target file contains `run_cmd`/`get_env`/AWS-identity
calls anywhere Terragrunt eagerly evaluates (see the ternary-eagerness note below), the read poisons the
calling unit too. Treat as PURE-if-clean, propagate `unknown` otherwise — same rule as everything else.

### File Tracking & Validation — all PURE

| Function | Signature | Returns | Class | Prevalence |
|---|---|---|---|---|
| `mark_as_read` | `(path string) → string` | the path itself, side channel: registers file for change-queue tracking | **PURE** | Rare — advanced queue/CI patterns |
| `mark_glob_as_read` | `(pattern string, [--terragrunt-boundary=dir]) → list(string)` | matching paths | **PURE** | Rare |
| `constraint_check` | `(version string, constraint string) → bool` | whether version satisfies constraint | **PURE** | Rare |

### Encryption/Decryption — SIDE-EFFECTING, must suppress

| Function | Signature | Returns | Class | Prevalence |
|---|---|---|---|---|
| `sops_decrypt_file` | `(path string) → string` | decrypted file content (YAML/JSON/INI/ENV/raw) | **SIDE-EFFECTING** | Common in teams using secrets-in-HCL; near-universal in repos with sensitive `inputs` |

Verified (MEDIUM confidence, [GitHub issue #2172](https://github.com/gruntwork-io/terragrunt/issues/2172)): decryption calls out to KMS/Key Vault/Vault/PGP backends — always network- and credential-dependent, categorically SIDE-EFFECTING, no offline path exists. Critically, the same issue confirms **eager evaluation inside ternary expressions**: `condition ? sops_decrypt_file(a) : sops_decrypt_file(b)` attempts to decrypt **both branches**, even the one not selected, because HCL's `gohcl`-based full-body decode evaluates all referenced expressions regardless of branch selection (this does *not* apply inside `for` loops with an `if` filter, which are lazy per-element). **This directly validates the partial-parse strategy in the Lead Finding**: the only reliable way to avoid tripping `sops_decrypt_file`/`run_cmd` is to never decode the section of the file that references them, not to rely on conditional branches being "unreachable."

### Native OpenTofu/Terraform Functions (inherited layer) — mostly PURE, two determinism hazards

Not Terragrunt-specific, but available anywhere Terragrunt evaluates HCL (`locals`, `inputs`, etc.).
Functions like `jsonencode`, `merge`, `concat`, `file`, `templatefile`, `formatdate`, `base64encode`,
`yamldecode` are **PURE** (local computation or local file reads, deterministic). Two are **not**
deterministic even though they touch no external system: `timestamp()` and `uuid()` (and `bcrypt()`
non-deterministically salts). These violate gruntled's "same input → same output" invariant if
evaluated — classify as a fourth informal bucket, **NON-DETERMINISTIC**, and treat identically to
SIDE-EFFECTING for gruntled's purposes (suppress, mark `unknown`) even though no I/O occurs. UNVERIFIED
whether OpenTofu's function set as vendored by Terragrunt differs meaningfully from vanilla
OpenTofu — treat as HIGH confidence given both projects share the `zclconf/go-cty` functions
implementation lineage.

---

## Answers to Research Questions

### Question 1 — Prevalence

No public usage-frequency telemetry exists for Terragrunt function calls across real repositories (this
is an honest gap, not an oversight — Terragrunt is a stateless CLI with no client analytics, and no
third party publishes a corpus-wide function-frequency study as of this research date). The prevalence
column above is a **reasoned MEDIUM/LOW-confidence estimate** based on: (a) the structure of Gruntwork's
own reference-architecture patterns (`find_in_parent_folders`, `path_relative_to_include` appear in
nearly every published example root `terragrunt.hcl`), (b) the semantic purpose of each function (path
functions are structurally required by the include/inherit pattern that *is* Terragrunt's core value
proposition; AWS-identity and `run_cmd` are optional conveniences layered on top), and (c) GitHub issue
volume as a rough proxy for how often a function is used-and-hit-a-bug. **Near-universal:**
`find_in_parent_folders`, `path_relative_to_include`, `get_env` (with default), `get_terragrunt_dir`.
**Common:** `path_relative_from_include`, `get_parent_terragrunt_dir`, `sops_decrypt_file` (in
secrets-conscious orgs), `get_aws_account_id`, `run_cmd`. **Rare:** everything in the "Terraform
invocation-context," "File tracking," and "Validation" tables above, plus `deep_merge` (opt-in
experiment) and `get_original_terragrunt_dir`. This should be treated as a **testable hypothesis**, not
a fact — the synthetic-repository generator and real-repository validation already planned for v0.1
(per `PROJECT.md`) are the correct place to confirm or refute it empirically.

### Question 2 — Fraction of a repo becoming `unknown` if `run_cmd`/`sops_decrypt_file` can't be evaluated

**Lower than intuition suggests, likely near-zero for v0.1's actual scope, for a structural reason:**
`run_cmd` and `sops_decrypt_file` overwhelmingly feed `inputs` values (tags, secrets, computed version
strings) — data that flows *into* Terraform, never data that gates *which file `include`/`dependency`
points to*. Filesystem paths and side-effecting-function outputs are semantically disjoint categories no
real repo pattern was found to blend (composing a `config_path` from an AWS account ID or a decrypted
secret would be a very unusual, arguably broken pattern). Because v0.1's graph construction and `GRT001`
only need `config_path`/`include.path` resolution plus AST-level name extraction (see Lead Finding), a
`run_cmd` present in the SAME root config included by 100% of units does not by itself poison 100% of
units — **as long as gruntled partial-parses**, decoding only `IncludeBlock`/`DependencyBlock` sections
and never touching `inputs`/unrelated `locals`. If gruntled instead naively calls the upstream library's
full `ParseConfigFile` (decoding everything, including `inputs`), then yes — because of the eager
ternary-evaluation behavior confirmed in Question 3's source, a single `run_cmd`/`sops_decrypt_file`
anywhere in a root config used by every unit would poison the entire repository, since HCL evaluates all
referenced expressions in a decoded body regardless of which branch is logically selected. **The tool
remains useful either way for v0.1's one diagnostic**, because that diagnostic doesn't need those
functions — but the *engineering choice* between partial-parse and full-parse is the difference between
"works on 100% of realistic repos" and "silently degrades to `unknown` on all of them the moment one root
config uses `run_cmd` for a git-sha tag," which is common. This is the most consequential
implementation decision surfaced by this research.

### Question 3 — `include` blocks: `expose`, `merge_strategy`, multiple/nested includes

Verified against current docs (MEDIUM-HIGH confidence, `docs.terragrunt.com`):

- **`expose = true`** makes the *entire* included config available as `include.<name>.*` in the child,
  without merging it. This is orthogonal to graph resolution — it only matters if `config_path` or
  `path` expressions reference `include.<name>.something`, which does happen (e.g.
  `config_path = "${include.root.locals.base_dir}/vpc"`). gruntled's partial-decode of `IncludeBlock`
  must therefore resolve exposed values transitively, not just the include's own path.
- **`merge_strategy`**: `no_merge` (parent loaded, not merged — only `expose`d values visible),
  `shallow` (default; top-level replace, except `dependencies` blocks always deep-merge), `deep`
  (recursive merge; `remote_state` and `generate` blocks are shallow even under `deep` due to
  implementation limits). This matters for resolving the *effective* `dependency`/`inputs` set a unit
  ends up with — a child's `dependency` block can be entirely inherited from a parent under `deep`
  merge, meaning gruntled cannot determine a unit's full dependency set by reading its own file alone;
  it must resolve the include chain first.
- **Multiple includes**: supported, each requires a unique label (`include "root" {}`, `include
  "region" {}`), each independently merged per its own `merge_strategy`.
- **Nested includes are NOT supported** — "Terragrunt only supports a single level of `include`
  blocks"; a parent config that itself has an `include` block errors. This bounds gruntled's
  include-resolution recursion depth to exactly 2 (child → parent), simplifying the shared-parse-once
  design in the project's Key Decisions table — there is no unbounded include chain to worry about.
- **Critical ordering constraint** (directly relevant to Question 5): if the **parent** config contains
  `dependency` blocks, Terragrunt restricts what's available to the child's `include`/`dependency`
  blocks to only the parent's `locals` and `include` — explicitly to avoid a circular bootstrapping
  problem (you can't need an applied dependency's output to figure out your own dependency graph).
  gruntled should mirror this exact restriction rather than trying to be cleverer than Terragrunt here;
  it's an existing, documented invariant, not a gruntled invention.

### Question 4 — `terragrunt.stack.hcl` / units-and-stacks

Verified (MEDIUM confidence — docs confirm stability, exact adoption data absent): stacks are a
**stable** feature as of Terragrunt v1.0 (not experimental), with two forms — **implicit stacks**
(plain directory organization, always been possible, not a new function surface) and **explicit
stacks** (`terragrunt.stack.hcl`, a genuinely new file type that generates a `.terragrunt-stack/`
directory containing real, generated `terragrunt.hcl` + `terragrunt.values.hcl` files via `terragrunt
stack generate`).

**Recommendation: defer explicit `terragrunt.stack.hcl` parsing to v1.x/v2, not v0.1**, for two
independent reasons:
1. **It generates real files.** `terragrunt stack generate` materializes ordinary `terragrunt.hcl`
   units on disk inside `.terragrunt-stack/`. If a repository has already run `stack generate` (the
   generated directory is typically present, sometimes gitignored, sometimes committed), gruntled's
   existing file-tree walk discovers those units for free — no special-case code needed. Only repos
   that check in `terragrunt.stack.hcl` *without* a committed generated output would be invisible to
   v0.1, and gruntled would silently under-report (fewer units found) rather than mis-report (a false
   positive) — consistent with the zero-false-positive rule, since undiscovered units are absent from
   the graph rather than incorrectly analyzed.
2. **Adoption is unverified as high.** No evidence found of `terragrunt.stack.hcl` being prevalent in
   existing production repositories as of this research date; it's a v1.0-era feature. Building a
   `values.hcl`-aware, stack-block-aware parser before validating the core `GRT001` value proposition
   would front-load complexity against the project's own stated principle: "v0.1 ships GRT001 + syntax
   only... more checks before the idea is validated means more bug surface."

**Action for v0.1**: explicitly document in the README/known-limitations that repositories using
explicit stacks without committing generated output will have incomplete graphs (fewer units than
expected), and treat this as a documented gap, not a silent failure — consistent with "when not certain,
mark unknown," extended here to "when a unit can't be discovered at all, it's simply absent, not
misrepresented."

---

## Feature Landscape (reframed for a function-classification research task)

### Table Stakes — functions gruntled MUST correctly evaluate for v0.1's graph

| Feature | Why Expected | Complexity | Notes |
|---|---|---|---|
| `find_in_parent_folders` resolution | Nearly every repo uses it to locate root config | LOW | Pure filesystem walk |
| `path_relative_to_include` / `path_relative_from_include` | Standard pattern for `config_path` composition | LOW | Pure string/path math |
| `get_terragrunt_dir` / `get_parent_terragrunt_dir` / `get_original_terragrunt_dir` | Common in path composition | LOW | Pure |
| `include` merge semantics (`no_merge`/`shallow`/`deep`, `expose`) | Determines a unit's *effective* `dependency`/`inputs` set | MEDIUM | Single-level nesting only — bounded recursion |
| AST-level `dependency.<label>.outputs.<attr>` traversal extraction | This IS the GRT001 check | MEDIUM | Must not require evaluating the surrounding expression |
| Partial-parse (decode only `IncludeBlock`/`DependencyBlock`) | Avoids triggering `run_cmd`/`sops_decrypt_file`/env reads that live in unrelated `inputs`/`locals` | MEDIUM-HIGH | The central architectural decision from Question 2 |

### Differentiators — graceful degradation, not evaluation completeness

| Feature | Value Proposition | Complexity | Notes |
|---|---|---|---|
| Unit-level `unknown` marking on unresolvable `config_path` | Zero-false-positive guarantee preserved even on repos using `run_cmd`-in-path patterns (rare, but must not crash or misreport) | LOW | Ten false negatives beat one false positive |
| Reimplemented (non-shelling) `get_repo_root` family | Converts 3 otherwise SIDE-EFFECTING functions into PURE, expanding the safely-evaluable set beyond what Terragrunt itself offers offline | LOW | Direct differentiator vs. naively reusing the upstream library |
| `--terragrunt-quiet`/eager-ternary awareness | Correctly avoids executing `run_cmd`/`sops_decrypt_file` even when they appear inside conditional expressions gruntled's partial-parse happens not to reach | MEDIUM | Defense in depth beyond partial-parse alone |

### Anti-Features — functions/behaviors gruntled should explicitly NOT attempt

| Feature | Why Requested | Why Problematic | Alternative |
|---|---|---|---|
| Real evaluation of `get_env` without a `default` | "Just read the actual environment, like Terragrunt does" | Breaks determinism guarantee (same input → same output regardless of *who* runs it); also a security-adjacent behavior to avoid by default | Mark consuming value `unknown`; require explicit opt-in mode later if ever |
| Full evaluation of `run_cmd`/`sops_decrypt_file`/AWS identity functions | "Terragrunt does it, so results would be 'more accurate'" | Directly violates the no-external-process, no-network, air-gap architectural guarantees that are gruntled's core differentiator | Suppress; mark `unknown`; the graph/GRT001 check never needed the value anyway |
| Full `terragrunt.stack.hcl` generation/expansion in v0.1 | "Complete coverage from day one" | Front-loads a parser for a feature of unverified prevalence before the core value prop (GRT001) is validated | Defer; rely on file-tree discovery of already-generated units; document the gap |
| Treating unreached ternary branches as "safe not to evaluate" | Seems like free safety — "it's inside an `if`, so it won't run" | Confirmed false for full-body HCL decode: both branches of a ternary are eagerly evaluated | Partial-parse at the section level, not branch-level trust |

## Feature Dependencies

```
find_in_parent_folders / get_terragrunt_dir / path_relative_to_include family (PURE)
    └──requires──> nothing (filesystem-tree only)

include resolution (merge_strategy + expose)
    └──requires──> path functions above (to locate the included file)
    └──enables───> effective config_path / dependency set for a unit

dependency.config_path resolution
    └──requires──> include resolution (config_path may be inherited)
    └──requires──> path functions (config_path often composed from them)
    └──enables───> graph edge (unit → unit)

GRT001 check (dependency.X.outputs.Y name existence)
    └──requires──> graph edge (must know which module X resolves to)
    └──requires──> AST traversal extraction of "Y" (NOT value evaluation)
    └──does NOT require──> run_cmd / get_env / sops_decrypt_file / AWS identity / terraform state

partial-parse strategy (IncludeBlock + DependencyBlock only)
    └──enables───> avoiding run_cmd/sops_decrypt_file/get_env entirely for v0.1
    └──conflicts with──> naive use of upstream library's full ParseConfigFile
```

### Dependency Notes

- **GRT001 does not require dependency output VALUES**, only the graph edge and the AST-extracted
  identifier — this is the single fact that makes v0.1 tractable offline (see Lead Finding).
- **`include` resolution must happen before `dependency.config_path` can be trusted**, because
  `merge_strategy = deep` can inherit `dependency` blocks wholesale from the parent; reading a child
  unit's file in isolation is insufficient.
- **Partial-parse conflicts with the "import the library wholesale" path** described in `PROJECT.md`'s
  Key Decisions table (`Import gruntwork-io/terragrunt as a library in M1`). This is not a
  contradiction to resolve now but a design detail to get right during M1: import the library for its
  *parsing primitives* (`PartialParseConfigFile`, `PartialDecodeSectionType`,
  `WithSkipOutputsResolution()`), not necessarily its top-level `ParseConfigFile` entry point, which
  decodes and evaluates everything.

## MVP Definition

### Launch With (v0.1)

- [ ] All 6 PURE path/directory functions (`find_in_parent_folders`, `path_relative_to_include`,
      `path_relative_from_include`, `get_terragrunt_dir`, `get_parent_terragrunt_dir`,
      `get_original_terragrunt_dir`) — required for `config_path`/`include.path` resolution
- [ ] `include` block resolution: `merge_strategy` (all 3 values), `expose`, single-level nesting only
- [ ] AST-level extraction of `dependency.<label>.outputs.<attr>` traversals from `inputs` — no
      evaluation, no `mock_outputs`, no `terragrunt output`
- [ ] Partial-parse strategy (decode `IncludeBlock`/`DependencyBlock` only) to structurally avoid
      `run_cmd`/`get_env`/`sops_decrypt_file`/AWS-identity functions for the vast majority of repos
- [ ] Reimplemented (non-git-shelling) `get_repo_root`/`get_path_from_repo_root`/`get_path_to_repo_root`
      — small effort, expands the safe function set
- [ ] `unknown` marking for any unit whose `config_path`/`include.path` resolution hits an
      unresolvable/side-effecting expression

### Add After Validation (v1.x)

- [ ] `get_env` with an explicit opt-in "read real environment" mode, clearly separated from default
      offline behavior — triggered if real repos commonly gate `config_path` on env vars (currently
      believed rare, unverified)
- [ ] `read_terragrunt_config` chain resolution, if `GRT005`/`GRT006` (inputs-vs-variables) are
      implemented and need cross-config value reading
- [ ] `terragrunt.stack.hcl` awareness — parse the stack file directly (not just discover generated
      output) — triggered if real-repo validation surfaces stacks-without-committed-output as common
- [ ] `mock_outputs`-aware STATE-DEPENDENT evaluation — only needed if a future diagnostic requires
      actual output *values*, not just names (no current v0.1/v1.x diagnostic needs this)

### Future Consideration (v2+)

- [ ] Any real evaluation of SIDE-EFFECTING functions (`run_cmd`, `sops_decrypt_file`, AWS identity) —
      would require an explicit, loudly-flagged opt-in "trust mode" that breaks the air-gap guarantee;
      likely never, given it contradicts the project's core positioning
- [ ] `deep_merge` support — experimental upstream feature (`deep-merge` flag), low prevalence
- [ ] `constraint_check`, `mark_as_read`/`mark_glob_as_read` — narrow, low-prevalence functions with no
      graph impact

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---|---|---|---|
| Path/directory function family (PURE) | HIGH | LOW | P1 |
| `include` merge_strategy/expose resolution | HIGH | MEDIUM | P1 |
| AST-level dependency-output-name extraction | HIGH | MEDIUM | P1 |
| Partial-parse (avoid full body decode) | HIGH | MEDIUM-HIGH | P1 |
| Reimplemented git-root functions (non-shelling) | MEDIUM | LOW | P2 |
| `get_env` opt-in real-environment mode | LOW (unverified need) | MEDIUM | P3 |
| `terragrunt.stack.hcl` direct parsing | LOW (unverified prevalence) | HIGH | P3 |
| STATE-DEPENDENT (`mock_outputs`, real state) evaluation | LOW (no current diagnostic needs it) | HIGH | P3 |

**Priority key:** P1: must have for v0.1's falsifiable experiment. P2: should have, cheap and expands
safe-evaluation coverage. P3: defer until real-repo validation shows it's needed.

## Sources

- [Terragrunt built-in functions reference](https://docs.terragrunt.com/reference/hcl/functions/) — HIGH confidence, official docs, fetched 2026-09-01 (canonical URL; old `terragrunt.gruntwork.io` host 308-redirects here)
- [Terragrunt `include` block reference](https://docs.terragrunt.com/reference/hcl/blocks/#include) — HIGH confidence, official docs
- [Terragrunt `dependency` block reference](https://docs.terragrunt.com/reference/hcl/blocks/#dependency) — HIGH confidence, official docs
- [Terragrunt stacks feature overview](https://docs.terragrunt.com/features/stacks/) — MEDIUM confidence, official docs, stability claim cross-checked
- [Gruntwork Blog — The Road to 1.0: Terragrunt Stacks Feature Complete](https://www.gruntwork.io/blog/the-road-to-1-0-terragrunt-stacks-feature-complete) — MEDIUM confidence
- [`terragrunt stack generate` CLI reference](https://terragrunt.gruntwork.io/docs/reference/cli/commands/stack/generate) — MEDIUM confidence, confirms `.terragrunt-stack/` + `terragrunt.values.hcl` generation behavior
- [GitHub issue #2873 — `get_repo_root` shells out to `git`](https://github.com/gruntwork-io/terragrunt/issues/2873) — MEDIUM confidence, confirms process-execution implementation
- [GitHub issue #5976 — `get_repo_root` uses `git rev-parse --show-toplevel`](https://github.com/gruntwork-io/terragrunt/issues/5976) — MEDIUM confidence
- [GitHub issue #2172 — `sops_decrypt_file` eager ternary evaluation](https://github.com/gruntwork-io/terragrunt/issues/2172) — MEDIUM confidence, directly validates the partial-parse strategy
- [`github.com/gruntwork-io/terragrunt/config` Go package docs](https://pkg.go.dev/github.com/gruntwork-io/terragrunt/config) — MEDIUM confidence; confirms `PartialParseConfigFile`, `PartialDecodeSectionType`, `ParsingContext.WithSkipOutputsResolution()` exist as public API. **Exact function signatures UNVERIFIED against the specific version gruntled will pin — re-check against `go.mod`-pinned version at M1 implementation start.**
- Prevalence estimates throughout: **LOW-MEDIUM confidence, reasoned, not empirically measured** — no public corpus-wide usage study exists; flagged explicitly as a hypothesis for the project's own planned synthetic-generator + real-repo validation to confirm or refute.

---
*Feature research for: gruntled (Terragrunt static analyser, offline function evaluation)*
*Researched: 2026-09-01*
