---
phase: 14-transitive-impact-with-paths
plan: 01
subsystem: impact
tags: [blast, transitive, bfs, oracle, determinism]
requires:
  - phase: 09
    provides: impact.Compute, SurfaceDiff, NewFindings
  - phase: 13
    provides: blasting.Between (unchanged; reaches the transitive Compute)
provides:
  - impact.Reach and propagate (restricted reverse-edge BFS with canonical predecessor)
  - ImpactedUnit.Reach, BrokenUnit.Reach, Result.Changes, Result.WithMaxDistance
  - independent brute-force path oracle (exhaustive and xorshift)
affects: [14-02, 14-03, 16]
tech-stack:
  added: []
  patterns: [level-synchronous BFS with sorted frontier for canonical shortest paths; brute-force simple-path oracle as test]
key-files:
  created:
    - internal/domain/impact/transitive.go
    - internal/domain/impact/transitive_internal_test.go
    - internal/domain/impact/oracle_test.go
  modified:
    - internal/domain/impact/impact.go
    - internal/domain/impact/impact_test.go
    - cmd/gruntled/testdata/script/blast_impacted.txtar
    - cmd/gruntled/testdata/script/blast_grt004.txtar
key-decisions:
  - "Exhaustive oracle uses the plan's fallback: every 5-unit graph with up to 4 edges was 473,556 graphs and about 7.2 s, over the 3 s budget. It checks all 4-unit graphs with up to 3 edges and every seed set, plus every 5-unit 4-edge DAG with every two-unit seed set (40,955 graphs, 0.7 s), which contains the sec 111 counterexample."
  - "Half of the random cases are layered (edges only from layer L+1 to layer L, seeds in layer 0); plain random graphs almost never separate the two path readings (counter 0-4 of 3000)"
  - "Compute takes a reached unit's Change from its Source's module through a seed-to-module map built with the seeds; Broken units get their Reach in place"
  - "blastUsage and docs/cli.md still say only direct consumers are listed; the wording belongs to 14-02 (CLI, presenter and docs)"
requirements-completed: []
duration: 25min
completed: 2026-10-10
---

# Phase 14 Plan 01: Transitive Impact in the Domain Summary

**Impacted is now transitive. Propagation runs a restricted reverse-edge BFS from the units that instantiate a changed module. It follows only dependency blocks whose enabled is literally true or absent and whose skip_outputs is literally false or absent, and only when the target is a unit. Each reached unit gets its minimum distance, its source and its smallest predecessor. A depth filter and `Result.Changes` complete the domain side. An independent brute-force path oracle agrees with it on 40,955 exhaustive graphs and 3,000 generated ones.**

## Performance

- Duration: ~25 min
- Tasks: 3
- Files: 3 created, 4 modified

## Accomplishments

- transitive.go:
  - `Reach` (:13).
  - `propagates` (:25): Kind Block (:26), Enabled True and SkipOutputs False (:29), target is a unit of the graph (:32).
  - `dependents` (:38) builds the reverse adjacency once, sorted and deduplicated.
  - `propagate` (:65): multi-source level-synchronous BFS. A unit is visited only once (:79), and the next frontier is sorted (:86).
  - Doc comments state the tie-break property: the path read from a unit back to its source is the smallest at the first differing hop.
- impact.go:
  - `BrokenUnit.Reach`, `ImpactedUnit.Reach`, `Result.Changes`.
  - `WithMaxDistance` (:105). It panics on n < 1 (:107), builds new slices and zeroes Broken Reach beyond n.
  - `Compute` (:133) takes seeds before the Broken filter, calls `propagate` (:154), gives Broken units their Reach (:159), excludes them from Impacted (:163), and takes each Change from the Source's module (:166).
  - `NoBaseline` sets `Changes: []SurfaceChange{}` (:177).
  - The doc comments no longer say "one hop".
- Intended test updates:
  - TestCompute: live/web is now Impacted at distance 2 via live/app, and Broken live/db carries Reach 1. Changes equals SurfaceDiff.
  - blast_impacted.txtar: `Impacted (3)`, `stdout -count=1 '^  live/edge'` with a new comment, live/other still absent, JSON summary `"impacted": 3`.
  - blast_grt004.txtar case 2: `Impacted (2)` plus `stdout -count=1 '^  live/app'`.
  - No other script, golden or presenter test changed. escape_test.go needed no change: every dependent of the crafted producer is Broken.
- New tests:
  - transitive_internal_test.go: TestPropagatesEdgeRule (7 rows plus the unresolved case), Chain, Direction, CycleSelfLoopDiamond, TieBreak (the sec 111 graph: z via x from source b), OrderIndependent (50 xorshift permutations of units, edges and seeds, duplicates included), NoRecursion (20,000-unit chain).
  - impact_test.go: TestComputeBrokenTraversed, TestComputeTwoModules, TestComputeNoChangesNoPropagation, TestWithMaxDistance (depth 1 equals the v0.3 instantiating set; n=0 panics; the receiver is not mutated).
- oracle_test.go (package impact_test; shares no code with transitive.go):
  - The BLAST-03 rule (`oracleRule`) is written from the requirement text over the raw generated edges. It enumerates every simple path from a unit to a seed by DFS.
  - It keeps the shortest paths. Among those it takes the read-back-smallest path as the expected answer, and also computes the seed-first-smallest path.
  - `oCheck` compares Distance, Source and Via, walks the whole Via chain, and checks Impacted versus Broken and each Change's module, at depths 0 (unlimited) and 1 to 4.

### Oracle runs

```
TestTransitiveMatchesOracleExhaustive: 40955 graphs (3050 5-unit 4-edge DAGs); read-back-smallest != seed-first-smallest in 30
TestTransitiveMatchesOracleRandom: 3000 cases: cycle reached 486, tie-break mattered 768, blocked edge changed reachability 155, Broken unit on a shortest path 369, distance >= 4 8, read-back != seed-first 56
```

