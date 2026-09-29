---
phase: 04-real-repo-validation-experiment
plan: 02
subsystem: validation-experiment
tags: [corpus, oracle, mutation, terragrunt, valid-03, valid-04, valid-05]

requires:
  - phase: 03-grt001-diagnostic-cli
    provides: "run(args, stdout, stderr) composition root, JSON schema v1"
  - phase: 02-parsing-graph-construction
    provides: "gap closure 02-06..02-11"
provides:
  - "cmd/gruntled/corpus_test.go: TestCorpusClean (VALID-03), TestCorpusMutation (VALID-04), corpusRequire, corpusDigest, corpusCopy, corpusApply, corpusOracle, corpusMutations, corpusPinnedCommit"
  - "cmd/gruntled/terragrunt_gap_test.go: TestTerragruntGap (VALID-05), tgVerifyPinned, tgEnv, tgRun, tgNormalize"
affects: [04-03]

tech-stack:
  added: []
  patterns:
    - "Env-gated corpus tests (GRUNTLED_CORPUS, GRUNTLED_TERRAGRUNT_BIN), skipped in CI"
    - "Faithful symlink-preserving scratch copy proven by tree digest"
    - "Textual oracle independent of gruntled's parser, cross-checked by grep and a crude byte count"

key-files:
  created:
    - cmd/gruntled/corpus_test.go
    - cmd/gruntled/terragrunt_gap_test.go
  modified: []

key-decisions:
  - "EXPERIMENT PASSED for VALID-03, VALID-04 and VALID-05 on the pinned primary corpus; no assertion, oracle or production code was adjusted"
  - "terragrunt runs with an explicit minimal env (PATH = pinned bin dir only); os/exec stays confined to _test.go"

requirements-completed: [VALID-03, VALID-04, VALID-05]

duration: ~35min
completed: 2026-09-29
---

# Phase 4 Plan 02: Primary-Corpus Experiment Summary

**On the pinned primary corpus gruntled reports zero diagnostics unmutated, exactly the 8 + 3 oracle-computed GRT001s after a rename and a deletion, and hash-pinned `terragrunt hcl validate` v1.1.6 exits 0 on both mutations without a single added output line.**

## Results

- **VALID-03: EXPERIMENT PASSED.** Unmutated corpus: exit 0, 0 diagnostics (any code), 0 errors, 0 warnings; text run exit 0 with empty stdout. Copy fidelity: JSON on the scratch copy is byte-identical to the JSON on the corpus.
- **VALID-04: EXPERIMENT PASSED.** `rename_role_name`: 8/8 oracle entries, 0 missing, 0 extra. `delete_mq_region`: 3/3, 0 missing, 0 extra. Every diagnostic GRT001/error. Revert: exit 0, 0 diagnostics, JSON byte-identical to the pre-state.
- **VALID-05: EXPERIMENT PASSED.** Plain `terragrunt hcl validate` exits 0 on the unmutated copy and on both mutated copies. Normalized output is 12 lines in every run, and the mutations add 0 lines. On the identical mutated trees gruntled exits 1 with the exact oracle set.
- Corpus checkout: tree digest unchanged across every test; `git status --porcelain` empty.
- terragrunt side effects: plain `hcl validate` wrote nothing into either copy (digest unchanged).

### Pinned terragrunt (measured at run time)

```
VALIDATION-TG sha256=d75a80bb264758ba00dabcb17f4b507fcdab4ca90d9e41f96750df036bf69b04 path=/home/giulio/.cache/gruntled-phase4/bin/terragrunt_linux_amd64
VALIDATION-TG version=terragrunt version v1.1.6
```

### Unknown units observed (recorded, not asserted)

summary: units=65 resolved=62 module_unknown=0 config_unknown=3 unknown_modules=0

- iac.cicd/codebuild_project config-unknown: invalid-dependency
- iac.src/scheduler_recover config-unknown: invalid-dependency
- iac.src/scheduler_timeout config-unknown: invalid-dependency

### Timings (informational; VALID-06 is 04-03's)

PREP baseline: terragrunt plain `hcl validate` mean 0.148s on primary. The machine was noticeably busier during this session:

- Paired process-vs-process, 10 alternating runs on the corpus: gruntled binary mean **0.169s** (0.07-0.29), terragrunt mean **0.377s** (0.19-0.64). The terragrunt figure is about 2.5x PREP's 0.148s, which shows how loaded the machine was.
- In-test durations (first isolated TestTerragruntGap run): terragrunt 135-186ms per run, in-process gruntled ~53ms.
- In the combined verbose log below, the first terragrunt run took 5.49s (cold start or disk cache under load). This is not a stable signal.

