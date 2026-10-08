---
phase: 09-blast-radius
plan: 01
subsystem: domain
tags: [blast-radius, impact, diagnostics, surface-diff]
requires:
  - diagnostic.Diagnostic / Set / Key
  - repograph.RepositoryGraph / Module.Surface / Unit.Module
provides:
  - internal/domain/impact (FindingKey, KeyOf, NewFindings, SurfaceChange, SurfaceDiff, BrokenUnit, ImpactedUnit, Result, Compute, NoBaseline)
  - GRT100 message position-stability regression test
affects: [09-02, 09-03, phase-10, phase-11]
tech-stack:
  added: []
  patterns: [pure domain set-difference, deterministic LCG property loop (domain tests stay on the pure allowlist)]
key-files:
  created:
    - internal/domain/impact/impact.go
    - internal/domain/impact/surface.go
    - internal/domain/impact/impact_test.go
    - internal/domain/impact/surface_test.go
    - internal/infrastructure/hclconv/blast_stability_test.go
  modified: []
decisions:
  - GRT100 messages are position-free (5 broken inputs, 3-line shift, byte-identical) so KeyOf keeps Message verbatim for every code
  - nil graphs are treated as empty in SurfaceDiff/Compute
  - Domain property test uses a fixed-seed LCG instead of rapid/fmt: check-architecture.sh applies the domain allowlist to test imports too
requirements-completed: [BLAST-02]  # BLAST-01 advanced (domain core); CLI in 09-02/09-03
metrics:
  duration: 2min
  completed: 2026-10-08
  tasks: 2
  files: 5
---

# Phase 9 Plan 01: Blast Radius Domain Core Summary

Pure `internal/domain/impact` package: position-free finding identity (diagnostic Key minus Line/Column), module surface-name diff, and `Compute` producing sorted, unique, disjoint Broken/Impacted, plus a `NoBaseline` degraded mode.

## Tasks

| # | Task | Commit |
|---|------|--------|
| 1 | GRT100 message stability test | 1bfcf14 |
| 2 | impact package (identity, surface diff, Compute) | 683d333 |

## Task 1 outcome

All 5 broken inputs (unclosed block, missing `=`, unterminated string, bad attribute value, stray `}`) produce byte-identical GRT100 messages when shifted down 3 lines; only Line moves. So `KeyOf` uses Message verbatim, GRT100 included. If an hcl upgrade breaks that, the test fails.

## Semantics implemented

- Broken = findings whose FindingKey is absent from base, grouped by subject (Unit, else Pos().File()). Moving a finding or changing its severity does not make it Broken.
- Impacted = units of the current graph whose `Module()` has a non-empty surface change. Only modules present and known in both trees count. One hop only: edges are never followed. Config-unknown units are skipped. Broken subjects are removed.
- All result slices are non-nil and sorted by `RepoPath.Compare`.

## Verification

- `go test ./internal/domain/... ./internal/infrastructure/hclconv/ -count=1`: pass
- `gofmt -l internal/`: empty
- `scripts/check-architecture.sh`: exit 0
- Mutation check: turning off the Broken-exclusion in Compute fails both TestCompute and TestDisjoint

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] Domain tests cannot import rapid or fmt**
- **Found during:** Task 2
- **Issue:** The plan said fmt is fine in tests and suggested rapid for TestDisjoint. But `check-architecture.sh` runs domain-stdlib-allowlist and domain-external-deps on TestImports/XTestImports too, so both rules failed.
- **Fix:** TestDisjoint now runs a deterministic loop over 2000 seeds driven by a fixed-seed LCG, and `strconv` replaces `fmt`. Test helpers take a small `tb` interface.
- **Files modified:** internal/domain/impact/impact_test.go, internal/domain/impact/surface_test.go
- **Commit:** 683d333

**2. TDD commit granularity:** Task 2's implementation and tests were committed together, not as separate RED and GREEN commits. To show the tests still catch bugs, a mutation check was run instead (see Verification).

## Self-Check: PASSED
