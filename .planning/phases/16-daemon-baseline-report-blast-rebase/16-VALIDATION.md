---
phase: 16
slug: daemon-baseline-report-blast-rebase
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-10-12
---

# Phase 16: Validation Strategy

> The validation contract for each phase, used to sample feedback during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing (cmd tests: in-test xorshift property; ipc: raw AF_UNIX fakes and a frozen v0.3.0 `respond`), testscript (usage txtars), a built-binary daemon RSS test (CI), and the shell gate `scripts/compat-v1.sh` (real v0.3.0 binary) |
| **Config file** | none. `.github/workflows/ci.yml` fuzz step changed in 16-00 |
| **Quick run command** | `go test -count=1 ./internal/infrastructure/ipc/ ./internal/interfaces/presenter/ && go test -count=1 -short -run 'TestReport\|TestRebase\|TestPublisher\|TestBaseline\|TestDaemon\|TestScripts/(report\|rebase\|usage)\|TestHelpMatchesDocs' -timeout 600s ./cmd/gruntled/` |
| **Full suite command (local)** | `go vet ./... && go test -count=1 -short -timeout 900s ./internal/... && go test -count=1 -short -timeout 900s ./cmd/gruntled/ && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh && bash scripts/compare-ref.sh v0.3.0 && bash scripts/compat-v1.sh v0.3.0` |
| **CI adds** | `go test -race -count=1 ./...` on linux, macos and windows (incl. TestRebaseConcurrentPublish, TestReportBlastSizeBound, TestDaemonMemorySynthrepo at 5,000 units, TestBlastSizeBound), and FuzzTypeSig 30 s with `-fuzzminimizetime 200x` |
| **Estimated runtime** | ~30 s (quick), ~4-6 min (local full incl. compare-ref and compat-v1) |

Environment for every local command:
`export PATH=$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH GOMAXPROCS=2 GOFLAGS=-p=2`.

**RAM rules (8 GB WSL):**
- Use targeted package tests with `-short`.
- Never run `go test ./...` without `-short`, and never use `-race` or `-fuzz` locally.
- The 5,000-unit and 4 MiB tests (TestReportBlastSizeBound, TestDaemonMemorySynthrepo at
  5,000 units) run in CI. They may run locally once to record numbers, only after `free -m`
  shows at least 4 GB available, one at a time, under `timeout 900`, with nothing else heavy
  running.

---

## Sampling Rate

- **After every task commit:** quick run command, plus the task's own `<automated>` line.
- **After every plan wave:** local full suite without compat-v1 (that one only in 16-04).
- **Before `/gsd:verify-work`:** 16-04 gate evidence in 16-04-SUMMARY.md, and CI green on
  three OSes with -race, the bound tests and the fuzz step showing a non-zero final rate.
- **Max feedback latency:** 30 seconds.

---

## Success Criteria → Tests

| # | Phase success criterion (ROADMAP) | Proven by |
|---|-----------------------------------|-----------|
| 1 | After a save, `report --blast` text and JSON show Broken and Impacted against the first index, with both generations. Bytes equal `blast --base <copy>` except for the label line and the JSON member, including with hostile names | `TestDaemonBlastEqualsBlastBase` (property, fresh and shared-loader variants, non-vacuity counters), `TestReportBlastE2E` (both transports), `TestTextOutputsEscapeControls` `report --blast` subtests, `TestPublisherBlastView`, `TestBlastJSONDaemonMember`, `TestDaemonBaselineLabel` |
| 2 | Exit 0/1 (Broken errors), 2 for sarif and --depth, 3 for no daemon, indexing, failed-initial or mismatch. Previous view plus warning after a failed reindex. windows via the report file | `report_usage.txtar`, `TestReportBlastStates`, `TestReportBlastRefusals`, `TestReportBlastV1`, `TestReportNoDaemonCreatesNothing` (extended), `TestPublisherBlastView` (failed keeps the view) |
| 3 | `rebase [--expect N]` moves the baseline in memory to the last published report. It prints the generation and accepted errors, refuses on indexing, failed and expect mismatch, says "outcome unknown" on timeout, exits 3 on windows, writes nothing, never starts a daemon. `report --blast` is then empty. Docs fixed | `TestPublisherRebase`, `TestRebaseConcurrentPublish` (-race CI), `TestRebaseNeverSkippedReport`, `TestRebaseBaselineNoGRT004`, `TestInstanceRebaseHook`, `TestDaemonRebaseSocket`, `TestShutdownWithRebaseInFlight`, `rebase_usage.txtar`, `TestRebaseCLI`, `TestRebaseRefusalsCLI`, `TestRebaseTimeoutUnknown`, `TestRebaseWindowsUnsupported`, `TestRebaseWritesNothing`, `TestReportBlastPitfall15`, `TestDaemonDocs`, `TestHelpMatchesDocs` |
| 4 | Protocol v2: a mismatch is an explicit exit 3, never empty. The matrix (fake v1 daemon × v2 client, v2 daemon × v1 request, v1 report file) passes. Plain `report` bytes unchanged | `TestRespond`, `TestDecodeResponse`, `TestCompatV1DaemonV2Client`, `TestCompatV2DaemonV1Request`, `TestCompatV1Dump`, `TestReportResponseHasNoBlast`, `TestReportWindowsFile` "protocol v1 dump", `TestReportBlastV1`, `TestRebaseV1`, **`scripts/compat-v1.sh v0.3.0` (real binary, hard gate)**, existing `TestReportParity*`, `compare-ref.sh v0.3.0` N/N |
| 5 | The baseline holds only a `checking.Report`. A blast response over 64 MiB is an explicit exit 3 naming the cap. Memory measured at 65, 500 and 5,000 units and recorded | `TestBaselineHoldsOnlyReport`, `TestBlastResponseCap`, `TestCapString`, `TestWriteDumpDropsOversizeView`, `TestReportBlastRefusals` (too_large and TooLarge), `TestDaemonMemorySynthrepo` + docs/validation.md section (pinned by `TestValidationDocPins`), `TestReportBlastSizeBound` (CI) |

