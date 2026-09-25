# Phase 2: Parsing & Graph Construction - Context

**Gathered:** 2026-09-25
**Status:** Ready for planning
**Source:** Orchestrator decisions (user delegated all choices), the Phase 1 architecture review, 02-RESEARCH.md open questions, 02-TERRAGRUNT-EDGECASES.md

<domain>
## Phase Boundary

Turn a Terragrunt repository on disk into a `repograph.RepositoryGraph` that carries every
fact GRT001 (Phase 3) needs, using structural HCL decoding only. Phase 2 does NOT implement
GRT001, the mock_outputs rule, the CLI or presenters. It DOES extend the domain so those facts
have a home (see "Domain changes"). Everything uncertain fails toward `unknown`, never toward a
diagnostic: zero false positives is the milestone's claim.
</domain>

<decisions>
## Implementation Decisions

### Layering (locked, CI-enforced)
- HCL (`hashicorp/hcl/v2`, `zclconf/go-cty`) only in `internal/infrastructure/...`.
- Ports live in `internal/application/ports`. Two ports: `UnitLoader` and `SurfaceReader`
  (research Pattern 10). `sourceresolve` is a pure collaborator inside infrastructure, not a port.
- One use case: `internal/application/indexing.Build`, tested with fakes only (no filesystem).
- `cmd/gruntled` stays the composition root; Phase 2 need not wire a CLI.
- New arch-check rules, each with a self-test case that must fail on THAT rule name
  (the self-test already asserts the specific `=== RULE FAILED: <name> ===` line):
  1. `application-stdlib-allowlist`: application may import only the domain allowlist plus
     `context` (tests additionally `testing`, `reflect`). No `fmt`, `os`, `io/fs`, `log`.
     Error wrapping uses `errors` and custom error types, not `fmt.Errorf`.
  2. `application-external-deps`: non-std deps of `./internal/application/...` (with -test)
     must match `^<module>/internal/(domain|application)/`.
  3. `hcl-only-in-infrastructure`: no package outside `internal/infrastructure/` may directly
     import `github.com/hashicorp/...` or `github.com/zclconf/...` (Imports, TestImports,
     XTestImports).
  4. `application-platform-neutral`: same build-constraint rule as the domain.
  5. A non-vacuous guard for application (at least one package).
  The existing domain rules must not be weakened. The arch rules and self-tests land BEFORE any
  infrastructure package exists.

### Domain changes (Phase 2 owns them, per the architecture review)
- **Unit whose module is unknown keeps its deps and refs.** GRT001 checks the TARGET unit's
  module outputs, not the referencing unit's module. So distinguish:
  - config-unknown unit (its own terragrunt.hcl or a merged include is broken, or an include /
    config path attribute is dynamic): no deps, no refs, silent;
  - config-known unit whose module is unknown (remote source per GRAPH-03, dynamic source,
    module dir missing, module surface unknown): deps and refs retained, the unit's own module
    is reported as unknown with a reason. GRAPH-03's "marks the owning unit unknown" is met by
    this module-unknown state.
  - `NewRepositoryGraph` must accept a module whose surface is unknown (reason string) so
    `ModuleOf(target)` can answer "unknown surface" instead of the unit being dropped.
- **Per-dependency unresolved state.** A `dependency` whose `config_path` is dynamic,
  unresolvable or escapes the repo is kept on the unit as unresolved (with reason), never
  dropped. Dropping it would turn `dependency.X.outputs.Y` into a reference to an undeclared
  dependency, which is a false-positive risk. A resolved dependency whose target directory is
  not a discovered unit is also kept (target recorded, `DependencyTarget` returns false).
- **Dependency options struct** (constructor takes an options value so the signature does not
  grow): `enabled`, `skip_outputs`, `mock_outputs` (present / keys known / sorted keys),
  `mock_outputs_merge_with_state` (plus `mock_outputs_merge_strategy_with_state` != no_merge),
  `mock_outputs_allowed_terraform_commands` (known / list). Every fact is tri-state:
  literal true/false/list, or "not literal" (unknown). Absent attribute is its Terragrunt
  default. Phase 3 decides the DIAG-03 rule; Phase 2 only captures facts faithfully.
- **Diagnostic identity.** `Key` becomes `{Code, Unit, File, Line, Column, Message}`, where
  Unit is an optional RepoPath (zero for file-level diagnostics like GRT100). Reason: a
  reference written in a shared include file is evaluated per including unit; two units can
  resolve the same `dependency.vpc` differently, and without Unit their diagnostics would
  collapse into one. Message stays in the Key. Severity stays OUT of the Key on purpose (a
  severity change of the same finding is not a new finding); document this on `Key` and `Diff`
  and add a test asserting Diff ignores a severity-only change.
