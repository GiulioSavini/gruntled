---
phase: 04-real-repo-validation-experiment
plan: 04
subsystem: validation-experiment
tags: [corpus, denis256, secondary-corpus, grt001, determinism, valid-02]

requires:
  - phase: 03-grt001-diagnostic-cli
    provides: "run(args, stdout, stderr), JSON schema v1, GRT001 message contract (docs/cli.md)"
  - phase: 02-parsing-graph-construction
    provides: "gap closure 02-06..02-13 (lazy guard, stack/include targets, 02-13 include-target rule)"
provides:
  - "cmd/gruntled/denis256_test.go: TestDenis256Corpus, denisPinnedCommit, denisExpected (8), denisSilent (1), denisMessage, denisMaskSuffix, denisRequire, denisHeadCommit, denisDigest"
affects: [04-03]

tech-stack:
  added: []
  patterns:
    - "Env-gated secondary corpus (GRUNTLED_CORPUS_DENIS256), skipped in CI, fails on wrong commit"
    - "Hand-derived expectation self-checked against corpus bytes before gruntled runs"
    - "Misses explained, not hidden: each known reference must map to an unknown unit/module"

key-files:
  created:
    - cmd/gruntled/denis256_test.go
  modified:
    - .planning/phases/04-real-repo-validation-experiment/04-04-PLAN.md

key-decisions:
  - "Orchestrator amendment: after 02-13 the expected GRT001 set on denis256 is EMPTY; the 8 hand-derived references are kept as corpus facts and each must lie on an unknown referring unit, target unit or target module"
  - "Amendment check widened from 'referring unit unknown' to 'referring unit OR target unit OR target module unknown': #1 and #5 have a resolved referring unit and an include-target target, which DIAG-03 silences"
  - "No production code, no 02-13 rule, no expectation row changed"

requirements-completed: []

duration: ~13min
completed: 2026-09-29
---

# Phase 4 Plan 04: denis256 Secondary-Corpus Check Summary

**Env-gated exact-set check on denis256/terragrunt-tests @ 726485e6. It proves no panic, deterministic JSON and zero false positives (0 GRT001) on 1146 hostile units. All 8 hand-derived references go unreported because the 02-13 include-target rule makes their units unknown.**

**SECONDARY CORPUS CHECK PASSED (amended expectation: empty GRT001 set; every one of the 8 known references is explained by an unknown include-target unit).**

## Performance

- Duration: ~13 min
- Tasks: 2/2
- Files: 1 created (test), 1 amended (plan intent)

## Task Commits

1. Task 1: denis256_test.go (pinned-commit guard, text self-check, amended set assertions): `66da448`
2. Task 2: env-gated run and full suite. There was no code change, so there is no separate commit. The results are in this SUMMARY.

## Results

| Check | Result |
|---|---|
| Env unset | `--- SKIP: TestDenis256Corpus` ("GRUNTLED_CORPUS_DENIS256 not set") |
| Pinned commit (HEAD `ref: refs/heads/master` resolved) | 726485e699a70c02dabbde629f66c0119e197357 OK |
| Text self-check (8 expected + 1 silent) | PASS |
| Exit code, two runs | 1, 1 |
| JSON stdout byte-identical | yes |
| Codes seen | GRT100 x4, GRT001 x0, nothing else |
| GRT001 set | empty (asserted), nothing at the silent position :18:12 |
| 8 known references | all unreported, all explained by `config-unknown/include-target` (below) |
| Corpus digest before/after | identical |

### Coverage trade-off (input for 04-03 docs/validation.md and a v2 include-target refinement)

The 02-13 conservatism costs recall on denis256. When any include's file name is dynamic or `terragrunt.hcl`, every include-free unit becomes an include target and is skipped as unknown. This keeps zero false positives, but here it silences all 8 references that the locked DIAG-03 rules would report if the units resolved: the 2 genuine fixture bugs (#4 `issue-2631/main` `dep.outputs.a`, #7 `mocks/module1` `module2.outputs.vpc_id2`) and the 6 mock-only keys (#1, #2, #3, #5, #6, #8, of which 5 would carry the mock-masking suffix).

