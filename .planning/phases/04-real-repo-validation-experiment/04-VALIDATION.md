---
phase: 4
slug: real-repo-validation-experiment
status: draft
nyquist_compliant: true
wave_0_complete: false
created: 2026-09-28
---

# Phase 4 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go `testing` (go 1.27): table and golden tests, `b.Loop()` benchmarks; `rogpeppe/go-internal/txtar` (test-only, already in go.mod from 03-03) for golden fixtures; `internal/testsupport/synthrepo` for full-scale trees; `os/exec` in `_test.go` only, for the pinned terragrunt binary and the gruntled build |
| **Config file** | none. The env gates `GRUNTLED_CORPUS` (pinned primary-corpus checkout), `GRUNTLED_TERRAGRUNT_BIN` (pinned terragrunt v1.1.6) and `GRUNTLED_CORPUS_DENIS256` (pinned secondary corpus, 04-CONTEXT amendment) replace config. All are unset in CI, where the gated tests skip |
| **Quick run command** | `go test -count=1 ./cmd/gruntled -run 'TestGolden\|TestValidationDocPins'` |
| **Full suite command** | `test -z "$("$(go env GOROOT)/bin/gofmt" -l .)" && go mod tidy -diff && go vet ./... && GOTOOLCHAIN=go1.27.0 go run honnef.co/go/tools/cmd/staticcheck@v0.8.1 ./... && go test -count=1 ./... && bash scripts/check-architecture.sh && bash scripts/test-check-architecture.sh` (no `-race` locally; CI adds it) |
| **Experiment command (env-gated, local only)** | `GRUNTLED_CORPUS=$HOME/.cache/gruntled-phase4/corpus/primary GRUNTLED_TERRAGRUNT_BIN=$HOME/.cache/gruntled-phase4/bin/terragrunt GRUNTLED_CORPUS_DENIS256=$HOME/.cache/gruntled-phase4/corpus/denis256 go test -count=1 -v -run 'TestCorpusClean\|TestCorpusMutation\|TestTerragruntGap\|TestDenis256Corpus\|TestBenchmarkVsTerragrunt' ./cmd/gruntled` |
| **Estimated runtime** | quick ~15 s (2000 synthrepo units); full ~100 s; experiment ~35 s (terragrunt runs about 0.15 s each, and the benchmark does 48 of them; denis256 is two gruntled runs of ~1.2 s plus a tree digest) |

---

## Sampling Rate

- **After every task commit:** Run the quick run command, plus that task's own env-gated command once the pinned assets are in use.
- **After every plan wave:** Run the full suite command with the env vars unset (CI parity), then the experiment command once with them set.
- **Before `/gsd:verify-work`:** Full suite green with env unset, experiment run recorded in docs/validation.md, corpus checkout `git status --porcelain` empty, and ci.yml unchanged.
- **Max feedback latency:** ~15 seconds (quick).

---

## Per-Task Verification Map

| Task ID | Plan | Wave | Requirement | Test Type | Automated Command | File Exists | Status |
|---------|------|------|-------------|-----------|-------------------|-------------|--------|
| 4-01-01 | 01 | 1 | VALID-02 | golden (synthrepo full scale + 3 txtar) | `go test -count=1 ./cmd/gruntled -run 'TestGoldenSynthrepoFullScale\|TestGoldenFixtures/(live_clean\|live_broken\|shared_include)' -v` | ❌ W0 (created in task) | ⬜ pending |
| 4-01-02 | 01 | 1 | VALID-02 | golden (7 more txtar) + full suite | `go test -count=1 ./cmd/gruntled -run TestGolden -v && <full suite>` | ❌ W0 (created in task) | ⬜ pending |
| 4-02-01 | 02 | 1 | VALID-03, VALID-04 | integration (env-gated, corpus) | `GRUNTLED_CORPUS=... go test -count=1 -v -run 'TestCorpusClean\|TestCorpusMutation' ./cmd/gruntled` (+ skip check with env unset) | ❌ W0 (created in task) | ⬜ pending |
| 4-02-02 | 02 | 1 | VALID-05 | integration (env-gated, corpus + pinned terragrunt) | `GRUNTLED_CORPUS=... GRUNTLED_TERRAGRUNT_BIN=... go test -count=1 -v -run TestTerragruntGap ./cmd/gruntled && <full suite> && test -z "$(go list -deps ./cmd/gruntled \| grep -x os/exec)"` | ❌ W0 (created in task) | ⬜ pending |
| 4-04-01 | 04 | 1 | VALID-02 (secondary corpus) | integration (env-gated, denis256): pinned-commit guard + text self-check of the expectation table | `go test -count=1 -run TestDenis256Corpus -v ./cmd/gruntled` (skip check) `&& GRUNTLED_CORPUS_DENIS256=... go test -count=1 -v -run 'TestDenis256Corpus/expectations_match_corpus_text' ./cmd/gruntled` | ❌ W0 (created in task) | ⬜ pending |
| 4-04-02 | 04 | 1 | VALID-02 (secondary corpus) | integration (env-gated, denis256): exact GRT001 set, no panic, determinism + full suite | `GRUNTLED_CORPUS_DENIS256=... go test -count=1 -v -run TestDenis256Corpus ./cmd/gruntled && <full suite>` | ❌ W0 (created in task) | ⬜ pending |
| 4-03-01 | 03 | 2 | VALID-06 | benchmark (env-gated, process vs process) | `GRUNTLED_CORPUS=... GRUNTLED_TERRAGRUNT_BIN=... go test -count=1 -v -run TestBenchmarkVsTerragrunt ./cmd/gruntled` (+ skip check with env unset) | ❌ W0 (created in task) | ⬜ pending |
| 4-03-02 | 03 | 2 | VALID-02..06 (results record) | doc drift guard + full suite | `go test -count=1 -run 'TestValidationDocPins\|TestGolden' ./cmd/gruntled && grep -q '^## Outcome' docs/validation.md && <full suite>` | ❌ W0 (created in task) | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

