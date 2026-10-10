---
phase: 01-domain-foundation-test-substrate
plan: 03
subsystem: testing
tags: [go, math-rand-v2, deterministic-generation, sha256, test-fixtures]

# Dependency graph
requires:
  - phase: 01-domain-foundation-test-substrate
    provides: "repograph.RepoPath/Position and diagnostic.Code/CodeUnknownOutput (01-01); scripts/check-architecture.sh's binary-links-testsupport rule (01-02)"
provides:
  - "internal/testsupport/synthrepo: Spec/ErrorKind/Validate, Render(spec) (Tree, Manifest, error) pure in-memory generator, Generate(spec, destDir) (Manifest, error) disk writer"
  - "Deterministic synthetic Terragrunt repository fixtures (byte-identical across runs/processes for a given Spec+Seed) for Phases 2-4 golden tests"
  - "BadOutputRef error injection with an exact Manifest oracle (unit, dependency, output, line, column) independently proven by a regexp-based scan in generate_test.go"
affects: [02-parser, 03-cli, 04-validation]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "math/rand/v2 PCG with a single documented rng-consumption order (unit identity, then dependency edges, then error injection) so the digest is a stable, auditable function of Spec+Seed"
    - "Position tracking via byte-offset capture during bytes.Buffer construction (buf.Len() recorded mid-write), converted to line/column only after the buffer is finalized, rather than re-parsing output"
    - "Tree.Digest() = SHA-256 over Path+\\x00+Content+\\x00 per file in sorted-Path order, used both as the pinned-digest regression test and as the equality check between Render's in-memory tree and Generate's on-disk tree"
    - "Test-local oracle scan (regexp over generated terragrunt.hcl/main.tf, independent lineCol reimplementation) proves the Manifest is exact, not just plausible, without importing any synthrepo-internal helper"

key-files:
  created:
    - internal/testsupport/synthrepo/spec.go
    - internal/testsupport/synthrepo/manifest.go
    - internal/testsupport/synthrepo/render.go
    - internal/testsupport/synthrepo/render_test.go
    - internal/testsupport/synthrepo/generate.go
    - internal/testsupport/synthrepo/generate_test.go
  modified: []

key-decisions:
  - "Injected-output naming uses 0-based ordinals (missing_out_0, missing_out_1, ...) matching spec.Errors iteration order; no test asserts the literal string, only that it is absent from the target module's declared outputs, so the exact numbering was Claude's discretion per plan"
  - "relPath and lineCol are pure string/byte helpers with zero filepath/regexp dependency in render.go itself, keeping Render fully I/O-free as the plan's layering note requires"
  - "generate_test.go's oracle scan reimplements lineCol locally (localLineCol) rather than reusing any exported helper, so it is a genuinely independent proof that Manifest.Expected is both complete and exact, not a tautology"

requirements-completed: [VALID-01]

# Metrics
duration: ~20min
completed: 2026-09-25
---

# Phase 1 Plan 03: Deterministic Synthetic Repository Generator Summary

**Stdlib-only `internal/testsupport/synthrepo` package: pure `Render(Spec) (Tree, Manifest, error)` plus a thin `Generate(Spec, destDir)` disk writer, producing byte-identical Terragrunt repository trees from a seed and an exact BadOutputRef diagnostic oracle, independently verified by a regexp-based scan.**

## Performance

- **Duration:** ~20 min
- **Started:** 2026-09-25T10:15:00Z
- **Completed:** 2026-09-25T10:35:03Z
- **Tasks:** 2/2 completed
- **Files modified:** 6 created (4 renderer files incl. tests, 2 generator files incl. tests)

