---
gsd_state_version: 1.0
milestone: v0.1
milestone_name: milestone
status: verifying
stopped_at: Completed 04-03-PLAN.md
last_updated: "2026-09-29T11:23:25.118Z"
last_activity: "2026-09-29 — 313f856 narrowed the 02-13 include-target rule (unreachable includes mark nothing); denis256 now 8/8 recall, 0 false positives, include-target 719→54 of 1146"
progress:
  total_phases: 4
  completed_phases: 4
  total_plans: 26
  completed_plans: 26
  percent: 100
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-09-01)

**Core value:** Tell the user, before they run anything slow, that `dependency.X.outputs.Y` does not exist in the module it points to.
**Current focus:** Phase 4 (Real-Repo Validation Experiment) — executing

## Current Position

Phase: 4 of 4 (Real-World Validation) — complete
Plan: 4 of 4 in current phase (04-01, 04-02, 04-03, 04-04 complete)
Phase 4: complete — 4/4 plans, 04-VERIFICATION.md status: passed (2026-09-29)
Phase 3: complete — 5/5 plans, 03-VERIFICATION.md status: passed (2026-09-29)
Phase 2: complete — 14/14 plans, 02-VERIFICATION.md status: passed (re-verified 2026-09-29 after gap cycle 1)
Status: Milestone v1 complete (all 4 phases verified)
Last activity: 2026-09-29 — 313f856 narrowed the 02-13 include-target rule (unreachable includes mark nothing); denis256 now 8/8 recall, 0 false positives, include-target 719→54 of 1146

Progress: [██████████] 100% (26 of 26 plans)

## Performance Metrics

**Velocity:**
- Total plans completed: 8
- Average duration: ~29 min
- Total execution time: 3.99 hours

**By Phase:**

| Phase | Plans | Total | Avg/Plan |
|-------|-------|-------|----------|
| Phase 1 P1 | 35min | 3 tasks | 14 files |
| Phase 1 P2 | 15min | 2 tasks | 3 files |
| Phase 1 P3 | 20min | 2 tasks | 6 files |
| Phase 2 P1 | 25min | 2 tasks | 9 files |
| Phase 2 P2 | 24min | 3 tasks | 5 files |
| Phase 2 P3 | 53min | 3 tasks | 13 files |
| Phase 2 P4 | 27min | 3 tasks | 8 files |
| Phase 2 P5 | 40min | 3 tasks | 7 files |

