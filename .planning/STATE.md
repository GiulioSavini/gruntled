---
gsd_state_version: 1.0
milestone: v0.2
milestone_name: CI-Ready
status: executing
stopped_at: Completed 07-04-PLAN.md
last_updated: "2026-10-01T12:48:22.363Z"
last_activity: 2026-10-01 — completed 07-04 (pre-commit hook, docs/ci.md recipes, recipe-check CI job + TestCIDoc)
progress:
  total_phases: 3
  completed_phases: 2
  total_plans: 17
  completed_plans: 16
---

# Project State

## Project Reference

See: .planning/PROJECT.md (updated 2026-09-29)

**Core value:** Tell the user, before they run anything slow, that `dependency.X.outputs.Y` does not exist in the module it points to.
**Current focus:** v0.2 CI-Ready — Phase 7 (Distribution & CI Integration) executing, 07-04 complete

## Current Position

Phase: 7 of 7 (Distribution & CI Integration) — v0.2 covers phases 5-7
Plan: 4 of 5 complete (07-01, 07-02, 07-03, 07-04 done)
Status: Executing Phase 7
Last activity: 2026-10-01 — completed 07-04 (pre-commit hook, docs/ci.md recipes, recipe-check CI job + TestCIDoc)

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
| Phase 05 P01 | interrupted | 3 tasks | 14 files |
| Phase 05 P03 | 2 | 2 tasks | 4 files |
| Phase 05 P02 | 5 | 3 tasks | 7 files |
| Phase 05 P04 | 6min | 3 tasks | 10 files |
| Phase 05 P05 | 15min | 2 tasks | 9 files |
| Phase 05 P06 | 35min | 3 tasks | 5 files |
| Phase 06 P01 | 6min | 2 tasks | 4 files |
| Phase 06 P02 | 10min | 2 tasks | 3 files |
| Phase 06 P03 | 9min | 2 tasks | 8 files |
| Phase 06 P04 | 10min | 2 tasks | 4 files |
| Phase 06 P05 | 15min | 3 tasks | 7 files |
| Phase 07 P01 | 12min | 2 tasks | 8 files |
| Phase 07 P02 | 19min | 3 tasks | 9 files |
| Phase 07 P03 | 5min | 1 tasks | 1 files |
| Phase 07 P04 | 8min | 3 tasks | 5 files |

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
- [Phase 05]: Dependency carries config_path value pos + TargetState; path deps kept apart from Dependency; loader passes TargetUnknown until 05-03
- [Phase 05 P03]: classifyTarget: only fs.ErrNotExist is DirMissing, any other fs error Unknown; resolveTargetExpr (shared with 05-04 paths) returns RepoPath; nonexistent .../terragrunt.hcl maps to parent dir
- [Phase 05 P02]: GRT003 self-loop renders as one-member ring; multi-member anchor position ignores the anchor's self-edge; all analyzers in Check share Stage "analyze"
- [Phase 05 P02]: dependency_edges golden is red (new GRT002) until 05-05 hand review
- [Phase 05 P04]: dependencies paths shape from raw AST; non-list literal drops path edges, dynamic paths = one unresolved entry; union merge dedup by target child-first
- [Phase 05 P04]: E2E paths test lives in infrastructure/terragrunt (arch rule forbids application tests importing infrastructure)
- [Phase 05 P05]: dependency_edges GRT002 (../nodir, no terragrunt.hcl) reviewed correct, golden updated by hand; goldens gain optional _golden/messages
- [Phase 05 P05]: block config_path = "" is a GRT003 self-loop (matches Terragrunt filepath.Join) — SUPERSEDED by 05-07; check -h exit line names GRT001-GRT003
- [Phase 05]: 05-06: terragrunt oracle = queue-construction message of run --all --no-auto-init -- version on scratch copies (iso20022, secret, denis256 issue-2565 subtree); textual oracle graphOracle on all three
- [Phase 05]: 05-06 open issue (config_path = "" GRT003 vs terragrunt 'config_path could not be resolved') RESOLVED by 05-07, see docs/validation.md "Resolved: config_path = """
- [Phase 05 P07]: empty config_path matches terragrunt v1.1.6 per case: dependency block "" unresolved config-path-empty, silent; dependencies paths entry "" resolves to the unit itself, GRT003 self-loop (terragrunt: "cycle detected during queue construction"); reverses the 05-04 rule that a paths "" is never a self-edge
- [Phase 06]: 06-01: Edge carries name/target state/skip_outputs (paths edge: no name, skip false); presenter.Graph builds edges from Edges() only, unresolved block (PathPos) + paths entries merged per unit by position
- [Phase 06]: SARIF: unit notifications point at <unit>/terragrunt.hcl, module notifications have no location; unknown rule code -> error, nothing written
- [Phase 06]: graph builds via indexing.Build without analyzers; exit codes 0/2/3 only, help shows 3 exit-code lines
- [Phase 06]: check and graph share parseArgs/openRepo/writeOut in cmd/gruntled
- [Phase 06]: SARIF doc-sync test in cmd/gruntled/sarif_test.go: docs/cli.md GRT headings must equal emitted rule shortDescriptions
- [Phase 06]: 06-05: repo made public for code scanning; checkout_path does NOT rewrite SARIF URIs, CI jq-prefixes artifactLocation.uri with fixture path
- [Phase 06]: 06-05: SARIF URIs relative to analysed dir; subdirectory analysis needs URI prefix or run from repo root; Phase 7 CI recipes must handle this
- [Phase 07]: 07-01: --version prints 'gruntled <version> (<commit>)' from -X main.version/main.commit (defaults dev/none); ReadBuildInfo fallback dropped (local go build stamps pseudo-version), so go install @tag prints dev (none)
- [Phase 07]: 07-01: cmd/gruntled/testdata/clean-fixture is the exit-0 fixture; TestFixtureExitCodes pins clean=0, sarif-fixture=1
- [Phase 07]: 07-02: USER DECISION 2026-10-01 windows/arm64 is the 6th release target; release list lives only in check-architecture.sh Step 9 and build-release.sh, kept equal by TestReleaseTargetsInSync
- [Phase 07]: 07-02: scripts/build-release.sh is the single packaging path (gruntled_<version>_<os>_<arch>.tar.gz|zip + checksums.txt, host --version smoke); ci.yml release-build runs it on every push/PR with v0.0.0-ci
- [Phase 07]: 07-03: release.yml re-runs check-architecture.sh + go test on the tagged commit (needs: cannot span workflows); guard = vX.Y.Z[-suffix] regex + tag on master; single gh release create at end, --prerelease for suffixed tags
- [Phase 07]: docs/ci.md: go install @tag prints gruntled dev (none); only release binaries carry the tag version
- [Phase 07]: recipe-check job proves docs/ci.md recipes (clean 0, broken exactly 1, pre-commit try-repo); TestCIDoc guards drift

### Pending Todos

None yet.

### Blockers/Concerns

- Phase 4: no naturally occurring wiring bug exists in any maintained corpus surveyed — VALID-04/05 depend on a deliberately injected, single-line mutation (rename/delete an output), documented as such, not an organic bug

## Session Continuity

Last session: 2026-10-01T12:48:22.360Z
Stopped at: Completed 07-04-PLAN.md
Resume file: None
