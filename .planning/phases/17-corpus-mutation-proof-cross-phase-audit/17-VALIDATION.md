---
phase: 17
slug: corpus-mutation-proof-cross-phase-audit
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-10-12
---

# Phase 17: Validation Strategy

> The validation contract for each phase, used to sample feedback during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go testing in cmd/gruntled: env-gated corpus tests (GRUNTLED_CORPUS, _SECRET, _DENIS256) with an independent textual oracle, plus one always-on harness self-test. Shell gates: `scripts/compare-ref.sh` (COMPARE_REF_CORPORA), `scripts/corpus-gate.sh`, `scripts/fetch-corpora.sh` + `scripts/test-fetch-corpora.sh`, `scripts/compat-v1.sh` |
| **Config file** | none. New `.github/workflows/corpus.yml` (workflow_dispatch + weekly); `ci.yml` unchanged |
| **Quick run command** | `go test -count=1 -short -run 'TestCorpusHarnessSelfTest\|TestValidationDocPins\|TestCIDoc' ./cmd/gruntled/` |
| **Corpus gate (local, required)** | `bash scripts/corpus-gate.sh` (serial shards: compare-ref with corpora, TestCorpusBlastSelf, TestCorpusBlastModel, TestCorpusReformat, TestCorpusMutation17/<corpus>/<class>, each `-p=1 -count=1 -timeout 90m` under `timeout`, `free -m` first, memory floor 1500 MB, no-net re-exec) |
| **Full suite command (local)** | `go vet ./... && go test -count=1 -short -timeout 900s ./internal/... && go test -count=1 -short -timeout 1200s ./cmd/gruntled/ && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh && bash scripts/compare-ref.sh v0.3.0 && bash scripts/compat-v1.sh v0.3.0 && bash scripts/test-txtar-extract.sh && bash scripts/test-fetch-corpora.sh` |
| **CI adds** | ci.yml unchanged: `go test -race -count=1 ./...` on linux, macos and windows (the harness self-test runs there; corpus tests SKIP without env). corpus.yml on demand and weekly: fetch + corpus gate |
| **Estimated runtime** | quick ~20 s; corpus gate ~35-45 min serial (measured per run: 0.10 s iso20022, 0.51 s denis256, 13-25 MB RSS); local full ~5-7 min |

Environment for every local command:
`export PATH=$HOME/.local/go/bin:$HOME/.local/bin:$HOME/go/bin:$PATH GOMAXPROCS=2 GOFLAGS=-p=2`.

**RAM rules (8 GB WSL):**
- Corpus runs are serial and single-binary: one corpus, one class, one process at a time,
  `free -m` before each, every command under `timeout`, gated suites with `-timeout 90m`.
- Never run `go test ./...` without `-short`, never `-race` or `-fuzz` locally, never the bound
  tests (TestBlastSizeBound, TestReportBlastSizeBound, 5,000-unit memory) locally.
- Never run the corpus gate next to another heavy job. Do not start the agent-bus GUI.
- The only network step is `scripts/fetch-corpora.sh` (once); everything after it is offline.

---

## Sampling Rate

- **After every task commit:** the quick run command, plus the task's own `<automated>` line.
- **After every plan wave:** the full local suite with `-short`; corpus shards of that wave only.
- **Before `/gsd:verify-work`:** `corpus-gate: N/N` recorded in 17-03-SUMMARY (re-run in 17-05 if
  code changed after it), compare-ref 256/256, compat-v1 N/N, ci.yml green on three OSes, 17-SECURITY
  verdict SECURED.
- **Max feedback latency:** 30 seconds for the quick command; corpus shards are gate-time only.

---

## Success Criteria → Tests

