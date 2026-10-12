---
phase: 17-corpus-mutation-proof-cross-phase-audit
audited: 2026-10-12
auditor: gruntled-sec (cross-phase code-audit for milestone v0.4, 17-05 Task 1 step 1, plus secure-phase 17; re-audit of 4c42aab..a11dc37)
scope: "git diff v0.3.0..HEAD -- ':!.planning' at master 7bf149c; phase 17 scripts and corpus.yml; integration F1/F2/F4 fixes 0682227..7bf149c; re-audit 88a62aa..a11dc37"
verdict: SECURED (1 open condition: F1 closes on corpus run 38264937436)
threats_total: 46
threats_mitigated: 45
threats_accepted: 1
threats_na: 0
threats_open: 0
findings: { blocker: 0, high: 0, medium: 1, low: 0, info: 4 }  # F1 fixed in 4c42aab/acf2759 (closes on corpus run 38264937436); F2, F4, F5 closed; F3 accepted
---

# Phase 17 Security (cross-phase audit, v0.4)

## Re-audit (88a62aa..a11dc37)

Verdict **SECURED, with one open condition.** The MEDIUM (F1) is fixed in code and test. It
closes when corpus run 38264937436 on a11dc37 finishes green. At the time of writing, its
`download modules`, `fetch corpora` and `allow unprivileged user namespaces` steps had
passed, and the `corpus gate` step was still running. Every other finding is closed or
accepted, and all 46 threat rows stay CLOSED.

- **F1** (4c42aab red, acf2759 fix).
  - 4c42aab: TestCIDoc requires a `download modules` step that runs `go mod download` and
    then `go mod verify`, placed before `bash scripts/corpus-gate.sh`.
  - acf2759: corpus.yml adds that step right after setup-go. It keeps `cache: false`. The
    step runs before `fetch corpora`, so no corpus content is on disk while the job has
    network for module downloads, and `go mod verify` checks the downloads against go.sum.
    The gate stays offline (`GOPROXY=off`, `-mod=readonly`, `unshare -rn`, `NONET=1`).
  - The earlier failure (run 38263198748) came seconds after the gate started. The new run
    is past that point, but it is not yet green.
  - docs/validation.md records the step.
  - TestCIDoc and TestValidationDocPins pass locally.
- **F2 / row 7 / integration F3.** CI run 38264926783 on a11dc37 is green on every job:
  check (staticcheck, test, fuzz, govulncheck, release build), architecture, test-os
  windows and macos, recipe-check and sarif-upload. Its `test` step runs
  TestBlastSizeBound and TestReportBlastSizeBound, so row 7 now **holds at HEAD**.
- **F4** (a11dc37). The commit moves `noReplyError`, `readReply`, `getHandler` and
  `readLine` from ipc.go into `sock_unix.go` (linux || darwin), and `editorPattern` from
  ignore.go into `native_unix.go` (!windows).
  - It is a pure move: the sorted removed and added lines are identical except for one
    added `"io"` import.
  - Windows needs none of the moved code. It has no socket server or client, and
    `ReadDump` keeps its own cap.
  - `GOOS=windows go vet` on ipc and watch is clean, and windows CI is green on
    38264926783.
- **F5** (8bdb620). The no-execution claim is now scoped to fetch-corpora.sh, the gate and
  corpus.yml. The phase 5 terragrunt oracle is named as the exception, gated by
  `GRUNTLED_TERRAGRUNT_BIN`. This is in validation.md, the script header and the
  T-17-01-2, T-17-03-7 and T-17-04-5 rows of the SUMMARYs. No code changed.

## Initial audit verdict (7bf149c, superseded by the re-audit above)

**OPEN_THREATS (0 blocking).** 0 BLOCKER, 0 HIGH, 1 MEDIUM, 0 LOW, 4 INFO.

- All 10 rows of the 17-RESEARCH section 6 invariant table **hold** on the final tree.
  Row 7 (bounded cost) holds on the record. Its CI proof at HEAD is the `test` step of run
  38263188666, still running at the time of writing.
- All 46 threat rows of the phase 17 plans are CLOSED: 45 mitigate rows with evidence and
  1 accept row.
