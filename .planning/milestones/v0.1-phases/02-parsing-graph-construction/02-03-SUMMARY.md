---
phase: 02-parsing-graph-construction
plan: 03
subsystem: infrastructure
tags: [go, hcl, hcl-v2, go-cty, terragrunt, hexagonal, adapters]

# Dependency graph
requires:
  - phase: 02-parsing-graph-construction (plan 01)
    provides: config-unknown/module-unknown unit split, NewSurface, NewModule/NewUnknownModule, Tristate, byte-column Position, diagnostic.Key.Unit
  - phase: 02-parsing-graph-construction (plan 02)
    provides: "ports.UnitLoader/SurfaceReader contracts, hcl-only-in-infrastructure arch rule landed before this plan's first commit"
provides:
  - "internal/infrastructure/hclconv: Position (byte-accurate, recomputed from hcl.Pos.Byte) and FirstSyntaxError (one deterministic GRT100 per file, tie-broken by Summary/Detail)"
  - "internal/infrastructure/sourceresolve: Classify(raw) -> Source{Kind, Root, Subdir}, pure/offline, mirrors Terragrunt's detector chain with the FileDetector catch-all"
  - "internal/infrastructure/terragrunt: the six Terragrunt path functions (get_terragrunt_dir, get_original_terragrunt_dir, find_in_parent_folders, path_relative_to_include, path_relative_from_include, get_parent_terragrunt_dir) closed over S0/S1/S2 evaluation scopes, plus evalPath/resolvePath/literalString/literalBool"
  - "internal/infrastructure/tfsurface.Reader: a working ports.SurfaceReader implementation reading .tf/.tf.json/.tofu/.tofu.json module surfaces, with OpenTofu precedence, ignored-file handling, in-repo symlink following via os.Root, and the module-file-unreadable > syntax-error > invalid-module-block > no-terraform-files precedence"
affects: [02-04, 02-05, phase-3-analysis]

# Tech tracking
tech-stack:
  added:
    - "github.com/hashicorp/hcl/v2 v2.25.0 (direct)"
    - "github.com/zclconf/go-cty v1.19.0 (direct, promoted from indirect in Task 2)"
  patterns:
    - "Every hcl.Pos becomes a domain Position through hclconv.Position, which recomputes the column from p.Byte rather than trusting hcl.Pos.Column (grapheme clusters)"
    - "The six path functions are built once per evalScope (fsys, unitDir, scopeKind, includes, included) and closed over an hcl.EvalContext with Variables: nil, so any construct outside the six fails closed automatically rather than needing a manual denylist"
    - "resolvePath never runs path.Clean on a still-'/'-rooted string to detect an escape (it clamps silently); it strips the virtual-root prefix into a relative path first, then checks for a leading '..' "
    - "tfsurface.Reader collects every kept file's names into a set and only decides UnknownReason vs NewSurface after the whole directory is processed, so a syntax error in one file never lets a partial surface from other files stand in for it"
  library-choices:
    - "Hand-rolled sourceresolve.Classify and tfsurface.Reader over go-getter/v2 and terraform-config-inspect respectively (both rejected in 02-RESEARCH.md: go-getter diverges from Terragrunt's actual chain and adds ~10 modules; terraform-config-inspect ignores .tofu/.tofu.json and returns partial results on syntax errors, which would under-count a surface)"

key-files:
  created:
    - internal/infrastructure/hclconv/hclconv.go
    - internal/infrastructure/hclconv/hclconv_test.go
    - internal/infrastructure/sourceresolve/classify.go
    - internal/infrastructure/sourceresolve/classify_test.go
    - internal/infrastructure/terragrunt/doc.go
    - internal/infrastructure/terragrunt/pathfuncs.go
    - internal/infrastructure/terragrunt/eval.go
    - internal/infrastructure/terragrunt/pathfuncs_test.go
    - internal/infrastructure/tfsurface/reader.go
    - internal/infrastructure/tfsurface/reader_test.go
    - internal/infrastructure/tfsurface/realfs_test.go
  modified:
    - go.mod
    - go.sum

