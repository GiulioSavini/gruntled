---
phase: 09-blast-radius
verified: 2026-10-08T00:00:00Z
status: passed
score: 4/4 must-haves verified
---

# Phase 9: Blast Radius Verification Report

**Phase Goal:** User can see which units a change breaks and which it puts at risk, against a baseline directory.
**Status:** passed. Initial verification, not a re-verification.

## Truths

| # | Truth | Status | Evidence |
|---|-------|--------|----------|
| 1 | `blast --base <dir> <path>` prints disjoint, sorted Broken and Impacted in text and json | VERIFIED | Manual run printed both formats. The json output has `broken`, `impacted` and `summary`. Script tests blast_impacted, blast_nobase and blast_exitcodes pass. |
| 2 | A finding new in path is Broken; a finding in both is never Broken | VERIFIED | Removing `output vpc_id` made live/app Broken (GRT001). Base vs base gave Broken (0), exit 0. The unit tests in domain/impact and the pre-existing-finding script test pass. |
| 3 | A direct consumer of a module whose variable/output names changed is Impacted; an unchanged surface impacts nothing | VERIFIED | live/network was Impacted (`modules/network: -output vpc_id`). Base vs base gave Impacted (0). |
| 4 | Without `--base`, only Broken is reported, labelled "no baseline" | VERIFIED | Output was `baseline: none (no baseline)` and Broken (1), with no Impacted section. Exit code 1. |

**Score:** 4/4.

## Evidence

- `go test -count=1 ./...` passed in every package, including cmd/gruntled, application/blasting, domain/impact and interfaces/presenter. I did not run -race or scripts/test-check-architecture.sh.
- `go test ./cmd/gruntled -run TestScripts/blast -v` passed for blast_nobase, blast_impacted and blast_exitcodes.
- Manual run on two copies of clean-fixture, with `modules/network/outputs.tf` emptied in cur:
  - `blast --base base cur` (text) printed `baseline: base`, then Broken (1) with live/app GRT001, then Impacted (1) with live/network. Exit 1.
  - `blast --base base cur --format json` returned valid JSON with `baseline: true`, one broken entry, one impacted entry (`removed_outputs: ["vpc_id"]`) and a summary of 1 broken and 1 impacted. Exit 0 in my shell, but the exit code was masked by a `||` fallback in my command. The text run and the no-base run show exit 1 for the same condition, and the script test blast_exitcodes passes.
  - `blast cur` (no base) printed `baseline: none (no baseline)` and Broken (1). Exit 1.
  - `blast --base base base` printed Broken (0) and Impacted (0). Exit 0.
  - `--json` is not a flag. It gave a usage error with exit 2, and the real flag is `--format json`.

## Requirements

| ID | Plans | Status |
|----|-------|--------|
| BLAST-01 | 09-01, 09-02, 09-03 | SATISFIED. Marked complete in REQUIREMENTS.md. 09-03-SUMMARY lists it as completed. |
| BLAST-02 | 09-01, 09-03 | SATISFIED. Marked complete in REQUIREMENTS.md. 09-01 and 09-03 SUMMARYs list it as completed. |

No orphaned requirements.

## Anti-patterns

None blocking. I did not run a stub scan beyond the behavioral checks above, which exercised real output.

## Human verification

None required.

_Verifier: Claude (gsd-verifier)_