## Accomplishments
- `Spec{Units, IncludeDepth, DependencyFanout, Seed, Errors}` with `Validate()` rejecting structurally invalid specs; package doc on `spec.go` precisely documents the generated layout (root.hcl, unit naming, inline modules, dependency/inputs convention, position semantics) for Phase 2's parser tests to rely on
- `Render(spec)`: pure, no I/O, consumes a single `*rand.Rand` built from `rand.NewPCG` in a documented fixed order (unit identity draws, then dependency-edge draws, then error-injection draws); two calls with the same Spec are `reflect.DeepEqual`
- `TestRender_PinnedDigest` hard-codes the SHA-256 digest for `Spec{Units:12, IncludeDepth:3, DependencyFanout:2, Seed:42, Errors:[BadOutputRef]}`, confirmed stable across 5 in-process runs (`-count=5`) and 3 separate `go test` process invocations before being pinned
- `BadOutputRef` injection rewrites one reference's output to `missing_out_<N>`, keeping the input key and consuming variable name untouched (a single-token rename bug), and Render rejects requesting more BadOutputRef injections than the tree has references (e.g. `Units:1` with one error)
- `relPath`/`lineCol` pure helpers verified against the plan's exact tables (3 relPath cases crossing shared/unshared group prefixes, 3 lineCol cases including a mid-line offset)
- `Generate(spec, destDir)`: validates destDir exists, is a directory, and is empty before writing anything (never partial-writes on a bad destination); on-disk digest computed by an independent walk-and-hash helper in the test matches `Render`'s `Tree.Digest()` exactly
- `TestGenerate_ManifestIsExactOracle`: a from-scratch regexp scan of every generated `terragrunt.hcl`/`main.tf` pair (own `localLineCol` reimplementation, no import of synthrepo's unexported helpers) reproduces `Manifest.Expected` exactly — same unit, dependency, output, line and column — for 2 injected `BadOutputRef` errors, and finds zero bad references for a clean spec
- Confirmed `scripts/check-architecture.sh`'s `binary-links-testsupport` rule stays green with `synthrepo` present, and `go list -deps ./cmd/gruntled | grep testsupport` is empty
- Confirmed on GitHub Actions: run `36124721653` for commit `90ce415` concluded `success` for both the `check` and `architecture` jobs

## Task Commits

Each task was committed atomically:

1. **Task 1: Pure renderer: Spec, Manifest, Render** - `d7f84df` (feat)
2. **Task 2: Generate to disk, with determinism and injection-oracle tests** - `90ce415` (feat)

_Both tasks were TDD (`tdd="true"`): tests were written alongside/immediately after each implementation and run to confirm PASS before commit, per the plan's own single-commit-per-task instruction (not split RED/GREEN commits)._

## Files Created/Modified
- `internal/testsupport/synthrepo/spec.go` - package doc (generated layout), `ErrorKind`/`BadOutputRef`, `Spec`, `Validate`
- `internal/testsupport/synthrepo/manifest.go` - `ExpectedDiagnostic`, `Manifest` (domain types: `diagnostic.Code`, `repograph.Position`, `repograph.RepoPath`)
- `internal/testsupport/synthrepo/render.go` - `Render`, `File`, `Tree`, `Tree.Digest`, `relPath`, `lineCol`; the documented PCG-consumption-order algorithm
- `internal/testsupport/synthrepo/render_test.go` - Validate table, determinism, pinned-digest, seed-divergence, shape, clean-manifest, relPath table, lineCol table tests
- `internal/testsupport/synthrepo/generate.go` - `Generate(spec, destDir)`: destDir validation, `os.MkdirAll`/`os.WriteFile` per file
- `internal/testsupport/synthrepo/generate_test.go` - on-disk determinism, injection-position, independent-oracle-scan, and dest-dir-validation tests

## Decisions Made
- Used 0-based ordinals for `missing_out_<N>` naming since no behavior or test depends on the exact numbering, only on the name being absent from the target module's declared outputs
- Kept `render.go` free of any `filepath`/`regexp`/`os` import — `relPath` and `lineCol` are pure string/byte helpers — so `Render` remains provably I/O-free per the plan's locked layering note
- The oracle scan in `generate_test.go` deliberately duplicates a `lineCol`-equivalent function locally instead of exporting/reusing synthrepo's internal one, so the proof that `Manifest` is exact does not depend on trusting the code under test

## Deviations from Plan

None - plan executed exactly as written. The interfaces block's exact type/method signatures, the RNG-consumption-order algorithm, and the file-content templates were precise enough that implementation matched on the first pass; all `<behavior>` bullets were satisfied without needing Rule 1-4 auto-fixes.

## Issues Encountered
- `depRefRE.FindAllStringSubmatchIndex` in `generate_test.go` initially took a `[]byte` argument (`vet` caught it immediately); switched to `FindAllSubmatchIndex` for the `[]byte` overload. Normal within-task correction, not a plan deviation.

## User Setup Required
None - no external service configuration required.

## Next Phase Readiness
- `internal/testsupport/synthrepo` is complete, tested, and provably outside the domain/application/infrastructure/interfaces layers; `cmd/gruntled` does not and cannot link it without breaking `check-architecture.sh`'s `binary-links-testsupport` rule (still green with the package present).
- Phase 2 (HCL parser) can now generate arbitrarily large, deterministic Terragrunt fixture trees via `synthrepo.Generate`, and can validate its own diagnostics against `synthrepo.Manifest.Expected` as a golden oracle for the `BadOutputRef`/`GRT001` case.
- Phase 1 is now fully complete: all 3 plans (domain foundation, architecture enforcement CI, synthetic repo generator) executed, committed, pushed, and green on CI.
- No blockers identified for Phase 2.

---
*Phase: 01-domain-foundation-test-substrate*
*Completed: 2026-09-25*

## Self-Check: PASSED

All 6 created source/test files and this SUMMARY.md were verified present on disk; both task commit hashes (`d7f84df`, `90ce415`) were verified present in `git log --oneline --all`.
