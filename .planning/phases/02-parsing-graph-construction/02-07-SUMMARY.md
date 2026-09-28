---
phase: 02-parsing-graph-construction
plan: 07
subsystem: terragrunt-loader
tags: [go, terragrunt, hcl, gap-closure, false-positives]

requires:
  - phase: 02-parsing-graph-construction
    provides: "Loader (02-05): resolveUnit fixed-order pipeline, resolveIncludes, resolveOneDependency, merge.go"
provides:
  - "Dependency-target guards: config-path-stack, config-path-nondefault-file, config-path-invalid (per-dependency, siblings stay resolved)"
  - "Include-target post-pass (step 13): a terragrunt.hcl included by another unit is config-unknown include-target"
  - "JSON include refusal: include-json-unsupported, no false GRT100"
  - "Overlay ReadDir failure -> module-unknown module-file-unreadable"
  - "Malformed / in-file-duplicate generate blocks -> config-unknown invalid-generate"
  - "Exact 03-RESEARCH Pattern 4 reproduction test and env-gated secret-corpus test"
affects: [03-grt001-diagnostic-cli, 02-11]

tech-stack:
  added: []
  patterns:
    - "Guards fail toward unknown at the narrowest scope: a bad dependency target is a dependency-level reason, never a whole-unit reason"
    - "LoadUnits-level post-pass for cross-unit facts (include-target), first config-unknown reason wins"

key-files:
  created:
    - internal/infrastructure/terragrunt/include_target_test.go
  modified:
    - internal/infrastructure/terragrunt/loader.go
    - internal/infrastructure/terragrunt/merge.go
    - internal/infrastructure/terragrunt/reasons.go
    - internal/infrastructure/terragrunt/loader_test.go

key-decisions:
  - "A directory holding both terragrunt.stack.hcl and terragrunt.hcl is still config-path-stack (Terragrunt tries stack outputs first)"
  - "Include paths are recorded as located before the json/nested/syntax checks: over-marking include-target fails toward unknown"
  - "An existing config-unknown reason on an included unit is kept (first check wins)"
  - "mergeGenerateUnknownReason's now-unreachable label-count guard returns invalid-generate (fail closed) instead of silently skipping"

patterns-established:
  - "Every new Reason* constant has a TestUnknownReasons row"

requirements-completed: [GRAPH-01, GRAPH-04, PARSE-02, PARSE-03, PARSE-04]

duration: multi-session (two executors)
completed: 2026-09-28
---

# Phase 2 Plan 07: Loader False-Positive Gap Closure Summary

**Seven loader gaps from 02-REVIEW (G2, G3, G4, G5, G6, G8, G9) now fail toward unknown, and the G11 comment is accurate. The exact Pattern 4 corpus reproduction no longer produces the false GRT001 at live/terragrunt.hcl:4:21.**

## Accomplishments

- Dependency targets that are stacks, non-default config files or invalid paths are unresolved on that dependency alone. Before this, G8 turned the whole unit config-unknown.
- A unit whose terragrunt.hcl is included by another unit is config-unknown `include-target`. Its references are still checked once per including unit through mergeReferences.
- JSON includes, whether explicit or found by find_in_parent_folders probing terragrunt.hcl.json, are `include-json-unsupported` with no GRT100.
- If the unit directory can't be read during the overlay check, the module is unknown (`module-file-unreadable`). It is no longer assumed to have no overlay.
- A generate block with zero labels, two or more labels, or a label repeated within one file makes the unit `invalid-generate`. A label shared between the child and an include is still a valid merge.

## New reasons and fixtures

