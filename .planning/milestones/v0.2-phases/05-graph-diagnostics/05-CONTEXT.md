# Phase 5: Graph Diagnostics - Context

**Gathered:** 2026-09-29
**Status:** Ready for planning

<domain>
## Phase Boundary

Two new pure-domain analyzers over the existing repository graph, wired into `checking.Check`:
`GRT002` (a statically resolved dependency path points at a directory with no unit config) and
`GRT003` (dependency cycle, one per SCC). This phase also models `dependencies { paths = [...] }`
edges, which the graph ignores today. The three-repo corpus is validated, and the result goes into
`docs/validation.md`. Out of scope: machine-readable graph output (Phase 6) and releases/CI (Phase 7).

</domain>

<decisions>
## Implementation Decisions

Decisions come from the user (Q&A) unless marked *[auto]*. The user asked Claude to answer
the planner-level questions on its own: *[auto]* items follow the question agent's
recommendations after review.

**Corrections (recorded in 05-01, after research):**
- The pinned oracle is `~/.cache/gruntled-phase4/bin/terragrunt_linux_amd64` (v1.1.6), not the
  PATH `terragrunt` (v0.99.3).
- The oracle command is `run --all --non-interactive --tf-path <tofu> -- version` on scratch
  copies. `hcl validate` does NOT detect missing targets; `dag graph` only warns on cycles.
- Include merge of `dependencies` is a union in both shallow and deep merge (see below).
- denis256 is a deliberately broken suite: on the unmutated corpus it must report exactly the set
  an independent oracle derives, not nothing (ROADMAP SC3 / MORE-06 amended).

### GRT002 trigger (user)
- Fires only when a resolved dependency target directory is missing, or holds no unit config
  file on disk. A directory that holds a config file the walk skipped (hidden dir,
  `.terragrunt-cache`, size limit, config-unknown) stays silent. Non-literal, unresolvable and
  escapes-repo paths stay silent (they are unresolved dependencies today).
- Silent unless `enabled` is literally `true` or absent (default). A non-literal `enabled` is
  silent. A deep-merged label's options are Unknown, so it is silent too *[auto]*.
- One code, two messages: "directory does not exist" / "directory has no terragrunt.hcl".
- Anchor: the position of the `config_path` attribute value (new position field on `Dependency`;
  the block position stays on `Pos()`).
- Also applies to each literal entry of `dependencies { paths }`, anchored at the list element.
  Non-literal elements are silent.
- `skip_outputs` and `mock_outputs` do not gate GRT002 *[auto]*. The oracle confirms that Terragrunt
  still errors on a missing target with `skip_outputs = true`. If the oracle shows that Terragrunt
  tolerates it, switch to silent and record why.

### GRT002 edge cases [auto]
- "Literal" means statically resolvable: plain strings plus the six path functions that `evalPath`
  already resolves. The docs say "statically resolvable".
- The domain cannot stat. The loader records a target-state enum on resolved deps:
  `Unknown` (zero value, silent) / `DirMissing` / `NoConfig` / `HasConfig`. Unresolved deps are
  always `Unknown`. The enum is validated in the constructors.
- "Missing" only when `errors.Is(err, fs.ErrNotExist)`. Any other stat error (EACCES, ENOTDIR,
  symlink escaping the `os.Root`) gives `Unknown` and stays silent.
- A config file is `terragrunt.hcl` OR `terragrunt.hcl.json`. Symlinks are followed. A dangling
  symlink counts as absent.
- Existence is checked case-exactly (read the parent directory, compare names), so Linux, macOS
  and Windows give byte-identical output.
- `config_path = "../x/terragrunt.hcl"` that does not exist: map it to the parent directory, then
  classify as usual. Non-default file names and stack targets stay unresolved and silent, as today.
- `config_path = ""` stays silent (invalid, not a cycle; the oracle confirms Terragrunt's
  behaviour). `"."` and `"../<self>"` are self-loop edges: GRT003, not GRT002.
- Message: `dependency "vpc" config_path resolves to "live/dev/vpc": directory does not exist`,
  and for paths `dependencies path "../x" resolves to "a/x": ...`. Paths use `strconv.Quote`.
  No hints (Message is part of the Key).
- A shared include with a missing target gives one GRT002 per including unit, at the same
  file:line:col (Key includes Unit), consistent with GRT001.
- A `dependency` block and a `dependencies` path naming the same missing target both fire.
  Each one is a real edit.

### GRT003 cycle definition (user)
- One diagnostic per strongly connected component (SCC). A self-loop is a one-member cycle.
- Edges: resolved deps whose `enabled` is literally true or absent. `skip_outputs` still counts,
  because Terragrunt still orders the units. Disabled and non-literal `enabled` drop the edge.
- `dependencies { paths }` edges ARE part of the cycle graph (they are modeled in this phase).
- Anchor: Unit = lexically smallest SCC member. Position = its first out-edge (by
  `Position.Compare`) to another SCC member, or the self-edge for a one-member SCC. The anchor is
  the path-value position, the same rule as GRT002 *[auto]*.
