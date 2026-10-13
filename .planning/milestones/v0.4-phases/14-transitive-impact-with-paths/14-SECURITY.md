---
phase: 14-transitive-impact-with-paths
audited: 2026-10-10
auditor: gruntled-sec (code-audit, bus verdict #234, findings #232-#233)
scope: "git diff 67c9ace..abcb39e -- ':!.planning'"
verdict: SECURED
threats_total: 17
threats_mitigated: 16
threats_accepted: 1
threats_na: 0
threats_open: 0
findings: { blocker: 0, high: 0, medium: 0, low: 1, info: 1 }
---

# Phase 14 Security

Verdict **SECURED**, 0 open threats. Every `mitigate` row was verified at file:line and by
mutation: 26 mutations in a throwaway copy (`git archive abcb39e`), 24 killed by the expected
tests, 2 survived as equivalent mutants (removing the adjacency sort, since `Edges()` is already
ordered by `from` and duplicates are skipped by the visited set; removing the digit loop in
`parseDepth`, since the first-character check plus `strconv.Atoi` already reject sign and
underscore). T-14-03-3 holds for its stated threat (the refactor did not weaken the phase 13
checks). Two further symlink escapes survive in the extraction (F1, LOW, non-blocking).

Baseline at abcb39e: `go vet` clean, `go test ./...` green, CI green on all OSes, no
`net`/`net/*`/`os/exec`/`plugin` in `go list -deps` on the six release targets,
`check-architecture.sh` OK, `go mod verify` OK, go.mod/go.sum unchanged in the range,
`govulncheck` clean, `scripts/test-txtar-extract.sh` 19/19.
Snapshot gate re-run: `blast-snapshots.sh 67c9ace` into a temp dir is identical to
`cmd/gruntled/testdata/blast_v1` (`diff -r`); 67c9ace is an ancestor of abcb39e; 67c9ace has no
`internal/domain/impact/transitive.go`; every PROVENANCE sha256 matches.

Accepted executor deviations (not findings): fallback exhaustive oracle set, layered generator,
synthrepo seed choice, usage.txtar pins, newline-name approximation in test-txtar-extract.sh.

## Threat register

| ID | Disposition | Status | Evidence |
|----|-------------|--------|----------|
| T-14-01-1 | mitigate | CLOSED | `propagates` `transitive.go:25-34` (EdgeBlock only, Enabled True, SkipOutputs False, target is a unit; absent maps to True/False in `depfacts.go`); mutations: paths edges propagate, unknown enabled, unknown skip_outputs, no skip check, non-unit target: all killed by `TestPropagatesEdgeRule` (+ `TestTransitiveMatchesOracleRandom`) |
| T-14-01-2 | mitigate | CLOSED | iterative level BFS with visited map `transitive.go:65-89`, no recursion; `TestPropagateNoRecursion` (20,000 chain); dropping the visited check killed by `TestPropagateCycleSelfLoopDiamond`, both oracle tests |
| T-14-01-3 | mitigate | CLOSED | sorted seeds `transitive.go:68`, sorted frontier `:86`, first discovery wins `:79-82`; Impacted sorted after map walk `impact.go`; unsorted frontier / unsorted seeds killed (`TestPropagateTieBreak`, oracle); unsorted adjacency equivalent (see above) |
| T-14-01-4 | mitigate | CLOSED | reverse edges `out[e.To()] += e.From()` `transitive.go:42`; forward mutation killed by `TestPropagateDirection`, `TestPropagateChain`, `TestCompute*` |
| T-14-01-5 | mitigate | CLOSED | Broken excluded after traversal `impact.go:163`; `HasErrors` reads Broken only `impact.go:88-97` (unchanged); mutation killed by `TestDisjoint`, `TestComputeBrokenTraversed`, `TestWithMaxDistance` |
| T-14-01-6 | mitigate | CLOSED | `Reach` is three scalars; `Change` shares `byModule` values; JSON lists only at distance 1 `blast.go:282`; always-emit-lists mutation killed by `TestBlastJSONLinear`, goldens |
| T-14-02-1 | mitigate | CLOSED | `escapeTerm` on every hop `blast.go:206`, source `:214`, from-module `:165-166`; `escapeJSON` whole buffer `:309`; each removal killed (`TestBlastTextEscapesControls`, `TestBlastJSONEscapesTerminalRunes`, e2e `TestTextOutputsEscapeControls`) |
| T-14-02-2 | mitigate | CLOSED | path cap `textPathMaxUnits, textPathHead = 6, 4` `blast.go:179`; JSON via only; no-elision mutation killed by `TestBlastTextLinear`, `TestBlastTextPathElision` |
| T-14-02-3 | mitigate | CLOSED | `docs/cli.md:533` "a v1 reader must reject version 2"; `blastSchemaVersion = 2` `blast.go:16`; doc and version mutations killed by `TestRuleRegistryDoc`, `TestBlastJSONV1KeysKeepType` and goldens |
| T-14-02-4 | mitigate | CLOSED | walk bounded by `limit` `blast.go:194`, stops on zero Via or source; unbounded walk mutation runs out of memory in `TestBlastTextMalformedVia` / `TestBlastTextPathElision` |
| T-14-02-5 | mitigate | CLOSED | path follows Via only `blast.go:187-215`; Via deterministic (T-14-01-3); `TestBlastTextPathElision` |
| T-14-03-1 | mitigate | CLOSED | `depthFlag.Set` records set `main.go:403`, strict `parseDepth` `main.go:410` (`^[1-9][0-9]*$` + Atoi overflow), exit 2 with `%q` value `main.go:331`; `blast_exitcodes.txtar:61-` (0, -1, +2, 02, x, 1.5, empty, missing value); accept-zero mutation killed by `TestScripts`; off-by-one in `WithMaxDistance` killed by `TestWithMaxDistance`, `TestBlastDepthFlag`, `TestBlastDepth1MatchesV1` |
| T-14-03-2 | mitigate | CLOSED | `TestBlastDepth1MatchesV1` `blast_depth_test.go:132` (PROVENANCE hash format, sha256 per file, unlisted files rejected, v1 non-vacuity); tampering a snapshot killed; gate re-run by sec (see above) |
| T-14-03-3 | mitigate | CLOSED | lib functions-only, sourced via `BASH_SOURCE` before any `cd`; callers keep `set -euo pipefail`, trap, `rev-parse --verify --quiet --end-of-options "$ref^{commit}"`, offline env; 19 negative cases pass incl. `--upload-pack=x`, `HEAD:foo`; names with spaces and a leading dash handled (`--` everywhere). Two link escapes remain (F1) | Remaining link escapes (F1) closed in f08d9ad.
| T-14-03-4 | mitigate | CLOSED | snapshots produced with cwd at the extraction root on relative paths; no `/tmp`, `/home` or `$WORK` in `testdata/blast_v1/*`; `blast_transitive.txtar:12` `! stdout '^/'` |
| T-14-03-5 | accept | CLOSED | text O(units) with the 6-unit path cap; `--depth`; 5,000-unit bound in Phase 15 (BLAST-11) |
| T-14-03-6 | mitigate | CLOSED | `TestRuleRegistryDoc` `rules_doc_test.go:122` (no "one hop", lower-bound bullet `docs/cli.md:1034`); both mutations killed |

## Findings

| # | Severity | Ref | Finding | Disposition |
|---|----------|-----|---------|-------------|
| F1 | LOW | `scripts/lib/txtar-extract.sh:107-108` | Two symlink escapes survive. (1) A link checked before a later link changes its path: `l -> d/../x` then `d -> .`; `realpath -m` resolves the missing `d` lexically, then `d -> .` makes `l` resolve to `<dest>/../x`. With `d/d/…/../..` it reaches any absolute path (repro read `/etc/hostname`, rc 0). (2) A duplicate name onto a directory link: `x/y/d -> ../..` then `x/y/d -> ../z`; `ln -s` without `-n` follows `d` and creates `<dest>/z -> ../z` outside the tree, while `realpath` checks `<dest>/x/y/d`. No trust boundary is crossed today: both callers already build and run code from the same tree or ref, and gruntled is read-only with no network access. | Fixed in 917c7aa (red) + f08d9ad: `ln -sn`, refuse existing link paths and links under links, final `realpath` re-check of every link; 4 new cases, 23/23 pass |
| F2 | INFO | `scripts/lib/txtar-extract.sh:108`, `scripts/blast-snapshots.sh:79` | macOS/BSD has no `realpath -m` and no `sha256sum`. The scripts fail closed there: `set -e` in the caller, an empty resolved path fails the check, and the trap cleans up. | Documented in 65a0b2b (script headers, README development section) |

## Extraction probes (txtar-extract.sh at abcb39e)

- Late link (`l -> d/../x`, then `d -> .`): accepted, escapes (F1).
- Duplicate link name onto a directory link: accepted, escapes (F1).
- Leading-dash names (`-n`, link `-l -> -n`) and names with spaces or an embedded `--`: extracted inside the tree.
- Newline in a name: not expressible in a txtar marker; split markers are content (the test case covers it).
- Hardlinks: not expressible in the archive format; N/A.
- TOCTOU against another user: dest is under `mktemp -d` (0700, owner only); the only race is the extractor's own ordering (F1).
- Hostile refs (`--upload-pack=x`, `HEAD:foo`, `-`, missing): refused by `rev-parse --verify --quiet --end-of-options "$ref^{commit}"` before any worktree or build.