- Measured: 1146 units, 249 resolved, 157 module-unknown, 740 config-unknown, 4 unknown modules. That is 897 unknown, of which 719 are `include-target`.
- Recall on the known references: 0/8. Precision: no false positives (0 GRT001; the `enabled = false` reference stays silent).
- Per reference (from the log):
  - #1 issue-2163/app and #5 issue-2718/app: the referring unit resolved, the target is `config-unknown/include-target`.
  - #2, #3, #4, #6, #7, #8: both the referring unit and the target are `config-unknown/include-target`.
- A v2 refinement that narrows the include-target rule would recover these 8. The 8 rows (with messages and suffixes) stay in `denisExpected` as the regression target.

## Hand cross-check (Task 2 step 1)

```
$ cd ~/.cache/gruntled-phase4/corpus/denis256 && git rev-parse HEAD && grep -n 'dependency\.[a-z0-9_]*\.outputs\.' ...
726485e699a70c02dabbde629f66c0119e197357
issue-2163/app/terragrunt.hcl:14:  fa_service_plan_id = dependency.app_service_plan01.outputs.asp_id
issue-2718/app/terragrunt.hcl:23:  aws_vpc_subnet_a_id = dependency.vpc_main.outputs.aws_subnet_public_output[format("%s-%s-default-public-a", local.aws_project_name, local.env)].id
mock-output/module1/terragrunt.hcl:20:  attribute  = dependency.module2.outputs.attribute
mock-output/module1/terragrunt.hcl:21:  hello      = dependency.module2.outputs.hello
mock-output/module1/terragrunt.hcl:22:  subnetwork = dependency.module2.outputs.subnets["dummy"]["name"]
issue-2631/main/terragrunt.hcl:9:  a   = dependency.dep.outputs.a
optional-dependency/reference-disabled-dependency/app/terragrunt.hcl:17:  vpc_id = dependency.vpc.outputs.vpc_id
optional-dependency/reference-disabled-dependency/app/terragrunt.hcl:18:  db     = dependency.db.outputs.db
issue-2405/app/terragrunt.hcl:16:  vpc_id          = dependency.vpc.outputs.vpc_id
issue-2405/app/terragrunt.hcl:17:  private_subnets = dependency.vpc.outputs.private_subnets
mocks/module1/terragrunt.hcl:10:  vpc_id = dependency.module2.outputs.vpc_id2
$ grep -rn 'enabled' optional-dependency/reference-disabled-dependency/app/terragrunt.hcl
optional-dependency/reference-disabled-dependency/app/terragrunt.hcl:3:  enabled     = true
optional-dependency/reference-disabled-dependency/app/terragrunt.hcl:10:  enabled     = false
```

## Verbatim env-gated log

`GRUNTLED_CORPUS_DENIS256=$HOME/.cache/gruntled-phase4/corpus/denis256 GOTOOLCHAIN=go1.27.0 go test -count=1 -v -run TestDenis256Corpus ./cmd/gruntled`

