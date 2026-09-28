---
phase: 02-parsing-graph-construction
status: gaps_found
source: post-execution reviews (architect-reviewer, golang-pro, code-reviewer) + 03-RESEARCH.md Patterns 2-4
created: 2026-09-25
---

# Phase 2 — Confirmed Review Gaps (input for `plan-phase 2 --gaps`)

Each reviewer reproduced every item below in a throwaway copy. Every gap needs a failing fixture
or test FIRST, then the fix. Zero false positives is the goal, so every fix fails toward unknown.

## False-positive sources (blockers)

- **G1: lazy-evaluation reference guard** (03-RESEARCH.md Pattern 2, Pitfall 2). A
  `dependency.X.outputs.Y` that appears in a conditional (ternary) branch, in either operand
  position of `&&`/`||` where HCL may not evaluate it, or in a for-expression body/condition
  (an empty collection means it is never evaluated) must yield NO Reference, like try()/can().
  Update refs.go's doc comment and the catalogue deviation table. File: refs.go, refs_test.go.
- **G2: stack-target guard** (Pattern 3). A dependency whose config_path resolves to a directory
  holding `terragrunt.stack.hcl` (and no terragrunt.hcl) is unresolved (a new reason), never a
  target. Files: loader.go, reasons.go.
- **G3: include-target units** (Pattern 4). A `terragrunt.hcl` that other units include (it is a
  parent config) must not be analysed as a standalone unit, because its references resolve per
  including unit. It becomes config-unknown with reason `include-target`. Its references are
  still checked through each including unit. Requires the EXACT corpus reproduction from
  03-RESEARCH.md as a fixture. Files: loader.go, reasons.go.
- **G4: config_path naming a non-default file** (architect B1). `config_path = "../vpc/alt.hcl"`
  is mapped to unit `live/vpc`, but Terragrunt reads alt.hcl, which may set a different source.
  Only a regular file named `terragrunt.hcl` maps to its directory; any other file gives an
  unresolved dependency (`config-path-nondefault-file`). File: loader.go:318-320.
- **G5: valid JSON include gives a false GRT100** (code-review #1). Include targets ending in
  `.json` (explicit `root.hcl.json`, or the no-argument find_in_parent_folders probing
  `terragrunt.hcl.json`) are parsed with hclsyntax. Fix: config-unknown
  `include-json-unsupported`, no diagnostic. Files: parse.go:149, loader.go:211, pathfuncs.go:172.
- **G6: overlay ReadDir failure hides .tf files** (code-review #3). `unitDirOverlaysModule`
  returns false on a ReadDir error. It must make the module unknown
  (`module-file-unreadable`). File: merge.go:299-301.

## Robustness

- **G7: deeply nested or huge HCL crashes the process** (code-review #2). A 3M-deep nesting
  causes a fatal stack overflow in hclsyntax, which cannot be recovered. Fix: a size cap before
  parsing (for example 4 MiB) plus a cheap bracket-depth pre-scan (for example cap 10k,
  string-aware). An oversize or overdeep unit/include file is config-unknown
  `config-too-large`/`config-too-deep`, and a module file makes the module unknown. The
  regression test must not need 6 MB in the repo; generate the input in the test. Files:
  parse.go, tfsurface/reader.go.
- **G8: a malformed config_path removes the whole unit** (golang-pro). If `NewRepoPath` fails on
  the resolved target (for example a backslash), only THAT dependency becomes unresolved
  (`config-path-invalid`). The whole unit must not become config-unknown. File: loader.go:318-325.
- **G9: malformed generate blocks are accepted** (code-review #5). Zero or 2+ labels, or a
  duplicate label in one file, gives config-unknown (an invalid-block reason), matching
  dependency. File: merge.go:277.
- **G10: fuzz only covers one file** (code-review #6). Fuzz a second input written to
  `root.hcl` (reached through `find_in_parent_folders("root.hcl")`) and to `live/vpc/main.tf`.
  File: fuzz_test.go.
- **G11**: the comment at loader.go:81 is wrong (architect N4). Fix the comment, or return
  config-unknown with a valid path.

## Domain invariants (architect B3, N3)

- **G12**: zero values must be invalid everywhere. `NewDependency`, `NewUnresolvedDependency`
  and `NewReference` reject a zero Position. Unit constructors reject zero-value
  Dependency/Reference entries. `NewRepositoryGraph` rejects zero-path units and modules,
  UnitStatus 0 and a zero Module. An unknown module needs a non-empty reason. `Tristate` gets
  `IsValid()`, and dependency constructors reject invalid option values. Files:
  internal/domain/repograph/*.

## Architecture check holes (architect B2, N5; code-review #4)

- **G13**: new top-level `internal/*` dirs are unpoliced. Rule `internal-layout`: every
  `internal/<x>` must be one of {domain, application, infrastructure, interfaces, testsupport}.
  (`interfaces` is reserved for Phase 3 presenters; that phase adds its own
  interfaces-external-deps rule.) Rule `infrastructure-importers`: only `cmd/...` and
  `internal/infrastructure/...` may import `internal/infrastructure/...`. Rule
  `testsupport-only-in-tests`: only `_test.go` files may import `internal/testsupport`.
- **G14**: `hcl-only-in-infrastructure` misses build-constrained files. Add a source-level scan
  of every `*.go` file outside internal/infrastructure (including tagged and `_GOOS` files)
  for `"github.com/(hashicorp|zclconf)/` imports.
- Each new or strengthened rule gets a self-test case asserting its exact
  `=== RULE FAILED: <name> ===`. Probes: `internal/analysis/a.go` importing os + infrastructure;
  `cmd/gruntled/zz_windows.go` importing hcl; `internal/testsupport/zz/zz.go` with
  `//go:build integration` importing go-cty. Existing rules are never weakened.
  File: scripts/check-architecture.sh, scripts/test-check-architecture.sh.

## Not taken now (recorded)
- Architect N1 (explicit state enum in the UnitConfig DTO) and N2 (a single `TargetSurface`
  graph query). Phase 3 plan 03-01 already encodes the two-level check in the analyzer.
  Revisit if a second analyzer appears.
