---
phase: 14-transitive-impact-with-paths
verified: 2026-10-10T20:42:20Z
status: passed
score: 5/5 must-haves verified
covered_files:
  - .planning/phases/14-transitive-impact-with-paths/14-01-PLAN.md
  - .planning/phases/14-transitive-impact-with-paths/14-01-SUMMARY.md
  - .planning/phases/14-transitive-impact-with-paths/14-02-PLAN.md
  - .planning/phases/14-transitive-impact-with-paths/14-02-SUMMARY.md
  - .planning/phases/14-transitive-impact-with-paths/14-03-PLAN.md
  - .planning/phases/14-transitive-impact-with-paths/14-03-SUMMARY.md
  - README.md
  - cmd/gruntled/blast_depth_test.go
  - cmd/gruntled/blast_transitive_test.go
  - cmd/gruntled/escape_test.go
  - cmd/gruntled/main.go
  - cmd/gruntled/rules_doc_test.go
  - cmd/gruntled/testdata/blast_v1/PROVENANCE
  - cmd/gruntled/testdata/script/blast_exitcodes.txtar
  - cmd/gruntled/testdata/script/blast_grt004.txtar
  - cmd/gruntled/testdata/script/blast_impacted.txtar
  - cmd/gruntled/testdata/script/blast_transitive.txtar
  - cmd/gruntled/testdata/script/usage.txtar
  - docs/cli.md
  - internal/domain/impact/impact.go
  - internal/domain/impact/impact_test.go
  - internal/domain/impact/oracle_test.go
  - internal/domain/impact/transitive.go
  - internal/domain/impact/transitive_internal_test.go
  - internal/interfaces/presenter/blast.go
  - internal/interfaces/presenter/blast_test.go
  - scripts/blast-snapshots.sh
  - scripts/compare-ref.sh
  - scripts/lib/txtar-extract.sh
  - scripts/test-txtar-extract.sh
covered_digest: "v3:sha256:a81a1c4de989890a2cbd4b04192cc14e7a588983942b31bcc58fcec713fee1ee"
behavior_unverified: 0
overrides_applied: 0
deferred:
  - truth: "The 5,000-unit size bound on blast output and memory (SC5 parenthesis)"
    addressed_in: "Phase 15"
    evidence: "ROADMAP Phase 14 SC5: 'the 5,000-unit size bound is proven in Phase 15 under BLAST-11'. Phase 14 ships the linear representation (distance, source, via; elided text path), measured linear here up to 2,000 units."
---

# Phase 14: Transitive Impact with Paths Verification Report

**Phase Goal:** User sees a surface change reach every unit that depends on an affected unit, directly or through a chain, and each impacted unit shows how far it is from the change and the path that connects it.
**Verified:** 2026-10-10T20:42:20Z, HEAD 4c0406d. The code is identical to c1dbf6c: `git diff --stat c1dbf6c HEAD -- . ':!.planning'` is empty. The three later commits (f0a2084, 00c8eea, 4c0406d) only plan Phase 15.
**Status:** passed
**Re-verification:** No. This is the initial verification.

## Observable Truths

