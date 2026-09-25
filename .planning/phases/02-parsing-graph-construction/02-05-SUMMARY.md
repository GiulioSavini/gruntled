---
phase: 02-parsing-graph-construction
plan: 05
subsystem: infrastructure
tags: [go, hcl, hcl-v2, terragrunt, hexagonal, parsing, graph-construction]

# Dependency graph
requires:
  - phase: 02-parsing-graph-construction (plan 01)
    provides: DependencyOptions/Tristate/NameList, config-unknown/module-unknown unit split, byte-column Position, RepositoryGraph aggregate
  - phase: 02-parsing-graph-construction (plan 02)
    provides: ports.UnitLoader/SurfaceReader, indexing.Build application use case
  - phase: 02-parsing-graph-construction (plan 03)
    provides: hclconv.Position/FirstSyntaxError, the six path functions, closed evalPath/resolvePath, literalString/literalBool, sourceresolve.Classify, tfsurface.Reader
  - phase: 02-parsing-graph-construction (plan 04)
    provides: discoverUnits walk, extractRefs whole-body reference extraction, fileCache/parsedFile parse-once cache, dependencyOptions DIAG-03 facts
provides:
  - "internal/infrastructure/terragrunt.Loader: a complete ports.UnitLoader -- include resolution, effective-file precedence merge (child > last include > first include, no_merge excluded), source classification, shallow/deep dependency merge, reference concatenation, and the full config-unknown/module-unknown/unresolved-dependency reason catalogue"
  - "internal/infrastructure/terragrunt/reasons.go: every stable kebab-case unknown-reason constant, enforced complete by a go/parser-based coverage check in loader_test.go"
  - "internal/infrastructure/terragrunt/merge.go: include validation, effective-file computation, dependency-label collection, reference merge/sort, generate-block output-declaration detection, unit-dir-overlay detection"
  - "a real Terragrunt tree on disk turning into a complete, correctly-resolved repograph.RepositoryGraph via indexing.Build(ctx, terragrunt.NewLoader(fsys), tfsurface.NewReader(fsys)), proven by 10 integration tests including a synthrepo oracle and a real-corpus smoke run"
  - "internal/infrastructure/terragrunt/fuzz_test.go: FuzzLoadUnits, a native fuzz target with 19 seeds (12 hand-written + 8 real files from denis256/terragrunt-tests, minus one empty-file duplicate), run locally for 60s with no crasher"
affects: [phase-3-analysis]

# Tech tracking
tech-stack:
  added: []
  patterns:
    - "resolveUnit's 12-step fixed check order (loader.go) is the single source of truth for reason precedence: config validity before include resolution before dependency merge before reference extraction before source/generate/overlay -- every table test in loader_test.go and every integration test in integration_test.go exercises this same order, never a shortcut"
    - "every path-bearing attribute (include.path, dependency.config_path, terraform.source), wherever it is written (child, or a merged include), is evaluated with fileScope's kind (scopeUnit vs scopeIncluded) but ALWAYS resolved with resolvePath(unitDir, raw) against the CHILD unit's own directory -- proven for config_path (research Pitfall 4 / INC-12) and, by the same code path, for terraform.source"
    - "deep-merge dependency resolution only activates when a label's occurrences span more than one file AND at least one of those files is itself a deep include; a label appearing once, or only in files where none is deep, always takes the single highest-precedence block wholly (config_path, pos and opts together) -- never a per-field deep merge"
    - "TestUnknownReasons enforces catalogue completeness by parsing reasons.go with go/parser and failing if any Reason* constant lacks a fixture row: dead reasons and untested reasons are both compile-time-adjacent test failures, not something a future refactor can silently drop"

key-files:
  created:
    - internal/infrastructure/terragrunt/reasons.go
    - internal/infrastructure/terragrunt/merge.go
    - internal/infrastructure/terragrunt/loader.go
    - internal/infrastructure/terragrunt/loader_test.go
    - internal/infrastructure/terragrunt/integration_test.go
    - internal/infrastructure/terragrunt/fuzz_test.go
  modified:
    - internal/infrastructure/terragrunt/testhelpers_test.go