Assertion discipline (from 04-CONTEXT):
- VALID-03 asserts zero diagnostics of any code. It does not assert PREP's unit counts, which are logged.
- VALID-04 asserts exact set equality against a textual oracle recomputed at test time, cross-checked with `grep` and against the pinned-text counts 8 and 3.
- VALID-05 asserts exit 0 on both the baseline and the mutated copy, and that no added output line names the broken output.
- VALID-06 asserts median(gruntled) < median(terragrunt), with every run's exit code checked.
- denis256 (secondary) asserts exact GRT001 set equality with the 8 hand-derived entries (position, unit, full message including suffix presence), the disabled-dependency reference silent, exit 1, codes only GRT001/GRT100, byte-identical reruns and an unchanged tree digest. GRT100 count and unit counts are logged only.
- A failing assertion is recorded, never loosened.

---

## Wave 0 Requirements

Every task creates its own tests in the same task. There are no separate Wave 0 stubs.

- [ ] Phase 3 executed and verified: `run()` in cmd/gruntled, the JSON presenter, go-internal in go.mod. Every plan checks this precondition first.
- [ ] Phase 2 gap-closure plans (02-06 lazy guard, 02-07 stack, include-target and non-default-file guards) merged. The lazy_guards and dependency_edges goldens depend on them.
- [ ] Pinned assets present: `~/.cache/gruntled-phase4/bin/terragrunt` (SHA256 d75a80bb…), `~/.cache/gruntled-phase4/corpus/primary` at e6c55d11… (detached), and `~/.cache/gruntled-phase4/corpus/denis256` at 726485e6… (HEAD on refs/heads/master, resolved by the test).
- [ ] `cmd/gruntled/golden_test.go` + `testdata/golden/*.txtar` (4-01-01, 4-01-02)
- [ ] `cmd/gruntled/corpus_test.go` (4-02-01), `cmd/gruntled/terragrunt_gap_test.go` (4-02-02)
- [ ] `cmd/gruntled/denis256_test.go` (4-04-01, 4-04-02)
- [ ] `cmd/gruntled/bench_test.go` (4-03-01), `cmd/gruntled/validation_doc_test.go` + `docs/validation.md` (4-03-02)
- [ ] No framework install and no go.mod change.

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Experiment run against the pinned corpus and binary | VALID-03..06 | The assets live outside the repo and are deliberately not in CI (04-CONTEXT). The tests are automated but need the two env vars set locally | Run the experiment command above once per wave-2 completion. Paste the verbose log into the SUMMARY and transcribe it into docs/validation.md |
| denis256 run against the pinned secondary checkout | VALID-02 (secondary) | Asset outside the repo, not in CI | Run with `GRUNTLED_CORPUS_DENIS256` set once in wave 1 (04-04) and again in 04-03's full run; paste the log into the SUMMARY |
| Corpus checkout untouched | VALID-04/05 (mutation hygiene) | Checks a directory outside the repo | `test -z "$(git -C ~/.cache/gruntled-phase4/corpus/primary status --porcelain)"`. The tests also assert a before/after tree digest |

*Every requirement behavior has an automated test. The only manual part is supplying the three env vars locally.*

---

## Validation Sign-Off

- [x] All tasks have `<automated>` verify or Wave 0 dependencies
- [x] Sampling continuity: no 3 consecutive tasks without automated verify
- [x] Wave 0 covers all MISSING references
- [x] No watch-mode flags
- [x] Feedback latency < 20s (quick)
- [x] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
