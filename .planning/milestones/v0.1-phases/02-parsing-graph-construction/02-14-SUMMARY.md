---
phase: 02-parsing-graph-construction
plan: 14
subsystem: architecture-check
tags: [ci, architecture, gap-cycle-1, go-parser, go-modules]

requires:
  - phase: 02-parsing-graph-construction
    provides: "check-architecture.sh layering rules and self-test harness (01-02, 02-09)"
provides:
  - "scripts/archscan: go/parser-based import scanner replacing the awk scan, so a comment or ';' inside an import block can no longer hide an import (G21)"
  - "single-module rule: no nested go.mod (outside pruned dot/underscore dirs), no root go.work/go.work.sum, no required module under the main module path (G22)"
  - "GOWORK=off in check-architecture.sh, so a go.work in any parent directory cannot change what the rules see"
  - "addstub creates stub modules outside the copy, so self-test stubs are not nested modules"
affects: [02-VERIFICATION, 03-grt001-diagnostic-cli]

key-files:
  created:
    - scripts/archscan/main.go
    - scripts/archscan/main_test.go
    - .planning/phases/02-parsing-graph-construction/02-14-SUMMARY.md
  modified:
    - scripts/check-architecture.sh
    - scripts/test-check-architecture.sh

key-decisions:
  - "single-module runs in Step 0, before the compile gate: a nested module must be reported even when it breaks compilation"
  - "The module-path check reads go.mod directly (awk on the module line) so it needs no compiler; go list -m -e all failures are reported as a rule failure, never as a pass"

requirements-completed: [ARCH-01]
---

# Phase 2 Plan 14: Architecture-check holes G21 and G22 Summary

**The import scan is now a real Go parser, and the repository must stay a single Go module.**

## Tasks

1. **G21, Go import scanner** (`bc710bc`). `scripts/archscan` parses each file with `go/parser` (imports only) and prints its imports; `check-architecture.sh` uses it instead of the awk scan. Self-test cases for the bypass shapes pass by rule name: `hcl-comment-in-import-block`, `hcl-semicolon-import-block`, `infra-comment-in-import-block`, `testsupport-semicolon-in-tagged-prod`, `hcl-aliased-import-block`; zero cases `hcl-mention-in-comment-allowed`, `hcl-raw-string-in-func-allowed`, `hcl-in-testdata-allowed` stay green. `go test ./scripts/archscan/` passes.
2. **G22, single-module rule** (`3a51453`). RED: the exact G22 repro (nested module under `internal/domain` importing `os`) printed `architecture: OK` with the pre-fix script. GREEN: the new `single-module` block fails on nested `go.mod`, root `go.work`/`go.work.sum`, and any module under the main module path. New cases `nested-go-mod`, `nested-go-mod-dot-dir`, `go-work` fail with `single-module`; `unrelated-go-mod-in-dot-dir-allowed` stays zero, like the real `.claude/worktrees` copies.

## Verification

- `bash scripts/check-architecture.sh`: OK on the real tree, including `.claude/worktrees`.
- `bash scripts/test-check-architecture.sh`: all 56 cases PASS, including the 6 named in the plan's verify line.
- `grep zzstub scripts/test-check-architecture.sh`: no match.
- gofmt, `go vet ./...`, staticcheck v0.8.1, `go mod tidy -diff`: clean.

## Deviations from Plan

None in behaviour. The work was split across a session restart; the Task 1 commit was reworded from a checkpoint commit.

## Self-Check: PASSED
