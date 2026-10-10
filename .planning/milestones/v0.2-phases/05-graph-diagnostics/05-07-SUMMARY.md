---
phase: 05-graph-diagnostics
plan: 07
subsystem: terragrunt-loader
tags: [gap-closure, grt002, grt003, config-path, oracle]
requires:
  - phase: 05-graph-diagnostics
    provides: "05-03 resolveTargetExpr, 05-04 dependencies paths, 05-05 goldens, 05-06 terragrunt oracle and open issue"
provides:
  - "Empty config_path semantics matching terragrunt v1.1.6 per case"
  - "resolveTargetExpr(..., emptyUnresolved bool): blocks pass true, paths pass false"
  - "grt002_missing_target golden: silent empty block, GRT003 self-loop on the paths \"\" entry"
affects: [phase-06, phase-07]
tech-stack:
  added: []
  patterns:
    - "Oracle-driven semantics: each corner case is decided by the pinned terragrunt v1.1.6 run on a scratch tree"
key-files:
  created:
    - .planning/phases/05-graph-diagnostics/05-07-SUMMARY.md
  modified:
    - internal/infrastructure/terragrunt/loader.go
    - internal/infrastructure/terragrunt/parse.go
    - internal/infrastructure/terragrunt/reasons.go
    - internal/infrastructure/terragrunt/loader_test.go
    - internal/infrastructure/terragrunt/pathdeps_test.go
    - cmd/gruntled/testdata/golden/grt002_missing_target.txtar
    - docs/validation.md
    - docs/cli.md
    - .planning/STATE.md
    - .planning/ROADMAP.md
key-decisions:
  - "Option A (user decision at the checkpoint): match terragrunt per case. A dependency block config_path \"\" is unresolved (config-path-empty) and silent; a dependencies paths entry \"\" resolves to the unit itself and is a GRT003 self-loop"
  - "This reverses the 05-04 rule that a paths entry \"\" is never a self-edge, and supersedes the 05-05 rule that a block \"\" is a self-loop"
  - "The empty check stays in resolveTargetExpr behind an emptyUnresolved flag (not a split helper): one evaluation, one place that documents all outcomes"
  - "elemLiteral returns `\"\"` for an empty string literal: the domain requires a non-empty PathDependency literal"
metrics:
  duration: "~65 min (incl. checkpoint)"
  completed: 2026-09-30
---

# Phase 5 Plan 7: Empty config_path gap closure Summary

Empty `config_path` now matches terragrunt v1.1.6 per case: a `dependency` block `""` is unresolved and silent, while a `dependencies { paths }` entry `""` is a self-edge reported as GRT003.

## What changed

- `resolveTargetExpr` takes an `emptyUnresolved` flag. `resolveOneDependency` (blocks) passes `true`, so `""` gives `ReasonConfigPathEmpty`. `filePathDependencies` passes `false`, so `""` resolves to the unit's own directory.
- `elemLiteral` keeps an empty literal as `""`. Without this, `NewPathDependency` would reject the self-edge.
- The `ReasonConfigPathEmpty` doc now says it covers dependency blocks only. The reason is still used, so there is no U1000.
- Tests: the loader covers blocks with plain `""`, `"${""}"` and `""` inherited through an include (all unresolved). `TestPathDepsEmptyIsSelf` covers paths with plain, mixed `["../b", ""]`, template and include-inherited forms (all self-edges). In `TestPathDependencies` the `""` element is now the self-edge `u|has-config|...|""`.
- Golden `grt002_missing_target`: GRT003 at `28:41` with the message `dependency cycle: "live/app" -> "live/app"`. Column checked by hand: 11 + 13 + 16 = 40. Exit code stays 1.

## Oracle (pinned terragrunt v1.1.6, tofu 1.12.6, scratch trees only)

Command: `run --all --non-interactive --no-auto-init --no-color --tf-path <bin>/tofu -- version`

| Tree | terragrunt | gruntled |
|------|------------|----------|
| blk: `dependency "x" { config_path = "" }` | `ERROR skipping dependency "x" ... config_path could not be resolved`, no cycle (run --all exit 1; single-unit run exit 0) | 0 errors, exit 0 |
| pth: `paths = [""]` | `ERROR cycle detected during queue construction` | `a/terragrunt.hcl:2:12: GRT003 dependency cycle: "a" -> "a"` |
| pth2: `paths = ["../b", ""]` (b exists) | `ERROR cycle detected during queue construction` | `a/terragrunt.hcl:2:20: GRT003 dependency cycle: "a" -> "a"`, no GRT002 |

## Commits

| Step | Commit |
|------|--------|
| Task 1 RED (block "") | 829abec |
| Task 1 GREEN (block "") | e033c49 |
| Task 2 golden (drop block self-loop) | 038f2d9 |
| Option A RED (paths "" self-edge) | 1c25f37 |
| Option A GREEN (emptyUnresolved flag, elemLiteral) | 8be81ae |
| Golden: paths "" GRT003 | 2dd68a0 |
| Docs + STATE + ROADMAP + SUMMARY | this commit (docs(05-07)) |

## Deviations from Plan

### User-directed change (checkpoint decision)

**1. [Decision checkpoint] Paths `""` is a self-loop, not silent**
- **Found during:** Task 2 oracle run
- **Issue:** The plan assumed paths `""` would stay silent. Terragrunt reports a cycle for it.
- **Fix:** Option A, as described above. The plan truth "dependencies { paths = [\"\"] } stays unresolved and silent" was replaced by the oracle-backed behaviour.
- **Commits:** 1c25f37, 8be81ae, 2dd68a0

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Empty paths literal was rejected by the domain**
- **Issue:** `NewPathDependency` requires a non-empty literal, but `elemLiteral` returned `""` (empty) for `""`.
- **Fix:** `elemLiteral` now returns `""` (two quote characters) for an empty string literal.
- **Commit:** 8be81ae

### Environment note

`go run honnef.co/go/tools/cmd/staticcheck@v0.8.1` with `GOTOOLCHAIN=auto` switched to go1.26.8, which cannot build this go1.27 module. It passes with `GOTOOLCHAIN=go1.27.0`. The code needed no change; CI is not affected because it uses a single toolchain.

## Corpus

No corpus has an active empty `config_path` or paths entry. The only `""` list element, in denis256 `dependency-merge/common.hcl`, is an `inputs` key. iso20022, secret and denis256 counts are unchanged, and the env-gated corpus tests pass.

## Self-Check: PASSED
