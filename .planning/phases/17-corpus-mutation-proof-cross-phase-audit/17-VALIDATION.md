---
phase: 17
slug: corpus-mutation-proof-cross-phase-audit
status: validated
nyquist_compliant: true
wave_0_complete: true
created: 2026-10-12
revised: 2026-10-12 (sec plan review #412-#423: HIGH 1, MEDIUM 5, LOW 4, INFO 1)
validated: 2026-10-13 (post-execution audit, no gaps)
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
| **Corpus gate (local, required)** | `bash scripts/corpus-gate.sh` (re-execs under `unshare -rn` first and prints `network=isolated`; exports GOMAXPROCS=2 and GOFLAGS='-mod=readonly -p=1' itself; serial shards: compare-ref with corpora, TestCorpusBlastSelf, TestCorpusBlastModel, TestCorpusReformat, TestCorpusMutation17/<corpus>/<class>, each `-p=1 -count=1 -timeout 90m` under `timeout`, `free -m` first, memory floor 1500 MB, no-net re-exec) |
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
| 17-00-01 | 00 | 1 | (audit bookkeeping) | doc check | verify-summary loop over 13-16 SUMMARYs + requirements-claim loop (see plan) | ✅ SUMMARY files | ✅ green |
| 17-00-02 | 00 | 1 | (audit bookkeeping) | validate-phase | `grep '^status: validated'` on 15/16-VALIDATION.md + per-row commands | ✅ VALIDATION files | ✅ green |
| 17-00-03 | 00 | 1 | (tech debt) | comment-only diff + tests | `go test -count=1 -short ./internal/infrastructure/watch/ && go test -count=1 -short -run TestRebase ./cmd/gruntled/` + windows CI run ids | ✅ run.go, rebase_test.go | ✅ green |
| 17-01-01 | 01 | 1 | REL-03 | shell self-test | `bash scripts/test-fetch-corpora.sh` | ✅ fetch-corpora.sh, test-fetch-corpora.sh | ✅ green |
| 17-01-02 | 01 | 1 | REL-03 (SC1a) | byte gate | `bash scripts/compare-ref.sh v0.3.0` (244) / clean-fixture (248) / corpora (256) | ✅ compare-ref.sh (extended) | ✅ green (256/256) |
| 17-01-03 | 01 | 1 | REL-03 (SC1b) | gated e2e | `go test -p=1 -count=1 -run TestCorpusBlastSelf -timeout 15m ./cmd/gruntled/` | ✅ corpus_blast_test.go | ✅ green |
| 17-02-01 | 02 | 2 | REL-03 (oracle) | gated model check | `go test -p=1 -count=1 -run 'TestCorpusBlastModel/<corpus>' -timeout 15m ./cmd/gruntled/` | ✅ corpus_model_test.go | ✅ green |
| 17-02-02 | 02 | 2 | REL-03 (harness) | always-on self-test | `go test -count=1 -run TestCorpusHarnessSelfTest ./cmd/gruntled/` | ✅ corpus_edit_test.go, corpus_harness_test.go | ✅ green |
| 17-02-03 | 02 | 2 | REL-03 (SC2) | gated transforms | `go test -p=1 -count=1 -run 'TestCorpusReformat/<corpus>' -timeout 90m ./cmd/gruntled/` | ✅ corpus_reformat_test.go | ✅ green |
| 17-03-01 | 03 | 3 | REL-03 (SC3 M1, M2) | gated mutation + dual oracle | `go test -p=1 -count=1 -run 'TestCorpusMutation17/<corpus>/M[12]' -timeout 90m ./cmd/gruntled/` | ✅ corpus_mutation_test.go | ✅ green |
| 17-03-02 | 03 | 3 | REL-03 (SC3 M3-M7, M9) | gated mutation | `go test -p=1 -count=1 -run 'TestCorpusMutation17/<corpus>/M<k>' -timeout 90m ./cmd/gruntled/` | ✅ corpus_mutation_test.go | ✅ green (4,345 mutations caught) |
| 17-03-03 | 03 | 3 | REL-03 (SC3 M8, gate) | serial gate | `bash scripts/corpus-gate.sh` → `corpus-gate: N/N` | ✅ corpus-gate.sh | ✅ green (31/31, network-isolated) |
| 17-04-01 | 04 | 4 | REL-03 (daemon path) | gated daemon e2e | `go test -p=1 -count=1 -run 'TestCorpusDaemonSample/<corpus>' -timeout 90m ./cmd/gruntled/` | ✅ corpus_daemon_test.go | ✅ green |
| 17-04-02 | 04 | 4 | REL-03 (record) | doc pin | `go test -count=1 -short -run TestValidationDocPins ./cmd/gruntled/` | ✅ validation_doc_test.go (extended) | ✅ green |
| 17-04-03 | 04 | 4 | REL-03 (CI) | doc pin | `go test -count=1 -run TestCIDoc ./cmd/gruntled/` + `! grep GRUNTLED_CORPUS .github/workflows/ci.yml` | ✅ corpus.yml | ✅ green |
| 17-05-01 | 05 | 5 | REL-03 (SC4) | review (checkpoint) | 17-SECURITY.md + 17-INTEGRATION.md exist with per-row verdicts | ✅ 17-INTEGRATION.md, 17-SECURITY.md | ✅ green (SECURED) |
| 17-05-02 | 05 | 5 | REL-03 (SC4) | red→green per finding | `go vet ./... && go test -count=1 -short ./internal/... ./cmd/gruntled/` | n/a | ✅ green (flake fixed 78aa8b5) |
| 17-05-03 | 05 | 5 | REL-03 (SC4) | final gate | architecture + arch self-tests + doc pins + eval-site grep (4) + compare-ref 256/256 + compat-v1 + mod verify + no tag at HEAD | ✅ scripts | ✅ green (17-05-SUMMARY, CI 38264926783 + 568e260) |

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
| Fetch runs a config-defined filter or template hook, or accepts an escaping symlink (sec #414) | test-fetch-corpora.sh filter, templateDir and escaping-link cases (markers absent, exit 1) | 17-01-SUMMARY |
| compare-ref digest follows links (dir symlink, dangling, absolute, /dev/zero) (sec #413) | compare-ref link-shape row: completes under timeout, 248/248, links listed as `p -> target` | 17-01-SUMMARY |
| Oracle copies gruntled: uncategorised difference, item failing its predicate, gruntled-reports-less without a predicate, count or share drift (sec #412) | TestCorpusBlastModel with empty per-item lists (red), predicate check per item, direction rule, pinned included count and share; sec acceptance of the table before 17-03 | 17-02-SUMMARY |
| applyEdit writes through an escaping link or a dir-symlink alias (sec #415) | TestCorpusHarnessSelfTest refusal rows | 17-02-SUMMARY |
| Driver or comparator accepts a wrong result (sec #416). Faults injected through the real runMutation hook, each must record a failure and leave caught < sites: Impacted unit dropped; unit added; distance off by one; wrong via; wrong source; wrong module on an Impacted unit; wrong name in added/removed_variables or added/removed_outputs; GRT001 left at a GRT004 site; one GRT004 site missing; GRT004 at a silent site; extra changes[] entry; missing changes[] entry; wrong type_changes kind; wrong type_changes new; old-rule violation; wrong exit code; summary mismatch; wrong version, kind or baseline | TestCorpusHarnessSelfTest | 17-02-SUMMARY |
| Reformat breaks the parse and passes vacuously | non-vacuity guard (red R3 case) | 17-02-SUMMARY |
| GRT004 reported as GRT001, or one site missing | M1 comparator + v0.3.0 dual oracle (red on the self-test tree) | 17-03-SUMMARY |
| SENTINEL wrong | M5 red with empty constant, hand check | 17-03-SUMMARY |
| Reference binary is not v0.3.0 (sec #417) | gate: `v0.3.0^{commit}` == pinned before build, resolved sha in ldflags; test: `--version` label AND fingerprint (`rebase` unknown command, `blast --depth` undefined flag); a HEAD build with v0.3.0 ldflags fails | 17-03-SUMMARY |
| Gate runs with network (sec #417) | `exec unshare -rn` first, /proc/net/dev only lo, `network=isolated` in the record; CORPUS_GATE_NONET=1 without unshare exits 1 | 17-03-SUMMARY |
| Site or included-module counts shrink (sec #418) | pinned package-level constants asserted by TestCorpusMutation17 | 17-03-SUMMARY |
| Eval site swapped while the count stays 4 (sec #421) | 17-05 gate compares the exact (file, function) set | 17-05-SUMMARY |
| corpus.yml persists the token or shares the Go cache (sec #420) | TestCIDoc pins `persist-credentials: false` and `cache: false` | 17-04-SUMMARY |
| Daemon label off by one | TestCorpusDaemonSample normaliser mutant | 17-04-SUMMARY |
| Doc numbers drift | TestValidationDocPins with shared package-level values | 17-04-SUMMARY |
| Corpus suite leaks into ci.yml | TestCIDoc corpus_workflow | 17-04-SUMMARY |

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| One-time corpus fetch | REL-03 | Needs network | `bash scripts/fetch-corpora.sh`; record stripped counts and HEADs in 17-01-SUMMARY |
| Exclusion table review | REL-03 | A human must justify each item | The per-item table (path, predicate category, direction, evidence) is posted to sec and accepted before 17-03 runs (bus id in 17-02-SUMMARY); re-checked in the 17-05 review |
| Exactness exceptions | REL-03 | Claim wording | The four named exceptions (type old by inequality for non-primitives, GRT004 message prefix, text by tokens once per class, SENTINEL from HEAD) appear in the 17-03 truths and the docs claim table |
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

**Approval:** confirmed — plan review by sec (SECURED), execution complete, post-execution Nyquist audit 2026-10-13 found no gaps.

---

## Validation Audit 2026-10-13

| Metric | Count |
|--------|-------|
| Gaps found | 0 |
| Resolved | 0 |
| Escalated | 0 |

All 17 Per-Task Map rows cross-checked against their plan's SUMMARY: each has a real automated command with real green evidence, matching gsd-verifier's independent PASSED verdict in 17-VERIFICATION.md. No gsd-nyquist-auditor spawn needed.
