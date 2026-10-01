---
phase: 06-machine-readable-output
plan: 05
subsystem: ci
tags: [sarif, ci, code-scanning, upload-sarif, schema, int-02]
requires:
  - "gruntled check --format sarif (06-03)"
  - "SARIF goldens/structure tests (06-04)"
provides:
  - "vendored OASIS SARIF 2.1.0 schema (cmd/gruntled/testdata/sarif-schema-2.1.0.json)"
  - "deliberate-defect fixture (GRT001, GRT002, GRT003)"
  - "check job step sarif-schema (jv validation of real output)"
  - "master-only sarif-upload job (upload-sarif, category gruntled-fixture)"
  - "green upload run evidence for Phase 6 verification"
affects: [07]
tech-stack:
  added: ["github/codeql-action/upload-sarif (SHA-pinned, v4)", "santhosh-tekuri/jsonschema jv v0.7.0 (go run, CI only)"]
  patterns: ["CI step tolerates exit 1 only, fails on any other code or empty output", "jq rewrite of artifactLocation.uri to repo-root-relative paths before upload"]
key-files:
  created:
    - cmd/gruntled/testdata/sarif-schema-2.1.0.json
    - cmd/gruntled/testdata/sarif-fixture/live/app/terragrunt.hcl
    - cmd/gruntled/testdata/sarif-fixture/live/network/terragrunt.hcl
    - cmd/gruntled/testdata/sarif-fixture/live/ring/a/terragrunt.hcl
    - cmd/gruntled/testdata/sarif-fixture/live/ring/b/terragrunt.hcl
    - cmd/gruntled/testdata/sarif-fixture/modules/network/outputs.tf
  modified:
    - .github/workflows/ci.yml
decisions:
  - "Repository made public (user decision at checkpoint) so code scanning is available without GHAS; history secret-scanned clean first"
  - "checkout_path does NOT rewrite SARIF artifact URIs; CI prefixes every artifactLocation.uri with the fixture path via jq instead (checkout_path removed)"
  - "SARIF URIs are relative to the analysed directory; users analysing a subdirectory must prefix URIs or run from repo root; Phase 7 CI recipes must handle this"
metrics:
  duration: ~15min (excl. checkpoint wait)
  completed: 2026-10-01
  tasks: 3
  files: 7
---

# Phase 6 Plan 05: SARIF CI Acceptance Proof Summary

CI validates real gruntled SARIF against the vendored OASIS 2.1.0 schema in the `check` job, and a master-only job uploads fixture SARIF via SHA-pinned `upload-sarif`; GitHub accepted it with GRT001/GRT002/GRT003 alerts on the correct fixture files and lines.

## Tasks

| Task | Name | Commit |
| ---- | ---- | ------ |
| 1 | Vendor schema, create fixture, add schema-validation step | 05aa8c0 |
| 2 | Master-only sarif-upload job | 2777ba4 |
| 3 | Confirm upload-sarif run on master (checkpoint, approved) | db01fe8 (fix) |

## Evidence

- Final green run: https://github.com/GiulioSavini/gruntled/actions/runs/36846222629 (check incl. sarif-schema, architecture, sarif-upload: all success)
- Code scanning alerts, category `gruntled-fixture`, all open:
  - #4 GRT002 `cmd/gruntled/testdata/sarif-fixture/live/app/terragrunt.hcl:6`
  - #5 GRT001 `cmd/gruntled/testdata/sarif-fixture/live/app/terragrunt.hcl:10`
  - #6 GRT003 `cmd/gruntled/testdata/sarif-fixture/live/ring/a/terragrunt.hcl:2`
- First run https://github.com/GiulioSavini/gruntled/actions/runs/36845887482: green, upload accepted, but alerts #1-3 landed on `live/app/terragrunt.hcl` (repo-root relative). Auto-closed as fixed after the second run.

## Deviations from Plan

### Auto-fixed Issues

**1. [Rule 1 - Bug] SARIF alerts attached to wrong paths**
- **Found during:** Task 3 (first upload run)
- **Issue:** `checkout_path: cmd/gruntled/testdata/sarif-fixture` does not rewrite `artifactLocation.uri`; GitHub resolved `live/app/terragrunt.hcl` against the repo root.
- **Fix:** removed `checkout_path`; CI rewrites every `artifactLocation.uri` with jq prefix `cmd/gruntled/testdata/sarif-fixture/` into `upload.sarif` before upload. actionlint v1.7.12 clean.
- **Files modified:** .github/workflows/ci.yml
- **Commit:** db01fe8
- Note: plan must_have "checkout_path set to the fixture" is superseded by this fix.

### Checkpoint decision

- Repository was private with code scanning off (upload 403). User chose to make the repo public (`gh repo edit --visibility public`) after a clean secret scan of history.

## Consequence for Phase 7

SARIF URIs are relative to the analysed directory. Users who run `gruntled check` on a subdirectory must prefix URIs (or run from the repo root); `checkout_path` alone is not enough. Phase 7 CI recipes must cover this.

## Self-Check: PASSED
