---
phase: 14-transitive-impact-with-paths
plan: 03
subsystem: cli-blast
tags: [blast, depth, snapshots, txtar, provenance, docs, gate]
requires:
  - phase: 14-01
    provides: transitive Compute, Result.WithMaxDistance
  - phase: 14-02
    provides: blast text and JSON version 2
provides:
  - "blast --depth N with strict decimal validation"
  - "scripts/lib/txtar-extract.sh shared by compare-ref.sh and blast-snapshots.sh, with link checks on real paths"
  - "scripts/test-txtar-extract.sh negative tests (names, links, refs)"
  - "scripts/blast-snapshots.sh and cmd/gruntled/testdata/blast_v1 with PROVENANCE"
  - "blast_transitive.txtar, TestBlastTransitiveSynthrepo, TestBlastDepth1MatchesV1, TestBlastDepthFlag"
affects: [15, 16, 17]
tech-stack:
  added: []
  patterns: ["snapshots of a prior commit with hash and sha256 provenance, regenerated and diffed in the gate", "shell lib sourced through BASH_SOURCE before any cd"]
key-files:
  created:
    - cmd/gruntled/blast_depth_test.go
    - cmd/gruntled/blast_transitive_test.go
    - cmd/gruntled/testdata/script/blast_transitive.txtar
    - cmd/gruntled/testdata/blast_v1/PROVENANCE
    - scripts/blast-snapshots.sh
    - scripts/lib/txtar-extract.sh
    - scripts/test-txtar-extract.sh
  modified:
    - cmd/gruntled/main.go
    - cmd/gruntled/rules_doc_test.go
    - cmd/gruntled/testdata/script/blast_exitcodes.txtar
    - cmd/gruntled/testdata/script/usage.txtar
    - scripts/compare-ref.sh
    - docs/cli.md
    - README.md
key-decisions:
  - "--depth is a flag.Value that records whether it was set, so an explicit empty value is exit 2; parseDepth accepts only ^[1-9][0-9]*$ that fits an int"
  - "The txtar-extract baseline ran against the three functions extracted with sed from the pre-refactor compare-ref.sh, loaded through TXTAR_EXTRACT_LIB (sec #208 note 2); it was red on two symlink cases, which the lib now rejects"
  - "The lib is not a verbatim move: it re-checks each link from its real parent directory and resolves the created link with realpath -m (GNU coreutils, fails closed when missing)"
  - "blast-snapshots.sh extracts the fixture trees from the ref worktree, not HEAD (sec #208 note 3); snapshots come from 67c9ace9a09f46d5662acc1fffddb6d2057034e8, the parent of 14-01's first commit"
  - "The synthrepo seed unit is the one whose reverse cone is closest to half the tree (g0/unit-049, 148 of 300 units), so the exact-set check can fail in both directions"
requirements-completed: [BLAST-03, BLAST-04, BLAST-05]
duration: 35min
completed: 2026-10-10
---

# Phase 14 Plan 03: --depth, End-to-End Proof, Snapshots and Phase Gate Summary

**`blast --depth N` is available, with strict validation. Transitive Impacted is proven end to end on real HCL and on a 300-unit synthrepo tree. `--depth 1` matches the pre-Phase-14 binary on all 22 blast fixture pairs, through snapshots whose provenance is checked. The txtar extraction now lives in one hardened lib with negative tests. Usage, docs/cli.md and README no longer say "only direct consumers", and pinned tests enforce that.**

## Performance

- Duration: ~35 min
- Tasks: 3
- Files: 7 created (plus 66 snapshot files), 7 modified

## Accomplishments

- **main.go:**
  - `depthFlag` (:394-406) records whether `--depth` was set. `parseDepth` (:410) accepts only `^[1-9][0-9]*$` that fits an int.
  - An invalid value prints `gruntled: invalid --depth "<v>" (want an integer >= 1)` plus the usage and exits 2 (:331).
  - `res.WithMaxDistance(maxDepth)` runs after Blast (:372).
  - blastUsage: the "only direct consumers" sentence is replaced by the transitive edge rule, distance and path, and `--depth N stops at distance N`. It adds the `--depth N` flag line, the synopsis `[--depth N]`, and the exit-2 line `invalid --format or --depth`. docs/cli.md mirrors it verbatim.