- Message: a simple ring renders as `dependency cycle: "a" -> "b" -> "c" -> "a"`, walking unique
  successors from the smallest member. Any other SCC renders as
  `dependency cycle among: "a", "b", "c"` (sorted).

### GRT003 edge cases [auto]
- Before ring detection, out-edges are deduplicated by target (a `dependency` plus `dependencies`
  pair to the same target is one edge). An SCC is a ring only when every member has exactly one
  distinct in-SCC successor. A self-loop inside a larger SCC gives one "among" diagnostic and no
  extra.
- Members are quoted. There is no size cap. Document that the Key changes when membership changes.
- Nodes are all units. Edges come from resolved and module-unknown units only. Config-unknown units
  have no edges, so cycles through them are invisible (false negatives only; documented).
- An edge to a target that is not a graph unit (symlink alias, vendor, cache) is dropped. There is
  no canonicalisation in v0.2 (documented limitation).
- Algorithm: iterative Tarjan. Units are visited in RepoPath order and adjacency is sorted, using
  only the domain allowlist (`slices`/`sort`). A test proves that input order does not change the
  output, and a deep chain (10k units) does not overflow.

### `dependencies` block modeling [auto]
- New domain type `PathDependency{target, resolved, reason, pos, targetState}` on `Unit`, exposed
  as `Unit.PathDependencies()` and sorted by position. It is not folded into `Dependency`
  (name uniqueness invariant).
- `paths` is evaluated element by element with `evalPath`, using the same scope and base directory
  as `config_path` (the child `unitDir`, the include `fileScope`). A non-tuple `paths` expression
  (`local.x`, `concat`) is recorded as Unknown (not absent), so it gives no edges and no GRT002,
  and Phase 6 can show it.
- Resolution shares one helper with `resolveOneDependency` (file, stack, escape classification).
- Include merge: UNION with de-duplication in BOTH shallow and deep merge. CORRECTED per
  research: terragrunt v1.1.6 `pkg/config/include.go` Merge (~L402-407) calls
  `ModuleDependencies.Merge` (`pkg/config/config.go`) which appends absent paths, and DeepMerge
  (~L506-545) also unions. Edges are deduped by target so cycles are unaffected; `no_merge`
  includes contribute nothing.
- A structurally invalid `dependencies` block (duplicate, label, missing or non-list `paths`)
  drops only the path edges. The unit stays resolved, so pinned corpus counts and GRT001 are
  unchanged.
- `NewConfigUnknownUnit` stays dep-free.

### Domain/API [auto]
- `NewDependency` and `NewUnresolvedDependency` get an explicit path-position parameter (zero is
  rejected). Every call site is updated; there is no parallel constructor.
- New `RepositoryGraph.Edges()`: deterministic, with kind (block/paths), from, to, position and
  enabled fact. The "which edges count" rule stays in the analyzer. Phase 6 reuses `Edges()`.
- Codes: `CodeMissingDependencyTarget = "GRT002"` and `CodeDependencyCycle = "GRT003"` in
  `diagnostic.go`.
- Analyzers live in `internal/domain/analysis` (pure, no `fmt`). `checking.Check` runs all three.
  The canonical Set makes order irrelevant. Update the `Report.Diagnostics` doc comment.
- Severity: both are `error` (exit 1), the same as GRT001.

### Corpus validation, MORE-06 (user)
- Per repo (iso20022, cds-snc/secret, denis256), four mutations, each applied alone to a fresh
  copy: a `dependency` `config_path` pointing at a missing directory, a `dependencies` path
  pointing at a missing directory, a `dependency` back-edge closing a cycle, and a self-loop. Each
  must add exactly the expected diagnostic. A repo without edges gets a minimal synthetic edge,
  documented as synthetic.
- Oracle: pinned terragrunt on each mutated tree, where it can run.
- Extend the Phase 4 harness and pinned SHAs. `docs/validation.md` gets v0.2 sections.

### Corpus edge cases [auto]
- **denis256 unmutated is NOT clean.** It already contains real missing targets
  (`perf-tests/code-v2/app-template` -> `../../deps/dep-1..5`, `tf-lint-regeneration/dev/apps/app-1`,
  `module-output-broken/m1`, `hcl/terragrunt.hcl:19`). It is a deliberately broken test suite.
  Resolution: for denis256, SC3 becomes an exact expected GRT002/GRT003 set, derived from an
  independent oracle, the same way GRT001 is treated on denis256. `denis256_test.go` (which today
  fails on any code other than GRT001) is updated. The planner amends the ROADMAP SC3 and MORE-06
  wording: "iso20022 and cds-snc/secret report nothing; denis256 matches its oracle set exactly".
  iso20022 and secret must stay strictly clean.
