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

---

# Round 2 — Confirmed Gaps After 02-06..02-11 (input for gap cycle 1: 02-12..02-14)

Sources: 02-VERIFICATION.md (2026-09-28, the partial G3 truth) plus a second
verifier/code-review/architect pass. Every item below was reproduced by a reviewer in a
throwaway copy. As in round 1, each fix needs a failing test or fixture first, and every
fix fails toward unknown. G15/G16 are one reviewer finding (F1) split into its two
mechanisms. Plan mapping: G17, G18, G20 go to 02-12; G15, G16, G19 go to 02-13 (which
also owns the catalogue); G21, G22 go to 02-14.

## False-positive sources (blockers first)

- **G15 [blocker]: include-target guard keyed by the lexical path.** loader.go
  resolveIncludes records `located[p] = true` with the lexical include path, and step 13
  looks up `<unit dir>/terragrunt.hcl`. An include that reaches the parent through an
  in-repo symlink (`../../link/terragrunt.hcl` with `link -> parent`, a symlinked
  directory, or a symlinked file) records `live/link/terragrunt.hcl`, so the real parent
  stays resolved and is analysed standalone, which gives the false GRT001 of
  TestIncludeTargetCorpusReproduction. Fix: compare canonical in-repo paths (resolve
  every symlink inside the repo through fs.ReadLinkFS). A link that escapes the repo, or
  one that cannot be resolved, fails closed. Test: a real-FS fixture (t.TempDir +
  os.Symlink + os.OpenRoot, like realfs_test.go). File: loader.go. Plan 02-13.
- **G16 [blocker]: a parent is only marked when its includer resolves.** `located` is
  written only after the includer passes the JSON check, read/syntax, validateIncludeDecls
  and evalPath. Reproductions, all against the TestIncludeTargetCorpusReproduction
  fixture, leave the parent `live` resolved and bring back its false GRT001:
  - the child includes `"${get_repo_root()}/live/terragrunt.hcl"` (an unsupported
    function, so the path is dynamic);
  - the same with `get_path_to_repo_root()`;
  - the child uses `find_in_parent_folders()` and also has a syntax error somewhere else.
  Fix direction (fail closed): when a unit's includes cannot be known (steps 1-2),
  mark every ancestor-directory unit as include-target, since that is the reach of
  find_in_parent_folders. When the include blocks are parsed, evaluate every decl for
  marking even if the unit itself fails (steps 3-4). A dynamic path whose target is not
  confined to ancestors needs a stronger rule: the executor evaluates it and the
  residual goes in the catalogue. Coverage on the primary corpus must not drop (65
  units, 3 config-unknown, 22 refs); measure it. File: loader.go. Plan 02-13.
- **G19 [major, false positive]: generate output detector is a line regex.**
  merge.go:279 `(?m)^\s*output\b|"output"\s*:` misses `/* generated */ output "id" {
  value = 1 }` (and any output block that doesn't start its line) in literal generate
  contents. The module surface is under-counted and GRT001 fires falsely. Fix: the most
  conservative rule that keeps the primary corpus unchanged (it has no generate blocks).
  File: merge.go. Plan 02-13.
- **G20 [major, false positive]: OpenTofu precedence drops x.tf when x.tofu exists.**
  tfsurface/reader.go:254. Terraform ignores `.tofu` files, and which binary runs is
  unknown statically, so dropping either view under-counts. Repro: outputs.tf declares
  `id`, outputs.tofu declares `other`, and a reference to `id` is reported missing. Fix:
  take the union of both views (over-counting outputs is safe). File:
  tfsurface/reader.go. Plan 02-12.

## Robustness

- **G17 [blocker]: CheckNativeDepth ignores ternary nesting.** hclsyntax
  parseTernaryConditional recurses through ParseExpression for BOTH the true and the
  false branch. `strings.Repeat("1?", n)+"1"+strings.Repeat(":1", n)` and the else-chain
  `strings.Repeat("a?b:", 1_000_000)+"1"` (4,000,019 bytes, under the 4 MiB cap) both
  die with an unrecoverable fatal stack overflow. A naive push-on-`?`/pop-on-`:` misses
  the else-chain, where depth would stay at 1. Related, same pass: a 2M-long `+` chain
  does not crash but peaks at about 2.2 GB and builds a 2M-deep left-leaning AST that
  every later recursive walk descends. Files: hclconv/limits.go, with regressions in
  hclconv, terragrunt/limits_test.go and tfsurface. Plan 02-12.
- **G18 [major]: a non-regular file hangs the process.** walk.go discoverUnits accepts a
  FIFO named terragrunt.hcl, and tfsurface only skips directories. hclconv.ReadFileLimited
  trusts Stat().Size() and then fs.ReadFile blocks forever on the FIFO. Fix:
  ReadFileLimited requires a regular file and reads through io.LimitReader, plus
  regular-file checks in discoverUnits and tfsurface. Test with syscall.Mkfifo behind a
  unix build tag. Files: hclconv/limits.go, terragrunt/walk.go, tfsurface/reader.go.
  Plan 02-12.

## Architecture check holes

- **G21 [minor]: the source-level import scan is an awk heuristic.** In a `_windows.go`
  file, `import ( /* x */ _ "github.com/hashicorp/hcl/v2" )` and `import ( _ "fmt"; _
  "github.com/hashicorp/hcl/v2" )` both get past scan_import_lines. Fix: a small Go
  helper (go/parser ImportsOnly over every .go file regardless of build constraints,
  pruning the dirs the go tool ignores) that the script runs with `go run`. It lives
  outside internal/ and is never linked into cmd/gruntled. Both probes go into the
  self-test. Files: scripts/check-architecture.sh, scripts/test-check-architecture.sh, the
  helper. Plan 02-14.
- **G22 [minor]: nested go.mod bypass.** `internal/domain/zz/go.mod` (module path under
  internal/domain), with require+replace in the root go.mod, lets the domain import `os`
  while the check prints OK. go list ./internal/domain/... never enters the nested module,
  and domain-external-deps' allow regex matches its path. Fix: fail on any go.mod other
  than the root (pruning dot/underscore dirs), and on any required or replaced module
  whose path is under the main module path. Add a self-test case. Plan 02-14.