## Oracle hand cross-check (grep, before any env-gated run)

```
$ cd ~/.cache/gruntled-phase4/corpus/primary && grep -rn 'dependency\.s3\.outputs\.role_name\b' --include=terragrunt.hcl . && grep -rn 'dependency\.mq\.outputs\.region\b' --include=terragrunt.hcl .
iac.src/ecr_timeout/terragrunt.hcl:13:  ROLE_NAME  = dependency.s3.outputs.role_name
iac.src/ecr_outbox/terragrunt.hcl:13:  ROLE_NAME  = dependency.s3.outputs.role_name
iac.src/ecr_inbox/terragrunt.hcl:13:  ROLE_NAME  = dependency.s3.outputs.role_name
iac.src/ecr_process/terragrunt.hcl:13:  ROLE_NAME  = dependency.s3.outputs.role_name
iac.src/ecr_recover/terragrunt.hcl:13:  ROLE_NAME  = dependency.s3.outputs.role_name
iac.src/ecr_uuid/terragrunt.hcl:13:  ROLE_NAME  = dependency.s3.outputs.role_name
iac.src/ecr_health/terragrunt.hcl:13:  ROLE_NAME  = dependency.s3.outputs.role_name
iac.src/ecr_release/terragrunt.hcl:13:  ROLE_NAME  = dependency.s3.outputs.role_name
iac.mq/ecr_mq_reader/terragrunt.hcl:14:  RP2_REGION = dependency.mq.outputs.region
iac.mq/ecr_mq_writer/terragrunt.hcl:14:  RP2_REGION = dependency.mq.outputs.region
iac.mq/ecr_mq_generator/terragrunt.hcl:14:  RP2_REGION = dependency.mq.outputs.region
```

The oracle's file:line list (below, `oracle:` lines) matches it: 8 files at line 13 and 3 files at line 14. The column is 16 (`dependency.` starts at byte 16).

## Verbatim env-gated log

Command, run once against the pinned assets, with 0 SKIP lines:

```
GRUNTLED_CORPUS=$HOME/.cache/gruntled-phase4/corpus/primary \
GRUNTLED_TERRAGRUNT_BIN=$HOME/.cache/gruntled-phase4/bin/terragrunt \
go test -count=1 -v -run 'TestCorpusClean|TestCorpusMutation|TestTerragruntGap' ./cmd/gruntled
```