```
=== RUN   TestDenis256Corpus
=== RUN   TestDenis256Corpus/expectations_match_corpus_text
=== RUN   TestDenis256Corpus/no_panic_deterministic
    denis256_test.go:357: 
        DENIS256 units=1146
        DENIS256 resolved=249
        DENIS256 module_unknown=157
        DENIS256 config_unknown=740
        DENIS256 unknown_modules=4
        DENIS256 errors=4
        DENIS256 warnings=0
        DENIS256 grt100=4
        DENIS256 grt001=0
        DENIS256 unknown_reason include-target=719
        DENIS256 unknown_reason remote-source=132
        DENIS256 unknown_reason generate-may-declare-outputs=14
        DENIS256 unknown_reason include-dynamic-path=11
        DENIS256 unknown_reason source-dynamic-path=9
        DENIS256 unknown_reason syntax-error=5
        DENIS256 unknown_reason invalid-include=2
        DENIS256 unknown_reason config-too-deep=1
        DENIS256 unknown_reason include-not-found=1
        DENIS256 unknown_reason json-config-unsupported=1
        DENIS256 unknown_reason source-outside-repo=1
        DENIS256 unknown_reason unit-dir-overlays-module=1
        DENIS256 grt100 encryption/terragrunt.hcl:14:20
        DENIS256 grt100 include-error/terragrunt.hcl:28:19
        DENIS256 grt100 issue-3368/terragrunt.hcl:24:68
        DENIS256 grt100 scaffold/test1/.boilerplate/terragrunt.hcl:7:24
=== RUN   TestDenis256Corpus/grt001_exact_set
=== RUN   TestDenis256Corpus/expected_refs_explained_by_unknown
    denis256_test.go:414: not reported issue-2163/app/terragrunt.hcl:14:24 issue-2163/app | dependency "app_service_plan01" output "asp_id" is not declared by module "issue-2163/module" (target unit "issue-2163/module")
            unit issue-2163/app: resolved; target issue-2163/module: config-unknown/include-target; target module: known
    denis256_test.go:414: not reported issue-2405/app/terragrunt.hcl:16:21 issue-2405/app | dependency "vpc" output "vpc_id" is not declared by module "issue-2405/vpc" (target unit "issue-2405/vpc"); mock_outputs supplies it, so apply would silently use the mock value
            unit issue-2405/app: config-unknown/include-target; target issue-2405/vpc: config-unknown/include-target; target module: known
    denis256_test.go:414: not reported issue-2405/app/terragrunt.hcl:17:21 issue-2405/app | dependency "vpc" output "private_subnets" is not declared by module "issue-2405/vpc" (target unit "issue-2405/vpc"); mock_outputs supplies it, so apply would silently use the mock value
            unit issue-2405/app: config-unknown/include-target; target issue-2405/vpc: config-unknown/include-target; target module: known
    denis256_test.go:414: not reported issue-2631/main/terragrunt.hcl:9:9 issue-2631/main | dependency "dep" output "a" is not declared by module "issue-2631/dependency" (target unit "issue-2631/dependency")
            unit issue-2631/main: config-unknown/include-target; target issue-2631/dependency: config-unknown/include-target; target module: known
    denis256_test.go:414: not reported issue-2718/app/terragrunt.hcl:23:25 issue-2718/app | dependency "vpc_main" output "aws_subnet_public_output" is not declared by module "issue-2718/vpc" (target unit "issue-2718/vpc"); mock_outputs supplies it, so apply would silently use the mock value
            unit issue-2718/app: resolved; target issue-2718/vpc: config-unknown/include-target; target module: known
    denis256_test.go:414: not reported mock-output/module1/terragrunt.hcl:22:16 mock-output/module1 | dependency "module2" output "subnets" is not declared by module "mock-output/module2" (target unit "mock-output/module2"); mock_outputs supplies it, so apply would silently use the mock value
            unit mock-output/module1: config-unknown/include-target; target mock-output/module2: config-unknown/include-target; target module: known
    denis256_test.go:414: not reported mocks/module1/terragrunt.hcl:10:12 mocks/module1 | dependency "module2" output "vpc_id2" is not declared by module "mocks/module2" (target unit "mocks/module2")
            unit mocks/module1: config-unknown/include-target; target mocks/module2: config-unknown/include-target; target module: known
    denis256_test.go:414: not reported optional-dependency/reference-disabled-dependency/app/terragrunt.hcl:17:12 optional-dependency/reference-disabled-dependency/app | dependency "vpc" output "vpc_id" is not declared by module "optional-dependency/reference-disabled-dependency/vpc" (target unit "optional-dependency/reference-disabled-dependency/vpc"); mock_outputs supplies it, so apply would silently use the mock value
            unit optional-dependency/reference-disabled-dependency/app: config-unknown/include-target; target optional-dependency/reference-disabled-dependency/vpc: config-unknown/include-target; target module: known
--- PASS: TestDenis256Corpus (60.71s)
    --- PASS: TestDenis256Corpus/expectations_match_corpus_text (0.00s)
    --- PASS: TestDenis256Corpus/no_panic_deterministic (0.79s)
    --- PASS: TestDenis256Corpus/grt001_exact_set (0.00s)
    --- PASS: TestDenis256Corpus/expected_refs_explained_by_unknown (0.00s)
PASS
ok  	github.com/GiulioSavini/gruntled/cmd/gruntled	60.871s
```