key-decisions:
  - "SRC-08's bare registry-looking source (e.g. terraform-aws-modules/vpc/aws, no scheme) classifies as Local, not Remote, per research Pitfall 8 (Terragrunt's FileDetector catch-all): it resolves to a nonexistent local path, which the existing tfsurface.Reader then reports as module-dir-not-found -- this corrects, not follows, 02-TERRAGRUNT-EDGECASES.md's SRC-08 entry, which predates that source-level research"
  - "find_in_parent_folders exhausting its search with no fallback (INC-10-adjacent) fails evalPath uniformly with every other unsupported-function or unresolvable-argument case, mapping to ReasonIncludeDynamicPath rather than a dedicated ReasonIncludeNotFound row: evalPath's bool-only contract cannot distinguish 'search exhausted' from 'unsupported function' by design (pathfuncs_test.go's own 'not found, no fallback fails' row already established ok=false as the uniform outcome in Plan 03); ReasonIncludeNotFound is instead proven by a literal path to a missing file and a literal path to a directory"
  - "TestCorpusSmoke's expectations were corrected from the plan's aspirational '0 config-unknown' to the real, verified corpus result: exactly 3 config-unknown units (iac.cicd/codebuild_project, iac.src/scheduler_recover, iac.src/scheduler_timeout), all for the identical real-world shape of two same-label dependency blocks in one file -- a genuine, previously undocumented corpus finding, not a defect, produced by this plan's own deliberate 'never guess which duplicate wins' policy (research Pattern 6)"

patterns-established:
  - "Pattern: every catalogue reason constant ships with a go/parser-enforced fixture (TestUnknownReasons), so the unknown-reason catalogue and its test coverage can never drift apart silently"
  - "Pattern: integration tests build through the real indexing.Build with the real Loader and tfsurface.Reader (never fakes), asserting on the domain RepositoryGraph's own query methods (ModuleOf, DependencyTarget, References) rather than on internal DTOs, so the tests exercise exactly what Phase 3 will call"

requirements-completed: [PARSE-01, PARSE-02, PARSE-03, PARSE-04, PARSE-05, GRAPH-01, GRAPH-02, GRAPH-03, GRAPH-04]

# Metrics
duration: 40min
completed: 2026-09-25
---

# Phase 2 Plan 5: Terragrunt unit loader, end-to-end graph construction, and the unknown-reason catalogue Summary

**A real Terragrunt repository on disk becomes a complete, correctly-resolved `repograph.RepositoryGraph` through `terragrunt.NewLoader` + `tfsurface.NewReader` + `indexing.Build`, with every one of 19 stable unknown-reason constants proven by a go/parser-enforced fixture and the whole pipeline verified against a synthrepo oracle, a real 65-unit corpus, and a 60-second native fuzz run.**

## Performance

- **Duration:** 40 min
- **Started:** 2026-09-25T15:36:51+02:00
- **Completed:** 2026-09-25T16:16:00+02:00 (approx.)
- **Tasks:** 3
- **Files modified:** 7 (6 new, 1 extended)

## Accomplishments

