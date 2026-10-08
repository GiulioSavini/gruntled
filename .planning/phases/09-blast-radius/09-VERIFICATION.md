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

## Security

Audit by `proj-sec:auditor` on 2026-10-08, diff `48a41d4..b96c4c8`. Verdict: fix-small.

| # | Severity | Location | Problem | Status |
|---|----------|----------|---------|--------|
| 1 | medium | `internal/domain/impact/impact.go` `NewFindings` | The baseline was treated as a set. A second identical GRT001 added on another line in the same unit and file was hidden, and `blast` exited 0. | Fixed in `fix(09-sec)`: the baseline is now a multiset. Regression test `TestNewFindingsCountsDuplicates`. |
| 2 | low | `internal/interfaces/presenter/blast.go:95-105,135-147` | Text output prints variable and output names, paths and GRT100 messages raw. `tfsurface` does not validate labels, so a hostile repo can inject terminal escape sequences. JSON output is safe. The `check` text presenter has the same exposure. | Open. Quote with `strconv.Quote`, or reject labels that `hclsyntax.ValidIdentifier` refuses. Fix across presenters in one go. |
| 3 | low | `cmd/gruntled/main.go` runBlast → `BlastText(..., *base)` | The text header echoes the `--base` argument verbatim, so an absolute path appears on stdout. | Open. Print a fixed label or `filepath.Base`. |
| info | — | `impact.FindingKey` | A finding that goes from warning to error is not counted as new. This is the documented design. | Revisit if exit codes should count it. |

Checked and OK:
- Both trees are opened with `os.OpenRoot`, so neither symlinks nor `..` escape the root.
- Every error path exits 3, with no silent fallback to "no baseline".
- No new imports or dependencies.
- Memory grows linearly.
- JSON output is deterministic.

Not run:
- `govulncheck`: go1.26 toolchain against go1.27 packages.
- `-race`: no C compiler.
