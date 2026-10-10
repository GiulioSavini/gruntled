---
phase: 12
slug: gap-closure-watcher-directory-ignore-and-v0-3-audit-findings
status: validated
nyquist_compliant: true
wave_0_complete: true
created: 2026-10-10
---

# Phase 12 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing + pgregory.net/rapid v1.3.0 + testscript v1.16.0 + Go fuzzing |
| **Config file** | none |
| **Quick run command** | `go test -count=1 ./internal/infrastructure/watch/... ./internal/infrastructure/terragrunt/... ./internal/infrastructure/statusfile/... ./internal/interfaces/presenter/... && go test -count=1 -run 'Watch\|Report\|Instance\|Escape\|TestHelpMatchesDocs\|TestScripts\|Golden' ./cmd/gruntled/` |
| **Full suite command** | `go test -count=1 ./... && go vet ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` (CI adds `-race` on linux/macos/windows; no local gcc) |
| **Estimated runtime** | ~15 seconds (quick), ~70 seconds (full) |

---

## Sampling Rate

- **After every task commit:** Run the quick run command
- **After every plan wave:** Run the full suite command
- **Before `/gsd:verify-work`:** Full suite must be green, plus CI green on the three OSes
- **Max feedback latency:** 15 seconds

---

## Success Criteria → Tests

