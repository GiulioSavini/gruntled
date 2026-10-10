---
phase: 13-removed-output-diagnostic-shared-blast-core
plan: 03
subsystem: cli-docs-gate
tags: [grt004, more-07, docs, byte-compare, release-gate]
requires:
  - phase: 13-02
    provides: blasting.Between and GRT004 in blast --base
provides:
  - TestNoGRT004OutsideBlast and TestCheckCannotProduceGRT004 (MORE-07 by behaviour)
  - TestRuleRegistryDoc (GRT004 pinned in usage, docs/cli.md, README)
  - scripts/compare-ref.sh, a committed byte comparison against any git ref, with a perturb self-test
  - GRT004 documented (blast usage, docs/cli.md Diagnostics/Blast/Known limitations, README code table)
affects: [14, 16, 17]
tech-stack:
  added: []
  patterns: [byte comparison against the previous release binary built offline from a git worktree]
key-files:
  created:
    - cmd/gruntled/more07_test.go
    - cmd/gruntled/rules_doc_test.go
    - scripts/compare-ref.sh
  modified:
    - cmd/gruntled/main.go
    - docs/cli.md
    - README.md
key-decisions:
  - "compare-ref.sh extracts txtar fixtures with awk (no Go file under scripts/, sec #151) and prints a fixed DIFF line per mismatch; the perturb self-test asserts exit 1 and a ^DIFF line (sec #154)"
  - "A script fixture whose exec lines analyse '.' is one fixture; otherwise each top-level directory of the archive is one (diag03_corpus_shape, diag03_no_mocks, diag03_silent_rows, blast_grt004)"
  - "more07_test uses shortBase temp dirs: t.TempDir embeds the test name, which contains GRT004 and would leak into any output that echoes a path"
requirements-completed: [MORE-07, MORE-03]
duration: 12min
completed: 2026-10-10
---

# Phase 13 Plan 03: MORE-07 Proof, GRT004 Docs and Phase Gate Summary

**GRT004 never leaves the blast view. `check` (text, json, sarif), `report` through a live daemon on both transports, `graph --json` and the watch status file all report GRT001, or nothing, on a tree where blast reports GRT004. A v0.3.0 binary built offline from the tag prints the same bytes as HEAD on 244/244 (fixture, command) pairs. GRT004 is now documented in the blast usage text, docs/cli.md and README, and a doc test pins every one of those places.**

## Performance

- Duration: ~12 min
- Tasks: 3
- Files: 3 created, 3 modified

## Accomplishments

- `TestNoGRT004OutsideBlast` (more07_test.go:42) builds a GRT004-shaped base/cur pair: two reading sites plus an unrelated GRT002.
  - Non-vacuity: `blast --base` exits 1 with exactly two GRT004 and no GRT001 (:50).
  - `check` text, json and sarif contain no GRT004 and report GRT001 at both sites, and the SARIF rule table equals sarifRuleNames (:85-91).
  - `graph --json` contains no GRT004 (:94).
  - `report` text, json and sarif, through the linux socket and the windows report file, are byte-equal to `check`. The status file reaches `GRT001×2` with no GRT004 (:115-117).
- `TestCheckCannotProduceGRT004` extracts every base*/cur* tree of blast_grt004.txtar (at least 20 required; there are 28) and runs `check --format json` on each: never `"GRT004"`.
- Mutation: when GRT001 was emitted under the GRT004 code, both tests failed (check text/json print GRT004; base6 in TestCheckCannotProduceGRT004). The mutation was reverted.
- `blastUsage` (main.go:69-71) has one new sentence: "A reference to an output that the baseline module declared and path removed is reported as GRT004 instead of GRT001." docs/cli.md mirrors the usage block byte for byte.
- docs/cli.md:
  - The Broken bullet now links to the new GRT004 section.
  - The Blast text example line is GRT004.
  - Blast JSON carries the sec #137 sentence (:510-512).
  - `### GRT004: dependency output removed (error, blast only)` (:594) has the four-condition table, the re-pointed-within-the-same-module rule, mock/enabled/skip handling and the exact message. It says check, report, graph, the status file and SARIF never show GRT004.
  - Reserved codes names only GRT005 and GRT006 (:725).
  - Known limitations has two new bullets (:1015-1019).