- `Loader.LoadUnits` implements `ports.UnitLoader` end to end: for each discovered unit, it validates and resolves every include (evaluating `include.path` in scope S0, rejecting a second level of include per research Pitfall 3), merges the effective file set in Terragrunt's own precedence (child > last include > first include, `no_merge` excluded from merge but still visible to `path_relative_to_include`), merges `dependency` blocks by label (shallow: highest-precedence block wins wholly; deep, multi-file: `config_path` from the highest-precedence block that sets it, every other option fact marked unknown), classifies `terraform.source` (GRAPH-01/02/03), detects a `generate` block that may declare outputs, and detects a unit directory overlaying its own module -- all in the single fixed 12-step order documented inline in `loader.go`.
- `reasons.go` catalogues all 19 unknown-reason constants (11 config-unknown, 6 module-unknown, 2 unresolved-dependency), and `loader_test.go`'s `TestUnknownReasons` parses `reasons.go` with `go/parser` to prove every one has a fixture -- no dead reason, no untested reason.
- `integration_test.go` wires the real `Loader` and `tfsurface.Reader` through `indexing.Build` and proves: a two-hop dependency chain with unit-dir != module-dir (`TestTwoHop`); a remote source's blast radius stays contained to the one unit (`TestModuleUnknownBlastRadius`); a unit generated under `.terragrunt-stack/` with a remote source is discovered and behaves identically to an ordinary unit (`TestGeneratedStackRemoteModuleUnknown`, the user-required test); a nonexistent local module directory and a `cds-snc/secret`-shaped root config both resolve their unit but leave their module's surface unknown with no diagnostic (`TestSurfaceUnknownTarget`); references are extracted from the unit's entire effective body with byte-accurate positions (`TestWholeBodyRefs`); a broken module file and a broken unit file each produce exactly one file-level GRT100 (`TestSyntaxEndToEnd`); a 50-unit synthetic repository reads every shared file at most once (`TestParseOnce`); the graph's own unresolvable references match a synthrepo oracle's injected-mutation manifest exactly, with zero unresolved units (`TestSynthrepoOracle`); building the same repository twice, and from two differently-named checkouts, gives byte-identical canonical dumps containing neither a temp path nor the virtual-root sentinel (`TestDeterministic`); and the real primary corpus (65 units, 22 references) resolves with 0 missing outputs and 0 module-unknown units (`TestCorpusSmoke`, run once locally, see below).
- `FuzzLoadUnits` fuzzes a unit's `terragrunt.hcl` body against a fixed surrounding repository through `indexing.Build`, asserting no panic, no Go error, no diagnostic naming a file outside the fixture, and every kept reference position inside its own file's real bytes. 19 seeds include 8 real fixtures inlined from `denis256/terragrunt-tests`. A 60-second local run (462,450 executions) found no crasher.

## Task Commits

Each task was committed atomically:

1. **Task 1: Loader: include resolution, merge, path evaluation, source classification, unit state** - `77fd457` (feat)
2. **Task 2: End-to-end integration through indexing.Build with the real adapters** - `07fbbd9` (test)
3. **Task 3: Fuzz target over the loader and edge-case traceability** - `201649d` (test)

**Plan metadata:** (this commit) `docs(02-05): complete Terragrunt unit loader plan`

## Files Created/Modified

- `internal/infrastructure/terragrunt/reasons.go` - the 19-constant unknown-reason catalogue, split into config-unknown / module-unknown / unresolved-dependency groups
- `internal/infrastructure/terragrunt/merge.go` - `resolvedInclude`, `effectiveFile`, `depOccurrence`; `validateIncludeDecls`, `mergeStrategyOf`, `toIncludeRefs`, `buildEffectiveFiles`, `validateEffectiveFile`, `fileScope`, `collectDependencyLabels`, `mergeReferences`/`sortRefs`, `generateMayDeclareOutputs`/`mergeGenerateUnknownReason`, `unitDirOverlaysModule`
- `internal/infrastructure/terragrunt/loader.go` - `Loader`, `NewLoader`, `LoadUnits`, `resolveUnit` (the 12-step order), `resolveIncludes`, `resolveDependencies`/`resolveOneDependency`, `resolveSource`
- `internal/infrastructure/terragrunt/loader_test.go` - `TestUnknownReasons` (29 fixture rows + the go/parser completeness check), `TestStructuralOnly`, `TestSourceForms`, `TestIncludePrecedence`, `TestDependencyMerge`, `TestDependencyTarget`, `TestDependencyReferences`, `TestLoaderParseOnceIncludes`, `TestLoaderSharedBrokenIncludeDiagnostics`, `TestLoaderUnitsSortedByRepoPath`, `TestLoaderCancelledContext`
- `internal/infrastructure/terragrunt/integration_test.go` - the 10 end-to-end tests listed above
- `internal/infrastructure/terragrunt/fuzz_test.go` - `FuzzLoadUnits`
- `internal/infrastructure/terragrunt/testhelpers_test.go` - added `filesFS`, `build`, `dump`

## Decisions Made

