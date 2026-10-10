---
phase: 02-parsing-graph-construction
plan: 04
subsystem: infrastructure
tags: [go, hcl, hcl-v2, terragrunt, hexagonal, parsing]

# Dependency graph
requires:
  - phase: 02-parsing-graph-construction (plan 01)
    provides: DependencyOptions/Tristate/NameList, config-unknown/module-unknown unit split, byte-column Position
  - phase: 02-parsing-graph-construction (plan 03)
    provides: hclconv.Position/FirstSyntaxError, the six path functions and closed evalPath, literalString/literalBool
provides:
  - "internal/infrastructure/terragrunt.discoverUnits: fs.WalkDir-based unit discovery, skipping .git/.terraform/.terragrunt-cache/vendor, never following or counting a symlinked directory or terragrunt.hcl/terragrunt.hcl.json, walking .terragrunt-stack on purpose"
  - "internal/infrastructure/terragrunt.extractRefs: whole-body AST walk extracting every static dependency.<name>.outputs.<out> traversal, sorted by byte offset, with a try()/can() guard and no evaluation"
  - "internal/infrastructure/terragrunt.fileCache/parsedFile: parse-once structural cache recording include/terraform/dependency/generate blocks as unevaluated hcl.Expression, plus refs, per file; a syntax error yields one GRT100 and zero facts"
  - "internal/infrastructure/terragrunt.dependencyOptions: DIAG-03 facts (enabled, skip_outputs, mock_outputs keys, mock_outputs_merge*_with_state, mock_outputs_allowed_terraform_commands) captured as literal or unknown, never interpreted"
affects: [02-05, phase-3-analysis]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "discoverUnits relies on fs.WalkDir never recursing into a non-directory DirEntry: the fs.ModeSymlink guard exists only to keep a symlinked terragrunt.hcl/terragrunt.hcl.json from matching the name switch, not to prevent descent (WalkDir already refuses that on its own)"
    - "extractRefs and dependencyOptions never evaluate an expression to decide whether it is a reference or a literal fact: outputRef inspects the raw hcl.Traversal shape, and dependencyOptions/mockOutputKeys/allowedCommands inspect the raw *hclsyntax.ObjectConsExpr/*hclsyntax.TupleConsExpr AST plus literalString/literalBool, so an uncertain construct fails to unknown by construction rather than by a runtime evaluation error"
    - "fileCache.parse treats an internal hclconv/extractRefs error exactly like an unreadable file (readErr), never a panic, even though that path should be unreachable given hcl's own guarantees"

key-files:
  created:
    - internal/infrastructure/terragrunt/walk.go
    - internal/infrastructure/terragrunt/walk_test.go
    - internal/infrastructure/terragrunt/refs.go
    - internal/infrastructure/terragrunt/refs_test.go
    - internal/infrastructure/terragrunt/parse.go
    - internal/infrastructure/terragrunt/depfacts.go
    - internal/infrastructure/terragrunt/parse_test.go
    - internal/infrastructure/terragrunt/testhelpers_test.go

key-decisions:
  - "mock_outputs_merge_strategy_with_state recognizes exactly three literal values (no_merge=false, shallow/deep_map_only=true); any other literal string (e.g. \"weird\") is Unknown rather than guessed true, since Terragrunt itself does not accept it either -- this reads more conservatively than the DependencyOptions.MockMergeWithState doc comment's simplified \"any value other than no_merge\" wording, and the plan's own behavior table (not the Phase 1 doc comment) was followed as the executable spec"
  - "countingFS added in this plan's Task 3 (not Task 1) per the plan's explicit staticcheck note: an unused test helper in an earlier commit would fail staticcheck's dead-code check"
  - "TestDependencyOptionsNoAttributes and assertNameListEqual compare DependencyOptions/NameList field-by-field rather than with ==, since NameList holds a slice and is therefore not comparable"

patterns-established:
  - "Pattern: a table test per uncertain-construct-to-unknown decision point (try/can guard, whole-object reference, dynamic index, expanded dependency, non-literal mock_outputs/allowed-commands, unrecognized merge strategy string), matching research Pattern 5's verified-shapes table and the plan's DIAG-03 fact table one-for-one"

