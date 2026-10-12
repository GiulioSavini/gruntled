---
phase: 16-daemon-baseline-report-blast-rebase
audited: 2026-10-12
auditor: gruntled-sec (code-audit #388-#392; re-audit of e685454..57d74d8)
scope: "git diff 56c0e39..f05cecc -- ':!.planning'; re-audit e685454..ce737ea"
verdict: SECURED
threats_total: 45
threats_mitigated: 42
threats_accepted: 3
threats_na: 0
threats_open: 0
findings: { blocker: 0, high: 0, medium: 0, low: 1, info: 5 }  # F1 and F2 fixed in 0efc333/ce737ea
---

# Phase 16 Security

## Re-audit (e685454..57d74d8)

Verdict **SECURED**, 0 open threats. F1 (LOW) and F2 (INFO) are fixed. T-16-03-8 is closed.

- **F1** (`report.go:132-136`, 0efc333).
  - A new `case errors.As(err, &rerr)` prints `presenter.SanitizeReason(rerr.Msg)` and exits 3.
  - The generic branch now prints `SanitizeReason(err.Error())`.
  - TestReportRefusalSanitised (e685454, red first) sends ESC and C1 in a `malformed` refusal.
    It checks text, json, `--blast` and `--blast --format json`: stderr exact, no raw control,
    exit 3, empty stdout.
  - Mutation:
    - removing `SanitizeReason` from the refusal case is caught by all four subtests;
    - removing the new case is caught, because the generic text differs;
    - removing `SanitizeReason` from the generic branch **survives**. That is defence in depth
      and no test pins it. No daemon-controlled raw text reaches that branch: refusals take the
      new case, and json and syscall errors quote or are fixed. No action needed.
- **Test placement.** The F1 rows are in TestReportRefusalSanitised (`fakev1_unix_test.go`,
  linux||darwin) instead of TestReportBlastRefusals. This is acceptable. A refusal exists only
  over the socket, and only the raw-syscall fake daemon in that file can send an arbitrary one.
  The windows report-file path has no refusal: its daemon strings are LastError and the view
  error, already covered by M22, M26 and TestReportBlastRefusals. `--format sarif` is not a
  row, but it takes the same branch.
- **F2** (`compat-v1.sh:139`, ce737ea). After the `wait`, `stop()` runs
  `unset 'pids[${#pids[@]}-1]'`, so `cleanup` never signals a reaped pid. Indices stay
  contiguous because only the last element is removed. The empty-array expansion in `cleanup`
  is safe under `set -u`. A run alone gave 7/7 with no daemon left, the worktree removed, and
  master clean.
- **Tests.** `go test -short -run 'TestReport|TestRebase|TestScripts' ./cmd/gruntled` passes.
- **Docs** (57d74d8). The executor's T-16-03-8 row is accurate and is kept.

## Initial audit verdict (f05cecc, superseded by the re-audit above)

**OPEN_THREATS (0 blocking).** One LOW finding and five INFO. 41 of the 42 mitigate rows are
closed with file:line evidence, and the important ones are also closed by mutation. T-16-03-8 is
only partly closed: `report` (plain and `--blast`) still prints the text of a daemon refusal
raw (F1). This was already the case in v0.3.0 for plain `report`. Only the socket owner's uid
can reach it.

All nine plan-review findings (F1-F9, #348-#356) and the INFO notes #357/#372 are implemented as
answered in #360-#369:
- Seal at publish (F1).
- Canonical request (F2).
- Atomic commit and rebase-first build (F3).
- EventFailed race interleave and full-field walk (F4).
- `ValidBlastView` (F5).
- `ErrOutcomeUnknown` (F6).
- Exact label normaliser (F7).
- compat-v1.sh trap (F8).
- Fuzz log tee (F9).

## Method and baseline at f05cecc

- **CI.** Run 38225491068 on f05cecc is green, including `go test -race ./...` and the windows
  jobs. That run is the evidence for the -race-only and windows-only rows. Locally there was no
  -race, no -fuzz and no bound or memory test, per the RAM rule.
- **Local tests** (`GOMAXPROCS=2 -p=2`):
  - `go test -short` passes on `ipc` and `presenter`.
  - In `cmd/gruntled`, `-run 'Rebase|ReportBlast|Publisher|Instance|Baseline|Daemon|Report|Watch|NoGRT004|Normalise|Compat|CIDoc|Scripts' -skip 'Memory|SizeBound'` passes.
  - `go vet ./cmd/... ./internal/...` is clean, and so is
    `GOOS=windows go vet ./cmd/gruntled ./internal/infrastructure/ipc`.
- **Architecture.** `check-architecture.sh` is OK. `go list -deps ./cmd/gruntled` on
  linux/darwin/windows x amd64/arm64 shows 0 `net`, `net/*`, `os/exec`, `plugin` or
  `crypto/tls`. `golang.org/x/sys/unix` is there on unix, as it was at 56c0e39. `ipc` itself
  has no `x/sys`, `net` or `os/exec`. go.mod and go.sum are unchanged.
- **Evaluation sites.**
  `grep -rnE '\.Value\(|typeConstraint\(' internal cmd --include=*.go | grep -v _test.go` gives
  the same four lines as at 56c0e39: `hclconv/literal.go:26`, `tfsurface/typesig.go:161`,
  `terragrunt/eval.go:28` and `:76`. The diff adds no HCL parse or evaluation outside tests.
- **compat-v1.sh.** It was run alone: 7/7, and no daemon was left. It was then run again and
  interrupted with SIGINT once the v0.3.0 daemon was ready. The daemon was killed, the worktree
  and the temp dir were removed, the exit code was non-zero, and `pgrep` was empty afterwards.
  The repo is on master and clean.
- **Frozen v1 reference.** The blob hashes in `v1frozen_test.go:17-18` equal
  `git rev-parse v0.3.0:internal/infrastructure/ipc/{ipc,dump}.go`.

## Mutation (26 mutants, applied to the working tree one at a time and restored with `git checkout`)

24 were killed by the expected tests and 2 survived, both explained:

| # | Mutant | Result |
|---|--------|--------|
| M1 | `answer` ignores the sealed line (re-encodes per request) | killed: TestBlastRequestAllocsConstant/in_cap |
| M2 | drop the canonical byte-equality in `strictRequest` | killed: TestRespond expect_null, case_variants, duplicate_op |
| M3 | drop the trailing-value `dec.Token()` check | **survived**, an equivalent mutant: the canonical check rejects every trailing value |
| M4 | allow `expect` on any op | killed: TestRespond expect_on_report/blast |
| M5 | Seal never shrinks an over-cap view | killed: TestSealBlast, TestBlastRequestAllocsConstant/over_cap |
| M6 | Seal writes through `*s.Blast` in place | killed: TestSealBlast |
| M7 | Seal skips when already sealed (#372) | killed: TestDaemonRebaseSocket (stale line) |
| M8 | Rebase client never wraps ErrOutcomeUnknown | killed: TestRebaseOutcomeUnknown (cap, never answers, ok without rebase) |
| M9 | wrap refusals and version errors too | killed: TestRebaseOutcomeUnknown refused/version |
| M10 | WriteDump keeps an over-cap view | killed: TestWriteDumpDropsOversizeView |
| M11 | `ValidBlastView` accepts an empty view | killed: TestReportBlastRefusals (empty, text-only, json-only) |
| M12 | the report response carries the blast view | killed: TestRespond report_ready, TestReportResponseHasNoBlast |
| M13 | rebase ignores `--expect` | killed: TestPublisherRebase, TestRebaseCLI |
| M14 | rebase over a `Blast.Error` view | killed: TestPublisherBetweenError |
| M15 | rebase over a failed snapshot | killed: TestPublisherRebase |
| M16 | `lastRep` set before the snapshot build (not atomic) | killed: TestPublisherBuildSnapshotError |
| M17 | rebase moves the base before building its view | killed: TestRebaseRenderError |
| M18 | EventFailed copy of `*p.last` outside `p.mu` | **survived** without -race, which is expected. Only `TestRebaseConcurrentPublish` under -race catches it, and CI ran it green on f05cecc |
| M19 | the report-file instance accepts rebase | killed: TestInstanceRebaseHook/report_file |
| M20 | no windows check in rebase | killed: TestRebaseWindowsUnsupported |
| M21 | report --blast ignores TooLarge | killed: TestReportBlastRefusals linux and windows too_large |
| M22 | the CLI prints the refusal message unsanitised | killed: TestRebaseRefusalsCLI/failed |
| M23 | a nil hook gives `internal` instead of `indexing` | killed: TestInstanceRebaseHook/socket |
| M24 | `instance.store` does not seal | killed: TestInstanceStoreSeals (socket and report file) |
| M25 | `blastViewOrError` returns nil | killed: TestPublisherBetweenError |
| M26 | daemon stores LastError unsanitised | killed: TestPublisherSnapshots |

## Checks the orchestrator asked for

- **Request decoding** (`ipc.go:306-366`).
  - The version check runs first and leniently (`:312-320`).
  - Then `strictRequest` (`:348-366`) applies DisallowUnknownFields and accepts a single value.
  - `expect` is accepted on rebase only.
  - Last comes the byte-equality with `json.Marshal(req)`. It rejects duplicate keys, case
    variants, `null`, whitespace and `\u` escapes.
  - Every refusal is typed: `malformed`, `unknown_op`, the version error, or the handler's
    code. A handler error that is not a refusal becomes `internal` with a fixed message.
  - The request is capped at 4 KiB. M2 and M4 are killed; M3 is an equivalent mutant.
- **Seal** (`ipc.go:441-469`, called once from `instance.store`, `instance.go:197`).
  - Each published snapshot is encoded once. A rebase candidate and the EventFailed copy each
    pass through `setSnapshot` and are sealed once, so the cost is one encode per publish.
  - Over the cap, it takes one full encode plus one small one, and swaps in a copy of the view
    (`v := *s.Blast`). It never writes `*s.Blast`. M6 is killed.
  - `answer` (`:476-500`) writes the shared line, so a request is O(1). M1 is killed for the in-cap case.
  - Over the cap, the sealed line is the small TooLarge line, so a request is O(1) there too.
  - `answer` clears `blastLine` only on the per-request struct copy made in
    `snapshotResponse` (`:371-392`), never on the shared snapshot.
- **190b82d race slack.** The bound is `64 KiB + 256 KiB`, and only under `-race`
  (`race_on_test.go`). One per-request encode of the 2 MiB in-cap view costs about 2.8 MB per
  request, or about 56 MB over 20 requests. That is 175 times the loosened bound, so the slack
  cannot hide a per-request encode. In the over-cap subtest, a per-request encode of the
  shrunken line is small by construction. The mutant that matters there is "Seal keeps the full
  view", and M5 kills it at 0 slack. The bound is still sound.
- **Publish and rebase** (`watch.go:322-394`).
  - `commitReady` builds the snapshot and the view under `p.mu`. It commits `lastRep`, `gen`,
    the base and `setSnapshot` together, and only after the build succeeds (M16).
  - `rebase` takes the same mutex. It refuses with `indexing`, `failed`, `unavailable`
    (`Blast == nil || Blast.Error != ""`, `:372`) and `expect_mismatch` against
    `last.Generation` (`:374`).
  - It builds its view before it touches the base, and a build error gives `internal` with
    nothing changed (M17).
  - stdout and the status file are written outside the mutex (`:309`).
  - The report-file instance refuses rebase with `unsupported` (`instance.go:177`, M19).
- **T-13-02-7 (GRT004 never in the baseline).**
  - The base is assigned only from `rep` (the EventReady Report) and from `p.lastRep`.
  - Production code creates GRT004 only in `analysis.SupersedeUnknownOutputs`, through
    `blasting.Between` (grep), and never in the Indexer.
  - TestRebaseBaselineNoGRT004 walks the base after a rebase over a view that holds GRT004.
- **Outcome unknown** (`sock_unix.go:270-321`). `exchange` sets `sent` only after a full
  write. Every failure after that which is not a decoded `RefusedError` or `VersionError` is
  wrapped in ErrOutcomeUnknown (`:281`): timeout, EOF, a truncated reply, an over-cap reply,
  garbage, or ok without a result. A failed or partial write cannot be acted on, because the
  server's `readLine` (`ipc.go:288-301`) needs the newline and refuses a line that ends in EOF.
  M8 and M9 are killed.
- **Windows.**
  - `WriteDump` (`dump.go:39-48`) drops the blast bytes from an over-cap dump and keeps the
    caller's snapshot (M10). The dump size check and `ReadDump` use the same cap.
  - `report --blast` exits 3 on TooLarge (`report.go:185`, M21) and validates the view read
    from the file (`report.go:255`).
  - rebase exits 3 before anything else when `goos == windows` (`rebase.go:68`, M20).
  - The windows CI jobs are green.
- **compat-v1.sh.** `trap cleanup EXIT INT TERM HUP` (`:71`). Each daemon's pid comes from a
  plain `&`. The cleanup sends TERM, waits up to 5 s, sends KILL and waits, then runs
  `worktree remove --force`, `prune` and `rm -rf`, each step with `|| true`. It never reports
  success after an interruption. Each client runs under `timeout 20`, `GOTOOLCHAIN=local` is
  set, and the `pgrep` matrix line is there. Verified by a real SIGINT (above). For one
  leftover, see F2.
- **The three messages the executor wrote** (16-03 deviation 4, `rebase.go:95-147`).
  - Daemon `unsupported` is a fixed sentence.
  - An unknown code prints `SanitizeReason(msg)`. A repro with ESC in the message printed
    `[2Jpwn` with the ESC removed.
  - The pre-send failure is reached only when `sent` is false, so "cannot reach" is accurate.
    It prints `<root>` raw, like the existing `noDaemon` and report messages (see F1).
- **LastError in the `failed` refusal** (`watch.go:371`). It is sanitised once at publish
  (`s.LastError = presenter.SanitizeReason(reason)`, M26) and again in `refusalMessage` (M22).
  JSON escapes it on the wire. SanitizeReason maps C0, DEL, C1, format runes and separators
  to spaces.
- **Docs** (`docs/cli.md`, README). They match the behaviour:
  - the trust boundary: the 0600 socket in an owner-checked 0700 dir, client CheckDir on both
    levels (`rebase.go:84`), and peer credentials not checked;
  - the 64 MiB blast cap decided once per index, and the windows dump drop;
  - in-memory-only rebase with no generation change;
  - every refusal exits 3 with nothing changed;
  - the 10 s outcome-unknown message;
  - the GRT004/GRT001 note after a rebase;
  - the help texts, pinned by TestHelpMatchesDocs and TestDaemonDocs.
- **run.go:128-131 republish branch.** See F3. It is no risk to T-13-02-7 and adds nothing to rebase.
- **Phase 15 lesson.** No new evaluation site (grep above). No new cost proportional to
  request bytes: the request is at most 4 KiB, canonical, and costs O(1). Rebase costs O(repo)
  per request, not O(input), and requests are serialised (F4).

## Threat register

| ID | Disposition | Status | Evidence |
|----|-------------|--------|----------|
| T-16-00-1 | mitigate | CLOSED | TestTypeSigFormerFuzzSeeds shares `fuzzTypeSigOnce` with the fuzz target. The boundary seed is MaxNestingDepth+1 (16-00 SUMMARY) |
| T-16-00-2 | mitigate | CLOSED | `-fuzzminimizetime 200x` in `ci.yml`, pinned by TestCIDoc (green locally) |
| T-16-00-3 | mitigate | CLOSED | the `ci.yml` diff touches only the run block and comments: no `secrets.`, no `permissions:` |
| T-16-00-4 | accept | CLOSED | `timeout-minutes: 5` kept |
| T-16-00-5 | mitigate | CLOSED | output tee'd to `$RUNNER_TEMP/fuzz.log`. On failure, `tail -n 200` and `cat` of each FuzzTypeSig input. `pipefail` is set, so the exit code is the go test one. Pinned by TestCIDoc |
| T-16-01-1 | mitigate | CLOSED | protocol 2, version checked first (`ipc.go:312-320`). `decodeResponse` requires a snapshot for report and blast (`:422`) and a result for rebase (`:414`). `ValidBlastView` (`:175`) runs on both transports (`report.go:255`). M11 and M12 killed |
| T-16-01-2 | mitigate | CLOSED | `strictRequest` (`ipc.go:348-366`). M2 and M4 killed; M3 is an equivalent mutant |
| T-16-01-3 | mitigate | CLOSED | Seal at publish (`instance.go:197`, `ipc.go:441`), `answer` writes the shared line (`:478`). M1, M5, M6, M7 and M24 killed. The race slack does not hide a per-request encode (see above) |
| T-16-01-3b | accept | CLOSED | the retained sealed line is at most 64 MiB per live snapshot, replaced at each publish. Plain report unchanged from v1 |
| T-16-01-4 | mitigate | CLOSED | `dump.go:39-48`, M10 killed |
| T-16-01-5 | mitigate | CLOSED | a handler error that is not a refusal becomes `internal` with "internal error" (`ipc.go:334`). Refusal messages are the daemon's own fixed strings or the already-sanitised LastError |
| T-16-01-6 | mitigate | CLOSED | `exchange` returns `sent` after a full write. ErrOutcomeUnknown wraps every post-send failure that is not a refusal or version error (`sock_unix.go:281`). M8 and M9 killed |
| T-16-01-7 | mitigate | CLOSED | six targets: 0 forbidden packages. `ipc` has no x/sys. check-architecture OK |
| T-16-01-8 | mitigate | CLOSED | blob hashes match v0.3.0. compat-v1.sh 7/7 against the real v0.3.0 |
| T-16-02-1 | mitigate | CLOSED | one `p.mu` (`watch.go:295, 323, 363`) over the base, lastRep, gen, last, setSnapshot and the failed copy. M18 is caught only by -race, which CI ran green |
| T-16-02-2 | mitigate | CLOSED | `commitReady` is atomic (M16). `expect` is compared with `last.Generation` (M13). TestRebaseNeverSkippedReport passes |
| T-16-02-3 | mitigate | CLOSED | the base comes only from an EventReady Report. GRT004 is created only in `blasting.Between`. TestRebaseBaselineNoGRT004 passes |
| T-16-02-4 | mitigate | CLOSED | the base is typed `checking.Report`. TestBaselineHoldsOnlyReport walks every publisher field, with a Sources probe. The DAEMON-10 record ran in CI |
| T-16-02-5 | mitigate | CLOSED | `writeOut` and the status write come after the unlock (`watch.go:309-317`) |
| T-16-02-6 | mitigate | CLOSED | publish never waits on the server. TestShutdownWithRebaseInFlight passes |
| T-16-02-7 | mitigate | CLOSED | rebase has no write path. The dump instance refuses rebase (M19). TestPublisherRebase and TestInstanceRebaseHook pass |
| T-16-02-8 | mitigate | CLOSED | the label goes through BlastText escaping and the JSON through `escapeJSON`. The Between error is sanitised (`watch.go:349`) |
| T-16-02-9 | mitigate | CLOSED | `blastViewOrError` is never nil (M25) |
| T-16-02-10 | mitigate | CLOSED | the eval-site grep gives the same four lines |
| T-16-02-11 | mitigate | CLOSED | `unavailable` on `Blast.Error` (`watch.go:372`, M14). The view is built first and a build error changes nothing (M17) |
| T-16-02-12 | mitigate | CLOSED | TestInstanceStoreSeals (M24) |
| T-16-03-1 | mitigate (by boundary) | CLOSED | CheckDir on both levels before connecting (`rebase.go:83-91`). The docs state the boundary and the root residual |
| T-16-03-2 | mitigate | CLOSED | the accepted count is printed (`rebase.go:97`). TestRebaseCLI passes |
| T-16-03-3 | mitigate | CLOSED | `--expect` compare-and-swap (M13). `parseGeneration` accepts only `^[1-9][0-9]*$` and values that fit a uint64 |
| T-16-03-4 | mitigate | CLOSED | `rebaseFailed` maps ErrOutcomeUnknown to the "may or may not have moved" message (`rebase.go:115`). TestRebaseOutcomeUnknownCLI and TestRebaseTimeoutUnknown pass |
| T-16-03-5 | mitigate | CLOSED | `rebase.go:68` (M20) |
| T-16-03-6 | mitigate | CLOSED | no EnsureRepoDir and CheckDir only. TestRebaseWritesNothing and TestReportBlastNoDaemonCreatesNothing pass |
| T-16-03-7 | mitigate | CLOSED | VersionError, missing view and empty view all exit 3 on both transports (M11). compat-v1 7/7 |
| T-16-03-8 | mitigate | CLOSED (re-audit; F1 fixed in 0efc333) | sanitised: LastError (M22, M26), the view error (`report.go:188`), every rebase refusal, and now every report refusal (`report.go:131`, `presenter.SanitizeReason(rerr.Msg)`) and the generic query error (`SanitizeReason(err.Error())`). TestReportRefusalSanitised (fakev1_unix_test.go, red first in e685454): ESC and C1 in a `malformed` refusal for report text/json and report --blast text/json give exit 3, empty stdout, `gruntled: the daemon refused the request: [2Jpwn`, no raw control |
| T-16-03-9 | mitigate | CLOSED | TestDaemonDocs and TestHelpMatchesDocs pass. The docs claims were checked against the code (above) |
| T-16-03-10 | mitigate | CLOSED | the parity tests pass. compare-ref v0.3.0 was 244/244 at the executor gate, and the plain report path is unchanged apart from the warning refactor |
| T-16-04-1 | mitigate | CLOSED | TestNormaliseDaemonBlast passes. The executor's three mutants are recorded, including the off-by-one label |
| T-16-04-2 | mitigate | CLOSED | the property runs fresh and through the shared invalidating loader (TestDaemonBlastEqualsBlastBase, `-short`, green) |
| T-16-04-3 | mitigate | CLOSED | SEC-01 report --blast subtests (`escape_test.go`) pass |
| T-16-04-4 | mitigate | CLOSED | TestReportBlastSizeBound and TestDaemonMemorySynthrepo passed in CI. They were not run locally (RAM rule) |
| T-16-04-5 | mitigate | CLOSED | `-short` skips. This audit ran no heavy test |
| T-16-04-6 | mitigate | CLOSED | compat-v1.sh 7/7, with exact stderr in both directions |
| T-16-04-7 | mitigate | CLOSED | `GOFLAGS=-mod=readonly GOPROXY=off GOTOOLCHAIN=local`, `rev-parse --verify --quiet --end-of-options "$ref^{commit}"`, and the worktree removed by the trap |
| T-16-04-8 | accept | CLOSED | in-run control; absolute values are only logged |
| T-16-04-9 | mitigate | CLOSED | trap on EXIT INT TERM HUP, with TERM, a wait, then KILL before rm. A real SIGINT left nothing behind. See F2 for the pid list |

## Findings

| # | Severity | Ref | Finding | Disposition |
|---|----------|-----|---------|-------------|
| F1 | LOW | `cmd/gruntled/report.go:131-132` | The generic `report` error branch prints `%v` of the error. For a decoded refusal that is `RefusedError.Error()` = `"ipc: daemon refused the request: " + Msg`, with the daemon's message unsanitised. Repro (throwaway raw-socket test, removed): a reply `{"v":2,"ok":false,"error":"\u001b[2Jpwn\u009b","code":"malformed"}` makes `report --blast` and `report --format json` exit 3 with a raw ESC and C1 on stderr. The same reply to `rebase` is sanitised (`[2Jpwn`). T-16-03-8 says SanitizeReason runs on every daemon string. This was already true of v0.3.0 plain `report` (same branch), and only the socket owner's uid can serve the socket, so the impact is low | REQUIRED: in `runReportWith` add `case errors.As(err, &rerr):` before `err != nil`. It prints `gruntled: the daemon refused the request: ` + `presenter.SanitizeReason(rerr.Msg)` and exits 3. The generic branch should also print `presenter.SanitizeReason(err.Error())`. Add a TestReportBlastRefusals row (plain report and `--blast`) with ESC and C1 in a non-too_large refusal, asserting no raw control on stderr **Fixed** in 0efc333 (red test e685454): the refusal branch at `report.go:131` and the sanitised generic branch; the rows live in the new TestReportRefusalSanitised (fakev1_unix_test.go, linux and darwin), because a non-too_large refusal needs the raw-socket fake daemon, which report_test.go (all platforms) cannot use |
| F2 | INFO | `scripts/compat-v1.sh:128-138, 43-61` | `stop()` waits on (reaps) the last daemon but leaves its pid in `pids`, so `cleanup` sends TERM and KILL to a pid that may by then belong to an unrelated process of the same user. The window is the few seconds left in the script, and it needs PID reuse | Optional: after the `wait` in `stop()`, drop the pid (`unset 'pids[${#pids[@]}-1]'`), or keep a separate list of live pids **Fixed** in ce737ea: `stop()` unsets the reaped pid after its `wait`. compat-v1.sh v0.3.0 rerun alone: 7/7, `pgrep -fa gruntled` empty; SIGINT mid-run still leaves no daemon, worktree or temp dir |
| F3 | INFO | `internal/infrastructure/watch/run.go:128-131` | Republish branch (unreachable, 16-02 deviation 2). If it were reached, it would publish `last`, which is an Indexer Report and never holds GRT004, so T-13-02-7 is not affected. For rebase, it would be one more published result: visible in `report --blast` at its own generation, and pinned by `--expect`. That is the same exposure as any published result. The publisher side is already pinned by TestRebaseNeverSkippedReport | None required. A watch-package test asserting the branch is unreachable would only pin debouncer internals |
| F4 | INFO | `cmd/gruntled/watch.go:362-394` | Each rebase request costs O(repo + view) under `p.mu`: `Between(lastRep, lastRep)`, both renders, and a re-Seal through `setSnapshot`. The cost does not depend on the request bytes. Requests are serialised, only the owner uid can send them, and BLAST-11 bounds the cost. It is not a new amplifier proportional to attacker input | None required. Optionally, return early when the base is already `lastRep` from a rebase at the same generation |
| F5 | INFO | `ipc.go:449`, `dump.go:42`, `report.go:205-211` | TooLarge and the message "the blast view is N bytes" report the size of the encoded response line: base64 plus JSON, and in report-file mode the whole dump including the plain report. So N overstates the view by about 4/3 or more. The cap itself is enforced correctly | Optional wording: "the blast response is N bytes" |
| F6 | INFO | mutation | M3 (drop the trailing-value check) survives because the canonical byte-equality subsumes it: it is defence in depth. M18 (the failed copy outside the mutex) survives without -race and is caught only by CI -race, which ran green on f05cecc | None |
