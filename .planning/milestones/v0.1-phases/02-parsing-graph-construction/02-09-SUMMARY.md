---
phase: 02-parsing-graph-construction
plan: 09
subsystem: architecture-enforcement
tags: [bash, ci, architecture, hexagonal, ddd, gap-closure]

# Dependency graph
requires:
  - phase: 02-parsing-graph-construction (plans 01-05)
    provides: scripts/check-architecture.sh with its Phase 2 layering rules and the run_case/mkcopy/addstub self-test harness
provides:
  - "rule internal-layout: a top-level internal/<x> holding Go code must be domain, application, infrastructure, interfaces or testsupport; a .go file directly in internal/ fails"
  - "rule infrastructure-importers: only cmd/... and internal/infrastructure/... may import internal/infrastructure/... (prod and test imports)"
  - "rule testsupport-only-in-tests: no non-_test.go file outside internal/testsupport may import it, whether or not cmd links it"
  - "hcl-only-in-infrastructure now also catches _GOOS/_GOARCH-suffixed and //go:build-tagged files"
  - "scan_import_lines: shared source-level import scanner behind the last three rules"
affects: [phase-3-analysis, 03-02]

tech-stack:
  added: []
  patterns:
    - "Two engines per importer rule: go list (host-platform view, exact package graph) unioned with a source scan of import declarations (every platform, every build tag)"
    - "The source scan reads import declarations only (single-line import or import ( ... ) block specs, stopping at the first func/type/var/const), so comments, string literals and raw-string Go snippets in tests never match"

key-files:
  created: []
  modified:
    - scripts/check-architecture.sh
    - scripts/test-check-architecture.sh

key-decisions:
  - "scan_import_lines is an awk state machine over import declarations, not the plan's plain per-line grep ERE: the grep would flag a slice literal line such as \"github.com/hashicorp/x\", or an import line inside a raw-string Go snippet in a test, which is a false positive"
  - "The scan prunes the directories the go tool ignores (names starting with . or _, testdata, vendor), matching go list ./... scope; this also keeps .git and .claude/worktrees copies of the repo out of the scan when run from the main checkout"
  - "internal-layout counts *.go files anywhere below an unknown directory, testdata included, and inspects dot entries too: the conservative reading"
  - "Import paths in raw-string form (import _ `x`) are matched too; Go accepts them"

requirements-completed: [ARCH-01]

duration: 25min
completed: 2026-09-28
---

# Phase 2 Plan 9: Architecture Blind Spots (G13, G14) Summary

**Three new architecture rules (internal-layout, infrastructure-importers, testsupport-only-in-tests) plus a build-tag-proof hcl-only-in-infrastructure, all backed by an import-declaration-aware source scan and 14 new self-test cases that each assert the exact failing rule name.**

## Performance

- **Duration:** about 25 min (continuation executor; RED commit for Task 1 came from the previous executor)
- **Completed:** 2026-09-28
- **Tasks:** 2
- **Files modified:** 2

## Accomplishments

- A new top-level `internal/<x>` package, a stray `internal/*.go`, a non-cmd package importing infrastructure and production code importing testsupport now each fail CI under their own rule name.
- A `_windows.go` file or a `//go:build`-tagged file can no longer bypass hcl-only-in-infrastructure, infrastructure-importers or testsupport-only-in-tests. go list never sees these files on linux, so the source scan is what catches them.
- The clean tree still exits 0. All 22 pre-existing self-test cases pass unchanged (36 cases in total).

## Rules and their self-test cases

| Rule | Failing cases | Allowed (zero) cases |
| ---- | ------------- | -------------------- |
| internal-layout | layout-unknown-dir (02-REVIEW probe `internal/analysis/a.go`, also fails infrastructure-importers), layout-stray-go-file | layout-interfaces-allowed, layout-non-go-dir-allowed |
| infrastructure-importers | layout-unknown-dir, infra-from-testsupport, infra-from-tagged-file (`_windows.go`, source scan only) | infra-from-cmd-allowed |
| testsupport-only-in-tests | testsupport-in-prod, testsupport-in-tagged-prod (`_windows.go`, source scan only) | (existing _test.go importers in the clean tree) |
| hcl-only-in-infrastructure (strengthened) | hcl-windows-file-in-cmd (02-REVIEW probe), hcl-tagged-in-testsupport (02-REVIEW probe, `//go:build integration`), hcl-aliased-import-block | hcl-tagged-in-infrastructure-allowed, hcl-mention-in-comment-allowed |

`run_case` now accepts a space-separated list of rule names. Every name has to show up as an exact `=== RULE FAILED: <name> ===` line.

## Task Commits

1. **Task 1: internal-layout, infrastructure-importers, testsupport-only-in-tests (G13)**
   - `4fde165` test(02-09): add failing self-tests for internal layout and importer rules
   - `c3a6c54` feat(02-09): enforce internal layout, infrastructure importers and testsupport-only-in-tests
2. **Task 2: source-level HCL import scan (G14)**
   - `bb1c3e1` test(02-09): add failing self-tests for build-constrained HCL imports
   - `847d5b0` feat(02-09): scan build-constrained files for HCL imports

RED was confirmed for every new failing case before its GREEN commit. With the old Step 6, hcl-windows-file-in-cmd, hcl-tagged-in-testsupport and hcl-aliased-import-block each exited 0.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] Line-based import ERE replaced with an import-declaration-aware scanner**
- **Found during:** Task 1 (reviewing the previous executor's uncommitted WIP)
- **Issue:** the plan's grep ERE, applied to every line of a file, also matches ordinary code. Examples: a slice literal line `"github.com/hashicorp/x",`, or an `import "..."` line inside a raw-string Go snippet in a test. Either would be a false positive. In addition, running from the main checkout would have scanned the `.claude/worktrees/*` repo copies, whose `internal/infrastructure` files are not excluded by `./internal/infrastructure/*`.
- **Fix:** `scan_import_lines` is now a portable awk program (checked identical under gawk and mawk). It reads only single-line imports and `import ( ... )` specs, stops at the first func/type/var/const, skips `/* */` header comments, and accepts `"` or backtick quotes. find prunes `.`/`_`-prefixed, testdata and vendor directories, which is the go tool's own scope.
- **Files modified:** scripts/check-architecture.sh
- **Commit:** c3a6c54

Everything else follows the plan. The previous executor's WIP (internal-layout block, Steps 7 and 8) was kept as written, apart from the scanner.

## Notes for Phase 3 (03-02)

- `internal/interfaces` is already in the internal-layout allowed set (proven by layout-interfaces-allowed), so 03-02 needs no layout change.
- 03-02's own interfaces-external-deps rule sits on top. infrastructure-importers already forbids `internal/interfaces/...` from importing `internal/infrastructure/...` directly, so presenters must receive infrastructure through cmd/gruntled wiring.
- New importer-style rules should reuse `scan_import_lines <target-ERE> <find predicates...>` so build-tagged files are covered.

## Verification

Run in the worktree with GOTOOLCHAIN=go1.27.0: `gofmt -l .` printed nothing. `go mod tidy -diff`, `go vet ./...`, `staticcheck@v0.8.1 ./...` and `go test -count=1 ./...` all passed. `bash scripts/check-architecture.sh` printed OK. `bash scripts/test-check-architecture.sh` printed "all architecture self-tests passed" (36 cases).

Not pushed: this plan ran in a parallel worktree. The orchestrator owns merge, push and the STATE/ROADMAP/REQUIREMENTS updates.

## Self-Check: PASSED
