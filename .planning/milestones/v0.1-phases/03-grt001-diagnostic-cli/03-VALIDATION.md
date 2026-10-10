---
phase: 3
slug: grt001-diagnostic-cli
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-09-25
---

# Phase 3 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` (go1.27.0): table tests over hand-built graphs; `rogpeppe/go-internal/testscript` v1.16.0 (test-only, added in 03-03); `internal/testsupport/synthrepo` fixtures; bash self-tests for arch rules |
| **Config file** | none (go.mod only); scripts in `cmd/gruntled/testdata/script/` |
| **Quick run command** | `go test -count=1 ./internal/domain/analysis ./internal/application/checking ./internal/interfaces/... ./cmd/gruntled` |
| **Full suite command** | `test -z "$("$(go env GOROOT)/bin/gofmt" -l .)" && go mod tidy -diff && go vet ./... && GOTOOLCHAIN=go1.27.0 go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./... && go test -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` (`-race` and govulncheck in CI; govulncheck also locally in 03-03) |
| **Estimated runtime** | quick ~10 s; full ~90 s (self-test copies dominate) |

---

## Sampling Rate

- **After every task commit:** Run the quick run command
- **After every plan wave:** Run the full suite command (+ cross-build loop after 03-03 and 03-05)
- **Before `/gsd:verify-work`:** Full suite green locally and CI green; corpus gate recorded in 03-05-SUMMARY
- **Max feedback latency:** ~10 seconds (quick), ~90 seconds (full)

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 3-01-01 | 01 | 1 | DIAG-01, DIAG-03 | unit (table, hand-built graphs) | `go test -count=1 ./internal/domain/analysis -run 'TestDIAG03\|TestUnknownOutputs' && bash scripts/check-architecture.sh` | ❌ W0 (created in task, TDD) | ⬜ pending |
| 3-01-02 | 01 | 1 | DIAG-01, DIAG-02 | unit (fakes) | `go test -count=1 ./internal/application/... && bash scripts/check-architecture.sh` | ❌ W0 (created in task, TDD) | ⬜ pending |
| 3-01-03 | 01 | 1 | DIAG-03 | unit (table) + doc grep | `go test -count=1 ./internal/infrastructure/... -run TestDependencyOptions && grep -q 'otherwise the deprecated' internal/domain/repograph/options.go` | ✅ parse_test.go (rows updated), options.go (doc only) | ⬜ pending |
| 3-02-01 | 02 | 1 | CLI-01, CLI-03, DIAG-04 | unit (golden strings) | `go test -count=1 ./internal/interfaces/...` | ❌ W0 (created in task, TDD) | ⬜ pending |
| 3-02-02 | 02 | 1 | CLI-04 (+ layering) | script self-test (binary rule looped over all 5 release targets; binary-exec-windows-file case) | `bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` | ✅ extend | ⬜ pending |
| 3-04-01 | 04 | 1 | CLI-02, DIAG-03 | doc grep | `grep -c -F -x '  3  analysis could not run: path missing, not a directory or unreadable, or an internal failure' docs/cli.md` | ❌ created in task | ⬜ pending |
| 3-04-02 | 04 | 1 | DIAG-03 (decision record) | doc grep | `grep -q 'mock_outputs never suppresses GRT001' .planning/PROJECT.md && grep -q 'stdlib \`flag\`' .planning/PROJECT.md` | ✅ edit | ⬜ pending |
| 3-03-01 | 03 | 2 | CLI-01, CLI-02 | e2e (in-process + testscript) | `go test -count=1 ./cmd/gruntled -run 'TestRunExitCodes\|TestRunStdoutWriteFailure\|TestScripts/(usage\|exitcodes\|flags_after_path)' && bash scripts/check-architecture.sh` | ❌ W0 (created in task) | ⬜ pending |
| 3-03-02 | 03 | 2 | DIAG-01, DIAG-02, DIAG-03, DIAG-04 | e2e (testscript txtar) | `go test -count=1 ./cmd/gruntled -run TestScripts` | ❌ W0 (created in task) | ⬜ pending |
| 3-05-01 | 05 | 3 | DIAG-01, CLI-03, CLI-05, DIAG-04, CLI-02 | e2e (Go, synthrepo) | `go test -count=1 ./cmd/gruntled -run 'TestOracle\|TestDeterministicAcrossCheckouts\|TestNoWrites\|TestMutationDiff\|TestHelpMatchesDocs'` | ❌ W0 (created in task) | ⬜ pending |
| 3-05-02 | 05 | 3 | CLI-01 (docs), phase gate | corpus run | built binary on `$HOME/.cache/gruntled-phase4/corpus/primary` at e6c55d1: exit 0 + empty stdout; role_name renamed in iac.src/s3_runtime/state.tf only: exit 1 + exactly 8 GRT001 lines with the mock suffix (full command in 03-05 Task 2) | ✅ pinned corpus clone (Phase 2) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

DIAG-03 silent-row coverage (zero false positives): rows 1, 2a/2b, 3a/3b, 4a/4b, 5a/5b/5c and 6a/6b each have a named analyzer subtest (3-01-01). Rows 1, 2a/2b, 3a/3b, 4, 5a/5b/5c also have an end-to-end testscript block (`diag03_silent_rows.txtar`, 3-03-02). The corpus shape is a named test at both levels.

---

## Wave 0 Requirements

Every task creates its own tests TDD-first in the same task. There are no separate Wave 0 stubs.

- [ ] Hard gate: ALL Phase 2 gap-closure plans 02-06, 02-07, 02-08, 02-09, 02-10 and 02-11 are merged to master and green before Wave 1 starts (`for p in 06 07 08 09 10 11; do test -f .planning/phases/02-parsing-graph-construction/02-$p-SUMMARY.md || exit 1; done`). 02-09 reshaped scripts/check-architecture.sh (03-02 builds on it) and 02-11 edits terragrunt/parse_test.go (03-01 Task 3 edits it too)
- [ ] `internal/domain/analysis/grt001_test.go`: hand-built graph helper (3-01-01)
- [ ] `internal/application/checking/check_test.go`: hand-written fakes (3-01-02)
- [ ] `internal/interfaces/presenter/presenter_test.go` (3-02-01)
- [ ] `GOTOOLCHAIN=go1.27.0 go get github.com/rogpeppe/go-internal@v1.16.0 && go mod tidy` (3-03-01)
- [ ] `cmd/gruntled/main_test.go` + `testdata/script/` (3-03-01)
- [ ] New arch rules land with self-test cases (3-02-02) before cmd/gruntled wires the presenters (3-03)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Primary corpus clean + role_name mutation gives 8 GRT001 | phase gate (precursor to VALID-03/04) | Needs an external clone; Phase 4 owns the committed experiment | 03-05 Task 2 runs it as an automated command and records the result in the SUMMARY |

*All requirement behaviors have automated verification. The corpus run is a recorded gate, not a requirement test.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 90s
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