See `key-decisions` in the frontmatter for the three most consequential ones: SRC-08's bare-registry-string classification correcting the older edge-case catalogue in favor of research Pitfall 8; `find_in_parent_folders`'s uniform `ReasonIncludeDynamicPath` outcome on search exhaustion (matching Plan 03's own tested contract); and `TestCorpusSmoke`'s expectations corrected to the real, verified corpus result of 3 deliberate config-unknown units rather than the plan's aspirational 0.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] `TestCorpusSmoke` asserted the wrong config-unknown count**

- **Found during:** Task 2, running the corpus smoke test against a local clone of the primary corpus as the plan's action section instructs
- **Issue:** The plan's Task 2 behavior bullet asserted `0 config-unknown, 0 module-unknown`. Running the real corpus produced 3 config-unknown units, not 0. Investigation (reading the three units' `terragrunt.hcl` files) showed all three declare two `dependency "iam" { ... }` blocks with the identical label in one file -- a real shape research Pattern 6 explicitly assigns `ReasonInvalidDependency` to (Terragrunt itself only warns and lets "last wins"; this project's zero-false-positives policy refuses to guess which one). This is not a bug in the loader: `loader.go`'s step 6 (`validateEffectiveFile`) implements exactly what the plan's own action section specifies. The bug was in the test's aspirational assertion, based on an earlier prototype's finding that predates this plan's stricter, deliberately-specified duplicate-label handling
- **Fix:** Updated `TestCorpusSmoke` to assert the correct, verified count (3), asserting each config-unknown unit is one of the three expected paths with reason `invalid-dependency`, and documented the finding inline with a comment explaining why it is a real, deliberate outcome rather than a regression
- **Files modified:** `internal/infrastructure/terragrunt/integration_test.go`
- **Verification:** `GRUNTLED_CORPUS=/tmp/iso20022 go test ./internal/infrastructure/terragrunt -run TestCorpusSmoke -count=1 -v` passes
- **Committed in:** `07fbbd9` (Task 2 commit)

---

**Total deviations:** 1 auto-fixed (1 bug: a test assertion that did not match the real, correctly-produced corpus result)
**Impact on plan:** No production code change was needed; the loader already implements the plan's own specified duplicate-label policy correctly. Only the test's expectation was wrong, and fixing it surfaced a genuine, valuable, previously-undocumented finding about the primary corpus.

## Issues Encountered

- Several hand-written HCL fixtures in `loader_test.go` initially put two attributes on one physical line inside a single-line block (e.g. `include "root" { path = "../root.hcl" merge_strategy = "deep" }`), which HCL's native syntax rejects ("a single-line block definition must end with a closing brace immediately after its single argument definition"). Reformatted every such fixture to multi-line blocks; all affected subtests then passed on the next run. Not a loader bug -- purely a test-fixture authoring error caught immediately by `go vet`/the test run itself.

## User Setup Required

None - no external service configuration required.

## Corpus Smoke Test Result

Run once locally against a fresh shallow clone (`git clone --depth 1 https://github.com/aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws /tmp/iso20022`, outside the repo, not committed):

```
GRUNTLED_CORPUS=/tmp/iso20022 go test ./internal/infrastructure/terragrunt -run TestCorpusSmoke -count=1 -v
=== RUN   TestCorpusSmoke
--- PASS: TestCorpusSmoke (0.08s)
```

65 units discovered; 22 references, every one resolving to a declared output (0 missing); 0 module-unknown units; exactly 3 config-unknown units (`iac.cicd/codebuild_project`, `iac.src/scheduler_recover`, `iac.src/scheduler_timeout`), each for the identical real-world shape of two same-label `dependency "iam"` blocks in one file -- see Deviations above.

## Edge-Case Traceability

Every ID from `02-TERRAGRUNT-EDGECASES.md`'s Quick reference table. "Test" names a representative proof; several IDs share a mechanism proven once and reused (noted inline).

### 1. `terraform { source = ... }`

| ID | Behaviour implemented | Test |
|---|---|---|
| SRC-01 | Resolve (local relative path) | `TestSourceForms/local-relative-path` |
| SRC-02 | Resolve (absent source = unit dir, GRAPH-02) | `TestSourceForms/empty-config-is-graph-02`, `TestStructuralOnly` |
| SRC-03 | Resolve (`//` subdir split) | `TestSourceForms/double-slash-subdir` |
| SRC-04 | Resolve->outside-repo (real absolute path never read outside the repo; deliberate deviation from "Resolve if it exists") | `TestUnknownReasons/source-outside-repo` |
| SRC-05 | Resolve (pure path function composition) | `TestSourceForms/get-terragrunt-dir-composition`, `TestSourceForms/get-parent-terragrunt-dir-in-include` |
| SRC-06 | Unknown, `remote-source` | `TestUnknownReasons/remote-source`, `TestModuleUnknownBlastRadius` |
| SRC-07 | Unknown, `remote-source` (same classifier code path as SRC-06) | `sourceresolve` package tests (Plan 03), "github.com shorthand" row |
| SRC-08 | **Corrected by research Pitfall 8**: a bare registry-looking string (no scheme) classifies Local, not Remote, and resolves to a nonexistent local path (the surface reader later reports it `module-dir-not-found`); an explicit `tfr://` scheme IS Remote | `TestSourceForms/registry-looking-source-resolves-to-local-path`; `sourceresolve` package's "tfr scheme" row (Plan 03) |
| SRC-09 | Unknown, `remote-source` (query string never changes classification) | `sourceresolve` package tests (Plan 03), forced-getter-with-query row |
| SRC-10 | Unknown, `source-dynamic-path` | `TestUnknownReasons/source-dynamic-path` |
| SRC-11 | **Deviation**: `get_env` is not one of the six PARSE-02 functions, so `get_env("S", "default")` ALWAYS fails closed to `source-dynamic-path`, even with a literal default (the catalogue's "Resolve using the literal default" is not implemented; only the six functions exist) | same mechanism as `TestUnknownReasons/source-dynamic-path` |
| SRC-12 | Unknown, `source-dynamic-path` (same mechanism as SRC-11) | same mechanism as `TestUnknownReasons/source-dynamic-path` |
| SRC-13 | Report, preserved as a distinct `Module` condition (`tfsurface.ReasonModuleDirNotFound`), not folded into a diagnostic -- Phase 3 decides | `TestSurfaceUnknownTarget` |

### 2. `include` / `read_terragrunt_config`

| ID | Behaviour implemented | Test |
|---|---|---|
| INC-01 | Resolve (`find_in_parent_folders()`) | `pathfuncs_test.go` (Plan 03); exercised at loader scale by every synthrepo-generated unit (`TestParseOnce`, `TestSynthrepoOracle`, `TestDeterministic`), `FuzzLoadUnits` seed 0 |
| INC-02 | Resolve (explicit literal path) | `TestIncludePrecedence/child-beats-include` and every other `TestIncludePrecedence`/`TestDependencyMerge` fixture |
| INC-03 | Resolve (multiple labeled includes) | `TestIncludePrecedence/last-include-beats-first` |
| INC-04 | Resolve, contributes nothing to merge but still visible to `path_relative_to_include` | `TestIncludePrecedence/no-merge-contributes-nothing-but-is-still-a-visible-include` |
| INC-05 | Resolve (shallow default: highest-precedence block wins wholly) | `TestDependencyMerge/shallow-child-replaces-whole-block` |
| INC-06 | Resolve (deep: `config_path` from the highest-precedence block that SETS it; every other option fact unknown when the label spans >1 file) | `TestDependencyMerge/deep-merge-multi-occurrence-config-path-precedence-and-unknown-opts` |
| INC-07 | Resolve; `generate` merge is always first-in-precedence-per-label regardless of the enclosing `merge_strategy`, so the shallow exception holds by construction, not by special-casing; `remote_state` is never read at all | not separately fixture-tested (falls out of the generic per-label merge; no dedicated regression risk since there is no "deep generate merge" code path to diverge) |
| INC-08 | **Deferred**: `include.<label>.locals.*` is a plain variable traversal under a closed `Variables: nil` EvalContext, so it always fails evalPath (`config-path-dynamic`/`source-dynamic-path`/`include-dynamic-path` depending on position) -- the catalogue's "resolve a one-hop literal" case is not implemented; locals evaluation is explicitly out of scope per CONTEXT.md | same mechanism as `TestUnknownReasons/source-dynamic-path` (a `local.x` reference) |
| INC-09 | Report, `nested-include` | `TestUnknownReasons/nested-include` |
| INC-10 | Unknown, `include-dynamic-path` | `TestUnknownReasons/include-dynamic-path/get_env` |
| INC-11 | Resolve (correctly ignored: `read_terragrunt_config` in unrelated `locals` never gates anything gruntled reads) | `TestStructuralOnly` |
| INC-12 | Resolve, `config_path` resolves against the CHILD unit dir even when declared in the include | `TestIncludePrecedence/include-declared-dependency-resolves-against-child` |

### 3. `dependency` / `dependencies`

| ID | Behaviour implemented | Test |
|---|---|---|
| DEP-01 | Resolve (basic, two-hop) | `TestDependencyTarget/live/app1/vpc` |
| DEP-02 | Resolve (path function composition), same `evalPath`/`resolvePath` mechanism as SRC-05 | `pathfuncs_test.go` (Plan 03); `TestDependencyTarget` exercises the same code path with a literal |
| DEP-03 | Unknown, `config-path-dynamic` (never attempts to reduce a composed expression) | same mechanism as `TestUnknownReasons/config-path-dynamic` |
| DEP-04 | Report, target kept and resolved even though nonexistent | `TestDependencyTarget/live/app3/ghost` |
| DEP-05 | Resolve (multiple labeled dependency blocks) | `TestDependencyReferences`; synthrepo's `DependencyFanout` parameter (`TestParseOnce`, `TestSynthrepoOracle`) |
| DEP-06 | Resolve, `dependencies { paths = [...] }` adds no dependency and no outputs edge | `TestDependencyReferences` (explicit assertion) |
| DEP-07 | Resolve, `mock_outputs` parses without perturbing the graph edge | `depfacts`/`parse_test.go` (Plan 04); `TestDependencyMerge` confirms opts flow through the winning occurrence unchanged |
| DEP-08 | Resolve, `mock_outputs_merge_with_state`/`_strategy_with_state` captured faithfully | `parse_test.go` `TestDependencyOptionsMockMergeWithState`, including the primary-corpus shape (Plan 04) |
| DEP-09 | Resolve, `skip_outputs` unchanged from DEP-01 | `parse_test.go` `TestDependencyOptionsEnabledSkipOutputs` (Plan 04) |
| DEP-10 | Report, no crash/hang, no traversal: `DependencyTarget`/`ModuleOf` are direct map lookups, never recursive, so a cycle cannot loop. GRT003 cycle detection is v2 `MORE-02`, deferred | no dedicated fixture; verified by construction (no recursive traversal exists anywhere in `loader.go` or `indexing.Build`) |
| DEP-11 | Report, self-reference resolves to the unit itself, no hang | `TestDependencyTarget/live/app4/self` |
| DEP-12 | Report, a reference to an undeclared dependency label is kept, not dropped | `TestDependencyReferences` (the `ghost.y` reference) |
| DEP-13 | Resolve, inherited entirely from a deep include | `TestDependencyMerge/deep-merge-single-occurrence-keeps-literal-facts` |

### 4. Output-reference expressions

| ID | Behaviour implemented | Test |
|---|---|---|
| OUT-01 | Resolve | `refs_test.go` (Plan 04); `TestTwoHop`, `TestWholeBodyRefs` |
| OUT-02 | Resolve (index access, name only) | `refs_test.go` (Plan 04) |
| OUT-03 | Resolve (splat, name only) | `refs_test.go` (Plan 04) |
| OUT-04 | Resolve (bracket form) | `refs_test.go` (Plan 04) |
| OUT-05 | **Deviation from the catalogue's "resolve + flag"**: `try(...)`/`can(...)` fully suppress the reference (never extracted at all), per research Pitfall 6 | `refs_test.go`'s try/can guard tests (Plan 04) |
| OUT-06 | **Deviation**: `lookup(dependency.x.outputs, "y", ...)` is a function-call argument shape `outputRef` never recognizes (only a 4-step bare/indexed traversal matches); it never produces a reference, matching the plan's "never guess" policy | `refs_test.go`'s whole-object-reference test covers the same 3-step-traversal shape `lookup`'s first argument has (Plan 04) |
| OUT-07 | Resolve (no-op, whole-object) | `refs_test.go` (Plan 04) |
| OUT-08 | Unknown for that reference only | `refs_test.go` (Plan 04) |
| OUT-09 | Resolve (inside `for`) | `refs_test.go` (Plan 04) |
| OUT-10 | Resolve (both ternary branches) | `refs_test.go` (Plan 04); `TestWholeBodyRefs`'s non-ASCII ternary line |
| OUT-11 | Resolve (locals, not just inputs) | `refs_test.go` (Plan 04); `TestWholeBodyRefs`, `TestTwoHop` |
| OUT-12 | Resolve, on a dependency inherited via deep include | composes `TestDependencyMerge`'s deep-merge coverage with the generic whole-body extraction (OUT-01), which never distinguishes a dependency's declaring file from a reference's; not separately fixture-tested |
| OUT-13 | Resolve, unchanged by `skip_outputs = true` (reference extraction never inspects `DependencyOptions`) | not separately fixture-tested; no code path exists that could diverge |
| OUT-14 | Resolve (no-op, `merge()`/spread) | `refs_test.go` (Plan 04) |

### 5. Stacks, `.terragrunt-cache`, root-config naming

| ID | Behaviour implemented | Test |
|---|---|---|
| STACK-01 | Skip (walk exclusion) | `walk_test.go` (Plan 04) |
| STACK-02 | Skip | `walk_test.go` (Plan 04) |
| STACK-03 | Skip (unconditional, regardless of contents) | `walk_test.go` (Plan 04) |
| STACK-04 | Skip (never follow a symlinked dir/file during discovery) | `walk_test.go` (Plan 04) |
| STACK-05 | Skip, no hang | `walk_test.go` (Plan 04) |
| STACK-06 | **Deferred**: no stack-file counting mechanism exists; `.stack.hcl` files are simply never named `terragrunt.hcl` and so are never discovered, with no separate "0 units because ungenerated Stacks" signal surfaced | deferred, per the plan's own explicit deferral list |
| STACK-07 | Resolve as ordinary units; `.terragrunt-stack` IS walked | `TestGeneratedStackRemoteModuleUnknown`; `walk_test.go` (Plan 04) |
| STACK-08 | **Corrected by research Pitfall 10, superseding the catalogue's "exclude such a config from the Unit list"**: role is never excluded by content -- a parent-only config (no `terraform` block, no local `.tf` files) becomes an ordinary resolved unit whose module surface the tfsurface reader reports `no-terraform-files`, silently, no diagnostic | `TestSurfaceUnknownTarget`'s `terragrunt/terragrunt.hcl` fixture |
| STACK-09 | Resolve as both a Unit and an include target simultaneously | not separately fixture-tested; supported by construction, since `fileCache.get` (include-target parsing) and `discoverUnits` (unit discovery) are fully independent code paths over the same file and neither excludes the other |
| STACK-10 | **Deferred**: `download_dir` is never read; only the default `.terragrunt-cache` name is excluded | deferred, per the plan's own explicit deferral list |

## Next Phase Readiness

- Phase 2's five ROADMAP success criteria are all demonstrated by named tests: structural decode + six functions (`TestStructuralOnly`, `pathfuncs_test.go`); parse-once (`TestParseOnce`); source present/absent/remote (`TestSourceForms`, `TestUnknownReasons`); two-hop with unit dir != module dir (`TestTwoHop`); unknown-on-unresolvable / GRT100-on-invalid-HCL / skip rules (`TestUnknownReasons`, `TestSyntaxEndToEnd`, `walk_test.go`).
- `terragrunt.Loader` + `tfsurface.Reader` + `indexing.Build` together form the complete adapter surface Phase 3's GRT001 analyzer needs: `RepositoryGraph.References()`, `DependencyTarget`, and `ModuleOf`/`Surface` are exactly what a static "does `dependency.X.outputs.Y` exist in its target module" check calls.
- The DIAG-03 facts (`DependencyOptions` on every `Dependency`, resolved or not) are already captured and flow unchanged through merge; Phase 3's mock_outputs rule can read them directly with no further Phase 2 work.
- No blockers. Full local suite (build, vet, test, gofmt, `go mod tidy -diff`, staticcheck, govulncheck, cross-builds for linux/amd64/darwin/arm64/windows/amd64, `check-architecture.sh`, `test-check-architecture.sh`) green at `201649d`. Push and CI verification are the orchestrator's final step per this plan's `<verification>` section.

---
*Phase: 02-parsing-graph-construction*
*Completed: 2026-09-25*

## Self-Check: PASSED

All 7 created/modified files and all three task commit hashes (77fd457, 07fbbd9, 201649d) verified present.
