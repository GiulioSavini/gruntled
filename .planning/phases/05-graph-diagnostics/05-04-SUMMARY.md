---
phase: 05-graph-diagnostics
plan: 04
subsystem: infrastructure-loader
tags: [terragrunt, loader, dependencies-paths, include-merge, indexing]

requires:
  - phase: 05-graph-diagnostics
    provides: "05-01 PathDependency + Unit.WithPathDependencies; 05-02 GRT002/GRT003 over path deps; 05-03 resolveTargetExpr + classifyTarget"
provides:
  - "ports.UnitConfig.PathDependencies (include-merged, empty for config-unknown)"
  - "parse.go pathsDecl: shape decided on raw AST (list / unknown / invalid)"
  - "(*Loader).filePathDependencies + mergePathDeps (union, dedup by target, child first)"
  - "indexing.Build attaches path deps to resolved and module-unknown units"
  - "reasons config-path-empty, dependencies-paths-dynamic"
affects: [05-05, 05-06]

tech-stack:
  added: []
  patterns:
    - "Structurally invalid dependencies block drops only its own path edges, never changes unit status"

key-files:
  created:
    - internal/infrastructure/terragrunt/pathdeps_test.go
    - internal/infrastructure/terragrunt/check_paths_test.go
  modified:
    - internal/application/ports/ports.go
    - internal/infrastructure/terragrunt/parse.go
    - internal/infrastructure/terragrunt/loader.go
    - internal/infrastructure/terragrunt/merge.go
    - internal/infrastructure/terragrunt/reasons.go
    - internal/infrastructure/terragrunt/loader_test.go
    - internal/application/indexing/build.go
    - internal/application/indexing/build_test.go

key-decisions:
  - "paths shape from AST: TupleConsExpr = list; LiteralValue/Template/ObjectCons = invalid (dropped); anything else (traversal, call, for, \"${x}\" wrap) = one unresolved dependencies-paths-dynamic entry at the paths value"
  - "Element evaluating to \"\" is unresolved config-path-empty (never a self-edge); config_path blocks unchanged"
  - "Literal = string value for plain literal, else source text with template quotes stripped"
  - "Unknown paths in an included file keep their unresolved entry (no edge); other files' literals kept"
  - "End-to-end test lives in internal/infrastructure/terragrunt: architecture rule forbids application tests importing infrastructure"

requirements-completed: []

duration: 6min
completed: 2026-09-30
---

# Phase 5 Plan 04: dependencies { paths } Summary

`dependencies { paths = [...] }` is now modeled end to end: parsed from the raw AST, each element resolved like a `config_path` (same scope, child unit dir, target state), union-merged across includes with de-dup by target, carried on `ports.UnitConfig.PathDependencies` and attached to graph units, so GRT002 and GRT003 see these edges.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 (RED) | Failing TestPathDependencies | 541012b |
| 1 (GREEN) | Parse paths, resolve elements, port field, 2 reasons | 56613bc |
| 2 (RED) | Failing TestPathDepsMerge + TestBuild_PathDependencies | 0051c47 |
| 2 (GREEN) | mergePathDeps union + indexing wiring | 1ea3d82 |
| 3 | TestPathDepsEndToEnd through loader/Build/Check | 0ed51ac |

## Details

- Element rows: literal, `get_terragrunt_dir()` template, `local.x` (config-path-dynamic), `""` (config-path-empty), stack, escape, no-config, missing. Positions are the element start (byte column).
- Whole-block: `local.x`, `concat(...)`, `"${local.x}"` give one unresolved entry at the paths value. Duplicate block, labeled block, missing paths, string/object/template paths, empty list: zero path edges, unit resolved, block deps kept.
- Merge: shallow and deep both union (comment cites terragrunt v1.1.6 include.go Merge ~L402-407 / ModuleDependencies.Merge, DeepMerge ~L506-545); duplicate target kept once at child position; no_merge contributes nothing; include-only block yields edges.
- `assembleUnit` calls `WithPathDependencies` only for resolved/module-unknown units; a zero entry is an `*indexing.Error{Stage: "assemble"}`.
- End-to-end pinned messages: `dependencies path "../nodir" resolves to "nodir": directory does not exist` (x:1:25); `dependency cycle: "p" -> "q" -> "p"`; block + paths to acm give no cycle, and with a back edge a ring `"acm" -> "s" -> "acm"` (not "among").

## Verification

- `go test -count=1 ./internal/infrastructure/terragrunt -run 'TestPathDependencies|TestPathDepsMerge|TestPathDepsEndToEnd'` OK
- `go test -count=1 ./internal/infrastructure/terragrunt/... ./internal/application/...` OK; `go vet ./...` OK; `scripts/check-architecture.sh` OK
- `go build ./... && go test -count=1 ./...`: exactly one red, the expected `TestGoldenFixtures/dependency_edges` (extra `GRT002 edge/consumer/terragrunt.hcl:2:17`, unchanged from after 05-02). Left for 05-05 hand review, goldens not regenerated.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] End-to-end test location**
- **Issue:** Plan fixes the file at `internal/application/checking/check_paths_test.go`, but it must import the terragrunt loader, and `infrastructure-importers` in check-architecture.sh counts TestImports/XTestImports: only cmd/ and internal/infrastructure/ may import infrastructure.
- **Fix:** Test is `internal/infrastructure/terragrunt/check_paths_test.go` (this package's tests already import application/indexing via `build()`).
- **Commit:** 0ed51ac

**2. [Rule 3 - Blocking] TestUnknownReasons coverage**
- **Issue:** The two new reason constants need a fixture row in TestUnknownReasons (the test enumerates all reason constants).
- **Fix:** Added rows config-path-empty and dependencies-paths-dynamic with `wantPathDepUnresolved`.
- **Commit:** 56613bc

## Requirements

MORE-01 / MORE-02 not marked complete: goldens (05-05) and corpus validation (05-06) still pending.

## Self-Check: PASSED