```
=== RUN   TestCorpusClean
    corpus_test.go:495: summary: units=65 resolved=62 module_unknown=0 config_unknown=3 unknown_modules=0 errors=0 warnings=0
    corpus_test.go:498: unknown unit: iac.cicd/codebuild_project config-unknown: invalid-dependency
    corpus_test.go:498: unknown unit: iac.src/scheduler_recover config-unknown: invalid-dependency
    corpus_test.go:498: unknown unit: iac.src/scheduler_timeout config-unknown: invalid-dependency
--- PASS: TestCorpusClean (0.40s)
=== RUN   TestCorpusMutation
=== RUN   TestCorpusMutation/rename_role_name
    corpus_test.go:538: oracle: GRT001 error iac.src/ecr_health/terragrunt.hcl:13:16 iac.src/ecr_health
    corpus_test.go:538: oracle: GRT001 error iac.src/ecr_inbox/terragrunt.hcl:13:16 iac.src/ecr_inbox
    corpus_test.go:538: oracle: GRT001 error iac.src/ecr_outbox/terragrunt.hcl:13:16 iac.src/ecr_outbox
    corpus_test.go:538: oracle: GRT001 error iac.src/ecr_process/terragrunt.hcl:13:16 iac.src/ecr_process
    corpus_test.go:538: oracle: GRT001 error iac.src/ecr_recover/terragrunt.hcl:13:16 iac.src/ecr_recover
    corpus_test.go:538: oracle: GRT001 error iac.src/ecr_release/terragrunt.hcl:13:16 iac.src/ecr_release
    corpus_test.go:538: oracle: GRT001 error iac.src/ecr_timeout/terragrunt.hcl:13:16 iac.src/ecr_timeout
    corpus_test.go:538: oracle: GRT001 error iac.src/ecr_uuid/terragrunt.hcl:13:16 iac.src/ecr_uuid
    corpus_test.go:555: gruntled: iac.src/ecr_health/terragrunt.hcl:13:16: GRT001 dependency "s3" output "role_name" is not declared by module "iac.src/s3_runtime" (target unit "iac.src/s3_runtime"); mock_outputs supplies it, so apply would silently use the mock value
    corpus_test.go:555: gruntled: iac.src/ecr_inbox/terragrunt.hcl:13:16: GRT001 dependency "s3" output "role_name" is not declared by module "iac.src/s3_runtime" (target unit "iac.src/s3_runtime"); mock_outputs supplies it, so apply would silently use the mock value
    corpus_test.go:555: gruntled: iac.src/ecr_outbox/terragrunt.hcl:13:16: GRT001 dependency "s3" output "role_name" is not declared by module "iac.src/s3_runtime" (target unit "iac.src/s3_runtime"); mock_outputs supplies it, so apply would silently use the mock value
    corpus_test.go:555: gruntled: iac.src/ecr_process/terragrunt.hcl:13:16: GRT001 dependency "s3" output "role_name" is not declared by module "iac.src/s3_runtime" (target unit "iac.src/s3_runtime"); mock_outputs supplies it, so apply would silently use the mock value
    corpus_test.go:555: gruntled: iac.src/ecr_recover/terragrunt.hcl:13:16: GRT001 dependency "s3" output "role_name" is not declared by module "iac.src/s3_runtime" (target unit "iac.src/s3_runtime"); mock_outputs supplies it, so apply would silently use the mock value
    corpus_test.go:555: gruntled: iac.src/ecr_release/terragrunt.hcl:13:16: GRT001 dependency "s3" output "role_name" is not declared by module "iac.src/s3_runtime" (target unit "iac.src/s3_runtime"); mock_outputs supplies it, so apply would silently use the mock value
    corpus_test.go:555: gruntled: iac.src/ecr_timeout/terragrunt.hcl:13:16: GRT001 dependency "s3" output "role_name" is not declared by module "iac.src/s3_runtime" (target unit "iac.src/s3_runtime"); mock_outputs supplies it, so apply would silently use the mock value
    corpus_test.go:555: gruntled: iac.src/ecr_uuid/terragrunt.hcl:13:16: GRT001 dependency "s3" output "role_name" is not declared by module "iac.src/s3_runtime" (target unit "iac.src/s3_runtime"); mock_outputs supplies it, so apply would silently use the mock value
=== RUN   TestCorpusMutation/delete_mq_region
    corpus_test.go:538: oracle: GRT001 error iac.mq/ecr_mq_generator/terragrunt.hcl:14:16 iac.mq/ecr_mq_generator
    corpus_test.go:538: oracle: GRT001 error iac.mq/ecr_mq_reader/terragrunt.hcl:14:16 iac.mq/ecr_mq_reader
    corpus_test.go:538: oracle: GRT001 error iac.mq/ecr_mq_writer/terragrunt.hcl:14:16 iac.mq/ecr_mq_writer
    corpus_test.go:555: gruntled: iac.mq/ecr_mq_generator/terragrunt.hcl:14:16: GRT001 dependency "mq" output "region" is not declared by module "iac.mq/mq_broker" (target unit "iac.mq/mq_broker"); mock_outputs supplies it, so apply would silently use the mock value
    corpus_test.go:555: gruntled: iac.mq/ecr_mq_reader/terragrunt.hcl:14:16: GRT001 dependency "mq" output "region" is not declared by module "iac.mq/mq_broker" (target unit "iac.mq/mq_broker"); mock_outputs supplies it, so apply would silently use the mock value
    corpus_test.go:555: gruntled: iac.mq/ecr_mq_writer/terragrunt.hcl:14:16: GRT001 dependency "mq" output "region" is not declared by module "iac.mq/mq_broker" (target unit "iac.mq/mq_broker"); mock_outputs supplies it, so apply would silently use the mock value
--- PASS: TestCorpusMutation (0.69s)
    --- PASS: TestCorpusMutation/rename_role_name (0.37s)
    --- PASS: TestCorpusMutation/delete_mq_region (0.28s)
=== RUN   TestTerragruntGap
    terragrunt_gap_test.go:147: VALIDATION-TG sha256=d75a80bb264758ba00dabcb17f4b507fcdab4ca90d9e41f96750df036bf69b04 path=/home/giulio/.cache/gruntled-phase4/bin/terragrunt_linux_amd64
    terragrunt_gap_test.go:147: VALIDATION-TG version=terragrunt version v1.1.6
=== RUN   TestTerragruntGap/rename_role_name
    terragrunt_gap_test.go:166: terragrunt unmutated: exit 0 in 5.486180175s, 12 output lines
    terragrunt_gap_test.go:167: terragrunt mutated:   exit 0 in 877.042493ms, 12 output lines
    terragrunt_gap_test.go:168: terragrunt lines added by the mutation: 0
    terragrunt_gap_test.go:195: gruntled mutated:     exit 1 in 345.656887ms, 8 GRT001
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.src/ecr_health/terragrunt.hcl:13:16 iac.src/ecr_health
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.src/ecr_inbox/terragrunt.hcl:13:16 iac.src/ecr_inbox
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.src/ecr_outbox/terragrunt.hcl:13:16 iac.src/ecr_outbox
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.src/ecr_process/terragrunt.hcl:13:16 iac.src/ecr_process
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.src/ecr_recover/terragrunt.hcl:13:16 iac.src/ecr_recover
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.src/ecr_release/terragrunt.hcl:13:16 iac.src/ecr_release
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.src/ecr_timeout/terragrunt.hcl:13:16 iac.src/ecr_timeout
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.src/ecr_uuid/terragrunt.hcl:13:16 iac.src/ecr_uuid
    terragrunt_gap_test.go:204: terragrunt side effects: unmutated copy changed=false, mutated copy changed=false
=== RUN   TestTerragruntGap/delete_mq_region
    terragrunt_gap_test.go:166: terragrunt unmutated: exit 0 in 491.82967ms, 12 output lines
    terragrunt_gap_test.go:167: terragrunt mutated:   exit 0 in 655.160697ms, 12 output lines
    terragrunt_gap_test.go:168: terragrunt lines added by the mutation: 0
    terragrunt_gap_test.go:195: gruntled mutated:     exit 1 in 235.118173ms, 3 GRT001
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.mq/ecr_mq_generator/terragrunt.hcl:14:16 iac.mq/ecr_mq_generator
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.mq/ecr_mq_reader/terragrunt.hcl:14:16 iac.mq/ecr_mq_reader
    terragrunt_gap_test.go:197:   gruntled GRT001 error iac.mq/ecr_mq_writer/terragrunt.hcl:14:16 iac.mq/ecr_mq_writer
    terragrunt_gap_test.go:204: terragrunt side effects: unmutated copy changed=false, mutated copy changed=false
--- PASS: TestTerragruntGap (20.85s)
    --- PASS: TestTerragruntGap/rename_role_name (15.64s)
    --- PASS: TestTerragruntGap/delete_mq_region (1.99s)
PASS
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	21.942s
```