Deferred from phase 15 and closed here: BLAST-11 report half (`TestReportBlastSizeBound`),
SEC-01 report half (`TestTextOutputsEscapeControls` report --blast subtests), T-13-02-7
(`TestRebaseBaselineNoGRT004`, `TestReportBlastPitfall15`).

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 16-00-01 | 00 | 1 | (CI health, D-16-09) | unit | `go test -count=1 -run 'TestFuzzTypeSigSeedsSmall\|TestTypeSigFormerFuzzSeeds\|FuzzTypeSig' -timeout 120s ./internal/infrastructure/tfsurface/` | ✅ typesig_test.go (extended) | ⬜ pending |
| 16-00-02 | 00 | 1 | (CI health) | doc pin + CI log | `go test -count=1 -run TestCIDoc ./cmd/gruntled/` | ✅ ci_doc_test.go (extended) | ⬜ pending |
| 16-01-01 | 01 | 1 | DAEMON-09, DAEMON-10 | unit (wire) | `go test -count=1 -run 'TestRespond\|TestRequestFitsMaxRequest\|TestDecodeResponse\|TestSnapshotJSONByteExact' ./internal/infrastructure/ipc/` | ✅ ipc_test.go (extended) | ⬜ pending |
| 16-01-02 | 01 | 1 | DAEMON-08 (transport), DAEMON-10 | socket + dump | `go test -count=1 -timeout 120s ./internal/infrastructure/ipc/ && GOOS=windows GOARCH=arm64 go build ./cmd/gruntled` | ✅ sock_unix_test.go (extended) | ⬜ pending |
| 16-01-03 | 01 | 1 | DAEMON-09 | compat matrix | `go test -count=1 -run 'TestCompat\|TestReportResponseHasNoBlast' ./internal/infrastructure/ipc/ && go test -count=1 -run 'TestReport\|TestPublisherSnapshots' ./cmd/gruntled/` | ❌ W0 (compat_unix_test.go, v1frozen_test.go) | ⬜ pending |
| 16-02-01 | 02 | 2 | DAEMON-07 | presenter unit | `go test -count=1 ./internal/interfaces/presenter/` | ❌ W0 (blast_daemon_test.go) | ⬜ pending |
| 16-02-02 | 02 | 2 | DAEMON-07, 08, 10 | white-box + concurrency | `go test -count=1 -run 'TestPublisher\|TestRebase\|TestBaselineHoldsOnlyReport' -timeout 300s ./cmd/gruntled/` | ❌ W0 (baseline_test.go) | ⬜ pending |
| 16-02-03 | 02 | 2 | DAEMON-08 | daemon socket | `go test -count=1 -run 'TestInstance\|TestRebase\|TestDaemonRebaseSocket\|TestShutdownWithRebaseInFlight\|TestWatch\|TestReport' -timeout 300s ./cmd/gruntled/` | ✅ instance_test.go (extended) | ⬜ pending |
| 16-03-01 | 03 | 3 | DAEMON-07, 09 | CLI + txtar | `go test -count=1 -run 'TestReport\|TestScripts/report' -timeout 300s ./cmd/gruntled/` | ❌ W0 (fakev1_unix_test.go) | ⬜ pending |
| 16-03-02 | 03 | 3 | DAEMON-08 | CLI + txtar | `go test -count=1 -run 'TestRebase\|TestScripts/(rebase\|usage)\|TestRunExitCodes' -timeout 300s ./cmd/gruntled/` | ❌ W0 (rebase.go, rebase_test.go, rebase_usage.txtar) | ⬜ pending |
| 16-03-03 | 03 | 3 | DAEMON-08 (docs) | doc guard | `go test -count=1 -run 'TestHelpMatchesDocs\|TestDaemonDocs\|TestReadme\|TestRuleRegistryDoc\|TestCIDoc\|TestScripts' ./cmd/gruntled/` | ❌ W0 (daemon_doc_test.go) | ⬜ pending |
| 16-04-01 | 04 | 4 | DAEMON-07, SEC-01, T-13-02-7 | property + e2e | `go test -count=1 -short -run 'TestDaemonBlastEqualsBlastBase\|TestNormaliseDaemonBlast\|TestReportBlastE2E\|TestReportBlastPitfall15\|TestTextOutputsEscapeControls' -timeout 600s ./cmd/gruntled/` | ❌ W0 (daemon_blast_test.go) | ⬜ pending |
| 16-04-02 | 04 | 4 | DAEMON-10, BLAST-11 | memory + built-binary bound | `go test -count=1 -short -run 'TestDaemonMemorySynthrepo\|TestValidationDocPins\|TestReportBlastSizeBound' -v -timeout 600s ./cmd/gruntled/` (5,000 units and the bound: CI) | ❌ W0 (daemon_mem_test.go) | ⬜ pending |
| 16-04-03 | 04 | 4 | DAEMON-09, all | gate | `bash scripts/compat-v1.sh v0.3.0` + architecture + compare-ref + txtar-extract + mod verify + build-release + vet + `-short` suites + eval-site grep | ❌ W0 (compat-v1.sh) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers the phase. Each ❌ W0 file is created by the first step of its
own task, which writes the failing test first.

