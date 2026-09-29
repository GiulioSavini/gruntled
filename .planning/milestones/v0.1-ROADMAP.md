# Roadmap: gruntled

## Overview

v0.1 is a falsifiable experiment, not a feature list: prove that a warm-index static
analyzer can catch a real class of Terragrunt wiring bug (`dependency.X.outputs.Y`
referencing an output that doesn't exist) with zero false positives, faster than
`terragrunt hcl validate`, on a real public repository. Everything before the final
phase exists to make that one real-world run trustworthy.

The build proceeds bottom-up through the hexagonal architecture: pure domain types
and a synthetic-repository generator first (so every later phase has real fixtures
to test against and the domain stays provably HCL-free), then the infrastructure
adapters that turn a Terragrunt repository on disk into a `RepositoryGraph`, then
the one diagnostic (`GRT001`) and the `check` CLI that make the graph useful, and
finally the experiment itself — golden tests at scale, the unmutated corpus, an
injected mutation, and a benchmark against `terragrunt hcl validate`. If the final
phase fails, the idea is wrong and that is learned in one milestone.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [x] **Phase 1: Domain Foundation & Test Substrate** - Pure domain types (HCL-free, CI-enforced) plus a deterministic synthetic Terragrunt repo generator that every later phase uses for fixtures (completed 2026-09-25)
- [x] **Phase 2: Parsing & Graph Construction** - Walk a real Terragrunt repository, resolve units to modules, and build a correctly-resolved `RepositoryGraph` without evaluating any expression value (14/14 plans, verified passed) (completed 2026-09-29)
- [x] **Phase 3: GRT001 Diagnostic & CLI** - `gruntled check` runs end-to-end, reporting correct, deterministic diagnostics with documented exit codes and no side effects (completed 2026-09-29)
- [x] **Phase 4: Real-Repo Validation Experiment** - The falsifiable claim is settled: zero false positives and every injected mutation caught on a real public corpus, faster than `terragrunt hcl validate` (completed 2026-09-29)

## Phase Details

### Phase 1: Domain Foundation & Test Substrate
**Goal**: The domain layer (`Unit`, `Module`, `Surface`, `Reference`, `RepositoryGraph`, `Diagnostic`) exists, is provably free of HCL/filesystem imports, and a deterministic synthetic-repository generator exists so every subsequent phase has real fixtures instead of ad hoc HCL snippets.
**Depends on**: Nothing (first phase)
**Requirements**: ARCH-01, ARCH-02, VALID-01
**Success Criteria** (what must be TRUE):
  1. The domain package's dependency list contains no HCL, filesystem, or infrastructure import, enforced by an automated CI check that fails the build if one appears
  2. Analyzers and graph queries can be exercised in unit tests using hand-built domain objects only — no filesystem access, no HCL parsing
  3. Given a `Spec` (unit count, include nesting depth, dependency fanout, seed), the generator produces a Terragrunt repository tree deterministically — the same `Spec` and seed produce a byte-identical tree on repeated runs
  4. The generator can inject at least one known error kind (e.g. a bad output reference) and return a manifest describing the diagnostics a correct analyzer should report against the generated tree
**Plans**: 3 plans

Plans:
- [x] 01-01-PLAN.md — Go module bootstrap + pure domain: repograph (value objects, RepositoryGraph aggregate, pure queries) and diagnostic (Diagnostic, Set, Diff)
- [x] 01-02-PLAN.md — ARCH-01 enforcement: architecture check script + self-test proving each rule fails + 2-job CI on master
- [x] 01-03-PLAN.md — synthrepo: deterministic Render/Generate with pinned digest and BadOutputRef injection manifest (exact oracle)

### Phase 2: Parsing & Graph Construction
**Goal**: gruntled walks a real Terragrunt repository on disk and builds a complete, correctly-resolved `RepositoryGraph` — every unit, the module it resolves to, and that module's public surface — using only structural HCL decoding, never evaluating an expression to a value.
**Depends on**: Phase 1
**Requirements**: PARSE-01, PARSE-02, PARSE-03, PARSE-04, PARSE-05, PARSE-06, GRAPH-01, GRAPH-02, GRAPH-03, GRAPH-04, GRAPH-05
**Success Criteria** (what must be TRUE):
  1. gruntled reads only `include`, `terraform.source` and `dependency` blocks structurally, and correctly evaluates all six pure path functions (`find_in_parent_folders`, `path_relative_to_include`, `path_relative_from_include`, `get_terragrunt_dir`, `get_parent_terragrunt_dir`, `get_original_terragrunt_dir`) to resolve which `include` file applies to a given unit
  2. Given a directory tree with nested `include` files, each `include` file is parsed exactly once and every unit that includes it shares the same parsed result
  3. A unit resolves to its module correctly both when `terraform.source` is present (local path) and when it is absent (the unit's own directory); a unit whose source is remote or dynamically computed is classified as remote/unresolvable without any network access
  4. Given a `dependency` block, gruntled resolves it through both hops — to the target unit, then from that unit to its module — and extracts that module's `variable` and `output` names correctly even when the unit's directory differs from the module's directory
  5. A unit hitting any construct it cannot resolve offline (remote source, unquoted/dynamic constructs it can't evaluate) is marked `unknown` rather than analyzed further; invalid HCL (including a file saved mid-edit) produces a diagnostic instead of a crash; `.terragrunt-cache`, `.terraform`, vendored module directories and symlinks are never walked into
**Plans**: 14 plans (02-01..02-05 in sequential waves 1-5; gap closure 02-06..02-11 in two waves: 02-06..02-10 parallel, then 02-11; gap cycle 1 02-12..02-14 in two waves: 02-12 and 02-14 parallel, then 02-13)

Plans:
- [x] 02-01-PLAN.md — Domain: config-unknown / module-unknown units, unknown-surface modules, unresolved dependencies, DependencyOptions (DIAG-03 facts), diagnostic Key with Unit
- [x] 02-02-PLAN.md — Application: UnitLoader/SurfaceReader ports, indexing.Build over fakes, arch rules (application allowlist/deps/platform/guard, hcl-only-in-infrastructure) with self-tests
- [x] 02-03-PLAN.md — hcl/v2 + leaf adapters: byte-column positions and GRT100, offline source classifier, six path functions with closed evaluation, module surface reader
- [x] 02-04-PLAN.md — Terragrunt structure: unit discovery walk (skip rules, symlinks), whole-body reference extraction, parse-once cache with dependency facts
- [x] 02-05-PLAN.md — Terragrunt loader: include merge, path evaluation, source classification, unknown catalogue; end-to-end integration (two-hop, parse-once, determinism, synthrepo oracle, corpus smoke) and fuzz

Gap closure (from 02-REVIEW.md G1..G14; wave 1 = 02-06..02-10 in parallel with disjoint files, wave 2 = 02-11):
- [x] 02-06-PLAN.md — G1: lazy-evaluation guard (ternary branches, &&/||, for body) in reference extraction; OUT-09/OUT-10 reversal
- [x] 02-07-PLAN.md — G2-G6, G8, G9, G11: stack targets, include-target units, non-default/invalid config_path, JSON includes, overlay ReadDir failure, malformed generate blocks
- [x] 02-08-PLAN.md — G12: zero values invalid everywhere in repograph (positions, options, entries, graph units/modules)
- [x] 02-09-PLAN.md — G13, G14: internal-layout, infrastructure-importers, testsupport-only-in-tests rules, and a source-level HCL import scan for build-constrained files
- [x] 02-10-PLAN.md — G7a: hclconv size cap and nesting-depth pre-scan; tfsurface refuses oversize/overdeep module files
- [x] 02-11-PLAN.md — G7b, G10: unit/include size and depth limits, two-input loader fuzz, edge-case catalogue update (STACK-09 and new reasons)

Gap cycle 1 (from 02-REVIEW.md Round 2, G15..G22, and 02-VERIFICATION.md; wave 1 = 02-12 and 02-14 in parallel with disjoint files, wave 2 = 02-13, which shares loader_test.go with 02-12):
- [x] 02-12-PLAN.md — G17, G18, G20: ternary- and chain-aware nesting pre-scan, non-regular files never block, .tf/.tofu union surface
- [x] 02-13-PLAN.md — G15, G16, G19: include targets by canonical path, parents of failing/dynamic includers marked include-target, conservative generate output detector, catalogue for G15..G22
- [x] 02-14-PLAN.md — G21, G22: go/parser import scanner replaces the awk scan, single-module rule (requires 03-02 merged)

### Phase 3: GRT001 Diagnostic & CLI
**Goal**: `gruntled check` runs end-to-end against a repository on disk and reports `GRT001`/`GRT100` diagnostics that are correct, deterministic, and safe to script against in CI.
**Depends on**: Phase 2
**Requirements**: DIAG-01, DIAG-02, DIAG-03, DIAG-04, CLI-01, CLI-02, CLI-03, CLI-04, CLI-05
**Success Criteria** (what must be TRUE):
  1. Running `gruntled check` on a repository with a genuine `dependency.X.outputs.Y` mismatch reports `GRT001`, and a file with invalid HCL reports `GRT100` — both with a repository-relative path, line, column and stable code, never an absolute path
  2. The `mock_outputs` interaction rule is explicit, documented and covered by tests (per DIAG-03): whether a mock covering a missing output suppresses `GRT001`, downgrades it, or leaves it unchanged is decided deliberately in this phase — the corpus pattern (`mock_outputs_merge_with_state = true` with `apply` among allowed commands, where a missing output genuinely works at runtime) must be among the tested cases, and the absence of mocks never manufactures a false diagnostic
  3. Running `gruntled check` twice on the same unmodified repository — including from two differently-named checkout paths — produces byte-identical stdout, in identical order, with the same exit code
  4. `gruntled check` exits 0 on a clean repository and non-zero when it reports an error, per a documented exit-code table
  5. `gruntled check` makes no network calls, starts no external processes, and writes nothing inside the repository it analyzes, verified by a test
**Plans**: 5 plans in 3 waves (wave 1 runs 03-01, 03-02 and 03-04 in parallel with disjoint files; 03-03 alone touches go.mod; assumes the Phase 2 gap-closure plans for lazy evaluation, stack targets and include-target units have landed)

Plans:
- [x] 03-01-PLAN.md — Pure GRT001 analyzer with the DIAG-03 decision table, checking.Check use case, merge-strategy precedence fix
- [x] 03-02-PLAN.md — Text/JSON/summary presenters in internal/interfaces, and the interfaces-external-deps and binary-no-net-no-exec arch rules with self-tests
- [x] 03-03-PLAN.md — `gruntled check` composition root on stdlib flag (exit codes 0/1/2/3, flags after path), and testscript end-to-end runs for GRT001/GRT100/DIAG-03
- [x] 03-04-PLAN.md — docs/cli.md (usage, exit codes, JSON schema, DIAG-03 rule and rejected alternatives), and PROJECT.md Key Decisions and stack line
- [x] 03-05-PLAN.md — Synthrepo oracle, determinism across checkouts, no-writes, mutation diff, and help/docs sync tests; README Status/usage; corpus gate

### Phase 4: Real-Repo Validation Experiment
**Goal**: The milestone's falsifiable claim is settled. gruntled is proven correct, safe, and faster than the existing alternative on a real public Terragrunt repository — or the project stops here, having learned that in one milestone.
**Depends on**: Phase 3
**Requirements**: VALID-02, VALID-03, VALID-04, VALID-05, VALID-06
**Success Criteria** (what must be TRUE):
  1. Golden tests pass over fixture repositories — both hand-written and generated at full scale via the Phase 1 generator — asserting the exact expected diagnostic set for each
  2. `gruntled check` reports zero diagnostics on the unmutated primary corpus (`aws-solutions-library-samples/guidance-for-iso20022-messaging-workflows-on-aws`)
  3. After a deliberate output rename or deletion is injected into the corpus, `gruntled check` reports every reference broken by it
  4. `terragrunt hcl validate` is run against the same mutated corpus and confirmed not to report the injected breakage, evidencing the gap `GRT001` closes
  5. A reproducible benchmark shows `gruntled check` faster than `terragrunt hcl validate` on the same repository
**Plans**: 4 plans in 2 waves (wave 1 runs 04-01, 04-02 and 04-04 in parallel with disjoint files; 04-03 needs all three; assumes Phase 3 and the Phase 2 gap-closure plans are executed; corpus and terragrunt tests are env-gated and never in CI)

Plans:
- [x] 04-01-PLAN.md — VALID-02 golden tests: 10 hand-written txtar fixture repos with hand-computed, self-checked exact diagnostic sets, plus full-scale synthrepo trees (400-600 units) against Manifest.Expected
- [x] 04-02-PLAN.md — VALID-03/04/05 on the pinned primary corpus: zero diagnostics unmutated, exact textual-oracle match for a rename (8 refs) and a deletion (3 refs) on scratch copies, and plain `terragrunt hcl validate` (hash-pinned v1.1.6) confirmed not to report either
- [x] 04-04-PLAN.md — Secondary corpus denis256/terragrunt-tests (pinned 726485e6, env-gated): 8 hand-derived hits kept as self-checked corpus facts; amended after 02-13 to an EMPTY GRT001 set with each hit explained by an include-target unknown unit (0/8 recall, 0 false positives), the enabled=false reference silent, no panic, deterministic output
- [x] 04-03-PLAN.md — VALID-06 process-vs-process benchmark (warm-up, 21 interleaved samples, medians), a full experiment run, and the committed docs/validation.md results record with a doc-pin drift guard

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Domain Foundation & Test Substrate | 3/3 | Complete    | 2026-09-25 |
| 2. Parsing & Graph Construction | 11/14 | Complete    | 2026-09-29 |
| 3. GRT001 Diagnostic & CLI | 5/5 | Complete    | 2026-09-29 |
| 4. Real-Repo Validation Experiment | 4/4 | Complete    | 2026-09-29 |

---
*Roadmap created: 2026-09-01*