**Recent Trend:**
- Last 5 plans: 25min, 24min, 53min, 27min, 40min
- Trend: Plan 02-05 (loader + integration + fuzz, the phase's largest wiring surface) ran longer than the P4 baseline but in line with P3's similar-scope leaf-infrastructure plan; Phase 2 is now complete

*Updated after each plan completion*
| Phase 02 P05 | 40 | 3 tasks | 7 files |
| Phase 03 P01 | 20 | 3 tasks | 7 files |
| Phase 03 P03 | 40 | 3 tasks | 16 files |
| Phase 03 P05 | 14 | 2 tasks | 2 files |
| Phase 04 P03 | 35 | 2 tasks | 3 files |

## Accumulated Context

### Decisions

Decisions are logged in PROJECT.md Key Decisions table.
Recent decisions affecting current work:

- Roadmap: v0.1 is a falsifiable experiment — Phase 4 (real-repo validation) is the terminal phase; a failure there stops the project rather than triggering more feature work
- Roadmap: VALID-01 (synthetic repo generator) placed in Phase 1, not late — it is the test substrate for Phases 2-4, per research build-order guidance
- Roadmap: own HCL parser confirmed (not the Terragrunt library) — see PROJECT.md Key Decisions for the compile-verified rationale
- REQUIREMENTS.md stated "27 total" but 28 unique requirement IDs are actually listed (PARSE 6 + GRAPH 5 + DIAG 4 + CLI 5 + VALID 6 + ARCH 2 = 28); traceability corrected to 28/28 mapped
- [Phase 1]: go.mod declares go 1.27; both go1.26.8 and go1.27.0 toolchains were already cached, so GOTOOLCHAIN=auto switched with zero network calls
- [Phase 1]: RepositoryGraph aggregate root sorts and defensively clones units/modules at construction time so any input order yields an identical graph
- [Phase 1]: domain-external-deps rule is an allowlist match on `^github.com/GiulioSavini/gruntled/internal/domain/`, not a blacklist of specific libraries, so it also catches future internal-layer leaks a blacklist wouldn't name
- [Phase 1]: ARCH-01 is enforced by `scripts/check-architecture.sh` (compile gate, non-vacuous guard, domain-direct-io, domain-external-deps, binary-links-testsupport) plus a 7-case self-test proving each rule can genuinely fail; wired into a 2-job GitHub Actions workflow (`check`, `architecture`), both green on master
- [Phase 01]: VALID-01: synthrepo Render/Generate are stdlib-only, math/rand/v2 PCG with a documented fixed draw order; Manifest.Expected proven exact by an independent regexp-based oracle scan in generate_test.go
- [Phase 02 P1]: Dependency.Target() and Module.Surface() both changed from single-value to (value, bool) returns so every caller must handle the unresolved/unknown case explicitly, rather than adding separate IsResolved()/IsKnown() predicates
- [Phase 02 P1]: UnitStatus split into StatusResolved / StatusModuleUnknown / StatusConfigUnknown — a module-unknown unit keeps its deps/refs (GRT001 checks the target unit's module, not the referencing unit's) while a config-unknown unit carries none
- [Phase 02 P1]: diagnostic.Key gained Unit (a shared include's reference is evaluated once per including unit, so two units can resolve the same dependency differently) but deliberately excludes Severity (a severity-only change is not a new finding); both documented and tested
- [Phase 02 P2]: internal/application may import only the domain allowlist plus context (no fmt, even in tests); ports (UnitLoader, SurfaceReader) live in internal/application/ports, the one use case (indexing.Build) in internal/application/indexing, tested entirely with hand-written fakes, no filesystem
- [Phase 02 P2]: indexing.Build assembles every unit from its UnitConfig before reading any module surface, so a DTO the domain constructors reject (e.g. a "resolved" unit with a zero Module) fails at the assemble stage without ever calling ReadSurface
- [Phase 02 P2]: hcl-only-in-infrastructure (no package outside internal/infrastructure may directly import hashicorp/hcl or zclconf/go-cty) landed in scripts/check-architecture.sh before any infrastructure package exists, so Plan 03's first commit is already policed
- [Phase 02 P2]: check-architecture.sh's non-vacuous guards now run before its compile gate — internal/application imports internal/domain, so emptying internal/domain would otherwise trip compile-gate instead of non-vacuous-guard
- [Phase 02 P3]: hclconv.Position recomputes every hcl position's column from hcl.Pos.Byte, never hcl.Pos.Column (grapheme clusters); the non-ASCII test asserts byte column 22 for a "ééé"-prefixed line against hcl's own grapheme column 19, independently verified outside Go before trusting the test
- [Phase 02 P3]: sourceresolve.Classify hand-rolls Terragrunt's detector chain (not go-getter/v2): the FileDetector catch-all makes any unmatched string local, so a bare `units/chicken` is local like Terragrunt, unlike plain Terraform; misclassification fails safe through the surface reader's existence check either direction
- [Phase 02 P3]: get_terragrunt_dir/get_original_terragrunt_dir take Params: nil (no VarParam), so hcl/cty itself rejects any argument before Impl ever runs — no manual arity check needed
- [Phase 02 P3]: tfsurface.Reader always calls fs.Stat on every kept file (not just symlinks), verified against a real os.Root-backed FS to be exactly where an escaping or dangling symlink surfaces as an error, so one code path handles directories, in-repo symlinks and escapes/dangling links uniformly
- [Phase 02 P4]: mock_outputs_merge_strategy_with_state recognizes exactly three literal values (no_merge/shallow/deep_map_only); any other literal string is Unknown rather than guessed true, following the plan's behavior table over the Phase 1 doc comment's simplified wording
- [Phase 02 P4]: discoverUnits relies on fs.WalkDir never recursing into a non-directory DirEntry; the fs.ModeSymlink guard exists only to keep a symlinked terragrunt.hcl from matching the unit-file name switch, not to prevent descent
- [Phase 02 P4]: extractRefs and dependencyOptions never evaluate an expression to decide reference/fact status; they inspect the raw hcl.Traversal/*hclsyntax.ObjectConsExpr/*hclsyntax.TupleConsExpr AST shape so an uncertain construct fails to unknown by construction
- [Phase 02 P5]: resolveUnit's 12-step fixed check order is the single source of truth for reason precedence (config validity -> include resolution -> dependency merge -> references -> source/generate/overlay); every path-bearing attribute always resolves against the CHILD unit dir regardless of which file (child or a merged include) it is written in
- [Phase 02 P5]: SRC-08's bare registry-looking source (no scheme, e.g. terraform-aws-modules/vpc/aws) classifies Local per research Pitfall 8, not Remote as 02-TERRAGRUNT-EDGECASES.md's older entry says; it resolves to a nonexistent local path that tfsurface later reports module-dir-not-found
- [Phase 02 P5]: deep-merge dependency resolution only activates when a label's occurrences span more than one file AND at least one is itself a deep include; a single-occurrence label always keeps its literal facts, never guessed-merged
- [Phase 02 P5]: TestUnknownReasons parses reasons.go with go/parser and fails if any Reason* constant lacks a fixture row, so the unknown-reason catalogue and its test coverage can never silently drift apart
- [Phase 02 P5]: the real primary corpus has 3 units (not 0) that are deliberately config-unknown: two same-label `dependency "iam"` blocks in one file, a real shape this project's "never guess which duplicate wins" policy (research Pattern 6) refuses to resolve -- verified once locally against a fresh clone, documented in 02-05-SUMMARY.md, not a defect
- [Phase 03 P1]: GRT001 always SeverityError; mock_outputs only appends the masking suffix when mock keys, allowed commands and merge/zero-output facts are certain literals
- [Phase 03 P1]: checking.Check returns *indexing.Error unchanged (exit 3 in the CLI); analyzer failures are *checking.Error{Stage: analyze}
- [Phase 03 P1]: mock_outputs_merge_strategy_with_state, when present, decides MockMergeWithState alone (Terragrunt getMockOutputsMergeStrategy), reversing the Phase 2 either-true rule
- [Phase 03 P3]: cmd/gruntled run(args, stdout, stderr) int is the only entry point; tests call it in-process and through testscript, where a gruntled-exit helper asserts the exact numeric exit code
- [Phase 03 P3]: text mode with no diagnostics performs no stdout write; JSON always prints one document and no stderr summary
- [Phase 03 P5]: CLI e2e tests run in-process over synthrepo trees and compare decoded JSON, importing no application internals
- [Phase 04 P3]: VALID-06 PASS in 3 of 3 process-vs-process runs (terragrunt/gruntled median 2.81, 2.13, 1.99) on a loaded WSL2 machine (4 CPUs); all runs reported, none discarded
- [Phase 04 P3]: docs/validation.md is the single record of VALID-02..06; TestValidationDocPins keeps its pins and denis256 positions in step with the tests
- [Phase 04 P3]: denis256 recall 0/8 caused by the 02-13 include-target rule is documented as a known limitation and v2 refinement candidate
- [Post-v1 fix 313f856]: an unevaluable include marks no include-free units when Terragrunt 1.1.6 rejects its path (variables other than `values` outside try/can) or when it is a `find_in_parent_folders` with no argument or a bare file name that finds nothing in the repo; this supersedes the 02-13 behavior for `local.*` include paths and resolves the denis256 recall limitation (8/8)

### Pending Todos

None yet.

### Blockers/Concerns

- Phase 4: no naturally occurring wiring bug exists in any maintained corpus surveyed — VALID-04/05 depend on a deliberately injected, single-line mutation (rename/delete an output), documented as such, not an organic bug

## Session Continuity

Last session: 2026-09-29T11:20:00.000Z
Stopped at: Completed 04-03-PLAN.md
Resume file: None
