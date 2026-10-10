---
phase: 05-graph-diagnostics
plan: 03
subsystem: infrastructure-loader
tags: [terragrunt, loader, target-state, grt002, dependency]

requires:
  - phase: 05-graph-diagnostics
    provides: "05-01 TargetState, NewDependency(..., state, ...), config_path value pos"
provides:
  - "(*Loader).classifyTarget(dir) repograph.TargetState (case-exact ReadDir, symlinks followed, dangling = absent)"
  - "(*Loader).resolveTargetExpr(expr, scope, unitDir) (repograph.RepoPath, reason string), shared with 05-04 paths"
  - "resolved block dependencies carry real TargetState; unresolved stay TargetUnknown"
  - "nonexistent .../terragrunt.hcl config_path maps to its parent dir"
affects: [05-02, 05-04, 05-05]

tech-stack:
  added: []
  patterns:
    - "Only errors.Is(err, fs.ErrNotExist) means missing; every other fs error is Unknown (never report what was not observed)"

key-files:
  created:
    - internal/infrastructure/terragrunt/targetstate.go
    - internal/infrastructure/terragrunt/targetstate_test.go
    - internal/infrastructure/terragrunt/targetstate_unix_test.go
  modified:
    - internal/infrastructure/terragrunt/loader.go

key-decisions:
  - "resolveTargetExpr returns repograph.RepoPath (not string): RepoPath validation already happens inside, and 05-04 needs a RepoPath for NewPathDependency"
  - "Nonexistent config_path named terragrunt.hcl maps to parent dir only on fs.ErrNotExist; other stat errors keep the old behavior"

requirements-completed: []

duration: 2min
completed: 2026-09-30
---

# Phase 5 Plan 03: Block Dependency Target State Summary

The loader now classifies every resolved `dependency` block target on disk (DirMissing / NoConfig / HasConfig, Unknown on any unobservable error) through `classifyTarget`, and dependency resolution lives in a shared `resolveTargetExpr` helper ready for `dependencies.paths` in 05-04.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 (RED) | Failing classifyTarget tests (MapFS + real os.Root) | 13aa913 |
| 1 (GREEN) | Implement classifyTarget | 78e3e1f |
| 2 (RED) | Failing TestBlockTargetState | 5db8ea8 |
| 2 (GREEN) | Shared resolveTargetExpr helper, fill block TargetState | 5c32a6a |

## Details

- `classifyTarget`: `fs.ReadDir` then exact name match on `terragrunt.hcl` / `terragrunt.hcl.json`, `fs.Stat` on a match (a directory with that name does not count, a dangling symlink does not count, an escaping symlink is Unknown). A regular file as target, an unreadable dir (chmod 000) and a dir symlink escaping the root all give Unknown.
- `resolveTargetExpr`: same resolution order and reasons as before (dynamic, outside-repo, stack file, non-default file, invalid RepoPath, stack dir wins). The only new behavior is that a nonexistent `../x/terragrunt.hcl` now targets `x` instead of `x/terragrunt.hcl`.
- `TestBlockTargetState` checks target, state and config_path value position (line and byte column) for 6 resolved rows, and reason plus Unknown for 4 unresolved rows.

## Verification

- `go test -count=1 ./internal/infrastructure/terragrunt -run TestTargetState` passes (unreadable-dir test actually ran, not skipped)
- `go test -count=1 ./internal/infrastructure/terragrunt/... ./internal/application/...` passes; existing loader tests unchanged
- `go vet ./...` OK; `scripts/check-architecture.sh` OK
- `go build ./... && go test -count=1 ./...` all green, **goldens included**. The plan expects `TestGolden` dependency_edges to go red, but only once the GRT002 analyzer from 05-02 exists. 05-02 has not run yet, so nothing emits GRT002 and the goldens are still green. Expect the dependency_edges red after 05-02, until 05-05 fixes it.

## Deviations from Plan

**1. [Rule 3 - Blocking] resolveTargetExpr return type**
- **Issue:** The plan's signature returns `target string`. The helper already has to build and validate the RepoPath (ReasonConfigPathInvalid), and callers need a RepoPath.
- **Fix:** It returns `(repograph.RepoPath, string)`. It is also a `*Loader` method because it needs `l.fsys`.
- **Commit:** 5c32a6a

## Requirements

MORE-01 is not marked complete: this plan only adds the loader fact. The GRT002 diagnostic comes in 05-02 and the goldens in 05-05.

## Self-Check: PASSED
