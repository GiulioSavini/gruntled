---
phase: 05-graph-diagnostics
plan: 01
subsystem: domain-contracts
tags: [repograph, diagnostic, grt002, grt003, target-state, path-dependency, edges]

requires:
  - phase: 04-real-repo-validation-experiment
    provides: "stable loader, goldens, denis256 corpus check"
provides:
  - "repograph.TargetState (Unknown zero value, DirMissing, NoConfig, HasConfig) with validation"
  - "NewDependency(name, target, pos, pathPos, state, opts) / NewUnresolvedDependency(name, reason, pos, pathPos, opts)"
  - "repograph.PathDependency, Unit.WithPathDependencies / PathDependencies (sorted by position)"
  - "RepositoryGraph.Edges(): deterministic block + paths edges (kind, from, to, pos = config_path value pos, enabled)"
  - "diagnostic.CodeMissingDependencyTarget = GRT002, CodeDependencyCycle = GRT003"
  - "loader: depDecl.cpPos threaded to both constructors (TargetUnknown until 05-03)"
affects: [05-02, 05-03, 05-04, 05-05, 05-06]

tech-stack:
  added: []
  patterns:
    - "Path deps kept apart from Dependency to preserve name-uniqueness invariant"
    - "config_path value position with block-position fallback when absent/unconvertible"

key-files:
  created:
    - internal/domain/repograph/edges_test.go
  modified:
    - .planning/phases/05-graph-diagnostics/05-CONTEXT.md
    - .planning/ROADMAP.md
    - .planning/REQUIREMENTS.md
    - internal/domain/diagnostic/diagnostic.go
    - internal/domain/diagnostic/diagnostic_test.go
    - internal/domain/repograph/unit.go
    - internal/domain/repograph/graph.go
    - internal/domain/repograph/valueobjects_test.go
    - internal/infrastructure/terragrunt/parse.go
    - internal/infrastructure/terragrunt/loader.go
    - internal/application/indexing/build_test.go
    - internal/application/checking/check_test.go
    - internal/domain/analysis/grt001_test.go

key-decisions:
  - "dependencies include merge is a union under both shallow and deep merge (CONTEXT correction)"
  - "denis256 criterion amended: report exactly the terragrunt v1.1.6 oracle-derived set"
  - "Under deep merge, pathPos comes from the occurrence that supplies config_path, pos stays the first occurrence's block pos"
  - "Loader passes TargetUnknown for every resolved dependency; 05-03 fills real state"

requirements-completed: []

duration: interrupted run (Task 1-2 on 2026-09-29, Task 3 on 2026-09-30)
completed: 2026-09-30
---

# Phase 5 Plan 01: Graph Diagnostic Domain Contracts Summary

Dependency now carries its config_path value position and a validated TargetState; Unit gains PathDependencies, RepositoryGraph gains deterministic Edges(), GRT002/GRT003 codes exist, and the loader feeds the real config_path position with zero behavior change.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | Record include-merge and oracle corrections, amend denis256 criterion | 6586c2b |
| 2 (RED) | Failing tests for TargetState, PathDependency, Edges, codes | 6efc8ce |
| 2 (GREEN) | Implement domain types and GRT002/GRT003 codes | ac523eb |
| 3 | Migrate call sites; loader passes config_path value position | 2157a40 |

## Task 3 details

- `parse.go`: `depDecl.cpPos` set from `config_path` `Expr.Range().Start` via `hclconv.Position` (same conversion as block pos); falls back to block pos if attribute absent or conversion fails.
- `loader.go`: `pathPos` chosen alongside `chosen` occurrence (shallow: occs[0]; deep: first occurrence with config_path), passed to `resolveOneDependency` and on to all 7 constructor calls; resolved deps get `TargetUnknown`.
- Tests (11 constructor calls in 3 files) reuse their existing nonzero pos as pathPos and `TargetUnknown`.

## Verification

- `go build ./... && go vet ./...` OK
- `go test -count=1 ./...` all green
- `go test -count=1 ./cmd/gruntled -run 'TestGolden|TestValidationDocPins'` OK (goldens unchanged)
- `scripts/check-architecture.sh`: OK

## Deviations from Plan

None - plan executed exactly as written. (Executor interrupted after Task 2; Task 3 done by continuation agent.)

## Requirements

MORE-01/02/06 are only prepared here (contracts); not marked complete, later plans deliver them.

## Self-Check: PASSED
