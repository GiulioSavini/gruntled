# Phase 17: Cross-Phase Integration Check (v0.4, phases 13-17)

- **Head:** master 03b9e98
- **Scope:** `git diff v0.3.0..HEAD -- ':!.planning'` (181 files)
- **Method:** I read the code and ran targeted `-short` tests (GOMAXPROCS=2, -p=2). I did not run the corpus gate, -race or -fuzz. The gate evidence is the recorded run in 17-03-SUMMARY: 31/31 shards, compare-ref 256/256, 4,345 mutations caught.

## Verdict

- **Wiring:** every cross-phase connection traces end to end. There are no BLOCKER or HIGH findings.
- **Findings:** 2 MEDIUM (CI hygiene), 2 LOW, 1 INFO.

## Wiring Summary

**Connected.** Every phase export below is called at the right point:

- **Phase 13, GRT004 core.** `analysis.RemovedOutputs` and `SupersedeUnknownOutputs` (internal/domain/analysis/grt004.go:62,141) are called only in `blasting.Between` (internal/application/blasting/blasting.go:77-86).
- **Callers of `Between`.**
  - `blasting.Blast` (blasting.go:60), which `runBlast` calls at cmd/gruntled/main.go:370.
  - The daemon seam `between = blasting.Between` (cmd/gruntled/render.go:63), which `buildBlastView` calls at render.go:70.
- **Phase 14, depth and paths.**
  - `impact.Compute` calls `propagate` (internal/domain/impact/impact.go:153).
  - The CLI applies `Result.WithMaxDistance` (impact.go:105) at main.go:374-376.
  - The presenter's `writePath` and `writeChangeTokens` (internal/interfaces/presenter/blast.go:236-296) render the reach.
- **Phase 15, type facts.**
  - The tfsurface facts reach `SurfaceDiff`, which builds `Changes` and the seeds (impact.go:135-150).
  - The presenter prints them through `writeTypeChanges` (blast.go:307) and the `typeToken` table (blast.go:299).
  - `ext/typeexpr` is imported only by internal/infrastructure/tfsurface/typesig.go.
- **Phase 16, daemon.**
  - `commitReady` (cmd/gruntled/watch.go:322) calls `blastViewOrError`, then `buildBlastView`, then the same `Between` and presenters.
  - `rebase` (watch.go:362) swaps `p.base` to `p.lastRep` under `p.mu`.
  - `report --blast` (cmd/gruntled/report.go:66-93, `printBlast` at 186) and `rebase` (cmd/gruntled/rebase.go) are wired into `runCtx` (main.go:161-164).

**Orphaned:** none. **Missing:** none.

## API (socket op) Coverage

- `ipc.OpBlast` is consumed by `report --blast` (report.go:92).
- `ipc.Rebase` is consumed by `runRebase` (rebase.go:93).
- `OpReport` is unchanged.
- The v1/v2 compatibility tests pass: TestCompatV1DaemonV2Client, TestCompatV2DaemonV1Request, TestCompatV1Dump, TestCompatV1DecoderRefusesV2Dump, TestQueryVersionMismatch and TestBlastRequestAllocsConstant.

## Auth / Trust Boundary

The socket ops keep the 0600/0700 owner-uid boundary. `report` checks both runtime-directory levels (report.go:108-110). I found no new unprotected entry point.

## Invariant Table (17-RESEARCH section 6)