- **blast_exitcodes.txtar:** eight new exit-2 cases: 0, -1, +2, 02, x, 1.5, '' and a missing value.
- **TestBlastDepthFlag:** a 4-unit chain run with no flag, then depth 1, 2 and 3; `--depth=3` before the path equals `--depth 3` after it; JSON at depth 2 has no distance above 2; without --base, --depth changes nothing.
- **blast_transitive.txtar:**
  - Case 1: chain distances 1/2/3 with paths.
  - Case 2: enabled=false, skip_outputs=true, non-literal enabled, non-literal skip_outputs and `dependencies { paths }` all block.
  - Case 3: Broken live/b is traversed and appears only under Broken; live/c is at distance 3 through it.
  - Case 4: a cycle plus a self-loop terminates and each unit appears once.
  - Case 5: a diamond goes through the smaller branch.
  - Case 6: `--depth 2`.
  - Case 7: JSON version 2, changes[] and the via chain.
  - Path-leak checks use `! stdout '^/'` and the drive-letter pattern; no `$WORK`.
- **TestBlastTransitiveSynthrepo:**
  - Synthrepo with 300 units, IncludeDepth 2 and fanout 3. The dependency list is parsed from the rendered `config_path` lines, not taken from gruntled.
  - The seed is the unit whose reverse cone is closest to 150; that is g0/unit-049, with a cone of 148 units and deepest distance 9. Base adds an output to it that nothing reads.
  - Checked: the exact Impacted set and BFS distances, each `via` being a dependency one level closer, and text under 300 bytes per unit (19,115 bytes).
- **scripts/lib/txtar-extract.sh:**
  - Contains `safe_rel` (:14), `inside_after_link` (:26) and `extract` (:48). Functions only.
  - The link check is hardened: the real parent must stay inside the tree (:92-93), and the created link is resolved with `realpath -m` (:108).
  - compare-ref.sh and blast-snapshots.sh source it through `BASH_SOURCE` before any `cd`. Both keep `set -euo pipefail`, their trap, the `rev-parse --verify --quiet --end-of-options` check and `GOFLAGS=-mod=readonly GOPROXY=off`.
- **scripts/test-txtar-extract.sh:**
  - 13 archive cases, each must be rejected with nothing written outside the target. The benign control must extract.
  - 3 hostile refs for each of the two scripts must be refused with "is not a commit", without a worktree being left behind.
  - A member name cannot hold a newline in txtar, so the newline case is a marker split over two lines combined with an unsafe member.
- **scripts/blast-snapshots.sh** (trap :33, GOFLAGS :35, ref check :37):
  - Builds `<ref>` offline and extracts the blast_impacted, blast_exitcodes and blast_grt004 trees from the ref worktree (:49).
  - Runs every `--base B P` pair whose two trees exist, writing `<case>.txt/.json/.exit`.
  - Writes PROVENANCE: the hash, then a sha256 line per file (:78-82).
- **blast_v1/:** 22 cases from `67c9ace9a09f46d5662acc1fffddb6d2057034e8`, the parent of 14-01's first commit a46222d.
- **TestBlastDepth1MatchesV1:**
  - Provenance first: the sha256 of every file matches and every file is listed. Non-vacuity: version 1, no distance/changes, no `distance ` in the text.
  - For each case: the exit code is equal; the text is equal after removing `distance 1, `; the JSON is equal after dropping version, changes and distance/source/via; Impacted without --depth is a superset.
  - blast_impacted base/cur: live/edge is absent from the snapshot and present without --depth.
- **docs/cli.md and README:**
  - The blast intro Impacted bullet uses the terms instantiating unit and dependent, and states the pinned edge-rule sentence, that Broken units are traversed, and --depth.
  - Known limitations swaps the one-hop bullet for the pinned lower-bound bullet (:1034).
  - The README blast paragraph mentions distance, path and --depth.
  - All of these are pinned in TestRuleRegistryDoc.

### txtar-extract baseline (sec #208 note 2)

Run before the move, against the functions taken with `sed` from the pre-refactor compare-ref.sh (`TXTAR_EXTRACT_LIB=<extracted> TXTAR_REF_SCRIPTS=compare-ref.sh`):

```
ok benign
ok dotdot-name
ok absolute-name
ok inner-dotdot-name
ok empty-name
ok newline-name
ok backslash-name
ok link-escapes
ok link-absolute
ok link-name-escapes
FAIL link-chain: hostile archive accepted
```

