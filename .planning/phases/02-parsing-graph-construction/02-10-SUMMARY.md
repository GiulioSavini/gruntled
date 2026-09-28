---
phase: 02-parsing-graph-construction
plan: 10
subsystem: infra
tags: [hcl, hclsyntax, hcl-json, stack-overflow, input-limits, tfsurface]

requires:
  - phase: 02-parsing-graph-construction
    provides: hclconv shared HCL helpers and the tfsurface.Reader (plans 02-02/02-04)
provides:
  - hclconv.MaxFileBytes / MaxNestingDepth / ErrFileTooLarge / ErrNestingTooDeep
  - hclconv.ReadFileLimited, CheckNativeDepth, CheckJSONDepth
  - tfsurface reasons module-file-too-large and module-file-too-deep, checked before any parse
affects: [02-11 (wires the same hclconv API into terragrunt parse.go/loader.go)]

tech-stack:
  added: []
  patterns:
    - "Pre-parse guard: size cap then nesting-depth pre-scan before any recursive hcl parser"
    - "Depth counting never under-counts: mismatched/unmatched closers are ignored, never popped"

key-files:
  created:
    - internal/infrastructure/hclconv/limits.go
    - internal/infrastructure/hclconv/limits_test.go
  modified:
    - internal/infrastructure/hclconv/hclconv.go
    - internal/infrastructure/tfsurface/reader.go
    - internal/infrastructure/tfsurface/reader_test.go

key-decisions:
  - "The native depth pre-scan uses hclsyntax.LexConfig (the parser's own tokenizer) instead of a hand-written byte scanner, so it is exact by construction. The JSON scan mirrors hcl/json scanString byte for byte"
  - "Too-large and too-deep files emit no diagnostic. They are not syntax errors, so a GRT100 would be a false claim"
  - "Precedence: unreadable > too-large > too-deep > syntax-error > invalid-module-block > no-terraform-files"
  - "The .json depth check runs only for .json files and the native check only for the rest. The HCL lexer never runs on JSON input"

patterns-established:
  - "Guarded parse: hclconv.ReadFileLimited then CheckNativeDepth/CheckJSONDepth, then parse. 02-11 follows the same order"

requirements-completed: [PARSE-05, GRAPH-05]

duration: 29min
completed: 2026-09-28
---

# Phase 2 Plan 10: Module-file size cap and nesting-depth guard (G7a) Summary

**hclconv gains a 4 MiB size cap and a lexer-exact 1000-level nesting pre-scan. tfsurface refuses any module file that fails either check, and the surface becomes unknown (module-file-too-large / module-file-too-deep) instead of a fatal stack overflow killing the process.**

## Performance

- **Duration:** ~29 min (two executor sessions)
- **Started:** 2026-09-28T09:46:30+02:00
- **Completed:** 2026-09-28T10:15:00+02:00
- **Tasks:** 2/2
- **Files modified:** 5

## Accomplishments

- `hclconv/limits.go` provides the API that 02-11 consumes, with the exact names from the plan.
- `tfsurface.Reader` runs every kept file through the size cap and depth pre-scan before `parseModuleFile`. A refused file is never parsed and makes the whole surface unknown, never partially known.
- The G7 crash shape from 02-REVIEW (a 200k-deep paren nest in `main.tf`) now returns `module-file-too-deep` with no diagnostic. So does the JSON counterpart.

## Final hclconv API (for 02-11)

```go
const MaxFileBytes = 4 << 20   // 4 MiB
const MaxNestingDepth = 1000   // inclusive bound
var ErrFileTooLarge = errors.New("hclconv: file exceeds MaxFileBytes")
var ErrNestingTooDeep = errors.New("hclconv: nesting exceeds MaxNestingDepth")
func ReadFileLimited(fsys fs.FS, name string) ([]byte, error) // Stat error as-is; Stat size > cap -> ErrFileTooLarge unread; exactly one fs.ReadFile; post-read len check
func CheckNativeDepth(src []byte) error // .hcl, .tf, .tofu
func CheckJSONDepth(src []byte) error   // .tf.json, .tofu.json
```

Depth counts every enclosing block's braces, so an `output "x" { value = (...) }` file is at depth n+1 for n parens.

## Task Commits

1. **Task 1: hclconv size cap and nesting-depth pre-scan**
   - RED `80f0a68` test(02-10): add failing size-cap and nesting-depth cases
   - GREEN `20c9fb7` feat(02-10): add hclconv size cap and nesting-depth pre-scan
2. **Task 2: tfsurface refuses oversize or overdeep module files**
   - RED `e42c39b` test(02-10): add failing module-file size and depth cases
   - GREEN `5d9160b` fix(02-10): refuse oversize and overdeep module files before parsing

## Observed RED crash

Against the pre-fix reader, `TestReadSurfaceDeepNestingNoCrash` (200k parens) killed the test binary with `fatal error: stack overflow` inside hclsyntax's `parseTernaryConditional`/`parseExpressionTerm` recursion. The input is generated in the test, and no fixture is committed.

## Peak RSS of the new tests

Measured on compiled test binaries with `/usr/bin/time -v` (go1.27.0, linux/amd64, no -race):
- hclconv test binary: ~574 MiB (587616 KB), driven by `LexConfig` on the ~200k-token inputs
- tfsurface test binary: ~145 MiB (148736 KB)
- `go test` for both packages including the toolchain: ~849 MiB (869444 KB)

No test input exceeds MaxFileBytes+1 bytes. The oversize inputs are refused on the Stat size and never lexed.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] The at-limit test was one level over the limit**
- **Found during:** Task 2 GREEN
- **Issue:** The committed `TestReadSurfaceAtDepthLimitStillReads` nested `MaxNestingDepth` parens inside `output "x" { ... }`. The block's braces add one level, so the file was at depth 1001 and was correctly refused.
- **Fix:** The value now nests `MaxNestingDepth - 1` parens, so the whole file sits exactly at the inclusive limit. The limit itself is unchanged. hclconv's own boundary test still covers exactly `MaxNestingDepth` in isolation.
- **Files modified:** internal/infrastructure/tfsurface/reader_test.go
- **Commit:** 5d9160b

**2. [Rule 1 - Bug] The native pre-scan also ran on JSON files**
- **Found during:** Task 2 (review of the previous executor's WIP)
- **Issue:** The WIP ran `CheckNativeDepth` on every file and then overwrote the result with `CheckJSONDepth` for `.json`. The HCL lexer's memory-heavy pass ran on JSON input for nothing.
- **Fix:** Changed to if/else, so each file gets exactly one pre-scan.
- **Files modified:** internal/infrastructure/tfsurface/reader.go
- **Commit:** 5d9160b

## Issues Encountered

The previous executor stopped with Task 2 GREEN uncommitted. Its WIP was reviewed, kept, fixed (deviation 2) and committed.

## Next Phase Readiness

- 02-11 (G7b) can wire `ReadFileLimited` and `CheckNativeDepth`/`CheckJSONDepth` into `internal/infrastructure/terragrunt` using the API above.
- Not pushed and not merged: this plan runs in a parallel worktree. STATE.md, ROADMAP.md and REQUIREMENTS.md are left to the orchestrator.

## Self-Check: PASSED

- Files exist: limits.go, limits_test.go, hclconv.go, reader.go, reader_test.go
- Commits present: 80f0a68, 20c9fb7, e42c39b, 5d9160b
- Local suite green (GOTOOLCHAIN=go1.27.0): gofmt -l empty, go mod tidy -diff, go vet, staticcheck v0.8.1, go test -count=1 ./..., check-architecture.sh, test-check-architecture.sh