| # | Invariant | Verdict | Evidence |
|---|---|---|---|
| 1 | GRT004 only reclassifies GRT001, only in `Between`, never in check/report/graph/status | **holds** | `CodeRemovedOutput` is built only at grt004.go:108. `SupersedeUnknownOutputs` drops any GRT004 with no matching (Unit, Pos) GRT001 (grt004.go:170-172), so it never adds. Its only caller is blasting.go:84. The sarif rule table has no GRT004 (presenter/sarif.go is unchanged since v0.3.0). TestBetweenOnlyReclassifiesGRT001 and TestNoGRT004OutsideBlast pass. Corpus M1 dual oracle: 17-03-SUMMARY. |
| 2 | Type facts are never Broken, never a diagnostic, never change the exit code | **holds** | Broken comes only from `NewFindings(baseD, curD)` (impact.go:134). Type facts enter only `Changes` and `Impacted`. `HasErrors` reads Broken only (impact.go:88-97). Exit codes: blast at main.go:390, report --blast at report.go:203, and `BrokenErrors` for rebase counted from Broken only (render.go:81-87). TestTypeFactsNeverChangeBrokenProperty passes. |
| 3 | Depth and paths: BFS, canonical tie-break, `--depth 1` = v0.3, Broken traversed but not listed; `report --blast --depth` exits 2 | **holds** | Broken units are skipped from Impacted but keep their Reach (impact.go:155-166). `WithMaxDistance` is applied after `Blast` (main.go:374). `report` refuses `--depth` with exit 2 (report.go:87-91). TestBlastDepth1MatchesV1 passes. The oracle and M8 depth counters are recorded in 17-03-SUMMARY. |
| 4 | Baseline equivalence: daemon view = `blast --base`; baseline = Report only; rebase installs the last published Report atomically | **holds** | The daemon uses the same `Between` and presenters as `blast --base` (render.go:69-79). `p.base` is a `checking.Report` (watch.go:331-337). The first successful commit becomes the baseline (watch.go:330-332). Rebase runs under `p.mu`, refuses on indexing, failed, unavailable view or expect mismatch, and installs `p.lastRep`, which is set together with `p.last` (watch.go:336, 382). TestDaemonBlastEqualsBlastBase and TestBaselineHoldsOnlyReport pass. |
| 5 | Escape pipeline | **holds** | Every variable string in BlastText goes through `escapeTerm`: label, subject, file, message, unit, module, path hops, names, type strings (blast.go:161-339). JSON output goes through `escapeJSON` (blast.go:433). Daemon and refusal text goes through `SanitizeReason` (watch.go:292,299,349; report.go:192; rebase.go:130). TestTextOutputsEscapeControls passes. |
| 6 | for-expression guard class | **holds** | The grep finds exactly the 4 known sites. Each is preceded by its guard: terragrunt/eval.go:28 (guard :25), eval.go:76 (guard :73), hclconv/literal.go:26 (guard :23), and tfsurface/typesig.go:161 (guards `hasForKeyword` :155 for JSON and `HasForExpr` :158 for native). There are no new sites. |
| 7 | Bounded cost | **holds (recorded); unverified at HEAD** | TestBlastRequestAllocsConstant passes locally. TestBlastSizeBound and TestReportBlastSizeBound skip under `-short` and run in the CI `test` job. That job is skipped on master since 17-02 (see F1). No production code changed after the last green CI run (d3b71c4, run 38236401050), so that run still covers the product code. |
| 8 | No regressions versus v0.3.0 | **holds** | The recorded gate covers compare-ref 256/256 and the corpora (17-03-SUMMARY). Non-blast presenters, the checking package and the statusfile code are unchanged in the diff (only presenter/blast.go changed). The grt001.go refactor is covered by compare-ref and the blast_v1 goldens. |
| 9 | No-net/no-exec on 6 targets; HCL only in infrastructure; typeexpr only in tfsurface | **holds** | `scripts/check-architecture.sh` reports OK locally. The typeexpr importer grep finds only tfsurface. |
| 10 | Docs = shipped behaviour | **holds** | TestRuleRegistryDoc, TestHelpMatchesDocs, TestDaemonDocs, TestCIDoc and TestValidationDocPins pass. |

## check / graph / report Unchanged versus v0.3.0

**Holds.**

- These paths gained no code change: `check`, `graph` and plain `report`, including their presenters (text, json, sarif, graph, status) and `internal/application/checking`.
- GRT004 never enters a `checking.Report`.
- `report` without `--blast` still sends `OpReport`.
- MORE-07 is pinned by TestNoGRT004OutsideBlast and the compat-v1 dump tests.

## E2E Flows (CLI)

| Flow | Status |
|---|---|
| `blast --base B [--depth N] [--format text\|json]`: Broken with GRT004, Impacted with distance and path, Type changes, exit 0/1/2/3 | complete |
| `watch`, then `report --blast [--format]`: baseline view, exit 0/1/2/3, windows dump | complete |
| `watch`, then `rebase [--expect N]`, then `report --blast`: new generation and accepted count; refusals exit 3 | complete |
| `check`, `graph --json`, plain `report`, status file: v0.3.0 bytes | complete |

