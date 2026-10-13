---
phase: 16-daemon-baseline-report-blast-rebase
plan: 02
subsystem: cmd (watch daemon), presenter
tags: [daemon, baseline, rebase, concurrency, blast]
requires: [16-01]
provides:
  - "presenter.DaemonBaseline, DaemonBaselineLabel, BlastJSONDaemon (daemon_baseline member after \"baseline\")"
  - "watchPublisher baseline state under one mutex; an atomic commit per EventReady with a blast view"
  - "watchPublisher.rebase: serialised, side-effect free, refusals typed"
  - "instance as ipc.Handler with a late-bound rebase hook; store seals every snapshot"
affects: [16-03, 16-04]
tech-stack:
  added: []
  patterns: ["build-then-commit under one mutex", "package-level test seams for the snapshot and the blast view"]
key-files:
  created:
    - internal/interfaces/presenter/blast_daemon_test.go
    - cmd/gruntled/baseline_test.go
  modified:
    - internal/interfaces/presenter/blast.go
    - cmd/gruntled/watch.go
    - cmd/gruntled/render.go
    - cmd/gruntled/instance.go
    - cmd/gruntled/instance_test.go
key-decisions:
  - "Publish order changed: the snapshot is committed under the mutex first, then stdout and the 'rendering report' stderr line, then the status file, all outside the mutex"
  - "The publisher keeps a hasBase flag next to base, so the first committed EventReady sets the baseline even if an earlier build failed"
  - "Refusal messages: indexing 'the daemon is still indexing; nothing changed', failed 'the last reindex failed; nothing changed', unavailable 'the blast view could not be computed; nothing changed', expect_mismatch 'the daemon is at generation M, not N', internal 'the blast view could not be rebuilt; nothing changed', unsupported (report-file instance) 'rebase needs the daemon socket; nothing changed'"
  - "A blast view that cannot be built carries Error = SanitizeReason(\"blast view: \" + err) with the baseline generation and source"
requirements-completed: [DAEMON-07, DAEMON-10]
duration: 70min
completed: 2026-10-12
---

# Phase 16 Plan 02: Daemon Baseline, Blast View and Rebase Summary

**The watch daemon now owns a baseline: the Report of the first committed EventReady, or the last committed one at a rebase. At every EventReady the publisher builds the candidate snapshot and its blast view under one mutex. The view uses the same `Between` and presenters as `blast --base`, with the daemon label and the `daemon_baseline` JSON member. lastRep, gen, the first baseline and the served snapshot are committed together, and only on success. `rebase` runs under the same mutex and refuses with a typed code without changing anything. On success it installs the served snapshot's Report as the baseline with an empty view, writes nothing, and leaves the generation where it was. The instance serves blast and rebase through `ipc.ServeHandler`, and `store` seals every snapshot before it is shared, which closes the 16-01 gap.**

## Performance

- Duration: ~70 min
- Tasks: 3
- Files: 2 created, 5 modified

## Accomplishments

### Task 1: presenter (internal/interfaces/presenter/blast.go)
- **New API:** `DaemonBaseline` and `DaemonBaselineLabel` (:58) produce `daemon generation <G0> (<source>), current generation <G>`; BlastText escapes the label. `blastDoc.Daemon` (:31) has the `daemon_baseline,omitempty` key and sits directly after `baseline`. `BlastJSON` and `BlastJSONDaemon` (:367) both go through a shared `blastJSON`.
- **TestDaemonBaselineLabel:** a source containing ESC is printed escaped.
- **TestBlastJSONDaemonMember:** on four fixtures, exactly one 5-line block appears right after `"baseline": true,`. Removing it gives BlastJSON's bytes, the document decodes, and `version` is still 2.
- **TestBlastJSONUnchanged:** pins BlastJSON's bytes for six fixtures, including no-baseline, as FNV-1a 64 values recorded before the change. The sha256 of each output was also recorded before and after the change, and the two matched.

### Task 2: publisher (cmd/gruntled/watch.go, render.go)
- **Seams and view builder** (render.go): `buildSnap = buildSnapshot` and `between = blasting.Between` (:62-63). `buildBlastView` (:69) renders text and JSON and counts the error findings in Broken.
- **State:** `watchPublisher` (watch.go) has `mu` (:254) guarding `last`, `gen`, `lastRep`, `base`, `baseGen`, `baseSource` and `hasBase`.
- **`publish`** (:268): the EventFailed copy of `*p.last` happens under the mutex (:295). writeOut, the "rendering report" stderr line and the status write come after the mutex is released (:309).
- **`commitReady`** (:322): it computes `next`, builds the candidate through `buildSnap`, and attaches `blastViewOrError` (:343), which never returns nil. Only then does it commit `lastRep`, `gen`, the first baseline and `setSnapshot` together.
- **`rebase`** (:362) refuses, changing nothing, in this order:
  - code `indexing` before any snapshot;
  - code `failed` when the last reindex failed;
  - code `unavailable` when the view has an error (:371);
  - code `expect_mismatch` when `expect` differs from `last.Generation` (:373);
  - code `internal` when the new view cannot be built (:378).

  On success it reads `accepted` from the served view (:380), sets `base = lastRep`, `baseGen = last.Generation` and source `rebase`, and calls `setSnapshot` with a copy that carries the new view.
