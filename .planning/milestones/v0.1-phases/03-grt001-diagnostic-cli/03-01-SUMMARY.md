---
phase: 03-grt001-diagnostic-cli
plan: 01
subsystem: analysis, checking, terragrunt-depfacts
tags: [go, grt001, diag-03, mock-outputs, use-case]
requires:
  - phase: 02-parsing-graph-construction
    provides: "RepositoryGraph with lazy-eval, stack-target and include-target guards (02-06..02-14); indexing.Build"
provides:
  - "analysis.UnknownOutputs: pure GRT001 analyzer implementing the DIAG-03 table"
  - "checking.Check / Report / Error: indexing.Build + UnknownOutputs merged into one canonical diagnostic.Set"
  - "MockMergeWithState: mock_outputs_merge_strategy_with_state overrides mock_outputs_merge_with_state"
affects: [03-03, 03-05, 04-real-repo-validation-experiment]
tech-stack:
  added: []
  patterns:
    - "Analyzer as a pure domain function over RepositoryGraph queries; first matching DIAG-03 row wins"
    - "Mock facts select only a message suffix, never the diagnostic or its severity"
key-files:
  created:
    - internal/domain/analysis/grt001.go
    - internal/domain/analysis/grt001_test.go
    - internal/application/checking/check.go
    - internal/application/checking/check_test.go
  modified:
    - internal/infrastructure/terragrunt/depfacts.go
    - internal/infrastructure/terragrunt/parse_test.go
    - internal/domain/repograph/options.go
key-decisions:
  - "GRT001 is always SeverityError; mock_outputs only appends the masking suffix when mock keys, allowed commands and merge/zero-output facts are all certain literals"
  - "checking.Check returns *indexing.Error unchanged (CLI maps it to exit 3); analyzer failures are *checking.Error{Stage: analyze}"
  - "mock_outputs_merge_strategy_with_state, when present, decides MockMergeWithState alone (Terragrunt getMockOutputsMergeStrategy); reverses the Phase 2 either-true rule"
requirements-completed: [DIAG-01, DIAG-03]
duration: 20min
completed: 2026-09-29
---

# Phase 3 Plan 01: GRT001 analyzer and check use case Summary

**Pure DIAG-03 GRT001 analyzer in the domain, a `checking.Check` use case merging GRT100 and GRT001 into one canonical Set, and Terragrunt's strategy-over-bool precedence for `MockMergeWithState`.**

## Tasks

1. **GRT001 analyzer** (RED `cdbbc45`, GREEN `b5575ad`). `analysis.UnknownOutputs` walks `g.References()` and applies rows 1-7. `TestDIAG03` has 22 named subtests (row1 through row7j), including `row7b corpus shape: mock covers output, merge_with_state true, apply allowed` (GRT001 at error plus suffix) and `row6a output declared, no mocks` (nothing). Separate tests cover a module-unknown referencing unit, a shared include resolving to two targets, output order equal to `References()` order, determinism across calls and unit input order, and a nil result on an empty graph. The message is built with strconv only (no fmt).
2. **checking.Check** (RED `a1877fa`, GREEN `b0c0260`). `Report{Graph, Diagnostics}`, `Error{Stage, Err}`. Tests with hand-written fakes: clean repo, one GRT001, loader GRT100 plus surface GRT100 plus GRT001 in canonical `NewSet` order, loader error unwrapping to `*indexing.Error`, cancelled context.
3. **Merge strategy precedence** (RED `866f221`, GREEN `d4b39da`). `mockMergeWithState` now lets a present strategy decide alone; otherwise the bool; otherwise false. `options.go` field doc says the same.

## Deliberate expectation reversals (Task 3)

- `merge true wins over strategy no_merge` renamed to `strategy no_merge overrides merge true`: True -> False.
- New `strategy non-literal, merge literal true`: Unknown (was True under the old rule).
- New `strategy unrecognized literal, merge true`: Unknown (was True).
- New `strategy shallow overrides merge false`: True (same as before).
- New `merge_with_state non-literal alone`: Unknown (row the plan listed as unchanged but did not exist yet).
- No other fixture combines both attributes (`grep no_merge` hits in loader_test.go are the include `merge_strategy`, unrelated).

## Deviations from Plan

None in behaviour. The plan's precondition (02-06..02-11 SUMMARYs) holds; 02-12..02-14 were also merged and needed no changes here.

## Verification

gofmt, `go mod tidy -diff`, `go vet ./...`, staticcheck v0.8.1, `go test -count=1 ./...`, `check-architecture.sh` (3 domain, 3 application, 1 interfaces packages), `test-check-architecture.sh`, and the env-gated corpus tests (`TestCorpusSmoke`, `TestDependencyOptionsPrimaryCorpusShape`, `TestIncludeTargetSecretCorpus`, `TestIncludeTargetCorpusReproduction`) all pass with GOTOOLCHAIN=go1.27.0.

## Self-Check: PASSED