- README:
  - Later now says `GRT005`-`GRT006`.
  - The code table has a `GRT004` / `CodeRemovedOutput` row marked blast only (:389).
  - The emitter sentence now separates check from blast (:392-396).
  - The 396-399 paragraph is reworded to GRT005/GRT006 only (sec #135).
- `TestRuleRegistryDoc` (rules_doc_test.go:48) pins:
  - the GRT004 heading inside `## Diagnostics`, and the message line inside its section;
  - Reserved codes: GRT005 and GRT006, no GRT004;
  - no GRT004 in `### SARIF`;
  - the Blast text example line, with no GRT001 for that site;
  - the Blast JSON sentence;
  - a fenced block in docs/cli.md that equals `blast -h` stderr, which must mention GRT004;
  - README: no line pairing GRT004 with "no `Code` constant", no `GRT004`-`GRT006`, no "All four are emitted by `gruntled check`", a blast-only GRT004 row, and `GRT005`-`GRT006`.
  - TestSARIFDoc and TestHelpMatchesDocs are unchanged and green.
- scripts/compare-ref.sh:
  - Setup: `set -euo pipefail`; `trap` removes the ref worktree and the temp dir on every exit (:32); `GOFLAGS=-mod=readonly GOPROXY=off` (:34); a detached `git worktree` of the ref (:36); both binaries built with no ldflags, so both say version "dev".
  - Fixture extraction: an awk txtar splitter that skips want* and `_golden/` files and recreates `_golden/symlinks`. Fixtures: 13 golden repositories, text/json/sarif/graph goldens, the 4 diag03 scripts (per top-level dir when they do not analyse "."), the 28 blast_grt004 trees, clean-fixture and sarif-fixture.
  - Runs `check --format text|json|sarif` and `graph --json` with cwd = fixture on ".", comparing stdout bytes and exit codes.
  - Output: `same|DIFF <fixture> <command>` per pair (:120), then `N/M identical` (:125); exits non-zero on any difference.
  - Extraction fidelity check: HEAD's output on the extracted text_golden and sarif_golden trees equals their want.txt / want.sarif byte for byte, and the symlinked_module_files links are recreated (`svc/api/common.tf -> ../../shared/common.tf`). Exit codes per golden match `_golden/exit` (12×1, live_clean 0).

## Task Commits

1. Task 1: `4d33169` test(13-03): GRT004 never leaves the blast view (check, report, graph, status). Tests only: MORE-07 already held by construction, and the mutation above proves the test bites.
2. Task 2 red: `4450869` test(13-03): pin GRT004 in usage, docs/cli.md and README
3. Task 2 green: `3067d58` feat(13-03): document GRT004 in blast usage, docs/cli.md and README
4. Task 3: `70b7368` feat(13-03): scripts/compare-ref.sh byte-compares check/graph output with a ref (its non-vacuity test is the perturb self-test below)

## Phase Gate

### 1. Full suite, vet, architecture

```
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	9.095s
ok  	github.com/GiulioSavini/gruntled/internal/application/blasting	0.499s
ok  	github.com/GiulioSavini/gruntled/internal/application/checking	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/application/indexing	0.005s
?   	github.com/GiulioSavini/gruntled/internal/application/ports	[no test files]
ok  	github.com/GiulioSavini/gruntled/internal/application/watching	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/domain/analysis	0.202s
ok  	github.com/GiulioSavini/gruntled/internal/domain/diagnostic	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/domain/impact	0.051s
ok  	github.com/GiulioSavini/gruntled/internal/domain/repograph	0.003s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/hclconv	2.471s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.079s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/sourceresolve	0.004s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/statusfile	0.094s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/terragrunt	2.173s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/tfsurface	1.642s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/watch	1.231s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.020s
ok  	github.com/GiulioSavini/gruntled/internal/testsupport/synthrepo	0.050s
ok  	github.com/GiulioSavini/gruntled/scripts/archscan	0.034s
suite rc=0
VET-OK
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
all architecture self-tests passed
```

### 2. Goldens vs v0.3.0

`git diff --exit-code v0.3.0 -- cmd/gruntled/testdata/golden cmd/gruntled/testdata/script/{text,json,sarif,graph}_golden.txtar cmd/gruntled/testdata/script/report_nodaemon.txtar cmd/gruntled/testdata/script/diag03_*.txtar internal/interfaces/presenter/status_test.go` is empty (`GOLDENS-IDENTICAL-TO-v0.3.0`). Across all of cmd/gruntled/testdata, the only differences from v0.3.0 are blast_impacted.txtar (+4 -1) and the new blast_grt004.txtar, both from 13-02.

### 3. `bash scripts/compare-ref.sh v0.3.0` (full output, exit 0)

```
same golden/dependency_edges check --format text (exit 1)
same golden/dependency_edges check --format json (exit 1)
same golden/dependency_edges check --format sarif (exit 1)
same golden/dependency_edges graph --json (exit 0)
same golden/grt002_missing_target check --format text (exit 1)
same golden/grt002_missing_target check --format json (exit 1)
same golden/grt002_missing_target check --format sarif (exit 1)
same golden/grt002_missing_target graph --json (exit 0)
same golden/grt002_shared_include check --format text (exit 1)
same golden/grt002_shared_include check --format json (exit 1)
same golden/grt002_shared_include check --format sarif (exit 1)
same golden/grt002_shared_include graph --json (exit 0)
same golden/grt003_cycles check --format text (exit 1)
same golden/grt003_cycles check --format json (exit 1)
same golden/grt003_cycles check --format sarif (exit 1)
same golden/grt003_cycles graph --json (exit 0)
same golden/lazy_guards check --format text (exit 1)
same golden/lazy_guards check --format json (exit 1)
same golden/lazy_guards check --format sarif (exit 1)
same golden/lazy_guards graph --json (exit 0)
same golden/live_broken check --format text (exit 1)
same golden/live_broken check --format json (exit 1)
same golden/live_broken check --format sarif (exit 1)
same golden/live_broken graph --json (exit 0)
same golden/live_clean check --format text (exit 0)
same golden/live_clean check --format json (exit 0)
same golden/live_clean check --format sarif (exit 0)
same golden/live_clean graph --json (exit 0)
same golden/mock_shapes check --format text (exit 1)
same golden/mock_shapes check --format json (exit 1)
same golden/mock_shapes check --format sarif (exit 1)
same golden/mock_shapes graph --json (exit 0)
same golden/remote_local_mix check --format text (exit 1)
same golden/remote_local_mix check --format json (exit 1)
same golden/remote_local_mix check --format sarif (exit 1)
same golden/remote_local_mix graph --json (exit 0)
same golden/shared_include check --format text (exit 1)
same golden/shared_include check --format json (exit 1)
same golden/shared_include check --format sarif (exit 1)
same golden/shared_include graph --json (exit 0)
same golden/symlinked_module_files check --format text (exit 1)
same golden/symlinked_module_files check --format json (exit 1)
same golden/symlinked_module_files check --format sarif (exit 1)
same golden/symlinked_module_files graph --json (exit 0)
same golden/syntax_errors_mixed check --format text (exit 1)
same golden/syntax_errors_mixed check --format json (exit 1)
same golden/syntax_errors_mixed check --format sarif (exit 1)
same golden/syntax_errors_mixed graph --json (exit 0)
same golden/tf_json_surface check --format text (exit 1)
same golden/tf_json_surface check --format json (exit 1)
same golden/tf_json_surface check --format sarif (exit 1)
same golden/tf_json_surface graph --json (exit 0)
same script/text_golden check --format text (exit 1)
same script/text_golden check --format json (exit 1)
same script/text_golden check --format sarif (exit 1)
same script/text_golden graph --json (exit 0)
same script/json_golden check --format text (exit 1)
same script/json_golden check --format json (exit 1)
same script/json_golden check --format sarif (exit 1)
same script/json_golden graph --json (exit 0)
same script/sarif_golden check --format text (exit 1)
same script/sarif_golden check --format json (exit 1)
same script/sarif_golden check --format sarif (exit 1)
same script/sarif_golden graph --json (exit 0)
same script/graph_golden check --format text (exit 1)
same script/graph_golden check --format json (exit 1)
same script/graph_golden check --format sarif (exit 1)
same script/graph_golden graph --json (exit 0)
same script/diag03_corpus_shape/nomerge check --format text (exit 1)
same script/diag03_corpus_shape/nomerge check --format json (exit 1)
same script/diag03_corpus_shape/nomerge check --format sarif (exit 1)
same script/diag03_corpus_shape/nomerge graph --json (exit 0)
same script/diag03_corpus_shape/shape check --format text (exit 1)
same script/diag03_corpus_shape/shape check --format json (exit 1)
same script/diag03_corpus_shape/shape check --format sarif (exit 1)
same script/diag03_corpus_shape/shape graph --json (exit 0)
same script/diag03_issue2163 check --format text (exit 1)
same script/diag03_issue2163 check --format json (exit 1)
same script/diag03_issue2163 check --format sarif (exit 1)
same script/diag03_issue2163 graph --json (exit 0)
same script/diag03_no_mocks/declared check --format text (exit 0)
same script/diag03_no_mocks/declared check --format json (exit 0)
same script/diag03_no_mocks/declared check --format sarif (exit 0)
same script/diag03_no_mocks/declared graph --json (exit 0)
same script/diag03_no_mocks/missing check --format text (exit 1)
same script/diag03_no_mocks/missing check --format json (exit 1)
same script/diag03_no_mocks/missing check --format sarif (exit 1)
same script/diag03_no_mocks/missing graph --json (exit 0)
same script/diag03_silent_rows/row1 check --format text (exit 0)
same script/diag03_silent_rows/row1 check --format json (exit 0)
same script/diag03_silent_rows/row1 check --format sarif (exit 0)
same script/diag03_silent_rows/row1 graph --json (exit 0)
same script/diag03_silent_rows/row2a check --format text (exit 0)
same script/diag03_silent_rows/row2a check --format json (exit 0)
same script/diag03_silent_rows/row2a check --format sarif (exit 0)
same script/diag03_silent_rows/row2a graph --json (exit 0)
same script/diag03_silent_rows/row2b check --format text (exit 0)
same script/diag03_silent_rows/row2b check --format json (exit 0)
same script/diag03_silent_rows/row2b check --format sarif (exit 0)
same script/diag03_silent_rows/row2b graph --json (exit 0)
same script/diag03_silent_rows/row3a check --format text (exit 0)
same script/diag03_silent_rows/row3a check --format json (exit 0)
same script/diag03_silent_rows/row3a check --format sarif (exit 0)
same script/diag03_silent_rows/row3a graph --json (exit 0)
same script/diag03_silent_rows/row3b check --format text (exit 0)
same script/diag03_silent_rows/row3b check --format json (exit 0)
same script/diag03_silent_rows/row3b check --format sarif (exit 0)
same script/diag03_silent_rows/row3b graph --json (exit 0)
same script/diag03_silent_rows/row4 check --format text (exit 0)
same script/diag03_silent_rows/row4 check --format json (exit 0)
same script/diag03_silent_rows/row4 check --format sarif (exit 0)
same script/diag03_silent_rows/row4 graph --json (exit 0)
same script/diag03_silent_rows/row5a check --format text (exit 0)
same script/diag03_silent_rows/row5a check --format json (exit 0)
same script/diag03_silent_rows/row5a check --format sarif (exit 0)
same script/diag03_silent_rows/row5a graph --json (exit 0)
same script/diag03_silent_rows/row5b check --format text (exit 0)
same script/diag03_silent_rows/row5b check --format json (exit 0)
same script/diag03_silent_rows/row5b check --format sarif (exit 0)
same script/diag03_silent_rows/row5b graph --json (exit 0)
same script/diag03_silent_rows/row5c check --format text (exit 0)
same script/diag03_silent_rows/row5c check --format json (exit 0)
same script/diag03_silent_rows/row5c check --format sarif (exit 0)
same script/diag03_silent_rows/row5c graph --json (exit 0)
same script/blast_grt004/base1 check --format text (exit 0)
same script/blast_grt004/base1 check --format json (exit 0)
same script/blast_grt004/base1 check --format sarif (exit 0)
same script/blast_grt004/base1 graph --json (exit 0)
same script/blast_grt004/base2 check --format text (exit 0)
same script/blast_grt004/base2 check --format json (exit 0)
same script/blast_grt004/base2 check --format sarif (exit 0)
same script/blast_grt004/base2 graph --json (exit 0)
same script/blast_grt004/base3 check --format text (exit 0)
same script/blast_grt004/base3 check --format json (exit 0)
same script/blast_grt004/base3 check --format sarif (exit 0)
same script/blast_grt004/base3 graph --json (exit 0)
same script/blast_grt004/base4 check --format text (exit 0)
same script/blast_grt004/base4 check --format json (exit 0)
same script/blast_grt004/base4 check --format sarif (exit 0)
same script/blast_grt004/base4 graph --json (exit 0)
same script/blast_grt004/base5a check --format text (exit 1)
same script/blast_grt004/base5a check --format json (exit 1)
same script/blast_grt004/base5a check --format sarif (exit 1)
same script/blast_grt004/base5a graph --json (exit 0)
same script/blast_grt004/base5b check --format text (exit 0)
same script/blast_grt004/base5b check --format json (exit 0)
same script/blast_grt004/base5b check --format sarif (exit 0)
same script/blast_grt004/base5b graph --json (exit 0)
same script/blast_grt004/base6 check --format text (exit 1)
same script/blast_grt004/base6 check --format json (exit 1)
same script/blast_grt004/base6 check --format sarif (exit 1)
same script/blast_grt004/base6 graph --json (exit 0)
same script/blast_grt004/base7a check --format text (exit 0)
same script/blast_grt004/base7a check --format json (exit 0)
same script/blast_grt004/base7a check --format sarif (exit 0)
same script/blast_grt004/base7a graph --json (exit 0)
same script/blast_grt004/base7b check --format text (exit 0)
same script/blast_grt004/base7b check --format json (exit 0)
same script/blast_grt004/base7b check --format sarif (exit 0)
same script/blast_grt004/base7b graph --json (exit 0)
same script/blast_grt004/base7c check --format text (exit 0)
same script/blast_grt004/base7c check --format json (exit 0)
same script/blast_grt004/base7c check --format sarif (exit 0)
same script/blast_grt004/base7c graph --json (exit 0)
same script/blast_grt004/base7d check --format text (exit 0)
same script/blast_grt004/base7d check --format json (exit 0)
same script/blast_grt004/base7d check --format sarif (exit 0)
same script/blast_grt004/base7d graph --json (exit 0)
same script/blast_grt004/base8a check --format text (exit 0)
same script/blast_grt004/base8a check --format json (exit 0)
same script/blast_grt004/base8a check --format sarif (exit 0)
same script/blast_grt004/base8a graph --json (exit 0)
same script/blast_grt004/base8b check --format text (exit 0)
same script/blast_grt004/base8b check --format json (exit 0)
same script/blast_grt004/base8b check --format sarif (exit 0)
same script/blast_grt004/base8b graph --json (exit 0)
same script/blast_grt004/base9 check --format text (exit 0)
same script/blast_grt004/base9 check --format json (exit 0)
same script/blast_grt004/base9 check --format sarif (exit 0)
same script/blast_grt004/base9 graph --json (exit 0)
same script/blast_grt004/cur1 check --format text (exit 1)
same script/blast_grt004/cur1 check --format json (exit 1)
same script/blast_grt004/cur1 check --format sarif (exit 1)
same script/blast_grt004/cur1 graph --json (exit 0)
same script/blast_grt004/cur2 check --format text (exit 0)
same script/blast_grt004/cur2 check --format json (exit 0)
same script/blast_grt004/cur2 check --format sarif (exit 0)
same script/blast_grt004/cur2 graph --json (exit 0)
same script/blast_grt004/cur3 check --format text (exit 1)
same script/blast_grt004/cur3 check --format json (exit 1)
same script/blast_grt004/cur3 check --format sarif (exit 1)
same script/blast_grt004/cur3 graph --json (exit 0)
same script/blast_grt004/cur4 check --format text (exit 1)
same script/blast_grt004/cur4 check --format json (exit 1)
same script/blast_grt004/cur4 check --format sarif (exit 1)
same script/blast_grt004/cur4 graph --json (exit 0)
same script/blast_grt004/cur5a check --format text (exit 1)
same script/blast_grt004/cur5a check --format json (exit 1)
same script/blast_grt004/cur5a check --format sarif (exit 1)
same script/blast_grt004/cur5a graph --json (exit 0)
same script/blast_grt004/cur5b check --format text (exit 1)
same script/blast_grt004/cur5b check --format json (exit 1)
same script/blast_grt004/cur5b check --format sarif (exit 1)
same script/blast_grt004/cur5b graph --json (exit 0)
same script/blast_grt004/cur6 check --format text (exit 1)
same script/blast_grt004/cur6 check --format json (exit 1)
same script/blast_grt004/cur6 check --format sarif (exit 1)
same script/blast_grt004/cur6 graph --json (exit 0)
same script/blast_grt004/cur7a check --format text (exit 0)
same script/blast_grt004/cur7a check --format json (exit 0)
same script/blast_grt004/cur7a check --format sarif (exit 0)
same script/blast_grt004/cur7a graph --json (exit 0)
same script/blast_grt004/cur7b check --format text (exit 0)
same script/blast_grt004/cur7b check --format json (exit 0)
same script/blast_grt004/cur7b check --format sarif (exit 0)
same script/blast_grt004/cur7b graph --json (exit 0)
same script/blast_grt004/cur7c check --format text (exit 0)
same script/blast_grt004/cur7c check --format json (exit 0)
same script/blast_grt004/cur7c check --format sarif (exit 0)
same script/blast_grt004/cur7c graph --json (exit 0)
same script/blast_grt004/cur7d check --format text (exit 0)
same script/blast_grt004/cur7d check --format json (exit 0)
same script/blast_grt004/cur7d check --format sarif (exit 0)
same script/blast_grt004/cur7d graph --json (exit 0)
same script/blast_grt004/cur8a check --format text (exit 1)
same script/blast_grt004/cur8a check --format json (exit 1)
same script/blast_grt004/cur8a check --format sarif (exit 1)
same script/blast_grt004/cur8a graph --json (exit 0)
same script/blast_grt004/cur8b check --format text (exit 1)
same script/blast_grt004/cur8b check --format json (exit 1)
same script/blast_grt004/cur8b check --format sarif (exit 1)
same script/blast_grt004/cur8b graph --json (exit 0)
same script/blast_grt004/cur9 check --format text (exit 0)
same script/blast_grt004/cur9 check --format json (exit 0)
same script/blast_grt004/cur9 check --format sarif (exit 0)
same script/blast_grt004/cur9 graph --json (exit 0)
same dir/clean-fixture check --format text (exit 0)
same dir/clean-fixture check --format json (exit 0)
same dir/clean-fixture check --format sarif (exit 0)
same dir/clean-fixture graph --json (exit 0)
same dir/sarif-fixture check --format text (exit 1)
same dir/sarif-fixture check --format json (exit 1)
same dir/sarif-fixture check --format sarif (exit 1)
same dir/sarif-fixture graph --json (exit 0)
244/244 identical
```

Perturb self-test (sec #154): `out=$(COMPARE_REF_PERTURB=1 bash scripts/compare-ref.sh v0.3.0 2>&1); rc=$?; test "$rc" -eq 1 && printf "%s" "$out" | grep -q "^DIFF "`:

```
perturb rc=1
DIFF golden/dependency_edges check --format text (exit ref 1, head 1)
243/244 identical
PERTURB-SELFTEST-OK
```

### 4. Six-target release build and module check

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
smoke: gruntled v0.0.0-ci (70b7368)
release rc=0
checksums.txt
gruntled_v0.0.0-ci_darwin_amd64.tar.gz
gruntled_v0.0.0-ci_darwin_arm64.tar.gz
gruntled_v0.0.0-ci_linux_amd64.tar.gz
gruntled_v0.0.0-ci_linux_arm64.tar.gz
gruntled_v0.0.0-ci_windows_amd64.zip
gruntled_v0.0.0-ci_windows_arm64.zip
GOMOD-UNCHANGED
```

`git diff --exit-code go.mod go.sum`: empty (GOMOD-UNCHANGED). `-race` was not run locally: there is no gcc and no cgo. CI runs it on three OSes.

### Earlier non-vacuity evidence in this phase

- 13-01 necessity test and hand mutations: see 13-01-SUMMARY.md, "Necessity test" and "Manual mutations".
- 13-02 hand mutations (supersede removed, removed appended, v0.3 Blast body): see 13-02-SUMMARY.md, "Hand mutations".

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-13-03-1 | mitigated | internal/application/checking/check.go:47 (analyzers take one graph, so RemovedOutputs cannot be added); more07_test.go:42-121 (TestNoGRT004OutsideBlast with the non-vacuity blast run at :50); gate step 3 (244/244 identical against v0.3.0) |
| T-13-03-2 | mitigated | rules_doc_test.go:48-117 (TestRuleRegistryDoc); docs/cli.md:594, :725, :1018; README.md:389; TestSARIFDoc unchanged |
| T-13-03-3 | mitigated | compare-ref.sh uses awk only (no Go helper, no module); `git diff --exit-code go.mod go.sum` empty in Task 1 and the gate |
| T-13-03-4 | mitigated | gate step 4: six targets built, checksums verified, smoke OK; check-architecture OK (Step 9 no-net/no-exec unchanged) |
| T-13-03-5 | mitigated | scripts/compare-ref.sh:32 (trap removes the worktree and temp dir); `git worktree list` after runs shows only the main tree |
| T-13-03-6 | mitigated | scripts/compare-ref.sh:34 (`GOFLAGS=-mod=readonly GOPROXY=off`) |
| T-13-03-7 | mitigated | docs/cli.md:510-512 (Blast JSON sentence), pinned at rules_doc_test.go:77 |

## Deviations from Plan

**1. [Rule 1 - test bug] shortBase instead of t.TempDir in more07_test.go**
- The first run failed because `t.TempDir()` embeds the test name `TestNoGRT004OutsideBlast`, and blast's `baseline:` line echoes the path, so the GRT004 count was 3. Both tests now use the existing `shortBase` helper (os.MkdirTemp with a short name).

**2. [Scope] Task 1 and Task 3 have no red-then-green pair**
- Task 1 proves an existing property; MORE-07 already holds through the one-graph analyzer type. Its non-vacuity is the hand mutation described above.
- Task 3 is a script. Its "red" is the perturb self-test, which must exit 1 with a DIFF line.

**3. [Clarification] Fixture roots for script archives**
- The plan says "for a script that analyses `.`, the extracted root is the fixture". For the other scripts, each top-level directory is a fixture. That gives the 28 blast_grt004 trees and the per-row diag03 trees.

## Known Risks / Unverified

- `-race` not run locally (no gcc).
- compare-ref.sh compares stdout and exit codes, not stderr (check's summary line), as the plan specifies.

## Self-Check: PASSED