Run separately before the refactor, `link-through-symlink` (`d -> .`, `l -> d/../x`) was also accepted by the old lexical-only check. Every other case, including the compare-ref.sh ref cases, passed then. After the lib move (885f4f2), all cases pass for both scripts (gate section 3).

### Hand mutations (run, then reverted)

| Mutation | Fails |
|----------|-------|
| `res.WithMaxDistance(maxDepth)` removed from runBlast | TestBlastDepthFlag; TestBlastDepth1MatchesV1 (blast_grt004 base2/cur2, base6/cur6, cur1/base1, blast_impacted base/cur) |
| blast_v1 regenerated from HEAD | TestBlastDepth1MatchesV1: "snapshot JSON is not a version-1 document" |

## Task Commits

1. Task 1 red: `418a923` test(14-03): --depth limits Impacted and rejects anything but a decimal integer >= 1
2. Task 1 green: `cf6142c` feat(14-03): blast --depth N and usage text for transitive Impacted
3. Task 2 baseline tests: `53a3ab8` test(14-03): negative tests for txtar extraction and ref checks
4. Task 2 refactor: `885f4f2` refactor(14-03): txtar extraction moves to scripts/lib and re-checks links on real paths
5. Task 2 red: `bcff262` test(14-03): transitive blast end to end, synthrepo cone and --depth 1 against pre-phase-14 snapshots
6. Task 2 green: `f4fbeaf` feat(14-03): blast-snapshots.sh and the pre-phase-14 blast snapshots with PROVENANCE
7. Task 3 red: `ee928d6` test(14-03): pin the edge rule, the lower-bound limitation and the README blast paragraph
8. Task 3 green: `4273e30` docs(14-03): transitive Impacted and --depth in the blast intro, Known limitations and README

## Phase Gate

### 1. Suite, vet, architecture

```
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	15.771s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.852s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.008s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.005s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.014s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.364s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.011s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	1.757s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.014s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	3.466s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.101s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.015s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.171s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	2.523s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	1.769s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.180s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.097s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.083s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.054s
suite rc=0
VET-OK
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
all architecture self-tests passed
```

### 2. compare-ref.sh v0.3.0 and perturb self-test

```
244/244 identical
perturb rc=1
DIFF golden/dependency_edges check --format text (exit ref 1, head 1)
243/244 identical
PERTURB-SELFTEST-OK
```

### 3. test-txtar-extract.sh

```
ok benign
ok dotdot-name
ok absolute-name
ok inner-dotdot-name
ok empty-name
ok newline-name
ok backslash-name
ok link-escapes
ok link-absolute
ok link-name-escapes
ok link-chain
ok link-through-symlink
ok link-chain-outside
ok compare-ref.sh ref --upload-pack=x
ok compare-ref.sh ref HEAD:foo
ok compare-ref.sh ref no-such-ref-txtar-test
ok blast-snapshots.sh ref --upload-pack=x
ok blast-snapshots.sh ref HEAD:foo
ok blast-snapshots.sh ref no-such-ref-txtar-test
all txtar-extract cases passed
txtar rc=0
```

### 4. Snapshot provenance (sec #192)

Regenerated from the PROVENANCE hash into a temp dir, compared with `diff -r`, then checked for ancestry and for the absence of transitive.go:

```
H=67c9ace9a09f46d5662acc1fffddb6d2057034e8
SNAPSHOTS-REGENERATE-IDENTICAL
ANCESTOR-OK
NO-TRANSITIVE-AT-H
```

### 5. Six-target release build and module check

```
built gruntled_v0.0.0-ci_linux_amd64
built gruntled_v0.0.0-ci_linux_arm64
built gruntled_v0.0.0-ci_darwin_amd64
built gruntled_v0.0.0-ci_darwin_arm64
built gruntled_v0.0.0-ci_windows_amd64
built gruntled_v0.0.0-ci_windows_arm64
gruntled_v0.0.0-ci_darwin_amd64.tar.gz: OK
gruntled_v0.0.0-ci_darwin_arm64.tar.gz: OK
gruntled_v0.0.0-ci_linux_amd64.tar.gz: OK
gruntled_v0.0.0-ci_linux_arm64.tar.gz: OK
gruntled_v0.0.0-ci_windows_amd64.zip: OK
gruntled_v0.0.0-ci_windows_arm64.zip: OK
smoke: gruntled v0.0.0-ci (4273e30)
release rc=0
GOMOD-UNCHANGED
GOMOD-UNCHANGED
```