key-decisions:
  - "get_terragrunt_dir/get_original_terragrunt_dir are registered with Params: nil and no VarParam, so hcl/cty itself rejects any argument before the Impl ever runs (get_terragrunt_dir(\"extra-arg\") fails closed for free, no manual arity check needed)"
  - "path_relative_to_include/path_relative_from_include/get_parent_terragrunt_dir share two small selector helpers (toIncludeRel/fromIncludeRel) that encode the 0/1/2+-includes selection rule once, since get_parent_terragrunt_dir's S1 case (V(clean(unit + from))) is literally fromIncludeRel's result joined against unitDir"
  - "tfsurface.Reader always calls fs.Stat on every kept entry (even plain regular files) rather than branching on the DirEntry's reported type first: this was verified against a real os.Root-backed FS to be exactly the point where an escaping or dangling symlink surfaces as an error (statat: path escapes from parent / no such file or directory), so one code path handles directories, in-repo symlinks and escapes/dangling links uniformly"
  - "parseModuleFile trusts that hclsyntax.ParseConfig and hcljson.Parse always return a non-nil *hcl.File even for mid-edit or non-UTF-8 source (research Pattern 8, independently re-verified for hclconv in Task 1), so there is no nil-Body guard code that only exists for tests to skip"

patterns-established:
  - "Pattern: closed hcl.EvalContext (Variables nil, an explicit small Functions map) as the only way this codebase ever evaluates an HCL expression, reused identically by literalString/literalBool (functions nil) and evalPath (the six path functions) — never a custom mini-interpreter"

requirements-completed: [PARSE-02, PARSE-05, GRAPH-01, GRAPH-03, GRAPH-05]

# Metrics
duration: 53min
completed: 2026-09-25
---

# Phase 2 Plan 3: hcl/v2 adapter leaves — byte-accurate positions, source classifier, Terragrunt path functions, module surface reader Summary

**Four new `internal/infrastructure` packages (`hclconv`, `sourceresolve`, `terragrunt`, `tfsurface`) give Plans 04-05 byte-accurate HCL positions, an offline source classifier, closed evaluation of the six Terragrunt path functions across all three scopes, and a working `ports.SurfaceReader` that follows the primary corpus's in-repo `.tf` symlinks while refusing escapes.**

## Performance

- **Duration:** 53 min (Task 1 started 14:14, Task 3 finished 15:07)
- **Started:** 2026-09-25T14:14:26+02:00
- **Completed:** 2026-09-25T15:07:46+02:00
- **Tasks:** 3
- **Files modified:** 13 (11 new, 2 edited: go.mod, go.sum)

