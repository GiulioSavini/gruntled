---
phase: 05-graph-diagnostics
plan: 02
subsystem: domain-analysis
tags: [analysis, grt002, grt003, tarjan, checking]

requires:
  - phase: 05-graph-diagnostics
    provides: "05-01 TargetState, PathDependency, Edges(), GRT002/GRT003 codes; 05-03 real block TargetState"
provides:
  - "analysis.MissingTargets(g): GRT002 for block deps (enabled literally true) and path deps whose target is DirMissing / NoConfig"
  - "analysis.DependencyCycles(g): GRT003, one per SCC, iterative Tarjan, ring or 'among' message"
  - "checking.Check runs UnknownOutputs, MissingTargets, DependencyCycles into one canonical Set"
affects: [05-04, 05-05, 05-06]

tech-stack:
  added: []
  patterns:
    - "Analyzer list in checking.Check; every analyzer failure is *checking.Error{Stage: analyze}"
    - "Iterative Tarjan with explicit frame stack, nodes and adjacency in RepoPath order"

key-files:
  created:
    - internal/domain/analysis/grt002.go
    - internal/domain/analysis/grt002_test.go
    - internal/domain/analysis/grt003.go
    - internal/domain/analysis/grt003_test.go
  modified:
    - internal/application/checking/check.go
    - internal/application/checking/check_test.go
    - cmd/gruntled/testdata/script/diag03_silent_rows.txtar

key-decisions:
  - "GRT003 anchor position for a multi-member SCC ignores the anchor's self-edge (CONTEXT: first out-edge to another member); one-member SCC uses the self-edge"
  - "A self-loop is a one-member ring: dependency cycle: \"a\" -> \"a\""
  - "All analyzers share Stage \"analyze\"; the wrapped error names the analyzer"

requirements-completed: []

duration: 5min
completed: 2026-09-30
---

# Phase 5 Plan 02: GRT002 / GRT003 Analyzers Summary

GRT002 (missing dependency target, block and paths) and GRT003 (dependency cycle per SCC via iterative Tarjan) are pure domain analyzers, and checking.Check now returns GRT001+GRT002+GRT003 in one canonical Set.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 (RED) | Failing TestMissingTargets | 7c607ef |
| 1 (GREEN) | GRT002 MissingTargets | 58b24e7 |
| 2 (RED) | Failing TestDependencyCycles / Deterministic / DeepChain | c150002 |
| 2 (GREEN) | GRT003 DependencyCycles, iterative Tarjan | 6d10947 |
| 3 | Wire analyzers into checking.Check + TestCheckAllAnalyzers | 0f01fcb |
| fix | DIAG-03 row 4 fixture no longer triggers GRT002 | ca951d1 |

## Details

- GRT002 rows (first match wins): unresolved, TargetUnknown/HasConfig, block with enabled not literally true: silent; DirMissing: "directory does not exist"; NoConfig: "directory has no terragrunt.hcl". Paths entries skip the enabled row. skip_outputs/mock_outputs never gate. Messages use strconv.Quote; path deps use `Literal()`.
- GRT003: edges from `g.Edges()` with enabled == TristateTrue and both ends graph units; adjacency sorted and deduped by target (block + paths to the same unit = one edge). SCCs kept when size > 1 or with a self-edge. Ring iff every member has exactly one distinct in-SCC successor, so a self-loop inside a larger SCC gives "among". Anchor position is the minimum over all (non-deduped) anchor edges into the SCC.
- Determinism test rebuilds every case under reversed/rotated unit order and reversed edge order (domain test allowlist has no math/rand). Deep chain: 10000 units, open (nil) and closed (one ring diag with 10000 arrows).
- TestCheckAllAnalyzers: GRT002 (live/app:2:17), GRT001 (live/app:6:12), GRT003 (live/x:1:17) in canonical order. Indexing error pass-through stays covered by TestCheckLoaderErrorPassesThrough.

## Verification

- `go test -count=1 ./internal/domain/analysis -run TestMissingTargets` OK
- `go test -count=1 ./internal/domain/analysis -run 'TestDependencyCycles|TestCyclesDeterministic|TestCyclesDeepChain'` OK
- `go test -count=1 ./internal/domain/... ./internal/application/...` OK; `scripts/check-architecture.sh` OK; `go vet ./...` OK
- `go build ./... && go test -count=1 ./...`: one red, expected: `TestGoldenFixtures/dependency_edges` now also reports `GRT002 error edge/consumer/terragrunt.hcl:2:17`. Left for hand review in 05-05, goldens not regenerated.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] DIAG-03 row 4 testscript went red**
- **Found during:** full test run after Task 3
- **Issue:** `diag03_silent_rows` row 4 pointed `config_path` at an existing dir with no terragrunt.hcl, which is now a correct GRT002, so the "exit 0, silent" GRT001 row failed.
- **Fix:** the target is now `vendor/not-a-unit` holding a terragrunt.hcl the walk skips: still "not a unit" for GRT001 row 4, HasConfig for GRT002, so both stay silent.
- **Files modified:** cmd/gruntled/testdata/script/diag03_silent_rows.txtar
- **Commit:** ca951d1

## Requirements

MORE-01/MORE-02 logic is done at unit level; not marked complete until goldens/docs (05-05) and paths loading (05-04) land.

## Self-Check: PASSED