## Findings

| ID | Severity | Owner | Finding |
|---|---|---|---|
| F1 | MEDIUM | executor | **CI `check` job is red on master since 17-02** (runs 38239933631 and 38261042593). staticcheck fails on test code: `cmd/gruntled/corpus_model_test.go:616:3` S1011 (use `append(m.Sites, mu.Sites...)`) and `corpus_model_test.go:700:48` S1016 (convert `modelSourceFile` to `oracleSource`). Because of this, the `test`, `fuzz FuzzTypeSig`, govulncheck, sarif-schema and release-build steps are skipped at HEAD. Product code is unchanged since the last green run (d3b71c4), so this does not break any requirement. It must be green before the milestone closes. |
| F2 | MEDIUM | executor | **The corpus workflow can never pass on `ubuntu-latest`.** Run 38262035294 fails with `CORPUS_GATE_NONET=1 but an empty network namespace is not available (unshare -rn)`. Ubuntu 24 runners restrict unprivileged user namespaces through AppArmor. The job is documented as informational and fails closed, so this is not a correctness hole. But docs/validation.md:873 says the workflow "runs the same two scripts", and it never does. Either fix it (for example `sudo sysctl kernel.apparmor_restrict_unprivileged_userns=0` before the gate) or document it as a known limit. |
| F3 | LOW | executor | Invariant 7 has no green CI evidence at HEAD: the size-bound tests skip under `-short` and the CI `test` job is skipped (F1). This resolves when F1 is fixed. Re-check the next ci run. |
| F4 | LOW | planner | `topUsage` still describes `report` as "print the result of the running watch daemon, like check". It does not mention `--blast`; DAEMON-08's docs update covers the read-only wording, not this summary line. This is cosmetic. TestHelpMatchesDocs passes, so the docs and the help text agree. |
| F5 | INFO | orchestrator | REL-03 is still `[ ]` / Pending in .planning/REQUIREMENTS.md:49,97, although the gate evidence is recorded in 17-03-SUMMARY. Tick it when 17-05 closes. |

## Requirements Integration Map

| Requirement | Integration path | Status | Issue |
|---|---|---|---|
| MORE-03 | grt004.go → blasting.Between → Blast (main.go:370) and buildBlastView (render.go:70) → presenter → exit | WIRED | — |
| MORE-07 | GRT004 confined to Between; checking, presenters and status unchanged | WIRED | — |
| BLAST-03 | impact.propagate ← Compute ← Between ← blast and daemon | WIRED | — |
| BLAST-04 | Reach and Via → writePath (text) and BlastJSON v2 | WIRED | — |
| BLAST-05 | `--depth` (main.go:322-336) → WithMaxDistance (impact.go:105) | WIRED | — |
| BLAST-06/07/08 | tfsurface facts/typesig → repograph surface → SurfaceDiff TypeChanges → Impacted, Type changes section | WIRED | — |
| BLAST-09 | Broken from diagnostics only; HasErrors from Broken | WIRED | — |
| BLAST-10 | tfsurface/facts.go unknown handling → SurfaceDiff reports nothing | WIRED | covered by phase 15 tests and corpus R-series |
| BLAST-11 | linear Via, elided text path, 64 MiB cap | WIRED | CI proof pending F1/F3 |
| DAEMON-07 | commitReady → BlastView → OpBlast → report --blast | WIRED | — |
| DAEMON-08 | rebase CLI → ipc.Rebase → watchPublisher.rebase | WIRED | F4 (cosmetic) |
| DAEMON-09 | ipc v2 VersionError → exit 3 (rebase.go:111, report) | WIRED | — |
| DAEMON-10 | baseline = checking.Report; TooLarge → exit 3 (report.go:189, 211) | WIRED | — |
| SEC-01 | escapeTerm, escapeJSON, strconv.Quote (grt004.go:103-106, typesig), SanitizeReason | WIRED | — |
| REL-03 | corpus gate exercises blast, check and the daemon end to end | WIRED (recorded run) | F2, F5 |

**Requirements with no cross-phase wiring:** none. Every requirement is reachable from the CLI (`blast`, `report --blast`, `rebase`, `check`, `graph`, `report`).