## Accomplishments
- `hclconv.Position` recomputes every hcl position's column from `hcl.Pos.Byte` (never `hcl.Pos.Column`, which counts grapheme clusters); the non-ASCII test asserts byte column 22 against hcl's own grapheme column 19 for a `"ééé"`-prefixed line, independently verified in Python before trusting the Go test. `hclconv.FirstSyntaxError` collapses a file's error diagnostics into exactly one deterministic GRT100, and never panics on mid-edit or non-UTF-8 source.
- `sourceresolve.Classify` mirrors Terragrunt's detector chain (forced getter, URL scheme, host shorthand, then the `FileDetector` catch-all that makes a bare `units/chicken` local, unlike plain Terraform) with zero filesystem or network access; every row of the plan's classifier table is a test case.
- `internal/infrastructure/terragrunt` implements the six path functions exactly per research Pattern 4's S0/S1/S2 table, including the Terragrunt docs example (`"${path_relative_from_include()}/../sources//${path_relative_to_include()}"` → `"../../../sources//secrets/mysql"`), `find_in_parent_folders`' json-before-hcl probe order and repo-root search boundary, and `resolvePath`'s escape-safe virtual-root/relative/absolute resolution (never `path.Clean` on a still-`/`-rooted string). `go-cty` becomes a direct dependency.
- `tfsurface.Reader` reads a module directory's `variable`/`output` names from `.tf`, `.tf.json`, `.tofu` and `.tofu.json`, with OpenTofu precedence, Terraform's ignored-file rules, override-file deduplication through a set (never tripping `NewSurface`'s duplicate check), and a fixed unknown-reason precedence (`module-file-unreadable` > `syntax-error` > `invalid-module-block` > `no-terraform-files`). `realfs_test.go` proves the primary corpus's in-repo symlink shape is followed through a real `os.Root`, while an escaping or dangling symlink gives `module-file-unreadable` (empirically verified against `os.Root`'s actual error strings, not assumed from documentation).

## Task Commits

Each task was committed atomically:

1. **Task 1: Add hcl/v2; byte-accurate positions and GRT100; offline source classifier** - `fbc9c6b` (feat)
2. **Task 2: The six path functions and closed evaluation of path-bearing expressions** - `7adf45b` (feat)
3. **Task 3: Module surface reader (SurfaceReader port)** - `5327e84` (feat)

**Plan metadata:** (this commit) `docs(02-03): complete hcl/v2 adapter leaves plan`

## Files Created/Modified
- `internal/infrastructure/hclconv/hclconv.go` - `Position` (byte-column conversion), `FirstSyntaxError` (GRT100 collapse)
- `internal/infrastructure/sourceresolve/classify.go` - `Classify`, `Kind`, `Source`, pure and offline
- `internal/infrastructure/terragrunt/doc.go` - package doc: the Terragrunt anti-corruption layer, structural decoding only
- `internal/infrastructure/terragrunt/pathfuncs.go` - `virtualRoot`/`virtual`, `scopeKind` (S0/S1/S2), `includeRef`, `evalScope`, `functions()`, the six functions' implementations, `relDir`
- `internal/infrastructure/terragrunt/eval.go` - `evalPath`, `resolvePath`, `literalString`, `literalBool`
- `internal/infrastructure/tfsurface/reader.go` - `Reader`, `NewReader`, `ReadSurface`, the five `Reason*` constants, `keepModuleFiles`/`moduleExt`/`isIgnoredFileName`
- `go.mod` / `go.sum` - `hashicorp/hcl/v2 v2.25.0` and `zclconf/go-cty v1.19.0` both direct requires

## Decisions Made
See `key-decisions` in the frontmatter for the four most consequential ones (no-VarParam arity rejection, shared include-selector helpers, always-Stat in the surface reader, trusting hcl's non-nil-File guarantee).

## Deviations from Plan

None - plan executed exactly as written. Every `<behavior>` bullet across all three tasks has a corresponding test, and every `<done>` criterion was independently verified (Task 1's commit was inherited from a prior session and re-verified against its own `<verify>` command before Task 2 began, including an independent Python check of the byte/grapheme column math since the plan's illustrative "column 23" text differs slightly from the actual test string's arithmetic — the committed test asserts the mathematically correct value, 22).

## Issues Encountered

None.

## User Setup Required

None - no external service configuration required.

## Next Phase Readiness

- Plans 04-05 (the Terragrunt walker/parser/loader that make `internal/infrastructure/terragrunt` a complete `ports.UnitLoader`) can now call `hclconv.Position`/`FirstSyntaxError` for every diagnostic, `sourceresolve.Classify` for `terraform.source`, `evalPath`/`resolvePath` for the three path-bearing attributes, and hand a finished `tfsurface.Reader` to `indexing.Build` as-is — it already satisfies `ports.SurfaceReader` today.
- `go.mod`/`go.sum` changes are now closed for the rest of Phase 2: this was the only plan permitted to touch them, and `go mod tidy -diff` is clean.
- Full local suite green at `5327e84`: build, vet, test (all packages), gofmt, `go mod tidy -diff`, staticcheck, govulncheck (no vulnerabilities), cross-builds for linux/amd64, darwin/arm64, windows/amd64, `check-architecture.sh` and `test-check-architecture.sh`. Push and CI confirmation follow this summary.
- No blockers.

---
*Phase: 02-parsing-graph-construction*
*Completed: 2026-09-25*

## Self-Check: PASSED

All 11 created infrastructure files, this SUMMARY.md, and all three task commit hashes (fbc9c6b, 7adf45b, 5327e84) verified present.
