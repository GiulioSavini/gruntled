---
phase: 16-daemon-baseline-report-blast-rebase
plan: 03
subsystem: cli, docs
tags: [cli, daemon, rebase, report, docs, compatibility]
requires: [16-02]
provides:
  - "gruntled report --blast [--format text|json] on both transports, with every exit path explicit"
  - "gruntled rebase [--expect N]: strict --expect, 10 s timeout, the outcome-unknown message, windows exit 3"
  - "CLI rows of the DAEMON-09 matrix against a raw-syscall fake v0.3.0 daemon"
  - "docs/cli.md and README.md updated for the daemon baseline; TestDaemonDocs and TestHelpMatchesDocs pins"
affects: [16-04]
tech-stack:
  added: []
  patterns: ["raw syscall AF_UNIX test daemon with a non-blocking accept loop (portable stop, no net)"]
key-files:
  created:
    - cmd/gruntled/rebase.go
    - cmd/gruntled/rebase_test.go
    - cmd/gruntled/fakev1_unix_test.go
    - cmd/gruntled/daemon_doc_test.go
    - cmd/gruntled/testdata/script/rebase_usage.txtar
  modified:
    - cmd/gruntled/report.go
    - cmd/gruntled/main.go
    - cmd/gruntled/watch.go
    - cmd/gruntled/report_test.go
    - cmd/gruntled/e2e_test.go
    - cmd/gruntled/testdata/script/report_usage.txtar
    - cmd/gruntled/testdata/script/usage.txtar
    - docs/cli.md
    - README.md
key-decisions:
  - "The cap message always names 64 MiB (the CLI cannot see ipc.responseCap). <n> comes from Blast.TooLarge, or is parsed from a too_large refusal's message"
  - "rebase reuses depthFlag for --expect (it records set, so an empty value is exit 2) with its own parseGeneration (uint64)"
  - "A daemon-side unsupported refusal prints 'gruntled: the daemon does not support rebase; restart gruntled watch to reset the baseline'; an unknown refusal code prints 'gruntled: the daemon refused the rebase: <msg>; nothing changed'"
  - "A rebase failure before the request was sent (not no-daemon) prints 'gruntled: cannot reach the watch daemon for <root>: <err>'; no post-send failure reaches that line"
requirements-completed: [DAEMON-07, DAEMON-08, DAEMON-09]
duration: 80min
completed: 2026-10-12
---

# Phase 16 Plan 03: report --blast, rebase and Docs Summary

**`gruntled report --blast` prints the daemon's blast view over the socket or from the report file. stdout is exactly the view; exit 1 means Broken holds an error. Every other path is explicit: sarif and `--depth` exit 2, while too large, uncomputable, missing or empty views, no daemon, indexing and protocol mismatch all exit 3. `gruntled rebase [--expect N]` moves the baseline and prints the generation and the accepted error count. Each refusal exits 3 with a fixed message. Any post-send failure that is not a refusal prints the "may or may not have moved" message. On windows rebase exits 3 before touching anything. A raw-syscall fake v0.3.0 daemon proves the CLI mismatch rows. The docs no longer call the daemon read-only, and they cover the trust boundary, the blast cap, the label and the member. Plain `report` is unchanged: its parity tests are green and compare-ref v0.3.0 gives 244/244.**

## Performance

- Duration: ~80 min
- Tasks: 3
- Files: 5 created, 9 modified

## Accomplishments

### Task 1: report --blast (cmd/gruntled/report.go)
- **Flags:** runReportWith (:63) adds `--blast` and a `--depth` that always refuses. They are validated in this order: format, then sarif with `--blast` (:83), then `--depth` (:88). Each failure prints the usage and exits 2.
- **Query:** the op is a parameter (:92). queryDaemon (:235) queries op blast over the socket. In report-file mode it reads `snapshot.blast` and runs `ipc.ValidBlastView` on it (:255), so a missing or empty view is an error on both transports. The socket side gets the same check from decodeResponse (16-01).
- **Printing:** printBlast (:182) writes exactly Text or JSON. After a failed reindex it adds the existing warning (warnFailed, :168), and it exits 1 iff `Blast.HasErrors`.
- **Exit 3 cases:**
  - `TooLarge`, and a `too_large` refusal (:129, refusedSize :216), print `gruntled: the blast view is <n> bytes, over the 64 MiB response cap; run gruntled blast --base <dir>` (blastTooLarge, :205).
  - `Blast.Error` prints `gruntled: the daemon could not compute the blast view: <sanitised>`.
