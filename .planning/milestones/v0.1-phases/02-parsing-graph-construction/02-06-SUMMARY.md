---
phase: 02-parsing-graph-construction
plan: 06
subsystem: parsing
tags: [hcl, hclsyntax, terragrunt, reference-extraction, lazy-evaluation]

# Dependency graph
requires:
  - phase: 02-parsing-graph-construction
    provides: refWalker/extractRefs (02-02..02-05), the try()/can() reference guard it extends
provides:
  - refWalker.lazy guard suppressing References inside a ternary's unselected branch, a short-circuited &&/|| operand, and a for expression's key/value/if-condition (and their %{ if }/%{ for } template-directive forms)
  - OUT-09/OUT-10 catalogue rows reversed (02-TERRAGRUNT-EDGECASES.md) with a recorded rationale
affects: [03-grt001-diagnostic-cli]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "Fail-silent AST-range guard: push/pop a byte-range stack on hclsyntax.Walk Enter/Exit, suppress a match whose start byte falls in any pushed range (used for both try()/can() and the new lazy-evaluation guard)"

key-files:
  created: []
  modified:
    - internal/infrastructure/terragrunt/refs.go
    - internal/infrastructure/terragrunt/refs_test.go
    - internal/infrastructure/terragrunt/integration_test.go
    - .planning/phases/02-parsing-graph-construction/02-TERRAGRUNT-EDGECASES.md

key-decisions:
  - "refWalker gets a second guard (a lazy byte-range stack) alongside the existing try/can int guard, rather than a unified guard type, matching 03-RESEARCH.md Pattern 2's sketch exactly"
  - "Only ConditionalExpr.TrueResult/FalseResult, ForExpr.KeyExpr/ValExpr/CondExpr, and BinaryOpExpr{OpLogicalAnd,OpLogicalOr}.LHS/RHS are lazy; the ternary condition, the for collection, unary ops and non-logical binary ops stay checked"
  - "Template directives need no extra case: %{ if } parses to *hclsyntax.ConditionalExpr and %{ for } to *hclsyntax.TemplateJoinExpr wrapping *hclsyntax.ForExpr, so the same switch cases cover them; proven with dedicated table rows rather than assumed"

requirements-completed: [PARSE-01, PARSE-04]

# Metrics
duration: 15min
completed: 2026-09-28
---

# Phase 2 Plan 06: Lazy-evaluation reference guard Summary

**refWalker now suppresses a `dependency.X.outputs.Y` Reference sitting in an HCL sub-expression whose diagnostics HCL may drop without evaluating it as a runtime error: a ternary's unselected branch, a short-circuited `&&`/`||` operand, or a `for` expression's key/value/if-condition — closing gap G1 from 02-REVIEW.md.**

## Performance

- **Duration:** ~15 min
- **Tasks:** 2
- **Files modified:** 4 (2 source/test in `internal/infrastructure/terragrunt`, 1 test-only, 1 doc)
- **Commits:** 4 task commits (this SUMMARY commit not yet made)

## Accomplishments

- Added `refWalker.lazy`, a byte-range stack pushed by a new `lazyRanges(n hclsyntax.Node) []hcl.Range` helper on `Enter` and popped on the matching `Exit`, exactly mirroring the existing `try`/`can` `guard int` mechanism.
- `lazyRanges` covers exactly three node shapes per 03-RESEARCH.md Pattern 2: `*hclsyntax.ConditionalExpr` (`TrueResult`/`FalseResult`), `*hclsyntax.ForExpr` (`ValExpr`/`KeyExpr`/`CondExpr`), and `*hclsyntax.BinaryOpExpr` with `Op` `OpLogicalAnd`/`OpLogicalOr` (`LHS`/`RHS`). The ternary condition, the for collection, unary negation, and non-logical binary ops (arithmetic, comparison) are unaffected.
- `ScopeTraversalExpr` emission now additionally requires `!w.inLazyRange(start)`, matching on byte offsets (never line/column), consistent with the project's byte-column convention.
- 21-row `TestExtractRefsLazyEvaluation` table plus a dedicated `TestExtractRefsLazySiblingAttributesKeepPosition` test prove: both ternary branches suppressed, condition kept; both `&&`/`||` operand orders suppressed on both operators; unary/arithmetic unaffected; for-collection kept, for-key/value/if-condition suppressed; `%{ if }`/`%{ for }` template-directive forms behave identically to their expression forms; nesting (`try()` inside a lazy branch, a lazy ternary followed by a sibling ref, a nested ternary inside a lazy branch) all behave correctly; the lazy stack pops correctly across sibling attributes on different lines.
- `TestExtractRefsShapes`'s `OUT-10` row flipped from expecting two refs to expecting none (renamed `"OUT-10 ternary branches are lazy"`).
- `TestExtractRefsNonASCIIColumn` and `TestWholeBodyRefs` (integration) both moved their non-ASCII reference out of a ternary branch into a list literal (`["ééé", dependency.x.outputs.y]`), so they keep proving byte-vs-rune columns without being suppressed by the new guard. The integration fixture's expected column was hand-recomputed to 17 (16 bytes precede `dependency`: `' a = ["ééé", '`, independently verified with `printf ... | wc -c`) and its stale comment ("column 23", pointing at hclconv_test's unindented form) rewritten.
- Rewrote `extractRefs`' doc comment table and `refWalker`'s doc comment to describe both guards and list the lazy-evaluation shapes explicitly.
- `02-TERRAGRUNT-EDGECASES.md`: Quick reference table rows for OUT-09/OUT-10 updated; both sections gained a "Reversed by gap G1 (02-06)" paragraph explaining the hclsyntax evidence (per-branch/per-operand/per-element diagnostics dropping) and why the original "resolve all" call was a false-positive risk, plus a new paragraph on `&&`/`||` short-circuiting.

