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
- [x] **Phase 2: Parsing & Graph Construction** - Walk a real Terragrunt repository, resolve units to modules, and build a correctly-resolved `RepositoryGraph` without evaluating any expression value (completed 2026-09-25)
- [ ] **Phase 3: GRT001 Diagnostic & CLI** - `gruntled check` runs end-to-end, reporting correct, deterministic diagnostics with documented exit codes and no side effects
- [ ] **Phase 4: Real-Repo Validation Experiment** - The falsifiable claim is settled: zero false positives and every injected mutation caught on a real public corpus, faster than `terragrunt hcl validate`

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
**Plans**: 5 plans (sequential waves 1-5: each wave needs the previous one's API, arch rules must land before any infrastructure package, and only 02-03 touches go.mod)

Plans:
- [x] 02-01-PLAN.md — Domain: config-unknown / module-unknown units, unknown-surface modules, unresolved dependencies, DependencyOptions (DIAG-03 facts), diagnostic Key with Unit
- [x] 02-02-PLAN.md — Application: UnitLoader/SurfaceReader ports, indexing.Build over fakes, arch rules (application allowlist/deps/platform/guard, hcl-only-in-infrastructure) with self-tests
- [x] 02-03-PLAN.md — hcl/v2 + leaf adapters: byte-column positions and GRT100, offline source classifier, six path functions with closed evaluation, module surface reader
- [x] 02-04-PLAN.md — Terragrunt structure: unit discovery walk (skip rules, symlinks), whole-body reference extraction, parse-once cache with dependency facts
- [x] 02-05-PLAN.md — Terragrunt loader: include merge, path evaluation, source classification, unknown catalogue; end-to-end integration (two-hop, parse-once, determinism, synthrepo oracle, corpus smoke) and fuzz

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
**Plans**: TBD

Plans:
- [ ] 03-01: TBD

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
**Plans**: TBD

Plans:
- [ ] 04-01: TBD

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. Domain Foundation & Test Substrate | 3/3 | Complete    | 2026-09-25 |
| 2. Parsing & Graph Construction | 4/5 | In Progress|  |
| 3. GRT001 Diagnostic & CLI | 0/TBD | Not started | - |
| 4. Real-Repo Validation Experiment | 0/TBD | Not started | - |

---
*Roadmap created: 2026-09-01*