- **Usage:** documents `--blast`. The exit-1 line now ends with "in Broken with --blast", and exit 3 adds "blast view over the 64 MiB cap or unavailable".
- **Tests:**
  - report_usage.txtar: help, sarif with `--blast`, `--depth` with and without `--blast`, and no daemon.
  - TestReportBlastStates (report_test.go:423): indexing, clean (exit 0), broken (exit 1) and failed, on both transports. The snapshots are the real publisher output for A/B.
  - TestReportBlastRefusals (:477): too large, view error with an ESC, no view, `{}`, text-only and json-only, on both transports.
  - TestReportBlastNoDaemonCreatesNothing (:515).
  - fakev1_unix_test.go: TestReportBlastV1 (:77; report in every format and `--blast` against the fake v1 daemon, plus a v1 report file) and TestReportBlastTooLargeRefusal (:103).

### Task 2: rebase (cmd/gruntled/rebase.go, main.go)
- **Flow** (runRebaseWith, :50):
  1. parse args;
  2. strict `--expect` (parseGeneration :151, `^[1-9][0-9]*$` fitting uint64, recorded as set);
  3. windows check right after parsing (:68);
  4. openRepoRoot, statusfile.Dir and CheckDir on both levels, as report does (nothing is created);
  5. `ipc.Rebase` with `rebaseTimeout = 10 * time.Second` (:39, a test seam).
- **Errors:** rebaseFailed (:103) maps no daemon, ErrUnsupported, version mismatch and each refusal code (refusalMessage, :128) to fixed messages. ErrOutcomeUnknown (:115) gives the outcome-unknown message; on a timeout, `os.ErrDeadlineExceeded` (:117) adds ` (no answer within 10s)`.
- **main.go:** dispatches `rebase`. topUsage lists it and mentions `"gruntled rebase -h"`.
- **watch.go:371:** the daemon's `failed` refusal now carries the snapshot's sanitised LastError, so the CLI can print `the last reindex failed: <reason>` (deviation 1).
- **Tests:**
  - rebase_usage.txtar: help, seven invalid `--expect` values, `--bogus`, two paths, and no daemon creating nothing. usage.txtar: help lists rebase.
  - rebase_test.go:
    - TestRebaseCLI (:43), on a real daemon:
      - rebase with `--expect` set to the current generation accepts 0 findings;
      - after a new GRT001 is added, report `--blast` exits 1, rebase accepts `1 error findings`, then report `--blast` exits 0 with `Broken (0):` and the `(rebase)` label;
      - `--expect 1` gives the exact mismatch message, and the baseline stays put.
    - TestRebaseRefusalsCLI (:86): six codes, exact messages, ESC escaped.
    - TestRebaseWindowsUnsupported (:143): no runtime dir, and a held report-file daemon; the runtime base is unchanged.
    - TestRebaseWritesNothing (:172): the repo, runtime base and status file are identical (path, mode, size, mtime, sha256) after a success, a refusal and another success; no daemon creates nothing.
  - fakev1_unix_test.go:
    - TestRebaseOutcomeUnknownCLI (:116): an ok reply without a rebase result, garbage, and a truncated reply.
    - TestRebaseTimeoutUnknown (:134): the timeout seam set to 200 ms.
    - TestRebaseV1 (:146).

### Task 3: docs (docs/cli.md, README.md)
- **docs/cli.md usage and help:**
  - usage summary, examples and the `-h` sentence now include rebase;
  - top, report and rebase usage mirrors generated from the binary's `-h`;
  - exit-code paragraphs and tables for report and rebase.
