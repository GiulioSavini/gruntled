# Feature Research

**Domain:** Blast-radius / change-impact analysis for Terragrunt repositories (milestone v0.4, Blast-aware Diagnostics)
**Researched:** 2026-10-10
**Confidence:** MEDIUM-HIGH (comparable-tool behaviour verified against current vendor docs; Terraform type-system points partly from prior knowledge and flagged for phase research)

Scope: only the four v0.4 features (GRT004, `report --blast` + `rebase`, transitive Impacted with path, type-level surface changes). Existing behaviour (`check`, `graph`, one-hop name-level `blast --base`, `watch`, `report`) is taken from `docs/cli.md` and the code in `internal/domain/impact`.

Hard constraints applied to every verdict below: zero false positives (silent when unsure), byte-deterministic output, no network and no external process, structural decode only (never evaluate an expression), read-only.

---

## What comparable tools actually do (findings that drive the verdicts)

| Tool | Baseline | What is "changed" | Dependents | Shows why / path? | Source |
|------|----------|-------------------|------------|-------------------|--------|
| Terramate | git: HEAD by default, `--git-change-base` to pick another; also uncommitted and untracked files | file-level, per stack | `--only-all-dependents`, `--include-all-dependencies` (transitive, opt-in flags). Dependencies = data dependencies only (`input.from_stack_id`, Terragrunt `dependency` with data sharing). Order-only (`before`/`after`, `wants`) and Terragrunt ordering-only dependencies are explicitly NOT dependencies | No reason or causal chain printed | terramate.io/docs/cli/change-detection (HIGH) |
| Terragrunt `--filter` (replaces `--queue-include-*`) | git expression `[main...HEAD]` or `[ref]`; `--filter-affected` = default branch vs HEAD; Terragrunt builds temporary worktrees and runs `git diff` | a unit whose `terragrunt.hcl` is added/modified, or deleted; for stacks, files read via `read_terragrunt_config()`. `--queue-include-units-reading` = `reading=<file>` filter | Graph expressions: `...vpc` (target + all dependents, transitive), `vpc...` (target + dependencies), `...^vpc` (exclude target), numeric depth `1...vpc` (direct dependents only). Depth applies per target. Dependents traversal is documented as the expensive direction | No path | docs.terragrunt.com/features/filter, /graph, /git (HIGH) |
| Pants | `--changed-since=<ref>` (merge-base idiom for CI) | file-level | `--changed-dependents=direct\|transitive`. Docs warn about over-approximation through lockfiles and target generators | No path | pantsbuild.org advanced-target-selection (HIGH) |
| Nx | `--base` (default main) and `--head` (default working tree) | file-level, mapped to project | All direct and indirect dependents are affected. Lockfile change marks everything affected unless `projectsAffectedByDependencyUpdates: "auto"`. `nx graph --affected` draws the set | Draws the affected subgraph, no per-project reason | nx.dev/docs/features/ci-features/affected (HIGH) |
| Bazel query | none (static graph of one state) | n/a | `rdeps(universe, x[, depth])` with optional depth bound; `somepath(S,E)` returns "some arbitrary path", not shortest or longest; `allpaths` returns the whole subgraph; `--order_output=full` is the deterministic mode (sort nodes, post-order DFS with alphabetical edges) | `somepath`/`allpaths` exist, but `somepath` is explicitly non-canonical | bazel.build/query/language (HIGH) |
| Atlantis | git: files modified in the PR | directories containing modified `.tf` files; changes under `modules/` plan nothing unless `atlantis.yaml` `when_modified` or `autoplan-modules` is configured | Only via local-module indexing (opt-in), no output-aware logic | No | runatlantis.io/docs/autoplanning (HIGH) |
| Spacelift | tracked run | the stack that ran | Downstream stacks queue after an upstream tracked run. With reference outputs, a downstream stack is triggered "only if the referenced output has been created or changed" (value-level gate), unless "Trigger always" | Dependency chain visible, failure "breaks the chain" | docs.spacelift.io/concepts/stack/stack-dependencies (HIGH) |
| Digger | git diff plus `include_patterns` per project | files | Terragrunt dependencies between projects were not supported natively (issue #402); users generate `digger.yml` with a tool such as terragrunt-dag | No | github.com/diggerhq/digger/issues/402, docs.digger.dev include/exclude (MEDIUM, search snippets) |
| tflint | none: stateless, per directory | n/a | n/a | n/a | No diff or baseline mode in the rules gruntled cares about (MEDIUM, not fetched, 404 on rules index) |
| buf breaking / oasdiff | `--against` any prior input (git ref, dir, registry); oasdiff compares two specs | contract elements | n/a | Each break is a located diagnostic (`file:line:col: Field "1" on message "User" changed type from "int32" to "string"`). oasdiff grades ERR/WARN/INFO and defines breaking as "a request that was valid before can now be rejected, or a response can now carry something clients were never written to handle" | buf.build/docs/breaking, oasdiff.com (HIGH) |
| Watchman (daemon analogy) | explicit clock token the client holds; named cursors advance on every query and "cannot be rolled back" | files since clock | n/a | n/a | facebook.github.io/watchman/docs/clockspec (HIGH) |

Cross-cutting conclusions:

1. **Every graph tool propagates dependents transitively through every edge, regardless of what changed in the middle node.** None of them has a notion of "surface", so the edge itself is the contract. Only Spacelift gates on whether a referenced output changed, and it does so with runtime values, which gruntled cannot see. So "Impacted propagates through units whose own surface did not change" is the expected behaviour, and stopping at one hop (v0.3) is the outlier. Terramate's refinement is the one worth copying: **only data edges carry impact; ordering-only edges do not**.
2. **Direct-only vs transitive is always a user-visible switch** (Pants `direct|transitive`, Terragrunt `1...`, Bazel `rdeps` depth, Terramate opt-in flags). Keeping v0.3's one-hop view reproducible is table stakes.
3. **Nobody prints the connecting path.** Terramate prints no reason, Nx draws a subgraph, Pants and Terragrunt print a flat set, Bazel's `somepath` is deliberately arbitrary. A deterministic, explained path is a genuine differentiator, but it must be defined as "a shortest path, ties broken by path order", never "the path".
4. **Baselines everywhere are a git ref recomputed from scratch on every call.** None of them keeps a baseline in a long-running process. gruntled's in-memory baseline has no direct precedent, so its rules must be designed (below), not copied. Closest analogy is Watchman: an explicit, caller-held checkpoint, with the lesson that a checkpoint that advances implicitly (named cursors) makes results depend on query history.
5. **Breaking-change tooling reports each break as a located, typed diagnostic and separates "breaking" from "informational".** That is the model for GRT004 (located at the consumer reference, same as GRT001) and for grading type-level changes (fact vs verdict).
6. **Terragrunt upstream is moving toward static dependency-output contracts.** RFC gruntwork-io/terragrunt#6111 (open, filed 2026-05-14) proposes an `outputs` block on units declaring `type` and `mock_value` per output, with `hcl validate` checking declared outputs exist in the module, dependents reference declared outputs, and mocks match types. If it ships it narrows GRT001's niche but does not give a diff or a baseline, so GRT004 and blast stay differentiated. Reinforces the PROJECT.md stance that the warm index and the baseline diff are the defensible position, not the point checks. (HIGH on facts, speculative on outcome.)

---

## Feature Landscape

### Table Stakes (Users Expect These)

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| **Transitive Impacted** along data dependency edges | Pants/Nx/Terragrunt `...`/Terramate all propagate to indirect dependents; a one-hop blast radius reads as a bug | MEDIUM | Reverse BFS over the CURRENT graph. Seeds = units whose module surface changed (existing `SurfaceDiff`), computed BEFORE the Broken filter. Propagate only over edges that are certain data edges: `kind=block`, `enabled` literally true, `skip_outputs` literally false, `target_state=has-config`. `paths` edges (ordering only) and any tristate `unknown` do not propagate (Terramate precedent; zero-FP). Cycles (GRT003) handled by visited set. Unit reached through a Broken unit is still reported (a broken unit cannot deliver outputs), the Broken unit itself stays out of Impacted to keep the lists disjoint. Pass-through, but seeds stay surface-only: a new GRT002 in an unrelated unit does not seed impact. |
| **Direct-only switch** (`--depth N`; `--depth 1` == v0.3 behaviour) | Every comparable tool offers direct vs transitive; v0.3 docs promise one hop | LOW | Default unlimited. Keeps existing v0.3 golden tests valid under `--depth 1`. Bazel/Terragrunt use a number, so a number beats a boolean. |
| **Connecting path per transitive unit** | The point of the feature: "why is X in the list?" | MEDIUM | Shortest path (fewest hops), ties broken by lexicographic comparison of the path as a sequence of `RepoPath`, then the first seed. Arrow direction = "depends on", same as GRT003 messages: `live/web -> live/cache -> live/db`. Last element is a unit whose module changed; entry also names that module and its changes (existing fields). Direct entries have a 1-element path and `distance 1`. Bazel `--order_output=full` shows the sort-then-DFS recipe for determinism. |
| **`GRT004` located at the consumer reference, superseding GRT001 for the same reference** | buf/oasdiff report each break as a located diagnostic; PROJECT.md says "an error that names its consumers" | MEDIUM | Exists only where a baseline exists (blast, `report --blast`); `check` and plain `report` stay GRT001-only so `report == check` byte identity (DAEMON-04) is untouched. Fires iff ALL of: GRT001 would fire (same rows 1-5 of the DIAG-03 table, incl. `enabled`/`skip_outputs`/unknown-module silences), AND the same module path (target unit's module) had a known surface in the baseline that declared the output. Otherwise a new GRT001 stays GRT001 (output never existed: typo, not removal). Never both codes on one reference. `mock_outputs` rule unchanged: never suppresses, same suffix logic. Message names removal and consumer, e.g. `output "id" was removed from module "modules/vpc" (target unit "live/db") but dependency "db" still reads it`. Severity error, exit 1 in blast. Needs a SARIF rule entry only if emitted in SARIF (see differentiators). |
| **Consumers grouped per removed output in blast text/JSON** | "names its consumers": a user fixing a removed output wants the list, not 14 scattered lines | LOW | Pure presenter grouping by (module, output) over GRT004 findings; the diagnostics themselves stay one per reference so identity and SARIF stay simple. |
| **Daemon baseline = the daemon's first complete index, moved only by explicit `rebase`** | Watchman lesson: explicit, caller-owned checkpoint; implicit advancing makes output depend on history | MEDIUM | Baseline snapshot = `(graph, diagnostics)` of the published index, exactly the two inputs `impact.Compute` already takes ("serves a git-ref baseline today and an in-memory baseline later"). Do NOT keep ASTs. Output of `report --blast` is a pure function of (baseline snapshot, current index). |
| **`report --blast`** (text, json; exit codes like `blast`) | Milestone goal; parity with `blast --base` | MEDIUM | Same renderers as `blast`; label line `baseline: daemon start` / `baseline: rebase #N` (counter, not a clock, to keep bytes deterministic). Exit 1 only when Broken has an error (pre-existing findings never fail), unlike plain `report`. `sarif` stays a usage error unless the SARIF differentiator is taken. Reuses the "last published result" rule, `still indexing` = exit 3. |
| **`gruntled rebase [path]`** | Without it the baseline is frozen at startup and `report --blast` degrades to noise after the first merge | MEDIUM | Sets baseline := current published index; prints what it discarded (`baseline moved: dropped N broken, M impacted`) so rebasing over unresolved breakage is not silent. First mutating request on the socket, so protocol version bump and a clear mismatch message (existing mechanism). Idempotent. Exit 3 when no daemon / still indexing, as `report`. |
| **Variable "required" fact: `+required variable x` and `x now required` (default removed)** | Item 4 of the milestone; v0.3 limitation text names it explicitly | LOW-MEDIUM | Decidable structurally: a variable is required iff it has no `default` argument. Reader (`tfsurface`) must record presence of `default`, not its value. Shown as an Impacted change detail, never Broken (inputs are merged, env/var-file supplied, never evaluated). |
| **Variable type change as a fact: `~variable x: list(string) -> set(string)`** | Item 4; buf/oasdiff report "changed type from A to B" | MEDIUM | Parse the `type` expression with the HCL type-constraint parser (`hcl/v2/ext/typeexpr`, already in the dependency tree via hcl/v2; adapter converts to a canonical string, domain never imports HCL). Compare canonical forms so formatting/comments/whitespace/`optional()` spelling do not register. Absent `type` == `any`. Anything unparseable, a `.tf`/`.tofu` pair that declares the variable twice with different types, or an override file in play: unknown, silent. Report the fact old -> new; make no breaking/compatible claim (see anti-features). |
| **Deterministic ordering for every new list** | Project constraint | LOW | Impacted sorted by unit (unchanged); changes in the existing order, new kinds appended in a fixed order; GRT004 in `diagnostic.Compare` order. |

### Differentiators (Competitive Advantage)

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| **Explained, canonical path** (see table stakes) | No comparable tool prints why a unit is impacted; Terramate explicitly does not | MEDIUM | Differentiator in value even though the work sits in the table-stakes list. Per-hop `file:line` of the `dependency` block (position already in graph JSON `edges[].position`) lets users jump to the link. Optional. |
| **`report --blast` answered from the warm index on every save** | Stateless tools recompute worktrees and parse twice per call (Terragrunt builds git worktrees). Daemon answers in memory | MEDIUM | This is the milestone's defensible position. Property test: `report --blast` == `blast --base <copy of the tree at baseline time>` byte for byte, extending the DAEMON-02 rapid stateful property. |
| **`watch --base <dir>`: baseline from a directory at startup** | Gives the "compare against main" workflow every git-based tool has, without running git (user does `git worktree add`) | LOW-MEDIUM | Daemon indexes the base tree once with the existing blast code path. Cheap because `--base` plumbing exists. Priority P2: only if `rebase` + startup baseline is judged insufficient after dogfooding. |
| **Output sensitivity change as an informational detail** | Real signal for security review: `sensitive` true -> false exposes values in plan output | LOW | Only literal `true`/`false` on both sides; non-literal = unknown = silent. Never Broken. Terragrunt reads dependencies with `terraform output -json`, which prints sensitive values in plain text (HashiCorp outputs doc), so sensitivity flips do not break Terragrunt wiring. |
| **`blast --format sarif` (Broken findings only)** | GRT004 exists only in blast; without SARIF it cannot become a code-scanning annotation on the PR | MEDIUM | Needs GRT004 in SARIF rules, relaxing the v0.3 "sarif is a usage error for blast". Caveat already documented for SARIF: no `partialFingerprints`, byte columns. P2. |
| **Rebase safeguard summary** | Prevents the "rebased away my breakage" footgun | LOW | Already folded into `rebase` output above. |
| **Required-variable escalation only when certain** | Would turn "+required variable" into Broken when the unit's `inputs` is a literal object lacking the key with no `extra_arguments`, include merge or var-files | HIGH | This is GRT005 (deferred, overlaps `terragrunt hcl validate --inputs`, include-merge false-positive risk). Listed so nobody smuggles it into v0.4 through "type-level changes". |
| **Datum-gated propagation** (propagate only over edges with a recorded `dependency.X.outputs.Y` reference) | Fewer Impacted units, Terramate-style "data edges only" taken one step further | MEDIUM-HIGH | Unsafe today: whole-object use (`dependency.vpc.outputs`, `merge()`), lazily evaluated positions and `for` bodies are not recorded as named references, so gating would silently drop real consumers. Do not ship until reference extraction reports "uses outputs object whole" as a fact. |
| **Possible-rename hint on GRT004** (list outputs the module declares now) | Fix is usually a rename | LOW | Only as a plain fact ("module now declares: a, b"), never a computed best guess. |

### Anti-Features (Commonly Requested, Often Problematic)

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| **Baseline from git HEAD / merge-base** inside the daemon | Every comparable tool does it | Needs git objects: either an external process (breaks the no-exec proof) or a pack/loose-object reader (large, new attack surface, parses untrusted data). Already rejected in Key Decisions (v0.3) | `--base <dir>` (existing) and explicit `rebase`; `git worktree` stays the user's job |
| **Auto-rebase** (on clean state, on HEAD change, on timer, on every save) | "Why is it still showing last week's changes?" | Output then depends on history, not on two trees; breaks determinism, defeats the equivalence test, and silently hides unresolved breakage | Explicit `rebase` with a discard summary; optional later hint (not action) when `.git/HEAD` changed |
| **Named-cursor semantics** (each `report --blast` advances the baseline) | Watchman's convenience cursors | Watchman itself documents these cannot be rolled back; makes `report` mutating and two consecutive calls differ | `report` stays read-only; only `rebase` mutates |
| **Persisting the baseline to disk** | Survive daemon restarts | Violates the "no on-disk cache" decision (versioning, invalidation, corruption) and writes state that outlives the process | Documented: restart = new baseline = startup index. `watch --base <dir>` covers the durable case |
| **Output "type changed" detection** | Milestone bullet says "output type" | Terraform `output` blocks have no `type` argument; the type is inferred from evaluating `value` (resource attributes, function results), which structural decode cannot do. Flagging it would be guessing | Correct the milestone wording: outputs get name removal (GRT004), sensitivity flip, nothing else. Revisit only if Terragrunt's `outputs { type }` (RFC #6111) ships: then declared types become structural facts |
| **Breaking/compatible verdict on variable type changes** | oasdiff/buf grade changes ERR/WARN | Terraform converts values implicitly (number/bool -> string, list <-> set <-> tuple, map -> object with extra attributes dropped) and the consumer's actual values are expressions gruntled never evaluates. `number -> string` accepts everything; `string -> number` breaks only for non-numeric values | Report the canonical old -> new as a fact inside Impacted; no Broken |
| **Type-change Broken from literal input mismatch** | "unit passes `"abc"` to a `number` variable" | That is GRT006 territory (deferred, overlaps `hcl validate --inputs`) | Defer with GRT005/006 |
| **Propagating over `dependencies { paths }` edges** | Terragrunt `...` includes them | Ordering-only; Terramate explicitly excludes ordering-only Terragrunt dependencies from change impact. Including them inflates the list | Block edges only; document the choice |
| **Propagating over edges with unknown `enabled`/`skip_outputs`** | Over-approximation feels safe | An Impacted false positive is the "twelve units because a comment changed" failure from Key Decisions | Silent on unknown (accept false negatives, document "lower bound") |
| **Whole-repo Impacted on shared include/root change** (Nx lockfile style) | "Everything reads root.hcl" | Not a surface change; v0.4 only models module surface | Out of scope; `--queue-include-units-reading` equivalent is a separate, later feature |
| **Rename-detection heuristics** (Levenshtein "did you mean") | Friendlier GRT004 | A wrong suggestion is worse than none; non-deterministic across tie cases if not carefully specified | Plain fact list of current outputs (differentiators) |
| **Streaming blast to `watch` stdout or changing the status-file line** | "I want to see breakage as I save" | watch stdout/status are consumed by prompts and tmux; the documented contract is "diagnostics only / one fixed-format line" | Pull model: `report --blast`; revisit status suffix after dogfooding |
| **`nullable`, `validation`, `default` value, `ephemeral` change tracking** | Completeness of "type-level" | `validation`/`default` are expressions (value-level); `nullable` and ephemeral semantics vary by Terraform/OpenTofu version (ephemeral output rules: LOW confidence, verify) | Defer; document as invisible like today |

---

## Feature Dependencies

```
GRT004 (blast context)
    ├──requires──> baseline module surface (existing: impact.SurfaceDiff / repograph.Surface)
    ├──requires──> GRT001 decision table (existing: analysis/grt001.go rows 1-5, mock rule)
    └──hooks into──> impact.Compute (replaces a new GRT001 with GRT004 before NewFindings matching)

Transitive Impacted + path
    ├──requires──> direct Impacted seeds (existing: SurfaceDiff + unit.Module())
    ├──requires──> edge facts in repograph (existing: kind, enabled, skip_outputs, target_state, position)
    └──enhances──> GRT004 consumers view (dependents of the unit whose output was removed)

Type-level surface changes
    ├──requires──> tfsurface reader extension (record `default` presence, canonical `type`, output `sensitive`)
    ├──requires──> repograph.Surface carrying per-name facts, not only names
    └──requires──> SurfaceChange + presenter detail kinds (text + JSON additive fields)

report --blast
    ├──requires──> daemon retaining the baseline snapshot (graph + diagnostics)
    ├──requires──> impact.Compute rendered by the existing blast presenter (byte identity with `blast --base`)
    └──requires──> report protocol extension (new request, version bump)

rebase
    └──requires──> report --blast (otherwise nothing observable) and a mutating socket request

report --blast / rebase on windows ──conflicts──> report-file transport (no live query)

watch --base <dir> ──enhances──> report --blast (alternative baseline source)
blast --format sarif ──requires──> GRT004 rule metadata in SARIF
Required-variable escalation to Broken ──conflicts──> v0.4 scope (it is GRT005)
```

### Dependency Notes

- **GRT004 is a blast-domain feature, not an analyzer.** It cannot run in `check` because "was present in the baseline" needs two trees. The cleanest seam is inside `impact.Compute` (or a function it calls) post-processing the new GRT001 findings against the baseline surface. `FindingKey` (code, unit, file, message) means a GRT001 -> GRT004 swap on an existing reference is a NEW Broken finding, which is correct.
- **GRT004 and the baseline-matching rule.** If base already had GRT001 for the same reference, GRT004 must not appear (output was already missing). If the reference is newly added in the same change that removes the output, GRT004 is still right (output existed in base, is gone now).
- **Transitive Impacted must run before the Broken filter for seeds**, and after it for output. Today `Compute` skips Broken units while collecting direct consumers; the seed set must be computed first, otherwise a Broken direct consumer would cut the chain.
- **Type-level changes need a model change, not just a reader change.** `Surface` stores only sorted name slices; `SurfaceChange` stores four name slices. Adding facts means per-name records with canonical type, `hasDefault`, `sensitive`. This is the widest-reaching refactor of the milestone (touches `tfsurface`, `repograph`, `graph --json` possibly, `impact`, presenters), so keep it to its own phase.
- **`report --blast` response size.** The daemon already caps one response at 64 MiB and ships every format at once (base64). Rendering blast for every index eagerly would grow it; render blast on request from the retained snapshot instead.
- **Windows.** `report` reads a report file because there is no socket (no `net`). `rebase` cannot be a live request there. Options: unsupported with a clear exit 3 message (recommended), or a control-file protocol (extra race surface). `report --blast` on windows would need blast pre-rendered into the report file.

---

## MVP Definition

### Launch With (v0.4)

Ordered by dependency and by how much each can be proven with `blast --base` before any daemon work.

- [ ] **GRT004 in `blast`** (consumer-located, supersedes GRT001, grouped consumer list in presenter) - smallest, pure domain, closes MORE-03
- [ ] **Transitive Impacted with path and `--depth`** - pure domain + presenter; unblocks the headline of the milestone
- [ ] **Type-level facts: required-variable, variable type old -> new, (cheap) output sensitivity** - isolated refactor of surface model; ship as Impacted detail only
- [ ] **Daemon baseline, `report --blast`, `rebase`** - last, because it composes the three above through the same `impact.Compute`, so the equivalence test (`report --blast` == `blast --base <baseline copy>`) covers everything at once

### Add After Validation (v0.4.x)

- [ ] `watch --base <dir>` - trigger: users complain the startup baseline is wrong when the daemon starts on a dirty tree
- [ ] `blast --format sarif` - trigger: someone wants GRT004 as a PR annotation
- [ ] Hint when `.git/HEAD` changed since baseline (hint, not action) - trigger: confusing huge blast after a branch switch
- [ ] Per-hop `file:line` in path - trigger: paths longer than 2-3 hops get hard to follow

### Future Consideration (v0.5+)

- [ ] Reference-gated propagation, requires "whole-object use" fact in reference extraction
- [ ] Required-variable escalation to Broken (GRT005) and input type mismatch (GRT006)
- [ ] Output types from Terragrunt `outputs { type }` if RFC #6111 ships
- [ ] Windows `rebase` via control file, if windows users of `report --blast` exist
- [ ] `nullable` / `validation` / `ephemeral` change tracking

---

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| GRT004 (blast) | HIGH | MEDIUM | P1 |
| Transitive Impacted + canonical path | HIGH | MEDIUM | P1 |
| `--depth` direct-only switch | MEDIUM | LOW | P1 |
| Variable required-ness fact | HIGH | LOW-MEDIUM | P1 |
| Variable type old -> new fact | MEDIUM | MEDIUM | P1 |
| `report --blast` from daemon baseline | HIGH | MEDIUM | P1 |
| `rebase` with discard summary | HIGH | MEDIUM | P1 |
| Output sensitivity flip (informational) | LOW-MEDIUM | LOW | P2 |
| `watch --base <dir>` | MEDIUM | LOW-MEDIUM | P2 |
| `blast --format sarif` | MEDIUM | MEDIUM | P2 |
| Per-hop position in path | LOW | LOW | P2 |
| Possible-rename list on GRT004 | LOW | LOW | P3 |
| Reference-gated propagation | MEDIUM | MEDIUM-HIGH | P3 |
| Windows `rebase` | LOW | HIGH | P3 |
| GRT005/006 escalation | MEDIUM | HIGH | out of v0.4 |

---

## Behavioural Specification Summary (for requirements writing)

**Impacted, transitive**

- Seed: unit whose resolved module has a changed surface (names, required-ness, variable type, output sensitivity) between baseline and current. Same as v0.3 plus new change kinds.
- Edge: current-graph `dependency` block edge, `enabled` literally true, `skip_outputs` literally false, target has config. `paths` edges and `unknown` tristates are not edges for impact.
- Direction: A is a dependent of B if A has such an edge to B. Impact flows B -> A.
- Result entry: unit, `distance` (1 = uses the changed module itself), `path` (unit first, seed last), `module` and the change detail of the seed's module. Direct beats transitive; shortest beats longer; ties: lexicographically smallest path.
- Disjointness: Broken subjects never listed as Impacted, but may be traversed.
- Propagation does NOT require the intermediate unit's own surface to change. It is a lower bound on risk: config-unknown units, whole-object/lazy output uses and edges with unknown facts are silently not followed.
- Impacted never changes the exit code.

**GRT004**

- Code `GRT004`, severity error, `blast`/`report --blast` only, one per reference, anchored where GRT001 would be, unit = consumer.
- Supersedes (does not accompany) the GRT001 for that reference; `check` unchanged.
- Silent whenever the baseline surface of that module was unknown, the module path differs between trees, the unit is unknown, or any DIAG-03 silence applies.

**Baseline lifecycle**

- Created when the first complete index is published; never advanced implicitly; lost on daemon exit.
- `rebase` copies the last published index; prints counts discarded; bumps a counter shown in the label.
- Label contains no timestamp (determinism); the JSON may carry the counter.
- If the working tree was already dirty at startup, the baseline includes that dirt: document as "since daemon start", not "since commit".

**Type-level**

- Required-ness: presence of `default` argument. Type: canonical type-constraint string, absent == `any`. Output: literal `sensitive`.
- All three are facts inside Impacted; none produce Broken. Anything non-literal or unparseable is unknown and silent.

---

## Competitor Feature Analysis

| Feature | Terramate / Terragrunt filter / Pants / Nx | Bazel | Our Approach |
|---------|--------------------------------------------|-------|--------------|
| Baseline | git ref, recomputed per call | none | in-memory snapshot (daemon), directory (`--base`), explicit `rebase` |
| Change granularity | file / unit file | n/a | module surface (names, required-ness, type, sensitivity) |
| Dependents | transitive, flag or syntax to choose depth | `rdeps` with depth | transitive over data edges, `--depth` |
| Order-only edges | Terramate excludes; Terragrunt `...` includes | n/a | excluded |
| Reason shown | none (Terramate), subgraph (Nx) | `somepath` arbitrary | one canonical shortest path |
| Removed output still used | not detected | not applicable | GRT004, error |
| Type-level contract | not modelled | not applicable | facts, no verdict |

## Sources

- Terramate change detection: https://terramate.io/docs/cli/change-detection/ (HIGH)
- Terramate `list --changed` reason (none documented): https://terramate.io/docs/cli/cmdline/run (MEDIUM, negative finding)
- Terragrunt run / queue flags: https://docs.terragrunt.com/reference/cli/commands/run (HIGH)
- Terragrunt filter, graph, git: https://docs.terragrunt.com/features/filter, /features/filter/graph/, /features/filter/git/ (HIGH)
- Terragrunt `outputs` block RFC: https://github.com/gruntwork-io/terragrunt/issues/6111 (HIGH on facts)
- Terragrunt validate and dependency outputs: https://github.com/gruntwork-io/terragrunt/issues/5811 (HIGH)
- Pants target selection: https://www.pantsbuild.org/stable/docs/using-pants/advanced-target-selection (HIGH)
- Nx affected: https://nx.dev/docs/features/ci-features/affected (HIGH)
- Bazel query language: https://bazel.build/query/language (HIGH)
- Atlantis autoplanning: https://www.runatlantis.io/docs/autoplanning.html (HIGH)
- Spacelift stack dependencies: https://docs.spacelift.io/concepts/stack/stack-dependencies (HIGH)
- Digger Terragrunt dependencies: https://github.com/diggerhq/digger/issues/402 and https://docs.digger.dev/ce/howto/include-exclude-patterns (MEDIUM)
- buf breaking: https://buf.build/docs/breaking/ (HIGH)
- oasdiff: https://oasdiff.com/ (HIGH)
- Watchman clockspec: https://facebook.github.io/watchman/docs/clockspec (HIGH, used as analogy)
- Terraform type constraints and conversion: https://developer.hashicorp.com/terraform/language/expressions/type-constraints (HIGH)
- Terraform input variables: https://developer.hashicorp.com/terraform/language/values/variables (HIGH)
- Terraform outputs (sensitive shown in plain text by `-json`/`-raw`): https://developer.hashicorp.com/terraform/language/values/outputs (HIGH)
- Terraform module versioning conventions (removed variable / renamed output = major): https://spacelift.io/blog/terraform-module-versioning and web search results (MEDIUM)
- Local code read: `internal/domain/impact/impact.go`, `internal/domain/repograph/surface.go`, `internal/infrastructure/tfsurface/reader.go`, `docs/cli.md`, `.planning/PROJECT.md`

## Open points for phase-level research

- Confirm `hcl/v2/ext/typeexpr` API (`TypeConstraint`, `TypeConstraintWithDefaults`) and that it never evaluates; confirm the vendored hcl version includes `optional()` support. (MEDIUM now, from prior knowledge.)
- Confirm that an `output` block has no `type` argument in current Terraform and OpenTofu, and the current rules for `ephemeral` outputs (LOW confidence on the latter).
- How `tfsurface` treats `override.tf` / `_override.tf` and the `.tf`+`.tofu` union for type/default facts: a variable declared twice with different facts must become unknown.
- Daemon protocol: exact shape of the first mutating request, version bump, and how the 64 MiB cap interacts with an on-demand blast render.

---
*Feature research for: blast-aware diagnostics in a Terragrunt static analyzer*
*Researched: 2026-10-10*
