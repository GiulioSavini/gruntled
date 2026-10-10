---
phase: 04-real-repo-validation-experiment
plan: 03
subsystem: validation-experiment
tags: [benchmark, valid-06, validation-doc, terragrunt, drift-guard]

requires:
  - phase: 04-real-repo-validation-experiment
    provides: "04-01 goldens, 04-02 corpusRequire/corpusCopy/corpusDigest/tgVerifyPinned/tgEnv, 04-04 denisPinnedCommit/denisExpected/denisSilent"
provides:
  - "cmd/gruntled/bench_test.go: TestBenchmarkVsTerragrunt, BenchmarkGruntledCheckCorpus, BenchmarkTerragruntHCLValidate, benchMachineInfo"
  - "cmd/gruntled/validation_doc_test.go: TestValidationDocPins (always on)"
  - "docs/validation.md: committed record of VALID-02..06 and the denis256 secondary check"
affects: [phase-4-verification]

tech-stack:
  added: []
  patterns:
    - "Process-vs-process benchmark: same copy, same minimal env and cwd, 3 warm-ups, 21 interleaved samples, median verdict, exit code checked on every run"
    - "Doc-pin drift guard: test constants and expectation slices must appear verbatim in docs/validation.md"

key-files:
  created:
    - cmd/gruntled/bench_test.go
    - cmd/gruntled/validation_doc_test.go
    - docs/validation.md
  modified: []

key-decisions:
  - "VALID-06 PASS in 3 of 3 benchmark runs (terragrunt/gruntled median ratio 2.81, 2.13, 1.99); all runs reported, none discarded"
  - "The third benchmark run was taken after waiting for load average < 3, because runs 1-2 ran at load 8.8-17 on 4 CPUs; the verdict did not motivate it"
  - "denis256 presented as PASS with 0 false positives and 0/8 recall, a known 02-13 include-target limitation and v2 refinement candidate"
  - "benchMachineInfo also logs /proc/loadavg so machine load sits next to the numbers it produced"

requirements-completed: [VALID-06]

duration: ~35min
completed: 2026-09-29
---

# Phase 4 Plan 03: Benchmark and validation record Summary

**`gruntled check` beat plain `terragrunt hcl validate` v1.1.6 by a median ratio of 1.99-2.81 across 3 process-vs-process runs on the pinned primary corpus, and docs/validation.md now records every VALID-02..06 result, the denis256 0/8 recall limitation, and exact reproduction steps, guarded by an always-on doc-pin test.**

**MILESTONE EXPERIMENT: PASSED.** VALID-02, 03, 04, 05 and 06 all PASS on the pinned assets. The denis256 secondary check passes on precision (0 GRT001, no false positive) with recall 0/8, recorded as a known limitation.

## Task Commits

1. Task 1: bench_test.go (VALID-06): `11aa6a7`
2. Task 2: docs/validation.md + validation_doc_test.go: `9415f21`

Both pushed to origin/master.

## Experiment run (env-gated, once)

`go test -count=1 -v -run 'TestCorpusClean|TestCorpusMutation|TestTerragruntGap|TestDenis256Corpus|TestBenchmarkVsTerragrunt' ./cmd/gruntled` with all three env vars: all PASS, 0 skips.

- TestCorpusClean: `units=65 resolved=62 module_unknown=0 config_unknown=3 unknown_modules=0 errors=0 warnings=0`.
- TestCorpusMutation: rename 8/8, delete 3/3, 0 extra.
- TestTerragruntGap: terragrunt exit 0 on unmutated and mutated copies, 12 lines each, 0 added; gruntled exit 1 with 8 and 3 GRT001.
- TestDenis256Corpus: identical to 04-04 (1146 units, 897 unknown, 719 include-target, 4 GRT100, 0 GRT001).
- VALIDATION-TG: sha256 `d75a80bb...9b04`, `terragrunt version v1.1.6`.
- Goldens: all PASS. Shipped binary on the corpus: exit 0, empty stdout, stderr `gruntled: checked 65 units (3 unknown): 0 errors, 0 warnings`. Corpus `git status --porcelain` empty.

## Timing results (VALID-06)

Machine: i5-1335U, 4 logical CPUs visible to WSL2, 5.8 GiB, kernel 6.6.87.2-microsoft-standard-WSL2, go1.27.0.

| Run | loadavg (end) | gruntled median ms | terragrunt median ms | ratio | Verdict |
|---|---|---|---|---|---|
| 1 (Task 1 harness check) | 8.80 | 233.311 | 656.260 | 2.81 | PASS |
| 2 (experiment run of record) | 17.06 | 300.006 | 639.756 | 2.13 | PASS |
| 3 (after load < 3) | 3.78 | 150.653 | 299.080 | 1.99 | PASS |

Supplementary `go test -bench -benchtime=20x`: GruntledCheckCorpus (in process) 157.6 ms/op, TerragruntHCLValidate (process) 274.8 ms/op.

vs PREP baseline (terragrunt mean 0.148 s): not directly comparable. PREP ran with 12 logical CPUs and 7.6 GiB visible to WSL2; this run had 4 and 5.8 GiB, and the machine was never idle (up to 47% iowait, 2 GiB swap during run 2). The quietest run gave terragrunt a 0.299 s median, about 2x PREP. Both tools slowed together; the ratio held at 2-2.8x.

## Deviations from Plan

1. **Benchmark run 3 times, not once.** The plan allows up to 3 runs, all reported, verdict by majority. Run 1 was Task 1's verify step, run 2 was inside the full experiment, run 3 was after a bounded wait for load average < 3. The trigger was machine load, not the verdict (PASS in all). All three runs are in docs/validation.md.
2. **benchMachineInfo also logs `/proc/loadavg`** (not in the plan's field list). It adds evidence only; no assertion changed.
3. **VALID-05 output lines identified.** The doc names the 12 terragrunt lines (1 duplicate-dependency warning, 11 "has no outputs, but mock outputs provided" warnings). This was checked with one extra plain `hcl validate` run on a scratch copy, with the same minimal env.

No assertion, oracle, benchmark flag or production code was changed.

## Verification (env unset, before push)

gofmt -l . empty, go mod tidy -diff, go vet ./..., staticcheck v0.8.1, go test -count=1 ./..., check-architecture.sh, test-check-architecture.sh: all green. ci.yml untouched. Env-gated `go test ./...` with GRUNTLED_CORPUS, GRUNTLED_CORPUS_SECRET and GRUNTLED_CORPUS_DENIS256: all ok. docs/validation.md has no AI attribution.

## Self-Check: PASSED

- FOUND: cmd/gruntled/bench_test.go, cmd/gruntled/validation_doc_test.go, docs/validation.md
- FOUND: commits 11aa6a7, 9415f21 on origin/master