| # | Truth (ROADMAP success criterion) | Status | Evidence |
|---|-----------------------------------|--------|----------|
| 1 | With units A, B and C, where B depends on A, C depends on B and A instantiates the changed module, `gruntled blast --base` lists A (distance 1), B (distance 2) and C (distance 3) as Impacted. Each has one shortest path, read from that unit back to the unit that instantiates the module. | VERIFIED | **Code:** `propagate` (transitive.go) runs a multi-source, level-synchronous BFS over reverse propagating edges. Seeds get `Reach{Distance: 1, Source: s}` and each discovery gets `Distance+1, Source, Via: u`. `Compute` finds the seeds among cur units whose resolved module is in `SurfaceDiff`. `BlastText`/`writePath` follow Via from the unit back to Source. **Live, on my own tree** (scratchpad `big1`, 33 units, HEAD binary from `go build -o`): `live/a (distance 1, module modules/m: -output gone)`, `live/b (distance 2, from module modules/m, path live/b -> live/a)`, `live/c (distance 3, from module modules/m, path live/c -> live/b -> live/a)`. JSON has `distance`, `source: live/a` and `via` set to `live/a` for b and `live/b` for c. A second changed module (`modules/m2`, `+variable y`) seeds `live/m2u`. `live/w` depends on both m2u and c, and is listed once, at distance 2 via m2u (module m2), not at distance 4 via c. A unit that is a seed and also a dependent (`live/a2`) stays at distance 1. **Tests:** `TestPropagateChain`, `TestCompute` and `blast_transitive.txtar` cases 1 and 7 PASS. |
| 2 | Only live `dependency` block edges propagate. `enabled = false`, `skip_outputs = true`, any non-literal value for either, and `dependencies { paths }` edges stop propagation. Broken units are traversed but listed only under Broken, so Broken and Impacted stay disjoint and sorted. | VERIFIED | **Code:** `propagates` requires `Kind()==EdgeBlock`, `Enabled()==TristateTrue`, `SkipOutputs()==TristateFalse` and a target that is a unit of the cur graph. `Compute` finds seeds before the Broken filter, stores the Reach on the `BrokenUnit` and skips Broken subjects when it builds Impacted. **Live (`big1`):** dependents of `live/a` with `enabled = false`, `skip_outputs = true`, `enabled = local.on`, `skip_outputs = local.sk` and `dependencies { paths = ["../a"] }` are all absent. So is `t_after_stop`, which sits behind the `enabled = false` edge, and so is a dependency on a non-unit dir. Explicit `enabled = true` and `skip_outputs = false` do propagate (distance 2). `live/br` reads the removed output (GRT004). It appears only under Broken, its JSON broken entry carries `distance 2, source live/a, via live/a`, and `live/br2` behind it is Impacted at distance 3 with `path live/br2 -> live/br -> live/a`. **Literal forms (`lit` tree):** `enabled = "true"`, `enabled = "false"`, `skip_outputs = "false"`, `enabled = null` and `skip_outputs = null` stop propagation. The constant forms `true && true`, `(true)` and `"${true}"` propagate. In the same tree, `check` raises GRT001 only through the `true && true` edge and stays silent through `"true"` and `null`, so propagation and GRT001 rows 2-3 read the same Tristate facts, as BLAST-03 requires. **Tests:** `TestPropagatesEdgeRule`, `TestComputeBrokenTraversed`, `TestDisjoint` and `blast_transitive.txtar` cases 2-3 PASS. **My mutations** (scratch worktree): paths edges propagating, unknown `enabled` propagating, Broken listed in Impacted, and Broken seeds not traversed each make at least one test FAIL (`TestPropagatesEdgeRule`, the oracle, `TestDisjoint`, `TestComputeBrokenTraversed`, `TestCompute`). |
| 3 | Cycles, self-loops and diamonds terminate. Each unit appears once, at its minimum distance, with the path that is smallest under `RepoPath.Compare` at the first differing hop, identical for any input or map order (proven against a brute-force oracle on random graphs with shuffled input). | VERIFIED | **Code:** the loop is iterative with a `reach` visited map. Each frontier is sorted, and the first discovery wins, so Via is the smallest predecessor one level closer. By induction the read-back path is lexicographically smallest at the first differing hop. **Live (`big1`):** cycle `cy1 <-> cy2` plus the self-loop `cy2 -> cy2` terminate, each unit is listed once (cy1 at 2, cy2 at 3). Diamond `dm -> {dz, db} -> a`, with the `zz` dependency block written first, gives `path live/dm -> live/db -> live/a`. The first-differing-hop rule beats a smaller source: `fx -> {fb -> zz, fc -> a}` gives `path live/fx -> live/fb -> live/zz`. **Order independence:** a creation-order shuffle cannot change the input order here, because ext4 readdir is hash-ordered and Go `fs.ReadDir` sorts entries. So I shuffled the order of the dependency blocks inside every unit file instead (seeds 11-14; for example `dm` had `zz,bb` and then `bb,zz`, and `cy2` had `one,self` and then `self,one`). Text and JSON bytes were identical across all shuffles and creation orders. **Tests (run by name):** `TestTransitiveMatchesOracleExhaustive` and `TestTransitiveMatchesOracleRandom` PASS. The oracle in oracle_test.go shares no code with transitive.go: it builds its own adjacency from the raw edge list and enumerates simple paths by DFS. The random test runs 3000 cases built with `rng.perm` of units and edges. The counters were cycle 486, tie-break 768, blocked edge 155, Broken on a shortest path 369, distance >= 4 8, and read-back != seed-first 56, and the test asserts each is > 0. `TestPropagateCycleSelfLoopDiamond`, `TestPropagateTieBreak`, `TestPropagateOrderIndependent` and `TestPropagateNoRecursion` (20,000-unit chain) PASS. **My mutation:** with the frontier sort removed, `TestPropagateTieBreak` and both oracle tests FAIL. Sorting dependents in descending order is an equivalent mutant and the tests pass, as they should: `next` is re-sorted and Via comes from frontier order. |
| 4 | `blast --depth N` limits propagation (default unlimited). `--depth 1` with only name-level changes yields the v0.3 Impacted set. `--depth 0`, a negative or a non-integer value exits 2. | VERIFIED | **Code:** `depthFlag` records an explicit value. `parseDepth` accepts `^[1-9][0-9]*$` that fits an int. `runBlast` calls `res.WithMaxDistance(maxDepth)` after `blasting.Blast`. **Live (`big1`):** `--depth 1/2/3/9/10/100` gives Impacted 5/15/21/27/27/27, with max distance 1/2/3/9/9/9. No `--depth` gives 27. `--depth=2` and a flag after the path behave the same. `0`, `-1`, `x`, `''`, `1.5`, `01`, `+1`, `' 1'`, `1e2`, `99999999999999999999` and a missing value all exit 2 with empty stdout and `invalid --depth "<v>" (want an integer >= 1)` (or `flag needs an argument`). Under `--depth 1`, the Broken entry's Reach (distance 2) is cleared from JSON. **Against a real v0.3.0 binary** (built from tag e870e29 in a scratch worktree): `ghead blast --depth 1` and `gv03 blast` on `big1` both exit 1. The text is byte-identical after removing the `distance 1, ` token and mapping the one GRT004 line to its v0.3.0 GRT001 form. In JSON the Impacted units, modules and the four name lists are equal, Broken units and per-unit counts are equal, the summary is equal, and every v0.3.0 key is present in v2 with the same JSON type. **Snapshots:** `scripts/blast-snapshots.sh 67c9ace… <tmp>` reproduces `cmd/gruntled/testdata/blast_v1` byte-for-byte (`diff -r` empty, 67 files incl. PROVENANCE). `blast-snapshots.sh v0.3.0` gives 20 pre-GRT004 cases. 18 are byte-identical to the committed snapshots, and the other 2 differ only by GRT001 → GRT004 on `blast_impacted base/cur` (the Phase 13 reclassification). `TestBlastDepth1MatchesV1` (22 subtests) and `TestBlastDepthFlag` PASS. **My mutations:** an off-by-one in WithMaxDistance, `--depth` ignored, and `0` accepted each fail `TestWithMaxDistance`, the oracle, `TestBlastDepthFlag`, `TestBlastDepth1MatchesV1` or `TestScripts`. |
| 5 | JSON carries `"version": 2` (existing key, D-14-01) with additive fields only: distance, source and `via` = predecessor, so size is linear in chain length. Text prints the full path up to a fixed hop count and elides the rest deterministically. The 5,000-unit size bound is proven in Phase 15 under BLAST-11. | VERIFIED | **Code:** `blastSchemaVersion = 2` under the key `version` (not `schema_version`). `blastDoc` adds `changes[]`, `blastImpacted` adds `distance`/`source`/`via` (via omitted at distance 1), and `blastBroken` adds optional reach. The four name lists appear only at distance 1. Text uses `textPathMaxUnits, textPathHead = 6, 4`. **Live:** `big1` JSON has keys `version, kind, baseline, broken, impacted, changes, summary` with `version: 2`. **Linear:** chains of 250/500/1000/2000 units give JSON 38,395/76,395/152,397/305,397 bytes (152-153 B per unit) and text 146-148 B per unit, with the longest text line 147-149 chars (it grows only with digit count). **Elision:** n6 prints all 6 units. n7, n8 and n9 print `n7 -> n6 -> n5 -> n4 -> ... (2 more) -> n1`, then 3 more and 4 more, so the elided count is distance − 5. **Escaping:** a unit directory named `b ESC ]0;pwn BEL U+202E U+009B 31m` prints in text as `\x1b]0;pwn\x07\u202e\u009b31m` on its own line and on every path hop, including elided lines, and as `\u001b…\u0007\u202e\u009b…` in JSON. There are 0 raw ESC/BEL/C1/RLO bytes in either output. **Tests:** `TestBlastJSONGolden`, `TestBlastJSONV1KeysKeepType`, `TestBlastJSONLinear`, `TestBlastTextPathElision`, `TestBlastTextLinear`, `TestTextOutputsEscapeControls` and `TestBlastTransitiveSynthrepo` (148 of 300 units reach `g0/unit-049`, deepest distance 9, matching the generator's own reverse reachability) PASS. **My mutations:** with the elision head set to 3, `TestBlastTextPathElision` FAILS. With a path hop not escaped, `TestTextOutputsEscapeControls` FAILS. |

**Score:** 5/5 truths verified (0 present-but-behavior-unverified)

### Deferred Items

| # | Item | Addressed In | Evidence |
|---|------|-------------|----------|
| 1 | The 5,000-unit RSS and output-size bound | Phase 15 | ROADMAP Phase 14 SC5 itself defers it ("proven in Phase 15 under BLAST-11"). The linear representation it bounds is in place and measured linear here. |

## Required Artifacts

| Artifact | Expected | Status | Details |
|----------|----------|--------|---------|
| `internal/domain/impact/transitive.go` | `Reach`, `propagates`, `dependents`, `propagate` | VERIFIED | 90 lines, iterative BFS, no recursion. Called from `Compute` (`propagate(curG, seeds)`) |
| `internal/domain/impact/impact.go` | `ImpactedUnit.Reach`, `BrokenUnit.Reach`, `Result.Changes`, `WithMaxDistance` | VERIFIED | `WithMaxDistance` is non-mutating and panics on n < 1. The CLI validates first, so `--depth 0` never reaches it |
| `internal/domain/impact/oracle_test.go` | independent brute-force oracle, exhaustive and xorshift | VERIFIED | 550 lines. It imports only the public `impact` API and has non-vacuity counters |
| `internal/interfaces/presenter/blast.go` | BlastText/BlastJSON v2 | VERIFIED | Every hop goes through `escapeTerm(hop.String())`, and JSON keeps the whole-buffer `escapeJSON` |
| `cmd/gruntled/main.go` | `--depth`, `blastUsage` | VERIFIED | Usage mirrored in docs/cli.md (`TestHelpMatchesDocs`, `TestRuleRegistryDoc`) |
| `cmd/gruntled/blast_depth_test.go`, `blast_transitive_test.go`, `testdata/script/blast_transitive.txtar` | BLAST-03/04/05 end to end | VERIFIED | PASS |
| `scripts/blast-snapshots.sh`, `scripts/lib/txtar-extract.sh`, `testdata/blast_v1/` | reproducible pre-phase snapshots | VERIFIED | Regenerated from the PROVENANCE hash with an empty `diff -r`. `test-txtar-extract.sh`: all cases passed |
| `docs/cli.md`, `README.md` | Blast text/JSON v2, `--depth`, lower bound in Known limitations | VERIFIED | Blast JSON table defines `module`/`source`/`via`. Stability: "a v1 reader must reject version 2". Known limitations: "`blast` Impacted is a lower bound: a non-literal `enabled` or `skip_outputs` and a `dependencies { paths }` entry stop propagation, and propagation is not gated on which outputs a dependent reads" |

## Key Link Verification

| From | To | Via | Status | Details |
|------|----|-----|--------|---------|
| `impact.Compute` | `propagate` | seeds before the Broken filter, Broken removed from the listed set | WIRED | `reach := propagate(curG, seeds)`. Live Broken `live/br` carries Reach and `br2` is reached through it |
| `BlastText` path rendering | `escapeTerm` | each hop | WIRED | `b.WriteString(escapeTerm(hop.String()))`. The mutation that removes it is caught |
| `runBlast` | `impact.Result.WithMaxDistance` | after `blasting.Blast` when `--depth` is set | WIRED | `res = res.WithMaxDistance(maxDepth)`. Live `--depth` counts follow it, and removing the call fails 3 tests |
| `blasting.Blast` → `Between` → `impact.Compute` | presenter | unchanged Phase 13 composition | WIRED | Live HEAD output carries both GRT004 and transitive Impacted |

## Data-Flow Trace (Level 4)

| Artifact | Data | Source | Real data | Status |
|----------|------|--------|-----------|--------|
| `blast --base` text/JSON | Impacted distance/source/via, changes[] | real HCL in both trees → loader edges (Kind/Enabled/SkipOutputs) → `checking.Check` ×2 → `Between` → `Compute`/`propagate` → presenter | yes. Hand-built trees give the predicted distances and paths, and the edge facts change the result | FLOWING |

## Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| SC1-SC3 on a hand-built tree | `ghead blast --base base cur` (big1: 33 units, chain, long chain, 5 stoppers, 2 literal propagators, Broken in the middle, cycle with a self-loop, diamond, first-hop tie, two modules) | Impacted (27), Broken (1) live/br, every distance and path as predicted, exit 1 | PASS |
| Order independence | same spec, 3 creation-order seeds and 4 dependency-block-order seeds | `cmp` identical text and JSON | PASS |
| Edge literal forms | `lit` tree (9 dependents of a) | only the constant-true forms propagate, matching `check` GRT001 | PASS |
| `--depth` | 6 valid values, 10 invalid values and a missing value | counts limited; every invalid value exits 2 with empty stdout | PASS |
| `--depth 1` vs v0.3.0 | `ghead --depth 1` vs `gv03` on big1, text and JSON | equal modulo the `distance 1, ` token, GRT001→GRT004, and additive v2 keys | PASS |
| Snapshot provenance | `blast-snapshots.sh 67c9ace… <tmp>` then `diff -r` | identical (67 files) | PASS |
| v0.3.0 snapshots | `blast-snapshots.sh v0.3.0 <tmp>` vs committed | 18/20 identical, 2 differ only by GRT001→GRT004 | PASS |
| Linear size / bounded lines | chains of 250 to 2000 | about 152 B per unit in JSON and about 147 B per unit in text, longest line at most 149 | PASS |
| Escaping | hostile unit dir name on a path | 0 raw control bytes in text and JSON | PASS |
| Full suite | `go test -count=1 ./...` | ok in all 20 packages | PASS |
| Vet / fmt | `go vet ./...`, `gofmt -l .` | clean | PASS |
| Architecture | `bash scripts/check-architecture.sh` | `architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)` | PASS |
| Architecture self-test | `bash scripts/test-check-architecture.sh` | CI `architecture` job (runs it) green at c1dbf6c. The local run was still copying repos when this report was written | PASS (CI) |
| check/graph byte gate | `bash scripts/compare-ref.sh v0.3.0` | `244/244 identical`, exit 0. `COMPARE_REF_PERTURB=1` gives `243/244`, exit 1 | PASS |
| txtar extraction | `bash scripts/test-txtar-extract.sh` | `all txtar-extract cases passed`, rc 0 | PASS |
| Per-task commands | the 8 `Automated Command`s of 14-VALIDATION.md | rc 0 each | PASS |
| Release build | `bash scripts/build-release.sh v0.0.0-ci 4c0406d <tmp>` | all archives OK, smoke prints the version | PASS |
| No new deps | `git diff --exit-code v0.3.0 -- go.mod go.sum` | exit 0 | PASS |
| CI | `gh run list --branch master --limit 1` → run 38083669867 (headSha c1dbf6c) | success: check (`go test -race`), architecture, recipe-check, sarif-upload, test-os macos and windows | PASS |

## Probe Execution

Not applicable. The phase declares no `scripts/*/tests/probe-*.sh`. The gate scripts (`compare-ref.sh`, `blast-snapshots.sh`, `test-txtar-extract.sh`) were run above.

## Requirements Coverage

| Requirement | Source Plans | Description | Status | Evidence |
|-------------|--------------|-------------|--------|----------|
| BLAST-03 | 14-01, 14-03 | Reverse dependency-block edges with literal true/absent enabled and literal false/absent skip_outputs, target a cur unit; non-literal stops (lower bound); paths edges never; cycles/self-loops/diamonds terminate; Broken traversed, not listed | SATISFIED | Truths 2, 3 |
| BLAST-04 | 14-01, 14-02, 14-03 | Distance 1 for seeds, 1+k for dependents; one canonical shortest path; order-independent, brute-force oracle; JSON version 2 additive | SATISFIED | Truths 1, 3, 5 |
| BLAST-05 | 14-01, 14-03 | `--depth N` limits; `--depth 1` equals v0.3; N integer >= 1 else exit 2; default unlimited | SATISFIED | Truth 4 |
| Terms (instantiating unit, dependent, seeds) | — | Seeds = cur units whose resolved module is a changed module | SATISFIED | `Compute` seed loop over `curG.Units()` with `u.Module()` in `byModule` |

No orphaned requirements. REQUIREMENTS.md maps only BLAST-03, BLAST-04 and BLAST-05 to Phase 14, and every plan declares them. Their checkboxes and traceability rows still read Pending, to be ticked at phase completion.

## Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| None | | No TBD/FIXME/TODO/HACK/placeholder in the 23 files changed in 67c9ace..c1dbf6c. The `XXX` matches (docs/cli.md:379, blast.go:248) are `\uXXXX` escape notation | | |

Test quality: the only skip is `escape_test.go:103` on Windows (NTFS forbids control characters in names). The test runs on Linux and macOS. The oracle is independent: it uses its own adjacency and path enumeration, and every mutation that changes semantics was caught. `TestBlastDepth1MatchesV1` compares against committed snapshots, not against HEAD output. The snapshots reproduce from 67c9ace, and I compared v0.3.0's own snapshots and a v0.3.0 binary separately.

Informational (non-blocking):
- `--depth 01` and `--depth +1` exit 2. That is stricter than "an integer >= 1", and both the code comment and the plan intend it (`^[1-9][0-9]*$`).
- An edge fact written as a constant expression (`true && true`, `(true)`, `"${true}"`) counts as literal and propagates, while `"true"` and `null` stop propagation. This is the GRT001 Tristate, as BLAST-03 requires, and `check` agrees on the same tree.
- The text path joins hops with ` -> `, which a directory name may itself contain. docs/cli.md says so and names JSON `source`/`via` as the unambiguous form.

## Human Verification Required

None. This is a CLI phase, and every success criterion was observed by running the binary on hand-built trees and by named tests. The VALIDATION manual items are both covered: `-race` is green in CI `check` at c1dbf6c, and the snapshot provenance was reproduced here (`diff -r` empty).

## Gaps Summary

None. `blast --base` now reaches dependents through live dependency blocks only. It gives each unit its minimum distance and the canonical read-back path, terminates on cycles, self-loops and diamonds, keeps Broken units out of Impacted while walking through them, honours `--depth`, and equals v0.3.0 at `--depth 1`. The JSON is version 2, additive and linear, and the text path is bounded and escaped. I confirmed all of this on my own trees and against a v0.3.0 binary, and the oracle and the mutations show the tests can fail. The only open item is the 5,000-unit RSS and size bound, which the roadmap assigns to Phase 15 (BLAST-11).

## Security

Verdict **SECURED** (14-SECURITY.md, gruntled-sec bus verdict #234): 17 threats, 16 mitigated, 1 accepted, 0 open. F1 (LOW, symlink escapes in txtar-extract) is fixed in 917c7aa and f08d9ad, and F2 (INFO, GNU realpath/sha256sum) is documented in 65a0b2b. Both files are in the covered set, and `test-txtar-extract.sh` passes all cases on them.

---

_Verified: 2026-10-10T20:42:20Z_
_Verifier: Claude (gsd-verifier)_