---

## Mutation Proofs (non-vacuity)

| Mutation | Must fail | Where recorded |
|----------|-----------|----------------|
| `respond` checks op before version | `TestRespond` (v1 request with unknown op/field gives a version error), `TestCompatV2DaemonV1Request` | 16-01-SUMMARY |
| decodeResponse accepts a blast response without `blast` | `TestDecodeResponse` no-blast-view rows, `TestReportBlastRefusals` | 16-01-SUMMARY |
| `report` response keeps `Blast` | `TestRespond` report row, `TestReportResponseHasNoBlast` | 16-01-SUMMARY |
| Lenient request decode (unknown fields allowed) | `TestRespond` malformed rows | 16-01-SUMMARY |
| Server-side cap applied to plain report too | `TestBlastResponseCap` (report must fail client-side with no too_large code) | 16-01-SUMMARY |
| WriteDump keeps an oversize view | `TestWriteDumpDropsOversizeView` | 16-01-SUMMARY |
| Read errors after write not marked ErrNoReply / dial errors marked | `TestNoReplyAfterSend` | 16-01-SUMMARY |
| Baseline set from every publish (auto-rebase) | `TestPublisherBlastView`, `TestDaemonBlastEqualsBlastBase` | 16-02/04-SUMMARY |
| Rebase installs the Between-superseded diagnostics | `TestRebaseBaselineNoGRT004` | 16-02-SUMMARY |
| Rebase without the mutex | `TestRebaseConcurrentPublish` under -race (CI) | 16-02-SUMMARY (CI run id) |
| Rebase increments gen, or ignores `--expect` | `TestPublisherRebase`, `TestRebaseCLI` | 16-02/03-SUMMARY |
| Rebase writes the status file | `TestPublisherRebase` (mtime and content) | 16-02-SUMMARY |
| Daemon label used on lines other than the first | `TestDaemonBlastEqualsBlastBase` | 16-04-SUMMARY |
| report --blast prints the summary on stderr | `TestReportBlastStates` | 16-03-SUMMARY |
| rebase on windows reaches the runtime dir | `TestRebaseWindowsUnsupported` | 16-03-SUMMARY |
| Normaliser removes more than one block | `TestNormaliseDaemonBlast` | 16-04-SUMMARY |

Mutants run in a throwaway copy (`git archive`), one at a time, with GOMAXPROCS=2 and
targeted `-run`, never `-race` locally.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `-race` on rebase vs publish | DAEMON-08 | No local gcc; RAM | Confirm the CI `check` job is green, and record the run id |
| BLAST-11 report half, 5,000-unit memory | DAEMON-10, BLAST-11 | 8 GB WSL OOM risk | Confirm the CI linux and macos jobs ran TestReportBlastSizeBound and TestDaemonMemorySynthrepo at 5,000 units (not skipped). Copy the logged values into 16-04-SUMMARY and docs/validation.md |
| Fuzz step health | (CI) | Time budget; no local -fuzz | The CI fuzz step log tail shows a final `fuzz:` line with a non-zero rate |
| Real v0.3.0 compatibility | DAEMON-09 | Builds an old tag and spawns daemons (script, not `go test`) | `bash scripts/compat-v1.sh v0.3.0` prints `compat-v1: N/N`, recorded in 16-04-SUMMARY |
| Windows native report-file path | DAEMON-07 | Only the CI windows job runs real windows | Confirm the CI `test-os` windows job is green (report --blast via the report file, rebase exit 3) |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 30s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending (draft at plan time; set to validated after execution)
