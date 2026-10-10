---
phase: 14-transitive-impact-with-paths
type: research
created: 2026-10-10
sources:
  - .planning/research/SUMMARY.md (Reconciled Conflicts #3 transitive edge rule, binding)
  - .planning/research/ARCHITECTURE.md (Q5 BFS), PITFALLS.md (8, 9, 14, 18)
  - .planning/REQUIREMENTS.md (Terms, BLAST-03, BLAST-04, BLAST-05; BLAST-09/11 constraints from Phase 15)
  - agent bus #109 (Terms), #110 (depth-1 equivalence wording), #111 (tie-break = smallest predecessor), Phase 13 lessons (#158, #175)
---

# Phase 14 — Research (short)

Design is fixed by milestone research and the requirements. This file records only what the code at
HEAD `8a6dd48` (Phase 13 shipped) adds: exact integration points, the edge facts as they really
are, the tests that change on purpose, and the decisions the requirements leave open.

## 1. Integration points (file:line at 8a6dd48)

| Where | What is there | Phase 14 change |
|-------|---------------|-----------------|
| `internal/domain/impact/impact.go:64-68` | `ImpactedUnit{Unit, Change}` — "a direct consumer" | gains `Reach{Distance, Source, Via}` |
| `impact.go:56-62` | `BrokenUnit{Subject, Findings}` | gains `Reach` (zero = not reached by propagation; set when a Broken unit is traversed, so paths through it can be rebuilt) |
| `impact.go:70-78` | `Result{Baseline, Broken, Impacted}` | gains `Changes []SurfaceChange` (the `SurfaceDiff` output, sorted, never nil) so presenters render each module's change once (BLAST-11 shape) |
| `impact.go:92-134` | `Compute`: seeds = cur units whose `u.Module()` is a changed module, minus Broken, sorted | seeds computed before the Broken filter, then `propagate` (new `transitive.go`); Broken units traversed, not listed. Signature unchanged |
| `impact.go:136-140` | `NoBaseline` | `Changes: []SurfaceChange{}`; Reach zero |
| `internal/domain/impact/surface.go:27-33` | `SurfaceDiff` sorted by Module, never nil | unchanged; reused for `Result.Changes` |
| `internal/domain/repograph/graph.go:240-260, 262-304, 306-...` | `EdgeKind` (Block/Paths), `Edge{kind, from, to, pos, enabled, name, state, skipOutputs}`, `Edges()` deterministic, skips unresolved | read-only use. Paths edges report `Enabled=True, SkipOutputs=False`, so the rule MUST also test `Kind()==EdgeBlock` |
| `repograph/options.go:163-190` | `DependencyOptions`; `DefaultDependencyOptions` = Enabled True, SkipOutputs False when absent | "literally true or absent" == `Enabled()==TristateTrue`; "literally false or absent" == `SkipOutputs()==TristateFalse`; `TristateUnknown` (non-literal) stops propagation |
| `internal/application/blasting/blasting.go:77-87` | `Between` → `impact.Compute` | unchanged (transitive lives in Compute, so blast --base and the Phase 16 daemon agree by construction) |
| `internal/interfaces/presenter/blast.go:13-15` | `blastSchemaVersion = 1`, JSON key `"version"` | becomes 2, once for the milestone |
| `blast.go:26-62` | `blastDoc`, `blastBroken`, `blastImpacted{unit, module, 4 name lists}` | additive fields (section 4) |
| `blast.go:64-149` | `BlastText`, `writeChangeTokens`; every repo string via `escapeTerm` | new tokens `distance N`, `path a -> b -> ...` with elision; hops via `escapeTerm` |
| `blast.go:151-197` | `BlastJSON`, `escapeJSON` post-pass at 195 | unchanged post-pass covers new fields |
| `cmd/gruntled/main.go:64-87, 311-372` | `blastUsage`, `runBlast` (`parseArgs` 167, `blasting.Blast` 349, exit 1 iff `HasErrors` 368) | `--depth N` flag, usage text, `res.WithMaxDistance(n)` before presenting |
| `docs/cli.md` blast intro, usage mirror (pinned verbatim by `TestRuleRegistryDoc`), Blast text/JSON, Known limitations ("one hop" bullet) | | rewritten; `TestHelpMatchesDocs` pins the exit-code lines, so the exit 2 line change must land in docs in the same plan |
| `internal/testsupport/synthrepo/render.go:79-170` | each unit is its own module (main.tf in the unit dir); unit i depends on min(fanout, i) lower units, all default options | a synthrepo tree with one module's output removed gives a known reverse-reachability cone for an e2e/size test |

## 2. Tests that change on purpose

The edge rule makes these existing expectations wrong; every other diff is a bug.

| File | Today | After |
|------|-------|-------|
| `cmd/gruntled/testdata/script/blast_impacted.txtar` | `! stdout 'live/edge'` ("consumers of consumers are not Impacted") | live/edge (dependency "db" -> live/db, default options) Impacted at distance 2 via live/db; live/other still not Impacted |
| `cmd/gruntled/testdata/script/blast_grt004.txtar` case 2 (unreferenced output removed) | `Impacted (1)`: live/db | live/app (depends on live/db) also Impacted, distance 2 |
| `internal/domain/impact/impact_test.go:106` `TestCompute` | live/web (dep "app" -> live/app) never Impacted | live/web Impacted at distance 2 via live/app |
| `cmd/gruntled/escape_test.go` blast subtest | `  v\u202ep (module v\u202ep: -output old)` | new text shape (distance token); consumers of the crafted producer reached at distance 2 with escaped hops |
| presenter `blast_test.go` goldens (TestBlastTextGolden, TestBlastJSONGolden) | v1 shapes | v2 shapes |

`TestDisjoint` (impact_test.go:201) builds units without dependencies: unchanged.
Phase 13's `TestBetweenOnlyReclassifiesGRT001` compares `Between` with `impact.Compute`, both
transitive now: unchanged. `scripts/compare-ref.sh` compares check/graph only: must stay N/N.

## 3. Algorithm facts (BLAST-03/04)

- Reverse adjacency built once from `curG.Edges()`: keep `e` iff `Kind()==EdgeBlock &&
  Enabled()==TristateTrue && SkipOutputs()==TristateFalse && curG.Unit(e.To())` exists (the target
  is a unit of the current graph); `dependents[e.To()] += e.From()`. Paths edges, unresolved
  dependencies (not in `Edges()`) and edges to non-units never propagate.
- Seeds: every cur unit whose module is in `SurfaceDiff`, Broken or not, distance 1, `Source` =
  itself, `Via` zero. Multi-source level-synchronous BFS: level sorted by `RepoPath.Compare`, each
  unit's dependents visited in sorted order, first discovery wins. Because the level is sorted, a
  unit's `Via` is its smallest predecessor at the previous level; by induction the path read from the
  unit back to its seed is the smallest under `RepoPath.Compare` at the first differing hop (sec
  #111). `Source` is inherited from `Via`. Iterative with a visited set: cycles, self-loops (GRT003
  shapes) and diamonds terminate, each unit appears once at its minimum distance.
- `Change` of a reached unit = the change of its `Source`'s module (a unit has one module, so a
  seed has one change).
