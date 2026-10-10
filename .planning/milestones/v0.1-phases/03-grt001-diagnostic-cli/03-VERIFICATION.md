---
phase: 03-grt001-diagnostic-cli
verified: 2026-09-29T00:00:00Z
status: passed
score: 5/5 must-haves verified
---

# Phase 3: GRT001 Diagnostic & CLI Verification Report

**Phase Goal:** `gruntled check` runs end-to-end against a repository on disk and reports GRT001/GRT100 diagnostics that are correct, deterministic, and safe to script against in CI.
**Verified:** 2026-09-29 (master ad91ec6)
**Status:** passed
**Re-verification:** No, initial verification

## Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | GRT001 on genuine mismatch, GRT100 on invalid HCL, repo-relative path/line/col/code | VERIFIED | Corpus copy with `output "role_name"` renamed in s3_runtime: exit 1, exactly 8 GRT001, lines like `iac.src/ecr_health/terragrunt.hcl:13:16: GRT001 ...`. Unclosed-brace file: `terragrunt.hcl:1:11: GRT100 ...`, exit 1. No absolute paths in output. JSON has file/line/column/code/unit. |
| 2 | mock_outputs rule explicit, documented, tested; corpus pattern covered; no mocks never manufactures diagnostic | VERIFIED | Mocks never suppress GRT001 (message notes "mock_outputs supplies it"). docs/cli.md documents the rule. Testscripts diag03_corpus_shape (merge_with_state + apply), diag03_issue2163, diag03_no_mocks, diag03_silent_rows. Unmodified primary corpus: 0 errors. |
| 3 | Byte-identical output across runs and differently-named checkouts | VERIFIED | Two copies (m2, m3) diffed: identical. TestDeterministicAcrossCheckouts exists and passes. |
| 4 | Exit 0 clean, non-zero on error, documented table | VERIFIED | Primary corpus exit 0, 65 units (3 unknown); mutated exit 1; unknown flag exit 2; missing path exit 3. Table in `--help` and docs/cli.md; TestHelpMatchesDocs passes. |
| 5 | No network, no exec, no writes, verified by test | VERIFIED | TestNoWrites in cmd/gruntled/e2e_test.go; arch rule binary-no-net-no-exec in scripts/check-architecture.sh. No net/http, net or os/exec in non-test code (only integration_test.go uses them). |

**Score:** 5/5

## Behavioural Checks

- `GOTOOLCHAIN=go1.27.0 go test -count=1 ./...`: all packages ok.
- `bash scripts/check-architecture.sh`: OK (test-check-architecture.sh skipped as instructed, CI runs it).
- Secret corpus: 0 errors, exit 0, 4 units (1 unknown).
- JSON output has `"version": 1`.
- Locked decisions honoured: stdlib flag (flags after path, `--`), exit codes 0/1/2/3, DDD layering (arch check passes).

## Requirements Coverage

DIAG-01, DIAG-02, DIAG-03, DIAG-04, CLI-01, CLI-02, CLI-03, CLI-04, CLI-05: all SATISFIED by the evidence above. No orphaned requirements.

## Anti-Patterns

None blocking found.

## Human Verification Required

None.

## Gaps Summary

No gaps. Phase 3 goal achieved.

_Verified: 2026-09-29_
_Verifier: Claude (gsd-verifier)_
