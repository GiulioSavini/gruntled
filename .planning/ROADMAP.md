# Roadmap: gruntled

## Milestones

- ✅ **v0.1 Validated Engine** - Phases 1-4 (shipped 2026-09-29) — archived in `milestones/v0.1-ROADMAP.md`
- 🚧 **v0.2 CI-Ready** - Phases 5-7 (in progress)

## Overview

v0.2 makes the validated `check` engine something a team can drop into pre-commit and CI
today. The order follows what each step needs from the previous one. First the engine
grows its two remaining graph-only diagnostics, `GRT002` and `GRT003`, and proves them on
the real corpus, because every output format and every recipe should describe the final
rule set, not a moving one. Then the machine-readable outputs (`graph --json`, SARIF) are
added in the interfaces layer, where SARIF can declare exactly one rule per final `GRT`
code. Last comes distribution: version injection, tagged static-binary releases, the
pre-commit hook and the CI recipes, which consume everything before them.

Constraints from PROJECT.md apply to every phase: zero false positives, deterministic
output, repository-relative paths, no network or external processes, read-only. The domain
layer must not import HCL and analyzers stay pure.

## Phases

- [x] **Phase 5: Graph Diagnostics** - `GRT002` (`config_path` to no unit) and `GRT003` (dependency cycle) as pure analyzers over the existing graph, validated on the three-repo corpus (completed 2026-09-30)
- [x] **Phase 6: Machine-Readable Output** - `gruntled graph --json` and `gruntled check --format sarif`, both accepted by their consumers, over the final rule set (completed 2026-10-01)
- [ ] **Phase 7: Distribution & CI Integration** - Version injection, tagged static-binary releases, the pre-commit hook and GitHub Actions / GitLab CI recipes

<details>
<summary>✅ v0.1 Validated Engine (Phases 1-4) - SHIPPED 2026-09-29</summary>

See `milestones/v0.1-ROADMAP.md`.

</details>

## Phase Details

### Phase 5: Graph Diagnostics
**Goal**: Users see wiring mistakes that the existing graph already answers, a dependency pointing at no unit and a dependency cycle, with the same zero-false-positive guarantee as `GRT001`.
**Depends on**: Phase 4 (v0.1 engine, graph, `checking` use case)
**Requirements**: MORE-01, MORE-02, MORE-06
**Success Criteria** (what must be TRUE):
  1. `gruntled check` reports `GRT002` on a unit whose `dependency` block has a literal `config_path` resolving to a directory that holds no unit, and stays silent when the `config_path` is non-literal or unresolvable
  2. `gruntled check` reports `GRT003` exactly once per dependency cycle, with members listed starting from the lexically smallest unit path, identical across repeated runs
  3. On the unmutated corpus, iso20022 and cds-snc/secret report no `GRT002`/`GRT003`; denis256 (a deliberately broken suite) reports exactly the set an independent oracle derives
  4. On the same corpus, every injected mutation (a `config_path` pointed at a missing directory, a back-edge closing a cycle) is caught (denis256 validated against its exact set plus mutations), and the results are recorded in `docs/validation.md`
**Plans**: 7 plans
- [x] 05-01-PLAN.md — domain foundation
- [x] 05-02-PLAN.md — analyzers
- [x] 05-03-PLAN.md — loader target state
- [x] 05-04-PLAN.md — dependencies paths
- [x] 05-05-PLAN.md — goldens+docs
- [x] 05-06-PLAN.md — corpus validation
- [x] 05-07-PLAN.md — gap closure: empty config_path matches terragrunt (block silent, paths "" GRT003 self-loop)

### Phase 6: Machine-Readable Output
**Goal**: Users and tooling can consume gruntled's graph and diagnostics as stable, deterministic documents, including one GitHub code scanning accepts.
**Depends on**: Phase 5 (SARIF declares one rule per final `GRT` code)
**Requirements**: INT-01, INT-02
**Success Criteria** (what must be TRUE):
  1. `gruntled graph --json` prints one document with a schema version, units, modules, dependency edges and unknown reasons, all paths repository-relative, byte-identical across runs and machines
  2. `gruntled check --format sarif` prints a SARIF 2.1.0 document with one rule per `GRT` code and repository-relative locations
  3. `github/codeql-action/upload-sarif` accepts that document without schema errors
  4. Exit codes of `check` are unchanged by `--format sarif`, and `graph` writes nothing inside the analysed repository
**Plans**: 5 plans
- [x] 06-01-PLAN.md — domain Edge extension + graph JSON presenter
- [x] 06-02-PLAN.md — SARIF presenter
- [x] 06-03-PLAN.md — CLI wiring (graph --json, --format sarif), help, docs usage, loops
- [x] 06-04-PLAN.md — goldens, SARIF structure test, Graph JSON / SARIF docs
- [x] 06-05-PLAN.md — vendored schema, fixture, CI schema check and upload-sarif job

### Phase 7: Distribution & CI Integration
**Goal**: A team can install gruntled from a tagged release and wire it into pre-commit and CI by copying a documented snippet.
**Depends on**: Phase 6 (recipes upload SARIF)
**Requirements**: REL-02, REL-01, INT-03, INT-04
**Success Criteria** (what must be TRUE):
  1. `gruntled --version` prints the release version and commit, injected at build time
  2. A tagged GitHub release offers static binaries for linux, darwin and windows on amd64 and arm64, plus a checksums file, and the no-net/no-exec binary proof still passes on them
  3. A user adds one hook entry to `.pre-commit-config.yaml`, served by this repository's `.pre-commit-hooks.yaml`, and `pre-commit run` fails on a broken unit
  4. The documented GitHub Actions recipe (check plus SARIF upload) runs green in this repository's own CI, and a GitLab CI recipe is documented
**Plans**: 5 plans
- [x] 07-01-PLAN.md — `--version` ldflags injection, clean check fixture
- [ ] 07-02-PLAN.md — windows/arm64 6th target (user decision), build-release.sh, CI packaging dry run
- [ ] 07-03-PLAN.md — release.yml tag-triggered pipeline (SHA-pinned)
- [ ] 07-04-PLAN.md — pre-commit hook, docs/ci.md recipes, recipe-check CI job
- [ ] 07-05-PLAN.md — checkpoint: user-approved push of master and release tag, verify release

## Progress

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 5. Graph Diagnostics | 7/7 | Complete    | 2026-09-30 |
| 6. Machine-Readable Output | 5/5 | Complete    | 2026-10-01 |
| 7. Distribution & CI Integration | 1/5 | In Progress | - |
