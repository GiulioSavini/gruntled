---
phase: 03-grt001-diagnostic-cli
plan: 04
subsystem: docs
tags: [cli, exit-codes, json-schema, mock_outputs, diag-03]

requires:
  - phase: 03-grt001-diagnostic-cli
    provides: "CLI contract fixed in 03-02 (presenters) and 03-03 (usage text, exit codes)"
provides:
  - "docs/cli.md: usage, flag placement, exit codes, text and JSON formats, JSON schema v1, DIAG-03 rule, guarantees, limitations"
  - "PROJECT.md Key Decisions rows for DIAG-03 and stdlib flag; corrected tech stack line"
affects: [03-03, 03-05, README]

tech-stack:
  added: []
  patterns:
    - "docs/cli.md exit-code lines are the byte-exact source that TestHelpMatchesDocs (03-05) compares against `gruntled check -h`"

key-files:
  created: [docs/cli.md]
  modified: [.planning/PROJECT.md]

key-decisions:
  - "Text/JSON examples use the locked 03-02 line format (`path:line:col: CODE message (unit U)`), not the older research-spike format with a severity word"
  - "JSON example uses a remote-source unit to show unknown_units while keeping unknown_modules empty (a module-unknown unit has no Module in the graph)"
  - "Exit-code table row 3 also lists a failed stdout write (research Pattern 7); the fenced block keeps the exact -h wording"

requirements-completed: [CLI-02, DIAG-03]

duration: 12min
completed: 2026-09-28
---

# Phase 3 Plan 04: CLI reference and decision records Summary

**docs/cli.md documents `gruntled check`, its four exit-code lines byte-identical to the 03-03 usage text, JSON schema v1, and the DIAG-03 rule (mock_outputs never suppresses or downgrades GRT001); PROJECT.md records that rule and the move from cobra to stdlib flag.**

## Performance

- **Duration:** about 12 min
- **Completed:** 2026-09-28
- **Tasks:** 2/2
- **Files modified:** 2

## Accomplishments

- docs/cli.md has seven sections: usage (flags before or after the path, `--`, one path only), exit codes (table plus the verbatim `-h` block), output formats (text with the stderr summary, JSON field table and example), diagnostics (GRT001, GRT100, reserved codes), the DIAG-03 decision table with suffix conditions, strategy-over-bool precedence and the two rejected alternatives, guarantees (determinism, no net/exec across all five release targets, no writes via `root.FS()`, no cache or telemetry), and known limitations.
- Checked separately: each of the four `^  [0-3]  ` lines in the 03-03 checkUsage block appears as an exact line in docs/cli.md.
- The GRT001 example position (7:17) was recounted against the fixture: `inputs = { id = ` is 16 bytes.
- PROJECT.md: tech stack line now reads Go 1.27 with stdlib `flag` and the reason. Two Key Decisions rows were added, both marked `✓ Locked (Phase 3)`, and the footer was updated. Nothing else changed.

## Task Commits

1. **Task 1: docs/cli.md** - `ae5567d` (docs)
2. **Task 2: PROJECT.md Key Decisions and stack line** - `ae20a17` (docs)

## Files Created/Modified

- `docs/cli.md` - CLI reference (new)
- `.planning/PROJECT.md` - stack line, two Key Decisions rows, footer

## Deviations from Plan

- docs/cli.md is 282 lines, against the plan's "roughly 250". The full JSON example accounts for the difference. The plan only asked for "one example of each" format, and a complete JSON document was clearer than a trimmed one.
- The Key Decisions row names are plain text (no backticks) so the plan's literal verify greps (`mock_outputs never suppresses GRT001`, `stdlib flag instead of cobra`) match.

Otherwise the plan was executed as written.

## Issues Encountered

None.

## Verification

- Both task verify commands pass (run against the worktree path).
- Local suite with GOTOOLCHAIN=go1.27.0: `gofmt -l .` is empty; `go mod tidy -diff`, `go vet ./...` and `go test -count=1 ./...` pass; `scripts/check-architecture.sh` reports OK and `scripts/test-check-architecture.sh` reports all self-tests passed.

## Next Phase Readiness

- 03-03 must print the checkUsage text exactly as given in its plan. 03-05's TestHelpMatchesDocs will then match docs/cli.md.
- After 03-03 lands, check the JSON example in docs/cli.md against real output (field order follows the 03-02 struct order).

## Self-Check: PASSED

- FOUND: docs/cli.md
- FOUND: .planning/PROJECT.md edits
- FOUND: ae5567d, ae20a17