- Broken units are visited (they carry `Reach` on `BrokenUnit`) but never listed in Impacted:
  Broken/Impacted stay disjoint and sorted. File-level Broken subjects are never reached.
- `--depth N` == filtering `Distance <= N` after an unlimited BFS (BFS levels are exact minimum
  distances and every `Via` has a smaller distance), so a pure `Result.WithMaxDistance(n)` is
  equivalent to limiting propagation and keeps `Between` depth-free for the daemon (Phase 16
  rejects `--depth` on `report --blast`).

## 4. Output shape settled here (BLAST-04 now, BLAST-11 in Phase 15)

JSON `version` 2 (the existing key; see D-14-01), additive only:
- top-level `changes[]`: one per changed module `{module, added_variables, removed_variables,
  added_outputs, removed_outputs}`, sorted by module. Phase 15 adds type-level reasons here (once
  per module).
- `impacted[]` entry: v1 keys `unit, module` + new `distance`, `source`, `via` (omitted at distance
  1). The four v1 name lists are present on distance-1 entries (identical bytes to v1 for those
  entries) and omitted on distance >= 2 entries (the change is looked up in `changes[]` by
  `module`), so a transitive entry is O(1) bytes and the document is O(units + surface text).
- `broken[]` entry: optional `distance`, `source`, `via` when the Broken unit was reached, so a
  consumer can follow `via` through it.

Text: distance-1 line `  <unit> (distance 1, module <m>: <tokens>)` (v0.3 line plus one token);
distance >= 2 `  <unit> (distance <d>, from module <m>, path <unit> -> <hop> -> ... -> <source>)` (sec #195: impacted[] `module` is the changed module that reached the unit; for distance 1 it is also the unit's own module; readers must check `version`). A
path of more than 6 units prints its first 4, then `... (<k> more) ->`, then the source:
O(1) per line, deterministic. Every hop via `escapeTerm`.

## 5. Phase 13 lessons applied

- `internal/domain` and `internal/application` tests may import only stdlib + `testing`/`reflect`
  (check-architecture.sh:270-297): no `rapid`, no `math/rand`. Property loops use an in-test
  xorshift (as impact_test.go `lcg`) or exhaustive enumeration of small graphs.
- The BLAST-04 oracle must not share code with `propagate`: it enumerates every simple path over the
  reverse edge set it builds itself from the raw generated edge list (DFS, no BFS), takes the
  minimum length, then the lexicographically smallest unit sequence.
- txtar: never use `$WORK` inside a stdout/stderr regexp (windows backslashes); assert
  `! stdout '^/'` / relative paths instead (commit f60d6b2).

## 6. Decisions (to confirm)

| ID | Decision | Why |
|----|----------|-----|
| D-14-01 | JSON keeps the key `"version"` and sets it to 2. REQUIREMENTS/ROADMAP say "`schema_version` 2"; read as "schema version 2". | Renaming the key is not additive and breaks every v1 reader's version check. |
| D-14-02 | Name lists only on distance-1 Impacted entries; distance >= 2 entries omit them and reference `changes[]` by `module`. | Keeps v1 entries byte-equal in content and makes transitive entries O(1) (BLAST-11 linear shape settled now). |
| D-14-03 | `BrokenUnit` carries `Reach`; JSON broken entries get optional `distance/source/via`. | A shortest path may run through a Broken unit (traversed, not listed); without it `via` chains dangle. |
| D-14-04 | Text path cap: 6 units; longer paths print the first 4, `... (<k> more) ->`, the source. | Fixed, deterministic, linear; full path always reconstructible from JSON. |
| D-14-05 | `--depth` implemented as `Result.WithMaxDistance` after unlimited propagation; `--depth` given as 0, negative, non-integer or empty is exit 2; absent = unlimited. | Exact equivalence (section 3); Between stays depth-free for the daemon. |
| D-14-06 | Seeds include module-unknown units? No: a seed must have a resolved module equal to a changed module (as v0.3). A module-unknown or config-unknown unit can still be reached as a dependent (it is a unit with edges; config-unknown units have none). | Same seed rule as v0.3; reaching an unknown-module dependent is a fact about its edge, not its module. |