| # | Phase success criterion (ROADMAP) | Proven by |
|---|-----------------------------------|-----------|
| 1 | On each corpus `check` is byte-identical to v0.3.0 and `blast --base <same tree>` is empty | `COMPARE_REF_CORPORA=… scripts/compare-ref.sh v0.3.0` (256/256, stdout+stderr+exit for corpus rows, digest unchanged), `TestCorpusBlastSelf` (same dir and copy, text and JSON) |
| 2 | Reformat-only mutations give an empty Impacted set | `TestCorpusReformat` R1-R9 (empty broken/impacted/changes, non-vacuity guard, counters > 0), `TestCorpusHarnessSelfTest` (guard trips on a parse-breaking transform) |
| 3 | Every mutation is caught: GRT004 at every site and no GRT001; unreferenced output; required variable; default removed; variable type; output type; sensitive; depth ≥ 2 | `TestCorpusMutation17/<corpus>/M1..M9` with exact equality vs the independent model (`TestCorpusBlastModel`, pinned exclusions), M1 dual oracle against frozen v0.3.0, M8 counters; daemon path `TestCorpusDaemonSample` |
| 4 | Independent review with no open BLOCKER/MEDIUM; six-target proof and doc-sync pins pass on the final tree; docs match GRT004, --depth, report --blast, rebase | 17-SECURITY.md (gruntled-sec), 17-INTEGRATION.md (gsd-integration-checker), `check-architecture.sh` Step 9, `TestRuleRegistryDoc`, `TestHelpMatchesDocs`, `TestDaemonDocs`, `TestCIDoc`, `TestValidationDocPins`, eval-site grep = 4 lines |

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 17-00-01 | 00 | 1 | (audit bookkeeping) | doc check | verify-summary loop over 13-16 SUMMARYs + requirements-claim loop (see plan) | ✅ SUMMARY files | ⬜ pending |
| 17-00-02 | 00 | 1 | (audit bookkeeping) | validate-phase | `grep '^status: validated'` on 15/16-VALIDATION.md + per-row commands | ✅ VALIDATION files | ⬜ pending |
| 17-00-03 | 00 | 1 | (tech debt) | comment-only diff + tests | `go test -count=1 -short ./internal/infrastructure/watch/ && go test -count=1 -short -run TestRebase ./cmd/gruntled/` + windows CI run ids | ✅ run.go, rebase_test.go | ⬜ pending |
| 17-01-01 | 01 | 1 | REL-03 | shell self-test | `bash scripts/test-fetch-corpora.sh` | ❌ W0 (fetch-corpora.sh, test-fetch-corpora.sh) | ⬜ pending |
| 17-01-02 | 01 | 1 | REL-03 (SC1a) | byte gate | `bash scripts/compare-ref.sh v0.3.0` (244) / clean-fixture (248) / corpora (256) | ✅ compare-ref.sh (extended) | ⬜ pending |
| 17-01-03 | 01 | 1 | REL-03 (SC1b) | gated e2e | `go test -p=1 -count=1 -run TestCorpusBlastSelf -timeout 15m ./cmd/gruntled/` | ❌ W0 (corpus_blast_test.go) | ⬜ pending |
| 17-02-01 | 02 | 2 | REL-03 (oracle) | gated model check | `go test -p=1 -count=1 -run 'TestCorpusBlastModel/<corpus>' -timeout 15m ./cmd/gruntled/` | ❌ W0 (corpus_model_test.go) | ⬜ pending |
| 17-02-02 | 02 | 2 | REL-03 (harness) | always-on self-test | `go test -count=1 -run TestCorpusHarnessSelfTest ./cmd/gruntled/` | ❌ W0 (corpus_edit_test.go, corpus_harness_test.go) | ⬜ pending |
| 17-02-03 | 02 | 2 | REL-03 (SC2) | gated transforms | `go test -p=1 -count=1 -run 'TestCorpusReformat/<corpus>' -timeout 90m ./cmd/gruntled/` | ❌ W0 (corpus_reformat_test.go) | ⬜ pending |
| 17-03-01 | 03 | 3 | REL-03 (SC3 M1, M2) | gated mutation + dual oracle | `go test -p=1 -count=1 -run 'TestCorpusMutation17/<corpus>/M[12]' -timeout 90m ./cmd/gruntled/` | ❌ W0 (corpus_mutation_test.go) | ⬜ pending |
| 17-03-02 | 03 | 3 | REL-03 (SC3 M3-M7, M9) | gated mutation | `go test -p=1 -count=1 -run 'TestCorpusMutation17/<corpus>/M<k>' -timeout 90m ./cmd/gruntled/` | ❌ W0 | ⬜ pending |
| 17-03-03 | 03 | 3 | REL-03 (SC3 M8, gate) | serial gate | `bash scripts/corpus-gate.sh` → `corpus-gate: N/N` | ❌ W0 (corpus-gate.sh) | ⬜ pending |
| 17-04-01 | 04 | 4 | REL-03 (daemon path) | gated daemon e2e | `go test -p=1 -count=1 -run 'TestCorpusDaemonSample/<corpus>' -timeout 90m ./cmd/gruntled/` | ❌ W0 (corpus_daemon_test.go) | ⬜ pending |
| 17-04-02 | 04 | 4 | REL-03 (record) | doc pin | `go test -count=1 -short -run TestValidationDocPins ./cmd/gruntled/` | ✅ validation_doc_test.go (extended) | ⬜ pending |
| 17-04-03 | 04 | 4 | REL-03 (CI) | doc pin | `go test -count=1 -run TestCIDoc ./cmd/gruntled/` + `! grep GRUNTLED_CORPUS .github/workflows/ci.yml` | ❌ W0 (corpus.yml) | ⬜ pending |
| 17-05-01 | 05 | 5 | REL-03 (SC4) | review (checkpoint) | 17-SECURITY.md + 17-INTEGRATION.md exist with per-row verdicts | ❌ W0 (17-INTEGRATION.md) | ⬜ pending |
| 17-05-02 | 05 | 5 | REL-03 (SC4) | red→green per finding | `go vet ./... && go test -count=1 -short ./internal/... ./cmd/gruntled/` | n/a | ⬜ pending |
| 17-05-03 | 05 | 5 | REL-03 (SC4) | final gate | architecture + arch self-tests + doc pins + eval-site grep (4) + compare-ref 256/256 + compat-v1 + mod verify + no tag at HEAD | ✅ scripts | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