- **Columns are bytes.** Domain `Position` column is a 1-based byte column. Infrastructure
  converts from `hcl.Pos.Byte` (never `hcl.Pos.Column`, which counts grapheme clusters).
  A test with a non-ASCII prefix on the line is required.
- Domain stays pure: all changes pass the allowlist (no fmt).

### Parsing
- Only the six path functions, evaluated only inside the three path-bearing attributes
  (`include.path`, `dependency.config_path`, `terraform.source`). Anything else there
  (`dirname`, `get_repo_root`, `local.x`, `run_cmd`, `get_env`) -> `dynamic-path`. For include
  paths that makes the unit config-unknown; for `config_path` it makes that one dependency
  unresolved; for `source` it makes the unit's module unknown.
- **Includes can declare dependencies.** Infrastructure merges dependency blocks from includes
  into the unit (Terragrunt precedence: child > last include > first include; `no_merge`
  includes excluded; `config_path` resolves against the CHILD unit dir) BEFORE calling the
  domain constructor, which rejects duplicate names.
- **References come from the unit's ENTIRE effective body**: `locals`, `inputs`, `generate`,
  `remote_state`, `terraform` (hooks, extra_arguments), `dependency`/`dependencies` blocks,
  every attribute and nested block, plus the bodies of merged include files (positions in the
  include file, attributed to the including unit). AST traversal only, sorted by file then
  byte offset. `try()`/`can()`/whole-object `dependency.X.outputs` handling per research
  Pitfall 6 (a reference that is not a static `dependency.X.outputs.Y` traversal is not a
  reference; never guess).
- Invalid HCL: `hclsyntax.ParseConfig` partial bodies are never analysed. First error per file
  (sorted by position) becomes one GRT100 diagnostic; every unit touching that file is
  config-unknown (unit file / include) or module-unknown (module `.tf` file).
- Deep-merge (`merge_strategy = "deep"`) of dependency blocks: keep config_path precedence
  exactly, and mark mock facts "not literal" (unknown) rather than modelling the deep merge.

### Walk
- Never follow symlinked directories. File reads go through `os.Root` (Go 1.27), which follows
  in-repo file symlinks and refuses escapes. Symlinked `.tf` files in modules ARE followed
  (the primary corpus depends on them, research Pitfall 1). A symlinked `terragrunt.hcl` is not
  discovered as a unit.
- Skipped directories: `.git`, `.terragrunt-cache`, `.terraform`, `vendor`. Module dirs inside
  them are still readable when a unit's `source` points there (skipping only drops units, never
  adds diagnostics).
- `.terragrunt-stack` IS walked (PROJECT.md is authoritative over PITFALLS.md §6; Terragrunt
  discovery includes it; its units usually have remote sources and become module-unknown).
- Symlink-escape test uses a real `t.TempDir()` with `os.Symlink`; on platforms where the
  symlink cannot be created the test calls `t.Skip`. MapFS covers the rest.

### Testing
- Every uncertain decision point has a table test proving its uncertain input goes to unknown.
- A hand-written fixture puts `dependency.X.outputs.Y` refs in `locals`, `inputs`, a nested
  block and an include file, and asserts all are extracted with byte-correct positions.
- Determinism: building the graph twice, and from two differently-named temp dirs
  (synthrepo.Generate), gives an identical canonical dump; no absolute path appears.
- Parse-once: a counting FS proves each include file is read exactly once.
- Fuzz target over the unit loader with broken seeds; seeds run in plain `go test`.
- All new code race-free (`go test -race` runs in CI) and staticcheck-clean (no dead helpers).

### Claude's Discretion
- Exact type and function names, DTO field names, unknown-reason strings (stable kebab-case).
- Package split inside `internal/infrastructure`.
</decisions>

<specifics>
## Specific Ideas
- Research Pattern 7 unknown-reason catalogue, Pattern 9 source classifier and Pattern 11
  surface reader are adopted, adjusted for the config-unknown / module-unknown split above.
- 02-TERRAGRUNT-EDGECASES.md: every edge case the plans choose to handle or send to unknown
  should be traceable to a test; cases left for later must be listed in a plan's SUMMARY.
</specifics>

<deferred>
## Deferred Ideas
- GRT001 analyzer, DIAG-03 mock rule, CLI, presenters: Phase 3.
- Corpus run and benchmark: Phase 4 (an env-gated corpus smoke test is welcome in Phase 2).
- `dirname`, `get_repo_root`, `read_terragrunt_config`, locals evaluation, `terragrunt.stack.hcl`
  parsing: out of scope (units become unknown).
</deferred>

---

*Phase: 02-parsing-graph-construction*
*Context gathered: 2026-09-25 by the orchestrator on the user's behalf*