- **Blast sections:**
  - Blast text: the daemon view paragraph and the example `baseline: daemon generation 1 (watch start), current generation 1`.
  - Blast JSON: a `daemon_baseline` row and an example. The version stays 2.
- **`## gruntled report`:**
  - the Read-only bullet now reads `rebase is the only operation that changes the daemon, and only its in-memory baseline`;
  - a Blast view bullet;
  - protocol version 2;
  - the windows bullet now says `rebase is not available on windows`;
  - the Response cap bullet now says `report never carries the blast view`, gives the blast view's own cap, and says the windows report file drops an oversize view first.
- **New `## gruntled rebase` section:**
  - the success line format;
  - in memory only;
  - `--expect`;
  - refusals;
  - the timeout and the outcome-unknown message;
  - the trust boundary (`only the daemon owner's user can connect`);
  - windows;
  - the GRT004/GRT001 note.
- **Guarantees and limitations:** No writes and No cache mention rebase and the baseline. A new Known limitations bullet covers a dirty start, the baseline being lost on restart, and rebase as the repair.
- **README:** the report paragraph mentions `report --blast` and `gruntled rebase`, and the Windows limitation says rebase is unavailable there.
- **Tests:** TestHelpMatchesDocs gains `{"rebase", 3}`. TestDaemonDocs (daemon_doc_test.go:12) was red first in 2937f8a (19 missing pins).

## Task Commits

1. `9c508c1` test(16-03): report --blast on both transports, refusals, v1 daemon rows
2. `d441c63` feat(16-03): report --blast prints the daemon's blast view
3. `9722439` test(16-03): rebase command: usage, refusals, outcome unknown, windows, writes nothing
4. `9a23ad3` feat(16-03): gruntled rebase moves the daemon's blast baseline
5. `2937f8a` test(16-03): pin the daemon docs: rebase, trust boundary, cap, label and member
6. `41889f3` docs(16-03): report --blast, rebase, trust boundary and the blast cap

This summary is its own `docs(16-03): summary` commit.

## Gate (at 41889f3)

Run with `GOMAXPROCS=2 GOFLAGS=-p=2`; `free -m` showed about 6.9 GB available before each heavy step:

```
go test -count=1 -run 'TestReport|TestScripts/report' ./cmd/gruntled/
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	0.184s
go test -count=1 -run 'TestRebase|TestScripts/(rebase|usage)|TestRunExitCodes|TestPublisher|TestReportUsage' ./cmd/gruntled/
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	0.413s
go test -count=1 -run 'TestHelpMatchesDocs|TestDaemonDocs|TestReadme|TestRuleRegistryDoc|TestCIDoc|TestScripts|TestValidation' ./cmd/gruntled/
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	0.291s
go test -count=1 -short -timeout 600s ./cmd/gruntled/ ./internal/infrastructure/ipc/ ./internal/interfaces/presenter/
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	5.977s
ok  	github.com/GiulioSavini/gruntled/internal/infrastructure/ipc	0.669s
ok  	github.com/GiulioSavini/gruntled/internal/interfaces/presenter	0.079s
GOOS=windows GOARCH=amd64 go build ./cmd/gruntled
GOOS=windows GOARCH=arm64 go build ./cmd/gruntled
win-builds-ok
go vet ./...
vet-ok
gofmt -l .
(no output)
bash scripts/check-architecture.sh
architecture: OK (4 domain packages, 5 application packages, 1 interfaces packages)
bash scripts/compare-ref.sh v0.3.0
244/244 identical
```

After compare-ref the repo is back on branch `master` with a clean tree.

Not run locally: `-race`, the non-short cmd bound tests, and `scripts/test-check-architecture.sh`. The orchestrator asked to skip the last one because no architecture script changed. CI covers all three.

## Threat Mitigations

