---
phase: 05-graph-diagnostics
plan: 06
subsystem: corpus-validation
tags: [corpus, oracle, grt002, grt003, validation]
requires:
  - phase: 05-graph-diagnostics
    provides: "05-02..05-05 GRT002/GRT003 analyzers, loader target state, dependencies paths, goldens and docs"
provides:
  - "graphOracle: textual GRT002/GRT003 oracle independent of gruntled (scanner + regexp, os.Stat, mutual-reachability cycles)"
  - "TestCorpusGraphClean / TestCorpusGraphMutation (env-gated, 3 repos, 13 mutations, pinned terragrunt v1.1.6)"
  - "denisExpectedGraph (4 GRT002) and denisGraphOracleOnly (12, config-unknown templates)"
  - "docs/validation.md v0.2 section, pinned by TestValidationDocPins"
affects: [phase-06, phase-07]
tech-stack:
  added: []
  patterns:
    - "Terragrunt oracle signal = queue-construction message, not exit code: `run --all --non-interactive --no-auto-init --no-color --tf-path <tofu> -- version` on a scratch copy"
    - "corpusCopyFiltered: denis256 copy drops ELF binaries (1.85 GB), fidelity proven by byte-identical gruntled JSON"
key-files:
  created:
    - cmd/gruntled/corpus_graph_test.go
  modified:
    - cmd/gruntled/corpus_test.go
    - cmd/gruntled/denis256_test.go
    - cmd/gruntled/validation_doc_test.go
    - docs/validation.md
key-decisions:
  - "Pinned terragrunt is the oracle on iso20022 AND secret (secret's unnamed include {} does not break run --all -- version on v1.1.6); denis256 cannot be queued as a whole (stack/function errors), so terragrunt runs on the issue-2565 subtree and the textual oracle covers the whole tree"
  - "denis256 unmutated set = 4 GRT002 (5728-broken-includes/test.hcl:1:37, hcl/terragrunt.hcl:19:17, module-output-broken/m1:3:17, tf-lint-regeneration/dev/template:24:45); research candidates perf-tests app-template and tf-lint-regeneration/dev/apps/app-1 are NOT gruntled findings: the former is config-unknown (12 oracle-only entries, documented false negatives), the latter resolves to an existing dev/vpc"
  - "config_path = \"\" on a block: terragrunt v1.1.6 says 'config_path could not be resolved', not a cycle; gruntled's GRT003 self-loop kept unchanged and recorded as an open issue"
  - "Include paths union confirmed on real data: denis256 render-json/dependencies/app inherits ../d2; with d2 removed gruntled reports GRT002 at include.hcl:2:12 and terragrunt fails 'Found paths in the dependencies block that do not exist'"
requirements-completed: [MORE-06]
duration: 35min
completed: 2026-09-30
---

# Phase 5 Plan 06: Corpus Validation of GRT002/GRT003 Summary

GRT002/GRT003 validated on the pinned three-repo corpus: zero findings on iso20022 and secret, exactly 4 hand-checked GRT002 on denis256, 13/13 injected mutations caught exactly, all confirmed by an independent textual oracle, tsort and the pinned terragrunt v1.1.6. Results are in docs/validation.md v0.2.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | Oracle spike, textual oracle, denis256 expected set, shape test | 9a9d868 |
| 2 | Env-gated clean + mutation corpus tests | d05fd1a |
| 3 | docs/validation.md v0.2 + doc pins | c533209 |

## Spike results (pinned terragrunt v1.1.6, scratch copies)

