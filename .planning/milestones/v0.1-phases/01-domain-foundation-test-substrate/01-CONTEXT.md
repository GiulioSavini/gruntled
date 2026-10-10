# Phase 1: Domain Foundation & Test Substrate - Context

**Gathered:** 2026-09-25
**Status:** Ready for planning
**Source:** Direct user instruction ("ricorda architettura DDD ordinata") + design doc §5 + research/ARCHITECTURE.md §B.6-B.8

<domain>
## Phase Boundary

Phase 1 builds two things:
- the pure domain layer: `Unit`, `Module`, `Surface`, `Reference`, `RepositoryGraph`, `Diagnostic`;
- the deterministic synthetic Terragrunt repository generator that later phases use as their test substrate.

No HCL parsing, no filesystem walking of real repos, no CLI behavior. Those belong to Phases 2-3.

</domain>

<decisions>
## Implementation Decisions

### Architecture — clean DDD/hexagonal layering (LOCKED, non-negotiable)
- Follow the layout in `research/ARCHITECTURE.md` §B.6 exactly:
  - `cmd/gruntled/`: composition root only.
  - `internal/domain/`: zero external dependencies. The bounded contexts are subpackages, not separate Go modules.
  - `internal/application/`: use cases, plus `ports/` (interfaces owned by application).
  - `internal/infrastructure/`: the ONLY place where HCL, go-getter, fsnotify, sockets and the filesystem may appear.
  - `internal/interfaces/`: cli, presenter.
- Phase 1 creates only what it needs: `internal/domain/repograph`, `internal/domain/diagnostic` and `internal/testsupport/synthrepo`. Do not scaffold empty packages for later phases.
- The domain uses the ubiquitous language from design doc §5.3. Value objects are immutable. `RepositoryGraph` is the aggregate root and bakes in explicit-sort invariants from day one, which determinism depends on.
- ARCH-01: a CI job fails the build when the domain's transitive deps include hcl, terraform-config-inspect, go-getter or fsnotify (`go list -deps ./internal/domain/...`). It must also fail on any `os`/`io/fs`/`path/filepath` filesystem access from the domain. Keep the CI simple, with few jobs, and make sure each job can genuinely fail.
- ARCH-02: analyzers and graph queries are pure functions over domain types. They are tested with hand-built domain objects only.
- `internal/testsupport/synthrepo` is test support. It writes files (that is its job), but it lives outside `domain/`, `application/`, `infrastructure/` and `interfaces/`, and the shipped binary never links it.

### Generator (VALID-01)
- `Spec{Units, IncludeDepth, DependencyFanout, Seed}` → same Spec + seed produce a byte-identical tree on repeated runs.
- It can inject at least one known error kind (a bad `dependency.X.outputs.Y` reference) and returns a manifest of the diagnostics a correct analyzer must report.

### Repo conventions
- Commits authored as Giulio Savini <giuliosavini@proton.me>, with no Co-Authored-By trailer.
- Every change is committed, pushed and tested.

### Claude's Discretion
- Exact field names, constructor shapes, error types and file split inside each package, as long as the layering holds.

</decisions>

<specifics>
## Specific Ideas

- `research/ARCHITECTURE.md` §B.7 build order (Tier 0) and §B.8 (synthrepo design) are the reference.
- The design doc lives at `docs/superpowers/specs/2026-09-01-gruntled-design.md` (§5 Architettura, §6 Idempotenza, §9 Testing).

</specifics>

<deferred>
## Deferred Ideas

- blast, analysis (GRT001), application use cases, all adapters, and the CLI: Phases 2-3.

</deferred>

---

*Phase: 01-domain-foundation-test-substrate*
*Context gathered: 2026-09-25 from direct user instruction*
