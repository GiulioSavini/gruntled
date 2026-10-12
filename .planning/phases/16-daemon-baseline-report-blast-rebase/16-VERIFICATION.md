---
phase: 16-daemon-baseline-report-blast-rebase
verified: 2026-10-12T09:23:06Z
status: passed
score: 11/11 must-haves verified (5 roadmap success criteria + DAEMON-07, DAEMON-08, DAEMON-09, DAEMON-10 + the report --blast halves of SEC-01 and BLAST-11)
covered_files: [".github/workflows/ci.yml", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-00-PLAN.md", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-00-SUMMARY.md", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-01-PLAN.md", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-01-SUMMARY.md", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-02-PLAN.md", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-02-SUMMARY.md", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-03-PLAN.md", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-03-SUMMARY.md", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-04-PLAN.md", ".planning/phases/16-daemon-baseline-report-blast-rebase/16-04-SUMMARY.md", "README.md", "cmd/gruntled/baseline_test.go", "cmd/gruntled/blast_bound_test.go", "cmd/gruntled/ci_doc_test.go", "cmd/gruntled/daemon_blast_test.go", "cmd/gruntled/daemon_doc_test.go", "cmd/gruntled/daemon_mem_test.go", "cmd/gruntled/e2e_test.go", "cmd/gruntled/escape_test.go", "cmd/gruntled/fakev1_unix_test.go", "cmd/gruntled/instance.go", "cmd/gruntled/instance_test.go", "cmd/gruntled/main.go", "cmd/gruntled/rebase.go", "cmd/gruntled/rebase_test.go", "cmd/gruntled/render.go", "cmd/gruntled/report.go", "cmd/gruntled/report_test.go", "cmd/gruntled/testdata/script/rebase_usage.txtar", "cmd/gruntled/testdata/script/report_usage.txtar", "cmd/gruntled/testdata/script/usage.txtar", "cmd/gruntled/validation_doc_test.go", "cmd/gruntled/watch.go", "docs/cli.md", "docs/validation.md", "internal/infrastructure/ipc/compat_unix_test.go", "internal/infrastructure/ipc/dump.go", "internal/infrastructure/ipc/ipc.go", "internal/infrastructure/ipc/ipc_test.go", "internal/infrastructure/ipc/race_off_test.go", "internal/infrastructure/ipc/race_on_test.go", "internal/infrastructure/ipc/sock_other.go", "internal/infrastructure/ipc/sock_unix.go", "internal/infrastructure/ipc/sock_unix_test.go", "internal/infrastructure/ipc/v1frozen_test.go", "internal/infrastructure/tfsurface/typesig_test.go", "internal/interfaces/presenter/blast.go", "internal/interfaces/presenter/blast_daemon_test.go", "scripts/compat-v1.sh"]
covered_digest: "v3:sha256:8181b768d551485ca16e6a3dbed7a5f915d397b93c0eee564ebafda7b413f749"
behavior_unverified: 0
overrides_applied: 0
---

# Phase 16: Daemon Baseline, `report --blast` & `rebase` Verification Report

**Phase goal:** A user with `gruntled watch` running can ask what the current tree breaks and impacts against the baseline the daemon took at its first index, and can move that baseline deliberately.
**Verified:** 2026-10-12T09:23:06Z, code at master `a50d203`
**Status:** passed
**Re-verification:** No. This is the first verification.

Method: I read the production code (`ipc.go`, `dump.go`, `sock_unix.go`, `sock_other.go`, `watch.go`, `render.go`, `instance.go`, `report.go`, `rebase.go`) and the tests that carry each truth. I ran the targeted tests locally (`-short`, `GOMAXPROCS=2`, `-p=2`), ran both gate scripts, and read CI evidence for the tests that are CI-only. SUMMARY claims were not used as evidence.

## Goal Achievement

### Observable Truths (roadmap success criteria)

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | After a save, `report --blast [--format text\|json]` prints the daemon's Broken and Impacted against the baseline (the first successful index) and shows both generations. Apart from the label line or `daemon_baseline` object, its bytes equal `blast --base <copy of the baseline tree>` (property test), including hostile names (SEC-01 daemon half). | ✓ VERIFIED | `commitReady` (watch.go:322-339) sets `base` on the first committed EventReady and builds the view through `buildBlastView`, which uses `blasting.Between` + `presenter.BlastText`/`BlastJSONDaemon` (render.go:69-97). The test oracle is independent: `runCLI("blast","--base",D,C)`. The normaliser spells the label out with `fmt` and asserts the member's position and values. Local: `TestDaemonBlastEqualsBlastBase` PASS, 200 cases (broken 120, GRT004 42, distance ≥2 66, type changes 102). `TestReportBlastE2E` PASS: real daemon, both transports, 3 scenarios. `TestTextOutputsEscapeControls/report_--blast` and `/report_--blast_json` PASS: crafted C0/C1/U+202E/invalid UTF-8 names, `assertBlastEquivalent` against blast --base. |
| 2 | `report --blast` exits 0, or 1 iff Broken has an error. It exits 2 for `--format sarif` or `--depth`, and 3 for no daemon, indexing or a version mismatch. After a failed reindex it prints the previous view with the stderr warning. On windows it reads the view from the report dump. | ✓ VERIFIED | report.go:82-91 handles sarif and `--depth` (exit 2), :124-140 the exit-3 paths, :186-207 `printBlast` (exit 1 iff `v.HasErrors`, `warnFailed`), and :239-264 `queryDaemon` (dump path plus `ValidBlastView`). The EventFailed path keeps the previous `Blast` (watch.go:294-302). Local PASS: `TestReportBlastStates` (linux and windows goos: indexing, clean=0, broken=1, failed→previous view + warning), `TestReportBlastRefusals` (too large, view error, nil/empty/text-only/json-only views → 3, no raw controls), `TestReportBlastNoDaemonCreatesNothing`, `TestReportUsage`, `TestScripts/report_usage`. The real windows path is green in CI (see Spot-checks). |
| 3 | `rebase [--expect N]` moves the baseline to the last published report, in memory only. It prints the new generation and the accepted Broken error count. It refuses with exit 3 while indexing or failed or on an `--expect` mismatch, says the outcome is unknown on a client timeout, and exits 3 as unsupported on windows. It writes no file and never starts a daemon. `report --blast` then shows empty sets. The docs no longer call `report` read-only. | ✓ VERIFIED | `watchPublisher.rebase` (watch.go:362-387) runs under the same `mu` as `commitReady`, installs `p.lastRep`, leaves `gen` alone and writes nothing (no status or stdout call). The instance refuses rebase in report-file mode (instance.go:176-185). rebase.go: `--expect` is strictly `^[1-9][0-9]*$` (:151-162), windows exits before any filesystem access (:68-71), and every failure path is exit 3 with a fixed message (:103-148). After a send, `ipc.Rebase` wraps non-refusal failures in `ErrOutcomeUnknown` (sock_unix.go:270-286). report.go/rebase.go contain no exec, mkdir or file-create call. Local PASS: `TestPublisherRebase`, `TestRebaseCLI`, `TestRebaseRefusalsCLI`, `TestRebaseWindowsUnsupported`, `TestRebaseWritesNothing` (repo, runtime base and status file byte- and mtime-identical), `TestRebaseOutcomeUnknownCLI`, `TestRebaseTimeoutUnknown`, `TestDaemonRebaseSocket`, `TestShutdownWithRebaseInFlight`, `TestRebaseNeverSkippedReport`, `TestReportBlastE2E` (after rebase: `Broken (0):\nImpacted (0):`, `[]` lists, label `(rebase)`), `TestReportBlastPitfall15`, `TestDaemonDocs`. In docs/cli.md:1078-1080 the Read-only bullet now names rebase as the one changing op, and the `## gruntled rebase` section (:1125) documents the trust boundary, the timeout and windows. |
| 4 | The IPC protocol is version 2. A version mismatch is an explicit exit-3 error, never an empty answer. The compatibility matrix passes (fake v1 daemon × v2 client, v2 daemon × v1 request, v1 dump), and plain `report` bytes are unchanged. | ✓ VERIFIED | `ProtocolVersion = 2` (ipc.go:29). `respond` checks the version first (:310-320). `decodeResponse` returns `*VersionError` before any snapshot use (:403-405), and `ReadDump` does the same (dump.go:71-73). The ipc-level oracle `v1frozen_test.go` matches the v0.3.0 blob hashes, which I confirmed with `git rev-parse` (8ba5757…, 9cf0dbf…). Local PASS: the ipc package (incl. `TestCompatV1DaemonV2Client`, `TestCompatV2DaemonV1Request`, `TestCompatV1Dump`, `TestCompatV1DecoderRefusesV2Dump`, `TestReportResponseHasNoBlast`), the CLI rows `TestReportBlastV1` and `TestRebaseV1`, and `TestReportParity`/`TestReportParityClean`. Real-binary matrix: `bash scripts/compat-v1.sh v0.3.0` gave **compat-v1: 7/7**, and `pgrep -fa gruntled` afterwards showed no daemon. `bash scripts/compare-ref.sh v0.3.0` gave **244/244 identical**. |
| 5 | The baseline holds only the `checking.Report` (no loader, `fs.FS` or parse cache). A blast response over 64 MiB is an explicit exit-3 error naming the cap. Daemon memory on synthrepo at 65, 500 and 5,000 units is measured and recorded. | ✓ VERIFIED | `watchPublisher` fields (watch.go:240-266) hold only `checking.Report` values, plus bytes in `*ipc.Snapshot`. `checking.Report` is `{Graph, Diagnostics}` (check.go:18-25). `TestBaselineHoldsOnlyReport` PASS: a recursive type walk with an allowlist and a non-vacuity probe that rejects `blasting.Sources`. The cap: `Seal` turns an over-cap view into `TooLarge` at publish (ipc.go:441-464), `answer` backstops with code `too_large` naming `capString(responseCap)` (:488-494), and the CLI prints `... over the 64 MiB response cap ...` and exits 3 (report.go:209-216). Covered by `TestSealBlast`, `TestBlastResponseCap`, `TestWriteDumpDropsOversizeView`, `TestReportBlastTooLargeRefusal` and the `TestReportBlastRefusals/too large` case (local PASS). Memory: docs/validation.md:741-790 has the 65/500/5,000 table and the VALIDATION-BENCH lines. A local re-run (65, 500) gave ratios 2.29 and 2.53, within the asserted bound. The 5,000-unit run is CI-only (see below). |

**Score:** 5/5 success criteria verified, 0 behavior-unverified.

### Requirements Coverage

| Requirement | Source Plan | Status | Evidence |
|-------------|-------------|--------|----------|
| DAEMON-07 `report --blast` | 16-02, 16-03, 16-04 | ✓ SATISFIED | SC1 and SC2 above. The property, the e2e on both transports, the exit codes, the windows dump path and the failed-reindex view are all tested. |
| DAEMON-08 `rebase` | 16-02, 16-03 | ✓ SATISFIED | SC3. The request is fixed-shape (`strictRequest` plus a canonical byte-equality check, ipc.go:348-366). `maxRequest` stays 4 KiB (`TestRequestFitsMaxRequest`). The swap is serialised with publish (`TestRebaseConcurrentPublish`, run under `-race` in the CI check job). The trust boundary is documented (docs/cli.md:1159-1163). |
| DAEMON-09 IPC v2 | 16-01, 16-03, 16-04 | ✓ SATISFIED | SC4. The ipc, CLI and real-binary matrix rows pass, and compare-ref gives 244/244. |
| DAEMON-10 baseline + cap + memory | 16-01, 16-02, 16-04 | ✓ SATISFIED | SC5. The `report` op never carries blast bytes (`snapshotResponse` sets `Blast = nil` for OpReport, ipc.go:382-385; `TestReportResponseHasNoBlast`). |
| SEC-01, report --blast half (deferred from Phase 15) | 16-04 | ✓ SATISFIED | `TestTextOutputsEscapeControls/report_--blast` and `/report_--blast_json` PASS locally. The text label goes through `BlastText` escaping (`TestDaemonBaselineLabel`, ESC in the source is printed escaped). The JSON keeps the shared `blastJSON` path, and `TestBlastJSONUnchanged` pins the pre-change bytes. |
| BLAST-11, report --blast half (deferred from Phase 15) | 16-04 | ✓ SATISFIED (CI evidence) | `TestReportBlastSizeBound` (blast_bound_test.go:278) is a built-binary daemon on the 5,000-unit chain with a ~4 MiB type. Its only skip is `testing.Short()`, and CI runs `go test -race -count=1 ./...` without `-short`. `cmd/gruntled` was `ok` in the check job (311.96 s, linux, -race) and on macos (137.26 s) in run 38225491068 (f05cecc), and green again in run 38227195245 (a50d203). It was not run locally, as instructed. |

There are no orphaned requirements. REQUIREMENTS.md maps only DAEMON-07..10 to Phase 16, and every plan's `requirements:` covers them. 16-00 has `requirements: []`; it is the CI fuzz fix.

### Required Artifacts

| Artifact | Status | Details |
|----------|--------|---------|
| `internal/infrastructure/ipc/ipc.go` | ✓ VERIFIED | Has the v2 wire types, strict request, typed refusals, `Seal`/`answer` and `ValidBlastView`. Wired from sock_unix.go `handle` → `answer`. |
| `internal/infrastructure/ipc/compat_unix_test.go`, `v1frozen_test.go` | ✓ VERIFIED | The frozen oracle's blob hashes match `v0.3.0`. |
| `cmd/gruntled/watch.go`, `render.go` | ✓ VERIFIED | Hold the baseline state, the view per publish and `rebase`. `inst.setRebase(pub.rebase)` and `snapshots: inst.store` are wired before `watch.Run` (watch.go:172-186). |
| `internal/interfaces/presenter/blast.go` | ✓ VERIFIED | `DaemonBaseline`, `DaemonBaselineLabel` and `BlastJSONDaemon` are used by render.go:75-78. |
| `cmd/gruntled/rebase.go` | ✓ VERIFIED | Dispatched from main.go:163 and listed in topUsage (main.go:57). |
| `cmd/gruntled/daemon_doc_test.go` | ✓ VERIFIED | `TestDaemonDocs` PASS. |
| `cmd/gruntled/daemon_blast_test.go` | ✓ VERIFIED | Property, e2e and Pitfall 15 all PASS. |
| `scripts/compat-v1.sh` | ✓ VERIFIED | 7/7 against the real v0.3.0 binary, no leaked daemon, worktree removed (`git worktree list` shows only master). |
| `docs/validation.md` | ✓ VERIFIED | `## v0.4 daemon memory (DAEMON-10)` with a 3-row table. `TestValidationDocPins` PASS. |
| `internal/infrastructure/tfsurface/typesig_test.go`, `.github/workflows/ci.yml` (16-00) | ✓ VERIFIED | `TestFuzzTypeSigSeedsSmall` and `TestTypeSigFormerFuzzSeeds` PASS, `TestCIDoc/fuzz_step` PASS. In CI the fuzz step ran 30 s, 126,927 execs, and passed (run 38225491068). |

### Key Link Verification

| From | To | Via | Status |
|------|----|-----|--------|
| `respond` | `Handler.Rebase` | `h.Rebase(req.Expect)` (ipc.go:328) | WIRED |
| `handle` (OpBlast) | `Seal` | `answer` writes `r.Snapshot.blastLine` as is (ipc.go:478-480); `instance.store` calls `ipc.Seal` before `snap.Store` (instance.go:196-202) | WIRED |
| `decodeResponse` | "no blast view" | `ValidBlastView(r.Snapshot.Blast)` for OpBlast (ipc.go:424-428) | WIRED |
| `watchPublisher.commitReady` | `blasting.Between` | `blastViewOrError` → `buildBlastView` → `between(base, cur)` (render.go:70) | WIRED |
| `instance` (ipc.Handler) | `watchPublisher.rebase` | `ipc.ServeHandler(ln, inst.info, inst)` (instance.go:128) + `setRebase` (watch.go:179) | WIRED |
| `runReportWith --blast` | `ipc.Query(OpBlast)` / `ipc.ReadDump` | `queryDaemon(rt, goos, op)` (report.go:120, 239-264) | WIRED |
| `runRebaseWith` | `ipc.Rebase` | socket only (rebase.go:93) | WIRED |
| `TestDaemonBlastEqualsBlastBase` | `runBlast` | `runCLI(t, "blast", "--base", dDir, cDir)` | WIRED |

### Data-Flow Trace (Level 4)

| Artifact | Data | Source | Real data | Status |
|----------|------|--------|-----------|--------|
| `report --blast` stdout | `snap.Blast.Text/JSON` | `buildBlastView(base, lastRep)` from the `watching.Indexer` reports at publish → sealed line → socket, or WriteDump → ReadDump | Yes. The e2e asserts GRT004/distance/type-change lines from a real daemon. | ✓ FLOWING |
| `rebase` stdout | `RebaseResult{Generation, BrokenErrors}` | `last.Generation`, `last.Blast.BrokenErrors` read under `mu` before the swap | Yes. Pitfall15 asserts `1 error findings in Broken accepted`. | ✓ FLOWING |

### Behavioral Spot-Checks

| Behavior | Command | Result | Status |
|----------|---------|--------|--------|
| ipc package (v2, compat, cap, seal) | `go test -count=1 -short ./internal/infrastructure/ipc/` | ok 0.65s | ✓ PASS |
| Presenter daemon label/member | `go test -short -run 'TestDaemonBaselineLabel\|TestBlastJSONDaemonMember\|TestBlastJSONUnchanged' ./internal/interfaces/presenter/` | 3 PASS | ✓ PASS |
| Publisher, rebase, report and instance (38 tests) | `go test -short -run 'TestPublisher\|TestRebase\|TestBaseline\|TestInstance\|…\|TestReportBlast…' ./cmd/gruntled/` | all PASS (`TestReportBlastSizeBound` SKIP under -short) | ✓ PASS |
| DAEMON-07 property, SEC-01 half, memory (65/500), doc pins | `go test -short -run 'TestDaemonBlastEqualsBlastBase\|TestTextOutputsEscapeControls\|TestDaemonMemorySynthrepo\|TestValidationDocPins\|TestHelpMatchesDocs\|TestCIDoc' ./cmd/gruntled/` | all PASS; 5,000 units SKIP under -short | ✓ PASS |
| Usage txtars | `go test -short -v -run TestScript ./cmd/gruntled/` | `usage`, `report_usage`, `rebase_usage` PASS | ✓ PASS |
| Former fuzz seeds | `go test -short -run 'TestTypeSigFormerFuzzSeeds\|TestFuzzTypeSigSeedsSmall' ./internal/infrastructure/tfsurface/` | PASS | ✓ PASS |
| vet + architecture | `go vet` (3 pkgs); `bash scripts/check-architecture.sh` | vet ok; `architecture: OK` | ✓ PASS |
| Plain check/report bytes vs v0.3.0 | `bash scripts/compare-ref.sh v0.3.0` | 244/244 identical, 0 DIFF | ✓ PASS |
| Real-binary v1/v2 matrix | `bash scripts/compat-v1.sh v0.3.0` | compat-v1: 7/7; `pgrep -fa gruntled` shows no daemon process; branch master, clean tree | ✓ PASS |
| CI f05cecc (run 38225491068) | `gh run view` | success, 6/6 jobs (architecture, check incl. `-race` and fuzz, test-os macos, test-os windows, sarif-upload, recipe-check) | ✓ PASS |
| CI a50d203 (run 38227195245) | `gh run view` | success, 6/6 jobs | ✓ PASS |
| CI 57d74d8 (run 38227051604) | `gh run view --log-failed` | test-os (windows) FAIL: `TestRebaseWindowsUnsupported/report-file_daemon: rebase changed the runtime base`. a50d203 has identical code (only 16-SECURITY.md differs) and passed. | ⚠️ FLAKE, see Anti-Patterns |

Not run locally, as instructed: `go test ./...` in one go, `-race`, `-fuzz`, the 5,000-unit memory test, `TestBlastSizeBound` and `TestReportBlastSizeBound`. Their evidence is the CI runs above. CI runs tests without `-v`, so that evidence is package-level `ok` lines. The only skip guards on these tests are `testing.Short()` (or `linux || darwin`), and CI does not pass `-short`.

### Probe Execution

| Probe | Command | Result | Status |
|-------|---------|--------|--------|
| (none declared) | `find scripts -path '*/tests/probe-*.sh'` | No probe scripts. The phase's runnable gates are `compare-ref.sh` and `compat-v1.sh`, both run above. | N/A |

### Test Quality Audit

| Test | Linked Req | Circular | Assertion level | Verdict |
|------|-----------|----------|-----------------|---------|
| `TestDaemonBlastEqualsBlastBase` | DAEMON-07 | No. The oracle is the `blast --base` CLI; the normaliser asserts what it removes and spells the label with `fmt`. | Behavioral (byte equality + exit code + non-vacuity counters) | VALID |
| `TestCompatV1*` / `v1frozen_test.go` | DAEMON-09 | No. A verbatim v0.3.0 copy, with blob hashes checked against the tag. | Value | VALID |
| `scripts/compat-v1.sh` | DAEMON-09 | No. It uses the real v0.3.0 binary built from the tag. | Behavioral | VALID |
| `TestBaselineHoldsOnlyReport` | DAEMON-10 | n/a | Structural, with a non-vacuity probe | VALID |
| `TestDaemonMemorySynthrepo` | DAEMON-10 | n/a | Bound plus recorded numbers | VALID |
| `TestBlastJSONUnchanged` | SEC-01 / no-regression | Pre-change FNV hashes, recorded before the change | Value | VALID |

There are no disabled tests on requirements. Every `t.Skip` in the phase files is a platform guard (no unix sockets, NTFS names, POSIX permissions) or a `-short` guard on a CI-only heavy test.

### Anti-Patterns Found

| File | Line | Pattern | Severity | Impact |
|------|------|---------|----------|--------|
| (all 41 phase-modified files) | n/a | No `TBD`, `FIXME`, `XXX`, `TODO`, `HACK` or placeholder | n/a | none |
| `cmd/gruntled/rebase_test.go` | 161-174 | `TestRebaseWindowsUnsupported/report-file_daemon` failed once on windows CI (run 38227051604, 57d74d8) and passed on identical code (run 38227195245, a50d203) and on f05cecc. 16-04-SUMMARY also records two windows rebase rows "fixed test-side". | ⚠️ Warning | A test flake, not a product gap. `runRebaseWith` returns at rebase.go:68-71 before `openRepoRoot` or `statusfile.Dir`, so it cannot touch the runtime base. The likely cause is NTFS directory-entry metadata (mtime/size of the held lock or of the atomically replaced dump) settling after `holdDump`, since `treeState` compares mode, size and mtime. The stable `no runtime dir` subtest covers the same behavior. Worth hardening, for example by comparing names and hashes only for unreadable files on windows, before Phase 17 relies on CI being green. |
| `cmd/gruntled/rebase.go` | 123 | The pre-send failure line prints `%v` of a local error without `SanitizeReason` | ℹ️ Info | Only local dial/path errors reach it (16-SECURITY F1 analysis). No daemon-controlled text reaches it. |

### Human Verification Required

None. Every success criterion is checked by exact-byte or exit-code assertions through a real daemon (in-process and built-binary) on both transports, plus real windows CI. No truth is left behavior-unverified. The serialisation invariant is covered by `TestRebaseConcurrentPublish` under `-race` in CI, the skipped-result invariant by `TestRebaseNeverSkippedReport`, and the timeout path by `TestRebaseTimeoutUnknown`.

### Notes on wording

- SC2 says `report --blast` exits 3 when the daemon "is indexing or failed". In the code, a failed *initial* index ends the daemon (`watch.Run` error → exit 3, so later queries hit no daemon). A failed *reindex* keeps serving the previous view with the warning, as DAEMON-07 ("3 as `report`") and SC2's second clause require. Both behaviors are tested. I found no contradiction.
- The memory ratio varies between runs. Locally I got 2.53 at 500 units, against 2.41 in docs/validation.md. The asserted bound (`2.5 × Report + view + 1 MiB`) has slack for this, and the docs record one run, which they say. This is informational.

### Gaps Summary

There are no gaps. The phase goal is met in the code:
- The daemon keeps a Report-only baseline from its first committed index.
- At every publish it pre-renders a blast view through the same `Between` and presenters as `blast --base`, and serves it over protocol v2 or the windows dump.
- `rebase` moves the baseline atomically, in memory only, under the publish mutex.

Every exit path is explicit, and v1/v2 mismatches are errors in both directions against the real v0.3.0 binary. One warning stands: a windows-only test flake (`TestRebaseWindowsUnsupported/report-file_daemon`) that turned one CI run red on code that is green elsewhere.

### Bookkeeping left over (not verified artifacts; for the orchestrator)

- `.planning/REQUIREMENTS.md`: DAEMON-07..10 are still `[ ]` and `Pending` in the traceability table (lines 38-41, 93-96).
- `.planning/ROADMAP.md`: Phase 16 is still `[ ]` (line 50) with `**Plans**: TBD` (line 128), and the progress row reads `0/TBD | Not started` (should be 5/5 Complete, 2026-10-12).
- `.planning/STATE.md` still says "Phase 15 complete, ready to plan Phase 16".

---

_Verified: 2026-10-12T09:23:06Z_
_Verifier: Claude (gsd-verifier)_