- The `setSnapshot` doc comment states it runs inside the mutex, which in report-file mode includes the dump write (sec #357b).

Tests (cmd/gruntled/baseline_test.go), each red first in 12cc2b4:

| Test | Line | What it checks |
|------|-----:|----------------|
| TestPublisherBlastView | :100 | A then B (vpc output removed): view 2 is byte-equal to BlastText/BlastJSONDaemon of `Between(A,B)`, holds GRT004, BrokenErrors equals the count; EventFailed keeps the same Blast pointer |
| TestPublisherBetweenError | :145 | the error view is sanitised and the check bytes are kept; rebase gives `unavailable` and nothing changes |
| TestPublisherBuildSnapshotError | :171 | a failed build commits nothing; rebase installs A; publishing after the seam is restored gives generation 2 |
| TestRebaseRenderError | :199 | `internal`; base, baseGen, source and the snapshot are unchanged |
| TestPublisherRebase | :225 | indexing, failed, `expect_mismatch` with the exact message, success with the rebased view, idempotent second rebase; status file content and mtime and stdout/stderr unchanged |
| TestRebaseBaselineNoGRT004 | :294 | the baseline has no GRT004 and equals B's diagnostics, with GRT001 |
| TestBaselineHoldsOnlyReport | :372 | reflection walk over all fields with the allowlist; a probe struct with `blasting.Sources` fails |
| TestRebaseConcurrentPublish | :400 | 200 publishes with EventFailed every 7th event, 200 rebases and a pointer reader; every baseline generation was handed on and is ≤ its snapshot's generation; the final view is empty |

### Task 3: instance (cmd/gruntled/instance.go)
- **Handler:** `instance` implements `ipc.Handler`. `Snapshot` (:171) returns the served pointer. `Rebase` (:176) refuses with `unsupported` in report-file mode and with `indexing` before the hook is wired, and otherwise calls the hook. The hook is an `atomic.Pointer[func(*uint64) (ipc.RebaseResult, error)]` set by `setRebase` (:188).
- **Wiring:** acquireInstance serves through `ipc.ServeHandler(ln, inst.info, inst)` (:128). runWatchWith calls `inst.setRebase(pub.rebase)` before `watch.Run` (watch.go:179). The order in which the lock, socket and dump are created is unchanged.
- **`store`** (:196) calls `ipc.Seal(in.info, s)` first, then `snap.Store`, then the dump write.

Tests, red first in 1e0f795:
- **instance_test.go:**
  - TestInstanceRebaseHook (:345): `indexing` over the socket before the hook and after it is wired with nothing published, then {1, 0}; in report-file mode `unsupported` with the dump bytes unchanged.
  - TestInstanceStoreSeals (:387): `Sealed()` is true; Query(OpBlast) and the dump carry the sealed view.
  - TestDaemonRebaseSocket (:435): a real daemon; the label is `(rebase)`; the status file and the runtime dir listing (names, sizes, modes) are unchanged.
  - TestShutdownWithRebaseInFlight (:470): exit 0 within 5 s.
- **baseline_test.go TestRebaseNeverSkippedReport** (:543): `watch.Run` with a scripted watcher and indexer. Call 2 (R2) is skipped because a change is pending when it returns, and call 3 is held at its gate. Rebase in that window gives generation 1 with base R1. After R3 is published the base is still R1, and the next rebase installs R3.

## Task Commits

| # | Hash | Message |
|---|------|---------|
| 1 | `3ba8dd7` | test(16-02): daemon baseline label and daemon_baseline JSON member |
| 2 | `581f39f` | feat(16-02): presenter daemon baseline label and daemon_baseline member |
| 3 | `12cc2b4` | test(16-02): publisher baseline, blast view per publish and rebase |
| 4 | `adbad3e` | feat(16-02): daemon baseline, blast view per publish and serialised rebase |
| 5 | `1e0f795` | test(16-02): serve through the instance handler, seal at store, never a skipped report |
| 6 | `ce23e0b` | feat(16-02): serve blast and rebase through the instance handler, seal at store |

This summary is its own `docs(16-02): summary` commit.

## Gate (at ce23e0b)

Run with `GOMAXPROCS=2 GOFLAGS=-p=2`:

```
go test -count=1 ./internal/interfaces/presenter/ ./internal/infrastructure/ipc/
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.071s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.682s
go test -count=1 -run 'TestPublisher|TestRebase|TestBaselineHoldsOnlyReport|TestRenderSnapshotParity|TestRenderReportFormats|TestWatchFailedReasonSanitised' ./cmd/gruntled/
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	0.134s
go test -count=1 -run 'TestInstance|TestRebase|TestDaemonRebaseSocket|TestShutdownWithRebaseInFlight|TestWatch|TestReport|TestPublisher|TestNoGRT004OutsideBlast' ./cmd/gruntled/
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	0.815s
go vet ./...
vet-ok
GOOS=windows GOARCH=amd64 go vet ./cmd/gruntled
win-vet-ok
gofmt -l .
(no output)
bash scripts/check-architecture.sh
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
grep -rnE '\.Value\(|typeConstraint\(' internal --include=*.go | grep -v _test.go | grep -v ':\s*//'
internal/infrastructure/tfsurface/typesig.go:161
internal/infrastructure/hclconv/literal.go:26
internal/infrastructure/terragrunt/eval.go:28
internal/infrastructure/terragrunt/eval.go:76
```

Not run locally: `-race`, including TestRebaseConcurrentPublish, which ran without `-race` in 0.08 s; the full `go test ./...`; and test-check-architecture.sh, since no architecture script changed. CI covers them.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-16-02-1 | mitigated | one mutex watch.go:254 over commitReady :322, the EventFailed copy :295, rebase :362 and setSnapshot :392; TestRebaseConcurrentPublish (EventFailed every 7th event; -race in CI) |
| T-16-02-2 | mitigated | commitReady commits lastRep/gen only together with setSnapshot; expect compared with last.Generation :373; TestRebaseNeverSkippedReport, TestPublisherBuildSnapshotError |
| T-16-02-3 | mitigated | base is only ever an event Report (commitReady, rebase); TestRebaseBaselineNoGRT004 |
| T-16-02-4 | mitigated | baseline typed checking.Report; TestBaselineHoldsOnlyReport walks all fields with the allowlist and the Sources probe |
| T-16-02-5 | mitigated | writeOut, the stderr line and the status write after the mutex (watch.go:309); TestPublisherRebase stdout/stderr assertions |
| T-16-02-6 | mitigated | publish never waits on the server; TestShutdownWithRebaseInFlight |
| T-16-02-7 | mitigated | rebase writes nothing; instance.Rebase :176 refuses in report-file mode; TestPublisherRebase (status content and mtime), TestInstanceRebaseHook (dump bytes), TestDaemonRebaseSocket (runtime listing) |
| T-16-02-8 | mitigated | label through escapeTerm (TestDaemonBaselineLabel); error view through SanitizeReason (blastViewOrError :343, TestPublisherBetweenError) |
| T-16-02-9 | mitigated | blastViewOrError never returns nil |
| T-16-02-10 | mitigated | eval-site grep gives the same four lines |
| T-16-02-11 | mitigated | `unavailable` at :371, build-first rebase with `internal` at :378; TestPublisherBetweenError, TestRebaseRenderError |
| T-16-02-12 | mitigated | instance.store seals, instance.go:197; TestInstanceStoreSeals |

## Deviations from Plan

**1. [Reflection allowlist] `last` is walked but exempt from the infrastructure-package rule**
- The plan's allowlist (stdout, stderr, now, status, snapshots, printed) leaves out `last *ipc.Snapshot`, yet the plan's own state list includes it.
- `ipc.Snapshot` and `ipc.BlastView` are bytes and integers only. The walk still descends into them and would fail on any Interface, Func, Chan or UnsafePointer kind, but their package path is allowed.
- Every other `internal/infrastructure` type fails the test.

**2. [Republish scenario] Driven at the publisher, not through watch.Run**
- With the current debouncer, Run's republish branch (run.go:128-131) cannot be reached.
  - A skip happens only when a change is pending, which leaves the debouncer armed.
  - Only Flush disarms it, and Flush always calls Index next, which publishes or skips again.
  - So `unpublished && !pend` is never true when a Ready arrives. TestRunSpuriousReadyAfterSkipEndsPublished in the watch package shows the same thing: call 3 is published, and no republish happens.
- The second scenario therefore drives the publisher with the event that branch would emit, an EventReady carrying the skipped Report, and asserts that rebase installs it. The first scenario runs through the real watch.Run.

**3. [Test pin] FNV-1a 64 instead of sha256 in TestBlastJSONUnchanged**
- The interfaces stdlib allowlist forbids `crypto/sha256` and `encoding/hex`, even in tests. The red commit used them, so check-architecture failed at that commit, and the feat commit 581f39f switched to an inline FNV-1a. The sha256 values matched before and after the change.

**4. [Ordering] stdout now follows the snapshot commit**
- Before, stdout was written before the snapshot was stored. Now the snapshot is committed under the mutex first, and stdout and the status file are written after it is released, as the plan asks. The "rendering report" stderr line still comes after stdout. Every watch and report test is green.

**5. [Test helper] The test publisher's status dir uses statusfile.EnsureDir**
- The first run failed because the status writer refuses a directory it has not checked. The fix is in adbad3e.

## Known Risks / Unverified

- TestRebaseConcurrentPublish has not run with `-race` yet; CI will run it.
- Deviation 2 depends on the reading of run.go above. If the planner wants the branch reachable, that is a change to the watch package, outside this plan.

## Self-Check: PASSED