Each random case is built from two different shuffles of units and edges. The two Results must be deep-equal, and both must equal the oracle at every depth from 1 to 4. The "cycle reached" counter counts self-loops and two-unit cycles through reached units.

### Hand mutations (run, then reverted)

| Mutation | Failing tests |
|----------|---------------|
| (a) propagates ignores Kind (paths edges propagate) | TestPropagatesEdgeRule, TestTransitiveMatchesOracleRandom |
| (b) next frontier not sorted | TestPropagateTieBreak, TestTransitiveMatchesOracleExhaustive, TestTransitiveMatchesOracleRandom |
| (c) SkipOutputs Unknown propagates (`== TristateTrue` instead of `!= TristateFalse`) | TestPropagatesEdgeRule, TestTransitiveMatchesOracleRandom |
| (d) forward edges (`out[e.From()] += e.To()`) | TestPropagatesEdgeRule, Chain, Direction, CycleSelfLoopDiamond, TieBreak, OrderIndependent, NoRecursion, TestCompute, TestComputeBrokenTraversed, TestComputeTwoModules, TestWithMaxDistance, TestTransitiveMatchesOracleExhaustive |

## Task Commits

1. Task 1 red: `a46222d` test(14-01): edge rule, direction, cycles, tie-break and order independence of propagate
2. Task 1 green: `794fbcd` feat(14-01): propagate, the restricted reverse-edge BFS with canonical predecessor
3. Task 2 red: `a401741` test(14-01): transitive Compute with Reach, Changes and WithMaxDistance; blast scripts expect transitive consumers
4. Task 2 green: `93d2128` feat(14-01): transitive Compute with Reach, Result.Changes and WithMaxDistance
5. Task 3: `34df77c` test(14-01): brute-force path oracle for transitive Impacted, exhaustive and xorshift (test only; the code it checks landed in Tasks 1-2)

## Verification

Full gate, real output tail. The suite, vet, check-architecture, test-check-architecture and compare-ref v0.3.0 all pass. The last two diffs are the testdata changes against v0.3.0 and against 67c9ace (end of phase 13):

```
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	11.527s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.619s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.007s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.205s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	1.290s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	2.537s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.061s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.006s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.091s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	1.885s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	1.313s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.086s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.013s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.057s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.035s
VET-OK
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
all architecture self-tests passed
244/244 identical
 cmd/gruntled/testdata/script/blast_grt004.txtar   | 708 ++++++++++++++++++++++
 cmd/gruntled/testdata/script/blast_impacted.txtar |  15 +-
 2 files changed, 718 insertions(+), 5 deletions(-)
 cmd/gruntled/testdata/script/blast_grt004.txtar   |  6 ++++--
 cmd/gruntled/testdata/script/blast_impacted.txtar | 10 ++++++----
 2 files changed, 10 insertions(+), 6 deletions(-)
```

The diff against 67c9ace lists only the two intended scripts. check and graph output stay byte-identical to v0.3.0 (244/244).

`-race` was not run locally: there is no gcc and no cgo. CI runs it.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-14-01-1 | mitigated | transitive.go:26 (Kind Block), :29 (Enabled True, SkipOutputs False), :32 (target is a unit); TestPropagatesEdgeRule (transitive_internal_test.go:110); oracle rule from the requirement text (oracle_test.go `oracleRule`); mutations (a), (c) |
| T-14-01-2 | mitigated | transitive.go:65-89 (iterative loop), :79 (visited set); TestPropagateCycleSelfLoopDiamond (:187), TestPropagateNoRecursion (:289) |
| T-14-01-3 | mitigated | transitive.go:38-50 (sorted, deduplicated adjacency), :86 (sorted frontier); TestPropagateOrderIndependent (:258); oracle Random builds every case from two shuffles and requires deep-equal Results; mutation (b) |
| T-14-01-4 | mitigated | transitive.go:42 (dependents[e.To()] += e.From(): reverse edges only); TestPropagateDirection (:171); mutation (d) |
| T-14-01-5 | mitigated | impact.go:163 (Broken subjects never listed in Impacted), :159 (Reach recorded); TestComputeBrokenTraversed (impact_test.go:300), TestDisjoint unchanged; HasErrors still reads Broken only |
| T-14-01-6 | mitigated | Reach is three fixed fields per unit; each Change is the byModule value, which shares its slices with Changes (impact.go:166); no per-unit copies |

## Deviations from Plan

**1. [Plan fallback] Exhaustive enumeration size**
- The full 5-unit, up-to-4-edge enumeration ran in about 7.2 s. The plan names a fallback for anything over ~3 s, and I used it: 4-unit up to 3 edges plus every 5-unit 4-edge DAG with two seeds, 40,955 graphs in 0.7 s. It still separates the two path readings in 30 graphs.

**2. [Test design] Layered half in the random generator**
- With plain random graphs, the read-back versus seed-first counter was 0 to 4 out of 3000, which is too close to vacuous. Half of the cases are now layered DAG-like trees (seeds in layer 0, 6 to 12 units), which gives 56. The other half keeps every edge variant, non-unit targets, module-unknown units and cycles.

**3. [Scope note] Usage and docs wording**
- blastUsage (main.go:71) and docs/cli.md still say "only direct consumers are listed". The behaviour is now transitive. Plan 14-02 owns the CLI, presenter and docs, so I left them untouched.

## Known Risks / Unverified

- `-race` not run locally (no gcc).
- Until 14-02 lands, the blast text and JSON show transitive units in the v1 shape (`module <m>: <tokens>` with the change of the source's module), and nothing marks their distance.

## Self-Check: PASSED