With the env vars unset, all three tests SKIP (`GRUNTLED_CORPUS not set` / `GRUNTLED_TERRAGRUNT_BIN not set`), so CI is unaffected.

## Task Commits

1. **Task 1: corpus_test.go (VALID-03, VALID-04)**: `4ef9758`
2. **Task 2: terragrunt_gap_test.go (VALID-05)**: `465b260`

## Verification (env unset)

gofmt -l . is empty. go mod tidy -diff, go vet ./..., staticcheck v0.8.1 (GOTOOLCHAIN=go1.27.0), go test -count=1 ./..., scripts/check-architecture.sh and scripts/test-check-architecture.sh all pass. `go list -deps ./cmd/gruntled` contains no os/exec. ci.yml is unchanged.

## Deviations from Plan

- **Minor additions (not assertion changes):**
  - TestTerragruntGap also logs the normalized output line count per run, so "0 added lines" is visibly not vacuous (12 lines each run).
  - corpusMutation carries a `Label` field used only by the crude byte-count cross-check.
- **Scope note:** as the plan requires, `go test ./...` was never run with GRUNTLED_CORPUS set (TestCorpusSmoke in internal/infrastructure/terragrunt is outside this plan).

Otherwise the plan was executed as written. No harness defect was found, and no FAILURE RULE path was taken.

## Next Phase Readiness

04-03 can reuse corpusCopy, corpusDigest, corpusPinnedCommit, corpusRequire, corpusMutations, tgVerifyPinned, tgEnv and tgRun (all take testing.TB) and can quote the VALIDATION-TG lines.

## Self-Check: PASSED

Both files exist; commits 4ef9758 and 465b260 are in history.