- No security hole was found in the blast core, the socket ops, output escaping, the
  `blast --base` second tree, the instance store and lock, or the windows dump. The only
  product-code changes since the phase 16 re-audit (57d74d8) are one usage line in
  `main.go:56` and a comment in `watch/run.go:129-135`.
- **F1 (MEDIUM)** is integration F2 again. The AppArmor sysctl works: the next dispatch
  (run 38263198748 at 7bf149c) prints `network=isolated`. But the gate then fails at once
  because the Go module cache is empty offline. corpus.yml still cannot pass. It fails
  closed, so this is not a security hole. 17-05 Task 2 must close it.
- Integration F1 is closed. The CI `staticcheck` step is green at 7bf149c (run 38263188666).
  Integration F4 is closed: TestTopUsageMatchesDocs passes.

## Method and evidence at 7bf149c

- **RAM rules.** `GOMAXPROCS=2 GOFLAGS=-p=2`. Each run was preceded by `free -m` (at least
  6.6 GB available each time) and ran under `ulimit -v 4000000`. I ran no `go test ./...`,
  no -race, no -fuzz and no corpus gate. The gate evidence is the recorded run in
  17-03-SUMMARY: `network=isolated (/proc/net/dev: lo)`, 31/31 shards, compare-ref 256/256,
  4,345 mutations caught.
- **Targeted `-short` tests, all passing:**
  - `internal/application/blasting`: TestBetweenOnlyReclassifiesGRT001.
  - `internal/domain/impact`: TestTypeFactsNeverChangeBrokenProperty.
  - `internal/infrastructure/ipc`: TestRespond, TestRespondPassesExpect, TestSealBlast,
    TestBlastRequestAllocsConstant.
  - `cmd/gruntled`: TestNoGRT004OutsideBlast, TestBlastDepth1MatchesV1,
    TestDaemonBlastEqualsBlastBase, TestBaselineHoldsOnlyReport,
    TestTextOutputsEscapeControls, TestReportRefusalSanitised, TestRebaseRefusalsCLI,
    TestReportBlastRefusals, TestCIDoc, TestTopUsageMatchesDocs, TestHelpMatchesDocs,
    TestDaemonDocs, TestRuleRegistryDoc, TestValidationDocPins, TestCorpusHarnessSelfTest.
  - TestBlastSizeBound and TestReportBlastSizeBound SKIP under `-short`, by design. They are
    CI only.
- **Scripts.** `scripts/test-fetch-corpora.sh` passes 8/8: pinned HEAD, unknown sha, dest at
  another commit, rerun, malformed sha, HOME filter not run, templateDir hook not run, and
  escaping symlink. `scripts/check-architecture.sh` is OK.
- **Greps.**
  - `CodeRemovedOutput` is built only at `grt004.go:108`.
  - `RemovedOutputs` and `SupersedeUnknownOutputs` are called only from
    `blasting.go:80,84`.
  - `ext/typeexpr` is imported only by `tfsurface/typesig.go`.
  - The evaluation-site grep returns exactly the 4 known lines.
- **CI.**
  - Run 38263188666 (ci, 7bf149c): gofmt, `go mod tidy`, vet, staticcheck and govulncheck
    pass. `test` was still in progress when this was written.
  - Run 38263198748 (corpus, 7bf149c): fails, see F1.

## Invariant table (17-RESEARCH section 6)