- iso20022 unmutated: rc=1, but no queue-construction error; 36 units print `OpenTofu v1.12.6`. rc=1 comes from execution (`There is no variable named "dependency"`: no state), unrelated to the graph. `--no-auto-init` avoids the S3 backend prompt.
- iso20022 mutations (all in `iac.src/ecr_health`): missing dir and paths entry and module-only dir print `You attempted to run terragrunt in a folder that does not contain a terragrunt.hcl file ... Path: "<copy>/<target>/terragrunt.hcl"`; back edge to `lambda_health` and self-loop print `cycle detected during queue construction`. 0 units ran in each. gruntled agreed on every one.
- secret: `run --all -- version` works despite the unnamed `include {}` (queue built, 2 units ran). All 4 mutations confirmed by terragrunt.
- denis256 whole repo: fails before queue construction (stack errors: `get_repo_root` outside git in `6288`, `values.env` in `6289`). Subtree `issue-2565` (C -> B -> A) works; all 4 mutations confirmed there.
- `config_path = ""` (two-unit tree): terragrunt `ERROR skipping dependency "x" in "<tree>/a": config_path could not be resolved`, rc=1; gruntled reports `GRT003 dependency cycle: "a" -> "a"`. Both reject the tree, different reason. Recorded as open issue in docs/validation.md; semantics not changed.
- Textual oracle vs gruntled, unmutated: iso20022 0/0 (117 edges), secret 0/0 (3 edges), denis256 16 oracle vs 4 gruntled; the 12 extra are the perf-tests/perf-tests-v2 `app-template`/`dependency-template` units, config-unknown (`include-dynamic-path`). tsort (uutils 0.8.0): no loop on any corpus.

## Verification

- `go vet ./cmd/gruntled && go test -count=1 ./cmd/gruntled -run TestDenisExpectedGraphShape`: PASS
- Corpus command (all four env vars): `ok github.com/GiulioSavini/gruntled/cmd/gruntled 57.034s`; TestCorpusGraphClean (3 repos + terragrunt), TestCorpusGraphMutation 13/13 (+13 terragrunt subtests), TestCorpusClean, TestCorpusMutation, TestDenis256Corpus all PASS. `TestSecret` matches no test in cmd/gruntled (TestIncludeTargetSecretCorpus lives in internal/infrastructure/terragrunt).
- Negative check: breaking one expected message and one terragrunt expectation made the tests fail with MISS/EXTRA/oracle delta and "terragrunt does not print".
- `TestValidationDocPins`: PASS
- Full suite: gofmt OK, `go mod tidy -diff` OK, `go vet ./...` OK, staticcheck v0.8.1 OK, `go test -count=1 ./...` OK, `check-architecture.sh` OK, `test-check-architecture.sh` OK (all architecture self-tests passed)
- Corpus command rerun at c533209 (plus TestValidationDocPins): `ok ... 60.858s`, 45 PASS, 0 FAIL, 0 SKIP. Checkouts: primary and secret clean, denis256 only the pre-existing untracked crash log.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 3 - Blocking] denis256 copy does not fit in /tmp**
- **Found during:** Task 2
- **Issue:** denis256 is 1.8 GB (29 checked-in ELF terragrunt binaries, 1.85 GB); /tmp is a tmpfs with ~1 GB free, so `corpusCopy` per mutation was impossible.
- **Fix:** `corpusCopyFiltered` (corpusCopy now wraps it) skips ELF files for denis256; fidelity proven by byte-identical gruntled JSON between copy and checkout, in every mutation.
- **Commit:** d05fd1a

**2. [Rule 1 - Bug] Plan's candidate list was partly wrong**
- **Found during:** Task 1
- **Issue:** Plan listed `perf-tests/code-v2/app-template` and `tf-lint-regeneration/dev/apps/app-1` as candidates. app-1's `../../vpc` exists; the real finding is `tf-lint-regeneration/dev/template:24:45`. app-template is config-unknown, so gruntled is silent by documented design.
- **Fix:** Expected set derived from the oracle and checked by hand; shape test requires the confirmed files (incl. `5728-broken-includes/test.hcl`); disagreements recorded in `denisGraphOracleOnly` with reason.
- **Commit:** 9a9d868

**3. [Scope note] Terragrunt oracle also on secret and the denis256 subtree**
- The plan expected the textual oracle only for secret and denis256. Terragrunt v1.1.6 handles secret's unnamed include with `run --all -- version`, and runs on the denis256 `issue-2565` subtree, so every mutation is confirmed by terragrunt as well. The textual oracle still runs on all three.

## Issues Encountered / Open

- `config_path = ""` on a block: gruntled GRT003 self-loop vs terragrunt "config_path could not be resolved" (open issue in docs/validation.md; 05-05's claim that Terragrunt's filepath.Join gives the same is contradicted by the oracle).
- Untracked `terragrunt-crash-20260925T140125Z-29867.log` in the denis256 checkout predates this plan (Sep 25); left untouched. Digest checks cover it (unchanged).

## Self-Check: PASSED