## Full suite

- The following all ran with the env vars unset and are green: `gofmt -l .` (empty), `go mod tidy -diff`, `go vet ./...`, staticcheck v0.8.1, `go test -count=1 ./...`, `scripts/check-architecture.sh`, `scripts/test-check-architecture.sh`. `ci.yml` is untouched.
- Env-gated run with `GRUNTLED_CORPUS` (primary), `GRUNTLED_CORPUS_SECRET` and `GRUNTLED_CORPUS_DENIS256` set, over `./cmd/gruntled ./internal/...`: all ok.
  - The only skips were `TestTerragruntGap` (needs `GRUNTLED_TERRAGRUNT_BIN`) and `TestHeavyG17Reproductions` (needs `GRUNTLED_HEAVY_TESTS`).
  - `TestTerragruntGap` was then run separately with the pinned binary: PASS.
  - `TestDenis256Corpus` did not skip.

## Deviations from Plan

**1. [Orchestrator amendment] Expected GRT001 set changed from the 8 hand-derived entries to EMPTY**
- Cause: gap-closure 02-13 (include_targets.go / reasons.go) landed after this plan was written. On denis256, 719 units are `include-target`, covering every one of the 8 references.
- Applied: the 8 rows plus the text self-check are kept verbatim. `grt001_exact_set` now asserts an empty set, severity error and nothing at the silent position. A new subtest, `expected_refs_explained_by_unknown`, fails on any known reference that sits on a fully resolved path and is not reported. `denisMessage` is used there to log the message that would be emitted.
- Also recorded in the plan's `<objective>` as an AMENDMENT paragraph.
- No production code, 02-13 rule or table row changed.

**2. [Rule 1 - amendment wording] "Unit reported as unknown" widened to "referring unit, target unit or target module unknown"**
- The literal amendment ("every reference lies in a unit that gruntled reports as unknown") would fail #1 (issue-2163/app) and #5 (issue-2718/app). Both referring units resolve, but their targets (`issue-2163/module`, `issue-2718/vpc`) are `config-unknown/include-target`.
- Under DIAG-03 an unknown target is silent, so these are explained silences, not misses. The check accepts an unknown referring unit, target unit or target module, and logs each status. A reference with both sides resolved and no GRT001 still fails.

**3. Logged, not asserted.** The unknown-reason histogram has 12 reasons. The orchestrator's quick measurement listed 8; the extra four each have count 1: config-too-deep, json-config-unsupported, source-outside-repo, unit-dir-overlays-module. The totals match (897 unknown).

**4. `denisMaskSuffix`** matches docs/cli.md line 181 verbatim. No wording change.

## Issues Encountered

- The first env-gated run took about 61s, mostly the cold-cache digest walk. Later runs took about 12s.

## Next Phase Readiness

- 04-03 can read `denisPinnedCommit`, `denisExpected` (8, with Why), and `denisSilent` (1). docs/validation.md should present denis256 as: no panic, deterministic, 0 false positives, and 0/8 recall due to 02-13, with the numbers above.

## Self-Check: PASSED

- FOUND: cmd/gruntled/denis256_test.go
- FOUND: commit 66da448