| Reason | Kind | Fixture (TestUnknownReasons row / test) |
| --- | --- | --- |
| config-path-stack | dependency unresolved | `config-path-stack/dir-only-stack`, `/dir-with-both`, `/file` |
| config-path-nondefault-file | dependency unresolved | `config-path-nondefault-file` (`../vpc/alt.hcl`, `../vpc/terragrunt.hcl.json`) |
| config-path-invalid | dependency unresolved | `config-path-invalid` (`..\\vpc`), sibling `good` stays resolved |
| include-target | config-unknown | `include-target` row; TestIncludeTargetCorpusReproduction, TestIncludeTargetExplicitPath, TestIncludeTargetKeepsEarlierConfigUnknownReason |
| include-json-unsupported | config-unknown | `include-json-unsupported/explicit`, `/find-in-parent` (asserts no diagnostics) |
| module-file-unreadable | module-unknown | `module-file-unreadable` (readDirFailFS: first ReadDir succeeds, later ones fail) |
| invalid-generate | config-unknown | `invalid-generate/no-label`, `/two-labels`, `/duplicate-in-file`, `/duplicate-in-include`; regression row with the same label in child and include stays resolved |

Regression row: `config_path = "../vpc/terragrunt.hcl"` still resolves to `vpc`.

## Task Commits

1. **Task 1: Dependency-target guards (G2, G4, G8)**: `05cc662` (test), `eb34738` (fix)
2. **Task 2: Include-target (G3), JSON includes (G5), G11 comment**: `49a465f` (test), `91811e4` (fix)
3. **Task 3: Overlay ReadDir failure (G6), malformed generate (G9)**: `f57f7d1` (test), `7ad418e` (fix)

## Corpus results

- Primary (`GRUNTLED_CORPUS=$HOME/.cache/gruntled-phase4/corpus/primary go test -count=1 ./internal/infrastructure/terragrunt -run TestCorpusSmoke -v`): PASS and unchanged, with 65 units, 3 config-unknown and 22 references.
- Secret, cds-snc checkout at commit 341e8a9 (`GRUNTLED_CORPUS_SECRET=$HOME/.cache/gruntled-phase4/corpus/secret go test -count=1 ./internal/infrastructure/terragrunt -run TestIncludeTargetSecretCorpus -v`): PASS. It has 4 units under terragrunt/. `terragrunt` is include-target. acm, ecr and lambda resolve to aws/acm, aws/ecr and aws/lambda. There are 4 references, and none is missing its output.
- Pattern 4 reproduction: the missing-output count was 1 before the fix (the false GRT001) and is 0 after.

## Deviations from Plan

**1. [Rule 1 - Fail closed] Defensive generate label guard returns a reason**
- **Found during:** Task 3
- **Issue:** The previous executor's WIP kept `continue` in mergeGenerateUnknownReason for label count != 1, and its comment said this "fails closed". Skipping the block is actually fail-open.
- **Fix:** The guard now returns ReasonInvalidGenerate, so the module becomes unknown. The code is unreachable today because validateEffectiveFile runs first.
- **Files modified:** internal/infrastructure/terragrunt/merge.go
- **Commit:** 7ad418e

Task 3's fix was finished by a continuation executor. The previous executor left uncommitted WIP (the unitDirOverlaysModule signature change, the step-11 call site and the validateEffectiveFile generate checks). It was reviewed and kept, with the change above.

## Notes for 02-11

- In 02-TERRAGRUNT-EDGECASES.md, record STACK-09 as a documented false negative: a parent config's own references are only checked through its including units.
- Add catalogue rows for the new reasons: config-path-stack, config-path-nondefault-file, config-path-invalid, include-target, include-json-unsupported, module-file-unreadable (loader overlay variant) and invalid-generate.
- G7 (size/depth limits) builds on this plan's loader.go, reasons.go and loader_test.go.

## Verification

In the worktree with GOTOOLCHAIN=go1.27.0: `gofmt -l .` printed nothing, `go mod tidy -diff`, `go vet ./...`, staticcheck v0.8.1, `go test -count=1 ./...`, `scripts/check-architecture.sh` and `scripts/test-check-architecture.sh` all passed. -race was not run because there is no local gcc. Nothing was pushed, because this is a parallel worktree.

## Self-Check: PASSED

- internal/infrastructure/terragrunt/include_target_test.go exists
- Commits 05cc662, eb34738, 49a465f, 91811e4, f57f7d1 and 7ad418e are present on branch worktree-agent-a6b6c6506749cf109
