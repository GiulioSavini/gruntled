---
phase: 02-parsing-graph-construction
plan: 13
subsystem: terragrunt-loader
tags: [go, hcl, gap-cycle-1, include-target, symlink, generate, false-positive]

requires:
  - phase: 02-parsing-graph-construction
    provides: "include-target post-pass (02-07), canonical-free include resolution, generate-may-declare-outputs rule (02-05)"
provides:
  - "canonicalPath: segment-by-segment, fail-closed symlink resolution; include targets compared by canonical in-repo path (G15)"
  - "includeTargets with three marking rules: exact canonical match, ancestors of units whose includes are unknowable or failed, and every include-free unit once some include's file name is dynamic or terragrunt.hcl (G16)"
  - "markIncludeDecls evaluates EVERY include decl for marking, independently of resolveIncludes' early returns"
  - "dynamicIncludeFileNames: fixed file name of an unevaluated include path (literal, literal template, template ending in a literal with '/', conditional)"
  - "generateMayDeclareOutputs: literal contents containing \"output\" or a \\u escape anywhere count as may-declare (G19)"
  - "Catalogue rows INC-14..16, SRC-16..17, STACK-13..14, STACK-09 revision, gap-closure map 02-12..02-14"
affects: [02-VERIFICATION, 03-grt001-diagnostic-cli, 04-real-repo-validation-experiment]

key-files:
  created:
    - internal/infrastructure/terragrunt/include_targets.go
    - internal/infrastructure/terragrunt/include_targets_test.go
    - internal/infrastructure/terragrunt/include_target_realfs_test.go
    - internal/infrastructure/terragrunt/generate_test.go
  modified:
    - internal/infrastructure/terragrunt/loader.go
    - internal/infrastructure/terragrunt/merge.go
    - internal/infrastructure/terragrunt/reasons.go
    - internal/infrastructure/terragrunt/include_target_test.go
    - .planning/phases/02-parsing-graph-construction/02-TERRAGRUNT-EDGECASES.md

key-decisions:
  - "Unknowable includes (syntax error, JSON, oversize, unreadable) mark only ancestors, not every include-free unit: a mid-edit syntax error would otherwise wipe out coverage of the whole repository"
  - "Include-free rule accepted with its measured cost (denis256: 53 -> 719 include-target units of 1146), under 'ten false negatives beat one false positive'"
  - "G19 detector over-approximates on purpose (any 'output' substring); measured cost 0 extra units on denis256 and gc-articles"

requirements-completed: []
---

# Phase 2 Plan 13: Include-target false positives (G15, G16) and generate detector (G19) Summary

**Include targets are now matched by canonical path and marked for failing and dynamic includers; generate contents mentioning "output" anywhere make the module unknown.**

## Tasks

1. **G15, canonical include-target identity** (`872021a`). `canonicalPath` resolves each path segment with Lstat/ReadLink, fails closed on escaping or unresolvable links, and the include-target post-pass compares canonical paths. Tests: `TestCanonicalPath`, `TestIncludeTargetSymlinkedDir`, `TestIncludeTargetSymlinkedFile`, `TestIncludeTargetSymlinkChain` (real FS, `os.OpenRoot`).
2. **G16, parents of failing and dynamic includers** (`00e6a42`). RED: 11 new tests failed with the parent `resolved`. GREEN: `includeTargets` gained `markAncestors`, `markAllIncludeFree`, `noteIncludeFree`; `resolveUnit` marks ancestors on every early return in steps 1-4; `markIncludeDecls` runs over every decl before step 3; `resolveIncludes` no longer marks. No existing loader expectation changed.
3. **G19 and catalogue** (`3b14253`). RED: the G19 repro and the one-line and comment rows failed. GREEN: `generateOutputPatternRE` and the `regexp` import removed; the detector is `strings.Contains(text, "output") || strings.Contains(text, "\u")`. Catalogue appended (never rewritten): INC-14, INC-15, INC-16, SRC-16, SRC-17, STACK-13, STACK-14, STACK-09 revision, "Gap closure (02-12..02-14)".

## Measurements

| Corpus | Metric | Before | After |
|---|---|---|---|
| primary | missing outputs / include-target | 0 / unchanged | 0 / unchanged (no include blocks) |
| secret | `TestIncludeTargetSecretCorpus` | pass | pass |
| denis256 | include-target units (of 1146) | 53 | 719 |
| denis256 | generate-may-declare-outputs units, G19 alone | 14 | 14 |
| gc-articles | generate-may-declare-outputs units, G19 alone | 8 | 8 |

The denis256 jump comes from the include-free rule: one include there with an unfixed or `terragrunt.hcl` file name marks every include-free unit. This also hides that corpus's two genuine GRT001 findings (`issue-2631/main`, `mocks/module1`), which Phase 4's denis256 expectations must account for.

## Deviations from Plan

- `missingOutputs` already existed in `include_target_realfs_test.go`; the G16 tests reuse it instead of redeclaring it.
- `literalString` already existed in `eval.go` (evaluates with `Value(nil)`); `dynamicIncludeFileNames` reuses it.
- The lexing memory peak for STACK-13 is not recorded in 02-12-SUMMARY.md; the catalogue says so instead of inventing a number.

## Verification

`go vet ./...`, staticcheck v0.8.1, `go test -count=1 ./...`, and the env-gated `TestCorpusSmoke` and `TestIncludeTargetSecretCorpus` all pass. No `zz_*` files left.

## Self-Check: PASSED