- The oracle has limits: terragrunt v1.1.6 rejects secret's unnamed `include {}` and crashes on
  denis256. Plan: terragrunt on iso20022. For secret and denis256, use an independent textual
  oracle (grep + `test -d` for GRT002, coreutils `tsort` for cycles), and document why.
  Optionally, also run terragrunt on a scratch copy of secret with the include named, recorded
  separately.
- Oracle commands: pinned terragrunt v1.1.6 (`~/.cache/gruntled-phase4/bin/terragrunt_linux_amd64`)
  `run --all --non-interactive --tf-path <tofu> -- version` on scratch copies; it fails on both a
  missing dependency target and a cycle. `hcl validate` does NOT detect missing targets and
  `dag graph` only warns on cycles, so neither is the oracle. Never `run --all plan` (not offline).
- Mutation sites are resolved units only (not iso20022's `iac.cicd/codebuild_project`
  config-unknown units). Mutations are pinned via the existing `corpusMutation{File, Old, New}`
  exact-occurrence check. The self-loop is spelled `config_path = "../<self>"`; `"."` is covered
  in golden fixtures only.
- Fifth mutation on iso20022 only: point at a module-only directory, so the "has no terragrunt.hcl"
  message has real-corpus evidence.
- Each mutation asserts the exact diagnostic set equals the oracle, exit code 1, GRT001 counts
  unchanged, and that reverting gives byte-identical JSON (same as VALID-04).
- Pin cds-snc/secret: `GRUNTLED_CORPUS_SECRET` plus a pinned commit, failing (not skipping) on
  the wrong commit.

### Existing tests and docs [auto]
- Golden `dependency_edges.txtar` (`config_path = "../nodir"`) will now fire GRT002 and change
  exit code. Update the golden; the plan states that the new output is correct. Re-run every
  golden and review each change.
- `docs/validation.md`: retitle for v0.2, add a new VALID section for GRT002/GRT003 in the same
  file, and extend `validation_doc_test.go` pins. `docs/cli.md`: move GRT002/GRT003 from
  "reserved" to documented codes (decision tables and rejected alternatives, as for DIAG-03),
  and remove "cycles not reported" from the limitations. Small README edit to the codes list only.
- A Windows test uses a backslash path (already `config-path-invalid`).

### Claude's Discretion
- Package split inside `internal/domain/analysis`, test helper shapes, exact enum names, the
  internal adjacency representation, and optional per-analyzer `Stage` labels.

</decisions>

<code_context>
## Existing Code Insights

### Reusable Assets
- `repograph.RepositoryGraph.DependencyTarget` / `Units()` / `Unit()`: node set for SCC; GRT002 needs the new target state, not `DependencyTarget` (false conflates "not a unit" with "skipped").
- `repograph.DependencyOptions` Tristate `enabled`: edge gating identical to GRT001 row 2.
- `analysis/grt001.go`: pattern for a pure analyzer returning `[]diagnostic.Diagnostic` via `NewForUnit`.
- `terragrunt/loader.go:resolveOneDependency` + `eval.go:evalPath`: path resolution and classification to share with `dependencies` paths.
- `terragrunt/merge.go` (`buildEffectiveFiles`, `fileScope`): include precedence and `no_merge` handling for the new block.
- Phase 4 harness (`cmd/gruntled/corpus_test.go` `corpusMutation`, `denis256_test.go`): mutation pinning and exact-set assertions.

### Established Patterns
- Arch rules (`scripts/check-architecture.sh`): domain allowlist, no fmt; application stdlib allowlist; HCL only in infrastructure. The new code must pass them unchanged.
- Diagnostic Key `{Code, Unit, File, Line, Column, Message}`, Severity outside the Key; canonical `diagnostic.Set`.
- Byte columns from `hcl.Pos.Byte`; repo-relative paths; deterministic sort everywhere.
- Unknown means silent. It is never guessed.

### Integration Points
- `internal/application/checking/check.go:Check`: append the GRT002/GRT003 analyzers.
- `internal/domain/diagnostic/diagnostic.go`: new code constants.
- `internal/domain/repograph/unit.go` and `graph.go`: path position, target state, PathDependency, `Edges()`.
- `internal/infrastructure/terragrunt`: stat classification, `dependencies` parse, merge.
- `docs/cli.md`, `docs/validation.md`, `validation_doc_test.go`, testscript goldens.

</code_context>

<specifics>
## Specific Ideas

- The GRT003 message should read as a path the user can follow and fix, starting at the smallest member.
- The same zero-false-positive rule as GRT001: anything uncertain stays silent, and false negatives are documented.
- Both GRT002 messages need real-corpus evidence, not only goldens.

</specifics>

<deferred>
## Deferred Ideas

- Canonicalising symlink aliases to the real unit for cycle edges (v0.3 or later).
- Modelling `exclude {}` blocks to drop excluded units from the DAG. Today they keep their edges.
  Check whether any corpus unit uses `exclude` before deciding.
- A size cap or truncation for huge SCC messages.

</deferred>

---

*Phase: 05-graph-diagnostics*
*Context gathered: 2026-09-29*