| ID | Disposition | Where |
|----|-------------|-------|
| T-16-03-1 | mitigated (by boundary) | rebase.go CheckDir on both levels before connecting (as report.go); 0600 socket / 0700 dir unchanged; docs/cli.md "Trust boundary" bullet pinned by TestDaemonDocs |
| T-16-03-2 | mitigated | success line prints the accepted count (rebase.go, after ipc.Rebase); TestRebaseCLI asserts `1 error findings` |
| T-16-03-3 | mitigated | `--expect` compare-and-swap; mismatch exit 3, baseline unchanged (TestRebaseCLI) |
| T-16-03-4 | mitigated | rebaseFailed :115-118 ErrOutcomeUnknown → "may or may not have moved", plus the timeout note; TestRebaseOutcomeUnknownCLI, TestRebaseTimeoutUnknown |
| T-16-03-5 | mitigated | windows exit 3 right after parsing (rebase.go:68); TestRebaseWindowsUnsupported |
| T-16-03-6 | mitigated | no EnsureRepoDir in report or rebase; TestRebaseWritesNothing, TestReportBlastNoDaemonCreatesNothing, txtar `! exists $WORK/rt/gruntled` |
| T-16-03-7 | mitigated | VersionError rows (TestReportBlastV1, TestRebaseV1); ValidBlastView on the report-file path report.go:255, and decodeResponse on the socket; TestReportBlastRefusals |
| T-16-03-8 | mitigated | SanitizeReason on Blast.Error (printBlast), the failed reason and every refusal message (refusalMessage); ESC cases in TestReportBlastRefusals and TestRebaseRefusalsCLI |
| T-16-03-9 | mitigated | TestDaemonDocs, TestHelpMatchesDocs (rebase rows) |
| T-16-03-10 | mitigated | TestReportParity, TestReportParityClean, TestReportStates and TestReportWindowsFile unchanged and green; compare-ref v0.3.0 244/244 |

## Deviations from Plan

**1. [16-02 code] The daemon's `failed` refusal carries the reason (watch.go:371)**
- The plan's CLI message `the last reindex failed: <reason>` needs the reason. The 16-02 refusal message was a fixed sentence, so the CLI would have printed it twice. The daemon now sends the snapshot's LastError, which was already sanitised at publish, and the CLI sanitises it again. The 16-02 tests check only the code, and they stay green. This one line is outside the plan's file list.

**2. [Over-cap CLI row] Replaced by a truncated reply**
- The CLI uses the real 64 MiB cap, which a cmd test cannot lower. Sending a reply over 64 MiB would make the in-process client buffer more than 64 MiB, which breaks the RAM rule. TestRebaseOutcomeUnknownCLI therefore uses a truncated reply (a partial line, then close) as its third post-send case. The over-cap case is covered at the ipc level by TestRebaseOutcomeUnknown (cap lowered to 8 KiB), and the CLI maps every ErrOutcomeUnknown the same way.

**3. [Test placement] Raw-socket rebase tests in fakev1_unix_test.go**
- TestRebaseOutcomeUnknownCLI, TestRebaseTimeoutUnknown and TestReportBlastTooLargeRefusal need the raw syscall listener, so they live in the linux||darwin file next to the v1 rows. rebase_test.go stays untagged; its socket tests skip at runtime on other platforms.

**4. [Messages the plan does not fix]**
- A daemon-side `unsupported` refusal prints `gruntled: the daemon does not support rebase; restart gruntled watch to reset the baseline`.
- An unknown refusal code prints `gruntled: the daemon refused the rebase: <msg>; nothing changed`.
- A failure before the request was sent prints `gruntled: cannot reach the watch daemon for <root>: <err>`.

**5. [Docs] The Task 3 doc edit is a `docs(16-03)` commit**
- The plan's red/green pattern for Task 3 is test then docs, so the "implementation" commit is `docs(16-03): report --blast, rebase, ...`. The SUMMARY is a separate `docs(16-03): summary` commit.

## Known Risks / Unverified

- `-race` and macOS have not run these tests yet. The raw test daemon uses a non-blocking accept with a 2 ms poll so that it also stops cleanly on darwin; CI will confirm.
- The trust boundary relies on directory ownership and mode only; peer credentials are not checked (D-16-07).

## Self-Check: PASSED