requirements-completed: [PARSE-01, PARSE-05, PARSE-06]

# Metrics
duration: 27min
completed: 2026-09-25
---

# Phase 2 Plan 4: Terragrunt structural parser (walk, references, parse-once cache, dependency facts) Summary

**`internal/infrastructure/terragrunt` gained unit-discovery walking, whole-body `dependency.X.outputs.Y` reference extraction with byte-accurate positions, a parse-once file cache recording every structural fact as unevaluated `hcl.Expression`, and tri-state DIAG-03 dependency-option capture — all leaning toward `unknown` at every uncertain AST shape.**

## Performance

- **Duration:** 27 min
- **Started:** 2026-09-25T15:07:46+02:00
- **Completed:** 2026-09-25T15:34:48+02:00
- **Tasks:** 3
- **Files modified:** 8 (8 new)

## Accomplishments
- `discoverUnits` walks an `fs.FS` with `fs.WalkDir`, skipping `.git`, `.terraform`, `.terragrunt-cache` and `vendor`, never descending into or counting a symlinked directory or a symlinked `terragrunt.hcl`/`terragrunt.hcl.json`, while walking `.terragrunt-stack` on purpose (PROJECT.md over research/PITFALLS.md §6). Verified against decoy directories, a symlink cycle (returns promptly, proven with a 5-second `select`/`time.After` guard), a real `os.Root`-backed temp tree with a symlink escaping the repo, and a 50-unit `synthrepo.Render` tree whose discovered dirs equal `Manifest.Units` exactly.
- `extractRefs` walks a file's whole `*hclsyntax.Body` with an `hclsyntax.Walker`, collecting every static `dependency.<name>.outputs.<out>` traversal (research Pattern 5's verified-shapes table, one test case per row) while a `try()`/`can()` guard counter suppresses matches nested inside either call and restores correctly for a sibling reference after the guarded one. Covers `locals`, nested blocks (`terraform.extra_arguments`, `remote_state.config`, `dependency.mock_outputs`, the ordering-only `dependencies.paths`), templates and heredocs, `for` expressions and both ternary branches, and asserts a byte-accurate (not grapheme-accurate) column on a non-ASCII-prefixed line, and stable source order across 20 re-parses of a 10-reference file (`hclsyntax.Body.Attributes` is a map; `extractRefs` sorts by byte offset regardless).
- `fileCache`/`parsedFile` parse each file at most once (proven with a `countingFS` wrapping `fstest.MapFS`), recording `include`/`terraform`/`dependency`/`generate` blocks as `includeDecl`/`terraformDecl`/`depDecl`/`generateDecl` with unevaluated `hcl.Expression` fields, in source order (`body.Blocks` is already ordered; only `body.Attributes`, accessed by fixed key here, is a map). A syntax error (mid-edit or non-UTF-8 source) yields exactly one file-level GRT100 and zero facts, matching hcl's own guarantee of a non-nil partial body that must never be analysed; `syntaxDiagnostics()` reports one GRT100 per broken file `get()` actually touched, sorted by path, and nothing for an untouched or clean file.
- `dependencyOptions` captures `enabled`, `skip_outputs`, `mock_outputs` (key set via `*hclsyntax.ObjectConsExpr` inspection, rejecting a computed or duplicate key as unknown), `mock_outputs_merge_with_state` combined with `mock_outputs_merge_strategy_with_state` (either literally true wins; an unrecognized literal string is unknown, not guessed true), and `mock_outputs_allowed_terraform_commands` (via `*hclsyntax.TupleConsExpr` inspection) — every fact a `repograph.Tristate`/`NameList`, table-tested row by row including the primary corpus's two-key/`merge_with_state`/three-command shape (DEP-08).

## Task Commits

Each task was committed atomically:

1. **Task 1: Unit discovery walk (PARSE-06)** - `44e6862` (feat)
2. **Task 2: Whole-body reference extraction (PARSE-01, byte positions)** - `1c1ea2f` (feat)
3. **Task 3: Parse-once file cache with structural facts and dependency option facts (PARSE-01, PARSE-05)** - `04edb63` (feat)

**Plan metadata:** (this commit) `docs(02-04): complete Terragrunt structural parser plan`

## Files Created/Modified
- `internal/infrastructure/terragrunt/walk.go` - `unitEntry`, `skipDirNames`, `discoverUnits`
- `internal/infrastructure/terragrunt/walk_test.go` - decoy directories, `.terragrunt-stack`, symlinked dir/file/cycle, JSON-config merge, nested units, `vendor-x` prefix, synthrepo scale, real-FS outside symlink
- `internal/infrastructure/terragrunt/refs.go` - `extractRefs`, `refWalker` (try/can guard), `outputRef`, `rawRef`
- `internal/infrastructure/terragrunt/refs_test.go` - the Pattern 5 shapes table, whole-body/nested-block extraction, templates/heredocs, non-ASCII byte column, 20-run order stability
- `internal/infrastructure/terragrunt/parse.go` - `includeDecl`/`terraformDecl`/`depDecl`/`generateDecl`, `parsedFile`, `fileCache`/`newFileCache`/`get`/`syntaxDiagnostics`/`parse`, `attrExpr`, `hasBlock`
- `internal/infrastructure/terragrunt/depfacts.go` - `dependencyOptions`, `mockOutputKeys`, `mockMergeWithState`, `mergeStrategyState`, `allowedCommands`
- `internal/infrastructure/terragrunt/parse_test.go` - parse-once, missing/mid-edit/non-UTF-8 files, sorted `syntaxDiagnostics`, per-block-type structural fact tests, the full `dependencyOptions` fact table plus the DEP-08 primary-corpus shape
- `internal/infrastructure/terragrunt/testhelpers_test.go` - `mapFS` (Task 1), `countingFS` (Task 3, added at first use per staticcheck)

## Decisions Made
See `key-decisions` in the frontmatter for the three most consequential ones (the merge-strategy-string recognition set, `countingFS`'s placement, and non-comparable `NameList` struct comparison in tests).

## Deviations from Plan

None - plan executed exactly as written. Every `<behavior>` bullet across all three tasks has a corresponding test, and every `<done>` criterion was verified by the task's own `<verify>` command plus the plan's overall `<verification>` full local suite (build, vet, test, gofmt, `go mod tidy -diff`, staticcheck, govulncheck, cross-builds for linux/amd64, darwin/arm64, windows/amd64, `check-architecture.sh` and `test-check-architecture.sh`), all green before push, then confirmed green in CI (`check` and `architecture` jobs, run `36141588687`).

## Issues Encountered

None. Every table test (traversal shapes, dependency-fact combinations, walk decoys/symlinks) passed on its first run against the implementation as written from the plan's `<action>` sections; no shape needed a second iteration to match hcl's actual parsing behavior.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plan 05's loader has everything this plan promised: `discoverUnits` for the walk, `newFileCache`/`fileCache.get` for parse-once access to any file (unit config or include) by `repograph.RepoPath`, each `parsedFile`'s unevaluated `includeDecl`/`terraformDecl`/`depDecl`/`generateDecl`/`refs` to merge and resolve, and `dependencyOptions` already captured on every `depDecl`. Plan 05 only needs to evaluate the three path-bearing attributes (`include.path`, `dependency.config_path`, `terraform.source`) via the existing `evalPath`/`resolvePath` from Plan 03, apply Terragrunt's include-merge precedence, and hand the result to `indexing.Build` through a `ports.UnitLoader`.
- `fileCache.syntaxDiagnostics()` is ready to feed Plan 05's `LoadResult.Diagnostics` directly.
- No blockers. Full local suite and CI both green at `04edb63` (run `36141588687`).

---
*Phase: 02-parsing-graph-construction*
*Completed: 2026-09-25*

## Self-Check: PASSED

All 8 created files and all three task commit hashes (44e6862, 1c1ea2f, 04edb63) verified present.