| # | Invariant | Verdict | Evidence |
|---|-----------|---------|----------|
| 1 | GRT004 only reclassifies GRT001: one finding per site, never adds, only `Between` creates it; never in check/report/graph/status | **holds** | `NewForUnit(CodeRemovedOutput…)` appears only at `grt004.go:108`. `SupersedeUnknownOutputs` (`grt004.go:141-195`) drops a GRT004 with no (Unit, Pos) GRT001 (`:169-171`) and removes one GRT001 per GRT004 it adds, so the count never grows. Its only caller is `Between` (`blasting.go:79-85`). The daemon baseline is a `checking.Report` (16-SECURITY T-16-02-3/4). TestBetweenOnlyReclassifiesGRT001 and TestNoGRT004OutsideBlast pass. M1 dual oracle against a frozen v0.3.0 is in 17-03-SUMMARY |
| 2 | Type facts are never Broken, never a diagnostic, never change the exit code | **holds** | Broken is built from diagnostics only. `HasErrors` reads Broken (impact.go). The blast exit is at `main.go:392`, the `report --blast` exit is `v.HasErrors` (`report.go:203`), and rebase counts Broken only (`render.go`). TestTypeFactsNeverChangeBrokenProperty passes. Corpus M3-M7 exit 0 is recorded in 17-03 |
| 3 | Depth/paths: BFS over live edges, canonical tie-break, `--depth 1` = v0.3, Broken traversed not listed; `report --blast` refuses `--depth` | **holds** | `parseDepth` accepts only `^[1-9][0-9]*$` and int-sized values (`main.go:414-428`). The bad-value message uses `%q`. `WithMaxDistance` is applied after `Blast` (`main.go:375-377`). TestBlastDepth1MatchesV1 passes. M8 depth counters are in 17-03-SUMMARY |
| 4 | Baseline equivalence: daemon view = `blast --base`; baseline = Report only; rebase installs the last published Report atomically | **holds** | The same `Between` and presenters are used (`render.go`). Product code is unchanged since the phase 16 re-audit (26 mutants, 16-SECURITY). TestDaemonBlastEqualsBlastBase and TestBaselineHoldsOnlyReport pass. TestCorpusDaemonSample on 3 corpora (17-04-SUMMARY) ran a rebase cycle and R8 through a real daemon. The macos flake (#440) was a test race, fixed in ae1c1d0 |
| 5 | Escape pipeline: escapeTerm, escapeJSON, strconv.Quote, SanitizeReason | **holds** | In `presenter/blast.go`, every write that is not escaped is an integer, a fixed literal, a `diagnostic.Code`, a `typeToken` entry or a fixed prefix. `DaemonBaselineLabel` holds only fixed `Source` strings (`watch.go:334,377`) and is escaped at `:160`. GRT004 messages quote every name (`grt004.go:101-104`). Daemon-supplied strings go through `SanitizeReason` (`report.go:133,136,176,192`). `report --blast` writes the daemon's pre-rendered view as is (`report.go:197-202`). That relies on the owner-checked 0700/0600 boundary (both levels checked, `report.go:109-118`), as documented in phase 16. TestTextOutputsEscapeControls and TestReportRefusalSanitised pass |
| 6 | for-expression guard: every `Value`/typeexpr site is guarded; exact (file, function) set | **holds** | The grep returns exactly `hclconv/literal.go:26` LiteralBool (guard `:23`), `terragrunt/eval.go:28` evalPath (guard `:25`), `terragrunt/eval.go:76` literalString (guard `:73`) and `tfsurface/typesig.go:161` typeAttrSig (guards `hasForKeyword` `:155` for JSON and `HasForExpr` `:158` for native). The set matches T-17-05-6. The diff since 57d74d8 adds no HCL evaluation |
| 7 | Bounded cost: O(units+edges+surface), 64 MiB cap, seal once per publish | **holds (recorded; HEAD CI pending)** | TestBlastRequestAllocsConstant and TestSealBlast pass locally. The size-bound tests are CI only. The last complete green `test` step was run 38236401050 (d3b71c4). The product code since then differs only in the usage string and a comment. The `blast --base` second tree costs one more `checking.Check` (`blasting.go:56`), so the cost is linear in base plus cur, with no cross product. `RemovedOutputs` does one map build plus map lookups per reference |
| 8 | No regressions versus v0.3.0 | **holds** | The recorded gate gives compare-ref 256/256, including the three corpora with stderr and exit codes, and an unchanged tree digest (17-03). compat-v1.sh was 7/7 at phase 16. No check/graph/report output code changed after the gate run |
| 9 | No-net/no-exec on 6 targets; HCL only in infrastructure; typeexpr only in tfsurface | **holds** | `check-architecture.sh`: OK (4 domain, 5 application, 1 interfaces). typeexpr is imported by tfsurface only. In phase 17, `os/exec` appears only in `_test.go` files |
| 10 | Docs = shipped behaviour | **holds** | TestRuleRegistryDoc, TestHelpMatchesDocs, TestTopUsageMatchesDocs (new, ed1ce2f), TestDaemonDocs, TestCIDoc and TestValidationDocPins pass. Exception: `docs/validation.md:873` says corpus.yml "runs the same two scripts", but it cannot yet pass (F1) |

## Scope items the orchestrator named

- **Blast core.** grt004.go, `blasting.Between`, `impact` and the tfsurface facts. See rows
  1-3 and 6. No change since the phase 13-15 audits other than what those audits covered.
- **Socket ops.** ipc v2, rebase, instance, report and the rebase CLI are byte-identical to
  the phase 16 re-audit (57d74d8). That audit covered 26 mutants, the canonical request,
  seal-at-publish, ErrOutcomeUnknown and the refusal sanitising. I re-ran the refusal and
  escape tests above.
- **`blast --base` second tree.** `openRepo(*base)` is `os.OpenRoot` (`main.go:213-219`).
  - Both loaders read through `root.FS()`, so a symlink inside the base tree cannot leave it.
  - The `--base` argument itself may be a symlink. That is the user's own path.
  - A `--base` that cannot be opened exits 3 and never falls back to "no baseline"
    (`main.go:357-368`). The label is escaped (`blast.go:160`).
  - The second tree adds one `checking.Check`, bounded by the same phase 15 depth and
    for-expression guards. A base nested inside cur is read twice, which is at most 2x and
    linear.
- **Phase 16 instance store, lock and windows dump.** No change since 57d74d8.
  - `ReadDump` is capped at responseCap+1 (`dump.go:60-66`) and is read only while the lock
    is held (`report.go:246-256`).
  - `ValidBlastView` runs on the file path (`report.go:258-261`).
  - `WriteDump` drops an over-cap view without touching the caller's snapshot
    (`dump.go:39-48`).
- **17-02 per-item exclusion table.** Accepted in #454. My 3 LOW items (#453) are in
  8abad29:
  - `categoryBinding` (`corpus_model_test.go:2023`) is asserted for every covered diff
    (`:1819`);
  - `corpusDerived` is pinned (`:2041`, `:1845`);
  - the eligibility gap is pinned by reason (`:1858`);
  - catRefOther pairs count as excluded_by_touch in M1/M2 (17-03: denis256 M1 touch 8).
- **Phase 17 scripts and workflow.** See the threat register and F1/F3.
  - `fetch-corpora.sh`: `g()` isolation (`:101-112`); manifest validated before any git
    call (`:60-88`); escaping and absolute links fatal (`:136-162`); `find -P -type f`
    strip outside `.git` (`:171-184`); `mv -T` after verify. Its stdout carries only fixed
    text and `$dest/$name`, so the `GITHUB_ENV` append in corpus.yml cannot be steered by
    corpus content.
  - `corpus-gate.sh`: the /proc/net/dev proof is independent of the `CORPUS_GATE_INNER`
    marker (`:53-62`), so setting the marker by hand cannot fake `isolated`. Pinned tag sha
    (`:98-105`), EXIT trap, `ulimit -v` and `timeout` per shard, and a `--- SKIP` shard
    counts as a failure (`:153-156`).
  - `compare-ref.sh` corpus rows: `realpath -e` then `-d` before the build; corpus rows
    compare stdout, stderr and exit; the digest never follows a link.

## Threat register

| ID | Disposition | Status | Evidence |
|----|-------------|--------|----------|
| T-17-00-1 | mitigate | CLOSED | Presentation-only SUMMARY edits, with a word-diff in 17-00-SUMMARY. verify-summary passes 17/17 (8f7f441) |
| T-17-00-2 | mitigate | CLOSED | Each 15/16 VALIDATION row cites a local command or a CI run id (6ccee45). The one flaky row, 16-04-01, was reported (#440) and fixed with a red test (ae1c1d0) |
| T-17-00-3 | mitigate | CLOSED | The `git diff 57d74d8..HEAD -- internal/infrastructure/watch/run.go` diff is comment-only (`:129-135`) |
| T-17-00-4 | mitigate | CLOSED | No windows test change. test-os windows was green on 4 consecutive runs (17-00-SUMMARY) |
| T-17-00-5 | mitigate | CLOSED | `.planning/config.json` has `create_tag: false`. `git tag --points-at HEAD` is empty and the latest tag is v0.3.0 |
| T-17-01-1 | mitigate | CLOSED | Fetch by sha, then `rev-parse HEAD` == sha and `cat-file -t` == commit before `mv -T` (`fetch-corpora.sh:124-131,211-214`). Self-test case 2 |
| T-17-01-2 | mitigate | CLOSED | `g()` (`:101-112`): system and global config off, empty `--template`, hooksPath=/dev/null, fsmonitor off, submodule.recurse off, LFS smudge skipped, only https. The script never cds into a corpus. Self-test cases 6 and 7 |
| T-17-01-9 | mitigate | CLOSED | Absolute, unresolvable and escaping links exit 1 (`:136-162`, case 8). The compare-ref digest uses `find -P`, hashes `-type f` and records links as `%l`. The link-shape row was 248/248 (17-01-SUMMARY) |
| T-17-01-10 | mitigate | CLOSED | `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`, `GIT_LFS_SKIP_SMUDGE=1`, `protocol.allow=never` with only https allowed. filter= attributes are reported |
| T-17-01-3 | mitigate | CLOSED | `find -P … -type f` under the working tree with `.git` pruned. Temp dir plus `mv -T`. The EXIT trap removes `$tmp`. Cases 1-3 |
| T-17-01-4 | mitigate | CLOSED | compare-ref digest before and after (`DIFF corpus/<name> digest`, fails the run). corpusDigest in TestCorpusBlastSelf. Copies go to `t.TempDir` |
| T-17-01-5 | mitigate | CLOSED | `realpath -e --`, quoted expansions, `cd --`. Clean-fixture row |
| T-17-01-6 | mitigate | CLOSED | The COMPARE_REF_PERTURB row exits 1. The TestCorpusBlastSelf non-vacuity mutant fails all 3 corpora (17-01-SUMMARY) |
| T-17-01-7 | mitigate | CLOSED | compare-ref exports `GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local`. Proof by failure: run 38263198748 could not fetch a module offline (F1) |
| T-17-01-8 | accept | CLOSED | denis256 had 30 ELF files (1,856,441,161 B) stripped. Measured RSS was 181 MB for TestCorpusBlastSelf |
| T-17-02-1 | mitigate | CLOSED | Accepted in #454. The model imports no internal/HCL package (arch OK). `expectBlast` never reads `graph --json`. Direction rule, and exclusion removes and never corrects. The category to reason binding is enforced (8abad29) |
| T-17-02-2 | mitigate | CLOSED | The included-module counts are pinned: 62/62, 3/3 and 795/818. Uncovered differences, stale rows and unknown categories fail. The eligibility gap is pinned by reason |
| T-17-02-3 | mitigate | CLOSED | assertSurfaceUnchanged and counters > 0. The parse-breaking reformat trips the guard (TestCorpusHarnessSelfTest, passing locally) |
| T-17-02-4 | mitigate | CLOSED | The self-test drives the real `runMutation` with 22 fault subtests, and compares exit, summary, version, kind and baseline. Comparator and caught++ mutants fail (17-02-SUMMARY). It passes locally |
| T-17-02-5 | mitigate | CLOSED | `confined` (`corpus_edit_test.go:191-232`): root-relative only, Lstat per component, no symlinked component, and EvalSymlinks of the parent and of a materialised link inside the real root. Restore re-checks sha256. Refusal rows are in the self-test |
| T-17-02-6 | mitigate | CLOSED | Phase 15 guards (row 6). `timeout`/`-timeout` on every run. One corpus per shard |
| T-17-02-7 | mitigate | CLOSED | oracleMask, plus the op-level graph check in the self-test. 94c580b fixed the heredoc kinds |
| T-17-03-1 | mitigate | CLOSED | Expectations come from the descriptor and the model only. M1 has a dual oracle against v0.3.0. SENTINEL is the only gruntled-derived constant, hand-checked |
| T-17-03-2 | mitigate | CLOSED | `corpusMutationPins` (`corpus_mutation_test.go:592`) pins sites, excluded_by_touch and the included modules. caught == sites in the record |
| T-17-03-9 | mitigate | CLOSED | The four exactness exceptions are named in docs/validation.md and pinned by TestValidationDocPins (passing) |
| T-17-03-3 | mitigate | CLOSED | `corpus-gate.sh:98-105` checks the pinned sha before building. `checkRefBin` requires the label plus the `rebase` and `--depth` fingerprint (`corpus_mutation_test.go:90+`). A missing binary fails |
| T-17-03-4 | mitigate | CLOSED | `exec unshare -rn` first. The /proc/net/dev proof is independent of the marker. `CORPUS_GATE_NONET=1` is fatal (proved by run 38262035294). The record shows `network=isolated` |
| T-17-03-5 | mitigate | CLOSED | The gate exports GOMAXPROCS/GOFLAGS. Serial shards, 1500 MB floor, `ulimit -v 4000000` (`:151`), timeouts |
| T-17-03-6 | mitigate | CLOSED | `t.TempDir` copies, sha256 restore, a digest guard every n edits, and the checkout digest before and after |
| T-17-03-7 | mitigate | CLOSED | In the gate shards, the only external exec is GRUNTLED_REF_BIN (`runBin`, `corpus_mutation_test.go:74-87`), built from the pinned tag. See F5 for the phase 5 terragrunt oracle, which is outside the gate |
| T-17-03-8 | mitigate | CLOSED | `trap cleanup EXIT` plus INT/TERM/HUP turned into exits: `worktree remove --force`, `prune`, `rm -rf`. The 17-03 record shows only master in the worktree list |
| T-17-04-1 | mitigate | CLOSED | `corpusDaemon.stop`: TERM, 5 s, then KILL, and Wait. Clients time out after 20 s. `assertNoProcessMentions` scans /proc |
| T-17-04-2 | mitigate | CLOSED | The phase 16 exact-label normaliser is reused (`normaliseDaemonBlast`). The off-by-one mutant is recorded at 16-04 |
| T-17-04-8 | mitigate | CLOSED | `persist-credentials: false`, setup-go `cache: false`. Pinned by TestCIDoc (passing) |
| T-17-04-3 | mitigate | CLOSED | `permissions: contents: read`, no `secrets.`, actions pinned by sha to the same releases as ci.yml (TestCIDoc). The new `sudo sysctl` step is assessed in F3 |
| T-17-04-4 | mitigate | CLOSED | fetch-corpora verifies each sha, and the require helpers fail on another commit |
| T-17-04-5 | mitigate | CLOSED | Nothing from a corpus is executed in corpus.yml: the fetch is isolated and the gate shards run only gruntled and the ref binary. The job holds no secrets |
| T-17-04-6 | mitigate | CLOSED | TestValidationDocPins pins package-level values shared with the gated tests (passing) |
| T-17-04-7 | mitigate | CLOSED | TestCIDoc asserts that ci.yml has no corpus env or gate call (passing) |
| T-17-05-1 | mitigate | CLOSED | This review (gruntled-sec) and 17-INTEGRATION.md (gsd-integration-checker) were not written by the executor |
| T-17-05-2 | mitigate | CLOSED | The invariant table above, checked on 7bf149c |
| T-17-05-3 | mitigate | CLOSED | The fixes 0682227..7bf149c touch test code, CI, docs and the top-level usage text only. None touches check, graph or report output, so no gate re-run is due. A fix for F1 is CI-only |
| T-17-05-4 | mitigate | CLOSED | `create_tag: false`. No tag at HEAD |
| T-17-05-5 | mitigate | CLOSED | This audit ran one targeted step at a time after `free -m`, under `ulimit -v`. No -race, fuzz or gate |
| T-17-05-6 | mitigate | CLOSED | The exact (file, function) set and a guard in each function (row 6) |
| T-17-05-7 | mitigate | CLOSED | Every scope item is covered above: the `blast --base` tree, the instance store, lock and dump, the exclusion table, and the phase 17 scripts and workflow |

## Integration findings re-checked

| Integration ID | Fix | Status |
|---|---|---|
| F1 (staticcheck red) | 0682227: S1011 `append(m.Sites, mu.Sites...)`, S1016 `oracleSource(unit)`, removal of the unused `served`. All three preserve behaviour | **Closed.** The CI `staticcheck` step is green on 7bf149c (run 38263188666) |
| F2 (corpus.yml cannot pass) | 1a6467b (TestCIDoc pins the sysctl before the gate), 78fa801 (the step), 7bf149c (docs); then 4c42aab/acf2759 (module download) | **Fixed; closes on corpus run 38264937436.** Isolation works (run 38263198748), and the empty module cache is filled by the new step |
| F3 (no CI proof for row 7) | follows F1 | **Closed.** CI run 38264926783 on a11dc37 is green, including `test` |
| F4 (topUsage report line) | ed1ce2f (red: TestTopUsageMatchesDocs), 5ee63ff (`main.go:56`, docs/cli.md) | **Closed.** The test passes |

## Findings

| # | Severity | Owner | Ref | Finding | Disposition |
|---|----------|-------|-----|---------|-------------|
| F1 | MEDIUM | executor | `.github/workflows/corpus.yml:31-47`, `scripts/corpus-gate.sh:65,119` | **corpus.yml still cannot pass (integration F2 only half fixed).** Run 38263198748 at 7bf149c prints `corpus-gate: network=isolated (/proc/net/dev: lo)`, so the sysctl works. The reference build then fails with `module lookup disabled by GOPROXY=off` for hcl/v2, go-cty and fsnotify. setup-go has `cache: false` and no step fills the module cache before the isolated, offline gate. The job fails closed, so this is no security hole. But `docs/validation.md:873` ("runs the same two scripts") is still untrue, and the REL-03 weekly regression net does not exist | REQUIRED in 17-05 Task 2. First, red: TestCIDoc requires a networked `go mod download` step (followed by `go mod verify`) after checkout and before the gate step. Then add the step, keeping `cache: false` and `GOFLAGS=-mod=readonly`; go.sum verifies the downloads. go.mod and go.sum are identical at v0.3.0 and HEAD, so one download serves both builds and the test deps. Re-dispatch and record the green run id in validation.md and 17-05-SUMMARY |
| F2 | INFO | orchestrator | CI run 38263188666 | Integration F1 and F4 are closed. Integration F3, the row 7 CI proof, depends on the `test` step of 38263188666, which was in progress when this was written | Cite the run when it completes green |
| F3 | INFO | none | `corpus.yml:40-43` | `sudo sysctl -w kernel.apparmor_restrict_unprivileged_userns=0` lifts the AppArmor userns restriction for the rest of the job. This grants nothing new: the runner is ephemeral and already has passwordless sudo, the token is `contents: read`, there are no secrets, credentials are not persisted, and the gate shards run no corpus content. The step is needed for the network-isolation proof, and `CORPUS_GATE_NONET=1` still fails closed | Accepted. No action |
| F4 | INFO | executor (optional) | `ipc/ipc.go:507,512`, `watch/ignore.go:66` | **Windows staticcheck U1000 (the orchestrator's question): dead code on that build target only, not a security concern.** `readReply` and `noReplyError` are called only from `sock_unix.go:320` (`linux \|\| darwin`). `editorPattern` is called only from `native_unix.go:160,246` (`!windows`). On windows there is no socket client: report reads the dump through `ReadDump`, which applies the same cap (`dump.go:60-66`) and its own error mapping. The poll watcher uses `IgnoredEntry`. No check is missing on windows. CI runs staticcheck on linux only | Optional: move the three functions into the unix-tagged files, so a windows staticcheck is clean |
| F5 | INFO | orchestrator | `cmd/gruntled/corpus_graph_test.go:796-815` (phase 5, outside the v0.4 diff) | T-17-03-7 and T-17-04-5 say nothing from a corpus is executed. That holds for the gate and for corpus.yml: their shards are anchored (`^TestCorpus…$`) and never select it. But the env-gated phase 5 terragrunt oracle runs `terragrunt run --all … -- version` on corpus copies, and Terragrunt can run hooks a corpus defines. It is not reachable from the gate, ci.yml or corpus.yml | Wording only: scope the "nothing executed" claim to the gate and the workflow if it appears in validation.md. No code change |