### 6. Testdata changes vs 67c9ace (excluding the new blast_v1 snapshots)

```
 cmd/gruntled/testdata/script/blast_exitcodes.txtar |  36 +-
 cmd/gruntled/testdata/script/blast_grt004.txtar    |  15 +-
 cmd/gruntled/testdata/script/blast_impacted.txtar  |  19 +-
 .../testdata/script/blast_transitive.txtar         | 391 +++++++++++++++++++++
 cmd/gruntled/testdata/script/usage.txtar           |   2 +-
 5 files changed, 449 insertions(+), 14 deletions(-)
```

Across phase 14:
- blast_grt004.txtar and blast_impacted.txtar changed in 14-01 and 14-02.
- blast_transitive.txtar is new.
- blast_exitcodes.txtar adds the --depth cases and the new synopsis line.
- usage.txtar changes the blast synopsis line.

`-race` was not run locally: there is no gcc and no cgo. CI runs it.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-14-03-1 | mitigated | main.go:394-422 (depthFlag records being set; parseDepth enforces ^[1-9][0-9]*$ plus Atoi), :331 (exit 2 with usage); blast_exitcodes.txtar (8 cases) |
| T-14-03-2 | mitigated | TestBlastDepth1MatchesV1 (blast_depth_test.go:132): PROVENANCE sha256, version-1 and no-distance non-vacuity, the live/edge check; gate section 4 regenerates from 67c9ace with `diff -r`, checks ancestry and that transitive.go is absent at the hash |
| T-14-03-3 | mitigated | scripts/test-txtar-extract.sh: baseline red, then green after the move, and run in the gate; scripts/lib/txtar-extract.sh:92-108 (real-path link checks); callers keep set -euo pipefail, trap, ref check and offline env (compare-ref.sh, blast-snapshots.sh:33-37) |
| T-14-03-4 | mitigated | blast-snapshots.sh runs with the extraction root as cwd on relative paths; blast_transitive.txtar `! stdout '^/'` and the drive-letter pattern; no `$WORK` regexp |
| T-14-03-5 | accepted | text is O(units) with the 6-unit path cap; TestBlastTransitiveSynthrepo measures 19,115 bytes for 148 units |
| T-14-03-6 | mitigated | docs/cli.md intro edge-rule sentence and :1034 lower-bound bullet, both pinned in TestRuleRegistryDoc; the one-hop wording is asserted absent |

## Deviations from Plan

**1. [Rule 2 - hardening] The txtar lib is not a verbatim move**
- The negative tests found two symlink escapes that the lexical-only check accepted: `d -> .` followed by `d/l -> ../x`, and `d -> .` followed by `l -> d/../x`. The lib now re-checks a link from its real parent directory and resolves the created link with `realpath -m`, failing closed if realpath is missing. Error messages are prefixed `txtar-extract:` instead of `compare-ref.sh:`.
- compare-ref.sh v0.3.0 output is unchanged: 244/244, with the same lines as phase 13.

**2. [Consequence of the flag] Two synopsis pins changed**
- The blast synopsis now reads `[--depth N]`. The first-line pins in usage.txtar:33 and blast_exitcodes.txtar:44 were updated. The plan did not list usage.txtar.

**3. [Test design] Synthrepo seed unit**
- The plan names unit 0. With fanout 3, all 300 units reach unit 0, so an exact-set check could not fail by over-reporting. The test instead picks the unit whose reverse cone is closest to half the tree (g0/unit-049, 148 units), and fails unless the cone is between 20 and 280 units.

**4. [Format limit] The newline-name case**
- A txtar member name cannot contain a newline, because a marker is a single line. That case is therefore a two-line pseudo-marker combined with an unsafe member, and it checks that nothing is written outside the target.

## Known Risks / Unverified

- `-race` not run locally (no gcc).
- scripts/lib/txtar-extract.sh needs GNU `realpath -m` and fails closed without it (for example on stock macOS before 13). The scripts are run locally on Linux and are not in CI.

## Self-Check: PASSED
