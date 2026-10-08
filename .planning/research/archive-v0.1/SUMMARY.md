# Research Summary

**Date:** 2026-09-01
**Confidence:** HIGH — the two decision-changing findings were re-verified by hand
(compiling against the library; cloning and inspecting the corpus), not taken on the
researchers' word.

## The finding that changed the plan

**Terragrunt cannot be used as a Go library.** `ParseConfigFile` and
`PartialParseConfigFile` are exported and importable, but the only `*ParsingContext`
constructor requires `*venv.Venv` from `internal/venv` — unimportable from a foreign
module, obtainable from no exported function, and `nil` panics. Importing would also pull
672 modules including full AWS/Azure/GCP SDKs and require Go 1.27.

Two researchers disagreed on this; one recommended using the library's partial-parse API.
The disagreement was settled by compiling. See the correction note at the top of STACK.md.

**Consequence:** gruntled writes its own structural HCL decoder from M1. This is smaller
than it sounds, and it hands back the speed claim.

## Why the own parser is tractable

`GRT001` compares **names read off the HCL AST** — it never evaluates an expression to a
value. State, `mock_outputs` and most Terragrunt functions therefore leave scope *by
construction*, not by suppression.

The minimum function set for v0.1 is **six pure path functions**:
`find_in_parent_folders`, `path_relative_to_include`, `path_relative_from_include`,
`get_terragrunt_dir`, `get_parent_terragrunt_dir`, `get_original_terragrunt_dir` —
plus `include` merge resolution.

Functions that shell out, read the environment, decrypt or reach the network are **never
implemented at all**. Any unit using one is marked `unknown`.

Note: `get_repo_root` and friends shell out to `git` in Terragrunt's implementation, but
the answer is derivable from the file tree, so gruntled can implement them purely.

## Stack

| Concern | Choice | Basis |
|---|---|---|
| HCL parsing | `hashicorp/hcl/v2` v2.24.0 | The version Terragrunt itself pins |
| Module surface | `terraform-config-inspect` | Verified maintained, MPL-2.0, no `init`, no network. Returns variable/output *type source text*, not `cty.Type` — sufficient for name checks, a gap for later type checks |
| Remote source detection | `go-getter.Detect` (pure, offline) plus Terraform's rule that local paths start with `./` or `../` | Classification by construction, not heuristics |

## Corpus — verified by cloning

The obvious candidates are traps. `terragrunt-infrastructure-live-example` (the repo every
tutorial links) has **zero** `dependency` blocks in its whole history. Its official
replacement uses Stacks, which materialise units into a gitignored directory only after
`terragrunt stack generate` — invisible to a static walk.

| Repository | Licence | Role |
|---|---|---|
| `aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws` | MIT-0 | Primary. 65 units, 62 with `dependency`, all modules local |
| `cds-snc/secret` | MIT | Secondary cross-check |
| `denis256/terragrunt-tests` | MIT | Golden fixtures, robustness cases |

Hand-verified on the primary corpus: `dependency "s3"` → `config_path = "../s3_runtime"`
→ that module declares `output "region"` and `output "role_name"` → both references
resolve. `GRT001` correctly reports nothing.

**Structural surprise:** all 65 units omit `terraform { source }`. Terragrunt then runs
against the `.tf` files in the unit's own directory. The design assumed unit→module always
goes through `source`; both forms must be handled.

**No maintained repo contains a naturally occurring wiring bug** — `apply` catches them
before commit. Hence the mutation-based success criterion in PROJECT.md.

## Pitfalls to design against

1. **`mock_outputs` masks exactly the bug class `GRT001` targets.** Present in 11 files of
   the primary corpus. Its presence must not legitimise a missing output; its absence must
   not create an error. Needs an explicit, tested decision.
2. **The Unit-vs-Module two-hop trap.** A `dependency` points at a *unit*; the outputs live
   in the *module* that unit resolves to. Skipping the second hop gives wrong answers. A
   researcher hit this mid-research.
3. **`generate` blocks** can inject `.tf` files at run time, invisible to static analysis.
   Measured low risk for outputs, real risk for variables — relevant to later diagnostics.
4. **Unquoted HCL block labels are valid** and appear in production. Found twice.
5. **Directories to skip:** `.terragrunt-cache`, `.terraform`, vendored modules, symlinks.
6. **Determinism:** `filepath.WalkDir` is already ordered, so no sorting needed there — but
   absolute paths leaking into output is a real, unaddressed gap.

## Build order

Domain value objects → minimal fixtures → isolated adapters → the `GRT001` analyzer →
the Terragrunt anti-corruption layer → orchestration → CLI → scale, benchmark and
real-repo validation.