Existing infrastructure covers the phase (corpus helpers, textual oracle, compare-ref, daemon test
helpers). Each ❌ W0 file is created by the first step of its own task, which writes the failing
test first. Prerequisite outside the repo: the one-time `scripts/fetch-corpora.sh` run (network)
before 17-01 Task 2.

---

## Mutation Proofs (non-vacuity)

| Fault (recorded mutant) | Must be caught by | Recorded in |
|-------------------------|-------------------|-------------|
| compare-ref cannot fail | `COMPARE_REF_PERTURB=1` with corpus rows → DIFF, exit 1 | 17-01-SUMMARY |
| Self-blast cannot fail | TestCorpusBlastSelf non-vacuity row (output deleted → changes non-empty) | 17-01-SUMMARY |
| Fetch accepts a wrong commit / follows a symlink when stripping | test-fetch-corpora.sh cases 1-3 | 17-01-SUMMARY |
| Oracle copies gruntled (uncategorised difference, count drift) | TestCorpusBlastModel with empty tables (red), pinned counts | 17-02-SUMMARY |
| Comparator accepts a wrong result (9 faults: dropped/added unit, distance ±1, wrong via/source, GRT001 left, GRT004 at a silent site, extra change, wrong type kind) | TestCorpusHarnessSelfTest | 17-02-SUMMARY |
| Reformat breaks the parse and passes vacuously | non-vacuity guard (red R3 case) | 17-02-SUMMARY |
| GRT004 reported as GRT001, or one site missing | M1 comparator + v0.3.0 dual oracle (red on the self-test tree) | 17-03-SUMMARY |
| SENTINEL wrong | M5 red with empty constant, hand check | 17-03-SUMMARY |
| Reference binary is not v0.3.0 | `--version` check fails, never skips | 17-03-SUMMARY |
| Daemon label off by one | TestCorpusDaemonSample normaliser mutant | 17-04-SUMMARY |
| Doc numbers drift | TestValidationDocPins with shared package-level values | 17-04-SUMMARY |
| Corpus suite leaks into ci.yml | TestCIDoc corpus_workflow | 17-04-SUMMARY |

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| One-time corpus fetch | REL-03 | Needs network | `bash scripts/fetch-corpora.sh`; record stripped counts and HEADs in 17-01-SUMMARY |
| Exclusion table review | REL-03 | A human must justify each category | Each category row in 17-02-SUMMARY has a reason sentence; reviewed by sec in the 17-05 review |
| SENTINEL hand check | REL-03 | Orchestrator decision 4 | Derive the expected string from the D-15 rules and compare with HEAD's output; write it in 17-03-SUMMARY and docs/validation.md |
| Cross-phase review | REL-03 SC4 | Independent reviewers | 17-05 Task 1 checkpoint |
| corpus.yml first run | REL-03 (CI, informational) | Dispatch after push | `gh workflow run corpus.yml`; record run id |
| ci.yml on three OSes, -race | all | No local -race or windows | Record the green run id for the final commit in 17-05-SUMMARY |

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 30s (quick command)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending (plan review by sec, then execution).
