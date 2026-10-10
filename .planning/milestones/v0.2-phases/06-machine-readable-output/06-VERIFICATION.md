---
phase: 06-machine-readable-output
verified: 2026-10-01T00:00:00Z
status: passed
score: 4/4 must-haves verified
---

# Phase 6: Machine-readable output Verification Report

**Phase Goal:** Users and tooling can consume gruntled's graph and diagnostics as stable, deterministic documents, including one GitHub code scanning accepts.
**Status:** passed
**Re-verification:** No

## Observable Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `graph --json` prints one versioned document (units, modules, edges, unknown reasons), repo-relative, deterministic | VERIFIED | `internal/interfaces/presenter/graph.go` (graphSchemaVersion=1); `graph` subcommand in `cmd/gruntled/main.go`; golden testscripts under `cmd/gruntled/testdata/golden`; `go test -count=1 ./...` all ok |
| 2 | `check --format sarif` prints SARIF 2.1.0, one rule per GRT code, repo-relative locations | VERIFIED | `presenter/sarif.go` (version 2.1.0, rules GRT001/002/003/100); `--format text\|json\|sarif` in main.go; presenter and sarif tests pass |
| 3 | upload-sarif accepts the document | VERIFIED | CI run 36846222629 green (check, architecture, sarif-upload); `.github/workflows/ci.yml` has a sarif-schema job (vendored OASIS schema, jv) and an upload-sarif job; code-scanning alerts #4 GRT002, #5 GRT001, #6 GRT003 have repo-relative paths |
| 4 | check exit codes unchanged by sarif; graph writes nothing in repo | VERIFIED | Format switch in main.go only selects the presenter; usage and exit-code docs for graph; e2e/main tests pass |

## Requirements Coverage

| Requirement | Source Plan | Status | Evidence |
|---|---|---|---|
| INT-01 | 06-01, 06-04 | SATISFIED | graph JSON document, golden tests |
| INT-02 | 06-01, 06-02, 06-03, 06-05 | SATISFIED | SARIF presenter, schema validation, upload CI |

No orphaned requirements: REQUIREMENTS.md maps only INT-01 and INT-02 to Phase 6, and both are claimed by plans. The checkbox and traceability rows in REQUIREMENTS.md are still unticked (INT-01 "In Progress", INT-02 "Pending") and need updating.

## Automated Checks

- `go test -count=1 ./...`: all packages ok (no -race, no gcc)
- `bash scripts/check-architecture.sh`: OK

## Anti-Patterns

None blocking found in targeted scan.

## Notes

- Code-scanning alerts #1-#3 carry fixture-relative paths from an earlier upload. The CI jq step now prefixes the fixture path (alerts #4-#6), so this is historical.
- Byte-identity across machines rests on golden tests plus the deterministic presenter. Only one CI platform was exercised.

## Human Verification

None required.

_Verifier: Claude (gsd-verifier)_