| # | Phase success criterion (ROADMAP) | Proven by |
|---|-----------------------------------|-----------|
| 1 | Editor patterns apply only to files; `live/2024/` walked, watched (native, `--poll`), edits reindexed | `TestIgnoredEntry`; contract sub-tests "edit inside pattern-named dirs", "mkdir pattern-named dir", "rename pattern-named dir away", "mkdir pattern-named dir after start, then rename it away", "emacs lock symlink" on `TestWatcherContractPoll` + `TestWatcherContractNative`; `TestNativePatternDirWatched`, `TestNativePatternDirSafetyNet`, `TestNativePatternDirReplacedByFile`; contract "vim-style save" still green (patterns still apply to files) |
| 2 | Regression test reproduces the stale-daemon case on native and poll; rapid test generates dir names matching file ignore patterns | `TestWatchPatternDirParity/{poll,native}` (red on HEAD); `TestIncrementalEqualsFull` with `2024`/`x.tmp` dirs and a dirty filter modelling entry + ancestor-dir skip, counters `patternDirRenames > 0`, `linkedIncludes > 0`; `TestIncrementalModelNonVacuous` fixed tail (write `<dir>/terragrunt.hcl`, reindex, change output, reindex for `2024` and `x.tmp`); `TestIncrementalModelDetectsTypeBlindIgnore` (type-blind predicate makes the tail fail); shrunk mutation sequence pasted in 12-05-SUMMARY |
| 3 | Every BLOCKER/HIGH/MEDIUM sec finding fixed with a test or accepted in `12-SECURITY.md` | MEDIUM symlink alias: `TestAliasCacheTargetEditRealFS`, `TestAliasCacheDirLinkRealFS`, `TestAliasCacheRetargetChain`, `TestAliasCacheMapFS` (red on HEAD). LOWs too: `TestWatchRuntimeDirInsideRepo`, `TestDirInsideCaseVariant` + internal seam test, `TestEscapeTerm`/`FuzzEscapeTerm`/`TestEscapeJSONRoundTrip`/`FuzzEscapeJSON`/`TestTextOutputsEscapeControls` (text and JSON paths). Accepted INFO items listed for 12-SECURITY.md (sec writes it at `/gsd-secure-phase 12`) |
| 4 | `go test ./...`, `go vet ./...`, `scripts/check-architecture.sh`, CI linux/macos/windows green | 12-05 Task 3 gate (incl. six-target build and govulncheck) + CI run after push |

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 12-01-01 | 01 | 1 | DAEMON-01 | unit | `go test -count=1 -run 'TestIgnored\|TestPending' ./internal/infrastructure/watch/` | ❌ W0 (TestIgnoredEntry new) | ✅ green |
| 12-01-02 | 01 | 1 | DAEMON-01, DAEMON-02 | contract (poll + native) | `go test -count=1 ./internal/infrastructure/watch/` | ✅ contract_test.go (extended) | ✅ green |
| 12-01-03 | 01 | 1 | DAEMON-01, DAEMON-02 | integration (daemon) | `go test -count=1 -run 'TestWatchPatternDirParity\|TestWatchParity' ./cmd/gruntled/` | ❌ W0 (watch_patterndir_test.go) | ✅ green |
| 12-02-01 | 02 | 1 | DAEMON-02 | unit, real FS + MapFS (red first) | `go test -count=1 -run TestAliasCache ./internal/infrastructure/terragrunt/` (expected to fail) | ❌ W0 (alias_cache_test.go) | ✅ green |
| 12-02-02 | 02 | 1 | DAEMON-02 | unit + goldens | `go test -count=1 ./internal/infrastructure/terragrunt/ && go test -count=1 -run 'Golden\|TestScripts\|Corpus\|E2E' ./cmd/gruntled/` | ✅ | ✅ green |
| 12-03-01 | 03 | 1 | DAEMON-03 | unit (seam + case-insensitive host) | `go test -count=1 ./internal/infrastructure/statusfile/` | ❌ W0 (inside_internal_test.go) | ✅ green |
| 12-03-02 | 03 | 1 | DAEMON-05 | integration | `go test -count=1 -run 'TestWatchRuntimeDirInsideRepo\|TestWatchDumpMode\|TestPrintStatusPathTakesNoLock' ./cmd/gruntled/` | ✅ instance_test.go (extended) | ✅ green |
| 12-04-01 | 04 | 1 | DAEMON-04 | unit + fuzz | `go test -count=1 -run 'Escape\|Sanitize' ./internal/interfaces/presenter/ && go test -run '^$' -fuzz FuzzEscapeTerm -fuzztime 10s ./internal/interfaces/presenter/ && go test -run '^$' -fuzz FuzzEscapeJSON -fuzztime 10s ./internal/interfaces/presenter/` | ❌ W0 (escape_test.go) | ✅ green |
| 12-04-02 | 04 | 1 | DAEMON-04, BLAST-01 | unit (text + JSON encoders) + goldens | `go test -count=1 ./internal/interfaces/presenter/ && go test -count=1 -run 'Golden\|TestScripts\|Blast' ./cmd/gruntled/` | ✅ | ✅ green |
| 12-04-03 | 04 | 1 | DAEMON-04, BLAST-01 | e2e (check, watch, report, blast; check json/sarif, graph --json, blast json) + docs | `go test -count=1 -run 'TestTextOutputsEscapeControls\|TestHelpMatchesDocs' ./cmd/gruntled/` | ❌ W0 (cmd/gruntled/escape_test.go) | ✅ green |
| 12-05-01 | 05 | 2 | DAEMON-02 | property (rapid) | `go test -count=1 -run 'TestIncrementalEqualsFull\|TestIncrementalModelNonVacuous\|TestIncrementalModelDetectsTypeBlindIgnore' -v ./internal/infrastructure/terragrunt/` | ✅ incremental_test.go (extended) | ✅ green |
| 12-05-02 | 05 | 2 | DAEMON-01, DAEMON-04 (docs) | doc guard + stderr sanitise | `go test -count=1 -run 'TestHelpMatchesDocs\|TestReadme\|Doc\|TestWatch' ./cmd/gruntled/` | ❌ W0 (watch_stderr_test.go) | ✅ green |
| 12-05-03 | 05 | 2 | all | gate | full suite + `bash scripts/build-release.sh` + `~/go/bin/govulncheck ./...` + `go list -m golang.org/x/text` + `cp go.mod go.sum $T/ && go mod tidy && cmp go.mod $T/go.mod && cmp go.sum $T/go.sum` | ✅ | ✅ green |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers the phase; every ❌ W0 file above is created by the first (TDD, red-first)
step of its own task, so no separate Wave 0 plan is needed.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Case-variant `Inside` on a real case-insensitive filesystem | DAEMON-03 | Linux CI and local ext4 are case-sensitive; the real test skips there | Confirm `TestDirInsideCaseVariant` ran (not skipped) in the CI `test-os` job on macos-latest and windows-latest |
| `-race` on the new watch/terragrunt/cmd tests | DAEMON-01, DAEMON-02 | No local gcc | Confirm CI `check` job `go test -race -count=1 ./...` green after the push |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 15s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** validated 2026-10-10 — all rows green locally and in CI run 38043996664 (-race on linux/macos/windows); mutation proofs in 12-0x-SUMMARY.md and 12-SECURITY.md