## Task Commits

Each task was committed atomically:

1. **Task 1 RED: add failing lazy-evaluation reference cases** - `f27f31d` (test)
2. **Task 1 GREEN: suppress references in lazily evaluated sub-expressions** - `31ee3de` (fix)
3. **Task 2a: keep byte-column integration check outside lazy constructs** - `5e13e2b` (test)
4. **Task 2b: record OUT-09/OUT-10 reversal in edge-case catalogue** - `4f169bd` (docs)

## Files Created/Modified

- `internal/infrastructure/terragrunt/refs.go` - `lazyRanges` helper, `refWalker.lazy` stack, `inLazyRange`, rewritten doc comments
- `internal/infrastructure/terragrunt/refs_test.go` - new `TestExtractRefsLazyEvaluation` (21 rows) and `TestExtractRefsLazySiblingAttributesKeepPosition`; flipped `OUT-10`; moved the non-ASCII reference out of the ternary
- `internal/infrastructure/terragrunt/integration_test.go` - `TestWholeBodyRefs`'s `inc.hcl` fixture moved out of the ternary; expected byte column recomputed 23 -> 17
- `.planning/phases/02-parsing-graph-construction/02-TERRAGRUNT-EDGECASES.md` - OUT-09/OUT-10 quick-reference rows and sections updated with the G1 reversal

## Decisions Made

- Followed 03-RESEARCH.md Pattern 2's sketch exactly (byte-range stack, same three node-shape cases) rather than inventing an alternative representation, since Phase 3's 03-01 plan already assumes this exact guard exists.
- Kept `guard int` (try/can) and `lazy []hcl.Range` (lazy-evaluation) as two separate fields on `refWalker` rather than unifying them, since they have different semantics (a count vs. a set of ranges) and the plan's context explicitly showed them as parallel mechanisms.

## Deviations from Plan

None - plan executed exactly as written. The byte-column recomputation (23 -> 17) was explicitly called for by the plan itself, not a deviation; it was independently verified with `printf ' a = ["ééé", ' | wc -c` (16 bytes) before editing, per the plan's instruction.

## Issues Encountered

- The plan's verification commands are written as `cd /home/giulio/gruntled && ...`, which is the main checkout, not this worktree. Per this execution's own parallel-worktree instructions, all commands were run against this worktree's absolute path instead; behavior was identical since the worktree's `go.mod`/module path match the main checkout.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- `refWalker`'s lazy-evaluation guard is in place and tested; Phase 3's 03-01 plan (03-RESEARCH.md Pattern 2, Pitfall 2) can rely on it as already built.
- Full local suite green: build, vet, `go test ./...` (this worktree — 65-unit corpus smoke test also run manually and passed: `TestCorpusSmoke` PASS against `$HOME/.cache/gruntled-phase4/corpus/primary`), gofmt clean, `go mod tidy -diff` clean, staticcheck v0.8.1 clean, `scripts/check-architecture.sh` and `scripts/test-check-architecture.sh` both pass (all 22 architecture self-test probes PASS).
- Not pushed: this plan ran in a parallel worktree per its own instructions; the orchestrator merges and pushes.

---
*Phase: 02-parsing-graph-construction*
*Completed: 2026-09-28*

## Self-Check: PASSED

All key files found on disk (refs.go, refs_test.go, integration_test.go, 02-TERRAGRUNT-EDGECASES.md, this SUMMARY). All 4 task commits (f27f31d, 31ee3de, 5e13e2b, 4f169bd) found in `git log`.
