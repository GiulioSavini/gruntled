---
phase: 6
slug: machine-readable-output
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-10-01
---

# Phase 6 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` + `rogpeppe/go-internal/testscript` v1.16.0 |
| **Config file** | none (`cmd/gruntled/main_test.go` registers `gruntled-exit`) |
| **Quick run command** | `go test ./internal/interfaces/... ./internal/domain/repograph/... ./cmd/gruntled -run 'Graph|SARIF|Sarif|Scripts|Deterministic|NoWrites|StdoutWrite|HelpMatchesDocs' -count=1` |
| **Full suite command** | `go test -race -count=1 ./... && bash scripts/check-architecture.sh` |
| **Estimated runtime** | ~60 seconds |

---

## Sampling Rate

- **After every task commit:** Run quick run command
- **After every plan wave:** Run full suite command
- **Before `/gsd:verify-work`:** Full suite green, SARIF schema validation green, upload-sarif run URL recorded
- **Max feedback latency:** 60 seconds

---

## Per-Task Verification Map

Filled by planner/executor per task. Requirement-level map:

| Requirement | Behavior | Test Type | Automated Command | File Exists | Status |
|-------------|----------|-----------|-------------------|-------------|--------|
| INT-01 | graph JSON shape/golden (block, paths, unresolved, module-unknown, config-unknown, cycle) | testscript | `go test ./cmd/gruntled -run TestScripts/graph_golden -count=1` | ❌ W0 | ⬜ pending |
| INT-01 | Edge carries name/state/skipOutputs | unit | `go test ./internal/domain/repograph -run Edges -count=1` | extend | ⬜ pending |
| INT-01 | presenter: `[]` not null, key order, omitempty fields | unit | `go test ./internal/interfaces/presenter -run Graph -count=1` | ❌ W0 | ⬜ pending |
| INT-01 | byte-identical across checkouts; nothing written | e2e | `go test ./cmd/gruntled -run 'TestDeterministicAcrossCheckouts|TestNoWrites' -count=1` | extend | ⬜ pending |
| INT-01 | `--json` required => exit 2; exit 0 with unknowns; 3 on bad path | testscript | `go test ./cmd/gruntled -run TestScripts/graph_exitcodes -count=1` | ❌ W0 | ⬜ pending |
| INT-01 | stdout failure => 3 | unit | `go test ./cmd/gruntled -run TestRunStdoutWriteFailure -count=1` | extend | ⬜ pending |
| INT-02 | SARIF golden + exit codes unchanged (0/1) | testscript | `go test ./cmd/gruntled -run TestScripts/sarif_golden -count=1` | ❌ W0 | ⬜ pending |
| INT-02 | structural ingestion constraints | unit | `go test ./cmd/gruntled -run TestSARIFStructure -count=1` | ❌ W0 | ⬜ pending |
| INT-02 | percent-encoding, rule table, ruleIndex | unit | `go test ./internal/interfaces/presenter -run SARIF -count=1` | ❌ W0 | ⬜ pending |
| INT-02 | rule names/titles match docs/cli.md | doc test | `go test ./cmd/gruntled -run 'SARIFDoc|ValidationDocPins|HelpMatchesDocs' -count=1` | extend | ⬜ pending |
| INT-02 | schema validity vs OASIS schema | CI | `go run github.com/santhosh-tekuri/jsonschema/cmd/jv@v0.7.0 cmd/gruntled/testdata/sarif-schema-2.1.0.json <out.sarif>` | ❌ W0 | ⬜ pending |
| Both | architecture allowlists | script | `bash scripts/check-architecture.sh` | ✅ | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `cmd/gruntled/testdata/script/graph_golden.txtar`, `graph_exitcodes.txtar`, `sarif_golden.txtar`
- [ ] `cmd/gruntled/sarif_test.go` — structure test via `encoding/json` into generic maps
- [ ] `cmd/gruntled/testdata/sarif-schema-2.1.0.json` (vendored OASIS) and `testdata/sarif-fixture/`
- [ ] presenter tests for graph/sarif in `presenter_test.go` (stdlib only)

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| `upload-sarif` accepts document | INT-02 | Needs GitHub code scanning, runs on master push only | Push to master, record workflow run URL in VERIFICATION.md, confirm alerts on correct files (`checkout_path`) |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 60s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
